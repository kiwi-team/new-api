package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const geminiFilePlanKey = "gemini_file_plan"
const maxGeminiFiles = 16
const maxGeminiFileBytes int64 = 2 << 30
const maxGeminiTotalFileBytes int64 = 4 << 30
const maxGeminiProbeBytes int64 = 64 << 20

// The slot is held until the complete request finishes, including retries and SSE.
var geminiFileSlots = make(chan struct{}, 8)

type GeminiFilePlan struct {
	Config           model_setting.GeminiFileRouting
	Native           bool
	Files            []*GeminiPlannedFile
	RequiresStorage  bool
	Submitted        bool
	DeveloperAPIOnly bool
	client           *http.Client
	probeBytes       int64
	objects          map[string]geminiUploadedObject
	release          func()
}

type GeminiPlannedFile struct {
	MessageIndex  int
	PartIndex     int
	Position      string
	Source        types.FileSource
	MIMEType      string
	ExpectedType  string
	Size          int64
	Large         bool
	Special       bool
	ExistingGCS   bool
	prefixHash    [32]byte
	prefixSize    int64
	etag          string
	contentLength int64
	resolved      *relayconvert.GeminiMedia
}

func FileRoutingError(code, message string, status int) *types.NewAPIError {
	return types.NewErrorWithStatusCode(fmt.Errorf("%s", message), types.ErrorCode(code), status, types.ErrOptionWithSkipRetry())
}

func GetGeminiFilePlan(c *gin.Context) *GeminiFilePlan {
	if c == nil {
		return nil
	}
	value, _ := c.Get(geminiFilePlanKey)
	plan, _ := value.(*GeminiFilePlan)
	return plan
}

