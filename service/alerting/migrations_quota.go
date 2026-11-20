package alerting

import (
	"context"
	"database/sql"
	"math"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const legacyUserMilestoneOption = "quota_alert_user_cumulative_milestone"

type LegacyAccountPreview struct {
	OwnerUserID        int                            `json:"owner_user_id"`
	Name               string                         `json:"name"`
	LastReportDay      int64                          `json:"last_report_day"`
	Language           string                         `json:"language"`
	State              model.LegacyUserQuotaMilestone `json:"state"`
	UnnotifiedQuota    int64                          `json:"unnotified_quota,string"`
	AcceptedUsage      int64                          `json:"accepted_usage,string"`
	Ambiguous          bool                           `json:"ambiguous"`
	DestinationPresent bool                           `json:"destination_present"`
	SnapshotHash       string                         `json:"snapshot_hash"`
}

func LegacyUserSpendSnapshot(ctx context.Context, owner int) (LegacyAccountPreview, error) {
	var snapshot LegacyAccountPreview
	options := &sql.TxOptions{ReadOnly: true}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, _, err = legacyUserSpendSnapshot(tx, owner)
		return err
	}, options)
	return snapshot, err
}

func legacyUserSpendSnapshot(tx *gorm.DB, owner int) (LegacyAccountPreview, string, error) {
	var result LegacyAccountPreview
	var options []model.Option
	if err := tx.Where(map[string]any{"key": []string{legacyUserMilestoneOption, "uid_quota_warning_robot"}}).Find(&options).Error; err != nil {
		return result, "", err
	}
	var raw, address string
	for _, option := range options {
		if option.Key == legacyUserMilestoneOption {
			raw = option.Value
		} else {
			address = option.Value
		}
	}
	var states map[string]model.LegacyUserQuotaMilestone
	if common.UnmarshalJsonStr(raw, &states) != nil {
		return result, "", ErrInvalid
	}
	state := states[strconv.Itoa(owner)]
	if !state.Initialized || state.WindowStart <= 0 || state.MilestoneUSD < 0 {
		return result, "", ErrInvalid
	}
	var user model.User
	if err := tx.Select("id", "username", "display_name", "setting").First(&user, owner).Error; err != nil {
		return result, "", err
	}
	var window, accepted int64
	if err := tx.Model(&model.QuotaData{}).Where("user_id = ? AND created_at >= ?", owner, state.WindowStart).Select("COALESCE(SUM(quota),0)").Scan(&window).Error; err != nil {
		return result, "", err
	}
	if err := acceptedAlertQuota(tx, MonitoredTarget{Type: "account", ID: owner}, 0, &accepted); err != nil {
		return result, "", err
	}
	notifiedValue := decimal.NewFromInt(state.MilestoneUSD).Mul(decimal.NewFromInt(int64(common.QuotaPerUnit)))
	if notifiedValue.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return result, "", ErrInvalid
	}
	notified := notifiedValue.IntPart()
	unnotified := decimal.NewFromInt(state.AccumulatedQuota).Add(decimal.NewFromInt(window)).Sub(decimal.NewFromInt(state.WindowQuota)).Sub(decimal.NewFromInt(notified))
	if unnotified.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || unnotified.LessThan(decimal.NewFromInt(math.MinInt64)) {
		return result, "", ErrInvalid
	}
	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	result = LegacyAccountPreview{OwnerUserID: owner, Name: name, Language: i18n.ParseAcceptLanguage(user.GetSetting().Language), State: state, UnnotifiedQuota: unnotified.IntPart(), AcceptedUsage: accepted, Ambiguous: state.MilestoneUSD > 0, DestinationPresent: address != "", SnapshotHash: model.AlertIdentity("user_spend", owner, state, model.AlertIdentity(address), int64(common.QuotaPerUnit))}
	return result, address, nil
}

func LegacyUserReportSnapshot(ctx context.Context, owner int) (LegacyAccountPreview, error) {
	value, _, err := legacyUserReportSnapshot(model.DB.WithContext(ctx), owner)
	return value, err
}

