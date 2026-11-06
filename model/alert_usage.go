package model

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// AlertUsageObservation copies settled facts. It never derives usage or writes
// wallet, token quota, UID budget, or quota_data counters.
func AlertUsageObservation(event QuotaDataEvent) AlertObservation {
	d := event.Delta
	return AlertObservation{
		ID: AlertIdentity("usage", event.EventID), Namespace: "usage", SourceID: event.EventID,
		Kind: "settlement", OccurredAt: event.OccurredAt * 1000,
		UserID: d.UserID, TokenID: d.TokenId, ChannelID: d.ChannelId, ModelKey: d.ModelName,
		Quota: int64(d.Quota), RequestCount: int64(d.Count), InputTokens: int64(d.PromptTokens), OutputTokens: int64(d.CompletionTokens),
		CachedTokens: int64(d.CachedTokens), CacheWrite5mTokens: int64(d.ClaudeCacheCreation5mTokens),
		CacheWrite1hTokens: int64(d.ClaudeCacheCreation1hTokens), NodeID: d.NodeName,
		ClientUserID: d.ClientUserId, ClientScenario: d.ClientScenairo, ProjectName: d.ProjectName, PlanID: d.PlanId,
	}
}

func AcceptAlertUsage(event QuotaDataEvent) error {
	if !common.AlertingEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return EnqueueAlertObservation(ctx, AlertUsageObservation(event))
}

// AlertUsageSourceReady checks durable input boundaries, never wallet or billing
// values. A pending refund is as important as a pending charge for safe actions.
func AlertUsageSourceReady(ctx context.Context) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&QuotaDataEventRecord{}).Where("state IN ?", []string{"pending", "quarantined"}).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if common.RedisEnabled && common.RDB != nil {
		work, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer cancel()
		for _, stream := range []string{quotaDataStream, AlertUsageStream} {
			length, err := common.RDB.XLen(work, stream).Result()
			if err != nil || length > 0 {
				return false, err
			}
		}
	}
	return true, nil
}

// Local spools cannot be inspected by other nodes. Their health travels in the
// producer checkpoint and remains false for quarantined data requiring repair.
func AlertLocalUsageReady() bool {
	for _, dir := range []string{quotaDataSpoolDir(), alertSpoolDir()} {
		file, err := os.Open(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false
		}
		entries, err := file.ReadDir(256)
		_ = file.Close()
		if err != nil && !errors.Is(err, io.EOF) || len(entries) == 256 {
			return false
		}
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() == "quarantine" {
				return false
			}
			if entry.Type().IsRegular() && (strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".quarantined")) {
				return false
			}
		}
	}
	return true
}
