package model

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAlertConflict = errors.New("alert revision conflict")
var ErrAlertObservationConflict = errors.New("alert observation identity conflict")
var ErrInvalidAlertObservation = errors.New("invalid alert observation")

func SaveAlertRule(ctx context.Context, row *AlertRule, expectedRevision int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if row.ID == 0 {
			if err := LockAlertOwner(tx, row.OwnerUserID); err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&AlertRule{}).Where("owner_user_id = ?", row.OwnerUserID).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(AlertRuleOwnerLimit()) {
				return ErrAlertConflict
			}
			row.Revision, row.Generation = 1, 1
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		} else {
			var old AlertRule
			if err := lockForUpdate(tx).First(&old, row.ID).Error; err != nil {
				return err
			}
			if old.Revision != expectedRevision {
				return ErrAlertConflict
			}
			row.Revision = old.Revision + 1
			row.CreatedBy, row.CreatedAt = old.CreatedBy, old.CreatedAt
			var scheduled AlertMigration
			if err := tx.Where("rule_id = ? AND kind IN ? AND executor = ?", row.ID, []string{"key_hour", "key_day"}, "unified").Limit(1).Find(&scheduled).Error; err != nil {
				return err
			}
			// Resume or an edit cannot move a scheduled takeover into the old
			// executor's ambiguous current period.
			row.EffectiveAt = max(row.EffectiveAt, scheduled.CutoverAt)
			result := tx.Model(&AlertRule{}).Where("id = ? AND revision = ?", row.ID, expectedRevision).
				Select("owner_user_id", "updated_by", "name", "enabled", "revision", "generation", "effective_at", "next_evaluation_at", "evaluation_cursor", "config", "updated_at").Updates(row)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAlertConflict
			}
			if row.Generation != old.Generation {
				// Migrated cumulative rules initially bridge the legacy absolute
				// checkpoint. An explicit condition edit or resume establishes a
				// new accounting-time baseline, just like an ordinary new rule.
				if err := tx.Model(&AlertMigration{}).Where("rule_id = ? AND kind IN ?", row.ID, []string{"token_spend", "user_spend"}).Updates(map[string]any{"metric_start_at": row.EffectiveAt, "metric_offset": 0}).Error; err != nil {
					return err
				}
			}
			// Pausing keeps approved notices. Removing their recipients withdraws
			// that approval, including ready work held by a different worker.
			spec, err := AlertRuleConfig(*row)
			if err != nil {
				return err
			}
			allowed := append([]int64(nil), spec.Notify.TargetIDs...)
			allowed = append(allowed, spec.Acknowledgement.EscalationTargetIDs...)
			jobs := tx.Model(&AlertJob{}).Where("rule_id = ? AND kind = ? AND status IN ?", row.ID, "notify", []string{"pending", "retry_wait", "running"})
			if len(allowed) > 0 {
				jobs = jobs.Where("target_id NOT IN ?", allowed)
			}
			if err := jobs.Updates(map[string]any{"status": "canceled", "last_error": "authorization_revoked", "locked_by": "", "locked_until": 0, "updated_at": row.UpdatedAt}).Error; err != nil {
				return err
			}
			if err := reauthorizeAlertActions(tx, old, *row, spec); err != nil {
				return err
			}
		}
		if err := tx.Create(&AlertRuleRevision{RuleID: row.ID, Revision: row.Revision, Config: row.Config, CreatedBy: row.UpdatedBy, CreatedAt: row.UpdatedAt}).Error; err != nil {
			return err
		}
		return tx.Create(&AlertAudit{ActorID: row.UpdatedBy, OwnerUserID: row.OwnerUserID, Operation: "rule.save", ObjectID: row.ID, Revision: row.Revision, CreatedAt: row.UpdatedAt}).Error
	})
}

