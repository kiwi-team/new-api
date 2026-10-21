package plugins_test

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadOmniHumanPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("omnihuman")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{
		Key: "omnihuman", Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
	require.NoError(t, err)
	return plugin
}

func omniHumanBody() map[string]any {
	return map[string]any{"audio_url": "https://cdn.example/audio.mp3", "image_urls": []string{"https://cdn.example/person.png"}, "enable_watermark": false}
}

func TestOmniHumanProvidersAcceptUnifiedVideoRequest(t *testing.T) {
	for _, provider := range []struct {
		key      string
		plugin   *jsplugin.LoadedPlugin
		baseURL  string
		apiKey   string
		wantBody string
	}{
		{
			key: "omnihuman", plugin: loadOmniHumanPlugin(t), baseURL: "https://cv.byteplusapi.com", apiKey: "ak|sk",
			wantBody: `{"req_key":"realman_avatar_picture_omni20_cv","image_urls":["https://cdn.example/person.png"],"audio_url":"https://cdn.example/audio.mp3","prompt":"a person singing"}`,
		},
		{
			key: "fal", plugin: loadFalPlugin(t), baseURL: "https://queue.fal.run", apiKey: "fal-key",
			wantBody: `{"image_url":"https://cdn.example/person.png","audio_url":"https://cdn.example/audio.mp3","prompt":"a person singing"}`,
		},
	} {
		for _, surface := range []struct {
			name string
			body map[string]any
		}{
			{
				name: "json",
				body: map[string]any{"kind": "json", "value": map[string]any{
					"model": "omnihuman-2.0", "prompt": "a person singing", "input_reference": "https://cdn.example/person.png",
					"metadata": map[string]any{"audio_url": "https://cdn.example/audio.mp3"},
				}},
			},
			{
				name: "multipart",
				body: map[string]any{"kind": "multipart", "fields": map[string]any{
					"model": []string{"omnihuman-2.0"}, "prompt": []string{"a person singing"}, "input_reference": []string{"https://cdn.example/person.png"},
					"metadata": []string{`{"audio_url":"https://cdn.example/audio.mp3"}`},
				}},
			},
			{
				name: "json image alias",
				body: map[string]any{"kind": "json", "value": map[string]any{
					"model": "omnihuman-2.0", "prompt": "a person singing", "image": "https://cdn.example/person.png",
					"metadata": map[string]any{"audio_url": "https://cdn.example/audio.mp3"},
				}},
			},
			{
				name: "multipart image alias",
				body: map[string]any{"kind": "multipart", "fields": map[string]any{
					"model": []string{"omnihuman-2.0"}, "prompt": []string{"a person singing"}, "image": []string{"https://cdn.example/person.png"},
					"metadata": []string{`{"audio_url":"https://cdn.example/audio.mp3"}`},
				}},
			},
			{
				name: "input_reference takes precedence over image",
				body: map[string]any{"kind": "json", "value": map[string]any{
					"model": "omnihuman-2.0", "prompt": "a person singing", "input_reference": "https://cdn.example/person.png", "image": "https://cdn.example/other.png",
					"metadata": map[string]any{"audio_url": "https://cdn.example/audio.mp3"},
				}},
			},
		} {
			t.Run(provider.key+"/"+surface.name, func(t *testing.T) {
				value, err := provider.plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
					"model": "omnihuman-2.0", "body": surface.body,
				})
				require.NoError(t, err)
				intent, ok := value.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "omnihuman-2.0", intent["model"])
				assert.Equal(t, "image_to_video", intent["action"])
				value, err = provider.plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
					"model": intent["model"], "requestBody": intent["requestBody"], "baseUrl": provider.baseURL, "apiKey": provider.apiKey,
				})
				require.NoError(t, err)
				request, ok := value.(map[string]any)
				require.True(t, ok)
				wire, ok := request["body"].(string)
				if !ok {
					encoded, err := common.Marshal(request["body"])
					require.NoError(t, err)
					wire = string(encoded)
				}
				assert.JSONEq(t, provider.wantBody, wire)
			})
		}
	}
}

func TestOmniHumanResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "omnihuman", model: "omnihuman-2.0", wantAction: "image_to_video", wantVendorName: "omnihuman",
		requestBody: map[string]any{
			"model": "omnihuman-2.0", "metadata": map[string]any{"audio_url": "https://cdn.example/audio.mp3", "enable_watermark": false},
			"input": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "a person singing"},
				map[string]any{"type": "input_image", "image_url": "https://cdn.example/person.png"},
			}}},
		},
		wantRequest:   map[string]any{"prompt": "a person singing", "audio_url": "https://cdn.example/audio.mp3", "image_urls": []any{"https://cdn.example/person.png"}, "enable_watermark": false},
		wantUsageKeys: []string{"seconds", "videos"},
	})
}

