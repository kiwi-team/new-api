package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}

	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
	}

	// 复用原始任务的平台：聚合上游（云雾 / PPInfra 等）无法只靠渠道类型判定，
	// 原始任务里已经存了判定结果。
	if originTask.Platform != "" {
		c.Set("platform", string(originTask.Platform))
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			// 历史任务数据可能包含未经校验的时长，作为计费乘数前必须钳制
			if seconds > relaycommon.MaxTaskDurationSeconds {
				seconds = relaycommon.MaxTaskDurationSeconds
			}
			sizeStr, _ := taskData["size"].(string)
			info.PriceData.AddOtherRatio("seconds", float64(seconds))
			info.PriceData.AddOtherRatio("size", 1)
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.AddOtherRatio("size", 1.666667)
			}
		}
	}

	return nil
}

// resolveTaskPlatform 判定本次请求应该走哪个 task 平台。
//
// 大多数渠道的平台就是渠道类型（GetTaskPlatform 的行为）。但云雾、PPInfra、
// Novita、腾讯云、FAL 这些聚合上游复用通用渠道类型（OpenAI / Sora / Gemini…），
// 只能靠 base URL 区分，因此额外用独立的 TaskPlatform 常量标识它们。
//
// 判定只在提交时做一次，结果随 task 落库；fetch 阶段直接按 task.Platform
// 从 GetTaskAdaptor 取适配器，不再重复嗅探 base URL。
func resolveTaskPlatform(info *relaycommon.RelayInfo, platform constant.TaskPlatform) constant.TaskPlatform {
	baseURL := info.ChannelBaseUrl
	switch {
	case strings.Contains(baseURL, "yunwu"):
		if strings.Contains(info.UpstreamModelName, "veo") {
			return constant.TaskPlatformYunwuVeo
		}
		// 云雾逆向的 Sora 通道走 OpenAI 渠道类型；原生 Sora 渠道仍用默认平台。
		if info.ChannelType == constant.ChannelTypeOpenAI {
			return constant.TaskPlatformYunwuSora
		}
		return platform
	case strings.Contains(baseURL, "ppinfra"):
		if strings.Contains(info.UpstreamModelName, "hunyuan") {
			return constant.TaskPlatformPPioHunyuanImage
		}
		return constant.TaskPlatformPPio
	case strings.Contains(baseURL, "novita"):
		return constant.TaskPlatformNovitaImage
	case strings.Contains(baseURL, "tencentcloudapi"):
		return constant.TaskPlatformHunyuanImage
	case strings.Contains(baseURL, "fal"):
		return constant.TaskPlatformFAL
	default:
		return platform
	}
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*dto.TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	platform = resolveTaskPlatform(info, platform)
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 2.5 应用渠道的模型映射（与同步任务对齐）
	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}

	// 4. 价格计算：基础模型价格。适配器动态价格已经是完整的每秒/每次单价，
	// 不依赖全局模型价格配置，必须直接建立 PriceData；否则新模型会在动态价格
	// 应用前被 ModelPriceHelperPerCall 以“价格未配置”拒绝。
	info.OriginModelName = modelName
	priceData, err := resolveTaskBasePrice(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	// ModelPriceHelperPerCall 重建了 PriceData，remix 在 ResolveOriginTask 里
	// 预设的 OtherRatios 要接回来。
	inheritedRatios := info.PriceData.OtherRatios()
	info.PriceData = priceData
	for k, v := range inheritedRatios {
		info.PriceData.AddOtherRatio(k, v)
	}

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	// 6. 将 OtherRatios 应用到基础额度（饱和转换，防止溢出成负数）
	if !common.StringsContains(constant.TaskPricePatches, modelName) {
		quotaWithRatios := info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.Quota))
		quota, clamp := common.QuotaFromFloatChecked(quotaWithRatios)
		info.PriceData.Quota = quota
		noteTaskQuotaClamp(info, clamp)
	}

	// 7. 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && !isSuccessfulTaskSubmitStatus(resp.StatusCode) {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 10. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios()
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 12. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); len(adjustedRatios) > 0 {
		if adjustedQuota, ok := recalcQuotaFromRatios(info, adjustedRatios); ok {
			// 基于调整后的 ratios 重新计算 quota
			finalQuota = adjustedQuota
			info.PriceData.ReplaceOtherRatios(adjustedRatios)
			info.PriceData.Quota = finalQuota
		}
	}

	return &dto.TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}, nil
}

