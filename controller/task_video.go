package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/fal"
	"github.com/QuantumNous/new-api/relay/channel/task/hunyuan"
	"github.com/QuantumNous/new-api/relay/channel/task/hunyuan/ppio"
	hunyuanppio "github.com/QuantumNous/new-api/relay/channel/task/hunyuan/ppio"
	"github.com/QuantumNous/new-api/relay/channel/task/novita"
	"github.com/QuantumNous/new-api/relay/channel/task/vertex/yunwu"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if err := updateVideoTaskAll(ctx, platform, channelId, taskIds, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		errUpdate := model.TaskBulkUpdate(taskIds, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := relay.GetTaskAdaptor(platform)
	if strings.Contains(cacheGetChannel.GetBaseURL(), "yunwu") {
		adaptor = &yunwu.TaskAdaptor{}
	} else if strings.Contains(cacheGetChannel.GetBaseURL(), "ppinfra") {
		if strings.Contains(cacheGetChannel.GetBaseURL(), "hunyuan") {
			adaptor = &hunyuanppio.TaskAdaptor{}
		} else {
			adaptor = &ppio.TaskAdaptor{}
		}
	} else if strings.Contains(cacheGetChannel.GetBaseURL(), "novita") {
		adaptor = &novita.TaskAdaptor{}
	} else if strings.Contains(cacheGetChannel.GetBaseURL(), "tencentcloudapi") {
		adaptor = &hunyuan.TaskAdaptor{}
	} else if strings.Contains(cacheGetChannel.GetBaseURL(), "fal") {
		adaptor = &fal.TaskAdaptor{}
	}
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: cacheGetChannel.GetBaseURL(),
	}
	info.ApiKey = cacheGetChannel.Key
	adaptor.Init(info)
	for _, taskId := range taskIds {
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
		}
	}
	return nil
}

