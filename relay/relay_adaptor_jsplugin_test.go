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
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel"
	taskgemini "github.com/QuantumNous/new-api/relay/channel/task/gemini"
	jspluginadaptor "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskAdaptorMapsMigratedPlatformsToFactoryPlugins(t *testing.T) {
	platforms := []constant.TaskPlatform{
		constant.TaskPlatformSuno,
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMax)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeJimeng)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVidu)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOpenAI)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVertexAi)),
	}
	for _, platform := range platforms {
		adaptor := GetTaskAdaptor(platform)
		if google, ok := adaptor.(*googleVideoTaskAdaptor); ok {
			adaptor = google.TaskAdaptor
		}
		_, isJS := adaptor.(*jspluginadaptor.TaskAdaptor)
		assert.True(t, isJS, "platform %s should use its factory plugin", platform)
	}
}

func TestGetTaskAdaptorUsesPlatformAsThirdPartyPluginKey(t *testing.T) {
	t.Cleanup(func() {
		pluginruntime.DefaultRegistry.Unregister("registry-fallback")
	})
	source := `
export const meta = {apiVersion: 1, key: "registry-fallback", name: "Registry Fallback", version: "1.0.0", author: {name: "Test"}, channelTypes: [1999], models: ["fallback-v1"], fetchMode: "per_task"};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl + "/submit"}; }
export function parseSubmitResponse(ctx, resp) { return {taskId: "id", taskData: resp.body}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl + "/tasks/" + ctx.taskId}; }
export function parseTaskResult(ctx, body) { return {taskId: body.id, status: "SUCCESS"}; }
`
	_, err := pluginruntime.DefaultRegistry.Register(source, pluginruntime.Options{})
	require.NoError(t, err)

	adaptor := GetTaskAdaptor(constant.TaskPlatform("registry-fallback"))
	require.NotNil(t, adaptor)
	assert.Equal(t, "Registry Fallback", adaptor.GetChannelName())
}

func TestGetTaskAdaptorReturnsNilForUnknownPlatform(t *testing.T) {
	assert.Nil(t, GetTaskAdaptor(constant.TaskPlatform("missing-task-platform")))
}

func TestGetTaskAdaptorForRequestUsesExactPinnedPlugin(t *testing.T) {
	source := `
export const meta = {apiVersion: 1, key: "pinned-request", name: "Pinned Generation", version: "1.0.0", author: {name: "Test"}, models: ["pinned-v1"], fetchMode: "per_task"};
export function buildSubmitRequest(ctx) { return {url: ctx.baseUrl + "/submit"}; }
export function parseSubmitResponse(ctx) { return {taskId: "one"}; }
export function buildQueryRequest(ctx) { return {url: ctx.baseUrl + "/query"}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`
	pinned, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/vendor/submit", nil)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: pinned})

	platform, adaptor := getTaskAdaptorForRequest(c, constant.TaskPlatform("missing-task-platform"), "gemini-omni-1.1-flash-preview")
	require.NotNil(t, adaptor)
	assert.Equal(t, constant.TaskPlatform("pinned-request"), platform)
	assert.Equal(t, "Pinned Generation", adaptor.GetChannelName())
}

func TestGetTaskAdaptorForRequestPinsLegacyMappedPlugin(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/video_1/remix", nil)
	legacyPlatform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora))

	platform, adaptor := getTaskAdaptorForRequest(c, legacyPlatform, "sora-2")

	require.NotNil(t, adaptor)
	assert.Equal(t, legacyPlatform, platform)
	pinnedValue, exists := c.Get(pluginruntime.ContextKeyPinnedPlugin)
	require.True(t, exists)
	pinned, ok := pinnedValue.(pluginruntime.PinnedPlugin)
	require.True(t, ok)
	require.NotNil(t, pinned.Generation)
	require.NotNil(t, pinned.Plugin)
	assert.Equal(t, "sora", pinned.Plugin.Meta.Key)
	assert.Same(t, pinned.Generation, pluginruntime.DefaultRegistry.Generation())
}

