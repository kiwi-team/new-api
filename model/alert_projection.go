package model

import (
	"context"
	"crypto/sha256"
	"slices"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Revisions change in the receipt transaction. Redis snapshots can therefore
// detect late facts and replay without treating a cache entry as authority.
// Shards keep concurrent requests from contending on one global minute row.
type AlertMetricBucket struct {
	ID        string `gorm:"primaryKey;size:64"`
	Namespace string `gorm:"size:24;index:idx_alert_metric_minute,priority:1"`
	Minute    int64  `gorm:"index:idx_alert_metric_minute,priority:2"`
	Shard     int
	Version   int64
}

func alertMetricBucket(observation AlertObservation) (AlertMetricBucket, bool) {
	if observation.Namespace != "usage" && (observation.Namespace != "health" || observation.Kind != "finished" || observation.SampleClass != "" && observation.SampleClass != "production") {
		return AlertMetricBucket{}, false
	}
	hash := sha256.Sum256([]byte(observation.ID))
	bucket := AlertMetricBucket{Namespace: observation.Namespace, Minute: observation.OccurredAt / 60000, Shard: int(hash[0]) % 16}
	bucket.ID = AlertIdentity("metric-minute", bucket.Namespace, bucket.Minute, bucket.Shard)
	return bucket, true
}

func prepareAlertMetricBuckets(tx *gorm.DB, observations []AlertObservation) (map[string]bool, error) {
	rows := map[string]AlertMetricBucket{}
	for _, observation := range observations {
		if row, ok := alertMetricBucket(observation); ok {
			rows[row.ID] = row
		}
	}
	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	prepared := make(map[string]bool, len(ids))
	for _, id := range ids {
		row := rows[id]
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return nil, err
		}
		if err := lockForUpdate(tx).Select("id").First(&row, "id = ?", id).Error; err != nil {
			return nil, err
		}
		prepared[id] = true
	}
	return prepared, nil
}

// AlertMetricRevision reads small minute/shard rows, independent of the number
// of response records in a window. Amounts and tokens never enter Lua arithmetic.
func AlertMetricRevision(ctx context.Context, db *gorm.DB, namespace string, start, end int64) (int64, error) {
	var version int64
	err := db.WithContext(ctx).Model(&AlertMetricBucket{}).Where("namespace = ? AND minute >= ? AND minute <= ?", namespace, start/60000, end/60000).Select("COALESCE(SUM(version),0)").Scan(&version).Error
	return version, err
}

func publishAlertMetricBucket(ctx context.Context, client *redis.Client, fact AlertObservation) error {
	bucket, ok := alertMetricBucket(fact)
	if !ok {
		return nil
	}
	result := DB.WithContext(ctx).Where("id = ?", bucket.ID).Limit(1).Find(&bucket)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	// Compare decimal strings rather than converting an int64 revision to a Lua
	// double. A delayed publisher cannot replace a newer absolute snapshot.
	_, err := client.Eval(ctx, `local old=redis.call('HGET',KEYS[1],'version'); local v=ARGV[1]; if not old or #v>#old or (#v==#old and v>=old) then redis.call('HSET',KEYS[1],'version',v,'accepted_facts',v); redis.call('EXPIRE',KEYS[1],93600) end; return 1`, []string{AlertRedisKey("metric:" + bucket.Namespace + ":" + strconv.FormatInt(bucket.Minute, 10) + ":" + strconv.Itoa(bucket.Shard))}, strconv.FormatInt(bucket.Version, 10)).Result()
	return err
}

func cleanAlertMetricBuckets(ctx context.Context, now time.Time) error {
	var ids []string
	if err := DB.WithContext(ctx).Model(&AlertMetricBucket{}).Where("minute < ?", now.Add(-26*time.Hour).UnixMilli()/60000).Limit(1000).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Where("id IN ?", ids).Delete(&AlertMetricBucket{}).Error
}