func updateVideoSingleTask(ctx context.Context, adaptor channel.TaskAdaptor, channel *model.Channel, taskId string, taskM map[string]*model.Task) error {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}
	proxy := channel.GetSetting().Proxy

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}
	key := channel.Key

	privateData := task.PrivateData
	if privateData.Key != "" {
		key = privateData.Key
	}
	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": taskId,
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	//if resp.StatusCode != http.StatusOK {
	//return fmt.Errorf("get Video Task status code: %d", resp.StatusCode)
	//}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}

	logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask response: %s", string(responseBody)))

	taskResult := &relaycommon.TaskInfo{}
	// try parse as New API response format
	var responseItems dto.TaskResponse[model.Task]
	if r, ok := tryParseVideoGenerationsResponse(responseBody); ok {
		// 上游是 new-api 级联/中转，返回 /v1/video/generations 的 {data:{url,status,task_id,format}} 结构。
		// 该结构里视频地址在 data.url、状态用 succeeded/failed/processing 等语义，需要单独兼容。
		logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask parsed as video generations response: %+v", r))
		taskResult = r
		task.Data = responseBody
	} else if err = common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask parsed as new api response format: %+v", responseItems))
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.FailReason
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	} else {
		task.Data = redactVideoResponseBody(responseBody)
	}

	logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask taskResult: %+v", taskResult))

	now := time.Now().Unix()
	if taskResult.Status == "" {
		//return fmt.Errorf("task %s status is empty", taskId)
		taskResult = relaycommon.FailTaskInfo("upstream returned empty status")
	}

	// 记录原本的状态，防止重复退款
	shouldRefund := false
	quota := task.Quota
	preStatus := task.Status

	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = "10%"
	case model.TaskStatusQueued:
		task.Progress = "20%"
	case model.TaskStatusInProgress:
		task.Progress = "30%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess, "succeeded":
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if !(len(taskResult.Url) > 5 && taskResult.Url[:5] == "data:") {
			task.FailReason = taskResult.Url
		}

		// 如果返回了 total_tokens 并且配置了模型倍率(非固定价格),则重新计费
		if taskResult.TotalTokens > 0 {
			// 获取模型名称
			//var taskData map[string]interface{}
			//if err := json.Unmarshal(task.Data, &taskData); err == nil {
			//if modelName, ok := taskData["model"].(string); ok && modelName != "" {
			modelName := task.Properties.OriginModelName
			//if err := json.Unmarshal(task.Properties, &properties); err == nil {
			if modelName != "" {
				// 获取模型价格和倍率
				modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)
				// 只有配置了倍率(非固定价格)时才按 token 重新计费
				if hasRatioSetting && modelRatio > 0 {
					// 获取用户和组的倍率信息
					group := task.Group
					if group == "" {
						user, err := model.GetUserById(task.UserId, false)
						if err == nil {
							group = user.Group
						}
					}
					if group != "" {
						groupRatio := ratio_setting.GetGroupRatio(group)
						userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)

						var finalGroupRatio float64
						if hasUserGroupRatio {
							finalGroupRatio = userGroupRatio
						} else {
							finalGroupRatio = groupRatio
						}

						// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio
						actualQuota := int(float64(taskResult.TotalTokens) * modelRatio * finalGroupRatio)

						// 计算差额
						preConsumedQuota := task.Quota
						quotaDelta := actualQuota - preConsumedQuota

						if quotaDelta > 0 {
							// 需要补扣费
							logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费后补扣费：%s（实际消耗：%s，预扣费：%s，tokens：%d）",
								task.TaskID,
								logger.LogQuota(quotaDelta),
								logger.LogQuota(actualQuota),
								logger.LogQuota(preConsumedQuota),
								taskResult.TotalTokens,
							))
							if err := model.DecreaseUserQuota(task.UserId, quotaDelta); err != nil {
								logger.LogError(ctx, fmt.Sprintf("补扣费失败: %s", err.Error()))
							} else {
								model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
								model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
								task.Quota = actualQuota // 更新任务记录的实际扣费额度

								// 记录消费日志
								logContent := fmt.Sprintf("视频任务成功补扣费，模型倍率 %.2f，分组倍率 %.2f，tokens %d，预扣费 %s，实际扣费 %s，补扣费 %s",
									modelRatio, finalGroupRatio, taskResult.TotalTokens,
									logger.LogQuota(preConsumedQuota), logger.LogQuota(actualQuota), logger.LogQuota(quotaDelta))
								model.RecordLog(task.UserId, model.LogTypeSystem, logContent, actualQuota)
							}
						} else if quotaDelta < 0 {
							// 需要退还多扣的费用
							refundQuota := -quotaDelta
							logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费后返还：%s（实际消耗：%s，预扣费：%s，tokens：%d）",
								task.TaskID,
								logger.LogQuota(refundQuota),
								logger.LogQuota(actualQuota),
								logger.LogQuota(preConsumedQuota),
								taskResult.TotalTokens,
							))
							if err := model.IncreaseUserQuota(task.UserId, refundQuota, false); err != nil {
								logger.LogError(ctx, fmt.Sprintf("退还预扣费失败: %s", err.Error()))
							} else {
								task.Quota = actualQuota // 更新任务记录的实际扣费额度

								// 记录退款日志
								logContent := fmt.Sprintf("视频任务成功退还多扣费用，模型倍率 %.2f，分组倍率 %.2f，tokens %d，预扣费 %s，实际扣费 %s，退还 %s",
									modelRatio, finalGroupRatio, taskResult.TotalTokens,
									logger.LogQuota(preConsumedQuota), logger.LogQuota(actualQuota), logger.LogQuota(refundQuota))
								model.RecordLog(task.UserId, model.LogTypeSystem, logContent, refundQuota)
							}
						} else {
							// quotaDelta == 0, 预扣费刚好准确
							logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费准确（%s，tokens：%d）",
								task.TaskID, logger.LogQuota(actualQuota), taskResult.TotalTokens))
						}
					}
				}
			}
			//}
		}
	case model.TaskStatusFailure:
		logger.LogJson(ctx, fmt.Sprintf("Task %s failed", taskId), task)
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		taskResult.Progress = "100%"
		if quota != 0 {
			if preStatus != model.TaskStatusFailure {
				shouldRefund = true
			} else {
				logger.LogWarn(ctx, fmt.Sprintf("Task %s already in failure status, skip refund", task.TaskID))
			}
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", taskResult.Status, taskId)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
	if err := task.Update(); err != nil {
		common.SysLog("UpdateVideoTask task error: " + err.Error())
		shouldRefund = false
	}

	if shouldRefund {
		// 任务失败且之前状态不是失败才退还额度，防止重复退还
		if err := model.IncreaseUserQuota(task.UserId, quota, false); err != nil {
			logger.LogWarn(ctx, "Failed to increase user quota: "+err.Error())
		}
		// Restore token remain_quota / used_quota
		if tokenId := task.Properties.TokenId; tokenId != 0 {
			if token, err := model.GetTokenById(tokenId); err == nil {
				if refundErr := model.IncreaseTokenQuota(token.Id, token.Key, quota); refundErr != nil {
					logger.LogWarn(ctx, fmt.Sprintf("Failed to increase token quota for task %s: %s", task.TaskID, refundErr.Error()))
				}
			} else {
				logger.LogWarn(ctx, fmt.Sprintf("Failed to get token %d for task %s refund: %s", tokenId, task.TaskID, err.Error()))
			}
		}
		logContent := fmt.Sprintf("Video async task failed %s, refund %s", task.TaskID, logger.LogQuota(quota))
		model.RecordLog(task.UserId, model.LogTypeSystem, logContent, quota)
		// Offset the quota_data entry that was written at submission time
		if common.DataExportEnabled {
			model.RefundQuotaData(task)
		}
	}

	return nil
}

