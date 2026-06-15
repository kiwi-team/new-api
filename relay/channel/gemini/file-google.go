package gemini

import (
	"context"
	"encoding/json"
	"errors"
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
	// 单次上传(下载源文件 + 写入 GCS)的超时按文件大小动态计算:base + size/minThroughput,夹在 (0, maxTimeout]。
	// 大文件自动获得更多时间,不必再手动调常量;拿不到 Content-Length 时退回 default。超时兜底防止卡死导致泄漏。
	googleUploadBaseTimeout    = 5 * time.Minute  // 建连/握手/小文件的基础余量
	googleUploadMinThroughput  = 256 * 1024       // 最低预期端到端吞吐(B/s),据此折算大文件所需时间(实测约 340KB/s,留余量)
	googleUploadDefaultTimeout = 20 * time.Minute // 无法获取文件大小时的默认超时
	googleUploadMaxTimeout     = 60 * time.Minute // 硬上限,兜底防止卡死
	// googleUploadMaxFileSize 单文件上传大小上限,超过直接拒绝:不下载、不重试。
	googleUploadMaxFileSize = 2 * 1024 * 1024 * 1024 // 2GB
	// googleUploadMaxChunkSize GCS 写入分块缓冲上限,取 GCS 默认值 16MB:大文件用它保证上传吞吐
	// (分块越小,resumable 上传的往返次数越多、吞吐越低);小文件再按实际大小缩小(见下方 ContentLength 自适应),
	// 避免一张小图也占满 16MB。内存泄漏由超时机制兜底,故此处无需为省内存而牺牲大文件吞吐。
	googleUploadMaxChunkSize = 16 * 1024 * 1024
	// googleUploadChunkAlign GCS 要求 ChunkSize 必须是 256KB 的整数倍。
	googleUploadChunkAlign = 256 * 1024
)

// mimeProbeClient 仅用于探测远端文件的 Content-Type(只读响应头),带较短超时,避免连接挂死泄漏。
var mimeProbeClient = &http.Client{Timeout: 30 * time.Second}

// ErrFileTooLarge 表示源文件超过上传大小上限;命中后直接返回、不重试。
var ErrFileTooLarge = errors.New("file exceeds max upload size")

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

// uploadTimeoutForSize 按文件大小折算单次上传的超时:大文件给更多时间,小文件保持紧凑(卡住能快速失败)。
// size <= 0(大小未知)时退回默认超时。
func uploadTimeoutForSize(size int64) time.Duration {
	if size <= 0 {
		return googleUploadDefaultTimeout
	}
	d := googleUploadBaseTimeout + time.Duration(size/googleUploadMinThroughput)*time.Second
	if d > googleUploadMaxTimeout {
		return googleUploadMaxTimeout
	}
	return d
}

// probeContentLength 用 HEAD 探测远端文件大小(字节);失败、非 200 或未知返回 -1。
func probeContentLength(ctx context.Context, fileUri string) int64 {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, fileUri, nil)
	if err != nil {
		return -1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1
	}
	return resp.ContentLength
}

func UploadFileToGoogle(ctx context.Context, fileUri string, bucket string, credentials string) (*genai.File, error) {
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

	// 先 HEAD 拿 Content-Length:超过大小上限直接拒绝(不下载、不重试);否则按大小动态设超时
	// (大文件给更多时间、小文件保持紧凑)。该超时同时约束"下载源文件 + 写入 GCS",任一端卡死都会被 cancel。
	size := probeContentLength(ctx, fileUri)
	if size > googleUploadMaxFileSize {
		return nil, fmt.Errorf("%w: %d bytes > limit %d bytes", ErrFileTooLarge, size, int64(googleUploadMaxFileSize))
	}
	timeout := uploadTimeoutForSize(size)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
	// GCS writer 会按 ChunkSize 预分配缓冲区。大文件用上限(16MB,等于默认值,保证上传吞吐),
	// 小文件按实际大小缩小(256KB 对齐),避免一张小图也占 16MB —— 此前内存被放大的主因之一。
	chunkSize := googleUploadMaxChunkSize
	if response.ContentLength > 0 && response.ContentLength < int64(chunkSize) {
		chunkSize = int((response.ContentLength+googleUploadChunkAlign-1)/googleUploadChunkAlign) * googleUploadChunkAlign
	}
	wc.ChunkSize = chunkSize
	// io.Copy 同时在"下载源文件 + 上传 GCS",二者串联、谁慢谁拖累。失败时它仍会返回已拷贝字节数,
	// 连同总大小一起带出,便于判断是源站下载慢、GCS 上传慢,还是文件本身过大。
	// LimitReader 兜底:HEAD 拿不到大小(未知)时,这里保证最多传 上限+1 字节,超出即判定过大并中止。
	written, copyErr := io.Copy(wc, io.LimitReader(response.Body, googleUploadMaxFileSize+1))
	if copyErr != nil {
		// 失败时不调用 wc.Close()(可能阻塞在 flush);由上面的 defer cancel() 取消 ctx,
		// 让 writer 的后台 goroutine 退出并释放缓冲区。
		return nil, fmt.Errorf("io.Copy failed (transferred %d/%d bytes, timeout %s): %w", written, response.ContentLength, timeout, copyErr)
	}
	if written > googleUploadMaxFileSize {
		// 仅当源站未提供 Content-Length(HEAD 没能提前拦截)时才会走到这里。未 Close,ctx cancel 中止上传,不会提交对象。
		return nil, fmt.Errorf("%w: streamed over %d bytes", ErrFileTooLarge, int64(googleUploadMaxFileSize))
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
	var lastErr error
	attempts := 0
	for i := 0; i < retryTimes; i++ {
		attempts++
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
			// 记录每次失败的真实原因,便于定位(此前错误被静默吞掉,只能看到笼统的 "failed after N retries")。
			lastErr = err
			common.SysError(fmt.Sprintf("upload file to google failed (attempt %d/%d), fileUri: %s, err: %v", i+1, retryTimes, fileUri, err))
			// 以下情况不再重试,避免白白多等 N 倍超时:
			//  1) 父 ctx 已取消(如客户端断开)—— 重试也会立即再失败;
			//  2) 单次传输撞了硬超时(DeadlineExceeded)—— 同一文件从头重下也赶不上,只会再耗一个超时周期;
			//  3) 文件超过大小上限(ErrFileTooLarge)—— 重试结果必然相同。
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrFileTooLarge) {
				break
			}
			continue
		}
		if file == nil {
			lastErr = fmt.Errorf("upload returned nil file")
			common.SysError(fmt.Sprintf("upload file to google returned nil file (attempt %d/%d), fileUri: %s", i+1, retryTimes, fileUri))
			continue
		}
		return file, nil
	}
	if lastErr != nil {
		// 把最后一次的真实错误一并带出,channel error 日志里就能直接看到根因。
		// 用真实尝试次数:命中不可重试的错误(超大/超时/取消)时只试 1 次就退出,不再误报 "after 3 retries"。
		return nil, fmt.Errorf("upload file to google failed after %d attempt(s), fileUri: %s, last error: %w", attempts, fileUri, lastErr)
	}
	return nil, fmt.Errorf("upload file to google failed after %d attempt(s), fileUri: %s", attempts, fileUri)
}