func TestOmniHumanRequestSurfacesAndSigning(t *testing.T) {
	plugin := loadOmniHumanPlugin(t)
	for _, surface := range []string{"native", "video", "multipart"} {
		t.Run(surface, func(t *testing.T) {
			body := omniHumanBody()
			body["req_key"] = "realman_avatar_picture_omni20_cv"
			ctx := map[string]any{"model": "alias", "upstreamModel": "omnihuman-2.0", "body": map[string]any{"kind": "json", "value": body}, "query": map[string]any{"Action": []string{"CVSync2AsyncSubmitTask"}}}
			if surface == "multipart" {
				ctx["body"] = map[string]any{"kind": "multipart", "fields": map[string]any{
					"audio_url": []string{"https://cdn.example/audio.mp3"}, "input_reference": []string{"https://cdn.example/person.png"}, "enable_watermark": []string{"false"},
				}}
			}
			var intent any
			var err error
			if surface == "native" {
				intent, err = plugin.Engine.CallMember(t.Context(), "native", "decodeRequest", ctx)
			} else {
				intent, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, ctx)
			}
			require.NoError(t, err)
			resolved, ok := intent.(map[string]any)
			require.True(t, ok)
			if surface != "native" {
				assert.Equal(t, "alias", resolved["model"])
			}
			driver := map[string]any{"model": resolved["model"], "upstreamModel": "omnihuman-2.0", "requestBody": resolved["requestBody"], "baseUrl": "https://cv.byteplusapi.com/", "apiKey": "access-key|secret-key"}
			value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", driver)
			require.NoError(t, err)
			request := value.(map[string]any)
			assert.Equal(t, "https://cv.byteplusapi.com/?Action=CVSync2AsyncSubmitTask&Version=2024-06-06", request["url"])
			assert.Equal(t, "POST", request["method"])
			wire := request["body"].(string)
			assert.JSONEq(t, `{"req_key":"realman_avatar_picture_omni20_cv","audio_url":"https://cdn.example/audio.mp3","image_urls":["https://cdn.example/person.png"],"enable_watermark":false}`, wire)
			headers := request["headers"].(map[string]any)
			assert.Equal(t, "20260102T030405Z", headers["X-Date"])
			assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(wire))), headers["X-Content-Sha256"])
			assert.Contains(t, headers["Authorization"], "Credential=access-key/20260102/ap-singapore-1/cv/request")
			assert.Contains(t, headers["Authorization"], "Signature=ecd308ab93eeb22d00900c074e61ec95933c55da0de400befac57da4b3631b99")
			assert.NotContains(t, headers["Authorization"], "secret-key")
			assert.NotContains(t, wire, "access-key")
		})
	}

	t.Run("query uses private upstream task id and service key", func(t *testing.T) {
		value, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
			"baseUrl": "https://cv.byteplusapi.com", "apiKey": "access-key|secret-key", "taskId": "7392616336519610409", "publicTaskId": "task_public", "data": nil,
		})
		require.NoError(t, err)
		request := value.(map[string]any)
		assert.Equal(t, "https://cv.byteplusapi.com/?Action=CVSync2AsyncGetResult&Version=2024-06-06", request["url"])
		assert.JSONEq(t, `{"req_key":"realman_avatar_picture_omni20_cv","task_id":"7392616336519610409"}`, request["body"].(string))
	})

	t.Run("omitted watermark preserves provider default", func(t *testing.T) {
		body := omniHumanBody()
		delete(body, "enable_watermark")
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "omnihuman-2.0", "requestBody": body, "baseUrl": "https://cv.byteplusapi.com", "apiKey": "ak|sk",
		})
		require.NoError(t, err)
		assert.NotContains(t, value.(map[string]any)["body"], "enable_watermark")
	})

	t.Run("gateway model mapping reaches the native decoder", func(t *testing.T) {
		body := omniHumanBody()
		body["metadata"] = map[string]any{"req_key": "untrusted-service", "videos": 999}
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "alias", "upstreamModel": "omnihuman-2.0", "requestBody": body,
			"baseUrl": "https://gateway.example", "apiKey": "gateway-token", "authHeader": "Bearer gateway-token", "upstream": map[string]any{"kind": "new_api"},
		})
		require.NoError(t, err)
		request := value.(map[string]any)
		var forwarded map[string]any
		require.NoError(t, common.UnmarshalJsonStr(request["body"].(string), &forwarded))
		assert.Equal(t, "omnihuman-2.0", forwarded["req_key"])
		assert.NotContains(t, forwarded, "metadata")
		intent, err := plugin.Engine.CallMember(t.Context(), "native", "decodeRequest", map[string]any{
			"body": map[string]any{"kind": "json", "value": forwarded}, "query": map[string]any{"Action": []string{"CVSync2AsyncSubmitTask"}},
		})
		require.NoError(t, err)
		assert.Equal(t, "omnihuman-2.0", intent.(map[string]any)["model"])
	})
}

