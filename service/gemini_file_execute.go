package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"google.golang.org/api/option"
)

type geminiUploadedObject struct {
	Credentials string
	Bucket      string
	Name        string
	Size        int64
	MIMEType    string
	Generation  int64
}

func (o geminiUploadedObject) remove(ctx context.Context, c *gin.Context) {
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(o.Credentials)))
	if err == nil {
		defer client.Close()
		object := client.Bucket(o.Bucket).Object(o.Name)
		if o.Generation > 0 {
			object = object.Generation(o.Generation)
		}
		err = object.Delete(ctx)
		if err != nil && err != storage.ErrObjectNotExist && ctx.Err() == nil {
			err = object.Delete(ctx)
		}
	}
	if err != nil && err != storage.ErrObjectNotExist {
		logger.LogWarn(c, "Gemini request object cleanup failed; bucket lifecycle cleanup is required")
	}
}

// PrepareGeminiFileAttempt runs only after the host's billing reservation and
// after selection of the actual channel key. Uploaded objects outlive attempts.
func PrepareGeminiFileAttempt(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	plan := GetGeminiFilePlan(c)
	if plan == nil {
		return nil
	}
	if plan.DeveloperAPIOnly && info.ChannelType != constant.ChannelTypeGemini {
		return FileRoutingError("unsupported_file_reference", "Files API references require a Gemini API channel", 400)
	}
	if info.ChannelType != constant.ChannelTypeGemini && info.ChannelType != constant.ChannelTypeVertexAi {
		return FileRoutingError("file_route_conflict", "selected channel cannot execute Gemini file plans", 400)
	}
	if !strings.HasPrefix(strings.TrimPrefix(info.UpstreamModelName, "models/"), "gemini-") {
		return FileRoutingError("unsupported_file_input", "selected model does not support Gemini file plans", 400)
	}
	// Forwarding uses the selected service credential exclusively; header
	// overrides must not substitute caller credentials or cookies afterwards.
	if plan.RequiresStorage && plan.Config.Mode == "forward" {
		if err := ValidateGeminiFileForwardingHeaders(c, info); err != nil {
			return err
		}
		for _, file := range plan.Files {
			if file.Source.IsURL() && (file.Large || file.ExistingGCS) {
				file.resolved = &relayconvert.GeminiMedia{MIMEType: file.MIMEType, URI: file.Source.GetRawData()}
			}
		}
		return nil
	}
	if !plan.RequiresStorage {
		return nil
	}
	if info.ChannelType != constant.ChannelTypeVertexAi || info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey || info.ChannelSetting.GoogleFileBucket == "" {
		return FileRoutingError("file_route_conflict", "GCS processing requires a Vertex service account and channel bucket", 400)
	}
	var credentials struct {
		Type      string `json:"type"`
		ProjectID string `json:"project_id"`
	}
	if common.UnmarshalJsonStr(info.ApiKey, &credentials) != nil || credentials.Type != "service_account" || credentials.ProjectID == "" {
		return FileRoutingError("file_route_conflict", "invalid Vertex service account for file upload", 400)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
	defer cancel()
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(info.ApiKey)))
	if err != nil {
		return FileRoutingError("file_transfer_failed", "could not initialize GCS upload", 502)
	}
	defer client.Close()
	bucket := info.ChannelSetting.GoogleFileBucket
	var total int64
	for _, file := range plan.Files {
		if !file.Source.IsURL() {
			continue
		}
		if file.ExistingGCS {
			u, _ := url.Parse(file.Source.GetRawData())
			if u.Host != bucket {
				return FileRoutingError("file_access_denied", file.Position+": GCS bucket is outside this channel", 403)
			}
			attrs, err := client.Bucket(bucket).Object(strings.TrimPrefix(u.Path, "/")).Attrs(ctx)
			if err != nil || attrs.Metadata["new_api_user_id"] != strconv.Itoa(info.UserId) {
				return FileRoutingError("file_access_denied", file.Position+": GCS object is not owned by the calling account", 403)
			}
			if attrs.Size <= 0 || attrs.Size > maxGeminiFileBytes {
				return FileRoutingError("file_input_too_large", file.Position+": GCS object is too large", 413)
			}
			if normalizeGeminiMIME(attrs.ContentType) != normalizeGeminiMIME(file.MIMEType) {
				return FileRoutingError("invalid_file_input", file.Position+": GCS MIME type mismatch", 400)
			}
			total += attrs.Size
			file.Size = attrs.Size
			file.resolved = &relayconvert.GeminiMedia{MIMEType: file.MIMEType, URI: file.Source.GetRawData()}
		} else if file.Large {
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(info.ApiKey+"\x00"+bucket+"\x00"+file.Source.GetRawData())))
			object, exists := plan.objects[key]
			if !exists {
				var uploadErr *types.NewAPIError
				object, uploadErr = plan.upload(ctx, c, client, info, file, maxGeminiTotalFileBytes-total)
				if uploadErr != nil {
					return uploadErr
				}
				plan.objects[key] = object
			}
			total += object.Size
			file.Size = object.Size
			file.resolved = &relayconvert.GeminiMedia{MIMEType: object.MIMEType, URI: "gs://" + object.Bucket + "/" + object.Name}
		} else {
			total += file.Size
		}
		if total > maxGeminiTotalFileBytes {
			return FileRoutingError("file_input_too_large", "total file limit exceeded", 413)
		}
	}
	return nil
}

