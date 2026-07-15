package claude

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// Claude Code 客户端检测（CC Guard）
// ================================
//
// 目标：当渠道开启「Claude Code 客户端检测」后，只放行真实的 Claude Code CLI 请求，
// 拦截克隆工具（OpenCode / RooCode / KiloCode / Cline 等）以及「裸借 OAuth」的伪造客户端。
// 未通过检测的请求被视为该渠道请求失败（记录 error_logs 并转由其他渠道重试）。
//
// 设计参考 debug/cc_guard_reference.py 的两层互补思路，并结合 debug/cc-request-header.csv
// 中 448 条真实 CC 抓包统计得出的强不变量：
//
//   1) 黑名单快速拦截（detectFakeClient）：命中已知克隆工具指纹即拒。误伤低。
//   2) 白名单校验（validateIsClaudeCode）：只认真实 CC 的多维特征，不符即拒。最严格。
//
// 【关键修正】真实 CC 的 system prompt 正文并非恒定：主 agent 是
// "You are Claude Code, Anthropic's official CLI for Claude."，但标题生成子请求是
// "You are a Claude agent, ..."、安全监控子请求是 "You are a security monitor ..."。
// 因此不能把「system 必须等于 CC 模板」作为硬性条件，否则会误杀合法的 CC 子请求。
// 抓包中真正 448/448 全部命中的强不变量是：
//   - User-Agent 形如 claude-cli/x.y.z
//   - anthropic-beta 含 claude-code-20250219
//   - X-App: cli、anthropic-version 存在
//   - metadata.user_id 是含 session_id 的 JSON 串
//   - system 首块是 "x-anthropic-billing-header: cc_version=..." 签名块
// 白名单据此设计：system 命中 CC 官方前缀「或」带 billing 签名块即可（兼容子请求）。

// 真实 Claude Code 的 User-Agent 形如 "claude-cli/2.1.181 (external, cli)"。
var realCCUserAgent = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)

// 真实 CC 官方 system prompt 开头（半公开，维护多个官方变体前缀）。
var ccSystemPromptPrefixes = []string{
	"you are claude code, anthropic's official cli for claude.",
	"you are a claude agent, built on anthropic's claude agent sdk.",
}

// billing 签名块前缀：真实 CC 会把该块作为 system 的首个文本块下发。
const ccBillingHeaderPrefix = "x-anthropic-billing-header: cc_version="

// defaultCCBillingHeader 缺省的 billing 签名块文本。当渠道开启 CC 检测、请求通过检测后，
// 若 system 中不含 billing 块，则在 system 首位插入本文本（渠道可通过配置覆盖）。
const defaultCCBillingHeader = "x-anthropic-billing-header: cc_version=2.1.77.a6c; cc_entrypoint=cli; cch=2e011;"

// anthropic-beta 中真实 CC 恒定携带的特征标记。
const ccBetaMarker = "claude-code-20250219"