func legacyUserReportSnapshot(tx *gorm.DB, owner int) (LegacyAccountPreview, string, error) {
	var snapshot LegacyAccountPreview
	var rows []model.Option
	if err := tx.Where(map[string]any{"key": []string{"quota_alert_user_last_day_summary_map", "uid_quota_warning_robot"}}).Find(&rows).Error; err != nil {
		return snapshot, "", err
	}
	var days map[string]int64
	var address string
	for _, row := range rows {
		if row.Key == "uid_quota_warning_robot" {
			address = row.Value
			continue
		}
		if common.UnmarshalJsonStr(row.Value, &days) != nil {
			return snapshot, "", ErrInvalid
		}
	}
	var user model.User
	if err := tx.Select("id", "username", "display_name", "setting").First(&user, owner).Error; err != nil {
		return snapshot, "", err
	}
	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	day := days[strconv.Itoa(owner)]
	snapshot = LegacyAccountPreview{OwnerUserID: owner, Name: name, Language: i18n.ParseAcceptLanguage(user.GetSetting().Language), LastReportDay: day, Ambiguous: day > 0, DestinationPresent: address != "", SnapshotHash: model.AlertIdentity("user_report", owner, day, model.AlertIdentity(address))}
	return snapshot, address, nil
}

func ImportLegacyUserSpend(ctx context.Context, actor Actor, vault *TargetVault, owner int) (model.AlertMigration, error) {
	return importLegacyAccountRule(ctx, actor, vault, owner, "user_spend")
}

func ImportLegacyUserReport(ctx context.Context, actor Actor, vault *TargetVault, owner int) (model.AlertMigration, error) {
	return importLegacyAccountRule(ctx, actor, vault, owner, "user_report")
}

func importLegacyAccountRule(ctx context.Context, actor Actor, vault *TargetVault, owner int, kind string) (model.AlertMigration, error) {
	var migration model.AlertMigration
	if !actor.Root() {
		return migration, ErrForbidden
	}
	if vault == nil {
		return migration, ErrTargetKeyUnavailable
	}
	err := model.WithLegacyAlertLease(ctx, "quota_options", "user", func(ctx context.Context) error {
		id := model.AlertIdentity("legacy", kind, strconv.Itoa(owner))
		found := model.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&migration)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 {
			return nil
		}
		return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := model.LockAlertImportOwner(tx, owner, 1); err != nil {
				return err
			}
			var snapshot LegacyAccountPreview
			var address string
			var err error
			if kind == "user_report" {
				snapshot, address, err = legacyUserReportSnapshot(tx, owner)
			} else {
				snapshot, address, err = legacyUserSpendSnapshot(tx, owner)
			}
			if err != nil {
				return err
			}
			target := model.AlertTarget{OwnerUserID: owner, Name: "Imported account spending", Type: "feishu", Enabled: address != "", Language: snapshot.Language, Revision: 1}
			if kind == "user_report" {
				target.Name = "Imported daily report"
			}
			if address != "" {
				secret := TargetSecret{Address: address}
				hint := legacyTargetAddressHint("feishu", address)
				sealed, err := vault.Seal(owner, "feishu", secret)
				if err != nil {
					return err
				}
				target.AddressHint, target.Ciphertext, target.KeyID, target.RateKey = hint, sealed, vault.active, model.AlertIdentity("destination", "feishu", address, "")
			}
			now := time.Now().UnixMilli()
			target.CreatedAt, target.UpdatedAt = now, now
			if err := tx.Create(&target).Error; err != nil {
				return err
			}
			name := string([]rune(snapshot.Name)[:min(len([]rune(snapshot.Name)), 30)]) + " / spending"
			spec := model.AlertRuleSpec{QuotaUnit: int64(common.QuotaPerUnit), Name: name, Category: "usage", Severity: "P2", Enabled: address != "", ManagementMode: "user_managed", Scope: model.AlertScope{Type: "account", GroupMode: "aggregate"}, Condition: model.AlertCondition{Metric: "spend_net", Operator: "gte", ThresholdUSD: "1000", RecoveryThresholdUSD: "0", Window: model.AlertWindow{Type: "cumulative", Timezone: "Asia/Shanghai"}}, Trigger: model.AlertTrigger{Mode: "milestone", StepUSD: "1000", RepeatSeconds: 1800}, Notify: model.AlertNotify{TargetIDs: []int64{target.ID}}, Recovery: model.AlertRecovery{Mode: "manual"}}
			if kind == "user_report" {
				includeZero := false
				spec.Name = string([]rune(snapshot.Name)[:min(len([]rune(snapshot.Name)), 30)]) + " / daily report"
				spec.Category = "report"
				spec.Condition = model.AlertCondition{Metric: "spend_net", Operator: "gt", ThresholdUSD: "0", RecoveryThresholdUSD: "0", Window: model.AlertWindow{Type: "day", Timezone: "Asia/Shanghai", SendAt: "00:10"}}
				spec.Trigger = model.AlertTrigger{Mode: "schedule_report", RepeatSeconds: 1800, Report: model.AlertReportOptions{IncludeZero: &includeZero, BackfillPeriods: 7, PartialAfterSeconds: 1800, SendRevision: true}}
			}
			if err := validate(ctx, tx, actor, owner, &spec); err != nil {
				return err
			}
			encoded, err := common.Marshal(spec)
			if err != nil {
				return err
			}
			rule := model.AlertRule{OwnerUserID: owner, Name: spec.Name, Enabled: spec.Enabled, CreatedBy: actor.ID, UpdatedBy: actor.ID, Revision: 1, Generation: 1, EffectiveAt: now, NextEvaluationAt: now, CreatedAt: now, UpdatedAt: now, Config: string(encoded)}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AlertRuleRevision{RuleID: rule.ID, Revision: 1, Config: rule.Config, CreatedBy: actor.ID, CreatedAt: now}).Error; err != nil {
				return err
			}
			offset := decimal.NewFromInt(snapshot.UnnotifiedQuota).Sub(decimal.NewFromInt(snapshot.AcceptedUsage))
			if offset.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || offset.LessThan(decimal.NewFromInt(math.MinInt64)) {
				return ErrInvalid
			}
			checkpoint, err := common.Marshal(snapshot)
			if err != nil {
				return err
			}
			status := "observing"
			if snapshot.Ambiguous {
				status = "ambiguous"
			}
			migration = model.AlertMigration{ID: id, Kind: kind, ObjectID: strconv.Itoa(owner), OwnerUserID: owner, RuleID: rule.ID, Executor: "legacy", Status: status, Revision: 1, MetricOffset: offset.IntPart(), Checkpoint: string(checkpoint), LegacyConfigHash: model.AlertIdentity("1000"), ImportedAt: now, UpdatedAt: now, UpdatedBy: actor.ID}
			if err := tx.Create(&migration).Error; err != nil {
				return err
			}
			return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: owner, Operation: "migration.import", ObjectID: rule.ID, Revision: 1, CreatedAt: now}).Error
		})
	})
	return migration, err
}

