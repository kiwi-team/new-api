package speech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/elevenlabs"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type socketEndpoint struct {
	url     string
	headers http.Header
}

func (s socketEndpoint) GetRequestURL(*relaycommon.RelayInfo) (string, error) { return s.url, nil }
func (s socketEndpoint) SetupRequestHeader(_ *gin.Context, headers *http.Header, _ *relaycommon.RelayInfo) error {
	*headers = s.headers.Clone()
	return nil
}

func (o *output) socket(path string, headers http.Header) (*websocket.Conn, func(), error) {
	if strings.TrimSpace(o.info.ApiKey) == "" {
		return nil, nil, errors.New("speech channel API key is empty")
	}
	endpoint := socketEndpoint{url: o.baseURL() + path, headers: headers}
	// Doubao's resource and single-key authentication boundary must survive
	// wildcard passthrough and explicit channel header overrides.
	if o.r.Model == SeedTTS || o.r.Model == SeedICL {
		overrides, err := channel.ResolveHeaderOverride(o.info, o.c)
		if err != nil {
			return nil, nil, err
		}
		for key, value := range overrides {
			switch strings.ToLower(key) {
			case "authorization", "x-api-app-id", "x-api-access-key":
				return nil, nil, errors.New("legacy Doubao authentication headers are not supported")
			case "x-api-key":
				if value != o.info.ApiKey {
					return nil, nil, errors.New("Doubao X-Api-Key must use the channel key")
				}
			case "x-api-resource-id":
				if value != o.r.Model {
					return nil, nil, errors.New("Doubao resource must match the mapped model")
				}
			case "x-control-require-usage-tokens-return":
				if value != "*" {
					return nil, nil, errors.New("Doubao speech requires usage reporting")
				}
			}
		}
	}
	conn, err := channel.DoWssRequest(endpoint, o.c, o.info, nil)
	if err != nil {
		return nil, nil, err
	}
	conn.SetReadLimit(16 << 20)
	deadline, _ := o.c.Request.Context().Deadline()
	writeDeadline := time.Now().Add(30 * time.Second)
	if deadline.Before(writeDeadline) {
		writeDeadline = deadline
	}
	_ = conn.SetWriteDeadline(writeDeadline)
	_ = conn.SetReadDeadline(deadline)
	stop := context.AfterFunc(o.c.Request.Context(), func() { _ = conn.Close() })
	cleanup := func() { stop(); _ = conn.Close() }
	return conn, cleanup, nil
}

func (o *output) eleven() error {
	voice := o.r.Voice
	if alias, ok := elevenlabs.VoideNameIDMap[voice]; ok {
		voice = alias
	}
	query := url.Values{"model_id": {o.r.Model}, "output_format": {"mp3_44100_128"}}
	if native, ok := o.r.Native["query"].(map[string]any); ok {
		for key, value := range native {
			text := fmt.Sprint(value)
			if number, ok := value.(float64); ok {
				text = strconv.FormatFloat(number, 'f', -1, 64)
			}
			query.Set(key, text)
		}
	}
	conn, cleanup, err := o.socket("/v1/text-to-dialogue/stream-input?"+query.Encode(), http.Header{"Xi-Api-Key": {o.info.ApiKey}})
	if err != nil {
		return err
	}
	defer cleanup()
	init := map[string]any{"voices": []string{voice}, "xi_api_key": o.info.ApiKey}
	for key, value := range o.r.Native {
		if key != "query" {
			init[key] = value
		}
	}
	// Authentication is sent only to the upstream and excluded from logs.
	data, err := common.Marshal(init)
	if err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return err
	}
	input := map[string]any{"inputs": []any{map[string]any{"text": o.r.Input, "voice_id": voice, "new_turn": true}}}
	loggedInit := map[string]any{}
	for key, value := range init {
		if key != "xi_api_key" {
			loggedInit[key] = value
		}
	}
	o.c.Set("speech_request", common.JsonStringify(map[string]any{"model_id": o.r.Model, "query": query, "initialization": loggedInit, "input": input}))
	data, err = common.Marshal(input)
	if err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return err
	}
	// The close message flushes short text and requests the final audio event.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"close_socket":true}`)); err != nil {
		return err
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame struct {
			Audio     string          `json:"audio"`
			Final     bool            `json:"is_final"`
			Error     any             `json:"error"`
			Usage     map[string]any  `json:"usage"`
			Alignment json.RawMessage `json:"alignment"`
		}
		if err := common.Unmarshal(data, &frame); err != nil {
			return err
		}
		if frame.Error != nil {
			return errors.New("ElevenLabs speech returned a business error")
		}
		if len(frame.Alignment) > 0 && string(frame.Alignment) != "null" {
			if err := o.annotation("alignment", frame.Alignment); err != nil {
				return err
			}
		}
		if err := o.usage(frame.Usage, "characters"); err != nil {
			o.info.UsageBillingFallback = "invalid_usage"
			return err
		}
		if frame.Audio != "" {
			audio, err := base64.StdEncoding.Strict().DecodeString(frame.Audio)
			if err != nil {
				return errors.New("invalid ElevenLabs speech base64")
			}
			if err := o.audio(audio); err != nil {
				return err
			}
		}
		if frame.Final {
			o.complete = true
			return nil
		}
	}
}

