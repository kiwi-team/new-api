package alerting

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

type MonitoredTarget struct {
	OwnerUserID   int    `json:"owner_user_id,omitempty"`
	Type          string `json:"type"`
	ID            int    `json:"id"`
	Model         string `json:"model"`
	Name          string `json:"name"`
	ConfigVersion string `json:"config_version,omitempty"`
	ClientUserID  string `json:"client_user_id,omitempty"`
	ProjectID     int    `json:"project_id,omitempty"`
	PlanID        int    `json:"plan_id,omitempty"`
}

func (t MonitoredTarget) Key() string {
	if t.Type == "channel_model" || t.Type == "model" {
		return model.AlertIdentity(t.Type, t.ID, "routing", t.Model)
	}
	return model.AlertIdentity(t.Type, t.ID)
}

type Measurement struct {
	Traffic            *TrafficMeasurement `json:"traffic,omitempty"`
	Requests           []RequestEvidence   `json:"requests,omitempty"`
	Report             *ReportSummary      `json:"report,omitempty"`
	Value              int64               `json:"value,string"`
	Samples            int64               `json:"samples,string"`
	Numerator          int64               `json:"numerator,string"`
	Missing            int64               `json:"missing,string"`
	Status             string              `json:"status"`
	Reason             string              `json:"reason,omitempty"`
	WindowStart        int64               `json:"window_start"`
	WindowEnd          int64               `json:"window_end"`
	RequestIDs         []string            `json:"request_ids"`
	AttributionCovered bool                `json:"attribution_covered"`
	ThresholdCrossedAt int64               `json:"threshold_crossed_at"`
	SourceAcceptedAt   int64               `json:"source_accepted_at"`
}

// RequestEvidence contains timing references only. Raw bodies, headers and
// credentials remain behind the existing diagnostic APIs and their permissions.
type RequestEvidence struct {
	Kind         string `json:"kind"`
	RequestID    string `json:"request_id"`
	Layer        string `json:"layer"`
	ChannelName  string `json:"channel_name"`
	Model        string `json:"model"`
	Outcome      string `json:"outcome"`
	StartedAt    int64  `json:"started_at"`
	LastOutputAt int64  `json:"last_output_at"`
	FinishedAt   int64  `json:"finished_at"`
	DurationMS   *int64 `json:"duration_ms,omitempty"`
	TTFTMS       *int64 `json:"ttft_ms,omitempty"`
}

