package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	quotaDataStream          = "quota_data:v1:events"
	quotaDataConsumerGroup   = "quota-data-writers-v1"
	quotaDataMaxPayloadBytes = 32 * 1024
	quotaDataBatchBytes      = 1024 * 1024
)

func enqueueQuotaDataEvent(event QuotaDataEvent) error {
	var client *redis.Client
	if common.RedisEnabled && common.GetEnvOrDefaultBool("QUOTA_DATA_REDIS_INGEST_ENABLED", true) {
		client = common.RDB
	}
	return deliverQuotaDataEvent(event, client, quotaDataStream)
}

func deliverQuotaDataEvent(event QuotaDataEvent, client *redis.Client, stream string) error {
	if err := event.validate(); err != nil {
		return err
	}
	encoded, err := common.Marshal(event)
	if err != nil {
		return err
	}
	if len(encoded) > quotaDataMaxPayloadBytes {
		return fmt.Errorf("%w: payload too large", errInvalidQuotaDataEvent)
	}
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"event": string(encoded)}}).Err()
		cancel()
		if err == nil {
			return nil
		}
		common.SysError(common.LogText("Quota data Redis enqueue failed; persisting the same event in the database"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = persistQuotaDataEvent(ctx, event, string(encoded))
	if err == nil || errors.Is(err, errInvalidQuotaDataEvent) {
		return err
	}
	if spoolErr := spoolQuotaDataEvent(encoded); spoolErr != nil {
		return errors.Join(err, fmt.Errorf("quota data spool: %w", spoolErr))
	}
	common.SysError(common.LogText("Quota data event saved to local spool; awaiting database recovery: id=%s", event.EventID))
	return nil
}

// StartQuotaDataWorkers runs every source regardless of DataExportEnabled:
// disabling production must not strand previously accepted events.
func StartQuotaDataWorkers(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var workers sync.WaitGroup
		workers.Go(func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				workCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				spooled, spoolErr := replayQuotaDataSpool(workCtx)
				count, err := consumeQuotaDataInbox(workCtx, false)
				cancel()
				if spoolErr != nil && ctx.Err() == nil {
					common.SysError(common.LogText("Quota data spool replay failed; files retained: %v", spoolErr))
				}
				if err != nil && ctx.Err() == nil {
					common.SysError(common.LogText("Quota data inbox processing failed: %v", err))
				}
				if (count == quotaDataBatchSize || spooled == quotaDataBatchSize) && err == nil && spoolErr == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		})
		if common.RedisEnabled && common.RDB != nil {
			options := common.ParseRedisOption()
			options.PoolSize = 2
			client := redis.NewClient(options)
			workers.Go(func() { defer client.Close(); consumeQuotaDataStream(ctx, client, quotaDataStream, uuid.NewString()) })
		}
		workers.Wait()
	}()
	return done
}

func consumeQuotaDataInbox(ctx context.Context, force bool) (int, error) {
	query := DB.WithContext(ctx).Where("state = ?", "pending")
	if !force {
		query = query.Where("next_attempt <= ?", time.Now().Unix())
	}
	var rows []QuotaDataEventRecord
	if err := query.Order("created_at, event_id").Limit(quotaDataBatchSize).Find(&rows).Error; err != nil {
		return 0, err
	}
	bytes := 0
	for i, row := range rows {
		if i > 0 && bytes+len(row.Payload) > quotaDataBatchBytes {
			rows = rows[:i]
			break
		}
		bytes += len(row.Payload)
	}
	events := make([]QuotaDataEvent, 0, len(rows))
	for _, row := range rows {
		var event QuotaDataEvent
		err := common.UnmarshalJsonStr(row.Payload, &event)
		if err == nil {
			err = event.validate()
		}
		if err == nil && (row.EventID != event.EventID || row.PayloadHash != event.PayloadHash) {
			err = fmt.Errorf("%w: inbox identity mismatch", errInvalidQuotaDataEvent)
		}
		if err != nil {
			if updateErr := DB.WithContext(ctx).Model(&QuotaDataEventRecord{}).Where("event_id = ? AND state = ?", row.EventID, "pending").Updates(map[string]any{"state": "quarantined", "last_error": err.Error()}).Error; updateErr != nil {
				return len(rows), updateErr
			}
			common.SysError(common.LogText("Quota data inbox event quarantined: id=%s", row.EventID))
			continue
		}
		events = append(events, event)
	}
	err := applyQuotaDataEvents(ctx, events)
	if !errors.Is(err, errInvalidQuotaDataEvent) {
		if err != nil && ctx.Err() == nil && len(events) > 0 {
			ids := make([]string, 0, len(events))
			for _, event := range events {
				ids = append(ids, event.EventID)
			}
			retryErr := DB.WithContext(ctx).Model(&QuotaDataEventRecord{}).Where("event_id IN ? AND state = ?", ids, "pending").
				Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "next_attempt": time.Now().Unix() + 5, "last_error": err.Error()}).Error
			err = errors.Join(err, retryErr)
		}
		return len(rows), err
	}
	// A malformed/conflicting event must not prevent independent valid events
	// from being applied. Ordinary transaction failures retry the whole batch.
	for _, event := range events {
		if err = applyQuotaDataEvents(ctx, []QuotaDataEvent{event}); err == nil {
			continue
		}
		if !errors.Is(err, errInvalidQuotaDataEvent) {
			return len(rows), err
		}
		if err = DB.WithContext(ctx).Model(&QuotaDataEventRecord{}).Where("event_id = ? AND state = ?", event.EventID, "pending").Updates(map[string]any{"state": "quarantined", "last_error": err.Error()}).Error; err != nil {
			return len(rows), err
		}
		common.SysError(common.LogText("Quota data inbox event quarantined: id=%s", event.EventID))
	}
	return len(rows), nil
}

