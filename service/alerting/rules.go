// Package alerting owns rules, measurements, delivery and isolation state.
// Billing authorities and real provider probes remain in the host service.
package alerting

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("alert access denied")
var ErrInvalid = errors.New("invalid alert configuration")
var alertUSDPattern = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,8})?$`)

type Actor struct {
	ID, Role int
	ReadAll  bool
}

func (a Actor) Root() bool { return a.Role == common.RoleRootUser }

var Metrics = []string{"legacy_notice", "spend_net", "request_count", "input_tokens", "output_tokens", "cached_tokens", "cache_write_5m_tokens", "cache_write_1h_tokens", "success_rate", "failure_rate", "timeout_rate", "timeout_count", "consecutive_failures", "ttft_ms", "duration_ms", "slow_request_count", "slow_request_rate", "inflight_overdue_count", "task_failure_count", "task_overdue", "evaluation_lag", "source_lag", "delivery_backlog", "balance_remaining", "budget_remaining_ratio"}

func USDQuota(value string) (int64, error) {
	return usdQuota(value, int64(common.QuotaPerUnit))
}

func usdQuota(value string, unit int64) (int64, error) {
	if !alertUSDPattern.MatchString(value) {
		return 0, ErrInvalid
	}
	d, err := decimal.NewFromString(value)
	if err != nil || d.IsNegative() {
		return 0, ErrInvalid
	}
	quota, err := common.WalletQuotaFromDecimalStrict(d.Mul(decimal.NewFromInt(unit)))
	return int64(quota), err
}

// Validate checks ownership before returning any target or token information.
func Validate(ctx context.Context, actor Actor, owner int, spec *model.AlertRuleSpec) error {
	return validate(ctx, model.DB, actor, owner, spec)
}

func validate(ctx context.Context, db *gorm.DB, actor Actor, owner int, spec *model.AlertRuleSpec) error {
	if spec.QuotaUnit == 0 {
		spec.QuotaUnit = int64(common.QuotaPerUnit)
	}
	if spec.QuotaUnit != int64(common.QuotaPerUnit) {
		return ErrInvalid
	}
	if actor.ID <= 0 || (!actor.Root() && owner != actor.ID) {
		return ErrForbidden
	}
	if len(strings.TrimSpace(spec.Name)) == 0 || len(spec.Name) > 160 || !slices.Contains(Metrics, spec.Condition.Metric) {
		return ErrInvalid
	}
	if !slices.Contains([]string{"P0", "P1", "P2", "P3"}, spec.Severity) {
		return ErrInvalid
	}
	if !slices.Contains([]string{"report", "usage", "health", "system"}, spec.Category) {
		return ErrInvalid
	}
	if spec.ManagementMode == "" {
		spec.ManagementMode = "user_managed"
	}
	if !slices.Contains([]string{"user_managed", "platform_enforced"}, spec.ManagementMode) {
		return ErrInvalid
	}
	if spec.ManagementMode == "platform_enforced" && !actor.Root() {
		return ErrForbidden
	}
	scope := &spec.Scope
	if !slices.Contains([]string{"account", "api_key", "channel", "channel_model", "system", "uid_budget", "project_budget"}, scope.Type) {
		return ErrInvalid
	}
	platform := PlatformScope(scope.Type)
	if platform && (!actor.Root() || owner != 0 || spec.ManagementMode != "platform_enforced") {
		return ErrForbidden
	}
	if !platform && owner <= 0 {
		return ErrInvalid
	}
	if scope.GroupMode == "" {
		scope.GroupMode = "per_entity"
	}
	if !slices.Contains([]string{"per_entity", "aggregate"}, scope.GroupMode) {
		return ErrInvalid
	}
	if platform && scope.GroupMode != "per_entity" {
		return ErrInvalid
	}
	if len(scope.EntityIDs) > 50 || len(scope.ChannelIDs) > 50 || len(scope.ModelSelector.Models) > 100 || len(spec.Notify.TargetIDs) > 10 || len(spec.Actions) > 2 {
		return ErrInvalid
	}
	if scope.Type == "account" {
		scope.GroupMode = "aggregate"
	}
	if scope.Type == "api_key" {
		if scope.Selector == "" {
			scope.Selector = "ids"
		}
		if !slices.Contains([]string{"ids", "all_owned"}, scope.Selector) {
			return ErrInvalid
		}
		if scope.Selector == "all_owned" && !scope.IncludeFuture && len(scope.EntityIDs) == 0 {
			if err := db.WithContext(ctx).Model(&model.Token{}).Where("user_id = ?", owner).Order("id asc").Limit(51).Pluck("id", &scope.EntityIDs).Error; err != nil {
				return err
			}
			if len(scope.EntityIDs) > 50 {
				return ErrInvalid
			}
		}
		if scope.Selector == "ids" || !scope.IncludeFuture {
			if len(scope.EntityIDs) == 0 {
				return ErrInvalid
			}
			ids := slices.Clone(scope.EntityIDs)
			slices.Sort(ids)
			ids = slices.Compact(ids)
			var count int64
			if err := db.WithContext(ctx).Model(&model.Token{}).Where("id IN ? AND user_id = ?", ids, owner).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return ErrForbidden
			}
			scope.EntityIDs = ids
		}
	}
	if scope.Type == "channel" || scope.Type == "channel_model" {
		if len(scope.ChannelIDs) == 0 {
			return ErrInvalid
		}
		ids := slices.Clone(scope.ChannelIDs)
		slices.Sort(ids)
		ids = slices.Compact(ids)
		var count int64
		if err := db.WithContext(ctx).Model(&model.Channel{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return ErrInvalid
		}
		scope.ChannelIDs = ids
		if scope.Type == "channel_model" {
			if scope.ModelSelector.Mode == "" {
				scope.ModelSelector.Mode = "explicit"
			}
			if !slices.Contains([]string{"explicit", "all_configured"}, scope.ModelSelector.Mode) {
				return ErrInvalid
			}
			if scope.ModelSelector.Mode == "explicit" && len(scope.ModelSelector.Models) == 0 {
				return ErrInvalid
			}
			if scope.ModelSelector.Mode == "all_configured" && !scope.ModelSelector.IncludeFuture && len(scope.ModelSelector.Models) == 0 {
				var channels []model.Channel
				if err := db.WithContext(ctx).Select("id", "models").Where("id IN ?", ids).Find(&channels).Error; err != nil {
					return err
				}
				for _, channel := range channels {
					names, err := configuredAlertModels(ctx, channel)
					if err != nil {
						return err
					}
					scope.ModelSelector.Models = append(scope.ModelSelector.Models, names...)
				}
				slices.Sort(scope.ModelSelector.Models)
				scope.ModelSelector.Models = slices.Compact(scope.ModelSelector.Models)
				if len(scope.ModelSelector.Models) > 100 {
					return ErrInvalid
				}
			}
			var declared []model.Channel
			if err := db.WithContext(ctx).Select("id", "models").Where("id IN ?", ids).Find(&declared).Error; err != nil {
				return err
			}
			for _, name := range scope.ModelSelector.Models {
				supported := false
				for _, channel := range declared {
					_, matched, _ := model.ResolveChannelModelIdentity(&channel, name)
					if slices.Contains(channel.GetModels(), matched) {
						supported = true
						break
					}
				}
				if !supported {
					return ErrInvalid
				}
			}
			for _, name := range scope.ModelSelector.Models {
				if len(name) == 0 || len(name) > 4096 || strings.ContainsAny(name, "\x00\r\n") {
					return ErrInvalid
				}
			}
		}
	}
	c := &spec.Condition

	if c.Metric == "legacy_notice" && (!actor.Root() || spec.ManagementMode != "platform_enforced" || !IsLegacyNoticeKind(c.LegacySignal) || len(spec.Actions) > 0 || spec.Trigger.Mode != "once" || (c.LegacySignal == "user_notifications" && scope.Type != "account") || (c.LegacySignal != "user_notifications" && scope.Type != "system")) {
		return ErrInvalid
	}
	if c.Metric != "legacy_notice" && c.LegacySignal != "" {
		return ErrInvalid
	}
	if c.Metric == "legacy_notice" && (c.ThresholdCount != 1 || c.Operator != "gte" || c.HoldSeconds != 0 || c.Window.Type != "rolling" || c.Window.DurationSeconds != 60 || c.Window.Timezone != "Asia/Shanghai" || spec.Acknowledgement.Required) {
		return ErrInvalid
	}
	if spec.Trigger.Mode == "milestone" {
		if c.Window.Type != "cumulative" {
			return ErrInvalid
		}
		c.Operator, c.ThresholdUSD, c.RecoveryThresholdUSD = "gte", spec.Trigger.StepUSD, "0"
	}
	if scope.Type == "uid_budget" || scope.Type == "project_budget" {
		if spec.Condition.Metric != "budget_remaining_ratio" || len(scope.EntityIDs) == 0 || len(spec.Actions) != 0 {
			return ErrInvalid
		}
		var count int64
		query := db.WithContext(ctx).Model(&model.CliendUserQuota{})
		if scope.Type == "project_budget" {
			query = db.WithContext(ctx).Model(&model.ProjectAllocation{})
		}
		ids := slices.Clone(scope.EntityIDs)
		slices.Sort(ids)
		ids = slices.Compact(ids)
		if err := query.Where("id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return ErrInvalid
		}
		scope.EntityIDs = ids
	}
	if spec.Condition.Metric == "balance_remaining" {
		if scope.Type != "account" || len(spec.Actions) != 0 || !slices.Contains([]string{"", "wallet", "subscription"}, spec.Condition.BalancePool) {
			return ErrInvalid
		}
		if spec.Condition.BalancePool == "subscription" {
			var count int64
			if err := db.WithContext(ctx).Model(&model.UserSubscription{}).Where("id = ? AND user_id = ?", spec.Condition.SubscriptionID, owner).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrInvalid
			}
		}
	}
	if spec.Condition.Metric == "budget_remaining_ratio" && scope.Type != "uid_budget" && scope.Type != "project_budget" {
		return ErrInvalid
	}
	if slices.Contains([]string{"task_failure_count", "task_overdue", "evaluation_lag", "source_lag", "delivery_backlog"}, c.Metric) && scope.Type != "system" {
		return ErrInvalid
	}
	if c.Metric == "task_overdue" && (c.DeadlineMS < 10000 || c.DeadlineMS > 86400000) {
		return ErrInvalid
	}
	if c.TaskType != "" && !slices.Contains([]string{model.SystemTaskTypeChannelTest, model.SystemTaskTypeLogCleanup, model.SystemTaskTypeModelUpdate, model.SystemTaskTypeAsyncTaskPoll, model.SystemTaskTypeMidjourneyPoll}, c.TaskType) {
		return ErrInvalid
	}
	if c.Protocol != "" && !slices.Contains([]string{"openai", "claude", "gemini", "openai_responses", "task", "realtime"}, c.Protocol) {
		return ErrInvalid
	}
	usage := slices.Contains([]string{"spend_net", "input_tokens", "output_tokens", "cached_tokens", "cache_write_5m_tokens", "cache_write_1h_tokens"}, c.Metric)
	if usage && (scope.Type == "channel" || scope.Type == "channel_model" || c.Protocol != "" || c.Stream != nil) {
		return ErrInvalid
	}
	if c.Metric == "consecutive_failures" && scope.Type != "channel" && scope.Type != "channel_model" {
		return ErrInvalid
	}
	if c.ThresholdCount < 0 || c.RecoveryThresholdCount < 0 || c.RecoveryThresholdMS < 0 || c.RecoveryThresholdMS > 86400000 {
		return ErrInvalid
	}
	if !slices.Contains([]string{"gt", "gte", "lt", "lte"}, c.Operator) || c.HoldSeconds < 0 || c.HoldSeconds > 86400 || c.RecoverySeconds < 0 || c.RecoverySeconds > 86400 {
		return ErrInvalid
	}
	if c.LatencyBasis == "" {
		c.LatencyBasis = "gateway_request"
		if scope.Type == "channel" || scope.Type == "channel_model" {
			c.LatencyBasis = "channel_attempt"
		}
	}
	if !slices.Contains([]string{"gateway_request", "channel_attempt"}, c.LatencyBasis) {
		return ErrInvalid
	}
	if (scope.Type == "channel" || scope.Type == "channel_model") && c.LatencyBasis != "channel_attempt" {
		return ErrInvalid
	}
	if !platform && c.LatencyBasis != "gateway_request" {
		return ErrInvalid
	}
	if c.Metric == "inflight_overdue_count" {
		if !platform || len(spec.Actions) != 0 || c.MinSamples != 0 || c.Window.Type != "" || c.Window.DurationSeconds != 0 || c.DeadlineMS < 10000 || c.DeadlineMS > 86400000 || c.ThresholdCount <= 0 || !slices.Contains([]string{"first_output", "output_idle", "total"}, c.DeadlineKind) {
			return ErrInvalid
		}
		if c.Operator != "gte" || c.RecoveryThresholdCount != 0 {
			return ErrInvalid
		}
	} else {
		if c.Window.Type == "" {
			c.Window.Type = "rolling"
		}
		if !slices.Contains([]string{"rolling", "hour", "day", "month", "cumulative"}, c.Window.Type) {
			return ErrInvalid
		}
		if c.Window.Type == "rolling" && (c.Window.DurationSeconds < 60 || c.Window.DurationSeconds > 86400) {
			return ErrInvalid
		}
		if c.Window.Timezone == "" {
			c.Window.Timezone = "Asia/Shanghai"
		}
		if _, err := time.LoadLocation(c.Window.Timezone); err != nil {
			return ErrInvalid
		}
		if c.MinSamples < 0 || c.MinSamples > 100000 {
			return ErrInvalid
		}
		if c.Metric != "spend_net" && c.MinSamples == 0 {
			c.MinSamples = 1
		}
	}
	if c.Metric == "duration_ms" || c.Metric == "ttft_ms" || c.Metric == "evaluation_lag" || c.Metric == "source_lag" {
		if c.Aggregation == "" {
			c.Aggregation = "p95"
		}
		if !slices.Contains([]string{"mean", "p95", "p99"}, c.Aggregation) || c.ThresholdMS <= 0 || c.ThresholdMS > 86400000 {
			return ErrInvalid
		}
	}
	if strings.HasSuffix(c.Metric, "_rate") || c.Metric == "budget_remaining_ratio" {
		if c.ThresholdBPS < 0 || c.ThresholdBPS > 10000 || c.RecoveryThresholdBPS < 0 || c.RecoveryThresholdBPS > 10000 {
			return ErrInvalid
		}
	}
	if strings.HasPrefix(c.Metric, "slow_request_") && (c.SlowThresholdMS <= 0 || c.SlowThresholdMS > 86400000) {
		return ErrInvalid
	}
	if c.Metric == "spend_net" || c.Metric == "balance_remaining" {
		if _, err := USDQuota(c.ThresholdUSD); err != nil {
			return ErrInvalid
		}
		if c.RecoveryThresholdUSD != "" {
			if _, err := USDQuota(c.RecoveryThresholdUSD); err != nil {
				return ErrInvalid
			}
		}
	}
	if spec.Trigger.Mode == "" {
		spec.Trigger.Mode = "incident"
	}
	if !slices.Contains([]string{"incident", "once", "once_per_period", "milestone", "schedule_report"}, spec.Trigger.Mode) {
		return ErrInvalid
	}
	if c.Metric == "inflight_overdue_count" && spec.Trigger.Mode != "incident" {
		return ErrInvalid
	}
	if spec.Trigger.RepeatSeconds == 0 {
		spec.Trigger.RepeatSeconds = 1800
	}
	if spec.Trigger.RepeatSeconds < 60 || spec.Trigger.RepeatSeconds > 604800 {
		return ErrInvalid
	}
	if spec.Trigger.Mode == "schedule_report" {
		if (scope.Type != "account" && scope.Type != "api_key") || c.Metric != "spend_net" {
			return ErrInvalid
		}
		if spec.Trigger.Report.BackfillPeriods == 0 {
			spec.Trigger.Report.BackfillPeriods = 7
		}
		if spec.Trigger.Report.PartialAfterSeconds == 0 {
			spec.Trigger.Report.PartialAfterSeconds = 1800
		}
		if spec.Trigger.Report.BackfillPeriods < 1 || spec.Trigger.Report.BackfillPeriods > 7 || spec.Trigger.Report.PartialAfterSeconds < 600 || spec.Trigger.Report.PartialAfterSeconds > 86400 {
			return ErrInvalid
		}
	}
	if spec.Trigger.Mode == "schedule_report" && (len(spec.Actions) > 0 || c.Window.Type == "rolling" || c.Window.Type == "cumulative") {
		return ErrInvalid
	}
	if spec.Trigger.Mode == "schedule_report" {
		if c.Window.SendAt == "" {
			c.Window.SendAt = "00:10"
		}
		if _, err := time.Parse("15:04", c.Window.SendAt); err != nil {
			return ErrInvalid
		}
	}
	if spec.Trigger.Mode == "once_per_period" && !slices.Contains([]string{"hour", "day", "month"}, c.Window.Type) {
		return ErrInvalid
	}
	if spec.Trigger.Mode == "milestone" && c.Window.Type != "cumulative" {
		return ErrInvalid
	}
	if spec.Trigger.Mode == "milestone" {
		if c.Metric != "spend_net" {
			return ErrInvalid
		}
		step, err := USDQuota(spec.Trigger.StepUSD)
		if err != nil || step <= 0 {
			return ErrInvalid
		}
	}
	if len(spec.Notify.TargetIDs) == 0 && len(spec.Actions) == 0 {
		return ErrInvalid
	}
	slices.Sort(spec.Notify.TargetIDs)
	spec.Notify.TargetIDs = slices.Compact(spec.Notify.TargetIDs)
	slices.Sort(spec.Acknowledgement.EscalationTargetIDs)
	spec.Acknowledgement.EscalationTargetIDs = slices.Compact(spec.Acknowledgement.EscalationTargetIDs)
	ids := append(slices.Clone(spec.Notify.TargetIDs), spec.Acknowledgement.EscalationTargetIDs...)
	for _, id := range ids {
		var target model.AlertTarget
		if err := db.WithContext(ctx).First(&target, id).Error; err != nil {
			return ErrForbidden
		}
		if target.OwnerUserID != owner && !(actor.Root() && spec.ManagementMode == "platform_enforced" && target.OwnerUserID == 0) {
			return ErrForbidden
		}
		if spec.Enabled && !target.Enabled {
			return ErrInvalid
		}
	}
	for _, action := range spec.Actions {
		if action.Target != "matched_entity" {
			return ErrInvalid
		}
		switch action.Type {
		case "disable_api_key":
			if scope.Type != "api_key" {
				return ErrInvalid
			}
		case "disable_account_keys":
			if scope.Type != "account" {
				return ErrInvalid
			}
		case "disable_channel":
			if !actor.Root() || scope.Type != "channel" {
				return ErrForbidden
			}
		case "disable_channel_model":
			if !actor.Root() || scope.Type != "channel_model" {
				return ErrForbidden
			}
		default:
			return ErrInvalid
		}
		if platform && (spec.ActionGuard == nil || spec.ActionGuard.MinRemainingChannels < 1 || spec.ActionGuard.MinRemainingChannels > 50 || !slices.Contains([]string{"notify_only", "isolate_anyway"}, spec.ActionGuard.OnNoAlternative)) {
			return ErrInvalid
		}
	}
	if spec.Recovery.Mode == "" {
		spec.Recovery.Mode = "manual"
	}
	if !slices.Contains([]string{"manual", "probe"}, spec.Recovery.Mode) {
		return ErrInvalid
	}
	if spec.Recovery.Mode == "probe" && (!platform || spec.Recovery.Successes < 3 || spec.Recovery.ObserveSeconds < 120 || (strings.Contains(c.Metric, "duration") || strings.Contains(c.Metric, "slow") || c.Metric == "ttft_ms") && spec.Recovery.DurationLimitMS <= 0 && spec.Recovery.TTFTLimitMS <= 0) {
		return ErrInvalid
	}
	if spec.Recovery.Mode == "probe" && (scope.Type != "channel" && scope.Type != "channel_model" || len(spec.Actions) == 0) {
		return ErrInvalid
	}
	if spec.Recovery.Mode == "probe" {
		if !slices.Contains([]string{"openai", "claude", "gemini", "openai_responses"}, spec.Recovery.Protocol) || c.Protocol != "" && c.Protocol != spec.Recovery.Protocol {
			return ErrInvalid
		}
		if len(spec.Actions) == 0 || spec.Recovery.Successes > 10 || spec.Recovery.ObserveSeconds > 3600 || spec.Recovery.DurationLimitMS < 0 || spec.Recovery.TTFTLimitMS < 0 {
			return ErrInvalid
		}
		if c.Metric == "ttft_ms" && spec.Recovery.TTFTLimitMS <= 0 {
			return ErrInvalid
		}
		if (c.Metric == "duration_ms" || strings.HasPrefix(c.Metric, "slow_request_")) && spec.Recovery.DurationLimitMS <= 0 {
			return ErrInvalid
		}
		if scope.Type == "channel" {
			if len(spec.Recovery.Models) == 0 || len(spec.Recovery.Models) > 20 {
				return ErrInvalid
			}
			var channels []model.Channel
			if err := db.WithContext(ctx).Select("id", "models").Where("id IN ?", scope.ChannelIDs).Find(&channels).Error; err != nil {
				return err
			}
			for _, channel := range channels {
				for _, name := range channel.GetModels() {
					if strings.Contains(name, "*") || !slices.Contains(spec.Recovery.Models, name) {
						return ErrInvalid
					}
				}
			}
		}
	}
	if spec.Trigger.Mode == "incident" {
		trigger, err := RuleThreshold(*spec, false)
		if err != nil {
			return ErrInvalid
		}
		recover, err := RuleThreshold(*spec, true)
		if err != nil {
			return ErrInvalid
		}
		if c.Operator == "gt" || c.Operator == "gte" {
			if recover > trigger || c.Operator == "gte" && recover == trigger {
				return ErrInvalid
			}
		} else if recover < trigger || c.Operator == "lte" && recover == trigger {
			return ErrInvalid
		}
	}
	return nil
}

func CanRead(actor Actor, owner int) bool {
	return actor.ID > 0 && (owner == actor.ID || actor.Root() || actor.ReadAll)
}
func CanManage(actor Actor, row model.AlertRule) bool {
	if actor.Root() {
		return true
	}
	spec, err := model.AlertRuleConfig(row)
	return err == nil && actor.ID == row.OwnerUserID && spec.ManagementMode == "user_managed"
}

func SaveRule(ctx context.Context, actor Actor, owner int, id, revision int64, spec model.AlertRuleSpec) (*model.AlertRule, error) {
	if id == 0 && revision != 0 || id > 0 && revision <= 0 {
		return nil, ErrInvalid
	}
	if id != 0 {
		var old model.AlertRule
		if err := model.DB.WithContext(ctx).First(&old, id).Error; err != nil {
			return nil, err
		}
		if !CanManage(actor, old) || old.OwnerUserID != owner {
			return nil, ErrForbidden
		}
	}
	if id > 0 {
		var migration model.AlertMigration
		if err := model.DB.WithContext(ctx).Where("rule_id = ?", id).Limit(1).Find(&migration).Error; err != nil {
			return nil, err
		}

		if IsLegacyNoticeKind(migration.Kind) && (spec.Condition.Metric != "legacy_notice" || spec.Condition.LegacySignal != migration.Kind || spec.ManagementMode != "platform_enforced" || len(spec.Actions) > 0) {
			return nil, &BlockedAction{Code: "migration_rule_changed"}
		}
		if migration.Kind == "token_spend" {
			tokenID, err := strconv.Atoi(migration.ObjectID)
			if err != nil || spec.Scope.Type != "api_key" || spec.Scope.Selector != "ids" || spec.Scope.IncludeFuture || spec.Scope.GroupMode != "per_entity" || len(spec.Scope.EntityIDs) != 1 || spec.Scope.EntityIDs[0] != tokenID || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != "cumulative" || spec.Trigger.Mode != "milestone" || len(spec.Actions) > 0 {
				return nil, &BlockedAction{Code: "migration_rule_changed"}
			}
		}
		if migration.Kind == "user_spend" && (owner != migration.OwnerUserID || spec.Scope.Type != "account" || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != "cumulative" || spec.Trigger.Mode != "milestone") {
			return nil, &BlockedAction{Code: "migration_rule_changed"}
		}
		if migration.Kind == "user_report" && (owner != migration.OwnerUserID || spec.Scope.Type != "account" || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != "day" || spec.Condition.Window.Timezone != "Asia/Shanghai" || spec.Trigger.Mode != "schedule_report") {
			return nil, &BlockedAction{Code: "migration_rule_changed"}
		}
		if migration.Kind == "key_hour" || migration.Kind == "key_day" {
			var old model.AlertRule
			if err := model.DB.WithContext(ctx).First(&old, id).Error; err != nil {
				return nil, err
			}
			before, err := model.AlertRuleConfig(old)
			if err != nil {
				return nil, err
			}
			// Preserve the legacy threshold/action identity while allowing delivery
			// and presentation edits through the ordinary normalized rule editor.
			oldActions, _ := common.Marshal(before.Actions)
			newActions, _ := common.Marshal(spec.Actions)
			if spec.ManagementMode != "platform_enforced" || spec.Scope.Type != "api_key" || spec.Scope.Selector != "all_owned" || !spec.Scope.IncludeFuture || spec.Scope.GroupMode != "per_entity" || spec.Condition.Metric != "spend_net" || spec.Condition.Window.Type != before.Condition.Window.Type || spec.Condition.Window.Timezone != "Asia/Shanghai" || spec.Condition.ThresholdUSD != before.Condition.ThresholdUSD || spec.Trigger.Mode != "once_per_period" || string(oldActions) != string(newActions) {
				return nil, &BlockedAction{Code: "migration_rule_changed"}
			}

		}
	}
	if err := Validate(ctx, actor, owner, &spec); err != nil {
		return nil, err
	}
	config, err := common.Marshal(spec)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	row := &model.AlertRule{ID: id, OwnerUserID: owner, CreatedBy: actor.ID, UpdatedBy: actor.ID, Name: spec.Name, Enabled: spec.Enabled, Config: string(config), CreatedAt: now, UpdatedAt: now, EffectiveAt: now, NextEvaluationAt: now, Generation: 1}
	if id != 0 {
		var old model.AlertRule
		if err := model.DB.WithContext(ctx).First(&old, id).Error; err != nil {
			return nil, err
		}
		oldSpec, err := model.AlertRuleConfig(old)
		if err != nil {
			return nil, err
		}
		// Presentation/delivery edits retain the condition generation and progress.
		oldSpec.Name, oldSpec.Notify, oldSpec.Acknowledgement, oldSpec.Actions, oldSpec.ActionGuard = spec.Name, spec.Notify, spec.Acknowledgement, spec.Actions, spec.ActionGuard
		oldSpec.Severity, oldSpec.Category, oldSpec.Recovery = spec.Severity, spec.Category, spec.Recovery
		oldJSON, _ := common.Marshal(oldSpec)
		row.Generation, row.EffectiveAt = old.Generation, old.EffectiveAt
		row.EvaluationCursor = old.EvaluationCursor
		if string(oldJSON) != string(config) {
			row.Generation++
			row.EffectiveAt = now
			row.EvaluationCursor = ""
		}
	}
	var count int64
	if id == 0 {
		if err := model.DB.WithContext(ctx).Model(&model.AlertRule{}).Where("owner_user_id = ?", owner).Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= int64(model.AlertRuleOwnerLimit()) {
			return nil, ErrInvalid
		}
	}
	if err := model.SaveAlertRule(ctx, row, revision); err != nil {
		return nil, err
	}
	return row, nil
}

func RuleResponse(row model.AlertRule) (map[string]any, error) {
	spec, err := model.AlertRuleConfig(row)
	if err != nil {
		// Keep other rules and the immutable row identity readable after corruption.
		// Root can remove the invalid configuration; its active holds remain intact.
		spec = model.AlertRuleSpec{Name: row.Name, Enabled: row.Enabled, Scope: model.AlertScope{Type: "account"}, Notify: model.AlertNotify{TargetIDs: []int64{}}, Actions: []model.AlertAction{}}
	}
	return map[string]any{"id": row.ID, "owner_user_id": row.OwnerUserID, "revision": row.Revision, "condition_generation": row.Generation, "effective_at": row.EffectiveAt, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt, "config": spec, "config_valid": err == nil, "last_evaluation_error": row.LastEvaluationError}, nil
}

func RuleThreshold(spec model.AlertRuleSpec, recovery bool) (int64, error) {
	c := spec.Condition
	if c.Metric == "spend_net" || c.Metric == "balance_remaining" {
		value := c.ThresholdUSD
		if recovery {
			value = c.RecoveryThresholdUSD
			if value == "" {
				value = c.ThresholdUSD
			}
		}
		unit := spec.QuotaUnit
		if unit == 0 {
			unit = int64(common.QuotaPerUnit)
		}
		return usdQuota(value, unit)
	}
	if strings.HasSuffix(c.Metric, "_rate") || c.Metric == "budget_remaining_ratio" {
		if recovery {
			return c.RecoveryThresholdBPS, nil
		}
		return c.ThresholdBPS, nil
	}
	if c.Metric == "duration_ms" || c.Metric == "ttft_ms" || c.Metric == "evaluation_lag" || c.Metric == "source_lag" {
		if recovery {
			return c.RecoveryThresholdMS, nil
		}
		return c.ThresholdMS, nil
	}
	if recovery {
		return c.RecoveryThresholdCount, nil
	}
	return c.ThresholdCount, nil
}

func VisibleRule(ctx context.Context, actor Actor, id int64, write bool) (model.AlertRule, error) {
	var row model.AlertRule
	if err := model.DB.WithContext(ctx).First(&row, id).Error; err != nil {
		return row, err
	}
	if !CanRead(actor, row.OwnerUserID) || write && !CanManage(actor, row) {
		return row, gorm.ErrRecordNotFound
	}
	return row, nil
}

// PlatformScope names domains that may only be configured by Root.
func PlatformScope(scope string) bool {
	return slices.Contains([]string{"channel", "channel_model", "system", "uid_budget", "project_budget"}, scope)
}