// PlanGeminiFiles runs after authorization and before selecting any channel.
// It leaves the original request body intact for route matching and retry replay.
func PlanGeminiFiles(c *gin.Context, modelName string) (*GeminiFilePlan, *types.NewAPIError) {
	path := c.Request.URL.Path
	native := (strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/")) &&
		(strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent"))
	chat := path == "/v1/chat/completions"
	if !native && !chat {
		return nil, nil
	}
	config, err := model_setting.GetGeminiFileRouting()
	if err != nil {
		return nil, FileRoutingError("large_file_routing_config_invalid", err.Error(), 503)
	}
	supported := config.Enabled && (native || slices.ContainsFunc(config.ChatModelPatterns, func(pattern string) bool {
		return pattern == modelName || common.RegMatch(pattern, modelName)
	}))
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, FileRoutingError("invalid_file_input", "cannot read request body", 400)
	}
	body, err := storage.Bytes()
	if err != nil || !gjson.ValidBytes(body) {
		return nil, FileRoutingError("invalid_file_input", "invalid JSON request", 400)
	}
	plan := &GeminiFilePlan{Config: config, Native: native, objects: make(map[string]geminiUploadedObject)}
	root := gjson.ParseBytes(body)
	if supported && !validGeminiFileJSON(root, 0) {
		return nil, FileRoutingError("invalid_file_input", "ambiguous or excessively nested request JSON", 400)
	}
	if supported && hasNoncanonicalFileFields(root, "messages", "contents", "systemInstruction", "system_instruction", "cachedContent", "cached_content", "requests") {
		return nil, FileRoutingError("invalid_file_input", "invalid file request field casing", 400)
	}
	if supported && ((native && !root.Get("contents").IsArray()) || (chat && !root.Get("messages").IsArray())) {
		return nil, FileRoutingError("invalid_file_input", "file routing requires canonical contents or messages", 400)
	}
	if native {
		var request dto.GeminiChatRequest
		if err := common.Unmarshal(body, &request); err != nil {
			return nil, FileRoutingError("invalid_file_input", "invalid or conflicting Gemini attachment fields", 400)
		}
		if !supported {
			return nil, nil
		}
		if root.Get("requests").Exists() {
			return nil, FileRoutingError("unsupported_file_input", "batch requests are not supported by file routing", 400)
		}
		for mi, content := range request.Contents {
			for pi, part := range content.Parts {
				position := fmt.Sprintf("contents[%d].parts[%d]", mi, pi)
				rawPart := root.Get(fmt.Sprintf("contents.%d.parts.%d", mi, pi))
				if rawPart.Get("function_response.parts").Exists() || (part.FunctionResponse != nil && len(part.FunctionResponse.Parts) > 0) {
					return nil, FileRoutingError("unsupported_file_reference", position+": nested function response attachments are unsupported", 400)
				}
				file := &GeminiPlannedFile{MessageIndex: mi, PartIndex: pi, Position: position}
				if part.InlineData != nil {
					if !rawPart.Get("inlineData").Exists() && !rawPart.Get("inline_data").Exists() {
						return nil, FileRoutingError("invalid_file_input", position+": invalid attachment field casing", 400)
					}
					file.Source = types.NewBase64FileSource(part.InlineData.Data, part.InlineData.MimeType)
					file.MIMEType = part.InlineData.MimeType
				} else if part.FileData != nil {
					if !rawPart.Get("fileData").Exists() && !rawPart.Get("file_data").Exists() {
						return nil, FileRoutingError("invalid_file_input", position+": invalid attachment field casing", 400)
					}
					file.Source = types.NewURLFileSource(part.FileData.FileUri)
					file.MIMEType = part.FileData.MimeType
				} else {
					continue
				}
				plan.Files = append(plan.Files, file)
			}
		}
		for _, key := range []string{"systemInstruction", "system_instruction"} {
			for _, part := range root.Get(key + ".parts").Array() {
				if part.Get("inlineData").Exists() || part.Get("inline_data").Exists() || part.Get("fileData").Exists() || part.Get("file_data").Exists() || part.Get("functionResponse.parts").Exists() || part.Get("function_response.parts").Exists() {
					return nil, FileRoutingError("unsupported_file_reference", "system instruction attachments are unsupported", 400)
				}
			}
		}
	} else {
		for mi, message := range root.Get("messages").Array() {
			if supported && hasNoncanonicalFileFields(message, "content", "role") {
				return nil, FileRoutingError("invalid_file_input", fmt.Sprintf("messages[%d]: invalid attachment field casing", mi), 400)
			}
			content := message.Get("content")
			if content.Type == gjson.String {
				if supported && strings.Contains(content.String(), "](data:") {
					return nil, FileRoutingError("unsupported_file_input", fmt.Sprintf("messages[%d]: use image_url for embedded markdown images", mi), 400)
				}
				continue
			}
			for pi, part := range content.Array() {
				kind := part.Get("type").String()
				position := fmt.Sprintf("messages[%d].content[%d]", mi, pi)
				if kind == dto.ContentTypePDFURL && !supported {
					return nil, FileRoutingError("unsupported_file_input", position+": PDF file routing is not enabled for this model", 400)
				}
				if !supported {
					continue
				}
				var raw, mimeType, expectedType string
				switch kind {
				case dto.ContentTypeImageURL, dto.ContentTypeAudioUrl, dto.ContentTypeVideoUrl, dto.ContentTypePDFURL:
					expectedType = strings.TrimSuffix(kind, "_url")
					value := part.Get(kind)
					if value.Type == gjson.String && kind != dto.ContentTypePDFURL {
						raw = value.String()
					} else if value.IsObject() && value.Get("url").Type == gjson.String {
						raw = value.Get("url").String()
						mimeType = value.Get("mime_type").String()
					}
					if kind == dto.ContentTypePDFURL {
						mimeType = "application/pdf"
						expectedType = "application"
					}
				case dto.ContentTypeInputAudio:
					expectedType = "audio"
					if part.Get("input_audio.data").Type != gjson.String || part.Get("input_audio.format").Type != gjson.String {
						return nil, FileRoutingError("invalid_file_input", position+": input_audio requires string data and format", 400)
					}
					raw = part.Get("input_audio.data").String()
					mimeType = "audio/" + part.Get("input_audio.format").String()
				case dto.ContentTypeFile:
					if part.Get("file.file_id").Exists() {
						return nil, FileRoutingError("unsupported_file_reference", position+": file_id cannot be resolved by this file route", 400)
					}
					if part.Get("file.file_data").Type == gjson.String {
						raw = part.Get("file.file_data").String()
					}
				case dto.ContentTypeText:
					if strings.Contains(part.Get("text").String(), "](data:") {
						return nil, FileRoutingError("unsupported_file_input", position+": use image_url for embedded markdown images", 400)
					}
					continue
				default:
					return nil, FileRoutingError("unsupported_file_input", position+": unsupported content type", 400)
				}
				if !slices.Contains([]string{"user", "assistant", "model"}, message.Get("role").String()) {
					return nil, FileRoutingError("unsupported_file_reference", position+": attachments are unsupported in this message role", 400)
				}
				if strings.TrimSpace(raw) == "" {
					return nil, FileRoutingError("invalid_file_input", position+"."+kind+": a non-empty file source is required", 400)
				}
				file := &GeminiPlannedFile{MessageIndex: mi, PartIndex: pi, Position: position, MIMEType: mimeType, ExpectedType: expectedType}
				if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "gs://") || strings.HasPrefix(raw, "http://") {
					file.Source = types.NewURLFileSource(raw)
				} else {
					file.Source = types.NewBase64FileSource(raw, mimeType)
				}
				plan.Files = append(plan.Files, file)
			}
		}
	}
	if len(plan.Files) == 0 {
		return nil, nil
	}
	if len(plan.Files) > maxGeminiFiles {
		return nil, FileRoutingError("file_input_too_large", "too many attachments (maximum 16)", 413)
	}
	select {
	case geminiFileSlots <- struct{}{}:
		plan.release = func() { <-geminiFileSlots }
	default:
		return nil, FileRoutingError("file_processing_busy", "file processing capacity exhausted", 503)
	}
	c.Set(geminiFilePlanKey, plan)
	// No environment proxy: destination DNS/IP checks must happen at our dialer.
	protection := system_setting.GetFetchSetting()
	if !protection.EnableSSRFProtection || !protection.ApplyIPFilterForDomain || common.TLSInsecureSkipVerify {
		return plan, FileRoutingError("large_file_routing_config_invalid", "file routing requires SSRF protection, domain IP checks and TLS verification", 503)
	}
	plan.client = newProtectedFetchHTTPClientWithProxy(nil, nil, nil, func(*http.Request) (*url.URL, error) { return nil, nil })
	plan.client.Timeout = 20 * time.Minute
	transport := plan.client.Transport.(*ssrfProtectedRoundTripper)
	transport.responseHeaderTimeout = 30 * time.Second
	transport.tlsHandshakeTimeout = 10 * time.Second
	plan.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.User != nil {
			return fmt.Errorf("unsafe file redirect")
		}
		return checkProtectedFetchRedirect(req, via)
	}
	var tags []string
	var totalSize, inlineBytes int64
	probed := make(map[[32]byte]*GeminiPlannedFile)
	for _, file := range plan.Files {
		key := sha256.Sum256([]byte(file.MIMEType + "\x00" + file.Source.GetRawData()))
		if previous, ok := probed[key]; ok && previous.Source.IsURL() == file.Source.IsURL() {
			file.MIMEType, file.Size, file.Large = previous.MIMEType, previous.Size, previous.Large
			file.ExistingGCS, file.Special = previous.ExistingGCS, previous.Special
			file.prefixHash, file.prefixSize, file.resolved = previous.prefixHash, previous.prefixSize, previous.resolved
			file.etag, file.contentLength = previous.etag, previous.contentLength
		} else {
			if err := plan.probe(c.Request.Context(), file); err != nil {
				return plan, err
			}
			probed[key] = file
		}
		if file.ExpectedType != "" && !strings.HasPrefix(file.MIMEType, file.ExpectedType+"/") {
			return plan, FileRoutingError("invalid_file_input", file.Position+": content does not match the attachment type", 400)
		}
		if !file.Large && !file.ExistingGCS && file.resolved != nil {
			inlineBytes += int64(len(file.resolved.Base64))
			if inlineBytes > config.MaxInlineRequestBytes {
				return plan, FileRoutingError("file_input_too_large", "inline attachments exceed the upstream request size limit", 413)
			}
		}
		if file.Source.IsURL() {
			totalSize += max(file.Size, file.contentLength)
		}
		if totalSize > maxGeminiTotalFileBytes {
			return plan, FileRoutingError("file_input_too_large", "total file limit exceeded", 413)
		}
		plan.RequiresStorage = plan.RequiresStorage || file.Large || file.ExistingGCS
		category, _, _ := strings.Cut(file.MIMEType, "/")
		if slices.Contains([]string{"image", "audio", "video"}, category) && !slices.Contains(tags, category) {
			tags = append(tags, category)
		}
	}
	if plan.RequiresStorage && (root.Get("cachedContent").Exists() || root.Get("cached_content").Exists() || slices.ContainsFunc(plan.Files, func(file *GeminiPlannedFile) bool { return file.Special })) {
		return plan, FileRoutingError("unsupported_file_reference", "cachedContent, Files API and YouTube references cannot be moved to a different file channel", 400)
	}
	if old, ok := c.Get("multi_model_tags"); ok {
		if existing, ok := old.([]string); ok {
			for _, tag := range existing {
				if !slices.Contains(tags, tag) {
					tags = append(tags, tag)
				}
			}
		}
	}
	c.Set("multi_model_tags", tags)
	logger.LogInfo(c, fmt.Sprintf("Gemini file plan: attachments=%d storage=%t mode=%s", len(plan.Files), plan.RequiresStorage, config.Mode))
	return plan, nil
}

