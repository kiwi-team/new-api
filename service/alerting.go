package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/alerting"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm/clause"
)

var AlertEngine = &alerting.Engine{BootID: common.AlertingBootID}
var AlertTargetVault *alerting.TargetVault
var alertingStartOnce sync.Once
var alertingFailed atomic.Bool
var alertingOffered atomic.Int64
var alertingStartedAt = time.Now().UnixMilli()
var alertingClockStart = time.Now()
var alertingClockFailed atomic.Bool

// A clock jump invalidates the meaning of stored wall-clock windows. Keep the
// source unverified until a new boot establishes a fresh coverage boundary.
func alertingClockStable() bool {
	wall := time.Now().UnixMilli() - alertingClockStart.UnixMilli()
	drift := wall - time.Since(alertingClockStart).Milliseconds()
	if drift < -5000 || drift > 5000 {
		alertingClockFailed.Store(true)
	}
	return !alertingClockFailed.Load()
}

func StartAlerting() {
	alertingStartOnce.Do(func() {
		// Persisted controls survive disabling collection or rolling back rules.
		common.AlertingControlsEnabled = true
		common.AlertingEnabled = os.Getenv("ALERTING_ENABLED") == "true"
		common.AlertingActionsEnabled = os.Getenv("ALERTING_ACTIONS_ENABLED") == "true"
		common.AlertingShadowMode = common.GetEnvOrDefaultBool("ALERTING_SHADOW_MODE", true)
		var keys map[string]string
		if raw := os.Getenv("ALERTING_ENCRYPTION_KEYS"); raw != "" && common.UnmarshalJsonStr(raw, &keys) == nil {
			vault, err := alerting.NewTargetVault(os.Getenv("ALERTING_ACTIVE_KEY_ID"), keys)
			if err == nil {
				AlertTargetVault = vault
				AlertEngine.Deliver = vault.Deliver
			} else {
				common.SysError(common.LogText("alert target encryption key ring is invalid"))
			}
		}
		if err := model.InitializeAlertNamespace(context.Background()); err != nil {
			common.AlertingEnabled = false
			common.SysError(common.LogText("alerting deployment namespace initialization failed"))
			return
		}
		go runAlertingTransport()
		go runAlertingMaintenance()
		if !common.AlertingEnabled {
			return
		}
		alertingClockStart = time.Now()
		alertingStartedAt = alertingClockStart.UnixMilli()
		common.SetAlertHealthSink(recordAlertHealth)
		go runAlertingHeartbeat()
		// SQL leases/CAS provide leadership and job ownership. Running on every
		// enabled node permits takeover when the former evaluator disappears.
		go runAlertingEvaluation()
		go runAlertingJobs("notify", false)
		go runAlertingJobs("notify", true)
		go runAlertingJobs("action", false)
		go runAlertingJobs("action", true)
		go runAlertingRecovery()
		go runAlertingWatchdog()
	})
}

func runAlertingRecovery() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !alertingClockStable() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		if err := AlertEngine.RecoverHolds(ctx, time.Now()); err != nil {
			common.SysError(common.LogText("alerting recovery validation failed"))
		}
		cancel()
	}
}

func recordAlertHealth(_ context.Context, event common.AlertHealthEvent) error {
	now := event.OccurredAt
	id := model.AlertIdentity("lifecycle", event.Layer, event.SourceID)
	o := model.AlertObservation{ID: model.AlertIdentity("health", id, event.Sequence), Namespace: "health", SourceID: id, Kind: event.Kind, Layer: event.Layer, OccurredAt: now, UserID: event.UserID, TokenID: event.TokenID, ChannelID: event.ChannelID, ChannelName: event.ChannelName, ModelKey: event.Model, Protocol: event.Protocol, Stream: event.Stream, Outcome: event.Outcome, SampleClass: "production", DurationMS: event.DurationMS, TTFTMS: event.TTFTMS, RequestID: event.RequestID, NodeID: common.NodeName, BootID: AlertEngine.BootID, Sequence: event.Sequence, StartedAt: event.StartedAt, FirstOutputAt: event.FirstOutputAt, LastOutputAt: event.LastOutputAt, OutputCovered: event.OutputCovered, AttributionCovered: event.AttributionCovered}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	o.SourceSequence = alertingOffered.Add(1)
	o.ConfigVersion = event.ConfigVersion
	if event.SampleClass != "" {
		o.SampleClass = event.SampleClass
	}
	if !alertingClockStable() {
		o.SampleClass = "unsupported"
	}
	err := model.EnqueueAlertObservation(ctx, o)
	if err != nil {
		alertingFailed.Store(true)
	}
	return err
}

func runAlertingHeartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	// Publish the previous snapshot so continuous traffic does not require a
	// moment with zero concurrent writes. SQL proves its entire offered prefix.
	capturedAt, offered, publicationCovered := alertingStartedAt-1, int64(0), true
	for {
		nextCapturedAt, nextCovered := common.AlertPublicationWatermark(time.Now())
		nextOffered := alertingOffered.Load()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		row := model.AlertWorkerCheckpoint{ID: AlertEngine.BootID, NodeID: common.NodeName, BootID: AlertEngine.BootID, Kind: "producer", UpdatedAt: time.Now().UnixMilli(), Healthy: !alertingFailed.Load() && publicationCovered}
		if !alertingClockStable() {
			row.Healthy, row.LastError = false, "clock_changed"
		}
		row.CoverageStartedAt, row.ProtocolVersion = alertingStartedAt, model.AlertSourceProtocolVersion
		row.UsageHealthy = model.AlertLocalUsageReady()
		row.OfferedSequence, row.CapturedAt = offered, capturedAt
		err := model.DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"node_id", "boot_id", "kind", "updated_at", "healthy", "last_error", "offered_sequence", "captured_at", "coverage_started_at", "protocol_version", "usage_healthy"})}).Create(&row).Error
		if err == nil {
			err = model.VerifyAlertSourcePrefix(ctx, row.ID)
		}
		if err != nil {
			common.SysError(common.LogText("alerting producer checkpoint failed"))
		}
		cancel()
		capturedAt, offered, publicationCovered = nextCapturedAt, nextOffered, nextCovered
		<-ticker.C
	}
}

func runAlertingTransport() {
	var consumer *redis.Client
	if common.RedisEnabled && common.RDB != nil {
		options := common.ParseRedisOption()
		options.PoolSize = 2
		consumer = redis.NewClient(options)
		defer consumer.Close()
	}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := model.ReplayAlertSpool(ctx)
		if consumer != nil {
			if e := model.ConsumeAlertHealth(ctx, consumer, AlertEngine.BootID); e != nil {
				err = e
			}
			if e := model.ConsumeAlertStream(ctx, consumer, AlertEngine.BootID, model.AlertUsageStream); e != nil {
				err = e
			}
			if e := model.PublishAlertOutbox(ctx, common.RDB); e != nil {
				err = e
			}
		}
		cancel()
		if err != nil {
			common.SysError(common.LogText("alerting transport is degraded; durable inputs retained"))
		}
		time.Sleep(time.Second)
	}
}

func runAlertingEvaluation() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !alertingClockStable() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		owner := AlertEngine.BootID + "-evaluate"
		ok, err := model.AcquireResourceLease(ctx, "alerting:evaluate", owner, time.Now().Unix()+30)
		if err == nil && ok {
			err = AlertEngine.Evaluate(ctx, time.Now())
			if err == nil {
				checkpoint := model.AlertWorkerCheckpoint{ID: model.AlertIdentity("evaluator", AlertEngine.BootID), BootID: AlertEngine.BootID, NodeID: common.NodeName, Kind: "evaluator", UpdatedAt: time.Now().UnixMilli(), Healthy: true}
				err = model.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&checkpoint).Error
			}
			_ = model.ReleaseResourceLease(ctx, "alerting:evaluate", owner)
		}
		if err != nil {
			common.SysError(common.LogText("alerting evaluation failed"))
		}
		cancel()
	}
}

