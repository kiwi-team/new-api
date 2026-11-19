package plugins_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
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
		model:     "kling-v3",
		requestBody: map[string]any{
			"model": "kling-v3",
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
			"model":    "kling-v3",
			"prompt":   "camera orbit",
			"image":    "https://cdn.example/frame.png",
			"duration": float64(10),
			"metadata": map[string]any{"mode": "pro"},
		},
		wantUsageKeys:  []string{"units"},
		wantVendorName: "kling",
	})

	source, err := builtinplugins.Source("kling")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "kling"})
	require.NoError(t, err)
	submit := func(t *testing.T, decoder []string, model string, body map[string]any) (*pluginadaptor.TaskAdaptor, *gin.Context, *relaycommon.RelayInfo, *dto.TaskError) {
		t.Helper()
		value, err := plugin.Engine.CallPath(t.Context(), decoder[0], decoder[1:], map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": body}})
		require.NoError(t, err)
		info := &relaycommon.RelayInfo{
			ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api-singapore.klingai.com", ApiKey: "ak|sk", UpstreamModelName: model},
			OriginModelName: model,
			TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		}
		adaptor := pluginadaptor.New(plugin)
		adaptor.Init(info)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		c.Set("task_request", alibabaObject(t, value)["requestBody"])
		return adaptor, c, info, adaptor.ValidateRequestAndSetAction(c, info)
	}
	sentBody := func(t *testing.T, adaptor *pluginadaptor.TaskAdaptor, c *gin.Context, info *relaycommon.RelayInfo) map[string]any {
		t.Helper()
		reader, err := adaptor.BuildRequestBody(c, info)
		require.NoError(t, err)
		encoded, err := io.ReadAll(reader)
		require.NoError(t, err)
		var sent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &sent))
		return sent
	}
	native := []string{"native", "decodeSubmit"}
	openaiVideo := []string{"protocols", "openai_video", "decodeRequest"}

	// Kling bills the duration it sends, and metadata.duration (the whole body
	// on native routes) overrides the top-level one, so the plugin bounds that
	// value to a whole number of seconds up to 3600 before the host reserves
	// quota. Kling checks the durations of each model itself.
	t.Run("bounds the billed duration before billing", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			decoder []string
			model   string
			body    map[string]any
		}{
			{"native duration above an hour", native, "kling-v3", map[string]any{"model_name": "kling-v3", "prompt": "a cat", "duration": 1e12}},
			{"native fractional duration", native, "kling-v3", map[string]any{"model_name": "kling-v3", "prompt": "a cat", "duration": "5.5"}},
			{"metadata duration behind valid seconds", openaiVideo, "kling-v3", map[string]any{"model": "kling-v3", "prompt": "a cat", "seconds": "5", "metadata": map[string]any{"duration": 3601}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := plugin.Engine.CallPath(t.Context(), tc.decoder[0], tc.decoder[1:], map[string]any{"model": tc.model, "body": map[string]any{"kind": "json", "value": tc.body}})
				if err == nil {
					_, _, _, taskErr := submit(t, tc.decoder, tc.model, tc.body)
					require.NotNil(t, taskErr)
					assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
					err = taskErr.Error
				}
				assert.ErrorContains(t, err, "duration must be a positive integer of at most 3600")
			})
		}

		adaptor, c, info, taskErr := submit(t, openaiVideo, "kling-v3", map[string]any{"model": "kling-v3", "prompt": "a cat", "seconds": "5", "metadata": map[string]any{"duration": "10"}})
		require.Nil(t, taskErr)
		assert.Equal(t, "10", sentBody(t, adaptor, c, info)["duration"])
		facts, err := adaptor.ExtractUsageFactsValidated(c, info)
		require.NoError(t, err)
		assert.InDelta(t, 6.0, facts["units"], 1e-9, "kling-v3 std costs 0.6 units per second")

		adaptor, c, info, taskErr = submit(t, openaiVideo, "kling-v2-6", map[string]any{"model": "kling-v2-6", "prompt": "a cat", "seconds": "7"})
		require.Nil(t, taskErr)
		assert.Equal(t, "7", sentBody(t, adaptor, c, info)["duration"], "Kling checks the model's durations")
		facts, err = adaptor.ExtractUsageFactsValidated(c, info)
		require.NoError(t, err)
		assert.InDelta(t, 2.1, facts["units"], 1e-9)
	})

	// A host-protocol size gets the nearest of Kling's text-to-video aspect
	// ratios.
	t.Run("sizes take the nearest aspect ratio", func(t *testing.T) {
		for size, want := range map[string]string{"1792x1024": "16:9", "720x720": "1:1", "1024x1792": "9:16", "2560x1080": "16:9"} {
			adaptor, c, info, taskErr := submit(t, openaiVideo, "kling-v3", map[string]any{"model": "kling-v3", "prompt": "a cat", "size": size})
			require.Nil(t, taskErr)
			assert.Equal(t, want, sentBody(t, adaptor, c, info)["aspect_ratio"], size)
		}
		_, _, _, taskErr := submit(t, openaiVideo, "kling-v3", map[string]any{"model": "kling-v3", "prompt": "a cat", "size": "large"})
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		assert.ErrorContains(t, taskErr.Error, "size must be WIDTHxHEIGHT")
	})

	// A native request reaches Kling as the client sent it, for Kling to check:
	// no model alias, no default aspect ratio, cfg_scale, mode or duration. The
	// reservation follows Kling's units for the mode and audio at Kling's
	// defaults, and the dearest units the plugin knows for a mode or audio the
	// model's table lacks.
	t.Run("native bodies and their units", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			model     string
			body      map[string]any
			wantUnits float64
		}{
			{
				name: "defaults", model: "kling-v2-5-turbo",
				body:      map[string]any{"model_name": "kling-v2-5-turbo", "prompt": "a cat"},
				wantUnits: 1.5,
			},
			{
				name: "2.6 pro with audio", model: "kling-v2-6",
				body:      map[string]any{"model_name": "kling-v2-6", "prompt": "a cat", "mode": "pro", "sound": "on", "duration": "10", "aspect_ratio": "9:16"},
				wantUnits: 10,
			},
			{
				name: "2.6 with a reference voice", model: "kling-v2-6",
				body:      map[string]any{"model_name": "kling-v2-6", "image": "https://cdn.example/a.png", "mode": "pro", "sound": "on", "voice_list": []any{map[string]any{"voice_id": "v1"}}},
				wantUnits: 6,
			},
			{
				name: "v3 in 4k", model: "kling-v3",
				body:      map[string]any{"model_name": "kling-v3", "prompt": "a cat", "mode": "4k", "duration": 15},
				wantUnits: 45,
			},
			{
				name: "a duration the model does not list", model: "kling-v2-6",
				body:      map[string]any{"model_name": "kling-v2-6", "prompt": "a cat", "duration": "8"},
				wantUnits: 2.4,
			},
			{
				name: "audio on a model without an audio price", model: "kling-v2-5-turbo",
				body:      map[string]any{"model_name": "kling-v2-5-turbo", "prompt": "a cat", "sound": "on"},
				wantUnits: 4.5,
			},
			{
				name: "audio in a mode the model's audio table lacks", model: "kling-v2-6",
				body:      map[string]any{"model_name": "kling-v2-6", "prompt": "a cat", "sound": "on"},
				wantUnits: 5,
			},
			{
				name: "a mode the model does not list", model: "kling-v2-6",
				body:      map[string]any{"model_name": "kling-v2-6", "prompt": "a cat", "mode": "4k"},
				wantUnits: 2.5,
			},
			{
				name: "a model the plugin does not list", model: "kling-v9",
				body:      map[string]any{"model_name": "kling-v9", "prompt": "a cat"},
				wantUnits: 4.5,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				adaptor, c, info, taskErr := submit(t, native, tc.model, tc.body)
				require.Nil(t, taskErr)
				assert.Equal(t, decodePluginValue(t, tc.body), sentBody(t, adaptor, c, info))
				facts, err := adaptor.ExtractUsageFactsValidated(c, info)
				require.NoError(t, err)
				assert.InDelta(t, tc.wantUnits, facts["units"], 1e-9)
			})
		}

		for _, tc := range []struct {
			name    string
			body    map[string]any
			wantErr string
		}{
			{"model is not model_name", map[string]any{"model": "kling-v3", "prompt": "a cat"}, "model_name is required"},
			{"callbacks carry Kling's task ids", map[string]any{"model_name": "kling-v3", "prompt": "a cat", "callback_url": "https://client.example/hook"}, "callback_url is not supported"},
			{"external ids are not looked up", map[string]any{"model_name": "kling-v3", "prompt": "a cat", "external_task_id": "mine-1"}, "external_task_id is not supported"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"decodeSubmit"}, map[string]any{"body": map[string]any{"kind": "json", "value": tc.body}})
				assert.ErrorContains(t, err, tc.wantErr)
			})
		}
	})

	// A /v1/videos client cannot pass a body off as a native one.
	t.Run("a vendorBody field is not a native body", func(t *testing.T) {
		adaptor, c, info, taskErr := submit(t, openaiVideo, "kling-v2-6", map[string]any{"model": "kling-v2-6", "prompt": "a cat",
			"vendorBody": map[string]any{"prompt": "x", "callback_url": "https://client.example/hook"}})
		require.Nil(t, taskErr)
		assert.Equal(t, map[string]any{"model_name": "kling-v2-6", "prompt": "a cat", "mode": "std", "duration": "5"}, sentBody(t, adaptor, c, info))
	})

	t.Run("native replies keep Kling's envelope", func(t *testing.T) {
		processing := map[string]any{"code": float64(0), "message": "SUCCEED", "request_id": "req-1", "data": map[string]any{"task_id": "task_public", "task_status": "processing"}}
		failed, err := plugin.Engine.CallPath(t.Context(), "native", []string{"taskStatus"}, map[string]any{},
			map[string]any{"task_id": "task_public", "status": "FAILURE", "fail_reason": "task timed out", "data": processing})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{
			"code": float64(0), "message": "SUCCEED", "request_id": "req-1",
			"data": map[string]any{"task_id": "task_public", "task_status": "failed", "task_status_msg": "task timed out"},
		}, alibabaObject(t, failed), "a task the gateway failed does not read as running")

		rendered, err := plugin.Engine.CallPath(t.Context(), "native", []string{"error"}, map[string]any{},
			map[string]any{"code": "invalid_request", "message": "duration must be one of 5, 10", "httpStatus": 400, "requestId": "req-2"})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"code": float64(1201), "message": "duration must be one of 5, 10", "request_id": "req-2"}, alibabaObject(t, rendered))
	})
}