func TestOmniHumanRejectsInvalidInputsAndCredentials(t *testing.T) {
	plugin := loadOmniHumanPlugin(t)
	for _, tc := range []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{"missing audio", map[string]any{"audio_url": nil}, "audio_url"},
		{"inline audio", map[string]any{"audio_url": "data:audio/wav;base64,AAAA"}, "audio_url"},
		{"no image", map[string]any{"image_urls": []string{}}, "exactly one"},
		{"multiple images", map[string]any{"image_urls": []string{"https://cdn.example/a.png", "https://cdn.example/b.png"}}, "exactly one"},
		{"inline image", map[string]any{"image_urls": []string{"data:image/png;base64,AAAA"}}, "image_urls"},
		{"watermark string", map[string]any{"enable_watermark": "false"}, "boolean"},
		{"watermark null", map[string]any{"enable_watermark": nil}, "boolean"},
		{"prompt object", map[string]any{"prompt": map[string]any{}}, "prompt"},
		{"metadata array", map[string]any{"metadata": []any{}}, "metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := omniHumanBody()
			maps.Copy(body, tc.fields)
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
				"model": "omnihuman-2.0", "body": map[string]any{"kind": "json", "value": body},
			})
			require.ErrorContains(t, err, tc.want)
		})
	}
	for _, key := range []string{"", "sk-gateway-token", "access-missing|", "|secret-missing", "access|secret|extra"} {
		_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "omnihuman-2.0", "requestBody": omniHumanBody(), "baseUrl": "https://cv.byteplusapi.com", "apiKey": key,
		})
		require.ErrorContains(t, err, "ak|sk")
		if key != "" {
			assert.NotContains(t, err.Error(), key)
		}
	}
	_, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": "http://cv.byteplusapi.com", "apiKey": "ak|sk", "taskId": "t1"})
	require.ErrorContains(t, err, "HTTPS")
	_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "omnihuman-2.0", "body": map[string]any{"kind": "multipart", "files": []any{map[string]any{"field": "input_reference"}}},
	})
	require.ErrorContains(t, err, "file uploads are unsupported")
}

