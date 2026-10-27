package plugins_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
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

func TestOmniHumanSignedRequestSurvivesHTTPTransport(t *testing.T) {
	service.InitHttpClient()
	plugin := loadOmniHumanPlugin(t)
	type receivedRequest struct {
		body    string
		headers http.Header
		query   string
		err     error
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- receivedRequest{body: string(body), headers: r.Header.Clone(), query: r.URL.RawQuery, err: err}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":10000,"data":{"task_id":"test-task"}}`)
	}))
	t.Cleanup(server.Close)
	client := service.GetHttpClient()
	original := *client
	t.Cleanup(func() { *client = original })
	client.Transport = server.Client().Transport
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: server.URL, ApiKey: "test-ak|test-sk", UpstreamModelName: "omnihuman-2.0",
		},
		OriginModelName: "omnihuman-2.0",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public", Action: "image_to_video"},
	}
	body := omniHumanBody()
	body["prompt"] = "根据图片和音频，生成一个奶茶的广告，还要有一个可爱的女孩"
	// The reference hook signs the same payload before host conversion/transport.
	expected, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl": server.URL, "apiKey": info.ApiKey, "model": info.OriginModelName, "requestBody": body,
	})
	require.NoError(t, err)
	descriptor := expected.(map[string]any)
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Set("task_request", body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	response, err := adaptor.DoRequest(c, info, reader)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	wire := <-received
	require.NoError(t, wire.err)
	assert.Equal(t, descriptor["body"], wire.body, "the signed UTF-8 body must not be re-encoded by the gateway")
	assert.Equal(t, "Action=CVSync2AsyncSubmitTask&Version=2024-06-06", wire.query)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(wire.body))), wire.headers.Get("X-Content-Sha256"))
	for name, value := range descriptor["headers"].(map[string]any) {
		assert.Equal(t, value, wire.headers.Get(name), name)
	}
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

type omniHumanMediaTransport func(*http.Request) (*http.Response, error)

func (f omniHumanMediaTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func omniHumanMP4Box(kind string, payload []byte) []byte {
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header, uint32(8+len(payload)))
	copy(header[4:], kind)
	return append(header, payload...)
}

func omniHumanMovie(version byte, scale uint32, ticks uint64) []byte {
	payload := make([]byte, 112)
	payload[0] = version
	if version == 1 {
		binary.BigEndian.PutUint32(payload[20:], scale)
		binary.BigEndian.PutUint64(payload[24:], ticks)
	} else {
		binary.BigEndian.PutUint32(payload[12:], scale)
		binary.BigEndian.PutUint32(payload[16:], uint32(ticks))
	}
	return omniHumanMP4Box("moov", omniHumanMP4Box("mvhd", payload))
}

