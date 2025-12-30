package pixverse

import "time"

// https://docs.platform.pixverse.ai/image-to-video-generation-13016633e0
type ToVideoRequest struct {
	// Aspect ratio (16:9, 4:3, 1:1, 3:4, 9:16)
	AspectRatio string `json:"aspect_ratio"`
	// Video duration
	// v.3.5/v4/v4.5 : 5/8 (v3.5 1080p cannot use 8)
	// v5 : 5/8
	// v5.5 : 5/8/10 (1080p cannot use 10)
	Duration int64 `json:"duration"`
	// Image ID from Upload image API. single image or single-image templates
	ImgID int64 `json:"img_id"`
	// img_ids is only for multi-image templates. ex) "img_ids ":[0,0]
	ImgIDS []int64 `json:"img_ids,omitempty"`
	// Only in v5.5, Audio switch. Controls whether the video has multiple clips or a single
	// clip. **true**: Audio on , **false**: Audio off
	GenerateAudioSwitch *bool `json:"generate_audio_switch,omitempty"`
	// Only in v5.5, Single or multi-clip switch. controls single-clip and multi-clip generation
	// modes. **true**: Multi-clip , **false**: Single-clip
	GenerateMultiClipSwitch *bool `json:"generate_multi_clip_switch,omitempty"`
	// Set to true if you want to enable this feature. Default is false.
	LipSyncSwitch *bool `json:"lip_sync_switch,omitempty"`
	// ~140 (UTF-8) characters
	LipSyncTTSContent *string `json:"lip_sync_tts_content,omitempty"`
	// id from Get speech tts list
	LipSyncTTSSpeakerID *string `json:"lip_sync_tts_speaker_id,omitempty"`
	// Model version (now supports v3.5/v4/v4.5/v5/v5.5)
	Model string `json:"model"`
	// Motion mode (normal, fast, --fast only available when duration=5; --quality=1080p does
	// not support fast) , not supports on v5
	MotionMode *string `json:"motion_mode,omitempty"`
	// Negative prompt
	NegativePrompt *string `json:"negative_prompt,omitempty"`
	// Prompt
	Prompt string `json:"prompt"`
	// Video quality ("360p"(Turbo model), "540p", "720p", "1080p")
	Quality string `json:"quality"`
	// Random seed, range: 0 - 2147483647
	Seed               *int64  `json:"seed,omitempty"`
	SoundEffectContent *string `json:"sound_effect_content,omitempty"`
	SoundEffectSwitch  *bool   `json:"sound_effect_switch,omitempty"`
	// Style (effective when model=v3.5, "anime", "3d_animation", "clay", "comic", "cyberpunk")
	// Do not include style parameter unless needed
	Style *string `json:"style,omitempty"`
	// Template ID (template_id must be activated before use)
	TemplateID *int64 `json:"template_id,omitempty"`
	// Only in v5.5, Prompt reasoning enhancement. Controls whether the system should enhance
	// your prompt with internal reasoning and optimization. **"enabled"** : Turn on
	// system-level optimization. **"disabled"** : Turn off system-level optimization.
	// **"auto"** or **omitted**: Let the model decide automatically
	ThinkingType *string `json:"thinking_type,omitempty"`
}

/*
	{
	    "ErrCode": 0,
	    "ErrMsg": "string",
	    "Resp": {
	        "video_id": 0
	    }
	}
*/
type ToVideoResponse struct {
	ErrCode int64  `json:"ErrCode"`
	ErrMsg  string `json:"ErrMsg"`
	Resp    struct {
		VideoID int64 `json:"video_id"`
	} `json:"Resp"`
}

const (
	VideoStatusGenerationSuccessful     = 1
	VideoStatusGenerating               = 5
	VideoStatusDeleted                  = 6
	VideoStatusContentsModerationFailed = 7
	VideoStatusGenerationFailed         = 8
)

type VideoTaskResult struct {
	ErrCode int    `json:"ErrCode"`
	ErrMsg  string `json:"ErrMsg"`
	Resp    struct {
		ID              int64     `json:"id,omitempty"`
		Prompt          string    `json:"prompt,omitempty"`
		NegativePrompt  string    `json:"negative_prompt,omitempty"`
		ResolutionRatio int       `json:"resolution_ratio,omitempty"`
		URL             string    `json:"url,omitempty"`
		Size            int       `json:"size,omitempty"`
		Seed            int       `json:"seed,omitempty"`
		Status          int       `json:"status,omitempty"` //video status: Generation succesful = 1; Generating=5; Deleted = 6; Contents moderation failed = 7; Generation failed= 8;
		Style           string    `json:"style,omitempty"`
		CreateTime      time.Time `json:"create_time,omitempty"`
		ModifyTime      time.Time `json:"modify_time,omitempty"`
		OutputWidth     int       `json:"outputWidth,omitempty"`
		OutputHeight    int       `json:"outputHeight,omitempty"`
		HasAudio        bool      `json:"has_audio,omitempty"`
		Credits         int       `json:"credits,omitempty"`
		CustomerPaths   struct {
			ExtInfo        any    `json:"ext_info,omitempty"`
			AgentInfo      any    `json:"agent_info,omitempty"`
			CameoIDList    any    `json:"cameo_id_list,omitempty"`
			DurationType   string `json:"duration_type,omitempty"`
			ThinkingType   string `json:"thinking_type,omitempty"`
			FusionIDList   any    `json:"fusion_id_list,omitempty"`
			ModelEvaluator string `json:"model_evaluator,omitempty"`
		} `json:"customer_paths,omitzero"`
	} `json:"Resp,omitzero"`
}

type UploadResponse struct {
	ErrCode int    `json:"ErrCode"`
	ErrMsg  string `json:"ErrMsg"`
	Resp    struct {
		ImgID  int64  `json:"img_id"`
		ImgURL string `json:"img_url"`
	} `json:"Resp"`
}
