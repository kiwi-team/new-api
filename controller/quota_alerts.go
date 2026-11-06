package controller

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/alerting"
)

var userLastDaySummary = make(map[string]int64)
var uidDayAlerted = make(map[string]int64)

type userQuotaMilestoneState = model.LegacyUserQuotaMilestone

var userMilestoneAlerted = make(map[string]userQuotaMilestoneState)

const (
	optKeyUserLastDayMap    = "quota_alert_user_last_day_summary_map"
	optKeyUserMilestone     = "quota_alert_user_cumulative_milestone"
	optKeyUidMap            = "quota_alert_uid_day_map"
	optKeyUidMonth          = "quota_alert_uid_month_threshold_map"
	optKeyUidProject        = "quota_alert_uid_project_threshold_map"
	userMilestoneUSD        = int64(1000)
	previousDaySummaryDelay = 10 * time.Minute
)

func initQuotaAlertStateFromOptions(options map[string]string) error {
	userLastDaySummary, uidDayAlerted = make(map[string]int64), make(map[string]int64)
	userMilestoneAlerted = make(map[string]userQuotaMilestoneState)
	uidMonthThresholdMap, uidProjectThresholdMap = make(map[string]map[string]int64), make(map[string]map[string]int64)
	if v := options[optKeyUserLastDayMap]; v != "" {
		tmp := make(map[string]int64)
		if err := common.Unmarshal([]byte(v), &tmp); err != nil {
			return err
		}
		if len(tmp) > 0 {
			userLastDaySummary = tmp
		}
	}
	if v := options[optKeyUserMilestone]; v != "" {
		tmp := make(map[string]userQuotaMilestoneState)
		if err := common.Unmarshal([]byte(v), &tmp); err != nil {
			return err
		}
		if len(tmp) > 0 {
			userMilestoneAlerted = tmp
		}
	}
	if v := options[optKeyUidMap]; v != "" {
		tmp := make(map[string]int64)
		if err := common.Unmarshal([]byte(v), &tmp); err != nil {
			return err
		}
		if len(tmp) > 0 {
			uidDayAlerted = tmp
		}
	}
	// month threshold map
	if v := options[optKeyUidMonth]; v != "" {
		tmp := make(map[string]map[string]int64)
		if err := common.Unmarshal([]byte(v), &tmp); err != nil {
			return err
		}
		if len(tmp) > 0 {
			uidMonthThresholdMap = tmp
		}
	}
	if v := options[optKeyUidProject]; v != "" {
		tmp := make(map[string]map[string]int64)
		if err := common.Unmarshal([]byte(v), &tmp); err != nil {
			return err
		}
		if len(tmp) > 0 {
			uidProjectThresholdMap = tmp
		}
	}
	return nil
}

func persistUserLastDaySummary(ctx context.Context) error {
	b, err := common.Marshal(userLastDaySummary)
	if err != nil {
		return err
	}
	return model.SaveLegacyAlertOption(ctx, optKeyUserLastDayMap, string(b))
}

func persistUserMilestones(ctx context.Context) error {
	b, err := common.Marshal(userMilestoneAlerted)
	if err != nil {
		return err
	}
	return model.SaveLegacyAlertOption(ctx, optKeyUserMilestone, string(b))
}

func persistUidMap(ctx context.Context) {
	b, _ := common.Marshal(uidDayAlerted)
	_ = model.SaveLegacyAlertOption(ctx, optKeyUidMap, string(b))
}

var uidMonthThresholdMap = make(map[string]map[string]int64)
var uidProjectThresholdMap = make(map[string]map[string]int64)

