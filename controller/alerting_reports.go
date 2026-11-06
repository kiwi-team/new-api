package controller

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/alerting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// QueryAlertReport reads an explicit historical period without creating report
// events, notification jobs, or accounting adjustments. Key details use a
// bounded cursor; totals always cover the whole requested scope and period.
func QueryAlertReport(c *gin.Context) {
	id, ok := alertPathID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	rule, err := alerting.VisibleRule(ctx, alertActor(c), id, false)
	if err != nil {
		alertError(c, err)
		return
	}
	spec, err := model.AlertRuleConfig(rule)
	if err != nil || spec.Trigger.Mode != "schedule_report" || spec.QuotaUnit != 0 && spec.QuotaUnit != int64(common.QuotaPerUnit) {
		alertError(c, alerting.ErrInvalid)
		return
	}
	start, startErr := strconv.ParseInt(c.Query("start"), 10, 64)
	end, endErr := strconv.ParseInt(c.Query("end"), 10, 64)
	now := time.Now()
	format := c.DefaultQuery("format", "json")
	if startErr != nil || endErr != nil || start < 0 || end <= start || end > now.UnixMilli() || end-start > int64(31*24*time.Hour/time.Millisecond) || format != "json" && format != "csv" {
		alertError(c, alerting.ErrInvalid)
		return
	}
	page := alerting.ReportKeyPage{AfterTokenID: -1, Limit: 200}
	if raw := c.Query("after_token_id"); raw != "" {
		page.AfterTokenID, err = strconv.Atoi(raw)
		if err != nil || page.AfterTokenID < -1 {
			alertError(c, alerting.ErrInvalid)
			return
		}
	}
	targets, err := alerting.ResolveTargets(ctx, rule, spec)
	if err != nil {
		alertError(c, err)
		return
	}
	var selected *alerting.MonitoredTarget
	if raw := c.Query("target_id"); raw != "" {
		targetID, err := strconv.Atoi(raw)
		if err != nil {
			alertError(c, alerting.ErrInvalid)
			return
		}
		for i := range targets {
			if targets[i].ID == targetID {
				selected = &targets[i]
				break
			}
		}
	} else if len(targets) == 1 {
		selected = &targets[0]
	}
	if selected == nil {
		alertError(c, alerting.ErrInvalid)
		return
	}
	m, err := alerting.MeasureReport(ctx, rule, spec, *selected, start, end, now, page)
	if err != nil {
		alertError(c, err)
		return
	}
	if format == "json" {
		alertOK(c, m)
		return
	}
	var output bytes.Buffer
	output.WriteString("\xef\xbb\xbf")
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"row_type", "token_name", "token_id", "quota", "net_usd", "requests", "input_tokens", "output_tokens", "cached_tokens", "cache_write_5m_tokens", "cache_write_1h_tokens", "coverage", "reason", "window_start_ms", "window_end_ms"}); err != nil {
		alertError(c, err)
		return
	}
	unit := spec.QuotaUnit
	if unit == 0 {
		unit = int64(common.QuotaPerUnit)
	}
	for i, row := range append([]alerting.ReportUsage{m.Report.Totals}, m.Report.Keys...) {
		kind, name := "key", row.TokenName
		if i == 0 {
			kind, name = "total", selected.Name
		}
		// CSV quoting alone does not prevent spreadsheet formula execution.
		trimmed := strings.TrimLeft(name, " \t\r\n")
		if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
			name = "'" + name
		}
		values := []string{kind, name, strconv.Itoa(row.TokenID), strconv.FormatInt(row.Quota, 10), decimal.NewFromInt(row.Quota).Div(decimal.NewFromInt(unit)).String()}
		for _, value := range []int64{row.Requests, row.InputTokens, row.OutputTokens, row.CachedTokens, row.CacheWrite5mTokens, row.CacheWrite1hTokens} {
			values = append(values, strconv.FormatInt(value, 10))
		}
		values = append(values, m.Status, m.Reason, strconv.FormatInt(start, 10), strconv.FormatInt(end, 10))
		if err := writer.Write(values); err != nil {
			alertError(c, err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		alertError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="alert-report.csv"`)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Alert-Has-More", strconv.FormatBool(m.Report.HasMoreKeys))
	c.Header("X-Alert-Next-Token-ID", strconv.Itoa(m.Report.NextTokenID))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", output.Bytes())
}
