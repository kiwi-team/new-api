package model

import (
	"errors"

	"gorm.io/gorm"
)

// SettlementConfig 结算价格配置表
type SettlementConfig struct {
	Id           int     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId       int     `json:"user_id" gorm:"index;not null;uniqueIndex:idx_settlement_user_model"`
	ModelName    string  `json:"model_name" gorm:"size:200;not null;uniqueIndex:idx_settlement_user_model"`
	InputPrice   float64 `json:"input_price" gorm:"type:decimal(20,10);not null;default:0"`
	OutputPrice  float64 `json:"output_price" gorm:"type:decimal(20,10);not null;default:0"`
	RequestPrice float64 `json:"request_price" gorm:"type:decimal(20,10);not null;default:0"`
	CreatedAt    int64   `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt    int64   `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

// CreateSettlementConfig creates a new settlement config record.
// Validates that input_price, output_price and request_price are non-negative.
func CreateSettlementConfig(config *SettlementConfig) error {
	if config.InputPrice < 0 {
		return errors.New("输入价格不能为负数")
	}
	if config.OutputPrice < 0 {
		return errors.New("输出价格不能为负数")
	}
	if config.RequestPrice < 0 {
		return errors.New("次数价格不能为负数")
	}
	return DB.Create(config).Error
}

// UpdateSettlementConfig updates an existing settlement config record.
// Validates that input_price, output_price and request_price are non-negative.
func UpdateSettlementConfig(config *SettlementConfig) error {
	if config.InputPrice < 0 {
		return errors.New("输入价格不能为负数")
	}
	if config.OutputPrice < 0 {
		return errors.New("输出价格不能为负数")
	}
	if config.RequestPrice < 0 {
		return errors.New("次数价格不能为负数")
	}
	return DB.Model(config).Select("UserId", "ModelName", "InputPrice", "OutputPrice", "RequestPrice").Updates(config).Error
}

// DeleteSettlementConfig deletes a settlement config by its ID.
func DeleteSettlementConfig(id int) error {
	if id == 0 {
		return errors.New("id 为空")
	}
	result := DB.Delete(&SettlementConfig{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return errors.New("记录不存在")
	}
	return result.Error
}

// GetSettlementConfigById retrieves a settlement config by its ID.
func GetSettlementConfigById(id int) (*SettlementConfig, error) {
	if id == 0 {
		return nil, errors.New("id 为空")
	}
	var config SettlementConfig
	err := DB.First(&config, "id = ?", id).Error
	return &config, err
}

// GetSettlementConfigsByUserId retrieves all settlement configs for a given user.
func GetSettlementConfigsByUserId(userId int) ([]*SettlementConfig, error) {
	var configs []*SettlementConfig
	err := DB.Where("user_id = ?", userId).Find(&configs).Error
	return configs, err
}

// GetSettlementConfigByUserAndModel retrieves a settlement config by user_id and model_name.
func GetSettlementConfigByUserAndModel(userId int, modelName string) (*SettlementConfig, error) {
	var config SettlementConfig
	err := DB.Where("user_id = ? AND model_name = ?", userId, modelName).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

// BatchCreateSettlementConfigs creates or updates multiple settlement configs in a single transaction.
// If a record with the same user_id + model_name already exists, it updates the prices.
func BatchCreateSettlementConfigs(configs []*SettlementConfig) error {
	if len(configs) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, config := range configs {
			if config.InputPrice < 0 {
				return errors.New("输入价格不能为负数")
			}
			if config.OutputPrice < 0 {
				return errors.New("输出价格不能为负数")
			}
			if config.RequestPrice < 0 {
				return errors.New("次数价格不能为负数")
			}
			// Check if record already exists
			var existing SettlementConfig
			err := tx.Where("user_id = ? AND model_name = ?", config.UserId, config.ModelName).First(&existing).Error
			if err == nil {
				// Record exists — update prices
				err = tx.Model(&existing).Updates(map[string]interface{}{
					"input_price":   config.InputPrice,
					"output_price":  config.OutputPrice,
					"request_price": config.RequestPrice,
				}).Error
				if err != nil {
					return err
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				// Record does not exist — create
				if err := tx.Create(config).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return nil
	})
}