func pendingUserQuotaMilestone(windowQuota int64, quotaPerUnit float64, alerted userQuotaMilestoneState) int64 {
	if !alerted.Initialized || quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return 0
	}
	deltaQuota := alerted.AccumulatedQuota + windowQuota - alerted.WindowQuota
	if deltaQuota <= 0 {
		return 0
	}

	milestoneQuotaValue := quotaPerUnit * float64(userMilestoneUSD)
	if milestoneQuotaValue <= 0 || milestoneQuotaValue > float64(math.MaxInt64) {
		return 0
	}
	milestoneQuota := int64(math.Ceil(milestoneQuotaValue))
	milestoneCount := deltaQuota / milestoneQuota
	if milestoneCount == 0 {
		return 0
	}
	if milestoneCount > math.MaxInt64/userMilestoneUSD {
		return math.MaxInt64
	}
	milestoneUSD := milestoneCount * userMilestoneUSD
	if milestoneUSD > alerted.MilestoneUSD {
		return milestoneUSD
	}
	return 0
}

func persistUidMonthMap(ctx context.Context) {
	b, _ := common.Marshal(uidMonthThresholdMap)
	_ = model.SaveLegacyAlertOption(ctx, optKeyUidMonth, string(b))
}

func persistUidProjectMap(ctx context.Context) {
	b, _ := common.Marshal(uidProjectThresholdMap)
	_ = model.SaveLegacyAlertOption(ctx, optKeyUidProject, string(b))
}

type uidBudgetAlertPool struct {
	Name      string
	StateKey  string
	PeriodKey string
	PeriodID  int64
	TotalUSD  float64
	UsedUSD   float64
	IsProject bool
}

type uidBudgetAlertMarker struct {
	StateKey  string
	PeriodKey string
	PeriodID  int64
	Threshold string
	IsProject bool
}

func buildUIDBudgetAlert(clientUserId string, pools []uidBudgetAlertPool) (string, []uidBudgetAlertMarker) {
	var lines []string
	var triggers []string
	var markers []uidBudgetAlertMarker
	thresholds := []int{50, 20, 10, 5}

	for _, pool := range pools {
		if pool.TotalUSD <= 0 {
			lines = append(lines, fmt.Sprintf("- %s：未配置预算，已用 $%.2f", pool.Name, pool.UsedUSD))
			continue
		}

		remainingUSD := pool.TotalUSD - pool.UsedUSD
		if remainingUSD < 0 {
			remainingUSD = 0
		}
		usedPct := pool.UsedUSD / pool.TotalUSD * 100
		remainingPct := remainingUSD / pool.TotalUSD * 100
		lines = append(lines, fmt.Sprintf("- %s：已用 $%.2f / $%.2f（%.1f%%），剩余 $%.2f（%.1f%%）", pool.Name, pool.UsedUSD, pool.TotalUSD, usedPct, remainingUSD, remainingPct))

		if pool.StateKey == "" {
			continue
		}
		state := uidMonthThresholdMap
		if pool.IsProject {
			state = uidProjectThresholdMap
		}
		entry := state[pool.StateKey]
		periodMatches := entry != nil && entry[pool.PeriodKey] == pool.PeriodID
		lowestNewThreshold := 101
		for _, threshold := range thresholds {
			thresholdKey := strconv.Itoa(threshold)
			if remainingPct <= float64(threshold) && (!periodMatches || entry[thresholdKey] != pool.PeriodID) {
				markers = append(markers, uidBudgetAlertMarker{
					StateKey:  pool.StateKey,
					PeriodKey: pool.PeriodKey,
					PeriodID:  pool.PeriodID,
					Threshold: thresholdKey,
					IsProject: pool.IsProject,
				})
				if threshold < lowestNewThreshold {
					lowestNewThreshold = threshold
				}
			}
		}
		if lowestNewThreshold <= 100 {
			triggers = append(triggers, fmt.Sprintf("%s 剩余 %.1f%%（达到 <=%d%%）", pool.Name, remainingPct, lowestNewThreshold))
		}
	}

	if len(markers) == 0 {
		return "", nil
	}
	content := fmt.Sprintf(
		"UID预算预警：UID=%s\n触发：%s\n预算使用情况：\n%s",
		clientUserId,
		strings.Join(triggers, "；"),
		strings.Join(lines, "\n"),
	)
	return content, markers
}

