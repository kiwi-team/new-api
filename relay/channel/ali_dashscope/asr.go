package ali_dashscope

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const maxASRResponseBytes = 64 << 20

type asrFileSizeMB int64

func (size *asrFileSizeMB) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*size = 0
		return nil
	}

	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := common.Unmarshal(data, &value); err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			*size = 0
			return nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid file size in MB %q: %w", value, err)
		}
		*size = asrFileSizeMB(parsed)
		return nil
	}

	var value int64
	if err := common.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("invalid numeric file size in MB: %w", err)
	}
	*size = asrFileSizeMB(value)
	return nil
}

type asrUploadPolicyResponse struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Data      struct {
		Policy             string        `json:"policy"`
		Signature          string        `json:"signature"`
		UploadDir          string        `json:"upload_dir"`
		UploadHost         string        `json:"upload_host"`
		MaxFileSizeMB      asrFileSizeMB `json:"max_file_size_mb"`
		OSSAccessKeyID     string        `json:"oss_access_key_id"`
		OSSObjectACL       string        `json:"x_oss_object_acl"`
		OSSForbidOverwrite string        `json:"x_oss_forbid_overwrite"`
	} `json:"data"`
}

type asrRequest struct {
	Model string `json:"model"`
	Input struct {
		FileURLs []string `json:"file_urls"`
	} `json:"input"`
	Parameters map[string]any `json:"parameters"`
}

type asrFlashRequest struct {
	Model string `json:"model"`
	Input struct {
		Messages []asrFlashMessage `json:"messages"`
	} `json:"input"`
	Parameters map[string]any `json:"parameters"`
}

type asrFlashMessage struct {
	Role    string            `json:"role"`
	Content []asrFlashContent `json:"content"`
}

type asrFlashContent struct {
	Type       string `json:"type"`
	InputAudio struct {
		Data string `json:"data"`
	} `json:"input_audio"`
}

type asrFlashResponse struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Output    struct {
		Text     string `json:"text"`
		Sentence struct {
			BeginTime   int64  `json:"begin_time"`
			EndTime     int64  `json:"end_time"`
			SentenceEnd bool   `json:"sentence_end"`
			Text        string `json:"text"`
		} `json:"sentence"`
	} `json:"output"`
	Usage struct {
		Duration float64 `json:"duration"`
	} `json:"usage"`
}

func IsFunASRFlashModel(model string) bool {
	return strings.HasPrefix(model, "fun-asr-flash-")
}

func IsQwen3ASRFlashModel(model string) bool {
	return model == "qwen3-asr-flash" ||
		(strings.HasPrefix(model, "qwen3-asr-flash-") && !strings.Contains(model, "realtime") && !strings.Contains(model, "filetrans"))
}

func NormalizeQwen3ASRUsage(info *relaycommon.RelayInfo, usage *dto.Usage) {
	if info == nil || usage == nil || usage.Seconds <= 0 {
		return
	}
	audioTokens, clamp := common.QuotaRoundChecked(math.Ceil(float64(usage.Seconds)) / 60 * 1000)
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	usage.PromptTokens = audioTokens
	usage.PromptTokensDetails.AudioTokens = audioTokens
	usage.TotalTokens = audioTokens + usage.CompletionTokens
}

type asrTaskResponse struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Output    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Results    []struct {
			SubtaskStatus    string `json:"subtask_status"`
			TranscriptionURL string `json:"transcription_url"`
			Code             string `json:"code"`
			Message          string `json:"message"`
		} `json:"results"`
	} `json:"output"`
	Usage struct {
		Duration float64 `json:"duration"`
	} `json:"usage"`
}

type asrTranscript struct {
	Properties struct {
		OriginalDurationMilliseconds int64 `json:"original_duration_in_milliseconds"`
	} `json:"properties"`
	Transcripts []asrTranscriptItem `json:"transcripts"`
}

type asrTranscriptItem struct {
	ContentDurationMilliseconds int64         `json:"content_duration_in_milliseconds"`
	Text                        string        `json:"text"`
	Sentences                   []asrSentence `json:"sentences"`
}

type asrSentence struct {
	BeginTime int64  `json:"begin_time"`
	EndTime   int64  `json:"end_time"`
	Text      string `json:"text"`
}

