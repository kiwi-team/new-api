package controller

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

var userLastDaySummary = make(map[string]int64)
var uidDayAlerted = make(map[string]int64)

type userQuotaMilestoneState struct {
	Initialized      bool  `json:"initialized"`
	BaselineAt       int64 `json:"baseline_at"`
	WindowStart      int64 `json:"window_start"`
	WindowQuota      int64 `json:"window_quota"`
	AccumulatedQuota int64 `json:"accumulated_quota"`
	MilestoneUSD     int64 `json:"milestone_usd"`
}

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

func initQuotaAlertStateFromOptions() {
	if v := common.OptionMap[optKeyUserLastDayMap]; v != "" {
		tmp := make(map[string]int64)
		_ = common.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			userLastDaySummary = tmp
		}
	}
	if v := common.OptionMap[optKeyUserMilestone]; v != "" {
		tmp := make(map[string]userQuotaMilestoneState)
		_ = common.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			userMilestoneAlerted = tmp
		}
	}
	if v := common.OptionMap[optKeyUidMap]; v != "" {
		tmp := make(map[string]int64)
		_ = common.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			uidDayAlerted = tmp
		}
	}
	// month threshold map
	if v := common.OptionMap[optKeyUidMonth]; v != "" {
		tmp := make(map[string]map[string]int64)
		_ = common.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			uidMonthThresholdMap = tmp
		}
	}
	if v := common.OptionMap[optKeyUidProject]; v != "" {
		tmp := make(map[string]map[string]int64)
		_ = common.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			uidProjectThresholdMap = tmp
		}
	}
}

func persistUserLastDaySummary() error {
	b, err := common.Marshal(userLastDaySummary)
	if err != nil {
		return err
	}
	return model.UpdateOption(optKeyUserLastDayMap, string(b))
}

func persistUserMilestones() error {
	b, err := common.Marshal(userMilestoneAlerted)
	if err != nil {
		return err
	}
	return model.UpdateOption(optKeyUserMilestone, string(b))
}

