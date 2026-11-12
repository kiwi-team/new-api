package relay

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/speech"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func speechContext(input *dto.AudioRequest) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(common.JsonStringify(input)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func TestNativeSpeechRequestContract(t *testing.T) {
	zero, disabled := 0.0, false
	for _, tc := range []struct {
		model string
		chars int
		url   bool
	}{
		{speech.QwenPlus, 3, false}, {speech.QwenFlash, 2, false}, {speech.QwenNext, 2, false},
		{speech.ElevenV4, 2, false}, {speech.ElevenTurbo, 2, false}, {speech.SeedTTS, 2, true}, {speech.SeedICL, 2, false},
		{speech.MiniMaxHD, 3, false}, {speech.MiniMaxTurbo, 3, false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			request := &dto.AudioRequest{Model: tc.model, Input: "中a", Voice: "voice-id"}
			if tc.model == speech.QwenNext {
				request.Voice = ""
			}
			prepared, err := speech.Prepare(request)
			require.NoError(t, err)
			if tc.model == speech.ElevenV4 || tc.model == speech.ElevenTurbo {
				assert.True(t, speech.Supports(constant.ChannelTypeElevenLabs, tc.model))
				assert.False(t, speech.Supports(constant.ChannelTypeOpenAI, tc.model))
			}
			assert.Equal(t, tc.chars, prepared.Characters)
			assert.Equal(t, tc.url, prepared.URL)
		})
	}
	for _, tc := range []struct {
		name    string
		request dto.AudioRequest
	}{
		{"metadata model override", dto.AudioRequest{Model: speech.QwenPlus, Input: "hi", Voice: "v", Metadata: json.RawMessage(`{"model":"other"}`)}},
		{"metadata text override", dto.AudioRequest{Model: speech.MiniMaxHD, Input: "hi", Voice: "v", Metadata: json.RawMessage(`{"text":"changed"}`)}},
		{"URL with stream", dto.AudioRequest{Model: speech.QwenPlus, Input: "hi", Voice: "v", ResponseMode: "url", StreamFormat: "audio"}},
		{"Next voice", dto.AudioRequest{Model: speech.QwenNext, Input: "hi", Voice: "v"}},
		{"Next SSE", dto.AudioRequest{Model: speech.QwenNext, Input: "hi", StreamFormat: "sse"}},
		{"unsupported speed", dto.AudioRequest{Model: speech.ElevenTurbo, Input: "hi", Voice: "v", Speed: &zero}},
		{"ICL instructions", dto.AudioRequest{Model: speech.SeedICL, Input: "hi", Voice: "v", Instructions: "sad"}},
		{"MiniMax weighted limit", dto.AudioRequest{Model: speech.MiniMaxHD, Input: strings.Repeat("中", 5000), Voice: "v"}},
		{"malformed SSML", dto.AudioRequest{Model: speech.QwenPlus, Input: "<speak>bad", Voice: "v", Metadata: json.RawMessage(`{"enable_ssml":true}`)}},
	} {
		t.Run(tc.name, func(t *testing.T) { _, err := speech.Prepare(&tc.request); require.Error(t, err) })
	}
	metadata, err := common.Marshal(speech.Extensions{Volume: &zero, WordTimestamp: &disabled})
	require.NoError(t, err)
	prepared, err := speech.Prepare(&dto.AudioRequest{Model: speech.QwenPlus, Input: "hi", Voice: "v", Metadata: metadata})
	require.NoError(t, err)
	require.NotNil(t, prepared.Extensions.Volume)
	assert.Zero(t, *prepared.Extensions.Volume)
	require.NotNil(t, prepared.Extensions.WordTimestamp)
	assert.False(t, *prepared.Extensions.WordTimestamp)
}

