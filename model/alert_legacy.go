package model

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LegacyUserQuotaMilestone struct {
	Initialized      bool  `json:"initialized"`
	BaselineAt       int64 `json:"baseline_at"`
	WindowStart      int64 `json:"window_start"`
	WindowQuota      int64 `json:"window_quota"`
	AccumulatedQuota int64 `json:"accumulated_quota"`
	MilestoneUSD     int64 `json:"milestone_usd"`
}

type LegacyKeyQuotaRule struct {
	QuotaUSD float64 `json:"quota_usd"`
	Action   string  `json:"action"`
}

type LegacyKeyQuotaUserConfig struct {
	FeishuRobotURL string               `json:"feishu_robot_url"`
	OneHour        []LegacyKeyQuotaRule `json:"one_hour"`
	OneDay         []LegacyKeyQuotaRule `json:"one_day"`
}

func LegacyKeyQuotaObject(owner int, period string, rule LegacyKeyQuotaRule) string {
	return strconv.Itoa(owner) + ":" + AlertIdentity(period, decimal.NewFromFloat(rule.QuotaUSD).String(), rule.Action)
}

// DisableLegacyQuotaToken changes only credential status. Never save a stale
// token snapshot over concurrent quota, permission or name changes.
func DisableLegacyQuotaToken(ctx context.Context, id int) error {
	var token Token
	if err := DB.WithContext(ctx).Select([]string{"id", "key"}).First(&token, id).Error; err != nil {
		return err
	}
	if err := invalidateTokenCacheForMutation(token.Key); err != nil {
		common.SysError(common.LogText("failed to invalidate token cache before quota protection"))
	}
	return DB.WithContext(ctx).Model(&Token{}).Where("id = ? AND status <> ?", id, common.TokenStatusDisabled).Updates(map[string]any{
		"status": common.TokenStatusDisabled, "auth_version": gorm.Expr("auth_version + 1"),
	}).Error
}

// A migrated owner's old settings become read-only, including before a
// scheduled cutover. Otherwise changing the legacy identity would run both
// executors after the boundary. Unrelated owners can still use the old form.
func ValidateLegacyKeyQuotaEdit(ctx context.Context, raw string) error {
	var migrations []AlertMigration
	if err := DB.WithContext(ctx).Where("kind IN ? AND executor = ?", []string{"key_hour", "key_day"}, "unified").Limit(10001).Find(&migrations).Error; err != nil {
		return err
	}
	if len(migrations) > 10000 {
		return ErrAlertConflict
	}
	if len(migrations) == 0 {
		return nil
	}
	var option Option
	if err := DB.WithContext(ctx).Where(map[string]any{"key": "key_quota_warning_config"}).First(&option).Error; err != nil {
		return err
	}
	var old, next map[string]*LegacyKeyQuotaUserConfig
	if common.UnmarshalJsonStr(option.Value, &old) != nil || common.UnmarshalJsonStr(raw, &next) != nil {
		return ErrAlertConflict
	}
	owners := make([]int, 0, len(migrations))
	for _, migration := range migrations {
		owners = append(owners, migration.OwnerUserID)
	}
	slices.Sort(owners)
	for _, owner := range slices.Compact(owners) {
		before, _ := common.Marshal(old[strconv.Itoa(owner)])
		after, _ := common.Marshal(next[strconv.Itoa(owner)])
		if string(before) != string(after) {
			return ErrAlertConflict
		}
	}
	return nil
}

func WithLegacyNoticeLease(ctx context.Context, kind, object string, work func(context.Context) error) error {
	key := kind + ":" + object
	if object == "site" {
		key = "site"
	}
	return WithLegacyAlertLease(ctx, "notice_adapter", key, work)
}

func IsLegacyNoticeOption(key string) bool {
	return slices.Contains([]string{"uid_quota_warning_robot", "uid_quota_warning_email", "feishu_qianfei_webhook_url", "feishu_qianfei_secret", "slow_error_feishu_webhook_url", "slow_error_feishu_secret", "slow_error_threshold_seconds", "ErrorWarningEnvName"}, key)
}

// Legacy event adapters retain the old trigger configuration. Redirect edits
// to the unified rule/target after takeover instead of keeping two authorities.
func ValidateLegacyNoticeEdit(ctx context.Context, values map[string]string) error {
	var rows []AlertMigration
	if err := DB.WithContext(ctx).Where("kind IN ? AND executor = ?", []string{"uid_day", "uid_budget_notice", "channel_arrears", "slow_error"}, "unified").Find(&rows).Error; err != nil {
		return err
	}
	protected := make(map[string]bool)
	for _, row := range rows {
		switch row.Kind {
		case "uid_day", "uid_budget_notice":
			protected["uid_quota_warning_robot"] = true
			if row.Kind == "uid_budget_notice" {
				protected["uid_quota_warning_email"] = true
			}
		case "channel_arrears":
			protected["feishu_qianfei_webhook_url"], protected["feishu_qianfei_secret"] = true, true
		case "slow_error":
			for _, key := range []string{"slow_error_feishu_webhook_url", "slow_error_feishu_secret", "slow_error_threshold_seconds", "ErrorWarningEnvName"} {
				protected[key] = true
			}
		}
	}
	for key, value := range values {
		if !protected[key] {
			continue
		}
		var option Option
		if err := DB.WithContext(ctx).Where(map[string]any{"key": key}).First(&option).Error; err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if option.Value != value {
			return ErrAlertConflict
		}
	}
	return nil
}

