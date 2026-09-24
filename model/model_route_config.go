package model

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

const (
	ModelRouteApplyModeFallback = "fallback"
	ModelRouteApplyModeEnforce  = "enforce"

	maxModelRouteBodyMatchBytes  = 32 * 1024
	maxModelRouteBodyMatchDepth  = 16
	maxModelRouteBodyMatchFields = 128
)

// ModelRouteConfig 模型路由配置
type ModelRouteConfig struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	Name          string `json:"name" gorm:"type:varchar(100);not null;index"`        // 配置名称
	ModelPatterns string `json:"model_patterns" gorm:"type:text"`                     // 模型名称正则表达式列表，JSON数组
	BodyPatterns  string `json:"body_patterns" gorm:"type:text"`                      // Body关键词列表，JSON数组
	BodyMatch     string `json:"-" gorm:"type:text"`                                  // 结构化请求体子集匹配，JSON对象
	UrlPatterns   string `json:"url_patterns" gorm:"type:text"`                       // URL关键词列表，JSON数组
	ChannelGroups string `json:"channel_groups" gorm:"type:text;not null"`            // 渠道组配置，JSON数组的数组 [[1,2],[3,4]]
	RandomType    string `json:"random_type" gorm:"type:varchar(20);default:'order'"` // order 或 random
	MaxRetry      int    `json:"max_retry" gorm:"default:3"`                          // 最大重试次数
	Priority      int    `json:"priority" gorm:"default:0;index"`                     // 优先级，数字越大优先级越高
	ApplyMode     string `json:"-" gorm:"type:varchar(20)"`                           // fallback 或 enforce，空值兼容为 fallback
	Enabled       int    `json:"enabled" gorm:"default:1;index"`                      // 是否启用 1启用 0禁用
	CreatedTime   int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime   int64  `json:"updated_time" gorm:"bigint"`
}

func (ModelRouteConfig) TableName() string {
	return "model_route_configs"
}

// MarshalJSON 自定义JSON序列化，将字符串字段转换为数组
func (m *ModelRouteConfig) MarshalJSON() ([]byte, error) {
	type Alias ModelRouteConfig

	// 解析 JSON 字符串字段为数组
	var modelPatterns []string
	var bodyPatterns []string
	var urlPatterns []string
	var channelGroups [][]int
	var bodyMatch common.RawMessage

	if m.ModelPatterns != "" {
		_ = common.Unmarshal([]byte(m.ModelPatterns), &modelPatterns)
	}
	if m.BodyPatterns != "" {
		_ = common.Unmarshal([]byte(m.BodyPatterns), &bodyPatterns)
	}
	if m.UrlPatterns != "" {
		_ = common.Unmarshal([]byte(m.UrlPatterns), &urlPatterns)
	}
	if m.ChannelGroups != "" {
		_ = common.Unmarshal([]byte(m.ChannelGroups), &channelGroups)
	}
	if m.BodyMatch != "" {
		bodyMatch = common.RawMessage(m.BodyMatch)
	}

	return common.Marshal(&struct {
		*Alias
		ModelPatterns []string          `json:"model_patterns"`
		BodyPatterns  []string          `json:"body_patterns"`
		BodyMatch     common.RawMessage `json:"body_match"`
		UrlPatterns   []string          `json:"url_patterns"`
		ChannelGroups [][]int           `json:"channel_groups"`
		ApplyMode     string            `json:"apply_mode"`
	}{
		Alias:         (*Alias)(m),
		ModelPatterns: modelPatterns,
		BodyPatterns:  bodyPatterns,
		BodyMatch:     bodyMatch,
		UrlPatterns:   urlPatterns,
		ChannelGroups: channelGroups,
		ApplyMode:     m.GetApplyMode(),
	})
}

func (m *ModelRouteConfig) GetApplyMode() string {
	if m.ApplyMode == ModelRouteApplyModeEnforce {
		return ModelRouteApplyModeEnforce
	}
	return ModelRouteApplyModeFallback
}

// GetModelPatterns 获取模型正则表达式列表
func (m *ModelRouteConfig) GetModelPatterns() []string {
	if m.ModelPatterns == "" {
		return []string{}
	}
	var patterns []string
	_ = common.Unmarshal([]byte(m.ModelPatterns), &patterns)
	return patterns
}