func TestNativeSpeechHTTPContract(t *testing.T) {
	for _, tc := range []struct {
		name, model, format, body string
		chars                     *int
		wantErr, generated        bool
	}{
		{name: "Qwen snapshots and final URL do not replay audio", model: speech.QwenPlus, body: "data: {\"output\":{\"audio\":{\"data\":\"SUQz\"}},\"usage\":{\"characters\":3}}\n\ndata: {\"output\":{\"finish_reason\":\"stop\",\"audio\":{\"url\":\"https://unused.example/audio.mp3\"}},\"usage\":{}}\n\n", chars: new(3), generated: true},
		{name: "MiniMax decodes hex", model: speech.MiniMaxHD, body: "data: {\"data\":{\"audio\":\"494433\",\"status\":1},\"base_resp\":{\"status_code\":0}}\n\ndata: {\"data\":{\"status\":2},\"extra_info\":{\"usage_characters\":3},\"base_resp\":{\"status_code\":0}}\n\n", chars: new(3), generated: true},
		{name: "explicit zero stays present", model: speech.QwenPlus, body: "data: {\"output\":{\"audio\":{\"data\":\"SUQz\"},\"finish_reason\":\"stop\"},\"usage\":{\"characters\":0}}\n\n", chars: new(0), generated: true},
		{name: "partial failure SSE never sends done", model: speech.QwenPlus, format: "sse", body: "data: {\"output\":{\"audio\":{\"data\":\"SUQz\"}}}\n\ndata: {\"code\":\"InternalError\"}\n\n", wantErr: true, generated: true},
		{name: "truncated stream fails", model: speech.MiniMaxHD, body: "data: {\"data\":{\"audio\":\"494433\",\"status\":1},\"base_resp\":{\"status_code\":0}}\n\n", wantErr: true, generated: true},
		{name: "HTTP 200 business error", model: speech.MiniMaxHD, body: "data: {\"base_resp\":{\"status_code\":1001}}\n\n", wantErr: true},
		{name: "invalid hex", model: speech.MiniMaxHD, body: "data: {\"data\":{\"audio\":\"XYZ\",\"status\":2},\"base_resp\":{\"status_code\":0}}\n\n", wantErr: true},
		{name: "Qwen word timing SSE", model: speech.QwenPlus, format: "sse", body: "data: {\"output\":{\"type\":\"sentence-end\",\"sentence\":{\"words\":[{\"text\":\"hi\",\"begin_time\":0,\"end_time\":100}]},\"finish_reason\":\"stop\",\"audio\":{\"data\":\"SUQz\"}}}\n\n", generated: true},
		{name: "MiniMax subtitle SSE", model: speech.MiniMaxHD, format: "sse", body: "data: {\"data\":{\"audio\":\"494433\",\"status\":2,\"subtitle\":{\"text\":\"hi\",\"time_start\":0,\"time_end\":100}},\"base_resp\":{\"status_code\":0}}\n\n", generated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				assert.Equal(t, "Bearer upstream-key", req.Header.Get("Authorization"))
				var body map[string]any
				assert.NoError(t, common.DecodeJson(req.Body, &body))
				assert.Equal(t, tc.model, body["model"])
				if tc.model == speech.QwenPlus {
					assert.Equal(t, "/api/v1/services/audio/tts/SpeechSynthesizer", req.URL.Path)
					assert.Equal(t, "enable", req.Header.Get("X-DashScope-SSE"))
				} else {
					assert.Equal(t, "/v1/t2a_v2", req.URL.Path)
					assert.Equal(t, true, body["stream_options"].(map[string]any)["exclude_aggregated_audio"])
					assert.Equal(t, float64(128000), body["audio_setting"].(map[string]any)["bitrate"], "legacy kbps still converts to native bps")
					assert.Equal(t, "angry", body["voice_setting"].(map[string]any)["emotion"])
					assert.Equal(t, false, body["subtitle_enable"])
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			audioRequest := &dto.AudioRequest{Model: tc.model, Input: "中a", Voice: "v", StreamFormat: tc.format}
			if tc.model == speech.MiniMaxHD {
				audioRequest.Metadata = json.RawMessage(`{"bit_rate":128,"emotion":"angry","subtitle_enable":false}`)
			}
			request, err := speech.Prepare(audioRequest)
			require.NoError(t, err)
			ctx, recorder := speechContext(&request.AudioRequest)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "upstream-key"}, StartTime: time.Now()}
			result, err := speech.Run(ctx, info, request)
			assert.Equal(t, tc.wantErr, err != nil)
			assert.Equal(t, tc.generated, result.Generated)
			assert.Equal(t, tc.chars, result.Usage.Characters)
			assert.Zero(t, result.Usage.TotalTokens)
			assert.NotContains(t, result.Response, "upstream-key")
			if tc.generated && tc.chars == nil {
				assert.Equal(t, "missing_usage", info.UsageBillingFallback)
			}
			if tc.format == "sse" {
				assert.Contains(t, recorder.Body.String(), "speech.audio.delta")
				if tc.wantErr {
					assert.Contains(t, recorder.Body.String(), "event: error")
					assert.NotContains(t, recorder.Body.String(), "speech.audio.done")
				} else {
					assert.Contains(t, recorder.Body.String(), "speech.audio.metadata")
					assert.Contains(t, recorder.Body.String(), `"text":"hi"`)
					assert.Contains(t, recorder.Body.String(), "speech.audio.done")
				}
			} else if tc.generated {
				assert.Equal(t, "ID3", recorder.Body.String())
			}
		})
	}
}

