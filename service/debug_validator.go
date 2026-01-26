package service

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/model"

	"github.com/xeipuuv/gojsonschema"
)

// ResponseValidator 响应验证器
type ResponseValidator struct {
	schema           map[string]interface{}
	streamValidation *model.StreamValidationConfig
}

// NewResponseValidator 创建响应验证器
func NewResponseValidator(schema map[string]interface{}, streamValidation *model.StreamValidationConfig) *ResponseValidator {
	return &ResponseValidator{
		schema:           schema,
		streamValidation: streamValidation,
	}
}

// ValidateResponse 验证非流式响应
func (v *ResponseValidator) ValidateResponse(responseBody string) *model.ValidationResult {
	result := &model.ValidationResult{
		Valid:       true,
		SchemaValid: true,
		StreamValid: true,
		Errors:      make([]string, 0),
		Details:     make(map[string]interface{}),
	}

	// 如果没有 schema，跳过验证
	if v.schema == nil || len(v.schema) == 0 {
		result.Details["message"] = "No schema provided, validation skipped"
		return result
	}

	// 解析响应体
	var responseData interface{}
	if err := json.Unmarshal([]byte(responseBody), &responseData); err != nil {
		result.Valid = false
		result.SchemaValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Invalid JSON response: %v", err))
		return result
	}

	// 使用 gojsonschema 验证
	schemaLoader := gojsonschema.NewGoLoader(v.schema)
	documentLoader := gojsonschema.NewGoLoader(responseData)

	validationResult, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		result.Valid = false
		result.SchemaValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Schema validation error: %v", err))
		return result
	}

	if !validationResult.Valid() {
		result.Valid = false
		result.SchemaValid = false
		for _, err := range validationResult.Errors() {
			result.Errors = append(result.Errors, err.String())
		}
	}

	result.Details["schema_validation"] = validationResult.Valid()
	return result
}

// ValidateStreamResponse 验证流式响应
func (v *ResponseValidator) ValidateStreamResponse(chunks []model.StreamChunk) *model.ValidationResult {
	result := &model.ValidationResult{
		Valid:       true,
		SchemaValid: true,
		StreamValid: true,
		Errors:      make([]string, 0),
		Details:     make(map[string]interface{}),
	}

	// 如果没有流式验证配置，跳过验证
	if v.streamValidation == nil || !v.streamValidation.Enabled {
		result.Details["message"] = "Stream validation not enabled, validation skipped"
		return result
	}

	if len(chunks) == 0 {
		result.Valid = false
		result.StreamValid = false
		result.Errors = append(result.Errors, "No stream chunks received")
		return result
	}

	// 创建事件类型到 schema 的映射
	eventSchemas := make(map[string]map[string]interface{})
	for _, rule := range v.streamValidation.Rules {
		eventSchemas[rule.Event] = rule.Schema
	}

	// 验证每个 chunk
	chunkResults := make([]map[string]interface{}, 0)
	for i, chunk := range chunks {
		chunkResult := v.validateStreamChunk(chunk, eventSchemas, i)
		chunkResults = append(chunkResults, chunkResult)

		if !chunkResult["valid"].(bool) {
			result.Valid = false
			result.StreamValid = false
			if errors, ok := chunkResult["errors"].([]string); ok {
				result.Errors = append(result.Errors, errors...)
			}
		}
	}

	result.Details["chunk_validations"] = chunkResults
	result.Details["total_chunks"] = len(chunks)

	return result
}

// validateStreamChunk 验证单个流式响应块
func (v *ResponseValidator) validateStreamChunk(chunk model.StreamChunk, eventSchemas map[string]map[string]interface{}, index int) map[string]interface{} {
	result := map[string]interface{}{
		"index":  index,
		"event":  chunk.Event,
		"valid":  true,
		"errors": make([]string, 0),
	}

	// 如果没有事件类型，跳过验证
	if chunk.Event == "" {
		result["message"] = "No event type, validation skipped"
		return result
	}

	// 查找对应的 schema
	schema, exists := eventSchemas[chunk.Event]
	if !exists {
		result["valid"] = false
		result["errors"] = []string{fmt.Sprintf("No schema defined for event type: %s", chunk.Event)}
		return result
	}

	// 验证数据
	schemaLoader := gojsonschema.NewGoLoader(schema)
	documentLoader := gojsonschema.NewGoLoader(chunk.Data)

	validationResult, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		result["valid"] = false
		result["errors"] = []string{fmt.Sprintf("Schema validation error: %v", err)}
		return result
	}

	if !validationResult.Valid() {
		result["valid"] = false
		errors := make([]string, 0)
		for _, err := range validationResult.Errors() {
			errors = append(errors, err.String())
		}
		result["errors"] = errors
	}

	return result
}

