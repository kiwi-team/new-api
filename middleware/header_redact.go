package middleware

// sensitiveHeaderKeys 落库前需要脱敏的请求头（小写匹配，HTTP header 大小写不敏感）。
// 写入 logs/error_logs 的 header 列时，这些头的每个值都会被替换为 "***" + 末 4 位，
// 避免 token / cookie / API key 随日志泄露。
// 新增项时一律小写。
var sensitiveHeaderKeys = map[string]bool{
	"authorization":          true,
	"proxy-authorization":    true,
	"cookie":                 true,
	"set-cookie":             true,
	"x-api-key":              true,
	"api-key":                true,
	"x-api-token":            true,
	"x-auth-token":           true,
	"x-goog-api-key":         true,
	"mj-api-secret":          true,
	"sec-websocket-protocol": true, // 可能携带 openai-insecure-api-key.sk-xxx
}

// redactHeaderValue 把整段值替换为 "***" + 末 4 位；
// 长度 <= 4 时直接返回 "***" 不保留任何字符，避免短 token 被反推。
func redactHeaderValue(v string) string {
	if len(v) <= 4 {
		return "***"
	}
	return "***" + v[len(v)-4:]
}