func markUIDBudgetAlerted(markers []uidBudgetAlertMarker) (bool, bool) {
	nonProjectDirty, projectDirty := false, false
	for _, marker := range markers {
		state := uidMonthThresholdMap
		if marker.IsProject {
			state = uidProjectThresholdMap
			projectDirty = true
		} else {
			nonProjectDirty = true
		}
		entry := state[marker.StateKey]
		if entry == nil || entry[marker.PeriodKey] != marker.PeriodID {
			entry = map[string]int64{marker.PeriodKey: marker.PeriodID}
			state[marker.StateKey] = entry
		}
		entry[marker.Threshold] = marker.PeriodID
	}
	return nonProjectDirty, projectDirty
}

func FeishuQuotaAlerts() {
	var nextLegacyCheck time.Time
	for {
		delay, due := 5*time.Minute, true
		if common.AlertingEnabled {
			delay = 30 * time.Second
			if time.Now().Before(nextLegacyCheck) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				markers, err := model.UnifiedLegacyQuotaAlerts(ctx, []string{"uid_day", "uid_budget_notice"})
				cancel()
				due = err == nil && len(markers) > 0
				if err != nil {
					common.SysError(common.LogText("legacy quota alert marker lookup failed"))
				}
			}
		}
		if due {
			if err := model.WithLegacyAlertLease(context.Background(), "quota_options", "user", runFeishuQuotaAlertsOnce); err != nil {
				common.SysError(common.LogText("legacy quota alert pass failed"))
			}
			nextLegacyCheck = time.Now().Add(5 * time.Minute)
		}
		// Check takeover promptly, while retaining the old five-minute work
		// cadence until a UID compatibility adapter actually owns delivery.
		time.Sleep(delay)
	}
}

