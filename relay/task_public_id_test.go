package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// task.TaskID 存的是预生成的 task_xxxx 公开 ID，上游真实 ID 只进 PrivateData.UpstreamTaskID。
// 全链路的查询（GET /v1/videos/:id、/content 代理、remix）都只按 task_id 列查，没有任何
// 按上游 ID 的反查，所以 DoResponse 必须：对外响应写 PublicTaskID，返回值给上游 ID。
// 任何一边写反，用户拿到的 ID 就查不到自己的任务。
func TestTaskAdaptorDoResponseReturnsPublicIDToClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name           string
		channelType    int
		model          string
		upstreamBody   string
		wantUpstreamID string
	}{
		{
			name:           "runwayml",
			channelType:    constant.ChannelTypeRunwayML,
			model:          "gen4_turbo",
			upstreamBody:   `{"id":"runway-upstream-1"}`,
			wantUpstreamID: "runway-upstream-1",
		},
		{
			name:           "hedra",
			channelType:    constant.ChannelTypeHedra,
			model:          "hedra-character-3",
			upstreamBody:   `{"id":"hedra-upstream-1","status":"queued"}`,
			wantUpstreamID: "hedra-upstream-1",
		},
		{
			name:           "heygen",
			channelType:    constant.ChannelTypeHeyGen,
			model:          "heygen-avatar",
			upstreamBody:   `{"code":100,"data":{"video_id":"heygen-upstream-1"}}`,
			wantUpstreamID: "heygen-upstream-1",
		},
		{
			name:           "minimax",
			channelType:    constant.ChannelTypeMiniMaxVideo,
			model:          "MiniMax-Hailuo-02",
			upstreamBody:   `{"task_id":"minimax-upstream-1"}`,
			wantUpstreamID: "minimax-upstream-1",
		},
		{
			name:           "pixverse",
			channelType:    constant.ChannelTypePixverse,
			model:          "pixverse-v4",
			upstreamBody:   `{"ErrCode":0,"ErrMsg":"","Resp":{"video_id":987654}}`,
			wantUpstreamID: "987654",
		},
		{
			name:           "replicate",
			channelType:    constant.ChannelTypeReplicate,
			model:          "replicate-video",
			upstreamBody:   `{"id":"replicate-upstream-1","status":"starting"}`,
			wantUpstreamID: "replicate-upstream-1",
		},
		{
			name:           "worldlabs",
			channelType:    constant.ChannelTypeWorldLabs,
			model:          "marble-1",
			upstreamBody:   `{"done":false,"operation_id":"worldlabs-upstream-1"}`,
			wantUpstreamID: "worldlabs-upstream-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(tc.channelType)))
			require.NotNil(t, adaptor)

			const publicTaskID = "task_publicidfortest"
			info := &relaycommon.RelayInfo{
				OriginModelName: tc.model,
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: publicTaskID},
			}

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)

			upstreamResp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tc.upstreamBody)),
			}

			upstreamID, _, taskErr := adaptor.DoResponse(c, upstreamResp, info)
			require.Nil(t, taskErr)

			// 返回值进 PrivateData.UpstreamTaskID，轮询靠它跟上游对话。
			assert.Equal(t, tc.wantUpstreamID, upstreamID)

			var clientResp map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &clientResp))
			assert.Equal(t, publicTaskID, clientResp["id"])
			assert.Equal(t, publicTaskID, clientResp["task_id"])
		})
	}
}
