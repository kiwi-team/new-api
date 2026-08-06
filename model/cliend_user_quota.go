package model

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
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

// CheckClientUserNonProjectBudget 判定某 uid 本月的非项目消耗是否仍在 固定预算+临时预算 之内。
//
// 项目消耗完全不参与本判定：带 project header 的请求走 service.ValidateProjectRequest 的项目额度
// 闸门。两个预算池相互独立——项目消耗不会吃掉非项目预算，项目额度也不会被非项目请求借用。
//
// 口径：
//   - 消耗：SUM(quota_data.quota)，project_name 为空且 created_at >= 本月起点（UTC+8）
//   - 预算：fixed_quota + temp_quota，临时额度在读时判过期
//   - 判定：消耗 >= 预算即拒，与项目侧 used >= allocated 的方向一致
//   - 未配置预算记录的 uid 一律拒绝
func CheckClientUserNonProjectBudget(clientUserId string) (bool, error) {
	if clientUserId == "" {
		return false, nil
	}
	var row CliendUserQuota
	err := DB.Where("client_user_id = ?", clientUserId).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 没有预算记录即没有额度，这是正常的业务结论而非查询失败。
			return false, nil
		}
		return false, err
	}

	budgetUSD := row.FixedQuota
	// 后台的 ResetExpiredCliendUserTempQuota 每分钟才把过期临时额度清零，读时不判过期的话，
	// 过期后仍有最长一分钟的窗口能继续用它放行。这里与 service.GetUserTotalBudget 和前端保持一致。
	if row.ExpiredAt == 0 || row.ExpiredAt > common.GetTimestamp() {
		budgetUSD += row.TempQuota
	}
	if budgetUSD <= 0 {
		// 零预算必拒，顺带省掉热路径上的聚合查询。
		return false, nil
	}

	budgetQuota := float64(budgetUSD) * common.QuotaPerUnit
	used, err := getMonthlyNonProjectQuotaCached(clientUserId, budgetQuota)
	if err != nil {
		return false, err
	}
	return float64(used) < budgetQuota, nil
}

const clientUserNonProjectUsedCacheNamespace = "new-api:uid_nonproject_used:v1"

// nonProjectBudgetRefreshRatio 缓存值达到预算这一比例后强制回源。
// 缓存把超支敞口放大成「TTL 内可以无限花」；对快要用尽预算的 uid 每次实时校验，
// 就把敞口压缩到最后这一小段预算之内，而这类 uid 只占少数，回源成本可控。
const nonProjectBudgetRefreshRatio = 0.9

var (
	clientUserNonProjectUsedCacheOnce sync.Once
	clientUserNonProjectUsedCache     *cachex.HybridCache[int64]
)

func clientUserNonProjectUsedCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("UID_NONPROJECT_USED_CACHE_TTL", 60)
	if ttlSeconds <= 0 {
		ttlSeconds = 60
	}
	return time.Duration(ttlSeconds) * time.Second
}

func clientUserNonProjectUsedCacheCapacity() int {
	capacity := common.GetEnvOrDefault("UID_NONPROJECT_USED_CACHE_CAP", 10000)
	if capacity <= 0 {
		capacity = 10000
	}
	return capacity
}

