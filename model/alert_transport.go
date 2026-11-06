package model

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var alertRedisPrefix string
var AlertHealthStream string
var AlertUsageStream string

// InitializeAlertNamespace must finish before producers or workers start. It
// persists a random identity instead of mixing independent databases in Redis.
func InitializeAlertNamespace(ctx context.Context) error {
	row := AlertDeployment{ID: 1, Namespace: common.GetUUID()}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	if err := DB.WithContext(ctx).First(&row, 1).Error; err != nil {
		return err
	}
	if len(row.Namespace) < 16 || strings.ContainsAny(row.Namespace, "/\\:") {
		return errors.New("invalid alert deployment namespace")
	}
	alertRedisPrefix = row.Namespace + ":new-api:alerts:v1:"
	AlertHealthStream = AlertRedisKey("ingest:health")
	AlertUsageStream = AlertRedisKey("ingest:usage")
	return nil
}

func AlertRedisKey(suffix string) string { return alertRedisPrefix + suffix }

const alertHealthGroup = "alerts-health-v1"
const alertPayloadLimit = 32 * 1024

var alertRedisRetryAt atomic.Int64
var alertRedisFailedClient atomic.Pointer[redis.Client]

// EnqueueAlertObservation uses its own stream; quota_data's XDEL lifecycle
// cannot delete observations before this module has applied them.
func EnqueueAlertObservation(ctx context.Context, observation AlertObservation) error {
	payload, err := common.Marshal(observation)
	if err != nil {
		return err
	}
	if len(payload) > alertPayloadLimit {
		return errors.New("alert observation exceeds size limit")
	}
	if common.RedisEnabled && common.RDB != nil && common.GetEnvOrDefaultBool("ALERTING_REDIS_INGEST_ENABLED", false) && (alertRedisFailedClient.Load() != common.RDB || time.Now().UnixMilli() >= alertRedisRetryAt.Load()) {
		stream := AlertHealthStream
		if observation.Namespace == "usage" {
			stream = AlertUsageStream
		}
		redisCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		// Refuse before capacity instead of trimming unacknowledged facts.
		// 2,048 bounded payloads cap each stream at 64 MiB of payload.
		_, err = common.RDB.Eval(redisCtx, `if redis.call('XLEN',KEYS[1])>=2048 then return redis.error_reply('alert ingress capacity') end; return redis.call('XADD',KEYS[1],'*','event',ARGV[1])`, []string{stream}, string(payload)).Result()
		cancel()
		if err == nil {
			return nil
		}
		alertRedisFailedClient.Store(common.RDB)
		alertRedisRetryAt.Store(time.Now().Add(5 * time.Second).UnixMilli())
	}
	if err = AcceptAlertObservation(ctx, observation); err == nil {
		return nil
	}
	// A synced local spool is the last durable boundary when both dependencies
	// fail. Source coverage remains unhealthy until replay is proven complete.
	if spoolErr := SpoolAlertObservation(payload); spoolErr != nil {
		return errors.Join(err, spoolErr)
	}
	return nil
}

func alertSpoolDir() string {
	if dir := os.Getenv("ALERTING_SPOOL_DIR"); dir != "" {
		return filepath.Join(filepath.Clean(dir), strings.Split(alertRedisPrefix, ":")[0])
	}
	return filepath.Join("alerting-spool", strings.Split(alertRedisPrefix, ":")[0])
}

func SpoolAlertObservation(payload []byte) error {
	dir := alertSpoolDir()
	if err := os.MkdirAll(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := syncQuotaDataSpoolDir(filepath.Dir(dir)); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(payload)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, common.GetUUID()+".json")); err != nil {
		return err
	}
	return syncQuotaDataSpoolDir(dir)
}

