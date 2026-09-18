package constant

type TaskPlatform string

const (
	TaskPlatformSuno             TaskPlatform = "suno"
	TaskPlatformMidjourney                    = "mj"
	TaskPlatformYunwuVeo                      = "yunwu-veo"
	TaskPlatformYunwuSora                     = "yunwu-sora"
	TaskPlatformPPioHunyuanImage              = "ppio-hunyuan-image"
	TaskPlatformNovitaImage                   = "novita-image"
	TaskPlatformHunyuanImage                  = "hunyuan-image"
	TaskPlatformFALImage                      = "fal-image"
	TaskPlatformPPio                          = "ppio"
	TaskPlatformFAL                           = "fal"
	TaskPlatformPixverse                      = "pixverse"
	TaskPlatformLtx                           = "ltx"
)

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate           = "generate"
	TaskActionTextGenerate       = "textGenerate"
	TaskActionImageGenerate      = "imageGenerate" // image to video
	TaskActionFirstTailGenerate  = "firstTailGenerate"
	TaskActionReferenceGenerate  = "referenceGenerate"
	TaskActionRemixGenerate      = "remixGenerate"
	TaskActionMultiImageGenerate = "multiImageGenerate"
	TaskActionImageToVideo       = "image_to_video"
	TaskActionTextToVideo        = "text_to_video"
	TaskActionFirstTailToVideo   = "first_tail_to_video"
	TaskActionReferenceToVideo   = "reference_to_video"
	TaskActionRemix              = "remix"
)

var legacyTaskActionAliases = map[string]string{
	"generate":          TaskActionImageToVideo,
	"textGenerate":      TaskActionTextToVideo,
	"firstTailGenerate": TaskActionFirstTailToVideo,
	"referenceGenerate": TaskActionReferenceToVideo,
	"remixGenerate":     TaskActionRemix,
}

// TaskPluginEnabled is the master switch for the whole task-plugin system.
// When disabled, factory and override plugins both stop serving.
var TaskPluginEnabled = true

// NormalizeTaskAction maps persisted legacy action names to the canonical task
// action vocabulary. Unknown platform-specific actions pass through unchanged.
func NormalizeTaskAction(action string) string {
	if canonical, ok := legacyTaskActionAliases[action]; ok {
		return canonical
	}
	return action
}
