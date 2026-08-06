package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 级联部署（本实例作为另一台 new-api 的 openai 类型上游）时，下游的 sora adaptor
// 只从 /v1/videos/{id} 响应的顶层 video_url / url / result_url 读取视频地址。
// 这里漏输出，下游就只能退回拼接它自己的 /v1/videos/{id}/content 代理地址。
func TestConvertToOpenAIVideoExposesDirectLink(t *testing.T) {
	const proxyTaskID = "task_abc"
	cases := []struct {
		name    string
		status  model.TaskStatus
		result  string
		wantURL string
	}{
		{
			name:    "s3 direct link is exposed",
			status:  model.TaskStatusSuccess,
			result:  "https://bucket.s3.example.com/videos/1.mp4",
			wantURL: "https://bucket.s3.example.com/videos/1.mp4",
		},
		{
			name:   "own content proxy url is not exposed",
			status: model.TaskStatusSuccess,
			result: "https://mixrouter.example.com/v1/videos/" + proxyTaskID + "/content",
		},
		{
			name:   "unfinished task exposes nothing",
			status: model.TaskStatusInProgress,
			result: "https://bucket.s3.example.com/videos/1.mp4",
		},
	}

	adaptor := &TaskAdaptor{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: proxyTaskID, Status: tc.status}
			task.PrivateData.ResultURL = tc.result

			body, err := adaptor.ConvertToOpenAIVideo(task)
			require.NoError(t, err)

			assert.Equal(t, tc.wantURL, gjson.GetBytes(body, "video_url").String())
			assert.Equal(t, tc.wantURL, gjson.GetBytes(body, "url").String())
		})
	}
}

// 非 Omni 任务的 model 字段过去被内层的 := 遮蔽，恒为空字符串，
// 下游按 model 计费或路由时拿不到模型名。
func TestConvertToOpenAIVideoReportsModel(t *testing.T) {
	task := &model.Task{TaskID: "task_abc", Status: model.TaskStatusSuccess}
	task.PrivateData.UpstreamTaskID = taskcommon.EncodeLocalTaskID(
		"models/veo-3.1-generate-001/operations/xyz")

	body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assert.Equal(t, "veo-3.1-generate-001", gjson.GetBytes(body, "model").String())
}
