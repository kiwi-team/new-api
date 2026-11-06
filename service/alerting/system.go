package alerting

import (
	"context"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func systemMetric(metric string) bool {
	return slices.Contains([]string{"task_failure_count", "task_overdue", "evaluation_lag", "source_lag", "delivery_backlog"}, metric)
}

func measureSystem(ctx context.Context, spec model.AlertRuleSpec, m Measurement, now time.Time) (Measurement, error) {
	c := spec.Condition
	switch c.Metric {
	case "task_failure_count", "task_overdue":
		query := model.DB.WithContext(ctx).Model(&model.SystemTask{})
		if c.TaskType != "" {
			query = query.Where("type = ?", c.TaskType)
		}
		if c.Metric == "task_failure_count" {
			query = query.Where("status = ? AND updated_at >= ? AND updated_at < ?", model.SystemTaskStatusFailed, m.WindowStart/1000, m.WindowEnd/1000)
		} else {
			// Only accepted work creates an obligation. An idle on-demand
			// poller with no pending business is never declared overdue.
			query = query.Where("status IN ? AND created_at <= ?", []model.SystemTaskStatus{model.SystemTaskStatusPending, model.SystemTaskStatusRunning}, (now.UnixMilli()-c.DeadlineMS)/1000)
		}
		if err := query.Count(&m.Value).Error; err != nil {
			return m, err
		}
	case "delivery_backlog":
		if err := model.DB.WithContext(ctx).Model(&model.AlertJob{}).Where("kind = ? AND status IN ? AND available_at <= ?", "notify", []string{"pending", "retry_wait", "running"}, now.UnixMilli()).Count(&m.Value).Error; err != nil {
			return m, err
		}
	case "evaluation_lag":
		var checkpoint model.AlertWorkerCheckpoint
		result := model.DB.WithContext(ctx).Where("kind = ?", "evaluator").Order("updated_at desc").Limit(1).Find(&checkpoint)
		if result.Error != nil {
			return m, result.Error
		}
		if result.RowsAffected == 0 {
			m.Status, m.Reason = "warming_up", "evaluator_starting"
			return m, nil
		}
		m.Value = max(now.UnixMilli()-checkpoint.UpdatedAt, 0)
		if checkpoint.UpdatedAt > now.Add(5*time.Second).UnixMilli() {
			m.Status, m.Reason = "stale", "clock_skew"
		}
	case "source_lag":
		var instances []model.SystemInstance
		if err := model.DB.WithContext(ctx).Where("last_seen_at >= ?", now.Add(-90*time.Second).Unix()).Limit(101).Find(&instances).Error; err != nil {
			return m, err
		}
		if len(instances) == 0 {
			m.Status, m.Reason = "no_data", "producer_registry_empty"
			return m, nil
		}
		if len(instances) > 100 {
			m.Status, m.Reason = "partial", "source_capacity"
			return m, nil
		}
		sources, err := ProducerCheckpointsForInstances(ctx, instances)
		if err != nil {
			return m, err
		}
		byNode := make(map[string]model.AlertWorkerCheckpoint, len(sources))
		for _, source := range sources {
			byNode[source.NodeID] = source
		}
		for _, instance := range instances {
			source, found := byNode[instance.NodeName]
			if !found {
				m.Value = max(m.Value, max(now.UnixMilli()-instance.StartedAt*1000, 15000))
				continue
			}
			lag := max(now.UnixMilli()-source.UpdatedAt, 0)
			if source.CapturedAt > 0 {
				lag = max(lag, now.UnixMilli()-source.CapturedAt)
			}
			if !source.Healthy || source.AcceptedThrough < source.OfferedSequence || source.ProtocolVersion < model.AlertSourceProtocolVersion {
				lag = max(lag, 15000)
				if source.LastCompleteAt > 0 {
					lag = max(lag, now.UnixMilli()-source.LastCompleteAt)
				}
			}
			if source.UpdatedAt > now.Add(5*time.Second).UnixMilli() || source.CapturedAt > now.Add(5*time.Second).UnixMilli() {
				lag = max(lag, 15000)
			}
			m.Value = max(m.Value, lag)
		}
	default:
		return m, ErrInvalid
	}
	m.Samples = 1
	return m, nil
}
