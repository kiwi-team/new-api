package worldlabs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	ChannelName       = "worldlabs"
	DefaultBaseURL    = "https://api.worldlabs.ai"
	GenerateEndpoint  = "/marble/v1/worlds:generate"
	OperationEndpoint = "/marble/v1/operations"
)

var ModelList = []string{
	"Marble 0.1-mini",
	"Marble 0.1-plus",
}

// ============================
// Request structures
// ============================

// ImagePromptSource represents the source type for image prompt
// Can be "uri" or "data_base64"
type ImagePromptSource string

const (
	ImagePromptSourceURI    ImagePromptSource = "uri"
	ImagePromptSourceBase64 ImagePromptSource = "data_base64"
)

// ImagePrompt represents the image_prompt field which can be either URI or Base64
// URI format: {"uri": "xxx", "source": "uri"}
// Base64 format: {"data_base64": "xxx", "extension": "png", "source": "data_base64"}
type ImagePrompt struct {
	// For URI source
	URI string `json:"uri,omitempty"`

	// For Base64 source
	DataBase64 string `json:"data_base64,omitempty"`
	Extension  string `json:"extension,omitempty"`

	// Source type: "uri" or "data_base64"
	Source ImagePromptSource `json:"source"`
}

type MultiImagePromptItem struct {
	Content *ImagePrompt `json:"content,omitempty"`
	Azimuth *float32     `json:"azimuth,omitempty"`
}

// NewImagePromptFromURI creates an ImagePrompt from a URI
func NewImagePromptFromURI(uri string) *ImagePrompt {
	return &ImagePrompt{
		URI:    uri,
		Source: ImagePromptSourceURI,
	}
}

// NewImagePromptFromBase64 creates an ImagePrompt from base64 data
func NewImagePromptFromBase64(data, extension string) *ImagePrompt {
	return &ImagePrompt{
		DataBase64: data,
		Extension:  extension,
		Source:     ImagePromptSourceBase64,
	}
}

// WorldPrompt is a union type for different prompt types
// The API uses discriminated union pattern: type field determines the structure
type WorldPrompt struct {
	Type string `json:"type"` // "text", "image", "multi_image", "video"

	// Common fields
	TextPrompt       string `json:"text_prompt,omitempty"`
	DisableRecaption bool   `json:"disable_recaption,omitempty"`
	IsPano           bool   `json:"is_pano,omitempty"`

	// For image prompt (type="image")
	ImagePrompt *ImagePrompt `json:"image_prompt,omitempty"`

	// For multi-image prompt (type="multi_image")
	MultiImagePrompt  *[]MultiImagePromptItem `json:"multi_image_prompt,omitempty"`
	ReconstructImages bool                    `json:"reconstruct_images,omitempty"`

	// For video prompt (type="video")
	VideoPrompt *VideoPromptInput `json:"video_prompt,omitempty"`
}

// VideoPromptInput represents the video input for world generation
type VideoPromptInput struct {
	URI        string            `json:"uri,omitempty"`
	DataBase64 string            `json:"data_base64,omitempty"`
	Extension  string            `json:"extension,omitempty"`
	Source     ImagePromptSource `json:"source"`
}

type Permission struct {
	AllowedReaders []string `json:"allowed_readers,omitempty"`
	AllowedWriters []string `json:"allowed_writers,omitempty"`
	Public         bool     `json:"public"`
}

type GenerateWorldRequest struct {
	WorldPrompt WorldPrompt `json:"world_prompt"`
	DisplayName string      `json:"display_name,omitempty"`
	Model       string      `json:"model,omitempty"`
	Permission  *Permission `json:"permission,omitempty"`
	Seed        *int        `json:"seed,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
}

// ============================
// Response structures
// ============================

type OperationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ProgressInfo struct {
	Status      string `json:"status"`
	Description string `json:"description"`
}

type OperationMetadata struct {
	Progress *ProgressInfo `json:"progress,omitempty"`
	WorldID  string        `json:"world_id,omitempty"`
}

type MeshAssets struct {
	ColliderMeshUrl string `json:"collider_mesh_url,omitempty"`
}

type ImageryAssets struct {
	PanoUrl string `json:"pano_url,omitempty"`
}

type SpzUrls struct {
	Url500k string `json:"500k,omitempty"`
	Url100k string `json:"100k,omitempty"`
	FullRes string `json:"full_res,omitempty"`
}

type SplatAssets struct {
	SpzUrls *SpzUrls `json:"spz_urls,omitempty"`
}

type WorldAssets struct {
	Mesh         *MeshAssets    `json:"mesh,omitempty"`
	Imagery      *ImageryAssets `json:"imagery,omitempty"`
	Splats       *SplatAssets   `json:"splats,omitempty"`
	ThumbnailUrl string         `json:"thumbnail_url,omitempty"`
	Caption      string         `json:"caption,omitempty"`
}

type WorldResponse struct {
	WorldID        string       `json:"world_id"`
	DisplayName    string       `json:"display_name,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
	Assets         *WorldAssets `json:"assets,omitempty"`
	CreatedAt      string       `json:"created_at,omitempty"`
	UpdatedAt      string       `json:"updated_at,omitempty"`
	WorldMarbleURL string       `json:"world_marble_url,omitempty"`
}

