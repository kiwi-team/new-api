package alerting

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

type ProbeResult struct {
	Success       bool
	DurationMS    *int64
	TTFTMS        *int64
	ConfigVersion string
}

// RecoverHolds uses independent, explicitly marked probes. No-data production
// windows after isolation are not evidence of recovery.
func (e *Engine) RecoverHolds(ctx context.Context, now time.Time) error {
	if !common.AlertingActionsEnabled || common.AlertingShadowMode || e.Probe == nil {
		return nil
	}
	var holds []model.AlertDisableHold
	if err := model.DB.WithContext(ctx).Where("active = ? AND next_probe_at > 0 AND next_probe_at <= ?", true, now.UnixMilli()).Order("next_probe_at asc").Limit(10).Find(&holds).Error; err != nil {
		return err
	}
	for _, hold := range holds {
		owner := e.BootID + "-probe-" + strconv.FormatInt(hold.ID, 10)
		key := "alerts:probe:" + strconv.FormatInt(hold.ID, 10)
		ok, err := model.AcquireResourceLease(ctx, key, owner, time.Now().Add(90*time.Second).Unix())
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		work, cancel := context.WithTimeout(ctx, 45*time.Second)
		err = e.recoverHold(work, hold, now)
		cancel()
		release, done := context.WithTimeout(context.Background(), 2*time.Second)
		_ = model.ReleaseResourceLease(release, key, owner)
		done()
		if err != nil {
			// One malformed or unavailable object must not starve other holds.
			_ = model.DB.WithContext(ctx).Model(&model.AlertDisableHold{}).Where("id = ? AND active = ?", hold.ID, true).Updates(map[string]any{"probe_count": 0, "probe_started_at": 0, "last_probe_status": "probe_unavailable", "next_probe_at": now.Add(5 * time.Minute).UnixMilli()}).Error
		}
	}
	return nil
}

