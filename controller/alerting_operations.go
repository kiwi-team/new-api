package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type alertTargetTestCooldownError struct {
	RetryAfterSeconds int64
}

func (e *alertTargetTestCooldownError) Error() string {
	return "Please wait 60 seconds between notification tests."
}

func GetAlertTarget(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var row model.AlertTarget
	if err := model.DB.WithContext(c.Request.Context()).First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	if !alerting.CanRead(alertActor(c), row.OwnerUserID) {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	alertOK(c, row)
}

func TestAlertTarget(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	actor := alertActor(c)
	var row model.AlertTarget
	if err := model.DB.WithContext(c.Request.Context()).First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	if row.OwnerUserID != actor.ID && !actor.Root() {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	if !common.AlertingEnabled || !row.Enabled || service.AlertTargetVault == nil {
		alertError(c, alerting.ErrTargetRejected)
		return
	}
	now := time.Now().UnixMilli()
	var event model.AlertEvent
	err := model.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AlertTarget{}).Where("id = ? AND revision = ? AND last_test_at < ?", row.ID, row.Revision, now-60000).Updates(map[string]any{"last_test_at": now, "last_test_status": "queued"})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var current model.AlertTarget
			if err := tx.Select("id", "revision", "last_test_at").First(&current, row.ID).Error; err != nil {
				return err
			}
			if current.Revision == row.Revision && current.LastTestAt >= now-60000 {
				return &alertTargetTestCooldownError{RetryAfterSeconds: max(1, (current.LastTestAt+60000-now+999)/1000)}
			}
			return model.ErrAlertConflict
		}
		event = model.AlertEvent{DedupKey: model.AlertIdentity("target-test", row.ID, now), OwnerUserID: row.OwnerUserID, Kind: "test", Severity: "P3", TargetName: row.Name, Summary: "Alert delivery test", CreatedAt: now}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		job := model.AlertJob{DedupKey: model.AlertIdentity("target-test-job", event.DedupKey), EventID: event.ID, Kind: "notify", TargetName: row.Name, TargetType: row.Type, TargetHint: row.AddressHint, TargetID: row.ID, TargetRevision: row.Revision, Priority: 3, Status: "pending", AvailableAt: now, ExpiresAt: now + 3600000, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: row.OwnerUserID, Operation: "target.test", ObjectID: row.ID, Revision: row.Revision, CreatedAt: now}).Error
	})
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, event)
}