func getClientUserNonProjectUsedCache() *cachex.HybridCache[int64] {
	clientUserNonProjectUsedCacheOnce.Do(func() {
		ttl := clientUserNonProjectUsedCacheTTL()
		clientUserNonProjectUsedCache = cachex.NewHybridCache[int64](cachex.HybridCacheConfig[int64]{
			Namespace: cachex.Namespace(clientUserNonProjectUsedCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[int64]{},
			Memory: func() *hot.HotCache[string, int64] {
				return hot.NewHotCache[string, int64](hot.LRU, clientUserNonProjectUsedCacheCapacity()).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return clientUserNonProjectUsedCache
}

// clientUserNonProjectUsedCacheKey 把计费月起点编进 key，这样跨月时旧值自然不可见，
// 不必依赖 TTL 过期——否则 1 号凌晨最长一个 TTL 内会拿上月的消耗去卡新月的预算。
func clientUserNonProjectUsedCacheKey(clientUserId string, monthStart int64) string {
	return clientUserId + ":" + strconv.FormatInt(monthStart, 10)
}

// invalidateMonthlyNonProjectQuotaCache 在新的非项目消耗落库后让对应 uid 的缓存失效，
// 使缓存的滞后不超过 quota_data 本身的落库周期。只清本进程内存层与 Redis，
// 未开 Redis 的多实例部署下其它实例仍需等 TTL 过期——这也是 TTL 取 60s 的原因。
func invalidateMonthlyNonProjectQuotaCache(clientUserIds map[string]struct{}) {
	if len(clientUserIds) == 0 {
		return
	}
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())
	keys := make([]string, 0, len(clientUserIds))
	for clientUserId := range clientUserIds {
		keys = append(keys, clientUserNonProjectUsedCacheKey(clientUserId, monthStart))
	}
	_, _ = getClientUserNonProjectUsedCache().DeleteMany(keys)
}

// getMonthlyNonProjectQuotaCached 读取本月非项目消耗（带缓存）。
// budgetQuota 为该 uid 的预算（quota 单位）：缓存值已逼近预算时绕过缓存直接查库，
// 避免临界用户在 TTL 窗口内持续超支。
func getMonthlyNonProjectQuotaCached(clientUserId string, budgetQuota float64) (int64, error) {
	monthStart := common.BillingMonthStartUnix(common.GetTimestamp())
	key := clientUserNonProjectUsedCacheKey(clientUserId, monthStart)
	cache := getClientUserNonProjectUsedCache()
	if used, found, err := cache.Get(key); err == nil && found {
		if float64(used) < nonProjectBudgetRefreshRatio*budgetQuota {
			return used, nil
		}
	}
	used, err := GetMonthlyNonProjectQuota(clientUserId, monthStart)
	if err != nil {
		return 0, err
	}
	_ = cache.SetWithTTL(key, used, clientUserNonProjectUsedCacheTTL())
	return used, nil
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
			resetMonthlyFixedQuotaOnce()
		}
	}
}

const (
	lastResetMonthFile       = "data/last_reset_month.txt"
	defaultMonthlyFixedQuota = 100
)

// resetMonthlyFixedQuotaOnce 每月将所有用户的固定额度重置为 defaultMonthlyFixedQuota
func resetMonthlyFixedQuotaOnce() {
	var rows []CliendUserQuota
	err := DB.Where("fixed_quota != ?", defaultMonthlyFixedQuota).Find(&rows).Error
	if err != nil {
		common.SysError(fmt.Sprintf("resetMonthlyFixedQuotaOnce query error: %v", err))
		return
	}

	for _, row := range rows {
		oldFixed := row.FixedQuota
		err2 := DB.Model(&CliendUserQuota{}).
			Where("id = ?", row.Id).
			Updates(map[string]any{
				"fixed_quota": defaultMonthlyFixedQuota,
				"updated_at":  common.GetTimestamp(),
			}).Error
		if err2 != nil {
			common.SysError(fmt.Sprintf("resetMonthlyFixedQuotaOnce update error for id %d: %v", row.Id, err2))
			continue
		}
		log := CliendUserQuotaLog{
			ClientUserId: row.ClientUserId,
			AdminUserId:  0,
			Action:       "monthly_fixed_reset",
			OldFixed:     oldFixed,
			NewFixed:     defaultMonthlyFixedQuota,
			OldTemp:      row.TempQuota,
			OldUsed:      row.UsedQuota,
			NewTemp:      row.TempQuota,
			NewUsed:      row.UsedQuota,
			Remark:       fmt.Sprintf("每月固定预算自动重置为%d", defaultMonthlyFixedQuota),
			CreatedAt:    time.Now().Unix(),
		}
		_ = DB.Create(&log).Error
	}
	common.SysLog(fmt.Sprintf("resetMonthlyFixedQuotaOnce completed, reset %d records", len(rows)))
}

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