// GetBodyPatterns 获取Body关键词列表
func (m *ModelRouteConfig) GetBodyPatterns() []string {
	if m.BodyPatterns == "" {
		return []string{}
	}
	var patterns []string
	_ = common.Unmarshal([]byte(m.BodyPatterns), &patterns)
	return patterns
}

// GetUrlPatterns 获取URL关键词列表
func (m *ModelRouteConfig) GetUrlPatterns() []string {
	if m.UrlPatterns == "" {
		return []string{}
	}
	var patterns []string
	_ = common.Unmarshal([]byte(m.UrlPatterns), &patterns)
	return patterns
}

// GetChannelGroups 获取渠道组配置
func (m *ModelRouteConfig) GetChannelGroups() [][]int {
	if m.ChannelGroups == "" {
		return [][]int{}
	}
	var groups [][]int
	_ = common.Unmarshal([]byte(m.ChannelGroups), &groups)
	return groups
}

// MatchModel 检查模型名称是否匹配
func (m *ModelRouteConfig) MatchModel(modelName string) bool {
	patterns := m.GetModelPatterns()
	if len(patterns) == 0 {
		//return true // 如果没有配置模型匹配规则，则匹配所有
		return false
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, modelName); matched {
			return true
		}
	}
	return false
}

// MatchBody 检查请求体是否匹配
func (m *ModelRouteConfig) MatchBody(body string) bool {
	patterns := m.GetBodyPatterns()
	if len(patterns) == 0 {
		return true // 如果没有配置body匹配规则，则匹配所有
	}

	for _, pattern := range patterns {
		if strings.Contains(body, pattern) {
			return true
		}
	}
	return false
}

// MatchBodyJSON reports whether the request body contains the configured JSON
// object as a recursive subset. Object keys in the request that are absent
// from the template are ignored; arrays are compared exactly.
func (m *ModelRouteConfig) MatchBodyJSON(body string) bool {
	if m.BodyMatch == "" {
		return true
	}
	if body == "" {
		return false
	}
	return modelRouteJSONSubsetMatches(common.RawMessage(m.BodyMatch), common.RawMessage(body))
}

func modelRouteJSONSubsetMatches(expected, actual common.RawMessage) bool {
	expectedType := common.GetJsonType(expected)
	if expectedType != common.GetJsonType(actual) {
		return false
	}

	switch expectedType {
	case "object":
		var expectedObject map[string]common.RawMessage
		var actualObject map[string]common.RawMessage
		if common.Unmarshal(expected, &expectedObject) != nil || common.Unmarshal(actual, &actualObject) != nil {
			return false
		}
		for key, expectedValue := range expectedObject {
			actualValue, exists := actualObject[key]
			if !exists || !modelRouteJSONSubsetMatches(expectedValue, actualValue) {
				return false
			}
		}
		return true
	case "array":
		var expectedArray []common.RawMessage
		var actualArray []common.RawMessage
		if common.Unmarshal(expected, &expectedArray) != nil || common.Unmarshal(actual, &actualArray) != nil || len(expectedArray) != len(actualArray) {
			return false
		}
		for i := range expectedArray {
			if !modelRouteJSONSubsetMatches(expectedArray[i], actualArray[i]) {
				return false
			}
		}
		return true
	case "string":
		var expectedString string
		var actualString string
		return common.Unmarshal(expected, &expectedString) == nil &&
			common.Unmarshal(actual, &actualString) == nil &&
			expectedString == actualString
	case "number":
		expectedNumber, expectedErr := decimal.NewFromString(string(bytes.TrimSpace(expected)))
		actualNumber, actualErr := decimal.NewFromString(string(bytes.TrimSpace(actual)))
		return expectedErr == nil && actualErr == nil && expectedNumber.Equal(actualNumber)
	case "boolean", "null":
		return bytes.Equal(bytes.TrimSpace(expected), bytes.TrimSpace(actual))
	default:
		return false
	}
}

