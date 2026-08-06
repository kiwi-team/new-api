package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const authIdentityContextKey = "auth_identity"

type dashboardCredentialKind int

const (
	dashboardCredentialUnmatched dashboardCredentialKind = iota
	dashboardCredentialInternal
	dashboardCredentialPAT
)

func validUserInfo(username string, role int) bool {
	// check username is empty
	if strings.TrimSpace(username) == "" {
		return false
	}
	if !common.IsValidateRole(role) {
		return false
	}
	return true
}

func authHelper(c *gin.Context, minRole int) {
	user, identity, useAccessToken, err := authenticateDashboardRequest(c)
	if err != nil {
		writeDashboardAuthError(c, err)
		return
	}
	if user.Status != common.UserStatusEnabled {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_USER_DISABLED", "message": common.TranslateMessage(c, i18n.MsgAuthUserBanned)})
		return
	}
	if user.Role < minRole {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "AUTH_INSUFFICIENT_PRIVILEGE", "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
		return
	}
	if !validUserInfo(user.Username, user.Role) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_USER_INVALID", "message": common.TranslateMessage(c, i18n.MsgAuthUserInfoInvalid)})
		return
	}
	setDashboardAuthContext(c, user, identity, useAccessToken)

	// 管理/root 写操作审计兜底：内聚在鉴权链路里，保证任何经过 AdminAuth/RootAuth
	// 的写接口都会自动留痕（无需在路由上单独挂审计中间件，避免漏挂）。
	// handler 内手动埋点者会设置 ContextKeyAuditLogged，finishAdminAudit 据此跳过。
	var auditWriter *auditResponseWriter
	if minRole >= common.RoleAdminUser {
		auditWriter = beginAdminAudit(c)
	}

	c.Next()

	finishAdminAudit(c, auditWriter)
}

func TryUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		user, identity, credentialKind, err := classifyDashboardCredential(c)
		if err != nil {
			writeDashboardAuthError(c, err)
			return
		}
		if credentialKind != dashboardCredentialUnmatched {
			setDashboardAuthContext(c, user, identity, credentialKind == dashboardCredentialPAT)
		}
		c.Next()
	}
}

func UserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
	}
}

func LeaderAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleLeaderUser)
	}
}

func AdminAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleAdminUser)
	}
}

func RootAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleRootUser)
	}
}

// MixRouterAuth 允许 api.mixrouter.com 域名下的普通用户访问，
// 同时保留 Leader 及以上用户在任何域名下的访问权限。
// 普通用户访问时会设置 force_self_user_id 标记，控制器据此限制只能查看自己的数据。
func MixRouterAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 先做用户认证（不调用 c.Next()，手动执行认证逻辑）
		mixRouterAuthHelper(c, common.RoleCommonUser)
	}
}

// mixRouterAuthHelper 复用 authHelper 的认证逻辑，但在 c.Next() 之前插入域名检查和 flag 设置
func mixRouterAuthHelper(c *gin.Context, minRole int) {
	user, identity, useAccessToken, err := authenticateDashboardRequest(c)
	if err != nil {
		writeDashboardAuthError(c, err)
		return
	}
	// 这里曾额外要求 New-Api-User 头与登录用户一致。那是 cookie session 时代
	// 防冒充的兜底；改成无状态 Bearer token 后身份完全由 token 决定，该头不再
	// 提供任何保证，前端也不再发送，继续强制只会让所有请求 401。
	if user.Status != common.UserStatusEnabled {
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		return
	}
	if !validUserInfo(user.Username, user.Role) || user.Role < minRole {
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，用户信息无效",
		})
		return
	}

	setDashboardAuthContext(c, user, identity, useAccessToken)

	// Leader 及以上用户在任何域名下都允许
	if user.Role < common.RoleLeaderUser {
		// 组织标签:role=common 但 (org_code, org_role) 在 menu 里有 quota_statistics 的也放行
		// 比如 mt-admin / wl-admin。详见 org.md 4.2。
		// 通过组织获得的访问也属于"自助视图",同样要设置 force_self_user_id,让 controller 走
		// resolveSelfScope 路径(mt-admin → 全 mt 组织 uid;wl-admin → 全 wl 组织 user_id)。
		allowedByOrg := false
		if orgUser, err := model.GetUserById(user.Id, false); err == nil && orgUser != nil {
			allowedByOrg = service.HasPage(orgUser, service.PageQuotaStatistics)
		}
		host := c.Request.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if host != "api.mixrouter.com" && !allowedByOrg {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，权限不足",
			})
			return
		}
		// 在 c.Next() 之前设置标记，控制器将强制只查看自己的数据(或本 org 的数据)
		c.Set("force_self_user_id", true)
	}

	c.Next()
}

