package model

type CliendUserQuotaLog struct {
	Id           int    `json:"id"`
	ClientUserId string `json:"client_user_id" gorm:"index;size:200;default:''"`
	AdminUserId  int    `json:"admin_user_id" gorm:"index"`
	Action       string `json:"action" gorm:"size:64;default:''"`
	OldFixed     int    `json:"old_fixed_quota" gorm:"type:int;default:0"`
	NewFixed     int    `json:"new_fixed_quota" gorm:"type:int;default:0"`
	OldTemp      int    `json:"old_temp_quota" gorm:"type:int;default:0"`
	NewTemp      int    `json:"new_temp_quota" gorm:"type:int;default:0"`
	OldUsed      int    `json:"old_used_quota" gorm:"type:int;default:0"`
	NewUsed      int    `json:"new_used_quota" gorm:"type:int;default:0"`
	Remark       string `json:"remark" gorm:"size:255;default:''"`
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint;index"`
}

func (CliendUserQuotaLog) TableName() string {
	return "cliend_user_quota_log"
}
