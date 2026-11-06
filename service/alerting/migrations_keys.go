package alerting

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type LegacyKeyPreview struct {
	OwnerUserID        int                      `json:"owner_user_id"`
	Name               string                   `json:"name"`
	Period             string                   `json:"period"`
	Rule               model.LegacyKeyQuotaRule `json:"rule"`
	CutoverAt          int64                    `json:"cutover_at"`
	Ambiguous          bool                     `json:"ambiguous"`
	DestinationPresent bool                     `json:"destination_present"`
	SnapshotHash       string                   `json:"snapshot_hash"`
}

func LegacyKeySnapshot(ctx context.Context, kind, object string, now time.Time) (LegacyKeyPreview, error) {
	snapshot, _, err := legacyKeySnapshot(model.DB.WithContext(ctx), kind, object, now)
	return snapshot, err
}

func legacyKeySnapshot(tx *gorm.DB, kind, object string, now time.Time) (LegacyKeyPreview, string, error) {
	var snapshot LegacyKeyPreview
	text, _, ok := strings.Cut(object, ":")
	owner, err := strconv.Atoi(text)
	if !ok || err != nil || owner <= 0 || kind != "key_hour" && kind != "key_day" {
		return snapshot, "", ErrInvalid
	}
	var option model.Option
	if err := tx.Where(map[string]any{"key": "key_quota_warning_config"}).First(&option).Error; err != nil {
		return snapshot, "", err
	}
	var configs map[string]*model.LegacyKeyQuotaUserConfig
	if common.UnmarshalJsonStr(option.Value, &configs) != nil || configs[text] == nil {
		return snapshot, "", ErrInvalid
	}
	config := configs[text]
	period, rules := "hour", config.OneHour
	local := now.In(time.FixedZone("CST", 8*60*60))
	boundary := time.Date(local.Year(), local.Month(), local.Day(), local.Hour()+1, 0, 0, 0, local.Location())
	if kind == "key_day" {
		period, rules = "day", config.OneDay
		boundary = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, local.Location())
	}
	var selected *model.LegacyKeyQuotaRule
	for _, rule := range rules {
		if rule.QuotaUSD <= 0 || rule.Action != "warn" && rule.Action != "disable_key" {
			continue
		}
		if model.LegacyKeyQuotaObject(owner, period, rule) == object {
			copy := rule
			selected = &copy
			break
		}
	}
	if selected == nil {
		return snapshot, "", ErrInvalid
	}
	var user model.User
	if err := tx.Select("id", "username", "display_name").First(&user, owner).Error; err != nil {
		return snapshot, "", err
	}
	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	// Old markers omit the threshold and failed sends may have advanced them.
	// Treat the entire current period as ambiguous; never infer delivery.
	snapshot = LegacyKeyPreview{OwnerUserID: owner, Name: name, Period: period, Rule: *selected, CutoverAt: boundary.UnixMilli(), Ambiguous: true, DestinationPresent: config.FeishuRobotURL != ""}
	snapshot.SnapshotHash = model.AlertIdentity(kind, object, *selected, model.AlertIdentity(config.FeishuRobotURL), snapshot.CutoverAt, int64(common.QuotaPerUnit))
	return snapshot, config.FeishuRobotURL, nil
}

