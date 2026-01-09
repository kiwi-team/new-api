package hunyuan

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/common"
	aiart "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/aiart/v20221229"
	aiartcommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	aiarterrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

// ============================
// https://hunyuan.ai/docs/api-reference/model-apis-flux-1-kontext-pro
// https://hunyuan.ai/docs/api-reference/model-apis-task-result
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
	Sign        string
	Action      string
	Version     string
	Timestamp   int64
	JobId       string
	Region      string
	Service     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
	a.Action = "SubmitTextToImageJob"
	a.Version = "2022-12-29"
	a.Timestamp = time.Now().Unix()
	a.Region = HunyuanRegionShanghai
	a.Service = "aiart"
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	///v3/async/flux-1-kontext-pro
	return "https://aiart.tencentcloudapi.com", nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	req.Header.Set("Authorization", a.Sign)
	req.Header.Set("X-TC-Action", a.Action)
	req.Header.Set("X-TC-Version", a.Version)
	req.Header.Set("X-TC-Region", a.Region)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(a.Timestamp, 10))
	req.Header.Set("Host", "aiart.tencentcloudapi.com")
	return nil
}

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body := HunyuanTaskSubmitRequest{
		Prompt: req.Prompt,
		Seed:   0,
	}

	// 同步扩展字段的厂商自定义metadata
	if req.Metadata != nil {
		if v, ok := req.Metadata["resolution"]; ok {
			if s, ok := v.(string); ok && s != "" {
				body.Resolution = s
			}
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	_, secretId, secretKey, err := parseTencentConfig(apiKey)
	//a.AppID = appId
	if err != nil {
		return nil, err
	}
	//fmt.Printf("secretId: %s, secretKey: %s\n", secretId, secretKey)
	a.Sign = getTencentSign2(data, "SubmitTextToImageJob", a.Timestamp, secretId, secretKey)
	//c.Header("X-TC-Sign", a.Sign)
	//c.Request.Header.Set("Authorization", a.Sign)
	//c.Header("Authorization", a.Sign)
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	//return channel.DoTaskApiRequest(a, c, info, requestBody)
	// 密钥信息从环境变量读取，需要提前在环境变量中设置 TENCENTCLOUD_SECRET_ID 和 TENCENTCLOUD_SECRET_KEY
	// 使用环境变量方式可以避免密钥硬编码在代码中，提高安全性
	// 生产环境建议使用更安全的密钥管理方案，如密钥管理系统(KMS)、容器密钥注入等
	// 请参见：https://cloud.tencent.com/document/product/1278/85305
	// 密钥可前往官网控制台 https://console.cloud.tencent.com/cam/capi 进行获取
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	_, secretId, secretKey, err := parseTencentConfig(apiKey)
	//a.AppID = appId
	if err != nil {
		return nil, err
	}
	credential := aiartcommon.NewCredential(
		secretId,
		secretKey,
	)
	// 使用临时密钥示例
	// credential := common.NewTokenCredential("SecretId", "SecretKey", "Token")
	// 实例化一个client选项，可选的，没有特殊需求可以跳过
	body := HunyuanTaskSubmitRequest{}
	if data, err1 := io.ReadAll(requestBody); err1 == nil {
		json.Unmarshal(data, &body)
	} else {
		return nil, err1
	}
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "aiart.tencentcloudapi.com"
	// 实例化要请求产品的client对象,clientProfile是可选的
	client, err2 := aiart.NewClient(credential, "ap-shanghai", cpf)
	if err2 != nil {
		return nil, err2
	}

	// 实例化一个请求对象,每个接口都会对应一个request对象
	request := aiart.NewSubmitTextToImageJobRequest()
	request.Prompt = aiartcommon.StringPtr(body.Prompt)
	if body.Seed != 0 {
		request.Seed = aiartcommon.Int64Ptr(int64(body.Seed))
	}
	if body.Resolution != "" {
		request.Resolution = aiartcommon.StringPtr(body.Resolution)
	}

	// 返回的resp是一个SubmitTextToImageJobResponse的实例，与请求对象对应
	response, err := client.SubmitTextToImageJob(request)
	if _, ok := err.(*aiarterrors.TencentCloudSDKError); ok {
		fmt.Printf("An API error has returned: %s", err)
	}
	if err != nil {
		return nil, err
	}
	// The SDK call already returned the jobId string, so we build a minimal HTTP response
	// to satisfy the method signature (*http.Response, error).
	jobId := *response.Response.JobId
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"Response":{"JobId":"%s"}}`, jobId))),
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp, nil
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var s HunyuanTaskSubmitResponse
	if err := json.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Response.JobId) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing taskId"), "invalid_response", http.StatusInternalServerError)
	}
	localID := s.Response.JobId
	c.JSON(http.StatusOK, gin.H{"task_id": localID})
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string { return []string{"hunyuan-image-3"} }
func (a *TaskAdaptor) GetChannelName() string { return "hunyuan" }

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTaskBak(baseUrl, key string, body map[string]any) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	url := "https://aiart.tencentcloudapi.com"
	tbody := map[string]string{
		"JobId": taskID,
	}
	a.JobId = taskID
	jsonBody, err := json.Marshal(tbody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	_, secretId, secretKey, err := parseTencentConfig(key)
	if err != nil {
		return nil, err
	}
	stamp := time.Now().Unix()
	auth := getTencentSign2(jsonBody, "QueryTextToImageJob", stamp, secretId, secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", auth)
	req.Header.Set("X-TC-Action", "QueryTextToImageJob")
	req.Header.Set("X-TC-Version", "2022-12-29")
	req.Header.Set("X-TC-Region", a.Region)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(stamp, 10))
	/*
	 {"id":"xxxxxx","status":"pending",.....}
	*/
	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	// url := "https://aiart.tencentcloudapi.com"
	// tbody := map[string]string{
	// 	"JobId": taskID,
	// }
	// a.JobId = taskID
	// jsonBody, err := json.Marshal(tbody)
	// if err != nil {
	// 	return nil, err
	// }

	apiKey := strings.TrimPrefix(key, "Bearer ")
	_, secretId, secretKey, err := parseTencentConfig(apiKey)
	//a.AppID = appId
	if err != nil {
		return nil, err
	}
	credential := aiartcommon.NewCredential(
		secretId,
		secretKey,
	)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "aiart.tencentcloudapi.com"
	// 实例化要请求产品的client对象,clientProfile是可选的
	client, err2 := aiart.NewClient(credential, "ap-shanghai", cpf)
	if err2 != nil {
		return nil, err2
	}

	// 实例化一个请求对象,每个接口都会对应一个request对象
	request := aiart.NewQueryTextToImageJobRequest()
	request.JobId = aiartcommon.StringPtr(taskID)

	// 返回的resp是一个SubmitTextToImageJobResponse的实例，与请求对象对应
	response, err := client.QueryTextToImageJob(request)
	if _, ok := err.(*aiarterrors.TencentCloudSDKError); ok {
		fmt.Printf("An API error has returned: %s", err)
	}
	if err != nil {
		return nil, err
	}
	response.Response.RequestId = aiartcommon.StringPtr(taskID)
	jsonStr := response.ToJsonString()
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(jsonStr)),
		Header:     make(http.Header),
	}, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op HunyuanTaskResultResponse
	if err := json.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	ti := &relaycommon.TaskInfo{}
	//taskId := a.JobId
	taskId := op.Response.RequestId
	ti.TaskID = taskId
	status := op.Response.JobStatusCode
	if status == HunyuanTaskStatusFailed {
		ti.Status = model.TaskStatusFailure
		ti.Reason = fmt.Sprintf("%v", op.Response.JobErrorMsg)
		ti.Progress = "100%"
		return ti, nil
	}
	ti.Status = model.TaskStatusInProgress
	ti.Progress = fmt.Sprintf("%d%%", 50)
	if status == HunyuanTaskStatusRunning || status == HunyuanTaskStatusWaiting {
		return ti, nil
	}
	if status == HunyuanTaskStatusSuccess {
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
	}
	if len(op.Response.ResultImage) > 0 { // some variants use `video` as base64
		if v, ok := taskcommon.FinishedTaskCache.Get(taskId); ok {
			ti.Url = v
			return ti, nil
		}
		imageUrl := op.Response.ResultImage[0]
		task, exists, _ := model.GetByOnlyTaskId(taskId)
		if exists && task != nil {
			if strings.HasPrefix(task.FailReason, "https://toio") {
				ti.Url = task.FailReason
				return ti, nil
			}
		}
		if file, err := service.SimpleUploadToS3(context.Background(), imageUrl); err == nil {
			ti.Url = file
			taskcommon.FinishedTaskCache.Set(taskId, file)
		} else {
			ti.Url = imageUrl
		}
		return ti, nil
	}
	return ti, nil
}

func sha256hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func hmacSha256(s, key string) string {
	hashed := hmac.New(sha256.New, []byte(key))
	hashed.Write([]byte(s))
	return string(hashed.Sum(nil))
}

func getTencentSign(payload []byte, action string, timestamp int64, secId, secKey string) string {
	// build canonical request string
	host := "hunyuan.tencentcloudapi.com"
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		"application/json", host, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	//payload, _ := json.Marshal(req)
	hashedRequestPayload := sha256hex(string(payload))
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload)
	// build string to sign
	algorithm := "TC3-HMAC-SHA256"
	requestTimestamp := strconv.FormatInt(int64(timestamp), 10)
	timestamp1, _ := strconv.ParseInt(requestTimestamp, 10, 64)
	t := time.Unix(timestamp1, 0).UTC()
	// must be the format 2006-01-02, ref to package time for more info
	date := t.Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, "aiart")
	hashedCanonicalRequest := sha256hex(canonicalRequest)
	string2sign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		requestTimestamp,
		credentialScope,
		hashedCanonicalRequest)

	// sign string
	secretDate := hmacSha256(date, "TC3"+secKey)
	secretService := hmacSha256("aiart", secretDate)
	secretKey := hmacSha256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSha256(string2sign, secretKey)))

	// build authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		secId,
		credentialScope,
		signedHeaders,
		signature)
	return authorization
}

func parseTencentConfig(config string) (appId int64, secretId string, secretKey string, err error) {
	parts := strings.Split(config, "|")
	if len(parts) != 3 {
		err = errors.New("invalid tencent config")
		return
	}
	appId, err = strconv.ParseInt(parts[0], 10, 64)
	secretId = parts[1]
	secretKey = parts[2]
	return
}

func getTencentSign2(payload []byte, action string, timestamp int64, secId, secKey string) string {
	// 需要设置环境变量 TENCENTCLOUD_SECRET_ID，值为示例的 AKID********************************
	secretId := secId
	// 需要设置环境变量 TENCENTCLOUD_SECRET_KEY，值为示例的 ********************************
	secretKey := secKey
	host := "aiart.tencentcloudapi.com"
	algorithm := "TC3-HMAC-SHA256"
	service := "aiart"
	//version := "2017-03-12"
	//region := HunyuanRegionShanghai
	//var timestamp int64 = time.Now().Unix()

	// step 1: build canonical request string
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		"application/json; charset=utf-8", host, strings.ToLower(action))
	signedHeaders := "content-type;host"
	//payload := `{"Limit": 1, "Filters": [{"Values": ["\u672a\u547d\u540d"], "Name": "instance-name"}]}`
	//hashedRequestPayload := sha256hex(payload)
	hashedRequestPayload := sha256hex(string(payload))
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload)
	//fmt.Println("canonicalRequest:", canonicalRequest)

	// step 2: build string to sign
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	hashedCanonicalRequest := sha256hex(canonicalRequest)
	string2sign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm,
		timestamp,
		credentialScope,
		hashedCanonicalRequest)
	//fmt.Println("string2sign:", string2sign)

	// step 3: sign string
	secretDate := hmacSha256(date, "TC3"+secretKey)
	secretService := hmacSha256(service, secretDate)
	secretSigning := hmacSha256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSha256(string2sign, secretSigning)))
	//fmt.Println("signature:", signature)

	// step 4: build authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		secretId,
		credentialScope,
		signedHeaders,
		signature)
	return authorization

}