type GenerateWorldResponse struct {
	Done        bool               `json:"done"`
	OperationID string             `json:"operation_id"`
	CreatedAt   string             `json:"created_at,omitempty"`
	Error       *OperationError    `json:"error,omitempty"`
	ExpiresAt   string             `json:"expires_at,omitempty"`
	Metadata    *OperationMetadata `json:"metadata,omitempty"`
	Response    *WorldResponse     `json:"response,omitempty"`
	UpdatedAt   string             `json:"updated_at,omitempty"`
}

// ============================
// TaskAdaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	if a.baseURL == "" {
		a.baseURL = DefaultBaseURL
	}
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
		return err
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	// Determine action based on input
	action := constant.TaskActionTextGenerate
	if req.HasImage() {
		if len(req.Images) > 1 {
			action = constant.TaskActionMultiImageGenerate
		} else if len(req.Images) == 1 || len(req.Image) > 0 {
			action = constant.TaskActionImageGenerate
		} else {
			action = constant.TaskActionGenerate
		}
	}
	info.Action = action

	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s%s", a.baseURL, GenerateEndpoint), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("WLT-Api-Key", a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var wResp GenerateWorldResponse
	if err := json.Unmarshal(responseBody, &wResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if wResp.Error != nil {
		taskErr = service.TaskErrorWrapperLocal(
			fmt.Errorf("worldlabs api error: %s", wResp.Error.Message),
			fmt.Sprintf("%d", wResp.Error.Code),
			http.StatusBadRequest,
		)
		return
	}

	if wResp.OperationID == "" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("operation_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = wResp.OperationID
	ov.TaskID = wResp.OperationID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	if wResp.Done {
		ov.Status = dto.VideoStatusCompleted
		ov.Progress = 100
	} else {
		ov.Status = dto.VideoStatusQueued
		ov.Progress = 0
	}

	c.JSON(http.StatusOK, ov)
	return wResp.OperationID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s%s/%s", baseUrl, OperationEndpoint, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("WLT-Api-Key", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var opResp GenerateWorldResponse
	if err := json.Unmarshal(respBody, &opResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskInfo := &relaycommon.TaskInfo{
		Code: 0,
	}

	if opResp.Error != nil {
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Reason = opResp.Error.Message
		taskInfo.Progress = "100%"
		return taskInfo, nil
	}

	if opResp.Done {
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Progress = "100%"

		// Extract URLs from response
		if opResp.Response != nil {
			taskInfo.WorldMarbleUrl = opResp.Response.WorldMarbleURL
			taskInfo.Url = opResp.Response.WorldMarbleURL

			if opResp.Response.Assets != nil {
				// Mesh URL
				if opResp.Response.Assets.Mesh != nil {
					taskInfo.ColliderMeshUrl = opResp.Response.Assets.Mesh.ColliderMeshUrl
				}
				// Splat URLs
				if opResp.Response.Assets.Splats != nil && opResp.Response.Assets.Splats.SpzUrls != nil {
					taskInfo.SplatUrl500k = opResp.Response.Assets.Splats.SpzUrls.Url500k
					taskInfo.SplatUrlFullRes = opResp.Response.Assets.Splats.SpzUrls.FullRes
				}
			}
		}
	} else {
		taskInfo.Status = model.TaskStatusInProgress
		// Extract progress from metadata
		if opResp.Metadata != nil && opResp.Metadata.Progress != nil {
			switch opResp.Metadata.Progress.Status {
			case "SUCCEEDED":
				taskInfo.Status = model.TaskStatusSuccess
				taskInfo.Progress = "100%"
			case "FAILED":
				taskInfo.Status = model.TaskStatusFailure
				taskInfo.Progress = "100%"
				taskInfo.Reason = opResp.Metadata.Progress.Description
			default:
				taskInfo.Progress = "50%"
			}
		} else {
			taskInfo.Progress = "50%"
		}
	}

	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var wResp GenerateWorldResponse
	if err := json.Unmarshal(originTask.Data, &wResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal worldlabs task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	// Extract URLs from response
	if wResp.Response != nil {
		openAIVideo.SetMetadata("world_marble_url", wResp.Response.WorldMarbleURL)
		openAIVideo.SetMetadata("world_id", wResp.Response.WorldID)
		openAIVideo.SetMetadata("url", wResp.Response.WorldMarbleURL)

		if wResp.Response.Assets != nil {
			if wResp.Response.Assets.Mesh != nil {
				openAIVideo.SetMetadata("collider_mesh_url", wResp.Response.Assets.Mesh.ColliderMeshUrl)
			}
			if wResp.Response.Assets.Splats != nil && wResp.Response.Assets.Splats.SpzUrls != nil {
				openAIVideo.SetMetadata("splat_url_500k", wResp.Response.Assets.Splats.SpzUrls.Url500k)
				openAIVideo.SetMetadata("splat_url_full_res", wResp.Response.Assets.Splats.SpzUrls.FullRes)
			}
		}
	}

	if wResp.Error != nil {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: wResp.Error.Message,
			Code:    fmt.Sprintf("%d", wResp.Error.Code),
		}
	}

	jsonData, err := common.Marshal(openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "marshal openai video failed")
	}

	return jsonData, nil
}

// ============================
// Helper functions
// ============================

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*GenerateWorldRequest, error) {
	worldReq := &GenerateWorldRequest{
		Model: defaultString(req.Model, "Marble 0.1-plus"),
	}

	// Build world_prompt based on input type
	switch info.Action {
	case constant.TaskActionMultiImageGenerate:
		// Multi-image to world
		var imagePrompts []MultiImagePromptItem
		for _, imgURL := range req.Images {
			imagePrompts = append(imagePrompts, MultiImagePromptItem{Content: createImagePrompt(imgURL)})
		}
		worldReq.WorldPrompt = WorldPrompt{
			Type:             "multi-image",
			MultiImagePrompt: &imagePrompts,
			TextPrompt:       req.Prompt,
		}
	case constant.TaskActionImageGenerate:
		// Single image to world
		imageURL := req.Image
		if imageURL == "" && len(req.Images) > 0 {
			imageURL = req.Images[0]
		}
		worldReq.WorldPrompt = WorldPrompt{
			Type:        "image",
			ImagePrompt: createImagePrompt(imageURL),
			TextPrompt:  req.Prompt,
		}
	default:
		// Text to world
		worldReq.WorldPrompt = WorldPrompt{
			Type:       "text",
			TextPrompt: req.Prompt,
		}
	}

	// Apply metadata overrides
	if req.Metadata != nil {
		if displayName, ok := req.Metadata["display_name"].(string); ok {
			worldReq.DisplayName = displayName
		}
		if tags, ok := req.Metadata["tags"].([]interface{}); ok {
			for _, t := range tags {
				if tagStr, ok := t.(string); ok {
					worldReq.Tags = append(worldReq.Tags, tagStr)
				}
			}
		}
		if seed, ok := req.Metadata["seed"].(float64); ok {
			seedInt := int(seed)
			worldReq.Seed = &seedInt
		}
		if disableRecaption, ok := req.Metadata["disable_recaption"].(bool); ok {
			worldReq.WorldPrompt.DisableRecaption = disableRecaption
		}
		if isPano, ok := req.Metadata["is_pano"].(bool); ok {
			worldReq.WorldPrompt.IsPano = isPano
		}
		// Handle permission from metadata
		if permission, ok := req.Metadata["permission"].(map[string]interface{}); ok {
			worldReq.Permission = &Permission{}
			if public, ok := permission["public"].(bool); ok {
				worldReq.Permission.Public = public
			}
			if readers, ok := permission["allowed_readers"].([]interface{}); ok {
				for _, r := range readers {
					if rStr, ok := r.(string); ok {
						worldReq.Permission.AllowedReaders = append(worldReq.Permission.AllowedReaders, rStr)
					}
				}
			}
			if writers, ok := permission["allowed_writers"].([]interface{}); ok {
				for _, w := range writers {
					if wStr, ok := w.(string); ok {
						worldReq.Permission.AllowedWriters = append(worldReq.Permission.AllowedWriters, wStr)
					}
				}
			}
		}
	}

	return worldReq, nil
}

// createImagePrompt creates an ImagePrompt from a string
// If the string starts with "data:" or looks like base64, it creates a base64 prompt
// Otherwise, it creates a URI prompt
func createImagePrompt(input string) *ImagePrompt {
	// Check if it's a base64 data URL (data:image/png;base64,xxx)
	if len(input) > 5 && input[:5] == "data:" {
		// Parse data URL: data:image/png;base64,xxx
		// Find the comma that separates metadata from data
		commaIdx := -1
		for i, c := range input {
			if c == ',' {
				commaIdx = i
				break
			}
		}
		if commaIdx > 0 {
			metadata := input[5:commaIdx] // e.g., "image/png;base64"
			data := input[commaIdx+1:]

			// Extract extension from mime type
			extension := "png" // default
			if len(metadata) > 6 && metadata[:6] == "image/" {
				// Find the end of mime type (before ;base64)
				endIdx := len(metadata)
				for i, c := range metadata {
					if c == ';' {
						endIdx = i
						break
					}
				}
				extension = metadata[6:endIdx]
			}

			return NewImagePromptFromBase64(data, extension)
		}
	}

	// Default to URI
	return NewImagePromptFromURI(input)
}

func defaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