func runFeishuQuotaAlertsOnce(ctx context.Context) error {
	options, err := model.LegacyQuotaAlertOptions(ctx)
	if err != nil {
		return err
	}
	if err := initQuotaAlertStateFromOptions(options); err != nil {
		return err
	}
	unified, err := model.UnifiedLegacyQuotaAlerts(ctx, []string{"user_spend", "user_report", "uid_day", "uid_budget", "project_budget"})
	if err != nil {
		return err
	}
	webhook := options["uid_quota_warning_robot"]
	if webhook == "" {
		return nil
	}

	secret := "" // optional: if you want signature, add another option key

	loc := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(loc)

	// natural day window
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
	dayEnd := dayStart + 86400 - 1

	// Per-user cumulative alert: notify once for every additional $1,000 consumed
	// by that user. The rolling database window is periodically compacted into
	// AccumulatedQuota to bound query cost without mixing consumption across users.
	hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc).Unix()
	windowStart := hourStart - 3600
	for userID, state := range userMilestoneAlerted {
		if unified[model.AlertIdentity("legacy", "user_spend", userID)] {
			continue
		}
		if state.Initialized && state.WindowStart > 0 && state.WindowStart < windowStart {
			windowStart = state.WindowStart
		}
	}
	type userQuotaRow struct {
		UserID      int
		WindowQuota int64
	}
	queryUserWindowQuotas := func(start int64) (map[int]int64, error) {
		var rows []userQuotaRow
		err := model.DB.WithContext(ctx).Table("quota_data").
			Select("user_id, COALESCE(sum(quota),0) AS window_quota").
			Where("user_id > 0 AND created_at >= ?", start).
			Group("user_id").
			Scan(&rows).Error
		quotas := make(map[int]int64, len(rows))
		for _, row := range rows {
			quotas[row.UserID] = row.WindowQuota
		}
		return quotas, err
	}

	userWindowQuotas, milestoneErr := queryUserWindowQuotas(windowStart)
	if milestoneErr != nil {
		common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to query user quota: %s", milestoneErr.Error()))
	} else {
		compacted := false
		compactWindowStart := hourStart - 3600
		if windowStart < compactWindowStart {
			compactedWindowQuotas, err := queryUserWindowQuotas(compactWindowStart)
			if err != nil {
				milestoneErr = err
				common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to compact user quota window: %s", err.Error()))
			} else {
				for userID, state := range userMilestoneAlerted {
					if unified[model.AlertIdentity("legacy", "user_spend", userID)] {
						continue
					}
					id, err := strconv.Atoi(userID)
					if err != nil || !state.Initialized {
						continue
					}
					state.AccumulatedQuota += userWindowQuotas[id] - state.WindowQuota
					state.WindowStart = compactWindowStart
					state.WindowQuota = compactedWindowQuotas[id]
					userMilestoneAlerted[userID] = state
				}
				userWindowQuotas = compactedWindowQuotas
				windowStart = compactWindowStart
				compacted = true
			}
		}

		if milestoneErr == nil {
			dirty := compacted
			initialized := make(map[int]bool)
			for userID, windowQuota := range userWindowQuotas {
				key := strconv.Itoa(userID)
				if unified[model.AlertIdentity("legacy", "user_spend", key)] {
					continue
				}
				state, exists := userMilestoneAlerted[key]
				if exists && state.Initialized {
					continue
				}
				userMilestoneAlerted[key] = userQuotaMilestoneState{Initialized: true, BaselineAt: now.Unix(), WindowStart: windowStart, WindowQuota: windowQuota}
				initialized[userID], dirty = true, true
			}
			// Save new baselines/compaction before any network work. Each later
			// accepted notice saves its checkpoint before the next send, so a
			// bounded pass cannot lose all earlier progress when its time expires.
			if dirty {
				if err := persistUserMilestones(ctx); err != nil {
					return err
				}
			}
			for userID, windowQuota := range userWindowQuotas {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				userIDKey := strconv.Itoa(userID)
				if unified[model.AlertIdentity("legacy", "user_spend", userIDKey)] {
					continue
				}
				if initialized[userID] {
					continue
				}
				state := userMilestoneAlerted[userIDKey]

				milestoneUSD := pendingUserQuotaMilestone(windowQuota, common.QuotaPerUnit, state)
				if milestoneUSD == 0 {
					continue
				}
				addedQuota := state.AccumulatedQuota + windowQuota - state.WindowQuota
				content := fmt.Sprintf(
					"用户消耗预警：UserID=%d 自 %s 起累计新增消耗约 $%.0f，达到 $%d 累计档位（每新增 $1000 告警一次），@管理员",
					userID,
					time.Unix(state.BaselineAt, 0).In(loc).Format("2006-01-02 15:04:05"),
					float64(addedQuota)/common.QuotaPerUnit,
					milestoneUSD,
				)
				if err := service.SendFeishuNotifyContext(ctx, webhook, secret, dto.FeishuNotify{
					MsgType: "text",
					Content: dto.FeishuContent{Text: content},
				}); err != nil {
					common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to notify user %d quota milestone: %s", userID, err.Error()))
					continue
				}
				state.MilestoneUSD = milestoneUSD
				userMilestoneAlerted[userIDKey] = state
				if err := persistUserMilestones(ctx); err != nil {
					return err
				}
			}
		}
	}

	// Push each user's previous natural-day total once.
	previousDayStart := dayStart - 86400
	if now.Unix() >= dayStart+int64(previousDaySummaryDelay/time.Second) {
		var previousDayRows []struct {
			UserID     int
			TotalQuota int64
		}
		err := model.DB.WithContext(ctx).Table("quota_data").
			Select("user_id, COALESCE(sum(quota),0) AS total_quota").
			Where("user_id > 0 AND created_at >= ? AND created_at < ?", previousDayStart, dayStart).
			Group("user_id").
			Scan(&previousDayRows).Error
		if err != nil {
			common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to query previous day user quota: %s", err.Error()))
		} else {
			for _, row := range previousDayRows {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				userIDKey := strconv.Itoa(row.UserID)
				if unified[model.AlertIdentity("legacy", "user_report", userIDKey)] {
					continue
				}
				if userLastDaySummary[userIDKey] == previousDayStart {
					continue
				}
				content := fmt.Sprintf(
					"用户昨日消耗汇总：UserID=%d，%s 总消耗约 $%.2f",
					row.UserID,
					time.Unix(previousDayStart, 0).In(loc).Format("2006-01-02"),
					float64(row.TotalQuota)/common.QuotaPerUnit,
				)
				if err := service.SendFeishuNotifyContext(ctx, webhook, secret, dto.FeishuNotify{
					MsgType: "text",
					Content: dto.FeishuContent{Text: content},
				}); err != nil {
					common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to notify user %d previous day quota: %s", row.UserID, err.Error()))
					continue
				}
				userLastDaySummary[userIDKey] = previousDayStart
				if err := persistUserLastDaySummary(ctx); err != nil {
					common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to persist user %d previous day summary: %s", row.UserID, err.Error()))
				}
			}
		}
	}

	// uid day alert: single uid > $10000
	var rows []struct {
		ClientUserId string
		TotalQuota   int
	}
	err = model.DB.WithContext(ctx).Table("quota_data").
		Select("client_user_id, COALESCE(sum(quota),0) as total_quota").
		Where("created_at >= ? AND created_at <= ?", dayStart, dayEnd).
		Group("client_user_id").
		Scan(&rows).Error
	if err == nil {
		for _, r := range rows {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.ClientUserId == "" || unified[model.AlertIdentity("legacy", "uid_day", r.ClientUserId)] {
				continue
			}
			usd := float64(r.TotalQuota) / common.QuotaPerUnit
			if usd >= 10000 {
				if uidDayAlerted[r.ClientUserId] != dayStart {
					content := fmt.Sprintf("UID消耗预警：UID=%s 当日消耗约 $%.0f，阈值 $10000，@管理员", r.ClientUserId, usd)
					if err := alerting.DispatchLegacyNotice(ctx, "uid_day", "site", model.AlertIdentity(r.ClientUserId, dayStart), r.ClientUserId, alerting.LegacyNotice{Title: i18n.Translate("", "alert.legacy_uid_day"), Content: content}, func(ctx context.Context) error {
						return service.SendFeishuNotifyContext(ctx, webhook, secret, dto.FeishuNotify{MsgType: "text", Content: dto.FeishuContent{Text: content}})
					}); err != nil {
						continue
					}

					uidDayAlerted[r.ClientUserId] = dayStart
					persistUidMap(ctx)
				}
			}
		}
	}

	sendUIDBudgetAlerts(ctx, webhook, secret, now, options)

	return ctx.Err()
}

