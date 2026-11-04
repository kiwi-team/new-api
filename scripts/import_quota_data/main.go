package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type QuotaData struct {
	Id                          int    `gorm:"primaryKey"`
	UserID                      int    `gorm:"column:user_id"`
	Username                    string `gorm:"column:username"`
	ModelName                   string `gorm:"column:model_name"`
	CreatedAt                   int64  `gorm:"column:created_at"`
	TokenUsed                   int    `gorm:"column:token_used"`
	PromptTokens                int    `gorm:"column:prompt_tokens"`
	CompletionTokens            int    `gorm:"column:completion_tokens"`
	CachedTokens                int    `gorm:"column:cached_tokens"`
	ClaudeCacheCreation5mTokens int    `gorm:"column:claude_cache_creation5m_tokens"`
	ClaudeCacheCreation1hTokens int    `gorm:"column:claude_cache_creation1h_tokens"`
	TokenName                   string `gorm:"column:token_name"`
	Count                       int    `gorm:"column:count"`
	Quota                       int    `gorm:"column:quota"`
	TokenId                     int    `gorm:"column:token_id"`
	ChannelId                   int    `gorm:"column:channel_id"`
	ClientUserId                string `gorm:"column:client_user_id"`
	ClientScenairo              string `gorm:"column:client_scenairo"`
	ProjectName                 string `gorm:"column:project_name"`
	PlanId                      int    `gorm:"column:plan_id"`
}