func (p *GeminiFilePlan) probe(ctx context.Context, file *GeminiPlannedFile) *types.NewAPIError {
	if file.Source.IsURL() {
		u, err := url.Parse(file.Source.GetRawData())
		if err != nil || u.User != nil || u.Host == "" {
			return FileRoutingError("invalid_file_input", file.Position+": invalid file URI", 400)
		}
		if u.Scheme == "gs" {
			if file.MIMEType == "" || strings.Trim(u.Path, "/") == "" || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
				return FileRoutingError("invalid_file_input", file.Position+": GCS URI requires an object and MIME type", 400)
			}
			if p.Config.Mode == "forward" {
				return FileRoutingError("file_access_denied", file.Position+": existing GCS references require authentication directly at the processing site", 403)
			}
			file.ExistingGCS = true
			return nil
		}
		if u.Scheme != "https" {
			return FileRoutingError("file_access_denied", file.Position+": only HTTPS file URLs are supported", 403)
		}
		host := strings.ToLower(u.Hostname())
		if slices.Contains([]string{"www.youtube.com", "youtube.com", "youtu.be"}, host) || (host == "generativelanguage.googleapis.com" && strings.HasPrefix(u.Path, "/v1beta/files/")) {
			file.Special = true
			p.DeveloperAPIOnly = p.DeveloperAPIOnly || host == "generativelanguage.googleapis.com"
			if host == "generativelanguage.googleapis.com" && file.MIMEType == "" {
				return FileRoutingError("invalid_file_input", file.Position+": Files API reference requires MIME type", 400)
			}
			if file.MIMEType == "" {
				file.MIMEType = "video/mp4"
			}
			file.resolved = &relayconvert.GeminiMedia{MIMEType: file.MIMEType, URI: file.Source.GetRawData()}
			return nil
		}
		probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		response, apiErr := p.download(probeCtx, file)
		if apiErr != nil {
			return apiErr
		}
		defer response.Body.Close()
		file.etag, file.contentLength = response.Header.Get("ETag"), response.ContentLength
		limit := p.Config.InlineThresholdBytes
		if p.probeBytes+limit > maxGeminiProbeBytes {
			return FileRoutingError("file_input_too_large", "attachment probe budget exceeded", 413)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, limit))
		if err != nil {
			return FileRoutingError("file_transfer_failed", file.Position+": failed to read file", 502)
		}
		p.probeBytes += int64(len(data))
		file.Size = int64(len(data))
		file.Large = file.Size >= limit
		file.prefixSize = file.Size
		file.prefixHash = sha256.Sum256(data)
		mimeType, err := geminiFileMIME(data, file.MIMEType, response.Header.Get("Content-Type"))
		if err != nil {
			return FileRoutingError("invalid_file_input", file.Position+": "+err.Error(), 400)
		}
		file.MIMEType = mimeType
		if !file.Large {
			file.resolved = &relayconvert.GeminiMedia{MIMEType: mimeType, Base64: base64.StdEncoding.EncodeToString(data)}
		}
		return nil
	}
	data := file.Source.GetRawData()
	if after, ok := strings.CutPrefix(data, "data:"); ok {
		header, payload, found := strings.Cut(after, ",")
		if !found || !strings.HasSuffix(header, ";base64") {
			return FileRoutingError("invalid_file_input", file.Position+": invalid data URI", 400)
		}
		mimeType := strings.TrimSuffix(header, ";base64")
		if file.MIMEType != "" && normalizeGeminiMIME(file.MIMEType) != normalizeGeminiMIME(mimeType) {
			return FileRoutingError("invalid_file_input", file.Position+": conflicting data URI MIME type", 400)
		}
		file.MIMEType = mimeType
		data = payload
	}
	reader := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data))
	prefix := make([]byte, 512)
	n, err := io.ReadFull(reader, prefix)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return FileRoutingError("invalid_file_input", file.Position+": invalid base64", 400)
	}
	// Validate the complete base64 payload without allocating a decoded copy.
	// Its byte count is diagnostic only: inline data never triggers URL routing
	// thresholds or remote-file transfer limits. Request-body limits still apply.
	size, err := io.Copy(io.Discard, reader)
	file.Size = int64(n) + size
	if err != nil {
		return FileRoutingError("invalid_file_input", file.Position+": invalid base64", 400)
	}
	mimeType, err := geminiFileMIME(prefix[:n], file.MIMEType, "")
	if err != nil {
		return FileRoutingError("invalid_file_input", file.Position+": "+err.Error(), 400)
	}
	file.MIMEType = mimeType
	file.resolved = &relayconvert.GeminiMedia{MIMEType: mimeType, Base64: data}
	return nil
}

