package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	SyncTypeChannel    = "channel"
	SyncTypeModelPrice = "model_price"

	SyncStatusSuccess = 1
	SyncStatusFailed  = 2
)

// SyncResult 单个环境的同步结果
type SyncResult struct {
	EnvironmentId   int                 `json:"environment_id"`
	EnvironmentName string              `json:"environment_name"`
	Success         bool                `json:"success"`
	Error           string              `json:"error,omitempty"`
	SyncedCount     int                 `json:"synced_count,omitempty"`
	Details         []SyncChannelDetail `json:"details,omitempty"`
}

// SyncChannelDetail 渠道同步详情
type SyncChannelDetail struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Action      string `json:"action"` // created/updated
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// SyncClient 同步客户端
type SyncClient struct {
	env        *model.SyncEnvironment
	httpClient *http.Client
}

// NewSyncClient 创建同步客户端
func NewSyncClient(env *model.SyncEnvironment) *SyncClient {
	return &SyncClient{
		env: env,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest 发送HTTP请求到目标环境
func (c *SyncClient) doRequest(method, path string, body interface{}) ([]byte, error) {
	url := strings.TrimRight(c.env.ApiUrl, "/") + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.env.RootToken)
	req.Header.Set("new-api-user", c.env.NewApiUser)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// TestConnection 测试环境连接
func (c *SyncClient) TestConnection() error {
	_, err := c.doRequest("GET", "/api/status", nil)
	return err
}

// getFirstKey 从可能包含多个key的字符串中提取第一个key用于搜索
func getFirstKey(key string) string {
	if idx := strings.Index(key, "\n"); idx > 0 {
		return strings.TrimSpace(key[:idx])
	}
	return key
}

// SearchChannelByKeyAndType 在目标环境按key和type查询渠道
func (c *SyncClient) SearchChannelByKeyAndType(key string, channelType int) (*model.Channel, error) {
	// 使用第一个key进行搜索，避免多key场景下URL中包含换行符
	searchKey := getFirstKey(key)
	respBody, err := c.doRequest("GET", fmt.Sprintf("/api/channel/search?keyword=%s", url.QueryEscape(searchKey)), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Items []*model.Channel `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return nil, nil
	}

	// 按key和type精确匹配
	for _, ch := range result.Data.Items {
		if ch.Key == key && ch.Type == channelType {
			return ch, nil
		}
	}

	return nil, nil
}

// SearchAllChannelsByKeyAndType 在目标环境按key和type查询所有匹配的渠道
func (c *SyncClient) SearchAllChannelsByKeyAndType(key string, channelType int) ([]*model.Channel, error) {
	searchKey := getFirstKey(key)
	respBody, err := c.doRequest("GET", fmt.Sprintf("/api/channel/search?keyword=%s", url.QueryEscape(searchKey)), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Items []*model.Channel `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return nil, nil
	}

	// 按key和type精确匹配，返回所有匹配的渠道
	var matched []*model.Channel
	for _, ch := range result.Data.Items {
		if ch.Key == key && ch.Type == channelType {
			matched = append(matched, ch)
		}
	}

	return matched, nil
}

// AddChannelRequest 创建渠道请求
type AddChannelRequest struct {
	Mode    string         `json:"mode"`
	Channel *model.Channel `json:"channel"`
}

// CreateChannel 在目标环境创建渠道
func (c *SyncClient) CreateChannel(channel *model.Channel) error {
	req := AddChannelRequest{
		Mode:    "single",
		Channel: channel,
	}

	respBody, err := c.doRequest("POST", "/api/channel/", req)
	if err != nil {
		return err
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return fmt.Errorf("创建渠道失败: %s", result.Message)
	}

	return nil
}

// UpdateChannel 在目标环境更新渠道
func (c *SyncClient) UpdateChannel(channel *model.Channel) error {
	respBody, err := c.doRequest("PUT", "/api/channel/", channel)
	if err != nil {
		return err
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return fmt.Errorf("更新渠道失败: %s", result.Message)
	}

	return nil
}

// PrepareChannelForSync 准备渠道数据用于同步（排除不需要同步的字段）
func PrepareChannelForSync(channel *model.Channel) *model.Channel {
	syncChannel := &model.Channel{
		Type:               channel.Type,
		Key:                channel.Key,
		OpenAIOrganization: channel.OpenAIOrganization,
		TestModel:          channel.TestModel,
		Status:             channel.Status,
		Name:               channel.Name,
		Weight:             channel.Weight,
		BaseURL:            channel.BaseURL,
		Other:              channel.Other,
		Models:             channel.Models,
		Group:              channel.Group,
		ModelMapping:       channel.ModelMapping,
		Ratio:              channel.Ratio,
		Remark:             channel.Remark,
		StatusCodeMapping:  channel.StatusCodeMapping,
		Priority:           channel.Priority,
		AutoBan:            channel.AutoBan,
		OtherInfo:          channel.OtherInfo,
		Tag:                channel.Tag,
		Setting:            channel.Setting,
		ParamOverride:      channel.ParamOverride,
		HeaderOverride:     channel.HeaderOverride,
		ChannelInfo:        channel.ChannelInfo,
		OtherSettings:      channel.OtherSettings,
	}
	// 排除的字段: Id, UsedQuota, CreatedTime, TestTime, ResponseTime, Balance, BalanceUpdatedTime
	return syncChannel
}

// ChannelMatchInfo 目标环境中匹配到的渠道信息
type ChannelMatchInfo struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Models string `json:"models"`
	Type   int    `json:"type"`
}

// ChannelPreviewItem 单个渠道的预览信息
type ChannelPreviewItem struct {
	ChannelId   int                `json:"channel_id"`
	ChannelName string             `json:"channel_name"`
	Models      string             `json:"models"`
	Matches     []ChannelMatchInfo `json:"matches"` // 目标环境中匹配到的渠道列表
}

// EnvironmentPreview 单个环境的预览结果
type EnvironmentPreview struct {
	EnvironmentId   int                  `json:"environment_id"`
	EnvironmentName string               `json:"environment_name"`
	Channels        []ChannelPreviewItem `json:"channels"`
	Error           string               `json:"error,omitempty"`
}

// PreviewChannelSync 预览渠道同步匹配情况
func PreviewChannelSync(channels []*model.Channel, env *model.SyncEnvironment) *EnvironmentPreview {
	preview := &EnvironmentPreview{
		EnvironmentId:   env.Id,
		EnvironmentName: env.Name,
		Channels:        make([]ChannelPreviewItem, 0, len(channels)),
	}

	client := NewSyncClient(env)

	for _, channel := range channels {
		item := ChannelPreviewItem{
			ChannelId:   channel.Id,
			ChannelName: channel.Name,
			Models:      channel.Models,
			Matches:     make([]ChannelMatchInfo, 0),
		}

		matched, err := client.SearchAllChannelsByKeyAndType(channel.Key, channel.Type)
		if err != nil {
			preview.Error = fmt.Sprintf("查询渠道 %s 失败: %v", channel.Name, err)
			preview.Channels = append(preview.Channels, item)
			continue
		}

		for _, m := range matched {
			item.Matches = append(item.Matches, ChannelMatchInfo{
				Id:     m.Id,
				Name:   m.Name,
				Models: m.Models,
				Type:   m.Type,
			})
		}

		preview.Channels = append(preview.Channels, item)
	}

	return preview
}

// ChannelMapping 渠道映射关系: source_channel_id -> target_channel_id (0表示创建新渠道)
type ChannelMapping map[int]int

// SyncChannelsToEnvironment 同步渠道到指定环境
// channelMapping 可选：指定源渠道到目标渠道的映射关系，nil时使用自动匹配（兼容旧逻辑）
func SyncChannelsToEnvironment(channels []*model.Channel, env *model.SyncEnvironment, channelMapping ChannelMapping) *SyncResult {
	result := &SyncResult{
		EnvironmentId:   env.Id,
		EnvironmentName: env.Name,
		Success:         true,
		Details:         make([]SyncChannelDetail, 0),
	}

	client := NewSyncClient(env)

	for _, channel := range channels {
		detail := SyncChannelDetail{
			ChannelId:   channel.Id,
			ChannelName: channel.Name,
			Success:     true,
		}

		// 准备同步数据
		syncChannel := PrepareChannelForSync(channel)

		var targetChannelId int
		hasMapping := false

		if channelMapping != nil {
			if tid, ok := channelMapping[channel.Id]; ok {
				targetChannelId = tid
				hasMapping = true
			}
		}

		if hasMapping {
			if targetChannelId > 0 {
				// 用户指定了目标渠道，直接更新
				syncChannel.Id = targetChannelId
				err := client.UpdateChannel(syncChannel)
				detail.Action = "updated"
				if err != nil {
					detail.Success = false
					detail.Error = err.Error()
				} else {
					result.SyncedCount++
				}
			} else {
				// targetChannelId == 0，创建新渠道
				err := client.CreateChannel(syncChannel)
				detail.Action = "created"
				if err != nil {
					detail.Success = false
					detail.Error = err.Error()
				} else {
					result.SyncedCount++
				}
			}
		} else {
			// 无映射，使用旧的自动匹配逻辑
			existingChannel, err := client.SearchChannelByKeyAndType(channel.Key, channel.Type)
			if err != nil {
				detail.Success = false
				detail.Error = fmt.Sprintf("查询渠道失败: %v", err)
				result.Details = append(result.Details, detail)
				continue
			}

			if existingChannel != nil {
				syncChannel.Id = existingChannel.Id
				err = client.UpdateChannel(syncChannel)
				detail.Action = "updated"
			} else {
				err = client.CreateChannel(syncChannel)
				detail.Action = "created"
			}

			if err != nil {
				detail.Success = false
				detail.Error = err.Error()
			} else {
				result.SyncedCount++
			}
		}

		result.Details = append(result.Details, detail)
	}

	// 检查是否有失败的
	for _, d := range result.Details {
		if !d.Success {
			result.Success = false
			break
		}
	}

	return result
}

// UpdateOption 更新目标环境的配置项
func (c *SyncClient) UpdateOption(key string, value string) error {
	req := map[string]interface{}{
		"key":   key,
		"value": value,
	}

	respBody, err := c.doRequest("PUT", "/api/option/", req)
	if err != nil {
		return err
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return fmt.Errorf("更新配置失败: %s", result.Message)
	}

	return nil
}

// SyncModelPricesToEnvironment 同步模型价格到指定环境
func SyncModelPricesToEnvironment(modelRatio, modelPrice, completionRatio string, env *model.SyncEnvironment) *SyncResult {
	result := &SyncResult{
		EnvironmentId:   env.Id,
		EnvironmentName: env.Name,
		Success:         true,
	}

	client := NewSyncClient(env)

	// 同步ModelRatio
	if modelRatio != "" {
		if err := client.UpdateOption("ModelRatio", modelRatio); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("同步ModelRatio失败: %v", err)
			return result
		}
	}

	// 同步ModelPrice
	if modelPrice != "" {
		if err := client.UpdateOption("ModelPrice", modelPrice); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("同步ModelPrice失败: %v", err)
			return result
		}
	}

	// 同步CompletionRatio
	if completionRatio != "" {
		if err := client.UpdateOption("CompletionRatio", completionRatio); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("同步CompletionRatio失败: %v", err)
			return result
		}
	}

	return result
}