func GetAlertIncident(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var row model.AlertIncident
	if err := model.DB.WithContext(c.Request.Context()).First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	if !alerting.CanRead(alertActor(c), row.OwnerUserID) {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	var events []model.AlertEvent
	if err := model.DB.WithContext(c.Request.Context()).Where("incident_id = ?", id).Order("id desc").Limit(100).Find(&events).Error; err != nil {
		alertError(c, err)
		return
	}
	var holds []model.AlertDisableHold
	if err := model.DB.WithContext(c.Request.Context()).Where("incident_id = ?", id).Find(&holds).Error; err != nil {
		alertError(c, err)
		return
	}
	canManage := false
	if rule, err := alerting.VisibleRule(c.Request.Context(), alertActor(c), row.RuleID, false); err == nil {
		canManage = alerting.CanManage(alertActor(c), rule)
	}
	alertOK(c, gin.H{"incident": row, "events": events, "holds": holds, "can_manage": canManage})
}

func RetryAlertJob(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var job model.AlertJob
	if err := model.DB.WithContext(c.Request.Context()).First(&job, id).Error; err != nil {
		alertError(c, err)
		return
	}
	var event model.AlertEvent
	if err := model.DB.WithContext(c.Request.Context()).First(&event, job.EventID).Error; err != nil {
		alertError(c, err)
		return
	}
	actor := alertActor(c)
	var retryRule model.AlertRule
	if event.RuleID > 0 {
		var err error
		retryRule, err = alerting.VisibleRule(c.Request.Context(), actor, event.RuleID, true)
		if err != nil {
			alertError(c, err)
			return
		}
	} else if actor.ID != event.OwnerUserID && !actor.Root() {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	if job.Kind != "notify" && job.Kind != "action" || job.Status != "failed" && job.Status != "blocked" {
		alertError(c, alerting.ErrInvalid)
		return
	}
	var input struct {
		Preview        bool  `json:"preview"`
		TargetID       int64 `json:"target_id"`
		TargetRevision int64 `json:"target_revision"`
	}
	if c.Request.ContentLength != 0 && c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	operation := service.AlertManageContext{RuleID: job.RuleID, OwnerUserID: event.OwnerUserID, ExpectedRevision: job.RuleRevision, ConfigHash: model.AlertIdentity("job-retry", job.ID, job.Status, job.UpdatedAt, job.Payload), Operation: "save"}
	var target model.AlertTarget
	if job.Kind == "notify" {
		id := job.TargetID
		if input.TargetID > 0 {
			id = input.TargetID
		}
		if event.RuleID > 0 {
			spec, err := model.AlertRuleConfig(retryRule)
			if err != nil {
				alertError(c, err)
				return
			}
			if !model.AlertRuleAllowsNotification(spec, id) {
				alertError(c, alerting.ErrForbidden)
				return
			}
		}
		if err := model.DB.WithContext(c.Request.Context()).First(&target, id).Error; err != nil {
			alertError(c, err)
			return
		}
		if !target.Enabled || target.OwnerUserID != event.OwnerUserID && !(actor.Root() && target.OwnerUserID == 0) {
			alertError(c, alerting.ErrTargetRejected)
			return
		}
	}
	if input.Preview {
		alertOK(c, gin.H{"target": target, "verification_context": operation})
		return
	}
	if job.Kind == "action" {
		encoded, err := common.Marshal(operation)
		if err != nil {
			alertError(c, err)
			return
		}
		if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAlertManage, Context: encoded}) == nil {
			return
		}
	} else if target.Revision != job.TargetRevision && input.TargetRevision != target.Revision || input.TargetID > 0 && input.TargetRevision != target.Revision {
		alertError(c, model.ErrAlertConflict)
		return
	}
	now := time.Now().UnixMilli()
	err := model.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := model.LockAlertEvent(tx, job.EventID); err != nil {
			return err
		}
		if job.Kind == "notify" && (target.ID != job.TargetID || target.Revision != job.TargetRevision) {
			// Explicitly retarget into a new delivery identity. Keep the original
			// failure and its attempts immutable for investigation.
			replay := job
			replay.ID, replay.Attempts = 0, 0
			replay.DedupKey = model.AlertIdentity("manual-target-retry", job.ID, target.ID, target.Revision)
			replay.TargetID, replay.TargetRevision = target.ID, target.Revision
			replay.TargetName, replay.TargetType, replay.TargetHint = target.Name, target.Type, target.AddressHint
			replay.Status, replay.LastError, replay.LockedBy = "pending", "", ""
			replay.LockedUntil, replay.AcceptedAt = 0, 0
			replay.RenderedContent, replay.RenderedLanguage = "", ""
			replay.CreatedAt, replay.UpdatedAt, replay.AvailableAt, replay.ExpiresAt = now, now, now, now+86400000
			if err := tx.Create(&replay).Error; err != nil {
				return err
			}
			return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: event.OwnerUserID, Operation: "job.retarget", ObjectID: replay.ID, Revision: target.Revision, CreatedAt: now}).Error
		}
		result := tx.Model(&model.AlertJob{}).Where("id = ? AND status = ? AND updated_at = ?", id, job.Status, job.UpdatedAt).Updates(map[string]any{"status": "pending", "available_at": now, "expires_at": now + 86400000, "updated_at": now, "locked_by": "", "locked_until": 0})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrAlertConflict
		}
		return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: event.OwnerUserID, Operation: "job.retry", ObjectID: id, CreatedAt: now}).Error
	})
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}

func ListAlertHolds(c *gin.Context) {
	query, err := alertOwnerFilter(c, model.DB.WithContext(c.Request.Context()).Model(&model.AlertDisableHold{}))
	if err != nil {
		alertError(c, err)
		return
	}
	for _, column := range []string{"channel_id", "token_id"} {
		if raw := c.Query(column); raw != "" {
			id, err := strconv.Atoi(raw)
			if err != nil || id <= 0 {
				alertError(c, alerting.ErrInvalid)
				return
			}
			query = query.Where(column+" = ?", id)
		}
	}
	var rows []model.AlertDisableHold
	if err = query.Where("active = ?", true).Order("id desc").Limit(alertPageSize(c, 200)).Find(&rows).Error; err != nil {
		alertError(c, err)
		return
	}
	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]any{"hold": row, "verification_context": service.AlertManageContext{RuleID: row.RuleID, OwnerUserID: row.OwnerUserID, ConfigHash: model.AlertIdentity("hold", row.ID, row.ConfigVersion, row.CreatedAt), Operation: "release"}})
	}
	alertOK(c, data)
}