func (p *GeminiFilePlan) download(ctx context.Context, file *GeminiPlannedFile) (*http.Response, *types.NewAPIError) {
	if err := ValidateSSRFProtectedFetchURL(file.Source.GetRawData()); err != nil {
		return nil, FileRoutingError("file_access_denied", file.Position+": file access denied", 403)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.Source.GetRawData(), nil)
	if err != nil {
		return nil, FileRoutingError("invalid_file_input", file.Position+": invalid file URL", 400)
	}
	response, err := p.client.Do(req)
	if err != nil {
		status := 502
		code := "file_transfer_failed"
		if ctx.Err() != nil {
			status, code = 504, "file_processing_timeout"
		}
		return nil, FileRoutingError(code, file.Position+": file download failed", status)
	}
	if response.StatusCode != http.StatusOK || response.ContentLength > maxGeminiFileBytes {
		response.Body.Close()
		if response.ContentLength > maxGeminiFileBytes {
			return nil, FileRoutingError("file_input_too_large", file.Position+": file limit exceeded", 413)
		}
		return nil, FileRoutingError("file_transfer_failed", file.Position+": file source returned an unsuccessful status", 502)
	}
	return response, nil
}

func geminiFileMIME(prefix []byte, declared, received string) (string, error) {
	if len(prefix) == 0 {
		return "", fmt.Errorf("empty file")
	}
	// Honor the supplied MIME type; content sniffing only fills in missing types.
	// The upstream provider validates whether the content matches that type.
	mimeType := normalizeGeminiMIME(declared)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = normalizeGeminiMIME(received)
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(prefix)
		if bytes.HasPrefix(prefix, []byte("fLaC")) {
			mimeType = "audio/flac"
		}
		if bytes.HasPrefix(prefix, []byte("OggS")) {
			mimeType = "audio/ogg"
		}
		if len(prefix) >= 2 && prefix[0] == 0xff && prefix[1]&0xe0 == 0xe0 {
			mimeType = "audio/mpeg"
		}
		if bytes.HasPrefix(prefix, []byte("%PDF-")) {
			mimeType = "application/pdf"
		}
		if len(prefix) >= 12 && string(prefix[4:8]) == "ftyp" {
			mimeType = "video/mp4"
			switch string(prefix[8:12]) {
			case "heic", "heix", "mif1":
				mimeType = "image/heic"
			case "M4A ":
				mimeType = "audio/mp4"
			case "qt  ":
				mimeType = "video/quicktime"
			}
		}
		mimeType = normalizeGeminiMIME(mimeType)
	}
	if mimeType == "application/octet-stream" || strings.HasPrefix(mimeType, "text/") {
		return "", fmt.Errorf("unsupported or unrecognized file format")
	}
	if !strings.HasPrefix(mimeType, "image/") && !strings.HasPrefix(mimeType, "audio/") && !strings.HasPrefix(mimeType, "video/") && mimeType != "application/pdf" {
		return "", fmt.Errorf("unsupported file format")
	}
	return mimeType, nil
}

