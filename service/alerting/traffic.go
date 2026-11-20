package alerting

import (
	"context"
	"database/sql"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TrafficMeasurement is immutable notification evidence. Baselines use 1/10000
// of a call per current window, retaining fractional historical averages.
type TrafficMeasurement struct {
	Count           int64 `json:"count,string"`
	BaselineScaled  int64 `json:"baseline_scaled,string"`
	ChangeBPS       int64 `json:"change_bps,string"`
	BaselineStart   int64 `json:"baseline_start"`
	BaselineEnd     int64 `json:"baseline_end"`
	CurrentComplete bool  `json:"current_complete"`
	Frozen          bool  `json:"frozen"`
}

func trafficMetric(metric string) bool {
	return metric == "request_started_count" || metric == "request_volume_change"
}

func measureTraffic(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, now time.Time) (Measurement, error) {
	m.Traffic = &TrafficMeasurement{}
	if m.WindowEnd-m.WindowStart < spec.Condition.Window.DurationSeconds*1000 {
		m.Status, m.Reason = "warming_up", "window_coverage"
		return m, nil
	}
	options := &sql.TxOptions{ReadOnly: true}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Recheck the exact accepted window inside this snapshot. The outer
		// coverage check can use a newer wall-clock interval than the watermark.
		var unsupported int64
		if err := unsupportedFacts(tx, target, spec, m.WindowStart, m.WindowEnd, false).Count(&unsupported).Error; err != nil {
			return err
		}
		if unsupported > 0 {
			m.Status, m.Reason = "partial", "protocol_correlation_unsupported"
			return nil
		}
		count, err := countTrafficStarts(ctx, tx, spec, target, m.WindowStart, m.WindowEnd, m.WindowEnd)
		if err != nil {
			return err
		}
		m.Value, m.Samples, m.Traffic.Count = count, count, count
		m.Traffic.CurrentComplete = true
		if spec.Condition.Metric == "request_started_count" {
			return nil
		}
		// A not-yet-valid baseline has no percentage value. Keep its call count
		// in the evidence instead of exposing it as if it were basis points.
		m.Value = 0
		baseline := spec.Condition.Baseline
		if baseline == nil {
			return ErrInvalid
		}
		if baseline.Mode == "manual" {
			m, err = applyTrafficBaseline(m, baseline.Count*10000, spec.Condition.ChangeDirection)
			return err
		}
		m.Traffic.BaselineEnd = m.WindowStart
		m.Traffic.BaselineStart = m.WindowStart - baseline.HistorySeconds*1000
		status, _, err := WindowCoverage(ctx, m.Traffic.BaselineStart, m.Traffic.BaselineEnd, now)
		if err != nil {
			return err
		}
		if status != "complete" {
			m.Status, m.Reason = "warming_up", "baseline_window_coverage"
			return nil
		}
		if err := unsupportedFacts(tx, target, spec, m.Traffic.BaselineStart, m.Traffic.BaselineEnd, false).Count(&unsupported).Error; err != nil {
			return err
		}
		if unsupported > 0 {
			m.Status, m.Reason = "warming_up", "baseline_window_coverage"
			return nil
		}
		history, err := countTrafficStarts(ctx, tx, spec, target, m.Traffic.BaselineStart, m.Traffic.BaselineEnd, m.WindowEnd)
		if err != nil {
			return err
		}
		scaled := decimal.NewFromInt(history).Mul(decimal.NewFromInt(spec.Condition.Window.DurationSeconds)).Mul(decimal.NewFromInt(10000)).Div(decimal.NewFromInt(baseline.HistorySeconds))
		if scaled.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
			return ErrInvalid
		}
		m.Traffic.BaselineScaled = scaled.IntPart()
		if m.Traffic.BaselineScaled < baseline.MinimumCount*10000 || m.Traffic.BaselineScaled <= 0 {
			m.Status, m.Reason = "warming_up", "baseline_insufficient"
			return nil
		}
		m, err = applyTrafficBaseline(m, m.Traffic.BaselineScaled, spec.Condition.ChangeDirection)
		return err
	}, options)
	return m, err
}

// The first gateway receipt can precede authentication/model parsing. Count
// distinct source identities across accepted production metadata, instead of
// counting only an initially unattributed start or counting every progress row.
// OccurredAt's upper bound also prevents replay from seeing future metadata.
func countTrafficStarts(ctx context.Context, db *gorm.DB, spec model.AlertRuleSpec, target MonitoredTarget, start, end, accepted int64) (int64, error) {
	layer := "request"
	if spec.Condition.LatencyBasis == "channel_attempt" {
		layer = "channel_attempt"
	}
	query := model.AlertMetricRows(db.WithContext(ctx), func(q *gorm.DB) *gorm.DB {
		q = targetFacts(q, target, spec).Where("namespace = ? AND layer = ? AND sample_class = ? AND user_id > 0 AND source_id <> ''", "health", layer, "production").
			Where("started_at >= ? AND started_at < ? AND occurred_at >= ? AND occurred_at < ?", start, end, start, accepted)
		if target.ConfigVersion != "" {
			q = q.Where("config_version = ?", target.ConfigVersion)
		}
		return q
	}, "source_id", false)
	var count int64
	err := query.Distinct("source_id").Count(&count).Error
	return count, err
}

func applyTrafficBaseline(m Measurement, baseline int64, direction string) (Measurement, error) {
	if m.Traffic == nil || baseline <= 0 {
		return m, ErrInvalid
	}
	change := decimal.NewFromInt(m.Traffic.Count).Mul(decimal.NewFromInt(10000)).Sub(decimal.NewFromInt(baseline)).Mul(decimal.NewFromInt(10000)).Div(decimal.NewFromInt(baseline))
	if change.Abs().GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return m, ErrInvalid
	}
	m.Traffic.BaselineScaled = baseline
	m.Traffic.ChangeBPS = change.IntPart()
	m.Value = m.Traffic.ChangeBPS
	if direction == "decrease" {
		m.Value = -m.Value
	}
	m.Value = max(m.Value, 0)
	m.Status, m.Reason = "complete", ""
	return m, nil
}

// Only traffic incident rules reserve Baseline/PeriodKey for their frozen
// comparison. Other metrics retain their existing state and period contracts.
func restoreTrafficBaseline(state *model.AlertRuleState, m Measurement, direction string) (Measurement, error) {
	if m.Traffic == nil || !m.Traffic.CurrentComplete || state.Baseline <= 0 || (state.Status != "pending" && state.Status != "firing") {
		return m, nil
	}
	m, err := applyTrafficBaseline(m, state.Baseline, direction)
	if err != nil {
		return m, err
	}
	m.Traffic.Frozen = true
	first, last, _ := strings.Cut(state.PeriodKey, ":")
	m.Traffic.BaselineStart, _ = strconv.ParseInt(first, 10, 64)
	m.Traffic.BaselineEnd, _ = strconv.ParseInt(last, 10, 64)
	return m, nil
}
