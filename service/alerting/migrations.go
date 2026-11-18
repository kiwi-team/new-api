package alerting

import (
	"context"
	"database/sql"
	"math"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ImportLegacyNotification copies the selected legacy destination without
// changing its subscriptions or executor. Webhooks stay v1 unless the owner
// explicitly identifies the existing address as a native Feishu bot.
// Existing addresses are copied offline; delivery validates network access.
func ImportLegacyNotification(ctx context.Context, actor Actor, vault *TargetVault, owner int, feishu bool) (*model.AlertTarget, error) {
	if owner <= 0 || owner != actor.ID && !actor.Root() {
		return nil, ErrForbidden
	}
	if vault == nil {
		return nil, ErrTargetKeyUnavailable
	}
	var target *model.AlertTarget
	err := model.WithLegacyAlertLease(ctx, "notification_target", strconv.Itoa(owner), func(ctx context.Context) error {
		var user model.User
		if err := model.DB.WithContext(ctx).Select("id", "setting", "email").First(&user, owner).Error; err != nil {
			return err
		}
		setting := user.GetSetting()
		kind, secret := setting.NotifyType, TargetSecret{}
		switch kind {
		case "", dto.NotifyTypeEmail:
			kind, secret.Address = "email", setting.NotificationEmail
			if secret.Address == "" {
				secret.Address = user.Email
			}
		case dto.NotifyTypeWebhook:
			kind, secret.Address, secret.Secret = "webhook_v1", setting.WebhookUrl, setting.WebhookSecret
			if feishu {
				kind = "feishu"
			}
		case dto.NotifyTypeBark:
			kind, secret.Address = "bark", setting.BarkUrl
		case dto.NotifyTypeGotify:
			kind, secret.Address, secret.Token, secret.Priority = "gotify", setting.GotifyUrl, setting.GotifyToken, setting.GotifyPriority
		default:
			return ErrInvalid
		}
		if feishu && setting.NotifyType != dto.NotifyTypeWebhook || strings.TrimSpace(secret.Address) == "" {
			return ErrTargetRejected
		}
		rateKey := model.AlertIdentity("destination", kind, secret.Address, secret.Token)
		var existing model.AlertTarget
		found := model.DB.WithContext(ctx).Where("owner_user_id = ? AND type = ? AND rate_key = ?", owner, kind, rateKey).Order("id asc").Limit(1).Find(&existing)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 {
			target = &existing
			return nil
		}
		var err error
		target, err = vault.save(ctx, actor, owner, 0, TargetInput{Name: "Imported notification / " + kind, Type: kind, Enabled: true, Language: i18n.ParseAcceptLanguage(setting.Language), Credentials: &secret, CredentialAction: "replace"}, true)
		return err
	})
	return target, err
}

// Legacy targets are already configured. Their masked hints must not require
// DNS, connectivity or compliance with the policy for newly entered addresses.
func legacyTargetAddressHint(kind, address string) string {
	if kind == "email" {
		parsed, err := mail.ParseAddress(address)
		if err != nil {
			return "***"
		}
		local, domain, ok := strings.Cut(parsed.Address, "@")
		if !ok || local == "" {
			return "***"
		}
		return local[:1] + "***@" + domain
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" {
		return "***"
	}
	return u.Scheme + "://" + u.Hostname() + "/…"
}

type LegacyTokenPreview struct {
	TokenID            int    `json:"token_id"`
	Name               string `json:"name"`
	OwnerUserID        int    `json:"owner_user_id"`
	ThresholdUSD       string `json:"threshold_usd"`
	UsedQuota          int64  `json:"used_quota,string"`
	NotifiedQuota      int64  `json:"notified_quota,string"`
	LastNotifiedTime   int64  `json:"last_notified_time"`
	AcceptedUsage      int64  `json:"accepted_usage,string"`
	DestinationPresent bool   `json:"destination_present"`
	Ambiguous          bool   `json:"ambiguous"`
	SnapshotHash       string `json:"snapshot_hash"`
}

func LegacyTokenSnapshot(ctx context.Context, tokenID int) (LegacyTokenPreview, error) {
	var result LegacyTokenPreview
	options := &sql.TxOptions{ReadOnly: true}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token model.Token
		if err := tx.First(&token, tokenID).Error; err != nil {
			return err
		}
		if token.AlertThreshold <= 0 || math.IsNaN(token.AlertThreshold) || math.IsInf(token.AlertThreshold, 0) {
			return ErrInvalid
		}
		var user model.User
		if err := tx.Select("id", "setting").First(&user, token.UserId).Error; err != nil {
			return err
		}
		var used, accepted int64
		if err := tx.Model(&model.QuotaData{}).Where("token_id = ?", tokenID).Select("COALESCE(SUM(quota),0)").Scan(&used).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AlertObservation{}).Where("namespace = ? AND token_id = ?", "usage", tokenID).Select("COALESCE(SUM(quota),0)").Scan(&accepted).Error; err != nil {
			return err
		}
		result = LegacyTokenPreview{TokenID: token.Id, Name: token.Name, OwnerUserID: token.UserId, ThresholdUSD: decimal.NewFromFloat(token.AlertThreshold).String(), UsedQuota: used, NotifiedQuota: int64(token.AlertNotifiedQuota), LastNotifiedTime: token.AlertLastNotifiedTime, AcceptedUsage: accepted, DestinationPresent: user.GetSetting().WebhookUrl != ""}
		// A nonzero checkpoint without delivery evidence can have been advanced by
		// the old missing-destination branch. It cannot be reconstructed automatically.
		result.Ambiguous = token.AlertNotifiedQuota > 0
		result.SnapshotHash = model.AlertIdentity(token.Id, token.UserId, result.ThresholdUSD, token.AlertNotifiedQuota, token.AlertLastNotifiedTime, model.AlertIdentity(user.GetSetting().WebhookUrl))
		return nil
	}, options)
	return result, err
}

