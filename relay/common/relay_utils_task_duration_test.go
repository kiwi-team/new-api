package common

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// duration=-1 是「由模型自选时长」的约定值，只对支持它的模型系列放行；
// 其余负值与超上限值必须仍被 400 拦截，避免负数/超大时长进入计费倍率。
func TestValidateTaskDurationBounds(t *testing.T) {
	cases := []struct {
		name     string
		req      TaskSubmitReq
		wantErr  bool
		wantCode string
	}{
		{name: "normal duration", req: TaskSubmitReq{Model: "doubao-seedance-1-0-pro", Duration: 5}},
		{name: "unset duration", req: TaskSubmitReq{Model: "doubao-seedance-1-0-pro"}},
		{name: "seconds string", req: TaskSubmitReq{Model: "doubao-seedance-1-0-pro", Seconds: "10"}},
		{name: "seedance2 model-chosen duration", req: TaskSubmitReq{Model: "doubao-seedance-2-0-pro", Duration: -1}},
		{name: "wan3 model-chosen duration", req: TaskSubmitReq{Model: "wan3.0-video", Duration: -1}},
		{
			name:     "model-chosen duration rejected for other models",
			req:      TaskSubmitReq{Model: "doubao-seedance-1-0-pro", Duration: -1},
			wantErr:  true,
			wantCode: "invalid_seconds",
		},
		{
			name:     "other negative rejected for seedance2",
			req:      TaskSubmitReq{Model: "doubao-seedance-2-0-pro", Duration: -2},
			wantErr:  true,
			wantCode: "invalid_seconds",
		},
		{
			name:     "over max rejected",
			req:      TaskSubmitReq{Model: "doubao-seedance-2-0-pro", Duration: MaxTaskDurationSeconds + 1},
			wantErr:  true,
			wantCode: "invalid_seconds",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateTaskDurationBounds(tc.req)
			if !tc.wantErr {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.wantCode, got.Code)
			assert.Equal(t, http.StatusBadRequest, got.StatusCode)
		})
	}
}
