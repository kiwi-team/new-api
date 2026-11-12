package speech

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const maxAudioBytes = 128 << 20

// Result tracks generation separately from delivery. An archive/download/write
// failure after generation must settle usage and must never replay synthesis.
type Result struct {
	Usage     *dto.Usage
	Generated bool
	Written   bool
	Response  string
}

type output struct {
	c                                     *gin.Context
	info                                  *relaycommon.RelayInfo
	r                                     *Request
	result                                Result
	bytes                                 int64
	sequence                              int
	file                                  *os.File
	url                                   string
	complete                              bool
	native                                map[string]any
	characters, inputTokens, outputTokens *int
}

func (o *output) contentType() string {
	switch o.r.ResponseFormat {
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/pcm"
	case "opus":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	default:
		return "audio/mpeg"
	}
}

func (o *output) event(name string, payload any) error {
	data, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	helper.SetEventStreamHeaders(o.c)
	o.result.Written = true
	helper.ExtendWriteDeadline(o.c)
	_, err = fmt.Fprintf(o.c.Writer, "event: %s\ndata: %s\n\n", name, data)
	o.c.Writer.Flush()
	return err
}

// Binary audio cannot carry side-channel JSON. SSE clients receive provider
// timing/subtitle facts separately, without mixing them into the audio bytes.
func (o *output) annotation(kind string, data any) error {
	if o.r.StreamFormat != "sse" {
		return nil
	}
	return o.event("speech.audio.metadata", gin.H{"model": o.r.Model, "type": kind, "data": data})
}

func (o *output) audio(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	o.result.Generated = true
	o.info.SetFirstResponseTime()
	if o.bytes+int64(len(data)) > maxAudioBytes {
		return errors.New("speech audio exceeds the 128 MiB limit")
	}
	o.bytes += int64(len(data))
	if o.r.URL {
		if o.file == nil {
			file, err := os.CreateTemp("", "new-api-speech-*")
			if err != nil {
				return err
			}
			o.file = file
		}
		_, err := o.file.Write(data)
		return err
	}
	if o.r.StreamFormat == "sse" {
		err := o.event("speech.audio.delta", gin.H{"audio": base64.StdEncoding.EncodeToString(data), "sequence_number": o.sequence})
		o.sequence++
		return err
	}
	o.c.Header("Content-Type", o.contentType())
	o.c.Header("Cache-Control", "no-store")
	o.result.Written = true
	helper.ExtendWriteDeadline(o.c)
	_, err := o.c.Writer.Write(data)
	o.c.Writer.Flush()
	return err
}

func (o *output) download() error {
	o.result.Generated = true
	if o.r.URL {
		return nil
	}
	resp, err := service.DoDownloadRequestWithContext(o.c.Request.Context(), o.url, "speech output")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("speech audio download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxAudioBytes {
		return errors.New("speech audio exceeds the 128 MiB limit")
	}
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if err := o.audio(buffer[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if o.bytes == 0 {
		return errors.New("empty speech audio download")
	}
	return nil
}

func (o *output) finish() error {
	if o.url != "" && o.bytes == 0 {
		if err := service.ValidateSSRFProtectedFetchURL(o.url); err != nil {
			return errors.New("unsafe speech audio URL")
		}
		if err := o.download(); err != nil {
			return err
		}
	}
	if o.bytes == 0 && o.url == "" {
		return errors.New("upstream returned no speech audio")
	}
	if o.r.URL {
		if o.url == "" {
			if _, err := o.file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			url, err := service.UploadReaderToS3(o.c.Request.Context(), o.file, o.bytes, o.contentType(), "."+o.r.ResponseFormat)
			if err != nil {
				return fmt.Errorf("speech archive failed: %w", err)
			}
			o.url = url
		}
		common.SetContextKey(o.c, constant.ContextKeyAudioUrl, o.url)
		common.ApiSuccess(o.c, gin.H{"audio_url": o.url})
		o.result.Written = true
	}
	return nil
}

// usage retains presence and native snapshots; cumulative frames replace
// present fields, while empty final control frames cannot erase earlier usage.
func (o *output) usage(native map[string]any, charsKey string) error {
	if native == nil {
		return nil
	}
	if o.native == nil {
		o.native = make(map[string]any)
	}
	for key, value := range native {
		o.native[key] = value
	}
	for key, target := range map[string]**int{charsKey: &o.characters, "input_tokens": &o.inputTokens, "output_tokens": &o.outputTokens} {
		if key == "" {
			continue
		}
		value, present := native[key]
		if !present || value == nil {
			continue
		}
		encoded, err := common.Marshal(value)
		if err != nil {
			return err
		}
		var count int
		if err := common.Unmarshal(encoded, &count); err != nil || count < 0 || count > 2147483647 {
			return fmt.Errorf("invalid speech usage field %s", key)
		}
		*target = &count
	}
	return nil
}

func (o *output) finalize(err error) Result {
	if o.file != nil {
		name := o.file.Name()
		_ = o.file.Close()
		_ = os.Remove(name)
	}
	raw, _ := common.Marshal(o.native)
	speech := &dto.SpeechBillingUsage{Characters: o.characters, InputTokens: o.inputTokens, OutputTokens: o.outputTokens, Native: json.RawMessage(raw)}
	missing := o.characters == nil
	if !o.r.CharacterBilling() {
		missing = o.inputTokens == nil || o.outputTokens == nil
	}
	if missing && o.result.Generated && o.info.UsageBillingFallback == "" {
		o.info.UsageBillingFallback = "missing_usage"
	}
	billing := &dto.BillingUsage{Source: "speech", Semantic: dto.BillingUsageSemanticOpenAI, SpeechUsage: speech, Estimated: o.info.UsageBillingFallback != ""}
	o.result.Usage, _ = billing.CanonicalUsage()
	if o.result.Usage == nil {
		o.result.Usage = &dto.Usage{UsageSource: "speech", BillingUsage: billing}
	}
	response := gin.H{"request_id": o.c.GetString(common.UpstreamRequestIdKey), "usage": speech, "audio_bytes": o.bytes, "audio_url": o.url, "complete": o.complete}
	if err != nil {
		response["error"] = err.Error()
	}
	o.result.Response = common.JsonStringify(response)
	common.SetContextKey(o.c, constant.ContextKeySdkResponseStr, o.result.Response)
	return o.result
}