func ReleaseAlertHold(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	actor := alertActor(c)
	var row model.AlertDisableHold
	if err := model.DB.WithContext(c.Request.Context()).First(&row, id).Error; err != nil {
		alertError(c, err)
		return
	}
	if !actor.Root() && (row.OwnerUserID != actor.ID || row.Platform) {
		alertError(c, gorm.ErrRecordNotFound)
		return
	}
	operation := service.AlertManageContext{RuleID: row.RuleID, OwnerUserID: row.OwnerUserID, ConfigHash: model.AlertIdentity("hold", row.ID, row.ConfigVersion, row.CreatedAt), Operation: "release"}
	encoded, err := common.Marshal(operation)
	if err != nil {
		alertError(c, err)
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAlertManage, Context: encoded}) == nil {
		return
	}
	hold, err := model.ReleaseAlertHold(c.Request.Context(), id, actor.ID, actor.Root())
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, hold)
}

func SetAlertChannelPolicy(c *gin.Context) {
	actor := alertActor(c)
	if !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var input struct {
		Mode             string `json:"protection_mode"`
		ExpectedRevision int64  `json:"expected_revision"`
		Preview          bool   `json:"preview"`
	}
	if c.ShouldBindJSON(&input) != nil || input.Mode != "legacy" && input.Mode != "scoped" {
		alertError(c, alerting.ErrInvalid)
		return
	}
	operation := service.AlertManageContext{OwnerUserID: 0, ExpectedRevision: input.ExpectedRevision, ConfigHash: model.AlertIdentity("channel-policy", id, input.Mode, input.ExpectedRevision), Operation: "save"}
	if input.Preview {
		alertOK(c, gin.H{"verification_context": operation})
		return
	}
	encoded, err := common.Marshal(operation)
	if err != nil {
		alertError(c, err)
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAlertManage, Context: encoded}) == nil {
		return
	}
	if err = model.SetAlertChannelPolicy(c.Request.Context(), int(id), actor.ID, input.Mode, input.ExpectedRevision); err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}

func AlertActionImpact(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	row, err := alerting.VisibleRule(c.Request.Context(), alertActor(c), id, true)
	if err != nil {
		alertError(c, err)
		return
	}
	spec, err := model.AlertRuleConfig(row)
	if err != nil {
		alertError(c, err)
		return
	}
	targets, err := alerting.ResolveTargets(c.Request.Context(), row, spec)
	if err != nil {
		alertError(c, err)
		return
	}
	data := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		if target.Type != "channel" && target.Type != "channel_model" {
			continue
		}
		impact, err := alerting.ChannelActionImpact(model.DB.WithContext(c.Request.Context()), target, spec)
		if err != nil {
			alertError(c, err)
			return
		}
		data = append(data, map[string]any{"target": target, "impact": impact})
	}
	alertOK(c, data)
}

func GetAlertChannelModelControls(c *gin.Context) {
	if !alertActor(c).Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var channel model.Channel
	if err := model.DB.WithContext(c.Request.Context()).First(&channel, id).Error; err != nil {
		alertError(c, err)
		return
	}
	var controls []model.AlertChannelModelControl
	if err := model.DB.WithContext(c.Request.Context()).Where("channel_id = ?", id).Limit(501).Find(&controls).Error; err != nil {
		alertError(c, err)
		return
	}
	var holds []model.AlertDisableHold
	if err := model.DB.WithContext(c.Request.Context()).Where("channel_id = ? AND active = ?", id, true).Limit(501).Find(&holds).Error; err != nil {
		alertError(c, err)
		return
	}
	_, _, version := model.ResolveChannelModelIdentity(&channel, "")
	alertOK(c, gin.H{"name": channel.Name, "models": channel.GetModels(), "config_version": version, "controls": controls, "holds": holds})
}

func SetAlertChannelModelControl(c *gin.Context) {
	actor := alertActor(c)
	if !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	var input struct {
		Model            string `json:"model"`
		Paused           *bool  `json:"paused"`
		ExpectedRevision int64  `json:"expected_revision"`
		ConfigVersion    string `json:"config_version"`
		Preview          bool   `json:"preview"`
	}
	if c.ShouldBindJSON(&input) != nil || input.Paused == nil || len(input.Model) == 0 || len(input.Model) > 4096 || input.ConfigVersion == "" {
		alertError(c, alerting.ErrInvalid)
		return
	}
	proof := service.AlertManageContext{OwnerUserID: 0, ExpectedRevision: input.ExpectedRevision, ConfigHash: model.AlertIdentity("manual-model-control", id, input.Model, *input.Paused, input.ConfigVersion), Operation: "save"}
	if input.Preview {
		alertOK(c, gin.H{"verification_context": proof})
		return
	}
	encoded, err := common.Marshal(proof)
	if err != nil {
		alertError(c, err)
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeAlertManage, Context: encoded}) == nil {
		return
	}
	if err = model.SetManualAlertModelControl(c.Request.Context(), int(id), actor.ID, input.Model, *input.Paused, input.ExpectedRevision, input.ConfigVersion); err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}