// A presentation edit is a newly approved revision of the same action policy.
// Requeue its live work under that revision, fencing any previously claimed
// worker. Event identity, expiry and attempt history remain unchanged.
func reauthorizeAlertActions(tx *gorm.DB, old, current AlertRule, spec AlertRuleSpec) error {
	if !old.Enabled || !current.Enabled || old.OwnerUserID != current.OwnerUserID || old.Generation != current.Generation || len(spec.Actions) == 0 {
		return nil
	}
	previous, err := AlertRuleConfig(old)
	if err != nil {
		return err
	}
	previous.Name, previous.Notify, previous.Acknowledgement = spec.Name, spec.Notify, spec.Acknowledgement
	previous.Severity, previous.Category = spec.Severity, spec.Category
	if AlertIdentity(previous) != AlertIdentity(spec) {
		return nil
	}
	var ids []int64
	if err := lockForUpdate(tx).Model(&AlertJob{}).
		Where("rule_id = ? AND rule_revision = ? AND kind = ? AND expires_at > ? AND status IN ?", current.ID, old.Revision, "action", current.UpdatedAt, []string{"pending", "retry_wait", "running"}).
		Pluck("id", &ids).Error; err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 200 {
		batch := ids[start:min(start+200, len(ids))]
		result := tx.Model(&AlertJob{}).Where("id IN ? AND rule_revision = ?", batch, old.Revision).Updates(map[string]any{
			"rule_revision": current.Revision,
			"status":        gorm.Expr("CASE WHEN status = ? THEN ? ELSE status END", "running", "pending"),
			"locked_by":     "", "locked_until": 0, "updated_at": current.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(batch)) {
			return ErrAlertConflict
		}
		outbox := make([]AlertOutbox, 0, len(batch))
		audits := make([]AlertAudit, 0, len(batch))
		for _, id := range batch {
			outbox = append(outbox, AlertOutbox{ID: AlertIdentity("action-reauthorize", id, current.Revision), Kind: "action", Reference: strconv.FormatInt(id, 10), CreatedAt: current.UpdatedAt})
			audits = append(audits, AlertAudit{ActorID: current.UpdatedBy, OwnerUserID: current.OwnerUserID, Operation: "action.reauthorize", ObjectID: id, Revision: current.Revision, CreatedAt: current.UpdatedAt})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&outbox).Error; err != nil {
			return err
		}
		if err := tx.Create(&audits).Error; err != nil {
			return err
		}
	}
	return nil
}

func AlertRuleAllowsNotification(spec AlertRuleSpec, targetID int64) bool {
	return slices.Contains(spec.Notify.TargetIDs, targetID) || slices.Contains(spec.Acknowledgement.EscalationTargetIDs, targetID)
}

// LockAlertOwner serializes bounded configuration creation within one account.
// Platform objects use the persisted deployment singleton instead of a fake user.
func LockAlertOwner(tx *gorm.DB, owner int) error {
	if owner == 0 {
		return lockForUpdate(tx).Select("id").First(&AlertDeployment{}, 1).Error
	}
	return lockForUpdate(tx).Select("id").First(&User{}, owner).Error
}

func AcceptAlertObservation(ctx context.Context, observation AlertObservation) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return AcceptAlertObservationTx(tx, observation) })
}

// AcceptAlertObservationTx is also used by the billing event consumer, making
// its alert bridge atomic with the existing quota-data receipt and counters.
func AcceptAlertObservationTx(tx *gorm.DB, observation AlertObservation) error {
	return AcceptAlertObservationsTx(tx, []AlertObservation{observation})
}

type alertLifecycleUpdate struct {
	row              AlertLifecycle
	wasEmpty         bool
	receivedTerminal bool
	changed          bool
}

func (o *AlertObservation) tracksLifecycle() bool {
	return o.Namespace == "health" && o.SampleClass != "recovery_probe" && o.SampleClass != "transport"
}

