package model

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"

	"github.com/samber/hot"
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
	// Discount 模型折扣系数：该用户请求此模型时，最终消耗 quota 乘以该系数得到实际扣费。
	// 取值范围 [0.01, 10]；默认 1（不打折）。存量行由 default:1 兜底为原价。
	Discount  float64 `json:"discount" gorm:"type:decimal(20,10);not null;default:1"`
	CreatedAt int64   `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	UpdatedAt int64   `json:"updated_at" gorm:"type:bigint;autoUpdateTime"`
}

// 折扣取值范围：0.01 ~ 10.00
const (
	SettlementDiscountMin = 0.01
	SettlementDiscountMax = 10.0
)

// validateDiscount 校验折扣取值范围。0 视为未填写，归一化为 1（不打折）由调用方处理。
func validateDiscount(discount float64) error {
	if discount < SettlementDiscountMin || discount > SettlementDiscountMax {
		return errors.New("模型折扣必须在 0.01 到 10.00 之间")
	}
	return nil
}

// CreateSettlementConfig creates a new settlement config record.
// Validates that input_price, output_price and request_price are non-negative,
// and that discount is within [0.01, 10]. A zero discount is normalized to 1 (no discount).
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
	if config.Discount == 0 {
		config.Discount = 1
	}
	if err := validateDiscount(config.Discount); err != nil {
		return err
	}
	if err := DB.Create(config).Error; err != nil {
		return err
	}
	invalidateSettlementConfigCache(config.UserId)
	return nil
}

// UpdateSettlementConfig updates an existing settlement config record.
// Validates that input_price, output_price and request_price are non-negative,
// and that discount is within [0.01, 10]. A zero discount is normalized to 1 (no discount).
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
	if config.Discount == 0 {
		config.Discount = 1
	}
	if err := validateDiscount(config.Discount); err != nil {
		return err
	}
	if err := DB.Model(config).Select("UserId", "ModelName", "InputPrice", "OutputPrice", "RequestPrice", "Discount").Updates(config).Error; err != nil {
		return err
	}
	invalidateSettlementConfigCache(config.UserId)
	return nil
}

// DeleteSettlementConfig deletes a settlement config by its ID.
func DeleteSettlementConfig(id int) error {
	if id == 0 {
		return errors.New("id 为空")
	}
	// 先取出 user_id 以便删除后失效缓存（记录不存在时不影响主流程）
	var existing SettlementConfig
	_ = DB.Select("user_id").First(&existing, "id = ?", id).Error
	result := DB.Delete(&SettlementConfig{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return errors.New("记录不存在")
	}
	if result.Error == nil && existing.UserId != 0 {
		invalidateSettlementConfigCache(existing.UserId)
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

// GetAllSettlementConfigs returns every settlement config (all users). Optionally
// filtered by modelName (exact) when non-empty. Ordered by model_name then user_id
// so the "price center" per-model aggregation is stable.
func GetAllSettlementConfigs(modelName string) ([]*SettlementConfig, error) {
	var configs []*SettlementConfig
	tx := DB.Model(&SettlementConfig{})
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	err := tx.Order("model_name asc").Order("user_id asc").Find(&configs).Error
	return configs, err
}

// GetUsernamesByIds batch-loads id -> username for the given user ids (deduped).
// Missing ids simply won't appear in the returned map.
func GetUsernamesByIds(ids []int) (map[int]string, error) {
	result := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []struct {
		Id       int
		Username string
	}
	if err := DB.Model(&User{}).Where("id IN ?", ids).Select("id", "username").Find(&rows).Error; err != nil {
		return result, err
	}
	for _, r := range rows {
		result[r.Id] = r.Username
	}
	return result, nil
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
	userIds := make(map[int]struct{})
	err := DB.Transaction(func(tx *gorm.DB) error {
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
			if config.Discount == 0 {
				config.Discount = 1
			}
			if err := validateDiscount(config.Discount); err != nil {
				return err
			}
			userIds[config.UserId] = struct{}{}
			// Check if record already exists
			var existing SettlementConfig
			err := tx.Where("user_id = ? AND model_name = ?", config.UserId, config.ModelName).First(&existing).Error
			if err == nil {
				// Record exists — update prices
				err = tx.Model(&existing).Updates(map[string]interface{}{
					"input_price":   config.InputPrice,
					"output_price":  config.OutputPrice,
					"request_price": config.RequestPrice,
					"discount":      config.Discount,
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
	if err != nil {
		return err
	}
	for userId := range userIds {
		invalidateSettlementConfigCache(userId)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 折扣热路径查询 + 缓存
// ---------------------------------------------------------------------------

const settlementConfigCacheNamespace = "new-api:settlement_config:v1"

var (
	settlementConfigCacheOnce sync.Once
	settlementConfigCache     *cachex.HybridCache[[]*SettlementConfig]
)

func settlementConfigCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("SETTLEMENT_CONFIG_CACHE_TTL", 300)
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return time.Duration(ttlSeconds) * time.Second
}

func settlementConfigCacheCapacity() int {
	capacity := common.GetEnvOrDefault("SETTLEMENT_CONFIG_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getSettlementConfigCache() *cachex.HybridCache[[]*SettlementConfig] {
	settlementConfigCacheOnce.Do(func() {
		ttl := settlementConfigCacheTTL()
		settlementConfigCache = cachex.NewHybridCache[[]*SettlementConfig](cachex.HybridCacheConfig[[]*SettlementConfig]{
			Namespace: cachex.Namespace(settlementConfigCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[[]*SettlementConfig]{},
			Memory: func() *hot.HotCache[string, []*SettlementConfig] {
				return hot.NewHotCache[string, []*SettlementConfig](hot.LRU, settlementConfigCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return settlementConfigCache
}

func settlementConfigCacheKey(userId int) string {
	return strconv.Itoa(userId)
}

func invalidateSettlementConfigCache(userId int) {
	if userId <= 0 {
		return
	}
	cache := getSettlementConfigCache()
	_, _ = cache.DeleteMany([]string{settlementConfigCacheKey(userId)})
}

// getSettlementConfigsCached 读取用户全部结算配置（带缓存）。命中缓存直接返回，
// 未命中则查库并回填。查库为空也缓存空切片，避免未配置折扣的用户每次请求都打库。
func getSettlementConfigsCached(userId int) ([]*SettlementConfig, error) {
	if userId <= 0 {
		return nil, nil
	}
	cache := getSettlementConfigCache()
	key := settlementConfigCacheKey(userId)
	if configs, found, err := cache.Get(key); err == nil && found {
		return configs, nil
	}
	configs, err := GetSettlementConfigsByUserId(userId)
	if err != nil {
		return nil, err
	}
	_ = cache.SetWithTTL(key, configs, settlementConfigCacheTTL())
	return configs, nil
}

// matchSettlementConfig 给定用户的所有 settlement_config 和一个模型名称，返回匹配的配置。
// 匹配优先级：精确匹配 > 最长前缀通配符匹配 > 未匹配（返回 nil）。
// 通配符规则：配置的模型名称以 * 结尾时，去掉 * 后作为前缀匹配。
// 与 service.MatchSettlementConfig 口径一致（放在 model 包内以避免 import cycle）。
func matchSettlementConfig(configs []*SettlementConfig, modelName string) *SettlementConfig {
	var bestWildcard *SettlementConfig
	bestPrefixLen := -1
	for _, cfg := range configs {
		if cfg.ModelName == modelName {
			return cfg
		}
		if prefix, ok := strings.CutSuffix(cfg.ModelName, "*"); ok {
			if strings.HasPrefix(modelName, prefix) && len(prefix) > bestPrefixLen {
				bestWildcard = cfg
				bestPrefixLen = len(prefix)
			}
		}
	}
	return bestWildcard
}

// GetUserModelSettlementDiscount 返回该用户对某模型配置的折扣系数（热路径调用，带缓存）。
// 未配置、配置无效或查询出错时返回 1（不打折），保证计费不受影响。
func GetUserModelSettlementDiscount(userId int, modelName string) float64 {
	if userId <= 0 || modelName == "" {
		return 1
	}
	configs, err := getSettlementConfigsCached(userId)
	if err != nil || len(configs) == 0 {
		return 1
	}
	matched := matchSettlementConfig(configs, modelName)
	if matched == nil {
		return 1
	}
	discount := matched.Discount
	if discount < SettlementDiscountMin || discount > SettlementDiscountMax {
		return 1
	}
	return discount
}