// ToioAuth 已废弃,组织标签系统替代,详见 org.md。
// 残留删除时机:确认无前端/集成方调用 /api/toio/data 后下个版本一起清掉。
// GetAuthIdentity returns a dashboard session identity. PAT-authenticated
// requests intentionally have no SessionID and cannot manage browser sessions.
func GetAuthIdentity(c *gin.Context) (service.AuthIdentity, bool) {
	value, ok := c.Get(authIdentityContextKey)
	if !ok {
		return service.AuthIdentity{}, false
	}
	identity, ok := value.(service.AuthIdentity)
	return identity, ok
}

// GetSessionAuthIdentity returns only identities backed by a live dashboard
// session. PAT-authenticated requests intentionally fail this check.
func GetSessionAuthIdentity(c *gin.Context) (service.AuthIdentity, bool) {
	identity, ok := GetAuthIdentity(c)
	if !ok {
		identity = service.AuthIdentity{
			UserID:          c.GetInt("id"),
			SessionID:       c.GetString("session_id"),
			UserAuthVersion: c.GetInt64("auth_version"),
			SessionVersion:  c.GetInt64("session_version"),
		}
	}
	if identity.UserID <= 0 || identity.SessionID == "" || identity.UserAuthVersion <= 0 || identity.SessionVersion <= 0 {
		return service.AuthIdentity{}, false
	}
	return identity, true
}

func authenticateDashboardRequest(c *gin.Context) (*model.UserBase, service.AuthIdentity, bool, error) {
	user, identity, credentialKind, err := classifyDashboardCredential(c)
	if err != nil {
		return nil, service.AuthIdentity{}, credentialKind == dashboardCredentialPAT, err
	}
	if credentialKind == dashboardCredentialUnmatched {
		return nil, service.AuthIdentity{}, false, service.ErrAuthTokenInvalid
	}
	return user, identity, credentialKind == dashboardCredentialPAT, nil
}

func classifyDashboardCredential(c *gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error) {
	raw, ok := authorizationToken(c.GetHeader("Authorization"))
	if !ok {
		return nil, service.AuthIdentity{}, dashboardCredentialUnmatched, nil
	}
	identity, internal, err := service.ParseDashboardAccessToken(raw)
	if internal {
		if err != nil {
			return nil, service.AuthIdentity{}, dashboardCredentialInternal, err
		}
		_, user, err := service.ValidateLoginSession(identity)
		if err != nil {
			return nil, service.AuthIdentity{}, dashboardCredentialInternal, err
		}
		return user, identity, dashboardCredentialInternal, nil
	}
	patUser, err := model.ValidateAccessToken(raw)
	if err != nil {
		return nil, service.AuthIdentity{}, dashboardCredentialPAT, err
	}
	if patUser == nil || patUser.Id <= 0 {
		return nil, service.AuthIdentity{}, dashboardCredentialUnmatched, nil
	}
	user, err := model.GetUserCache(patUser.Id)
	if err != nil {
		return nil, service.AuthIdentity{}, dashboardCredentialPAT, err
	}
	return user, service.AuthIdentity{UserID: user.Id, UserAuthVersion: user.AuthVersion}, dashboardCredentialPAT, nil
}

func authorizationToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		header = parts[1]
	} else if len(parts) != 1 {
		return "", false
	}
	return header, header != ""
}

