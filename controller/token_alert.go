package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
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

	for _, token := range tokens {
		err := model.WithLegacyAlertLease(context.Background(), "token_spend", strconv.Itoa(token.Id), func(ctx context.Context) error { return processTokenKeyAlert(ctx, token.Id) })
		if err != nil && err != model.ErrAlertConflict {
			common.SysError(common.LogText("legacy key alert evaluation failed"))
		}
	}
}

func processTokenKeyAlert(ctx context.Context, tokenID int) error {
	unified, err := model.LegacyAlertUsesUnified(ctx, "token_spend", strconv.Itoa(tokenID))
	if err != nil || unified {
		return err
	}
	var t model.Token
	if err = model.DB.WithContext(ctx).First(&t, tokenID).Error; err != nil {
		return err
	}
	now := time.Now().Unix()
	if t.AlertThreshold <= 0 {
		return nil
	}
	thresholdQuota := int(t.AlertThreshold * common.QuotaPerUnit)
	if thresholdQuota <= 0 {
		return nil
	}

	var currentUsed int
	if err := model.DB.WithContext(ctx).Table("quota_data").
		Select("COALESCE(sum(quota),0)").
		Where("token_id = ?", t.Id).
		Scan(&currentUsed).Error; err != nil {
		common.SysError(fmt.Sprintf("TokenKeyAlert: sum quota failed token=%d: %s", t.Id, err.Error()))
		return nil
	}

	delta := currentUsed - t.AlertNotifiedQuota
	if delta < thresholdQuota {
		return nil
	}

	// Read settings inside the bounded migration lease; a stalled cache/database
	// read must not leave an old sender running after a cutover.
	var user model.User
	if err := model.DB.WithContext(ctx).Select("id", "setting").First(&user, t.UserId).Error; err != nil {
		return err
	}
	setting := user.GetSetting()
	webhook := setting.WebhookUrl
	if webhook == "" {
		// 用户未配置 webhook：仅将基线前移，防止后续无效检测反复累积
		_ = updateTokenAlertCheckpoint(t.Id, currentUsed, now)
		return nil
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

	if err := service.SendFeishuNotifyContext(ctx, webhook, "", dto.FeishuNotify{
		MsgType: "text",
		Content: dto.FeishuContent{Text: content},
	}); err != nil {
		common.SysError(fmt.Sprintf("TokenKeyAlert: send webhook failed token=%d user=%d: %s", t.Id, t.UserId, err.Error()))
		// 发送失败不前移基线，下轮重试
		return nil
	}

	if err := updateTokenAlertCheckpoint(t.Id, currentUsed, now); err != nil {
		common.SysError(fmt.Sprintf("TokenKeyAlert: update checkpoint failed token=%d: %s", t.Id, err.Error()))
		return nil
	}
	common.SysLog("TokenKeyAlert: " + content)
	return nil
}

// updateTokenAlertCheckpoint 直接更新告警基线，避免走 Token.Update() 的 Select 白名单。
func updateTokenAlertCheckpoint(tokenId int, notifiedQuota int, notifiedTime int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return model.DB.WithContext(ctx).Model(&model.Token{}).Where("id = ?", tokenId).
		Updates(map[string]any{
			"alert_notified_quota":     notifiedQuota,
			"alert_last_notified_time": notifiedTime,
		}).Error
}
