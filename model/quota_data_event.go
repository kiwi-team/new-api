package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const quotaDataBatchSize = 100

var errInvalidQuotaDataEvent = errors.New("invalid quota data event")

// QuotaDataActionID identifies a server-side action within its source namespace.
// The caller must reuse the original payload, including time, when replaying it.
func QuotaDataActionID(source, action string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s%s", len(source), source, action))))
}

// QuotaDataEvent is immutable after acceptance. EventID identifies a billing
// action, independently of Redis message IDs and consumer batch boundaries.
type QuotaDataEvent struct {
	Version     int       `json:"version"`
	EventID     string    `json:"event_id"`
	OccurredAt  int64     `json:"occurred_at"`
	Adjustment  bool      `json:"adjustment"`
	BucketKey   string    `json:"bucket_key"`
	Delta       QuotaData `json:"delta"`
	PayloadHash string    `json:"payload_hash"`
}

type QuotaDataAppliedEvent struct {
	EventID     string `gorm:"primaryKey;size:64"`
	PayloadHash string `gorm:"size:64;not null"`
	ApplyToken  string `gorm:"size:36;not null"`
	AppliedAt   int64
}

// QuotaDataEventRecord also retains quarantined messages for operator repair.
// Neither receipts nor accepted payloads expire automatically.
type QuotaDataEventRecord struct {
	EventID     string `gorm:"primaryKey;size:64"`
	PayloadHash string `gorm:"size:64;not null"`
	Payload     string `gorm:"type:text;not null"`
	State       string `gorm:"size:16;not null;index:idx_quota_event_poll,priority:1"`
	NextAttempt int64  `gorm:"index:idx_quota_event_poll,priority:2"`
	Attempts    int
	LastError   string `gorm:"type:text"`
	Source      string `gorm:"size:128"`
	SourceID    string `gorm:"size:64"`
	CreatedAt   int64
}

func (QuotaDataEventRecord) TableName() string { return "quota_data_events" }

func (q QuotaData) identity() quotaDataKey {
	return quotaDataKey{
		UserID: q.UserID, Username: q.Username, ModelName: q.ModelName, CreatedAt: q.CreatedAt,
		TokenID: q.TokenId, ChannelID: q.ChannelId, UseGroup: q.UseGroup, NodeName: q.NodeName,
		ClientUserID: q.ClientUserId, ClientScenairo: q.ClientScenairo, ProjectName: q.ProjectName, PlanID: q.PlanId,
	}
}

func (q QuotaData) bucketKey() (string, error) {
	// v1 uses exact UTF-8 values, including case and whitespace. Legacy SQL NULL
	// strings normalize to empty on migration. This is independent of DB collation.
	encoded, err := common.Marshal(q.identity())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte("quota-data:v1:"), encoded...))), nil
}

func (q *QuotaData) BeforeCreate(tx *gorm.DB) error {
	key, err := q.bucketKey()
	if err != nil {
		return err
	}
	if q.BucketKey != "" && q.BucketKey != key {
		return fmt.Errorf("%w: bucket identity mismatch", errInvalidQuotaDataEvent)
	}
	q.BucketKey = key
	return nil
}

func newQuotaDataEvent(params *LogQuotaDataCache) (QuotaDataEvent, error) {
	if params == nil || params.UseTimeSeconds < 0 || params.UseTimeSeconds > math.MaxInt/1000 {
		return QuotaDataEvent{}, fmt.Errorf("%w: invalid request duration", errInvalidQuotaDataEvent)
	}
	event := QuotaDataEvent{
		Version: 1, EventID: params.EventID, OccurredAt: params.CreatedAt,
		Adjustment: params.IsSettlementAdjustment,
		Delta:      *quotaDataDelta(params, params.CreatedAt-params.CreatedAt%3600),
	}
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	var err error
	event.BucketKey, err = event.Delta.bucketKey()
	if err != nil {
		return event, err
	}
	encoded, err := common.Marshal(event)
	if err != nil {
		return event, err
	}
	event.PayloadHash = fmt.Sprintf("%x", sha256.Sum256(encoded))
	return event, event.validate()
}