// ImportLegacyToken creates an observation-only rule and encrypted native Feishu
// target. The shared executor marker remains legacy until a separate cutover.
func ImportLegacyToken(ctx context.Context, actor Actor, vault *TargetVault, tokenID int) (model.AlertMigration, error) {
	var migration model.AlertMigration
	if !actor.Root() {
		return migration, ErrForbidden
	}
	if vault == nil {
		return migration, ErrTargetKeyUnavailable
	}
	err := model.WithLegacyAlertLease(ctx, "token_spend", strconv.Itoa(tokenID), func(ctx context.Context) error {
		id := model.AlertIdentity("legacy", "token_spend", strconv.Itoa(tokenID))
		existing := model.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&migration)
		if existing.Error != nil {
			return existing.Error
		}
		if existing.RowsAffected > 0 {
			return nil
		}
		snapshot, err := LegacyTokenSnapshot(ctx, tokenID)
		if err != nil {
			return err
		}
		var user model.User
		if err = model.DB.WithContext(ctx).First(&user, snapshot.OwnerUserID).Error; err != nil {
			return err
		}
		address := user.GetSetting().WebhookUrl
		target := model.AlertTarget{OwnerUserID: user.Id, Name: "Migrated key alerts", Type: "feishu", Enabled: address != "", Language: i18n.ParseAcceptLanguage(user.GetSetting().Language), Revision: 1}
		if address != "" {
			secret := TargetSecret{Address: address}
			hint := legacyTargetAddressHint("feishu", address)
			sealed, err := vault.Seal(user.Id, "feishu", secret)
			if err != nil {
				return err
			}
			target.AddressHint, target.Ciphertext, target.KeyID, target.RateKey = hint, sealed, vault.active, model.AlertIdentity("destination", "feishu", address, "")
		}
		tokenName := snapshot.Name
		if len(tokenName) > 120 {
			tokenName = string([]rune(tokenName)[:min(len([]rune(tokenName)), 30)])
		}
		spec := model.AlertRuleSpec{QuotaUnit: int64(common.QuotaPerUnit), Name: tokenName + " / spending", Category: "usage", Severity: "P2", Enabled: address != "", ManagementMode: "user_managed", Scope: model.AlertScope{Type: "api_key", Selector: "ids", EntityIDs: []int{tokenID}, GroupMode: "per_entity"}, Condition: model.AlertCondition{Metric: "spend_net", Operator: "gte", ThresholdUSD: snapshot.ThresholdUSD, RecoveryThresholdUSD: "0", Window: model.AlertWindow{Type: "cumulative", Timezone: "Asia/Shanghai"}}, Trigger: model.AlertTrigger{Mode: "milestone", StepUSD: snapshot.ThresholdUSD, RepeatSeconds: 1800}, Recovery: model.AlertRecovery{Mode: "manual"}}
		checkpoint, err := common.Marshal(snapshot)
		if err != nil {
			return err
		}
		offset := decimal.NewFromInt(snapshot.UsedQuota).Sub(decimal.NewFromInt(snapshot.NotifiedQuota)).Sub(decimal.NewFromInt(snapshot.AcceptedUsage))
		if offset.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || offset.LessThan(decimal.NewFromInt(math.MinInt64)) {
			return ErrInvalid
		}
		now := time.Now().UnixMilli()
		return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := model.LockAlertImportOwner(tx, user.Id, 1); err != nil {
				return err
			}
			target.CreatedAt, target.UpdatedAt = now, now
			if err := tx.Create(&target).Error; err != nil {
				return err
			}
			spec.Notify.TargetIDs = []int64{target.ID}
			if err := validate(ctx, tx, actor, user.Id, &spec); err != nil {
				return err
			}
			encoded, err := common.Marshal(spec)
			if err != nil {
				return err
			}
			rule := model.AlertRule{OwnerUserID: user.Id, CreatedBy: actor.ID, UpdatedBy: actor.ID, Name: spec.Name, Enabled: spec.Enabled, Revision: 1, Generation: 1, EffectiveAt: now, NextEvaluationAt: now, Config: string(encoded), CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AlertRuleRevision{RuleID: rule.ID, Revision: 1, Config: rule.Config, CreatedBy: actor.ID, CreatedAt: now}).Error; err != nil {
				return err
			}
			status := "observing"
			if snapshot.Ambiguous {
				status = "ambiguous"
			}
			migration = model.AlertMigration{ID: id, Kind: "token_spend", ObjectID: strconv.Itoa(tokenID), OwnerUserID: user.Id, RuleID: rule.ID, Executor: "legacy", Status: status, Revision: 1, MetricOffset: offset.IntPart(), Checkpoint: string(checkpoint), LegacyConfigHash: model.AlertIdentity(snapshot.ThresholdUSD), ImportedAt: now, UpdatedAt: now, UpdatedBy: actor.ID}
			if err := tx.Create(&migration).Error; err != nil {
				return err
			}
			return tx.Create(&model.AlertAudit{ActorID: actor.ID, OwnerUserID: user.Id, Operation: "migration.import", ObjectID: rule.ID, Revision: 1, CreatedAt: now}).Error
		})
	})
	return migration, err
}