// AcceptAlertObservationsTx updates each producer once, after its immutable
// receipts, metric revisions and lifecycle changes have succeeded.
func AcceptAlertObservationsTx(tx *gorm.DB, observations []AlertObservation) error {
	// Keep the caller's slice immutable. Receipts are inserted by ID while
	// lifecycle changes for each request retain the caller's event order.
	batch := slices.Clone(observations)
	order := make([]int, len(batch))
	lifecycles := make(map[string]*alertLifecycleUpdate)
	for i := range batch {
		o := &batch[i]
		if len(o.ID) != 64 || o.OccurredAt < 0 || o.SourceID == "" || o.Namespace != "health" && o.Namespace != "usage" {
			return ErrInvalidAlertObservation
		}
		if o.Namespace == "health" {
			if o.Sequence <= 0 || o.StartedAt <= 0 || o.Layer != "request" && o.Layer != "channel_attempt" || o.DurationMS != nil && *o.DurationMS < 0 || o.TTFTMS != nil && *o.TTFTMS < 0 {
				return ErrInvalidAlertObservation
			}
			if o.SampleClass == "" {
				o.SampleClass = "production"
			}
		}
		if o.tracksLifecycle() {
			lifecycles[o.SourceID] = nil
		}
		order[i] = i
	}
	if err := prepareAlertMetricBuckets(tx, batch); err != nil {
		return err
	}
	// Serialize only overlapping requests before acquiring their receipt locks.
	lifecycleIDs := slices.Sorted(maps.Keys(lifecycles))
	for _, id := range lifecycleIDs {
		state := &alertLifecycleUpdate{row: AlertLifecycle{ID: id}}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state.row).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Take(&state.row).Error; err != nil {
			return err
		}
		state.wasEmpty = state.row.Sequence == 0
		lifecycles[id] = state
	}

	slices.SortStableFunc(order, func(i, j int) int { return strings.Compare(batch[i].ID, batch[j].ID) })
	accepted := make([]bool, len(batch))
	for _, i := range order {
		var err error
		accepted[i], err = insertAlertObservationTx(tx, &batch[i])
		if err != nil {
			return err
		}
	}
	sources := make(map[string]AlertWorkerCheckpoint)
	for i, ok := range accepted {
		if !ok {
			continue
		}
		o := &batch[i]
		if o.BootID != "" && o.SourceSequence > 0 {
			source := sources[o.BootID]
			source.ID, source.BootID = o.BootID, o.BootID
			source.NodeID, source.Kind = o.NodeID, "producer"
			source.AcceptedCount++
			sources[o.BootID] = source
		}
		if state := lifecycles[o.SourceID]; state != nil && o.Namespace == "health" {
			if o.Kind == "finished" {
				state.receivedTerminal = true
			}
			if o.tracksLifecycle() {
				state.apply(o)
			}
		}
	}
	for _, id := range lifecycleIDs {
		if err := lifecycles[id].save(tx); err != nil {
			return err
		}
	}
	for _, id := range slices.Sorted(maps.Keys(sources)) {
		source := sources[id]
		increment := clause.Assignments(map[string]any{
			"accepted_count": gorm.Expr("COALESCE(?, 0) + ?", clause.Column{Table: clause.CurrentTable, Name: "accepted_count"}, source.AcceptedCount),
		})
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: increment}).Create(&source).Error; err != nil {
			return err
		}
	}
	return nil
}

// insertAlertObservationTx returns true only for the transaction that
// inserted the raw fact and its projection hint.
func insertAlertObservationTx(tx *gorm.DB, observation *AlertObservation) (bool, error) {
	observation.AcceptedAt = 0
	if observation.Namespace == "health" && observation.Kind == "finished" {
		key := AlertIdentity("terminal", observation.SourceID)
		observation.TerminalKey = &key
	} else {
		observation.TerminalKey = nil
	}
	observation.PayloadHash = ""
	observation.SourcePositionKey = nil
	if observation.Namespace == "health" && observation.BootID != "" && observation.SourceSequence > 0 {
		key := AlertIdentity("source_position", observation.BootID, observation.SourceSequence)
		observation.SourcePositionKey = &key
	}
	observation.ModelHash = AlertIdentity("model", observation.ModelKey)
	encoded, err := common.Marshal(observation)
	if err != nil {
		return false, err
	}
	observation.PayloadHash = AlertIdentity("observation", string(encoded))
	// All producers and cleanup claim the same unique identities. A point
	// lookup by ID distinguishes an exact replay from a terminal/sequence
	// collision even when MySQL reports one affected row for a no-op insert.
	identity := observation.receipt()
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&identity).Error; err != nil {
		return false, err
	}
	var persisted AlertObservationReceipt
	found := lockForUpdate(tx).Select("payload_hash", "archived").Where("id = ?", observation.ID).Limit(1).Find(&persisted)
	if found.Error != nil {
		return false, found.Error
	}
	if found.RowsAffected == 0 || persisted.PayloadHash != observation.PayloadHash {
		return false, ErrAlertObservationConflict
	}
	if persisted.Archived {
		return false, nil
	}

	observation.AcceptedAt = time.Now().UnixMilli()
	observation.ReceiptNonce = common.GetUUID()
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(observation)
	if result.Error != nil {
		return false, result.Error
	}
	// MySQL CLIENT_FOUND_ROWS reports one affected row even for the no-op
	// ON DUPLICATE KEY branch. A persisted nonce proves who inserted the fact.
	var receipt AlertObservation
	found = lockForUpdate(tx).Select("payload_hash", "receipt_nonce").Where("id = ?", observation.ID).Limit(1).Find(&receipt)
	if found.Error != nil {
		return false, found.Error
	}
	// A different ID claiming an existing terminal or producer position was
	// rejected by the unique index, including for pre-retention raw rows.
	if found.RowsAffected == 0 || receipt.PayloadHash != observation.PayloadHash {
		return false, ErrAlertObservationConflict
	}
	if receipt.ReceiptNonce != observation.ReceiptNonce {
		return false, nil
	}
	if bucket, ok := alertMetricBucket(*observation); ok {
		if err := tx.Model(&AlertMetricBucket{}).Where("id = ?", bucket.ID).Update("version", gorm.Expr("version + 1")).Error; err != nil {
			return false, err
		}
	}
	if err := tx.Create(&AlertOutbox{ID: AlertIdentity("projection", observation.ID), Kind: "projection", Reference: observation.ID, CreatedAt: observation.AcceptedAt}).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (lifecycle *alertLifecycleUpdate) apply(observation *AlertObservation) {
	row := &lifecycle.row
	// Terminal tombstones dominate a delayed start/progress, including after a
	// process restart. Each terminal observation is counted separately only once.
	if row.FinishedAt != 0 || row.Sequence >= observation.Sequence {
		return
	}
	if row.Sequence == 0 && lifecycle.receivedTerminal && observation.Kind != "finished" {
		return
	}
	row.Layer, row.RequestID = observation.Layer, observation.RequestID
	row.SampleClass = observation.SampleClass
	row.UserID, row.TokenID = observation.UserID, observation.TokenID
	row.ChannelID, row.ChannelName = observation.ChannelID, observation.ChannelName
	row.ModelKey, row.ModelHash = observation.ModelKey, observation.ModelHash
	row.Protocol, row.Stream = observation.Protocol, observation.Stream
	row.ConfigVersion = observation.ConfigVersion
	row.NodeID, row.BootID = observation.NodeID, observation.BootID
	row.StartedAt, row.OutputCovered = observation.StartedAt, observation.OutputCovered
	if row.FirstOutputAt == 0 {
		row.FirstOutputAt = observation.FirstOutputAt
	}
	row.LastOutputAt = max(row.LastOutputAt, observation.LastOutputAt)
	row.Sequence, row.UpdatedAt = observation.Sequence, observation.AcceptedAt
	if observation.Kind == "finished" {
		row.FinishedAt = observation.OccurredAt
		row.Outcome = observation.Outcome
	}
	lifecycle.changed = true
}

