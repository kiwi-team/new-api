package alerting

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Engine struct {
	BootID  string
	Deliver func(context.Context, model.AlertTarget, model.AlertEvent) (int, error)
	Probe   func(context.Context, model.AlertDisableHold) ProbeResult
}

func matches(value, threshold int64, operator string) bool {
	switch operator {
	case "gt":
		return value > threshold
	case "gte":
		return value >= threshold
	case "lt":
		return value < threshold
	case "lte":
		return value <= threshold
	}
	return false
}

func (e *Engine) Evaluate(ctx context.Context, now time.Time) error {
	var rules []model.AlertRule
	if err := model.DB.WithContext(ctx).Where("enabled = ? AND next_evaluation_at <= ?", true, now.UnixMilli()).Order("next_evaluation_at asc, id asc").Limit(100).Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		interval := int64(15000)
		spec, parseErr := model.AlertRuleConfig(rule)
		if spec.Condition.Metric == "inflight_overdue_count" {
			interval = 5000
		}
		err := parseErr
		cursor := rule.EvaluationCursor
		if err == nil {
			// Bound one rule independently of the whole batch. A large selector
			// resumes after its last committed target instead of repeatedly
			// consuming the worker deadline ahead of every unrelated rule.
			bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
			cursor, err = e.evaluateRule(bounded, rule, spec, now)
			cancel()
		}
		code := ""
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			code, interval = "evaluation_failed", 60000
			budget := errors.Is(err, context.DeadlineExceeded)
			if budget {
				code, interval = "evaluation_budget", 5000
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				code = "target_unavailable"
			}
			if parseErr != nil || errors.Is(err, ErrInvalid) {
				code = "invalid_configuration"
			}
			// One removed object or malformed rule cannot starve unrelated rules. Keep
			// firing/hold state, but clear pending time because this interval is unknown.
			if !budget {
				if updateErr := model.DB.WithContext(ctx).Model(&model.AlertRuleState{}).Where("rule_id = ? AND generation = ?", rule.ID, rule.Generation).Updates(map[string]any{"data_status": "error", "pending_at": 0, "recovery_at": 0}).Error; updateErr != nil {
					return updateErr
				}
			}
		}
		if err := model.DB.WithContext(ctx).Model(&model.AlertRule{}).Where("id = ? AND revision = ?", rule.ID, rule.Revision).Updates(map[string]any{"next_evaluation_at": now.UnixMilli() + interval, "evaluation_cursor": cursor, "last_evaluated_at": now.UnixMilli(), "last_evaluation_error": code}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) evaluateRule(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, now time.Time) (string, error) {
	// Compatibility signals keep the original trigger/checkpoint authority.
	// They enter the same durable event and delivery path, never a timer replay.
	if spec.Condition.Metric == "legacy_notice" {
		if !LegacyNoticeSourceReady(spec.Condition.LegacySignal) {
			return "", &BlockedAction{Code: "data_incomplete"}
		}
		return "", nil
	}
	targets, err := ResolveTargets(ctx, rule, spec)
	if err != nil {
		return rule.EvaluationCursor, err
	}
	slices.SortFunc(targets, func(a, b MonitoredTarget) int { return strings.Compare(a.Key(), b.Key()) })
	cursor := rule.EvaluationCursor
	for _, target := range targets {
		key := target.Key()
		if key <= rule.EvaluationCursor {
			continue
		}
		if spec.Trigger.Mode == "schedule_report" {
			if err := e.EvaluateReports(ctx, rule, spec, target, now); err != nil {
				return cursor, err
			}
			cursor = key
			continue
		}
		measurement, err := Measure(ctx, rule, spec, target, now)
		if err != nil {
			return cursor, err
		}
		if err = e.ApplyMeasurement(ctx, rule, spec, target, measurement, now); err != nil && !errors.Is(err, model.ErrAlertConflict) {
			return cursor, err
		}
		cursor = key
	}
	return "", nil
}

