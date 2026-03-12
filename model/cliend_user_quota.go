package model

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type CliendUserQuota struct {
	Id           int    `json:"id"`
	ClientUserId string `json:"client_user_id" gorm:"uniqueIndex;size:200;not null;default:''"`
	ClientName   string `json:"client_name" gorm:"default:''"`
	FixedQuota   int    `json:"fixed_quota" gorm:"type:int;default:0"`
	TempQuota    int    `json:"temp_quota" gorm:"type:int;default:0"`
	UsedQuota    int    `json:"used_quota" gorm:"type:int;default:0"`
	UpdatedAt    int64  `json:"updated_at" gorm:"type:bigint;index"`
	ExpiredAt    int64  `json:"expired_at" gorm:"type:bigint;index;default:0"`
}

func (CliendUserQuota) TableName() string {
	return "cliend_user_quota"
}

// CheckCliendUserIdExists 检查 client_user_id 是否在 cliend_user_quota 表中存在
func CheckCliendUserIdExists(clientUserId string) (bool, error) {
	var count int64
	err := DB.Model(&CliendUserQuota{}).Where("client_user_id = ?", clientUserId).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func CheckCliendUserQuota(clientUserId string, projectQuota int) (bool, error) {
	var row CliendUserQuota
	err := DB.Where("client_user_id = ?", clientUserId).First(&row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return false, err
	}
	return row.FixedQuota+row.TempQuota+projectQuota > int(float64(row.UsedQuota)/common.QuotaPerUnit), nil
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

// ResetExpiredCliendUserTempQuota 临时额度过期后重置为0
func ResetExpiredCliendUserTempQuota() {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("ResetExpiredCliendUserTempQuota panic: %v", r))
		}
	}()
	for {
		now := time.Now().Unix()
		var rows []CliendUserQuota
		err := DB.Where("expired_at > 0 AND expired_at <= ? AND temp_quota > 0", now).Find(&rows).Error
		if err != nil {
			common.SysError(fmt.Sprintf("ResetExpiredCliendUserTempQuota query error: %v", err))
			time.Sleep(time.Minute)
			continue
		}
		for _, row := range rows {
			oldTemp := row.TempQuota
			err2 := DB.Model(&CliendUserQuota{}).
				Where("id = ?", row.Id).
				Updates(map[string]any{
					"temp_quota": 0,
					"updated_at": common.GetTimestamp(),
				}).Error
			if err2 != nil {
				common.SysError(fmt.Sprintf("ResetExpiredCliendUserTempQuota update error for id %d: %v", row.Id, err2))
				continue
			}
			log := CliendUserQuotaLog{
				ClientUserId: row.ClientUserId,
				AdminUserId:  0,
				Action:       "temp_expired",
				OldFixed:     row.FixedQuota,
				NewFixed:     row.FixedQuota,
				OldTemp:      oldTemp,
				OldUsed:      row.UsedQuota,
				NewTemp:      0,
				NewUsed:      row.UsedQuota,
				Remark:       "临时额度过期自动重置",
				CreatedAt:    time.Now().Unix(),
			}
			_ = DB.Create(&log).Error
		}
		time.Sleep(time.Minute)
	}
}

// ResetMonthlyUsedQuota 每月1号（北京时间）重置所有用户的已使用额度
func ResetMonthlyUsedQuota() {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("ResetMonthlyUsedQuota panic: %v", r))
		}
	}()

	// 使用北京时间 UTC+8
	beijingLoc := time.FixedZone("CST", 8*60*60)
	lastResetMonth := loadLastResetMonth()
	if lastResetMonth == 0 {
		lastResetMonth = int(time.Now().In(beijingLoc).Month())
	}

	// 每小时检查一次是否到了新的月份
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().In(beijingLoc)
		currentMonth := int(now.Month())

		// 如果月份变了，说明进入了新的一个月
		if currentMonth != lastResetMonth {
			lastResetMonth = currentMonth
			saveLastResetMonth(lastResetMonth)
			resetMonthlyUsedQuotaOnce()
		}
	}
}

const lastResetMonthFile = "data/last_reset_month.txt"

func loadLastResetMonth() int {
	data, err := os.ReadFile(lastResetMonthFile)
	if err != nil {
		return 0
	}
	month, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return month
}

func saveLastResetMonth(month int) {
	// 确保目录存在
	dir := "data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		common.SysError(fmt.Sprintf("Failed to create data dir: %v", err))
		return
	}
	if err := os.WriteFile(lastResetMonthFile, []byte(strconv.Itoa(month)), 0644); err != nil {
		common.SysError(fmt.Sprintf("Failed to save last reset month: %v", err))
	}
}

func resetMonthlyUsedQuotaOnce() {
	// 查询所有 used_quota > 0 的记录
	var rows []CliendUserQuota
	err := DB.Where("used_quota > 0").Find(&rows).Error
	if err != nil {
		common.SysError(fmt.Sprintf("ResetMonthlyUsedQuota query error: %v", err))
		return
	}

	for _, row := range rows {
		oldUsed := row.UsedQuota
		err2 := DB.Model(&CliendUserQuota{}).
			Where("id = ?", row.Id).
			Updates(map[string]any{
				"used_quota": 0,
				"updated_at": common.GetTimestamp(),
			}).Error
		if err2 != nil {
			common.SysError(fmt.Sprintf("ResetMonthlyUsedQuota update error for id %d: %v", row.Id, err2))
			continue
		}
		log := CliendUserQuotaLog{
			ClientUserId: row.ClientUserId,
			AdminUserId:  0,
			Action:       "monthly_reset",
			OldFixed:     row.FixedQuota,
			NewFixed:     row.FixedQuota,
			OldTemp:      row.TempQuota,
			OldUsed:      oldUsed,
			NewTemp:      row.TempQuota,
			NewUsed:      0,
			Remark:       "每月已使用额度自动重置",
			CreatedAt:    time.Now().Unix(),
		}
		_ = DB.Create(&log).Error
	}
	common.SysLog(fmt.Sprintf("ResetMonthlyUsedQuota completed, reset %d records", len(rows)))
}
