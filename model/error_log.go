package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"one-api/types"
)

type ErrorLog struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index:idx_error_created_a"`
	ChannelId   int    `json:"channel_id" gorm:"index"`
	ChannelName string `json:"channel_name" gorm:"default:''"`
	ModelName   string `json:"model_name" gorm:"default:''"`
	Message     string `json:"message" gorm:"default:''"`
	Type        string `json:"type" gorm:"default:''"`
	Param       string `json:"param" gorm:"default:''"`
	Code        string `json:"code" gorm:"default:''"`
	RequestId   string `json:"request_id" gorm:"default:'';index:idx_error_request_id"`
	StatusCode  int    `json:"status_code" gorm:"default:0"`
	Body        string `json:"body" gorm:"default:''"`
	Ip          string `json:"ip" gorm:"default:''"`
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
	query := DB.Model(&ErrorLog{})
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
	var total int64
	_ = query.Count(&total)
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&errorLogs).Error
	return errorLogs, total, err
}

func SaveErrorLog(userId int, channelId int, channelName string, modelName string, err types.NewAPIError, body string, requestId string, ip string) error {
	log := &ErrorLog{
		UserId:      userId,
		CreatedAt:   common.GetTimestamp(),
		ChannelId:   channelId,
		Message:     err.ToOpenAIError().Message,
		Type:        string(err.ErrorType),
		Param:       err.ToOpenAIError().Param,
		ChannelName: channelName,
		ModelName:   modelName,
		Code:        fmt.Sprintf("%v", err.ToOpenAIError().Code),
		StatusCode:  err.StatusCode,
		Body:        body,
		Ip:          ip,

		RequestId: requestId,
	}
	err1 := DB.Create(log).Error
	if err1 != nil {
		common.SysError("failed to record error_log: " + err1.Error())
	}
	return err1
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
	DB.Model(&ErrorLog{}).Select("channel_id,max(channel_name) as channel_name,model_name,count(*) as total,max(message) as message,max(status_code) as status_code,code").Where("created_at > ? and created_at < ?", start, end).Group("model_name,channel_id,code").Order("total desc").Scan(&errorLogStatistics)
	return errorLogStatistics
}