func TestGoogleOmniTasksKeepInteractionsWithVeoPluginsEnabled(t *testing.T) {
	const omniModel = "gemini-omni-1.1-flash-preview"
	for _, tc := range []struct {
		name, baseURL, key, region, wantURL string
		channelType                         int
	}{
		{"Gemini", "https://generativelanguage.googleapis.com", "test-key", "", "https://generativelanguage.googleapis.com/v1beta/interactions", constant.ChannelTypeGemini},
		{"Vertex global", "", `{"project_id":"test-project"}`, "global", "https://aiplatform.googleapis.com/v1beta1/projects/test-project/locations/global/interactions", constant.ChannelTypeVertexAi},
		{"Vertex regional", "", `{"project_id":"test-project"}`, "us-central1", "https://us-central1-aiplatform.googleapis.com/v1beta1/projects/test-project/locations/us-central1/interactions", constant.ChannelTypeVertexAi},
	} {
		for _, clientModel := range []string{omniModel, "video-alias"} {
			t.Run(tc.name+"/"+clientModel, func(t *testing.T) {
				platform := constant.TaskPlatform(strconv.Itoa(tc.channelType))
				plugin, enabled := ResolveTaskPluginForPlatform(pluginruntime.DefaultRegistry.Generation(), platform)
				require.True(t, enabled)
				require.NotNil(t, plugin)
				body := `{"model":"` + clientModel + `","prompt":"dance in the rain","seconds":"8","resolution":"720p","aspect_ratio":"16:9"}`
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				selectedPlatform, adaptor := getTaskAdaptorForRequest(c, platform, omniModel)
				assert.Equal(t, platform, selectedPlatform)
				_, pinned := c.Get(pluginruntime.ContextKeyPinnedPlugin)
				assert.False(t, pinned, "native interactions must not acquire Veo plugin identity")
				info := &relaycommon.RelayInfo{
					OriginModelName: clientModel,
					ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channelType, ChannelBaseUrl: tc.baseURL,
						ApiKey: tc.key, ApiVersion: tc.region, UpstreamModelName: omniModel},
					TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_omni"},
				}
				adaptor.Init(info)
				require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
				requestURL, err := adaptor.BuildRequestURL(info)
				require.NoError(t, err)
				assert.Equal(t, tc.wantURL, requestURL)
				requestBody, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, common.DecodeJson(requestBody, &payload))
				assert.Equal(t, omniModel, payload["model"])
				assert.Equal(t, true, payload["background"])
				assert.Equal(t, map[string]any{"type": "video", "duration": "8s", "resolution": "720p", "aspect_ratio": "16:9"}, payload["response_format"])
				assert.NotContains(t, payload, "instances")
				response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"interaction-1","status":"in_progress"}`))}
				parsed, taskErr := adaptor.ParseResponse(c, response, info)
				require.Nil(t, taskErr)
				require.NotNil(t, parsed)
				assert.True(t, taskgemini.IsOmniTaskID(parsed.UpstreamTaskID))
				task := &model.Task{TaskID: info.PublicTaskID, Platform: platform, Status: model.TaskStatusInProgress,
					Properties:  model.Properties{OriginModelName: clientModel, UpstreamModelName: omniModel},
					PrivateData: model.TaskPrivateData{UpstreamTaskID: parsed.UpstreamTaskID}}
				poller := GetTaskAdaptor(platform)
				result, err := poller.ParseTaskResult(task, nil, []byte(`{"id":"interaction-1","status":"in_progress"}`))
				require.NoError(t, err)
				assert.Equal(t, model.TaskStatusInProgress, result.Status)
				converter, ok := poller.(channel.OpenAIVideoConverter)
				require.True(t, ok)
				video, err := converter.ConvertToOpenAIVideo(task)
				require.NoError(t, err)
				var rendered map[string]any
				require.NoError(t, common.Unmarshal(video, &rendered))
				assert.Equal(t, clientModel, rendered["model"])
				assert.Equal(t, info.PublicTaskID, rendered["id"])
				assert.Equal(t, info.PublicTaskID, rendered["task_id"])

				// An explicit plugin snapshot must keep its own parser, even if an
				// upstream task identifier happens to look like a native interaction.
				if tc.channelType == constant.ChannelTypeGemini {
					poller.Init(info)
					task.PrivateData.Execution = &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: plugin.Meta.Key}}
					result, err = poller.ParseTaskResult(task, nil, []byte(`{"name":"projects/p/locations/global/operations/veo-1","done":false}`))
					require.NoError(t, err)
					assert.Equal(t, model.TaskStatusInProgress, result.Status)
				}
			})
		}
	}
}

func TestGoogleOmniPollingUsesInteractionEndpoint(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1beta/interactions/interaction-1", r.URL.Path)
		_, _ = io.WriteString(w, `{"id":"interaction-1","status":"in_progress"}`)
	}))
	defer source.Close()
	task := &model.Task{TaskID: "task_public_omni", PrivateData: model.TaskPrivateData{UpstreamTaskID: taskgemini.EncodeOmniTaskID("interaction-1")}}
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)))
	response, err := adaptor.FetchTask(source.URL, "test-key", task, "")
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}