func persistUidMap() {
	b, _ := common.Marshal(uidDayAlerted)
	_ = model.UpdateOption(optKeyUidMap, string(b))
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

func persistUidMonthMap() {
	b, _ := common.Marshal(uidMonthThresholdMap)
	_ = model.UpdateOption(optKeyUidMonth, string(b))
}

func persistUidProjectMap() {
	b, _ := common.Marshal(uidProjectThresholdMap)
	_ = model.UpdateOption(optKeyUidProject, string(b))
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
	nonProjectDirty := false
	projectDirty := false
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
	initQuotaAlertStateFromOptions()
	for {
		webhook := common.OptionMap["uid_quota_warning_robot"]
		if webhook == "" {
			time.Sleep(time.Minute * 1)
			continue
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
		for _, state := range userMilestoneAlerted {
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
			err := model.DB.Table("quota_data").
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
				for userID, windowQuota := range userWindowQuotas {
					userIDKey := strconv.Itoa(userID)
					state, exists := userMilestoneAlerted[userIDKey]
					if !exists || !state.Initialized {
						userMilestoneAlerted[userIDKey] = userQuotaMilestoneState{
							Initialized: true,
							BaselineAt:  now.Unix(),
							WindowStart: windowStart,
							WindowQuota: windowQuota,
						}
						dirty = true
						continue
					}

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
					if err := service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
						MsgType: "text",
						Content: dto.FeishuContent{Text: content},
					}); err != nil {
						common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to notify user %d quota milestone: %s", userID, err.Error()))
						continue
					}
					state.MilestoneUSD = milestoneUSD
					userMilestoneAlerted[userIDKey] = state
					dirty = true
				}
				if dirty {
					if err := persistUserMilestones(); err != nil {
						common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to persist user quota milestones: %s", err.Error()))
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
			err := model.DB.Table("quota_data").
				Select("user_id, COALESCE(sum(quota),0) AS total_quota").
				Where("user_id > 0 AND created_at >= ? AND created_at < ?", previousDayStart, dayStart).
				Group("user_id").
				Scan(&previousDayRows).Error
			if err != nil {
				common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to query previous day user quota: %s", err.Error()))
			} else {
				for _, row := range previousDayRows {
					userIDKey := strconv.Itoa(row.UserID)
					if userLastDaySummary[userIDKey] == previousDayStart {
						continue
					}
					content := fmt.Sprintf(
						"用户昨日消耗汇总：UserID=%d，%s 总消耗约 $%.2f",
						row.UserID,
						time.Unix(previousDayStart, 0).In(loc).Format("2006-01-02"),
						float64(row.TotalQuota)/common.QuotaPerUnit,
					)
					if err := service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
						MsgType: "text",
						Content: dto.FeishuContent{Text: content},
					}); err != nil {
						common.SysError(fmt.Sprintf("FeishuQuotaAlerts: failed to notify user %d previous day quota: %s", row.UserID, err.Error()))
						continue
					}
					userLastDaySummary[userIDKey] = previousDayStart
					if err := persistUserLastDaySummary(); err != nil {
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
		err := model.DB.Table("quota_data").
			Select("client_user_id, COALESCE(sum(quota),0) as total_quota").
			Where("created_at >= ? AND created_at <= ?", dayStart, dayEnd).
			Group("client_user_id").
			Scan(&rows).Error
		if err == nil {
			for _, r := range rows {
				if r.ClientUserId == "" {
					continue
				}
				usd := float64(r.TotalQuota) / common.QuotaPerUnit
				if usd >= 10000 {
					if uidDayAlerted[r.ClientUserId] != dayStart {
						content := fmt.Sprintf("UID消耗预警：UID=%s 当日消耗约 $%.0f，阈值 $10000，@管理员", r.ClientUserId, usd)
						_ = service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
							MsgType: "text",
							Content: dto.FeishuContent{Text: content},
						})
						uidDayAlerted[r.ClientUserId] = dayStart
						persistUidMap()
					}
				}
			}
		}

		sendUIDBudgetAlerts(webhook, secret, now)

		time.Sleep(time.Minute * 5)
	}
}

func sendUIDBudgetAlerts(webhook string, secret string, now time.Time) {
	type clientQuotaRow struct {
		ClientUserId string
		FixedQuota   int
		TempQuota    int
		ExpiredAt    int64
	}
	var quotaRows []clientQuotaRow
	if err := model.DB.Model(&model.CliendUserQuota{}).
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
	if err := model.DB.Model(&model.ProjectAllocation{}).
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
	emailRecipients := common.OptionMap["uid_quota_warning_email"]
	nonProjectDirty := false
	projectDirty := false
	for _, clientUserId := range clientUserIds {
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
		if err := service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
			MsgType: "text",
			Content: dto.FeishuContent{Text: content},
		}); err != nil {
			common.SysError(fmt.Sprintf("sendUIDBudgetAlerts: failed to notify UID %s: %s", clientUserId, err.Error()))
			continue
		}
		if emailRecipients != "" {
			_ = common.SendEmail(emailRecipients, "UID预算预警", content)
		}
		uidDirty, uidProjectDirty := markUIDBudgetAlerted(markers)
		nonProjectDirty = nonProjectDirty || uidDirty
		projectDirty = projectDirty || uidProjectDirty
	}
	if nonProjectDirty {
		persistUidMonthMap()
	}
	if projectDirty {
		persistUidProjectMap()
	}
}

// KeyQuotaRule defines a single threshold rule for key-level quota alerts
type KeyQuotaRule struct {
	QuotaUSD float64 `json:"quota_usd"`
	Action   string  `json:"action"` // "warn" or "disable_key"
}

// KeyQuotaUserConfig defines per-user key quota alert configuration
type KeyQuotaUserConfig struct {
	FeishuRobotURL string         `json:"feishu_robot_url"`
	OneHour        []KeyQuotaRule `json:"one_hour"`
	OneDay         []KeyQuotaRule `json:"one_day"`
}

const optKeyKeyQuotaConfig = "key_quota_warning_config"
const optKeyKeyQuotaAlerted = "key_quota_warning_alerted"

// keyQuotaAlertedState tracks which (tokenId, ruleKey) combos have already been alerted
// format: map[tokenId_string] -> map[ruleKey] -> windowStart
var keyQuotaAlertedState = make(map[string]map[string]int64)

func loadKeyQuotaAlertedState() {
	if v := common.OptionMap[optKeyKeyQuotaAlerted]; v != "" {
		tmp := make(map[string]map[string]int64)
		_ = common.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			keyQuotaAlertedState = tmp
		}
	}
}