func (e *Engine) ApplyMeasurement(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, now time.Time) error {
	threshold, err := RuleThreshold(spec, false)
	if err != nil {
		return err
	}
	recovery, err := RuleThreshold(spec, true)
	if err != nil {
		return err
	}
	return model.WithAlertState(ctx, rule, target.Type, target.ID, target.Model, func(tx *gorm.DB, state *model.AlertRuleState) error {
		stamp := now.UnixMilli()
		state.TargetName = target.Name
		state.DataStatus, state.LastValue, state.LastCount, state.UpdatedAt = m.Status, strconv.FormatInt(m.Value, 10), m.Samples, stamp
		if m.Status != "complete" {
			state.PendingAt, state.RecoveryAt = 0, 0
			return nil
		}
		_, _, period, err := WindowBounds(rule, spec.Condition, now)
		if err != nil {
			return err
		}
		mode := spec.Trigger.Mode
		if mode == "once_per_period" && state.PeriodKey != period {
			state.PeriodKey = period
			state.Status = "normal"
			state.IncidentID = 0
			state.PendingAt = 0
		}
		if (mode == "once" || mode == "once_per_period") && state.Status == "firing" {
			return nil
		}
		firing := matches(m.Value, threshold, spec.Condition.Operator)
		if mode == "milestone" {
			step, err := USDQuota(spec.Trigger.StepUSD)
			if err != nil || step <= 0 {
				return ErrInvalid
			}
			tier := max(m.Value, 0) / step
			if tier <= state.HighestTier {
				return nil
			}
			previous := state.HighestTier
			state.HighestTier = tier
			return e.createEvent(tx, rule, spec, target, m, state, "milestone", fmt.Sprintf("%d-%d", previous+1, tier), stamp)
		}
		if mode == "schedule_report" {
			return e.applyReport(tx, rule, spec, target, m, state, now)
		}
		if state.Status == "firing" && state.IncidentID != 0 {
			var incident model.AlertIncident
			if err := tx.First(&incident, state.IncidentID).Error; err != nil {
				return err
			}
			if incident.ClosedAt != 0 {
				// Closing ends this follow-up, not future protection. A continuing
				// condition must satisfy a fresh pending interval and incident.
				state.Status, state.PendingAt, state.RecoveryAt, state.IncidentID = "normal", 0, 0, 0
				return nil
			}
		}
		if state.Status == "firing" {
			recovered := m.Value <= recovery
			if spec.Condition.Operator == "lt" || spec.Condition.Operator == "lte" {
				recovered = m.Value >= recovery
			}
			if recovered {
				if state.RecoveryAt == 0 {
					state.RecoveryAt = stamp
				}
				if stamp-state.RecoveryAt >= spec.Condition.RecoverySeconds*1000 {
					state.Status, state.PendingAt = "resolved", 0
					if state.IncidentID != 0 {
						if err := tx.Model(&model.AlertIncident{}).Where("id = ?", state.IncidentID).Updates(map[string]any{"status": "resolved", "resolved_at": stamp, "updated_at": stamp}).Error; err != nil {
							return err
						}
					}
					if spec.Notify.OnRecovery {
						return e.createEvent(tx, rule, spec, target, m, state, "recovery", strconv.FormatInt(state.IncidentID, 10), stamp)
					}
				}
				return nil
			}
			state.RecoveryAt = 0
			if stamp-state.LastNotifiedAt >= spec.Trigger.RepeatSeconds*1000 {
				return e.createEvent(tx, rule, spec, target, m, state, "repeat", strconv.FormatInt(stamp/(spec.Trigger.RepeatSeconds*1000), 10), stamp)
			}
			return e.maybeEscalate(tx, rule, spec, target, m, state, stamp)
		}
		if !firing {
			state.Status, state.PendingAt = "normal", 0
			return nil
		}
		if state.PendingAt == 0 {
			state.PendingAt = stamp
			if m.ThresholdCrossedAt > 0 {
				state.PendingAt = max(m.ThresholdCrossedAt, rule.EffectiveAt)
			}
		}
		state.Status = "pending"
		if stamp-state.PendingAt < spec.Condition.HoldSeconds*1000 {
			return nil
		}
		state.Status = "firing"
		incident := model.AlertIncident{RuleID: rule.ID, OwnerUserID: rule.OwnerUserID, TargetKey: target.Key(), TargetName: target.Name, Severity: spec.Severity, Status: "firing", HandlingStatus: "unacknowledged", OpenedAt: stamp, UpdatedAt: stamp}
		if err := tx.Create(&incident).Error; err != nil {
			return err
		}
		state.IncidentID = incident.ID
		return e.createEvent(tx, rule, spec, target, m, state, "trigger", strconv.FormatInt(incident.ID, 10), stamp)
	})
}

