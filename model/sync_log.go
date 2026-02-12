package model

import (
	"github.com/QuantumNous/new-api/common"
)

// SyncLog 同步历史记录
type SyncLog struct {
	Id              int    `json:"id"`
	SyncType        string `json:"sync_type" gorm:"type:varchar(32);not null"` // channel/model_price
	EnvironmentId   int    `json:"environment_id"`                             // 目标环境ID
	EnvironmentName string `json:"environment_name" gorm:"type:varchar(128)"`  // 目标环境名称
	DataSummary     string `json:"data_summary" gorm:"type:text"`              // 同步数据摘要
	Status          int    `json:"status"`                                     // 1=成功, 2=失败
	ErrorMessage    string `json:"error_message" gorm:"type:text"`             // 失败原因
	CreatedTime     int64  `json:"created_time" gorm:"bigint"`                 // 同步时间
	OperatorId      int    `json:"operator_id"`                                // 操作人ID
}

func (SyncLog) TableName() string {
	return "sync_logs"
}

// SyncLogFilter 同步历史筛选条件
type SyncLogFilter struct {
	SyncType      string `form:"sync_type"`
	EnvironmentId int    `form:"environment_id"`
	Status        int    `form:"status"`
	Page          int    `form:"page"`
	PageSize      int    `form:"page_size"`
}

// GetSyncLogs 获取同步历史（支持筛选和分页）
func GetSyncLogs(filter *SyncLogFilter) ([]*SyncLog, int64, error) {
	var logs []*SyncLog
	var total int64

	query := DB.Model(&SyncLog{})

	if filter.SyncType != "" {
		query = query.Where("sync_type = ?", filter.SyncType)
	}
	if filter.EnvironmentId > 0 {
		query = query.Where("environment_id = ?", filter.EnvironmentId)
	}
	if filter.Status > 0 {
		query = query.Where("status = ?", filter.Status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	page := filter.Page
	pageSize := filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	err = query.Order("id desc").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

// CreateSyncLog 创建同步记录
func CreateSyncLog(log *SyncLog) error {
	log.CreatedTime = common.GetTimestamp()
	return DB.Create(log).Error
}

// BatchCreateSyncLogs 批量创建同步记录
func BatchCreateSyncLogs(logs []*SyncLog) error {
	if len(logs) == 0 {
		return nil
	}
	timestamp := common.GetTimestamp()
	for _, log := range logs {
		log.CreatedTime = timestamp
	}
	return DB.Create(&logs).Error
}
