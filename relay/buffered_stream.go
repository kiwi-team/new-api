package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// Bound the whole upstream SSE body, including extensions and framing, rather
// than just the visible text. Never retain a second copy of the raw stream.
const maxBufferedStreamBytes = 64 << 20

var (
	errBufferedStreamUnsupported = errors.New("upstream protocol does not support buffered streaming")
	errBufferedStreamTooLarge    = errors.New("buffered response exceeds 64 MiB")
)

type bufferedJSON = map[string]json.RawMessage

// Client errors use the request language; retain technical causes in stream
// diagnostics so operators can distinguish truncation, parsing and conversion.
func newBufferedStreamError(c *gin.Context, info *relaycommon.RelayInfo, cause error, options ...types.NewAPIErrorOptions) *types.NewAPIError {
	key := i18n.MsgRelayBufferedStreamFailed
	if errors.Is(cause, errBufferedStreamUnsupported) {
		key = i18n.MsgRelayBufferedStreamUnsupported
	}
	if errors.Is(cause, errBufferedStreamTooLarge) {
		key = i18n.MsgRelayBufferedStreamTooLarge
	}
	if info.StreamStatus == nil {
		info.StreamStatus = relaycommon.NewStreamStatus()
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, cause)
		common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	}
	if !info.StreamStatus.HasErrors() {
		info.StreamStatus.RecordError(cause.Error())
	}
	if c.Request.Context().Err() != nil {
		info.StreamStatus.MarkCancelled()
	} else {
		info.StreamStatus.MarkFailed("", "upstream_stream_error", http.StatusBadGateway)
	}
	return types.NewErrorWithStatusCode(errors.New(i18n.T(c, key)), types.ErrorCodeBadResponseBody, http.StatusBadGateway, options...)
}

func forceBufferedStreamRequest(info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
	if !info.BufferStreamResponse {
		return body, nil
	}
	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatOpenAIResponses:
	default:
		return nil, errBufferedStreamUnsupported
	}
	if info.ApiType == constant.APITypeAws && info.ChannelOtherSettings.AwsApiType != dto.AwsApiTypeMantle &&
		info.ChannelOtherSettings.AwsKeyType != dto.AwsKeyTypeApiKey {
		return nil, fmt.Errorf("%w: AWS SDK transport", errBufferedStreamUnsupported)
	}
	patched, err := sjson.SetBytes(body, "stream", true)
	if err != nil {
		return nil, err
	}
	if info.GetFinalRequestRelayFormat() == types.RelayFormatOpenAI && !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled {
		if info.SupportStreamOptions {
			return sjson.SetBytes(patched, "stream_options.include_usage", true)
		}
		return sjson.DeleteBytes(patched, "stream_options")
	}
	return patched, nil
}

// Preserve the client body before protocol conversion, including passthrough
// extensions that may be absent from the parsed request DTO.
func bufferedClientRequestLog(c *gin.Context) (string, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", err
	}
	data, err := storage.Bytes()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Stage the final non-stream response too: converters may fail after parsing.
// No response headers, heartbeat or partial JSON may escape before success.
type bufferedResponseWriter struct {
	gin.ResponseWriter
	header http.Header
	body   bytes.Buffer
	status int
	err    error
}

func (w *bufferedResponseWriter) Header() http.Header               { return w.header }
func (w *bufferedResponseWriter) WriteHeader(code int)              { w.status = code }
func (w *bufferedResponseWriter) WriteHeaderNow()                   {}
func (w *bufferedResponseWriter) Flush()                            {}
func (w *bufferedResponseWriter) Status() int                       { return w.status }
func (w *bufferedResponseWriter) Size() int                         { return w.body.Len() }
func (w *bufferedResponseWriter) Written() bool                     { return w.body.Len() > 0 }
func (w *bufferedResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	if len(p) > maxBufferedStreamBytes-w.body.Len() {
		w.err = errBufferedStreamTooLarge
		return 0, w.err
	}
	return w.body.Write(p)
}

func doBufferedStreamResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, adaptor channel.Adaptor) (any, string, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, "", newBufferedStreamError(c, info, errors.New("buffered streaming requires an HTTP response"))
	}
	defer resp.Body.Close()
	writer := &bufferedResponseWriter{ResponseWriter: c.Writer, header: c.Writer.Header().Clone(), status: http.StatusOK}
	originalWriter := c.Writer
	c.Writer = writer
	upstreamStream := info.IsStream
	defer func() {
		c.Writer = originalWriter
		info.IsStream = upstreamStream
	}()

	var usage any
	var apiErr *types.NewAPIError
	data, err := aggregateBufferedStream(c, resp, info, maxBufferedStreamBytes)
	if err != nil {
		var existing *types.NewAPIError
		if errors.As(err, &existing) {
			return nil, "", existing
		}
		options := []types.NewAPIErrorOptions{}
		if c.Request.Context().Err() != nil {
			options = append(options, types.ErrOptionWithSkipRetry())
		}
		return nil, "", newBufferedStreamError(c, info, err, options...)
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.Header = resp.Header.Clone()
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Transfer-Encoding")
	resp.ContentLength = int64(len(data))
	// Only response dispatch sees the reconstructed non-stream response.
	info.IsStream = false
	if info.GetFinalRequestRelayFormat() == types.RelayFormatOpenAIResponses {
		usage, apiErr = openai.OaiResponsesToChatHandler(c, info, resp)
	} else {
		usage, apiErr = adaptor.DoResponse(c, resp, info)
	}
	if apiErr != nil {
		return nil, "", apiErr
	}
	if parsed, ok := usage.(*dto.Usage); !ok || parsed == nil {
		return nil, "", newBufferedStreamError(c, info, errors.New("invalid buffered billing usage"))
	}
	if writer.err != nil {
		return nil, "", newBufferedStreamError(c, info, writer.err)
	}
	if err := c.Request.Context().Err(); err != nil {
		return nil, "", newBufferedStreamError(c, info, err, types.ErrOptionWithSkipRetry())
	}
	if writer.body.Len() == 0 {
		return nil, "", newBufferedStreamError(c, info, errors.New("invalid buffered JSON response"))
	}
	var finalJSON bufferedJSON
	if err := common.Unmarshal(writer.body.Bytes(), &finalJSON); err != nil || finalJSON == nil {
		return nil, "", newBufferedStreamError(c, info, errors.New("invalid buffered JSON response"))
	}
	mapper, err := info.ResponseModelOutputMapper()
	if err != nil {
		return nil, "", newBufferedStreamError(c, info, err)
	}
	if mapper != nil {
		mapped := mapper.RewritePayloadForRequest(writer.body.Bytes(), info.UpstreamModelName, info.OriginModelName, info.ChannelSetting.ResponseModelUseRequestModel)
		writer.body.Reset()
		writer.body.Write(mapped)
	}
	if writer.body.Len() > maxBufferedStreamBytes {
		return nil, "", newBufferedStreamError(c, info, errBufferedStreamTooLarge)
	}
	writer.header.Set("Content-Length", fmt.Sprint(writer.body.Len()))
	info.SetFirstResponseTime()
	maps.Copy(originalWriter.Header(), writer.header)
	originalWriter.Header().Set("Content-Type", "application/json")
	originalWriter.Header().Del("Transfer-Encoding")
	originalWriter.WriteHeader(writer.status)
	if _, err := originalWriter.Write(writer.body.Bytes()); err != nil {
		return nil, "", newBufferedStreamError(c, info, err, types.ErrOptionWithSkipRetry())
	}
	if os.Getenv("SAVE_REQUEST_RESPONSE") == "true" {
		info.BufferedResponseJSON = writer.body.String()
	}
	return usage, info.BufferedResponseJSON, nil
}

type bufferedStreamAccumulator interface {
	add(bufferedJSON) error
	finish() (bufferedJSON, error)
}

