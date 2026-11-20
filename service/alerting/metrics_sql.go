package alerting

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func terminalFacts(db *gorm.DB, spec model.AlertRuleSpec, target MonitoredTarget, start, end int64) *gorm.DB {
	query := targetFacts(db, target, spec).Where("occurred_at >= ? AND occurred_at < ?", start, end)
	if target.ConfigVersion != "" {
		query = query.Where("config_version = ?", target.ConfigVersion)
	}
	layer := "request"
	if spec.Condition.LatencyBasis == "channel_attempt" {
		layer = "channel_attempt"
	}
	return query.Where("namespace = ? AND kind = ? AND layer = ? AND sample_class = ?", "health", "finished", layer, "production")
}

func metricFacts(db *gorm.DB, ctx context.Context, spec model.AlertRuleSpec, target MonitoredTarget, start, end int64) *gorm.DB {
	return model.AlertMetricRows(db.WithContext(ctx), func(q *gorm.DB) *gorm.DB {
		return terminalFacts(q, spec, target, start, end)
	}, "id, occurred_at, outcome, duration_ms, ttft_ms, attribution_covered", false)
}

// SQL aggregates and ordered rank lookups keep exact millisecond percentiles
// without loading a window's response records into application memory. All
// expressions use portable SQL supported by the three primary databases.
func measureMetricSQL(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, now time.Time) (Measurement, error) {
	cache := metricCacheEnabled(ctx, spec)
	if cache {
		if cached, ok := readMetricSnapshot(ctx, spec, target, m); ok {
			return cached, nil
		}
	}
	var revision int64
	options := &sql.TxOptions{ReadOnly: true}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if cache {
			revision, err = model.AlertMetricRevision(ctx, tx, metricNamespace(spec), m.WindowStart, m.WindowEnd)
			if err != nil {
				return err
			}
		}
		m, err = measureSQLWindow(tx, ctx, rule, spec, target, m, now)
		return err
	}, options)
	if err == nil && cache {
		writeMetricSnapshot(ctx, spec, target, m, revision)
	}
	return m, err
}