func runAlertingJobs(kind string, urgent bool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !alertingClockStable() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		if !urgent && common.RedisEnabled && common.RDB != nil {
			_ = model.ReadAlertWakeups(ctx, common.RDB, kind, AlertEngine.BootID)
		}
		var err error
		if urgent {
			err = AlertEngine.RunUrgentJobs(ctx, kind, time.Now())
		} else {
			err = AlertEngine.RunJobs(ctx, kind, time.Now())
		}
		if err != nil {
			common.SysError(common.LogText("alerting %s worker failed", kind))
		}
		cancel()
	}
}

func AlertEngineHealth(ctx context.Context) (map[string]any, error) {
	var instances []model.SystemInstance
	if err := model.DB.WithContext(ctx).Where("last_seen_at >= ?", time.Now().Add(-90*time.Second).Unix()).Limit(101).Find(&instances).Error; err != nil {
		return nil, err
	}
	sources, err := alerting.ProducerCheckpointsForInstances(ctx, instances)
	if err != nil {
		return nil, err
	}
	covered := make(map[string]bool, len(sources))
	evaluatorIDs := make([]string, 0, len(sources))
	for _, source := range sources {
		covered[source.NodeID] = true
		evaluatorIDs = append(evaluatorIDs, model.AlertIdentity("evaluator", source.BootID))
	}
	uncovered := make([]string, 0)
	for _, instance := range instances {
		if !covered[instance.NodeName] {
			uncovered = append(uncovered, instance.NodeName)
		}
	}
	if len(evaluatorIDs) > 0 {
		var evaluators []model.AlertWorkerCheckpoint
		if err := model.DB.WithContext(ctx).Where("id IN ? AND kind = ?", evaluatorIDs, "evaluator").Find(&evaluators).Error; err != nil {
			return nil, err
		}
		sources = append(sources, evaluators...)
	}
	var backlog int64
	if err := model.DB.WithContext(ctx).Model(&model.AlertJob{}).Where("status IN ?", []string{"pending", "retry_wait", "running", "blocked"}).Count(&backlog).Error; err != nil {
		return nil, err
	}
	var failedRules int64
	if err := model.DB.WithContext(ctx).Model(&model.AlertRule{}).Where("enabled = ? AND last_evaluation_error <> ?", true, "").Count(&failedRules).Error; err != nil {
		return nil, err
	}
	mode := model.AlertReceiveMode()
	hits, misses := alerting.MetricProjectionStats()
	return map[string]any{"metric_cache_hits": strconv.FormatInt(hits, 10), "metric_cache_misses": strconv.FormatInt(misses, 10), "failed_rules": failedRules, "enabled": common.AlertingEnabled, "actions_enabled": common.AlertingActionsEnabled, "shadow_mode": common.AlertingShadowMode, "encryption_configured": AlertTargetVault != nil, "receive_mode": mode, "sources": sources, "uncovered_nodes": uncovered, "job_backlog": strconv.FormatInt(backlog, 10)}, nil
}

// Maintenance only deletes disposable delivery hints and expired limit rows.
// Immutable receipts, cumulative progress, active holds and unresolved lifecycles
// remain authoritative and are deliberately retained.
func runAlertingMaintenance() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if !alertingClockStable() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := model.CleanAlertDeliveryHints(ctx, time.Now()); err != nil {
			common.SysError(common.LogText("alerting hint cleanup failed"))
		}
		cancel()
	}
}

func runAlertingWatchdog() {
	targets := make([]int64, 0, 10)
	for text := range strings.SplitSeq(os.Getenv("ALERTING_WATCHDOG_TARGET_IDS"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err == nil && id > 0 && len(targets) < 10 {
			targets = append(targets, id)
		}
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !alertingClockStable() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		owner := AlertEngine.BootID + "-watchdog"
		ok, err := model.AcquireResourceLease(ctx, "alerting:watchdog", owner, time.Now().Add(10*time.Second).Unix())
		if err == nil && ok {
			err = AlertEngine.Watchdog(ctx, targets, time.Now())
			_ = model.ReleaseResourceLease(ctx, "alerting:watchdog", owner)
		}
		if err != nil {
			common.SysError(common.LogText("alerting independent watchdog is unavailable"))
		}
		cancel()
	}
}
