package alerting

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

// BlockedAction is a durable outcome requiring configuration or operator input,
// rather than a transient failure that should repeatedly call the same action.
type BlockedAction struct{ Code string }

func (e *BlockedAction) Error() string { return e.Code }

type ImpactDomain struct {
	Group        string   `json:"group"`
	Model        string   `json:"model"`
	Alternatives []string `json:"alternatives"`
}
type ActionImpact struct {
	Status        string         `json:"status"`
	Reason        string         `json:"reason,omitempty"`
	Domains       []ImpactDomain `json:"domains"`
	ConfigVersion string         `json:"config_version"`
}

// ChannelActionImpact is deliberately conservative. Restricted key routes and
// advanced request constraints cannot be inferred from an ability count; those
// domains return unknown instead of promising a usable alternative.
func ChannelActionImpact(tx *gorm.DB, target MonitoredTarget, spec model.AlertRuleSpec) (ActionImpact, error) {
	impact := ActionImpact{Status: "complete"}
	var channel model.Channel
	if err := tx.First(&channel, target.ID).Error; err != nil {
		return impact, err
	}
	_, _, impact.ConfigVersion = model.ResolveChannelModelIdentity(&channel, target.Model)
	if target.ConfigVersion != "" && target.ConfigVersion != impact.ConfigVersion {
		impact.Status, impact.Reason = "unknown", "channel_changed"
		return impact, nil
	}
	if !channel.GetAutoBan() {
		impact.Status, impact.Reason = "blocked", "automatic_protection_disabled"
		return impact, nil
	}
	if target.Type == "channel_model" {
		var policy model.AlertChannelPolicy
		if err := tx.First(&policy, target.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return impact, err
		}
		if policy.ProtectionMode != "scoped" {
			impact.Status, impact.Reason = "blocked", "legacy_protection_conflict"
			return impact, nil
		}
	}
	// Strict sessions and provider-owned file handles may be pinned to one
	// channel regardless of compatible ability rows. Keep these domains unknown
	// until an operator can establish coverage instead of counting false backups.
	affinity := model.CurrentRequestPolicy().Affinity
	strict := affinity.Enabled && (affinity.SessionMode == "strict" || affinity.KeepOnChannelDisabled)
	affectedModels := channel.GetModels()
	if target.Type == "channel_model" {
		affectedModels = []string{target.Model}
	}
	if affinity.Enabled {
		for _, rule := range affinity.Rules {
			if rule.SessionMode != "strict" && !(rule.SessionMode == "" && rule.SkipRetryOnFailure) {
				continue
			}
			if len(rule.ModelRegex) == 0 {
				strict = true
				break
			}
			for _, pattern := range rule.ModelRegex {
				expression, err := regexp.Compile(pattern)
				if err != nil {
					strict = true
					break
				}
				for _, name := range affectedModels {
					if expression.MatchString(name) {
						strict = true
						break
					}
				}
			}
		}
	}
	if strict {
		impact.Status, impact.Reason = "unknown", "restricted_routing_requires_review"
		return impact, nil
	}

	if channel.Type == constant.ChannelTypeGemini || channel.Type == constant.ChannelTypeVertexAi || constant.IsAdvancedCustomChannel(channel.Type) {
		impact.Status, impact.Reason = "unknown", "attachment_routing_requires_review"
		return impact, nil
	}
	var restricted int64
	if err := tx.Model(&model.Token{}).Where("status = ? AND channel_rules <> ? AND channel_rules <> ?", common.TokenStatusEnabled, "", "{}").Count(&restricted).Error; err != nil {
		return impact, err
	}
	if restricted > 0 {
		impact.Status, impact.Reason = "unknown", "restricted_routing_requires_review"
		return impact, nil
	}
	models := channel.GetModels()
	if target.Type == "channel_model" {
		models = []string{target.Model}
	}
	for _, name := range models {
		if strings.Contains(name, "realtime") {
			impact.Status, impact.Reason = "unknown", "long_connection_routing_requires_review"
			return impact, nil
		}
	}
	groups := strings.Split(channel.Group, ",")
	if len(models)*len(groups) > 200 {
		impact.Status, impact.Reason = "unknown", "routing_domain_capacity"
		return impact, nil
	}
	for _, name := range models {
		if strings.Contains(name, "*") {
			impact.Status, impact.Reason = "unknown", "wildcard_coverage"
			return impact, nil
		}
		for _, group := range groups {
			domain := ImpactDomain{Group: group, Model: name, Alternatives: []string{}}
			var abilities []model.Ability
			if err := tx.Where(map[string]any{"group": group, "enabled": true}).Where("model IN ? AND channel_id <> ?", []string{name, ratio_setting.RoutingMatchModelName(name)}, channel.Id).Limit(501).Find(&abilities).Error; err != nil {
				return impact, err
			}
			if len(abilities) > 500 {
				impact.Status, impact.Reason = "unknown", "candidate_capacity"
				return impact, nil
			}
			seen := map[int]bool{}
			for _, ability := range abilities {
				if seen[ability.ChannelId] {
					continue
				}
				seen[ability.ChannelId] = true
				var candidate model.Channel
				if err := tx.First(&candidate, ability.ChannelId).Error; err != nil {
					return impact, err
				}
				// Only compare equivalent transport/settings after excluding pinned
				// attachment and strict-session domains above.
				if candidate.Status != common.ChannelStatusEnabled || candidate.Type != channel.Type || candidate.OtherSettings != channel.OtherSettings {
					continue
				}
				key, _, _ := model.ResolveChannelModelIdentity(&candidate, name)
				keys := []string{model.AlertIdentity("channel", candidate.Id), model.AlertIdentity("channel_model", candidate.Id, "routing", key)}
				var held int64
				if err := tx.Model(&model.AlertDisableHold{}).Where("active = ? AND target_key IN ?", true, keys).Count(&held).Error; err != nil {
					return impact, err
				}
				if held > 0 {
					continue
				}
				if err := tx.Model(&model.AlertChannelModelControl{}).Where("target_key = ? AND manual_paused = ?", keys[1], true).Count(&held).Error; err != nil {
					return impact, err
				}
				if held == 0 {
					domain.Alternatives = append(domain.Alternatives, candidate.Name)
				}
			}
			impact.Domains = append(impact.Domains, domain)
			if spec.ActionGuard == nil || len(domain.Alternatives) < spec.ActionGuard.MinRemainingChannels {
				if spec.ActionGuard == nil || spec.ActionGuard.OnNoAlternative != "isolate_anyway" {
					impact.Status, impact.Reason = "blocked", "no_alternative"
				}
			}
		}
	}
	return impact, nil
}

