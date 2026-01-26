package model

import (
	"encoding/json"
	"time"
)

// DebugRequestTemplate 请求模板
type DebugRequestTemplate struct {
	Id               int       `json:"id" gorm:"primaryKey"`
	Name             string    `json:"name" gorm:"type:varchar(255);not null"`
	Description      string    `json:"description" gorm:"type:text"`
	Method           string    `json:"method" gorm:"type:varchar(10);default:POST"`
	Path             string    `json:"path" gorm:"type:varchar(500);not null"`
	ContentType      string    `json:"content_type" gorm:"type:varchar(100);default:application/json"`
	BodyTemplate     string    `json:"body_template" gorm:"type:text"`
	Headers          string    `json:"headers" gorm:"type:text"`
	Variables        string    `json:"variables" gorm:"type:text"`
	ResponseSchema   string    `json:"response_schema" gorm:"type:text"`
	StreamValidation string    `json:"stream_validation" gorm:"type:text"`
	Tags             string    `json:"tags" gorm:"type:text"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (DebugRequestTemplate) TableName() string {
	return "debug_request_templates"
}

// Variable 变量定义
type Variable struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // string, number, boolean, object, array
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// StreamValidationConfig 流式验证配置
type StreamValidationConfig struct {
	Enabled bool                   `json:"enabled"`
	Rules   []StreamValidationRule `json:"rules"`
}

// StreamValidationRule 流式验证规则
type StreamValidationRule struct {
	Event  string                 `json:"event"`
	Schema map[string]interface{} `json:"schema"`
}

// DebugTestSuite 测试集
type DebugTestSuite struct {
	Id               int       `json:"id" gorm:"primaryKey"`
	Name             string    `json:"name" gorm:"type:varchar(255);not null"`
	Description      string    `json:"description" gorm:"type:text"`
	TemplateIds      string    `json:"template_ids" gorm:"type:text;not null"`
	VariableMappings string    `json:"variable_mappings" gorm:"type:text"`
	Tags             string    `json:"tags" gorm:"type:text"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (DebugTestSuite) TableName() string {
	return "debug_test_suites"
}

// VariableMapping 变量映射
type VariableMapping struct {
	TemplateIndex int                   `json:"template_index"`
	Mappings      []VariableMappingRule `json:"mappings"`
}

// VariableMappingRule 变量映射规则
type VariableMappingRule struct {
	Variable string `json:"variable"` // 目标变量名
	Source   string `json:"source"`   // 数据源，如 prev_response.id
}

// DebugConfiguration 调试配置
type DebugConfiguration struct {
	Id             int       `json:"id" gorm:"primaryKey"`
	Name           string    `json:"name" gorm:"type:varchar(255);not null"`
	Description    string    `json:"description" gorm:"type:text"`
	ChannelId      *int      `json:"channel_id"`
	BaseUrl        string    `json:"base_url" gorm:"type:varchar(500)"`
	ApiKey         string    `json:"api_key" gorm:"type:text"`
	TestSuiteId    *int      `json:"test_suite_id"`
	TemplateId     *int      `json:"template_id"`
	VariableValues string    `json:"variable_values" gorm:"type:text"`
	Tags           string    `json:"tags" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (DebugConfiguration) TableName() string {
	return "debug_configurations"
}

// DebugRequestLog 请求日志
type DebugRequestLog struct {
	Id               int       `json:"id" gorm:"primaryKey"`
	ConfigurationId  *int      `json:"configuration_id"`
	SuiteExecutionId string    `json:"suite_execution_id" gorm:"type:varchar(50)"`
	TemplateId       *int      `json:"template_id"`
	TemplateIndex    int       `json:"template_index" gorm:"default:0"`
	RequestMethod    string    `json:"request_method" gorm:"type:varchar(10)"`
	RequestUrl       string    `json:"request_url" gorm:"type:text"`
	RequestHeaders   string    `json:"request_headers" gorm:"type:text"`
	RequestBody      string    `json:"request_body" gorm:"type:text"`
	ModelName        string    `json:"model_name" gorm:"type:varchar(255)"`
	IsStream         bool      `json:"is_stream" gorm:"default:false"`
	ResponseStatus   int       `json:"response_status"`
	ResponseHeaders  string    `json:"response_headers" gorm:"type:text"`
	ResponseBody     string    `json:"response_body" gorm:"type:text"`
	ResponseChunks   string    `json:"response_chunks" gorm:"type:text"`
	DurationMs       int       `json:"duration_ms"`
	TtfbMs           int       `json:"ttfb_ms"` // Time to first byte
	ErrorMessage     string    `json:"error_message" gorm:"type:text"`
	ValidationResult string    `json:"validation_result" gorm:"type:text"`
	Status           string    `json:"status" gorm:"type:varchar(20)"` // success, failed, error
	Tags             string    `json:"tags" gorm:"type:text"`          // JSON array of tags
	Remark           string    `json:"remark" gorm:"type:text"`        // User remark
	CreatedAt        time.Time `json:"created_at"`
}

func (DebugRequestLog) TableName() string {
	return "debug_request_logs"
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid       bool                   `json:"valid"`
	SchemaValid bool                   `json:"schema_valid"`
	StreamValid bool                   `json:"stream_valid"`
	Errors      []string               `json:"errors,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// StreamChunk 流式响应块
type StreamChunk struct {
	Timestamp time.Time              `json:"timestamp"`
	Event     string                 `json:"event,omitempty"`
	Data      map[string]interface{} `json:"data"`
	Raw       string                 `json:"raw"`
}

// Helper methods for JSON marshaling/unmarshaling

func (t *DebugRequestTemplate) GetHeaders() (map[string]string, error) {
	if t.Headers == "" {
		return make(map[string]string), nil
	}
	var headers map[string]string
	err := json.Unmarshal([]byte(t.Headers), &headers)
	return headers, err
}

func (t *DebugRequestTemplate) SetHeaders(headers map[string]string) error {
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	t.Headers = string(data)
	return nil
}

func (t *DebugRequestTemplate) GetVariables() ([]Variable, error) {
	if t.Variables == "" {
		return []Variable{}, nil
	}
	var variables []Variable
	err := json.Unmarshal([]byte(t.Variables), &variables)
	return variables, err
}

func (t *DebugRequestTemplate) SetVariables(variables []Variable) error {
	data, err := json.Marshal(variables)
	if err != nil {
		return err
	}
	t.Variables = string(data)
	return nil
}

func (t *DebugRequestTemplate) GetTags() ([]string, error) {
	if t.Tags == "" {
		return []string{}, nil
	}
	var tags []string
	err := json.Unmarshal([]byte(t.Tags), &tags)
	return tags, err
}

func (t *DebugRequestTemplate) SetTags(tags []string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	t.Tags = string(data)
	return nil
}

func (t *DebugRequestTemplate) GetResponseSchema() (map[string]interface{}, error) {
	if t.ResponseSchema == "" {
		return nil, nil
	}
	var schema map[string]interface{}
	err := json.Unmarshal([]byte(t.ResponseSchema), &schema)
	return schema, err
}

func (t *DebugRequestTemplate) SetResponseSchema(schema map[string]interface{}) error {
	if schema == nil {
		t.ResponseSchema = ""
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	t.ResponseSchema = string(data)
	return nil
}

func (t *DebugRequestTemplate) GetStreamValidation() (*StreamValidationConfig, error) {
	if t.StreamValidation == "" {
		return nil, nil
	}
	var config StreamValidationConfig
	err := json.Unmarshal([]byte(t.StreamValidation), &config)
	return &config, err
}

func (t *DebugRequestTemplate) SetStreamValidation(config *StreamValidationConfig) error {
	if config == nil {
		t.StreamValidation = ""
		return nil
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	t.StreamValidation = string(data)
	return nil
}

func (s *DebugTestSuite) GetTemplateIds() ([]int, error) {
	if s.TemplateIds == "" {
		return []int{}, nil
	}
	var ids []int
	err := json.Unmarshal([]byte(s.TemplateIds), &ids)
	return ids, err
}

func (s *DebugTestSuite) SetTemplateIds(ids []int) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	s.TemplateIds = string(data)
	return nil
}

func (s *DebugTestSuite) GetVariableMappings() ([]VariableMapping, error) {
	if s.VariableMappings == "" {
		return []VariableMapping{}, nil
	}
	var mappings []VariableMapping
	err := json.Unmarshal([]byte(s.VariableMappings), &mappings)
	return mappings, err
}

func (s *DebugTestSuite) SetVariableMappings(mappings []VariableMapping) error {
	data, err := json.Marshal(mappings)
	if err != nil {
		return err
	}
	s.VariableMappings = string(data)
	return nil
}

func (s *DebugTestSuite) GetTags() ([]string, error) {
	if s.Tags == "" {
		return []string{}, nil
	}
	var tags []string
	err := json.Unmarshal([]byte(s.Tags), &tags)
	return tags, err
}

func (s *DebugTestSuite) SetTags(tags []string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	s.Tags = string(data)
	return nil
}

func (c *DebugConfiguration) GetVariableValues() (map[string]interface{}, error) {
	if c.VariableValues == "" {
		return make(map[string]interface{}), nil
	}
	var values map[string]interface{}
	err := json.Unmarshal([]byte(c.VariableValues), &values)
	return values, err
}

func (c *DebugConfiguration) SetVariableValues(values map[string]interface{}) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	c.VariableValues = string(data)
	return nil
}

func (c *DebugConfiguration) GetTags() ([]string, error) {
	if c.Tags == "" {
		return []string{}, nil
	}
	var tags []string
	err := json.Unmarshal([]byte(c.Tags), &tags)
	return tags, err
}

func (c *DebugConfiguration) SetTags(tags []string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	c.Tags = string(data)
	return nil
}

func (l *DebugRequestLog) GetRequestHeaders() (map[string]string, error) {
	if l.RequestHeaders == "" {
		return make(map[string]string), nil
	}
	var headers map[string]string
	err := json.Unmarshal([]byte(l.RequestHeaders), &headers)
	return headers, err
}

func (l *DebugRequestLog) SetRequestHeaders(headers map[string]string) error {
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	l.RequestHeaders = string(data)
	return nil
}

func (l *DebugRequestLog) GetResponseHeaders() (map[string]string, error) {
	if l.ResponseHeaders == "" {
		return make(map[string]string), nil
	}
	var headers map[string]string
	err := json.Unmarshal([]byte(l.ResponseHeaders), &headers)
	return headers, err
}

func (l *DebugRequestLog) SetResponseHeaders(headers map[string]string) error {
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	l.ResponseHeaders = string(data)
	return nil
}

func (l *DebugRequestLog) GetResponseChunks() ([]StreamChunk, error) {
	if l.ResponseChunks == "" {
		return []StreamChunk{}, nil
	}
	var chunks []StreamChunk
	err := json.Unmarshal([]byte(l.ResponseChunks), &chunks)
	return chunks, err
}

func (l *DebugRequestLog) SetResponseChunks(chunks []StreamChunk) error {
	data, err := json.Marshal(chunks)
	if err != nil {
		return err
	}
	l.ResponseChunks = string(data)
	return nil
}

func (l *DebugRequestLog) GetValidationResult() (*ValidationResult, error) {
	if l.ValidationResult == "" {
		return nil, nil
	}
	var result ValidationResult
	err := json.Unmarshal([]byte(l.ValidationResult), &result)
	return &result, err
}

func (l *DebugRequestLog) SetValidationResult(result *ValidationResult) error {
	if result == nil {
		l.ValidationResult = ""
		return nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	l.ValidationResult = string(data)
	return nil
}

func (l *DebugRequestLog) GetTags() ([]string, error) {
	if l.Tags == "" {
		return []string{}, nil
	}
	var tags []string
	err := json.Unmarshal([]byte(l.Tags), &tags)
	return tags, err
}

func (l *DebugRequestLog) SetTags(tags []string) error {
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	l.Tags = string(data)
	return nil
}