func ImportLegacyKey(ctx context.Context, actor Actor, vault *TargetVault, kind, object string) (model.AlertMigration, error) {
	var migration model.AlertMigration
	if !actor.Root() {
		return migration, ErrForbidden
	}
	if vault == nil {
		return migration, ErrTargetKeyUnavailable
	}
	err := model.WithLegacyAlertLease(ctx, "quota_options", "key", func(ctx context.Context) error {
		id := model.AlertIdentity("legacy", kind, object)
		found := model.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&migration)
		if found.Error != nil || found.RowsAffected > 0 {
			return found.Error
		}
		return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			snapshot, address, err := legacyKeySnapshot(tx, kind, object, time.Now())
			if err != nil {
				return err
			}
			if err := model.LockAlertImportOwner(tx, snapshot.OwnerUserID, 1); err != nil {
				return err
			}
			now := time.Now().UnixMilli()
			target := model.AlertTarget{OwnerUserID: snapshot.OwnerUserID, Name: "Imported Key period alert", Type: "feishu", Enabled: address != "", Language: "zh-CN", Revision: 1, CreatedAt: now, UpdatedAt: now}
			if address != "" {
				secret := TargetSecret{Address: address}
				hint, err := ValidateTargetAddress(ctx, "feishu", secret)
				if err != nil {
					return err
				}
				sealed, err := vault.Seal(snapshot.OwnerUserID, "feishu", secret)
				if err != nil {
					return err
				}
				target.AddressHint, target.Ciphertext, target.KeyID, target.RateKey = hint, sealed, vault.active, model.AlertIdentity("destination", "feishu", address, "")
			}
			if err := tx.Create(&target).Error; err != nil {
				return err
			}
			window := "hour"
			if kind == "key_day" {
				window = "day"
			}
			spec := model.AlertRuleSpec{QuotaUnit: int64(common.QuotaPerUnit), Name: snapshot.Name + " / Key " + window, Category: "usage", Severity: "P2", Enabled: address != "", ManagementMode: "platform_enforced", Scope: model.AlertScope{Type: "api_key", Selector: "all_owned", GroupMode: "per_entity", IncludeFuture: true}, Condition: model.AlertCondition{Metric: "spend_net", Operator: "gte", ThresholdUSD: decimal.NewFromFloat(snapshot.Rule.QuotaUSD).String(), RecoveryThresholdUSD: "0", Window: model.AlertWindow{Type: window, Timezone: "Asia/Shanghai"}}, Trigger: model.AlertTrigger{Mode: "once_per_period", RepeatSeconds: 1800}, Notify: model.AlertNotify{TargetIDs: []int64{target.ID}}, Recovery: model.AlertRecovery{Mode: "manual"}}
			if snapshot.Rule.Action == "disable_key" {
				spec.Actions = []model.AlertAction{{Type: "disable_api_key", Target: "matched_entity"}}
			}
			if err := validate(ctx, tx, actor, snapshot.OwnerUserID, &spec); err != nil {
				return err
			}
			encoded, err := common.Marshal(spec)
			if err != nil {
				return err
			}
			rule := model.AlertRule{OwnerUserID: snapshot.OwnerUserID, Name: spec.Name, Enabled: spec.Enabled, CreatedBy: actor.ID, UpdatedBy: actor.ID, Revision: 1, Generation: 1, EffectiveAt: now, NextEvaluationAt: now, CreatedAt: now, UpdatedAt: now, Config: string(encoded)}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AlertRuleRevision{RuleID: rule.ID, Revision: 1, Config: rule.Config, CreatedBy: actor.ID, CreatedAt: now}).Error; err != nil {
				return err
			}
			checkpoint, err := common.Marshal(snapshot)
			if err != nil {
				return err
			}
			migration = model.AlertMigration{ID: id, Kind: kind, ObjectID: object, OwnerUserID: snapshot.OwnerUserID, RuleID: rule.ID, Executor: "legacy", Status: "ambiguous", Revision: 1, Checkpoint: string(checkpoint), LegacyConfigHash: model.AlertIdentity(snapshot.Rule), ImportedAt: now, UpdatedAt: now, UpdatedBy: actor.ID}
			if err := tx.Create(&migration).Error; err != nil {
				return err
			}
			return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: snapshot.OwnerUserID, Operation: "migration.import", ObjectID: rule.ID, Revision: 1, CreatedAt: now}).Error
		})
	})
	return migration, err
}