func (p *GeminiFilePlan) upload(ctx context.Context, c *gin.Context, client *storage.Client, info *relaycommon.RelayInfo, file *GeminiPlannedFile, remaining int64) (geminiUploadedObject, *types.NewAPIError) {
	object := geminiUploadedObject{Credentials: info.ApiKey, Bucket: info.ChannelSetting.GoogleFileBucket, Name: "new-api/requests/" + common.GetUUID(), MIMEType: file.MIMEType}
	response, apiErr := p.download(ctx, file)
	if apiErr != nil {
		return object, apiErr
	}
	defer response.Body.Close()
	if (file.etag != "" && response.Header.Get("ETag") != file.etag) || (file.contentLength >= 0 && response.ContentLength != file.contentLength) {
		return object, FileRoutingError("invalid_file_input", file.Position+": file changed after planning", 400)
	}
	prefix, readErr := io.ReadAll(io.LimitReader(response.Body, file.prefixSize))
	if readErr != nil || sha256.Sum256(prefix) != file.prefixHash {
		return object, FileRoutingError("invalid_file_input", file.Position+": file changed after planning", 400)
	}
	reader := io.MultiReader(bytes.NewReader(prefix), response.Body)
	uploadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	writer := client.Bucket(object.Bucket).Object(object.Name).If(storage.Conditions{DoesNotExist: true}).NewWriter(uploadCtx)
	writer.ChunkSize = 1 << 20
	writer.ContentType = file.MIMEType
	writer.Metadata = map[string]string{"new_api_user_id": strconv.Itoa(info.UserId)}
	limit := min(maxGeminiFileBytes, max(remaining, 0))
	written, err := io.Copy(writer, io.LimitReader(reader, limit+1))
	if err != nil || written > limit {
		cancel()
		_ = writer.Close()
		if written > limit {
			return object, FileRoutingError("file_input_too_large", file.Position+": upload limit exceeded", 413)
		}
		return object, FileRoutingError("file_transfer_failed", file.Position+": upload failed", 502)
	}
	if err := writer.Close(); err != nil {
		// A failed commit acknowledgement can still leave an object behind.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		object.remove(cleanupCtx, c)
		cleanupCancel()
		return object, FileRoutingError("file_transfer_failed", file.Position+": upload commit failed", 502)
	}
	object.Size = written
	object.Generation = writer.Attrs().Generation
	logger.LogInfo(c, fmt.Sprintf("Gemini file uploaded: position=%s bytes=%d channel=%d", file.Position, written, info.ChannelId))
	return object, nil
}

func ApplyGeminiFilePlan(c *gin.Context, request *dto.GeminiChatRequest) error {
	plan := GetGeminiFilePlan(c)
	if plan == nil || !plan.Native {
		return nil
	}
	for _, file := range plan.Files {
		if file.MessageIndex >= len(request.Contents) || file.PartIndex >= len(request.Contents[file.MessageIndex].Parts) || file.resolved == nil {
			return fmt.Errorf("attachment changed after file routing")
		}
		part := &request.Contents[file.MessageIndex].Parts[file.PartIndex]
		var source string
		if part.InlineData != nil {
			source = part.InlineData.Data
		}
		if part.FileData != nil {
			source = part.FileData.FileUri
		}
		if source != file.Source.GetRawData() {
			return fmt.Errorf("attachment changed after file routing")
		}
		part.InlineData, part.FileData = nil, nil
		if file.resolved.URI != "" {
			part.FileData = &dto.GeminiFileData{FileUri: file.resolved.URI, MimeType: file.resolved.MIMEType}
		} else {
			part.InlineData = &dto.GeminiInlineData{Data: file.resolved.Base64, MimeType: file.resolved.MIMEType}
		}
	}
	return nil
}