func (o *output) doubao() error {
	headers := http.Header{}
	headers.Set("X-Api-Key", o.info.ApiKey)
	headers.Set("X-Api-Resource-Id", o.r.Model)
	headers.Set("X-Api-Connect-Id", uuid.NewString())
	headers.Set("X-Control-Require-Usage-Tokens-Return", "*")
	conn, cleanup, err := o.socket("/api/v3/tts/bidirection", headers)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := volcengine.StartConnection(conn); err != nil {
		return err
	}
	if _, err := o.waitDoubaoEvent(conn, volcengine.EventType_ConnectionStarted, ""); err != nil {
		return err
	}
	session := uuid.NewString()
	format, sampleRate := o.r.ResponseFormat, 24000
	if format == "opus" {
		format, sampleRate = "ogg_opus", 48000
	}
	if o.r.Extensions.SampleRate != nil {
		sampleRate = *o.r.Extensions.SampleRate
	}
	audio := map[string]any{"format": format, "sample_rate": sampleRate}
	if o.r.Speed != nil {
		audio["speech_rate"] = int(math.Round((*o.r.Speed - 1) * 100))
	}
	params := map[string]any{"speaker": o.r.Voice, "audio_params": audio}
	if o.r.Model == SeedICL {
		params["model"] = "seed-tts-2.0-standard"
	}
	if o.r.Instructions != "" {
		params["additions"] = map[string]any{"context_texts": []any{o.r.Instructions}}
	}
	start := map[string]any{"event": 100, "req_params": params}
	mergeNative(start, o.r.Native)
	if additions, ok := params["additions"].(map[string]any); ok {
		params["additions"] = common.JsonStringify(additions)
	}
	task := map[string]any{"event": 200, "req_params": map[string]any{"text": o.r.Input}}
	o.c.Set("speech_request", common.JsonStringify(map[string]any{"resource_id": o.r.Model, "start": start, "task": task}))
	data, err := common.Marshal(start)
	if err != nil {
		return err
	}
	if err := volcengine.StartSession(conn, data, session); err != nil {
		return err
	}
	started, err := o.waitDoubaoEvent(conn, volcengine.EventType_SessionStarted, session)
	if err != nil {
		return err
	}
	if started.SessionID != session {
		return errors.New("Doubao returned a mismatched speech session")
	}
	data, err = common.Marshal(task)
	if err != nil {
		return err
	}
	if err := volcengine.TaskRequest(conn, data, session); err != nil {
		return err
	}
	if err := volcengine.FinishSession(conn, session); err != nil {
		return err
	}
	for {
		message, err := volcengine.ReceiveMessage(conn)
		if err != nil {
			return err
		}
		if message.MsgType == volcengine.MsgTypeError || message.EventType == volcengine.EventType_SessionFailed || message.EventType == volcengine.EventType_ConnectionFailed {
			return o.doubaoFailure(message)
		}
		if message.SessionID != "" && message.SessionID != session {
			return errors.New("Doubao returned a mismatched speech session")
		}
		if message.MsgType == volcengine.MsgTypeFullServerResponse && message.EventType >= volcengine.EventType_TTSSentenceStart && message.EventType < volcengine.EventType_TTSEnded && message.EventType != volcengine.EventType_TTSResponse {
			var sentence json.RawMessage
			if err := common.Unmarshal(message.Payload, &sentence); err != nil {
				return errors.New("invalid Doubao sentence metadata")
			}
			if err := o.annotation(fmt.Sprintf("sentence.%d", message.EventType), sentence); err != nil {
				return err
			}
		}
		if message.MsgType == volcengine.MsgTypeAudioOnlyServer {
			if message.EventType != volcengine.EventType_TTSResponse {
				continue
			}
			if err := o.audio(message.Payload); err != nil {
				return err
			}
		}
		if message.EventType == volcengine.EventType_SessionFinished {
			var payload struct {
				Usage map[string]any `json:"usage"`
			}
			if err := common.Unmarshal(message.Payload, &payload); err != nil {
				return fmt.Errorf("invalid Doubao usage: %w", err)
			}
			if err := o.usage(payload.Usage, "text_words"); err != nil {
				o.info.UsageBillingFallback = "invalid_usage"
				return err
			}
			o.complete = true
			if err := volcengine.FinishConnection(conn); err != nil {
				return err
			}
			_, err := o.waitDoubaoEvent(conn, volcengine.EventType_ConnectionFinished, "")
			return err
		}
	}
}