func SwitchLegacyUserSpend(ctx context.Context, actor Actor, id string, expected int64, operation, snapshotHash string, acceptAmbiguous bool, expectedRule int64) error {
	if !actor.Root() {
		return ErrForbidden
	}
	if operation != "cutover" && operation != "rollback" {
		return ErrInvalid
	}
	var migration model.AlertMigration
	if err := model.DB.WithContext(ctx).First(&migration, "id = ?", id).Error; err != nil {
		return err
	}
	if migration.Kind != "user_spend" || migration.Revision != expected {
		return model.ErrAlertConflict
	}
	return model.WithLegacyAlertLease(ctx, "quota_options", "user", func(ctx context.Context) error {
		if operation == "cutover" {
			if !common.AlertingEnabled || common.AlertingShadowMode || !common.DataExportEnabled {
				return &BlockedAction{Code: "migration_not_ready"}
			}
			_, status, _, err := SourceWatermark(ctx, time.Now())
			if err != nil {
				return err
			}
			if status != "complete" {
				return &BlockedAction{Code: "data_incomplete"}
			}
		}
		return model.WithAlertMigration(ctx, migration, func(tx *gorm.DB, rule *model.AlertRule, current *model.AlertMigration) error {
			if rule.Revision != expectedRule {
				return model.ErrAlertConflict
			}
			option, err := model.LockLegacyAlertOption(tx, legacyUserMilestoneOption)
			if err != nil {
				return err
			}
			snapshot, _, err := legacyUserSpendSnapshot(tx, current.OwnerUserID)
			if err != nil {
				return err
			}
			if snapshot.SnapshotHash != snapshotHash {
				return model.ErrAlertConflict
			}
			spec, err := model.AlertRuleConfig(*rule)
			if err != nil {
				return err
			}
			if spec.QuotaUnit != int64(common.QuotaPerUnit) || spec.Scope.Type != "account" || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != "cumulative" || spec.Trigger.Mode != "milestone" || spec.Trigger.StepUSD != "1000" || len(spec.Actions) > 0 {
				return &BlockedAction{Code: "migration_rule_changed"}
			}
			next, executor := "unified", "legacy"
			if operation == "rollback" {
				next, executor = "legacy", "unified"
			}
			if current.Executor != executor {
				return model.ErrAlertConflict
			}
			now := time.Now().UnixMilli()
			if operation == "cutover" {
				if rule.DeletedAt.Valid || !rule.Enabled || snapshot.Ambiguous && !acceptAmbiguous || len(spec.Notify.TargetIDs) == 0 {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				var valid int64
				if err := tx.Model(&model.AlertTarget{}).Where("id IN ? AND enabled = ? AND ciphertext <> ?", spec.Notify.TargetIDs, true, "").Count(&valid).Error; err != nil {
					return err
				}
				if valid != int64(len(spec.Notify.TargetIDs)) {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				offset := decimal.NewFromInt(snapshot.UnnotifiedQuota).Sub(decimal.NewFromInt(snapshot.AcceptedUsage))
				// The old statistics and their accepted alert receipts share a
				// transaction. This absolute offset includes the old remainder,
				// while later queued charges and refunds still apply exactly once.
				if offset.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || offset.LessThan(decimal.NewFromInt(math.MinInt64)) {
					return ErrInvalid
				}
				current.MetricOffset, current.MetricStartAt, current.CutoverAt = offset.IntPart(), 0, now
			} else {
				var pending int64
				if err := tx.Model(&model.AlertJob{}).Where("rule_id = ? AND status IN ?", rule.ID, []string{"pending", "running", "retry_wait"}).Count(&pending).Error; err != nil {
					return err
				}
				if pending > 0 {
					return &BlockedAction{Code: "migration_jobs_pending"}
				}
				var progress model.AlertRuleState
				if err := tx.Where("id = ?", model.AlertIdentity("state", rule.ID, rule.Generation, "account", rule.OwnerUserID, "")).Limit(1).Find(&progress).Error; err != nil {
					return err
				}
				accepted := snapshot.AcceptedUsage
				if current.MetricStartAt > 0 {
					if err := acceptedAlertQuota(tx, MonitoredTarget{Type: "account", ID: rule.OwnerUserID}, current.MetricStartAt, &accepted); err != nil {
						return err
					}
				}
				step, err := USDQuota("1000")
				if err != nil {
					return err
				}
				tierUSD := decimal.NewFromInt(progress.HighestTier).Mul(decimal.NewFromInt(1000))
				if current.MetricStartAt == 0 {
					tierUSD = tierUSD.Add(decimal.NewFromInt(snapshot.State.MilestoneUSD))
				}
				if tierUSD.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
					return ErrInvalid
				}
				notifiedValue := tierUSD.Mul(decimal.NewFromInt(int64(common.QuotaPerUnit)))
				if notifiedValue.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
					return ErrInvalid
				}
				notified := notifiedValue.IntPart()
				remainder := decimal.NewFromInt(accepted).Add(decimal.NewFromInt(current.MetricOffset)).Sub(decimal.NewFromInt(progress.HighestTier).Mul(decimal.NewFromInt(step)))
				// A refund may put usage below a previously notified tier. Retain
				// that negative remainder; clamping it would replay old milestones.
				accumulated := decimal.NewFromInt(notified).Add(remainder)
				if accumulated.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || accumulated.LessThan(decimal.NewFromInt(math.MinInt64)) {
					return ErrInvalid
				}
				state := snapshot.State
				state.AccumulatedQuota, state.MilestoneUSD = accumulated.IntPart(), tierUSD.IntPart()
				state.WindowStart = now/1000 - now/1000%3600 - 3600
				if current.MetricStartAt > 0 {
					state.BaselineAt = current.MetricStartAt / 1000
				}
				if err := tx.Model(&model.QuotaData{}).Where("user_id = ? AND created_at >= ?", rule.OwnerUserID, state.WindowStart).Select("COALESCE(SUM(quota),0)").Scan(&state.WindowQuota).Error; err != nil {
					return err
				}
				var states map[string]model.LegacyUserQuotaMilestone
				if common.UnmarshalJsonStr(option.Value, &states) != nil {
					return ErrInvalid
				}
				states[strconv.Itoa(rule.OwnerUserID)] = state
				encoded, err := common.Marshal(states)
				if err != nil {
					return err
				}
				if err := tx.Model(&model.Option{}).Where(map[string]any{"key": legacyUserMilestoneOption}).Update("value", string(encoded)).Error; err != nil {
					return err
				}
			}
			checkpoint, err := common.Marshal(snapshot)
			if err != nil {
				return err
			}
			return model.CommitAlertMigration(tx, rule, current, actor.ID, next, now, string(checkpoint))
		})
	})
}

