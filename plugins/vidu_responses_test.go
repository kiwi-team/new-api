package plugins_test

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "vidu",
		model:     "viduq2",
		requestBody: map[string]any{
			"model": "viduq2",
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "move between frames"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/first.png"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/last.png"},
			}}},
			"seconds": 8,
			"size":    "720p",
		},
		wantAction: "first_tail_to_video",
		wantRequest: map[string]any{
			"model":    "viduq2",
			"prompt":   "move between frames",
			"images":   []any{"https://cdn.example/first.png", "https://cdn.example/last.png"},
			"duration": float64(8),
			"size":     "720p",
		},
		wantUsageKeys:       []string{"credits", "duration", "resolution"},
		wantSubmitUsageKeys: []string{"duration", "resolution"},
		wantVendorName:      "vidu",
	})
}

func TestViduQ4PreviewImageToVideo(t *testing.T) {
	source, err := builtinplugins.Source("vidu")
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "vidu"})
	require.NoError(t, err)
	assert.Contains(t, plugin.Meta.Models, "viduq4-preview")

	decode := func(body map[string]any) map[string]any {
		t.Helper()
		value, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "viduq4-preview",
			"body":  map[string]any{"kind": "json", "value": body},
		})
		require.NoError(t, callErr)
		encoded, marshalErr := common.Marshal(value)
		require.NoError(t, marshalErr)
		var decoded map[string]any
		require.NoError(t, common.Unmarshal(encoded, &decoded))
		return decoded
	}

	decoded := decode(map[string]any{
		"model":           "viduq4-preview",
		"prompt":          "The astronaut waves",
		"input_reference": "https://cdn.example/start.png",
		"seconds":         16,
		"size":            "4k",
		"audio":           false,
		"is_rec":          false,
		"seed":            0,
	})
	assert.Equal(t, "image_to_video", decoded["action"])
	requestBody, ok := decoded["requestBody"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(16), requestBody["duration"])
	assert.Equal(t, "4K", requestBody["resolution"])

	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       "https://api.vidu.cn",
		"apiKey":        "secret",
		"model":         "viduq4-preview",
		"upstreamModel": "viduq4-preview",
		"requestBody":   requestBody,
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var descriptor map[string]any
	require.NoError(t, common.Unmarshal(encoded, &descriptor))
	assert.Equal(t, "https://api.vidu.cn/ent/v2/img2video", descriptor["url"])
	upstreamBody, ok := descriptor["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "viduq4-preview", upstreamBody["model"])
	assert.Equal(t, "4K", upstreamBody["resolution"])
	assert.Equal(t, false, upstreamBody["audio"])
	assert.Equal(t, false, upstreamBody["is_rec"])
	assert.Equal(t, float64(0), upstreamBody["seed"])
	assert.NotContains(t, upstreamBody, "movement_amplitude")

	tooManyImages := make([]any, 16)
	for i := range tooManyImages {
		tooManyImages[i] = "https://cdn.example/reference.png"
	}
	tests := []struct {
		name string
		body map[string]any
		want string
	}{
		{name: "requires image", body: map[string]any{"model": "viduq4-preview", "prompt": "cat"}, want: "requires at least one image"},
		{name: "minimum duration", body: map[string]any{"model": "viduq4-preview", "input_reference": "https://cdn.example/start.png", "seconds": 2}, want: "duration must be between 3 and 16"},
		{name: "maximum duration", body: map[string]any{"model": "viduq4-preview", "input_reference": "https://cdn.example/start.png", "seconds": 17}, want: "duration must be between 3 and 16"},
		{name: "unsupported resolution", body: map[string]any{"model": "viduq4-preview", "input_reference": "https://cdn.example/start.png", "size": "360p"}, want: "resolution must be 540p, 720p, 1080p, 2K, or 4K"},
		{name: "maximum image count", body: map[string]any{"model": "viduq4-preview", "prompt": "Animate the references", "images": tooManyImages}, want: "supports at most 15 images"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, callErr := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "viduq4-preview",
				"body":  map[string]any{"kind": "json", "value": testCase.body},
			})
			require.ErrorContains(t, callErr, testCase.want)
		})
	}
}

// metadata carries Vidu's other fields but cannot replace the model, duration
// or resolution the request is billed for.
func TestViduMetadataCannotChangeWhatIsBilled(t *testing.T) {
	source, err := builtinplugins.Source("vidu")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "vidu"})
	require.NoError(t, err)
	ctx := map[string]any{"baseUrl": "https://api.vidu.example", "apiKey": "vendor-key", "model": "vidu1.5", "upstreamModel": "vidu1.5",
		"requestBody": map[string]any{"model": "vidu1.5", "prompt": "p", "duration": 4, "resolution": "360p",
			"metadata": map[string]any{"model": "viduq2", "duration": 8, "resolution": "1080p", "seed": 7}}}
	descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	usage, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	body := decodePluginValue(t, descriptor)["body"].(map[string]any)
	assert.Equal(t, map[string]any{"duration": float64(4), "resolution": "360p"}, decodePluginValue(t, usage))
	assert.Equal(t, "vidu1.5", body["model"])
	assert.Equal(t, float64(4), body["duration"])
	assert.Equal(t, "360p", body["resolution"])
	assert.Equal(t, float64(7), body["seed"])
}

