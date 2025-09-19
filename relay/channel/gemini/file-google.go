package gemini

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"one-api/common"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/genai"
)

func UploadFileToGoogle(ctx context.Context, fileUri string, bucket string, credentials string) (*genai.File, error) {
	bytes := []byte(credentials)
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON(bytes))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	//file, err := client.Files.UploadFromPath(ctx, fileUri, uploadConfig)
	response, err := http.Get(fileUri)
	if err != nil {
		return nil, fmt.Errorf("failed to download file from URL: %w", err)
	}
	defer response.Body.Close()

	// get mime type from response
	statusCode := response.StatusCode
	if statusCode != 200 {
		return nil, fmt.Errorf("failed to get file type from URL")
	}
	mimeType := response.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileUri))
	}
	object := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102150405"), common.GetRandomString(10), filepath.Ext(fileUri))
	obj := client.Bucket(bucket).Object(object)

	// 获取对象的写入器
	wc := obj.NewWriter(ctx)
	if _, err = io.Copy(wc, response.Body); err != nil {
		return nil, fmt.Errorf("io.Copy: %w", err)
	}

	// 关闭写入器以完成上传
	if err := wc.Close(); err != nil {
		return nil, fmt.Errorf("Writer.Close: %w", err)
	}

	fmt.Printf("File %s uploaded to gs://%s/%s\n", fileUri, bucket, object)

	return &genai.File{
		URI: fmt.Sprintf("gs://%s/%s", bucket, object),
		// https://storage.googleapis.com/toiotech2/2AQMKEtKkChtBGQk1ytkd0MT5fWS4q0c.mp3
		//URI:      fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, object),
		MIMEType: mimeType,
	}, nil
}

func RetryUploadFileToGoogle(ctx context.Context, fileUri string, bucket string, credentials string, retryTimes int) (*genai.File, error) {
	for i := 0; i < retryTimes; i++ {
		file, err := UploadFileToGoogle(ctx, fileUri, bucket, credentials)
		if err != nil {
			continue
		}
		return file, nil
	}
	return nil, fmt.Errorf("upload file to google failed after %d retries", retryTimes)
}