func ResolveTargets(ctx context.Context, row model.AlertRule, spec model.AlertRuleSpec) ([]MonitoredTarget, error) {
	scope := spec.Scope
	switch scope.Type {
	case "uid_budget":
		var rows []model.CliendUserQuota
		if err := model.DB.WithContext(ctx).Where("id IN ?", scope.EntityIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		out := make([]MonitoredTarget, 0, len(rows))
		for _, row := range rows {
			out = append(out, MonitoredTarget{Type: "uid_budget", ID: row.Id, Name: row.ClientName + " / " + row.ClientUserId, ClientUserID: row.ClientUserId})
		}
		return out, nil
	case "project_budget":
		var rows []model.ProjectAllocation
		if err := model.DB.WithContext(ctx).Where("id IN ?", scope.EntityIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		out := make([]MonitoredTarget, 0, len(rows))
		for _, row := range rows {
			var project model.Project
			var plan model.ProjectAllocationPlan
			if err := model.DB.WithContext(ctx).First(&project, row.ProjectId).Error; err != nil {
				return nil, err
			}
			if err := model.DB.WithContext(ctx).Where("id = ? AND project_id = ?", row.PlanId, row.ProjectId).First(&plan).Error; err != nil {
				return nil, err
			}
			out = append(out, MonitoredTarget{Type: "project_budget", ID: row.Id, Name: project.ProjectName + " / " + plan.PlanName + " / " + row.ClientUserId, ClientUserID: row.ClientUserId, ProjectID: row.ProjectId, PlanID: row.PlanId})
		}
		return out, nil
	case "model":
		out := make([]MonitoredTarget, 0, len(scope.ModelSelector.Models))
		for _, name := range scope.ModelSelector.Models {
			out = append(out, MonitoredTarget{Type: "model", Model: name, Name: name})
		}
		return out, nil
	case "account":
		var owner model.User
		if err := model.DB.WithContext(ctx).Select("id", "username", "display_name").First(&owner, row.OwnerUserID).Error; err != nil {
			return nil, err
		}
		name := owner.DisplayName
		if name == "" {
			name = owner.Username
		}
		return []MonitoredTarget{{Type: "account", ID: row.OwnerUserID, Name: name}}, nil
	case "system":
		return []MonitoredTarget{{Type: "system", Name: "System"}}, nil
	case "api_key":
		query := model.DB.WithContext(ctx).Model(&model.Token{}).Where("user_id = ?", row.OwnerUserID)
		if scope.Selector != "all_owned" || !scope.IncludeFuture {
			query = query.Where("id IN ?", scope.EntityIDs)
		}
		var tokens []model.Token
		if err := query.Select("id", "name").Limit(501).Find(&tokens).Error; err != nil {
			return nil, err
		}
		if len(tokens) > 500 {
			return nil, ErrInvalid
		}
		if scope.GroupMode == "aggregate" {
			return []MonitoredTarget{{Type: "api_key_set", ID: row.OwnerUserID, Name: spec.Name}}, nil
		}
		out := make([]MonitoredTarget, 0, len(tokens))
		for _, token := range tokens {
			out = append(out, MonitoredTarget{Type: "api_key", ID: token.Id, Name: token.Name, OwnerUserID: row.OwnerUserID})
		}
		return out, nil
	case "channel", "channel_model":
		var channels []model.Channel
		if err := model.DB.WithContext(ctx).Select("id", "name", "models", "model_mapping", "type", "base_url", "group", "settings").Where("id IN ?", scope.ChannelIDs).Find(&channels).Error; err != nil {
			return nil, err
		}
		out := make([]MonitoredTarget, 0, len(channels))
		for _, channel := range channels {
			if scope.Type == "channel" {
				_, _, version := model.ResolveChannelModelIdentity(&channel, "")
				out = append(out, MonitoredTarget{Type: "channel", ID: channel.Id, Name: channel.Name, ConfigVersion: version})
				continue
			}
			models := scope.ModelSelector.Models
			if scope.ModelSelector.Mode == "all_configured" && scope.ModelSelector.IncludeFuture {
				var err error
				models, err = configuredAlertModels(ctx, channel)
				if err != nil {
					return nil, err
				}
			}
			slices.Sort(models)
			models = slices.Compact(models)
			for _, name := range models {
				if name != "" {
					key, matched, version := model.ResolveChannelModelIdentity(&channel, name)
					if !slices.Contains(channel.GetModels(), matched) || strings.Contains(key, "*") {
						continue
					}
					out = append(out, MonitoredTarget{Type: "channel_model", ID: channel.Id, Model: key, Name: channel.Name + " / " + key, ConfigVersion: version})
				}
			}
			if len(out) > 500 {
				return nil, ErrInvalid
			}
		}
		return out, nil
	}
	return nil, ErrInvalid
}

func WindowBounds(rule model.AlertRule, c model.AlertCondition, now time.Time) (int64, int64, string, error) {
	loc, err := time.LoadLocation(c.Window.Timezone)
	if c.Window.Timezone == "" {
		loc = time.UTC
		err = nil
	}
	if err != nil {
		return 0, 0, "", err
	}
	t := now.In(loc)
	end := now.UnixMilli()
	var start time.Time
	switch c.Window.Type {
	case "rolling":
		start = now.Add(-time.Duration(c.Window.DurationSeconds) * time.Second)
	case "hour":
		start = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
	case "day":
		start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	case "month":
		start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	case "cumulative":
		return rule.EffectiveAt, end, "cumulative", nil
	default:
		return rule.EffectiveAt, end, "inflight", nil
	}
	return max(start.UnixMilli(), rule.EffectiveAt), end, start.Format(time.RFC3339), nil
}

func targetFacts(query *gorm.DB, target MonitoredTarget, spec model.AlertRuleSpec) *gorm.DB {
	switch target.Type {
	case "account":
		query = query.Where("user_id = ?", target.ID)
	case "api_key":
		query = query.Where("token_id = ?", target.ID)
		if target.OwnerUserID > 0 {
			query = query.Where("user_id = ?", target.OwnerUserID)
		}
	case "api_key_set":
		query = query.Where("user_id = ?", target.ID)
		if !spec.Scope.IncludeFuture || spec.Scope.Selector != "all_owned" {
			query = query.Where("token_id IN ?", spec.Scope.EntityIDs)
		}
	case "channel":
		query = query.Where("channel_id = ?", target.ID)
	case "model":
		query = query.Where("model_hash = ?", model.AlertIdentity("model", target.Model))
	case "channel_model":
		query = query.Where("channel_id = ? AND model_hash = ?", target.ID, model.AlertIdentity("model", target.Model))
	}
	if spec.Condition.Protocol != "" {
		query = query.Where("protocol = ?", spec.Condition.Protocol)
	}
	if spec.Condition.Stream != nil {
		query = query.Where("stream = ?", *spec.Condition.Stream)
	}
	return query
}

// Measure has an explicit coverage ceiling: a truncated result is unknown,
// never a deceptively healthy percentile or permission to isolate a target.
func Measure(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, now time.Time) (Measurement, error) {
	if spec.Condition.Metric == "legacy_notice" {
		if !LegacyNoticeSourceReady(spec.Condition.LegacySignal) {
			return Measurement{Status: "partial", Reason: "usage_source_disabled"}, nil
		}
		return Measurement{Status: "no_data", Reason: "legacy_event_trigger"}, nil
	}
	if rule.ID > 0 && (target.Type == "channel_model" || target.Type == "channel" || spec.Scope.IncludeFuture) {
		var since int64
		err := model.WithAlertState(ctx, rule, target.Type, target.ID, target.Model, func(_ *gorm.DB, state *model.AlertRuleState) error {
			if state.ObservedAt == 0 {
				state.ObservedAt = rule.EffectiveAt
				if spec.Scope.IncludeFuture || spec.Scope.ModelSelector.IncludeFuture {
					// Key identities have no facts before their creation. Use the
					// rule baseline so the first charge before discovery is retained.
					if target.Type != "api_key" && target.Type != "api_key_set" {
						state.ObservedAt = now.UnixMilli()
					}
				}
				state.ConfigVersion = target.ConfigVersion
			} else if state.ConfigVersion != target.ConfigVersion {
				state.ConfigVersion, state.ObservedAt = target.ConfigVersion, now.UnixMilli()
				state.PendingAt, state.RecoveryAt = 0, 0
			}
			since = state.ObservedAt
			return nil
		})
		if err != nil {
			return Measurement{}, err
		}
		rule.EffectiveAt = max(rule.EffectiveAt, since)
	}
	start, end, _, err := WindowBounds(rule, spec.Condition, now)
	if spec.Trigger.Mode == "schedule_report" {
		start, end, err = ReportBounds(spec.Condition.Window, now)
	}
	m := Measurement{Status: "complete", WindowStart: start, WindowEnd: end, AttributionCovered: true}
	if err != nil {
		return m, err
	}
	if spec.Trigger.Mode == "schedule_report" && start < rule.EffectiveAt {
		m.Status, m.Reason = "warming_up", "first_complete_period"
		return m, nil
	}
	c := spec.Condition
	if (c.Metric == "spend_net" || c.Metric == "balance_remaining" || c.Metric == "budget_remaining_ratio") && spec.QuotaUnit != 0 && spec.QuotaUnit != int64(common.QuotaPerUnit) {
		m.Status, m.Reason = "error", "quota_unit_changed"
		return m, nil
	}
	if spec.Trigger.Mode == "schedule_report" {
		return MeasureReport(ctx, rule, spec, target, start, end, now)
	}
	if c.Metric == "balance_remaining" || c.Metric == "budget_remaining_ratio" {
		return measureBudget(ctx, rule, spec, target, now)
	}
	if systemMetric(c.Metric) {
		return measureSystem(ctx, spec, m, now)
	}
	watermark, coverage, reason, coverageErr := SourceWatermark(ctx, now)
	if coverageErr != nil {
		return m, coverageErr
	}
	if coverage != "complete" {
		m.Status, m.Reason = coverage, reason
		return m, nil
	}
	if metricNamespace(spec) == "usage" {
		status, reason, err := UsageCoverage(ctx, now)
		if err != nil {
			return m, err
		}
		if status != "complete" {
			m.Status, m.Reason = status, reason
			return m, nil
		}
	}
	// An uncorrelated operation is missing coverage, not a failed or successful
	// request. Preserve the evidence and withhold automated conclusions.
	var unverified int64
	if err := targetFacts(model.DB.WithContext(ctx).Model(&model.AlertObservation{}), target, spec).Where("namespace = ? AND sample_class = ? AND occurred_at >= ? AND occurred_at <= ?", "health", "unsupported", start, end).Count(&unverified).Error; err != nil {
		return m, err
	}
	if unverified > 0 {
		m.Status, m.Reason = "partial", "protocol_correlation_unsupported"
		return m, nil
	}
	if c.Metric == "inflight_overdue_count" {
		return measureInflight(ctx, rule, spec, target, now)
	}
	// Evaluate the most recent fully accepted window. New input after this
	// source watermark cannot silently change its denominator during evaluation.
	acceptedNow := time.UnixMilli(watermark)
	m.WindowStart, m.WindowEnd, _, err = WindowBounds(rule, spec.Condition, acceptedNow)
	if spec.Trigger.Mode == "schedule_report" {
		m.WindowStart, m.WindowEnd, err = ReportBounds(spec.Condition.Window, acceptedNow)
	}
	if err != nil {
		return m, err
	}
	m.SourceAcceptedAt = watermark
	if m.WindowEnd <= m.WindowStart {
		m.Status, m.Reason = "warming_up", "window_coverage"
		return m, nil
	}
	status, reason, err := WindowCoverage(ctx, m.WindowStart, m.WindowEnd, now)
	if err != nil {
		return m, err
	}
	if status != "complete" {
		m.Status, m.Reason = status, reason
		return m, nil
	}
	if trafficMetric(c.Metric) {
		return measureTraffic(ctx, rule, spec, target, m, now)
	}
	return measureMetricSQL(ctx, rule, spec, target, m, acceptedNow)
}

// ReportBounds selects the previous complete civil period. AddDate preserves
// calendar boundaries across DST instead of assuming every day is 24 hours.
func ReportBounds(window model.AlertWindow, now time.Time) (int64, int64, error) {
	loc, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return 0, 0, err
	}
	t := now.In(loc)
	var end, start time.Time
	switch window.Type {
	case "hour":
		end = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
		start = end.Add(-time.Hour)
	case "day":
		end = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		start = end.AddDate(0, 0, -1)
	case "month":
		end = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		start = end.AddDate(0, -1, 0)
	default:
		return 0, 0, ErrInvalid
	}
	return start.UnixMilli(), end.UnixMilli(), nil
}

func measureInflight(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, now time.Time) (Measurement, error) {
	m := Measurement{Status: "complete", WindowStart: rule.EffectiveAt, WindowEnd: now.UnixMilli()}
	query := targetFacts(model.DB.WithContext(ctx).Model(&model.AlertLifecycle{}), target, spec).Where("finished_at = ? AND started_at >= ? AND sample_class = ?", 0, rule.EffectiveAt, "production")
	layer := "request"
	if spec.Condition.LatencyBasis == "channel_attempt" {
		layer = "channel_attempt"
	}
	var rows []model.AlertLifecycle
	if err := query.Where("layer = ?", layer).Limit(10001).Find(&rows).Error; err != nil {
		return m, err
	}
	if len(rows) > 10000 {
		m.Status, m.Reason = "partial", "lifecycle_capacity"
		return m, nil
	}
	// One producer commonly owns many active attempts. Resolve its persisted
	// checkpoint once per batch instead of issuing a query for each request.
	bootSet := map[string]bool{}
	bootIDs := make([]string, 0)
	for _, row := range rows {
		if !bootSet[row.BootID] {
			bootSet[row.BootID] = true
			bootIDs = append(bootIDs, row.BootID)
		}
	}
	if len(bootIDs) > 1000 {
		m.Status, m.Reason = "partial", "source_capacity"
		return m, nil
	}
	sources := map[string]model.AlertWorkerCheckpoint{}
	if len(bootIDs) > 0 {
		var checkpoints []model.AlertWorkerCheckpoint
		if err := model.DB.WithContext(ctx).Where("id IN ?", bootIDs).Find(&checkpoints).Error; err != nil {
			return m, err
		}
		for _, source := range checkpoints {
			sources[source.ID] = source
		}
	}
	var crossed []int64
	for _, row := range rows {
		source, found := sources[row.BootID]
		if !found || !source.Healthy || source.ProtocolVersion < model.AlertSourceProtocolVersion || now.UnixMilli()-source.UpdatedAt > 15000 || now.UnixMilli()-source.CapturedAt > 15000 || source.AcceptedThrough < source.OfferedSequence {
			m.Status, m.Reason = "stale", "source_unavailable"
			continue
		}
		asOf := min(now.UnixMilli(), source.CapturedAt)
		if row.StartedAt > asOf {
			continue
		}
		if source.UpdatedAt > now.Add(5*time.Second).UnixMilli() || source.CapturedAt > now.Add(5*time.Second).UnixMilli() {
			m.Status, m.Reason = "stale", "clock_skew"
			continue
		}
		m.SourceAcceptedAt = max(m.SourceAcceptedAt, row.UpdatedAt)
		base := row.StartedAt
		switch spec.Condition.DeadlineKind {
		case "first_output":
			if !row.OutputCovered {
				m.Status, m.Reason = "partial", "output_unsupported"
				continue
			}
			if row.FirstOutputAt != 0 {
				continue
			}
		case "output_idle":
			if !row.OutputCovered {
				m.Status, m.Reason = "partial", "output_unsupported"
				continue
			}
			if row.FirstOutputAt == 0 {
				continue
			}
			base = row.LastOutputAt
		}
		if base > 0 && asOf-base >= spec.Condition.DeadlineMS {
			m.Value++
			crossed = append(crossed, base+spec.Condition.DeadlineMS)
			if len(m.RequestIDs) < 5 {
				m.RequestIDs = append(m.RequestIDs, row.RequestID)
				m.Requests = append(m.Requests, RequestEvidence{Kind: "inflight", RequestID: row.RequestID, Layer: row.Layer, ChannelName: row.ChannelName, Model: row.ModelKey, StartedAt: row.StartedAt, LastOutputAt: row.LastOutputAt})
			}
		}
		m.Samples++
	}
	slices.Sort(crossed)
	if n := spec.Condition.ThresholdCount; n > 0 && int64(len(crossed)) >= n {
		m.ThresholdCrossedAt = crossed[n-1]
	}
	return m, nil
}

// configuredAlertModels expands only successful, concrete routing identities
// when a declaration is a wildcard. Arbitrary failed client names cannot grow
// the monitoring directory, and each channel has a hard expansion bound.
func configuredAlertModels(ctx context.Context, channel model.Channel) ([]string, error) {
	var models []string
	wildcard := false
	for name := range strings.SplitSeq(channel.Models, ",") {
		if strings.Contains(name, "*") {
			wildcard = true
		} else if name != "" {
			models = append(models, name)
		}
	}
	if wildcard {
		var observed []string
		if err := model.DB.WithContext(ctx).Model(&model.AlertObservation{}).Where("namespace = ? AND kind = ? AND outcome = ? AND channel_id = ? AND occurred_at >= ?", "health", "finished", "success", channel.Id, time.Now().Add(-35*24*time.Hour).UnixMilli()).Distinct("model_key").Limit(101).Pluck("model_key", &observed).Error; err != nil {
			return nil, err
		}
		if len(observed) > 100 {
			return nil, ErrInvalid
		}
		for _, name := range observed {
			_, matched, _ := model.ResolveChannelModelIdentity(&channel, name)
			if slices.Contains(channel.GetModels(), matched) {
				models = append(models, name)
			}
		}
	}
	slices.Sort(models)
	models = slices.Compact(models)
	if len(models) > 100 {
		return nil, ErrInvalid
	}
	return models, nil
}
