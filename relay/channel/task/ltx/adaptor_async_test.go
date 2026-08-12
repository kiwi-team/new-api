package ltx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestURLUsesAsyncEndpoint(t *testing.T) {
	adaptor := &TaskAdaptor{baseURL: "https://api.ltx.io/"}

	textURL, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionTextGenerate},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://api.ltx.io/v2/text-to-video", textURL)

	imageURL, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionImageGenerate},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://api.ltx.io/v2/image-to-video", imageURL)
}

func TestDoResponseStoresUpstreamJobID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(bytes.NewBufferString(`{"id":"job-123","created_at":"2026-08-12T12:00:00Z"}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "ltx-2-5-fast",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task-public"},
	}

	upstreamID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "job-123", upstreamID)
	assert.JSONEq(t, `{"id":"job-123","created_at":"2026-08-12T12:00:00Z"}`, string(taskData))

	var downstream map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &downstream))
	assert.Equal(t, "task-public", downstream["id"])
	assert.Equal(t, "task-public", downstream["task_id"])
}

func TestFetchTaskUsesSubmissionEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		taskID  string
		wantURL string
	}{
		{name: "text", action: constant.TaskActionTextGenerate, taskID: "job-123", wantURL: "https://api.ltx.io/v2/text-to-video/job-123"},
		{name: "image", action: constant.TaskActionImageGenerate, taskID: "job-123", wantURL: "https://api.ltx.io/v2/image-to-video/job-123"},
		{name: "first and last frame", action: constant.TaskActionFirstTailGenerate, taskID: "job/123", wantURL: "https://api.ltx.io/v2/image-to-video/job%2F123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantURL, ltxJobStatusURL("https://api.ltx.io/", tt.action, tt.taskID))
		})
	}
}

func TestParseTaskResultMapsLtxJobLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantStatus   model.TaskStatus
		wantProgress string
		wantURL      string
		wantReason   string
		wantUpload   string
		uploadErr    error
		wantErr      bool
	}{
		{
			name: "pending", body: `{"id":"job-1","status":"pending","created_at":"2026-08-12T12:00:00Z"}`,
			wantStatus: model.TaskStatusQueued, wantProgress: "20%",
		},
		{
			name: "processing", body: `{"id":"job-1","status":"processing","created_at":"2026-08-12T12:00:00Z"}`,
			wantStatus: model.TaskStatusInProgress, wantProgress: "30%",
		},
		{
			name: "completed", body: `{"id":"job-1","status":"completed","created_at":"2026-08-12T12:00:00Z","completed_at":"2026-08-12T12:01:00Z","result":{"video_url":"https://cdn.example/video.mp4"}}`,
			wantStatus: model.TaskStatusSuccess, wantProgress: "100%", wantURL: "https://s3.example/videos/video.mp4",
			wantUpload: "https://cdn.example/video.mp4",
		},
		{
			name: "failed", body: `{"id":"job-1","status":"failed","created_at":"2026-08-12T12:00:00Z","completed_at":"2026-08-12T12:01:00Z","error":{"type":"content_filtered_error","message":"content rejected"}}`,
			wantStatus: model.TaskStatusFailure, wantProgress: "100%", wantReason: "content rejected",
		},
		{
			name: "completed without URL", body: `{"id":"job-1","status":"completed","created_at":"2026-08-12T12:00:00Z","completed_at":"2026-08-12T12:01:00Z","result":{}}`,
			wantErr: true,
		},
		{
			name: "completed upload fails", body: `{"id":"job-1","status":"completed","created_at":"2026-08-12T12:00:00Z","completed_at":"2026-08-12T12:01:00Z","result":{"video_url":"https://cdn.example/video.mp4"}}`,
			wantUpload: "https://cdn.example/video.mp4", uploadErr: errors.New("S3 unavailable"), wantErr: true,
		},
		{
			name: "unknown status", body: `{"id":"job-1","status":"paused","created_at":"2026-08-12T12:00:00Z"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var uploadedURL string
			adaptor := &TaskAdaptor{
				uploadVideo: func(_ context.Context, remoteURL string) (string, error) {
					uploadedURL = remoteURL
					if tt.uploadErr != nil {
						return "", tt.uploadErr
					}
					return "https://s3.example/videos/video.mp4", nil
				},
			}
			result, err := adaptor.ParseTaskResult([]byte(tt.body))
			assert.Equal(t, tt.wantUpload, uploadedURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, model.TaskStatus(result.Status))
			assert.Equal(t, tt.wantProgress, result.Progress)
			assert.Equal(t, tt.wantURL, result.Url)
			assert.Equal(t, tt.wantReason, result.Reason)
		})
	}
}

func TestConvertToOpenAIVideoExposesCompletedResultURL(t *testing.T) {
	task := &model.Task{
		TaskID:      "task-public",
		Status:      model.TaskStatusSuccess,
		Progress:    "100%",
		PrivateData: model.TaskPrivateData{ResultURL: "https://s3.example/videos/video.mp4"},
		Properties:  model.Properties{OriginModelName: "ltx-2-5-fast"},
	}

	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(data, &response))
	assert.Equal(t, "task-public", response["id"])
	assert.Equal(t, "completed", response["status"])
	assert.Equal(t, "https://s3.example/videos/video.mp4", response["video_url"])
	assert.Equal(t, "https://s3.example/videos/video.mp4", response["url"])
}
