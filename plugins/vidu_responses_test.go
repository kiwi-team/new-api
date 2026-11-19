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
		model:     "viduq2-turbo",
		requestBody: map[string]any{
			"model": "viduq2-turbo",
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
			"model":    "viduq2-turbo",
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
	ctx := map[string]any{"baseUrl": "https://api.vidu.example", "apiKey": "vendor-key", "model": "viduq2", "upstreamModel": "viduq2",
		"requestBody": map[string]any{"model": "viduq2", "prompt": "p", "duration": 4, "resolution": "540p",
			"metadata": map[string]any{"model": "viduq1", "duration": 8, "resolution": "1080p", "seed": 7}}}
	descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	usage, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	body := decodePluginValue(t, descriptor)["body"].(map[string]any)
	assert.Equal(t, map[string]any{"duration": float64(4), "resolution": "540p"}, decodePluginValue(t, usage))
	assert.Equal(t, "viduq2", body["model"])
	assert.Equal(t, float64(4), body["duration"])
	assert.Equal(t, "540p", body["resolution"])
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

// /v1/videos takes any model: one the endpoint does not list gets the
// defaults of Vidu's current models, which the body states, and a client
// cannot pass a body off as a native one. Through another gateway both reserve
// the same facts.
func TestViduVideosProtocolTakesAnyModel(t *testing.T) {
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
	for _, tc := range []struct {
		name      string
		body      map[string]any
		upstream  string
		wantURL   string
		wantBody  map[string]any
		wantFacts map[string]any
	}{
		{
			name:      "a model the plugin does not list",
			body:      map[string]any{"model": "viduq5", "prompt": "a cat", "size": "1920x1080"},
			wantURL:   "https://api.vidu.example/ent/v2/text2video",
			wantBody:  map[string]any{"model": "viduq5", "prompt": "a cat", "duration": float64(5), "resolution": "1080p"},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "1080p"},
		},
		{
			name:      "a model the endpoint does not list",
			body:      map[string]any{"model": "viduq2", "prompt": "a cat", "input_reference": "https://cdn.example/a.png", "seconds": "8"},
			wantURL:   "https://api.vidu.example/ent/v2/img2video",
			wantBody:  map[string]any{"model": "viduq2", "prompt": "a cat", "images": []any{"https://cdn.example/a.png"}, "duration": float64(8), "resolution": "720p"},
			wantFacts: map[string]any{"duration": float64(8), "resolution": "720p"},
		},
		{
			name:      "an alias takes the channel model's defaults",
			body:      map[string]any{"model": "my-vidu", "prompt": "a cat"},
			upstream:  "viduq3-pro",
			wantURL:   "https://api.vidu.example/ent/v2/text2video",
			wantBody:  map[string]any{"model": "viduq3-pro", "prompt": "a cat", "duration": float64(5), "resolution": "720p"},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "720p"},
		},
		{
			name: "a vendorBody field is not a native body",
			body: map[string]any{"model": "viduq2", "prompt": "a cat",
				"vendorBody": map[string]any{"model": "viduq2", "prompt": "x", "callback_url": "https://client.example/hook"}},
			wantURL:   "https://api.vidu.example/ent/v2/text2video",
			wantBody:  map[string]any{"model": "viduq2", "prompt": "a cat", "duration": float64(5), "resolution": "720p"},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "720p"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decodeCtx := map[string]any{"model": tc.body["model"], "operation": "create", "body": map[string]any{"kind": "json", "value": tc.body}}
			upstream := tc.body["model"]
			if tc.upstream != "" {
				decodeCtx["upstreamModel"], upstream = tc.upstream, tc.upstream
			}
			intent := call("protocols", []string{"openai_video", "decodeRequest"}, decodeCtx)
			driver := map[string]any{"requestBody": intent["requestBody"], "model": tc.body["model"], "upstreamModel": upstream, "baseUrl": "https://api.vidu.example", "apiKey": "vendor-key"}
			sent := call("buildSubmitRequest", nil, driver)
			assert.Equal(t, tc.wantURL, sent["url"])
			assert.Equal(t, tc.wantBody, sent["body"])
			assert.Equal(t, tc.wantFacts, call("extractUsage", nil, driver))
		})
	}

	t.Run("through another gateway", func(t *testing.T) {
		requestBody := map[string]any{"model": "viduq5", "prompt": "a cat", "duration": 5, "resolution": "720p"}
		downstream := map[string]any{"requestBody": requestBody, "model": "viduq5", "upstreamModel": "viduq5",
			"baseUrl": "https://gateway.example", "apiKey": "sk-gateway", "upstream": map[string]any{"kind": "new_api"}}
		sent := call("buildSubmitRequest", nil, downstream)
		intent := call("native", []string{"createTask"}, map[string]any{"path": "/vidu/ent/v2/text2video", "body": map[string]any{"kind": "json", "value": sent["body"]}})
		upstream := map[string]any{"requestBody": intent["requestBody"], "model": "viduq5", "upstreamModel": "viduq5",
			"baseUrl": "https://api.vidu.example", "apiKey": "vendor-key", "upstream": map[string]any{"kind": "vendor"}}
		assert.Equal(t, sent["body"], call("buildSubmitRequest", nil, upstream)["body"])
		assert.Equal(t, call("extractUsage", nil, downstream), call("extractUsage", nil, upstream))
	})
}