func (state *alertLifecycleUpdate) save(tx *gorm.DB) error {
	if state.row.Sequence == 0 {
		// Replaying an archived receipt must not recreate its lifecycle.
		return tx.Delete(&state.row).Error
	}
	if !state.changed {
		return nil
	}
	if state.wasEmpty && !state.receivedTerminal {
		// All inserts precede this read, which can lock a missing key's gap on MySQL.
		var terminal AlertObservationReceipt
		found := lockForUpdate(tx).Select("id").Where("terminal_key = ?", AlertIdentity("terminal", state.row.ID)).Limit(1).Find(&terminal)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 {
			return tx.Delete(&state.row).Error
		}
	}
	return tx.Save(&state.row).Error
}

// VerifyAlertSourcePrefix proves every offered position has a durable receipt.
// A total receipt count is insufficient: a newer event must not conceal an
// older gap. Unique source positions make this indexed interval count exact.
func VerifyAlertSourcePrefix(ctx context.Context, bootID string) error {
	db := DB.WithContext(ctx)
	var source AlertWorkerCheckpoint
	if err := db.Where("id = ?", bootID).Take(&source).Error; err != nil {
		return err
	}
	updates := map[string]any{}
	if source.OfferedSequence > source.AcceptedThrough {
		positions := "boot_id = ? AND source_sequence > ? AND source_sequence <= ?"
		recent := db.Model(&AlertObservation{}).Where(positions, bootID, source.AcceptedThrough, source.OfferedSequence).Where("namespace = ?", "health").Select("COUNT(*) AS n")
		archived := db.Model(&AlertObservationReceipt{}).Where(positions, bootID, source.AcceptedThrough, source.OfferedSequence).Where("archived = ?", true).Select("COUNT(*) AS n")
		var count int64
		if err := db.Table("(? UNION ALL ?) AS positions", recent, archived).Select("SUM(n)").Scan(&count).Error; err != nil {
			return err
		}
		if count != source.OfferedSequence-source.AcceptedThrough {
			return nil
		}
		updates["accepted_through"] = source.OfferedSequence
	}
	if source.Healthy && source.CapturedAt != source.LastCompleteAt {
		updates["last_complete_at"] = source.CapturedAt
	}
	if len(updates) == 0 {
		return nil
	}
	// A newer heartbeat or verifier owns its own proof. A concurrent receipt
	// only adds a position and cannot invalidate a proven prefix.
	return db.Model(&AlertWorkerCheckpoint{}).
		Where("id = ? AND COALESCE(accepted_through, 0) = ? AND COALESCE(offered_sequence, 0) = ?", bootID, source.AcceptedThrough, source.OfferedSequence).
		Where("COALESCE(captured_at, 0) = ? AND COALESCE(healthy, ?) = ? AND COALESCE(last_complete_at, 0) = ?", source.CapturedAt, false, source.Healthy, source.LastCompleteAt).
		Updates(updates).Error
}

