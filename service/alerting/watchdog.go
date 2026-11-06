package alerting

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// Watchdog runs independently of evaluation. Its events never feed back into
// source facts, and a stable five-minute identity bounds persistent failures.
// SQL/worker-wide outages still require an external availability monitor.
func (e *Engine) Watchdog(ctx context.Context, targetIDs []int64, now time.Time) error {
	var evaluator model.AlertWorkerCheckpoint
	result := model.DB.WithContext(ctx).Where("kind = ?", "evaluator").Order("updated_at desc").Limit(1).Find(&evaluator)
	if result.Error != nil {
		return result.Error
	}
	reason := ""
	if result.RowsAffected == 0 || evaluator.UpdatedAt < now.Add(-time.Minute).UnixMilli() || evaluator.UpdatedAt > now.Add(5*time.Second).UnixMilli() {
		reason = "evaluation_lag"
	}
	var backlog int64
	if err := model.DB.WithContext(ctx).Model(&model.AlertJob{}).Where("kind = ? AND status IN ? AND available_at < ?", "notify", []string{"pending", "retry_wait"}, now.Add(-time.Minute).UnixMilli()).Count(&backlog).Error; err != nil {
		return err
	}
	if backlog > 0 {
		reason = "delivery_backlog"
	}
	_, status, _, err := SourceWatermark(ctx, now)
	if err != nil {
		return err
	}
	if status != "complete" {
		reason = "source_lag"
	}
	if reason == "" {
		return nil
	}
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		key := model.AlertIdentity("watchdog", reason, now.Unix()/300)
		event := model.AlertEvent{DedupKey: key, Kind: "engine_health", Severity: "P1", TargetName: "Alert engine", Summary: reason, CreatedAt: now.UnixMilli()}
		created, err := model.InsertAlertEventOnce(tx, &event)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		if len(targetIDs) == 0 {
			return nil
		}
		var targets []model.AlertTarget
		if err := tx.Where("id IN ? AND owner_user_id = ? AND enabled = ?", targetIDs, 0, true).Limit(10).Find(&targets).Error; err != nil {
			return err
		}
		for _, target := range targets {
			job := model.AlertJob{DedupKey: model.AlertIdentity("watchdog-delivery", key, target.ID), Platform: true, EventID: event.ID, Kind: "notify", TargetName: target.Name, TargetType: target.Type, TargetHint: target.AddressHint, TargetID: target.ID, TargetRevision: target.Revision, Priority: 1, Status: "pending", AvailableAt: now.UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli(), CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
			if err := createJob(tx, &job); err != nil {
				return err
			}
		}
		return tx.Create(&model.AlertAudit{Operation: "engine.watchdog", ObjectID: event.ID, Revision: now.Unix() / 300, CreatedAt: now.UnixMilli()}).Error
	})
}
