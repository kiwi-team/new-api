package relay

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	_ "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/advancedcustom"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/ali_dashscope"
	"github.com/QuantumNous/new-api/relay/channel/aws"
	"github.com/QuantumNous/new-api/relay/channel/awsv2"
	"github.com/QuantumNous/new-api/relay/channel/baidu"
	"github.com/QuantumNous/new-api/relay/channel/baidu_v2"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/cloudflare"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/QuantumNous/new-api/relay/channel/cohere"
	"github.com/QuantumNous/new-api/relay/channel/coze"
	"github.com/QuantumNous/new-api/relay/channel/deepseek"
	"github.com/QuantumNous/new-api/relay/channel/dify"
	"github.com/QuantumNous/new-api/relay/channel/elevenlabs"
	"github.com/QuantumNous/new-api/relay/channel/fal_sync"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/gemini_realtime"
	"github.com/QuantumNous/new-api/relay/channel/jimeng"
	"github.com/QuantumNous/new-api/relay/channel/jina"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/relay/channel/mistral"
	"github.com/QuantumNous/new-api/relay/channel/mokaai"
	"github.com/QuantumNous/new-api/relay/channel/moonshot"
	"github.com/QuantumNous/new-api/relay/channel/newapi"
	"github.com/QuantumNous/new-api/relay/channel/ollama"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/palm"
	"github.com/QuantumNous/new-api/relay/channel/perplexity"
	ppioImage "github.com/QuantumNous/new-api/relay/channel/ppio"
	"github.com/QuantumNous/new-api/relay/channel/qwen_realtime"
	"github.com/QuantumNous/new-api/relay/channel/replicate"
	"github.com/QuantumNous/new-api/relay/channel/reve"
	"github.com/QuantumNous/new-api/relay/channel/sensenova"
	"github.com/QuantumNous/new-api/relay/channel/siliconflow"
	"github.com/QuantumNous/new-api/relay/channel/sub2api"
	"github.com/QuantumNous/new-api/relay/channel/submodel"
	taskali "github.com/QuantumNous/new-api/relay/channel/task/ali"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	taskFal "github.com/QuantumNous/new-api/relay/channel/task/fal"
	taskGemini "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	"github.com/QuantumNous/new-api/relay/channel/task/hailuo"
	taskHedra "github.com/QuantumNous/new-api/relay/channel/task/hedra"
	taskHeyGen "github.com/QuantumNous/new-api/relay/channel/task/heygen"
	taskHunyuan "github.com/QuantumNous/new-api/relay/channel/task/hunyuan"
	taskHunyuanPPio "github.com/QuantumNous/new-api/relay/channel/task/hunyuan/ppio"
	taskjimeng "github.com/QuantumNous/new-api/relay/channel/task/jimeng"
	jspluginadaptor "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel/task/kling"
	taskLtx "github.com/QuantumNous/new-api/relay/channel/task/ltx"
	taskMiniMax "github.com/QuantumNous/new-api/relay/channel/task/minimax"
	taskNovita "github.com/QuantumNous/new-api/relay/channel/task/novita"
	taskPixverse "github.com/QuantumNous/new-api/relay/channel/task/pixverse"
	taskPPio "github.com/QuantumNous/new-api/relay/channel/task/ppio"
	taskReplicateTask "github.com/QuantumNous/new-api/relay/channel/task/replicatetask"
	taskRunwayML "github.com/QuantumNous/new-api/relay/channel/task/runwayml"
	tasksora "github.com/QuantumNous/new-api/relay/channel/task/sora"
	taskSoraYunwu "github.com/QuantumNous/new-api/relay/channel/task/sora/yunwu"
	"github.com/QuantumNous/new-api/relay/channel/task/suno"
	taskvertex "github.com/QuantumNous/new-api/relay/channel/task/vertex"
	taskVertexYunwu "github.com/QuantumNous/new-api/relay/channel/task/vertex/yunwu"
	taskVidu "github.com/QuantumNous/new-api/relay/channel/task/vidu"
	taskWorldLabs "github.com/QuantumNous/new-api/relay/channel/task/worldlabs"
	"github.com/QuantumNous/new-api/relay/channel/tencent"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	"github.com/QuantumNous/new-api/relay/channel/visualvolcengine"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	"github.com/QuantumNous/new-api/relay/channel/xai"
	"github.com/QuantumNous/new-api/relay/channel/xunfei"
	"github.com/QuantumNous/new-api/relay/channel/zhipu"
	"github.com/QuantumNous/new-api/relay/channel/zhipu_4v"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func GetAdaptor(apiType int) channel.Adaptor {
	switch apiType {
	case constant.APITypeAli:
		return &ali.Adaptor{}
	case constant.APITypeAnthropic:
		return &claude.Adaptor{}
	case constant.APITypeBaidu:
		return &baidu.Adaptor{}
	case constant.APITypeGemini:
		return &gemini.Adaptor{}
	case constant.APITypeOpenAI:
		return &openai.Adaptor{}
	case constant.APITypePaLM:
		return &palm.Adaptor{}
	case constant.APITypeTencent:
		return &tencent.DispatchAdaptor{}
	case constant.APITypeXunfei:
		return &xunfei.Adaptor{}
	case constant.APITypeZhipu:
		return &zhipu.Adaptor{}
	case constant.APITypeZhipuV4:
		return &zhipu_4v.Adaptor{}
	case constant.APITypeOllama:
		return &ollama.Adaptor{}
	case constant.APITypePerplexity:
		return &perplexity.Adaptor{}
	case constant.APITypeAws:
		return &aws.Adaptor{}
	case constant.APITypeCohere:
		return &cohere.Adaptor{}
	case constant.APITypeDify:
		return &dify.Adaptor{}
	case constant.APITypeJina:
		return &jina.Adaptor{}
	case constant.APITypeCloudflare:
		return &cloudflare.Adaptor{}
	case constant.APITypeSiliconFlow:
		return &siliconflow.Adaptor{}
	case constant.APITypeVertexAi:
		return &vertex.Adaptor{}
	case constant.APITypeMistral:
		return &mistral.Adaptor{}
	case constant.APITypeDeepSeek:
		return &deepseek.Adaptor{}
	case constant.APITypeMokaAI:
		return &mokaai.Adaptor{}
	case constant.APITypeVolcEngine:
		return &volcengine.Adaptor{}
	case constant.APITypeBaiduV2:
		return &baidu_v2.Adaptor{}
	case constant.APITypeOpenRouter:
		return &openai.Adaptor{}
	case constant.APITypeXinference:
		return &openai.Adaptor{}
	case constant.APITypeXai:
		return &xai.Adaptor{}
	case constant.APITypeCoze:
		return &coze.Adaptor{}
	case constant.APITypeSensenova:
		return &sensenova.Adaptor{}
	case constant.APITypeVisualVolcEngine:
		return &visualvolcengine.Adaptor{}
	case constant.APITypeJimeng:
		return &jimeng.Adaptor{}
	case constant.APITypeMoonshot:
		return &moonshot.Adaptor{} // Moonshot uses Claude API
	case constant.APITypeSubmodel:
		return &submodel.Adaptor{}
	case constant.APITypeMiniMax:
		return &minimax.Adaptor{}
	case constant.APITypeElevenLabs:
		return &elevenlabs.Adaptor{}
	case constant.APITypeAliDashScope:
		return &ali_dashscope.Adaptor{}
	case constant.APITypeFAL:
		return &openai.Adaptor{}
	case constant.APITypeReplicate:
		return &replicate.Adaptor{}
	case constant.APITypeFALSync:
		return &fal_sync.Adaptor{}
	case constant.APITypeAwsV2:
		return &awsv2.Adaptor{}
	case constant.APITypeCodex:
		return &codex.Adaptor{}
	case constant.APITypeQwenRealtime:
		return &qwen_realtime.Adaptor{}
	case constant.APITypeGeminiRealtime:
		return &gemini_realtime.Adaptor{}
	case constant.APITypeVolcEngineRealtime:
		return &volcengine.RealtimeAdaptor{}
	case constant.APITypePPIO:
		return &ppioImage.Adaptor{}
	case constant.APITypeReve:
		return &reve.Adaptor{}
	case constant.APITypeAdvancedCustom:
		return &advancedcustom.Adaptor{}
	case constant.APITypeSub2API:
		return &sub2api.Adaptor{}
	case constant.APITypeNewAPI:
		return &newapi.Adaptor{}
	}
	return nil
}

