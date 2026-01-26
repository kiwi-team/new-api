package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// DebugExecutor 调试执行器
type DebugExecutor struct {
	config    *model.DebugConfiguration
	channel   *model.Channel
	baseUrl   string
	apiKey    string
	variables map[string]interface{}
}

// NewDebugExecutor 创建调试执行器
func NewDebugExecutor(config *model.DebugConfiguration, channel *model.Channel) *DebugExecutor {
	executor := &DebugExecutor{
		config:  config,
		channel: channel,
	}

	// 确定 baseUrl 和 apiKey
	if config.BaseUrl != "" {
		executor.baseUrl = config.BaseUrl
	} else if channel != nil {
		executor.baseUrl = channel.GetBaseURL()
	}

	if config.ApiKey != "" {
		executor.apiKey = config.ApiKey
	} else if channel != nil {
		executor.apiKey = channel.Key
	}

	// 获取变量值
	variables, _ := config.GetVariableValues()
	executor.variables = variables

	return executor
}

// ExecuteTemplate 执行单个模板
func (e *DebugExecutor) ExecuteTemplate(template *model.DebugRequestTemplate) (*model.DebugRequestLog, error) {
	log := &model.DebugRequestLog{
		ConfigurationId: &e.config.Id,
		TemplateId:      &template.Id,
		TemplateIndex:   0,
		Status:          "success",
	}

	startTime := time.Now()

	// 替换变量
	replacer := NewVariableReplacer(e.variables, nil)

	// 替换路径
	path, err := replacer.ReplaceInPath(template.Path)
	if err != nil {
		log.Status = "error"
		log.ErrorMessage = fmt.Sprintf("Failed to replace path variables: %v", err)
		return log, err
	}

	// 构建完整 URL
	url := strings.TrimRight(e.baseUrl, "/") + "/" + strings.TrimLeft(path, "/")
	log.RequestUrl = url
	log.RequestMethod = template.Method

	// 替换请求体
	var requestBody string
	if template.BodyTemplate != "" {
		requestBody, err = replacer.Replace(template.BodyTemplate)
		if err != nil {
			log.Status = "error"
			log.ErrorMessage = fmt.Sprintf("Failed to replace body variables: %v", err)
			return log, err
		}

		// 验证 JSON
		if err := replacer.ValidateJSON(requestBody); err != nil {
			log.Status = "error"
			log.ErrorMessage = fmt.Sprintf("Invalid JSON after variable replacement: %v", err)
			return log, err
		}
	}
	log.RequestBody = requestBody

	// 提取模型名称（如果存在）
	if requestBody != "" {
		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(requestBody), &bodyMap); err == nil {
			if model, ok := bodyMap["model"].(string); ok {
				log.ModelName = model
			}
			if stream, ok := bodyMap["stream"].(bool); ok {
				log.IsStream = stream
			}
		}
	}

	// 构建请求头
	headers := make(map[string]string)
	if e.apiKey != "" {
		headers["Authorization"] = "Bearer " + e.apiKey
	}
	headers["Content-Type"] = template.ContentType

	// 添加模板中的额外 headers
	templateHeaders, _ := template.GetHeaders()
	for k, v := range templateHeaders {
		headers[k] = v
	}
	log.SetRequestHeaders(headers)

	// 发送请求
	var responseBody string
	var responseChunks []model.StreamChunk
	var responseStatus int
	var responseHeaders map[string]string

	if log.IsStream {
		responseBody, responseChunks, responseStatus, responseHeaders, err = e.sendStreamRequest(url, template.Method, headers, requestBody)
	} else {
		responseBody, responseStatus, responseHeaders, err = e.sendRequest(url, template.Method, headers, requestBody)
	}

	// 记录响应
	log.ResponseStatus = responseStatus
	log.SetResponseHeaders(responseHeaders)
	log.ResponseBody = responseBody
	if len(responseChunks) > 0 {
		log.SetResponseChunks(responseChunks)
	}

	// 计算耗时
	log.DurationMs = int(time.Since(startTime).Milliseconds())

	// 处理错误
	if err != nil {
		log.Status = "error"
		log.ErrorMessage = err.Error()
		return log, err
	}

	// 验证响应
	if responseStatus >= 400 {
		log.Status = "failed"
		log.ErrorMessage = fmt.Sprintf("HTTP error: %d", responseStatus)
	} else {
		// 执行验证
		schema, _ := template.GetResponseSchema()
		streamValidation, _ := template.GetStreamValidation()
		validator := NewResponseValidator(schema, streamValidation)

		var validationResult *model.ValidationResult
		if log.IsStream {
			validationResult = validator.ValidateStreamResponse(responseChunks)
		} else {
			validationResult = validator.ValidateResponse(responseBody)
		}

		log.SetValidationResult(validationResult)
		if !validationResult.Valid {
			log.Status = "failed"
		}
	}

	return log, nil
}