func TestOmniHumanSettlesOnVideoDuration(t *testing.T) {
	service.InitHttpClient()
	plugin := loadOmniHumanPlugin(t)
	ftyp := omniHumanMP4Box("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41"))
	mdat := omniHumanMP4Box("mdat", make([]byte, 300<<10))
	tail := bytes.Join([][]byte{ftyp, mdat, omniHumanMovie(1, 1000, 12500)}, nil)
	largeHeader := make([]byte, 16)
	binary.BigEndian.PutUint32(largeHeader, 1)
	copy(largeHeader[4:], "moov")
	v1 := omniHumanMovie(1, 1000, 8000)[8:]
	binary.BigEndian.PutUint64(largeHeader[8:], uint64(16+len(v1)))
	for _, tc := range []struct {
		name       string
		file       []byte
		status     int
		whole      bool
		seconds    float64
		wantQuota  int
		wantReads  int
		wantSource string
	}{
		{"short video refunds", omniHumanMovie(0, 1000, 6000), 200, true, 6, 210000, 1, "video_metadata"},
		{"tail moov rounds up fractional seconds and charges extra", tail, 200, false, 13, 455000, 2, "video_metadata"},
		{"extended box size", append(largeHeader, v1...), 200, false, 8, 280000, 1, "video_metadata"},
		{"zero timescale", omniHumanMovie(0, 0, 6000), 200, false, 10, 350000, 1, "default"},
		{"zero duration", omniHumanMovie(0, 1000, 0), 200, false, 10, 350000, 1, "default"},
		{"duration above provider limit", omniHumanMovie(0, 1, 301), 200, false, 10, 350000, 1, "default"},
		{"overflow duration", omniHumanMovie(1, 1, ^uint64(0)), 200, false, 10, 350000, 1, "default"},
		{"unsupported version", omniHumanMovie(2, 1000, 6000), 200, false, 10, 350000, 1, "default"},
		{"invalid file", []byte("not a video"), 200, false, 10, 350000, 1, "default"},
		{"expired media URL", nil, 403, false, 10, 350000, 1, "default"},
		{"redirect is not followed", nil, 302, false, 10, 350000, 1, "default"},
		{"oversized unbounded response", make([]byte, jsplugin.MaxFetchBodyBytes+1), 200, true, 10, 350000, 1, "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := service.GetSSRFProtectedHTTPClient()
			original := *client
			t.Cleanup(func() { *client = original })
			assetURL := "https://93.184.216.34/output.mp4?signature=asset-token"
			asset, err := url.Parse(assetURL)
			require.NoError(t, err)
			client.Jar, err = cookiejar.New(nil)
			require.NoError(t, err)
			client.Jar.SetCookies(asset, []*http.Cookie{{Name: "session", Value: "must-not-leak"}})
			var ranges []string
			// Intercept only the media client's transport: policy checks, the
			// adaptor, ranged reads and settlement still run through real code.
			client.Transport = omniHumanMediaTransport(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, assetURL, r.URL.String())
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("Cookie"))
				ranges = append(ranges, r.Header.Get("Range"))
				recorder := httptest.NewRecorder()
				if tc.status != http.StatusOK {
					recorder.Header().Set("Location", "http://127.0.0.1/private")
					recorder.WriteHeader(tc.status)
				} else if tc.whole {
					_, _ = recorder.Write(tc.file)
				} else {
					http.ServeContent(recorder, r, "output.mp4", time.Time{}, bytes.NewReader(tc.file))
				}
				return recorder.Result(), nil
			})
			adaptor := taskplugin.New(plugin)
			adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://cv.byteplusapi.com", ApiKey: "ak|sk", UpstreamModelName: "omnihuman-2.0",
			}})
			body, err := common.Marshal(map[string]any{"code": 10000, "data": map[string]any{"status": "done", "video_url": assetURL}})
			require.NoError(t, err)
			task := &model.Task{Properties: model.Properties{OriginModelName: "omnihuman-2.0"}}
			if tc.wantSource == "video_metadata" {
				task.PrivateData.PluginState = []byte(`{"input_audio_url":"https://93.184.216.34/audio.mp3"}`)
			}
			result, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, body)
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", result.Status)
			assert.JSONEq(t, fmt.Sprintf(`{"billing_seconds":%g,"billing_duration_source":%q}`, tc.seconds, tc.wantSource), string(result.PluginState))
			facts := map[string]any{"videos": float64(1)}
			if tc.wantSource == "video_metadata" {
				facts["seconds"] = tc.seconds
			}
			assert.Equal(t, facts, result.UsageFacts)
			snapshot := &billingexpr.BillingSnapshot{
				ExprString: `tier("base", u("seconds") * 0.07)`, TaskUsageBilling: true, GroupRatio: 1, QuotaPerUnit: 500000,
				UsageFacts: map[string]any{"seconds": 10, "videos": 1}, EstimatedQuotaAfterGroup: 350000,
			}
			settled, _, err := service.EvaluateTaskCompletionUsage(snapshot, result.UsageFacts)
			require.NoError(t, err)
			assert.Equal(t, tc.wantQuota, settled.ActualQuotaAfterGroup)
			assert.Len(t, ranges, tc.wantReads)
			if tc.wantReads == 2 {
				offset := len(ftyp) + len(mdat)
				assert.Equal(t, []string{"bytes=0-65535", fmt.Sprintf("bytes=%d-%d", offset, offset+65535)}, ranges)
			}
		})
	}
}

