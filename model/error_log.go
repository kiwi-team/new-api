package model

import (
	"fmt"
	"one-api/common"
	"one-api/dto"
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
	Ip          string `json:"ip gorm:"default:''"`
}

func GetAllErrorLog(startIdx int, num int, channelId int, modelName string) ([]*ErrorLog, error) {
	var errorLogs []*ErrorLog
	var err error
	if channelId > 0 {
		if modelName == "" {
			err = DB.Order("id desc").Where("channel_id = ? ", channelId).Limit(num).Offset(startIdx).Find(&errorLogs).Error
		} else {
			err = DB.Order("id desc").Where("channel_id = ? and model_name = ?", channelId, modelName).Limit(num).Offset(startIdx).Find(&errorLogs).Error
		}
	} else {
		if modelName != "" {
			err = DB.Order("id desc").Where("model_name = ?", modelName).Limit(num).Offset(startIdx).Find(&errorLogs).Error
		}
	}
	return errorLogs, err
}

func SaveErrorLog(userId int, channelId int, channelName string, modelName string, err dto.OpenAIErrorWithStatusCode, body string, requestId string, ip string) error {
	log := &ErrorLog{
		UserId:      userId,
		CreatedAt:   common.GetTimestamp(),
		ChannelId:   channelId,
		Message:     err.Error.Message,
		Type:        err.Error.Type,
		Param:       err.Error.Param,
		ChannelName: channelName,
		ModelName:   modelName,
		Code:        fmt.Sprintf("%v", err.Error.Code),
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