func SwitchLegacyKey(ctx context.Context, actor Actor, id string, expected int64, operation, hash string, accept bool, expectedRule int64, now time.Time) error {
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
	if migration.Revision != expected || migration.Kind != "key_hour" && migration.Kind != "key_day" {
		return model.ErrAlertConflict
	}
	return model.WithLegacyAlertLease(ctx, "quota_options", "key", func(ctx context.Context) error {
		if operation == "cutover" {
			if !common.AlertingEnabled || common.AlertingShadowMode || !common.DataExportEnabled || !accept {
				return &BlockedAction{Code: "migration_not_ready"}
			}
			_, status, _, err := SourceWatermark(ctx, now)
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
			snapshot, _, err := legacyKeySnapshot(tx, current.Kind, current.ObjectID, now)
			if err != nil {
				return err
			}
			if snapshot.SnapshotHash != hash {
				return model.ErrAlertConflict
			}
			spec, err := model.AlertRuleConfig(*rule)
			if err != nil {
				return err
			}
			if spec.QuotaUnit != int64(common.QuotaPerUnit) || spec.Scope.Type != "api_key" || spec.Scope.Selector != "all_owned" || !spec.Scope.IncludeFuture || spec.Scope.GroupMode != "per_entity" || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != snapshot.Period || spec.Condition.Window.Timezone != "Asia/Shanghai" || spec.Trigger.Mode != "once_per_period" || spec.Condition.ThresholdUSD != decimal.NewFromFloat(snapshot.Rule.QuotaUSD).String() {
				return &BlockedAction{Code: "migration_rule_changed"}
			}
			next, effective := "unified", snapshot.CutoverAt
			if operation == "cutover" {
				if current.Executor != "legacy" {
					return model.ErrAlertConflict
				}
				if !rule.Enabled || rule.DeletedAt.Valid || len(spec.Notify.TargetIDs) == 0 || len(spec.Actions) > 0 && !common.AlertingActionsEnabled {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				var valid int64
				if err := tx.Model(&model.AlertTarget{}).Where("id IN ? AND enabled = ? AND ciphertext <> ?", spec.Notify.TargetIDs, true, "").Count(&valid).Error; err != nil {
					return err
				}
				if valid != int64(len(spec.Notify.TargetIDs)) {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				current.CutoverAt = snapshot.CutoverAt
			} else {
				if current.Executor != "unified" {
					return model.ErrAlertConflict
				}
				next, effective = "legacy", now.UnixMilli()
				var pending int64
				if err := tx.Model(&model.AlertJob{}).Where("rule_id = ? AND status IN ?", rule.ID, []string{"pending", "running", "retry_wait"}).Count(&pending).Error; err != nil {
					return err
				}
				if pending > 0 {
					return &BlockedAction{Code: "migration_jobs_pending"}
				}
				if now.UnixMilli() >= current.CutoverAt {
					if err := restoreLegacyKeyMarkers(tx, *rule, snapshot, now); err != nil {
						return err
					}
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

func restoreLegacyKeyMarkers(tx *gorm.DB, rule model.AlertRule, snapshot LegacyKeyPreview, now time.Time) error {
	local := now.In(time.FixedZone("CST", 8*60*60))
	start := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, local.Location())
	duration, prefix := time.Hour, "1h"
	if snapshot.Period == "day" {
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		duration, prefix = 24*time.Hour, "1d"
	}
	// Only successful delivery or a committed disable action is transferable.
	// Failed attempts do not prove that a user received a warning.
	var events []model.AlertEvent
	if err := tx.Where("rule_id = ? AND created_at >= ?", rule.ID, start.UnixMilli()).Limit(10001).Find(&events).Error; err != nil {
		return err
	}
	if len(events) > 10000 {
		return model.ErrAlertConflict
	}
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	var jobs []model.AlertJob
	if len(ids) > 0 {
		if err := tx.Where("event_id IN ? AND (accepted_at > 0 OR (kind = ? AND status = ?))", ids, "action", "succeeded").Find(&jobs).Error; err != nil {
			return err
		}
	}
	accepted := make(map[int64]bool, len(jobs))
	for _, job := range jobs {
		accepted[job.EventID] = true
	}
	option, err := model.LockLegacyAlertOption(tx, "key_quota_warning_alerted")
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	states := make(map[string]map[string]int64)
	if option.Value != "" && common.UnmarshalJsonStr(option.Value, &states) != nil {
		return ErrInvalid
	}
	key := prefix + "_" + strconv.FormatInt(start.Unix(), 10) + "_" + strconv.FormatInt(start.Add(duration).Unix()-1, 10) + "_" + snapshot.Rule.Action
	for _, event := range events {
		if !accepted[event.ID] {
			continue
		}
		var evidence struct {
			Target MonitoredTarget `json:"target"`
		}
		if common.UnmarshalJsonStr(event.Evidence, &evidence) != nil || evidence.Target.Type != "api_key" || evidence.Target.ID <= 0 {
			return ErrInvalid
		}
		token := strconv.Itoa(evidence.Target.ID)
		if states[token] == nil {
			states[token] = make(map[string]int64)
		}
		states[token][key] = start.Unix()
	}
	encoded, err := common.Marshal(states)
	if err != nil {
		return err
	}
	return tx.Save(&model.Option{Key: "key_quota_warning_alerted", Value: string(encoded)}).Error
}