func (event QuotaDataEvent) validate() error {
	invalid := func(reason string) error { return fmt.Errorf("%w: %s", errInvalidQuotaDataEvent, reason) }
	if event.Version != 1 || event.EventID == "" || len(event.EventID) > 64 || event.OccurredAt < 0 {
		return invalid("invalid version, identity or timestamp")
	}
	if strings.ContainsAny(event.EventID, "\x00\r\n") || !utf8.ValidString(event.EventID) {
		return invalid("invalid event identity")
	}
	// Canonical IDs also have identical equality under all supported SQL
	// collations; an upper/lower-case pair cannot alias only on MySQL.
	if len(event.EventID) == 36 {
		id, err := uuid.Parse(event.EventID)
		if err != nil || id.String() != event.EventID {
			return invalid("noncanonical event identity")
		}
	} else if len(event.EventID) == 64 {
		for _, c := range event.EventID {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return invalid("noncanonical event identity")
			}
		}
	} else {
		return invalid("noncanonical event identity")
	}
	q := event.Delta
	if q.Id != 0 || q.UserID <= 0 || q.TokenId < 0 || q.ChannelId < 0 || q.PlanId < 0 || q.CreatedAt != event.OccurredAt-event.OccurredAt%3600 {
		return invalid("invalid aggregation dimensions")
	}
	for _, field := range []struct {
		value string
		limit int
	}{
		{q.Username, 64}, {q.ModelName, 128}, {q.TokenName, 64}, {q.UseGroup, 64}, {q.NodeName, 64},
		{q.ClientUserId, 200}, {q.ClientScenairo, 200}, {q.ProjectName, 200},
	} {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.limit || strings.ContainsRune(field.value, 0) {
			return invalid("invalid dimension text")
		}
	}
	key, err := q.bucketKey()
	if err != nil {
		return err
	}
	if key != event.BucketKey {
		return invalid("bucket checksum mismatch")
	}
	for _, n := range []int{q.Count, q.TokenUsed, q.PromptTokens, q.CompletionTokens, q.CachedTokens,
		q.ClaudeCacheCreation5mTokens, q.ClaudeCacheCreation1hTokens, q.CacheWrite5mRequestCount,
		q.CacheWrite1hRequestCount, q.CacheReadRequestCount, q.StreamRequestCount, q.FrtSum, q.RequestTimeSum} {
		if n < 0 {
			return invalid("negative usage")
		}
		if event.Adjustment && n != 0 {
			return invalid("adjustment contains request usage")
		}
	}
	if !event.Adjustment && (q.Count != 1 || q.Quota < 0) {
		return invalid("invalid consumption")
	}
	if q.StreamRequestCount > 1 || (q.StreamRequestCount == 0) != (q.FrtSum == 0) {
		return invalid("invalid first-token sample")
	}
	if q.CacheWrite5mRequestCount > 1 || q.CacheWrite1hRequestCount > 1 || q.CacheReadRequestCount > 1 {
		return invalid("invalid cache request counts")
	}
	checksum := event.PayloadHash
	event.PayloadHash = ""
	encoded, err := common.Marshal(event)
	if err != nil {
		return err
	}
	if checksum != fmt.Sprintf("%x", sha256.Sum256(encoded)) {
		return invalid("payload checksum mismatch")
	}
	return nil
}

func (q *QuotaData) accumulateChecked(delta QuotaData) error {
	for _, pair := range []struct {
		target *int
		delta  int
	}{
		{&q.Count, delta.Count}, {&q.Quota, delta.Quota}, {&q.TokenUsed, delta.TokenUsed},
		{&q.PromptTokens, delta.PromptTokens}, {&q.CompletionTokens, delta.CompletionTokens}, {&q.CachedTokens, delta.CachedTokens},
		{&q.ClaudeCacheCreation5mTokens, delta.ClaudeCacheCreation5mTokens}, {&q.ClaudeCacheCreation1hTokens, delta.ClaudeCacheCreation1hTokens},
		{&q.CacheWrite5mRequestCount, delta.CacheWrite5mRequestCount}, {&q.CacheWrite1hRequestCount, delta.CacheWrite1hRequestCount},
		{&q.CacheReadRequestCount, delta.CacheReadRequestCount}, {&q.StreamRequestCount, delta.StreamRequestCount},
		{&q.FrtSum, delta.FrtSum}, {&q.RequestTimeSum, delta.RequestTimeSum},
	} {
		if (pair.delta > 0 && *pair.target > math.MaxInt-pair.delta) || (pair.delta < 0 && *pair.target < math.MinInt-pair.delta) {
			return fmt.Errorf("%w: aggregate overflow", errInvalidQuotaDataEvent)
		}
		*pair.target += pair.delta
	}
	return nil
}

var quotaDataCounterColumns = []string{
	"count", "quota", "token_used", "prompt_tokens", "completion_tokens", "cached_tokens",
	"claude_cache_creation5m_tokens", "claude_cache_creation1h_tokens", "cache_write_5m_request_count",
	"cache_write_1h_request_count", "cache_read_request_count", "stream_request_count", "frt_sum", "request_time_sum",
}

func persistQuotaDataEvent(ctx context.Context, event QuotaDataEvent, payload string) error {
	row := QuotaDataEventRecord{EventID: event.EventID, PayloadHash: event.PayloadHash, Payload: payload, State: "pending", Source: "database", SourceID: event.EventID, CreatedAt: time.Now().Unix()}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var existing QuotaDataEventRecord
		if err := lockForUpdate(tx).First(&existing, "event_id = ?", event.EventID).Error; err != nil {
			return err
		}
		if existing.PayloadHash != event.PayloadHash || existing.Payload != payload {
			return fmt.Errorf("%w: event identity conflict", errInvalidQuotaDataEvent)
		}
		return nil
	})
}

