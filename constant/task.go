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
	TaskActionRemix              = "remixGenerate"
	TaskActionMultiImageGenerate = "multiImageGenerate"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
