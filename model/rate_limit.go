package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RateLimitBucket is the shared authority for security-sensitive limits, even
// during partial Redis partitions. Keys are hashed to avoid retaining raw IPs.
type RateLimitBucket struct {
	KeyHash   string `gorm:"primaryKey;size:64"`
	Count     int64
	ExpiresAt int64 `gorm:"index"`
}

var rateLimitCleanupAfter atomic.Int64

func TakeSecurityRateLimit(ctx context.Context, key string, maximum int, duration int64) (bool, int64, error) {
	if key == "" || maximum <= 0 || duration <= 0 {
		return false, 0, fmt.Errorf("invalid rate limit")
	}
	if DB == nil {
		return false, 0, gorm.ErrInvalidDB
	}
	now := time.Now().Unix()
	if duration > math.MaxInt64-now {
		return false, 0, fmt.Errorf("security rate limit duration overflow")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var allowed bool
	var ttl int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := RateLimitBucket{KeyHash: fmt.Sprintf("%x", sha256.Sum256([]byte(key)))}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&row, "key_hash = ?", row.KeyHash).Error; err != nil {
			return err
		}
		if row.ExpiresAt <= now {
			row.Count, row.ExpiresAt = 0, now+duration
		}
		ttl = row.ExpiresAt - now
		if row.Count >= int64(maximum) {
			return nil
		}
		row.Count++
		allowed = true
		return tx.Model(&row).Select("count", "expires_at").Updates(&row).Error
	})
	cleanupAfter := rateLimitCleanupAfter.Load()
	if err == nil && now >= cleanupAfter && rateLimitCleanupAfter.CompareAndSwap(cleanupAfter, now+300) {
		var expired []string
		if DB.WithContext(ctx).Model(&RateLimitBucket{}).Where("expires_at < ?", now-86400).Limit(1000).Pluck("key_hash", &expired).Error == nil && len(expired) > 0 {
			// Re-check expiry: another instance might have reused the bucket.
			DB.WithContext(ctx).Where("key_hash IN ? AND expires_at < ?", expired, now-86400).Delete(&RateLimitBucket{})
		}
	}
	return allowed, ttl, err
}