func (e *Engine) ExecuteAction(ctx context.Context, job model.AlertJob, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, event model.AlertEvent) error {
	if !slices.ContainsFunc(spec.Actions, func(a model.AlertAction) bool { return a.Type == job.Action && a.Target == "matched_entity" }) {
		return &BlockedAction{Code: "action_not_authorized"}
	}
	if spec.ManagementMode != "platform_enforced" {
		var owner model.User
		if err := model.DB.WithContext(ctx).Select("id", "status").First(&owner, rule.OwnerUserID).Error; err != nil {
			return err
		}
		if owner.Status != common.UserStatusEnabled {
			return &BlockedAction{Code: "owner_disabled"}
		}
	} else {
		var principal model.User
		if err := model.DB.WithContext(ctx).Select("id", "status", "role").First(&principal, rule.UpdatedBy).Error; err != nil {
			return err
		}
		if principal.Role != common.RoleRootUser || principal.Status != common.UserStatusEnabled {
			return &BlockedAction{Code: "platform_authorization_revoked"}
		}
	}
	hold := model.AlertDisableHold{DedupKey: model.AlertIdentity(event.ID, job.Action, target.Key()), TargetKey: target.Key(), TargetType: target.Type, RuleID: rule.ID, IncidentID: event.IncidentID, OwnerUserID: rule.OwnerUserID, Platform: spec.ManagementMode == "platform_enforced", ConfigVersion: target.ConfigVersion}
	hold.TargetName = target.Name
	if target.Type == "api_key" {
		hold.TokenID = target.ID
	} else if target.Type == "channel" || target.Type == "channel_model" {
		hold.ChannelID, hold.ModelKey = target.ID, target.Model
	} else {
		return &BlockedAction{Code: "action_target_invalid"}
	}
	recovery, err := common.Marshal(spec.Recovery)
	if err != nil {
		return err
	}
	hold.Recovery = string(recovery)
	if spec.Recovery.Mode == "probe" {
		hold.NextProbeAt = time.Now().Add(time.Minute).UnixMilli()
	}
	if hold.ChannelID > 0 {
		// A single authority covers overlapping channel and model domains. The
		// transaction sees holds committed by the preceding lease owner.
		owner := e.BootID + "-" + job.DedupKey
		ok, err := model.AcquireResourceLease(ctx, "alerts:routing-controls", owner, time.Now().Add(30*time.Second).Unix())
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("routing control lease busy")
		}
		defer model.ReleaseResourceLease(context.Background(), "alerts:routing-controls", owner)
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = model.CommitAlertHold(bounded, &hold, rule, func(tx *gorm.DB) error {
			impact, err := ChannelActionImpact(tx, target, spec)
			if err != nil {
				return err
			}
			if impact.Status != "complete" {
				return &BlockedAction{Code: impact.Reason}
			}
			return nil
		})
		return err
	}
	if err = model.CommitAlertHold(ctx, &hold, rule, nil); err != nil {
		return err
	}
	return model.InvalidateAlertToken(hold.TokenID)
}
