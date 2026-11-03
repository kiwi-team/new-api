package gemini

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"google.golang.org/genai"
)

func UploadFileToGemini(ctx context.Context, fileUri string, apiKey string, baseUrl string) (*genai.File, error) {
	if strings.HasPrefix(fileUri, "https://storage.googleapis.com/") {
		mimeType, err := GetFileMimeType(fileUri, ctx)
		if err != nil {
			return nil, err
		}
		return &genai.File{
			URI:      fileUri,
			MIMEType: mimeType,
		}, nil
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
		//HTTPOptions: genai.HTTPOptions{
		//	BaseURL:    baseUrl,
		//	APIVersion: "v1",
		//},
	})
	if err != nil {
		fmt.Printf("failed to create genai client, err: %v\n", err)
		return nil, err
	}

	//file, err := client.Files.UploadFromPath(ctx, fileUri, uploadConfig)
	req, err := http.NewRequest(http.MethodGet, fileUri, nil)
	if err != nil {
		return nil, err
	}
	response, err := common.DoTimedHTTP(ctx, http.DefaultClient, req, "file_download", false)
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
	//timeout := 10 * time.Minute
	uploadConfig := &genai.UploadFileConfig{
		MIMEType: mimeType,
		//HTTPOptions: &genai.HTTPOptions{
		//	BaseURL: baseUrl,
		//	Timeout: &timeout,
		//},
	}
	uploadTiming := common.BeginTimingOperation(ctx, "file_upload", false)
	file, err := client.Files.Upload(ctx, response.Body, uploadConfig)
	uploadTiming.End()
	if err != nil {
		fmt.Printf("failed to upload file to gemini, err: %v\n", err)
		return nil, err
	}

	// check file state
	file, err = CheckFileState(ctx, client, file)
	if err != nil {
		fmt.Printf("failed to check file state, err: %v\n", err)
		return nil, err
	}
	return &genai.File{
		Name:     file.Name,
		URI:      file.URI,
		MIMEType: mimeType,
	}, nil
}

// DeleteFileFromGemini deletes a previously uploaded file from the Gemini Files API.
// This should be called after the request completes to free up storage space.
func DeleteFileFromGemini(apiKey string, fileName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Printf("failed to create genai client for file deletion, err: %v\n", err)
		return
	}

	_, err = client.Files.Delete(ctx, fileName, nil)
	if err != nil {
		fmt.Printf("failed to delete file %s from gemini, err: %v\n", fileName, err)
	}
	fmt.Printf(" delete file %s from gemini\n", fileName)
}

// CleanupGeminiFiles deletes all uploaded files in the background.
func CleanupGeminiFiles(apiKey string, fileNames []string) {
	for _, name := range fileNames {
		go DeleteFileFromGemini(apiKey, name)
	}
}

func CheckFileState(ctx context.Context, client *genai.Client, file *genai.File) (*genai.File, error) {
	if file == nil {
		return nil, fmt.Errorf("file is nil")
	}
	max := 0
	for file.State != genai.FileStateActive && max < 10 {
		fmt.Printf("file state is processing, wait for 10 * %d seconds\n", max+1)
		time.Sleep(2 * time.Second)
		pollTiming := common.BeginTimingOperation(ctx, "task_query", false)
		file, _ = client.Files.Get(ctx, file.Name, nil)
		pollTiming.End()
		max = max + 1
	}
	if file == nil {
		return nil, fmt.Errorf("file is nil after 10 retries")
	}
	return file, nil
}
