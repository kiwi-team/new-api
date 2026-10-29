package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
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