// Daily reports start at a fully covered natural-day boundary. Cutover waits
// for the previous day's legacy report, so no partial day silently disappears
// between the two executors and historical reports are not sent a second time.
func SwitchLegacyUserReport(ctx context.Context, actor Actor, id string, expected int64, operation, snapshotHash string, acceptAmbiguous bool, expectedRule int64, now time.Time) error {
	if !actor.Root() {
		return ErrForbidden
	}
	if operation != "cutover" && operation != "rollback" {
		return ErrInvalid
	}
	var migration model.AlertMigration
	if err := model.DB.WithContext(ctx).First(&migration, "id = ?", id).Error; err != nil {
		return err
	}
	if migration.Kind != "user_report" || migration.Revision != expected {
		return model.ErrAlertConflict
	}
	return model.WithLegacyAlertLease(ctx, "quota_options", "user", func(ctx context.Context) error {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			return err
		}
		local := now.In(loc)
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		if operation == "cutover" {
			if !common.AlertingEnabled || common.AlertingShadowMode || !common.DataExportEnabled {
				return &BlockedAction{Code: "migration_not_ready"}
			}
			watermark, status, _, err := SourceWatermark(ctx, now)
			if err != nil {
				return err
			}
			if status != "complete" {
				return &BlockedAction{Code: "data_incomplete"}
			}
			status, _, err = WindowCoverage(ctx, day.UnixMilli(), watermark, now)
			if err != nil {
				return err
			}
			if status != "complete" {
				return &BlockedAction{Code: "migration_window_uncovered"}
			}
			status, _, err = UsageCoverage(ctx, now)
			if err != nil {
				return err
			}
			if status != "complete" {
				return &BlockedAction{Code: "data_incomplete"}
			}
		}
		return model.WithAlertMigration(ctx, migration, func(tx *gorm.DB, rule *model.AlertRule, current *model.AlertMigration) error {
			if rule.Revision != expectedRule {
				return model.ErrAlertConflict
			}
			snapshot, _, err := legacyUserReportSnapshot(tx, current.OwnerUserID)
			if err != nil {
				return err
			}
			if snapshot.SnapshotHash != snapshotHash {
				return model.ErrAlertConflict
			}
			spec, err := model.AlertRuleConfig(*rule)
			if err != nil {
				return err
			}
			if spec.QuotaUnit != int64(common.QuotaPerUnit) || spec.Scope.Type != "account" || spec.Trigger.Mode != "schedule_report" || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != "day" || spec.Condition.Window.Timezone != "Asia/Shanghai" || len(spec.Actions) > 0 {
				return &BlockedAction{Code: "migration_rule_changed"}
			}
			executor, next, effective := "legacy", "unified", day.UnixMilli()
			if operation == "rollback" {
				executor, next, effective = "unified", "legacy", now.UnixMilli()
			}
			if current.Executor != executor {
				return model.ErrAlertConflict
			}
			if operation == "cutover" {
				if !rule.Enabled || rule.DeletedAt.Valid || snapshot.Ambiguous && !acceptAmbiguous || len(spec.Notify.TargetIDs) == 0 {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				var valid int64
				if err := tx.Model(&model.AlertTarget{}).Where("id IN ? AND enabled = ? AND ciphertext <> ?", spec.Notify.TargetIDs, true, "").Count(&valid).Error; err != nil {
					return err
				}
				if valid != int64(len(spec.Notify.TargetIDs)) {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				previous := day.AddDate(0, 0, -1).Unix()
				var rows int64
				if err := tx.Model(&model.QuotaData{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", rule.OwnerUserID, previous, day.Unix()).Count(&rows).Error; err != nil {
					return err
				}
				if rows > 0 && snapshot.LastReportDay < previous {
					return &BlockedAction{Code: "legacy_report_pending"}
				}
				current.CutoverAt = now.UnixMilli()
			} else {
				var pending int64
				if err := tx.Model(&model.AlertJob{}).Where("rule_id = ? AND status IN ?", rule.ID, []string{"pending", "running", "retry_wait"}).Count(&pending).Error; err != nil {
					return err
				}
				if pending > 0 {
					return &BlockedAction{Code: "migration_jobs_pending"}
				}
				// Only an accepted notification may advance the old report date.
				// Batch receipt lookup keeps the checkpoint transaction bounded.
				var events []model.AlertEvent
				if err := tx.Where("rule_id = ? AND kind IN ? AND created_at >= ?", rule.ID, []string{"report", "report_revision"}, current.CutoverAt).Order("id desc").Limit(100).Find(&events).Error; err != nil {
					return err
				}
				eventIDs := make([]int64, 0, len(events))
				for _, event := range events {
					eventIDs = append(eventIDs, event.ID)
				}
				var receipts []struct{ EventID int64 }
				if len(eventIDs) > 0 {
					if err := tx.Model(&model.AlertJob{}).Select("event_id").Where("event_id IN ? AND kind = ? AND accepted_at > 0", eventIDs, "notify").Group("event_id").Scan(&receipts).Error; err != nil {
						return err
					}
				}
				accepted := make(map[int64]bool, len(receipts))
				for _, receipt := range receipts {
					accepted[receipt.EventID] = true
				}
				var archived struct{ Earliest, Latest, WindowEnd int64 }
				if err := tx.Model(&model.AlertEventReceipt{}).Where("report_rule_id = ? AND report_created_at >= ?", rule.ID, current.CutoverAt).
					Select("COALESCE(MIN(report_start),0) AS earliest, COALESCE(MAX(report_start),0) AS latest, COALESCE(MAX(report_end),0) AS window_end").Scan(&archived).Error; err != nil {
					return err
				}
				if archived.Earliest < 0 {
					return ErrInvalid
				}
				if archived.WindowEnd > now.UnixMilli() {
					return &BlockedAction{Code: "clock_skew"}
				}
				last := max(snapshot.LastReportDay, archived.Latest/1000)
				for _, event := range events {
					if !accepted[event.ID] {
						continue
					}
					var evidence struct {
						Measurement struct {
							WindowStart int64 `json:"window_start"`
							WindowEnd   int64 `json:"window_end"`
						} `json:"measurement"`
					}
					if common.UnmarshalJsonStr(event.Evidence, &evidence) != nil {
						return ErrInvalid
					}
					if evidence.Measurement.WindowEnd > now.UnixMilli() {
						return &BlockedAction{Code: "clock_skew"}
					}
					last = max(last, evidence.Measurement.WindowStart/1000)
				}

				option, err := model.LockLegacyAlertOption(tx, "quota_alert_user_last_day_summary_map")
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				}
				days := make(map[string]int64)
				if option.Value != "" && common.UnmarshalJsonStr(option.Value, &days) != nil {
					return ErrInvalid
				}
				if days[strconv.Itoa(rule.OwnerUserID)] != snapshot.LastReportDay {
					return model.ErrAlertConflict
				}
				days[strconv.Itoa(rule.OwnerUserID)] = last
				value, err := common.Marshal(days)
				if err != nil {
					return err
				}
				if err := tx.Save(&model.Option{Key: "quota_alert_user_last_day_summary_map", Value: string(value)}).Error; err != nil {
					return err
				}
			}
			checkpoint, err := common.Marshal(snapshot)
			if err != nil {
				return err
			}
			return model.CommitAlertMigration(tx, rule, current, actor.ID, next, effective, string(checkpoint))
		})
	})
}
