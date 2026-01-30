package fal_sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Adaptor implements the channel.Adaptor interface for FAL Sync channel
type Adaptor struct{}

// Init initializes the adaptor with relay info
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// No initialization needed for now
}

// GetChannelName returns the channel name
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// GetModelList returns the list of supported models
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetRequestURL constructs the FAL API submit URL
// Model name mapping:
// - flux-2-pro -> fal-ai/flux-2-pro/edit
// - hunyuan-image-v3 -> fal-ai/hunyuan-image/v3/instruct/edit
// - qwen-image-max -> fal-ai/qwen-image-edit-2511
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// Get base URL from channel configuration
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = "https://queue.fal.run"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Get model name
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = DefaultModel
	}

	// Map model name to FAL API endpoint
	var endpoint string
	switch {
	case strings.Contains(modelName, "hunyuan"):
		endpoint = "fal-ai/hunyuan-image/v3/instruct/edit"
	case strings.Contains(modelName, "qwen"):
		endpoint = "fal-ai/qwen-image-edit-2511"
	case strings.Contains(modelName, "flux"):
		endpoint = "fal-ai/flux-2-pro/edit"
	default:
		// For unknown models, try to use the model name directly
		endpoint = fmt.Sprintf("fal-ai/%s/edit", modelName)
	}

	return fmt.Sprintf("%s/%s", baseURL, endpoint), nil
}

// SetupRequestHeader sets up the request headers for FAL API
// Implements Requirements 5.1, 5.2, 5.3
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// Call common header setup first
	channel.SetupApiRequestHeader(info, c, req)

	// Set Authorization header with "Key {api_key}" format - Requirements 5.1, 5.2
	// Note: FAL uses "Key" prefix, not "Bearer"
	req.Set("Authorization", "Key "+info.ApiKey)

	// Set Content-Type to application/json - Requirement 5.3
	req.Set("Content-Type", "application/json")

	return nil
}

// ConvertImageRequest converts OpenAI image request to FAL format
// Supports both image (string) and images ([]string) parameters
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// Validate prompt
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, errors.New("fal_sync adaptor: prompt is required")
	}

	// Determine model name and store in info for later use
	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(request.Model)
	}
	if modelName == "" {
		modelName = DefaultModel
	}
	info.UpstreamModelName = modelName

	// Parse extra_fields
	var extraFields map[string]any
	if len(request.ExtraFields) > 0 {
		if err := common.Unmarshal(request.ExtraFields, &extraFields); err != nil {
			return nil, fmt.Errorf("fal_sync adaptor: failed to decode extra_fields: %w", err)
		}
	}

	// Parse Extra map fields
	var extraMap map[string]any
	if len(request.Extra) > 0 {
		extraMap = make(map[string]any)
		for key, raw := range request.Extra {
			if raw == nil {
				continue
			}
			var val any
			if err := common.Unmarshal(raw, &val); err != nil {
				return nil, fmt.Errorf("fal_sync adaptor: failed to decode extra field %s: %w", key, err)
			}
			extraMap[key] = val
		}
	}

	// Extract image URLs from request (supports both image and images fields)
	imageURLs, err := request.GetImageURLs()
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to get image URLs: %w", err)
	}

	// Extract size field
	imageSize := strings.TrimSpace(request.Size)

	// Build model-specific request using typed structs
	return buildModelRequest(modelName, prompt, imageURLs, imageSize, extraFields, extraMap)
}

// buildModelRequest creates the appropriate typed request struct based on model name
func buildModelRequest(modelName, prompt string, imageURLs []string, imageSize string, extraFields, extraMap map[string]any) (any, error) {
	// Determine model type and build appropriate request
	switch {
	case isFlux2ProModel(modelName):
		return buildFlux2ProRequest(prompt, imageURLs, imageSize, extraFields, extraMap), nil
	case isHunyuanImageV3Model(modelName):
		return buildHunyuanImageV3Request(prompt, imageURLs, imageSize, extraFields, extraMap), nil
	case isQwenImageMaxModel(modelName):
		return buildQwenImageMaxRequest(prompt, imageURLs, imageSize, extraFields, extraMap), nil
	default:
		// Use generic request for unknown models
		return buildGenericFALRequest(prompt, imageURLs, imageSize, extraFields, extraMap), nil
	}
}

