package minimax

// MiniMax 视频生成 v2 API（MiniMax-H3）。
//
// 与 v1（hailuo 渠道）是两套不兼容的协议，故单独成渠道：
//   - 提交： POST /v2/video_generation      多模态 content[] 结构
//   - 查询： GET  /v2/query/video_generation/{task_id}   （path 参数，v1 是 query 参数）
//   - 成片： task.content.url 直接给出，无需再用 file_id 换取
//   - 错误： OpenAI 风格 {type, error:{type,message,http_code}, request_id}，v2 没有 base_resp
//
// 文档：https://platform.minimaxi.com/docs/api-reference/video-generation-v2-create

const ChannelName = "minimax-video"

// ModelList 目前 v2 仅支持 MiniMax-H3。
var ModelList = []string{
	"MiniMax-H3",
}

const (
	SubmitEndpoint = "/v2/video_generation"
	QueryEndpoint  = "/v2/query/video_generation"
)

// 任务状态（v2 只有这六个值，没有 v1 的 Preparing/Processing）。
const (
	TaskStatusQueued    = "queued"
	TaskStatusRunning   = "running"
	TaskStatusSucceeded = "succeeded"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
	TaskStatusExpired   = "expired"
)

// content[].type
const (
	ContentTypeText     = "text"
	ContentTypeImageURL = "image_url"
	ContentTypeVideoURL = "video_url"
	ContentTypeAudioURL = "audio_url"
)

// content[].role
const (
	RoleFirstFrame     = "first_frame"
	RoleLastFrame      = "last_frame"
	RoleReferenceImage = "reference_image"
	RoleReferenceVideo = "reference_video"
	RoleReferenceAudio = "reference_audio"
)

const (
	// DefaultResolution v2 目前只支持 2K。
	DefaultResolution = "2K"
	// DefaultDuration 官方示例与默认值。
	DefaultDuration = 5
	// DefaultRatio 文生视频场景 ratio 必填且不能为 adaptive。
	DefaultRatio = "16:9"

	MinDuration = 4
	MaxDuration = 15
)

// 素材数量上限（官方文档）。
const (
	MaxFirstFrame      = 1
	MaxLastFrame       = 1
	MaxReferenceImages = 9
	MaxReferenceVideos = 3
	MaxReferenceAudios = 3
	// MaxTotalFiles 混合输入的总上限。
	MaxTotalFiles = 12
)

// ValidRatios 是 ratio 的完整枚举。
var ValidRatios = []string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}