func TestNativeSpeechMetadataContract(t *testing.T) {
	for _, modelName := range []string{speech.QwenPlus, speech.QwenFlash, speech.QwenNext, speech.MiniMaxHD, speech.MiniMaxTurbo} {
		t.Run(modelName, func(t *testing.T) {
			metadata := `{"input":{"volume":0,"enable_aigc_tag":false,"future_control":{"flags":[false,0]},"hot_fix":{"replace":[{"hi":"hello"}]}}}`
			if modelName == speech.MiniMaxHD || modelName == speech.MiniMaxTurbo {
				metadata = `{"voice_setting":{"emotion":"angry","pitch":0},"audio_setting":{"bitrate":128000,"channel":1},"voice_modify":{"pitch":0},"subtitle_enable":false,"future_control":{"flags":[false,0]}}`
			}
			if modelName == speech.QwenNext {
				metadata = `{"input":{"volume":0,"enable_aigc_tag":false,"future_control":{"flags":[false,0]},"references":[{"audio_data":"data:audio/wav;base64,UklGRg==","future_reference_field":false}]}}`
			}
			request := &dto.AudioRequest{Model: modelName, Input: "hi", Voice: "v", Metadata: json.RawMessage(metadata), StreamFormat: "audio"}
			if modelName == speech.QwenNext {
				request.Voice = ""
			}
			prepared, err := speech.Prepare(request)
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body map[string]any
				if !assert.NoError(t, common.DecodeJson(req.Body, &body)) {
					return
				}
				assert.Equal(t, modelName, body["model"])
				if strings.HasPrefix(modelName, "qwen-") {
					input := body["input"].(map[string]any)
					assert.Equal(t, float64(0), input["volume"])
					assert.Equal(t, false, input["enable_aigc_tag"])
					assert.Equal(t, []any{false, float64(0)}, input["future_control"].(map[string]any)["flags"])
					if modelName == speech.QwenNext {
						assert.Equal(t, "hi", input["text_prompt"])
					} else {
						assert.Equal(t, "v", input["voice"])
						assert.Equal(t, "hi", input["text"])
					}
					response := `{"output":{"finish_reason":"stop","audio":{"data":"SUQz"}}}`
					if modelName != speech.QwenNext {
						response = "data: " + response + "\n\n"
					}
					_, _ = io.WriteString(w, response)
					return
				}
				voice := body["voice_setting"].(map[string]any)
				assert.Equal(t, "v", voice["voice_id"], "nested patch must retain the mapped voice")
				assert.Equal(t, "angry", voice["emotion"])
				assert.Equal(t, float64(0), voice["pitch"])
				assert.Equal(t, false, body["subtitle_enable"])
				assert.Equal(t, float64(128000), body["audio_setting"].(map[string]any)["bitrate"])
				assert.Equal(t, []any{false, float64(0)}, body["future_control"].(map[string]any)["flags"])
				_, _ = io.WriteString(w, "data: {\"data\":{\"audio\":\"494433\",\"status\":2},\"base_resp\":{\"status_code\":0}}\n\n")
			}))
			defer server.Close()
			ctx, recorder := speechContext(&prepared.AudioRequest)
			result, err := speech.Run(ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "upstream-key"}}, prepared)
			require.NoError(t, err)
			assert.True(t, result.Generated)
			assert.Equal(t, "ID3", recorder.Body.String())
			if modelName == speech.QwenNext {
				assert.NotContains(t, ctx.GetString("speech_request"), "UklGRg==")
				assert.Contains(t, ctx.GetString("speech_request"), "[audio omitted]")
				assert.Contains(t, ctx.GetString("speech_request"), `"future_reference_field":false`)
			}
		})
	}
	for _, tc := range []struct{ name, model, metadata string }{
		{"native speed limit", speech.MiniMaxHD, `{"voice_setting":{"speed":9}}`},
		{"speed conflict", speech.MiniMaxHD, `{"voice_setting":{"speed":1.2}}`},
		{"format conflict", speech.MiniMaxHD, `{"audio_setting":{"format":"wav"}}`},
		{"voice conflict", speech.SeedTTS, `{"req_params":{"speaker":"different"}}`},
		{"native reference count", speech.QwenNext, `{"input":{"references":[{},{},{},{}]}}`},
		{"native text override", speech.QwenPlus, `{"input":{"text":"unbilled text"}}`},
		{"native quantity", speech.QwenPlus, `{"input":{"future":{"max_tokens":999999999999}}}`},
		{"nested array quantity", speech.QwenPlus, `{"input":{"future":[[{"n":999999999999}]]}}`},
		{"ICL string additions bypass", speech.SeedICL, `{"req_params":{"additions":"{\"context_texts\":[\"angry\"]}"}}`},
		{"ICL effect version", speech.SeedICL, `{"req_params":{"model":"other"}}`},
		{"credentials", speech.ElevenTurbo, `{"query":{"single_use_token":"other-token"}}`},
		{"model override", speech.ElevenTurbo, `{"query":{"model_id":"other"}}`},
		{"case-insensitive model override", speech.QwenPlus, `{"Model":"other"}`},
		{"encoding mismatch", speech.ElevenTurbo, `{"query":{"output_format":"pcm_24000"}}`},
		{"seed bound", speech.ElevenTurbo, `{"query":{"seed":4294967296}}`},
		{"transport override", speech.MiniMaxHD, `{"stream_options":{"exclude_aggregated_audio":false}}`},
		{"explicit null transport override", speech.MiniMaxHD, `{"stream_options":{"exclude_aggregated_audio":null}}`},
		{"TTD settings bound", speech.ElevenTurbo, `{"voice_settings":{"stability":2}}`},
		{"TTD locator version", speech.ElevenTurbo, `{"pronunciation_dictionary_locators":[{"pronunciation_dictionary_id":"dictionary"}]}`},
		{"expanded input bound", speech.QwenPlus, `{"input":{"hot_fix":{"replace":[{"hi":"` + strings.Repeat("a", 10001) + `"}]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &dto.AudioRequest{Model: tc.model, Input: "hi", Voice: "v", ResponseFormat: "mp3", Metadata: json.RawMessage(tc.metadata)}
			if tc.model == speech.QwenNext {
				request.Voice = ""
			}
			if tc.name == "speed conflict" {
				request.Speed = new(1.0)
			}
			_, err := speech.Prepare(request)
			require.Error(t, err)
		})
	}
	prepared, err := speech.Prepare(&dto.AudioRequest{Model: speech.QwenPlus, Input: "hi", Voice: "v", Metadata: json.RawMessage(`{"input":{"hot_fix":{"replace":[{"hi":"hello"}]}}}`)})
	require.NoError(t, err)
	assert.Equal(t, 5, prepared.Characters, "pre-consume includes native text expansion")
	assert.Equal(t, "hi", prepared.Input, "the provider applies replacement once")
}

// The server envelope includes connection/session IDs as specified by the
// provider. This fixture exercises the real binary decoder over a socket.
func speechServerFrame(t *testing.T, event int32, session string, audio bool, payload []byte) []byte {
	t.Helper()
	kind := byte(0x94)
	if audio {
		kind = 0xb4
	}
	var frame bytes.Buffer
	frame.Write([]byte{0x11, kind, 0x10, 0})
	require.NoError(t, binary.Write(&frame, binary.BigEndian, event))
	require.NoError(t, binary.Write(&frame, binary.BigEndian, uint32(len(session))))
	frame.WriteString(session)
	require.NoError(t, binary.Write(&frame, binary.BigEndian, uint32(len(payload))))
	frame.Write(payload)
	return frame.Bytes()
}

func TestNativeSpeechWebSocketContract(t *testing.T) {
	for _, name := range []string{speech.ElevenV4, speech.ElevenTurbo, speech.SeedTTS, speech.SeedICL} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				assert.Empty(t, req.Header.Get("Authorization"))
				conn, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
				if !assert.NoError(t, err) {
					return
				}
				defer conn.Close()
				if name == speech.ElevenV4 || name == speech.ElevenTurbo {
					assert.Equal(t, "/v1/text-to-dialogue/stream-input", req.URL.Path)
					assert.Equal(t, name, req.URL.Query().Get("model_id"))
					assert.Equal(t, "4294967295", req.URL.Query().Get("seed"), "integer query values must not use scientific notation")
					assert.Equal(t, "false", req.URL.Query().Get("enable_logging"))
					_, data, err := conn.ReadMessage()
					if !assert.NoError(t, err) {
						return
					}
					expectedVoice := "JBFqnCBsd6RMkjVDRZzb"
					if name == speech.ElevenV4 {
						expectedVoice = "hpp4J3VqNfWAUOO0d1Us"
					}
					assert.Contains(t, string(data), `"voices":["`+expectedVoice+`"]`)
					assert.Contains(t, string(data), `"stability":0`)
					assert.Contains(t, string(data), `"future_control":false`)
					assert.NotContains(t, string(data), `"query"`)
					_, data, err = conn.ReadMessage()
					if !assert.NoError(t, err) {
						return
					}
					assert.Contains(t, string(data), `"inputs"`)
					_, data, err = conn.ReadMessage()
					if !assert.NoError(t, err) {
						return
					}
					assert.Contains(t, string(data), `"close_socket":true`)
					data, err = common.Marshal(map[string]any{"audio": base64.StdEncoding.EncodeToString([]byte("ID3")), "is_final": true, "alignment": map[string]any{"chars": []string{"h"}, "char_start_times_ms": []int{0}}})
					assert.NoError(t, err)
					assert.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
					return
				}
				assert.Equal(t, "/api/v3/tts/bidirection", req.URL.Path)
				assert.Equal(t, "upstream-key", req.Header.Get("X-Api-Key"))
				assert.Equal(t, name, req.Header.Get("X-Api-Resource-Id"))
				assert.Empty(t, req.Header.Get("X-Api-App-Id"))
				assert.Equal(t, "*", req.Header.Get("X-Control-Require-Usage-Tokens-Return"))
				message, err := volcengine.ReceiveMessage(conn)
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, volcengine.EventType_StartConnection, message.EventType)
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 50, "connection", false, []byte("{}"))))
				message, err = volcengine.ReceiveMessage(conn)
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, volcengine.EventType_StartSession, message.EventType)
				assert.Contains(t, string(message.Payload), `"loudness_rate":0`)
				assert.Contains(t, string(message.Payload), `"enable_subtitle":true`)
				assert.Contains(t, string(message.Payload), `\"disable_markdown_filter\":false`)
				if name == speech.SeedICL {
					assert.Contains(t, string(message.Payload), `"model":"seed-tts-2.0-standard"`)
				} else {
					assert.NotContains(t, string(message.Payload), `"model"`)
					assert.Contains(t, string(message.Payload), `\"context_texts\":[\"friendly\"]`)
				}
				session := message.SessionID
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 150, session, false, []byte("{}"))))
				message, err = volcengine.ReceiveMessage(conn)
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, volcengine.EventType_TaskRequest, message.EventType)
				assert.Contains(t, string(message.Payload), `"text":"hello"`)
				message, err = volcengine.ReceiveMessage(conn)
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, volcengine.EventType_FinishSession, message.EventType)
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 351, session, false, []byte(`{"text":"hello","words":[{"word":"hello","start_time":0,"end_time":1}]}`))))
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 352, session, true, []byte("ID3"))))
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 152, session, false, []byte(`{"usage":{"text_words":5}}`))))
				message, err = volcengine.ReceiveMessage(conn)
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, volcengine.EventType_FinishConnection, message.EventType)
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 52, "connection", false, []byte("{}"))))
			}))
			defer server.Close()
			metadata := `{"req_params":{"audio_params":{"loudness_rate":0,"enable_subtitle":true},"additions":"{\"disable_markdown_filter\":false}"}}`
			instructions := ""
			if name == speech.SeedTTS {
				metadata = `{"req_params":{"audio_params":{"loudness_rate":0,"enable_subtitle":true},"additions":{"disable_markdown_filter":false}}}`
				instructions = "friendly"
			}
			if name == speech.ElevenV4 || name == speech.ElevenTurbo {
				metadata = `{"query":{"seed":4294967295,"sync_alignment":true,"enable_logging":false},"voice_settings":{"stability":0,"clone_strength":0,"future_control":false}}`
			}
			voice := "George"
			if name == speech.ElevenV4 {
				voice = "hpp4J3VqNfWAUOO0d1Us"
			}
			request, err := speech.Prepare(&dto.AudioRequest{Model: name, Input: "hello", Voice: voice, Instructions: instructions, StreamFormat: "sse", Metadata: json.RawMessage(metadata)})
			require.NoError(t, err)
			ctx, recorder := speechContext(&request.AudioRequest)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "upstream-key"}, StartTime: time.Now()}
			result, err := speech.Run(ctx, info, request)
			require.NoError(t, err)
			assert.Contains(t, recorder.Body.String(), "speech.audio.metadata")
			assert.Contains(t, recorder.Body.String(), `"audio":"SUQz"`)
			assert.Contains(t, recorder.Body.String(), "speech.audio.done")
			assert.Zero(t, result.Usage.TotalTokens)
			if name != speech.ElevenV4 && name != speech.ElevenTurbo {
				require.NotNil(t, result.Usage.Characters)
				assert.Equal(t, 5, *result.Usage.Characters)
			} else {
				assert.Equal(t, "missing_usage", info.UsageBillingFallback)
			}
			assert.NotContains(t, ctx.GetString("speech_request"), "upstream-key")
		})
	}
}

