package model

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id               int    `json:"id"`
	UserID           int    `json:"user_id" gorm:"index"`
	Username         string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName        string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed        int    `json:"token_used" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	TokenName        string `json:"token_name" gorm:"size:64;default:''"`
	Count            int    `json:"count" gorm:"default:0"`
	Quota            int    `json:"quota" gorm:"default:0"`
	TokenId          int    `json:"token_id" gorm:"index"`
	ChannelId        int    `json:"channel_id" gorm:"index"`
	ClientUserId     string `json:"client_user_id" gorm:"index;size:200;default:''"`
	ClientScenairo   string `json:"client_scenairo" gorm:"index;size:200;default:''"`
}

type LogQuotaDataCache struct {
	UserId           int
	Username         string
	ModelName        string
	CreatedAt        int64
	TokenUsed        int
	PromptTokens     int
	CompletionTokens int
	Quota            int
	TokenName        string
	TokenId          int
	ChannelId        int
	ClientUserId     string
	ClientScenairo   string
}

func UpdateQuotaData() {
	// recover
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("UpdateQuotaData panic: %s\n", r))
		}
	}()
	for {
		if common.DataExportEnabled {
			fmt.Println("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(userId int, username string, modelName string, quota int, createdAt int64, tokenUsed int, tokenName string, promptTokens int, completionTokens int, tokenId int, channelId int, clientUserId string, clientScenairo string) {
	key := fmt.Sprintf("%d-%s-%s-%d-%d-%d-%s-%s", userId, username, modelName, tokenId, createdAt, channelId, clientUserId, clientScenairo)
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += quota
		quotaData.TokenUsed += tokenUsed
		quotaData.PromptTokens += promptTokens
		quotaData.CompletionTokens += completionTokens
	} else {
		quotaData = &QuotaData{
			UserID:           userId,
			Username:         username,
			ModelName:        modelName,
			CreatedAt:        createdAt,
			Count:            1,
			Quota:            quota,
			TokenUsed:        tokenUsed,
			TokenName:        tokenName,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TokenId:          tokenId,
			ChannelId:        channelId,
			ClientUserId:     clientUserId,
			ClientScenairo:   clientScenairo,
		}
	}
	CacheQuotaData[key] = quotaData
}

// func LogQuotaData(userId int, username string, modelName string, quota int, createdAt int64, tokenUsed int, tokenName string) {
func LogQuotaData(logQuotaData *LogQuotaDataCache) {
	userId := logQuotaData.UserId
	username := logQuotaData.Username
	modelName := logQuotaData.ModelName
	createdAt := logQuotaData.CreatedAt
	tokenUsed := logQuotaData.TokenUsed
	tokenName := logQuotaData.TokenName
	promptTokens := logQuotaData.PromptTokens
	completionTokens := logQuotaData.CompletionTokens
	quota := logQuotaData.Quota
	tokenId := logQuotaData.TokenId
	channelId := logQuotaData.ChannelId
	clientUserId := logQuotaData.ClientUserId
	clientScenairo := logQuotaData.ClientScenairo
	// 只精确到小时
	createdAt = createdAt - (createdAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(userId, username, modelName, quota, createdAt, tokenUsed, tokenName, promptTokens, completionTokens, tokenId, channelId, clientUserId, clientScenairo)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ? and token_id = ? and channel_id = ? and client_user_id = ? and client_scenairo = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.TokenId, quotaData.ChannelId, quotaData.ClientUserId, quotaData.ClientScenairo).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.Count, quotaData.Quota, quotaData.CreatedAt, quotaData.TokenUsed, quotaData.TokenId, quotaData.ChannelId, quotaData.PromptTokens, quotaData.CompletionTokens, quotaData.ClientUserId, quotaData.ClientScenairo)
			_ = IncreaseCliendUserUsedQuota(quotaData.ClientUserId, quotaData.Quota)
		} else {
			DB.Table("quota_data").Create(quotaData)
			_ = IncreaseCliendUserUsedQuota(quotaData.ClientUserId, quotaData.Quota)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据\n", size))
}

type QuotaDataStatistics struct {
	Date            string  `json:"date"`
	ClientUserId    string  `json:"client_user_id"`
	ModelName       string  `json:"model_name"`
	TotalCount      int64   `json:"total_count"`
	TotalQuota      float64 `json:"total_quota"`
	TotalPrompt     int64   `json:"total_prompt"`
	TotalCompletion int64   `json:"total_completion"`
	FixedQuota      int     `json:"fixed_quota"`
	TempQuota       int     `json:"temp_quota"`
}

func GetQuotaDataStatistics(startTime int64, endTime int64, modelName string, clientUserId string, clientScenairos string, expandModels bool, expandDates bool, userId int) ([]*QuotaDataStatistics, error) {
	statistics := make([]*QuotaDataStatistics, 0)
	var err error

	// Date logic based on DB type
	dateField := ""
	if common.UsingSQLite {
		dateField = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch', '+8 hours'))"
	} else if common.UsingMySQL {
		dateField = "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d')"
	} else if common.UsingPostgreSQL {
		dateField = "TO_CHAR(TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	} else {
		dateField = "DATE(created_at)"
	}

	var selectFields string
	if expandDates {
		if expandModels {
			selectFields = dateField + " as date, client_user_id, model_name, sum(count) as total_count, sum(quota) as total_quota, sum(prompt_tokens) as total_prompt, sum(completion_tokens) as total_completion"
		} else {
			selectFields = dateField + " as date, client_user_id, '' as model_name, sum(count) as total_count, sum(quota) as total_quota, sum(prompt_tokens) as total_prompt, sum(completion_tokens) as total_completion"
		}
	} else {
		if expandModels {
			selectFields = "'' as date, client_user_id, model_name, sum(count) as total_count, sum(quota) as total_quota, sum(prompt_tokens) as total_prompt, sum(completion_tokens) as total_completion"
		} else {
			selectFields = "'' as date, client_user_id, '' as model_name, sum(count) as total_count, sum(quota) as total_quota, sum(prompt_tokens) as total_prompt, sum(completion_tokens) as total_completion"
		}
	}
	tx := DB.Model(&QuotaData{}).
		Select(selectFields).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime)

	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if clientUserId != "" {
		tx = tx.Where("client_user_id LIKE ?", "%"+clientUserId+"%")
	}
	// 处理多选scenairo筛选
	if clientScenairos != "" {
		scenairoList := strings.Split(clientScenairos, ",")
		// 已知的三个场景（不含空值）
		knownScenairos := []string{"PersonalExperiment", "ReleaseEvaluation", "DailyExternalModelEvaluation"}
		hasOther := false
		hasPersonalExperiment := false
		normalScenairos := make([]string, 0)
		for _, s := range scenairoList {
			s = strings.TrimSpace(s)
			if s == "Other" {
				hasOther = true
			} else if s == "PersonalExperiment" {
				hasPersonalExperiment = true
				normalScenairos = append(normalScenairos, s)
			} else if s != "" {
				normalScenairos = append(normalScenairos, s)
			}
		}

		// 构建查询条件
		var conditions []string
		var args []interface{}

		// 个人实验：包含 PersonalExperiment 和空值
		if hasPersonalExperiment {
			conditions = append(conditions, "(client_scenairo = ? OR client_scenairo = '')")
			args = append(args, "PersonalExperiment")
		}

		// 其他普通场景（发版评测、日常外部模型评测）
		otherNormalScenairos := make([]string, 0)
		for _, s := range normalScenairos {
			if s != "PersonalExperiment" {
				otherNormalScenairos = append(otherNormalScenairos, s)
			}
		}
		if len(otherNormalScenairos) > 0 {
			conditions = append(conditions, "client_scenairo IN ?")
			args = append(args, otherNormalScenairos)
		}

		// "其他"：不在三个已知场景中，且不为空
		if hasOther {
			conditions = append(conditions, "(client_scenairo NOT IN ? AND client_scenairo != '')")
			args = append(args, knownScenairos)
		}

		if len(conditions) > 0 {
			combinedCondition := "(" + strings.Join(conditions, " OR ") + ")"
			tx = tx.Where(combinedCondition, args...)
		}
	}
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}

	if expandDates {
		if expandModels {
			err = tx.Group("date, client_user_id, model_name").
				Order("date DESC").
				Scan(&statistics).Error
		} else {
			err = tx.Group("date, client_user_id").
				Order("date DESC").
				Scan(&statistics).Error
		}
	} else {
		if expandModels {
			err = tx.Group("client_user_id, model_name").
				Scan(&statistics).Error
		} else {
			err = tx.Group("client_user_id").
				Scan(&statistics).Error
		}
	}

	for _, s := range statistics {
		s.TotalQuota /= common.QuotaPerUnit
		if !expandModels && s.ClientUserId != "" {
			var cu CliendUserQuota
			_ = DB.Where("client_user_id = ?", s.ClientUserId).First(&cu).Error
			s.FixedQuota = cu.FixedQuota
			s.TempQuota = cu.TempQuota
		}
	}

	if statistics == nil {
		statistics = make([]*QuotaDataStatistics, 0)
	}
	return statistics, err
}

func increaseQuotaData(userId int, username string, modelName string, count int, quota int, createdAt int64, tokenUsed int, tokenId int, channelId int, promptTokens int, completionTokens int, clientUserId string, clientScenairo string) {
	err := DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ? and token_id = ? and channel_id = ? and client_user_id = ? and client_scenairo = ?",
		userId, username, modelName, createdAt, tokenId, channelId, clientUserId, clientScenairo).Updates(map[string]interface{}{
		"count":             gorm.Expr("count + ?", count),
		"quota":             gorm.Expr("quota + ?", quota),
		"token_used":        gorm.Expr("token_used + ?", tokenUsed),
		"prompt_tokens":     gorm.Expr("prompt_tokens + ?", promptTokens),
		"completion_tokens": gorm.Expr("completion_tokens + ?", completionTokens),
	}).Error
	if err != nil {
		common.SysLog("increaseQuotaData error:" + err.Error())
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64, defaultTime string) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	//err = DB.Table("quota_data").Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used,sum(prompt_tokens) as prompt_tokens, sum(completion_tokens) as  completion_tokens , created_at").Where("created_at >= ? and created_at <= ? and user_id = ?", startTime, endTime, userId).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string, defaultTime string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used,sum(prompt_tokens) as prompt_tokens, sum(completion_tokens) as  completion_tokens , created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaByTime(userId int, startTime int64, endTime int64) (int, error) {
	var quota int
	err := DB.Table("quota_data").Select("COALESCE(sum(quota), 0) as quota").Where("created_at >= ? and created_at <= ? and user_id = ?", startTime, endTime, userId).Find(&quota).Error
	return quota, err
}

// ChannelQuotaStatistics 渠道消耗统计
type ChannelQuotaStatistics struct {
	ChannelId   int     `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	ModelName   string  `json:"model_name"`
	TotalCount  int64   `json:"total_count"`
	TotalQuota  float64 `json:"total_quota"`
}