func GetTaskPlatform(c *gin.Context) constant.TaskPlatform {
	if pluginKey := c.GetString("task_plugin_key"); pluginKey != "" {
		return constant.TaskPlatform(pluginKey)
	}
	channelType := c.GetInt("channel_type")
	if channelType > 0 {
		return constant.TaskPlatform(strconv.Itoa(channelType))
	}
	return constant.TaskPlatform(c.GetString("platform"))
}

var taskPluginKeys = map[constant.TaskPlatform]string{
	constant.TaskPlatformSuno:                                            "sunoapi",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)):         "alibaba",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)):       "kling",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeJimeng)):      "jimeng",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVidu)):        "vidu",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)): "doubao",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)):  "doubao",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)):      "google",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMax)):     "hailuo",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)):        "sora",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOpenAI)):      "sora",
	constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVertexAi)):    "vertex-ai",
}

func ResolveTaskPluginForPlatform(generation *pluginruntime.RoutingGeneration, platform constant.TaskPlatform) (*pluginruntime.LoadedPlugin, bool) {
	if generation == nil {
		return nil, false
	}
	if key, ok := taskPluginKeys[platform]; ok {
		if plugin, found := generation.Get(key); found {
			return plugin, true
		}
	}
	return generation.Get(string(platform))
}

