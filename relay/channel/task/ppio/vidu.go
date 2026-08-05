package ppio

import (
	"bytes"
	"errors"
	"io"
	"slices"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// BuildRequestBody converts request into Vertex specific format.
func ViduRequestBody(req relaycommon.TaskSubmitReq) (io.Reader, error) {

	seconds := common.String2Int(req.Seconds)
	if seconds <= 0 {
		seconds = 4
	}
	if req.Duration > 0 {
		seconds = req.Duration
	}
	if req.Image == "" {
		return nil, errors.New("image is required")
	}
	if !slices.Contains([]int{4, 8}, seconds) {
		return nil, errors.New("duration must be 4 or 8 seconds")
	}

	body := ViduTaskSubmitRequest{
		Prompt:   req.Prompt,
		Duration: seconds,
	}
	if req.Image != "" {
		body.Images = append(body.Images, req.Image)
	}

	// 同步扩展字段的厂商自定义metadata
	if req.Metadata != nil {
		if v, ok := req.Metadata["bgm"]; ok {
			if s, ok := v.(bool); ok {
				body.BGM = s
			}
		}
		if v, ok := req.Metadata["movement_amplitude"]; ok {
			if s, ok := v.(string); ok {
				body.MovementAmplitude = s
			}
		}
		if v, ok := req.Metadata["resolution"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Resolution = s
			}
		}
		if v, ok := req.Metadata["seed"]; ok {
			if s, ok := v.(int); ok {
				body.Seed = s
			}
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