func sendUIDBudgetAlerts(ctx context.Context, webhook string, secret string, now time.Time, options map[string]string) {
	type clientQuotaRow struct {
		ClientUserId string
		FixedQuota   int
		TempQuota    int
		ExpiredAt    int64
	}
	var quotaRows []clientQuotaRow
	if err := model.DB.WithContext(ctx).Model(&model.CliendUserQuota{}).
		Select("client_user_id, fixed_quota, temp_quota, expired_at").
		Scan(&quotaRows).Error; err != nil {
		common.SysError(fmt.Sprintf("sendUIDBudgetAlerts: failed to query UID budgets: %s", err.Error()))
		return
	}

	clientUserIdSet := make(map[string]struct{}, len(quotaRows))
	quotaByClientUserId := make(map[string]clientQuotaRow, len(quotaRows))
	for _, row := range quotaRows {
		if row.ClientUserId == "" {
			continue
		}
		clientUserIdSet[row.ClientUserId] = struct{}{}
		quotaByClientUserId[row.ClientUserId] = row
	}
	var projectClientUserIds []string
	if err := model.DB.WithContext(ctx).Model(&model.ProjectAllocation{}).
		Where("client_user_id <> ?", "").
		Distinct("client_user_id").
		Pluck("client_user_id", &projectClientUserIds).Error; err != nil {
		common.SysError(fmt.Sprintf("sendUIDBudgetAlerts: failed to query project UID allocations: %s", err.Error()))
		return
	}
	for _, clientUserId := range projectClientUserIds {
		clientUserIdSet[clientUserId] = struct{}{}
	}
	clientUserIds := make([]string, 0, len(clientUserIdSet))
	for clientUserId := range clientUserIdSet {
		clientUserIds = append(clientUserIds, clientUserId)
	}
	if len(clientUserIds) == 0 {
		return
	}
	sort.Strings(clientUserIds)

	const summaryBatchSize = 500
	summaries := make(map[string]*model.ProjectBudgetSummary, len(clientUserIds))
	for start := 0; start < len(clientUserIds); start += summaryBatchSize {
		end := min(start+summaryBatchSize, len(clientUserIds))
		batchSummaries, err := model.GetBatchProjectBudgetSummary(clientUserIds[start:end])
		if err != nil {
			common.SysError(fmt.Sprintf("sendUIDBudgetAlerts: failed to query UID budget usage: %s", err.Error()))
			return
		}
		for clientUserId, summary := range batchSummaries {
			summaries[clientUserId] = summary
		}
	}
	monthStart := common.BillingMonthStartUnix(now.Unix())
	emailRecipients := options["uid_quota_warning_email"]
	for _, clientUserId := range clientUserIds {
		if ctx.Err() != nil {
			return
		}
		summary := summaries[clientUserId]
		if summary == nil {
			continue
		}

		quotaRow, hasNonProjectBudget := quotaByClientUserId[clientUserId]
		nonProjectBudgetUSD := quotaRow.FixedQuota
		if quotaRow.ExpiredAt == 0 || quotaRow.ExpiredAt > now.Unix() {
			nonProjectBudgetUSD += quotaRow.TempQuota
		}
		nonProjectStateKey := ""
		if hasNonProjectBudget && nonProjectBudgetUSD > 0 {
			nonProjectStateKey = clientUserId
		}
		pools := []uidBudgetAlertPool{{
			Name:      "非项目预算（本月）",
			StateKey:  nonProjectStateKey,
			PeriodKey: "month_start",
			PeriodID:  monthStart,
			TotalUSD:  float64(nonProjectBudgetUSD),
			UsedUSD:   summary.MonthlyNonProjectUsedUSD,
		}}
		for _, project := range summary.Projects {
			pools = append(pools, uidBudgetAlertPool{
				Name:      fmt.Sprintf("项目「%s」/ 方案「%s」（%s-%s）", project.ProjectName, project.PlanName, project.StartDate, project.EndDate),
				StateKey:  fmt.Sprintf("%s|%d", clientUserId, project.ProjectId),
				PeriodKey: "plan_id",
				PeriodID:  int64(project.PlanId),
				TotalUSD:  float64(project.AllocatedQuota),
				UsedUSD:   project.UsedQuotaUSD,
				IsProject: true,
			})
		}
		if len(summary.Projects) == 0 {
			pools = append(pools, uidBudgetAlertPool{Name: "项目预算（当前无生效分配）"})
		}

		content, markers := buildUIDBudgetAlert(clientUserId, pools)
		if content == "" {
			continue
		}
		if err := alerting.DispatchLegacyNotice(ctx, "uid_budget_notice", "site", model.AlertIdentity(clientUserId, markers), clientUserId, alerting.LegacyNotice{Title: i18n.Translate("", "alert.legacy_uid_budget"), Content: content}, func(ctx context.Context) error {
			if err := service.SendFeishuNotifyContext(ctx, webhook, secret, dto.FeishuNotify{MsgType: "text", Content: dto.FeishuContent{Text: content}}); err != nil {
				return err
			}
			if emailRecipients != "" {
				_ = common.SendEmailContext(ctx, "UID预算预警", emailRecipients, content)
			}
			return nil
		}); err != nil {
			common.SysError(fmt.Sprintf("sendUIDBudgetAlerts: failed to notify UID %s: %s", clientUserId, err.Error()))
			continue
		}
		uidDirty, uidProjectDirty := markUIDBudgetAlerted(markers)
		if uidDirty {
			persistUidMonthMap(ctx)
		}
		if uidProjectDirty {
			persistUidProjectMap(ctx)
		}
	}
}