// isFlux2ProModel checks if the model is flux-2-pro
func isFlux2ProModel(model string) bool {
	return strings.Contains(model, "flux")
}

// isHunyuanImageV3Model checks if the model is hunyuan-image-v3
func isHunyuanImageV3Model(model string) bool {
	return strings.Contains(model, "hunyuan")
}

// isQwenImageMaxModel checks if the model is qwen-image-max
func isQwenImageMaxModel(model string) bool {
	return strings.Contains(model, "qwen")
}

// buildFlux2ProRequest builds a Flux2ProRequest for flux-2-pro model
func buildFlux2ProRequest(prompt string, imageURLs []string, imageSize string, extraFields, extraMap map[string]any) *Flux2ProRequest {
	req := &Flux2ProRequest{
		Prompt:    prompt,
		ImageURLs: imageURLs,
	}

	if imageSize != "" {
		req.ImageSize = imageSize
	}

	// Apply extra_fields
	applyFlux2ProExtraFields(req, extraFields)
	applyFlux2ProExtraFields(req, extraMap)

	return req
}

// applyFlux2ProExtraFields applies extra fields to Flux2ProRequest
func applyFlux2ProExtraFields(req *Flux2ProRequest, extra map[string]any) {
	if extra == nil {
		return
	}
	for key, val := range extra {
		if strings.HasPrefix(key, "_") {
			continue // Skip internal fields
		}
		switch key {
		case "seed":
			if v, ok := toInt64(val); ok {
				req.Seed = v
			}
		case "safety_tolerance":
			if v, ok := toInt(val); ok {
				req.SafetyTolerance = v
			}
		case "output_format":
			if v, ok := val.(string); ok {
				req.OutputFormat = v
			}
		case "num_images":
			if v, ok := toInt(val); ok {
				req.NumImages = v
			}
		case "image_size":
			if v, ok := val.(string); ok {
				req.ImageSize = v
			}
		}
	}
}

// buildHunyuanImageV3Request builds a HunyuanImageV3Request for hunyuan-image-v3 model
func buildHunyuanImageV3Request(prompt string, imageURLs []string, imageSize string, extraFields, extraMap map[string]any) *HunyuanImageV3Request {
	req := &HunyuanImageV3Request{
		Prompt:    prompt,
		ImageURLs: imageURLs,
	}

	if imageSize != "" {
		req.ImageSize = imageSize
	}

	// Apply extra_fields
	applyHunyuanImageV3ExtraFields(req, extraFields)
	applyHunyuanImageV3ExtraFields(req, extraMap)

	return req
}

// applyHunyuanImageV3ExtraFields applies extra fields to HunyuanImageV3Request
func applyHunyuanImageV3ExtraFields(req *HunyuanImageV3Request, extra map[string]any) {
	if extra == nil {
		return
	}
	for key, val := range extra {
		if strings.HasPrefix(key, "_") {
			continue // Skip internal fields
		}
		switch key {
		case "seed":
			if v, ok := toInt64(val); ok {
				req.Seed = v
			}
		case "output_format":
			if v, ok := val.(string); ok {
				req.OutputFormat = v
			}
		case "image_size":
			if v, ok := val.(string); ok {
				req.ImageSize = v
			}
		}
	}
}

// buildQwenImageMaxRequest builds a QwenImageMaxRequest for qwen-image-max model
func buildQwenImageMaxRequest(prompt string, imageURLs []string, imageSize string, extraFields, extraMap map[string]any) *QwenImageMaxRequest {
	req := &QwenImageMaxRequest{
		Prompt:    prompt,
		ImageURLs: imageURLs,
	}

	if imageSize != "" {
		req.ImageSize = imageSize
	}

	// Apply extra_fields
	applyQwenImageMaxExtraFields(req, extraFields)
	applyQwenImageMaxExtraFields(req, extraMap)

	return req
}