// 克隆工具 User-Agent 特征（按需扩展）。
var fakeUAPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)opencode/\d`),
	regexp.MustCompile(`(?i)roo-?code/\d`),
	regexp.MustCompile(`(?i)kilo-?code/\d`),
	regexp.MustCompile(`(?i)cline/\d`),
}

// 克隆工具 system prompt 品牌特征（按需扩展）。
var fakePromptSignatures = []string{
	"you are opencode", "opencode.ai",
	"you are roo", "roocode.com",
	"you are kilo", "kilocode.ai",
	"you are cline",
}

// 非 CC 框架的工具名。CC 原生工具名是 PascalCase（Bash/Read/Write/Edit/Glob/Grep/…）；
// 下面这些 snake_case 名是 Cursor/Cline/Windsurf/Copilot 等第三方框架的特征。
var nonCCToolNames = map[string]struct{}{
	"execute_command":    {},
	"write_to_file":      {},
	"apply_diff":         {},
	"read_file":          {},
	"edit_file":          {},
	"run_terminal_cmd":   {},
	"attempt_completion": {},
}

// dice 相似度阈值：用模糊匹配容忍 CC 不同版本间 system prompt 的细微改版。
const ccSimilarityThreshold = 0.5

// DetectClaudeCode 对一个 Claude 请求做「是否为真实 Claude Code 客户端」的检测。
// 返回 nil 表示通过（是真实 CC）；否则返回带具体原因的 error，供上层写入 error_logs。
//
// 检测顺序：先跑黑名单快速拦截，再跑白名单校验。
func DetectClaudeCode(request *dto.ClaudeRequest, header http.Header, path string) error {
	if request == nil {
		return fmt.Errorf("empty request")
	}

	// 功能 1：黑名单快速拦截。
	if reason := detectFakeClient(request, header); reason != "" {
		return fmt.Errorf("fake client blocked: %s", reason)
	}

	// 功能 2：严格白名单。
	if reason := validateIsClaudeCode(request, header, path); reason != "" {
		return fmt.Errorf("not a genuine claude code request: %s", reason)
	}

	return nil
}

// detectFakeClient 黑名单：命中已知克隆工具指纹返回非空原因串。
func detectFakeClient(request *dto.ClaudeRequest, header http.Header) string {
	ua := strings.ToLower(headerGet(header, "User-Agent"))
	for _, pat := range fakeUAPatterns {
		if pat.MatchString(ua) {
			return "fake_ua:" + pat.String()
		}
	}

	sysText := strings.ToLower(systemText(request))
	for _, sig := range fakePromptSignatures {
		if strings.Contains(sysText, sig) {
			return "fake_prompt:" + sig
		}
	}

	for _, name := range toolNames(request) {
		if _, ok := nonCCToolNames[strings.ToLower(name)]; ok {
			return "non_cc_tool:" + strings.ToLower(name)
		}
	}

	return ""
}

// validateIsClaudeCode 白名单：仅放行真实 CC，不符返回非空失败原因串。
func validateIsClaudeCode(request *dto.ClaudeRequest, header http.Header, path string) string {
	// 1) User-Agent 必须是 claude-cli/x.y.z。
	if !realCCUserAgent.MatchString(headerGet(header, "User-Agent")) {
		return "user_agent_mismatch"
	}

	// 2) 非 messages 路径（如 count_tokens / models），UA 通过即放行。
	if !strings.Contains(path, "messages") {
		return ""
	}

	// 3) 必需请求头（真实 CC 一定携带）。
	if headerGet(header, "X-App") == "" {
		return "missing_header:x-app"
	}
	beta := headerGet(header, "Anthropic-Beta")
	if beta == "" {
		return "missing_header:anthropic-beta"
	}
	if !strings.Contains(strings.ToLower(beta), ccBetaMarker) {
		return "anthropic_beta_mismatch"
	}
	if headerGet(header, "Anthropic-Version") == "" {
		return "missing_header:anthropic-version"
	}

	// 4) system 必须命中 CC 官方前缀「或」带 billing 签名块（二者满足其一即可，兼容子请求）。
	if !looksLikeCCSystem(request) {
		return "system_prompt_mismatch"
	}

	// 5) metadata.user_id 必须存在且为含 session_id 的新版 JSON 格式。
	if !hasValidCCMetadata(request) {
		return "invalid_metadata_user_id"
	}

	return ""
}

// looksLikeCCSystem 判断 system 是否符合真实 CC 特征：
// 命中官方 prompt 前缀，或含 billing 签名块，或与官方模板 Dice 相似度达阈值。
func looksLikeCCSystem(request *dto.ClaudeRequest) bool {
	// billing 签名块：真实 CC 的强信号，任意 system 块命中即可。
	if hasBillingBlock(request) {
		return true
	}

	norm := normalizeText(systemText(request))
	if norm == "" {
		return false
	}
	for _, prefix := range ccSystemPromptPrefixes {
		if strings.HasPrefix(norm, prefix) {
			return true
		}
		if diceCoefficient(norm, prefix) >= ccSimilarityThreshold {
			return true
		}
	}
	return false
}

// hasValidCCMetadata 校验 metadata.user_id 是含 session_id 的 JSON 串。
func hasValidCCMetadata(request *dto.ClaudeRequest) bool {
	if len(request.Metadata) == 0 {
		return false
	}
	var meta struct {
		UserID string `json:"user_id"`
	}
	if err := common.Unmarshal(request.Metadata, &meta); err != nil {
		return false
	}
	uid := strings.TrimSpace(meta.UserID)
	return strings.HasPrefix(uid, "{") && strings.Contains(uid, "session_id")
}

// hasBillingBlock 判断 system 中是否已包含 billing 签名块（任意块命中即可）。
func hasBillingBlock(request *dto.ClaudeRequest) bool {
	for _, block := range systemBlocks(request) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(block)), ccBillingHeaderPrefix) {
			return true
		}
	}
	return false
}

// InsertClaudeCodeBillingHeader 若 system 中缺少 billing 签名块，则在 system 首位插入一个 billing 文本块。
// billingHeader 为要插入的文本，留空时使用内置默认值 defaultCCBillingHeader。
//
// 兼容 system 的三种形态：
//   - nil：置为仅含 billing 块的数组；
//   - 字符串：转成 [billing 块, 原字符串文本块]；
//   - 结构化数组：在首位前插 billing 块。
//
// 该函数仅应在请求已通过 CC 检测后调用（受渠道「Claude Code 客户端检测」开关控制）。
func InsertClaudeCodeBillingHeader(request *dto.ClaudeRequest, billingHeader string) {
	if request == nil {
		return
	}
	if hasBillingBlock(request) {
		return
	}

	text := strings.TrimSpace(billingHeader)
	if text == "" {
		text = defaultCCBillingHeader
	}

	billingBlock := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
	billingBlock.SetText(text)

	switch {
	case request.System == nil:
		request.System = []dto.ClaudeMediaMessage{billingBlock}
	case request.IsStringSystem():
		existing := request.GetStringSystem()
		blocks := []dto.ClaudeMediaMessage{billingBlock}
		if existing != "" {
			original := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
			original.SetText(existing)
			blocks = append(blocks, original)
		}
		request.System = blocks
	default:
		request.System = append([]dto.ClaudeMediaMessage{billingBlock}, request.ParseSystem()...)
	}
}

// ---------- metadata.user_id 格式规整 ----------

// ccUserID 是真实 CC 的 metadata.user_id 内层 JSON 结构。字段顺序固定为
// device_id → account_uuid → session_id，与真实 CC 形态一致。
type ccUserID struct {
	DeviceID    string `json:"device_id"`
	AccountUUID string `json:"account_uuid"`
	SessionID   string `json:"session_id"`
}

// underscoreUserID 匹配下划线连接格式：user_<device>_account_<account>_session_<session>。
// device/account 用非贪婪匹配，靠字面标记 _account_ / _session_ 定界；account 可为空。
var underscoreUserID = regexp.MustCompile(`^user_(.*?)_account_(.*?)_session_(.+)$`)

// NormalizeClaudeCodeMetadata 把下划线连接格式的 metadata.user_id 规整为真实 CC 的
// 序列化 JSON 字符串格式，就地修改 request.Metadata。
//
//	转换前: "user_id": "user_<device>_account_<account>_session_<session>"
//	转换后: "user_id": "{\"device_id\":\"<device>\",\"account_uuid\":\"<account>\",\"session_id\":\"<session>\"}"
//
// 仅当 user_id 是字符串且命中下划线格式时才转换；已是 JSON 格式或不匹配的一律原样保留，
// 避免误伤。该函数必须在 DetectClaudeCode 之前调用。
func NormalizeClaudeCodeMetadata(request *dto.ClaudeRequest) {
	if request == nil || len(request.Metadata) == 0 {
		return
	}

	// 保留 metadata 中的其他字段，只改写 user_id。
	var meta map[string]any
	if err := common.Unmarshal(request.Metadata, &meta); err != nil || meta == nil {
		return
	}
	rawUID, ok := meta["user_id"]
	if !ok {
		return
	}
	uid, ok := rawUID.(string)
	if !ok {
		return
	}
	uid = strings.TrimSpace(uid)

	// 已是 JSON 形态则无需转换。
	if strings.HasPrefix(uid, "{") {
		return
	}
	m := underscoreUserID.FindStringSubmatch(uid)
	if m == nil {
		return
	}

	jsonUID, err := common.Marshal(ccUserID{
		DeviceID:    m[1],
		AccountUUID: m[2],
		SessionID:   m[3],
	})
	if err != nil {
		return
	}

	meta["user_id"] = string(jsonUID)
	newMeta, err := common.Marshal(meta)
	if err != nil {
		return
	}
	request.Metadata = newMeta
}

// ---------- 工具方法 ----------

// headerGet 大小写不敏感地取 header 值（http.Header.Get 已做 canonical，此处再兜底）。
func headerGet(header http.Header, key string) string {
	if header == nil {
		return ""
	}
	if v := header.Get(key); v != "" {
		return v
	}
	// 兜底：遍历匹配（应对非规范 key）。
	for k, vs := range header {
		if strings.EqualFold(k, key) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// systemBlocks 返回 system 的所有文本块（字符串形式则作为单块返回）。
func systemBlocks(request *dto.ClaudeRequest) []string {
	if request.System == nil {
		return nil
	}
	if request.IsStringSystem() {
		return []string{request.GetStringSystem()}
	}
	blocks := request.ParseSystem()
	texts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		texts = append(texts, b.GetText())
	}
	return texts
}

// systemText 把 system 拼成纯文本。
func systemText(request *dto.ClaudeRequest) string {
	return strings.Join(systemBlocks(request), "\n")
}

// toolNames 提取 tools 中的工具名（兼容 []any / map 结构）。
func toolNames(request *dto.ClaudeRequest) []string {
	if request.Tools == nil {
		return nil
	}
	list, ok := request.Tools.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := m["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// normalizeText 归一化：小写 + 合并空白。
func normalizeText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

// diceCoefficient 计算两串的 Dice 二元组相似度 ∈ [0,1]，用于容忍 CC 版本间 prompt 改版。
func diceCoefficient(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return 1.0
	}
	if len(a) < 2 || len(b) < 2 {
		return 0.0
	}
	bigrams := func(s string) map[string]int {
		r := []rune(s)
		d := make(map[string]int, len(r))
		for i := 0; i < len(r)-1; i++ {
			d[string(r[i:i+2])]++
		}
		return d
	}
	ba, bb := bigrams(a), bigrams(b)
	inter, total := 0, 0
	for g, c := range ba {
		if bc, ok := bb[g]; ok {
			if bc < c {
				inter += bc
			} else {
				inter += c
			}
		}
		total += c
	}
	for _, c := range bb {
		total += c
	}
	if total == 0 {
		return 0.0
	}
	return 2.0 * float64(inter) / float64(total)
}