// GetChannelQuotaStatistics 获取渠道消耗统计数据
func GetChannelQuotaStatistics(startTime int64, endTime int64) ([]*ChannelQuotaStatistics, error) {
	statistics := make([]*ChannelQuotaStatistics, 0)

	// 查询每个渠道下各模型的消耗数据
	err := DB.Table("quota_data").
		Select("channel_id, model_name, sum(count) as total_count, sum(quota) as total_quota").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("channel_id, model_name").
		Order("total_quota DESC").
		Scan(&statistics).Error

	if err != nil {
		return nil, err
	}

	// 获取渠道名称
	channelIds := make([]int, 0)
	channelIdMap := make(map[int]bool)
	for _, s := range statistics {
		if !channelIdMap[s.ChannelId] {
			channelIds = append(channelIds, s.ChannelId)
			channelIdMap[s.ChannelId] = true
		}
	}

	if len(channelIds) > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", channelIds).Find(&channels).Error; err == nil {
			channelNameMap := make(map[int]string)
			for _, ch := range channels {
				channelNameMap[ch.Id] = ch.Name
			}
			for _, s := range statistics {
				s.ChannelName = channelNameMap[s.ChannelId]
				// 转换为美元单位
				s.TotalQuota = s.TotalQuota / common.QuotaPerUnit
			}
		}
	}

	if statistics == nil {
		statistics = make([]*ChannelQuotaStatistics, 0)
	}
	return statistics, nil
}