func setDashboardAuthContext(c *gin.Context, user *model.UserBase, identity service.AuthIdentity, useAccessToken bool) {
	c.Header("Auth-Version", "864b7076dbcd0a3c01b5520316720ebf")
	c.Set("username", user.Username)
	c.Set("role", user.Role)
	c.Set("id", user.Id)
	c.Set("group", user.Group)
	c.Set("user_group", user.Group)
	c.Set("use_access_token", useAccessToken)
	c.Set("session_id", identity.SessionID)
	c.Set("auth_version", identity.UserAuthVersion)
	c.Set("session_version", identity.SessionVersion)
	c.Set(authIdentityContextKey, identity)
	user.WriteContext(c)
}

func writeDashboardAuthError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrAuthTokenExpired) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_TOKEN_EXPIRED", "message": common.TranslateMessage(c, i18n.MsgAuthNotLoggedIn)})
		return
	}
	if errors.Is(err, service.ErrLoginSessionRevoked) || errors.Is(err, gorm.ErrRecordNotFound) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_SESSION_REVOKED", "message": common.TranslateMessage(c, i18n.MsgAuthNotLoggedIn)})
		return
	}
	if errors.Is(err, service.ErrAuthTokenInvalid) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "AUTH_UNAUTHORIZED", "message": common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid)})
		return
	}
	common.SysLog("dashboard authentication error: " + err.Error())
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUTH_INTERNAL_ERROR", "message": common.TranslateMessage(c, i18n.MsgDatabaseError)})
}

func RequirePermission(permission authz.Permission) func(c *gin.Context) {
	return func(c *gin.Context) {
		role := c.GetInt("role")
		userID := c.GetInt("id")
		if authz.Can(userID, role, permission) {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
		})
		c.Abort()
	}
}

func WssAuth(c *gin.Context) {

}

// TokenOrUserAuth allows either session-based user auth or API token auth.
// Used for endpoints that need to be accessible from both the dashboard and API clients.
func TokenOrUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		raw, ok := authorizationToken(c.GetHeader("Authorization"))
		if ok {
			identity, internal, err := service.ParseDashboardAccessToken(raw)
			if !internal {
				TokenAuth()(c)
				return
			}
			if err != nil {
				writeDashboardAuthError(c, err)
				return
			}
			_, user, err := service.ValidateLoginSession(identity)
			if err != nil {
				writeDashboardAuthError(c, err)
				return
			}
			setDashboardAuthContext(c, user, identity, false)
			c.Next()
			return
		}
		// Opaque credentials are relay API keys here, never dashboard PATs.
		TokenAuth()(c)
	}
}

// TokenAuthReadOnly 宽松版本的令牌认证中间件，用于只读查询接口。
// 只验证令牌 key 是否存在，不检查令牌状态、过期时间和额度。
// 即使令牌已过期、已耗尽或已禁用，也允许访问。
// 仍然检查用户是否被封禁。
func TokenAuthReadOnly() func(c *gin.Context) {
	return func(c *gin.Context) {
		key := c.Request.Header.Get("Authorization")
		if key == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgTokenNotProvided),
			})
			c.Abort()
			return
		}
		if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		key = strings.TrimPrefix(key, "sk-")
		parts := strings.Split(key, "-")
		key = parts[0]

		token, err := model.GetTokenByKey(key, false)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgTokenInvalid),
				})
			} else {
				common.SysLog("TokenAuthReadOnly GetTokenByKey database error: " + err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
				})
			}
			c.Abort()
			return
		}

		// TokenAuthReadOnly must keep allowing other token states to query read-only
		// data, such as token usage logs; only explicitly disabled tokens are denied.
		if token.Status == common.TokenStatusDisabled {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgTokenStatusUnavailable),
			})
			c.Abort()
			return
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			common.SysLog(fmt.Sprintf("TokenAuthReadOnly GetUserCache error for user %d: %v", token.UserId, err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
			})
			c.Abort()
			return
		}
		if userCache.Status != common.UserStatusEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthUserBanned),
			})
			c.Abort()
			return
		}

		c.Set("id", token.UserId)
		c.Set("token_id", token.Id)
		c.Set("token_key", token.Key)
		c.Next()
	}
}

func TokenAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 快照请求头到 context，落库时给 logs/error_logs.header 列用。必须在任何 header
		// 修改之前（ws / x-api-key / x-goog-api-key 等会改写 Authorization）。
		//
		// 落库形态：扁平 {name: value} JSON 对象（{"authorization":"Bearer ...","Content-Type":"application/json"}）。
		// 优先从 ExtractRawHeaders 拿 wire 字节解析出的有序切片，保留原始大小写；
		// 抓不到的（HTTP/2、未挂 CaptureListener 的 dev 路径）回落 canonical http.Header（大小写已被 net/http
		// 规范化，无法挽回）。两条路径都拍平到同一 map shape，前端展示无差异。
		//
		// 取舍：
		//   - 同名重复 header（HTTP 协议允许）会被合并成 "v1, v2"，符合 RFC 7230 §3.2.2 的合并语义；
		//   - JSON 对象的 key 顺序在 PG jsonb 存储后不保证保留（jsonb 会重排），所以"按到达顺序"
		//     无法从落库结果还原；若以后真要看顺序，需要回到 array-of-pairs shape 或改 TEXT 列。
		//
		// 是否对 Authorization/Cookie/X-Api-Key 等敏感头脱敏由 options 表 LogHeaderRedactEnabled 控制，
		// 默认 true（!= "false" 才视为关闭，老库未写入此 key 也按 true 处理）。
		// 序列化失败则忽略，日志记录链路不能因为序列化而阻塞请求。
		headerMap := make(map[string]string)
		if rawPairs, _ := ExtractRawHeaders(c.Request); rawPairs != nil {
			for _, p := range rawPairs {
				if existing, ok := headerMap[p.Name]; ok {
					headerMap[p.Name] = existing + ", " + p.Value
				} else {
					headerMap[p.Name] = p.Value
				}
			}
		} else {
			for k, vs := range c.Request.Header {
				headerMap[k] = strings.Join(vs, ", ")
			}
		}
		if common.OptionMap["LogHeaderRedactEnabled"] != "false" {
			for k, v := range headerMap {
				if sensitiveHeaderKeys[strings.ToLower(k)] {
					headerMap[k] = redactHeaderValue(v)
				}
			}
		}
		if headerBytes, err := common.Marshal(headerMap); err == nil {
			common.SetContextKey(c, constant.ContextKeyHeader, string(headerBytes))
		}
		// 先检测是否为ws
		if c.Request.Header.Get("Sec-WebSocket-Protocol") != "" {
			// Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.sk-xxx, openai-beta.realtime-v1
			// read sk from Sec-WebSocket-Protocol
			key := c.Request.Header.Get("Sec-WebSocket-Protocol")
			parts := strings.Split(key, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "openai-insecure-api-key") {
					key = strings.TrimPrefix(part, "openai-insecure-api-key.")
					break
				}
			}
			c.Request.Header.Set("Authorization", "Bearer "+key)
		}
		// 检查path包含/v1/messages 或 /v1/models
		if strings.Contains(c.Request.URL.Path, "/v1/messages") || strings.Contains(c.Request.URL.Path, "/v1/models") {
			anthropicKey := c.Request.Header.Get("x-api-key")
			if anthropicKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+anthropicKey)
			}
		}
		// gemini api 从query中获取key
		if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1beta/openai/models") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
			skKey := c.Query("key")
			if skKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+skKey)
			}
			// 从x-goog-api-key header中获取key
			xGoogKey := c.Request.Header.Get("x-goog-api-key")
			if xGoogKey != "" {
				c.Request.Header.Set("Authorization", "Bearer "+xGoogKey)
			}
		}
		key := c.Request.Header.Get("Authorization")
		if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		clientUserId := c.Request.Header.Get("uid")
		clientScenairo := c.Request.Header.Get("scenairo")
		extra := c.Request.Header.Get("extra")
		// 从key中提取client_user_id
		tmpArr := strings.Split(key, "_")
		if len(tmpArr) >= 2 {
			key = tmpArr[0]
			clientUserId = strings.Join(tmpArr[1:], "_")
		}
		aiceKey := common.OptionMap["AICE_KEY"]
		aiceKeyArr := strings.Split(aiceKey, ",")
		isAiceKey := false
		for _, item := range aiceKeyArr {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if strings.Contains(key, item) {
				isAiceKey = true
				break
			}
		}
		parts := make([]string, 0)
		if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		if key == "" || key == "midjourney-proxy" {
			key = c.Request.Header.Get("mj-api-secret")
			if strings.HasPrefix(key, "Bearer ") || strings.HasPrefix(key, "bearer ") {
				key = strings.TrimSpace(key[7:])
			}
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]
		} else {
			key = strings.TrimPrefix(key, "sk-")
			parts = strings.Split(key, "-")
			key = parts[0]

		}
		if isAiceKey {
			parts = []string{key}
		}
		token, err := model.ValidateUserToken(key)
		if token != nil {
			id := c.GetInt("id")
			if id == 0 {
				c.Set("id", token.UserId)
			}
		}
		if err != nil {
			if errors.Is(err, model.ErrDatabase) {
				common.SysLog("TokenAuth ValidateUserToken database error: " + err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError,
					common.TranslateMessage(c, i18n.MsgDatabaseError))
			} else {
				abortWithOpenAiMessage(c, http.StatusUnauthorized,
					common.TranslateMessage(c, i18n.MsgTokenInvalid))
			}
			return
		}

		allowIps := token.GetIpLimits()
		if len(allowIps) > 0 {
			clientIp := c.ClientIP()
			logger.LogDebug(c, "Token has IP restrictions, checking client IP %s", clientIp)
			ip := net.ParseIP(clientIp)
			if ip == nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, "无法解析客户端 IP 地址")
				return
			}
			if common.IsIpInCIDRList(ip, allowIps) == false {
				abortWithOpenAiMessage(c, http.StatusForbidden, "您的 IP 不在令牌允许访问的列表中", types.ErrorCodeAccessDenied)
				return
			}
			logger.LogDebug(c, "Client IP %s passed the token IP restrictions check", clientIp)
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			common.SysLog(fmt.Sprintf("TokenAuth GetUserCache error for user %d: %v", token.UserId, err))
			abortWithOpenAiMessage(c, http.StatusInternalServerError,
				common.TranslateMessage(c, i18n.MsgDatabaseError))
			return
		}
		userEnabled := userCache.Status == common.UserStatusEnabled
		if !userEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, common.TranslateMessage(c, i18n.MsgAuthUserBanned))
			return
		}

		userCache.WriteContext(c)

		userGroup := userCache.Group
		tokenGroup := token.Group
		if tokenGroup != "" {
			// check common.UserUsableGroups[userGroup]
			if _, ok := service.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("无权访问 %s 分组", tokenGroup))
				return
			}
			// check group in common.GroupRatio
			if !ratio_setting.ContainsGroupRatio(tokenGroup) {
				if tokenGroup != "auto" {
					abortWithOpenAiMessage(c, http.StatusForbidden, fmt.Sprintf("分组 %s 已被弃用", tokenGroup))
					return
				}
			}
			userGroup = tokenGroup
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, userGroup)

		// Project header validation
		// Extract X-Project or project header
		projectName := c.GetHeader("X-Project")
		if projectName == "" {
			projectName = c.GetHeader("project")
		}

		// 带项目头的请求由项目额度独家把关：ValidateProjectRequest 已覆盖
		// 项目存在、未暂停、有生效方案、方案内额度未用尽。
		if projectName != "" {
			allocation, err := service.ValidateProjectRequest(projectName, clientUserId)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, err.Error())
				return
			}
			common.SetContextKey(c, constant.ContextKeyProjectName, projectName)
			common.SetContextKey(c, constant.ContextKeyProjectId, allocation.ProjectId)
			common.SetContextKey(c, constant.ContextKeyProjectPlanId, allocation.PlanId)
			common.SetContextKey(c, constant.ContextKeyProjectAllocationId, allocation.Id)
		}

		// 是否对当前令牌所属用户强制进行 uid 鉴权：
		// 优先读取用户级设置（用户管理里勾选），兼容旧的全局开关 CKECK_CLIENT_USER_ID。
		requireUidCheck := userCache.GetSetting().CheckUid
		if !requireUidCheck && common.OptionMap["CKECK_CLIENT_USER_ID"] == "true" {
			requireUidCheck = true
		}
		if requireUidCheck && !isAiceKey {
			if len(clientUserId) <= 8 {
				abortWithOpenAiMessage(c, http.StatusForbidden, "uid鉴权失败")
				return
			}
			// 只有非项目请求才查非项目预算池。项目请求已由上面的项目额度闸门裁决，
			// 两个预算池互不透支。
			if projectName == "" {
				ok, err := model.CheckClientUserNonProjectBudget(clientUserId)
				if err != nil {
					common.SysError(fmt.Sprintf("check non-project budget failed for uid %s: %s", clientUserId, err.Error()))
				}
				if !ok || err != nil {
					common.SysLog(fmt.Sprintf("uid %s 非项目预算不足，请求被拒绝", clientUserId))
					abortWithOpenAiMessage(c, http.StatusForbidden, "请求失败，本月非项目预算已用尽，请联系管理员")
					return
				}
			}
		}
		common.SetContextKey(c, constant.ContextKeyClientUserId, clientUserId)
		common.SetContextKey(c, constant.ContextKeyClientScenairo, clientScenairo)
		common.SetContextKey(c, constant.ContextKeyExtra, extra)

		// Handle scenario header, default to personal_experiment
		scenario := c.GetHeader("scenario")
		if scenario == "" {
			scenario = "personal_experiment"
		}
		common.SetContextKey(c, constant.ContextKeyClientScenairo, scenario)

		err = SetupContextForToken(c, token, parts...)
		if err != nil {
			return
		}
		c.Next()
	}
}

