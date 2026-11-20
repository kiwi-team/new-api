package alerting

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// Usage totals are independent fields. Refund observations change quota only;
// neither reporting nor report revisions issue billing adjustments.
type ReportUsage struct {
	TokenID            int    `json:"token_id"`
	TokenName          string `json:"token_name"`
	Quota              int64  `json:"quota,string"`
	Requests           int64  `json:"requests,string"`
	InputTokens        int64  `json:"input_tokens,string"`
	OutputTokens       int64  `json:"output_tokens,string"`
	CachedTokens       int64  `json:"cached_tokens,string"`
	CacheWrite5mTokens int64  `json:"cache_write_5m_tokens,string"`
	CacheWrite1hTokens int64  `json:"cache_write_1h_tokens,string"`
}

type ReportSummary struct {
	Totals          ReportUsage   `json:"totals"`
	Keys            []ReportUsage `json:"keys"`
	CalculatedAt    int64         `json:"calculated_at"`
	Backfill        bool          `json:"backfill"`
	FirstPartial    bool          `json:"first_partial"`
	SourceStatus    string        `json:"source_status"`
	SourceReason    string        `json:"source_reason"`
	SourceWatermark int64         `json:"source_watermark"`
	HasMoreKeys     bool          `json:"has_more_keys"`
	NextTokenID     int           `json:"next_token_id"`
}

type ReportKeyPage struct {
	AfterTokenID int
	Limit        int
}

// EvaluateReports revisits bounded civil periods, including partial reports
// awaiting a complete revision. Stable period identities make restart/backfill
// safe without replacing any previously sent snapshot.
func (e *Engine) EvaluateReports(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, now time.Time) error {
	periods := spec.Trigger.Report.BackfillPeriods
	if periods == 0 {
		periods = 7
	}
	cursor := now
	for i := 0; i < periods; i++ {
		start, end, err := ReportBounds(spec.Condition.Window, cursor)
		if err != nil {
			return err
		}
		if end <= rule.EffectiveAt {
			break
		}
		firstPartial := start < rule.EffectiveAt
		if firstPartial && !spec.Trigger.Report.AllowFirstPartial {
			break
		}
		if firstPartial {
			start = rule.EffectiveAt
		}
		loc, err := time.LoadLocation(spec.Condition.Window.Timezone)
		if err != nil {
			return err
		}
		endLocal := time.UnixMilli(end).In(loc)
		clock, err := time.Parse("15:04", spec.Condition.Window.SendAt)
		if err != nil {
			clock, _ = time.Parse("15:04", "00:10")
		}
		due := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		if spec.Condition.Window.Type == "hour" {
			due = endLocal.Add(time.Duration(clock.Minute()) * time.Minute)
		}
		if !now.Before(due) {
			m, err := MeasureReport(ctx, rule, spec, target, start, end, now)
			if err != nil {
				return err
			}
			m.Report.Backfill = i > 0
			m.Report.FirstPartial = firstPartial
			partialDelay := spec.Trigger.Report.PartialAfterSeconds
			if partialDelay == 0 {
				partialDelay = 1800
			}
			if m.Status == "complete" || now.UnixMilli() >= end+partialDelay*1000 {
				if m.Status != "complete" {
					m.Status = "partial"
				}
				if err = e.applyReportPeriod(ctx, rule, spec, target, m, now); err != nil {
					return err
				}
			}
		}
		cursor = time.UnixMilli(end).Add(-time.Millisecond)
	}
	return nil
}

