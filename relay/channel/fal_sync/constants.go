package fal_sync

import "time"

const (
	ChannelName         = "fal_sync"
	DefaultModel        = "flux-2-pro"
	Flux3Model          = "flux-3-image"
	DefaultPollInterval = 1 * time.Second
	DefaultTimeout      = 300 * time.Second
	MaxRetries          = 3

	flux3TextToImageEndpoint = "blackforestlabs/flux-3/text-to-image"
	flux3EditImageEndpoint   = "blackforestlabs/flux-3/edit-image"
	flux3MaxReferenceImages  = 10
)

// ModelList contains the list of supported FAL models
var ModelList = []string{
	"flux-2-pro",
	Flux3Model,
	"hunyuan-image-v3",
	"qwen-image-max",
	"gemini-3.1-flash-image-preview",
	"gpt-image-2",
}

// FAL CDN upload constants. See https://fal.ai/docs/documentation/development/file-storage
const (
	// falTokenURL issues a short-lived token for uploading to the fal CDN (v3).
	falTokenURL = "https://rest.fal.ai/storage/auth/token?storage_type=fal-cdn-v3"
	// falCDNUploadURL is the simple (single-shot) upload endpoint for files < 100MB.
	falCDNUploadURL = "https://v3.fal.media/files/upload"
)
