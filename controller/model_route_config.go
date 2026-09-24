package controller

import (
	"bytes"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetAllModelRouteConfigs 获取所有模型路由配置
func GetAllModelRouteConfigs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	configs, total, err := model.GetAllModelRouteConfigs(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     configs,
			"total":     total,
			"page":      pageInfo.GetPage(),
			"page_size": pageInfo.GetPageSize(),
		},
	})
}

// SearchModelRouteConfigs 搜索模型路由配置
func SearchModelRouteConfigs(c *gin.Context) {
	keyword := c.Query("keyword")
	modelKeyword := c.Query("model_keyword")
	channelIdStr := c.Query("channel_id")
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	channelId := 0
	if channelIdStr != "" {
		channelId, _ = strconv.Atoi(channelIdStr)
	}

	startIdx := (page - 1) * pageSize
	configs, total, err := model.SearchModelRouteConfigs(keyword, modelKeyword, channelId, startIdx, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     configs,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetModelRouteConfig 获取单个模型路由配置
func GetModelRouteConfig(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的ID",
		})
		return
	}

	config, err := model.GetModelRouteConfigById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config,
	})
}

// validateModelRouteConfig 验证配置数据
func validateModelRouteConfig(config *model.ModelRouteConfig) error {
	// 验证名称
	if config.Name == "" {
		return errors.New("配置名称不能为空")
	}

	// 验证渠道组配置
	if config.ChannelGroups == "" {
		return errors.New("渠道组配置不能为空")
	}

	// 验证JSON格式
	var groups [][]int
	if err := common.Unmarshal([]byte(config.ChannelGroups), &groups); err != nil {
		return errors.New("渠道组配置格式错误，必须是二维数组")
	}

	if len(groups) == 0 {
		return errors.New("至少需要配置一个渠道组")
	}
	seenChannelIds := make(map[int]struct{})
	for _, group := range groups {
		if len(group) == 0 {
			return errors.New("每个渠道组至少需要一个渠道")
		}
		for _, channelId := range group {
			if channelId <= 0 {
				return errors.New("渠道ID必须是正整数")
			}
			if _, exists := seenChannelIds[channelId]; exists {
				return errors.New("同一个渠道不能重复出现在渠道组中")
			}
			seenChannelIds[channelId] = struct{}{}
		}
	}

	// 验证模型匹配规则
	if config.ModelPatterns != "" {
		var patterns []string
		if err := common.Unmarshal([]byte(config.ModelPatterns), &patterns); err != nil {
			return errors.New("模型匹配规则格式错误，必须是字符串数组")
		}
		if len(patterns) == 0 {
			return errors.New("至少需要配置一个模型名称规则")
		}
		for _, pattern := range patterns {
			if _, err := regexp.Compile(pattern); err != nil {
				return errors.New("模型名称规则包含无效的正则表达式")
			}
		}
	} else {
		return errors.New("至少需要配置一个模型名称规则")
	}

	// 验证Body匹配规则
	if config.BodyPatterns != "" {
		var patterns []string
		if err := common.Unmarshal([]byte(config.BodyPatterns), &patterns); err != nil {
			return errors.New("Body匹配规则格式错误，必须是字符串数组")
		}
	}
	if config.BodyPatterns != "" && config.BodyMatch != "" {
		return errors.New("请求体关键词和JSON字段匹配不能同时配置")
	}
	if err := model.ValidateModelRouteBodyMatch(config.BodyMatch); err != nil {
		return errors.New("请求体JSON字段匹配无效: " + err.Error())
	}

	// 验证URL匹配规则
	if config.UrlPatterns != "" {
		var patterns []string
		if err := common.Unmarshal([]byte(config.UrlPatterns), &patterns); err != nil {
			return errors.New("URL匹配规则格式错误，必须是字符串数组")
		}
	}

	// 验证随机类型
	if config.RandomType != "order" && config.RandomType != "random" {
		return errors.New("随机类型必须是 order 或 random")
	}

	// 验证重试次数
	if config.MaxRetry < 0 {
		return errors.New("最大重试次数不能为负数")
	}
	if config.ApplyMode != model.ModelRouteApplyModeFallback && config.ApplyMode != model.ModelRouteApplyModeEnforce {
		return errors.New("应用模式必须是 fallback 或 enforce")
	}

	return nil
}

