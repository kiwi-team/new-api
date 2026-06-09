package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/genai"
)

const (
	// googleUploadTimeout 单次上传(下载源文件 + 写入 GCS)的硬超时上限,防止任一端卡死导致 goroutine 与缓冲区泄漏。
	googleUploadTimeout = 10 * time.Minute
	// googleUploadMaxChunkSize GCS 写入分块缓冲的上限。GCS writer 默认 ChunkSize=16MB 且按此预分配缓冲区,过大。
	googleUploadMaxChunkSize = 4 * 1024 * 1024
	// googleUploadChunkAlign GCS 要求 ChunkSize 必须是 256KB 的整数倍。
	googleUploadChunkAlign = 256 * 1024
)

// mimeProbeClient 仅用于探测远端文件的 Content-Type(只读响应头),带较短超时,避免连接挂死泄漏。
var mimeProbeClient = &http.Client{Timeout: 30 * time.Second}

// urlExt extracts the file extension from a URL, stripping query parameters and fragments.
func urlExt(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return filepath.Ext(u.Path)
	}
	return filepath.Ext(rawURL)
}

func GetFileMimeType(fileUri string) (string, error) {
	response, err := mimeProbeClient.Get(fileUri)
	if err != nil {
		return "", fmt.Errorf("failed to get file type from URL: %w", err)
	}
	defer response.Body.Close()

	statusCode := response.StatusCode
	if statusCode != 200 {
		return "", fmt.Errorf("failed to get file type from URL")
	}
	mimeType := response.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(urlExt(fileUri))
	}
	return mimeType, nil
}

func UploadFileToGoogle(ctx context.Context, fileUri string, bucket string, credentials string) (*genai.File, error) {
	// 给整个上传(下载源文件 + 写入 GCS)加硬超时:任一端卡死时,defer cancel() 会取消 ctx,
	// 从而终止 GCS writer 的后台 goroutine 并释放其分块缓冲区,杜绝 goroutine / 内存泄漏。
	ctx, cancel := context.WithTimeout(ctx, googleUploadTimeout)
	defer cancel()

	if strings.HasPrefix(fileUri, "https://storage.googleapis.com/") {
		mimeType, err := GetFileMimeType(fileUri)
		if err != nil {
			return nil, err
		}
		return &genai.File{
			URI:         fileUri,
			DownloadURI: fileUri,
			MIMEType:    mimeType,
		}, nil
	}
	bytes := []byte(credentials)
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON(bytes))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	//file, err := client.Files.UploadFromPath(ctx, fileUri, uploadConfig)
	// 下载源文件:使用带超时的请求级 ctx,避免源站慢或挂死时无限阻塞(此前用默认 http.Get 无超时)。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileUri, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build download request: %w", err)
	}
	response, err := http.DefaultClient.Do(req)
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
		mimeType = mime.TypeByExtension(urlExt(fileUri))
	}
	object := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102150405"), common.GetRandomString(10), urlExt(fileUri))
	obj := client.Bucket(bucket).Object(object)

	// 获取对象的写入器
	wc := obj.NewWriter(ctx)
	// GCS writer 默认 ChunkSize=16MB,且会按 ChunkSize 预分配缓冲区 —— 高并发上传时这是内存被打满的主因。
	// 按源文件大小自适应缓冲(小文件用更小的块,256KB 对齐),并设上限,显著降低单个上传的常驻内存。
	chunkSize := googleUploadMaxChunkSize
	if response.ContentLength > 0 && response.ContentLength < int64(chunkSize) {
		chunkSize = int((response.ContentLength+googleUploadChunkAlign-1)/googleUploadChunkAlign) * googleUploadChunkAlign
	}
	wc.ChunkSize = chunkSize
	if _, err = io.Copy(wc, response.Body); err != nil {
		// 失败时不调用 wc.Close()(可能阻塞在 flush);由上面的 defer cancel() 取消 ctx,
		// 让 writer 的后台 goroutine 退出并释放缓冲区。
		return nil, fmt.Errorf("io.Copy: %w", err)
	}

	// 关闭写入器以完成上传
	if err := wc.Close(); err != nil {
		return nil, fmt.Errorf("Writer.Close: %w", err)
	}

	//fmt.Printf("upload file to google: %s/%s", bucket, object)
	return &genai.File{
		URI:         fmt.Sprintf("gs://%s/%s", bucket, object),
		DownloadURI: fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, object),
		// https://storage.googleapis.com/toiotech2/2AQMKEtKkChtBGQk1ytkd0MT5fWS4q0c.mp3
		//URI:      fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, object),
		MIMEType: mimeType,
	}, nil
}

// 通过配置的api接口，上传到google cloud storage
func UploadByConfigAPI(ctx context.Context, fileUri string) (*genai.File, error) {
	baseUrl := common.OptionMap["upload_google_newapi_url"]
	authToken := common.OptionMap["upload_google_newapi_token"]
	userId := common.OptionMap["upload_google_newapi_user_id"]

	if baseUrl == "" || authToken == "" || userId == "" {
		return nil, fmt.Errorf("upload_google_newapi_url, upload_google_newapi_token, upload_google_newapi_user_id are required")
	}

	body := strings.NewReader(fmt.Sprintf(`{"file_uri": "%s"}`, fileUri))
	req, err := http.NewRequest("POST", baseUrl, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("New-Api-User", userId)
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload file to google failed, status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	type RespBody struct {
		FileURI  string `json:"file_url"`
		MIMEType string `json:"mime_type"`
	}
	var respBody RespBody
	if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
		return nil, err
	}
	fileUri = respBody.FileURI
	return &genai.File{
		URI:         fileUri,
		DownloadURI: fileUri,
		MIMEType:    respBody.MIMEType,
	}, nil

}

// DeleteGCSObject deletes an object from Google Cloud Storage.
func DeleteGCSObject(credentials string, bucket string, object string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(credentials)))
	if err != nil {
		fmt.Printf("failed to create GCS client for object deletion, err: %v\n", err)
		return
	}
	defer client.Close()

	if err := client.Bucket(bucket).Object(object).Delete(ctx); err != nil {
		fmt.Printf("failed to delete GCS object %s/%s, err: %v\n", bucket, object, err)
	}
}

// CleanupGCSObjects deletes all uploaded GCS objects in the background.
func CleanupGCSObjects(objects []relaycommon.GCSObjectRef) {
	for _, obj := range objects {
		go DeleteGCSObject(obj.Credentials, obj.Bucket, obj.Object)
	}
}

func RetryUploadFileToGoogle(ctx context.Context, fileUri string, bucket string, credentials string, retryTimes int) (*genai.File, error) {
	for i := 0; i < retryTimes; i++ {
		var file *genai.File
		var err error
		if bucket == "" {
			//file, err = UploadByConfigAPI(ctx, fileUri)
			file, err = UploadFileToGemini(ctx, fileUri, credentials, "")
			if err != nil {
				return nil, err
			}
		} else {
			file, err = UploadFileToGoogle(ctx, fileUri, bucket, credentials)
		}
		if err != nil {
			continue
		}
		if file == nil {
			continue
		}
		return file, nil
	}
	//fmt.Printf("upload file to google failed after %d retries, fileUri: %s\n", retryTimes, fileUri)
	return nil, fmt.Errorf("upload file to google failed after %d retries, fileUri: %s", retryTimes, fileUri)
}