func AlertRuleConfig(row AlertRule) (AlertRuleSpec, error) {
	var spec AlertRuleSpec
	if err := common.UnmarshalJsonStr(row.Config, &spec); err != nil {
		return spec, fmt.Errorf("invalid stored alert rule: %w", err)
	}
	return spec, nil
}

// InsertAlertEventOnce proves insertion independently of RowsAffected, whose
// no-op semantics differ when MySQL CLIENT_FOUND_ROWS is enabled.
func InsertAlertEventOnce(tx *gorm.DB, event *AlertEvent) (bool, error) {
	event.ReceiptNonce = common.GetUUID()
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event).Error; err != nil {
		return false, err
	}
	var persisted AlertEvent
	if err := lockForUpdate(tx).Select("id", "receipt_nonce").Where("dedup_key = ?", event.DedupKey).First(&persisted).Error; err != nil {
		return false, err
	}
	event.ID = persisted.ID
	if persisted.ReceiptNonce != event.ReceiptNonce {
		return false, nil
	}
	var receipt AlertEventReceipt
	found := lockForUpdate(tx).Where("dedup_key = ?", event.DedupKey).Limit(1).Find(&receipt)
	if found.Error != nil {
		return false, found.Error
	}
	if found.RowsAffected == 0 {
		return true, nil
	}
	if err := tx.Delete(event).Error; err != nil {
		return false, err
	}
	event.ID = receipt.EventID
	return false, nil
}

