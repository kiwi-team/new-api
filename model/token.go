package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

type Token struct {
	Id                       int     `json:"id"`
	AuthVersion              int64   `json:"-" gorm:"not null;default:1"`
	UserId                   int     `json:"user_id" gorm:"index"`
	Key                      string  `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status                   int     `json:"status" gorm:"default:1"`
	Name                     string  `json:"name" gorm:"index" `
	CreatedTime              int64   `json:"created_time" gorm:"bigint"`
	AccessedTime             int64   `json:"accessed_time" gorm:"bigint"`
	ExpiredTime              int64   `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota              int     `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota           bool    `json:"unlimited_quota"`
	ModelLimitsEnabled       bool    `json:"model_limits_enabled"`
	ModelLimits              string  `json:"model_limits" gorm:"type:text"`
	AllowIps                 *string `json:"allow_ips" gorm:"default:''"`
	UsedQuota                int     `json:"used_quota" gorm:"default:0"`        // used quota
	ChannelRules             string  `json:"channel_rules" gorm:"default:'{}'"`  // 这个key配置的渠道规则
	ChannelRulesHighPriority bool    `json:"channel_rules_high_priority"`        // Key 渠道规则是否覆盖特殊渠道规则
	ChannelRatios            string  `json:"channel_ratios" gorm:"default:'{}'"` // 这个key下面，配置渠道的倍率
	Group                    string  `json:"group" gorm:"default:''"`
	CrossGroupRetry          bool    `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	AutoGroups               string  `json:"-" gorm:"type:text"`
	// Key维度消耗预警：每消耗 AlertThreshold 美元，通过用户设置的 WebhookUrl 发送告警。
	// AlertThreshold = 0 表示关闭告警。AlertNotifiedQuota 记录上次告警时累计消耗（quota 单位），
	// AlertLastNotifiedTime 记录上次告警时间戳（用于展示告警时间范围）。
	AlertThreshold        float64        `json:"alert_threshold" gorm:"default:0"`
	AlertNotifiedQuota    int            `json:"alert_notified_quota" gorm:"default:0"`
	AlertLastNotifiedTime int64          `json:"alert_last_notified_time" gorm:"bigint;default:0"`
	DeletedAt             gorm.DeletedAt `gorm:"index"`
	// New tokens opt in; migrated tokens keep quota queries disabled.
	AllowUserQuotaQuery bool `json:"allow_user_quota_query"`
}

func (token *Token) GetAutoGroups() ([]string, error) {
	if token.AutoGroups == "" {
		return nil, nil
	}
	var groups []string
	if err := common.UnmarshalJsonStr(token.AutoGroups, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (token *Token) SetAutoGroups(groups []string) error {
	if len(groups) == 0 {
		token.AutoGroups = ""
		return nil
	}
	data, err := common.Marshal(groups)
	if err != nil {
		return err
	}
	token.AutoGroups = string(data)
	return nil
}

func (token *Token) Clean() {
	token.Key = ""
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func (token *Token) GetFullKey() string {
	return token.Key
}

func (token *Token) GetMaskedKey() string {
	return MaskTokenKey(token.Key)
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.SplitSeq(cleanIps, "\n")
	for ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	tx := DB.Order("id desc").Limit(num).Offset(startIdx)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	err = tx.Find(&tokens).Error
	return tokens, err
}

// GetTokenListForDropdown returns a lightweight token list (id + name) for dropdown selection.
// If userId > 0, only returns tokens belonging to that user; otherwise returns all tokens.
func GetTokenListForDropdown(userId int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	tx := DB.Model(&Token{}).Select("id, name, user_id, " + commonKeyCol)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	err := tx.Order("id desc").Find(&results).Error
	return results, err
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

func validateLikePattern(input string) error {
	// 1. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return common.NewMessage("The search pattern cannot contain consecutive % wildcards")
	}

	// 2. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return common.NewMessage("The search pattern can contain at most 2 % wildcards")
	}

	// 3. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return common.NewMessage("Fuzzy search keywords must be at least 2 characters long")
		}
	}

	return nil
}

const searchHardLimit = 100