func ReplayAlertSpool(ctx context.Context) error {
	dir := alertSpoolDir()
	f, err := os.Open(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(101)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	quarantined := false
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		source, e := os.Open(path)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		payload, e := io.ReadAll(io.LimitReader(source, alertPayloadLimit+1))
		e = errors.Join(e, source.Close())
		if e != nil {
			return e
		}
		var observation AlertObservation
		if len(payload) > alertPayloadLimit || common.Unmarshal(payload, &observation) != nil {
			e = ErrInvalidAlertObservation
		} else {
			e = AcceptAlertObservation(ctx, observation)
		}
		if e != nil {
			if !errors.Is(e, ErrInvalidAlertObservation) && !errors.Is(e, ErrAlertObservationConflict) {
				return e
			}
			failure := AlertDeadLetter{ID: AlertIdentity("spool", entry.Name()), Source: "spool", PayloadHash: AlertIdentity(string(payload)), ErrorCode: "invalid_observation", CreatedAt: time.Now().UnixMilli()}
			if e = DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&failure).Error; e != nil {
				return e
			}
			// Retain the original bytes for authorized repair, outside the replay
			// directory so poison files cannot starve later valid observations.
			quarantine := filepath.Join(dir, "quarantine")
			if e = os.MkdirAll(quarantine, 0700); e != nil {
				return e
			}
			if e = os.Rename(path, filepath.Join(quarantine, entry.Name()+".quarantined")); e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
			if e = syncQuotaDataSpoolDir(quarantine); e != nil {
				return e
			}
			quarantined = true
			continue
		}
		if e = os.Remove(path); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	if err := syncQuotaDataSpoolDir(dir); err != nil {
		return err
	}
	if quarantined {
		return ErrInvalidAlertObservation
	}
	return nil
}

// ConsumeAlertHealth commits SQL before XACK/XDEL. Expired pending deliveries
// are reclaimed, and exact duplicates keep the same SQL receipt identity.
func ConsumeAlertHealth(ctx context.Context, client *redis.Client, consumer string) error {
	return ConsumeAlertStream(ctx, client, consumer, AlertHealthStream)
}

func ConsumeAlertStream(ctx context.Context, client *redis.Client, consumer, stream string) error {
	// Drain bursts without sleeping between full batches, while retaining a
	// bound so neither source can monopolize the transport worker.
	for range 10 {
		count, err := consumeAlertBatch(ctx, client, consumer, stream)
		if err != nil || count < 100 {
			return err
		}
	}
	return nil
}

func consumeAlertBatch(ctx context.Context, client *redis.Client, consumer, stream string) (int, error) {
	if err := client.XGroupCreateMkStream(ctx, stream, alertHealthGroup, "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return 0, err
	}
	// Filter idle time locally: XPENDING's IDLE option requires Redis 6.2,
	// whereas the interval form and XCLAIM also work on Redis 5.
	pending, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: stream, Group: alertHealthGroup, Start: "-", End: "+", Count: 100}).Result()
	if err != nil {
		return 0, err
	}
	var messages []redis.XMessage
	if len(pending) > 0 {
		ids := make([]string, 0, len(pending))
		for _, row := range pending {
			if row.Idle >= time.Minute {
				ids = append(ids, row.ID)
			}
		}
		if len(ids) > 0 {
			messages, err = client.XClaim(ctx, &redis.XClaimArgs{Stream: stream, Group: alertHealthGroup, Consumer: consumer, MinIdle: time.Minute, Messages: ids}).Result()
			if err != nil {
				return 0, err
			}
		}
	}
	if len(messages) == 0 {
		streams, e := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: alertHealthGroup, Consumer: consumer, Streams: []string{stream, ">"}, Count: 100, Block: -1}).Result()
		if errors.Is(e, redis.Nil) {
			return 0, nil
		}
		if e != nil {
			return 0, e
		}
		for _, stream := range streams {
			messages = append(messages, stream.Messages...)
		}
	}
	observations := make([]AlertObservation, 0, len(messages))
	for _, message := range messages {
		payload, ok := message.Values["event"].(string)
		var observation AlertObservation
		if ok && len(payload) <= alertPayloadLimit && common.UnmarshalJsonStr(payload, &observation) == nil {
			observations = append(observations, observation)
		}
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return AcceptAlertObservationsTx(tx, observations) })
	if err != nil && !errors.Is(err, ErrInvalidAlertObservation) && !errors.Is(err, ErrAlertObservationConflict) {
		return 0, err
	}
	batchAccepted := err == nil
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		payload, ok := message.Values["event"].(string)
		var observation AlertObservation
		err = nil
		if !ok || len(payload) > alertPayloadLimit || common.UnmarshalJsonStr(payload, &observation) != nil {
			err = ErrInvalidAlertObservation
		} else if !batchAccepted {
			err = AcceptAlertObservation(ctx, observation)
		}
		if err != nil {
			if !errors.Is(err, ErrInvalidAlertObservation) && !errors.Is(err, ErrAlertObservationConflict) {
				return 0, err
			}
			failure := AlertDeadLetter{ID: AlertIdentity("stream", stream, message.ID), Source: stream, PayloadHash: AlertIdentity(payload), ErrorCode: "invalid_observation", CreatedAt: time.Now().UnixMilli()}
			if err = DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&failure).Error; err != nil {
				return 0, err
			}
		}
		ids = append(ids, message.ID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	_, err = client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.XAck(ctx, stream, alertHealthGroup, ids...)
		pipe.XDel(ctx, stream, ids...)
		return nil
	})
	return len(ids), err
}

