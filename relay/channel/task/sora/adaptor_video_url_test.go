package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 成功任务的 S3 直链存在 FailReason 里。OpenAI 原生 /v1/videos/{id} 不吐地址，
// 级联下游只能读顶层 video_url/url，所以这里必须合并进去。
func TestConvertToOpenAIVideoMergesDirectURL(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{
		TaskID:     "task_abc",
		Status:     model.TaskStatusSuccess,
		FailReason: "https://s3.example.com/v.mp4",
		Data:       []byte(`{"id":"upstream","status":"queued"}`),
	}

	data, err := a.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assert.Equal(t, "task_abc", gjson.GetBytes(data, "id").String())
	assert.Equal(t, "https://s3.example.com/v.mp4", gjson.GetBytes(data, "video_url").String())
	assert.Equal(t, "https://s3.example.com/v.mp4", gjson.GetBytes(data, "url").String())
	assert.Equal(t, "completed", gjson.GetBytes(data, "status").String())
}

// 上游已经给了地址就不要覆盖，也不要改动非成功任务。
func TestConvertToOpenAIVideoLeavesExistingAndNonSuccess(t *testing.T) {
	a := &TaskAdaptor{}

	existing := &model.Task{
		TaskID:     "task_abc",
		Status:     model.TaskStatusSuccess,
		FailReason: "https://s3.example.com/v.mp4",
		Data:       []byte(`{"id":"upstream","video_url":"https://upstream/v.mp4"}`),
	}
	data, err := a.ConvertToOpenAIVideo(existing)
	require.NoError(t, err)
	assert.Equal(t, "https://upstream/v.mp4", gjson.GetBytes(data, "video_url").String())

	failed := &model.Task{
		TaskID:     "task_abc",
		Status:     model.TaskStatusFailure,
		FailReason: "upstream rejected",
		Data:       []byte(`{"id":"upstream","status":"failed"}`),
	}
	data, err = a.ConvertToOpenAIVideo(failed)
	require.NoError(t, err)
	assert.Empty(t, gjson.GetBytes(data, "video_url").String())
	assert.Equal(t, "failed", gjson.GetBytes(data, "status").String())
}