// ExecuteTestSuite 执行测试集
func (e *DebugExecutor) ExecuteTestSuite(suite *model.DebugTestSuite) ([]*model.DebugRequestLog, error) {
	templateIds, err := suite.GetTemplateIds()
	if err != nil {
		return nil, fmt.Errorf("failed to get template IDs: %w", err)
	}

	if len(templateIds) == 0 {
		return nil, fmt.Errorf("test suite has no templates")
	}

	// 生成执行批次 ID
	suiteExecutionId := uuid.New().String()

	// 获取变量映射
	variableMappings, _ := suite.GetVariableMappings()
	mappingMap := make(map[int][]model.VariableMappingRule)
	for _, mapping := range variableMappings {
		mappingMap[mapping.TemplateIndex] = mapping.Mappings
	}

	logs := make([]*model.DebugRequestLog, 0)
	var prevResponse map[string]interface{}

	// 依次执行每个模板
	for index, templateId := range templateIds {
		// 获取模板
		template := &model.DebugRequestTemplate{}
		if err := model.DB.First(template, templateId).Error; err != nil {
			return logs, fmt.Errorf("failed to get template %d: %w", templateId, err)
		}

		// 如果不是第一个模板，应用变量映射
		if index > 0 && prevResponse != nil {
			if mappings, exists := mappingMap[index]; exists {
				for _, mapping := range mappings {
					// 从上一个响应中提取值
					value := extractValueFromResponse(prevResponse, mapping.Source)
					if value != nil {
						e.variables[mapping.Variable] = value
					}
				}
			}
		}

		// 创建新的替换器（包含上一个响应）
		replacer := NewVariableReplacer(e.variables, prevResponse)

		// 执行模板（使用更新后的变量）
		log, err := e.executeTemplateWithReplacer(template, replacer, index)
		log.SuiteExecutionId = suiteExecutionId
		logs = append(logs, log)

		if err != nil || log.Status != "success" {
			// 如果执行失败，停止后续执行
			break
		}

		// 保存响应用于下一个模板
		if log.ResponseBody != "" {
			json.Unmarshal([]byte(log.ResponseBody), &prevResponse)
		}
	}

	return logs, nil
}

// executeTemplateWithReplacer 使用指定的替换器执行模板
func (e *DebugExecutor) executeTemplateWithReplacer(template *model.DebugRequestTemplate, replacer *VariableReplacer, index int) (*model.DebugRequestLog, error) {
	log := &model.DebugRequestLog{
		ConfigurationId: &e.config.Id,
		TemplateId:      &template.Id,
		TemplateIndex:   index,
		Status:          "success",
	}

	startTime := time.Now()

	// 替换路径
	path, err := replacer.ReplaceInPath(template.Path)
	if err != nil {
		log.Status = "error"
		log.ErrorMessage = fmt.Sprintf("Failed to replace path variables: %v", err)
		return log, err
	}

	// 构建完整 URL
	url := strings.TrimRight(e.baseUrl, "/") + "/" + strings.TrimLeft(path, "/")
	log.RequestUrl = url
	log.RequestMethod = template.Method

	// 替换请求体
	var requestBody string
	if template.BodyTemplate != "" {
		requestBody, err = replacer.Replace(template.BodyTemplate)
		if err != nil {
			log.Status = "error"
			log.ErrorMessage = fmt.Sprintf("Failed to replace body variables: %v", err)
			return log, err
		}
	}
	log.RequestBody = requestBody

	// 提取模型名称
	if requestBody != "" {
		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(requestBody), &bodyMap); err == nil {
			if model, ok := bodyMap["model"].(string); ok {
				log.ModelName = model
			}
			if stream, ok := bodyMap["stream"].(bool); ok {
				log.IsStream = stream
			}
		}
	}

	// 构建请求头
	headers := make(map[string]string)
	if e.apiKey != "" {
		headers["Authorization"] = "Bearer " + e.apiKey
	}
	headers["Content-Type"] = template.ContentType

	templateHeaders, _ := template.GetHeaders()
	for k, v := range templateHeaders {
		headers[k] = v
	}
	log.SetRequestHeaders(headers)

	// 发送请求
	var responseBody string
	var responseChunks []model.StreamChunk
	var responseStatus int
	var responseHeaders map[string]string

	if log.IsStream {
		responseBody, responseChunks, responseStatus, responseHeaders, err = e.sendStreamRequest(url, template.Method, headers, requestBody)
	} else {
		responseBody, responseStatus, responseHeaders, err = e.sendRequest(url, template.Method, headers, requestBody)
	}

	log.ResponseStatus = responseStatus
	log.SetResponseHeaders(responseHeaders)
	log.ResponseBody = responseBody
	if len(responseChunks) > 0 {
		log.SetResponseChunks(responseChunks)
	}

	log.DurationMs = int(time.Since(startTime).Milliseconds())

	if err != nil {
		log.Status = "error"
		log.ErrorMessage = err.Error()
		return log, err
	}

	if responseStatus >= 400 {
		log.Status = "failed"
		log.ErrorMessage = fmt.Sprintf("HTTP error: %d", responseStatus)
	} else {
		// 验证响应
		schema, _ := template.GetResponseSchema()
		streamValidation, _ := template.GetStreamValidation()
		validator := NewResponseValidator(schema, streamValidation)

		var validationResult *model.ValidationResult
		if log.IsStream {
			validationResult = validator.ValidateStreamResponse(responseChunks)
		} else {
			validationResult = validator.ValidateResponse(responseBody)
		}

		log.SetValidationResult(validationResult)
		if !validationResult.Valid {
			log.Status = "failed"
		}
	}

	return log, nil
}