// KeyQuotaRule defines a single threshold rule for key-level quota alerts
type KeyQuotaRule = model.LegacyKeyQuotaRule

type KeyQuotaUserConfig = model.LegacyKeyQuotaUserConfig

const optKeyKeyQuotaConfig = "key_quota_warning_config"
const optKeyKeyQuotaAlerted = "key_quota_warning_alerted"

// keyQuotaAlertedState tracks which (tokenId, ruleKey) combos have already been alerted
// format: map[tokenId_string] -> map[ruleKey] -> windowStart
var keyQuotaAlertedState = make(map[string]map[string]int64)

func loadKeyQuotaAlertedState(options map[string]string) error {
	keyQuotaAlertedState = make(map[string]map[string]int64)
	if v := options[optKeyKeyQuotaAlerted]; v != "" {
		tmp := make(map[string]map[string]int64)
		if err := common.Unmarshal([]byte(v), &tmp); err != nil {
			return err
		}
		if len(tmp) > 0 {
			keyQuotaAlertedState = tmp
		}
	}
	return nil
}

func persistKeyQuotaAlertedState(ctx context.Context) {
	b, _ := common.Marshal(keyQuotaAlertedState)
	_ = model.SaveLegacyAlertOption(ctx, optKeyKeyQuotaAlerted, string(b))
}

