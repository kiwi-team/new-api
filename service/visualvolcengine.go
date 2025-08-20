package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// 请求地址
	Addr = "https://visual.volcengineapi.com"
	Path = "/" // 路径，不包含 Query

	// 请求接口信息
	Service = "cv"
	Region  = "cn-north-1"
	//Action  = "CVSync2AsyncSubmitTask"
	Version = "2022-08-31"
)

func hmacSHA256(key []byte, content string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(content))
	return mac.Sum(nil)
}

func getSignedKey(secretKey, date, region, service string) []byte {
	kDate := hmacSHA256([]byte(secretKey), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "request")

	return kSigning
}

func hashSHA256(data []byte) []byte {
	hash := sha256.New()
	if _, err := hash.Write(data); err != nil {
		log.Printf("input hash err:%s", err.Error())
	}

	return hash.Sum(nil)
}

func doRequest(method string, queries url.Values, body []byte, key string, secret string) (*http.Response, error) {
	// 1. 构建请求
	//queries.Set("Action", Action)
	queries.Set("Version", Version)
	requestAddr := fmt.Sprintf("%s%s?%s", Addr, Path, queries.Encode())
	//log.Printf("request addr: %s\n", requestAddr)

	request, err := http.NewRequest(method, requestAddr, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("bad request: %w", err)
	}

	// 2. 构建签名材料
	now := time.Now()
	date := now.UTC().Format("20060102T150405Z")
	authDate := date[:8]
	request.Header.Set("X-Date", date)

	payload := hex.EncodeToString(hashSHA256(body))
	request.Header.Set("X-Content-Sha256", payload)
	request.Header.Set("Content-Type", "application/json")

	queryString := strings.ReplaceAll(queries.Encode(), "+", "%20")
	signedHeaders := []string{"host", "x-date", "x-content-sha256", "content-type"}
	var headerList []string
	for _, header := range signedHeaders {
		if header == "host" {
			headerList = append(headerList, header+":"+request.Host)
		} else {
			v := request.Header.Get(header)
			headerList = append(headerList, header+":"+strings.TrimSpace(v))
		}
	}
	headerString := strings.Join(headerList, "\n")

	canonicalString := strings.Join([]string{
		method,
		Path,
		queryString,
		headerString + "\n",
		strings.Join(signedHeaders, ";"),
		payload,
	}, "\n")
	//log.Printf("canonical string:\n%s\n", canonicalString)

	hashedCanonicalString := hex.EncodeToString(hashSHA256([]byte(canonicalString)))
	//log.Printf("hashed canonical string: %s\n", hashedCanonicalString)

	credentialScope := authDate + "/" + Region + "/" + Service + "/request"
	signString := strings.Join([]string{
		"HMAC-SHA256",
		date,
		credentialScope,
		hashedCanonicalString,
	}, "\n")
	//log.Printf("sign string:\n%s\n", signString)

	// 3. 构建认证请求头
	//signedKey := getSignedKey(common.OptionMap["VolcstackSecretAccessKey"], authDate, Region, Service)
	signedKey := getSignedKey(secret, authDate, Region, Service)

	signature := hex.EncodeToString(hmacSHA256(signedKey, signString))
	//log.Printf("signature: %s\n", signature)

	//" Credential=" + common.OptionMap["VolcstackAccessKeyID"] + "/" + credentialScope +
	authorization := "HMAC-SHA256" +
		" Credential=" + key + "/" + credentialScope +
		", SignedHeaders=" + strings.Join(signedHeaders, ";") +
		", Signature=" + signature
	request.Header.Set("Authorization", authorization)

	// 4. 打印请求，发起请求
	// requestRaw, err := httputil.DumpRequest(request, true)
	// if err != nil {
	// 	return nil, fmt.Errorf("dump request err: %w", err)
	// }

	//log.Printf("request:\n%s\n", string(requestRaw))

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("do request err: %w", err)
	}

	// 5. 打印响应
	// defer response.Body.Close()
	// bodyBytes, err := io.ReadAll(response.Body)
	// if err != nil {
	// 	return nil, fmt.Errorf("read response body err: %w", err)
	// }

	if response.StatusCode == http.StatusOK {
		return response, nil
	} else {
		return response, fmt.Errorf("request failed with status %d", response.StatusCode)
	}

}

type SubmitTaskRequest struct {
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"` // base64图片字符串
	ImageUrls        []string `json:"image_urls,omitempty"`
	Prompt           string   `json:"prompt"`
	ReqKey           string   `json:"req_key,omitempty"`
	Scale            float32  `json:"scale,omitempty"`
	Seed             int      `json:"seed,omitempty"`
}

type SubmitTaskResponse struct {
	Code int64 `json:"code"`
	Data struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
	Message     string `json:"message"`
	RequestID   string `json:"request_id"`
	Status      int64  `json:"status"`
	TimeElapsed string `json:"time_elapsed"`
}

type GetTaskResultRequest struct {
	TaskID  string `json:"task_id"`
	ReqKey  string `json:"req_key,omitempty"`
	ReqJson string `json:"req_json,omitempty"` //"{\"logo_info\":{\"add_logo\":true,\"position\":0,\"language\":0,\"opacity\":0.3,\"logo_text_content\":\"这里是明水印内容\"},\"return_url\":true}"
}

type GetTaskResultData struct {
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"` // base64图片字符串
	ImageUrls        []string `json:"image_urls,omitempty"`
	ResponseData     string   `json:"response_data,omitempty"`
	Status           string   `json:"status,omitempty"` // in_queue,generating,done,not_found,expired
}

type GetTaskResultResponse struct {
	Code        int64             `json:"code"`
	Data        GetTaskResultData `json:"data"`
	Message     string            `json:"message"`
	RequestID   string            `json:"request_id"`
	Status      int64             `json:"status"`
	TimeElapsed string            `json:"time_elapsed"`
}

func SubmitTask(req *SubmitTaskRequest, key string, secret string) (*http.Response, error) {
	queries := make(url.Values)
	queries.Set("Action", "CVSync2AsyncSubmitTask")
	queries.Set("Version", "2022-08-31")
	req.ReqKey = "seededit_v3.0"
	jsonstr, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := doRequest(http.MethodPost, queries, jsonstr, key, secret)
	if err != nil {
		return nil, err
	}
	return resp, nil
	/*
		var resp SubmitTaskResponse
		err = json.Unmarshal(body, &resp)
		if err != nil {
			return "", err
		}
		return resp.Data.TaskID, nil
	*/
}

func GetTaskResult(req *GetTaskResultRequest, key string, secret string) (*GetTaskResultData, error) {
	queries := make(url.Values)
	queries.Set("Action", "CVSync2AsyncGetResult")
	queries.Set("Version", "2022-08-31")
	req.ReqKey = "seededit_v3.0"
	jsonstr, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := doRequest(http.MethodPost, queries, jsonstr, key, secret)
	if err != nil {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.New(strings.ReplaceAll(string(body), "Post", ""))
	}
	var respData GetTaskResultResponse
	// 复制resp,获取body数据
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, &respData)
	if err != nil {
		return nil, err
	}
	return &respData.Data, nil
}