func TestOmniHumanMediaFetchRestrictions(t *testing.T) {
	service.InitHttpClient()
	settings := system_setting.GetFetchSetting()
	original := *settings
	t.Cleanup(func() { *settings = original })
	*settings = system_setting.FetchSetting{EnableSSRFProtection: true, AllowedPorts: []string{"80", "443"}, ApplyIPFilterForDomain: true}
	client := service.GetSSRFProtectedHTTPClient()
	originalClient := *client
	t.Cleanup(func() { *client = originalClient })
	client.Transport = omniHumanMediaTransport(func(*http.Request) (*http.Response, error) {
		t.Error("rejected fetch must not reach the transport")
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	source, err := builtinplugins.Source("omnihuman")
	require.NoError(t, err)
	// Exercise the same host fetch binding with intentionally invalid requests.
	source = strings.Replace(source, "export function parseTaskResult(ctx, body) {", "export function parseTaskResult(ctx, body) { return utils.fetch(body); }\nfunction originalResult(ctx, body) {", 1)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "omnihuman"})
	require.NoError(t, err)
	adaptor := taskplugin.New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://cv.byteplusapi.com"}})
	for _, tc := range []struct {
		name    string
		request map[string]any
		want    string
	}{
		{"private address", map[string]any{"url": "http://127.0.0.1/secret?token=hidden", "credentialless": true}, "fetch policy"},
		{"IPv6 loopback", map[string]any{"url": "http://[::1]/secret", "credentialless": true}, "fetch policy"},
		{"cloud metadata", map[string]any{"url": "http://169.254.169.254/secret", "credentialless": true}, "fetch policy"},
		{"URL credentials", map[string]any{"url": "https://user:hidden@93.184.216.34/", "credentialless": true}, "credentials or a fragment"},
		{"authorization header", map[string]any{"url": "https://93.184.216.34/", "credentialless": true, "headers": map[string]any{"authorization": "Bearer hidden"}}, "only Range and Accept"},
		{"cookie header", map[string]any{"url": "https://93.184.216.34/", "credentialless": true, "headers": map[string]any{"Cookie": "session=hidden"}}, "only Range and Accept"},
		{"ordinary fetch still needs allowed host", map[string]any{"url": "https://93.184.216.34/", "credentialless": false}, "not allowed"},
		{"invalid flag", map[string]any{"url": "https://93.184.216.34/", "credentialless": "true"}, "must be a boolean"},
		{"audio needs credentialless request", map[string]any{"url": "https://93.184.216.34/", "responseType": "audio_duration"}, "requires a credentialless GET"},
		{"audio HEAD is not a duration", map[string]any{"url": "https://93.184.216.34/", "responseType": "audio_duration", "credentialless": true, "method": "HEAD"}, "requires a credentialless GET"},
		{"partial audio must not underbill", map[string]any{"url": "https://93.184.216.34/", "responseType": "audio_duration", "credentialless": true, "headers": map[string]any{"Range": "bytes=0-100"}}, "does not allow partial audio"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := common.Marshal(tc.request)
			require.NoError(t, err)
			_, err = adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: 200}, body)
			require.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "hidden")
		})
	}
}