// ConvertASRRequest uploads the OpenAI multipart file to DashScope's temporary
// OSS storage and builds the matching synchronous or asynchronous ASR request.
func ConvertASRRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse audio form: %w", err)
	}
	defer form.RemoveAll()
	files := form.File["file"]
	if len(files) == 0 {
		return nil, errors.New("file is required")
	}
	fileHeader := files[0]

	policy, err := getASRUploadPolicy(c, info, request.Model)
	if err != nil {
		return nil, err
	}
	if maxMB := int64(policy.Data.MaxFileSizeMB); maxMB > 0 && fileHeader.Size > maxMB<<20 {
		return nil, fmt.Errorf("audio file exceeds DashScope upload limit of %d MB", maxMB)
	}

	ossURL, err := uploadASRFile(c, info, policy, fileHeader)
	if err != nil {
		return nil, err
	}

	parameters := make(map[string]any)
	if values := form.Value["parameters"]; len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		if err := common.UnmarshalJsonStr(values[0], &parameters); err != nil {
			return nil, fmt.Errorf("parameters must be a JSON object: %w", err)
		}
	}
	if values := form.Value["language"]; len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		parameters["language_hints"] = []string{strings.TrimSpace(values[0])}
	}

	var upstream any
	if IsFunASRFlashModel(request.Model) {
		if _, ok := parameters["format"]; !ok {
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileHeader.Filename)), ".")
			if format == "" {
				return nil, errors.New("parameters.format is required when the audio filename has no extension")
			}
			parameters["format"] = format
		}
		flashRequest := asrFlashRequest{Model: request.Model, Parameters: parameters}
		content := asrFlashContent{Type: "input_audio"}
		content.InputAudio.Data = ossURL
		flashRequest.Input.Messages = []asrFlashMessage{{Role: "user", Content: []asrFlashContent{content}}}
		upstream = flashRequest
	} else {
		asrRequest := asrRequest{Model: request.Model, Parameters: parameters}
		asrRequest.Input.FileURLs = []string{ossURL}
		upstream = asrRequest
	}
	body, err := common.Marshal(upstream)
	if err != nil {
		return nil, fmt.Errorf("marshal DashScope ASR request: %w", err)
	}
	return bytes.NewReader(body), nil
}

func getASRUploadPolicy(c *gin.Context, info *relaycommon.RelayInfo, model string) (*asrUploadPolicyResponse, error) {
	endpoint := strings.TrimRight(info.ChannelBaseUrl, "/") + "/api/v1/uploads?action=getPolicy&model=" + url.QueryEscape(model)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create DashScope upload policy request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	if info.Organization != "" {
		req.Header.Set("X-DashScope-WorkSpace", info.Organization)
	}
	resp, err := doASRRequest(info, req)
	if err != nil {
		return nil, fmt.Errorf("request DashScope upload policy: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := readASRBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DashScope upload policy returned status %d: %s", resp.StatusCode, common.LocalLogPreview(string(body)))
	}
	var result asrUploadPolicyResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode DashScope upload policy: %w", err)
	}
	if result.Code != "" || result.Data.UploadHost == "" || result.Data.UploadDir == "" {
		return nil, fmt.Errorf("DashScope upload policy failed: %s %s", result.Code, result.Message)
	}
	return &result, nil
}

func uploadASRFile(c *gin.Context, info *relaycommon.RelayInfo, policy *asrUploadPolicyResponse, fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open audio file: %w", err)
	}
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	fields := map[string]string{
		"OSSAccessKeyId":         policy.Data.OSSAccessKeyID,
		"Signature":              policy.Data.Signature,
		"policy":                 policy.Data.Policy,
		"x-oss-object-acl":       policy.Data.OSSObjectACL,
		"x-oss-forbid-overwrite": policy.Data.OSSForbidOverwrite,
		"success_action_status":  "200",
	}
	filename := filepath.Base(fileHeader.Filename)
	if filename == "." || filename == "" {
		filename = "audio"
	}
	objectKey := strings.TrimRight(policy.Data.UploadDir, "/") + "/" + filename
	fields["key"] = objectKey
	go func() {
		defer file.Close()
		for name, value := range fields {
			if err := writer.WriteField(name, value); err != nil {
				_ = pipeWriter.CloseWithError(fmt.Errorf("write DashScope upload field: %w", err))
				return
			}
		}
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("create DashScope audio upload part: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("copy DashScope audio upload: %w", err))
			return
		}
		if err := writer.Close(); err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("finish DashScope audio upload: %w", err))
			return
		}
		_ = pipeWriter.Close()
	}()

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, policy.Data.UploadHost, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		return "", fmt.Errorf("create DashScope audio upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := doASRRequest(info, req)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		return "", fmt.Errorf("upload audio to DashScope: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)
	responseBody, readErr := readASRBody(resp.Body)
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("DashScope audio upload returned status %d: %s", resp.StatusCode, common.LocalLogPreview(string(responseBody)))
	}
	return "oss://" + objectKey, nil
}

