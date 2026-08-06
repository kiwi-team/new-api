package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrUsageDayNotFound = errors.New("usage day not found")

// UsageAdjustment records an admin-only correction without mutating quota_data.
// Deltas are applied to the matching day, key and model when reports are read.
// HourStart is retained as the database column name for compatibility. New
// records store the UTC+8 day start; legacy hourly records are folded into the
// same day snapshot.
type UsageAdjustment struct {
	Id                               int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId                           int    `json:"user_id" gorm:"index;not null"`
	HourStart                        int64  `json:"hour_start" gorm:"index:idx_usage_adjustment_target,priority:1;not null"`
	TokenId                          int    `json:"token_id" gorm:"index:idx_usage_adjustment_target,priority:2;not null"`
	TokenName                        string `json:"token_name" gorm:"size:64;not null"`
	ModelName                        string `json:"model_name" gorm:"size:200;index:idx_usage_adjustment_target,priority:3;not null"`
	PromptTokensDelta                int64  `json:"prompt_tokens_delta" gorm:"type:bigint;not null"`
	CompletionTokensDelta            int64  `json:"completion_tokens_delta" gorm:"type:bigint;not null"`
	CachedTokensDelta                int64  `json:"cached_tokens_delta" gorm:"type:bigint;not null"`
	ClaudeCacheCreation5mTokensDelta int64  `json:"claude_cache_creation_5m_tokens_delta" gorm:"type:bigint;not null"`
	ClaudeCacheCreation1hTokensDelta int64  `json:"claude_cache_creation_1h_tokens_delta" gorm:"type:bigint;not null"`
	QuotaDelta                       int64  `json:"quota_delta" gorm:"type:bigint;not null"`
	Reason                           string `json:"reason" gorm:"type:text;not null"`
	Ticket                           string `json:"ticket" gorm:"size:200;not null"`
	OperatorId                       int    `json:"operator_id" gorm:"index;not null"`
	OperatorName                     string `json:"operator_name" gorm:"size:64;not null"`
	CreatedAt                        int64  `json:"created_at" gorm:"type:bigint;autoCreateTime"`
	RevertedAt                       int64  `json:"reverted_at" gorm:"type:bigint;index;not null"`
	RevertedBy                       int    `json:"reverted_by" gorm:"not null"`
	RevertedByName                   string `json:"reverted_by_name" gorm:"size:64;not null"`
	RevertReason                     string `json:"revert_reason" gorm:"type:text;not null"`
}

type UsageDayValues struct {
	PromptTokens                int64   `json:"input_tokens"`
	CompletionTokens            int64   `json:"output_tokens"`
	CachedTokens                int64   `json:"cache_read_tokens"`
	ClaudeCacheCreation5mTokens int64   `json:"cache_write_5m_tokens"`
	ClaudeCacheCreation1hTokens int64   `json:"cache_write_1h_tokens"`
	Quota                       int64   `json:"quota"`
	CostUsd                     float64 `json:"cost_usd"`
}

type UsageDaySnapshot struct {
	Date        string             `json:"date"`
	UserId      int                `json:"user_id"`
	TokenId     int                `json:"token_id"`
	TokenName   string             `json:"token_name"`
	ModelName   string             `json:"model_name"`
	Original    UsageDayValues     `json:"original"`
	Effective   UsageDayValues     `json:"effective"`
	Adjustments []*UsageAdjustment `json:"adjustments"`
}

type UsageCorrectionTarget struct {
	PromptTokens                int64
	CompletionTokens            int64
	CachedTokens                int64
	ClaudeCacheCreation5mTokens int64
	ClaudeCacheCreation1hTokens int64
	CostUsd                     float64
}

type usageDayAggregate struct {
	RowCount                    int64  `gorm:"column:row_count"`
	UserId                      int    `gorm:"column:user_id"`
	TokenName                   string `gorm:"column:token_name"`
	PromptTokens                int64  `gorm:"column:prompt_tokens"`
	CompletionTokens            int64  `gorm:"column:completion_tokens"`
	CachedTokens                int64  `gorm:"column:cached_tokens"`
	ClaudeCacheCreation5mTokens int64  `gorm:"column:claude_cache_creation5m_tokens"`
	ClaudeCacheCreation1hTokens int64  `gorm:"column:claude_cache_creation1h_tokens"`
	Quota                       int64  `gorm:"column:quota"`
}

type usageAdjustmentRowKey struct {
	Date      string
	UserId    int
	TokenId   int
	ModelName string
}