// applyQwenImageMaxExtraFields applies extra fields to QwenImageMaxRequest
func applyQwenImageMaxExtraFields(req *QwenImageMaxRequest, extra map[string]any) {
	if extra == nil {
		return
	}
	for key, val := range extra {
		if strings.HasPrefix(key, "_") {
			continue // Skip internal fields
		}
		switch key {
		case "seed":
			if v, ok := toInt64(val); ok {
				req.Seed = v
			}
		case "negative_prompt":
			if v, ok := val.(string); ok {
				req.NegativePrompt = v
			}
		case "acceleration":
			if v, ok := val.(string); ok {
				req.Acceleration = v
			}
		case "output_format":
			if v, ok := val.(string); ok {
				req.OutputFormat = v
			}
		case "image_size":
			if v, ok := val.(string); ok {
				req.ImageSize = v
			}
		}
	}
}

// buildGenericFALRequest builds a GenericFALRequest for unknown models
func buildGenericFALRequest(prompt string, imageURLs []string, imageSize string, extraFields, extraMap map[string]any) GenericFALRequest {
	req := make(GenericFALRequest)
	req["prompt"] = prompt

	// All models use image_urls (array format)
	if len(imageURLs) > 0 {
		req["image_urls"] = imageURLs
	}

	if imageSize != "" {
		req["image_size"] = imageSize
	}

	// Apply extra_fields (skip internal fields)
	for key, val := range extraFields {
		if !strings.HasPrefix(key, "_") {
			req[key] = val
		}
	}
	for key, val := range extraMap {
		if !strings.HasPrefix(key, "_") {
			req[key] = val
		}
	}

	return req
}

// toInt64 converts various numeric types to int64
func toInt64(val any) (int64, bool) {
	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	}
	return 0, false
}

// toInt converts various numeric types to int
func toInt(val any) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	}
	return 0, false
}

// DoRequest submits the request to FAL and handles async polling
// Implements Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 6.4
// Returns a synthetic *http.Response containing the FAL result for DoResponse to process
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// Get HTTP client (with proxy support if configured)
	client, err := getHTTPClient(info)
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to create HTTP client: %w", err)
	}

	// Step 1: Submit request to FAL and get request_id - Requirement 3.1
	submitURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to get request URL: %w", err)
	}

	// Read request body for submission
	bodyBytes, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to read request body: %w", err)
	}

	queueStatus, err := a.submitRequest(c, client, submitURL, bodyBytes, info)
	if err != nil {
		return nil, err
	}

	var resultBody []byte

	// If already completed, fetch result immediately
	if queueStatus.Status == "COMPLETED" {
		resultBody, err = a.fetchResultWithRetry(c, client, info, queueStatus.RequestID)
		if err != nil {
			return nil, err
		}
	} else {
		// Step 2: Poll status endpoint until COMPLETED or timeout - Requirements 3.2, 3.4, 3.5
		resultBody, err = a.pollUntilComplete(c, client, info, queueStatus.RequestID)
		if err != nil {
			return nil, err
		}
	}

	// Return a synthetic *http.Response containing the FAL result
	// This allows the image handler to process it normally
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBuffer(resultBody)),
		Header:     make(http.Header),
	}, nil
}

// submitRequest submits the initial request to FAL and returns the queue status
func (a *Adaptor) submitRequest(c *gin.Context, client *http.Client, submitURL string, bodyBytes []byte, info *relaycommon.RelayInfo) (*FALQueueStatus, error) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, submitURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to create submit request: %w", err)
	}

	// Set headers
	headers := req.Header
	if err := a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to setup request headers: %w", err)
	}

	// Submit with retry logic - Requirement 6.4
	var resp *http.Response
	var lastErr error
	for retry := 0; retry <= MaxRetries; retry++ {
		if retry > 0 {
			// Wait before retry
			select {
			case <-c.Request.Context().Done():
				return nil, fmt.Errorf("fal_sync adaptor: request cancelled during submit retry")
			case <-time.After(time.Second):
			}
			// Recreate request for retry
			req, _ = http.NewRequestWithContext(c.Request.Context(), http.MethodPost, submitURL, bytes.NewReader(bodyBytes))
			headers = req.Header
			_ = a.SetupRequestHeader(c, &headers, info)
		}

		resp, lastErr = client.Do(req)
		if lastErr == nil {
			break
		}
		// Check if error is retryable (network errors)
		if !isRetryableError(lastErr) {
			return nil, fmt.Errorf("fal_sync adaptor: submit request failed: %w", lastErr)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("fal_sync adaptor: submit request failed after %d retries: %w", MaxRetries, lastErr)
	}
	defer resp.Body.Close()

	// Check for non-2xx status - Requirement 6.1
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseErrorResponse(resp, "submit")
	}

	// Parse queue status response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to read submit response: %w", err)
	}

	var queueStatus FALQueueStatus
	if err := json.Unmarshal(respBody, &queueStatus); err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to parse queue status: %w, body: %s", err, string(respBody))
	}

	if queueStatus.RequestID == "" {
		return nil, fmt.Errorf("fal_sync adaptor: missing request_id in queue status response")
	}

	return &queueStatus, nil
}