// func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
func SearchUserTokens(userId int, keyword string, token string, modelName string, channel string, offset int, limit int) (tokens []*Token, total int64, err error) {
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog(common.LogText("failed to count user tokens: %s", err.Error()))
			return nil, 0, common.NewMessage("Failed to get the token count")
		}
		if int(count) > maxTokens {
			return nil, 0, common.NewMessage("Token count exceeds the limit. Only exact search is allowed, do not use the % wildcard")
		}
	}

	baseQuery := DB.Model(&Token{})
	if userId > 0 {
		baseQuery = baseQuery.Where("user_id = ?", userId)
	}

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError(common.LogText("failed to count search tokens: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search tokens")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError(common.LogText("failed to search tokens: %s", err.Error()))
		return nil, 0, common.NewMessage("Failed to search tokens")
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted ||
			token.Status == common.TokenStatusExpired ||
			token.Status != common.TokenStatusEnabled {
			return token, ErrTokenInvalid
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog(common.LogText("failed to update token status%s", err.Error()))
				}
			}
			return token, ErrTokenInvalid
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog(common.LogText("failed to update token status%s", err.Error()))
				}
			}
			return token, ErrTokenInvalid
		}
		return token, nil
	}
	common.SysLog(common.LogText("ValidateUserToken: failed to get token: %s", err.Error()))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("无效的令牌")
	} else {
		return nil, errors.New("无效的令牌，数据库查询出错，请联系管理员")
	}
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 {
		return nil, common.NewMessage("Token ID or user ID is empty")
	}
	token := &Token{}
	tx := DB.Where("id = ?", id)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	err := tx.First(token).Error
	return token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("token id is empty")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	token = &Token{}
	if err = DB.Where(commonKeyCol+" = ?", key).First(token).Error; err != nil {
		return nil, err
	}
	if common.RedisEnabled {
		// 冷缓存时用数据库快照初始化；已存在的哈希只刷新 TTL，
		// 避免快照覆盖 Redis 中已被原子预扣的余额。初始化失败不影响本次读取。
		if _, cacheErr := cacheInitToken(*token); cacheErr != nil {
			common.SysLog(common.LogText("failed to init token cache: %s", cacheErr.Error()))
		}
	}
	return token, nil
}

func (token *Token) Insert() error {
	var err error
	err = DB.Create(token).Error
	return err
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	// 写库前失效缓存并设置 fence，防止并发读者把过期快照重新写回缓存。
	if cacheErr := invalidateTokenCacheForMutation(token.Key); cacheErr != nil {
		common.SysLog(common.LogText("failed to invalidate token cache before update: %s", cacheErr.Error()))
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Token{}).Where("id = ?", token.Id).Update("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
			return err
		}
		return tx.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
			"model_limits_enabled", "model_limits", "allow_ips", "group", "channel_rules", "channel_rules_high_priority", "channel_ratios", "cross_group_retry", "auto_groups", "alert_threshold", "allow_user_quota_query").Updates(token).Error
	})
}

func (token *Token) SelectUpdate() (err error) {
	if cacheErr := invalidateTokenCacheForMutation(token.Key); cacheErr != nil {
		common.SysLog(common.LogText("failed to invalidate token cache before status update: %s", cacheErr.Error()))
	}
	// This can update zero values
	return DB.Model(token).Updates(map[string]any{"accessed_time": token.AccessedTime, "status": token.Status, "auth_version": gorm.Expr("auth_version + 1")}).Error
}

func (token *Token) Delete() (err error) {
	if cacheErr := invalidateTokenCacheForMutation(token.Key); cacheErr != nil {
		common.SysLog(common.LogText("failed to invalidate token cache before delete: %s", cacheErr.Error()))
	}
	return DB.Delete(token).Error
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func (token *Token) GetChannelRules() map[string]dto.ChannelRulesItem {
	channelRules := token.ChannelRules
	if channelRules == "{}" || channelRules == "" {
		return nil
	}
	var channelRulesMap map[string]dto.ChannelRulesItem
	err := common.Unmarshal([]byte(channelRules), &channelRulesMap)
	if err != nil {
		return nil
	}
	return channelRulesMap
}

func (token *Token) GetChannelRatios() map[int]float64 {
	channelRatios := token.ChannelRatios
	if channelRatios == "{}" || channelRatios == "" {
		return nil
	}
	channelRatioMap := make(map[int]float64)
	var jsonData map[string]float64
	err := common.Unmarshal([]byte(channelRatios), &jsonData)
	if err != nil {
		return nil
	}
	for k, v := range jsonData {
		if channelId, err := strconv.Atoi(k); err == nil {
			channelRatioMap[channelId] = v
		}
	}
	return channelRatioMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// userId > 0: normal user can only delete own tokens
	// userId == 0: root user can delete any token
	if id == 0 {
		return common.NewMessage("Token ID or user ID is empty")
	}
	token := Token{}
	tx := DB.Where("id = ?", id)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	err = tx.First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(tokenId int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if err := increaseTokenQuota(tokenId, quota); err != nil {
		return err
	}
	if err := invalidateTokenCacheForMutation(key); err != nil {
		common.SysLog(common.LogText("failed to increase token quota: %s", err.Error()))
	}
	return nil
}

func increaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]any{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if err := decreaseTokenQuota(id, quota); err != nil {
		return err
	}
	if err := invalidateTokenCacheForMutation(key); err != nil {
		common.SysLog(common.LogText("failed to decrease token quota: %s", err.Error()))
	}
	return nil
}

func decreaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]any{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
// BillTokenOption 账单页 key 筛选下拉框的选项
type BillTokenOption struct {
	Id       int    `json:"id" gorm:"column:id"`
	Name     string `json:"name" gorm:"column:name"`
	UserId   int    `json:"user_id" gorm:"column:user_id"`
	Username string `json:"username" gorm:"column:username"`
}

// SearchTokensForBill 为账单页 key 筛选下拉框返回 token 选项。
// userId > 0 时只返回该用户的 token（普通用户）；userId == 0 时返回所有用户的 token（root）。
// keyword 非空时按 token 名称模糊匹配，limit 限制返回条数。
func SearchTokensForBill(userId int, keyword string, limit int) ([]*BillTokenOption, error) {
	opts := make([]*BillTokenOption, 0)
	tx := DB.Model(&Token{}).
		Select("tokens.id as id, tokens.name as name, tokens.user_id as user_id, users.username as username").
		Joins("left join users on users.id = tokens.user_id")
	if userId > 0 {
		tx = tx.Where("tokens.user_id = ?", userId)
	}
	if keyword != "" {
		tx = tx.Where("tokens.name LIKE ?", "%"+keyword+"%")
	}
	err := tx.Order("tokens.id desc").Limit(limit).Find(&opts).Error
	return opts, err
}

func CountUserTokens(userId int) (int64, error) {
	var total int64
	tx := DB.Model(&Token{})
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	err := tx.Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, common.NewMessage("Token IDs cannot be empty")
	}

	tx := DB.Begin()

	var tokens []Token
	query := tx.Where("id IN (?)", ids)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if err := query.Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysLog(common.LogText("failed to invalidate token cache before batch delete: %s", err.Error()))
	}

	delQuery := tx.Where("id IN (?)", ids)
	if userId > 0 {
		delQuery = delQuery.Where("user_id = ?", userId)
	}
	if err := delQuery.Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	return len(tokens), nil
}