// ValidatorRegistry 验证器注册表（可扩展）
type ValidatorRegistry struct {
	validators map[string]Validator
}

// Validator 验证器接口
type Validator interface {
	Validate(data interface{}) (bool, []string)
	Name() string
}

// NewValidatorRegistry 创建验证器注册表
func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{
		validators: make(map[string]Validator),
	}
}

// Register 注册验证器
func (r *ValidatorRegistry) Register(validator Validator) {
	r.validators[validator.Name()] = validator
}

// Validate 执行所有验证器
func (r *ValidatorRegistry) Validate(data interface{}) (bool, []string) {
	allValid := true
	allErrors := make([]string, 0)

	for _, validator := range r.validators {
		valid, errors := validator.Validate(data)
		if !valid {
			allValid = false
			allErrors = append(allErrors, errors...)
		}
	}

	return allValid, allErrors
}

// CustomValidator 自定义验证器示例
type CustomValidator struct {
	name     string
	validate func(interface{}) (bool, []string)
}

func (v *CustomValidator) Validate(data interface{}) (bool, []string) {
	return v.validate(data)
}

func (v *CustomValidator) Name() string {
	return v.name
}

// NewCustomValidator 创建自定义验证器
func NewCustomValidator(name string, validateFunc func(interface{}) (bool, []string)) *CustomValidator {
	return &CustomValidator{
		name:     name,
		validate: validateFunc,
	}
}

// StreamEventOrderValidator 流式事件顺序验证器
type StreamEventOrderValidator struct {
	expectedOrder []string
}

func NewStreamEventOrderValidator(expectedOrder []string) *StreamEventOrderValidator {
	return &StreamEventOrderValidator{
		expectedOrder: expectedOrder,
	}
}

func (v *StreamEventOrderValidator) Validate(data interface{}) (bool, []string) {
	chunks, ok := data.([]model.StreamChunk)
	if !ok {
		return false, []string{"Invalid data type for stream event order validation"}
	}

	if len(chunks) < len(v.expectedOrder) {
		return false, []string{fmt.Sprintf("Expected at least %d events, got %d", len(v.expectedOrder), len(chunks))}
	}

	errors := make([]string, 0)
	for i, expectedEvent := range v.expectedOrder {
		if i >= len(chunks) {
			break
		}
		if chunks[i].Event != expectedEvent {
			errors = append(errors, fmt.Sprintf("Event %d: expected '%s', got '%s'", i, expectedEvent, chunks[i].Event))
		}
	}

	return len(errors) == 0, errors
}

func (v *StreamEventOrderValidator) Name() string {
	return "stream_event_order"
}

// StreamEventTypeValidator 流式事件类型验证器
type StreamEventTypeValidator struct {
	allowedTypes map[string]bool
}

func NewStreamEventTypeValidator(allowedTypes []string) *StreamEventTypeValidator {
	typeMap := make(map[string]bool)
	for _, t := range allowedTypes {
		typeMap[t] = true
	}
	return &StreamEventTypeValidator{
		allowedTypes: typeMap,
	}
}

func (v *StreamEventTypeValidator) Validate(data interface{}) (bool, []string) {
	chunks, ok := data.([]model.StreamChunk)
	if !ok {
		return false, []string{"Invalid data type for stream event type validation"}
	}

	errors := make([]string, 0)
	for i, chunk := range chunks {
		if !v.allowedTypes[chunk.Event] {
			errors = append(errors, fmt.Sprintf("Chunk %d: event type '%s' is not allowed", i, chunk.Event))
		}
	}

	return len(errors) == 0, errors
}

func (v *StreamEventTypeValidator) Name() string {
	return "stream_event_type"
}

// RequiredFieldsValidator 必填字段验证器
type RequiredFieldsValidator struct {
	requiredFields []string
}

func NewRequiredFieldsValidator(requiredFields []string) *RequiredFieldsValidator {
	return &RequiredFieldsValidator{
		requiredFields: requiredFields,
	}
}

func (v *RequiredFieldsValidator) Validate(data interface{}) (bool, []string) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return false, []string{"Invalid data type for required fields validation"}
	}

	errors := make([]string, 0)
	for _, field := range v.requiredFields {
		if _, exists := dataMap[field]; !exists {
			errors = append(errors, fmt.Sprintf("Required field '%s' is missing", field))
		}
	}

	return len(errors) == 0, errors
}

func (v *RequiredFieldsValidator) Name() string {
	return "required_fields"
}