// A New API channel reaches Vidu through another gateway's /vidu routes. The
// gateway rebuilds the request this driver sent, so Vidu receives one body and
// both gateways bill the same facts, and the gateway's task reads back as the
// same state and credits.
func TestViduThroughNewAPIGateway(t *testing.T) {
	source, err := builtinplugins.Source("vidu")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "vidu"})
	require.NoError(t, err)
	call := func(export string, members []string, args ...any) map[string]any {
		t.Helper()
		value, err := plugin.Engine.CallPath(t.Context(), export, members, args...)
		require.NoError(t, err)
		return decodePluginValue(t, value)
	}
	gateway := map[string]any{"kind": "new_api"}

	for _, tc := range []struct {
		name        string
		requestBody map[string]any
		route       string
	}{
		{
			name: "text to video with vendor fields",
			requestBody: map[string]any{"model": "viduq2", "prompt": "p", "duration": 8, "size": "1280x720",
				"metadata": map[string]any{"bgm": true, "seed": 7, "aspect_ratio": "9:16"}},
			route: "/vidu/ent/v2/text2video",
		},
		{
			name: "two reference images stay a reference request",
			requestBody: map[string]any{"model": "viduq2", "prompt": "p", "images": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"},
				"metadata": map[string]any{"action": "referenceGenerate"}},
			route: "/vidu/ent/v2/reference2video",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			downstream := map[string]any{"requestBody": tc.requestBody, "model": "viduq2", "upstreamModel": "viduq2",
				"baseUrl": "https://gateway.example", "apiKey": "sk-gateway", "upstream": gateway}
			sent := call("buildSubmitRequest", nil, downstream)
			assert.Equal(t, "https://gateway.example"+tc.route, sent["url"])
			assert.Equal(t, map[string]any{"Accept": "application/json", "Content-Type": "application/json", "Authorization": "Bearer sk-gateway"}, sent["headers"])

			intent := call("native", []string{"createTask"}, map[string]any{"path": tc.route, "body": map[string]any{"kind": "json", "value": sent["body"]}})
			assert.Equal(t, sent["action"], intent["action"])
			upstream := map[string]any{"requestBody": intent["requestBody"], "model": "viduq2", "upstreamModel": "viduq2",
				"baseUrl": "https://api.vidu.example", "apiKey": "vendor-key", "upstream": map[string]any{"kind": "vendor"}}
			forwarded := call("buildSubmitRequest", nil, upstream)
			assert.Equal(t, "https://api.vidu.example"+strings.TrimPrefix(tc.route, "/vidu"), forwarded["url"])
			assert.Equal(t, sent["body"], forwarded["body"])
			assert.Equal(t, call("extractUsage", nil, downstream), call("extractUsage", nil, upstream))
		})
	}

	created := call("native", []string{"taskCreated"}, map[string]any{}, map[string]any{"task_id": "task_pub", "status": "SUBMITTED",
		"data": map[string]any{"task_id": "task_pub", "state": "created", "credits": 10}})
	assert.Equal(t, "task_pub", call("parseSubmitResponse", nil, map[string]any{}, map[string]any{"body": created})["taskId"])

	video := "https://cdn.vidu.example/v.mp4"
	for _, tc := range []struct {
		name       string
		task       map[string]any
		wantStatus string
		wantReason string
	}{
		{
			name: "before the first poll", wantStatus: "SUBMITTED",
			task: map[string]any{"status": "SUBMITTED", "data": map[string]any{"task_id": "task_pub", "state": "created", "credits": 10}},
		},
		{
			name: "succeeded", wantStatus: "SUCCESS",
			task: map[string]any{"status": "SUCCESS", "data": map[string]any{"id": "task_pub", "state": "success", "credits": 12,
				"creations": []any{map[string]any{"id": "c1", "url": video}}}},
		},
		{
			name: "failed by the gateway poller", wantStatus: "FAILURE", wantReason: "task timed out",
			task: map[string]any{"status": "FAILURE", "fail_reason": "task timed out", "data": map[string]any{"id": "task_pub", "state": "processing"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.task["task_id"] = "task_pub"
			rendered := call("native", []string{"taskStatus"}, map[string]any{}, tc.task)
			result := call("parseTaskResult", nil, map[string]any{}, rendered)
			assert.Equal(t, tc.wantStatus, result["status"])
			assert.Equal(t, tc.wantReason, common.Interface2String(result["reason"]))
			if tc.wantStatus != "SUCCESS" {
				return
			}
			assert.Equal(t, map[string]any{"credits": float64(12)}, call("extractUsageOnComplete", nil, nil, nil, rendered))
			content := call("buildContentRequest", nil, map[string]any{"artifactKey": "video", "data": rendered, "upstream": gateway,
				"clientRequest": map[string]any{"method": "GET", "headers": map[string]any{}}})
			assert.Equal(t, map[string]any{"url": video, "method": "GET", "credentialless": true}, content)
		})
	}
}