func (QuotaData) TableName() string {
	return "quota_data"
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go <csv_file>")
		fmt.Println("数据库连接从 .env 文件的 SQL_DSN 读取")
		os.Exit(1)
	}

	// 从 .env 文件加载数据库连接
	_ = godotenv.Load(".env")
	dsn := os.Getenv("SQL_DSN")
	if dsn == "" {
		fmt.Println("错误: 未找到 SQL_DSN 环境变量，请确保 .env 文件存在且包含 SQL_DSN")
		os.Exit(1)
	}

	csvFile := os.Args[1]

	// 连接数据库
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Printf("连接数据库失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("数据库连接成功")

	// 读取 CSV
	f, err := os.Open(csvFile)
	if err != nil {
		fmt.Printf("打开 CSV 文件失败: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	// 读取表头
	header, err := reader.Read()
	if err != nil {
		fmt.Printf("读取 CSV 表头失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("CSV 列: %v\n", header)

	// 读取所有行
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("读取 CSV 数据失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共 %d 条记录\n", len(records))

	// 建立列名到索引的映射
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[col] = i
	}

	// 创建备份文件，记录被覆盖前的旧数据
	backupFile, err := os.Create("backup_before_update.csv")
	if err != nil {
		fmt.Printf("创建备份文件失败: %v\n", err)
		os.Exit(1)
	}
	defer backupFile.Close()
	backupWriter := csv.NewWriter(backupFile)
	defer backupWriter.Flush()
	backupWriter.Write([]string{
		"id", "user_id", "username", "model_name", "created_at",
		"token_used", "prompt_tokens", "completion_tokens",
		"cached_tokens", "claude_cache_creation5m_tokens", "claude_cache_creation1h_tokens",
		"token_name", "count", "quota",
		"token_id", "channel_id", "client_user_id", "client_scenairo", "project_name", "plan_id",
	})

	updated := 0
	inserted := 0
	failed := 0

	for i, row := range records {
		data := parseRow(colIdx, row)

		// 查找是否已存在
		var existing QuotaData
		result := db.Where(
			"user_id = ? AND username = ? AND model_name = ? AND created_at = ? AND token_id = ? AND channel_id = ? AND client_user_id = ? AND client_scenairo = ? AND project_name = ? AND plan_id = ?",
			data.UserID, data.Username, data.ModelName, data.CreatedAt,
			data.TokenId, data.ChannelId, data.ClientUserId, data.ClientScenairo,
			data.ProjectName, data.PlanId,
		).First(&existing)

		if result.Error == nil && existing.Id > 0 {
			// 已存在，累加更新
			err := db.Model(&QuotaData{}).Where("id = ?", existing.Id).Updates(map[string]interface{}{
				"count":                          data.Count,
				"quota":                          data.Quota,
				"token_used":                     data.TokenUsed,
				"prompt_tokens":                  data.PromptTokens,
				"completion_tokens":              data.CompletionTokens,
				"cached_tokens":                  data.CachedTokens,
				"claude_cache_creation5m_tokens": data.ClaudeCacheCreation5mTokens,
				"claude_cache_creation1h_tokens": data.ClaudeCacheCreation1hTokens,
			}).Error
			if err != nil {
				fmt.Printf("[%d] UPDATE 失败: %v\n", i, err)
				failed++
			} else {
				updated++
				// 备份旧数据
				backupWriter.Write([]string{
					strconv.Itoa(existing.Id),
					strconv.Itoa(existing.UserID),
					existing.Username,
					existing.ModelName,
					strconv.FormatInt(existing.CreatedAt, 10),
					strconv.Itoa(existing.TokenUsed),
					strconv.Itoa(existing.PromptTokens),
					strconv.Itoa(existing.CompletionTokens),
					strconv.Itoa(existing.CachedTokens),
					strconv.Itoa(existing.ClaudeCacheCreation5mTokens),
					strconv.Itoa(existing.ClaudeCacheCreation1hTokens),
					existing.TokenName,
					strconv.Itoa(existing.Count),
					strconv.Itoa(existing.Quota),
					strconv.Itoa(existing.TokenId),
					strconv.Itoa(existing.ChannelId),
					existing.ClientUserId,
					existing.ClientScenairo,
					existing.ProjectName,
					strconv.Itoa(existing.PlanId),
				})
			}
		} else {
			// 不存在，插入
			err := db.Create(&data).Error
			if err != nil {
				fmt.Printf("[%d] INSERT 失败: %v\n", i, err)
				failed++
			} else {
				inserted++
			}
		}

		if (i+1)%100 == 0 {
			fmt.Printf("进度: %d/%d (插入: %d, 更新: %d, 失败: %d)\n", i+1, len(records), inserted, updated, failed)
		}
	}

	fmt.Printf("\n完成! 总计: %d, 插入: %d, 更新: %d, 失败: %d\n", len(records), inserted, updated, failed)
}

func parseRow(colIdx map[string]int, row []string) QuotaData {
	return QuotaData{
		UserID:                      atoi(row, colIdx, "user_id"),
		Username:                    str(row, colIdx, "username"),
		ModelName:                   str(row, colIdx, "model_name"),
		CreatedAt:                   atoi64(row, colIdx, "created_at"),
		TokenUsed:                   atoi(row, colIdx, "token_used"),
		PromptTokens:                atoi(row, colIdx, "prompt_tokens"),
		CompletionTokens:            atoi(row, colIdx, "completion_tokens"),
		CachedTokens:                atoi(row, colIdx, "cached_tokens"),
		ClaudeCacheCreation5mTokens: atoi(row, colIdx, "claude_cache_creation5m_tokens"),
		ClaudeCacheCreation1hTokens: atoi(row, colIdx, "claude_cache_creation1h_tokens"),
		TokenName:                   str(row, colIdx, "token_name"),
		Count:                       atoi(row, colIdx, "count"),
		Quota:                       atoi(row, colIdx, "quota"),
		TokenId:                     atoi(row, colIdx, "token_id"),
		ChannelId:                   atoi(row, colIdx, "channel_id"),
		ClientUserId:                str(row, colIdx, "client_user_id"),
		ClientScenairo:              str(row, colIdx, "client_scenairo"),
		ProjectName:                 str(row, colIdx, "project_name"),
		PlanId:                      atoi(row, colIdx, "plan_id"),
	}
}

func str(row []string, colIdx map[string]int, col string) string {
	if idx, ok := colIdx[col]; ok && idx < len(row) {
		return row[idx]
	}
	return ""
}

func atoi(row []string, colIdx map[string]int, col string) int {
	s := str(row, colIdx, col)
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

func atoi64(row []string, colIdx map[string]int, col string) int64 {
	s := str(row, colIdx, col)
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
