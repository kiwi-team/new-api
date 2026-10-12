package plugins_test

import (
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