// ASRHandler waits for the asynchronous task, fetches its transcript and
// renders the result in the OpenAI transcription response format.
func ASRHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	if IsFunASRFlashModel(info.UpstreamModelName) {
		return asrFlashHandler(c, resp, info)
	}

	defer service.CloseResponseBodyGracefully(resp)
	body, err := readASRBody(resp.Body)
	if err != nil {
		return asrAPIError(err), nil
	}
	var submitted asrTaskResponse
	if err := common.Unmarshal(body, &submitted); err != nil {
		return asrAPIError(fmt.Errorf("decode DashScope ASR submission: %w", err)), nil
	}
	if submitted.Code != "" || submitted.Output.TaskID == "" {
		return asrAPIError(fmt.Errorf("DashScope ASR submission failed: %s %s", submitted.Code, submitted.Message)), nil
	}

	task, err := waitForASRTask(c, info, submitted.Output.TaskID)
	if err != nil {
		return asrAPIError(err), nil
	}
	if len(task.Output.Results) == 0 {
		return asrAPIError(errors.New("DashScope ASR task returned no results")), nil
	}
	result := task.Output.Results[0]
	if result.SubtaskStatus != "SUCCEEDED" || result.TranscriptionURL == "" {
		return asrAPIError(fmt.Errorf("DashScope ASR subtask failed: %s %s", result.Code, result.Message)), nil
	}
	transcript, err := fetchASRTranscript(c, info, result.TranscriptionURL)
	if err != nil {
		return asrAPIError(err), nil
	}

	textParts := make([]string, 0, len(transcript.Transcripts))
	for _, item := range transcript.Transcripts {
		if item.Text != "" {
			textParts = append(textParts, item.Text)
		}
	}
	text := strings.Join(textParts, "\n")
	duration := task.Usage.Duration
	if duration <= 0 {
		for _, item := range transcript.Transcripts {
			duration += float64(max(item.ContentDurationMilliseconds, 0)) / 1000
		}
	}
	if duration <= 0 {
		duration = float64(max(transcript.Properties.OriginalDurationMilliseconds, 0)) / 1000
	}
	if err := writeASRResponse(c, info, transcript, text, duration); err != nil {
		return asrAPIError(err), nil
	}

	audioTokens, clamp := common.QuotaRoundChecked(math.Ceil(max(duration, 0)) / 60 * 1000)
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	usage := &dto.Usage{
		PromptTokens: audioTokens,
		TotalTokens:  audioTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			AudioTokens: audioTokens,
		},
	}
	return nil, usage
}

func asrFlashHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)
	body, err := readASRBody(resp.Body)
	if err != nil {
		return asrAPIError(err), nil
	}
	var result asrFlashResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return asrAPIError(fmt.Errorf("decode DashScope ASR flash response: %w", err)), nil
	}
	if result.Code != "" {
		return asrAPIError(fmt.Errorf("DashScope ASR flash failed: %s %s", result.Code, result.Message)), nil
	}

	var transcript asrTranscript
	transcript.Properties.OriginalDurationMilliseconds = int64(math.Ceil(max(result.Usage.Duration, 0) * 1000))
	if result.Output.Sentence.Text != "" {
		transcript.Transcripts = append(transcript.Transcripts, asrTranscriptItem{
			ContentDurationMilliseconds: max(result.Output.Sentence.EndTime-result.Output.Sentence.BeginTime, 0),
			Text:                        result.Output.Text,
			Sentences: []asrSentence{{
				BeginTime: result.Output.Sentence.BeginTime,
				EndTime:   result.Output.Sentence.EndTime,
				Text:      result.Output.Sentence.Text,
			}},
		})
	}
	if err := writeASRResponse(c, info, &transcript, result.Output.Text, result.Usage.Duration); err != nil {
		return asrAPIError(err), nil
	}

	audioTokens, clamp := common.QuotaRoundChecked(math.Ceil(max(result.Usage.Duration, 0)) / 60 * 1000)
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	return nil, &dto.Usage{
		PromptTokens: audioTokens,
		TotalTokens:  audioTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			AudioTokens: audioTokens,
		},
	}
}