func setMultiModelTags(c *gin.Context) []string {
	textRequest := &dto.GeneralOpenAIRequest{}
	common.UnmarshalBodyReusable(c, textRequest)
	tags := make([]string, 0)
	if len(textRequest.Tools) > 0 {
		tags = append(tags, "tools")
	}
	for _, message := range textRequest.Messages {
		arr := message.ParseContent()
		for _, content := range arr {
			switch content.Type {
			case dto.ContentTypeAudioUrl:
				tags = append(tags, "audio")
			case dto.ContentTypeImageURL:
				tags = append(tags, "image")
			case dto.ContentTypeVideoUrl:
				tags = append(tags, "video")
			}
		}
	}
	// tags 去掉重复元素
	tags = slices.Compact(tags)
	c.Set("multi_model_tags", tags)
	return tags
}
func SetupContextForToken(c *gin.Context, token *model.Token, parts ...string) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}
	c.Set("id", token.UserId)
	c.Set("token_id", token.Id)
	c.Set("token_key", token.Key)
	c.Set("token_name", token.Name)
	c.Set("token_unlimited_quota", token.UnlimitedQuota)
	if !token.UnlimitedQuota {
		c.Set("token_quota", token.RemainQuota)
	}
	if token.ModelLimitsEnabled {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", token.GetModelLimitsMap())
	} else if us, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting); ok &&
		us.ModelLimitsEnabled && len(us.ModelLimits) > 0 {
		// 令牌未启用模型限制时，回退到用户级模型限制
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", us.GetModelLimitsMap())
	} else {
		c.Set("token_model_limit_enabled", false)
	}

	setMultiModelTags(c)
	//c.Set("allow_ips", token.GetIpLimitsMap())
	c.Set("token_group", token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)
	if token.AutoGroups != "" {
		autoGroups, err := token.GetAutoGroups()
		if err != nil {
			common.SysError(fmt.Sprintf("failed to parse auto groups for token %d: %v", token.Id, err))
			autoGroups = []string{}
			common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, autoGroups)
		} else if len(autoGroups) > 0 {
			common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, autoGroups)
		}
	}
	if len(parts) > 1 {
		if model.IsAdmin(token.UserId) {
			c.Set("specific_channel_id", parts[1])
		} else {
			c.Header("specific_channel_version", "701e3ae1dc3f7975556d354e0675168d004891c8")
			abortWithOpenAiMessage(c, http.StatusForbidden, "普通用户不支持指定渠道")
			return fmt.Errorf("普通用户不支持指定渠道")
		}
	}
	c.Set("token_channel_rules", token.GetChannelRules())
	c.Set("token_channel_ratios", token.GetChannelRatios())
	return nil
}