// ValidateModelRouteBodyMatch validates the persisted structured body matcher.
func ValidateModelRouteBodyMatch(raw string) error {
	trimmed := bytes.TrimSpace([]byte(raw))
	if len(trimmed) == 0 {
		return nil
	}
	if len(trimmed) > maxModelRouteBodyMatchBytes {
		return fmt.Errorf("body_match exceeds %d bytes", maxModelRouteBodyMatchBytes)
	}
	if common.GetJsonType(trimmed) != "object" {
		return errors.New("body_match must be a JSON object")
	}

	var root map[string]common.RawMessage
	if err := common.Unmarshal(trimmed, &root); err != nil {
		return fmt.Errorf("invalid body_match JSON: %w", err)
	}
	if len(root) == 0 {
		return errors.New("body_match cannot be an empty object")
	}

	fieldCount := 0
	var validate func(common.RawMessage, int) error
	validate = func(value common.RawMessage, depth int) error {
		if depth > maxModelRouteBodyMatchDepth {
			return fmt.Errorf("body_match exceeds maximum depth %d", maxModelRouteBodyMatchDepth)
		}
		switch common.GetJsonType(value) {
		case "object":
			var object map[string]common.RawMessage
			if err := common.Unmarshal(value, &object); err != nil {
				return err
			}
			fieldCount += len(object)
			if fieldCount > maxModelRouteBodyMatchFields {
				return fmt.Errorf("body_match exceeds maximum field count %d", maxModelRouteBodyMatchFields)
			}
			for _, child := range object {
				if err := validate(child, depth+1); err != nil {
					return err
				}
			}
		case "array":
			var values []common.RawMessage
			if err := common.Unmarshal(value, &values); err != nil {
				return err
			}
			for _, child := range values {
				if err := validate(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return validate(trimmed, 1)
}

// MatchUrl 检查URL是否匹配
func (m *ModelRouteConfig) MatchUrl(url string) bool {
	patterns := m.GetUrlPatterns()
	if len(patterns) == 0 {
		return true // 如果没有配置URL匹配规则，则匹配所有
	}

	for _, pattern := range patterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}
	return false
}

// MatchesRequest requires every configured request dimension to match.
func (m *ModelRouteConfig) MatchesRequest(modelName, body, url string) bool {
	return m.MatchModel(modelName) && m.MatchBody(body) && m.MatchBodyJSON(body) && m.MatchUrl(url)
}

// Insert 插入新配置
func (m *ModelRouteConfig) Insert() error {
	m.CreatedTime = common.GetTimestamp()
	m.UpdatedTime = common.GetTimestamp()
	return DB.Create(m).Error
}

// Update 更新配置
func (m *ModelRouteConfig) Update() error {
	var count int64
	if err := DB.Model(&ModelRouteConfig{}).Where("id = ?", m.Id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errors.New("model route config not found")
	}

	m.UpdatedTime = common.GetTimestamp()
	result := DB.Model(&ModelRouteConfig{}).Where("id = ?", m.Id).Select(
		"name", "model_patterns", "body_patterns", "body_match", "url_patterns",
		"channel_groups", "random_type", "max_retry", "priority", "apply_mode",
		"enabled", "updated_time",
	).Updates(m)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// Delete 删除配置
func (m *ModelRouteConfig) Delete() error {
	return DB.Delete(m).Error
}

// GetAllModelRouteConfigs 获取所有配置（按优先级排序）
func GetAllModelRouteConfigs(startIdx int, num int) ([]*ModelRouteConfig, int64, error) {
	var configs []*ModelRouteConfig
	var total int64

	err := DB.Model(&ModelRouteConfig{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = DB.Order("priority DESC, id DESC").Limit(num).Offset(startIdx).Find(&configs).Error
	return configs, total, err
}

// GetEnabledModelRouteConfigs 获取所有启用的配置（按优先级排序）
func GetEnabledModelRouteConfigs() ([]*ModelRouteConfig, error) {
	var configs []*ModelRouteConfig
	err := DB.Where("enabled = ?", 1).Order("priority DESC, id DESC").Find(&configs).Error
	return configs, err
}

// GetModelRouteConfigById 根据ID获取配置
func GetModelRouteConfigById(id int) (*ModelRouteConfig, error) {
	config := &ModelRouteConfig{}
	err := DB.First(config, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return config, nil
}

// SearchModelRouteConfigs 搜索配置
func SearchModelRouteConfigs(keyword string, modelKeyword string, channelId int, startIdx int, num int) ([]*ModelRouteConfig, int64, error) {
	var configs []*ModelRouteConfig
	var total int64

	query := DB.Model(&ModelRouteConfig{})

	// 按名称搜索
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	// 按模型关键词搜索
	if modelKeyword != "" {
		query = query.Where("model_patterns LIKE ?", "%"+modelKeyword+"%")
	}

	// 按渠道ID搜索
	if channelId > 0 {
		// 搜索 channel_groups 中包含该渠道ID的配置
		// 例如: [[1,2],[3,4]] 包含渠道 2
		channelIdStr := strconv.Itoa(channelId)
		query = query.Where("channel_groups LIKE ?", "%"+channelIdStr+"%")
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("priority DESC, id DESC").Limit(num).Offset(startIdx).Find(&configs).Error
	return configs, total, err
}

// BatchDeleteModelRouteConfigs 批量删除配置
func BatchDeleteModelRouteConfigs(ids []int) error {
	if len(ids) == 0 {
		return errors.New("ids cannot be empty")
	}
	return DB.Where("id IN ?", ids).Delete(&ModelRouteConfig{}).Error
}

// UpdateModelRouteConfigStatus 更新配置状态
func UpdateModelRouteConfigStatus(id int, enabled int) error {
	return DB.Model(&ModelRouteConfig{}).Where("id = ?", id).Update("enabled", enabled).Error
}

// FindMatchingRouteConfig 查找匹配的路由配置
func FindMatchingRouteConfig(modelName, body, url string, applyMode ...string) (*ModelRouteConfig, error) {
	configs, err := GetEnabledModelRouteConfigs()
	if err != nil {
		return nil, err
	}

	wantedMode := ""
	if len(applyMode) > 0 {
		wantedMode = applyMode[0]
	}

	// 按优先级从高到低匹配
	for _, config := range configs {
		if wantedMode != "" && config.GetApplyMode() != wantedMode {
			continue
		}
		if config.MatchesRequest(modelName, body, url) {
			return config, nil
		}
	}

	return nil, nil // 没有匹配的配置
}

type ModelRouteMatch struct {
	Matched    bool
	ConfigID   int
	ConfigName string
	ApplyMode  string
	ChannelIDs []int
	MaxRetry   int
}

// GetChannelRouteByModel resolves one matching rule in the requested mode and
// preserves whether a rule matched even when it produced no channel IDs.
func GetChannelRouteByModel(modelName, body, url, applyMode string) (*ModelRouteMatch, error) {
	config, err := FindMatchingRouteConfig(modelName, body, url, applyMode)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return &ModelRouteMatch{ApplyMode: applyMode}, nil
	}

	result := &ModelRouteMatch{
		Matched:    true,
		ConfigID:   config.Id,
		ConfigName: config.Name,
		ApplyMode:  config.GetApplyMode(),
		MaxRetry:   config.MaxRetry,
	}
	channelGroups := config.GetChannelGroups()
	if config.RandomType == "random" {
		for _, group := range channelGroups {
			result.ChannelIDs = append(result.ChannelIDs, group...)
		}
		common.ShuffleSlice(result.ChannelIDs)
		return result, nil
	}

	for _, group := range channelGroups {
		groupCopy := append([]int(nil), group...)
		common.ShuffleSlice(groupCopy)
		result.ChannelIDs = append(result.ChannelIDs, groupCopy...)
	}
	return result, nil
}

// GetChannelIdsByModel 根据模型名称获取匹配的渠道ID列表
func GetChannelIdsByModel(modelName, body, url string) ([]int, int, error) {
	match, err := GetChannelRouteByModel(modelName, body, url, ModelRouteApplyModeFallback)
	if err != nil {
		return nil, 0, err
	}
	if !match.Matched {
		// 没有匹配的路由配置，返回空列表
		return []int{}, 0, nil
	}
	return match.ChannelIDs, match.MaxRetry, nil
}
