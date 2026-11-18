package alerting

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// renderFeishuCard uses the same immutable evidence and units as text delivery.
// Dynamic names and legacy content are plain text, so they cannot create links,
// markup or @all mentions inside a card.
func renderFeishuCard(language string, event model.AlertEvent) map[string]any {
	_ = i18n.Init()
	var spec model.AlertRuleSpec
	var evidence notificationEvidence
	structured := common.UnmarshalJsonStr(event.RuleConfig, &spec) == nil && common.UnmarshalJsonStr(event.Evidence, &evidence) == nil
	severity := event.Severity
	if severity == "" {
		severity = spec.Severity
	}
	color, level := "grey", "⚪"
	switch severity {
	case "P0":
		color, level = "red", "🆘"
	case "P1":
		color, level = "red", "🔴"
	case "P2":
		color, level = "orange", "🟠"
	case "P3":
		color, level = "blue", "🔵"
	}
	icon := "🔔"
	kind := i18n.Translate(language, "alert.kind."+event.Kind)
	switch event.Kind {
	case "trigger":
		icon = "🚨"
	case "repeat":
		icon = "⏳"
	case "recovery":
		icon, color = "✅", "green"
	case "recovery_probe":
		icon, color = "🧪", "green"
	case "escalation":
		icon, color = "📣", "purple"
	case "unacknowledged":
		icon = "⏰"
	case "milestone":
		icon = "💰"
	case "report":
		icon, color = "📊", "blue"
	case "report_revision":
		icon, color = "📝", "blue"
	case "report_data_gap":
		icon, color = "⚠️", "yellow"
	case "test":
		kind, color = i18n.Translate(language, "alert.card.test"), "blue"
	case "engine_health":
		icon, color, kind = "🛠️", "red", i18n.Translate(language, "alert.card.engine_health")
	case "legacy_notice":
		icon, kind = "📣", i18n.Translate(language, "alert.card.notification")
	default:
		kind = i18n.Translate(language, "alert.card.notification")
	}
	title := icon + " " + kind
	if severity != "" {
		title += " · " + level + " " + severity
	}
	if spec.Name != "" {
		title += " · " + spec.Name
	}
	elements := []map[string]any{}
	if spec.Name != "" {
		elements = append(elements, feishuParagraph("📌 "+i18n.Translate(language, "alert.card.rule")+"\n"+spec.Name))
	}
	if event.Kind == "recovery" || event.Kind == "repeat" || event.Kind == "escalation" || event.Kind == "unacknowledged" {
		elements = append(elements, feishuParagraph(i18n.Translate(language, "alert.card.status."+event.Kind)))
	}
	target := evidence.Target
	if target.Name == "" {
		target.Name = event.TargetName
	}
	if event.Kind == "legacy_notice" {
		var notice LegacyNotice
		if common.UnmarshalJsonStr(event.Evidence, &notice) == nil {
			if notice.Title != "" {
				title = "📣 " + notice.Title
				if severity != "" {
					title += " · " + level + " " + severity
				}
			}
			if notice.ChannelID > 0 {
				target.Type, target.ID = "channel", notice.ChannelID
			}
		}
	}
	fields := []map[string]any{}
	switch target.Type {
	case "channel_model", "channel":
		name := target.Name
		if target.Type == "channel_model" && target.Model != "" {
			name = strings.TrimSuffix(name, " / "+target.Model)
		}
		if name != "" {
			name += " · "
		}
		fields = append(fields, feishuField("🌐 "+i18n.Translate(language, "alert.card.channel"), name+"ID "+strconv.Itoa(target.ID)))
		if target.Type == "channel_model" {
			fields = append(fields, feishuField("🧩 "+i18n.Translate(language, "alert.card.model"), target.Model))
		}
	case "model":
		fields = append(fields, feishuField("🧩 "+i18n.Translate(language, "alert.card.model"), target.Model))
	case "account":
		fields = append(fields, feishuField("👤 "+i18n.Translate(language, "alert.card.account"), target.Name))
	case "api_key", "api_key_set":
		fields = append(fields, feishuField("🔑 "+i18n.Translate(language, "alert.card.api_key"), target.Name))
	case "system":
		fields = append(fields, feishuField("⚙️ "+i18n.Translate(language, "alert.card.system"), target.Name))
	case "uid_budget", "project_budget":
		fields = append(fields, feishuField("👛 "+i18n.Translate(language, "alert.card.budget"), target.Name))
	default:
		if target.Name != "" {
			fields = append(fields, feishuField("🎯 "+i18n.Translate(language, "alert.card.target"), target.Name))
		}
	}
	if len(fields) > 0 {
		elements = append(elements, map[string]any{"tag": "div", "fields": fields})
	}
	location, timezone := notificationTimezone(spec)
	switch event.Kind {
	case "legacy_notice":
		var notice LegacyNotice
		if common.UnmarshalJsonStr(event.Evidence, &notice) == nil {
			elements = append(elements, feishuParagraph(notice.Content))
		} else {
			elements = append(elements, feishuParagraph(event.Summary))
		}
	case "test", "engine_health":
		elements = append(elements, feishuParagraph(RenderNotification(language, event)))
	case "recovery_probe":
		// Probe events carry an in-flight safety check, not a fresh sample of
		// the original rule metric. Do not present that zero as a success rate.
		elements = append(elements, feishuParagraph(i18n.Translate(language, "alert.kind.recovery_probe")))
	default:
		if structured {
			measurement := evidence.Measurement
			value, threshold := notificationMetricValues(spec, measurement, event.Kind == "recovery")
			metrics := []map[string]any{feishuField("📊 "+notificationMetricName(language, spec.Condition), value)}
			if event.Kind != "report" && event.Kind != "report_revision" && event.Kind != "report_data_gap" {
				label := "alert.card.threshold"
				operator := map[string]string{"gt": ">", "gte": "≥", "lt": "<", "lte": "≤", "eq": "="}[spec.Condition.Operator]
				if event.Kind == "recovery" {
					label, operator = "alert.card.recovery_threshold", "≤"
					if spec.Condition.Operator == "lt" || spec.Condition.Operator == "lte" {
						operator = "≥"
					}
				}
				metrics = append(metrics, feishuField("🎯 "+i18n.Translate(language, label), strings.TrimSpace(operator+" "+threshold)))
			}
			coverage := i18n.Translate(language, "alert.card.coverage."+measurement.Status)
			if coverage == "alert.card.coverage."+measurement.Status {
				coverage = i18n.Translate(language, "alert.card.coverage.unknown")
			}
			metrics = append(metrics, feishuField("🧪 "+i18n.Translate(language, "alert.card.samples"), strconv.FormatInt(measurement.Samples, 10)), feishuField("📡 "+i18n.Translate(language, "alert.card.coverage"), coverage))
			elements = append(elements, map[string]any{"tag": "hr"}, map[string]any{"tag": "div", "fields": metrics})
			if measurement.WindowStart > 0 && measurement.WindowEnd > 0 {
				start := time.UnixMilli(measurement.WindowStart).In(location).Format("2006-01-02 15:04:05")
				end := time.UnixMilli(measurement.WindowEnd).In(location).Format("2006-01-02 15:04:05")
				elements = append(elements, feishuParagraph("📆 "+i18n.Translate(language, "alert.card.window")+"\n"+start+" → "+end+"\n"+timezone))
			}
			if details := strings.TrimSpace(notificationDetails(language, event, spec, evidence)); details != "" {
				elements = append(elements, feishuParagraph(details))
			}
		} else {
			elements = append(elements, feishuParagraph(RenderNotification(language, event)))
		}
	}
	if event.CreatedAt > 0 {
		elements = append(elements, feishuParagraph("🕒 "+i18n.Translate(language, "alert.card.time")+" · "+time.UnixMilli(event.CreatedAt).In(location).Format("2006-01-02 15:04:05")+" ("+timezone+")"))
	}
	footer := "new-api"
	if event.ID > 0 {
		footer += " · " + i18n.Translate(language, "alert.card.event") + " #" + strconv.FormatInt(event.ID, 10)
		link := strings.TrimRight(system_setting.ServerAddress, "/") + "/alerts?event_id=" + strconv.FormatInt(event.ID, 10)
		if parsed, err := url.Parse(link); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil && len(link) <= 2048 {
			elements = append(elements, map[string]any{"tag": "action", "actions": []any{map[string]any{"tag": "button", "text": feishuPlainText("🔎 "+i18n.Translate(language, "alert.card.details"), 80), "type": "primary", "url": link}}})
		}
	}
	elements = append(elements, map[string]any{"tag": "note", "elements": []any{feishuPlainText(footer, 160)}})
	card := map[string]any{"config": map[string]any{"wide_screen_mode": true}, "header": map[string]any{"template": color, "title": feishuPlainText(title, 120)}, "elements": elements}
	// Leave room for the bot signature and keep unusually verbose reports
	// deliverable. The detail link still opens the complete persisted evidence.
	if data, err := common.Marshal(card); err == nil && len(data) > 19*1024 {
		for _, element := range elements {
			if text, ok := element["text"].(map[string]string); ok {
				element["text"] = feishuPlainText(text["content"], 500)
			}
		}
	}
	return card
}

func feishuPlainText(content string, limit int) map[string]string {
	characters := []rune(content)
	if len(characters) > limit {
		content = string(characters[:limit]) + "…"
	}
	return map[string]string{"tag": "plain_text", "content": content}
}

func feishuParagraph(content string) map[string]any {
	if strings.TrimSpace(content) == "" {
		content = "—"
	}
	return map[string]any{"tag": "div", "text": feishuPlainText(content, 1800)}
}

func feishuField(label, value string) map[string]any {
	return map[string]any{"is_short": true, "text": feishuPlainText(label+"\n"+value, 400)}
}