func (e *Engine) createEvent(tx *gorm.DB, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, state *model.AlertRuleState, kind, identity string, stamp int64) error {
	evidence, err := common.Marshal(struct {
		Target      MonitoredTarget `json:"target"`
		Measurement Measurement     `json:"measurement"`
	}{target, m})
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("%s | %s | %s=%s | samples=%d", spec.Name, target.Name, spec.Condition.Metric, strconv.FormatInt(m.Value, 10), m.Samples)
	if kind == "recovery" && spec.Condition.Metric == "inflight_overdue_count" {
		summary = "Waiting ended | " + summary
	}
	event := model.AlertEvent{DedupKey: model.AlertIdentity(rule.ID, rule.Generation, target.Key(), kind, identity), RuleID: rule.ID, IncidentID: state.IncidentID, OwnerUserID: rule.OwnerUserID, Kind: kind, Severity: spec.Severity, TargetName: target.Name, Summary: summary, Evidence: string(evidence), RuleConfig: rule.Config, ConditionSatisfiedAt: state.PendingAt + spec.Condition.HoldSeconds*1000, CreatedAt: stamp}
	if kind == "milestone" {
		first, last, _ := strings.Cut(identity, "-")
		event.MilestoneFirst, _ = strconv.ParseInt(first, 10, 64)
		event.MilestoneLast, _ = strconv.ParseInt(last, 10, 64)
	}
	event.ThresholdCrossedAt, event.SourceAcceptedAt = m.ThresholdCrossedAt, m.SourceAcceptedAt
	if event.ThresholdCrossedAt == 0 {
		event.ThresholdCrossedAt = state.PendingAt
		event.TimingBasis = "first_verifiable_evaluation"
	} else {
		event.TimingBasis = "lifecycle_deadline"
	}
	var existing model.AlertEvent
	if found := tx.Where("dedup_key = ?", event.DedupKey).Limit(1).Find(&existing); found.Error != nil {
		return found.Error
	} else if found.RowsAffected != 0 {
		return nil
	}
	created, err := model.InsertAlertEventOnce(tx, &event)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	priority := 3
	switch spec.Severity {
	case "P0":
		priority = 0
	case "P1":
		priority = 1
	case "P2":
		priority = 2
	}
	for _, id := range spec.Notify.TargetIDs {
		var target model.AlertTarget
		lookup := tx.Where("id = ?", id).Limit(1).Find(&target)
		if lookup.Error != nil {
			return lookup.Error
		}
		status := "pending"
		if lookup.RowsAffected == 0 || !target.Enabled {
			status = "blocked"
		}
		job := model.AlertJob{Platform: spec.ManagementMode == "platform_enforced", DedupKey: model.AlertIdentity(event.ID, "notify", id, target.Revision), EventID: event.ID, RuleID: rule.ID, RuleRevision: rule.Revision, Kind: "notify", TargetName: target.Name, TargetType: target.Type, TargetHint: target.AddressHint, TargetID: id, TargetRevision: target.Revision, Priority: priority, Status: status, AvailableAt: stamp, ExpiresAt: stamp + 86400000, CreatedAt: stamp, UpdatedAt: stamp}
		if err := createJob(tx, &job); err != nil {
			return err
		}
	}
	if (kind == "trigger" || kind == "milestone") && spec.Condition.Metric != "inflight_overdue_count" {
		for _, action := range spec.Actions {
			objects := []MonitoredTarget{target}
			if action.Type == "disable_account_keys" || target.Type == "api_key_set" {
				var tokens []model.Token
				query := tx.Select("id", "name").Where("user_id = ?", rule.OwnerUserID)
				if target.Type == "api_key_set" && (!spec.Scope.IncludeFuture || spec.Scope.Selector != "all_owned") {
					query = query.Where("id IN ?", spec.Scope.EntityIDs)
				}
				if err := query.Limit(501).Find(&tokens).Error; err != nil {
					return err
				}
				if len(tokens) > 500 {
					return ErrInvalid
				}
				objects = nil
				for _, token := range tokens {
					objects = append(objects, MonitoredTarget{Type: "api_key", ID: token.Id, Name: token.Name})
				}
			}
			for _, object := range objects {
				payload, err := common.Marshal(object)
				if err != nil {
					return err
				}
				job := model.AlertJob{Platform: spec.ManagementMode == "platform_enforced", DedupKey: model.AlertIdentity(event.ID, "action", action.Type, object.Key()), EventID: event.ID, RuleID: rule.ID, RuleRevision: rule.Revision, Kind: "action", TargetName: object.Name, TargetType: object.Type, Action: action.Type, Payload: string(payload), Priority: priority, Status: "pending", AvailableAt: stamp, ExpiresAt: stamp + 300000, CreatedAt: stamp, UpdatedAt: stamp}
				if err := createJob(tx, &job); err != nil {
					return err
				}
			}
		}
	}
	state.LastNotifiedAt = stamp
	return nil
}