func aggregateBufferedStream(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, limit int) ([]byte, error) {
	defer resp.Body.Close()
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RequireTerminal()
	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	var accumulator bufferedStreamAccumulator
	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatOpenAI:
		accumulator = &bufferedChat{root: bufferedJSON{"object": json.RawMessage(`"chat.completion"`)}, choices: map[int]*bufferedChatChoice{}}
	case types.RelayFormatOpenAIResponses:
		accumulator = &bufferedResponses{accumulator: relayconvert.NewResponsesBufferedAccumulator()}
	case types.RelayFormatClaude:
		accumulator = &bufferedMessages{blocks: map[int]*bufferedMessageBlock{}}
	default:
		return nil, errBufferedStreamUnsupported
	}
	upstreamBody := resp.Body
	stopCancel := context.AfterFunc(c.Request.Context(), func() { _ = upstreamBody.Close() })
	defer stopCancel()
	var timedOut atomic.Bool
	idle := time.Duration(constant.StreamingTimeout) * time.Second
	if idle <= 0 {
		idle = 300 * time.Second
	}
	timer := time.AfterFunc(idle, func() { timedOut.Store(true); _ = upstreamBody.Close() })
	defer timer.Stop()
	limited := &io.LimitedReader{R: upstreamBody, N: int64(limit) + 1}
	scanner := helper.NewStreamScanner(limited, limit+1)
	var event strings.Builder
	total := 0
	done := false
	protocolComplete := false
	consume := func() error {
		data := strings.TrimSpace(event.String())
		event.Reset()
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			done = true
			return nil
		}
		var payload bufferedJSON
		if err := common.UnmarshalJsonStr(data, &payload); err != nil || payload == nil {
			return errors.New("invalid upstream SSE JSON")
		}
		if raw := payload["error"]; len(raw) > 0 && string(raw) != "null" {
			if info.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
				var response dto.ClaudeResponse
				if common.UnmarshalJsonStr(data, &response) == nil {
					if upstreamErr := response.GetClaudeError(); upstreamErr != nil {
						return types.WithClaudeError(*upstreamErr, http.StatusBadGateway)
					}
				}
			}
			var errorValue any
			_ = common.Unmarshal(raw, &errorValue)
			if upstreamErr := dto.GetOpenAIError(errorValue); upstreamErr != nil && upstreamErr.Type != "" {
				return types.WithOpenAIError(*upstreamErr, http.StatusBadGateway)
			}
			return errors.New("upstream SSE error event")
		}
		if info.UpstreamFirstChunkTime.IsZero() {
			info.UpstreamFirstChunkTime = time.Now()
		}
		info.ReceivedResponseCount++
		if err := accumulator.add(payload); err != nil {
			return err
		}
		switch a := accumulator.(type) {
		case *bufferedMessages:
			protocolComplete = a.stopped
		case *bufferedResponses:
			protocolComplete = a.final != nil
		}
		return nil
	}
	var parseErr error
	for scanner.Scan() {
		timer.Reset(idle)
		line := scanner.Text()
		total += len(line) + 1
		if total > limit {
			parseErr = errBufferedStreamTooLarge
			break
		}
		if line == "" {
			parseErr = consume()
			if parseErr != nil || done || protocolComplete {
				break
			}
			continue
		}
		if data, ok := strings.CutPrefix(line, "data:"); ok {
			data, _ = strings.CutPrefix(data, " ")
			if event.Len() > 0 {
				event.WriteByte('\n')
			}
			event.WriteString(data)
		}
	}
	if parseErr == nil && !done && !protocolComplete {
		parseErr = consume()
	}
	if err := c.Request.Context().Err(); err != nil {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		return nil, err
	}
	if timedOut.Load() {
		parseErr = errors.New("buffered upstream stream timed out")
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, parseErr)
	}
	if limited.N == 0 && parseErr == nil {
		parseErr = errBufferedStreamTooLarge
	}
	if parseErr == nil {
		parseErr = scanner.Err()
	}
	var result bufferedJSON
	if parseErr == nil {
		result, parseErr = accumulator.finish()
	}
	if parseErr != nil {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, parseErr)
		info.StreamStatus.RecordError(parseErr.Error())
		info.StreamStatus.MarkFailed("", "upstream_stream_error", http.StatusBadGateway)
		return nil, parseErr
	}
	info.StreamStatus.MarkCompleted()
	reason := relaycommon.StreamEndReasonEOF
	if protocolComplete {
		reason = relaycommon.StreamEndReasonHandlerStop
	}
	if done {
		reason = relaycommon.StreamEndReasonDone
	}
	info.StreamStatus.SetEndReason(reason, nil)
	if _, ok := accumulator.(*bufferedChat); ok {
		for key, fallback := range map[string]any{"id": helper.GetResponseID(c), "created": common.GetTimestamp(), "model": info.UpstreamModelName} {
			if value := string(result[key]); value == "" || value == `""` || value == "null" || key == "created" && value == "0" {
				result[key] = rawBufferedValue(fallback)
			}
		}
	}
	if messages, ok := accumulator.(*bufferedMessages); ok {
		var usage bufferedJSON
		if raw := result["usage"]; len(raw) > 0 {
			_ = common.Unmarshal(raw, &usage)
		}
		if usage == nil {
			usage = bufferedJSON{}
		}
		_, hasInput := usage["input_tokens"]
		_, hasOutput := usage["output_tokens"]
		hasOutput = hasOutput && messages.outputUsageReported
		if !hasInput || !hasOutput {
			var text strings.Builder
			for _, index := range slices.Sorted(maps.Keys(messages.blocks)) {
				block := messages.blocks[index]
				for _, key := range []string{"text", "thinking"} {
					var value string
					if common.Unmarshal(block.fields[key], &value) == nil {
						text.WriteString(value)
					}
				}
				text.WriteString(block.input.String())
			}
			estimate := service.ResponseText2Usage(c, text.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
			if !hasInput {
				usage["input_tokens"] = rawBufferedValue(estimate.PromptTokens)
			}
			if !hasOutput {
				usage["output_tokens"] = rawBufferedValue(estimate.CompletionTokens)
			}
			result["usage"] = rawBufferedValue(usage)
		}
	}
	return common.Marshal(result)
}

