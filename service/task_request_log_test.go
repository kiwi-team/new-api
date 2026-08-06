package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 异步任务的提交请求体要和同步链路一样能落到消费日志的「请求体」列，
// 开关同为 SAVE_REQUEST_RESPONSE。multipart 提交的原始 body 带二进制分段，
// 直接落 TEXT 列不可读，改记归一化后的 TaskSubmitReq。
func TestTaskRequestLogBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const jsonBody = `{"model":"sora-2","prompt":"a cat","seconds":"5"}`

	cases := []struct {
		name        string
		saveEnabled bool
		contentType string
		body        string
		taskRequest *relaycommon.TaskSubmitReq
		want        string
	}{
		{
			name:        "disabled records nothing",
			saveEnabled: false,
			contentType: "application/json",
			body:        jsonBody,
			want:        "",
		},
		{
			name:        "json body recorded verbatim",
			saveEnabled: true,
			contentType: "application/json",
			body:        jsonBody,
			want:        jsonBody,
		},
		{
			name:        "multipart falls back to normalized task request",
			saveEnabled: true,
			contentType: "multipart/form-data; boundary=----x",
			body:        "------x\r\nContent-Disposition: form-data; name=\"prompt\"\r\n\r\na cat\r\n------x--\r\n",
			taskRequest: &relaycommon.TaskSubmitReq{Model: "sora-2", Prompt: "a cat", Seconds: "5"},
			want:        `{"prompt":"a cat","model":"sora-2","seconds":"5"}`,
		},
		{
			name:        "multipart without parsed request records nothing",
			saveEnabled: true,
			contentType: "multipart/form-data; boundary=----x",
			body:        "------x--\r\n",
			want:        "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.saveEnabled {
				t.Setenv("SAVE_REQUEST_RESPONSE", "true")
			} else {
				t.Setenv("SAVE_REQUEST_RESPONSE", "")
			}

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", tc.contentType)

			storage, err := common.CreateBodyStorage([]byte(tc.body))
			require.NoError(t, err)
			t.Cleanup(func() { storage.Close() })
			c.Set(common.KeyBodyStorage, storage)

			if tc.taskRequest != nil {
				c.Set("task_request", *tc.taskRequest)
			}

			assert.Equal(t, tc.want, TaskRequestLogBody(c))
		})
	}
}