// TaskPlatformUnavailableError explains why no adaptor serves the platform:
// the task-plugin system is switched off, the resolved plugin is disabled,
// or the platform simply names nothing. The distinction is user-actionable,
// so it must survive into the client-facing message.
func TaskPlatformUnavailableError(platform constant.TaskPlatform) (string, string) {
	if !pluginruntime.DefaultRegistry.Enabled() {
		return "task_plugin_system_disabled", "the task plugin system is disabled on this gateway"
	}
	key := string(platform)
	if mapped, ok := taskPluginKeys[platform]; ok {
		key = mapped
	}
	for _, meta := range pluginruntime.DefaultRegistry.Snapshot().Factory {
		if meta.Key == key {
			return "task_plugin_disabled", fmt.Sprintf("task plugin %q is disabled on this gateway", key)
		}
	}
	return "invalid_api_platform", fmt.Sprintf("invalid api platform: %s", platform)
}

func GetTaskAdaptor(platform constant.TaskPlatform) channel.TaskAdaptor {
	plugin, ok := ResolveTaskPluginForPlatform(pluginruntime.DefaultRegistry.Generation(), platform)
	if ok {
		return jspluginadaptor.New(plugin)
	}
	return getLegacyTaskAdaptor(platform)
}

type legacyTaskAdaptor interface {
	Init(info *relaycommon.RelayInfo)
	ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError
	EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64
	AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int
	BuildRequestURL(info *relaycommon.RelayInfo) (string, error)
	BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error
	BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error)
	DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error)
	DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError)
	GetModelList() []string
	GetChannelName() string
	FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error)
	ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error)
}

type legacyTaskAdaptorBridge struct {
	legacyTaskAdaptor
}

func (a *legacyTaskAdaptorBridge) ParseResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *taskdto.TaskError) {
	recorder := httptest.NewRecorder()
	bridgeContext, _ := gin.CreateTestContext(recorder)
	bridgeContext.Request = c.Request
	bridgeContext.Keys = c.Keys
	taskID, taskData, taskErr := a.DoResponse(bridgeContext, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}
	var clientResponse any
	if recorder.Body.Len() > 0 {
		if err := common.Unmarshal(recorder.Body.Bytes(), &clientResponse); err != nil {
			return nil, &taskdto.TaskError{Error: err, Code: "unmarshal_response_failed", StatusCode: http.StatusInternalServerError}
		}
	}
	return &channel.TaskSubmitResponse{UpstreamTaskID: taskID, TaskData: taskData, ClientResponse: clientResponse}, nil
}

func (a *legacyTaskAdaptorBridge) FetchTask(baseURL, key string, task *model.Task, proxy string) (*http.Response, error) {
	return a.legacyTaskAdaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
		"model":   task.Properties.UpstreamModelName,
	}, proxy)
}

func (a *legacyTaskAdaptorBridge) ParseTaskResult(_ *model.Task, _ *http.Response, body []byte) (*relaycommon.TaskInfo, error) {
	return a.legacyTaskAdaptor.ParseTaskResult(body)
}

func bridgeLegacyTaskAdaptor(adaptor legacyTaskAdaptor) channel.TaskAdaptor {
	return &legacyTaskAdaptorBridge{legacyTaskAdaptor: adaptor}
}