func measureSQLWindow(db *gorm.DB, ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, now time.Time) (Measurement, error) {
	c := spec.Condition
	usageColumn := map[string]string{"spend_net": "quota", "input_tokens": "input_tokens", "output_tokens": "output_tokens", "cached_tokens": "cached_tokens", "cache_write_5m_tokens": "cache_write5m_tokens", "cache_write_1h_tokens": "cache_write1h_tokens"}[c.Metric]
	if usageColumn != "" {
		var migration model.AlertMigration
		if rule.ID > 0 && c.Metric == "spend_net" && c.Window.Type == "cumulative" {
			if err := db.WithContext(ctx).Where("rule_id = ?", rule.ID).Limit(1).Find(&migration).Error; err != nil {
				return m, err
			}
		}
		var totals struct {
			Value   int64
			Samples int64
		}
		start, end := m.WindowStart, m.WindowEnd
		if migration.RuleID > 0 {
			start, end = migration.MetricStartAt, 0
		}
		query := usageFacts(db.WithContext(ctx), spec, target, start, end, usageColumn)
		if err := query.Select("COALESCE(SUM(" + usageColumn + "),0) AS value, COALESCE(SUM(samples),0) AS samples").Scan(&totals).Error; err != nil {
			return m, err
		}
		m.Value, m.Samples = totals.Value, totals.Samples
		if migration.RuleID > 0 {
			value := decimal.NewFromInt(m.Value).Add(decimal.NewFromInt(migration.MetricOffset))
			if !value.Equal(decimal.NewFromInt(value.IntPart())) || value.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || value.LessThan(decimal.NewFromInt(math.MinInt64)) {
				return m, ErrInvalid
			}
			m.Value = value.IntPart()
		}
	} else {
		var totals struct{ Requests, Success, Failure, Timeout, Durations, Ttfts, DurationSum, TtftSum, Uncovered int64 }
		selects := `COUNT(*) AS requests,
COALESCE(SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END),0) AS success,
COALESCE(SUM(CASE WHEN outcome IN ('failure','timeout') THEN 1 ELSE 0 END),0) AS failure,
COALESCE(SUM(CASE WHEN outcome = 'timeout' THEN 1 ELSE 0 END),0) AS timeout,
COALESCE(SUM(CASE WHEN outcome IN ('success','failure','timeout') AND duration_ms >= 0 THEN 1 ELSE 0 END),0) AS durations,
COALESCE(SUM(CASE WHEN outcome IN ('success','failure','timeout') AND ttft_ms >= 0 THEN 1 ELSE 0 END),0) AS ttfts,
COALESCE(SUM(CASE WHEN outcome IN ('success','failure','timeout') THEN duration_ms ELSE 0 END),0) AS duration_sum,
COALESCE(SUM(CASE WHEN outcome IN ('success','failure','timeout') THEN ttft_ms ELSE 0 END),0) AS ttft_sum,
COALESCE(SUM(CASE WHEN attribution_covered = ? THEN 1 ELSE 0 END),0) AS uncovered`
		if err := metricFacts(db, ctx, spec, target, m.WindowStart, m.WindowEnd).Select(selects, false).Scan(&totals).Error; err != nil {
			return m, err
		}
		m.Samples = totals.Success + totals.Failure
		m.AttributionCovered = totals.Uncovered == 0
		switch c.Metric {
		case "request_count":
			m.Value = totals.Requests
			m.Samples = totals.Requests
		case "success_rate":
			if m.Samples > 0 {
				m.Value = totals.Success * 10000 / m.Samples
			}
			m.Numerator = totals.Success
		case "failure_rate":
			if m.Samples > 0 {
				m.Value = totals.Failure * 10000 / m.Samples
			}
			m.Numerator = totals.Failure
		case "timeout_rate":
			if m.Samples > 0 {
				m.Value = totals.Timeout * 10000 / m.Samples
			}
			m.Numerator = totals.Timeout
		case "timeout_count":
			m.Value = totals.Timeout
		case "slow_request_count", "slow_request_rate":
			var slow int64
			if err := metricFacts(db, ctx, spec, target, m.WindowStart, m.WindowEnd).Where("outcome IN ? AND duration_ms > ?", []string{"success", "failure", "timeout"}, c.SlowThresholdMS).Count(&slow).Error; err != nil {
				return m, err
			}
			m.Samples, m.Numerator, m.Missing = totals.Durations, slow, m.Samples-totals.Durations
			m.Value = slow
			if c.Metric == "slow_request_rate" && m.Samples > 0 {
				m.Value = slow * 10000 / m.Samples
			}
		case "duration_ms", "ttft_ms":
			column, sum, count := "duration_ms", totals.DurationSum, totals.Durations
			if c.Metric == "ttft_ms" {
				column, sum, count = "ttft_ms", totals.TtftSum, totals.Ttfts
			}
			m.Missing, m.Samples = m.Samples-count, count
			if count > 0 {
				if c.Aggregation == "mean" {
					m.Value = sum / count
				} else {
					percent := int64(95)
					if c.Aggregation == "p99" {
						percent = 99
					}
					rank := (count*percent + 99) / 100
					var values []int64
					if err := metricFacts(db, ctx, spec, target, m.WindowStart, m.WindowEnd).Where("outcome IN ? AND "+column+" >= 0", []string{"success", "failure", "timeout"}).Order(column+" asc").Offset(int(rank-1)).Limit(1).Pluck(column, &values).Error; err != nil {
						return m, err
					}
					if len(values) != 1 {
						m.Status, m.Reason = "partial", "concurrent_input_change"
					} else {
						m.Value = values[0]
					}
				}
			}
		case "consecutive_failures":
			var recent []model.AlertObservation
			if err := metricFacts(db, ctx, spec, target, m.WindowStart, m.WindowEnd).Select("outcome").Where("outcome IN ?", []string{"success", "failure", "timeout"}).Order("occurred_at desc, id desc").Limit(10001).Find(&recent).Error; err != nil {
				return m, err
			}
			for _, fact := range recent {
				if fact.Outcome == "success" {
					break
				}
				m.Value++
			}
			if m.Value > 10000 {
				m.Status, m.Reason = "partial", "failure_sequence_capacity"
			}
		default:
			return m, ErrInvalid
		}
		column, threshold := "duration_ms", max(c.SlowThresholdMS, c.ThresholdMS)
		if c.Metric == "ttft_ms" {
			column, threshold = "ttft_ms", c.ThresholdMS
		}
		for _, kind := range []string{"slow", "normal"} {
			query := terminalFacts(db.WithContext(ctx).Model(&model.AlertObservation{}), spec, target, m.WindowStart, m.WindowEnd).Where("request_id <> ?", "")
			if kind == "slow" {
				if threshold > 0 {
					query = query.Where(column+" > ?", threshold)
				} else {
					query = query.Where("outcome IN ?", []string{"failure", "timeout"})
					kind = "failed"
				}
			} else {
				query = query.Where("outcome = ?", "success")
				if threshold > 0 {
					query = query.Where(column+" <= ?", threshold)
				}
			}
			var examples []model.AlertObservation
			if err := query.Select("request_id", "layer", "channel_name", "model_key", "outcome", "started_at", "last_output_at", "occurred_at", "duration_ms", "ttft_ms").Order(column + " desc, id desc").Limit(3).Find(&examples).Error; err != nil {
				return m, err
			}
			for _, example := range examples {
				if kind != "normal" {
					m.RequestIDs = append(m.RequestIDs, example.RequestID)
				}
				m.Requests = append(m.Requests, RequestEvidence{Kind: kind, RequestID: example.RequestID, Layer: example.Layer, ChannelName: example.ChannelName, Model: example.ModelKey, Outcome: example.Outcome, StartedAt: example.StartedAt, LastOutputAt: example.LastOutputAt, FinishedAt: example.OccurredAt, DurationMS: example.DurationMS, TTFTMS: example.TTFTMS})
			}
		}
		layer := "request"
		if c.LatencyBasis == "channel_attempt" {
			layer = "channel_attempt"
		}
		active := targetFacts(db.WithContext(ctx).Model(&model.AlertLifecycle{}), target, spec).Where("layer = ? AND sample_class = ? AND finished_at = 0 AND started_at >= ? AND started_at <= ?", layer, "production", rule.EffectiveAt, m.WindowEnd)
		if target.ConfigVersion != "" {
			active = active.Where("config_version = ?", target.ConfigVersion)
		}
		var unfinished []model.AlertLifecycle
		if err := active.Select("request_id", "layer", "channel_name", "model_key", "started_at", "last_output_at").Order("started_at asc, id asc").Limit(3).Find(&unfinished).Error; err != nil {
			return m, err
		}
		for _, example := range unfinished {
			m.Requests = append(m.Requests, RequestEvidence{Kind: "inflight", RequestID: example.RequestID, Layer: example.Layer, ChannelName: example.ChannelName, Model: example.ModelKey, StartedAt: example.StartedAt, LastOutputAt: example.LastOutputAt})
		}
	}
	if spec.Trigger.Mode != "schedule_report" && c.Metric != "request_count" && m.Samples < int64(c.MinSamples) {
		m.Status, m.Reason = "warming_up", "insufficient_samples"
	}
	if c.Window.Type == "rolling" && now.UnixMilli()-rule.EffectiveAt < c.Window.DurationSeconds*1000 {
		m.Status, m.Reason = "warming_up", "window_coverage"
	}
	return m, nil
}

