package middleware

import (
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
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
	session := sessions.Default(c)
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	useAccessToken := false
	if username == nil {
		// Check access token
		accessToken := c.Request.Header.Get("Authorization")
		if accessToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录且未提供 access token",
			})
			c.Abort()
			return
		}
		user := model.ValidateAccessToken(accessToken)
		if user != nil && user.Username != "" {
			if !validUserInfo(user.Username, user.Role) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，用户信息无效",
				})
				c.Abort()
				return
			}
			// Token is valid
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，access token 无效",
			})
			c.Abort()
			return
		}
	}
	// get header New-Api-User
	apiUserIdStr := c.Request.Header.Get("New-Api-User")
	if apiUserIdStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，未提供 New-Api-User",
		})
		c.Abort()
		return
	}
	apiUserId, err := strconv.Atoi(apiUserIdStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，New-Api-User 格式错误",
		})
		c.Abort()
		return

	}
	if id != apiUserId {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，New-Api-User 与登录用户不匹配",
		})
		c.Abort()
		return
	}
	if status.(int) == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		c.Abort()
		return
	}
	if role.(int) < minRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，权限不足",
		})
		c.Abort()
		return
	}
	if !validUserInfo(username.(string), role.(int)) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，用户信息无效",
		})
		c.Abort()
		return
	}
	// 防止不同newapi版本冲突，导致数据不通用
	c.Header("Auth-Version", "864b7076dbcd0a3c01b5520316720ebf")
	c.Set("username", username)
	c.Set("role", role)
	c.Set("id", id)
	c.Set("group", session.Get("group"))
	c.Set("user_group", session.Get("group"))
	c.Set("use_access_token", useAccessToken)

	c.Next()
}

func TryUserAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id := session.Get("id")
		if id != nil {
			c.Set("id", id)
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
	session := sessions.Default(c)
	username := session.Get("username")
	role := session.Get("role")
	id := session.Get("id")
	status := session.Get("status")
	useAccessToken := false
	if username == nil {
		accessToken := c.Request.Header.Get("Authorization")
		if accessToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无权进行此操作，未登录且未提供 access token",
			})
			c.Abort()
			return
		}
		user := model.ValidateAccessToken(accessToken)
		if user != nil && user.Username != "" {
			if !validUserInfo(user.Username, user.Role) {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "无权进行此操作，用户信息无效",
				})
				c.Abort()
				return
			}
			username = user.Username
			role = user.Role
			id = user.Id
			status = user.Status
			useAccessToken = true
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，access token 无效",
			})
			c.Abort()
			return
		}
	}
	apiUserIdStr := c.Request.Header.Get("New-Api-User")
	if apiUserIdStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，未提供 New-Api-User",
		})
		c.Abort()
		return
	}
	apiUserId, err := strconv.Atoi(apiUserIdStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，New-Api-User 格式错误",
		})
		c.Abort()
		return
	}
	if id != apiUserId {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "无权进行此操作，New-Api-User 与登录用户不匹配",
		})
		c.Abort()
		return
	}
	if status.(int) == common.UserStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已被封禁",
		})
		c.Abort()
		return
	}
	// 认证通过后，检查角色权限
	roleInt := role.(int)
	if !validUserInfo(username.(string), roleInt) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权进行此操作，用户信息无效",
		})
		c.Abort()
		return
	}

	// Leader 及以上用户在任何域名下都允许
	if roleInt >= common.RoleLeaderUser {
		// 正常设置上下文，继续执行
	} else {
		// 普通用户：仅在 api.mixrouter.com 域名下允许
		host := c.Request.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if host != "api.mixrouter.com" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，权限不足",
			})
			c.Abort()
			return
		}
		// 在 c.Next() 之前设置标记，控制器将强制只查看自己的数据
		c.Set("force_self_user_id", true)
	}

	c.Header("Auth-Version", "864b7076dbcd0a3c01b5520316720ebf")
	c.Set("username", username)
	c.Set("role", role)
	c.Set("id", id)
	c.Set("group", session.Get("group"))
	c.Set("user_group", session.Get("group"))
	c.Set("use_access_token", useAccessToken)

	c.Next()
}

func ToioAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleCommonUser)
		session := sessions.Default(c)
		flag := session.Get("is_toio")
		if flag != true {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无权进行此操作，未通过 toio 登录",
			})
			c.Abort()
			return
		}
	}
}
func WssAuth(c *gin.Context) {

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
				"message": "未提供 Authorization 请求头",
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
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "无效的令牌",
			})
			c.Abort()
			return
		}

		userCache, err := model.GetUserCache(token.UserId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			c.Abort()
			return
		}
		if userCache.Status != common.UserStatusEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "用户已被封禁",
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
		var parts []string
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
			abortWithOpenAiMessage(c, http.StatusUnauthorized, err.Error())
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
			abortWithOpenAiMessage(c, http.StatusInternalServerError, err.Error())
			return
		}
		userEnabled := userCache.Status == common.UserStatusEnabled
		if !userEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, "用户已被封禁")
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

		// Validate project if provided
		projectQuota := 0
		if projectName != "" {
			allocation, err := service.ValidateProjectRequest(projectName, clientUserId)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, err.Error())
				return
			}
			projectQuota = allocation.AllocatedQuota
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
		if requireUidCheck {
			if len(clientUserId) <= 8 && !isAiceKey {
				abortWithOpenAiMessage(c, http.StatusForbidden, "uid鉴权失败")
				return
			}
			if !isAiceKey {
				if ok, err1 := model.CheckCliendUserQuota(clientUserId, projectQuota); !ok || err1 != nil {
					abortWithOpenAiMessage(c, http.StatusForbidden, "请求失败，预算不足，请联系管理员")
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
	} else {
		c.Set("token_model_limit_enabled", false)
	}

	setMultiModelTags(c)
	//c.Set("allow_ips", token.GetIpLimitsMap())
	c.Set("token_group", token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)
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
