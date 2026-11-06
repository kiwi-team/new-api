package model

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResolveChannelModelIdentity follows the existing ability fallback without
// collapsing concrete models to a wildcard or a billing/upstream alias.
func ResolveChannelModelIdentity(channel *Channel, requested string) (key, matched, version string) {
	models := channel.GetModels()
	key, matched = requested, requested
	if !slices.Contains(models, requested) {
		base := reasoning.BaseModelName(requested)
		fallback := ratio_setting.RoutingMatchModelName(requested)
		switch {
		case slices.Contains(models, base):
			key, matched = base, base
		case slices.Contains(models, fallback):
			matched = fallback
			if !strings.Contains(fallback, "*") {
				key = fallback
			} else {
				key = base
			}
		}
	}
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = *channel.BaseURL
	}
	version = AlertIdentity("channel-config-v2", channel.Id, channel.Type, baseURL, channel.Models, channel.ModelMapping, channel.Group, channel.OtherSettings)
	return
}

func SetAlertChannelPolicy(ctx context.Context, channelID, actor int, mode string, revision int64) error {
	if mode != "legacy" && mode != "scoped" {
		return ErrAlertConflict
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id").First(&channel, channelID).Error; err != nil {
			return err
		}
		row := AlertChannelPolicy{ChannelID: channelID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&row, channelID).Error; err != nil {
			return err
		}
		if row.Revision != revision {
			return ErrAlertConflict
		}
		if mode == "legacy" {
			var active int64
			if err := tx.Model(&AlertDisableHold{}).Where("channel_id = ? AND target_type = ? AND active = ?", channelID, "channel_model", true).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return ErrAlertConflict
			}
		}
		row.ProtectionMode, row.Revision, row.UpdatedBy, row.UpdatedAt = mode, revision+1, actor, time.Now().UnixMilli()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return tx.Create(&AlertAudit{ActorID: actor, Operation: "channel.protection_mode", ObjectID: int64(channelID), Revision: row.Revision, CreatedAt: row.UpdatedAt}).Error
	})
}

func AlertChannelProtectionMode(ctx context.Context, channelID int) (string, error) {
	if !common.AlertingControlsEnabled {
		return "legacy", nil
	}
	var row AlertChannelPolicy
	err := DB.WithContext(ctx).First(&row, channelID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "legacy", nil
	}
	return row.ProtectionMode, err
}

// CommitAlertHold serializes overlapping channel actions with the same SQL
// authority as candidate checks. The guard runs inside this short transaction;
// it must use tx and must never call an upstream provider.
func CommitAlertHold(ctx context.Context, hold *AlertDisableHold, rule AlertRule, guard func(*gorm.DB) error) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current AlertRule
		if err := lockForUpdate(tx).First(&current, rule.ID).Error; err != nil {
			return err
		}
		if !current.Enabled || current.Revision != rule.Revision {
			return ErrAlertConflict
		}
		var exists AlertDisableHold
		if err := tx.Where("dedup_key = ?", hold.DedupKey).First(&exists).Error; err == nil {
			*hold = exists
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if hold.ChannelID > 0 {
			var channel Channel
			if err := lockForUpdate(tx).Select("id").First(&channel, hold.ChannelID).Error; err != nil {
				return err
			}
		}
		if guard != nil {
			if err := guard(tx); err != nil {
				return err
			}
		}
		if hold.TokenID > 0 {
			var token Token
			if err := lockForUpdate(tx).First(&token, hold.TokenID).Error; err != nil {
				return err
			}
			if token.UserId != hold.OwnerUserID {
				return ErrAlertConflict
			}
			// Preserve a manual/legacy disable. Only the last hold with the exact
			// version produced by us may later restore the original enabled state.
			if token.Status == common.TokenStatusEnabled {
				hold.TokenVersion = token.AuthVersion + 1
				if err := tx.Model(&token).Updates(map[string]any{"status": common.TokenStatusDisabled, "auth_version": hold.TokenVersion}).Error; err != nil {
					return err
				}
			}
		}
		hold.Active, hold.CreatedAt = true, time.Now().UnixMilli()
		if err := tx.Create(hold).Error; err != nil {
			return err
		}
		return tx.Create(&AlertAudit{ActorID: rule.UpdatedBy, OwnerUserID: hold.OwnerUserID, Operation: "action.hold", ObjectID: hold.ID, Revision: rule.Revision, CreatedAt: hold.CreatedAt}).Error
	})
}

// ReleaseAlertHold removes one source only. It never enables a channel or
// overrides a token edit made after automatic disabling.
func ReleaseAlertHold(ctx context.Context, id int64, actor int, root bool) (*AlertDisableHold, error) {
	var hold AlertDisableHold
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return ReleaseAlertHoldTx(tx, &hold, id, actor, root) })
	if err == nil && hold.TokenID > 0 {
		_ = InvalidateAlertToken(hold.TokenID)
	}
	return &hold, err
}