func isKeyQuotaAlerted(tokenIdStr string, ruleKey string, windowStart int64) bool {
	entry, ok := keyQuotaAlertedState[tokenIdStr]
	if !ok {
		return false
	}
	return entry[ruleKey] == windowStart
}

func markKeyQuotaAlerted(tokenIdStr string, ruleKey string, windowStart int64) {
	if _, ok := keyQuotaAlertedState[tokenIdStr]; !ok {
		keyQuotaAlertedState[tokenIdStr] = make(map[string]int64)
	}
	keyQuotaAlertedState[tokenIdStr][ruleKey] = windowStart
}

func FeishuQuotaKeyAlerts() {
	for {
		if err := model.WithLegacyAlertLease(context.Background(), "quota_options", "key", runFeishuQuotaKeyAlertsOnce); err != nil {
			common.SysError(common.LogText("legacy key quota alert pass failed"))
		}
		time.Sleep(5 * time.Minute)
	}
}

func runFeishuQuotaKeyAlertsOnce(ctx context.Context) error {
	options, err := model.LegacyQuotaAlertOptions(ctx)
	if err != nil {
		return err
	}
	if err := loadKeyQuotaAlertedState(options); err != nil {
		return err
	}
	unified, err := model.UnifiedLegacyQuotaAlerts(ctx, []string{"key_hour", "key_day"})
	if err != nil {
		return err
	}
	configStr := options[optKeyKeyQuotaConfig]
	if configStr == "" {
		return nil
	}

	// Parse config: map[user_id_string] -> KeyQuotaUserConfig
	userConfigs := make(map[string]*KeyQuotaUserConfig)
	if err := common.Unmarshal([]byte(configStr), &userConfigs); err != nil {
		common.SysError("FeishuQuotaKeyAlerts: failed to parse config: " + err.Error())
		return err
	}

	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)

	hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc).Unix()
	hourEnd := hourStart + 3600 - 1
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
	dayEnd := dayStart + 86400 - 1

	for userIdStr, cfg := range userConfigs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cfg == nil {
			continue
		}
		if cfg.FeishuRobotURL == "" {
			continue
		}

		userId, err := strconv.Atoi(userIdStr)
		if err != nil {
			continue
		}

		// Process one_hour rules
		if len(cfg.OneHour) > 0 {
			var hourRows []struct {
				TokenId    int
				TotalQuota int
			}
			err := model.DB.WithContext(ctx).Table("quota_data").
				Select("token_id, COALESCE(sum(quota),0) as total_quota").
				Where("user_id = ? AND created_at >= ? AND created_at <= ? AND token_id > 0", userId, hourStart, hourEnd).
				Group("token_id").
				Scan(&hourRows).Error
			if err == nil {
				for _, row := range hourRows {
					usd := float64(row.TotalQuota) / common.QuotaPerUnit
					tokenIdStr := strconv.Itoa(row.TokenId)
					for _, rule := range cfg.OneHour {
						if unified[model.AlertIdentity("legacy", "key_hour", model.LegacyKeyQuotaObject(userId, "hour", rule))] {
							continue
						}
						if usd >= rule.QuotaUSD {
							ruleKey := fmt.Sprintf("1h_%d_%d_%s", hourStart, hourEnd, rule.Action)
							if !isKeyQuotaAlerted(tokenIdStr, ruleKey, hourStart) {
								if err := processKeyQuotaAction(ctx, cfg.FeishuRobotURL, rule.Action, row.TokenId, usd, rule.QuotaUSD, "1小时"); err != nil {
									continue
								}
								markKeyQuotaAlerted(tokenIdStr, ruleKey, hourStart)
								persistKeyQuotaAlertedState(ctx)
							}
						}
					}
				}
			}
		}

		// Process one_day rules
		if len(cfg.OneDay) > 0 {
			var dayRows []struct {
				TokenId    int
				TotalQuota int
			}
			err := model.DB.WithContext(ctx).Table("quota_data").
				Select("token_id, COALESCE(sum(quota),0) as total_quota").
				Where("user_id = ? AND created_at >= ? AND created_at <= ? AND token_id > 0", userId, dayStart, dayEnd).
				Group("token_id").
				Scan(&dayRows).Error
			if err == nil {
				for _, row := range dayRows {
					usd := float64(row.TotalQuota) / common.QuotaPerUnit
					tokenIdStr := strconv.Itoa(row.TokenId)
					for _, rule := range cfg.OneDay {
						if unified[model.AlertIdentity("legacy", "key_day", model.LegacyKeyQuotaObject(userId, "day", rule))] {
							continue
						}
						if usd >= rule.QuotaUSD {
							ruleKey := fmt.Sprintf("1d_%d_%d_%s", dayStart, dayEnd, rule.Action)
							if !isKeyQuotaAlerted(tokenIdStr, ruleKey, dayStart) {
								if err := processKeyQuotaAction(ctx, cfg.FeishuRobotURL, rule.Action, row.TokenId, usd, rule.QuotaUSD, "当日"); err != nil {
									continue
								}
								markKeyQuotaAlerted(tokenIdStr, ruleKey, dayStart)
								persistKeyQuotaAlertedState(ctx)
							}
						}
					}
				}
			}
		}
	}

	return ctx.Err()
}

