package controller

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"

	"github.com/gin-gonic/gin"
)

func ImportAlertNotification(c *gin.Context) {
	actor := alertActor(c)
	var input struct {
		OwnerUserID int  `json:"owner_user_id"`
		Feishu      bool `json:"feishu"`
	}
	if c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	if input.OwnerUserID == 0 {
		input.OwnerUserID = actor.ID
	}
	target, err := alerting.ImportLegacyNotification(c.Request.Context(), actor, service.AlertTargetVault, input.OwnerUserID, input.Feishu)
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, target)
}

func ListAlertMigrations(c *gin.Context) {
	if !alertActor(c).Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	var migrations []model.AlertMigration
	if err := model.DB.WithContext(c.Request.Context()).Order("updated_at desc").Limit(501).Find(&migrations).Error; err != nil {
		alertError(c, err)
		return
	}
	var tokens []struct {
		ID             int     `json:"id"`
		Name           string  `json:"name"`
		UserID         int     `json:"owner_user_id"`
		AlertThreshold float64 `json:"threshold_usd"`
		Configured     bool    `json:"configured" gorm:"-"`
	}
	if err := model.DB.WithContext(c.Request.Context()).Model(&model.Token{}).Select("id", "name", "user_id", "alert_threshold").Where("alert_threshold > 0").Order("id asc").Limit(501).Scan(&tokens).Error; err != nil {
		alertError(c, err)
		return
	}
	owners := make([]int, 0, len(tokens))
	for _, token := range tokens {
		owners = append(owners, token.UserID)
	}
	var tokenOwners []model.User
	if len(owners) > 0 {
		if err := model.DB.WithContext(c.Request.Context()).Select("id", "setting").Where("id IN ?", owners).Find(&tokenOwners).Error; err != nil {
			alertError(c, err)
			return
		}
	}
	configuredOwners := make(map[int]bool, len(tokenOwners))
	for _, owner := range tokenOwners {
		configuredOwners[owner.Id] = strings.TrimSpace(owner.GetSetting().WebhookUrl) != ""
	}
	for i := range tokens {
		tokens[i].Configured = configuredOwners[tokens[i].UserID]
	}
	options, err := model.LegacyQuotaAlertOptions(c.Request.Context())
	if err != nil {
		alertError(c, err)
		return
	}
	var states map[string]model.LegacyUserQuotaMilestone
	if raw := options["quota_alert_user_cumulative_milestone"]; raw != "" && common.UnmarshalJsonStr(raw, &states) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	ids := make([]int, 0, len(states))
	for text, state := range states {
		id, err := strconv.Atoi(text)
		if err == nil && id > 0 && state.Initialized {
			ids = append(ids, id)
		}
	}
	var reportDays map[string]int64
	if raw := options["quota_alert_user_last_day_summary_map"]; raw != "" && common.UnmarshalJsonStr(raw, &reportDays) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for text := range reportDays {
		id, err := strconv.Atoi(text)
		if err == nil && id > 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}

	var keyConfigs map[string]*model.LegacyKeyQuotaUserConfig
	if raw := options["key_quota_warning_config"]; raw != "" && common.UnmarshalJsonStr(raw, &keyConfigs) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	for text := range keyConfigs {
		id, err := strconv.Atoi(text)
		if err == nil && id > 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	sort.Ints(ids)

	var noticeUsers []model.User
	if err := model.DB.WithContext(c.Request.Context()).Select("id", "username", "display_name").Order("id asc").Limit(501).Find(&noticeUsers).Error; err != nil {
		alertError(c, err)
		return
	}
	for _, user := range noticeUsers {
		if !seen[user.Id] {
			ids = append(ids, user.Id)
			seen[user.Id] = true
		}
	}
	sort.Ints(ids)
	var users []model.User
	if len(ids) > 0 {
		if err := model.DB.WithContext(c.Request.Context()).Select("id", "username", "display_name", "email", "setting").Where("id IN ?", ids[:min(len(ids), 501)]).Order("id asc").Find(&users).Error; err != nil {
			alertError(c, err)
			return
		}
	}
	items := make([]gin.H, 0, len(users))
	for _, kind := range []string{"uid_day", "uid_budget_notice", "channel_arrears", "slow_error"} {
		snapshot, err := alerting.LegacyNoticeSnapshot(c.Request.Context(), kind, "site")
		if err != nil {
			alertError(c, err)
			return
		}
		items = append(items, gin.H{"kind": kind, "object_id": "site", "name": kind, "owner_user_id": 0, "threshold_usd": "", "configured": snapshot.DestinationPresent})
	}
	accountConfigured := strings.TrimSpace(options["uid_quota_warning_robot"]) != ""
	for _, user := range users {
		name := user.DisplayName
		if name == "" {
			name = user.Username
		}
		items = append(items, gin.H{"kind": "user_spend", "object_id": strconv.Itoa(user.Id), "name": name, "owner_user_id": user.Id, "threshold_usd": "1000", "configured": accountConfigured})
		if !states[strconv.Itoa(user.Id)].Initialized {
			items = items[:len(items)-1]
		}
		items = append(items, gin.H{"kind": "user_report", "object_id": strconv.Itoa(user.Id), "name": name, "owner_user_id": user.Id, "threshold_usd": "", "configured": accountConfigured})
		if config := keyConfigs[strconv.Itoa(user.Id)]; config != nil {
			unique := make(map[string]bool)
			for _, period := range []string{"hour", "day"} {
				rules := config.OneHour
				if period == "day" {
					rules = config.OneDay
				}
				for _, rule := range rules {
					if rule.QuotaUSD <= 0 || rule.Action != "warn" && rule.Action != "disable_key" {
						continue
					}
					object := model.LegacyKeyQuotaObject(user.Id, period, rule)
					if unique[object] {
						continue
					}
					unique[object] = true
					items = append(items, gin.H{"kind": "key_" + period, "object_id": object, "name": name + " / " + period + " / " + rule.Action, "owner_user_id": user.Id, "threshold_usd": strconv.FormatFloat(rule.QuotaUSD, 'f', -1, 64), "configured": strings.TrimSpace(config.FeishuRobotURL) != ""})
				}
			}
		}

	}

	for _, user := range users {
		name := user.DisplayName
		if name == "" {
			name = user.Username
		}
		setting := user.GetSetting()
		configured := false
		switch setting.NotifyType {
		case "", dto.NotifyTypeEmail:
			configured = strings.TrimSpace(setting.NotificationEmail) != "" || strings.TrimSpace(user.Email) != ""
		case dto.NotifyTypeWebhook:
			configured = strings.TrimSpace(setting.WebhookUrl) != ""
		case dto.NotifyTypeBark:
			configured = strings.TrimSpace(setting.BarkUrl) != ""
		case dto.NotifyTypeGotify:
			configured = strings.TrimSpace(setting.GotifyUrl) != "" && strings.TrimSpace(setting.GotifyToken) != ""
		}
		items = append(items, gin.H{"kind": "user_notifications", "object_id": strconv.Itoa(user.Id), "name": name, "owner_user_id": user.Id, "threshold_usd": "", "configured": configured})
	}
	alertOK(c, gin.H{"migrations": migrations[:min(len(migrations), 500)], "legacy_tokens": tokens[:min(len(tokens), 500)], "legacy_quota_items": items[:min(len(items), 1000)], "truncated": len(migrations) > 500 || len(tokens) > 500 || len(ids) > 500 || len(items) > 1000})
}

type alertMigrationInput struct {
	TokenID  int    `json:"token_id,omitempty"`
	Kind     string `json:"kind"`
	ObjectID string `json:"object_id"`
}

func ImportAlertMigration(c *gin.Context) {
	actor := alertActor(c)
	if !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	var input alertMigrationInput
	if c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	migration, err := importLegacyAlert(c.Request.Context(), actor, input)
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, migration)
}

