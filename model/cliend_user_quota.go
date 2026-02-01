package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type CliendUserQuota struct {
	Id           int    `json:"id"`
	ClientUserId string `json:"client_user_id" gorm:"uniqueIndex;size:200;not null;default:''"`
	FixedQuota   int    `json:"fixed_quota" gorm:"type:int;default:0"`
	TempQuota    int    `json:"temp_quota" gorm:"type:int;default:0"`
	UsedQuota    int    `json:"used_quota" gorm:"type:int;default:0"`
	UpdatedAt    int64  `json:"updated_at" gorm:"type:bigint;index"`
	ExpiredAt    int64  `json:"expired_at" gorm:"type:bigint;index;default:0"`
}

func (CliendUserQuota) TableName() string {
	return "cliend_user_quota"
}

func CheckCliendUserQuota(clientUserId string) (bool, error) {
	var row CliendUserQuota
	err := DB.Where("client_user_id = ?", clientUserId).First(&row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return false, err
	}
	return row.FixedQuota+row.TempQuota > int(float64(row.UsedQuota)/common.QuotaPerUnit), nil
}

func IncreaseCliendUserUsedQuota(clientUserId string, delta int) error {
	if clientUserId == "" || delta == 0 {
		return nil
	}
	var row CliendUserQuota
	err := DB.Where("client_user_id = ?", clientUserId).First(&row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if row.Id == 0 {
		row.ClientUserId = clientUserId
		row.UsedQuota = delta
		row.UpdatedAt = common.GetTimestamp()
		return DB.Create(&row).Error
	}
	return DB.Model(&CliendUserQuota{}).
		Where("client_user_id = ?", clientUserId).
		Updates(map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", delta),
			"updated_at": common.GetTimestamp(),
		}).Error
}

func ResetExpiredCliendUserTempQuota() {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("ResetExpiredCliendUserTempQuota panic: %v", r))
		}
	}()
	for {
		now := time.Now().Unix()
		var rows []CliendUserQuota
		err := DB.Where("expired_at > 0 AND expired_at <= ? AND (temp_quota > 0 or used_quota > 0)", now).Find(&rows).Error
		if err == nil {
			for _, row := range rows {
				oldTemp := row.TempQuota
				err2 := DB.Model(&CliendUserQuota{}).
					Where("id = ?", row.Id).
					Updates(map[string]interface{}{
						"temp_quota": 0,
						"used_quota": 0,
						"updated_at": common.GetTimestamp(),
					}).Error
				if err2 != nil {
					continue
				}
				log := CliendUserQuotaLog{
					ClientUserId: row.ClientUserId,
					AdminUserId:  0,
					Action:       "reset",
					OldFixed:     row.FixedQuota,
					NewFixed:     row.FixedQuota,
					OldTemp:      oldTemp,
					OldUsed:      row.UsedQuota,
					NewTemp:      0,
					NewUsed:      0,
					Remark:       "",
					CreatedAt:    time.Now().Unix(),
				}
				_ = DB.Create(&log).Error
			}
		}
		time.Sleep(time.Minute)
	}
}
