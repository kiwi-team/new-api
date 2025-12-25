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
)

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
)

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