func (e *Engine) recoverHold(ctx context.Context, hold model.AlertDisableHold, now time.Time) error {
	var recovery model.AlertRecovery
	if common.UnmarshalJsonStr(hold.Recovery, &recovery) != nil || recovery.Mode != "probe" {
		return model.DB.WithContext(ctx).Model(&model.AlertDisableHold{}).Where("id = ? AND active = ?", hold.ID, true).Updates(map[string]any{"probe_count": 0, "probe_started_at": 0, "last_probe_status": "configuration_invalid", "next_probe_at": 0}).Error
	}
	var rule model.AlertRule
	if err := model.DB.WithContext(ctx).First(&rule, hold.RuleID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return model.DB.WithContext(ctx).Model(&model.AlertDisableHold{}).Where("id = ? AND active = ?", hold.ID, true).Updates(map[string]any{"probe_count": 0, "probe_started_at": 0, "last_probe_status": "rule_unavailable", "next_probe_at": 0}).Error
	}
	spec, err := model.AlertRuleConfig(rule)
	if err != nil {
		return err
	}
	if !rule.Enabled || model.AlertIdentity(spec.Recovery) != model.AlertIdentity(recovery) {
		status, next := "rule_paused", now.Add(5*time.Minute).UnixMilli()
		if rule.Enabled {
			status, next = "rule_changed", 0
		}
		return model.DB.WithContext(ctx).Model(&model.AlertDisableHold{}).Where("id = ? AND active = ?", hold.ID, true).Updates(map[string]any{"probe_count": 0, "probe_started_at": 0, "last_probe_status": status, "next_probe_at": next}).Error
	}
	var principal model.User
	if err := model.DB.WithContext(ctx).Select("id", "role", "status").First(&principal, rule.UpdatedBy).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if principal.Role != common.RoleRootUser || principal.Status != common.UserStatusEnabled {
		return model.DB.WithContext(ctx).Model(&model.AlertDisableHold{}).Where("id = ? AND active = ?", hold.ID, true).Updates(map[string]any{"probe_count": 0, "probe_started_at": 0, "last_probe_status": "authorization_revoked", "next_probe_at": 0}).Error
	}

	// Whole-channel probes need an explicit complete model set. An arbitrary
	// channel test model cannot prove recovery of every affected model.
	var channel model.Channel
	if err := model.DB.WithContext(ctx).Select("id", "key").First(&channel, hold.ChannelID).Error; err != nil {
		return err
	}
	credentialVersion := model.AlertIdentity("recovery-credential-v1", channel.Key)
	models := []string{hold.ModelKey}
	if hold.TargetType == "channel" {
		models = recovery.Models
	}
	qualified := len(models) > 0
	for _, name := range models {
		probe := hold
		probe.ModelKey = name
		result := e.Probe(ctx, probe)
		qualified = qualified && result.Success && result.ConfigVersion == hold.ConfigVersion
		if recovery.DurationLimitMS > 0 {
			qualified = qualified && result.DurationMS != nil && *result.DurationMS <= recovery.DurationLimitMS
		}
		if recovery.TTFTLimitMS > 0 {
			qualified = qualified && result.TTFTMS != nil && *result.TTFTMS <= recovery.TTFTLimitMS
		}
		if !qualified {
			break
		}
	}
	count, started, status := hold.ProbeCount, hold.ProbeStartedAt, "qualified"
	var current model.Channel
	if err := model.DB.WithContext(ctx).Select("id", "key").First(&current, hold.ChannelID).Error; err != nil {
		return err
	}
	qualified = qualified && credentialVersion == model.AlertIdentity("recovery-credential-v1", current.Key)
	if hold.ProbeCredentialVersion != credentialVersion {
		// Access tokens may rotate without changing public routing identity.
		// Recovery must still collect a fresh sequence under the current keys.
		count, started = 0, 0
	}
	if !qualified || hold.LastProbeAt > 0 && now.UnixMilli()-hold.LastProbeAt > 180000 {
		count, started, status = 0, 0, "not_qualified"
	}
	if qualified {
		if started == 0 {
			started = now.UnixMilli()
		}
		count++
	}
	updates := map[string]any{"probe_count": count, "probe_started_at": started, "probe_credential_version": credentialVersion, "last_probe_at": now.UnixMilli(), "last_probe_status": status, "next_probe_at": now.Add(time.Minute).UnixMilli()}
	resultUpdate := model.DB.WithContext(ctx).Model(&model.AlertDisableHold{}).Where("id = ? AND active = ? AND last_probe_at = ? AND config_version = ?", hold.ID, true, hold.LastProbeAt, hold.ConfigVersion).Updates(updates)
	if resultUpdate.Error != nil {
		return resultUpdate.Error
	}
	if resultUpdate.RowsAffected != 1 {
		return model.ErrAlertConflict
	}
	if count < recovery.Successes || now.UnixMilli()-started < recovery.ObserveSeconds*1000 {
		return nil
	}
	hold.ProbeCredentialVersion = credentialVersion
	target := MonitoredTarget{Type: hold.TargetType, ID: hold.ChannelID, Model: hold.ModelKey, ConfigVersion: hold.ConfigVersion}
	// A currently overdue production attempt vetoes release even if small
	// probes succeed. Unknown lifecycle coverage also vetoes release.
	check := spec
	check.Condition = model.AlertCondition{Metric: "inflight_overdue_count", LatencyBasis: "channel_attempt", DeadlineKind: "total", DeadlineMS: max(recovery.DurationLimitMS, 30000), ThresholdCount: 1}
	m, err := Measure(ctx, rule, check, target, now)
	if err != nil {
		return err
	}
	if m.Status != "complete" || m.Value > 0 {
		return nil
	}
	return model.WithAlertState(ctx, rule, target.Type, target.ID, target.Model, func(tx *gorm.DB, state *model.AlertRuleState) error {
		if err := model.ReleaseRecoveredAlertHoldTx(tx, hold, rule); err != nil {
			return err
		}
		if err := tx.Model(&model.AlertIncident{}).Where("id = ?", hold.IncidentID).Updates(map[string]any{"status": "resolved", "resolved_at": now.UnixMilli()}).Error; err != nil {
			return err
		}
		// An old hold may outlive a closed incident. Releasing that source must
		// not resolve or emit recovery for a newer incident on the same target.
		if state.IncidentID != hold.IncidentID {
			return nil
		}
		state.Status = "resolved"
		if spec.Notify.OnRecovery {
			return e.createEvent(tx, rule, spec, target, m, state, "recovery_probe", strconv.FormatInt(hold.ID, 10), now.UnixMilli())
		}
		return nil
	})
}