// CheckChannelModelAvailability uses primary-database holds, so a stale Redis
// candidate cannot admit a new inference after isolation has committed.
func CheckChannelModelAvailability(ctx context.Context, channelID int, modelKey string) error {
	if !common.AlertingControlsEnabled {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ctx = bounded
	var channel Channel
	if err := DB.WithContext(ctx).Select("id", "type", "models", "model_mapping", "group", "settings", "status").First(&channel, channelID).Error; err != nil {
		return err
	}
	if channel.Status != common.ChannelStatusEnabled {
		return errors.New("channel is disabled")
	}
	modelKey, _, _ = ResolveChannelModelIdentity(&channel, modelKey)
	keys := []string{AlertIdentity("channel", channelID), AlertIdentity("channel_model", channelID, "routing", modelKey)}
	var count int64
	if err := DB.WithContext(ctx).Model(&AlertDisableHold{}).Where("target_key IN ? AND active = ?", keys, true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("channel model is isolated by an alert rule")
	}
	if err := DB.WithContext(ctx).Model(&AlertChannelModelControl{}).Where("target_key = ? AND manual_paused = ?", keys[1], true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("channel model is manually paused")
	}
	return nil
}

func AlertTokenHeld(ctx context.Context, tokenID int) (bool, error) {
	if !common.AlertingControlsEnabled {
		return false, nil
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ctx = bounded
	var count int64
	err := DB.WithContext(ctx).Model(&AlertDisableHold{}).Where("token_id = ? AND active = ?", tokenID, true).Count(&count).Error
	return count > 0, err
}

func ClaimAlertJob(ctx context.Context, kind, owner string, now int64) (*AlertJob, error) {
	return claimAlertJob(ctx, kind, owner, now, false)
}

// ClaimUrgentAlertJob reserves capacity for platform P0/P1 work. A personal
// rule choosing P0 must not consume the platform's dedicated worker.
func ClaimUrgentAlertJob(ctx context.Context, kind, owner string, now int64) (*AlertJob, error) {
	return claimAlertJob(ctx, kind, owner, now, true)
}

func claimAlertJob(ctx context.Context, kind, owner string, now int64, urgent bool) (*AlertJob, error) {
	var expired []int64
	if err := DB.WithContext(ctx).Model(&AlertJob{}).Where("kind = ? AND expires_at <= ? AND (status IN ? OR (status = ? AND locked_until <= ?))", kind, now, []string{"pending", "retry_wait"}, "running", now).Limit(200).Pluck("id", &expired).Error; err != nil {
		return nil, err
	}
	if len(expired) > 0 {
		if err := DB.WithContext(ctx).Model(&AlertJob{}).Where("id IN ? AND expires_at <= ?", expired, now).Updates(map[string]any{"status": "expired", "last_error": "delivery_expired", "updated_at": now}).Error; err != nil {
			return nil, err
		}
	}

	var claimed *AlertJob
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row AlertJob
		query := lockForUpdate(tx)
		if urgent {
			query = query.Where("platform = ? AND priority <= ?", true, 1)
		}
		err := query.Where("kind = ? AND available_at <= ? AND expires_at > ? AND (status IN ? OR (status = ? AND locked_until <= ?))", kind, now, now, []string{"pending", "retry_wait"}, "running", now).
			Order(clause.OrderByColumn{Column: clause.Column{Raw: true, Name: fmt.Sprintf("CASE WHEN created_at <= %d THEN 0 ELSE priority END", now-300000)}}).Order("available_at asc, id asc").First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result := tx.Model(&AlertJob{}).Where("id = ? AND status = ? AND locked_until = ?", row.ID, row.Status, row.LockedUntil).
			Updates(map[string]any{"status": "running", "locked_by": owner, "locked_until": now + 60000, "attempts": row.Attempts + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		row.Status, row.LockedBy, row.LockedUntil, row.Attempts = "running", owner, now+60000, row.Attempts+1
		claimed = &row
		return nil
	})
	return claimed, err
}

// WithAlertState commits condition progress, incident, event, and jobs together.
// The callback must not perform network I/O or mutate billing state.
func WithAlertState(ctx context.Context, rule AlertRule, targetType string, targetID int, modelKey string, apply func(*gorm.DB, *AlertRuleState) error) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current AlertRule
		if err := lockForUpdate(tx).First(&current, rule.ID).Error; err != nil {
			return err
		}
		if !current.Enabled || current.Revision != rule.Revision || current.Generation != rule.Generation {
			return ErrAlertConflict
		}
		state := AlertRuleState{ID: AlertIdentity("state", rule.ID, rule.Generation, targetType, targetID, modelKey), RuleID: rule.ID, Generation: rule.Generation, TargetType: targetType, TargetID: targetID, ModelKey: modelKey, Status: "normal"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&state, "id = ?", state.ID).Error; err != nil {
			return err
		}
		if err := apply(tx, &state); err != nil {
			return err
		}
		return tx.Save(&state).Error
	})
}

// FilterAlertAvailableChannels checks a candidate batch with three bounded SQL
// reads rather than issuing three queries for every cached candidate. The final
// dispatch still calls CheckChannelModelAvailability against current authority.
func FilterAlertAvailableChannels(ctx context.Context, ids []int, requested string) ([]int, error) {
	if !common.AlertingControlsEnabled || len(ids) == 0 {
		return ids, nil
	}
	available := make([]int, 0, len(ids))
	for start := 0; start < len(ids); start += 200 {
		batch := ids[start:min(start+200, len(ids))]
		var channels []Channel
		if err := DB.WithContext(ctx).Select("id", "status", "models", "model_mapping").Where("id IN ?", batch).Find(&channels).Error; err != nil {
			return nil, err
		}
		var holds []AlertDisableHold
		if err := DB.WithContext(ctx).Select("target_key").Where("channel_id IN ? AND active = ?", batch, true).Find(&holds).Error; err != nil {
			return nil, err
		}
		blocked := make(map[string]bool, len(holds))
		for _, hold := range holds {
			blocked[hold.TargetKey] = true
		}
		var pauses []AlertChannelModelControl
		if err := DB.WithContext(ctx).Select("target_key").Where("channel_id IN ? AND manual_paused = ?", batch, true).Find(&pauses).Error; err != nil {
			return nil, err
		}
		for _, pause := range pauses {
			blocked[pause.TargetKey] = true
		}
		allowed := make(map[int]bool, len(channels))
		for _, channel := range channels {
			key, _, _ := ResolveChannelModelIdentity(&channel, requested)
			allowed[channel.Id] = channel.Status == common.ChannelStatusEnabled && !blocked[AlertIdentity("channel", channel.Id)] && !blocked[AlertIdentity("channel_model", channel.Id, "routing", key)]
		}
		for _, id := range batch {
			if allowed[id] {
				available = append(available, id)
			}
		}
	}
	return available, nil
}