// A failed delivery remains eligible for retry. Disabling is idempotent and
// advances the credential version so cached credentials cannot remain usable.
func processKeyQuotaAction(ctx context.Context, webhook, action string, tokenId int, currentUSD, thresholdUSD float64, window string) error {
	token := &model.Token{}
	if err := model.DB.WithContext(ctx).First(token, tokenId).Error; err != nil {
		return err
	}
	tokenName := token.Name
	if tokenName == "" {
		tokenName = fmt.Sprintf("ID:%d", tokenId)
	}
	content := ""
	switch action {
	case "warn":
		content = fmt.Sprintf("Key消耗预警：Key[%s] %s消耗约 $%.2f，阈值 $%.0f", tokenName, window, currentUSD, thresholdUSD)
	case "disable_key":
		if err := model.DisableLegacyQuotaToken(ctx, tokenId); err != nil {
			return err
		}
		content = fmt.Sprintf("Key已停用：Key[%s] %s消耗约 $%.2f，阈值 $%.0f，已自动停用", tokenName, window, currentUSD, thresholdUSD)
	default:
		return fmt.Errorf("unsupported key quota action")
	}
	if err := service.SendFeishuNotifyContext(ctx, webhook, "", dto.FeishuNotify{MsgType: "text", Content: dto.FeishuContent{Text: content}}); err != nil {
		return err
	}
	common.SysLog(content)
	return nil
}