// tryParseVideoGenerationsResponse 兼容上游为 new-api 级联/中转时返回的
// /v1/video/generations 响应结构：{code:"success", data:{error,format,metadata,status,task_id,url}}。
// 该结构里视频地址在 data.url，状态用 succeeded/failed/processing/queued 等语义字符串，
// 与内部 TaskResponse[model.Task]（data 为 model.Task）不同，需要单独识别并转换。
func tryParseVideoGenerationsResponse(body []byte) (*relaycommon.TaskInfo, bool) {
	var wrapper struct {
		Code string `json:"code"`
		Data struct {
			Status   string      `json:"status"`
			TaskID   string      `json:"task_id"`
			URL      string      `json:"url"`
			Error    interface{} `json:"error"`
			Progress string      `json:"progress"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &wrapper); err != nil {
		return nil, false
	}
	if wrapper.Code != "success" {
		return nil, false
	}
	// 仅当 data 携带 video-generations 独有特征时才认定为该格式，避免与内部
	// TaskResponse[model.Task]（status 为大写 SUCCESS/QUEUED，转小写后会与部分值重合）混淆：
	//   - data.url 非空（model.Task 无此字段），或
	//   - status 为 OpenAI 风格的 succeeded/failed/processing（内部常量里没有这些值）。
	d := wrapper.Data
	statusLower := strings.ToLower(strings.TrimSpace(d.Status))
	isVideoGenOnlyStatus := statusLower == "succeeded" || statusLower == "failed" || statusLower == "processing"
	if d.URL == "" && !isVideoGenOnlyStatus {
		return nil, false
	}

	ti := &relaycommon.TaskInfo{
		TaskID:   d.TaskID,
		Progress: d.Progress,
	}
	switch statusLower {
	case "succeeded", "success":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		ti.Url = d.URL
	case "failed", "error":
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		if msg := stringifyError(d.Error); msg != "" {
			ti.Reason = msg
		} else {
			ti.Reason = "upstream task failed"
		}
	case "queued":
		ti.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		ti.Status = model.TaskStatusInProgress
	default:
		// 有 url 但状态未知：按成功处理
		if d.URL != "" {
			ti.Status = model.TaskStatusSuccess
			ti.Progress = "100%"
			ti.Url = d.URL
		} else {
			return nil, false
		}
	}
	return ti, true
}

// stringifyError 尽量从任意形态的 error 字段中提取可读信息。
func stringifyError(e interface{}) string {
	switch v := e.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]any:
		if msg, ok := v["message"].(string); ok {
			return msg
		}
	}
	return ""
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	// Gemini Omni interactions: strip inline base64 video data from steps[].content[].
	if steps, ok := m["steps"].([]any); ok {
		for _, s := range steps {
			step, ok := s.(map[string]any)
			if !ok {
				continue
			}
			contents, ok := step["content"].([]any)
			if !ok {
				continue
			}
			for _, cc := range contents {
				cm, ok := cc.(map[string]any)
				if !ok {
					continue
				}
				if v, ok := cm["data"].(string); ok {
					cm["data"] = truncateBase64(v)
				}
			}
		}
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}