func createJob(tx *gorm.DB, job *model.AlertJob) error {
	var migration model.AlertMigration
	if err := tx.Where("rule_id = ?", job.RuleID).Limit(1).Find(&migration).Error; err != nil {
		return err
	}
	if common.AlertingShadowMode || migration.RuleID > 0 && migration.Executor != "unified" {
		job.Status = "shadow"
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(job).Error; err != nil {
		return err
	}
	// Concurrent primary delivery failures may request the same fallback. Its
	// stable identity and outbox must not roll back either delivery result.
	if job.ID == 0 {
		if err := tx.Where("dedup_key = ?", job.DedupKey).First(job).Error; err != nil {
			return err
		}
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.AlertOutbox{ID: model.AlertIdentity("job", job.DedupKey), Kind: job.Kind, Reference: strconv.FormatInt(job.ID, 10), CreatedAt: job.CreatedAt}).Error
}

func (e *Engine) maybeEscalate(tx *gorm.DB, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, state *model.AlertRuleState, stamp int64) error {
	if spec.Severity != "P0" && spec.Severity != "P1" && !spec.Acknowledgement.Required {
		return nil
	}
	var incident model.AlertIncident
	if err := tx.First(&incident, state.IncidentID).Error; err != nil {
		return err
	}
	if incident.AcknowledgedAt != 0 || incident.ClosedAt != 0 {
		return nil
	}
	remind := spec.Acknowledgement.RemindAfterSeconds
	if remind == 0 {
		remind = 300
	}
	escalate := spec.Acknowledgement.EscalateAfterSeconds
	if escalate == 0 {
		escalate = 600
	}
	if stamp-incident.OpenedAt >= escalate*1000 {
		if len(spec.Acknowledgement.EscalationTargetIDs) > 0 {
			spec.Notify.TargetIDs = spec.Acknowledgement.EscalationTargetIDs
		}
		return e.createEvent(tx, rule, spec, target, m, state, "escalation", strconv.FormatInt(incident.ID, 10), stamp)
	}
	if stamp-incident.OpenedAt >= remind*1000 {
		return e.createEvent(tx, rule, spec, target, m, state, "unacknowledged", strconv.FormatInt(incident.ID, 10), stamp)
	}
	return nil
}

func (e *Engine) applyReport(tx *gorm.DB, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, state *model.AlertRuleState, now time.Time) error {
	loc, err := time.LoadLocation(spec.Condition.Window.Timezone)
	if err != nil {
		return err
	}
	local := now.In(loc)
	sendAt := spec.Condition.Window.SendAt
	if sendAt == "" {
		sendAt = "00:10"
	}
	clock, err := time.Parse("15:04", sendAt)
	if err != nil {
		return ErrInvalid
	}
	if spec.Condition.Window.Type == "hour" && local.Minute() < clock.Minute() {
		return nil
	}
	if spec.Condition.Window.Type != "hour" && local.Hour()*60+local.Minute() < clock.Hour()*60+clock.Minute() {
		return nil
	}
	period := strconv.FormatInt(m.WindowStart, 10)
	if state.PeriodKey == period {
		return nil
	}
	state.PeriodKey = period
	return e.createEvent(tx, rule, spec, target, m, state, "report", period, now.UnixMilli())
}

func (e *Engine) RunJobs(ctx context.Context, kind string, now time.Time) error {
	return e.runJobs(ctx, kind, now, false)
}
func (e *Engine) RunUrgentJobs(ctx context.Context, kind string, now time.Time) error {
	return e.runJobs(ctx, kind, now, true)
}

func (e *Engine) runJobs(ctx context.Context, kind string, now time.Time, urgent bool) error {
	for range 20 {
		if err := ctx.Err(); err != nil {
			return err
		}
		now = time.Now()
		var job *model.AlertJob
		var err error
		if urgent {
			job, err = model.ClaimUrgentAlertJob(ctx, kind, e.BootID+"-urgent", now.UnixMilli())
		} else {
			job, err = model.ClaimAlertJob(ctx, kind, e.BootID, now.UnixMilli())
		}
		if err != nil {
			return err
		}
		if job == nil {
			return nil
		}
		if err := e.runJob(ctx, *job, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) runJob(ctx context.Context, job model.AlertJob, now time.Time) error {
	started := time.Now()
	stamp := now.UnixMilli()
	status, code := "succeeded", ""
	httpStatus := 0
	rendered, language := "", ""
	var rule model.AlertRule
	var event model.AlertEvent
	if err := model.DB.WithContext(ctx).First(&event, job.EventID).Error; err != nil {
		return err
	}
	var err error
	if job.Kind == "action" {
		err = model.DB.WithContext(ctx).First(&rule, job.RuleID).Error
	}
	notifyAuthorized := true
	if job.Kind == "notify" && job.RuleID > 0 {
		var current model.AlertRule
		result := model.DB.WithContext(ctx).Unscoped().Where("id = ?", job.RuleID).Limit(1).Find(&current)
		if result.Error != nil {
			return result.Error
		}
		notifyAuthorized = result.RowsAffected > 0 && !current.DeletedAt.Valid
		if notifyAuthorized {
			spec, parseErr := model.AlertRuleConfig(current)
			if parseErr != nil {
				return parseErr
			}
			notifyAuthorized = model.AlertRuleAllowsNotification(spec, job.TargetID)
		}
	}
	if common.AlertingShadowMode && event.Kind != "test" {
		status, code, err = "shadow", "shadow_mode", nil
	} else if job.Kind == "notify" && !notifyAuthorized {
		status, code = "canceled", "authorization_revoked"
	} else if job.Kind == "action" && (err != nil || !rule.Enabled || rule.Revision != job.RuleRevision) {
		status, code = "canceled", "rule_changed"
		err = nil
	} else if kind := job.Kind; kind == "notify" {
		var target model.AlertTarget
		err = model.DB.WithContext(ctx).First(&target, job.TargetID).Error
		if err != nil || !target.Enabled || target.Revision != job.TargetRevision {
			status, code = "blocked", "target_changed"
			err = nil
		} else if e.Deliver == nil {
			status, code = "blocked", "delivery_unavailable"
		} else {
			rendered, language = RenderNotification(target.Language, event), target.Language
			var allowed bool
			limitPriority := job.Priority
			if !job.Platform {
				// A personal P0 label cannot consume the platform's reserved
				// destination capacity, even when both use the same robot.
				limitPriority = max(limitPriority, 2)
			}
			allowed, err = model.ReserveAlertNotification(ctx, target, limitPriority, now)
			if err == nil && !allowed {
				status, code = "retry_wait", "rate_limited"
			} else if err == nil {
				httpStatus, err = e.Deliver(ctx, target, event)
			}
		}
	} else {
		if !common.AlertingActionsEnabled {
			status, code = "blocked", "actions_disabled"
		} else {
			spec, parseErr := model.AlertRuleConfig(rule)
			if parseErr != nil {
				return parseErr
			}
			var target MonitoredTarget
			if err := common.UnmarshalJsonStr(job.Payload, &target); err != nil {
				return err
			}
			measureTarget, parseErr := TargetFromEvent(event)
			if parseErr != nil {
				return parseErr
			}
			measurement, measureErr := Measure(context.WithValue(ctx, freshMetricKey{}, true), rule, spec, measureTarget, now)
			threshold, thresholdErr := RuleThreshold(spec, false)
			operator := spec.Condition.Operator
			if event.Kind == "milestone" {
				step, stepErr := USDQuota(spec.Trigger.StepUSD)
				if stepErr != nil || event.MilestoneLast <= 0 || event.MilestoneLast > int64(^uint64(0)>>1)/max(step, 1) {
					thresholdErr = ErrInvalid
				} else {
					threshold = event.MilestoneLast * step
					operator = "gte"
				}
			}
			if measureErr != nil || thresholdErr != nil || measurement.Status != "complete" {
				status, code = "retry_wait", "data_incomplete"
			} else if !matches(measurement.Value, threshold, operator) {
				status, code = "canceled", "condition_changed"
			} else {
				err = e.ExecuteAction(ctx, job, rule, spec, target, event)
			}
		}
	}
	if err != nil {
		// Never persist an HTTP/client error: it may contain a webhook credential.
		status, code = "retry_wait", "delivery_failed"
		if job.Attempts >= 8 {
			status = "failed"
		}
		var blocked *BlockedAction
		if errors.As(err, &blocked) {
			status, code = "blocked", blocked.Code
		}
		if errors.Is(err, ErrTargetKeyUnavailable) || errors.Is(err, ErrTargetRejected) {
			status, code = "blocked", "target_unavailable"
		}
		if httpStatus >= 400 && httpStatus < 500 && httpStatus != 408 && httpStatus != 429 {
			status, code = "failed", "target_rejected"
		}
	}
	delay := []int64{10000, 30000, 120000, 300000, 900000}[min(max(job.Attempts-1, 0), 4)]
	var deliveryError *DeliveryError
	if errors.As(err, &deliveryError) {
		delay = max(delay, deliveryError.RetryAfter.Milliseconds())
	}
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": status, "last_error": code, "locked_by": "", "locked_until": 0, "updated_at": stamp, "available_at": stamp + delay}
		if rendered != "" {
			updates["rendered_content"], updates["rendered_language"] = rendered, language
		}
		if status == "succeeded" {
			updates["accepted_at"] = time.Now().UnixMilli()
		}
		result := tx.Model(&model.AlertJob{}).Where("id = ? AND locked_by = ? AND status = ? AND locked_until = ? AND locked_until > ?", job.ID, job.LockedBy, "running", job.LockedUntil, time.Now().UnixMilli()).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrSystemTaskLockLost
		}
		if status == "retry_wait" {
			if err := tx.Create(&model.AlertOutbox{ID: model.AlertIdentity("job-retry", job.ID, job.Attempts), Kind: job.Kind, Reference: strconv.FormatInt(job.ID, 10), CreatedAt: stamp + delay}).Error; err != nil {
				return err
			}
		}
		if job.Kind == "notify" && job.Priority <= 1 && (status == "failed" || status == "blocked" || status == "retry_wait" && code == "delivery_failed") {
			if err := e.enqueueFallback(tx, event, stamp); err != nil {
				return err
			}
		}
		return tx.Create(&model.AlertJobAttempt{JobID: job.ID, Status: status, HTTPStatus: httpStatus, ErrorCode: code, DurationMS: time.Since(started).Milliseconds(), CreatedAt: stamp}).Error
	})
}