func TestKling4(t *testing.T) {
	source, err := builtinplugins.Source("kling")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "kling"})
	require.NoError(t, err)
	call := func(t *testing.T, name string, args ...any) map[string]any {
		t.Helper()
		value, err := plugin.Engine.CallPath(t.Context(), name, nil, args...)
		require.NoError(t, err)
		return alibabaObject(t, value)
	}
	type submission struct {
		url   string
		body  map[string]any
		units float64
		state map[string]any
	}
	validate := func(t *testing.T, decoder []string, model string, body map[string]any) (*pluginadaptor.TaskAdaptor, *gin.Context, *relaycommon.RelayInfo, any, *dto.TaskError) {
		t.Helper()
		value, err := plugin.Engine.CallPath(t.Context(), decoder[0], decoder[1:], map[string]any{"model": model, "body": map[string]any{"kind": "json", "value": body}})
		require.NoError(t, err)
		info := &relaycommon.RelayInfo{
			ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api-singapore.klingai.com", ApiKey: "ak|sk", UpstreamModelName: model},
			OriginModelName: model,
			TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		}
		adaptor := pluginadaptor.New(plugin)
		adaptor.Init(info)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
		requestBody := alibabaObject(t, value)["requestBody"]
		c.Set("task_request", requestBody)
		return adaptor, c, info, requestBody, adaptor.ValidateRequestAndSetAction(c, info)
	}
	submit := func(t *testing.T, decoder []string, model string, body map[string]any) submission {
		t.Helper()
		adaptor, c, info, requestBody, taskErr := validate(t, decoder, model, body)
		require.Nil(t, taskErr)
		reader, err := adaptor.BuildRequestBody(c, info)
		require.NoError(t, err)
		encoded, err := io.ReadAll(reader)
		require.NoError(t, err)
		var sent map[string]any
		require.NoError(t, common.Unmarshal(encoded, &sent))
		url, err := adaptor.BuildRequestURL(info)
		require.NoError(t, err)
		facts, err := adaptor.ExtractUsageFactsValidated(c, info)
		require.NoError(t, err)
		parsed := call(t, "parseSubmitResponse", map[string]any{"model": model, "requestBody": requestBody},
			map[string]any{"statusCode": 200, "body": map[string]any{"code": 0, "message": "SUCCEED", "data": map[string]any{"id": "kling_4", "status": "submitted"}}})
		assert.Equal(t, "kling_4", parsed["taskId"])
		state, _ := parsed["state"].(map[string]any)
		return submission{url: url, body: sent, units: facts["units"].(float64), state: state}
	}

	// Kling 4.0 bills units per second of a total duration: the output, or,
	// with an input video, twice the output or input plus output. Kling picks
	// the length when the request has none, and only Kling knows how long an
	// input video is, so the longest of each is reserved.
	t.Run("native bodies and their reservation", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			decoder   string
			endpoint  string
			body      map[string]any
			wantUnits float64
		}{
			{"text at Kling's defaults", "kling4TextToVideo", "text-to-video", map[string]any{"prompt": "a cat"}, 45},
			{"text at 720p for 10 seconds", "kling4TextToVideo", "text-to-video",
				map[string]any{"prompt": "a cat", "settings": map[string]any{"resolution": "720p", "aspect_ratio": "9:16", "duration": 10}}, 10},
			{"image at 1080p for 5 seconds", "kling4ImageToVideo", "image-to-video",
				map[string]any{"contents": []any{map[string]any{"type": "prompt", "text": "a cat"}, map[string]any{"type": "first_frame", "url": "https://cdn.example/a.png"}},
					"settings": map[string]any{"duration": "5"}}, 7.5},
			{"omni with reference images only", "kling4OmniVideo", "omni-video",
				map[string]any{"contents": []any{map[string]any{"type": "prompt", "text": "<<<image_1>>> walks"}, map[string]any{"type": "refer_image", "url": "https://cdn.example/a.png", "id": "image_1"}},
					"settings": map[string]any{"resolution": "720p", "duration": 10, "aspect_ratio": "adaptive"}}, 10},
			{"omni with a reference video", "kling4OmniVideo", "omni-video",
				map[string]any{"contents": []any{map[string]any{"type": "prompt", "text": "in the style of <<<video_1>>>"}, map[string]any{"type": "feature_video", "url": "https://cdn.example/a.mp4", "id": "video_1"}},
					"settings": map[string]any{"duration": 10}}, 36},
			{"omni editing a video of unknown length", "kling4OmniVideo", "omni-video",
				map[string]any{"contents": []any{map[string]any{"type": "prompt", "text": "make it night"}, map[string]any{"type": "base_video", "url": "https://cdn.example/a.mp4", "id": "video_1"}},
					"settings": map[string]any{"resolution": "720p"}}, 36},
			{"omni with an element", "kling4OmniVideo", "omni-video",
				map[string]any{"contents": []any{map[string]any{"type": "prompt", "text": "<<<element_1>>> waves"}, map[string]any{"type": "element", "element_id": "123", "id": "element_1"}},
					"settings": map[string]any{"resolution": "720p", "duration": 30}}, 36},
		} {
			t.Run(tc.name, func(t *testing.T) {
				sent := submit(t, []string{"native", tc.decoder}, "kling-4.0", tc.body)
				assert.Equal(t, "https://api-singapore.klingai.com/"+tc.endpoint+"/kling-4.0", sent.url)
				assert.Equal(t, decodePluginValue(t, tc.body), sent.body, "Kling receives the body as the client wrote it")
				assert.InDelta(t, tc.wantUnits, sent.units, 1e-9)
			})
		}

		sent := submit(t, []string{"native", "kling4OmniVideo"}, "kling-4.0", map[string]any{
			"contents": []any{map[string]any{"type": "feature_video", "url": "https://cdn.example/in.mp4"}},
			"settings": map[string]any{"duration": 5},
		})
		assert.Equal(t, "video", sent.state["videoInput"])
		ctx := map[string]any{"taskId": "kling_4", "state": sent.state}
		result := map[string]any{"id": "kling_4", "status": "succeeded"}
		reply := map[string]any{"code": 0, "data": []any{result}}
		assert.Equal(t, true, call(t, "parseTaskResult", ctx, reply)["billingPending"])
		result["usage"] = map[string]any{"input_duration": "8.03", "output_duration": "5"}
		facts := call(t, "extractUsageOnComplete", ctx, nil, reply)
		assert.InDelta(t, 13.03, facts["billable_seconds"], 1e-9)
		assert.Equal(t, "video", facts["video_input"])

		for _, tc := range []struct {
			name    string
			body    map[string]any
			wantErr string
		}{
			{"callbacks carry Kling's task ids", map[string]any{"prompt": "a cat", "options": map[string]any{"callback_url": "https://client.example/hook"}}, "options.callback_url is not supported"},
			{"external ids are not looked up", map[string]any{"prompt": "a cat", "options": map[string]any{"external_task_id": "mine-1"}}, "options.external_task_id is not supported"},
			{"a duration above Kling 4.0's 30 seconds", map[string]any{"prompt": "a cat", "settings": map[string]any{"duration": 31}}, "duration must be an integer from 3 to 30"},
			{"a duration below its 3 seconds", map[string]any{"prompt": "a cat", "settings": map[string]any{"duration": "2"}}, "duration must be an integer from 3 to 30"},
			{"a fractional duration", map[string]any{"prompt": "a cat", "settings": map[string]any{"duration": "5.5"}}, "duration must be an integer from 3 to 30"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"kling4TextToVideo"}, map[string]any{"body": map[string]any{"kind": "json", "value": tc.body}})
				assert.ErrorContains(t, err, tc.wantErr)
			})
		}
	})

	// A Kling 4.0 task is looked up at GET /tasks?task_ids=, which answers a
	// list; the state the submission saved marks the task as one.
	t.Run("query and result", func(t *testing.T) {
		query := call(t, "buildQueryRequest", map[string]any{"taskId": "kling_4", "baseUrl": "https://api-singapore.klingai.com", "apiKey": "ak|sk", "state": map[string]any{"resolution": "1080p"}})
		assert.Equal(t, "https://api-singapore.klingai.com/tasks?task_ids=kling_4", query["url"])
		assert.Equal(t, "GET", query["method"])
		query = call(t, "buildQueryRequest", map[string]any{"taskId": "task_x", "baseUrl": "https://gateway.example", "apiKey": "sk-relay", "state": map[string]any{"resolution": "720p"}})
		assert.Equal(t, "https://gateway.example/kling/tasks?task_ids=task_x", query["url"], "another gateway serves Kling under /kling")
		query = call(t, "buildQueryRequest", map[string]any{"taskId": "old_1", "action": "text_to_video", "baseUrl": "https://api-singapore.klingai.com", "apiKey": "ak|sk"})
		assert.Equal(t, "https://api-singapore.klingai.com/v1/videos/text2video/old_1", query["url"], "earlier models keep their query path")

		ctx := map[string]any{"taskId": "kling_4", "state": map[string]any{"resolution": "1080p"}}
		reply := func(entry map[string]any) map[string]any {
			return map[string]any{"code": 0, "message": "SUCCEED", "data": []any{map[string]any{"id": "other", "status": "processing"}, entry}}
		}
		parsed := call(t, "parseTaskResult", ctx, reply(map[string]any{"id": "kling_4", "status": "succeeded",
			"outputs": []any{map[string]any{"type": "video", "url": "https://cdn.example/out.mp4", "duration": "10"}},
			"billing": []any{map[string]any{"charge_type": "unit", "amount": "14.5", "package_type": "video"}},
		}), map[string]any{"status": 200})
		assert.Equal(t, "SUCCESS", parsed["status"])
		assert.Equal(t, "kling_4", parsed["taskId"], "the task's own entry is read, not the first")
		assert.Equal(t, "https://cdn.example/out.mp4", parsed["url"])
		assert.Equal(t, float64(15), parsed["completionTokens"])
		assert.Equal(t, float64(15), parsed["totalTokens"])

		for _, tc := range []struct{ status, want string }{{"submitted", "SUBMITTED"}, {"processing", "IN_PROGRESS"}, {"failed", "FAILURE"}, {"paused", "UNKNOWN"}} {
			parsed := call(t, "parseTaskResult", ctx, reply(map[string]any{"id": "kling_4", "status": tc.status, "message": "content check failed"}), map[string]any{"status": 200})
			assert.Equal(t, tc.want, parsed["status"], tc.status)
			assert.Nil(t, parsed["url"])
			assert.Nil(t, parsed["completionTokens"])
		}
		failed := call(t, "parseTaskResult", ctx, reply(map[string]any{"id": "kling_4", "status": "failed", "message": "content check failed"}), map[string]any{"status": 200})
		assert.Equal(t, "content check failed", failed["reason"])

		task := map[string]any{"status": "SUCCESS", "taskId": "kling_4", "state": map[string]any{"resolution": "1080p"},
			"data": reply(map[string]any{"id": "kling_4", "status": "succeeded", "outputs": []any{map[string]any{"type": "video", "url": "https://cdn.example/out.mp4"}}})}
		artifacts, err := plugin.Engine.Call(t.Context(), "listArtifacts", task)
		require.NoError(t, err)
		encoded, err := common.Marshal(artifacts)
		require.NoError(t, err)
		assert.JSONEq(t, `[{"key":"video","type":"video"}]`, string(encoded))
		content := call(t, "buildContentRequest", map[string]any{"artifactKey": "video", "taskId": "kling_4", "state": task["state"], "data": task["data"], "clientRequest": map[string]any{"method": "GET"}})
		assert.Equal(t, "https://cdn.example/out.mp4", content["url"])
	})

	// Settles on deducted units (package amount, cash list_price), else on the reported durations.
	t.Run("settlement", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			resolution string
			entry      map[string]any
			wantUnits  any
		}{
			{"resource package units", "1080p", map[string]any{"billing": []any{
				map[string]any{"charge_type": "unit", "amount": "6", "package_type": "video"},
				map[string]any{"charge_type": "unit", "amount": "4.5", "package_type": "video"},
			}}, 10.5},
			{"cash without an input video", "720p", map[string]any{"billing": []any{map[string]any{"charge_type": "cash", "amount": "10", "currency": "CNY"}}}, 10.0},
			{"no billing at 1080p", "1080p", map[string]any{}, 15.0},
			{"part cash, input shorter than the output", "1080p", map[string]any{"usage": map[string]any{"input_duration": "4"}, "billing": []any{
				map[string]any{"charge_type": "unit", "amount": "3", "package_type": "video"},
				map[string]any{"charge_type": "cash", "amount": "15", "currency": "CNY"},
			}}, 18.0},
			{"cash, input longer than the output", "1080p", map[string]any{"usage": map[string]any{"input_duration": 25}, "billing": []any{map[string]any{"charge_type": "cash", "amount": "31.5", "currency": "CNY"}}}, 31.5},
			{"cash at 720p with an input video", "720p", map[string]any{"usage": map[string]any{"input_duration": "10"}}, 12.0},
			{"balance at its list price, not the reported duration", "720p", map[string]any{
				"outputs": []any{map[string]any{"type": "video", "url": "https://cdn.example/out.mp4", "duration": "3.041"}},
				"billing": []any{map[string]any{"charge_type": "cash", "cash_type": "balance", "amount": "2.55", "list_price": "3", "currency": "CNY"}},
			}, 3.0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				entry := map[string]any{"id": "kling_4", "status": "succeeded", "outputs": []any{map[string]any{"type": "video", "url": "https://cdn.example/out.mp4", "duration": "10"}}}
				for key, value := range tc.entry {
					entry[key] = value
				}
				facts := call(t, "extractUsageOnComplete", map[string]any{"taskId": "kling_4", "state": map[string]any{"resolution": tc.resolution}}, map[string]any{"status": "SUCCESS"},
					map[string]any{"code": 0, "data": []any{entry}})
				assert.InDelta(t, tc.wantUnits, facts["units"], 1e-9)
			})
		}
		running := call(t, "extractUsageOnComplete", map[string]any{"taskId": "kling_4", "state": map[string]any{"resolution": "1080p"}}, map[string]any{"status": "IN_PROGRESS"},
			map[string]any{"code": 0, "data": []any{map[string]any{"id": "kling_4", "status": "processing"}}})
		assert.Nil(t, running, "an unfinished task keeps its reservation")
	})

	// TestKling4Video exercises host video requests, channel aliases and billing.
	t.Run("host video result", func(t *testing.T) {
		rendered, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, map[string]any{"task_id": "task_public", "status": "SUCCESS", "progress": "100%",
			"data": map[string]any{"code": 0, "message": "SUCCEED", "data": []any{map[string]any{"id": "kling_4", "status": "succeeded", "create_time": 1700000000000, "update_time": 1700000060000,
				"outputs": []any{map[string]any{"type": "video", "url": "https://cdn.example/out.mp4", "duration": "10"}}}}}})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"id": "task_public", "object": "video", "model": "", "status": "completed", "progress": float64(100),
			"created_at": float64(1700000000000), "completed_at": float64(1700000060000), "seconds": "10"}, alibabaObject(t, rendered))
	})

	// Native replies keep Kling 4.0's envelope with the gateway's task id.
	t.Run("native replies", func(t *testing.T) {
		created, err := plugin.Engine.CallPath(t.Context(), "native", []string{"kling4Created"}, map[string]any{}, map[string]any{"task_id": "task_public",
			"data": map[string]any{"code": 0, "message": "SUCCEED", "request_id": "req-1", "data": map[string]any{"id": "kling_4", "status": "submitted"}}})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"code": float64(0), "message": "SUCCEED", "request_id": "req-1", "data": map[string]any{"id": "task_public", "status": "submitted"}}, alibabaObject(t, created))

		tasks := func(task map[string]any) map[string]any {
			t.Helper()
			value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"kling4Tasks"}, map[string]any{}, task)
			require.NoError(t, err)
			return alibabaObject(t, value)
		}
		processing := map[string]any{"code": 0, "message": "SUCCEED", "request_id": "req-2", "data": []any{map[string]any{"id": "kling_4", "status": "processing", "create_time": 1700000000000}}}
		assert.Equal(t, map[string]any{"code": float64(0), "message": "SUCCEED", "request_id": "req-2", "data": []any{
			map[string]any{"id": "task_public", "status": "failed", "message": "task timed out", "create_time": float64(1700000000000), "update_time": float64(1700000060000)},
		}}, tasks(map[string]any{"task_id": "task_public", "status": "FAILURE", "fail_reason": "task timed out", "created_at": 1700000000, "updated_at": 1700000060, "data": processing}),
			"a task the gateway failed does not read as running")
		assert.Equal(t, map[string]any{"code": float64(0), "message": "", "request_id": "", "data": []any{
			map[string]any{"id": "task_public", "status": "submitted", "create_time": float64(1700000000000), "update_time": float64(1700000000000)},
		}}, tasks(map[string]any{"task_id": "task_public", "status": "SUBMITTED", "created_at": 1700000000}), "a task without a reply yet")

		status, err := plugin.Engine.CallPath(t.Context(), "native", []string{"taskStatus"}, map[string]any{},
			map[string]any{"task_id": "task_public", "status": "IN_PROGRESS", "created_at": 1700000000, "data": processing})
		require.NoError(t, err)
		data, _ := alibabaObject(t, status)["data"].(map[string]any)
		assert.Equal(t, "processing", data["task_status"], "the earlier query route does not pass a 4.0 reply off as its own")
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