// ModelRouteConfigDTO 用于接收前端数据的DTO
type ModelRouteConfigDTO struct {
	Id            int               `json:"id"`
	Name          string            `json:"name"`
	ModelPatterns []string          `json:"model_patterns"`
	BodyPatterns  []string          `json:"body_patterns"`
	BodyMatch     common.RawMessage `json:"body_match"`
	UrlPatterns   []string          `json:"url_patterns"`
	ChannelGroups [][]int           `json:"channel_groups"` // 前端发送的是二维数组
	RandomType    string            `json:"random_type"`
	MaxRetry      int               `json:"max_retry"`
	Priority      int               `json:"priority"`
	ApplyMode     string            `json:"apply_mode"`
	Enabled       int               `json:"enabled"`
}

// ToModel 将DTO转换为Model
func (dto *ModelRouteConfigDTO) ToModel() (*model.ModelRouteConfig, error) {
	bodyMatch := bytes.TrimSpace(dto.BodyMatch)
	if bytes.Equal(bodyMatch, []byte("null")) {
		bodyMatch = nil
	}
	applyMode := dto.ApplyMode
	if applyMode == "" {
		applyMode = model.ModelRouteApplyModeFallback
		if len(bodyMatch) > 0 {
			applyMode = model.ModelRouteApplyModeEnforce
		}
	}
	config := &model.ModelRouteConfig{
		Id:         dto.Id,
		Name:       dto.Name,
		RandomType: dto.RandomType,
		MaxRetry:   dto.MaxRetry,
		Priority:   dto.Priority,
		ApplyMode:  applyMode,
		Enabled:    dto.Enabled,
		BodyMatch:  string(bodyMatch),
	}

	// 转换 ModelPatterns
	if len(dto.ModelPatterns) > 0 {
		data, err := common.Marshal(dto.ModelPatterns)
		if err != nil {
			return nil, errors.New("模型匹配规则格式错误")
		}
		config.ModelPatterns = string(data)
	}

	// 转换 BodyPatterns
	if len(dto.BodyPatterns) > 0 {
		data, err := common.Marshal(dto.BodyPatterns)
		if err != nil {
			return nil, errors.New("请求体关键词格式错误")
		}
		config.BodyPatterns = string(data)
	}

	// 转换 UrlPatterns
	if len(dto.UrlPatterns) > 0 {
		data, err := common.Marshal(dto.UrlPatterns)
		if err != nil {
			return nil, errors.New("URL路径匹配格式错误")
		}
		config.UrlPatterns = string(data)
	}

	// 转换 ChannelGroups
	if len(dto.ChannelGroups) > 0 {
		data, err := common.Marshal(dto.ChannelGroups)
		if err != nil {
			return nil, errors.New("渠道组配置格式错误")
		}
		config.ChannelGroups = string(data)
	}

	return config, nil
}

// AddModelRouteConfig 添加模型路由配置
func AddModelRouteConfig(c *gin.Context) {
	dto := &ModelRouteConfigDTO{}
	err := c.ShouldBindJSON(dto)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	// 转换为 Model
	config, err := dto.ToModel()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证配置
	if err := validateModelRouteConfig(config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	err = config.Insert()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "添加成功",
		"data":    config,
	})
}

// UpdateModelRouteConfig 更新模型路由配置
func UpdateModelRouteConfig(c *gin.Context) {
	dto := &ModelRouteConfigDTO{}
	err := c.ShouldBindJSON(dto)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	if dto.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "ID不能为空",
		})
		return
	}

	// 转换为 Model
	config, err := dto.ToModel()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证配置
	if err := validateModelRouteConfig(config); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	err = config.Update()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	config, err = model.GetModelRouteConfigById(config.Id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
		"data":    config,
	})
}

// DeleteModelRouteConfig 删除模型路由配置
func DeleteModelRouteConfig(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的ID",
		})
		return
	}

	config := &model.ModelRouteConfig{Id: id}
	err = config.Delete()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// BatchDeleteModelRouteConfigs 批量删除模型路由配置
func BatchDeleteModelRouteConfigs(c *gin.Context) {
	var req struct {
		Ids []int `json:"ids"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil || len(req.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	err = model.BatchDeleteModelRouteConfigs(req.Ids)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "批量删除成功",
		"data":    len(req.Ids),
	})
}

// UpdateModelRouteConfigStatus 更新模型路由配置状态
func UpdateModelRouteConfigStatus(c *gin.Context) {
	var req struct {
		Id      int `json:"id"`
		Enabled int `json:"enabled"`
	}

	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	if req.Enabled != 0 && req.Enabled != 1 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "状态值必须是 0 或 1",
		})
		return
	}

	err = model.UpdateModelRouteConfigStatus(req.Id, req.Enabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "状态更新成功",
	})
}