func applyUsageAdjustmentsToDailyRows(rows []*ModelUsageAnalysisRow, userId int, startTime int64, endTime int64) ([]*ModelUsageAnalysisRow, error) {
	adjustments := make([]*UsageAdjustment, 0)
	query := DB.Where("reverted_at = 0 AND hour_start >= ? AND hour_start <= ?", startTime, endTime)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if err := query.Find(&adjustments).Error; err != nil {
		return rows, err
	}

	rowByKey := make(map[usageAdjustmentRowKey]*ModelUsageAnalysisRow, len(rows))
	for _, row := range rows {
		key := usageAdjustmentRowKey{Date: row.Date, UserId: row.UserId, TokenId: row.TokenId, ModelName: row.ModelName}
		rowByKey[key] = row
	}
	chinaTime := time.FixedZone("UTC+8", 8*60*60)
	for _, adjustment := range adjustments {
		key := usageAdjustmentRowKey{
			Date:      time.Unix(adjustment.HourStart, 0).In(chinaTime).Format("2006-01-02"),
			UserId:    adjustment.UserId,
			TokenId:   adjustment.TokenId,
			ModelName: adjustment.ModelName,
		}
		row := rowByKey[key]
		if row == nil {
			row = &ModelUsageAnalysisRow{
				Date:      key.Date,
				UserId:    key.UserId,
				TokenId:   key.TokenId,
				TokenName: adjustment.TokenName,
				ModelName: key.ModelName,
			}
			rows = append(rows, row)
			rowByKey[key] = row
		}
		row.InputTokens += adjustment.PromptTokensDelta
		row.OutputTokens += adjustment.CompletionTokensDelta
		row.CacheReadTokens += adjustment.CachedTokensDelta
		row.CacheWriteTokens += adjustment.ClaudeCacheCreation5mTokensDelta + adjustment.ClaudeCacheCreation1hTokensDelta
		row.Quota += adjustment.QuotaDelta
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date > rows[j].Date
		}
		if rows[i].UserId != rows[j].UserId {
			return rows[i].UserId < rows[j].UserId
		}
		if rows[i].TokenId != rows[j].TokenId {
			return rows[i].TokenId < rows[j].TokenId
		}
		return rows[i].ModelName < rows[j].ModelName
	})
	return rows, nil
}

func usageDayBounds(date string) (int64, int64, error) {
	date = strings.TrimSpace(date)
	dayStart, err := time.ParseInLocation("2006-01-02", date, time.FixedZone("UTC+8", 8*60*60))
	if err != nil || dayStart.Format("2006-01-02") != date {
		return 0, 0, errors.New("invalid usage day target")
	}
	return dayStart.Unix(), dayStart.AddDate(0, 0, 1).Unix(), nil
}

func GetUsageDaySnapshot(date string, tokenId int, modelName string) (*UsageDaySnapshot, error) {
	dayStart, dayEnd, err := usageDayBounds(date)
	if err != nil || tokenId <= 0 || strings.TrimSpace(modelName) == "" {
		return nil, errors.New("invalid usage day target")
	}

	var raw usageDayAggregate
	err = DB.Model(&QuotaData{}).
		Select("COUNT(*) as row_count, MAX(user_id) as user_id, MAX(token_name) as token_name, "+
			"SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, "+
			"SUM(cached_tokens) as cached_tokens, SUM(claude_cache_creation5m_tokens) as claude_cache_creation5m_tokens, "+
			"SUM(claude_cache_creation1h_tokens) as claude_cache_creation1h_tokens, SUM(quota) as quota").
		Where("created_at >= ? AND created_at < ? AND token_id = ? AND model_name = ?", dayStart, dayEnd, tokenId, modelName).
		Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	if raw.RowCount == 0 {
		return nil, ErrUsageDayNotFound
	}

	adjustments := make([]*UsageAdjustment, 0)
	if err := DB.Where("hour_start >= ? AND hour_start < ? AND token_id = ? AND model_name = ?", dayStart, dayEnd, tokenId, modelName).
		Order("id DESC").Find(&adjustments).Error; err != nil {
		return nil, err
	}

	original := UsageDayValues{
		PromptTokens:                raw.PromptTokens,
		CompletionTokens:            raw.CompletionTokens,
		CachedTokens:                raw.CachedTokens,
		ClaudeCacheCreation5mTokens: raw.ClaudeCacheCreation5mTokens,
		ClaudeCacheCreation1hTokens: raw.ClaudeCacheCreation1hTokens,
		Quota:                       raw.Quota,
	}
	original.CostUsd = float64(original.Quota) / common.QuotaPerUnit
	effective := original
	for _, adjustment := range adjustments {
		if adjustment.RevertedAt == 0 {
			applyUsageAdjustment(&effective, adjustment)
		}
	}
	effective.CostUsd = float64(effective.Quota) / common.QuotaPerUnit

	return &UsageDaySnapshot{
		Date:        strings.TrimSpace(date),
		UserId:      raw.UserId,
		TokenId:     tokenId,
		TokenName:   raw.TokenName,
		ModelName:   modelName,
		Original:    original,
		Effective:   effective,
		Adjustments: adjustments,
	}, nil
}