// pollUntilComplete polls the status endpoint until the request is completed or times out
func (a *Adaptor) pollUntilComplete(c *gin.Context, client *http.Client, info *relaycommon.RelayInfo, requestID string) ([]byte, error) {
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = "https://queue.fal.run"
	}
	modelName := info.UpstreamModelName

	statusURL := buildStatusURL(baseURL, modelName, requestID)
	timeout := time.After(DefaultTimeout)
	ticker := time.NewTicker(DefaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return nil, fmt.Errorf("fal_sync adaptor: request cancelled during polling")
		case <-timeout:
			// Requirement 6.3: Return timeout error with request_id
			return nil, fmt.Errorf("fal_sync adaptor: polling timeout after %v, request_id: %s", DefaultTimeout, requestID)
		case <-ticker.C:
			status, err := a.pollStatusWithRetry(c, client, statusURL, info.ApiKey)
			if err != nil {
				return nil, err
			}

			switch status.Status {
			case "COMPLETED":
				// Step 3: Fetch result - Requirement 3.3
				return a.fetchResultWithRetry(c, client, info, requestID)
			case "FAILED":
				// Requirement 3.6: Return FAL error
				return nil, fmt.Errorf("fal_sync adaptor: FAL request failed, request_id: %s", requestID)
			case "IN_QUEUE", "IN_PROGRESS":
				// Continue polling
				continue
			default:
				// Unknown status, continue polling
				continue
			}
		}
	}
}

// pollStatusWithRetry polls the status endpoint with retry logic
func (a *Adaptor) pollStatusWithRetry(c *gin.Context, client *http.Client, statusURL, apiKey string) (*FALQueueStatus, error) {
	var lastErr error
	for retry := 0; retry <= MaxRetries; retry++ {
		if retry > 0 {
			select {
			case <-c.Request.Context().Done():
				return nil, fmt.Errorf("fal_sync adaptor: request cancelled during status poll retry")
			case <-time.After(time.Second):
			}
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, statusURL, nil)
		if err != nil {
			return nil, fmt.Errorf("fal_sync adaptor: failed to create status request: %w", err)
		}
		req.Header.Set("Authorization", "Key "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableError(err) {
				return nil, fmt.Errorf("fal_sync adaptor: status poll failed: %w", err)
			}
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Try to parse error
			var errResp FALErrorResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Detail != "" {
				return nil, fmt.Errorf("fal_sync adaptor: status poll error: %s", errResp.Detail)
			}
			return nil, fmt.Errorf("fal_sync adaptor: status poll failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		var status FALQueueStatus
		if err := json.Unmarshal(respBody, &status); err != nil {
			lastErr = err
			continue
		}

		return &status, nil
	}

	return nil, fmt.Errorf("fal_sync adaptor: status poll failed after %d retries: %w", MaxRetries, lastErr)
}

// fetchResultWithRetry fetches the final result with retry logic
func (a *Adaptor) fetchResultWithRetry(c *gin.Context, client *http.Client, info *relaycommon.RelayInfo, requestID string) ([]byte, error) {
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = "https://queue.fal.run"
	}
	modelName := info.UpstreamModelName
	resultURL := buildResultURL(baseURL, modelName, requestID)

	var lastErr error
	for retry := 0; retry <= MaxRetries; retry++ {
		if retry > 0 {
			select {
			case <-c.Request.Context().Done():
				return nil, fmt.Errorf("fal_sync adaptor: request cancelled during result fetch retry")
			case <-time.After(time.Second):
			}
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, resultURL, nil)
		if err != nil {
			return nil, fmt.Errorf("fal_sync adaptor: failed to create result request: %w", err)
		}
		req.Header.Set("Authorization", "Key "+info.ApiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if !isRetryableError(err) {
				return nil, fmt.Errorf("fal_sync adaptor: result fetch failed: %w", err)
			}
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Try to parse error - Requirement 6.2
			var errResp FALErrorResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Detail != "" {
				return nil, fmt.Errorf("fal_sync adaptor: result fetch error: %s", errResp.Detail)
			}
			return nil, fmt.Errorf("fal_sync adaptor: result fetch failed with status %d: %s", resp.StatusCode, string(respBody))
		}

		// Validate that the response is valid JSON before returning
		var result FALResultResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			// Requirement 6.5: Return parse error with details
			return nil, fmt.Errorf("fal_sync adaptor: failed to parse result response: %w, body: %s", err, string(respBody))
		}

		return respBody, nil
	}

	return nil, fmt.Errorf("fal_sync adaptor: result fetch failed after %d retries: %w", MaxRetries, lastErr)
}