// PublishAlertOutbox publishes SQL-committed projections and wakeups only.
// Redis keys are disposable, versioned hints; a missing key is never zero data.
func PublishAlertOutbox(ctx context.Context, client *redis.Client) error {
	var rows []AlertOutbox
	if err := DB.WithContext(ctx).Where("published_at = 0").Order("created_at asc").Limit(1000).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.Kind == "projection" {
			var fact AlertObservation
			if err := DB.WithContext(ctx).First(&fact, "id = ?", row.Reference).Error; err != nil {
				return err
			}
			if err := publishAlertMetricBucket(ctx, client, fact); err != nil {
				return err
			}
			if fact.Namespace == "health" && fact.SampleClass != "recovery_probe" && fact.SampleClass != "transport" {
				// Project the current SQL lifecycle, never the delivery order of
				// outbox rows. A late started/progress event cannot undo terminal state.
				var lifecycle AlertLifecycle
				found := DB.WithContext(ctx).Where("id = ?", fact.SourceID).Limit(1).Find(&lifecycle)
				if found.Error != nil {
					return found.Error
				}
				var err error
				if found.RowsAffected > 0 && lifecycle.FinishedAt == 0 && lifecycle.SampleClass == "production" {
					// Active IDs are disposable hints; SQL retains every unresolved
					// lifecycle. Bound their shared-cache footprint after a node loss.
					_, err = client.Eval(ctx, `redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',ARGV[3]); if redis.call('ZSCORE',KEYS[1],ARGV[2]) or redis.call('ZCARD',KEYS[1])<10000 then redis.call('ZADD',KEYS[1],ARGV[1],ARGV[2]); return 1 end; return 0`, []string{AlertRedisKey("active")}, lifecycle.StartedAt, lifecycle.ID, time.Now().Add(-26*time.Hour).UnixMilli()).Result()
				} else {
					err = client.ZRem(ctx, AlertRedisKey("active"), fact.SourceID).Err()
				}
				if err != nil {
					return err
				}
			}
		} else {
			// At-least-once wakeups contain references, never credentials. The
			// SQL worker's CAS lease decides whether any job can execute.
			var job AlertJob
			id, err := strconv.ParseInt(row.Reference, 10, 64)
			if err != nil {
				return err
			}
			if err = DB.WithContext(ctx).First(&job, id).Error; err != nil {
				return err
			}
			if job.Status != "pending" && job.Status != "retry_wait" {
				if err = DB.WithContext(ctx).Model(&row).Update("published_at", time.Now().UnixMilli()).Error; err != nil {
					return err
				}
				continue
			}
			// Hints are disposable because SQL owns every job. Bound an idle lane
			// instead of letting repeated wakes consume the shared Redis budget.
			_, err = client.Eval(ctx, `if redis.call('XLEN',KEYS[1])>=2048 then return 0 end; redis.call('XADD',KEYS[1],'*','job',ARGV[1]); return 1`, []string{AlertRedisKey("ready:" + row.Kind + ":P" + strconv.Itoa(job.Priority))}, row.Reference).Result()
			if err != nil {
				return err
			}
			_, err = client.Eval(ctx, `redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',ARGV[3]); if redis.call('ZSCORE',KEYS[1],ARGV[2]) or redis.call('ZCARD',KEYS[1])<10000 then redis.call('ZADD',KEYS[1],ARGV[1],ARGV[2]) end; return 1`, []string{AlertRedisKey("due:" + row.Kind)}, job.AvailableAt, row.Reference, time.Now().Add(-time.Hour).UnixMilli()).Result()
			if err != nil {
				return err
			}
		}
		if err := DB.WithContext(ctx).Model(&AlertOutbox{}).Where("id = ? AND published_at = 0", row.ID).Update("published_at", time.Now().UnixMilli()).Error; err != nil {
			return err
		}
	}
	return nil
}

