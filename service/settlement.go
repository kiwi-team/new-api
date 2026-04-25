package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// SettlementBillItem 单个模型的结算账单项
type SettlementBillItem struct {
	ModelName     string  `json:"model_name"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	RequestCount  int64   `json:"request_count"`
	InputPrice    float64 `json:"input_price"`
	OutputPrice   float64 `json:"output_price"`
	RequestPrice  float64 `json:"request_price"`
	InputAmount   float64 `json:"input_amount"`
	OutputAmount  float64 `json:"output_amount"`
	RequestAmount float64 `json:"request_amount"`
	TotalAmount   float64 `json:"total_amount"`
	Configured    bool    `json:"configured"`
}

// SettlementBill 完整结算账单
type SettlementBill struct {
	UserId      int                   `json:"user_id"`
	StartTime   int64                 `json:"start_time"`
	EndTime     int64                 `json:"end_time"`
	Items       []*SettlementBillItem `json:"items"`
	TotalAmount float64               `json:"total_amount"`
}

// MatchSettlementConfig 给定用户的所有 settlement_config 和一个模型名称，返回匹配的配置。
// 匹配优先级：精确匹配 > 最长前缀通配符匹配 > 未匹配（返回 nil）。
// 通配符规则：模型名称以 * 结尾时，去掉 * 后作为前缀匹配。
func MatchSettlementConfig(configs []*model.SettlementConfig, modelName string) *model.SettlementConfig {
	var bestWildcard *model.SettlementConfig
	bestPrefixLen := -1

	for _, cfg := range configs {
		// 精确匹配优先
		if cfg.ModelName == modelName {
			return cfg
		}
		// 通配符匹配：以 * 结尾
		if prefix, ok := strings.CutSuffix(cfg.ModelName, "*"); ok {
			if strings.HasPrefix(modelName, prefix) && len(prefix) > bestPrefixLen {
				bestWildcard = cfg
				bestPrefixLen = len(prefix)
			}
		}
	}

	return bestWildcard
}

// modelUsage 用于从 quota_data 汇总查询结果
type modelUsage struct {
	ModelName                   string `gorm:"column:model_name"`
	PromptTokens                int64  `gorm:"column:prompt_tokens"`
	CachedTokens                int64  `gorm:"column:cached_tokens"`
	CompletionTokens            int64  `gorm:"column:completion_tokens"`
	Count                       int64  `gorm:"column:count"`
	ClaudeCacheCreation5mTokens int64  `gorm:"column:claude_cache_creation5m_tokens"`
	ClaudeCacheCreation1hTokens int64  `gorm:"column:claude_cache_creation1h_tokens"`
}

// CalculateSettlementBill 计算结算账单（纯只读）
// 1. 从 quota_data 按模型维度汇总 prompt_tokens、cached_tokens、completion_tokens
// 2. 从 settlement_config 获取用户的结算价格配置
// 3. 匹配模型名称（支持通配符），计算金额
// 4. 返回账单，不写入任何数据
func CalculateSettlementBill(userId int, startTime int64, endTime int64) (*SettlementBill, error) {
	// 1. 从 quota_data 按模型维度汇总 token 用量（纯 SELECT 只读查询）
	var usages []modelUsage
	err := model.DB.Table("quota_data").
		Select("model_name, SUM(prompt_tokens) as prompt_tokens, SUM(cached_tokens) as cached_tokens, SUM(completion_tokens) as completion_tokens, SUM(count) as count, SUM(claude_cache_creation5m_tokens) as claude_cache_creation5m_tokens, SUM(claude_cache_creation1h_tokens) as claude_cache_creation1h_tokens").
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userId, startTime, endTime).
		Group("model_name").
		Scan(&usages).Error
	if err != nil {
		return nil, fmt.Errorf("查询用量数据失败: %w", err)
	}

	// 2. 从 settlement_config 获取用户的所有结算价格配置
	configs, err := model.GetSettlementConfigsByUserId(userId)
	if err != nil {
		return nil, fmt.Errorf("查询结算价格配置失败: %w", err)
	}

	// 3. 对每个模型计算账单项
	bill := &SettlementBill{
		UserId:    userId,
		StartTime: startTime,
		EndTime:   endTime,
		Items:     make([]*SettlementBillItem, 0, len(usages)),
	}

	var totalAmount float64

	for _, usage := range usages {
		// 结算口径的输入 Token 计算：
		// - Claude 模型：prompt_tokens + claude_cache_creation_5m_tokens + claude_cache_creation_1h_tokens
		// - 非 Claude 模型：prompt_tokens
		var inputTokens int64
		if strings.HasPrefix(strings.ToLower(usage.ModelName), "claude") {
			inputTokens = usage.PromptTokens + usage.ClaudeCacheCreation5mTokens + usage.ClaudeCacheCreation1hTokens
		} else {
			inputTokens = usage.PromptTokens
		}
		outputTokens := usage.CompletionTokens
		requestCount := usage.Count

		item := &SettlementBillItem{
			ModelName:    usage.ModelName,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			RequestCount: requestCount,
		}

		// 通配符匹配
		matchedConfig := MatchSettlementConfig(configs, usage.ModelName)
		if matchedConfig != nil {
			item.Configured = true
			item.InputPrice = matchedConfig.InputPrice
			item.OutputPrice = matchedConfig.OutputPrice
			item.RequestPrice = matchedConfig.RequestPrice
			item.InputAmount = float64(inputTokens) * (matchedConfig.InputPrice / 1_000_000)
			item.OutputAmount = float64(outputTokens) * (matchedConfig.OutputPrice / 1_000_000)
			item.RequestAmount = float64(requestCount) * matchedConfig.RequestPrice
			item.TotalAmount = item.InputAmount + item.OutputAmount + item.RequestAmount
		}
		// 未配置的模型：configured=false, 金额为 0（零值默认）

		totalAmount += item.TotalAmount
		bill.Items = append(bill.Items, item)
	}

	bill.TotalAmount = totalAmount
	return bill, nil
}

// ValidateTimeRange 验证时间范围参数
// 验证 start_timestamp <= end_timestamp，且范围不超过 3 个月（约 90 天 = 7776000 秒）
func ValidateTimeRange(startTimestamp, endTimestamp int64) error {
	if startTimestamp > endTimestamp {
		return errors.New("开始时间不能晚于结束时间")
	}
	const maxRange int64 = 7776000 // 90 天（秒）
	if endTimestamp-startTimestamp > maxRange {
		return errors.New("时间范围不能超过3个月")
	}
	return nil
}

// ExportSettlementBillCSV 将结算账单导出为 CSV 格式
// CSV 列：模型名称, 输入Token数, 输出Token数, 输入金额, 输出金额, 合计金额
func ExportSettlementBillCSV(bill *SettlementBill) ([]byte, error) {
	var buf bytes.Buffer

	// 写入 UTF-8 BOM，确保 Excel 正确识别中文
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(&buf)

	// 写入表头
	header := []string{"模型名称", "输入Token数", "输出Token数", "请求次数", "输入金额", "输出金额", "次数金额", "合计金额"}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("写入 CSV 表头失败: %w", err)
	}

	// 写入数据行
	for _, item := range bill.Items {
		row := []string{
			item.ModelName,
			fmt.Sprintf("%d", item.InputTokens),
			fmt.Sprintf("%d", item.OutputTokens),
			fmt.Sprintf("%d", item.RequestCount),
			fmt.Sprintf("%.6f", item.InputAmount),
			fmt.Sprintf("%.6f", item.OutputAmount),
			fmt.Sprintf("%.6f", item.RequestAmount),
			fmt.Sprintf("%.6f", item.TotalAmount),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("写入 CSV 数据行失败: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV 写入失败: %w", err)
	}

	return buf.Bytes(), nil
}
