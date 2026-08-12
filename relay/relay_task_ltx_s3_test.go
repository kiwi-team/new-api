package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLtxVideoGenerationsResponseUsesStoredS3URL(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_ltx_s3",
		Platform: constant.TaskPlatform("64"),
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://bucket.s3.example/videos/generated.mp4",
		},
	}

	body := simpleVideoRespBody(task, nil, "")
	var response struct {
		Code string `json:"code"`
		Data struct {
			Status string `json:"status"`
			TaskID string `json:"task_id"`
			URL    string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "success", response.Code)
	assert.Equal(t, "succeeded", response.Data.Status)
	assert.Equal(t, "task_ltx_s3", response.Data.TaskID)
	assert.Equal(t, "https://bucket.s3.example/videos/generated.mp4", response.Data.URL)
}
