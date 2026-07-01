package fal_sync

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// resolveFormImageURLs reads image files uploaded via multipart/form-data (the
// OpenAI /v1/images/edits form-data flow), uploads each one to the fal CDN and
// returns the resulting public access URLs. If an upload fails, it falls back to
// a base64 `data:` URI, which fal model endpoints also accept natively.
//
// When the request is not multipart/form-data this returns (nil, nil).
func resolveFormImageURLs(c *gin.Context, info *relaycommon.RelayInfo) ([]string, error) {
	files := collectFormImageFiles(c)
	if len(files) == 0 {
		return nil, nil
	}

	urls := make([]string, 0, len(files))
	for _, fh := range files {
		data, mimeType, err := readMultipartFile(fh)
		if err != nil {
			return nil, err
		}

		// Try uploading to the fal CDN first; on any failure fall back to a
		// base64 data URI so the request can still proceed.
		url, err := uploadToFalCDN(c, info, data, mimeType, fh.Filename)
		if err != nil {
			logger.LogWarn(c, fmt.Sprintf("fal_sync adaptor: CDN upload failed, falling back to base64 data URI: %s", err.Error()))
			url = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
		}
		urls = append(urls, url)
	}
	return urls, nil
}

// collectFormImageFiles gathers all uploaded image file headers from the parsed
// multipart form, supporting the `image`, `image[]` and `image[N]` field names
// used by various OpenAI-compatible clients.
func collectFormImageFiles(c *gin.Context) []*multipart.FileHeader {
	mf := c.Request.MultipartForm
	if mf == nil {
		// Only attempt to parse if the request actually carries a multipart body.
		if !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			return nil
		}
		if _, err := c.MultipartForm(); err != nil {
			return nil
		}
		mf = c.Request.MultipartForm
	}
	if mf == nil || mf.File == nil {
		return nil
	}

	var files []*multipart.FileHeader
	if fs, ok := mf.File["image"]; ok {
		files = append(files, fs...)
	}
	if fs, ok := mf.File["image[]"]; ok {
		files = append(files, fs...)
	}
	for field, fs := range mf.File {
		if field == "image" || field == "image[]" {
			continue
		}
		if strings.HasPrefix(field, "image[") {
			files = append(files, fs...)
		}
	}
	return files
}

// readMultipartFile reads a multipart file header into memory and detects its
// MIME type.
func readMultipartFile(fh *multipart.FileHeader) ([]byte, string, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, "", fmt.Errorf("fal_sync adaptor: failed to open uploaded image: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", fmt.Errorf("fal_sync adaptor: failed to read uploaded image: %w", err)
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	return data, mimeType, nil
}

// uploadToFalCDN uploads raw file bytes to the fal CDN (v3) and returns the
// public access URL. It performs the two-step token + upload flow used by the
// official fal client.
func uploadToFalCDN(c *gin.Context, info *relaycommon.RelayInfo, data []byte, mimeType, fileName string) (string, error) {
	client, err := getHTTPClient(info)
	if err != nil {
		return "", fmt.Errorf("fal_sync adaptor: failed to create HTTP client: %w", err)
	}

	// Step 1: obtain a short-lived CDN upload token.
	token, err := fetchCDNToken(c, client, info.ApiKey)
	if err != nil {
		return "", err
	}

	// Step 2: upload the file bytes using the token.
	ctx := c.Request.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, falCDNUploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("fal_sync adaptor: failed to create upload request: %w", err)
	}
	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	req.Header.Set("Authorization", tokenType+" "+token.Token)
	req.Header.Set("Content-Type", mimeType)
	if fileName != "" {
		req.Header.Set("X-Fal-File-Name", fileName)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fal_sync adaptor: CDN upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fal_sync adaptor: failed to read CDN upload response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fal_sync adaptor: CDN upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var uploadResp FALCDNUploadResponse
	if err := common.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("fal_sync adaptor: failed to parse CDN upload response: %w, body: %s", err, string(respBody))
	}
	if strings.TrimSpace(uploadResp.AccessURL) == "" {
		return "", fmt.Errorf("fal_sync adaptor: CDN upload response missing access_url, body: %s", string(respBody))
	}
	return uploadResp.AccessURL, nil
}

// fetchCDNToken requests a short-lived fal CDN (v3) upload token using the
// channel API key.
func fetchCDNToken(c *gin.Context, client *http.Client, apiKey string) (*FALCDNTokenResponse, error) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, falTokenURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to create CDN token request: %w", err)
	}
	req.Header.Set("Authorization", "Key "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: CDN token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to read CDN token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fal_sync adaptor: CDN token request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var token FALCDNTokenResponse
	if err := common.Unmarshal(respBody, &token); err != nil {
		return nil, fmt.Errorf("fal_sync adaptor: failed to parse CDN token response: %w, body: %s", err, string(respBody))
	}
	if strings.TrimSpace(token.Token) == "" {
		return nil, fmt.Errorf("fal_sync adaptor: CDN token response missing token, body: %s", string(respBody))
	}
	return &token, nil
}
