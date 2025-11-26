package model

type DeletedData struct {
	Id        int    `json:"id"`
	Type      string `json:"type" gorm:"default:''"`
	Data      string `json:"data" gorm:"default:''"`
	CreatedAt int64  `json:"created_at"`
}

func CreateDeletedData(data *DeletedData) error {
	return DB.Table("deleted_data").Create(data).Error
}