func getLegacyTaskAdaptor(platform constant.TaskPlatform) channel.TaskAdaptor {
	switch platform {
	case constant.TaskPlatformSuno:
		return bridgeLegacyTaskAdaptor(&suno.TaskAdaptor{})
	case constant.TaskPlatformYunwuVeo:
		return bridgeLegacyTaskAdaptor(&taskVertexYunwu.TaskAdaptor{})
	case constant.TaskPlatformYunwuSora:
		return bridgeLegacyTaskAdaptor(&taskSoraYunwu.TaskAdaptor{})
	case constant.TaskPlatformPPio:
		return bridgeLegacyTaskAdaptor(&taskPPio.TaskAdaptor{})
	case constant.TaskPlatformPPioHunyuanImage:
		return bridgeLegacyTaskAdaptor(&taskHunyuanPPio.TaskAdaptor{})
	case constant.TaskPlatformNovitaImage:
		return bridgeLegacyTaskAdaptor(&taskNovita.TaskAdaptor{})
	case constant.TaskPlatformHunyuanImage:
		return bridgeLegacyTaskAdaptor(&taskHunyuan.TaskAdaptor{})
	case constant.TaskPlatformFAL, constant.TaskPlatformFALImage:
		return bridgeLegacyTaskAdaptor(&taskFal.TaskAdaptor{})
	}

	channelType, err := strconv.ParseInt(string(platform), 10, 64)
	if err != nil {
		return nil
	}
	switch channelType {
	case constant.ChannelTypeAli:
		return bridgeLegacyTaskAdaptor(&taskali.TaskAdaptor{})
	case constant.ChannelTypeKling:
		return bridgeLegacyTaskAdaptor(&kling.TaskAdaptor{})
	case constant.ChannelTypeJimeng:
		return bridgeLegacyTaskAdaptor(&taskjimeng.TaskAdaptor{})
	case constant.ChannelTypeVertexAi:
		return bridgeLegacyTaskAdaptor(&taskvertex.TaskAdaptor{})
	case constant.ChannelTypeVidu:
		return bridgeLegacyTaskAdaptor(&taskVidu.TaskAdaptor{})
	case constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine:
		return bridgeLegacyTaskAdaptor(&taskdoubao.TaskAdaptor{})
	case constant.ChannelTypeSora, constant.ChannelTypeOpenAI:
		return bridgeLegacyTaskAdaptor(&tasksora.TaskAdaptor{})
	case constant.ChannelTypeGemini:
		return bridgeLegacyTaskAdaptor(&taskGemini.TaskAdaptor{})
	case constant.ChannelTypeMiniMaxVideo:
		return bridgeLegacyTaskAdaptor(&taskMiniMax.TaskAdaptor{})
	case constant.ChannelTypeMiniMax:
		return bridgeLegacyTaskAdaptor(&hailuo.TaskAdaptor{})
	case constant.ChannelTypePixverse:
		return bridgeLegacyTaskAdaptor(&taskPixverse.TaskAdaptor{})
	case constant.ChannelTypeLtx:
		return bridgeLegacyTaskAdaptor(&taskLtx.TaskAdaptor{})
	case constant.ChannelTypeWorldLabs:
		return bridgeLegacyTaskAdaptor(&taskWorldLabs.TaskAdaptor{})
	case constant.ChannelTypeRunwayML:
		return bridgeLegacyTaskAdaptor(&taskRunwayML.TaskAdaptor{})
	case constant.ChannelTypeReplicate:
		return bridgeLegacyTaskAdaptor(&taskReplicateTask.TaskAdaptor{})
	case constant.ChannelTypeHedra:
		return bridgeLegacyTaskAdaptor(&taskHedra.TaskAdaptor{})
	case constant.ChannelTypeHeyGen:
		return bridgeLegacyTaskAdaptor(&taskHeyGen.TaskAdaptor{})
	case constant.ChannelTypeFAL:
		return bridgeLegacyTaskAdaptor(&taskFal.TaskAdaptor{})
	case constant.ChannelTypePPIO:
		return bridgeLegacyTaskAdaptor(&taskPPio.TaskAdaptor{})
	case constant.ChannelTypeTencent:
		return bridgeLegacyTaskAdaptor(&taskHunyuan.TaskAdaptor{})
	default:
		return nil
	}
}

// getTaskAdaptorForRequest preserves the exact plugin object pinned by the
// declarative or shared-endpoint router. Legacy task routes are pinned here
// from one registry generation before the adaptor is returned.
func getTaskAdaptorForRequest(c *gin.Context, platform constant.TaskPlatform) (constant.TaskPlatform, channel.TaskAdaptor) {
	if c != nil {
		if value, exists := c.Get(pluginruntime.ContextKeyPinnedPlugin); exists {
			if pinned, ok := value.(pluginruntime.PinnedPlugin); ok && pinned.Plugin != nil {
				platform = constant.TaskPlatform(pinned.Plugin.Meta.Key)
				return platform, jspluginadaptor.New(pinned.Plugin)
			}
			return platform, nil
		}
		if value, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint); exists {
			if pinned, ok := value.(pluginruntime.PinnedEndpoint); ok && pinned.Plugin != nil {
				platform = constant.TaskPlatform(pinned.Plugin.Meta.Key)
				return platform, jspluginadaptor.New(pinned.Plugin)
			}
			return platform, nil
		}
		if value, exists := c.Get(pluginruntime.ContextKeyPinnedRoute); exists {
			if pinned, ok := value.(pluginruntime.PinnedRoute); ok && pinned.Plugin != nil {
				platform = constant.TaskPlatform(pinned.Plugin.Meta.Key)
				return platform, jspluginadaptor.New(pinned.Plugin)
			}
			return platform, nil
		}
	}
	generation := pluginruntime.DefaultRegistry.Generation()
	plugin, ok := ResolveTaskPluginForPlatform(generation, platform)
	if !ok {
		return platform, getLegacyTaskAdaptor(platform)
	}
	if c != nil {
		c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{
			Generation: generation,
			Plugin:     plugin,
		})
	}
	return platform, jspluginadaptor.New(plugin)
}