// GetOption 获取目标环境的配置项
func (c *SyncClient) GetOption(key string) (string, error) {
	respBody, err := c.doRequest("GET", "/api/option/", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Success bool `json:"success"`
		Data    []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return "", fmt.Errorf("获取配置失败")
	}

	for _, opt := range result.Data {
		if opt.Key == key {
			return opt.Value, nil
		}
	}

	return "", nil
}

// SyncModelPricesIncrementalToEnvironment 增量同步模型价格到指定环境（合并模式）
func SyncModelPricesIncrementalToEnvironment(modelRatio, modelPrice, completionRatio string, env *model.SyncEnvironment) *SyncResult {
	result := &SyncResult{
		EnvironmentId:   env.Id,
		EnvironmentName: env.Name,
		Success:         true,
	}

	client := NewSyncClient(env)

	// 增量同步 ModelRatio
	if modelRatio != "" {
		if err := mergeAndUpdateOption(client, "ModelRatio", modelRatio); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("同步ModelRatio失败: %v", err)
			return result
		}
	}

	// 增量同步 ModelPrice
	if modelPrice != "" {
		if err := mergeAndUpdateOption(client, "ModelPrice", modelPrice); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("同步ModelPrice失败: %v", err)
			return result
		}
	}

	// 增量同步 CompletionRatio
	if completionRatio != "" {
		if err := mergeAndUpdateOption(client, "CompletionRatio", completionRatio); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("同步CompletionRatio失败: %v", err)
			return result
		}
	}

	return result
}

// mergeAndUpdateOption 合并并更新配置项
func mergeAndUpdateOption(client *SyncClient, key string, newValue string) error {
	// 获取目标环境的当前配置
	existingValue, err := client.GetOption(key)
	if err != nil {
		return fmt.Errorf("获取目标环境配置失败: %v", err)
	}

	// 解析现有配置
	existingMap := make(map[string]float64)
	if existingValue != "" {
		if err := json.Unmarshal([]byte(existingValue), &existingMap); err != nil {
			// 如果解析失败，使用空map
			existingMap = make(map[string]float64)
		}
	}

	// 解析新配置
	newMap := make(map[string]float64)
	if err := json.Unmarshal([]byte(newValue), &newMap); err != nil {
		return fmt.Errorf("解析新配置失败: %v", err)
	}

	// 合并配置（新值覆盖旧值）
	for k, v := range newMap {
		existingMap[k] = v
	}

	// 序列化合并后的配置
	mergedBytes, err := json.Marshal(existingMap)
	if err != nil {
		return fmt.Errorf("序列化合并配置失败: %v", err)
	}

	// 更新配置
	return client.UpdateOption(key, string(mergedBytes))
}
