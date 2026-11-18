package alerting

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// These adapters retain the established trigger, threshold, pool identity,
// cooldown and checkpoint. Only delivery changes executors. This is deliberate:
// a balance crossing must never be reconstructed as another billing operation.
func IsLegacyNoticeKind(kind string) bool {
	switch kind {
	case "uid_day", "uid_budget_notice", "channel_arrears", "slow_error", "user_notifications":
		return true
	}
	return false
}

// UID compatibility events still use the established quota_data checkpoint
// source. Balance and channel signals are emitted directly and do not need it.
func LegacyNoticeSourceReady(kind string) bool {
	if kind != "uid_day" && kind != "uid_budget_notice" {
		return true
	}
	return common.DataExportEnabled && os.Getenv("ENABLE_UID_FEISHU_ALERTS") == "true"
}

type LegacyNotice struct {
	ChannelID int    `json:"channel_id,omitempty"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Values    []any  `json:"values,omitempty"`
}

type LegacyNoticeCooldown struct {
	Key      string
	Duration time.Duration
}

type LegacyNoticePreview struct {
	Name               string `json:"name"`
	OwnerUserID        int    `json:"owner_user_id"`
	Ambiguous          bool   `json:"ambiguous"`
	DestinationPresent bool   `json:"destination_present"`
	SnapshotHash       string `json:"snapshot_hash"`
}

func LegacyNoticeSnapshot(ctx context.Context, kind, object string) (LegacyNoticePreview, error) {
	var result LegacyNoticePreview
	if !IsLegacyNoticeKind(kind) {
		return result, ErrInvalid
	}
	if kind == "user_notifications" {
		owner, err := strconv.Atoi(object)
		if err != nil || owner <= 0 {
			return result, ErrInvalid
		}
		var user model.User
		if err := model.DB.WithContext(ctx).Select("id", "username", "display_name", "email", "setting").First(&user, owner).Error; err != nil {
			return result, err
		}
		name := user.DisplayName
		if name == "" {
			name = user.Username
		}
		return LegacyNoticePreview{Name: name, OwnerUserID: owner, DestinationPresent: true, SnapshotHash: model.AlertIdentity(kind, owner, user.Setting, user.Email)}, nil
	}
	if object != "site" {
		return result, ErrInvalid
	}
	keys := legacyNoticeOptions(kind)
	var rows []model.Option
	if err := model.DB.WithContext(ctx).Where(map[string]any{"key": keys}).Find(&rows).Error; err != nil {
		return result, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return LegacyNoticePreview{Name: kind, DestinationPresent: strings.TrimSpace(values[keys[0]]) != "", SnapshotHash: model.AlertIdentity(kind, values)}, nil
}

func legacyNoticeOptions(kind string) []string {
	switch kind {
	case "channel_arrears":
		return []string{"feishu_qianfei_webhook_url", "feishu_qianfei_secret"}
	case "slow_error":
		return []string{"slow_error_feishu_webhook_url", "slow_error_feishu_secret", "slow_error_threshold_seconds", "ErrorWarningEnvName"}
	case "uid_budget_notice":
		return []string{"uid_quota_warning_robot", "uid_quota_warning_email"}
	default:
		return []string{"uid_quota_warning_robot"}
	}
}

func ImportLegacyNotice(ctx context.Context, actor Actor, vault *TargetVault, kind, object string) (model.AlertMigration, error) {
	var migration model.AlertMigration
	if !actor.Root() {
		return migration, ErrForbidden
	}
	if vault == nil {
		return migration, ErrTargetKeyUnavailable
	}
	err := model.WithLegacyNoticeLease(ctx, kind, object, func(ctx context.Context) error {
		id := model.AlertIdentity("legacy", kind, object)
		found := model.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&migration)
		if found.Error != nil || found.RowsAffected > 0 {
			return found.Error
		}
		snapshot, err := LegacyNoticeSnapshot(ctx, kind, object)
		if err != nil {
			return err
		}
		var targets []int64
		if kind == "user_notifications" {
			target, err := ImportLegacyNotification(ctx, actor, vault, snapshot.OwnerUserID, false)
			if err != nil {
				return err
			}
			targets = append(targets, target.ID)
		} else {
			keys := legacyNoticeOptions(kind)
			var rows []model.Option
			if err := model.DB.WithContext(ctx).Where(map[string]any{"key": keys}).Find(&rows).Error; err != nil {
				return err
			}
			values := make(map[string]string, len(rows))
			for _, row := range rows {
				values[row.Key] = row.Value
			}
			secret := TargetSecret{Address: strings.TrimSpace(values[keys[0]])}
			if kind == "channel_arrears" || kind == "slow_error" {
				secret.Secret = values[keys[1]]
			}
			if secret.Address != "" {
				target, err := vault.save(ctx, actor, 0, 0, TargetInput{Name: "Imported " + kind, Type: "feishu", Enabled: true, Language: "zh-CN", Credentials: &secret, CredentialAction: "replace"}, true)
				if err != nil {
					return err
				}
				targets = append(targets, target.ID)
			}
			if kind == "uid_budget_notice" {
				for _, email := range strings.FieldsFunc(values["uid_quota_warning_email"], func(r rune) bool { return r == ';' || r == ',' }) {
					email = strings.TrimSpace(email)
					if email == "" {
						continue
					}
					if len(targets) >= 10 {
						return ErrInvalid
					}
					secret := TargetSecret{Address: email}
					target, err := vault.save(ctx, actor, 0, 0, TargetInput{Name: "Imported UID budget email", Type: "email", Enabled: true, Language: "zh-CN", Credentials: &secret, CredentialAction: "replace"}, true)
					if err != nil {
						return err
					}
					targets = append(targets, target.ID)
				}
			}
		}
		spec := model.AlertRuleSpec{QuotaUnit: int64(common.QuotaPerUnit), Name: "Compatible " + snapshot.Name, Category: "system", Severity: "P1", Enabled: len(targets) > 0, ManagementMode: "platform_enforced", Scope: model.AlertScope{Type: "system", GroupMode: "per_entity"}, Condition: model.AlertCondition{Metric: "legacy_notice", LegacySignal: kind, Operator: "gte", ThresholdCount: 1, Window: model.AlertWindow{Type: "rolling", DurationSeconds: 60, Timezone: "Asia/Shanghai"}}, Trigger: model.AlertTrigger{Mode: "once", RepeatSeconds: 1800}, Notify: model.AlertNotify{TargetIDs: targets}, Recovery: model.AlertRecovery{Mode: "manual"}}
		if len(targets) == 0 {
			return &BlockedAction{Code: "migration_destination_missing"}
		}
		if kind == "user_notifications" {
			spec.Scope.Type, spec.Scope.GroupMode, spec.Severity = "account", "aggregate", "P2"
		}
		if err := Validate(ctx, actor, snapshot.OwnerUserID, &spec); err != nil {
			return err
		}
		return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := model.LockAlertImportOwner(tx, snapshot.OwnerUserID, 0); err != nil {
				return err
			}
			now := time.Now().UnixMilli()
			encoded, err := common.Marshal(spec)
			if err != nil {
				return err
			}
			rule := model.AlertRule{OwnerUserID: snapshot.OwnerUserID, Name: spec.Name, Enabled: spec.Enabled, Revision: 1, Generation: 1, CreatedBy: actor.ID, UpdatedBy: actor.ID, EffectiveAt: now, NextEvaluationAt: now, CreatedAt: now, UpdatedAt: now, Config: string(encoded)}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AlertRuleRevision{RuleID: rule.ID, Revision: 1, Config: rule.Config, CreatedBy: actor.ID, CreatedAt: now}).Error; err != nil {
				return err
			}
			migration = model.AlertMigration{ID: id, Kind: kind, ObjectID: object, OwnerUserID: snapshot.OwnerUserID, RuleID: rule.ID, Executor: "legacy", Status: "observing", Revision: 1, LegacyConfigHash: snapshot.SnapshotHash, ImportedAt: now, UpdatedAt: now, UpdatedBy: actor.ID}
			if err := tx.Create(&migration).Error; err != nil {
				return err
			}
			return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: rule.OwnerUserID, Operation: "migration.import", ObjectID: rule.ID, Revision: 1, CreatedAt: now}).Error
		})
	})
	return migration, err
}

func SwitchLegacyNotice(ctx context.Context, actor Actor, id string, expected int64, operation, hash string, expectedRule int64) error {
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
	if migration.Revision != expected || !IsLegacyNoticeKind(migration.Kind) {
		return model.ErrAlertConflict
	}
	return model.WithLegacyNoticeLease(ctx, migration.Kind, migration.ObjectID, func(ctx context.Context) error {
		if operation == "cutover" && !LegacyNoticeSourceReady(migration.Kind) {
			return &BlockedAction{Code: "data_incomplete"}
		}
		snapshot, err := LegacyNoticeSnapshot(ctx, migration.Kind, migration.ObjectID)
		if err != nil {
			return err
		}
		if snapshot.SnapshotHash != hash {
			return model.ErrAlertConflict
		}
		return model.WithAlertMigration(ctx, migration, func(tx *gorm.DB, rule *model.AlertRule, current *model.AlertMigration) error {
			if rule.Revision != expectedRule {
				return model.ErrAlertConflict
			}
			spec, err := model.AlertRuleConfig(*rule)
			if err != nil {
				return err
			}
			if spec.Condition.Metric != "legacy_notice" || spec.Condition.LegacySignal != current.Kind || len(spec.Actions) > 0 {
				return &BlockedAction{Code: "migration_rule_changed"}
			}
			next := "unified"
			if operation == "cutover" {
				if current.Executor != "legacy" {
					return model.ErrAlertConflict
				}
				if !common.AlertingEnabled || common.AlertingShadowMode || !rule.Enabled || rule.DeletedAt.Valid || !snapshot.DestinationPresent || len(spec.Notify.TargetIDs) == 0 {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				var valid int64
				if err := tx.Model(&model.AlertTarget{}).Where("id IN ? AND enabled = ? AND ciphertext <> ?", spec.Notify.TargetIDs, true, "").Count(&valid).Error; err != nil {
					return err
				}
				if valid != int64(len(spec.Notify.TargetIDs)) {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				current.CutoverAt = time.Now().UnixMilli()
			} else {
				if current.Executor != "unified" {
					return model.ErrAlertConflict
				}
				next = "legacy"
				var unfinished int64
				if err := tx.Model(&model.AlertJob{}).Where("rule_id = ? AND status IN ?", rule.ID, []string{"pending", "running", "retry_wait"}).Count(&unfinished).Error; err != nil {
					return err
				}
				if unfinished > 0 {
					return &BlockedAction{Code: "migration_jobs_pending"}
				}
			}
			return model.CommitAlertMigration(tx, rule, current, actor.ID, next, time.Now().UnixMilli(), "{}")
		})
	})
}

// DispatchLegacyNotice serializes the old send and the executor switch. Once
// unified, an SQL failure is returned, never followed by a second legacy send.
// Existing checkpoints advance only after this durable event/jobs transaction.
func DispatchLegacyNotice(ctx context.Context, kind, object, identity, name string, notice LegacyNotice, legacy func(context.Context) error, cooldowns ...LegacyNoticeCooldown) error {
	if !common.AlertingControlsEnabled {
		return legacy(ctx)
	}
	return model.WithLegacyNoticeLease(ctx, kind, object, func(ctx context.Context) error {
		var migration model.AlertMigration
		found := model.DB.WithContext(ctx).Where("id = ?", model.AlertIdentity("legacy", kind, object)).Limit(1).Find(&migration)
		if found.Error != nil {
			return found.Error
		}
		var cooldown model.AlertNotificationLimit
		if len(cooldowns) > 0 && cooldowns[0].Duration > 0 {
			cooldown.ID = model.AlertIdentity("legacy-notice-cooldown", kind, object, cooldowns[0].Key)
			if err := model.DB.WithContext(ctx).Where("id = ?", cooldown.ID).Limit(1).Find(&cooldown).Error; err != nil {
				return err
			}
			if cooldown.ExpiresAt > time.Now().UnixMilli() {
				return nil
			}
			cooldown.ExpiresAt = time.Now().Add(cooldowns[0].Duration).UnixMilli()
		}
		unified := found.RowsAffected > 0 && migration.Executor == "unified"
		if unified && (!common.AlertingEnabled || common.AlertingShadowMode) {
			return &BlockedAction{Code: "migration_not_ready"}
		}
		if unified && !LegacyNoticeSourceReady(kind) {
			return &BlockedAction{Code: "data_incomplete"}
		}
		if found.RowsAffected > 0 && common.AlertingEnabled {

			err := model.WithAlertMigration(ctx, migration, func(tx *gorm.DB, rule *model.AlertRule, _ *model.AlertMigration) error {
				if !rule.Enabled || rule.DeletedAt.Valid {
					if !unified {
						return nil
					}
					return &BlockedAction{Code: "migration_not_ready"}
				}
				spec, err := model.AlertRuleConfig(*rule)
				if err != nil {
					return err
				}
				if spec.Condition.Metric != "legacy_notice" || spec.Condition.LegacySignal != kind {
					return &BlockedAction{Code: "migration_rule_changed"}
				}
				if (kind == "channel_arrears" || kind == "slow_error") && len(spec.Scope.ChannelIDs) > 0 && !slices.Contains(spec.Scope.ChannelIDs, notice.ChannelID) {
					return nil
				}
				now := time.Now().UnixMilli()
				body, err := common.Marshal(notice)
				if err != nil {
					return err
				}
				if len(body) > 16384 {
					return ErrInvalid
				}
				event := model.AlertEvent{DedupKey: model.AlertIdentity("legacy-notice", migration.ID, migration.Executor, identity), RuleID: rule.ID, OwnerUserID: rule.OwnerUserID, Kind: "legacy_notice", Severity: spec.Severity, TargetName: name, Summary: notice.Title, Evidence: string(body), RuleConfig: rule.Config, CreatedAt: now}
				inserted, err := model.InsertAlertEventOnce(tx, &event)
				if err != nil || !inserted {
					return err
				}
				priority := 3
				switch spec.Severity {
				case "P0":
					priority = 0
				case "P1":
					priority = 1
				case "P2":
					priority = 2
				}
				for _, id := range spec.Notify.TargetIDs {
					var target model.AlertTarget
					if err := tx.First(&target, id).Error; err != nil {
						return err
					}
					job := model.AlertJob{DedupKey: model.AlertIdentity(event.ID, "notify", id, target.Revision), EventID: event.ID, RuleID: rule.ID, RuleRevision: rule.Revision, Kind: "notify", Platform: true, TargetID: id, TargetRevision: target.Revision, TargetName: target.Name, TargetType: target.Type, TargetHint: target.AddressHint, Priority: priority, Status: "pending", AvailableAt: now, ExpiresAt: now + 86400000, CreatedAt: now, UpdatedAt: now}
					if err := createJob(tx, &job); err != nil {
						return err
					}
				}
				if unified && cooldown.ID != "" {
					return tx.Save(&cooldown).Error
				}
				return nil
			})
			if err != nil {
				if unified {
					return err
				}
				common.SysError(common.LogText("legacy notification shadow observation failed"))
			}
		}
		if unified {
			return nil
		}
		if err := legacy(ctx); err != nil {
			return err
		}
		if cooldown.ID != "" {
			return model.DB.WithContext(ctx).Save(&cooldown).Error
		}
		return nil
	})
}