// MeasureReport exposes the same immutable summary contract to rule preview.
func MeasureReport(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, start, end int64, now time.Time, pages ...ReportKeyPage) (Measurement, error) {
	watermark, status, reason, err := SourceWatermark(ctx, now)
	m := Measurement{Status: status, Reason: reason, WindowStart: start, WindowEnd: end, SourceAcceptedAt: watermark, Report: &ReportSummary{CalculatedAt: now.UnixMilli(), SourceStatus: status, SourceReason: reason, SourceWatermark: watermark}}
	if err != nil {
		return m, err
	}
	if status == "complete" {
		rangeStatus, rangeReason, rangeErr := WindowCoverage(ctx, start, end, now)
		if rangeErr != nil {
			return m, rangeErr
		}
		if rangeStatus != "complete" {
			m.Status, m.Reason = rangeStatus, rangeReason
			m.Report.SourceStatus, m.Report.SourceReason = rangeStatus, rangeReason
		}
	}
	if m.Status == "complete" {
		usageStatus, usageReason, usageErr := UsageCoverage(ctx, now)
		if usageErr != nil {
			return m, usageErr
		}
		if usageStatus != "complete" {
			m.Status, m.Reason = usageStatus, usageReason
			m.Report.SourceStatus, m.Report.SourceReason = usageStatus, usageReason
		}
	}
	if status == "complete" && watermark < end {
		m.Status, m.Reason = "stale", "input_backlog"
		m.Report.SourceStatus, m.Report.SourceReason = m.Status, m.Reason
	}
	options := &sql.TxOptions{ReadOnly: true}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		options.Isolation = sql.LevelRepeatableRead
	}
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := usageFacts(tx.WithContext(ctx), spec, target, start, end, "quota, token_id, request_count, input_tokens, output_tokens, cached_tokens, cache_write5m_tokens, cache_write1h_tokens")
		var rows []ReportUsage
		columns := `COALESCE(SUM(quota),0) AS quota,COALESCE(SUM(request_count),0) AS requests,COALESCE(SUM(input_tokens),0) AS input_tokens,COALESCE(SUM(output_tokens),0) AS output_tokens,COALESCE(SUM(cached_tokens),0) AS cached_tokens,COALESCE(SUM(cache_write5m_tokens),0) AS cache_write5m_tokens,COALESCE(SUM(cache_write1h_tokens),0) AS cache_write1h_tokens`
		// SQL checks aggregate overflow during int64 scanning. Totals include every
		// key even when the bounded detail list needs pagination/export.
		if err := query.Session(&gorm.Session{}).Select(columns).Scan(&m.Report.Totals).Error; err != nil {
			return err
		}
		limit := 500
		detail := query.Session(&gorm.Session{}).Select("token_id," + columns).Group("token_id")
		if len(pages) > 0 {
			page := pages[0]
			if page.AfterTokenID < -1 || page.Limit < 1 || page.Limit > 200 {
				return ErrInvalid
			}
			limit = page.Limit
			detail = detail.Where("token_id > ?", page.AfterTokenID).Order("token_id asc")
		} else {
			detail = detail.Order("quota desc, token_id asc")
		}
		if err := detail.Limit(limit + 1).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			m.Report.HasMoreKeys = true
			m.Report.SourceReason = "report_key_capacity"
		}
		if len(rows) > 0 {
			m.Report.NextTokenID = rows[len(rows)-1].TokenID
		}
		var ids []int
		for _, row := range rows {
			if row.TokenID > 0 {
				ids = append(ids, row.TokenID)
			}
		}
		var tokens []model.Token
		if len(ids) > 0 {
			if err := tx.Select("id", "name").Where("id IN ? AND user_id = ?", ids, rule.OwnerUserID).Find(&tokens).Error; err != nil {
				return err
			}
		}
		names := make(map[int]string, len(tokens))
		for _, token := range tokens {
			names[token.Id] = token.Name
		}
		for i := range rows {
			rows[i].TokenName = names[rows[i].TokenID]
		}
		m.Report.Keys = rows
		m.Value, m.Samples = m.Report.Totals.Quota, m.Report.Totals.Requests
		return nil
	}, options)
	return m, err
}

func (e *Engine) applyReportPeriod(ctx context.Context, rule model.AlertRule, spec model.AlertRuleSpec, target MonitoredTarget, m Measurement, now time.Time) error {
	return model.WithAlertState(ctx, rule, target.Type, target.ID, target.Model, func(tx *gorm.DB, state *model.AlertRuleState) error {
		id := model.AlertIdentity("report-period", rule.ID, rule.Generation, target.Key(), m.WindowStart, m.WindowEnd)
		var period model.AlertReportPeriod
		result := tx.Where("id = ?", id).Limit(1).Find(&period)
		if result.Error != nil {
			return result.Error
		}
		kind := "report"
		if result.RowsAffected != 0 {
			if period.InitialStatus != "partial" || period.RevisedAt != 0 || m.Status != "complete" || !spec.Trigger.Report.SendRevision {
				return nil
			}
			kind = "report_revision"
			if err := tx.Model(&period).Update("revised_at", now.UnixMilli()).Error; err != nil {
				return err
			}
		} else {
			period = model.AlertReportPeriod{ID: id, RuleID: rule.ID, Generation: rule.Generation, TargetKey: target.Key(), WindowStart: m.WindowStart, WindowEnd: m.WindowEnd, InitialStatus: m.Status, CreatedAt: now.UnixMilli()}
			if err := tx.Create(&period).Error; err != nil {
				return err
			}
		}
		state.DataStatus, state.UpdatedAt = m.Status, now.UnixMilli()
		if spec.Trigger.Report.IncludeZero != nil && !*spec.Trigger.Report.IncludeZero && m.Value == 0 && m.Samples == 0 && m.Status == "complete" {
			return nil
		}
		identity := strconv.FormatInt(m.WindowStart, 10) + ":" + strconv.FormatInt(m.WindowEnd, 10)
		if err := e.createEvent(tx, rule, spec, target, m, state, kind, identity, now.UnixMilli()); err != nil {
			return err
		}
		if m.Status == "partial" {
			return e.createEvent(tx, rule, spec, target, m, state, "report_data_gap", identity, now.UnixMilli())
		}
		return nil
	})
}