// Native routes send Vidu's own body as is, for Vidu to check.
func TestViduNativeBodies(t *testing.T) {
	source, err := builtinplugins.Source("vidu")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "vidu"})
	require.NoError(t, err)
	decode := func(route string, body map[string]any) (map[string]any, error) {
		value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"path": route, "body": map[string]any{"kind": "json", "value": body}})
		if err != nil {
			return nil, err
		}
		return decodePluginValue(t, value), nil
	}

	// Vidu checks the body, which goes to it as sent. The reservation takes the
	// duration and resolution the plugin can read, the model's defaults on the
	// endpoint for those left out, and otherwise the dearest the model renders.
	for _, tc := range []struct {
		name      string
		route     string
		body      map[string]any
		upstream  string
		wantFacts map[string]any
	}{
		{
			name:      "defaults are reserved and nothing is added",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "viduq3-pro", "prompt": "a cat", "bgm": false},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "720p"},
		},
		{
			name:  "subjects need no top-level images",
			route: "/ent/v2/reference2video",
			body: map[string]any{"model": "viduq2-pro", "prompt": "[@cat] runs", "duration": 8,
				"subjects": []any{map[string]any{"name": "cat", "images": []any{"https://cdn.example/cat.png"}}}},
			wantFacts: map[string]any{"duration": float64(8), "resolution": "720p"},
		},
		{
			name:      "q4 renders 4K",
			route:     "/ent/v2/img2video",
			body:      map[string]any{"model": "viduq4-preview", "images": []any{"https://cdn.example/a.png"}, "resolution": "4K", "duration": 16},
			wantFacts: map[string]any{"duration": float64(16), "resolution": "4K"},
		},
		{
			name:      "drama defaults to eight seconds in 1080p",
			route:     "/ent/v2/reference2video",
			body:      map[string]any{"model": "viduq3-drama", "prompt": "a scene", "images": []any{"https://cdn.example/a.png"}},
			wantFacts: map[string]any{"duration": float64(8), "resolution": "1080p"},
		},
		{
			name:      "1.5 defaults to four seconds in 360p",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "vidu1.5", "prompt": "a cat"},
			wantFacts: map[string]any{"duration": float64(4), "resolution": "360p"},
		},
		{
			name:      "1.5 references take eight seconds",
			route:     "/ent/v2/reference2video",
			body:      map[string]any{"model": "vidu1.5", "prompt": "a scene", "images": []any{"a", "b", "c"}, "duration": 8},
			wantFacts: map[string]any{"duration": float64(8), "resolution": "720p"},
		},
		{
			name:      "a pixel size reserves the dearest resolution",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "viduq2", "prompt": "a cat", "resolution": "1280x720"},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "1080p"},
		},
		{
			name:      "a resolution the model does not list is reserved as asked",
			route:     "/ent/v2/img2video",
			body:      map[string]any{"model": "viduq1", "images": []any{"https://cdn.example/a.png"}, "resolution": "720p"},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "720p"},
		},
		{
			name:      "a model the endpoint does not list reserves its dearest elsewhere",
			route:     "/ent/v2/start-end2video",
			body:      map[string]any{"model": "viduq2", "images": []any{"a", "b"}},
			wantFacts: map[string]any{"duration": float64(10), "resolution": "1080p"},
		},
		{
			name:      "2.0 at eight seconds in 1080p",
			route:     "/ent/v2/img2video",
			body:      map[string]any{"model": "vidu2.0", "images": []any{"a"}, "duration": 8, "resolution": "1080p"},
			wantFacts: map[string]any{"duration": float64(8), "resolution": "1080p"},
		},
		{
			name:      "2.0 at six seconds",
			route:     "/ent/v2/img2video",
			body:      map[string]any{"model": "vidu2.0", "images": []any{"a"}, "duration": 6},
			wantFacts: map[string]any{"duration": float64(6), "resolution": "360p"},
		},
		{
			name:      "1.5 at five seconds",
			route:     "/ent/v2/start-end2video",
			body:      map[string]any{"model": "vidu1.5", "images": []any{"a", "b"}, "duration": 5},
			wantFacts: map[string]any{"duration": float64(5), "resolution": "360p"},
		},
		{
			name:      "a duration past the model's longest is reserved as asked",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "viduq3-turbo", "prompt": "a cat", "duration": 17},
			wantFacts: map[string]any{"duration": float64(17), "resolution": "720p"},
		},
		{
			name:      "a duration given as a string",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "viduq3-turbo", "prompt": "a cat", "duration": "8"},
			wantFacts: map[string]any{"duration": float64(8), "resolution": "720p"},
		},
		{
			name:      "an unreadable duration reserves the longest in the dearest resolution",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "viduq3-pro", "prompt": "a cat", "duration": "long"},
			wantFacts: map[string]any{"duration": float64(16), "resolution": "1080p"},
		},
		{
			name:      "a model the plugin does not list reserves the dearest of every model",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "viduq5", "prompt": "a cat"},
			wantFacts: map[string]any{"duration": float64(16), "resolution": "4K"},
		},
		{
			name:      "an alias goes as the channel's model",
			route:     "/ent/v2/text2video",
			body:      map[string]any{"model": "my-vidu", "prompt": "a cat"},
			upstream:  "viduq3-pro",
			wantFacts: map[string]any{"duration": float64(5), "resolution": "720p"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intent, err := decode(tc.route, tc.body)
			require.NoError(t, err)
			model, upstream := tc.body["model"], any(tc.upstream)
			if tc.upstream == "" {
				upstream = model
			}
			driver := map[string]any{"requestBody": intent["requestBody"], "model": model, "upstreamModel": upstream, "baseUrl": "https://api.vidu.example", "apiKey": "vendor-key"}
			descriptor, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", driver)
			require.NoError(t, err)
			sent := decodePluginValue(t, descriptor)
			assert.Equal(t, "https://api.vidu.example"+tc.route, sent["url"])
			wantBody := decodePluginValue(t, tc.body)
			wantBody["model"] = upstream
			assert.Equal(t, wantBody, sent["body"])
			facts, err := plugin.Engine.Call(t.Context(), "extractUsage", driver)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFacts, decodePluginValue(t, facts))
		})
	}

	for _, tc := range []struct {
		name    string
		route   string
		body    map[string]any
		wantErr string
	}{
		{"the model names the price", "/ent/v2/text2video", map[string]any{"prompt": "a cat"}, "model is required"},
		{"callbacks carry Vidu's task ids", "/ent/v2/text2video", map[string]any{"model": "viduq3-pro", "prompt": "a cat", "callback_url": "https://client.example/hook"}, "callback_url is not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decode(tc.route, tc.body)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}

	t.Run("the first query answers with query fields", func(t *testing.T) {
		value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"taskStatus"}, map[string]any{}, map[string]any{"task_id": "task_pub", "status": "SUBMITTED",
			"data": map[string]any{"task_id": "task_pub", "state": "created", "model": "viduq3-pro", "prompt": "a cat", "duration": 5, "credits": 40, "created_at": "2026-10-23T00:00:00Z"}})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"id": "task_pub", "state": "created", "err_code": "", "credits": float64(40), "creations": []any{}}, decodePluginValue(t, value))
	})
}
