package kling

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
)

// Omni 按 时长 × 清晰度 计价：时长必须始终产出倍率，
// 清晰度只有非 1 倍时才写入（720p 是基准档）。
func TestOmniPriceRatios(t *testing.T) {
	cases := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want map[string]float64
	}{
		{
			name: "defaults to 5s at 720p",
			req:  relaycommon.TaskSubmitReq{Model: "kling-3.0-omni"},
			want: map[string]float64{"seconds": 5},
		},
		{
			name: "explicit resolution field",
			req:  relaycommon.TaskSubmitReq{Model: "kling-3.0-omni", Duration: 10, Resolution: "1080P"},
			want: map[string]float64{"seconds": 10, "resolution-1080p": 2},
		},
		{
			name: "resolution from metadata",
			req: relaycommon.TaskSubmitReq{
				Model:    "kling-3.0-omni",
				Seconds:  "8",
				Metadata: map[string]interface{}{"resolution": "4k"},
			},
			want: map[string]float64{"seconds": 8, "resolution-4k": 4},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			assert.Equal(t, tc.want, omniPriceRatios(&req))
		})
	}
}