// buildStatusURL constructs the status URL for polling
// URL format: {base_url}/{fal_endpoint}/requests/{request_id}/status
func buildStatusURL(baseURL, model, requestID string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	endpoint := getModelEndpoint(model)
	return fmt.Sprintf("%s/%s/requests/%s/status", baseURL, endpoint, requestID)
}

// buildResultURL constructs the result URL for fetching the final result
// URL format: {base_url}/{fal_endpoint}/requests/{request_id}
func buildResultURL(baseURL, model, requestID string) string {
	baseURL = strings.TrimSuffix(baseURL, "/")
	endpoint := getModelEndpoint(model)
	return fmt.Sprintf("%s/%s/requests/%s", baseURL, endpoint, requestID)
}

// getModelEndpoint returns the FAL API endpoint path for a given model
func getModelEndpoint(model string) string {
	switch {
	case strings.Contains(model, "hunyuan"):
		return "fal-ai/hunyuan-image"
	case strings.Contains(model, "qwen"):
		return "fal-ai/qwen-image-edit-2511"
	case strings.Contains(model, "flux"):
		return "fal-ai/flux-2-pro"
	default:
		return fmt.Sprintf("fal-ai/%s", model)
	}
}

// getHTTPClient returns an HTTP client, with proxy support if configured
func getHTTPClient(info *relaycommon.RelayInfo) (*http.Client, error) {
	if info.ChannelSetting.Proxy != "" {
		return service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	}
	return service.GetHttpClient(), nil
}

// isRetryableError checks if an error is retryable (network errors)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Check for context cancellation - not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Network errors are generally retryable
	return true
}

// parseErrorResponse parses an error response from FAL
func parseErrorResponse(resp *http.Response, operation string) error {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fal_sync adaptor: %s failed with status %d", operation, resp.StatusCode)
	}

	var errResp FALErrorResponse
	if json.Unmarshal(respBody, &errResp) == nil && errResp.Detail != "" {
		return fmt.Errorf("fal_sync adaptor: %s error: %s (status %d)", operation, errResp.Detail, resp.StatusCode)
	}

	return fmt.Errorf("fal_sync adaptor: %s failed with status %d: %s", operation, resp.StatusCode, string(respBody))
}

