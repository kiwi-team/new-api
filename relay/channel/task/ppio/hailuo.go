package ppio

import (
	"bytes"
	"errors"
	"io"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

/*
	{
	  "prompt": "<string>",
	  "image": "<string>",
	  "duration": 123,
	  "resolution": "<string>",
	  "enable_prompt_expansion": true
	}
*/
type HailuoTaskSubmitRequest struct {
	Prompt                string `json:"prompt"`
	Image                 string `json:"image,omitempty"`
	Duration              int    `json:"duration,omitempty"`
	Resolution            string `json:"resolution,omitempty"`
	EnablePromptExpansion bool   `json:"enable_prompt_expansion,omitempty"`
}

func HailuoRequestBody(req relaycommon.TaskSubmitReq) (io.Reader, error) {

	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 4
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}
	// https://api.ppinfra.com/v3/async/minimax-hailuo-2.3-i2v
	if req.Model == "hailuo-2.3-i2v" && req.Image == "" {
		return nil, errors.New("image is required")
	}

	body := HailuoTaskSubmitRequest{
		Prompt:   req.Prompt,
		Duration: seconds,
		Image:    req.Image,
	}

	// 同步扩展字段的厂商自定义metadata
	if req.Metadata != nil {
		if v, ok := req.Metadata["enable_prompt_expansion"]; ok {
			if s, ok := v.(bool); ok {
				body.EnablePromptExpansion = s
			}
		}
		if v, ok := req.Metadata["resolution"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Resolution = s
			}
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
