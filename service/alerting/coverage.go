package alerting

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ProducerCheckpointsForInstances binds collection to the registry’s exact boot identities.
func ProducerCheckpointsForInstances(ctx context.Context, instances []model.SystemInstance) ([]model.AlertWorkerCheckpoint, error) {
	boots := make([]string, 0, len(instances))
	for _, instance := range instances {
		var info struct {
			Runtime struct {
				BootID string `json:"alerting_boot_id"`
			} `json:"runtime"`
		}
		if common.UnmarshalJsonStr(instance.Info, &info) == nil && info.Runtime.BootID != "" && len(info.Runtime.BootID) <= 64 {
			boots = append(boots, info.Runtime.BootID)
		}
	}
	if len(boots) == 0 {
		return []model.AlertWorkerCheckpoint{}, nil
	}
	var sources []model.AlertWorkerCheckpoint
	// Match the reporter's boot identity, never the largest wall-clock timestamp.
	// An old process with a fast clock must not conceal a newly restarted node.
	err := model.DB.WithContext(ctx).Where("kind = ? AND id IN ?", "producer", boots).Limit(1001).Find(&sources).Error
	return sources, err
}

// SourceWatermark uses the instance registry as the expected producer set.
// A heartbeat is insufficient while offered stream events are still pending.
func SourceWatermark(ctx context.Context, now time.Time) (int64, string, string, error) {
	var instances []model.SystemInstance
	if err := model.DB.WithContext(ctx).Where("last_seen_at >= ?", now.Add(-90*time.Second).Unix()).Limit(101).Find(&instances).Error; err != nil {
		return 0, "unknown", "registry_unavailable", err
	}
	if len(instances) == 0 {
		return 0, "no_data", "producer_registry_empty", nil
	}
	if len(instances) > 100 {
		return 0, "partial", "source_capacity", nil
	}
	rows, err := ProducerCheckpointsForInstances(ctx, instances)
	if err != nil {
		return 0, "unknown", "checkpoint_unavailable", err
	}
	if len(rows) > 1000 {
		return 0, "partial", "source_capacity", nil
	}
	sources := make(map[string]model.AlertWorkerCheckpoint, len(rows))
	for _, source := range rows {
		if _, found := sources[source.NodeID]; !found {
			sources[source.NodeID] = source
		}
	}
	watermark := now.UnixMilli()
	for _, instance := range instances {
		source, found := sources[instance.NodeName]
		if !found || source.UpdatedAt < instance.StartedAt*1000 {
			return 0, "partial", "producer_not_covered", nil
		}
		if source.ProtocolVersion < model.AlertSourceProtocolVersion || source.CapturedAt == 0 {
			return 0, "partial", "producer_not_covered", nil
		}
		if !source.Healthy || now.UnixMilli()-source.UpdatedAt > 15000 || now.UnixMilli()-source.CapturedAt > 15000 {
			if source.LastError == "clock_changed" {
				return 0, "stale", "clock_skew", nil
			}
			return 0, "stale", "producer_unavailable", nil
		}
		if source.UpdatedAt > now.Add(5*time.Second).UnixMilli() || source.CapturedAt > now.Add(5*time.Second).UnixMilli() {
			return 0, "stale", "clock_skew", nil
		}
		watermark = min(watermark, source.CapturedAt)
		if source.AcceptedThrough < source.OfferedSequence {
			return 0, "stale", "input_backlog", nil
		}
	}
	return watermark, "complete", "", nil
}

// WindowCoverage prevents a freshly restarted producer from claiming a full
// historical window merely because its current heartbeat is healthy. Missing
// historical intervals remain explicit instead of being filled with zeros.
func WindowCoverage(ctx context.Context, start, end int64, now time.Time) (string, string, error) {
	var instances []model.SystemInstance
	if err := model.DB.WithContext(ctx).Where("(started_at < ? AND last_seen_at >= ?) OR last_seen_at >= ?", end/1000, start/1000, now.Add(-90*time.Second).Unix()).Limit(101).Find(&instances).Error; err != nil {
		return "unknown", "registry_unavailable", err
	}
	if len(instances) == 0 {
		return "partial", "producer_registry_empty", nil
	}
	if len(instances) > 100 {
		return "partial", "source_capacity", nil
	}
	rows, err := ProducerCheckpointsForInstances(ctx, instances)
	if err != nil {
		return "unknown", "checkpoint_unavailable", err
	}
	if len(rows) > 1000 {
		return "partial", "source_capacity", nil
	}
	sources := make(map[string]model.AlertWorkerCheckpoint, len(rows))
	for _, source := range rows {
		if _, found := sources[source.NodeID]; !found {
			sources[source.NodeID] = source
		}
	}
	for _, instance := range instances {
		source, found := sources[instance.NodeName]
		if !found || source.ProtocolVersion < model.AlertSourceProtocolVersion || source.CoverageStartedAt == 0 {
			return "partial", "producer_not_covered", nil
		}
		if source.CoverageStartedAt >= end {
			// A prior boot can prove an entirely historical period. It cannot
			// bridge the unobserved restart gap or substitute for current liveness.
			var covered int64
			if err := model.DB.WithContext(ctx).Model(&model.AlertWorkerCheckpoint{}).
				Where("kind = ? AND node_id = ? AND id <> ? AND protocol_version >= ?", "producer", instance.NodeName, source.ID, model.AlertSourceProtocolVersion).
				Where("healthy = ? AND coverage_started_at > 0 AND coverage_started_at <= ? AND last_complete_at >= ?", true, start, end).
				Where("accepted_through >= offered_sequence AND updated_at <= ? AND captured_at <= ?", now.Add(5*time.Second).UnixMilli(), now.Add(5*time.Second).UnixMilli()).Count(&covered).Error; err != nil {
				return "unknown", "checkpoint_unavailable", err
			}
			if covered > 0 {
				continue
			}
		}
		// A node disappearing from the live registry must not turn a window
		// containing its missing terminal facts into an apparently healthy one.
		if !source.Healthy || source.AcceptedThrough < source.OfferedSequence || source.CapturedAt < end {
			return "partial", "historical_source_gap", nil
		}
		if source.CoverageStartedAt > start && source.CoverageStartedAt < end {
			return "warming_up", "producer_window_coverage", nil
		}
		if source.CoverageStartedAt >= end {
			return "partial", "producer_window_coverage", nil
		}
	}
	return "complete", "", nil
}

// UsageCoverage additionally checks the independent accounting input. Health
// heartbeats alone do not establish that a queued refund has been applied.
func UsageCoverage(ctx context.Context, now time.Time) (string, string, error) {
	var sources []model.AlertWorkerCheckpoint
	if err := model.DB.WithContext(ctx).Where("kind = ? AND updated_at >= ?", "producer", now.Add(-90*time.Second).UnixMilli()).Limit(101).Find(&sources).Error; err != nil {
		return "unknown", "checkpoint_unavailable", err
	}
	for _, source := range sources {
		if !source.UsageHealthy {
			return "partial", "usage_input_incomplete", nil
		}
	}
	ready, err := model.AlertUsageSourceReady(ctx)
	if err != nil {
		return "unknown", "usage_input_unavailable", err
	}
	if !ready {
		return "stale", "usage_input_backlog", nil
	}
	return "complete", "", nil
}