// ReadAlertWakeups isolates action and notification queues and reserves their
// priorities. SQL jobs remain claimable when a wakeup is lost or acknowledged
// before execution, which is essential to recovering a crashed worker.
func ReadAlertWakeups(ctx context.Context, client *redis.Client, kind, consumer string) error {
	streams := make([]string, 0, 8)
	for priority := range 4 {
		stream := AlertRedisKey("ready:" + kind + ":P" + strconv.Itoa(priority))
		if err := client.XGroupCreateMkStream(ctx, stream, "alert-workers-v1", "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			return err
		}
		pending, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: stream, Group: "alert-workers-v1", Start: "-", End: "+", Count: 50}).Result()
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			ids := make([]string, 0, len(pending))
			for _, entry := range pending {
				if entry.Idle >= time.Minute {
					ids = append(ids, entry.ID)
				}
			}
			if len(ids) == 0 {
				streams = append(streams, stream)
				continue
			}
			if err := client.XAck(ctx, stream, "alert-workers-v1", ids...).Err(); err != nil {
				return err
			}
			if err := client.XDel(ctx, stream, ids...).Err(); err != nil {
				return err
			}
		}
		streams = append(streams, stream)
	}
	for range 4 {
		streams = append(streams, ">")
	}
	batch, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: "alert-workers-v1", Consumer: consumer, Streams: streams, Count: 20, Block: -1}).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, stream := range batch {
		for _, message := range stream.Messages {
			if err = client.XAck(ctx, stream.Stream, "alert-workers-v1", message.ID).Err(); err != nil {
				return err
			}
			if err = client.XDel(ctx, stream.Stream, message.ID).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Shared limits fail to the same SQL counter during Redis outages. The SQL
// counter is always reserved first, so changing paths cannot double the limit.
var errAlertNotificationLimited = errors.New("alert notification rate limited")

func ReserveAlertNotification(ctx context.Context, target AlertTarget, priority int, now time.Time) (bool, error) {
	limit := 20
	if priority <= 1 {
		limit = 60
	}
	if target.Type == "wecom" {
		limit = 10
		if priority <= 1 {
			limit = 20
		}
	}
	destination := target.RateKey
	if destination == "" {
		destination = AlertIdentity("target", target.ID)
	}
	secondLimit := 2
	if priority <= 1 {
		secondLimit = 5
	}
	var allowed bool
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, window := range []struct {
			period int64
			limit  int
		}{{60, limit}, {1, secondLimit}} {
			id := AlertIdentity("notify-limit", destination, window.period, now.Unix()/window.period)
			row := AlertNotificationLimit{ID: id, ExpiresAt: now.Add(2 * time.Minute).UnixMilli()}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			result := tx.Model(&AlertNotificationLimit{}).Where("id = ? AND count < ?", id, window.limit).Update("count", gorm.Expr("count + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errAlertNotificationLimited
			}
		}
		allowed = true
		return nil
	})
	if errors.Is(err, errAlertNotificationLimited) {
		return false, nil
	}
	if err == nil && allowed && common.RedisEnabled && common.RDB != nil {
		id := AlertIdentity("notify-limit", destination, now.Unix()/60)
		_, _ = common.RDB.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],120) end; return n`, []string{AlertRedisKey("notify-limit:" + id)}).Result()
	}
	return allowed, err
}

// CleanAlertDeliveryHints bounds each deletion so cleanup cannot monopolize a
// database connection. Pending outbox rows and fact receipts are never removed.
func CleanAlertDeliveryHints(ctx context.Context, now time.Time) error {
	if err := cleanAlertMetricBuckets(ctx, now); err != nil {
		return err
	}
	var limits []string
	if err := DB.WithContext(ctx).Model(&AlertNotificationLimit{}).Where("expires_at < ?", now.UnixMilli()).Limit(500).Pluck("id", &limits).Error; err != nil {
		return err
	}
	if len(limits) > 0 {
		if err := DB.WithContext(ctx).Where("id IN ?", limits).Delete(&AlertNotificationLimit{}).Error; err != nil {
			return err
		}
	}
	var outbox []string
	if err := DB.WithContext(ctx).Model(&AlertOutbox{}).Where("published_at > 0 AND published_at < ?", now.Add(-48*time.Hour).UnixMilli()).Limit(500).Pluck("id", &outbox).Error; err != nil {
		return err
	}
	if len(outbox) > 0 {
		return DB.WithContext(ctx).Where("id IN ? AND published_at > 0", outbox).Delete(&AlertOutbox{}).Error
	}
	return nil
}

func AlertReceiveMode() string {
	if !common.RedisEnabled || common.RDB == nil || !common.GetEnvOrDefaultBool("ALERTING_REDIS_INGEST_ENABLED", false) {
		return "sql_first"
	}
	if alertRedisFailedClient.Load() == common.RDB && time.Now().UnixMilli() < alertRedisRetryAt.Load() {
		return "sql_fallback"
	}
	return "redis_stream"
}