// FinalizeGeminiFileBody also runs for body passthrough. The original native
// JSON supplies unmodeled fields; only the planned attachments are replaced.
func FinalizeGeminiFileBody(c *gin.Context, body []byte, beforeOverride []byte, passthrough bool) ([]byte, *types.NewAPIError) {
	plan := GetGeminiFilePlan(c)
	if plan == nil {
		return body, nil
	}
	if beforeOverride != nil && geminiAttachmentFingerprint(body) != geminiAttachmentFingerprint(beforeOverride) {
		return nil, FileRoutingError("file_route_conflict", "parameter overrides cannot change planned attachments", 400)
	}
	if passthrough && plan.Native {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, FileRoutingError("invalid_file_input", "cannot read native request body", 400)
		}
		body, err = storage.Bytes()
		if err != nil {
			return nil, FileRoutingError("invalid_file_input", "cannot read native request body", 400)
		}
		for _, file := range plan.Files {
			path := fmt.Sprintf("contents.%d.parts.%d", file.MessageIndex, file.PartIndex)
			for _, key := range []string{"inlineData", "inline_data", "fileData", "file_data"} {
				body, err = sjson.DeleteBytes(body, path+"."+key)
				if err != nil {
					break
				}
			}
			if err == nil && file.resolved != nil {
				if file.resolved.URI != "" {
					body, err = sjson.SetBytes(body, path+".fileData", map[string]string{"mimeType": file.resolved.MIMEType, "fileUri": file.resolved.URI})
				} else {
					body, err = sjson.SetBytes(body, path+".inlineData", map[string]string{"mimeType": file.resolved.MIMEType, "data": file.resolved.Base64})
				}
			}
			if err != nil {
				return nil, FileRoutingError("invalid_file_input", "could not prepare native attachment", 400)
			}
		}
	}
	if int64(len(body)) > plan.Config.MaxInlineRequestBytes {
		return nil, FileRoutingError("file_input_too_large", "serialized upstream request exceeds configured size limit", 413)
	}
	return body, nil
}

func geminiAttachmentFingerprint(body []byte) [32]byte {
	h := sha256.New()
	for mi, message := range gjson.GetBytes(body, "contents").Array() {
		for pi, part := range message.Get("parts").Array() {
			for _, key := range []string{"inlineData", "inline_data", "fileData", "file_data", "functionResponse.parts", "function_response.parts", "videoMetadata", "video_metadata", "mediaResolution", "media_resolution"} {
				if value := part.Get(key); value.Exists() {
					fmt.Fprintf(h, "%d:%d:%s:", mi, pi, key)
					hashGeminiJSONValue(h, value)
				}
			}
		}
	}
	for _, key := range []string{"systemInstruction", "system_instruction", "cachedContent", "cached_content", "requests"} {
		fmt.Fprintf(h, "root:%s:", key)
		hashGeminiJSONValue(h, gjson.GetBytes(body, key))
	}
	return [32]byte(h.Sum(nil))
}

// Hash decoded strings and sorted object keys so harmless JSON reformatting
// by parameter overrides does not look like an attachment change. Large
// base64 strings are hashed in bounded chunks without another complete copy.
func hashGeminiJSONValue(h hash.Hash, value gjson.Result) {
	switch {
	case value.IsObject():
		fields := value.Map()
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		fmt.Fprintf(h, "object:%d:", len(keys))
		for _, key := range keys {
			fmt.Fprintf(h, "key:%q:", key)
			hashGeminiJSONValue(h, fields[key])
		}
	case value.IsArray():
		values := value.Array()
		fmt.Fprintf(h, "array:%d:", len(values))
		for _, child := range values {
			hashGeminiJSONValue(h, child)
		}
	case value.Type == gjson.String:
		data := value.Str
		fmt.Fprintf(h, "string:%d:", len(data))
		for len(data) > 0 {
			size := min(len(data), 32<<10)
			_, _ = h.Write([]byte(data[:size]))
			data = data[size:]
		}
	default:
		fmt.Fprintf(h, "scalar:%s:", value.Raw)
	}
}

func RedactGeminiFileRequest(c *gin.Context, body string) string {
	plan := GetGeminiFilePlan(c)
	if plan == nil {
		return body
	}
	// Metadata only: this also covers error paths before a body is converted.
	files := make([]map[string]any, 0, len(plan.Files))
	for _, file := range plan.Files {
		files = append(files, map[string]any{"position": file.Position, "mime_type": file.MIMEType, "bytes_observed": file.Size, "large": file.Large})
	}
	data, _ := common.Marshal(map[string]any{"file_routing": plan.Config.Mode, "attachments": files})
	return string(data)
}

// GeminiFileUpstreamError discards upstream bodies that can echo signed URLs,
// inline data or account identifiers. Status still distinguishes provider failures.
func GeminiFileUpstreamError(response *http.Response) *types.NewAPIError {
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	return FileRoutingError("file_inference_failed", "upstream rejected the Gemini file request", response.StatusCode)
}

func ValidateGeminiFileForwardingHeaders(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	plan := GetGeminiFilePlan(c)
	if plan != nil && plan.RequiresStorage && plan.Config.Mode == "forward" && (len(info.HeadersOverride) > 0 || len(common.GetContextKeyStringMap(c, constant.ContextKeyChannelHeaderOverride)) > 0) {
		return FileRoutingError("file_route_conflict", "file forwarding does not support channel header overrides", 400)
	}
	return nil
}