// ReleaseAlertHoldTx permits recovery to commit hold release, incident state,
// and recovery notification jobs atomically under the rule's revision fence.
func ReleaseAlertHoldTx(tx *gorm.DB, hold *AlertDisableHold, id int64, actor int, root bool) error {
	var initial AlertDisableHold
	if err := tx.First(&initial, id).Error; err != nil {
		return err
	}
	if initial.TokenID > 0 {
		var token Token
		if err := lockForUpdate(tx).Select("id").First(&token, initial.TokenID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}

	if err := lockForUpdate(tx).First(hold, id).Error; err != nil {
		return err
	}
	if !root && (hold.OwnerUserID != actor || hold.Platform) {
		return gorm.ErrRecordNotFound
	}
	if !hold.Active {
		return nil
	}
	hold.Active, hold.ReleasedBy, hold.ReleasedAt = false, actor, time.Now().UnixMilli()
	if err := tx.Save(hold).Error; err != nil {
		return err
	}
	if hold.TokenID > 0 {
		var remaining int64
		if err := tx.Model(&AlertDisableHold{}).Where("token_id = ? AND active = ?", hold.TokenID, true).Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			// Another hold may have been the source that actually changed the
			// token. Read its version too, without treating manual disables as ours.
			var source AlertDisableHold
			if err := tx.Where("token_id = ? AND token_version > 0", hold.TokenID).Order("token_version desc").First(&source).Error; err == nil {
				if err := tx.Model(&Token{}).Where("id = ? AND user_id = ? AND status = ? AND auth_version = ?", hold.TokenID, hold.OwnerUserID, common.TokenStatusDisabled, source.TokenVersion).Updates(map[string]any{"status": common.TokenStatusEnabled, "auth_version": gorm.Expr("auth_version + 1")}).Error; err != nil {
					return err
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
	}
	return tx.Create(&AlertAudit{ActorID: actor, OwnerUserID: hold.OwnerUserID, Operation: "hold.release", ObjectID: hold.ID, CreatedAt: hold.ReleasedAt}).Error
}

func ReleaseRecoveredAlertHoldTx(tx *gorm.DB, expected AlertDisableHold, rule AlertRule) error {
	var principal User
	if err := tx.Select("id", "role", "status").First(&principal, rule.UpdatedBy).Error; err != nil {
		return err
	}
	if principal.Role != common.RoleRootUser || principal.Status != common.UserStatusEnabled {
		return ErrAlertConflict
	}
	var channel Channel
	if err := lockForUpdate(tx).First(&channel, expected.ChannelID).Error; err != nil {
		return err
	}
	_, _, version := ResolveChannelModelIdentity(&channel, expected.ModelKey)
	if version != expected.ConfigVersion {
		return ErrAlertConflict
	}
	credentialVersion := AlertIdentity("recovery-credential-v1", channel.Key)
	if expected.ProbeCredentialVersion == "" || expected.ProbeCredentialVersion != credentialVersion {
		return ErrAlertConflict
	}
	var current AlertDisableHold
	if err := tx.First(&current, expected.ID).Error; err != nil {
		return err
	}
	if !current.Active || current.RuleID != rule.ID || current.ConfigVersion != expected.ConfigVersion || current.ProbeCredentialVersion != credentialVersion {
		return ErrAlertConflict
	}
	return ReleaseAlertHoldTx(tx, &current, expected.ID, rule.UpdatedBy, true)
}

func InvalidateAlertToken(tokenID int) error {
	token, err := GetTokenById(tokenID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return invalidateTokenCacheForMutation(token.Key)
}

// SetManualAlertModelControl changes only the manual source. Neither resuming
// this control nor releasing one alert hold clears another effective source.
func SetManualAlertModelControl(ctx context.Context, channelID, actor int, requested string, paused bool, revision int64, expectedConfig string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		key, matched, version := ResolveChannelModelIdentity(&channel, requested)
		if version != expectedConfig || requested == "" || strings.Contains(requested, "*") {
			return ErrAlertConflict
		}
		if paused && !slices.Contains(channel.GetModels(), matched) {
			return ErrAlertConflict
		}
		// A removed declaration can still have a manual source. Root may clear
		// that exact old identity without temporarily restoring the declaration.
		if !paused {
			key = requested
		}
		identity := AlertIdentity("channel_model", channelID, "routing", key)
		row := AlertChannelModelControl{TargetKey: identity, ChannelID: channelID, ModelKey: key}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&row, "target_key = ?", identity).Error; err != nil {
			return err
		}
		if row.Revision != revision {
			return ErrAlertConflict
		}
		row.ManualPaused, row.Revision, row.UpdatedBy, row.UpdatedAt = paused, revision+1, actor, time.Now().UnixMilli()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return tx.Create(&AlertAudit{ActorID: actor, Operation: "channel_model.manual", ObjectID: int64(channelID), Revision: row.Revision, CreatedAt: row.UpdatedAt}).Error
	})
}