// The same dimensions and exact event-time boundaries apply to live samples
// and compacted measurements, including a migration starting mid-period.
func usageFacts(db *gorm.DB, spec model.AlertRuleSpec, target MonitoredTarget, start, end int64, columns string) *gorm.DB {
	return model.AlertMetricRows(db, func(q *gorm.DB) *gorm.DB {
		q = targetFacts(q, target, spec).Where("namespace = ?", "usage")
		if start > 0 {
			q = q.Where("occurred_at >= ?", start)
		}
		if end > 0 {
			q = q.Where("occurred_at < ?", end)
		}
		return q
	}, columns, true)
}

func acceptedAlertQuota(db *gorm.DB, target MonitoredTarget, start int64, amount *int64) error {
	return usageFacts(db, model.AlertRuleSpec{}, target, start, 0, "quota").Select("COALESCE(SUM(quota),0)").Scan(amount).Error
}

func unsupportedFacts(db *gorm.DB, target MonitoredTarget, spec model.AlertRuleSpec, start, end int64, inclusive bool) *gorm.DB {
	return model.AlertMetricRows(db, func(q *gorm.DB) *gorm.DB {
		q = targetFacts(q, target, spec).Where("namespace = ? AND sample_class = ? AND occurred_at >= ?", "health", "unsupported", start)
		if inclusive {
			return q.Where("occurred_at <= ?", end)
		}
		return q.Where("occurred_at < ?", end)
	}, "id", false)
}