func isSuccessfulTaskSubmitStatus(statusCode int) bool {
	return statusCode == http.StatusOK || statusCode == http.StatusAccepted
}

func resolveTaskBasePrice(c *gin.Context, info *relaycommon.RelayInfo) (hosttypes.PriceData, error) {
	if info.DynamicModelPrice <= 0 {
		return helper.ModelPriceHelperPerCall(c, info)
	}

	groupRatioInfo := helper.HandleGroupRatio(c, info)
	baseQuota, clamp := common.QuotaFromFloatChecked(
		info.DynamicModelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	noteTaskQuotaClamp(info, clamp)
	priceData := hosttypes.PriceData{
		ModelPrice:     info.DynamicModelPrice,
		UsePrice:       true,
		Quota:          baseQuota,
		GroupRatioInfo: groupRatioInfo,
	}
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && groupRatioInfo.GroupRatio == 0 {
		priceData.FreeModel = true
	}
	return priceData, nil
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) (int, bool) {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.RemoveOtherRatiosFromFloat(float64(info.PriceData.Quota))
	priceData := info.PriceData
	if !priceData.ReplaceOtherRatios(ratios) {
		return 0, false
	}
	// 应用新的 ratios
	result := priceData.ApplyOtherRatiosToFloat(baseQuota)
	quota, clamp := common.QuotaFromFloatChecked(result)
	noteTaskQuotaClamp(info, clamp)
	return quota, true
}

// noteTaskQuotaClamp records the first quota saturation event onto the task's
// RelayInfo so LogTaskConsumption can surface it on the submit log's
// admin_info. First non-nil clamp wins.
func noteTaskQuotaClamp(info *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || info == nil {
		return
	}
	if info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		// 不 return 会让下面直接调用 nil 函数并 panic。
		return service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")

	// 已成功并且结果已落到自有存储（S3）时直接复用，不再回源。
	// omni 的 GET interactions 每次都会返回完整视频，重复拉取会反复上传 S3。
	if originTask.Status == model.TaskStatusSuccess && strings.HasPrefix(originTask.GetResultURL(), "https://") {
		if isOpenAIVideoAPI {
			return openAIVideoRespBody(originTask)
		}
		return simpleVideoRespBody(originTask, nil, ""), nil
	}

	// 其余情况向上游实时拉取最新状态
	if realtimeResp, done := tryRealtimeFetch(c, originTask, isOpenAIVideoAPI); done {
		respBody = realtimeResp
		if len(respBody) != 0 {
			return
		}
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		return openAIVideoRespBody(originTask)
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// openAIVideoRespBody 用任务所属平台的 adaptor 把 task 转成 OpenAI Video API 响应。
func openAIVideoRespBody(task *model.Task) (respBody []byte, taskResp *dto.TaskError) {
	adaptor := GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", task.ChannelId), "invalid_channel_id", http.StatusBadRequest)
	}
	converter, ok := adaptor.(channel.OpenAIVideoConverter)
	if !ok {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", task.Platform), "not_implemented", http.StatusNotImplemented)
	}
	openAIVideoData, err := converter.ConvertToOpenAIVideo(task)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
	}
	return openAIVideoData, nil
}

// simpleVideoRespBody 构建非 OpenAI Video API 的精简响应体。
// taskResult 可为 nil（走 S3 复用短路时没有上游响应）；非 nil 时会附带
// World Labs 的额外产物地址。format 为空时从结果地址的扩展名推断。
func simpleVideoRespBody(task *model.Task, taskResult *relaycommon.TaskInfo, format string) []byte {
	resultURL := task.GetResultURL()
	if format == "" {
		format = videoFormatFromURL(resultURL)
	}
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      resultURL,
	}
	// 失败时 FailReason 存的是错误原因而非视频地址：放进 error 字段，url 置空。
	if task.Status == model.TaskStatusFailure {
		out["url"] = nil
		if reason := strings.TrimSpace(task.FailReason); reason != "" {
			out["error"] = map[string]any{"message": reason}
		}
	}
	if taskResult != nil {
		// World Labs 除视频外还会返回网格 / splat / marble 产物
		for key, url := range map[string]string{
			"collider_mesh_url":  taskResult.ColliderMeshUrl,
			"splat_url_500k":     taskResult.SplatUrl500k,
			"splat_url_full_res": taskResult.SplatUrlFullRes,
			"world_marble_url":   taskResult.WorldMarbleUrl,
		} {
			if url != "" {
				out[key] = url
			}
		}
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// tryRealtimeFetch 向上游实时拉取任务最新状态并回写 task。
// 适配器按 task.Platform 从注册表取得（提交时已判定并落库）；取不到时回退到渠道类型。
// done 表示确实完成了一次上游查询；respBody 仅在非 OpenAI Video API 时构建。
//
// 客户端轮询通常比 15 秒一轮的后台轮询更快，所以这里很可能是第一个把任务推进到
// 终态的地方。一旦写入终态，GetAllUnFinishSyncTasks / GetTimedOutUnfinishedTasks
// 都会过滤掉这条任务，后台轮询再也不会碰它——因此结算和退款必须在这里一并完成，
// 否则失败任务的预扣费永远不会退还。
func tryRealtimeFetch(c *gin.Context, task *model.Task, isOpenAIVideoAPI bool) (respBody []byte, done bool) {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil, false
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy

	adaptor := GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		// 历史任务可能没有存平台，回退到渠道类型
		adaptor = GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	}
	if adaptor == nil {
		return nil, false
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
		"model":   task.Properties.UpstreamModelName,
	}, proxy)
	if err != nil || resp == nil {
		return nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil, false
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if task.Status == model.TaskStatusSuccess && ti.Url == "" && ti.RemoteUrl != "" {
		// 上游地址需要凭 API key 才能访问（Gemini files download），先转存再对外给出。
		// 任务提交时用的可能是多 key 渠道里的某一把，优先用任务上存的那一把。
		videoKey := task.PrivateData.Key
		if videoKey == "" {
			videoKey = channelModel.Key
		}
		ti.Url = service.MaterializeRemoteTaskVideo(c, ti.RemoteUrl, videoKey)
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI（如 Vertex 转存 S3 失败时的内联视频）留在 Data 里，
		// 结果地址退回内容代理地址，由 /content 端点解码后返回。
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}
	if task.Status == model.TaskStatusFailure && ti.Reason != "" {
		task.FailReason = ti.Reason
	}
	task.Data = service.RedactVideoResponseBody(body)

	// 与轮询循环保持一致：终态一律补齐进度和完成时间，否则任务会以
	// SUCCESS + "20%" + finish_time=0 的形态留在库里。
	isTerminal := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
	if isTerminal {
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = time.Now().Unix()
		}
	}

	if snap.Equal(task.Snapshot()) {
		return realtimeFetchRespBody(task, ti, body, isOpenAIVideoAPI)
	}

	won, err := task.UpdateWithStatus(snap.Status)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("实时查询回写任务 %s 失败: %s", task.TaskID, err.Error()))
		return realtimeFetchRespBody(task, ti, body, isOpenAIVideoAPI)
	}

	// 只有真正赢下终态流转的一方才结算：CAS 失败说明后台轮询或另一个
	// 并发请求已经推进过，结算由它负责，这里重复执行会造成双倍退款。
	if won && isTerminal && snap.Status != task.Status {
		switch task.Status {
		case model.TaskStatusSuccess:
			service.SettleTaskBillingOnComplete(c, adaptor, task, ti)
		case model.TaskStatusFailure:
			if task.Quota != 0 {
				service.RefundTaskQuota(c, task, task.FailReason)
			}
		}
	}

	return realtimeFetchRespBody(task, ti, body, isOpenAIVideoAPI)
}

// realtimeFetchRespBody 构建 tryRealtimeFetch 的返回体。
// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理，这里只回 done。
func realtimeFetchRespBody(task *model.Task, ti *relaycommon.TaskInfo, body []byte, isOpenAIVideoAPI bool) (respBody []byte, done bool) {
	if isOpenAIVideoAPI {
		return nil, true
	}

	// 上游响应里的 mimeType 比从结果地址后缀推断更可靠，优先采用
	format := detectVideoFormat(body)
	if format == "mp4" {
		format = ""
	}
	return simpleVideoRespBody(task, ti, format), true
}

// videoFormatFromURL 从结果地址的扩展名推断视频格式，默认 mp4。
func videoFormatFromURL(url string) string {
	if url == "" {
		return "mp4"
	}
	withoutQuery := strings.SplitN(url, "?", 2)[0]
	idx := strings.LastIndex(withoutQuery, ".")
	if idx < 0 || idx == len(withoutQuery)-1 {
		return "mp4"
	}
	ext := withoutQuery[idx+1:]
	if strings.Contains(ext, "/") {
		return "mp4"
	}
	return ext
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Data:       task.Data,
	}
}