func CreateUsageAdjustment(date string, tokenId int, modelName string, target UsageCorrectionTarget, reason string, ticket string, operatorId int, operatorName string) (*UsageAdjustment, error) {
	reason = strings.TrimSpace(reason)
	ticket = strings.TrimSpace(ticket)
	if reason == "" || len(reason) > 2000 {
		return nil, errors.New("adjustment reason is required and must not exceed 2000 characters")
	}
	if len(ticket) > 200 {
		return nil, errors.New("ticket must not exceed 200 characters")
	}
	if operatorId <= 0 {
		return nil, errors.New("invalid adjustment operator")
	}
	if target.PromptTokens < 0 || target.CompletionTokens < 0 || target.CachedTokens < 0 ||
		target.ClaudeCacheCreation5mTokens < 0 || target.ClaudeCacheCreation1hTokens < 0 ||
		target.CostUsd < 0 || math.IsNaN(target.CostUsd) || math.IsInf(target.CostUsd, 0) {
		return nil, errors.New("corrected usage values must be non-negative")
	}

	dayStart, _, err := usageDayBounds(date)
	if err != nil {
		return nil, err
	}
	snapshot, err := GetUsageDaySnapshot(date, tokenId, modelName)
	if err != nil {
		return nil, err
	}
	quotaDelta, err := common.QuotaRoundStrict((target.CostUsd - snapshot.Effective.CostUsd) * common.QuotaPerUnit)
	if err != nil {
		return nil, fmt.Errorf("invalid corrected cost: %w", err)
	}
	adjustment := &UsageAdjustment{
		UserId:                           snapshot.UserId,
		HourStart:                        dayStart,
		TokenId:                          tokenId,
		TokenName:                        snapshot.TokenName,
		ModelName:                        modelName,
		PromptTokensDelta:                target.PromptTokens - snapshot.Effective.PromptTokens,
		CompletionTokensDelta:            target.CompletionTokens - snapshot.Effective.CompletionTokens,
		CachedTokensDelta:                target.CachedTokens - snapshot.Effective.CachedTokens,
		ClaudeCacheCreation5mTokensDelta: target.ClaudeCacheCreation5mTokens - snapshot.Effective.ClaudeCacheCreation5mTokens,
		ClaudeCacheCreation1hTokensDelta: target.ClaudeCacheCreation1hTokens - snapshot.Effective.ClaudeCacheCreation1hTokens,
		QuotaDelta:                       int64(quotaDelta),
		Reason:                           reason,
		Ticket:                           ticket,
		OperatorId:                       operatorId,
		OperatorName:                     operatorName,
	}
	if adjustment.isZero() {
		return nil, errors.New("corrected values are unchanged")
	}
	if err := DB.Create(adjustment).Error; err != nil {
		return nil, err
	}
	return adjustment, nil
}

func GetUsageAdjustmentById(id int) (*UsageAdjustment, error) {
	var adjustment UsageAdjustment
	if err := DB.First(&adjustment, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &adjustment, nil
}

func RevertUsageAdjustment(id int, operatorId int, operatorName string, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2000 {
		return errors.New("revert reason is required and must not exceed 2000 characters")
	}
	adjustment, err := GetUsageAdjustmentById(id)
	if err != nil {
		return err
	}
	if adjustment.RevertedAt != 0 {
		return errors.New("adjustment has already been reverted")
	}
	result := DB.Model(&UsageAdjustment{}).
		Where("id = ? AND reverted_at = 0", id).
		Updates(map[string]interface{}{
			"reverted_at":      time.Now().Unix(),
			"reverted_by":      operatorId,
			"reverted_by_name": operatorName,
			"revert_reason":    reason,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func applyUsageAdjustment(values *UsageDayValues, adjustment *UsageAdjustment) {
	values.PromptTokens += adjustment.PromptTokensDelta
	values.CompletionTokens += adjustment.CompletionTokensDelta
	values.CachedTokens += adjustment.CachedTokensDelta
	values.ClaudeCacheCreation5mTokens += adjustment.ClaudeCacheCreation5mTokensDelta
	values.ClaudeCacheCreation1hTokens += adjustment.ClaudeCacheCreation1hTokensDelta
	values.Quota += adjustment.QuotaDelta
}

func (adjustment *UsageAdjustment) isZero() bool {
	return adjustment.PromptTokensDelta == 0 && adjustment.CompletionTokensDelta == 0 &&
		adjustment.CachedTokensDelta == 0 && adjustment.ClaudeCacheCreation5mTokensDelta == 0 &&
		adjustment.ClaudeCacheCreation1hTokensDelta == 0 && adjustment.QuotaDelta == 0
}
