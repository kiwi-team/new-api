package model

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

type ErrorLog struct {
	Id             int     `json:"id"`
	UserId         int     `json:"user_id" gorm:"index"`
	CreatedAt      int64   `json:"created_at" gorm:"bigint;index:idx_error_created_a"`
	ChannelId      int     `json:"channel_id" gorm:"index"`
	ChannelName    string  `json:"channel_name" gorm:"default:''"`
	TokenId        int     `json:"token_id" gorm:"index"`
	TokenName      string  `json:"token_name"`
	ModelName      string  `json:"model_name" gorm:"default:''"`
	Message        string  `json:"message" gorm:"default:''"`
	Type           string  `json:"type" gorm:"default:''"`
	Param          string  `json:"param" gorm:"default:''"`
	Code           string  `json:"code" gorm:"default:''"`
	RequestId      string  `json:"request_id" gorm:"default:'';index:idx_error_request_id"`
	StatusCode     int     `json:"status_code" gorm:"default:0"`
	UseTimeMs      int64   `json:"use_time_ms" gorm:"default:0"`
	Body           string  `json:"body" gorm:"default:''"`
	Ip             string  `json:"ip" gorm:"default:''"`
	ClientUserId   string  `json:"client_user_id" gorm:"default:''"`
	ClientScenairo string  `json:"client_scenairo" gorm:"index;size:200;default:''"`
	Extra          *string `json:"extra,omitempty" gorm:"type:jsonb"`
	Header         *string `json:"header,omitempty" gorm:"type:jsonb"`
}

// GetErrorLogBody 根据ID获取错误日志的body字段
func GetErrorLogBody(id int) (string, error) {
	var body string
	err := LOG_DB.Model(&ErrorLog{}).Where("id = ?", id).Pluck("body", &body).Error
	return body, err
}

// GetSelfErrorLogBody 在自助视图范围内获取错误日志 body：
// 仅当该日志属于 (user_id = scopeUserId OR client_user_id IN scopeUids) 时才返回，否则返回空。
func GetSelfErrorLogBody(id int, scopeUserId int, scopeUids []string) (string, error) {
	q := LOG_DB.Model(&ErrorLog{}).Where("id = ?", id)
	if len(scopeUids) > 0 {
		q = q.Where("(user_id = ? OR client_user_id IN ?)", scopeUserId, scopeUids)
	} else {
		q = q.Where("user_id = ?", scopeUserId)
	}
	var body string
	err := q.Pluck("body", &body).Error
	return body, err
}

func GetAllErrorLog(req *dto.ErrorLogsRequest) ([]*ErrorLog, int64, error) {
	var errorLogs []*ErrorLog
	var err error
	channelId := req.ChannelId
	page := req.Page
	pageSize := req.PageSize
	if page <= 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	num := pageSize
	startIdx := (page - 1) * num
	var query *gorm.DB
	if os.Getenv("LOG_SQL_DSN") != "" {
		query = LOG_DB.Model(&ErrorLog{})
	} else {
		query = LOG_DB.Model(&ErrorLog{}).Joins("left join tokens on error_logs.token_id = tokens.id").Select("error_logs.*, tokens.name as token_name")
	}
	if channelId > 0 {
		query = query.Where("channel_id = ? ", channelId)
	}
	if req.EndTime-req.StartTime > 7*86400 {
		return nil, 0, errors.New("错误日志最多只支持查询7天的范围")
	}
	if req.StartTime > 0 {
		query = query.Where("created_at >= ?", req.StartTime)
	} else {
		query = query.Where("created_at >= ?", common.GetTimestamp()-7*86400)
	}
	if req.EndTime > 0 {
		query = query.Where("created_at <= ?", req.EndTime)
	}
	if req.RequestId != "" {
		query = query.Where("request_id = ?", req.RequestId)
	}
	if req.ModelName != "" {
		query = query.Where("model_name = ?", req.ModelName)
	}
	if req.TokenId > 0 {
		query = query.Where("token_id = ?", req.TokenId)
	}
	if req.ClientUserId != "" {
		query = query.Where("client_user_id = ?", req.ClientUserId)
	}
	// 非管理员自助视图：限定为本账号或其关联 uid 的错误日志（并集）
	if req.ScopeUserId > 0 {
		if len(req.ScopeUids) > 0 {
			query = query.Where("(user_id = ? OR client_user_id IN ?)", req.ScopeUserId, req.ScopeUids)
		} else {
			query = query.Where("user_id = ?", req.ScopeUserId)
		}
	}
	// extra 是 PG jsonb 列，按嵌套字段精确匹配（NULL 行天然不命中，符合预期）
	if req.MtSessionId != "" {
		query = query.Where("extra->>'mt_session_id' = ?", req.MtSessionId)
	}
	if req.TraceId != "" {
		query = query.Where("extra->>'trace_id' = ?", req.TraceId)
	}
	if req.TrajId != "" {
		query = query.Where("extra->>'traj_id' = ?", req.TrajId)
	}
	var total int64
	_ = query.Count(&total)
	// 不返回body字段，减少数据传输量
	err = query.Omit("body").Order("id desc").Limit(num).Offset(startIdx).Find(&errorLogs).Error
	tokenIds := make([]int, 0)
	for _, log := range errorLogs {
		if slices.Contains(tokenIds, log.TokenId) {
			continue
		}
		tokenIds = append(tokenIds, log.TokenId)
	}
	if len(tokenIds) > 0 && os.Getenv("LOG_SQL_DSN") != "" {
		var tokens []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err1 := DB.Model(&Token{}).
			Select("id, name").
			Where("id IN ?", tokenIds).
			Find(&tokens).Error; err1 == nil {
			tokenNames := make(map[int]string, len(tokens))
			for _, t := range tokens {
				tokenNames[t.Id] = t.Name
			}
			for i := range errorLogs {
				errorLogs[i].TokenName = tokenNames[errorLogs[i].TokenId]
			}
		}
	}
	return errorLogs, total, err
}

