package fal

import (
	"bytes"
	"io"

	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

/*
	{
	  "prompt": "A stark starting line divides two powerful cars, engines revving for the challenge ahead. They surge forward in the heat of competition, a blur of speed and chrome. The finish line looms as they race for victory.",
	  "num_inference_steps": 28,
	  "aspect_ratio": "16:9",
	  "resolution": "480p",
	  "num_frames": 121,
	  "enable_prompt_expansion": true,
	  "image_url": "https://v3.fal.media/files/panda/HnY2yf-BbzlrVQxR-qP6m_9912d0932988453aadf3912fc1901f52.jpg"
	}

	{
		"detail": "Request is still in progress",
		"request_id": "d9ba7295-4572-49f3-bdb5-cab6bab16381",
		"response_url": "https://queue.fal.run/fal-ai/hunyuan-video-v1.5/requests/d9ba7295-4572-49f3-bdb5-cab6bab16381",
		"status_url": "https://queue.fal.run/fal-ai/hunyuan-video-v1.5/requests/d9ba7295-4572-49f3-bdb5-cab6bab16381/status",
		"cancel_url": "https://queue.fal.run/fal-ai/hunyuan-video-v1.5/requests/d9ba7295-4572-49f3-bdb5-cab6bab16381/cancel"
	}
*/
type HunyuanImage2VideoTaskSubmitRequest struct {
	Prompt                string `json:"prompt"`
	NumInferenceSteps     int    `json:"num_inference_steps,omitempty"`
	AspectRatio           string `json:"aspect_ratio,omitempty"`
	ImageURL              string `json:"image_url,omitempty"`
	Resolution            string `json:"resolution,omitempty"`
	NumFrames             int    `json:"num_frames,omitempty"`
	EnablePromptExpansion bool   `json:"enable_prompt_expansion,omitempty"`
	NegativePrompt        string `json:"negative_prompt,omitempty"`
	Seed                  int    `json:"seed,omitempty"`
}

// BuildRequestBody converts request into Vertex specific format.
func HunyuanImage2VideoRequestBody(req relaycommon.TaskSubmitReq) (io.Reader, error) {
	if req.Image == "" {
		return nil, errors.New("image is required")
	}

	body := HunyuanImage2VideoTaskSubmitRequest{
		Prompt: req.Prompt,
	}
	if req.Image != "" {
		body.ImageURL = req.Image
	}

	// 同步扩展字段的厂商自定义metadata
	if req.Metadata != nil {

		metadata := req.Metadata
		medaBytes, err := common.Marshal(metadata)
		if err != nil {
			return nil, errors.Wrap(err, "metadata marshal metadata failed")
		}
		err = common.Unmarshal(medaBytes, &body)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