func TestOmniHumanTaskLifecycle(t *testing.T) {
	plugin := loadOmniHumanPlugin(t)
	for _, tc := range []struct {
		name string
		body any
		want string
	}{
		{"queued", map[string]any{"code": 10000, "data": map[string]any{"status": "in_queue"}}, "QUEUED"},
		{"generating", map[string]any{"code": 10000, "data": map[string]any{"status": "generating"}}, "IN_PROGRESS"},
		{"completed", map[string]any{"code": 10000, "data": map[string]any{"status": "done", "video_url": "https://cdn.example/video.mp4"}}, "SUCCESS"},
		{"missing video", map[string]any{"code": 10000, "data": map[string]any{"status": "done"}}, "UNKNOWN"},
		{"missing task", map[string]any{"code": 10000, "data": map[string]any{"status": "not_found"}}, "FAILURE"},
		{"expired", map[string]any{"code": 10000, "data": map[string]any{"status": "expired"}}, "FAILURE"},
		{"canceled", map[string]any{"code": 10000, "data": map[string]any{"status": "canceled"}}, "FAILURE"},
		{"unknown status", map[string]any{"code": 10000, "data": map[string]any{"status": "other"}}, "UNKNOWN"},
		{"null data on error", map[string]any{"code": 50400, "data": nil, "message": "Access Denied"}, "FAILURE"},
		{"error overrides success data", map[string]any{"code": 50510, "data": map[string]any{"status": "done", "video_url": "https://cdn.example/video.mp4"}}, "FAILURE"},
		{"qps limit", map[string]any{"code": 50429, "data": nil}, "UNKNOWN"},
		{"concurrency limit", map[string]any{"code": 50430, "data": nil}, "UNKNOWN"},
		{"server error", map[string]any{"code": 50500, "data": nil}, "UNKNOWN"},
		{"rpc error", map[string]any{"code": 50501, "data": nil}, "UNKNOWN"},
		{"missing envelope", nil, "UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, tc.body)
			require.NoError(t, err)
			assert.Equal(t, tc.want, value.(map[string]any)["status"])
		})
	}

	for _, body := range []any{nil, map[string]any{"code": 10000, "data": nil}, map[string]any{"code": 10000, "data": map[string]any{"task_id": 123}}, map[string]any{"code": 50215, "data": nil}} {
		_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"body": body})
		require.Error(t, err)
	}
	value, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"body": map[string]any{"code": 10000, "data": map[string]any{"task_id": "7392616336519610409"}}})
	require.NoError(t, err)
	assert.Equal(t, "7392616336519610409", value.(map[string]any)["taskId"])

	query, err := plugin.Engine.CallMember(t.Context(), "native", "decodeRequest", map[string]any{
		"body": map[string]any{"kind": "json", "value": map[string]any{"task_id": "task_public"}}, "query": map[string]any{"Action": []string{"CVSync2AsyncGetResult"}},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(query)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"query","taskIds":["task_public"]}`, string(encoded))

	taskData := map[string]any{"code": 10000, "data": map[string]any{"status": "done", "video_url": "https://cdn.example/video.mp4"}}
	value, err = plugin.Engine.Call(t.Context(), "listArtifacts", map[string]any{"status": "SUCCESS", "data": taskData})
	require.NoError(t, err)
	assert.Len(t, value, 1)
	value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "video", "data": taskData, "clientRequest": map[string]any{"method": "HEAD"}, "apiKey": "ak|sk"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"url": "https://cdn.example/video.mp4", "method": "HEAD", "credentialless": true}, value)
	_, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"artifactKey": "other", "data": taskData})
	require.ErrorContains(t, err, "artifact_not_found")

	t.Run("gateway chaining preserves terminal failures", func(t *testing.T) {
		for _, code := range []int{10000, 50500, 50429} {
			presented, err := plugin.Engine.CallMember(t.Context(), "native", "renderTask", map[string]any{}, map[string]any{
				"task_id": "task_public", "status": "FAILURE", "fail_reason": "Task polling failed",
				"data": map[string]any{"code": code, "data": nil},
			})
			require.NoError(t, err)
			parsed, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, presented)
			require.NoError(t, err)
			assert.Equal(t, "FAILURE", parsed.(map[string]any)["status"])
		}
	})
}

func TestOmniHumanUsageCannotBeOverriddenByRequest(t *testing.T) {
	plugin := loadOmniHumanPlugin(t)
	expression := `tier("base", u("seconds") * 0.07)`
	require.NoError(t, billing_setting.SmokeTestTaskExpr(expression, plugin.Meta.UsageSchema))
	for _, requestBody := range []map[string]any{
		{},
		{"videos": -1, "seconds": 0, "metadata": map[string]any{"videos": 1e30, "seconds": 300}},
		{"seconds": 1e30, "metadata": map[string]any{"duration": -1}},
	} {
		value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"requestBody": requestBody})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"seconds": int64(10), "videos": int64(1)}, value)
		facts, ok := value.(map[string]any)
		require.True(t, ok)
		snap := &billingexpr.BillingSnapshot{
			ExprString: expression, TaskUsageBilling: true, GroupRatio: 1, QuotaPerUnit: 500000, UsageFacts: facts,
		}
		reserved, err := billingexpr.ComputeTieredQuotaWithRequest(snap, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
		require.NoError(t, err)
		assert.Equal(t, 350000, reserved.ActualQuotaAfterGroup, "ten seconds at $0.07 reserves $0.70")
		snap.EstimatedQuotaAfterGroup = reserved.ActualQuotaAfterGroup

		task := &model.Task{Properties: model.Properties{OriginModelName: "omnihuman-2.0"}}
		result, err := taskplugin.New(plugin).ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, []byte(`{"code":10000,"data":{"status":"done","video_url":"https://cdn.example/video.mp4"}}`))
		require.NoError(t, err)
		assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
		assert.Equal(t, map[string]any{"seconds": float64(10), "videos": float64(1)}, result.UsageFacts)
		settled, _, err := service.EvaluateTaskCompletionUsage(snap, result.UsageFacts)
		require.NoError(t, err)
		assert.Equal(t, 350000, settled.ActualQuotaAfterGroup, "completion keeps the agreed ten-second charge")
	}
	// Existing per-video expressions still validate; legacy per-call pricing
	// must not acquire a tenfold multiplier from the new seconds fact.
	require.NoError(t, billing_setting.SmokeTestTaskExpr(`tier("base", u("videos") * 0.7)`, plugin.Meta.UsageSchema))
	value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"usagePurpose": "billing_ratios"})
	require.NoError(t, err)
	assert.Nil(t, value)
	for _, status := range []string{"FAILURE", "QUEUED", "IN_PROGRESS"} {
		value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{}, map[string]any{"status": status})
		require.NoError(t, err)
		assert.Nil(t, value)
	}
}
