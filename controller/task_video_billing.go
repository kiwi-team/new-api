package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"golang.org/x/net/context"
)

// task_video_billing.go 承载视频任务完成后的按时长重算计费。
//
// 背景：Seedance 2.0 支持 duration=-1（由模型在有效区间内自选时长）。提交时无法
// 得知最终时长，因此按上限 15s 预扣，任务完成后拿到实际时长再多退少补。
//
// 计费公式对时长是线性的（relay/relay_task.go: ratio = price × groupRatio × Π(otherRatios)，
// 其中 seconds 就是 otherRatios 的一项），因此可以直接等比缩放已扣额度，
// 无需持久化 modelPrice / groupRatio —— 这同时规避了任务行只存 UsingGroup
// 而不存 UserGroup、无法还原提交时 effGroupRatio 的问题。

// computeDurationRebill 按实际时长等比重算应扣额度。
//
// preQuota 为提交时已扣额度，assumedSeconds 为预扣所依据的假定时长，
// actualSeconds 为上游返回的实际时长。返回重算后的额度与相对已扣额度的差值
// （正数表示需要补扣，负数表示需要退还）。
//
// 当入参不足以重算时返回 ok=false，调用方应跳过重算。
func computeDurationRebill(preQuota int, assumedSeconds, actualSeconds float64) (actualQuota, delta int, ok bool) {
	if preQuota <= 0 || assumedSeconds <= 0 || actualSeconds <= 0 {
		return 0, 0, false
	}
	if actualSeconds == assumedSeconds {
		return preQuota, 0, true
	}
	actualQuota = int(float64(preQuota) * actualSeconds / assumedSeconds)
	if actualQuota < 0 {
		actualQuota = 0
	}
	return actualQuota, actualQuota - preQuota, true
}

// applyDurationRebill 在任务成功后按实际时长重算并结算差额。
//
// 与既有的 TotalTokens 重算路径不同，这里走 service.PostConsumeQuota，
// 使用户额度、token 级额度（tokens.remain_quota/used_quota）、订阅计费与额度告警
// 保持一致；并同步修正 used_quota / channel used_quota / quota_data。
//
// 幂等：结算前先落 DurationSettled 标记，避免 task.Update() 失败后下一轮轮询重复结算。
func applyDurationRebill(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo) {
	if task == nil || taskResult == nil {
		return
	}
	if task.Properties.DurationSettled {
		return
	}
	assumed := task.Properties.AssumedSeconds
	actual := taskResult.ActualSeconds
	actualQuota, delta, ok := computeDurationRebill(task.Quota, assumed, actual)
	if !ok {
		return
	}

	// 先置幂等标记并持久化，再动钱。若这一步失败则直接放弃本轮结算，
	// 下一轮轮询会重新尝试（此时尚未扣款，不会重复计费）。
	preQuota := task.Quota
	task.Properties.DurationSettled = true
	task.Quota = actualQuota
	if err := task.Update(); err != nil {
		// 回滚内存态，保持与数据库一致
		task.Properties.DurationSettled = false
		task.Quota = preQuota
		logger.LogError(ctx, fmt.Sprintf("视频任务 %s 按时长结算前置更新失败，跳过本轮结算: %s", task.TaskID, err.Error()))
		return
	}

	if delta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费准确（实际时长 %.2fs，预扣时长 %.2fs，%s）",
			task.TaskID, actual, assumed, logger.LogQuota(actualQuota)))
		return
	}

	relayInfo := buildTaskBillingRelayInfo(task)
	if err := service.PostConsumeQuota(relayInfo, delta, 0, false); err != nil {
		logger.LogError(ctx, fmt.Sprintf("视频任务 %s 按时长结算失败: %s", task.TaskID, err.Error()))
		return
	}

	// 同步统计口径：用户/渠道已用额度、以及数据看板的 quota_data。
	// 用 AdjustUserUsedQuota 而非 UpdateUserUsedQuotaAndRequestCount：
	// 请求早已计过数，事后校正不能再次累加 request_count。
	model.AdjustUserUsedQuota(task.UserId, delta)
	model.UpdateChannelUsedQuota(task.ChannelId, delta)
	if common.DataExportEnabled {
		recordDurationQuotaDelta(task, delta)
	}

	if delta > 0 {
		logContent := fmt.Sprintf("视频任务按实际时长补扣费，实际时长 %.2fs，预扣时长 %.2fs，预扣费 %s，实际扣费 %s，补扣费 %s",
			actual, assumed, logger.LogQuota(preQuota), logger.LogQuota(actualQuota), logger.LogQuota(delta))
		logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s %s", task.TaskID, logContent))
		model.RecordLog(task.UserId, model.LogTypeSystem, logContent, delta)
		return
	}

	refund := -delta
	logContent := fmt.Sprintf("视频任务按实际时长返还，实际时长 %.2fs，预扣时长 %.2fs，预扣费 %s，实际扣费 %s，返还 %s",
		actual, assumed, logger.LogQuota(preQuota), logger.LogQuota(actualQuota), logger.LogQuota(refund))
	logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s %s", task.TaskID, logContent))
	model.RecordLog(task.UserId, model.LogTypeSystem, logContent, refund)
}

// buildTaskBillingRelayInfo 由任务行合成结算所需的最小 RelayInfo。
// 轮询协程没有原始请求上下文，只能依赖提交时持久化到 Properties 的计费元数据。
func buildTaskBillingRelayInfo(task *model.Task) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		UserId: task.UserId,
	}
	if tokenId := task.Properties.TokenId; tokenId != 0 {
		info.TokenId = tokenId
		if token, err := model.GetTokenById(tokenId); err == nil && token != nil {
			info.TokenKey = token.Key
		}
	}
	return info
}

// recordDurationQuotaDelta 把按时长结算的差额写进数据看板。
// 正数为补扣，负数为退还，与 model.RefundQuotaData 的符号约定一致。
func recordDurationQuotaDelta(task *model.Task, delta int) {
	username, _ := model.GetUsernameById(task.UserId, false)
	p := task.Properties
	model.LogQuotaData(&model.LogQuotaDataCache{
		UserId:         task.UserId,
		Username:       username,
		ModelName:      p.OriginModelName,
		Quota:          delta,
		CreatedAt:      common.GetTimestamp(),
		TokenId:        p.TokenId,
		TokenName:      p.TokenName,
		ChannelId:      task.ChannelId,
		ClientUserId:   p.ClientUserId,
		ClientScenairo: p.ClientScenairo,
		ProjectName:    p.ProjectName,
		PlanId:         p.PlanId,
	})
}