// applyQuotaDataEvents is shared by Redis and database consumers. Receipts,
// aggregates, UID totals and matching inbox completion commit together.
func applyQuotaDataEvents(ctx context.Context, events []QuotaDataEvent) error {
	if len(events) == 0 {
		return nil
	}
	unique := make(map[string]QuotaDataEvent, len(events))
	for _, event := range events {
		if err := event.validate(); err != nil {
			return err
		}
		if prior, ok := unique[event.EventID]; ok && prior.PayloadHash != event.PayloadHash {
			return fmt.Errorf("%w: conflicting events in batch", errInvalidQuotaDataEvent)
		}
		unique[event.EventID] = event
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	nonProjectUsers := make(map[string]struct{})
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		applyToken := uuid.NewString()
		receipts := make([]QuotaDataAppliedEvent, 0, len(ids))
		for _, id := range ids {
			receipts = append(receipts, QuotaDataAppliedEvent{EventID: id, PayloadHash: unique[id].PayloadHash, ApplyToken: applyToken, AppliedAt: time.Now().Unix()})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&receipts, quotaDataBatchSize).Error; err != nil {
			return err
		}
		receipts = nil
		if err := lockForUpdate(tx).Where("event_id IN ?", ids).Order("event_id").Find(&receipts).Error; err != nil {
			return err
		}
		if len(receipts) != len(ids) {
			return errors.New("quota data receipt disappeared")
		}
		buckets := make(map[string]QuotaData)
		for _, receipt := range receipts {
			event := unique[receipt.EventID]
			if receipt.PayloadHash != event.PayloadHash {
				return fmt.Errorf("%w: committed event identity conflict", errInvalidQuotaDataEvent)
			}
			if receipt.ApplyToken != applyToken {
				continue
			}
			q, exists := buckets[event.BucketKey]
			if !exists {
				q = event.Delta
				q.BucketKey = event.BucketKey
			} else {
				if q.identity() != event.Delta.identity() {
					return fmt.Errorf("%w: bucket collision", errInvalidQuotaDataEvent)
				}
				if err := q.accumulateChecked(event.Delta); err != nil {
					return err
				}
			}
			buckets[event.BucketKey] = q
		}
		keys := make([]string, 0, len(buckets))
		for key := range buckets {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		uidDeltas := make(map[string]QuotaData)
		for _, key := range keys {
			delta := buckets[key]
			// Insert an empty bucket first. Its unique key serializes competing
			// writers even when the bucket did not exist before this transaction.
			empty := delta
			empty.Count, empty.Quota, empty.TokenUsed, empty.PromptTokens, empty.CompletionTokens, empty.CachedTokens = 0, 0, 0, 0, 0, 0
			empty.ClaudeCacheCreation5mTokens, empty.ClaudeCacheCreation1hTokens = 0, 0
			empty.CacheWrite5mRequestCount, empty.CacheWrite1hRequestCount, empty.CacheReadRequestCount = 0, 0, 0
			empty.StreamRequestCount, empty.FrtSum, empty.RequestTimeSum = 0, 0, 0
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&empty).Error; err != nil {
				return err
			}
			var row QuotaData
			if err := lockForUpdate(tx).Where("bucket_key = ?", key).First(&row).Error; err != nil {
				return err
			}
			if row.identity() != delta.identity() {
				return fmt.Errorf("%w: stored bucket collision", errInvalidQuotaDataEvent)
			}
			if err := row.accumulateChecked(delta); err != nil {
				return err
			}
			if err := tx.Model(&row).Select(quotaDataCounterColumns).Updates(&row).Error; err != nil {
				return err
			}
			if delta.ClientUserId != "" {
				total := uidDeltas[delta.ClientUserId]
				if err := total.accumulateChecked(QuotaData{Quota: delta.Quota}); err != nil {
					return err
				}
				uidDeltas[delta.ClientUserId] = total
				if delta.ProjectName == "" {
					nonProjectUsers[delta.ClientUserId] = struct{}{}
				}
			}
		}
		uids := make([]string, 0, len(uidDeltas))
		for uid := range uidDeltas {
			uids = append(uids, uid)
		}
		slices.Sort(uids)
		for _, uid := range uids {
			if err := increaseCliendUserUsedQuota(tx, uid, uidDeltas[uid].Quota); err != nil {
				return err
			}
		}
		// Do not mark an inbox entry with the same ID but different content done.
		condition := tx.Where("1 = 0")
		for _, id := range ids {
			condition = condition.Or("event_id = ? AND payload_hash = ?", id, unique[id].PayloadHash)
		}
		return tx.Model(&QuotaDataEventRecord{}).Where("state = ?", "pending").Where(condition).Updates(map[string]any{"state": "applied", "last_error": ""}).Error
	})
	if err == nil {
		invalidateMonthlyNonProjectQuotaCache(nonProjectUsers)
	}
	return err
}