func rawBufferedValue(value any) json.RawMessage {
	data, _ := common.Marshal(value)
	return data
}

// Merge only reported members. Missing fields in a terminal usage chunk must
// never erase input/cache counts from an earlier chunk; counts are cumulative.
func mergeBufferedObject(dst bufferedJSON, src bufferedJSON) error {
	for key, value := range src {
		if len(value) > 0 && value[0] == '{' && len(dst[key]) > 0 && dst[key][0] == '{' {
			var left, right bufferedJSON
			if common.Unmarshal(dst[key], &left) != nil || common.Unmarshal(value, &right) != nil {
				return errors.New("invalid buffered object")
			}
			if err := mergeBufferedObject(left, right); err != nil {
				return err
			}
			dst[key] = rawBufferedValue(left)
			continue
		}
		dst[key] = value
	}
	return nil
}

type bufferedChat struct {
	root    bufferedJSON
	choices map[int]*bufferedChatChoice
}

type bufferedChatChoice struct {
	message      bufferedJSON
	text         map[string]*strings.Builder
	tools        map[int]*bufferedChatTool
	finishReason json.RawMessage
}

type bufferedChatTool struct {
	fields    bufferedJSON
	function  bufferedJSON
	arguments strings.Builder
}

func (a *bufferedChat) add(payload bufferedJSON) error {
	// Preserve provider extensions used by existing non-stream usage hooks.
	for key, value := range payload {
		switch key {
		case "id", "created", "model", "system_fingerprint", "service_tier", "object", "choices", "usage", "error":
			continue
		default:
			if err := mergeBufferedObject(a.root, bufferedJSON{key: value}); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"id", "created", "model", "system_fingerprint", "service_tier"} {
		if len(a.root[key]) == 0 || string(a.root[key]) == `""` || string(a.root[key]) == "null" {
			if value := payload[key]; len(value) > 0 {
				a.root[key] = value
			}
		}
	}
	if usage := payload["usage"]; len(usage) > 0 && string(usage) != "null" {
		if err := mergeBufferedObject(a.root, bufferedJSON{"usage": usage}); err != nil {
			return err
		}
	}
	var choices []struct {
		Index        int             `json:"index"`
		Delta        bufferedJSON    `json:"delta"`
		FinishReason json.RawMessage `json:"finish_reason"`
		Logprobs     json.RawMessage `json:"logprobs"`
	}
	if err := common.Unmarshal(payload["choices"], &choices); err != nil {
		return errors.New("invalid upstream chat choices")
	}
	for _, chunk := range choices {
		if chunk.Index < 0 {
			return errors.New("invalid upstream choice index")
		}
		if len(chunk.Logprobs) > 0 && string(chunk.Logprobs) != "null" {
			return fmt.Errorf("%w: logprobs", errBufferedStreamUnsupported)
		}
		choice := a.choices[chunk.Index]
		if choice == nil {
			choice = &bufferedChatChoice{message: bufferedJSON{}, text: map[string]*strings.Builder{}, tools: map[int]*bufferedChatTool{}}
			a.choices[chunk.Index] = choice
		}
		for key, value := range chunk.Delta {
			if string(value) == "null" {
				continue
			}
			switch key {
			case "content", "reasoning_content", "reasoning", "signature", "refusal":
				var fragment string
				if common.Unmarshal(value, &fragment) != nil {
					return fmt.Errorf("unsupported streamed %s", key)
				}
				if choice.text[key] == nil {
					choice.text[key] = &strings.Builder{}
				}
				choice.text[key].WriteString(fragment)
			case "tool_calls":
				if err := choice.addTools(value); err != nil {
					return err
				}
			case "annotations":
				var annotations, incoming []json.RawMessage
				if len(choice.message[key]) > 0 {
					if err := common.Unmarshal(choice.message[key], &annotations); err != nil {
						return err
					}
				}
				if err := common.Unmarshal(value, &incoming); err != nil {
					return err
				}
				choice.message[key] = rawBufferedValue(append(annotations, incoming...))
			case "function_call", "audio", "images":
				return fmt.Errorf("%w: %s deltas", errBufferedStreamUnsupported, key)
			case "extra_content":
				if err := mergeBufferedObject(choice.message, bufferedJSON{key: value}); err != nil {
					return err
				}
			default:
				if len(choice.message[key]) == 0 || string(choice.message[key]) == `""` {
					choice.message[key] = value
				}
			}
		}
		if len(chunk.FinishReason) > 0 && string(chunk.FinishReason) != "null" && string(chunk.FinishReason) != `""` {
			choice.finishReason = chunk.FinishReason
		}
	}
	return nil
}

func (choice *bufferedChatChoice) addTools(data json.RawMessage) error {
	var calls []bufferedJSON
	if err := common.Unmarshal(data, &calls); err != nil {
		return err
	}
	for _, call := range calls {
		var index int
		if raw := call["index"]; len(raw) == 0 || common.Unmarshal(raw, &index) != nil || index < 0 {
			return errors.New("invalid streamed tool index")
		}
		tool := choice.tools[index]
		if tool == nil {
			tool = &bufferedChatTool{fields: bufferedJSON{}, function: bufferedJSON{}}
			choice.tools[index] = tool
		}
		if _, custom := call["custom"]; custom {
			return fmt.Errorf("%w: custom tool deltas", errBufferedStreamUnsupported)
		}
		for key, value := range call {
			if key == "index" {
				continue
			}
			if key != "function" {
				if len(tool.fields[key]) == 0 || string(tool.fields[key]) == `""` || string(tool.fields[key]) == "null" {
					tool.fields[key] = value
				}
				continue
			}
			var function bufferedJSON
			if err := common.Unmarshal(value, &function); err != nil {
				return err
			}
			for name, field := range function {
				if name == "arguments" {
					var fragment string
					if err := common.Unmarshal(field, &fragment); err != nil {
						return err
					}
					tool.arguments.WriteString(fragment)
				} else if len(tool.function[name]) == 0 || string(tool.function[name]) == `""` {
					tool.function[name] = field
				}
			}
		}
	}
	return nil
}

func (a *bufferedChat) finish() (bufferedJSON, error) {
	if len(a.choices) == 0 {
		return nil, errors.New("upstream stream has no choices")
	}
	choices := make([]bufferedJSON, 0, len(a.choices))
	for _, index := range slices.Sorted(maps.Keys(a.choices)) {
		choice := a.choices[index]
		if len(choice.finishReason) == 0 {
			return nil, errors.New("upstream stream ended before choice completion")
		}
		for key, builder := range choice.text {
			choice.message[key] = rawBufferedValue(builder.String())
		}
		if len(choice.message["role"]) == 0 || string(choice.message["role"]) == `""` {
			choice.message["role"] = json.RawMessage(`"assistant"`)
		}
		if len(choice.message["content"]) == 0 {
			choice.message["content"] = json.RawMessage("null")
		}
		if len(choice.tools) > 0 {
			tools := make([]bufferedJSON, 0, len(choice.tools))
			for _, toolIndex := range slices.Sorted(maps.Keys(choice.tools)) {
				tool := choice.tools[toolIndex]
				tool.function["arguments"] = rawBufferedValue(tool.arguments.String())
				tool.fields["function"] = rawBufferedValue(tool.function)
				tools = append(tools, tool.fields)
			}
			choice.message["tool_calls"] = rawBufferedValue(tools)
		}
		choices = append(choices, bufferedJSON{"index": rawBufferedValue(index), "message": rawBufferedValue(choice.message), "finish_reason": choice.finishReason})
	}
	a.root["choices"] = rawBufferedValue(choices)
	return a.root, nil
}

type bufferedMessageBlock struct {
	fields  bufferedJSON
	text    map[string]*strings.Builder
	input   strings.Builder
	stopped bool
}

type bufferedMessages struct {
	root                bufferedJSON
	blocks              map[int]*bufferedMessageBlock
	stopped             bool
	outputUsageReported bool
}

func (a *bufferedMessages) add(payload bufferedJSON) error {
	var eventType string
	if err := common.Unmarshal(payload["type"], &eventType); err != nil {
		return err
	}
	if eventType == "ping" {
		return nil
	}
	if a.stopped {
		return errors.New("upstream event after message_stop")
	}
	if eventType == "message_start" {
		if a.root != nil {
			return errors.New("duplicate message_start")
		}
		if err := common.Unmarshal(payload["message"], &a.root); err != nil || a.root == nil {
			return errors.New("invalid message_start")
		}
		var usage dto.ClaudeUsage
		if common.Unmarshal(a.root["usage"], &usage) == nil {
			a.outputUsageReported = usage.OutputTokens > 0
		}
		return nil
	}
	if a.root == nil {
		return errors.New("upstream message_start is missing")
	}
	if eventType == "message_delta" {
		var delta bufferedJSON
		if err := common.Unmarshal(payload["delta"], &delta); err != nil {
			return err
		}
		if err := mergeBufferedObject(a.root, delta); err != nil {
			return err
		}
		if usage := payload["usage"]; len(usage) > 0 && string(usage) != "null" {
			var fields bufferedJSON
			if err := common.Unmarshal(usage, &fields); err != nil {
				return err
			}
			if _, reported := fields["output_tokens"]; reported {
				a.outputUsageReported = true
			}
			return mergeBufferedObject(a.root, bufferedJSON{"usage": usage})
		}
		return nil
	}
	if eventType == "message_stop" {
		a.stopped = true
		return nil
	}
	var index int
	if raw := payload["index"]; len(raw) == 0 || common.Unmarshal(raw, &index) != nil || index < 0 {
		return errors.New("invalid content block index")
	}
	block := a.blocks[index]
	if eventType == "content_block_start" {
		if block != nil {
			return errors.New("duplicate content block")
		}
		block = &bufferedMessageBlock{text: map[string]*strings.Builder{}}
		if err := common.Unmarshal(payload["content_block"], &block.fields); err != nil || block.fields == nil {
			return errors.New("invalid content block")
		}
		a.blocks[index] = block
		return nil
	}
	if block == nil || block.stopped {
		return errors.New("content block is not active")
	}
	if eventType == "content_block_stop" {
		block.stopped = true
		if block.input.Len() > 0 {
			var input bufferedJSON
			if common.UnmarshalJsonStr(block.input.String(), &input) != nil || input == nil {
				return errors.New("invalid streamed tool input JSON")
			}
			block.fields["input"] = rawBufferedValue(input)
		}
		return nil
	}
	if eventType != "content_block_delta" {
		return fmt.Errorf("unsupported upstream message event %s", eventType)
	}
	var delta bufferedJSON
	if err := common.Unmarshal(payload["delta"], &delta); err != nil {
		return err
	}
	var deltaType string
	if err := common.Unmarshal(delta["type"], &deltaType); err != nil {
		return err
	}
	var blockType string
	if err := common.Unmarshal(block.fields["type"], &blockType); err != nil {
		return err
	}
	if (deltaType == "text_delta" || deltaType == "citations_delta") && blockType != "text" ||
		(deltaType == "thinking_delta" || deltaType == "signature_delta") && blockType != "thinking" ||
		deltaType == "input_json_delta" && blockType != "tool_use" && blockType != "server_tool_use" {
		return errors.New("streamed delta does not match content block type")
	}
	key := ""
	switch deltaType {
	case "text_delta":
		key = "text"
	case "thinking_delta":
		key = "thinking"
	case "signature_delta":
		key = "signature"
	case "input_json_delta":
		var fragment string
		if err := common.Unmarshal(delta["partial_json"], &fragment); err != nil {
			return err
		}
		block.input.WriteString(fragment)
		return nil
	case "citations_delta":
		var citations []json.RawMessage
		if raw := block.fields["citations"]; len(raw) > 0 {
			if err := common.Unmarshal(raw, &citations); err != nil {
				return err
			}
		}
		if len(delta["citation"]) == 0 {
			return errors.New("missing streamed citation")
		}
		block.fields["citations"] = rawBufferedValue(append(citations, delta["citation"]))
		return nil
	default:
		return fmt.Errorf("unsupported content delta %s", deltaType)
	}
	var fragment string
	if err := common.Unmarshal(delta[key], &fragment); err != nil {
		return err
	}
	if block.text[key] == nil {
		block.text[key] = &strings.Builder{}
		if raw := block.fields[key]; len(raw) > 0 {
			var start string
			if err := common.Unmarshal(raw, &start); err != nil {
				return err
			}
			block.text[key].WriteString(start)
		}
	}
	block.text[key].WriteString(fragment)
	return nil
}

func (a *bufferedMessages) finish() (bufferedJSON, error) {
	if a.root == nil || !a.stopped {
		return nil, errors.New("upstream stream ended before message_stop")
	}
	var id, stopReason string
	if common.Unmarshal(a.root["id"], &id) != nil || id == "" || common.Unmarshal(a.root["stop_reason"], &stopReason) != nil || stopReason == "" {
		return nil, errors.New("incomplete upstream message")
	}
	content := make([]bufferedJSON, 0, len(a.blocks))
	for _, index := range slices.Sorted(maps.Keys(a.blocks)) {
		block := a.blocks[index]
		if !block.stopped {
			return nil, errors.New("upstream content block did not stop")
		}
		for key, builder := range block.text {
			block.fields[key] = rawBufferedValue(builder.String())
		}
		content = append(content, block.fields)
	}
	a.root["content"] = rawBufferedValue(content)
	a.root["type"] = json.RawMessage(`"message"`)
	return a.root, nil
}

// Responses upstreams may be selected by the existing Chat-to-Responses rule.
// Reuse their structured accumulator, but enforce the same bounded transport and
// terminal-event requirement as Chat and Messages.
type bufferedResponses struct {
	accumulator *relayconvert.ResponsesBufferedAccumulator
	final       *dto.OpenAIResponsesResponse
}

func (a *bufferedResponses) add(payload bufferedJSON) error {
	data, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	var event dto.ResponsesStreamResponse
	if err := common.Unmarshal(data, &event); err != nil {
		return err
	}
	a.accumulator.ProcessEvent(&event)
	switch event.Type {
	case "response.completed", "response.done", "response.incomplete":
		if event.Response == nil {
			return errors.New("missing terminal Responses payload")
		}
		a.final = event.Response
	case "response.failed", "response.error", "error":
		return errors.New("upstream Responses stream failed")
	}
	return nil
}
func (a *bufferedResponses) finish() (bufferedJSON, error) {
	if a.final == nil {
		return nil, errors.New("upstream Responses stream ended without completion")
	}
	a.accumulator.SupplementResponseOutput(a.final)
	data, err := common.Marshal(a.final)
	if err != nil {
		return nil, err
	}
	var result bufferedJSON
	err = common.Unmarshal(data, &result)
	return result, err
}