func persistKeyQuotaAlertedState() {
	b, _ := common.Marshal(keyQuotaAlertedState)
	_ = model.UpdateOption(optKeyKeyQuotaAlerted, string(b))
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
	loadKeyQuotaAlertedState()
	for {
		// Read config from options
		configStr := common.OptionMap[optKeyKeyQuotaConfig]
		if configStr == "" {
			time.Sleep(time.Minute * 5)
			continue
		}

		// Parse config: map[user_id_string] -> KeyQuotaUserConfig
		userConfigs := make(map[string]*KeyQuotaUserConfig)
		if err := common.Unmarshal([]byte(configStr), &userConfigs); err != nil {
			common.SysError("FeishuQuotaKeyAlerts: failed to parse config: " + err.Error())
			time.Sleep(time.Minute * 5)
			continue
		}

		loc, _ := time.LoadLocation("Asia/Shanghai")
		now := time.Now().In(loc)

		hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc).Unix()
		hourEnd := hourStart + 3600 - 1
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
		dayEnd := dayStart + 86400 - 1

		dirty := false

		for userIdStr, cfg := range userConfigs {
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
				err := model.DB.Table("quota_data").
					Select("token_id, COALESCE(sum(quota),0) as total_quota").
					Where("user_id = ? AND created_at >= ? AND created_at <= ? AND token_id > 0", userId, hourStart, hourEnd).
					Group("token_id").
					Scan(&hourRows).Error
				if err == nil {
					for _, row := range hourRows {
						usd := float64(row.TotalQuota) / common.QuotaPerUnit
						tokenIdStr := strconv.Itoa(row.TokenId)
						for _, rule := range cfg.OneHour {
							if usd >= rule.QuotaUSD {
								ruleKey := fmt.Sprintf("1h_%d_%d_%s", hourStart, hourEnd, rule.Action)
								if !isKeyQuotaAlerted(tokenIdStr, ruleKey, hourStart) {
									processKeyQuotaAction(cfg.FeishuRobotURL, rule.Action, row.TokenId, usd, rule.QuotaUSD, "1小时", now)
									markKeyQuotaAlerted(tokenIdStr, ruleKey, hourStart)
									dirty = true
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
				err := model.DB.Table("quota_data").
					Select("token_id, COALESCE(sum(quota),0) as total_quota").
					Where("user_id = ? AND created_at >= ? AND created_at <= ? AND token_id > 0", userId, dayStart, dayEnd).
					Group("token_id").
					Scan(&dayRows).Error
				if err == nil {
					for _, row := range dayRows {
						usd := float64(row.TotalQuota) / common.QuotaPerUnit
						tokenIdStr := strconv.Itoa(row.TokenId)
						for _, rule := range cfg.OneDay {
							if usd >= rule.QuotaUSD {
								ruleKey := fmt.Sprintf("1d_%d_%d_%s", dayStart, dayEnd, rule.Action)
								if !isKeyQuotaAlerted(tokenIdStr, ruleKey, dayStart) {
									processKeyQuotaAction(cfg.FeishuRobotURL, rule.Action, row.TokenId, usd, rule.QuotaUSD, "当日", now)
									markKeyQuotaAlerted(tokenIdStr, ruleKey, dayStart)
									dirty = true
								}
							}
						}
					}
				}
			}
		}

		if dirty {
			persistKeyQuotaAlertedState()
		}

		time.Sleep(time.Minute * 5)
	}
}

func processKeyQuotaAction(webhook string, action string, tokenId int, currentUSD float64, thresholdUSD float64, window string, now time.Time) {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		common.SysError(fmt.Sprintf("FeishuQuotaKeyAlerts: failed to get token %d: %s", tokenId, err.Error()))
		return
	}

	tokenName := token.Name
	if tokenName == "" {
		tokenName = fmt.Sprintf("ID:%d", tokenId)
	}

	switch action {
	case "warn":
		content := fmt.Sprintf("Key消耗预警：Key[%s] %s消耗约 $%.2f，阈值 $%.0f", tokenName, window, currentUSD, thresholdUSD)
		_ = service.SendFeishuNotify(webhook, "", dto.FeishuNotify{
			MsgType: "text",
			Content: dto.FeishuContent{Text: content},
		})
		common.SysLog(content)

	case "disable_key":
		// Disable the token
		err := model.DB.Model(&model.Token{}).Where("id = ?", tokenId).Update("status", common.TokenStatusDisabled).Error
		if err != nil {
			common.SysError(fmt.Sprintf("FeishuQuotaKeyAlerts: failed to disable token %d: %s", tokenId, err.Error()))
			return
		}
		content := fmt.Sprintf("Key已停用：Key[%s] %s消耗约 $%.2f，阈值 $%.0f，已自动停用", tokenName, window, currentUSD, thresholdUSD)
		_ = service.SendFeishuNotify(webhook, "", dto.FeishuNotify{
			MsgType: "text",
			Content: dto.FeishuContent{Text: content},
		})
		common.SysLog(content)
	}
}
