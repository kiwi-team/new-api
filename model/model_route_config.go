package model

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ModelRouteConfig 模型路由配置
type ModelRouteConfig struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	Name          string `json:"name" gorm:"type:varchar(100);not null;index"`        // 配置名称
	ModelPatterns string `json:"model_patterns" gorm:"type:text"`                     // 模型名称正则表达式列表，JSON数组
	BodyPatterns  string `json:"body_patterns" gorm:"type:text"`                      // Body关键词列表，JSON数组
	UrlPatterns   string `json:"url_patterns" gorm:"type:text"`                       // URL关键词列表，JSON数组
	ChannelGroups string `json:"channel_groups" gorm:"type:text;not null"`            // 渠道组配置，JSON数组的数组 [[1,2],[3,4]]
	RandomType    string `json:"random_type" gorm:"type:varchar(20);default:'order'"` // order 或 random
	MaxRetry      int    `json:"max_retry" gorm:"default:3"`                          // 最大重试次数
	Priority      int    `json:"priority" gorm:"default:0;index"`                     // 优先级，数字越大优先级越高
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

	if m.ModelPatterns != "" {
		json.Unmarshal([]byte(m.ModelPatterns), &modelPatterns)
	}
	if m.BodyPatterns != "" {
		json.Unmarshal([]byte(m.BodyPatterns), &bodyPatterns)
	}
	if m.UrlPatterns != "" {
		json.Unmarshal([]byte(m.UrlPatterns), &urlPatterns)
	}
	if m.ChannelGroups != "" {
		json.Unmarshal([]byte(m.ChannelGroups), &channelGroups)
	}

	return json.Marshal(&struct {
		*Alias
		ModelPatterns []string `json:"model_patterns"`
		BodyPatterns  []string `json:"body_patterns"`
		UrlPatterns   []string `json:"url_patterns"`
		ChannelGroups [][]int  `json:"channel_groups"`
	}{
		Alias:         (*Alias)(m),
		ModelPatterns: modelPatterns,
		BodyPatterns:  bodyPatterns,
		UrlPatterns:   urlPatterns,
		ChannelGroups: channelGroups,
	})
}

// GetModelPatterns 获取模型正则表达式列表
func (m *ModelRouteConfig) GetModelPatterns() []string {
	if m.ModelPatterns == "" {
		return []string{}
	}
	var patterns []string
	_ = json.Unmarshal([]byte(m.ModelPatterns), &patterns)
	return patterns
}

// GetBodyPatterns 获取Body关键词列表
func (m *ModelRouteConfig) GetBodyPatterns() []string {
	if m.BodyPatterns == "" {
		return []string{}
	}
	var patterns []string
	_ = json.Unmarshal([]byte(m.BodyPatterns), &patterns)
	return patterns
}

// GetUrlPatterns 获取URL关键词列表
func (m *ModelRouteConfig) GetUrlPatterns() []string {
	if m.UrlPatterns == "" {
		return []string{}
	}
	var patterns []string
	_ = json.Unmarshal([]byte(m.UrlPatterns), &patterns)
	return patterns
}

// GetChannelGroups 获取渠道组配置
func (m *ModelRouteConfig) GetChannelGroups() [][]int {
	if m.ChannelGroups == "" {
		return [][]int{}
	}
	var groups [][]int
	_ = json.Unmarshal([]byte(m.ChannelGroups), &groups)
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

// Insert 插入新配置
func (m *ModelRouteConfig) Insert() error {
	m.CreatedTime = common.GetTimestamp()
	m.UpdatedTime = common.GetTimestamp()
	return DB.Create(m).Error
}

// Update 更新配置
func (m *ModelRouteConfig) Update() error {
	m.UpdatedTime = common.GetTimestamp()
	return DB.Save(m).Error
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
func FindMatchingRouteConfig(modelName, body, url string) (*ModelRouteConfig, error) {
	configs, err := GetEnabledModelRouteConfigs()
	if err != nil {
		return nil, err
	}

	// 按优先级从高到低匹配
	for _, config := range configs {
		if config.MatchModel(modelName) {
			return config, nil
		}
		//(len(body) > 0 && config.MatchBody(body)) &&
		//(len(url) > 0 && config.MatchUrl(url)) {
	}

	return nil, nil // 没有匹配的配置
}

// GetChannelIdsByModel 根据模型名称获取匹配的渠道ID列表
func GetChannelIdsByModel(modelName, body, url string) ([]int, int, error) {
	config, err := FindMatchingRouteConfig(modelName, body, url)
	if err != nil {
		return nil, 0, err
	}

	if config == nil {
		// 没有匹配的路由配置，返回空列表
		return []int{}, 0, nil
	}
	retryTimes := config.MaxRetry

	// 获取渠道组配置
	channelGroups := config.GetChannelGroups()
	if len(channelGroups) == 0 {
		return []int{}, 0, nil
	}

	// 根据随机类型选择渠道组
	var selectedGroup []int
	if config.RandomType == "random" {
		// 所有的渠道组里的渠道ID，全部放到selectedGroup,随机后返回
		for _, group := range channelGroups {
			selectedGroup = append(selectedGroup, group...)
		}
		// 随机打乱所有渠道ID
		common.ShuffleSlice(selectedGroup)
	} else {
		// 渠道组的顺序不变，但是每个渠道组里的渠道ID，要随机,最后把所有的渠道ID，按照渠道组的顺序拼接后返回。
		for _, group := range channelGroups {
			// 复制当前组的渠道ID
			groupCopy := make([]int, len(group))
			copy(groupCopy, group)
			// 随机打乱当前组内的渠道ID
			common.ShuffleSlice(groupCopy)
			// 按顺序拼接到结果中
			selectedGroup = append(selectedGroup, groupCopy...)
		}
	}

	return selectedGroup, retryTimes, nil
}