var LogList []*ErrorLog

// multipartFileInfo 描述 multipart 上传中文件 part 的元信息（不含原始字节）。
type multipartFileInfo struct {
	Name        string `json:"name"`
	Filename    string `json:"filename,omitempty"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type,omitempty"`
}

// sanitizedMultipartBody 是 multipart/form-data 脱敏后的可入库 JSON 形态。
type sanitizedMultipartBody struct {
	Multipart  bool                `json:"_multipart"`
	MediaType  string              `json:"_media_type"`
	Fields     map[string]string   `json:"fields,omitempty"`
	Files      []multipartFileInfo `json:"files,omitempty"`
	ParseError string              `json:"_parse_error,omitempty"`
}

// 单个文本字段最多保留的字节数；超过则截断并标注总长度。
const maxMultipartFieldBytes = 4096

// 回退路径下原始 body 最多保留的字节数（解析无果时用，避免日志过大）。
const maxFallbackBodyBytes = 4096

// sanitizeForPGText 把任意字节字符串净化成可写入 PostgreSQL TEXT 列的形态：
//  1. 替换非法 UTF-8 字节序列（如孤立的 0xff）为 U+FFFD '�'
//  2. 删除 NUL 字节 (0x00) —— 它们是合法 UTF-8，但 PG 的 TEXT 列单独拒绝，
//     会触发 SQLSTATE 22021 "invalid byte sequence for encoding UTF8: 0x00"
func sanitizeForPGText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if strings.IndexByte(s, 0x00) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return s
}

// safeUTF8Snapshot 返回 PG-TEXT 安全、必要时截断的 body 文本，用于无法结构化解析时的回退。
// 先按字节截断原始 body，再做 UTF-8 清洗：截断可能切到多字节字符中间，
// 留下孤儿字节，必须由 sanitizeForPGText 替换成 �，否则会触发 PG 22021。
func safeUTF8Snapshot(body string) string {
	if len(body) > maxFallbackBodyBytes {
		return sanitizeForPGText(body[:maxFallbackBodyBytes]) +
			fmt.Sprintf("... [truncated, total %d bytes]", len(body))
	}
	return sanitizeForPGText(body)
}

// sanitizeRequestBodyForLog 把请求 body 转成 PostgreSQL TEXT 列可安全写入的 UTF-8 字符串。
// 对 multipart/form-data：解析出文本字段，文件 part 替换为元信息占位，避免二进制字节写入，
// 并对超大文本字段做截断（form-data 可能夹带图片等二进制数据）。
// 其他 Content-Type（纯文本 / JSON 等）：完整保留，不做截断，只用 ToValidUTF8 过滤掉
// 非法 UTF-8 序列、剔除 NUL 字节，防止 PG 报 "invalid byte sequence for encoding UTF8"。
func sanitizeRequestBodyForLog(body string, contentType string) string {
	if body == "" {
		return body
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return sanitizeForPGText(body)
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return safeUTF8Snapshot(body)
	}

	sanitized := sanitizedMultipartBody{
		Multipart: true,
		MediaType: mediaType,
		Fields:    make(map[string]string),
	}

	reader := multipart.NewReader(strings.NewReader(body), boundary)
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			sanitized.ParseError = partErr.Error()
			break
		}

		partName := part.FormName()
		partFilename := part.FileName()
		partContentType := part.Header.Get("Content-Type")

		partBytes, _ := io.ReadAll(part)
		_ = part.Close()

		if partFilename != "" || isBinaryContentType(partContentType) {
			sanitized.Files = append(sanitized.Files, multipartFileInfo{
				Name:        partName,
				Filename:    partFilename,
				Size:        len(partBytes),
				ContentType: partContentType,
			})
			continue
		}

		value := string(partBytes)
		if len(value) > maxMultipartFieldBytes {
			value = value[:maxMultipartFieldBytes] + fmt.Sprintf("... [truncated, total %d bytes]", len(partBytes))
		}
		sanitized.Fields[partName] = sanitizeForPGText(value)
	}

	// multipart 解析没拿到任何 part：boundary 不匹配 / body 已被替换 / 客户端格式不对，
	// 退回到 UTF-8 安全的截断快照，至少保留诊断价值。
	if len(sanitized.Fields) == 0 && len(sanitized.Files) == 0 {
		return safeUTF8Snapshot(body)
	}

	out, marshalErr := common.Marshal(sanitized)
	if marshalErr != nil {
		return safeUTF8Snapshot(body)
	}
	return string(out)
}

func isBinaryContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	mt = strings.ToLower(mt)
	if strings.HasPrefix(mt, "text/") {
		return false
	}
	switch mt {
	case "application/json", "application/xml", "application/x-www-form-urlencoded":
		return false
	}
	return strings.HasPrefix(mt, "image/") ||
		strings.HasPrefix(mt, "audio/") ||
		strings.HasPrefix(mt, "video/") ||
		strings.HasPrefix(mt, "application/octet-stream")
}

func SaveErrorLog(userId int, channelId int, channelName string, modelName string, err types.OpenAIError, body string, contentType string, requestId string, ip string, tokenId int, clientUserId string, clientScenairo string, extra string, header string, useTimeMs int64, includeBody bool) error {
	// 只调用一次 ToOpenAIError() 方法，避免重复调用
	//openAIError := err.ToOpenAIError()

	bodyToSave := ""
	if includeBody {
		bodyToSave = sanitizeRequestBodyForLog(body, contentType)
	}

	log := &ErrorLog{
		UserId:         userId,
		CreatedAt:      common.GetTimestamp(),
		ChannelId:      channelId,
		Message:        err.Message,
		Type:           string(err.Type),
		Param:          err.Param,
		ChannelName:    channelName,
		ModelName:      modelName,
		Code:           fmt.Sprintf("%v", err.Code),
		StatusCode:     err.StatusCode,
		Body:           bodyToSave,
		Ip:             ip,
		UseTimeMs:      useTimeMs,
		TokenId:        tokenId,
		RequestId:      requestId,
		ClientUserId:   clientUserId,
		ClientScenairo: clientScenairo,
		Extra:          normalizeJsonbString(extra),
		Header:         normalizeJsonbString(header),
	}
	return LOG_DB.Create(log).Error
	// LogList = append(LogList, log)
	// size := len(LogList)
	// if size >= common.ErrorLogBatchSize {
	// 	err1 := LOG_DB.CreateInBatches(LogList, size).Error
	// 	if err1 != nil {
	// 		common.SysError("failed to record error_log: " + err1.Error())
	// 	} else {
	// 		LogList = make([]*ErrorLog, 0)
	// 	}
	// 	return err1
	// }
	//return nil
}

type ErrorLogStatistics struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ModelName   string `json:"model_name"`
	Total       int    `json:"total"`
	Message     string `json:"message"`
	StatusCode  int    `json:"status_code"`
	Code        string `json:"code"`
}

// 统计分析错误日志
// 按照模型统计分析
// 按照渠道统计分析
// 安装渠道-模型统计分析
func StatisticsErrorLog(start int64, end int64) []ErrorLogStatistics {
	var errorLogStatistics []ErrorLogStatistics
	LOG_DB.Model(&ErrorLog{}).Select("channel_id,max(channel_name) as channel_name,model_name,count(*) as total,max(message) as message,max(status_code) as status_code,code").Where("created_at > ? and created_at < ?", start, end).Group("model_name,channel_id,code").Order("total desc").Scan(&errorLogStatistics)
	return errorLogStatistics
}

func DeleteErrorLog(createTime int64, limit int) error {
	if limit == 0 {
		limit = 100
	}
	var ids []int
	err := LOG_DB.Model(&ErrorLog{}).Where("created_at < ?", createTime).Order("id asc").Limit(limit).Pluck("id", &ids).Error
	if err != nil {
		return err
	}
	if len(ids) <= 2 {
		return nil
	}
	maxId := slices.Max(ids)
	minId := slices.Min(ids)
	return LOG_DB.Where("id >= ? and id <= ?", minId, maxId).Delete(&ErrorLog{}).Error
}