func TestElevenV4ModelsUseDistinctCharacterPrices(t *testing.T) {
	for _, tc := range []struct {
		model string
		quota int
	}{
		{speech.ElevenV4, 40000},
		{speech.ElevenTurbo, 20000},
	} {
		t.Run(tc.model, func(t *testing.T) {
			expression, ok := billing_setting.GetBuiltinBillingExpr(tc.model)
			require.True(t, ok)
			result, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{
				ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000,
			}, billingexpr.TokenParams{Chars: 1000, P: 10000, C: 10000}, billingexpr.RequestInput{})
			require.NoError(t, err)
			assert.Equal(t, tc.quota, result.ActualQuotaAfterGroup)
		})
	}
}

func TestNativeSpeechBillingDatabaseMatrix(t *testing.T) {
	require.NoError(t, i18n.Init())
	t.Setenv("QUOTA_DATA_SPOOL_DIR", t.TempDir())
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupRelayBillingDatabase(t, dialect)
			extra := []any{&model.QuotaData{}, &model.QuotaDataEventRecord{}, &model.QuotaDataAppliedEvent{}, &model.CliendUserQuota{}, &model.ProjectAllocation{}}
			require.NoError(t, db.AutoMigrate(extra...))
			oldExport, oldCount := common.DataExportEnabled, constant.CountToken
			common.DataExportEnabled, constant.CountToken = true, false
			t.Cleanup(func() {
				require.NoError(t, model.SaveQuotaDataCache())
				common.DataExportEnabled, constant.CountToken = oldExport, oldCount
				require.NoError(t, db.Migrator().DropTable(extra...))
			})
			for index, tc := range []struct {
				name, usage                string
				want                       int
				partial, alias, perRequest bool
			}{
				{name: "actual chars", usage: `{"characters":10000}`, want: 100000},
				{name: "explicit zero", usage: `{"characters":0}`},
				{name: "missing usage reservation", usage: `{}`, want: 30},
				{name: "partial audio still settles reservation", usage: `{}`, want: 30, partial: true},
				{name: "frozen admin alias price", usage: `{"characters":10000}`, want: 150000, alias: true},
				{name: "admin per-request price with zero characters", usage: `{"characters":0}`, want: 500, perRequest: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					user := model.User{Username: fmt.Sprintf("speech-%d", index), AffCode: fmt.Sprintf("speech-%d", index), Quota: 1000000}
					require.NoError(t, db.Create(&user).Error)
					token := model.Token{UserId: user.Id, Key: fmt.Sprintf("speech-key-%d", index), RemainQuota: 1000000}
					require.NoError(t, db.Create(&token).Error)
					provider := model.Channel{Type: constant.ChannelTypeAliDashScope, Name: "speech"}
					require.NoError(t, db.Create(&provider).Error)
					require.NoError(t, db.Create(&model.ProjectAllocation{ProjectId: index + 1, PlanId: 0, ClientUserId: "speech-uid", AllocatedQuota: 10}).Error)
					require.NoError(t, db.Create(&model.ProjectAllocation{ProjectId: index + 1, PlanId: 2, ClientUserId: "speech-uid", AllocatedQuota: 10, UsedQuota: 50}).Error)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						var body map[string]any
						assert.NoError(t, common.DecodeJson(req.Body, &body))
						assert.Equal(t, speech.QwenPlus, body["model"])
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data: {\"output\":{\"audio\":{\"data\":\"SUQz\"}},\"usage\":%s}\n\n", tc.usage)
						if !tc.partial {
							_, _ = io.WriteString(w, "data: {\"output\":{\"finish_reason\":\"stop\"}}\n\n")
						}
					}))
					defer server.Close()
					for pass := range 2 {
						name := speech.QwenPlus
						if tc.perRequest {
							config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting).BillingMode[name] = billing_setting.BillingModeRatio
							require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"qwen-audio-3.1-tts-plus":0.001}`))
						}
						if tc.alias {
							name = "speech-alias"
							settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
							settings.BillingMode[name] = "tiered_expr"
							settings.BillingExpr[name] = `tier("custom", chars * 30)`
						}
						ctx, recorder := speechContext(&dto.AudioRequest{Model: name, Input: "中a", Voice: "v"})
						common.SetContextKey(ctx, constant.ContextKeyOriginalModel, name)
						common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeAliDashScope)
						common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, server.URL)
						common.SetContextKey(ctx, constant.ContextKeyChannelKey, "upstream-key")
						common.SetContextKey(ctx, constant.ContextKeyChannelId, provider.Id)
						common.SetContextKey(ctx, constant.ContextKeyClientUserId, "speech-uid")
						common.SetContextKey(ctx, constant.ContextKeyClientScenairo, "tts")
						common.SetContextKey(ctx, constant.ContextKeyProjectId, index+1)
						common.SetContextKey(ctx, constant.ContextKeyProjectName, "speech-project")
						common.SetContextKey(ctx, constant.ContextKeyProjectPlanId, 0)
						ctx.Set(common.RequestIdKey, fmt.Sprintf("speech-%s-%d-%d", dialect, index, pass))
						// A legacy TTS statistic must never replace the amount already
						// settled by the frozen character expression.
						common.SetContextKey(ctx, constant.ContextKeyTTSCount, 999)
						if tc.alias {
							ctx.Set("model_mapping", `{"speech-alias":"qwen-audio-3.1-tts-plus"}`)
						}
						request, err := helper.GetAndValidateRequest(ctx, types.RelayFormatOpenAIAudio)
						require.NoError(t, err)
						info, err := relaycommon.GenRelayInfo(ctx, types.RelayFormatOpenAIAudio, request, nil)
						require.NoError(t, err)
						info.UserId, info.TokenId, info.TokenKey = user.Id, token.Id, token.Key
						info.UserGroup, info.UsingGroup = "default", "default"
						info.UserSetting.BillingPreference, info.ForcePreConsume = "wallet_only", true
						require.Nil(t, PrepareRequestBilling(ctx, info))
						if !tc.perRequest {
							require.NotNil(t, info.TieredBillingSnapshot)
							assert.Equal(t, 3, info.TieredBillingSnapshot.EstimatedCharacters)
						}
						if tc.alias {
							config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting).BillingExpr[name] = `tier("changed", chars * 999)`
						}
						require.Nil(t, AudioHelper(ctx, info))
						assert.Equal(t, "ID3", recorder.Body.String())
						var log model.Log
						require.NoError(t, model.LOG_DB.Where("request_id = ?", ctx.GetString(common.RequestIdKey)).Take(&log).Error)
						assert.Equal(t, tc.want, log.Quota)
						assert.Zero(t, log.PromptTokens)
						assert.Zero(t, log.CompletionTokens)
						if !tc.perRequest {
							assert.Contains(t, log.Other, `"billing_unit":"character"`)
						}
						assert.NotEmpty(t, log.Response)
						assert.NotContains(t, log.Response, "upstream-key")
						require.NoError(t, db.First(&user, user.Id).Error)
						require.NoError(t, db.First(&token, token.Id).Error)
						assert.Equal(t, 1000000-(pass+1)*tc.want, user.Quota)
						assert.Equal(t, user.Quota, token.RemainQuota)
						assert.Equal(t, (pass+1)*tc.want, user.UsedQuota)
						assert.Equal(t, pass+1, user.RequestCount)
						var allocation model.ProjectAllocation
						require.NoError(t, db.Where("project_id = ? AND plan_id = 0", index+1).Take(&allocation).Error)
						assert.Equal(t, (pass+1)*tc.want, allocation.UsedQuota)
						require.NoError(t, model.SaveQuotaDataCache())
						var buckets []model.QuotaData
						require.NoError(t, db.Where("user_id = ?", user.Id).Find(&buckets).Error)
						require.Len(t, buckets, 1)
						assert.EqualValues(t, pass+1, buckets[0].Count)
						assert.EqualValues(t, (pass+1)*tc.want, buckets[0].Quota)
						assert.Zero(t, buckets[0].TokenUsed)
					}
				})
			}
		})
	}
}

func TestSpeechCharacterExpressions(t *testing.T) {
	for _, chars := range []float64{-1, .5, 2147483648} {
		_, _, err := billingexpr.RunExpr(`tier("chars", chars * 20)`, billingexpr.TokenParams{Chars: chars})
		require.Error(t, err)
	}
	quota, trace, err := billingexpr.RunExpr(`tier("chars", chars * 20)`, billingexpr.TokenParams{Chars: 10000})
	require.NoError(t, err)
	assert.Equal(t, 200000.0, quota)
	assert.Equal(t, billingexpr.BillingUnitCharacter, trace.BillingUnit)
}

func TestNativeSpeechNonStreamingAndTokenUsage(t *testing.T) {
	// These cases return a URL without fetching it. Keep DNS and the global
	// fetch policy out of this native response-contract fixture.
	fetch := system_setting.GetFetchSetting()
	originalFetch := *fetch
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() { *fetch = originalFetch })
	for _, tc := range []struct {
		name, model, usage string
		p, c               int
		missing            bool
	}{
		{"Flash actual tokens", speech.QwenFlash, `{"input_tokens":1000,"output_tokens":2000}`, 1000, 2000, false},
		{"Next actual tokens", speech.QwenNext, `{"input_tokens":1000,"output_tokens":2000}`, 1000, 2000, false},
		{"Next duration is not tokens", speech.QwenNext, `{"duration":99}`, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body struct {
					Model string         `json:"model"`
					Input map[string]any `json:"input"`
				}
				assert.NoError(t, common.DecodeJson(req.Body, &body))
				assert.Equal(t, tc.model, body.Model)
				if tc.model == speech.QwenNext {
					assert.Equal(t, "hello", body.Input["text_prompt"])
					assert.NotContains(t, body.Input, "voice")
				}
				_, _ = fmt.Fprintf(w, `{"output":{"finish_reason":"stop","audio":{"url":"https://example.com/audio.wav"}},"usage":%s}`, tc.usage)
			}))
			defer server.Close()
			input := &dto.AudioRequest{Model: tc.model, Input: "hello", ResponseMode: "url"}
			if tc.model != speech.QwenNext {
				input.Voice = "v"
			}
			prepared, err := speech.Prepare(input)
			require.NoError(t, err)
			ctx, recorder := speechContext(input)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "upstream-key"}}
			result, err := speech.Run(ctx, info, prepared)
			require.NoError(t, err)
			assert.Equal(t, tc.p, result.Usage.PromptTokens)
			assert.Equal(t, tc.c, result.Usage.CompletionTokens)
			assert.Nil(t, result.Usage.Characters)
			assert.Contains(t, recorder.Body.String(), `"audio_url":"https://example.com/audio.wav"`)
			assert.Equal(t, tc.missing, info.UsageBillingFallback != "")
		})
	}
	for _, format := range []string{"wav", "flac"} {
		t.Run("MiniMax raw "+format, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body map[string]any
				assert.NoError(t, common.DecodeJson(req.Body, &body))
				assert.Equal(t, false, body["stream"])
				assert.Equal(t, "hex", body["output_format"])
				_, _ = io.WriteString(w, `{"data":{"audio":"494433","status":2},"extra_info":{"usage_characters":5},"base_resp":{"status_code":0}}`)
			}))
			defer server.Close()
			prepared, err := speech.Prepare(&dto.AudioRequest{Model: speech.MiniMaxTurbo, Input: "hello", Voice: "v", ResponseFormat: format})
			require.NoError(t, err)
			ctx, recorder := speechContext(&prepared.AudioRequest)
			_, err = speech.Run(ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "upstream-key"}}, prepared)
			require.NoError(t, err)
			assert.Equal(t, "ID3", recorder.Body.String())
		})
	}
}

func TestDoubaoSpeechRejectsLegacyAuthenticationAndResourceOverride(t *testing.T) {
	prepared, err := speech.Prepare(&dto.AudioRequest{Model: speech.SeedICL, Input: "hello", Voice: "cloned", StreamFormat: "audio"})
	require.NoError(t, err)
	for _, overrides := range []map[string]any{
		{"Authorization": "Bearer legacy-token"}, {"X-Api-App-Id": "legacy-app"}, {"X-Api-Key": "other-key"}, {"X-Api-Resource-Id": speech.SeedTTS},
	} {
		ctx, recorder := speechContext(&prepared.AudioRequest)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVolcEngine, ChannelBaseUrl: "http://127.0.0.1:1", ApiKey: "upstream-key", HeadersOverride: overrides}}
		result, err := speech.Run(ctx, info, prepared)
		require.Error(t, err)
		assert.False(t, result.Generated)
		assert.Empty(t, recorder.Body.String())
		assert.NotContains(t, err.Error(), "dial", "must reject before attempting a handshake")
	}
	frame := speechServerFrame(t, 152, "session", false, []byte(`{"usage":{"text_words":5}}`))
	_, err = volcengine.NewMessageFromBytes(frame[:len(frame)-1])
	require.Error(t, err, "truncated payload must never become valid zero usage")
}

func TestDoubaoICLRejectsSystemVoicesBeforeReservation(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, tc := range []struct{ language, detail string }{
		{"en", "cloned voice"}, {"zh-CN", "复刻音色"}, {"zh-TW", "復刻音色"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			request := &dto.AudioRequest{Model: speech.SeedICL, Input: "hello", Voice: "zh_male_tiancaitongsheng_mars_bigtts", StreamFormat: "audio"}
			ctx, recorder := speechContext(request)
			ctx.Request.Header.Set("Accept-Language", tc.language)
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeVolcEngine)
			info := &relaycommon.RelayInfo{OriginModelName: speech.SeedICL, RelayMode: relayconstant.RelayModeAudioSpeech, Request: request}
			apiErr := PrepareRequestBilling(ctx, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Contains(t, apiErr.Error(), tc.detail)
			assert.Nil(t, info.Billing)
			assert.False(t, recorder.Flushed)
		})
	}
	for _, voice := range []string{"S_account_voice", "custom-account-voice"} {
		_, err := speech.Prepare(&dto.AudioRequest{Model: speech.SeedICL, Input: "hello", Voice: voice})
		require.NoError(t, err, "unknown account voice formats are left to the provider")
	}
}

func TestDoubaoSpeechFailurePreservesSafeProviderDetails(t *testing.T) {
	for _, stage := range []string{"connection", "session", "synthesis", "binary error"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, req, nil)
				if !assert.NoError(t, err) {
					return
				}
				defer conn.Close()
				_, err = volcengine.ReceiveMessage(conn)
				if !assert.NoError(t, err) {
					return
				}
				session := "connection"
				event := int32(51)
				if stage != "connection" {
					assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 50, session, false, []byte("{}"))))
					message, readErr := volcengine.ReceiveMessage(conn)
					if !assert.NoError(t, readErr) {
						return
					}
					session, event = message.SessionID, 153
					if stage != "session" {
						assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, speechServerFrame(t, 150, session, false, []byte("{}"))))
						_, err = volcengine.ReceiveMessage(conn)
						if !assert.NoError(t, err) {
							return
						}
						_, err = volcengine.ReceiveMessage(conn)
						if !assert.NoError(t, err) {
							return
						}
					}
				}
				payload := []byte(`{"code":45000001,"message":"invalid speaker for seed-icl-2.0; api_key: upstream-key","debug":"unrelated raw payload must not be exposed"}`)
				frame := speechServerFrame(t, event, session, false, payload)
				if stage == "binary error" {
					var body bytes.Buffer
					body.Write([]byte{0x11, 0xf0, 0x10, 0})
					assert.NoError(t, binary.Write(&body, binary.BigEndian, uint32(45000001)))
					assert.NoError(t, binary.Write(&body, binary.BigEndian, uint32(len(payload))))
					body.Write(payload)
					frame = body.Bytes()
				}
				assert.NoError(t, conn.WriteMessage(websocket.BinaryMessage, frame))
			}))
			defer server.Close()
			request, err := speech.Prepare(&dto.AudioRequest{Model: speech.SeedICL, Input: "hello", Voice: "S_account_voice", StreamFormat: "audio"})
			require.NoError(t, err)
			ctx, recorder := speechContext(&request.AudioRequest)
			result, err := speech.Run(ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "upstream-key"}}, request)
			require.Error(t, err)
			assert.Equal(t, http.StatusBadGateway, types.NewError(err, types.ErrorCodeBadResponse).StatusCode)
			assert.Contains(t, err.Error(), "45000001")
			assert.Contains(t, err.Error(), "invalid speaker for seed-icl-2.0")
			assert.NotContains(t, err.Error(), "upstream-key")
			assert.NotContains(t, err.Error(), "unrelated raw payload")
			assert.Contains(t, result.Response, "invalid speaker for seed-icl-2.0")
			assert.False(t, result.Generated)
			assert.Empty(t, recorder.Body.String())
		})
	}
}