func LockAlertImportOwner(tx *gorm.DB, owner, additionalTargets int) error {
	if err := LockAlertOwner(tx, owner); err != nil {
		return err
	}
	var rules, targets int64
	if err := tx.Model(&AlertRule{}).Where("owner_user_id = ?", owner).Count(&rules).Error; err != nil {
		return err
	}
	if err := tx.Model(&AlertTarget{}).Where("owner_user_id = ?", owner).Count(&targets).Error; err != nil {
		return err
	}
	if rules >= int64(AlertRuleOwnerLimit()) || targets+int64(additionalTargets) > int64(AlertTargetOwnerLimit()) {
		return ErrAlertConflict
	}
	return nil
}

// LegacyQuotaAlertOptions reads primary state while the caller holds the
// quota-options lease. An old process-local map must not overwrite a cutover
// checkpoint saved by another node.
func LegacyQuotaAlertOptions(ctx context.Context) (map[string]string, error) {
	keys := []string{"quota_alert_user_last_day_summary_map", "quota_alert_user_cumulative_milestone", "quota_alert_uid_day_map", "quota_alert_uid_month_threshold_map", "quota_alert_uid_project_threshold_map", "uid_quota_warning_robot", "uid_quota_warning_email", "key_quota_warning_config", "key_quota_warning_alerted"}
	var rows []Option
	if err := DB.WithContext(ctx).Where(map[string]any{"key": keys}).Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values, nil
}

func SaveLegacyAlertOption(ctx context.Context, key, value string) error {
	row := Option{Key: key, Value: value}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&row).Error; err != nil {
		return err
	}
	return updateOptionMap(key, value)
}

func LockLegacyAlertOption(tx *gorm.DB, key string) (Option, error) {
	var option Option
	err := lockForUpdate(tx).Where(map[string]any{"key": key}).First(&option).Error
	return option, err
}

func UnifiedLegacyQuotaAlerts(ctx context.Context, kinds []string) (map[string]bool, error) {
	var rows []AlertMigration
	if err := DB.WithContext(ctx).Select("id").Where("kind IN ? AND executor = ? AND (cutover_at = 0 OR cutover_at <= ?)", kinds, "unified", time.Now().UnixMilli()).Limit(10001).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 10000 {
		return nil, ErrAlertConflict
	}
	items := make(map[string]bool, len(rows))
	for _, row := range rows {
		items[row.ID] = true
	}
	return items, nil
}

// DisableLegacyAlertChannel serializes a legacy failure with a protection-mode
// change. The policy check and status write must share the channel row lock;
// checking in service before UpdateChannelStatus leaves a widening race.
func DisableLegacyAlertChannel(ctx context.Context, channelID int, usingKey, reason string) bool {
	if !common.AlertingControlsEnabled {
		return UpdateChannelStatus(channelID, usingKey, common.ChannelStatusAutoDisabled, reason)
	}
	polling := GetChannelPollingLock(channelID)
	polling.Lock()
	defer polling.Unlock()
	var channel Channel
	changed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		var policy AlertChannelPolicy
		if err := tx.First(&policy, channelID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if policy.ProtectionMode == "scoped" || !channel.GetAutoBan() || channel.Status == common.ChannelStatusManuallyDisabled || channel.Status == common.ChannelStatusAutoDisabled {
			return nil
		}
		if channel.ChannelInfo.IsMultiKey {
			handlerMultiKeyUpdate(&channel, usingKey, common.ChannelStatusAutoDisabled, reason)
		} else {
			info := channel.GetOtherInfo()
			info["status_reason"], info["status_time"] = reason, common.GetTimestamp()
			channel.SetOtherInfo(info)
			channel.Status = common.ChannelStatusAutoDisabled
		}
		updates := map[string]any{"status": channel.Status, "other_info": channel.OtherInfo}
		if channel.ChannelInfo.IsMultiKey {
			updates["channel_info"] = channel.ChannelInfo
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).Updates(updates).Error; err != nil {
			return err
		}
		changed = true
		return tx.Model(&Ability{}).Where("channel_id = ?", channelID).Update("enabled", channel.Status == common.ChannelStatusEnabled).Error
	})
	if err != nil || !changed {
		return false
	}
	if common.MemoryCacheEnabled {
		CacheUpdateChannel(&channel)
	}
	return true
}