func TestOmniHumanReservesAudioThenSettlesVideo(t *testing.T) {
	service.InitHttpClient()
	plugin := loadOmniHumanPlugin(t)
	wav := func(seconds float64, sampleRate uint32) []byte {
		dataBytes := uint32(seconds * float64(sampleRate) * 2)
		body := make([]byte, 44+dataBytes)
		copy(body, "RIFF")
		binary.LittleEndian.PutUint32(body[4:], 36+dataBytes)
		copy(body[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(body[16:], 16)
		binary.LittleEndian.PutUint16(body[20:], 1)
		binary.LittleEndian.PutUint16(body[22:], 1)
		binary.LittleEndian.PutUint32(body[24:], sampleRate)
		binary.LittleEndian.PutUint32(body[28:], sampleRate*2)
		binary.LittleEndian.PutUint16(body[32:], 2)
		binary.LittleEndian.PutUint16(body[34:], 16)
		copy(body[36:], "data")
		binary.LittleEndian.PutUint32(body[40:], dataBytes)
		return body
	}
	// MPEG-1 Layer III, 44.1 kHz: each frame carries 1152 samples even
	// when its bitrate changes. This catches duration guesses from file size.
	var mp3 []byte
	for range 100 {
		for _, frame := range []struct{ header, length int }{{0x90, 417}, {0xb0, 626}} {
			data := make([]byte, frame.length)
			copy(data, []byte{0xff, 0xfb, byte(frame.header), 0})
			mp3 = append(mp3, data...)
		}
	}
	for _, tc := range []struct {
		name       string
		audio      []byte
		status     int
		reserved   float64
		videoTicks uint64
		seconds    float64
		quota      int
		wantSource string
	}{
		{"WAV reserves rounded audio below default", wav(8.25, 8000), 200, 9, 0, 9, 315000, "audio_metadata"},
		{"WAV above 1 MiB reserves rounded audio", wav(12.5, 48000), 200, 13, 0, 13, 455000, "audio_metadata"},
		{"integer audio does not gain a second", wav(10, 8000), 200, 10, 0, 10, 350000, "audio_metadata"},
		{"maximum audio stays within limit", wav(300, 8000), 200, 300, 0, 300, 10500000, "audio_metadata"},
		{"variable bitrate MP3 counts frames then rounds up", mp3, 200, 6, 0, 6, 210000, "audio_metadata"},
		{"longer video charges the rounded difference", wav(11.1, 8000), 200, 12, 12001, 13, 455000, "video_metadata"},
		{"shorter video refunds the rounded difference", wav(12.5, 8000), 200, 13, 8250, 9, 315000, "video_metadata"},
		{"same rounded duration keeps charge", wav(8.25, 8000), 200, 9, 8750, 9, 315000, "video_metadata"},
		{"integer video does not gain a second", wav(10.1, 8000), 200, 11, 10000, 10, 350000, "video_metadata"},
		{"video corrects default when audio unavailable", nil, 403, 10, 12500, 13, 455000, "video_metadata"},
		{"out of range duration keeps default", wav(301, 8000), 200, 10, 0, 10, 350000, "default"},
		{"invalid audio keeps default", []byte("not audio"), 200, 10, 0, 10, 350000, "default"},
		{"empty audio keeps default", nil, 200, 10, 0, 10, 350000, "default"},
		{"expired audio keeps default", nil, 403, 10, 0, 10, 350000, "default"},
		{"unsolicited partial audio keeps default", mp3, 206, 10, 0, 10, 350000, "default"},
		{"oversized audio keeps default", make([]byte, jsplugin.MaxFetchAudioBodyBytes+1), 200, 10, 0, 10, 350000, "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := service.GetSSRFProtectedHTTPClient()
			original := *client
			t.Cleanup(func() { *client = original })
			var paths []string
			client.Transport = omniHumanMediaTransport(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("Cookie"))
				response := httptest.NewRecorder()
				if r.URL.Path == "/video.mp4" {
					if tc.videoTicks == 0 {
						response.WriteHeader(http.StatusForbidden)
					} else {
						http.ServeContent(response, r, "video.mp4", time.Time{}, bytes.NewReader(omniHumanMovie(0, 1000, tc.videoTicks)))
					}
				} else {
					assert.Equal(t, "/audio", r.URL.Path, "detect audio by content, not the signed URL extension")
					assert.Empty(t, r.Header.Get("Range"))
					response.WriteHeader(tc.status)
					_, _ = response.Write(tc.audio)
				}
				return response.Result(), nil
			})
			info := &relaycommon.RelayInfo{
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://cv.byteplusapi.com", ApiKey: "ak|sk", UpstreamModelName: "omnihuman-2.0"},
				OriginModelName: "omnihuman-2.0",
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public", Action: "image_to_video"},
			}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			request := omniHumanBody()
			request["audio_url"] = "https://93.184.216.34/audio?signature=private"
			request["state"] = map[string]any{"billing_seconds": 1, "billing_duration_source": "audio_metadata"}
			c.Set("task_request", request)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			reserved, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"seconds": tc.reserved, "videos": float64(1)}, reserved)
			// Repeated validation/usage reads reuse preparation, without reading
			// mutable audio again or letting client state replace measured facts.
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			repeated, err := adaptor.ExtractUsageFactsValidated(c, info)
			require.NoError(t, err)
			assert.Equal(t, reserved, repeated)
			assert.Equal(t, []string{"/audio"}, paths, "audio is measured before submission and pre-consumption")
			snapshot := &billingexpr.BillingSnapshot{
				ExprString: `tier("base", u("seconds") * 0.07)`, TaskUsageBilling: true, GroupRatio: 1, QuotaPerUnit: 500000,
				UsageFacts: reserved,
			}
			preConsumed, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: reserved})
			require.NoError(t, err)
			assert.EqualValues(t, tc.reserved*35000, preConsumed.ActualQuotaAfterGroup)
			snapshot.EstimatedQuotaAfterGroup = preConsumed.ActualQuotaAfterGroup
			accepted, taskErr := adaptor.ParseResponse(c, &http.Response{
				StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":10000,"data":{"task_id":"upstream-id"}}`)),
			}, info)
			require.Nil(t, taskErr)
			require.NotNil(t, accepted)
			var submissionState map[string]any
			require.NoError(t, common.Unmarshal(accepted.PluginState, &submissionState))
			assert.Equal(t, tc.reserved, submissionState["billing_seconds"])
			assert.NotContains(t, submissionState, "input_audio_url")
			// Rebuild the adaptor like a background poll after restart: only
			// persisted state supplies the frozen audio measurement.
			poller := taskplugin.New(plugin)
			poller.Init(info)
			task := &model.Task{
				Properties:  model.Properties{OriginModelName: "omnihuman-2.0"},
				PrivateData: model.TaskPrivateData{PluginState: accepted.PluginState},
			}
			result, err := poller.ParseTaskResult(task, &http.Response{StatusCode: 200}, []byte(`{"code":10000,"data":{"status":"done","video_url":"https://93.184.216.34/video.mp4"}}`))
			require.NoError(t, err)
			assert.Equal(t, "SUCCESS", result.Status)
			var state map[string]any
			require.NoError(t, common.Unmarshal(result.PluginState, &state))
			assert.Equal(t, tc.wantSource, state["billing_duration_source"])
			assert.Equal(t, tc.seconds, state["billing_seconds"])
			assert.NotContains(t, state, "input_audio_url")
			if tc.wantSource == "video_metadata" {
				assert.Equal(t, tc.seconds, result.UsageFacts["seconds"])
			} else {
				assert.NotContains(t, result.UsageFacts, "seconds")
			}
			settled, _, err := service.EvaluateTaskCompletionUsage(snapshot, result.UsageFacts)
			require.NoError(t, err)
			assert.Equal(t, tc.quota, settled.ActualQuotaAfterGroup)
			assert.Equal(t, []string{"/audio", "/video.mp4"}, paths)
		})
	}
}
