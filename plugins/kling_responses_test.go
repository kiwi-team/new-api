package plugins_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	pluginadaptor "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKlingResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "kling",
		model:     "kling-v2-master",
		requestBody: map[string]any{
			"model": "kling-v2-master",
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "camera orbit"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/frame.png"},
			}}},
			"seconds": 10,
			"metadata": map[string]any{
				"mode": "pro",
			},
		},
		wantAction: "image_to_video",
		wantRequest: map[string]any{
			"model":    "kling-v2-master",
			"prompt":   "camera orbit",
			"image":    "https://cdn.example/frame.png",
			"duration": float64(10),
			"metadata": map[string]any{"mode": "pro"},
		},
		wantUsageKeys:  []string{"units"},
		wantVendorName: "kling",
	})
}

func TestKling4Video(t *testing.T) {
	source, err := builtinplugins.Source("kling")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "kling"})
	require.NoError(t, err)
	binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", "kling-4.0")
	require.True(t, found)
	assert.Same(t, plugin, binding.Plugin)
	_, found = registry.Generation().LookupEndpoint("POST", "/v1/responses", "kling-4.0")
	assert.False(t, found)

	for _, tc := range []struct {
		name, action, path string
		body               map[string]any
		videoInput         string
	}{
		{"text", "text_to_video", "/text-to-video/kling-4.0", map[string]any{"prompt": "ocean waves"}, "none"},
		{"image", "image_to_video", "/image-to-video/kling-4.0", map[string]any{"input_reference": "https://cdn.example/first.png"}, "none"},
		{"keyframes", "image_to_video", "/image-to-video/kling-4.0", map[string]any{"references": []any{
			map[string]any{"type": "image", "role": "first_frame", "url": "https://cdn.example/first.png", "id": "a"},
			map[string]any{"type": "image", "role": "middle_frame", "url": "https://cdn.example/middle.png", "id": "b"},
			map[string]any{"type": "image", "role": "last_frame", "url": "https://cdn.example/last.png", "id": "c"},
		}}, "none"},
		{"reference video", "omni_video", "/omni-video/kling-4.0", map[string]any{"references": []any{
			map[string]any{"type": "video", "role": "reference_video", "url": "https://cdn.example/ref.mp4", "id": "video"},
			map[string]any{"type": "image", "role": "reference_image", "url": "https://cdn.example/ref.png", "id": "image"},
		}}, "video"},
		{"element and voice", "omni_video", "/omni-video/kling-4.0", map[string]any{"references": []any{
			map[string]any{"type": "element", "role": "element", "element_id": "element-1", "id": "actor"},
			map[string]any{"type": "voice", "role": "voice", "voice_id": "voice-1", "id": "voice"},
		}}, "unknown"},
		{"image ratio uses omni", "omni_video", "/omni-video/kling-4.0", map[string]any{"image": "https://cdn.example/first.png", "aspect_ratio": "21:9"}, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.body["model"], tc.body["seconds"] = "video-alias", "10"
			value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "video-alias", "upstreamModel": "kling-4.0", "body": map[string]any{"kind": "json", "value": tc.body},
			})
			require.NoError(t, err)
			intent := value.(map[string]any)
			assert.Equal(t, "video-alias", intent["model"])
			assert.Equal(t, tc.action, intent["action"])
			ctx := map[string]any{"model": "video-alias", "upstreamModel": "kling-4.0", "baseUrl": "https://api.example", "apiKey": "sk-official-key", "upstream": map[string]any{"kind": "vendor"}, "requestBody": intent["requestBody"]}
			value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			descriptor := value.(map[string]any)
			assert.Equal(t, "https://api.example"+tc.path, descriptor["url"])
			assert.Equal(t, "Bearer sk-official-key", descriptor["headers"].(map[string]any)["Authorization"])
			body := descriptor["body"].(map[string]any)
			assert.NotContains(t, body, "model_name")
			settings := body["settings"].(map[string]any)
			assert.EqualValues(t, 10, settings["duration"])
			assert.Equal(t, "1080p", settings["resolution"])
			state := descriptor["state"].(map[string]any)
			assert.Equal(t, tc.videoInput, state["videoInput"])
			ctx["state"] = state
			value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			wantSeconds := 10
			if tc.videoInput != "none" {
				wantSeconds = 40
			}
			assert.EqualValues(t, wantSeconds, value.(map[string]any)["billable_seconds"])
		})
	}

	t.Run("automatic duration is reserved without changing the request", func(t *testing.T) {
		ctx := map[string]any{"model": "kling-4.0", "apiKey": "official", "baseUrl": "https://api.example", "requestBody": map[string]any{"prompt": "a 15 second video"}}
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
		require.NoError(t, err)
		descriptor := value.(map[string]any)
		assert.NotContains(t, descriptor["body"].(map[string]any)["settings"], "duration")
		encoded, err := common.Marshal(descriptor["state"])
		require.NoError(t, err)
		var persisted map[string]any
		require.NoError(t, common.Unmarshal(encoded, &persisted))
		ctx["state"] = persisted
		value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 30, value.(map[string]any)["billable_seconds"])
	})

	t.Run("multipart preserves false and the host file reference", func(t *testing.T) {
		value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "kling-4.0", "body": map[string]any{"kind": "multipart", "fields": map[string]any{"seconds": []string{"5"}, "metadata": []string{`{"watermark":false}`}}, "files": []any{map[string]any{"field": "input_reference", "ref": "request_file:input_reference", "mimeType": "image/png", "size": 100}}},
		})
		require.NoError(t, err)
		value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "kling-4.0", "apiKey": "key", "baseUrl": "https://api.example", "requestBody": value.(map[string]any)["requestBody"]})
		require.NoError(t, err)
		body := value.(map[string]any)["body"].(map[string]any)
		assert.Equal(t, false, body["options"].(map[string]any)["watermark_info"].(map[string]any)["enabled"])
		image := body["contents"].([]any)[0].(map[string]any)["url"].(map[string]any)
		assert.Equal(t, "request_file:input_reference", image["__fileRef"])
	})

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"zero", map[string]any{"seconds": 0}}, {"negative", map[string]any{"seconds": -1}}, {"fraction", map[string]any{"seconds": 3.5}}, {"oversized", map[string]any{"seconds": "18446744073709551615"}},
		{"boolean", map[string]any{"seconds": true}}, {"conflicting duration", map[string]any{"seconds": 10, "duration": 5}},
		{"nested duration", map[string]any{"metadata": map[string]any{"settings": map[string]any{"duration": 31}}}},
		{"nested conflicting resolution", map[string]any{"resolution": "720p", "metadata": map[string]any{"settings": map[string]any{"resolution": "1080p"}}}},
		{"4k", map[string]any{"resolution": "4k"}}, {"audio off", map[string]any{"audio": "off"}}, {"original without video", map[string]any{"audio": "original"}},
		{"duplicate ids", map[string]any{"references": []any{map[string]any{"role": "first_frame", "url": "a", "id": "x"}, map[string]any{"role": "last_frame", "url": "b", "id": "x"}}}},
		{"invalid order", map[string]any{"references": []any{map[string]any{"role": "last_frame", "url": "a"}, map[string]any{"role": "middle_frame", "url": "b"}}}},
		{"unknown role", map[string]any{"references": []any{map[string]any{"role": "unknown", "url": "a"}}}},
		{"invalid metadata", map[string]any{"metadata": "invalid"}},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			tc.body["prompt"] = "video"
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "kling-4.0", "body": map[string]any{"kind": "json", "value": tc.body}})
			require.Error(t, err)
		})
	}

	adaptor := pluginadaptor.New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeKling, ChannelBaseUrl: "https://api.example"}})
	expression, configured := billing_setting.GetBillingExpr("kling-4.0")
	require.True(t, configured)
	for _, tc := range []struct {
		name, input, output, resolution string
		expected                        float64
	}{
		{"720p text", "0", "10", "720p", 1.57}, {"1080p image", "0", "10", "1080p", 2.35},
		{"720p video", "8.03", "10", "720p", 1.88}, {"input shorter", "8.03", "10", "1080p", 2.82},
		{"equal", "10", "10", "1080p", 2.82}, {"input longer", "20", "10", "1080p", 4.23},
		{"fractional", "8.03", "5", "1080p", 1.83723}, {"explicit zero", "0", "0", "1080p", 0},
	} {
		t.Run("billing "+tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "task-public", Properties: model.Properties{OriginModelName: "video-alias", UpstreamModelName: "kling-4.0"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "vendor-id"}}
			state, err := common.Marshal(map[string]any{"resolution": tc.resolution, "videoInput": "unknown"})
			require.NoError(t, err)
			task.PrivateData.PluginState = state
			row := map[string]any{"id": "vendor-id", "status": "succeeded", "outputs": []any{map[string]any{"type": "video", "url": "https://cdn.example/out.mp4", "duration": "9.98"}}, "usage": map[string]any{"input_duration": tc.input, "output_duration": tc.output}}
			for _, data := range []any{[]any{row}, map[string]any{"result": []any{row}}} {
				encoded, err := common.Marshal(map[string]any{"code": 0, "data": data})
				require.NoError(t, err)
				result, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, encoded)
				require.NoError(t, err)
				assert.Equal(t, "SUCCESS", result.Status)
				assert.False(t, result.BillingPending)
				assert.EqualValues(t, tc.input, fmt.Sprint(result.UsageFacts["input_seconds"]))
				assert.EqualValues(t, tc.output, fmt.Sprint(result.UsageFacts["output_seconds"]))
				assert.Equal(t, "https://cdn.example/out.mp4", result.Url)
				cost, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: result.UsageFacts})
				require.NoError(t, err)
				assert.InDelta(t, tc.expected, cost, 1e-10)
			}
		})
	}

	t.Run("missing and invalid usage stays successful pending billing", func(t *testing.T) {
		for _, usage := range []any{nil, map[string]any{"input_duration": "0", "output_duration": "NaN"}, map[string]any{"input_duration": "40", "output_duration": "5"}} {
			body, err := common.Marshal(map[string]any{"code": 0, "data": []any{map[string]any{"id": "vendor-id", "status": "succeeded", "usage": usage}}})
			require.NoError(t, err)
			task := &model.Task{Properties: model.Properties{OriginModelName: "kling-4.0"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "vendor-id", PluginState: []byte(`{"resolution":"1080p"}`)}}
			result, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, body)
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", result.Status)
			assert.True(t, result.BillingPending)
			assert.Empty(t, result.UsageFacts)
		}
	})

	t.Run("gateway uses the unified video API", func(t *testing.T) {
		ctx := map[string]any{"model": "alias", "upstreamModel": "kling-4.0", "baseUrl": "https://gateway.example", "apiKey": "sk-gateway", "upstream": map[string]any{"kind": "new_api"}, "requestBody": map[string]any{"model": "alias", "prompt": "waves"}}
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://gateway.example/v1/videos", value.(map[string]any)["url"])
		assert.Equal(t, "kling-4.0", value.(map[string]any)["body"].(map[string]any)["model"])
		ctx["taskId"] = "task-upstream"
		value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://gateway.example/v1/videos/task-upstream", value.(map[string]any)["url"])
		ctx["upstreamTaskId"], ctx["artifactKey"], ctx["clientRequest"] = "task-upstream", "video", map[string]any{"method": "GET"}
		value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://gateway.example/v1/videos/task-upstream/content", value.(map[string]any)["url"])
		gatewayAdaptor := pluginadaptor.New(plugin)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeNewAPI, ChannelBaseUrl: "https://gateway.example"}}
		info.ApiKey = "sk-gateway"
		gatewayAdaptor.Init(info)
		task := &model.Task{TaskID: "task-public", Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "kling-4.0"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task-upstream", PluginState: []byte(`{"kling4":true,"gateway":true,"resolution":"1080p"}`)}, Data: []byte(`{"id":"task-upstream","object":"video","status":"completed","seconds":"10","usage":{"input_duration":"0","output_duration":"10"}}`)}
		artifacts, err := gatewayAdaptor.ListArtifacts(task)
		require.NoError(t, err)
		require.Len(t, artifacts, 1)
		content, err := gatewayAdaptor.BuildContentRequest(t.Context(), task, "video", channel.TaskArtifactClientRequest{Method: http.MethodHead})
		require.NoError(t, err)
		assert.Equal(t, "https://gateway.example/v1/videos/task-upstream/content", content.URL)
		assert.Equal(t, "Bearer sk-gateway", content.Headers["Authorization"])
		rendered, err := gatewayAdaptor.ConvertToOpenAIVideo(task)
		require.NoError(t, err)
		var video map[string]any
		require.NoError(t, common.Unmarshal(rendered, &video))
		assert.Equal(t, "task-public", video["id"])
		assert.Equal(t, "10", video["seconds"])
		assert.Equal(t, "10", video["usage"].(map[string]any)["output_duration"])
	})

	t.Run("host HTTP submission freezes billing and enables video download", func(t *testing.T) {
		var received map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/text-to-video/kling-4.0", r.URL.Path)
			assert.Equal(t, "Bearer sk-official-key", r.Header.Get("Authorization"))
			assert.NoError(t, common.DecodeJson(r.Body, &received))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"vendor-id","status":"submitted"}}`))
		}))
		defer server.Close()
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeKling, ChannelBaseUrl: server.URL}, OriginModelName: "kling-4.0"}
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
		info.UpstreamModelName, info.ApiKey = "kling-4.0", "sk-official-key"
		adaptor := pluginadaptor.New(plugin)
		adaptor.Init(info)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("task_request", map[string]any{"model": "kling-4.0", "prompt": "a 15 second video"})
		require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
		facts, err := adaptor.ExtractUsageFactsValidated(c, info)
		require.NoError(t, err)
		assert.EqualValues(t, 30, facts["billable_seconds"])
		body, err := adaptor.BuildRequestBody(c, info)
		require.NoError(t, err)
		response, err := adaptor.DoRequest(c, info, body)
		require.NoError(t, err)
		defer response.Body.Close()
		parsed, taskErr := adaptor.ParseResponse(c, response, info)
		require.Nil(t, taskErr)
		require.NotNil(t, parsed)
		assert.Equal(t, "vendor-id", parsed.UpstreamTaskID)
		assert.NotContains(t, received["settings"], "duration")
		task := &model.Task{TaskID: "task-public", Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "kling-4.0"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: parsed.UpstreamTaskID, PluginState: parsed.PluginState}, Data: []byte(`{"code":0,"data":[{"id":"vendor-id","status":"succeeded","outputs":[{"type":"video","url":"https://cdn.example/result.mp4"}]}]}`)}
		artifacts, err := adaptor.ListArtifacts(task)
		require.NoError(t, err)
		require.Len(t, artifacts, 1)
		content, err := adaptor.BuildContentRequest(t.Context(), task, "video", channel.TaskArtifactClientRequest{Method: http.MethodGet})
		require.NoError(t, err)
		assert.Equal(t, "https://cdn.example/result.mp4", content.URL)
		assert.True(t, content.Credentialless)
		assert.Empty(t, content.Headers, "channel credentials must not be sent to the media host")
	})
}

func TestKlingV3(t *testing.T) {
	source, err := builtinplugins.Source("kling")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "kling"})
	require.NoError(t, err)

	for _, path := range []string{"/v1/videos", "/v1/responses"} {
		binding, found := registry.Generation().LookupEndpoint("POST", path, "kling-v3")
		require.True(t, found, path)
		assert.Same(t, plugin, binding.Plugin)
	}

	for _, surface := range []string{"native", "openai_video", "openai_responses"} {
		for _, action := range []string{"text_to_video", "image_to_video"} {
			for _, spec := range []struct {
				mode  string
				units float64
			}{
				{mode: "pro", units: 6},
				{mode: "4k", units: 15},
			} {
				t.Run(surface+"/"+action+"/"+spec.mode, func(t *testing.T) {
					body := map[string]any{
						"model": "kling-v3", "prompt": "waves with ocean sounds", "seconds": 5,
						"metadata": map[string]any{"mode": spec.mode, "sound": "on"},
					}
					if action == "image_to_video" {
						body["image"] = "https://cdn.example/frame.png"
					}
					namespace := "protocols"
					members := []string{surface, "decodeRequest"}
					expectedPath := "/v1/videos/text2video"
					if action == "image_to_video" {
						expectedPath = "/v1/videos/image2video"
					}
					if surface == "native" {
						body = map[string]any{
							"model_name": "kling-v3", "prompt": body["prompt"], "image": body["image"],
							"duration": "5", "mode": spec.mode, "sound": "on",
						}
						binding, found := registry.Generation().LookupDeclaredRoute("POST", "/kling"+expectedPath)
						require.True(t, found)
						namespace = "native"
						members = []string{binding.Route.Decode}
					} else if surface == "openai_responses" {
						body["input"] = body["prompt"]
						delete(body, "prompt")
					} else {
						// Metadata overrides the standard fields in the upstream body;
						// the reservation must use that same resolution tier.
						body["mode"] = "pro"
					}
					value, err := plugin.Engine.CallPath(t.Context(), namespace, members, map[string]any{
						"model": "kling-v3", "upstreamModel": "kling-v3",
						"body": map[string]any{"kind": "json", "value": body},
					})
					require.NoError(t, err)
					intent, ok := value.(map[string]any)
					require.True(t, ok)
					assert.Equal(t, "kling-v3", intent["model"])
					if surface != "native" {
						assert.Equal(t, action, intent["action"])
					}
					ctx := map[string]any{
						"model": intent["model"], "upstreamModel": "kling-v3", "action": action,
						"baseUrl": "https://provider.example", "apiKey": "access|secret",
						"requestBody": intent["requestBody"], "usagePurpose": "facts",
					}
					value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
					require.NoError(t, err)
					descriptor, ok := value.(map[string]any)
					require.True(t, ok)
					assert.Equal(t, "https://provider.example"+expectedPath, descriptor["url"])
					upstreamBody, ok := descriptor["body"].(map[string]any)
					require.True(t, ok)
					assert.Equal(t, "kling-v3", upstreamBody["model_name"])
					assert.Equal(t, spec.mode, upstreamBody["mode"])
					assert.Equal(t, "on", upstreamBody["sound"])
					assert.Equal(t, "5", upstreamBody["duration"])
					assert.Equal(t, "waves with ocean sounds", upstreamBody["prompt"])
					if action == "image_to_video" {
						assert.Equal(t, "https://cdn.example/frame.png", upstreamBody["image"])
					} else {
						assert.NotContains(t, upstreamBody, "image")
					}
					value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
					require.NoError(t, err)
					facts, ok := value.(map[string]any)
					require.True(t, ok)
					assert.InDelta(t, spec.units, facts["units"], 1e-9)
				})
			}
		}
	}

	t.Run("other audio and resolution tiers use v3 units through a channel alias", func(t *testing.T) {
		for _, spec := range []struct {
			mode  string
			sound string
			units float64
		}{
			{mode: "", sound: "off", units: 3},
			{mode: "std", sound: "on", units: 4.5},
			{mode: "pro", sound: "off", units: 4},
			{mode: "4k", sound: "off", units: 15},
		} {
			value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
				"model": "video-alias", "upstreamModel": "kling-v3", "usagePurpose": "facts",
				"requestBody": map[string]any{"duration": 5, "metadata": map[string]any{"mode": spec.mode, "sound": spec.sound}},
			})
			require.NoError(t, err)
			facts, ok := value.(map[string]any)
			require.True(t, ok)
			assert.InDelta(t, spec.units, facts["units"], 1e-9, spec.mode+"/"+spec.sound)
		}
	})

	t.Run("older models still reject 4k", func(t *testing.T) {
		for _, model := range []string{"kling-v1", "kling-v1-6", "kling-v2-master"} {
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"upstreamModel": model, "requestBody": map[string]any{"metadata": map[string]any{"mode": "4k"}},
			})
			require.ErrorContains(t, err, "mode must be", model)
		}
	})

	t.Run("completion keeps the actual upstream deduction", func(t *testing.T) {
		value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete",
			map[string]any{"model": "kling-v3"}, map[string]any{},
			map[string]any{"data": map[string]any{"final_unit_deduction": "14.5"}},
		)
		require.NoError(t, err)
		facts, ok := value.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, 14.5, facts["units"], 1e-9)
	})
}