func normalizeGeminiMIME(value string) string {
	value, _, _ = mime.ParseMediaType(value)
	switch value {
	case "audio/x-wav", "audio/wave":
		return "audio/wav"
	case "audio/m4a":
		return "audio/mp4"
	case "audio/mp3":
		return "audio/mpeg"
	case "image/jpg":
		return "image/jpeg"
	case "video/mov":
		return "video/quicktime"
	case "video/x-msvideo":
		return "video/avi"
	}
	return value
}

func (p *GeminiFilePlan) Close(c *gin.Context) {
	if p == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, object := range p.objects {
		object.remove(cleanupCtx, c)
	}
	clear(p.objects)
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	if p.release != nil {
		p.release()
		p.release = nil
	}
	p.Files = nil
}

// MediaForPosition refuses post-planning rewrites, rather than using a different
// attachment when content conversion merges or filters parts.
func (p *GeminiFilePlan) MediaForPosition(source types.FileSource, mi, pi int) (*relayconvert.GeminiMedia, error) {
	for _, file := range p.Files {
		if file.MessageIndex == mi && file.PartIndex == pi && file.Source.GetRawData() == source.GetRawData() {
			if file.resolved == nil {
				return nil, fmt.Errorf("file has not been prepared for the selected channel")
			}
			return file.resolved, nil
		}
	}
	return nil, fmt.Errorf("attachment changed after file routing")
}

// Duplicate keys can make the DTO decoder and the raw-body patcher disagree
// about which attachment is sent. Keep the file policy unambiguous.
// Go's DTO decoder matches field names without regard to case; the raw JSON
// planner must not overlook a differently cased field that the relay will use.
func hasNoncanonicalFileFields(value gjson.Result, fields ...string) bool {
	invalid := false
	value.ForEach(func(key, _ gjson.Result) bool {
		for _, field := range fields {
			if key.Str != field && strings.EqualFold(key.Str, field) {
				invalid = true
				return false
			}
		}
		return true
	})
	return invalid
}

func validGeminiFileJSON(value gjson.Result, depth int) bool {
	if depth > 100 {
		return false
	}
	if !value.IsObject() && !value.IsArray() {
		return true
	}
	seen := make(map[string]bool)
	valid := true
	value.ForEach(func(key, child gjson.Result) bool {
		if value.IsObject() {
			if seen[key.Str] {
				valid = false
				return false
			}
			seen[key.Str] = true
		}
		valid = validGeminiFileJSON(child, depth+1)
		return valid
	})
	return valid
}
