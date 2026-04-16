package controller

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// TokenAlertCheckInterval 每轮检查的间隔
const TokenAlertCheckInterval = 2 * time.Minute

// TokenKeyAlertLoop 后台循环：按 key(令牌) 维度进行消耗告警。
//
// 逻辑：
//  1. 遍历所有 alert_threshold > 0 的令牌
//  2. 从 quota_data 表中聚合该令牌累计消耗
//  3. 若本次聚合值 - 上次告警基线 >= alert_threshold * QuotaPerUnit，
//     则通过用户 UserSetting.WebhookUrl（飞书机器人地址）发送告警
//  4. 告警发送后把基线更新为当前聚合值，实现“每消耗 N 美元告警一次”
//
// 消耗数据来源于 quota_data，内存缓存未落库部分会在下一轮补齐，
// 因此允许存在一定的延迟 / 误差（例如阈值 $100 时可能在 $103 才触发）。
func TokenKeyAlertLoop() {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("TokenKeyAlertLoop panic: %v", r))
		}
	}()
	for {
		runTokenKeyAlertOnce()
		time.Sleep(TokenAlertCheckInterval)
	}
}

func runTokenKeyAlertOnce() {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("runTokenKeyAlertOnce panic: %v", r))
		}
	}()

	var tokens []model.Token
	if err := model.DB.
		Select("id, user_id, name, created_time, alert_threshold, alert_notified_quota, alert_last_notified_time").
		Where("alert_threshold > 0").
		Find(&tokens).Error; err != nil {
		common.SysError("TokenKeyAlert: load tokens failed: " + err.Error())
		return
	}
	if len(tokens) == 0 {
		return
	}

	// 用户设置按 user_id 缓存，避免重复读
	userSettingCache := make(map[int]dto.UserSetting)
	now := time.Now().Unix()

	for _, t := range tokens {
		if t.AlertThreshold <= 0 {
			continue
		}
		thresholdQuota := int(t.AlertThreshold * common.QuotaPerUnit)
		if thresholdQuota <= 0 {
			continue
		}

		var currentUsed int
		if err := model.DB.Table("quota_data").
			Select("COALESCE(sum(quota),0)").
			Where("token_id = ?", t.Id).
			Scan(&currentUsed).Error; err != nil {
			common.SysError(fmt.Sprintf("TokenKeyAlert: sum quota failed token=%d: %s", t.Id, err.Error()))
			continue
		}

		delta := currentUsed - t.AlertNotifiedQuota
		if delta < thresholdQuota {
			continue
		}

		// 获取用户 webhook
		setting, ok := userSettingCache[t.UserId]
		if !ok {
			s, err := model.GetUserSetting(t.UserId, true)
			if err != nil {
				common.SysError(fmt.Sprintf("TokenKeyAlert: get user setting failed user=%d: %s", t.UserId, err.Error()))
				// 避免没拿到设置时反复刷阈值 —— 先跳过本轮
				continue
			}
			userSettingCache[t.UserId] = s
			setting = s
		}
		webhook := setting.WebhookUrl
		if webhook == "" {
			// 用户未配置 webhook：仅将基线前移，防止后续无效检测反复累积
			_ = updateTokenAlertCheckpoint(t.Id, currentUsed, now)
			continue
		}

		// 时间范围：上次告警到现在（首次则从令牌创建时间起）
		start := t.AlertLastNotifiedTime
		if start <= 0 {
			start = t.CreatedTime
		}
		deltaUSD := float64(delta) / common.QuotaPerUnit

		tokenName := t.Name
		if tokenName == "" {
			tokenName = fmt.Sprintf("ID:%d", t.Id)
		}

		content := fmt.Sprintf(
			"Key消耗预警\nKey名称：%s\n时间范围：%s ~ %s\n本次新增消耗：$%.2f\n告警阈值：$%.2f\n累计消耗：$%.2f",
			tokenName,
			time.Unix(start, 0).Format("2006-01-02 15:04:05"),
			time.Unix(now, 0).Format("2006-01-02 15:04:05"),
			deltaUSD,
			t.AlertThreshold,
			float64(currentUsed)/common.QuotaPerUnit,
		)

		if err := service.SendFeishuNotify(webhook, "", dto.FeishuNotify{
			MsgType: "text",
			Content: dto.FeishuContent{Text: content},
		}); err != nil {
			common.SysError(fmt.Sprintf("TokenKeyAlert: send webhook failed token=%d user=%d: %s", t.Id, t.UserId, err.Error()))
			// 发送失败不前移基线，下轮重试
			continue
		}

		if err := updateTokenAlertCheckpoint(t.Id, currentUsed, now); err != nil {
			common.SysError(fmt.Sprintf("TokenKeyAlert: update checkpoint failed token=%d: %s", t.Id, err.Error()))
			continue
		}
		common.SysLog("TokenKeyAlert: " + content)
	}
}

// updateTokenAlertCheckpoint 直接更新告警基线，避免走 Token.Update() 的 Select 白名单。
func updateTokenAlertCheckpoint(tokenId int, notifiedQuota int, notifiedTime int64) error {
	return model.DB.Model(&model.Token{}).Where("id = ?", tokenId).
		Updates(map[string]interface{}{
			"alert_notified_quota":     notifiedQuota,
			"alert_last_notified_time": notifiedTime,
		}).Error
}