func BatchSetTokenGroup(ids []int, group string, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	if common.RedisEnabled {
		var tokens []Token
		selectQuery := DB.Model(&Token{}).Select("id", commonKeyCol).Where("id IN (?)", ids)
		if userId > 0 {
			selectQuery = selectQuery.Where("user_id = ?", userId)
		}
		if err := selectQuery.Find(&tokens).Error; err != nil {
			return 0, err
		}
		if err := invalidateTokensCache(tokens); err != nil {
			common.SysLog("failed to invalidate token cache before batch set group: " + err.Error())
		}
	}

	query := DB.Model(&Token{}).Where("id IN (?)", ids)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	result := query.Updates(map[string]any{"group": group, "auth_version": gorm.Expr("auth_version + 1")})
	if result.Error != nil {
		return 0, result.Error
	}

	return int(result.RowsAffected), nil
}

func BatchAppendTokenModelsByGroup(group string, newModels []string) (int, error) {
	if len(newModels) == 0 {
		return 0, errors.New("models 不能为空！")
	}

	var tokens []Token
	err := DB.Where(commonGroupCol+" = ?", group).Find(&tokens).Error
	if err != nil {
		return 0, err
	}
	if len(tokens) == 0 {
		return 0, nil
	}

	updated := 0
	for i := range tokens {
		token := &tokens[i]
		existingMap := token.GetModelLimitsMap()
		var toAdd []string
		for _, m := range newModels {
			m = strings.TrimSpace(m)
			if m != "" && !existingMap[m] {
				toAdd = append(toAdd, m)
			}
		}
		if len(toAdd) == 0 {
			continue
		}
		newLimits := token.ModelLimits
		if newLimits != "" {
			newLimits += ","
		}
		newLimits += strings.Join(toAdd, ",")

		updates := map[string]any{
			"auth_version":         gorm.Expr("auth_version + 1"),
			"model_limits":         newLimits,
			"model_limits_enabled": true,
		}
		if err := invalidateTokenCacheForMutation(token.Key); err != nil {
			common.SysLog("failed to invalidate token cache before appending models: " + err.Error())
		}
		if err := DB.Model(&Token{}).Where("id = ?", token.Id).Updates(updates).Error; err != nil {
			return updated, err
		}
		updated++
	}

	return updated, nil
}

func GetTokenKeysByIds(ids []int, userId int) ([]Token, error) {
	var tokens []Token
	err := DB.Select("id", commonKeyCol).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Find(&tokens).Error
	return tokens, err
}

// InvalidateUserTokensCache 清理指定用户所有令牌在 Redis 中的缓存，
// 配合 InvalidateUserCache 使用，可在用户被禁用/删除时立即阻断其令牌的请求。
// 下一次请求将从数据库重新加载令牌及用户状态，从而立即识别出被禁用的用户。
func InvalidateUserTokensCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 {
		return errors.New("invalid user id")
	}
	var tokens []Token
	if err := DB.Unscoped().
		Select("id", commonKeyCol).
		Where("user_id = ?", userId).
		Find(&tokens).Error; err != nil {
		return err
	}
	return invalidateTokensCache(tokens)
}

func invalidateTokensCache(tokens []Token) error {
	if !common.RedisEnabled {
		return nil
	}
	var firstErr error
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if err := invalidateTokenCacheForMutation(t.Key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