func waitForASRTask(c *gin.Context, info *relaycommon.RelayInfo, taskID string) (*asrTaskResponse, error) {
	endpoint := strings.TrimRight(info.ChannelBaseUrl, "/") + "/api/v1/tasks/" + url.PathEscape(taskID)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("create DashScope ASR task request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+info.ApiKey)
		if info.Organization != "" {
			req.Header.Set("X-DashScope-WorkSpace", info.Organization)
		}
		resp, err := doASRRequest(info, req)
		if err != nil {
			return nil, fmt.Errorf("query DashScope ASR task: %w", err)
		}
		body, readErr := readASRBody(resp.Body)
		service.CloseResponseBodyGracefully(resp)
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("DashScope ASR task returned status %d: %s", resp.StatusCode, common.LocalLogPreview(string(body)))
		}
		var task asrTaskResponse
		if err := common.Unmarshal(body, &task); err != nil {
			return nil, fmt.Errorf("decode DashScope ASR task: %w", err)
		}
		if task.Code != "" {
			return nil, fmt.Errorf("DashScope ASR task failed: %s %s", task.Code, task.Message)
		}
		switch task.Output.TaskStatus {
		case "SUCCEEDED":
			return &task, nil
		case "FAILED", "CANCELED", "UNKNOWN":
			return nil, fmt.Errorf("DashScope ASR task ended with status %s", task.Output.TaskStatus)
		}
		select {
		case <-c.Request.Context().Done():
			return nil, c.Request.Context().Err()
		case <-ticker.C:
		}
	}
}

func fetchASRTranscript(c *gin.Context, info *relaycommon.RelayInfo, transcriptURL string) (*asrTranscript, error) {
	parsed, err := url.Parse(transcriptURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("DashScope ASR returned an invalid transcription URL")
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, transcriptURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create DashScope transcript request: %w", err)
	}
	resp, err := doASRRequest(info, req)
	if err != nil {
		return nil, fmt.Errorf("fetch DashScope transcript: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := readASRBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DashScope transcript returned status %d", resp.StatusCode)
	}
	var transcript asrTranscript
	if err := common.Unmarshal(body, &transcript); err != nil {
		return nil, fmt.Errorf("decode DashScope transcript: %w", err)
	}
	return &transcript, nil
}

func writeASRResponse(c *gin.Context, info *relaycommon.RelayInfo, transcript *asrTranscript, text string, duration float64) error {
	request, _ := info.Request.(*dto.AudioRequest)
	responseFormat := "json"
	if request != nil && request.ResponseFormat != "" {
		responseFormat = request.ResponseFormat
	}
	if responseFormat == "text" {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
		return nil
	}
	if responseFormat == "verbose_json" {
		result := dto.WhisperVerboseJSONResponse{Task: "transcribe", Duration: duration, Text: text}
		segmentID := 0
		for _, channelTranscript := range transcript.Transcripts {
			for _, sentence := range channelTranscript.Sentences {
				result.Segments = append(result.Segments, dto.Segment{
					Id:    segmentID,
					Start: float64(sentence.BeginTime) / 1000,
					End:   float64(sentence.EndTime) / 1000,
					Text:  sentence.Text,
				})
				segmentID++
			}
		}
		c.JSON(http.StatusOK, result)
		return nil
	}
	if responseFormat == "srt" || responseFormat == "vtt" {
		var output strings.Builder
		if responseFormat == "vtt" {
			output.WriteString("WEBVTT\n\n")
		}
		segmentID := 1
		for _, channelTranscript := range transcript.Transcripts {
			for _, sentence := range channelTranscript.Sentences {
				if responseFormat == "srt" {
					fmt.Fprintf(&output, "%d\n", segmentID)
				}
				fmt.Fprintf(&output, "%s --> %s\n%s\n\n", formatASRTimestamp(sentence.BeginTime, responseFormat), formatASRTimestamp(sentence.EndTime, responseFormat), sentence.Text)
				segmentID++
			}
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(output.String()))
		return nil
	}
	if responseFormat == "json" {
		c.JSON(http.StatusOK, dto.AudioResponse{Text: text})
		return nil
	}
	return fmt.Errorf("unsupported response_format %q for DashScope ASR", responseFormat)
}

func formatASRTimestamp(milliseconds int64, responseFormat string) string {
	milliseconds = max(milliseconds, 0)
	hours := milliseconds / 3_600_000
	minutes := milliseconds / 60_000 % 60
	seconds := milliseconds / 1000 % 60
	millis := milliseconds % 1000
	separator := ','
	if responseFormat == "vtt" {
		separator = '.'
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", hours, minutes, seconds, separator, millis)
}

func doASRRequest(info *relaycommon.RelayInfo, req *http.Request) (*http.Response, error) {
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func readASRBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxASRResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read DashScope ASR response: %w", err)
	}
	if len(body) > maxASRResponseBytes {
		return nil, fmt.Errorf("DashScope ASR response exceeds %d bytes", maxASRResponseBytes)
	}
	return body, nil
}

func asrAPIError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
}
