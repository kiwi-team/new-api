package alerting

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type ReplayPoint struct {
	Target      MonitoredTarget `json:"target"`
	At          int64           `json:"at"`
	Measurement Measurement     `json:"measurement"`
	Matched     bool            `json:"matched"`
}

// ReplayRule reads recorded facts only. It never creates rule states, jobs or
// holds. Missing historical coverage stays explicit; replay cannot prove facts
// from before collection or reconstruct an unrecorded budget balance.
func ReplayRule(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, start, end int64, steps int, now time.Time) ([]ReplayPoint, error) {
	if spec.Condition.Metric == "legacy_notice" || steps < 1 || steps > 24 || start >= end || end > now.UnixMilli() || start < now.Add(-7*24*time.Hour).UnixMilli() || spec.Condition.Window.Type == "cumulative" || spec.Condition.Metric == "inflight_overdue_count" || spec.Condition.Metric == "balance_remaining" || spec.Condition.Metric == "budget_remaining_ratio" || systemMetric(spec.Condition.Metric) {
		return nil, ErrInvalid
	}
	targets, err := ResolveTargets(ctx, rule, spec)
	if err != nil {
		return nil, err
	}
	if len(targets) > 10 {
		return nil, ErrInvalid
	}
	threshold, err := RuleThreshold(spec, false)
	if err != nil {
		return nil, err
	}
	watermark, sourceStatus, sourceReason, err := SourceWatermark(ctx, now)
	if err != nil {
		return nil, err
	}
	if sourceStatus == "complete" && metricNamespace(spec) == "usage" {
		sourceStatus, sourceReason, err = UsageCoverage(ctx, now)
		if err != nil {
			return nil, err
		}
	}
	if sourceStatus == "complete" {
		end = min(end, watermark)
		if end <= start {
			return nil, ErrInvalid
		}
	}
	points := make([]ReplayPoint, 0, len(targets)*steps)
	for _, target := range targets {
		for i := range steps {
			at := start + (end-start)*int64(i+1)/int64(steps)
			historical := rule
			historical.ID = 0
			historical.EffectiveAt = start - 24*time.Hour.Milliseconds()
			windowStart, windowEnd, _, err := WindowBounds(historical, spec.Condition, time.UnixMilli(at))
			if err != nil {
				return nil, err
			}
			m := Measurement{Status: "complete", WindowStart: windowStart, WindowEnd: windowEnd, AttributionCovered: true}
			// SQL's consistent snapshot and exact rank calculation are shared with
			// live evaluation, rather than approximating p95 from means.
			if trafficMetric(spec.Condition.Metric) {
				m, err = measureTraffic(ctx, historical, spec, target, m, now)
			} else {
				m, err = measureMetricSQL(context.WithValue(ctx, freshMetricKey{}, true), historical, spec, target, m, time.UnixMilli(at))
			}
			if err != nil {
				return nil, err
			}
			status, reason, err := WindowCoverage(ctx, windowStart, windowEnd, now)
			if err != nil {
				return nil, err
			}
			if status != "complete" {
				m.Status, m.Reason = status, reason
			}
			if sourceStatus != "complete" {
				m.Status, m.Reason = sourceStatus, sourceReason
			}
			var unsupported int64
			if err := unsupportedFacts(model.DB.WithContext(ctx), target, spec, windowStart, windowEnd, false).Count(&unsupported).Error; err != nil {
				return nil, err
			}
			if unsupported > 0 {
				m.Status, m.Reason = "partial", "protocol_correlation_unsupported"
			}
			points = append(points, ReplayPoint{Target: target, At: at, Measurement: m, Matched: m.Status == "complete" && matches(m.Value, threshold, spec.Condition.Operator)})
		}
	}
	return points, nil
}