// sendRequest 发送非流式请求
func (e *DebugExecutor) sendRequest(url, method string, headers map[string]string, body string) (string, int, map[string]string, error) {
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		return "", 0, nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, nil, err
	}

	responseHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			responseHeaders[k] = v[0]
		}
	}

	return string(responseBody), resp.StatusCode, responseHeaders, nil
}

// sendStreamRequest 发送流式请求
func (e *DebugExecutor) sendStreamRequest(url, method string, headers map[string]string, body string) (string, []model.StreamChunk, int, map[string]string, error) {
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		return "", nil, 0, nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, 0, nil, err
	}
	defer resp.Body.Close()

	responseHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			responseHeaders[k] = v[0]
		}
	}

	// 读取流式响应
	chunks := make([]model.StreamChunk, 0)
	aggregatedBody := strings.Builder{}
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		chunk := model.StreamChunk{
			Timestamp: time.Now(),
			Raw:       line,
		}

		// 解析 SSE 格式
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				continue
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err == nil {
				chunk.Data = data

				// 尝试提取事件类型
				if eventType, ok := data["type"].(string); ok {
					chunk.Event = eventType
				}
			}

			aggregatedBody.WriteString(dataStr)
			aggregatedBody.WriteString("\n")
		} else if strings.HasPrefix(line, "event: ") {
			chunk.Event = strings.TrimPrefix(line, "event: ")
		}

		chunks = append(chunks, chunk)
	}

	if err := scanner.Err(); err != nil {
		return "", chunks, resp.StatusCode, responseHeaders, err
	}

	return aggregatedBody.String(), chunks, resp.StatusCode, responseHeaders, nil
}

// extractValueFromResponse 从响应中提取值
func extractValueFromResponse(response map[string]interface{}, source string) interface{} {
	// 移除 prev_response. 前缀
	path := strings.TrimPrefix(source, "prev_response.")

	// 转换为 JSON 以使用 gjson
	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil
	}

	// 使用 gjson 提取值
	result := gjson.GetBytes(jsonData, path)
	if !result.Exists() {
		return nil
	}

	return result.Value()
}

// DirectDebugExecutor 直接调试执行器（不使用模板）
type DirectDebugExecutor struct {
	baseUrl string
	apiKey  string
}

// DirectExecuteResult 直接执行结果
type DirectExecuteResult struct {
	StatusCode      int
	ResponseHeaders string
	ResponseBody    string
	ResponseChunks  string
	DurationMs      int
	TtfbMs          int
	ErrorMessage    string
	Status          string
}

// NewDirectDebugExecutor 创建直接调试执行器
func NewDirectDebugExecutor(baseUrl, apiKey string) *DirectDebugExecutor {
	return &DirectDebugExecutor{
		baseUrl: baseUrl,
		apiKey:  apiKey,
	}
}

