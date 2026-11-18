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

type notificationEvidence struct {
	Target      MonitoredTarget `json:"target"`
	Measurement Measurement     `json:"measurement"`
}

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
		message := notice.Title + "\n" + notice.Content
		if event.ID > 0 && event.CreatedAt > 0 {
			location := time.UTC
			var spec model.AlertRuleSpec
			if common.UnmarshalJsonStr(event.RuleConfig, &spec) == nil {
				if configured, err := time.LoadLocation(spec.Condition.Window.Timezone); err == nil {
					location = configured
				}
			}
			message += "\n" + i18n.Translate(language, "alert.notice.context", map[string]any{"Rule": spec.Name, "Time": time.UnixMilli(event.CreatedAt).In(location).Format(time.RFC3339), "EventID": event.ID, "Link": system_setting.ServerAddress + "/alerts?event_id=" + strconv.FormatInt(event.ID, 10)})
		}
		return message
	}
	if event.Kind == "test" {
		return i18n.Translate(language, "alert.test")
	}
	var spec model.AlertRuleSpec
	var evidence notificationEvidence
	if common.UnmarshalJsonStr(event.RuleConfig, &spec) != nil || common.UnmarshalJsonStr(event.Evidence, &evidence) != nil {
		return event.Summary
	}
	value, thresholdText := notificationMetricValues(spec, evidence.Measurement, event.Kind == "recovery")
	metric := notificationMetricName(language, spec.Condition)
	location, timezone := notificationTimezone(spec)
	message := i18n.Translate(language, "alert.message", map[string]any{
		"Severity": spec.Severity, "Rule": spec.Name, "Target": event.TargetName, "Kind": i18n.Translate(language, "alert.kind."+event.Kind), "Metric": metric, "Value": value, "Threshold": thresholdText, "Samples": evidence.Measurement.Samples, "Coverage": evidence.Measurement.Status,
		"Start": time.UnixMilli(evidence.Measurement.WindowStart).In(location).Format(time.RFC3339), "End": time.UnixMilli(evidence.Measurement.WindowEnd).In(location).Format(time.RFC3339), "Timezone": timezone, "EventID": event.ID, "Link": system_setting.ServerAddress + "/alerts?event_id=" + strconv.FormatInt(event.ID, 10),
	})
	return message + notificationDetails(language, event, spec, evidence)
}

func notificationDetails(language string, event model.AlertEvent, spec model.AlertRuleSpec, evidence notificationEvidence) string {
	message := ""
	if traffic := evidence.Measurement.Traffic; traffic != nil && spec.Condition.Metric == "request_volume_change" {
		message += "\n" + i18n.Translate(language, "alert.traffic.details", map[string]any{"Calls": traffic.Count, "Baseline": decimal.NewFromInt(traffic.BaselineScaled).Shift(-4).String(), "Change": decimal.NewFromInt(traffic.ChangeBPS).Shift(-2).String() + "%", "Direction": i18n.Translate(language, "alert.traffic."+spec.Condition.ChangeDirection)})
	}
	if target := evidence.Target; target.ID > 0 {
		switch target.Type {
		case "account":
			message += "\n" + i18n.Translate(language, "alert.target.account", map[string]any{"ID": target.ID})
		case "api_key":
			message += "\n" + i18n.Translate(language, "alert.target.api_key", map[string]any{"ID": target.ID, "Owner": target.OwnerUserID})
		}
	}
	if event.Kind == "milestone" {
		message += "\n" + i18n.Translate(language, "alert.milestone.range", map[string]any{"First": event.MilestoneFirst, "Last": event.MilestoneLast, "Step": spec.Trigger.StepUSD})
		if step, err := decimal.NewFromString(spec.Trigger.StepUSD); err == nil {
			message += "\n" + i18n.Translate(language, "alert.milestone.amount", map[string]any{"Amount": step.Mul(decimal.NewFromInt(event.MilestoneLast)).String(), "Step": step.String()})
		}
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

// notificationMetricValues keeps units consistent across text and card delivery.
func notificationMetricValues(spec model.AlertRuleSpec, measurement Measurement, recovery bool) (string, string) {
	threshold, _ := RuleThreshold(spec, recovery)
	value := strconv.FormatInt(measurement.Value, 10)
	thresholdText := strconv.FormatInt(threshold, 10)
	metric := spec.Condition.Metric
	if strings.HasSuffix(metric, "_rate") || metric == "budget_remaining_ratio" || metric == "request_volume_change" {
		return decimal.NewFromInt(measurement.Value).Shift(-2).String() + "%", decimal.NewFromInt(threshold).Shift(-2).String() + "%"
	}
	if metric == "spend_net" || metric == "balance_remaining" {
		unit := spec.QuotaUnit
		if unit == 0 {
			unit = int64(common.QuotaPerUnit)
		}
		amount := spec.Condition.ThresholdUSD
		if recovery && spec.Condition.RecoveryThresholdUSD != "" {
			amount = spec.Condition.RecoveryThresholdUSD
		}
		return decimal.NewFromInt(measurement.Value).Div(decimal.NewFromInt(unit)).String() + " USD", amount + " USD"
	}
	if metric == "duration_ms" || metric == "ttft_ms" || metric == "evaluation_lag" || metric == "source_lag" {
		value += " ms"
		thresholdText += " ms"
	}
	return value, thresholdText
}

func notificationMetricName(language string, condition model.AlertCondition) string {
	if condition.Metric == "request_volume_change" {
		return i18n.Translate(language, "alert.traffic."+condition.ChangeDirection)
	}
	name := i18n.Translate(language, "alert.metric."+condition.Metric)
	if name == "alert.metric."+condition.Metric {
		return condition.Metric
	}
	return name
}

func notificationTimezone(spec model.AlertRuleSpec) (*time.Location, string) {
	if name := spec.Condition.Window.Timezone; name != "" {
		if location, err := time.LoadLocation(name); err == nil {
			return location, name
		}
	}
	return time.UTC, "UTC"
}
