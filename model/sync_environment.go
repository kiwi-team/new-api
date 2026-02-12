package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// SyncEnvironment 同步环境配置
type SyncEnvironment struct {
	Id          int    `json:"id"`
	Name        string `json:"name" gorm:"type:varchar(128);not null"`
	ApiUrl      string `json:"api_url" gorm:"type:varchar(512);not null"`
	RootToken   string `json:"root_token,omitempty" gorm:"type:varchar(512);not null"`
	NewApiUser  string `json:"new_api_user" gorm:"type:varchar(256);not null"`
	Status      int    `json:"status" gorm:"default:1"` // 1=启用, 2=禁用
	Remark      string `json:"remark" gorm:"type:varchar(512)"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
}

func (SyncEnvironment) TableName() string {
	return "sync_environments"
}

// GetAllSyncEnvironments 获取所有同步环境
func GetAllSyncEnvironments() ([]*SyncEnvironment, error) {
	var environments []*SyncEnvironment
	err := DB.Order("id desc").Find(&environments).Error
	return environments, err
}

// GetEnabledSyncEnvironments 获取所有已启用的同步环境
func GetEnabledSyncEnvironments() ([]*SyncEnvironment, error) {
	var environments []*SyncEnvironment
	err := DB.Where("status = ?", 1).Order("id desc").Find(&environments).Error
	return environments, err
}

// GetSyncEnvironmentById 根据ID获取同步环境
func GetSyncEnvironmentById(id int) (*SyncEnvironment, error) {
	var environment SyncEnvironment
	err := DB.First(&environment, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &environment, nil
}

// GetSyncEnvironmentsByIds 根据ID列表获取同步环境
func GetSyncEnvironmentsByIds(ids []int) ([]*SyncEnvironment, error) {
	var environments []*SyncEnvironment
	err := DB.Where("id IN ?", ids).Find(&environments).Error
	return environments, err
}

// Insert 创建同步环境
func (env *SyncEnvironment) Insert() error {
	if env.Name == "" {
		return errors.New("环境名称不能为空")
	}
	if env.ApiUrl == "" {
		return errors.New("API地址不能为空")
	}
	if env.RootToken == "" {
		return errors.New("Root Token不能为空")
	}
	if env.NewApiUser == "" {
		return errors.New("new-api-user不能为空")
	}
	env.CreatedTime = common.GetTimestamp()
	env.UpdatedTime = common.GetTimestamp()
	return DB.Create(env).Error
}

// Update 更新同步环境
func (env *SyncEnvironment) Update() error {
	if env.Id == 0 {
		return errors.New("环境ID不能为空")
	}
	env.UpdatedTime = common.GetTimestamp()
	return DB.Model(env).Updates(env).Error
}

// Delete 删除同步环境
func (env *SyncEnvironment) Delete() error {
	if env.Id == 0 {
		return errors.New("环境ID不能为空")
	}
	return DB.Delete(env).Error
}

// UpdateStatus 更新环境状态
func (env *SyncEnvironment) UpdateStatus(status int) error {
	return DB.Model(env).Update("status", status).Error
}