// SwitchLegacyToken re-reads the compatible executor's checkpoint under the
// shared lease. Statistics and alert receipts share a transaction upstream, so
// an absolute offset bridges events queued across the cutover without a time gap.
func SwitchLegacyToken(ctx context.Context, actor Actor, id string, expected int64, operation, snapshotHash string, acceptAmbiguous bool, expectedRuleRevision ...int64) error {
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
	if migration.Kind != "token_spend" || migration.Revision != expected {
		return model.ErrAlertConflict
	}
	return model.WithLegacyAlertLease(ctx, migration.Kind, migration.ObjectID, func(ctx context.Context) error {
		tokenID, err := strconv.Atoi(migration.ObjectID)
		if err != nil {
			return ErrInvalid
		}
		snapshot, err := LegacyTokenSnapshot(ctx, tokenID)
		if err != nil {
			return err
		}
		if snapshot.SnapshotHash != snapshotHash {
			return model.ErrAlertConflict
		}
		if operation == "cutover" {
			if !common.AlertingEnabled || common.AlertingShadowMode || !common.DataExportEnabled || snapshot.Ambiguous && !acceptAmbiguous {
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
		return model.WithAlertMigration(ctx, migration, func(tx *gorm.DB, lockedRule *model.AlertRule, lockedMigration *model.AlertMigration) error {
			rule, current := *lockedRule, *lockedMigration
			if len(expectedRuleRevision) > 0 && rule.Revision != expectedRuleRevision[0] {
				return model.ErrAlertConflict
			}
			if operation == "cutover" && rule.DeletedAt.Valid {
				return &BlockedAction{Code: "migration_rule_changed"}
			}
			spec, err := model.AlertRuleConfig(rule)
			if err != nil {
				return err
			}
			if spec.QuotaUnit != int64(common.QuotaPerUnit) || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != "cumulative" || spec.Trigger.Mode != "milestone" || len(spec.Actions) > 0 || len(spec.Scope.EntityIDs) != 1 || spec.Scope.EntityIDs[0] != tokenID {
				return &BlockedAction{Code: "migration_rule_changed"}
			}
			step, err := USDQuota(spec.Trigger.StepUSD)
			if err != nil {
				return err
			}
			if operation == "cutover" {
				if !rule.Enabled || len(spec.Notify.TargetIDs) == 0 {
					return &BlockedAction{Code: "migration_not_ready"}
				}
				var valid int64
				if err = tx.Model(&model.AlertTarget{}).Where("id IN ? AND enabled = ? AND ciphertext <> ?", spec.Notify.TargetIDs, true, "").Count(&valid).Error; err != nil {
					return err
				}
				if valid != int64(len(spec.Notify.TargetIDs)) {
					return &BlockedAction{Code: "migration_not_ready"}
				}
			}

			expectedExecutor := "legacy"
			next := "unified"
			if operation == "rollback" {
				expectedExecutor, next = "unified", "legacy"
			}
			if current.Executor != expectedExecutor {
				return model.ErrAlertConflict
			}
			var used, accepted int64
			if err = tx.Model(&model.QuotaData{}).Where("token_id = ?", tokenID).Select("COALESCE(SUM(quota),0)").Scan(&used).Error; err != nil {
				return err
			}
			if err = tx.Model(&model.AlertObservation{}).Where("namespace = ? AND token_id = ?", "usage", tokenID).Select("COALESCE(SUM(quota),0)").Scan(&accepted).Error; err != nil {
				return err
			}
			if operation == "rollback" && current.MetricStartAt > 0 {
				if err = tx.Model(&model.AlertObservation{}).Where("namespace = ? AND token_id = ? AND occurred_at >= ?", "usage", tokenID, current.MetricStartAt).Select("COALESCE(SUM(quota),0)").Scan(&accepted).Error; err != nil {
					return err
				}
			}
			offset := decimal.NewFromInt(used).Sub(decimal.NewFromInt(snapshot.NotifiedQuota)).Sub(decimal.NewFromInt(accepted))
			now := time.Now().UnixMilli()
			if operation == "rollback" {
				var pending int64
				if err = tx.Model(&model.AlertJob{}).Where("rule_id = ? AND status IN ?", rule.ID, []string{"pending", "running", "retry_wait"}).Count(&pending).Error; err != nil {
					return err
				}
				if pending > 0 {
					return &BlockedAction{Code: "migration_jobs_pending"}
				}
				var state model.AlertRuleState
				if err = tx.Where("id = ?", model.AlertIdentity("state", rule.ID, rule.Generation, "api_key", tokenID, "")).Limit(1).Find(&state).Error; err != nil {
					return err
				}
				remainder := decimal.NewFromInt(accepted).Add(decimal.NewFromInt(current.MetricOffset)).Sub(decimal.NewFromInt(state.HighestTier).Mul(decimal.NewFromInt(step)))
				baseline := decimal.NewFromInt(used).Sub(remainder)
				if baseline.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || baseline.LessThan(decimal.NewFromInt(math.MinInt64)) {
					return ErrInvalid
				}
				if err = tx.Model(&model.Token{}).Where("id = ?", tokenID).Updates(map[string]any{"alert_notified_quota": baseline.IntPart(), "alert_last_notified_time": now / 1000, "alert_threshold": spec.Trigger.StepUSD}).Error; err != nil {
					return err
				}
			}
			if offset.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || offset.LessThan(decimal.NewFromInt(math.MinInt64)) {
				return ErrInvalid
			}
			if operation == "cutover" {
				current.MetricOffset, current.CutoverAt, current.MetricStartAt = offset.IntPart(), now, 0
			}
			checkpoint, err := common.Marshal(snapshot)
			if err != nil {
				return err
			}
			return model.CommitAlertMigration(tx, &rule, &current, actor.ID, next, now, string(checkpoint))
		})
	})
}
