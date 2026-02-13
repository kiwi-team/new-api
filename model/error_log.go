package model

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

type ErrorLog struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id" gorm:"index"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index:idx_error_created_a"`
	ChannelId      int    `json:"channel_id" gorm:"index"`
	ChannelName    string `json:"channel_name" gorm:"default:''"`
	TokenId        int    `json:"token_id" gorm:"index"`
	TokenName      string `json:"token_name"`
	ModelName      string `json:"model_name" gorm:"default:''"`
	Message        string `json:"message" gorm:"default:''"`
	Type           string `json:"type" gorm:"default:''"`
	Param          string `json:"param" gorm:"default:''"`
	Code           string `json:"code" gorm:"default:''"`
	RequestId      string `json:"request_id" gorm:"default:'';index:idx_error_request_id"`
	StatusCode     int    `json:"status_code" gorm:"default:0"`
	Body           string `json:"body" gorm:"default:''"`
	Ip             string `json:"ip" gorm:"default:''"`
	ClientUserId   string `json:"client_user_id" gorm:"default:''"`
	ClientScenairo string `json:"client_scenairo" gorm:"index;size:200;default:''"`
}

func GetAllErrorLog(req *dto.ErrorLogsRequest) ([]*ErrorLog, int64, error) {
	var errorLogs []*ErrorLog
	var err error
	channelId := req.ChannelId
	page := req.Page
	pageSize := req.PageSize
	if page <= 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	num := pageSize
	startIdx := (page - 1) * num
	var query *gorm.DB
	if os.Getenv("LOG_SQL_DSN") != "" {
		query = LOG_DB.Model(&ErrorLog{})
	} else {
		query = LOG_DB.Model(&ErrorLog{}).Joins("left join tokens on error_logs.token_id = tokens.id").Select("error_logs.*, tokens.name as token_name")
	}
	if channelId > 0 {
		query = query.Where("channel_id = ? ", channelId)
	}
	if req.EndTime-req.StartTime > 7*86400 {
		return nil, 0, errors.New("错误日志最多只支持查询7天的范围")
	}
	if req.StartTime > 0 {
		query = query.Where("created_at >= ?", req.StartTime)
	} else {
		query = query.Where("created_at >= ?", common.GetTimestamp()-7*86400)
	}
	if req.EndTime > 0 {
		query = query.Where("created_at <= ?", req.EndTime)
	}
	if req.RequestId != "" {
		query = query.Where("request_id = ?", req.RequestId)
	}
	if req.ModelName != "" {
		query = query.Where("model_name = ?", req.ModelName)
	}
	if req.TokenId > 0 {
		query = query.Where("token_id = ?", req.TokenId)
	}
	if req.ClientUserId != "" {
		query = query.Where("client_user_id = ?", req.ClientUserId)
	}
	var total int64
	_ = query.Count(&total)
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&errorLogs).Error
	tokenIds := make([]int, 0)
	for _, log := range errorLogs {
		if slices.Contains(tokenIds, log.TokenId) {
			continue
		}
		tokenIds = append(tokenIds, log.TokenId)
	}
	if len(tokenIds) > 0 && os.Getenv("LOG_SQL_DSN") != "" {
		var tokens []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err1 := DB.Model(&Token{}).
			Select("id, name").
			Where("id IN ?", tokenIds).
			Find(&tokens).Error; err1 == nil {
			tokenNames := make(map[int]string, len(tokens))
			for _, t := range tokens {
				tokenNames[t.Id] = t.Name
			}
			for i := range errorLogs {
				errorLogs[i].TokenName = tokenNames[errorLogs[i].TokenId]
			}
		}
	}
	return errorLogs, total, err
}

var LogList []*ErrorLog

func SaveErrorLog(userId int, channelId int, channelName string, modelName string, err types.OpenAIError, body string, requestId string, ip string, tokenId int, clientUserId string, clientScenairo string, includeBody bool) error {
	// 只调用一次 ToOpenAIError() 方法，避免重复调用
	//openAIError := err.ToOpenAIError()

	bodyToSave := ""
	if includeBody {
		bodyToSave = body
	}

	log := &ErrorLog{
		UserId:         userId,
		CreatedAt:      common.GetTimestamp(),
		ChannelId:      channelId,
		Message:        err.Message,
		Type:           string(err.Type),
		Param:          err.Param,
		ChannelName:    channelName,
		ModelName:      modelName,
		Code:           fmt.Sprintf("%v", err.Code),
		StatusCode:     err.StatusCode,
		Body:           bodyToSave,
		Ip:             ip,
		TokenId:        tokenId,
		RequestId:      requestId,
		ClientUserId:   clientUserId,
		ClientScenairo: clientScenairo,
	}
	return LOG_DB.Create(log).Error
	// LogList = append(LogList, log)
	// size := len(LogList)
	// if size >= common.ErrorLogBatchSize {
	// 	err1 := LOG_DB.CreateInBatches(LogList, size).Error
	// 	if err1 != nil {
	// 		common.SysError("failed to record error_log: " + err1.Error())
	// 	} else {
	// 		LogList = make([]*ErrorLog, 0)
	// 	}
	// 	return err1
	// }
	//return nil
}

type ErrorLogStatistics struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ModelName   string `json:"model_name"`
	Total       int    `json:"total"`
	Message     string `json:"message"`
	StatusCode  int    `json:"status_code"`
	Code        string `json:"code"`
}

// 统计分析错误日志
// 按照模型统计分析
// 按照渠道统计分析
// 安装渠道-模型统计分析
func StatisticsErrorLog(start int64, end int64) []ErrorLogStatistics {
	var errorLogStatistics []ErrorLogStatistics
	LOG_DB.Model(&ErrorLog{}).Select("channel_id,max(channel_name) as channel_name,model_name,count(*) as total,max(message) as message,max(status_code) as status_code,code").Where("created_at > ? and created_at < ?", start, end).Group("model_name,channel_id,code").Order("total desc").Scan(&errorLogStatistics)
	return errorLogStatistics
}

func DeleteErrorLog(createTime int64, limit int) error {
	if limit == 0 {
		limit = 100
	}
	var ids []int
	err := LOG_DB.Model(&ErrorLog{}).Where("created_at < ?", createTime).Order("id asc").Limit(limit).Pluck("id", &ids).Error
	if err != nil {
		return err
	}
	if len(ids) <= 2 {
		return nil
	}
	maxId := slices.Max(ids)
	minId := slices.Min(ids)
	return LOG_DB.Where("id >= ? and id <= ?", minId, maxId).Delete(&ErrorLog{}).Error
}
