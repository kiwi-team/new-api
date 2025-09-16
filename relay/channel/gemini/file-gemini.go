package gemini

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"google.golang.org/genai"
)

func UploadFileToGemini(ctx context.Context, fileUri string, apiKey string, baseUrl string) (*genai.File, error) {
	//client, err := storage.NewClient(ctx)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    baseUrl,
			APIVersion: "v1",
		},
	})
	if err != nil {
		return nil, err
	}

	//file, err := client.Files.UploadFromPath(ctx, fileUri, uploadConfig)
	response, err := http.Get(fileUri)
	if err != nil {
		return nil, fmt.Errorf("failed to download image from URL: %w", err)
	}
	defer response.Body.Close()

	// get mime type from response
	statusCode := response.StatusCode
	if statusCode != 200 {
		return nil, fmt.Errorf("failed to get image type from URL")
	}
	mimeType := response.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileUri))
	}

	// The BaseURL is already set by default in the genai client
	timeout := 10 * time.Minute
	uploadConfig := &genai.UploadFileConfig{
		MIMEType: mimeType,
		HTTPOptions: &genai.HTTPOptions{
			BaseURL: baseUrl,
			Timeout: &timeout,
		},
	}
	file, err := client.Files.Upload(ctx, response.Body, uploadConfig)
	if err != nil {
		return nil, err
	}
	return &genai.File{
		URI:      file.URI,
		MIMEType: mimeType,
	}, nil
}