// DoResponse processes the FAL response and converts to OpenAI format
// Implements Requirements 4.1, 4.2, 4.3, 4.4, 4.5
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if resp == nil {
		return nil, types.NewError(errors.New("fal_sync adaptor: empty response"), types.ErrorCodeBadResponse)
	}

	// Read response body
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewError(readErr, types.ErrorCodeReadResponseBodyFailed)
	}
	_ = resp.Body.Close()

	// Parse FAL result response
	var falResult FALResultResponse
	if unmarshalErr := json.Unmarshal(responseBody, &falResult); unmarshalErr != nil {
		return nil, types.NewError(fmt.Errorf("fal_sync adaptor: failed to decode response: %w", unmarshalErr), types.ErrorCodeBadResponseBody)
	}

	// Collect image URLs from FAL response
	// Handle both `images` array and single `image` object - Requirements 4.1, 4.2
	var imageURLs []string

	// Check for images array first
	if len(falResult.Images) > 0 {
		for _, img := range falResult.Images {
			if url := strings.TrimSpace(img.URL); url != "" {
				imageURLs = append(imageURLs, url)
			}
		}
	}

	// Check for single image object
	if falResult.Image != nil {
		if url := strings.TrimSpace(falResult.Image.URL); url != "" {
			imageURLs = append(imageURLs, url)
		}
	}

	// If no images found, check output field as fallback
	if len(imageURLs) == 0 && falResult.Output != nil {
		switch output := falResult.Output.(type) {
		case string:
			if url := strings.TrimSpace(output); url != "" {
				imageURLs = append(imageURLs, url)
			}
		case []any:
			for _, item := range output {
				if str, ok := item.(string); ok {
					if url := strings.TrimSpace(str); url != "" {
						imageURLs = append(imageURLs, url)
					}
				}
			}
		}
	}

	if len(imageURLs) == 0 {
		return nil, types.NewError(errors.New("fal_sync adaptor: no images in response"), types.ErrorCodeBadResponseBody)
	}

	// Check if user requested b64_json response format - Requirement 4.3
	var wantsBase64 bool
	if info != nil {
		if req, ok := info.Request.(*dto.ImageRequest); ok {
			wantsBase64 = strings.EqualFold(req.ResponseFormat, "b64_json")
		}
	}

	// Build OpenAI ImageResponse - Requirements 4.1, 4.4
	imageResponse := dto.ImageResponse{
		Created: common.GetTimestamp(), // Requirement 4.4: Include created timestamp
		Data:    make([]dto.ImageData, 0, len(imageURLs)),
	}

	// Handle response_format=b64_json by downloading and encoding images - Requirement 4.3
	if wantsBase64 {
		for _, url := range imageURLs {
			_, b64Data, downloadErr := service.GetImageFromUrl(url)
			if downloadErr != nil {
				return nil, types.NewError(fmt.Errorf("fal_sync adaptor: failed to download image from %s: %w", url, downloadErr), types.ErrorCodeBadResponse)
			}
			if b64Data == "" {
				continue
			}
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: b64Data})
		}
	} else {
		for _, url := range imageURLs {
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: url})
		}
	}

	if len(imageResponse.Data) == 0 {
		return nil, types.NewError(errors.New("fal_sync adaptor: no usable image data"), types.ErrorCodeBadResponse)
	}

	// Include seed in metadata if available - Requirement 4.5
	if falResult.Seed != 0 {
		metadata := map[string]any{
			"seed": falResult.Seed,
		}
		metadataBytes, marshalErr := json.Marshal(metadata)
		if marshalErr == nil {
			imageResponse.Metadata = metadataBytes
		}
	}

	// Write response to client
	responseBytes, marshalErr := common.Marshal(imageResponse)
	if marshalErr != nil {
		return nil, types.NewError(fmt.Errorf("fal_sync adaptor: encode response failed: %w", marshalErr), types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(responseBytes)

	// Return usage for billing
	return &dto.Usage{}, nil
}

// ConvertOpenAIRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("fal_sync adaptor: ConvertOpenAIRequest is not implemented")
}

// ConvertRerankRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("fal_sync adaptor: ConvertRerankRequest is not implemented")
}

// ConvertEmbeddingRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("fal_sync adaptor: ConvertEmbeddingRequest is not implemented")
}

// ConvertAudioRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("fal_sync adaptor: ConvertAudioRequest is not implemented")
}

// ConvertOpenAIResponsesRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("fal_sync adaptor: ConvertOpenAIResponsesRequest is not implemented")
}

// ConvertClaudeRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("fal_sync adaptor: ConvertClaudeRequest is not implemented")
}

// ConvertGeminiRequest is not implemented for FAL Sync channel
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("fal_sync adaptor: ConvertGeminiRequest is not implemented")
}