// Startup and shutdown events share the same filtered failure handling as
// synthesis; never return Message.String(), which includes the raw payload.
func (o *output) waitDoubaoEvent(conn *websocket.Conn, event volcengine.EventType, session string) (*volcengine.Message, error) {
	message, err := volcengine.ReceiveMessage(conn)
	if err != nil {
		return nil, err
	}
	if message.MsgType == volcengine.MsgTypeError || message.EventType == volcengine.EventType_ConnectionFailed || message.EventType == volcengine.EventType_SessionFailed {
		return nil, o.doubaoFailure(message)
	}
	if message.MsgType != volcengine.MsgTypeFullServerResponse || message.EventType != event {
		return nil, fmt.Errorf("unexpected Doubao speech event: type=%d event=%d, expected event=%d", message.MsgType, message.EventType, event)
	}
	if session != "" && message.SessionID != session {
		return nil, errors.New("Doubao returned a mismatched speech session")
	}
	return message, nil
}

func (o *output) doubaoFailure(message *volcengine.Message) error {
	code := fmt.Sprint(message.ErrorCode)
	detail := "upstream returned a failure event"
	var payload struct {
		Code       json.RawMessage `json:"code"`
		StatusCode json.RawMessage `json:"status_code"`
		Message    string          `json:"message"`
		Error      json.RawMessage `json:"error"`
	}
	if common.Unmarshal(message.Payload, &payload) == nil {
		codeValue := payload.Code
		if len(codeValue) == 0 {
			codeValue = payload.StatusCode
		}
		if payload.Message != "" {
			detail = payload.Message
		} else if len(payload.Error) > 0 {
			var errorText string
			if common.Unmarshal(payload.Error, &errorText) == nil && errorText != "" {
				detail = errorText
			} else {
				var nested struct {
					Code    json.RawMessage `json:"code"`
					Message string          `json:"message"`
				}
				if common.Unmarshal(payload.Error, &nested) == nil {
					if nested.Message != "" {
						detail = nested.Message
					}
					if len(codeValue) == 0 {
						codeValue = nested.Code
					}
				}
			}
		}
		if len(codeValue) > 0 && string(codeValue) != "null" {
			var number json.Number
			var text string
			if common.Unmarshal(codeValue, &text) == nil {
				code = text
			} else if common.Unmarshal(codeValue, &number) == nil {
				code = number.String()
			}
		}
	}
	text := fmt.Sprintf("Doubao speech session failed (event=%d, code=%s): %s", message.EventType, code, detail)
	for _, credential := range []string{o.info.ApiKey, o.info.TokenKey} {
		if credential != "" {
			text = strings.ReplaceAll(text, credential, "[redacted]")
		}
	}
	text = common.MaskSensitiveInfo(strings.Join(strings.Fields(text), " "))
	if characters := []rune(text); len(characters) > 1024 {
		text = string(characters[:1024]) + "..."
	}
	return types.NewErrorWithStatusCode(errors.New(text), types.ErrorCodeBadResponse, http.StatusBadGateway)
}
