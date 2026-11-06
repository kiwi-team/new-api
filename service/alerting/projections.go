package alerting

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/go-redis/redis/v8"
)

type freshMetricKey struct{}
type metricSnapshot struct {
	Revision    int64       `json:"revision,string"`
	Measurement Measurement `json:"measurement"`
}

var metricRedisRetryAt atomic.Int64
var metricCacheHits atomic.Int64
var metricCacheMisses atomic.Int64
var metricRedisFailedClient atomic.Pointer[redis.Client]

func metricCacheEnabled(ctx context.Context, spec model.AlertRuleSpec) bool {
	fresh, _ := ctx.Value(freshMetricKey{}).(bool)
	return !fresh && spec.Condition.Window.Type != "cumulative" && spec.Condition.Window.Type != "month" && common.RedisEnabled && common.RDB != nil && (metricRedisFailedClient.Load() != common.RDB || time.Now().UnixMilli() >= metricRedisRetryAt.Load())
}

func metricNamespace(spec model.AlertRuleSpec) string {
	switch spec.Condition.Metric {
	case "spend_net", "input_tokens", "output_tokens", "cached_tokens", "cache_write_5m_tokens", "cache_write_1h_tokens":
		return "usage"
	}
	return "health"
}

func metricSnapshotKey(spec model.AlertRuleSpec, target MonitoredTarget) string {
	return model.AlertRedisKey("metric:" + model.AlertIdentity("window-v2", target.Key(), target.OwnerUserID, target.ConfigVersion, spec.Scope, spec.Condition, spec.QuotaUnit) + ":window")
}

// A snapshot is valid only for the exact window and SQL-committed revision.
// Late facts invalidate it, even when their event time precedes the window end.
func readMetricSnapshot(ctx context.Context, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement) (Measurement, bool) {
	metricCacheMisses.Add(1)
	revision, err := model.AlertMetricRevision(ctx, model.DB, metricNamespace(spec), m.WindowStart, m.WindowEnd)
	if err != nil {
		return m, false
	}
	work, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	text, err := common.RDB.Get(work, metricSnapshotKey(spec, target)).Result()
	if errors.Is(err, redis.Nil) {
		return m, false
	}
	if err != nil {
		metricRedisFailedClient.Store(common.RDB)
		metricRedisRetryAt.Store(time.Now().Add(5 * time.Second).UnixMilli())
		return m, false
	}
	var snapshot metricSnapshot
	if common.UnmarshalJsonStr(text, &snapshot) != nil || snapshot.Revision != revision || snapshot.Measurement.Status != "complete" || snapshot.Measurement.WindowStart != m.WindowStart || snapshot.Measurement.WindowEnd != m.WindowEnd {
		return m, false
	}
	metricCacheHits.Add(1)
	metricCacheMisses.Add(-1)
	return snapshot.Measurement, true
}

func writeMetricSnapshot(ctx context.Context, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, revision int64) {
	if m.Status != "complete" {
		return
	}
	payload, err := common.Marshal(metricSnapshot{Revision: revision, Measurement: m})
	if err != nil || len(payload) > 16384 {
		return
	}
	work, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	key := metricSnapshotKey(spec, target)
	// Bound active filter/target combinations and expire inactive entries. The
	// Lua script stores opaque JSON; it never adds monetary/token quantities.
	_, err = common.RDB.Eval(work, `redis.call('ZREMRANGEBYSCORE',KEYS[2],'-inf',ARGV[1]-120000); if not redis.call('ZSCORE',KEYS[2],KEYS[1]) and redis.call('ZCARD',KEYS[2])>=10000 then return 0 end; redis.call('ZADD',KEYS[2],ARGV[1],KEYS[1]); redis.call('EXPIRE',KEYS[2],180); redis.call('SET',KEYS[1],ARGV[2],'PX',120000); return 1`, []string{key, model.AlertRedisKey("metric:active-snapshots")}, time.Now().UnixMilli(), string(payload)).Result()
	if err != nil {
		metricRedisFailedClient.Store(common.RDB)
		metricRedisRetryAt.Store(time.Now().Add(5 * time.Second).UnixMilli())
	}
}

// MetricProjectionStats exposes non-sensitive per-node cache health to operators.
func MetricProjectionStats() (hits, misses int64) {
	return metricCacheHits.Load(), metricCacheMisses.Load()
}