// Each item keeps the existing import lease, transaction and stable identity.
// Partial failures are retryable; imports never switch the notification executor.
func ImportAlertMigrationsBatch(c *gin.Context) {
	actor := alertActor(c)
	if !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	var input struct {
		Items []alertMigrationInput `json:"items"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.Items) == 0 || len(input.Items) > 10 {
		alertError(c, alerting.ErrInvalid)
		return
	}
	if service.AlertTargetVault == nil {
		alertError(c, alerting.ErrTargetKeyUnavailable)
		return
	}
	results := make([]gin.H, 0, len(input.Items))
	for _, item := range input.Items {
		migration, err := importLegacyAlert(c.Request.Context(), actor, item)
		if err != nil {
			status, fields := alertErrorResponse(err)
			results = append(results, gin.H{"kind": item.Kind, "object_id": item.ObjectID, "success": false, "status": status, "error": fields})
			continue
		}
		results = append(results, gin.H{"kind": migration.Kind, "object_id": migration.ObjectID, "success": true, "migration": migration})
	}
	alertOK(c, gin.H{"items": results})
}

func importLegacyAlert(ctx context.Context, actor alerting.Actor, input alertMigrationInput) (model.AlertMigration, error) {
	if input.TokenID > 0 && (input.Kind == "" || input.Kind == "token_spend") {
		return alerting.ImportLegacyToken(ctx, actor, service.AlertTargetVault, input.TokenID)
	}
	if input.Kind == "token_spend" || input.Kind == "user_spend" || input.Kind == "user_report" {
		owner, parseErr := strconv.Atoi(input.ObjectID)
		if parseErr != nil || owner <= 0 {
			return model.AlertMigration{}, alerting.ErrInvalid
		}
		if input.Kind == "token_spend" {
			return alerting.ImportLegacyToken(ctx, actor, service.AlertTargetVault, owner)
		}
		if input.Kind == "user_spend" {
			return alerting.ImportLegacyUserSpend(ctx, actor, service.AlertTargetVault, owner)
		}
		return alerting.ImportLegacyUserReport(ctx, actor, service.AlertTargetVault, owner)
	}
	if input.Kind == "key_hour" || input.Kind == "key_day" {
		return alerting.ImportLegacyKey(ctx, actor, service.AlertTargetVault, input.Kind, input.ObjectID)
	}
	if alerting.IsLegacyNoticeKind(input.Kind) {
		return alerting.ImportLegacyNotice(ctx, actor, service.AlertTargetVault, input.Kind, input.ObjectID)
	}
	return model.AlertMigration{}, alerting.ErrInvalid
}

func SwitchAlertMigration(c *gin.Context) {
	actor := alertActor(c)
	if !actor.Root() {
		alertError(c, alerting.ErrForbidden)
		return
	}
	id, operation := c.Param("id"), c.Param("operation")
	if len(id) != 64 || operation != "cutover" && operation != "rollback" {
		alertError(c, alerting.ErrInvalid)
		return
	}
	var input struct {
		ExpectedRevision int64  `json:"expected_revision"`
		SnapshotHash     string `json:"snapshot_hash"`
		AcceptAmbiguous  bool   `json:"accept_ambiguous"`
		Preview          bool   `json:"preview"`
	}
	if c.ShouldBindJSON(&input) != nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	var migration model.AlertMigration
	if err := model.DB.WithContext(c.Request.Context()).First(&migration, "id = ?", id).Error; err != nil {
		alertError(c, err)
		return
	}
	if migration.Revision != input.ExpectedRevision {
		alertError(c, model.ErrAlertConflict)
		return
	}

	objectID, err := strconv.Atoi(migration.ObjectID)
	numeric := migration.Kind == "token_spend" || migration.Kind == "user_spend" || migration.Kind == "user_report"
	if numeric && err != nil || !numeric && migration.Kind != "key_hour" && migration.Kind != "key_day" && !alerting.IsLegacyNoticeKind(migration.Kind) {
		alertError(c, alerting.ErrInvalid)
		return
	}
	var snapshot any
	var snapshotHash string
	if migration.Kind == "token_spend" {
		value, e := alerting.LegacyTokenSnapshot(c.Request.Context(), objectID)
		snapshot, err, snapshotHash = value, e, value.SnapshotHash
	} else if migration.Kind == "user_spend" {
		value, e := alerting.LegacyUserSpendSnapshot(c.Request.Context(), objectID)
		snapshot, err, snapshotHash = value, e, value.SnapshotHash

	} else if migration.Kind == "key_hour" || migration.Kind == "key_day" {
		value, e := alerting.LegacyKeySnapshot(c.Request.Context(), migration.Kind, migration.ObjectID, time.Now())
		snapshot, err, snapshotHash = value, e, value.SnapshotHash

	} else if alerting.IsLegacyNoticeKind(migration.Kind) {
		value, e := alerting.LegacyNoticeSnapshot(c.Request.Context(), migration.Kind, migration.ObjectID)
		snapshot, err, snapshotHash = value, e, value.SnapshotHash
	} else {
		value, e := alerting.LegacyUserReportSnapshot(c.Request.Context(), objectID)
		snapshot, err, snapshotHash = value, e, value.SnapshotHash
	}
	if err != nil {
		alertError(c, err)
		return
	}
	var rule model.AlertRule
	if err := model.DB.WithContext(c.Request.Context()).Unscoped().First(&rule, migration.RuleID).Error; err != nil {
		alertError(c, err)
		return
	}
	proof := service.AlertManageContext{RuleID: migration.RuleID, OwnerUserID: migration.OwnerUserID, ExpectedRevision: migration.Revision, ConfigHash: model.AlertIdentity("migration", id, operation, snapshotHash, input.AcceptAmbiguous, rule.Revision, rule.Config), Operation: "save"}
	if input.Preview {
		alertOK(c, gin.H{"snapshot": snapshot, "verification_context": proof})
		return
	}
	if input.SnapshotHash != snapshotHash {
		alertError(c, model.ErrAlertConflict)
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
	if migration.Kind == "token_spend" {
		err = alerting.SwitchLegacyToken(c.Request.Context(), actor, id, input.ExpectedRevision, operation, input.SnapshotHash, input.AcceptAmbiguous, rule.Revision)
	} else if migration.Kind == "user_spend" {
		err = alerting.SwitchLegacyUserSpend(c.Request.Context(), actor, id, input.ExpectedRevision, operation, input.SnapshotHash, input.AcceptAmbiguous, rule.Revision)

	} else if migration.Kind == "key_hour" || migration.Kind == "key_day" {
		err = alerting.SwitchLegacyKey(c.Request.Context(), actor, id, input.ExpectedRevision, operation, input.SnapshotHash, input.AcceptAmbiguous, rule.Revision, time.Now())

	} else if alerting.IsLegacyNoticeKind(migration.Kind) {
		err = alerting.SwitchLegacyNotice(c.Request.Context(), actor, id, input.ExpectedRevision, operation, input.SnapshotHash, rule.Revision)
	} else {
		err = alerting.SwitchLegacyUserReport(c.Request.Context(), actor, id, input.ExpectedRevision, operation, input.SnapshotHash, input.AcceptAmbiguous, rule.Revision, time.Now())
	}
	if err != nil {
		alertError(c, err)
		return
	}
	alertOK(c, nil)
}