// ExecuteStream 执行流式请求并直接写入 ResponseWriter（SSE）
func (e *DirectDebugExecutor) ExecuteStream(method, path string, headers map[string]string, body string, writer io.Writer) (*DirectExecuteResult, error) {
	result := &DirectExecuteResult{
		Status: "success",
	}

	startTime := time.Now()
	var ttfbTime time.Time

	// 构建完整 URL
	url := strings.TrimRight(e.baseUrl, "/") + "/" + strings.TrimLeft(path, "/")

	// 创建请求
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		return result, err
	}

	// 设置请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 创建 HTTP 客户端
	client := &http.Client{
		Timeout: 300 * time.Second, // 流式请求需要更长超时
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		result.DurationMs = int(time.Since(startTime).Milliseconds())
		return result, err
	}
	defer resp.Body.Close()

	// 记录 TTFB
	ttfbTime = time.Now()
	result.TtfbMs = int(ttfbTime.Sub(startTime).Milliseconds())
	result.StatusCode = resp.StatusCode

	// 收集响应头
	responseHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			responseHeaders[k] = v[0]
		}
	}
	headersBytes, _ := json.Marshal(responseHeaders)
	result.ResponseHeaders = string(headersBytes)

	// 检查状态码
	if resp.StatusCode >= 400 {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("HTTP error: %d", resp.StatusCode)
		// 读取错误响应体
		errorBody, _ := io.ReadAll(resp.Body)
		result.ResponseBody = string(errorBody)
		result.DurationMs = int(time.Since(startTime).Milliseconds())
		// 写入错误响应到 SSE
		fmt.Fprintf(writer, "data: %s\n\n", string(errorBody))
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return result, nil
	}

	// 流式读取并转发
	chunks := make([]model.StreamChunk, 0)
	aggregatedBody := strings.Builder{}
	scanner := bufio.NewScanner(resp.Body)

	// 增加 scanner 缓冲区大小
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// 直接转发原始行到客户端
		fmt.Fprintf(writer, "%s\n", line)
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}

		if line == "" {
			continue
		}

		chunk := model.StreamChunk{
			Timestamp: time.Now(),
			Raw:       line,
		}

		// 解析 SSE 格式用于日志记录
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				continue
			}

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err == nil {
				chunk.Data = data
				if eventType, ok := data["type"].(string); ok {
					chunk.Event = eventType
				}
			}

			aggregatedBody.WriteString(dataStr)
			aggregatedBody.WriteString("\n")
		} else if strings.HasPrefix(line, "event: ") {
			chunk.Event = strings.TrimPrefix(line, "event: ")
		}

		chunks = append(chunks, chunk)
	}

	if err := scanner.Err(); err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
	}

	result.ResponseBody = aggregatedBody.String()
	chunksBytes, _ := json.Marshal(chunks)
	result.ResponseChunks = string(chunksBytes)

	// 计算总耗时
	result.DurationMs = int(time.Since(startTime).Milliseconds())

	return result, nil
}

// Execute 执行直接请求
func (e *DirectDebugExecutor) Execute(method, path string, headers map[string]string, body string, isStream bool) (*DirectExecuteResult, error) {
	result := &DirectExecuteResult{
		Status: "success",
	}

	startTime := time.Now()
	var ttfbTime time.Time

	// 构建完整 URL
	url := strings.TrimRight(e.baseUrl, "/") + "/" + strings.TrimLeft(path, "/")

	// 创建请求
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		return result, err
	}

	// 设置请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 创建 HTTP 客户端
	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		result.DurationMs = int(time.Since(startTime).Milliseconds())
		return result, err
	}
	defer resp.Body.Close()

	// 记录 TTFB
	ttfbTime = time.Now()
	result.TtfbMs = int(ttfbTime.Sub(startTime).Milliseconds())
	result.StatusCode = resp.StatusCode

	// 收集响应头
	responseHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			responseHeaders[k] = v[0]
		}
	}
	headersBytes, _ := json.Marshal(responseHeaders)
	result.ResponseHeaders = string(headersBytes)

	// 读取响应
	if isStream {
		// 流式响应
		chunks := make([]model.StreamChunk, 0)
		aggregatedBody := strings.Builder{}
		scanner := bufio.NewScanner(resp.Body)

		// 增加 scanner 缓冲区大小
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			chunk := model.StreamChunk{
				Timestamp: time.Now(),
				Raw:       line,
			}

			// 解析 SSE 格式
			if strings.HasPrefix(line, "data: ") {
				dataStr := strings.TrimPrefix(line, "data: ")
				if dataStr == "[DONE]" {
					continue
				}

				var data map[string]interface{}
				if err := json.Unmarshal([]byte(dataStr), &data); err == nil {
					chunk.Data = data
					if eventType, ok := data["type"].(string); ok {
						chunk.Event = eventType
					}
				}

				aggregatedBody.WriteString(dataStr)
				aggregatedBody.WriteString("\n")
			} else if strings.HasPrefix(line, "event: ") {
				chunk.Event = strings.TrimPrefix(line, "event: ")
			}

			chunks = append(chunks, chunk)
		}

		if err := scanner.Err(); err != nil {
			result.Status = "error"
			result.ErrorMessage = err.Error()
		}

		result.ResponseBody = aggregatedBody.String()
		chunksBytes, _ := json.Marshal(chunks)
		result.ResponseChunks = string(chunksBytes)
	} else {
		// 非流式响应
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			result.Status = "error"
			result.ErrorMessage = err.Error()
			result.DurationMs = int(time.Since(startTime).Milliseconds())
			return result, err
		}
		result.ResponseBody = string(responseBody)
	}

	// 计算总耗时
	result.DurationMs = int(time.Since(startTime).Milliseconds())

	// 检查状态码
	if resp.StatusCode >= 400 {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("HTTP error: %d", resp.StatusCode)
	}

	return result, nil
}
