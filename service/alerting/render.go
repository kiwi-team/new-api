package alerting

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/shopspring/decimal"
)

// RenderNotification uses structured snapshots, never live credentials or raw
// upstream errors. The rule and target names remain the names at trigger time.
func RenderNotification(language string, event model.AlertEvent) string {
	_ = i18n.Init()
	if event.Kind == "engine_health" {
		return i18n.Translate(language, "alert.engine_health", map[string]any{"Reason": i18n.Translate(language, "alert.metric."+event.Summary)})
	}
	if event.Kind == "legacy_notice" {
		var notice struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if common.UnmarshalJsonStr(event.Evidence, &notice) != nil {
			return event.Summary
		}
		return notice.Title + "\n" + notice.Content
	}
	if event.Kind == "test" {
		return i18n.Translate(language, "alert.test")
	}
	var spec model.AlertRuleSpec
	var evidence struct {
		Target      MonitoredTarget `json:"target"`
		Measurement Measurement     `json:"measurement"`
	}
	if common.UnmarshalJsonStr(event.RuleConfig, &spec) != nil || common.UnmarshalJsonStr(event.Evidence, &evidence) != nil {
		return event.Summary
	}
	threshold, _ := RuleThreshold(spec, false)
	value := strconv.FormatInt(evidence.Measurement.Value, 10)
	thresholdText := strconv.FormatInt(threshold, 10)
	if spec.Condition.Metric == "spend_net" || spec.Condition.Metric == "balance_remaining" {
		unit := spec.QuotaUnit
		if unit == 0 {
			unit = int64(common.QuotaPerUnit)
		}
		value = decimal.NewFromInt(evidence.Measurement.Value).Div(decimal.NewFromInt(unit)).String() + " USD"
		thresholdText = spec.Condition.ThresholdUSD + " USD"
	}
	if spec.Condition.Metric == "duration_ms" || spec.Condition.Metric == "ttft_ms" {
		value += " ms"
		thresholdText += " ms"
	}
	metric := i18n.Translate(language, "alert.metric."+spec.Condition.Metric)
	if metric == "alert.metric."+spec.Condition.Metric {
		metric = spec.Condition.Metric
	}
	message := i18n.Translate(language, "alert.message", map[string]any{
		"Severity": spec.Severity, "Rule": spec.Name, "Target": event.TargetName, "Kind": i18n.Translate(language, "alert.kind."+event.Kind), "Metric": metric, "Value": value, "Threshold": thresholdText, "Samples": evidence.Measurement.Samples, "Coverage": evidence.Measurement.Status,
		"Start": time.UnixMilli(evidence.Measurement.WindowStart).UTC().Format(time.RFC3339), "End": time.UnixMilli(evidence.Measurement.WindowEnd).UTC().Format(time.RFC3339), "Timezone": spec.Condition.Window.Timezone, "EventID": event.ID, "Link": system_setting.ServerAddress + "/alerts?event_id=" + strconv.FormatInt(event.ID, 10),
	})
	if event.Kind == "milestone" {
		message += "\n" + i18n.Translate(language, "alert.milestone.range", map[string]any{"First": event.MilestoneFirst, "Last": event.MilestoneLast, "Step": spec.Trigger.StepUSD})
	}
	report := evidence.Measurement.Report
	if report == nil {
		return message
	}
	unit := spec.QuotaUnit
	if unit == 0 {
		unit = int64(common.QuotaPerUnit)
	}
	var detail strings.Builder
	detail.WriteString(message)
	detail.WriteString("\n" + i18n.Translate(language, "alert.report.totals", map[string]any{"Requests": report.Totals.Requests, "Input": report.Totals.InputTokens, "Output": report.Totals.OutputTokens, "Cached": report.Totals.CachedTokens, "Write5m": report.Totals.CacheWrite5mTokens, "Write1h": report.Totals.CacheWrite1hTokens}))
	for _, key := range report.Keys[:min(len(report.Keys), 20)] {
		name := key.TokenName
		if name == "" {
			name = i18n.Translate(language, "alert.report.no_key")
			if key.TokenID > 0 {
				name = i18n.Translate(language, "alert.report.deleted_key", map[string]any{"ID": key.TokenID})
			}
		}
		amount := decimal.NewFromInt(key.Quota).Div(decimal.NewFromInt(unit)).String()
		detail.WriteString("\n" + i18n.Translate(language, "alert.report.key", map[string]any{"Name": name, "Amount": amount, "Requests": key.Requests}))
	}
	if report.Backfill {
		detail.WriteString("\n" + i18n.Translate(language, "alert.report.backfill"))
	}
	if report.FirstPartial || evidence.Measurement.Status != "complete" {
		detail.WriteString("\n" + i18n.Translate(language, "alert.report.partial"))
	}
	return detail.String()
}