func consumeQuotaDataStream(ctx context.Context, client *redis.Client, stream, consumer string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	pendingStart := "-"
	cleanupStart := "-"
	cleanupAt := time.Time{}
	for ctx.Err() == nil {
		err := client.XGroupCreateMkStream(ctx, stream, quotaDataConsumerGroup, "0-0").Err()
		if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
			common.SysError(common.LogText("Quota data stream unavailable; waiting for Redis recovery"))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		// XPENDING/XCLAIM works with the pinned go-redis v8 parser and Redis
		// versions that return different XAUTOCLAIM response shapes.
		pending, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{Stream: stream, Group: quotaDataConsumerGroup, Start: pendingStart, End: "+", Count: quotaDataBatchSize, Idle: time.Minute}).Result()
		if err == nil && len(pending) > 0 {
			ids := make([]string, 0, len(pending))
			for _, item := range pending {
				ids = append(ids, item.ID)
			}
			pendingStart = "(" + ids[len(ids)-1]
			messages, claimErr := client.XClaim(ctx, &redis.XClaimArgs{Stream: stream, Group: quotaDataConsumerGroup, Consumer: consumer, MinIdle: time.Minute, Messages: ids}).Result()
			if claimErr == nil {
				err = processQuotaDataMessages(ctx, client, stream, messages)
			} else {
				err = claimErr
			}
		} else {
			pendingStart = "-"
		}
		if err == nil {
			messages, readErr := readQuotaDataBatch(ctx, client, stream, consumer)
			err = readErr
			if err == nil {
				err = processQuotaDataMessages(ctx, client, stream, messages)
			}
		}
		if time.Since(cleanupAt) >= time.Minute {
			cleanupAt = time.Now()
			next, cleanupErr := cleanupAppliedQuotaDataMessages(ctx, client, stream, cleanupStart)
			if cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			} else {
				cleanupStart = next
			}
		}
		if err != nil && ctx.Err() == nil {
			common.SysError(common.LogText("Quota data stream processing failed; unacknowledged events retained: %v", err))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}
}

// COUNT alone does not wait for a full batch: Redis returns as soon as one
// message arrives. Coalesce arrivals up to the count/byte/time bounds.
func readQuotaDataBatch(ctx context.Context, client *redis.Client, stream, consumer string) ([]redis.XMessage, error) {
	deadline := time.Now().Add(time.Second)
	batch := make([]redis.XMessage, 0, quotaDataBatchSize)
	size := 0
	for len(batch) < quotaDataBatchSize && size < quotaDataBatchBytes {
		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			break
		}
		result, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: quotaDataConsumerGroup, Consumer: consumer, Streams: []string{stream, ">"}, Count: int64(quotaDataBatchSize - len(batch)), Block: remaining}).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, streamResult := range result {
			for _, message := range streamResult.Messages {
				if raw, ok := message.Values["event"].(string); ok {
					size += len(raw)
				}
				batch = append(batch, message)
			}
		}
	}
	return batch, nil
}