// CommitScopedLegacyProtection preserves the existing single-error protection
// after Root explicitly changes its scope. It never edits channel-wide status.
func CommitScopedLegacyProtection(ctx context.Context, channelID int, requested string) (*AlertDisableHold, error) {
	var hold *AlertDisableHold
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		var policy AlertChannelPolicy
		if err := lockForUpdate(tx).First(&policy, channelID).Error; err != nil {
			return err
		}
		if policy.ProtectionMode != "scoped" {
			return ErrAlertConflict
		}
		if !channel.GetAutoBan() {
			return nil
		}
		key, _, version := ResolveChannelModelIdentity(&channel, requested)
		targetKey := AlertIdentity("channel_model", channelID, "routing", key)
		var existing AlertDisableHold
		result := tx.Where("target_key = ? AND rule_id = 0 AND active = ?", targetKey, true).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			return nil
		}
		now := time.Now().UnixMilli()
		hold = &AlertDisableHold{DedupKey: AlertIdentity("legacy-scoped", targetKey, common.GetUUID()), TargetKey: targetKey, TargetType: "channel_model", TargetName: channel.Name + " / " + key, ChannelID: channelID, ModelKey: key, Active: true, Platform: true, ConfigVersion: version, Recovery: `{"mode":"manual"}`, CreatedAt: now}
		if err := tx.Create(hold).Error; err != nil {
			return err
		}
		return tx.Create(&AlertAudit{Operation: "legacy.scoped_hold", ObjectID: hold.ID, CreatedAt: now}).Error
	})
	return hold, err
}

// WithLegacyAlertLease is shared by the compatible old loop and the explicit
// cutover transaction. Bounded work finishes before the authority lease expires.
func WithLegacyAlertLease(ctx context.Context, kind, objectID string, work func(context.Context) error) error {
	if !common.AlertingControlsEnabled {
		return work(ctx)
	}
	owner := common.GetUUID()
	key := AlertIdentity("alert-legacy-lease", kind, objectID)
	acquired, err := AcquireResourceLease(ctx, key, owner, time.Now().Add(35*time.Second).Unix())
	if err != nil {
		return err
	}
	if !acquired {
		return ErrAlertConflict
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = ReleaseResourceLease(release, key, owner)
	}()
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return work(bounded)
}

func LegacyAlertUsesUnified(ctx context.Context, kind, objectID string) (bool, error) {
	if !common.AlertingControlsEnabled {
		return false, nil
	}
	var row AlertMigration
	found := DB.WithContext(ctx).Where("id = ?", AlertIdentity("legacy", kind, objectID)).Limit(1).Find(&row)
	return found.RowsAffected > 0 && row.Executor == "unified" && (row.CutoverAt == 0 || row.CutoverAt <= time.Now().UnixMilli()), found.Error
}

// WithAlertMigration serializes checkpoint transfer with condition evaluation.
// A consistent snapshot includes quota_data and its transactional alert bridge.
func WithAlertMigration(ctx context.Context, expected AlertMigration, apply func(*gorm.DB, *AlertRule, *AlertMigration) error) error {
	options := &sql.TxOptions{}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		options.Isolation = sql.LevelRepeatableRead
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule AlertRule
		if err := lockForUpdate(tx.Unscoped()).First(&rule, expected.RuleID).Error; err != nil {
			return err
		}
		var migration AlertMigration
		if err := lockForUpdate(tx).First(&migration, "id = ?", expected.ID).Error; err != nil {
			return err
		}
		if migration.Revision != expected.Revision || migration.Executor != expected.Executor {
			return ErrAlertConflict
		}
		return apply(tx, &rule, &migration)
	}, options)
}

// CommitAlertMigration advances the executor and condition generation together.
// The caller already holds WithAlertMigration and has transferred its specific
// checkpoint in this transaction. Historical events and active holds survive.
func CommitAlertMigration(tx *gorm.DB, rule *AlertRule, migration *AlertMigration, actor int, executor string, effectiveAt int64, checkpoint string) error {
	if executor != "legacy" && executor != "unified" {
		return ErrAlertConflict
	}
	now := time.Now().UnixMilli()
	rule.Generation++
	rule.Revision++
	rule.EffectiveAt, rule.UpdatedAt, rule.NextEvaluationAt = effectiveAt, now, now
	rule.UpdatedBy, rule.EvaluationCursor = actor, ""
	if err := tx.Unscoped().Model(rule).Select("generation", "revision", "effective_at", "updated_at", "updated_by", "next_evaluation_at", "evaluation_cursor").Updates(rule).Error; err != nil {
		return err
	}
	migration.Executor, migration.Status, migration.UpdatedAt, migration.UpdatedBy, migration.Checkpoint = executor, "active", now, actor, checkpoint
	migration.Revision++
	if err := tx.Save(migration).Error; err != nil {
		return err
	}
	if err := tx.Create(&AlertRuleRevision{RuleID: rule.ID, Revision: rule.Revision, Config: rule.Config, CreatedBy: actor, CreatedAt: now}).Error; err != nil {
		return err
	}
	operation := "migration.cutover"
	if executor == "legacy" {
		operation = "migration.rollback"
	}
	return tx.Create(&AlertAudit{ActorID: actor, OwnerUserID: rule.OwnerUserID, Operation: operation, ObjectID: rule.ID, Revision: rule.Revision, CreatedAt: now}).Error
}
