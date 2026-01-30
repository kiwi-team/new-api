package fal_sync

import "time"

const (
	ChannelName         = "fal_sync"
	DefaultModel        = "flux-2-pro"
	DefaultPollInterval = 1 * time.Second
	DefaultTimeout      = 300 * time.Second
	MaxRetries          = 3
)

// ModelList contains the list of supported FAL models
var ModelList = []string{
	"flux-2-pro",
	"hunyuan-image-v3",
	"qwen-image-max",
}
