package plugins_test

import (
	"maps"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFalPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("fal")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "fal"})
	require.NoError(t, err)
	return plugin
}

func callFalHook(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	result, ok := value.(map[string]any)
	require.True(t, ok)
	return result
}

func TestFalOmniHumanResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "fal", model: "omnihuman-2.0", wantAction: "image_to_video", wantVendorName: "fal",
		requestBody: map[string]any{
			"model": "omnihuman-2.0", "metadata": map[string]any{"audio_url": "https://cdn.example/audio.mp3"},
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "a person singing"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/person.png"},
			}}},
		},
		wantRequest:   map[string]any{"prompt": "a person singing", "audio_url": "https://cdn.example/audio.mp3", "image_url": "https://cdn.example/person.png"},
		wantUsageKeys: []string{"seconds"},
	})
}

func TestFalOmniHumanRequestSurfaces(t *testing.T) {
	plugin := loadFalPlugin(t)
	for _, surface := range []string{"native", "video", "multipart"} {
		t.Run(surface, func(t *testing.T) {
			body := map[string]any{"model": "client-alias", "audio_url": "https://cdn.example/audio.mp3", "image_urls": []string{"https://cdn.example/person.png"}, "prompt": "sing", "duration": 0}
			if surface == "native" {
				delete(body, "model")
				delete(body, "image_urls")
				body["image_url"] = "https://cdn.example/person.png"
			}
			ctx := map[string]any{"model": "client-alias", "upstreamModel": "bytedance/omnihuman/v2", "body": map[string]any{"kind": "json", "value": body}}
			if surface == "multipart" {
				ctx["body"] = map[string]any{"kind": "multipart", "fields": map[string]any{
					"input_reference": []string{"https://cdn.example/person.png"}, "audio_url": []string{"https://cdn.example/audio.mp3"}, "prompt": []string{"sing"},
					"metadata": []string{`{"seconds":0,"duration":0}`},
				}}
			}
			var value any
			var err error
			if surface == "native" {
				value, err = plugin.Engine.CallMember(t.Context(), "native", "createTask", ctx)
			} else {
				value, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, ctx)
			}
			require.NoError(t, err)
			intent := value.(map[string]any)
			if surface == "native" {
				assert.Equal(t, "bytedance/omnihuman/v2", intent["model"])
			} else {
				assert.Equal(t, "client-alias", intent["model"])
			}
			driver := map[string]any{"model": intent["model"], "requestBody": intent["requestBody"], "baseUrl": "https://queue.fal.run/", "apiKey": "fal-secret"}
			request := callFalHook(t, plugin, "buildSubmitRequest", driver)
			assert.Equal(t, "https://queue.fal.run/bytedance/omnihuman/v2", request["url"])
			assert.Equal(t, "POST", request["method"])
			assert.Equal(t, "Key fal-secret", request["headers"].(map[string]any)["Authorization"])
			encoded, err := common.Marshal(request["body"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"image_url":"https://cdn.example/person.png","audio_url":"https://cdn.example/audio.mp3","prompt":"sing"}`, string(encoded))
			assert.Equal(t, map[string]any{"seconds": int64(300)}, callFalHook(t, plugin, "extractUsage", driver))
		})
	}
}

func TestFalOmniHumanValidation(t *testing.T) {
	plugin := loadFalPlugin(t)
	for _, tc := range []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{"missing audio", map[string]any{"audio_url": nil}, "audio_url"},
		{"inline audio", map[string]any{"audio_url": "data:audio/wav;base64,AAAA"}, "audio_url"},
		{"invalid image", map[string]any{"image_url": "not-a-url"}, "image_url"},
		{"multiple images", map[string]any{"image_urls": []string{"https://cdn.example/a.png", "https://cdn.example/b.png"}}, "exactly one"},
		{"invalid metadata", map[string]any{"metadata": []any{}}, "metadata"},
		{"invalid prompt", map[string]any{"prompt": true}, "prompt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"image_url": "https://cdn.example/person.png", "audio_url": "https://cdn.example/audio.mp3"}
			maps.Copy(body, tc.fields)
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "omnihuman-2.0", "body": map[string]any{"kind": "json", "value": body}})
			require.ErrorContains(t, err, tc.want)
		})
	}
	for _, key := range []string{"", " ", "secret\r\ninjected-header"} {
		_, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": "https://queue.fal.run", "taskId": "request-1", "apiKey": key})
		require.ErrorContains(t, err, "valid fal API key")
		assert.NotContains(t, err.Error(), "secret")
	}
	_, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": "http://queue.fal.run", "taskId": "request-1", "apiKey": "secret"})
	require.ErrorContains(t, err, "HTTPS")
	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "omnihuman-2.0", "body": map[string]any{"kind": "multipart", "files": []any{map[string]any{"field": "input_reference"}}}})
	require.ErrorContains(t, err, "file uploads are unsupported")
}

func TestFalOmniHumanPollPhasesAndUsage(t *testing.T) {
	plugin := loadFalPlugin(t)
	ctx := map[string]any{"baseUrl": "https://queue.fal.run", "apiKey": "secret", "taskId": "upstream-1"}
	submit := callFalHook(t, plugin, "parseSubmitResponse", ctx, map[string]any{"body": map[string]any{
		"request_id": "upstream-1", "status_url": "https://attacker.example/status", "response_url": "https://attacker.example/result",
	}})
	assert.Equal(t, "upstream-1", submit["taskId"])
	ctx["data"] = submit["taskData"]
	query := callFalHook(t, plugin, "buildQueryRequest", ctx)
	assert.Equal(t, "https://queue.fal.run/bytedance/omnihuman/requests/upstream-1/status", query["url"])

	task := &model.Task{}
	task.TaskID = "task_public"
	task.Properties.OriginModelName = "omnihuman-2.0"
	task.PrivateData.UpstreamTaskID = "upstream-1"
	response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	adaptor := taskplugin.New(plugin)
	queued, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"IN_QUEUE"}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusQueued), queued.Status)
	completed, err := adaptor.ParseTaskResult(task, response, []byte(`{"status":"COMPLETED","metrics":{"inference_time":0.1}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusInProgress), completed.Status, "queue completion is not model success")
	assert.Empty(t, completed.UsageFacts)
	require.NotEmpty(t, completed.PluginState)
	task.PrivateData.PluginState = completed.PluginState
	var state map[string]any
	require.NoError(t, common.Unmarshal(completed.PluginState, &state))
	ctx["state"] = state
	query = callFalHook(t, plugin, "buildQueryRequest", ctx)
	assert.Equal(t, "https://queue.fal.run/bytedance/omnihuman/requests/upstream-1", query["url"])

	// A new adaptor receives only the persisted phase and result snapshot.
	result, err := taskplugin.New(plugin).ParseTaskResult(task, response, []byte(`{"video":{"url":"https://cdn.example/video.mp4"},"duration":12.5}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)
	assert.Equal(t, map[string]any{"seconds": float64(12.5)}, result.UsageFacts)

	for _, duration := range []any{nil, "10", -1, 0, 3.99, 301, 1e30} {
		body, err := common.Marshal(map[string]any{"video": map[string]any{"url": "https://cdn.example/video.mp4"}, "duration": duration, "metrics": map[string]any{"inference_time": 2}})
		require.NoError(t, err)
		result, err := adaptor.ParseTaskResult(task, response, body)
		require.NoError(t, err)
		assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
		assert.Empty(t, result.UsageFacts, "invalid duration must leave the 300-second reservation unchanged")
	}
	value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"usagePurpose": "billing_ratios"})
	require.NoError(t, err)
	assert.Nil(t, value, "legacy per-call pricing must not be multiplied by 300")
}

func TestFalOmniHumanFailuresAndUnknownResponses(t *testing.T) {
	plugin := loadFalPlugin(t)
	for _, tc := range []struct {
		name       string
		body       any
		httpStatus int
		want       string
	}{
		{"generating", map[string]any{"status": "IN_PROGRESS"}, 200, "IN_PROGRESS"},
		{"queue failure", map[string]any{"status": "COMPLETED", "error": "generation failed"}, 200, "FAILURE"},
		{"typed failure", map[string]any{"status": "COMPLETED", "error_type": "content_policy_violation"}, 200, "FAILURE"},
		{"validation failure", map[string]any{"detail": []any{map[string]any{"msg": "invalid audio"}}}, 422, "FAILURE"},
		{"credentials expired", map[string]any{"detail": "unauthorized"}, 401, "UNKNOWN"},
		{"rate limited", map[string]any{"detail": "retry"}, 429, "UNKNOWN"},
		{"temporary error", map[string]any{"detail": "retry"}, 503, "UNKNOWN"},
		{"unknown status", map[string]any{"status": "unexpected"}, 200, "UNKNOWN"},
		{"missing video", map[string]any{"duration": 10}, 200, "UNKNOWN"},
		{"result endpoint only repeats queue completion", map[string]any{"status": "COMPLETED"}, 200, "UNKNOWN"},
		{"null response", nil, 200, "UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := callFalHook(t, plugin, "parseTaskResult", map[string]any{"state": map[string]any{"phase": "result"}}, tc.body, map[string]any{"status": tc.httpStatus})
			assert.Equal(t, tc.want, result["status"])
		})
	}
	for _, body := range []any{nil, map[string]any{}, map[string]any{"request_id": 123}, map[string]any{"request_id": " "}} {
		_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"body": body})
		require.ErrorContains(t, err, "missing request_id")
	}
}

func TestFalOmniHumanGatewayAndArtifacts(t *testing.T) {
	plugin := loadFalPlugin(t)
	ctx := map[string]any{
		"model": "alias", "upstreamModel": "omnihuman-2.0", "baseUrl": "https://gateway.example", "apiKey": "gateway-token", "authHeader": "Bearer gateway-token", "upstream": map[string]any{"kind": "new_api"},
		"requestBody": map[string]any{"image_url": "https://cdn.example/person.png", "audio_url": "https://cdn.example/audio.mp3"},
	}
	request := callFalHook(t, plugin, "buildSubmitRequest", ctx)
	assert.Equal(t, "Bearer gateway-token", request["headers"].(map[string]any)["Authorization"])
	intent, err := plugin.Engine.CallMember(t.Context(), "native", "createTask", map[string]any{"body": map[string]any{"kind": "json", "value": request["body"]}})
	require.NoError(t, err)
	assert.Equal(t, "omnihuman-2.0", intent.(map[string]any)["model"])

	for _, status := range []string{"SUBMITTED", "FAILURE", "SUCCESS"} {
		task := map[string]any{"task_id": "task_public", "status": status, "fail_reason": "generation failed", "data": map[string]any{"request_id": "private-id", "response_url": "https://queue.fal.run/private-id"}}
		value, err := plugin.Engine.CallMember(t.Context(), "native", "taskStatus", map[string]any{}, task)
		require.NoError(t, err)
		encoded, err := common.Marshal(value)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "private-id")
		assert.Contains(t, string(encoded), "task_public")
		parsed := callFalHook(t, plugin, "parseTaskResult", map[string]any{}, value)
		if status == "FAILURE" {
			assert.Equal(t, "FAILURE", parsed["status"])
		}
	}

	data := map[string]any{"video": map[string]any{"url": "https://cdn.example/video.mp4"}, "duration": 12.5}
	value, err := plugin.Engine.CallMember(t.Context(), "native", "taskResult", map[string]any{}, map[string]any{"task_id": "task_public", "status": "SUCCESS", "data": data})
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", callFalHook(t, plugin, "parseTaskResult", map[string]any{"state": map[string]any{"phase": "result"}}, value)["status"])
	artifacts, err := plugin.Engine.Call(t.Context(), "listArtifacts", map[string]any{"status": "SUCCESS", "data": data})
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	content := callFalHook(t, plugin, "buildContentRequest", map[string]any{"artifactKey": "video", "data": data, "apiKey": "secret", "clientRequest": map[string]any{"method": "HEAD"}})
	assert.Equal(t, map[string]any{"url": "https://cdn.example/video.mp4", "method": "HEAD", "credentialless": true}, content)
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "other", "data": data})
	require.ErrorContains(t, err, "artifact_not_found")
}
