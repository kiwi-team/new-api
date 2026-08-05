package ppio

import (
	"bytes"
	"io"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// BuildRequestBody converts request into Vertex specific format.
func WanRequestBody(req relaycommon.TaskSubmitReq) (io.Reader, error) {

	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 5
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}

	body := WanTaskSubmitRequest{
		Input: &TaskInput{
			Prompt:   req.Prompt,
			ImageURL: req.Image,
		},
		Parameters: &TaskParameters{
			Duration: seconds,
		},
	}

	// 同步扩展字段的厂商自定义metadata
	if req.Metadata != nil {
		if v, ok := req.Metadata["aspect_ratio"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Parameters.Size = s
			}
		} else {
			body.Parameters.Size = "1920*1080"
		}
		if v, ok := req.Metadata["negative_prompt"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Input.NegativePrompt = s
			}
		}
		if v, ok := req.Metadata["shot_type"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Parameters.ShotType = s
			}
		}
		if v, ok := req.Metadata["prompt_extend"]; ok {
			if s, ok := v.(bool); ok {
				body.Parameters.PromptExtend = s
			}
		}
		if v, ok := req.Metadata["resolution"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Parameters.Resolution = s
			}
		}
		if v, ok := req.Metadata["seed"]; ok {
			if s, ok := v.(int); ok {
				body.Parameters.Seed = s
			}
		}
		if v, ok := req.Metadata["watermark"]; ok {
			if s, ok := v.(bool); ok {
				body.Parameters.Watermark = s
			}
		}
		if v, ok := req.Metadata["audio"]; ok {
			if s, ok := v.(bool); ok {
				body.Parameters.Audio = s
			}
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
