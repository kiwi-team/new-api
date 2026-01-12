package model

import (
	"fmt"
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

func logQuotaDataCache(userId int, username string, modelName string, quota int, createdAt int64, tokenUsed int, tokenName string, promptTokens int, completionTokens int, tokenId int, channelId int, clientUserId string) {
	key := fmt.Sprintf("%d-%s-%s-%d-%d-%d-%s", userId, username, modelName, tokenId, createdAt, channelId, clientUserId)
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
	// 只精确到小时
	createdAt = createdAt - (createdAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(userId, username, modelName, quota, createdAt, tokenUsed, tokenName, promptTokens, completionTokens, tokenId, channelId, clientUserId)
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
		DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ? and token_id = ? and channel_id = ? and client_user_id = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.TokenId, quotaData.ChannelId, quotaData.ClientUserId).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.Count, quotaData.Quota, quotaData.CreatedAt, quotaData.TokenUsed, quotaData.TokenId, quotaData.ChannelId, quotaData.PromptTokens, quotaData.CompletionTokens, quotaData.ClientUserId)
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

func GetQuotaDataStatistics(startTime int64, endTime int64, modelName string, clientUserId string, expandModels bool, expandDates bool) ([]*QuotaDataStatistics, error) {
	var statistics []*QuotaDataStatistics
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
		tx = tx.Where("client_user_id = ?", clientUserId)
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

	return statistics, err
}

func increaseQuotaData(userId int, username string, modelName string, count int, quota int, createdAt int64, tokenUsed int, tokenId int, channelId int, promptTokens int, completionTokens int, clientUserId string) {
	err := DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ? and token_id = ? and channel_id = ? and client_user_id = ?",
		userId, username, modelName, createdAt, tokenId, channelId, clientUserId).Updates(map[string]interface{}{
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
	err := LOG_DB.Table("logs").Select("COALESCE(sum(quota), 0) as quota").Where("created_at >= ? and created_at <= ? and user_id = ?", startTime, endTime, userId).Find(&quota).Error
	return quota, err
}