func (e *Engine) enqueueFallback(tx *gorm.DB, event model.AlertEvent, stamp int64) error {
	var spec model.AlertRuleSpec
	if common.UnmarshalJsonStr(event.RuleConfig, &spec) != nil {
		return nil
	}
	for _, id := range spec.Acknowledgement.EscalationTargetIDs {
		if slices.Contains(spec.Notify.TargetIDs, id) {
			continue
		}
		dedup := model.AlertIdentity("fallback", event.ID, id)
		var existing model.AlertJob
		result := tx.Where("dedup_key = ?", dedup).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			continue
		}
		var target model.AlertTarget
		result = tx.Where("id = ? AND enabled = ?", id, true).Limit(1).Find(&target)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		job := model.AlertJob{Platform: spec.ManagementMode == "platform_enforced", DedupKey: dedup, EventID: event.ID, RuleID: event.RuleID, Kind: "notify", TargetName: target.Name, TargetType: target.Type, TargetHint: target.AddressHint, TargetID: id, TargetRevision: target.Revision, Priority: 0, Status: "pending", AvailableAt: stamp, ExpiresAt: stamp + 86400000, CreatedAt: stamp, UpdatedAt: stamp}
		if err := createJob(tx, &job); err != nil {
			return err
		}
	}
	return nil
}

func TargetFromEvent(event model.AlertEvent) (MonitoredTarget, error) {
	var evidence struct {
		Target MonitoredTarget `json:"target"`
	}
	err := common.UnmarshalJsonStr(event.Evidence, &evidence)
	return evidence.Target, err
}