func processQuotaDataMessages(ctx context.Context, client *redis.Client, stream string, messages []redis.XMessage) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for len(messages) > 0 {
		end, size := 0, 0
		for end < len(messages) && end < quotaDataBatchSize {
			raw, _ := messages[end].Values["event"].(string)
			if end > 0 && size+len(raw) > quotaDataBatchBytes {
				break
			}
			size += len(raw)
			end++
		}
		batch := messages[:end]
		messages = messages[end:]
		events := make([]QuotaDataEvent, 0, len(batch))
		ids := make([]string, 0, len(batch))
		validMessages := make([]redis.XMessage, 0, len(batch))
		for _, message := range batch {
			raw, ok := message.Values["event"].(string)
			var event QuotaDataEvent
			var err error
			if !ok || len(raw) > quotaDataMaxPayloadBytes {
				err = errInvalidQuotaDataEvent
			} else {
				err = common.UnmarshalJsonStr(raw, &event)
			}
			if err == nil {
				err = event.validate()
			}
			if err != nil {
				if err = quarantineQuotaDataMessage(ctx, stream, message, err); err != nil {
					return err
				}
				if err = acknowledgeQuotaDataMessages(ctx, client, stream, []string{message.ID}); err != nil {
					return err
				}
				continue
			}
			events = append(events, event)
			ids = append(ids, message.ID)
			validMessages = append(validMessages, message)
		}
		workCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := applyQuotaDataEvents(workCtx, events)
		cancel()
		if err == nil {
			if err = acknowledgeQuotaDataMessages(ctx, client, stream, ids); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, errInvalidQuotaDataEvent) {
			return err
		}
		for i, event := range events {
			if err = applyQuotaDataEvents(ctx, []QuotaDataEvent{event}); err != nil {
				if !errors.Is(err, errInvalidQuotaDataEvent) {
					return err
				}
				if err = quarantineQuotaDataMessage(ctx, stream, validMessages[i], err); err != nil {
					return err
				}
			}
			if err = acknowledgeQuotaDataMessages(ctx, client, stream, []string{ids[i]}); err != nil {
				return err
			}
		}
	}
	return nil
}

func quarantineQuotaDataMessage(ctx context.Context, stream string, message redis.XMessage, reason error) error {
	// Preserve the complete message even if its event field is absent or invalid.
	encoded, err := common.Marshal(message.Values)
	if err != nil {
		return err
	}
	row := QuotaDataEventRecord{
		EventID:     fmt.Sprintf("%x", sha256.Sum256([]byte(stream+":"+message.ID))),
		PayloadHash: fmt.Sprintf("%x", sha256.Sum256(encoded)), Payload: string(encoded),
		State: "quarantined", LastError: reason.Error(), CreatedAt: time.Now().Unix(),
		Source: stream, SourceID: message.ID,
	}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	var stored QuotaDataEventRecord
	if err := DB.WithContext(ctx).First(&stored, "event_id = ?", row.EventID).Error; err != nil {
		return err
	}
	if stored.PayloadHash != row.PayloadHash || stored.Payload != row.Payload {
		return fmt.Errorf("%w: quarantine identity conflict", errInvalidQuotaDataEvent)
	}
	common.SysError(common.LogText("Quota data stream event quarantined: id=%s", row.EventID))
	return nil
}

func acknowledgeQuotaDataMessages(ctx context.Context, client *redis.Client, stream string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 1, len(ids)+1)
	args[0] = quotaDataConsumerGroup
	for _, id := range ids {
		args = append(args, id)
	}
	// One consumer group owns this stream. Retrying after an ambiguous reply is
	// safe: the SQL receipt already proves the event was applied (or quarantined).
	return client.Eval(ctx, `redis.call('XACK', KEYS[1], ARGV[1], unpack(ARGV, 2)); return redis.call('XDEL', KEYS[1], unpack(ARGV, 2))`, []string{stream}, args...).Err()
}

func cleanupAppliedQuotaDataMessages(ctx context.Context, client *redis.Client, stream, start string) (string, error) {
	messages, err := client.XRangeN(ctx, stream, start, "+", quotaDataBatchSize).Result()
	if err != nil {
		return start, err
	}
	for _, message := range messages {
		raw, ok := message.Values["event"].(string)
		if !ok {
			continue
		}
		var event QuotaDataEvent
		if common.UnmarshalJsonStr(raw, &event) != nil || event.validate() != nil {
			continue
		}
		var count int64
		if err := DB.WithContext(ctx).Model(&QuotaDataAppliedEvent{}).Where("event_id = ? AND payload_hash = ?", event.EventID, event.PayloadHash).Count(&count).Error; err != nil {
			return start, err
		}
		if count == 1 {
			if err := acknowledgeQuotaDataMessages(ctx, client, stream, []string{message.ID}); err != nil {
				return start, err
			}
		}
	}
	if len(messages) < quotaDataBatchSize {
		return "-", nil
	}
	return "(" + messages[len(messages)-1].ID, nil
}
