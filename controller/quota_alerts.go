package controller

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

var lastHourAlerted int64
var lastDayAlerted int64
var uidDayAlerted = make(map[string]int64)

const (
	optKeyLastHour = "quota_alert_last_hour"
	optKeyLastDay  = "quota_alert_last_day"
	optKeyUidMap   = "quota_alert_uid_day_map"
	optKeyUidMonth = "quota_alert_uid_month_threshold_map"
)

func initQuotaAlertStateFromOptions() {
	if v := common.OptionMap[optKeyLastHour]; v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			lastHourAlerted = i
		}
	}
	if v := common.OptionMap[optKeyLastDay]; v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			lastDayAlerted = i
		}
	}
	if v := common.OptionMap[optKeyUidMap]; v != "" {
		tmp := make(map[string]int64)
		_ = json.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			uidDayAlerted = tmp
		}
	}
	// month threshold map
	if v := common.OptionMap[optKeyUidMonth]; v != "" {
		tmp := make(map[string]map[string]int64)
		_ = json.Unmarshal([]byte(v), &tmp)
		if len(tmp) > 0 {
			uidMonthThresholdMap = tmp
		}
	}
}

func persistLastHour(hourStart int64) {
	_ = model.UpdateOption(optKeyLastHour, strconv.FormatInt(hourStart, 10))
}
func persistLastDay(dayStart int64) {
	_ = model.UpdateOption(optKeyLastDay, strconv.FormatInt(dayStart, 10))
}
func persistUidMap() {
	b, _ := json.Marshal(uidDayAlerted)
	_ = model.UpdateOption(optKeyUidMap, string(b))
}

var uidMonthThresholdMap = make(map[string]map[string]int64)

func persistUidMonthMap() {
	b, _ := json.Marshal(uidMonthThresholdMap)
	_ = model.UpdateOption(optKeyUidMonth, string(b))
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

		loc, _ := time.LoadLocation("Asia/Shanghai")
		now := time.Now().In(loc)

		// natural hour window
		hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc).Unix()
		hourEnd := hourStart + 3600 - 1

		// natural day window
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
		dayEnd := dayStart + 86400 - 1
		// natural month window
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Unix()
		nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc).Unix()
		monthEnd := nextMonth - 1

		// platform hour alert: > $5000
		if lastHourAlerted != hourStart {
			var hourSum int
			err := model.DB.Table("quota_data").Select("COALESCE(sum(quota),0)").Where("created_at >= ? AND created_at <= ?", hourStart, hourEnd).Scan(&hourSum).Error
			if err == nil {
				usd := float64(hourSum) / common.QuotaPerUnit
				if usd >= 5000 {
					content := fmt.Sprintf("平台整体消耗预警：%s 内消耗约 $%.0f，阈值 $5000，@管理员", now.Format("2006-01-02 15:04"))
					_ = service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
						MsgType: "text",
						Content: dto.FeishuContent{Text: content},
					})
					lastHourAlerted = hourStart
					persistLastHour(hourStart)
				}
			}
		}

		// platform day alert: > $20000
		if lastDayAlerted != dayStart {
			var daySum int
			err := model.DB.Table("quota_data").Select("COALESCE(sum(quota),0)").Where("created_at >= ? AND created_at <= ?", dayStart, dayEnd).Scan(&daySum).Error
			if err == nil {
				usd := float64(daySum) / common.QuotaPerUnit
				if usd >= 20000 {
					content := fmt.Sprintf("平台整体消耗预警：%s 当日消耗约 $%.0f，阈值 $20000，@管理员", now.Format("2006-01-02"), usd)
					_ = service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
						MsgType: "text",
						Content: dto.FeishuContent{Text: content},
					})
					lastDayAlerted = dayStart
					persistLastDay(dayStart)
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

		// uid month threshold alerts: remaining percentage below 50/20/10/5
		var cuRows []struct {
			ClientUserId string
			FixedQuota   int
			TempQuota    int
		}
		_ = model.DB.Table("cliend_user_quota").Select("client_user_id, fixed_quota, temp_quota").Scan(&cuRows).Error
		emailRecipients := common.OptionMap["uid_quota_warning_email"] // optional, semicolon-separated
		for _, cu := range cuRows {
			if cu.ClientUserId == "" {
				continue
			}
			totalBudget := cu.FixedQuota + cu.TempQuota
			if totalBudget <= 0 {
				continue
			}
			var monthSum int
			err := model.DB.Table("quota_data").Select("COALESCE(sum(quota),0)").Where("client_user_id = ? AND created_at >= ? AND created_at <= ?", cu.ClientUserId, monthStart, monthEnd).Scan(&monthSum).Error
			if err != nil {
				continue
			}
			remain := totalBudget - int(float64(monthSum)/common.QuotaPerUnit)
			if remain < 0 {
				remain = 0
			}
			remainPct := float64(remain) / float64(totalBudget)
			thresholds := []float64{0.5, 0.2, 0.1, 0.05}
			for _, th := range thresholds {
				key := fmt.Sprintf("%.0f", th*100)
				entry, ok := uidMonthThresholdMap[cu.ClientUserId]
				if !ok || entry["month_start"] != monthStart {
					uidMonthThresholdMap[cu.ClientUserId] = map[string]int64{
						"month_start": monthStart,
					}
					entry = uidMonthThresholdMap[cu.ClientUserId]
				}
				if remainPct <= th && entry[key] != monthStart {
					content := fmt.Sprintf("UID预算预警：UID=%s 本月已消耗 %f，剩余预算 %d（总预算 %d，<=%s%%）", cu.ClientUserId, float64(monthSum)/common.QuotaPerUnit, remain, totalBudget, key)
					_ = service.SendFeishuNotify(webhook, secret, dto.FeishuNotify{
						MsgType: "text",
						Content: dto.FeishuContent{Text: content},
					})
					// optional email
					if emailRecipients != "" {
						_ = common.SendEmail(emailRecipients, "UID预算预警", content)
					}
					uidMonthThresholdMap[cu.ClientUserId][key] = monthStart
					persistUidMonthMap()
				}
			}
		}

		time.Sleep(time.Minute * 5)
	}
}
