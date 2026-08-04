package relay

import (
	"strconv"

	"github.com/QuantumNous/new-api/constant"
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
	channelType := c.GetInt("channel_type")
	if channelType > 0 {
		return constant.TaskPlatform(strconv.Itoa(channelType))
	}
	return constant.TaskPlatform(c.GetString("platform"))
}

func GetTaskAdaptor(platform constant.TaskPlatform) channel.TaskAdaptor {
	switch platform {
	//case constant.APITypeAIProxyLibrary:
	//	return &aiproxy.Adaptor{}
	case constant.TaskPlatformSuno:
		return &suno.TaskAdaptor{}
	// 聚合上游（云雾 / PPInfra / Novita / 腾讯云 / FAL）：这些渠道复用通用
	// ChannelType，只能靠 base URL 区分，所以用独立的 TaskPlatform 标识。
	// 平台由 resolveTaskPlatform 判定一次并落库到 task，fetch 阶段直接按平台派发。
	case constant.TaskPlatformYunwuVeo:
		return &taskVertexYunwu.TaskAdaptor{}
	case constant.TaskPlatformYunwuSora:
		return &taskSoraYunwu.TaskAdaptor{}
	case constant.TaskPlatformPPio:
		return &taskPPio.TaskAdaptor{}
	case constant.TaskPlatformPPioHunyuanImage:
		return &taskHunyuanPPio.TaskAdaptor{}
	case constant.TaskPlatformNovitaImage:
		return &taskNovita.TaskAdaptor{}
	case constant.TaskPlatformHunyuanImage:
		return &taskHunyuan.TaskAdaptor{}
	case constant.TaskPlatformFAL, constant.TaskPlatformFALImage:
		return &taskFal.TaskAdaptor{}
	}
	if channelType, err := strconv.ParseInt(string(platform), 10, 64); err == nil {
		switch channelType {
		case constant.ChannelTypeAli:
			return &taskali.TaskAdaptor{}
		case constant.ChannelTypeKling:
			return &kling.TaskAdaptor{}
		case constant.ChannelTypeJimeng:
			return &taskjimeng.TaskAdaptor{}
		case constant.ChannelTypeVertexAi:
			return &taskvertex.TaskAdaptor{}
		case constant.ChannelTypeVidu:
			return &taskVidu.TaskAdaptor{}
		case constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine:
			return &taskdoubao.TaskAdaptor{}
		case constant.ChannelTypeSora, constant.ChannelTypeOpenAI:
			return &tasksora.TaskAdaptor{}
		case constant.ChannelTypeGemini:
			return &taskGemini.TaskAdaptor{}
		case constant.ChannelTypeMiniMaxVideo:
			return &taskMiniMax.TaskAdaptor{}
		case constant.ChannelTypeMiniMax:
			return &hailuo.TaskAdaptor{}
		case constant.ChannelTypePixverse:
			return &taskPixverse.TaskAdaptor{}
		case constant.ChannelTypeLtx:
			return &taskLtx.TaskAdaptor{}
		case constant.ChannelTypeWorldLabs:
			return &taskWorldLabs.TaskAdaptor{}
		case constant.ChannelTypeRunwayML:
			return &taskRunwayML.TaskAdaptor{}
		case constant.ChannelTypeReplicate:
			return &taskReplicateTask.TaskAdaptor{}
		case constant.ChannelTypeHedra:
			return &taskHedra.TaskAdaptor{}
		case constant.ChannelTypeHeyGen:
			return &taskHeyGen.TaskAdaptor{}
		case constant.ChannelTypeFAL:
			return &taskFal.TaskAdaptor{}
		case constant.ChannelTypePPIO:
			return &taskPPio.TaskAdaptor{}
		case constant.ChannelTypeTencent:
			return &taskHunyuan.TaskAdaptor{}
		}
	}
	return nil
}
