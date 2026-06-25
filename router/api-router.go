package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.GET("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.GET("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", controller.CreemWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.POST("/epay/notify", controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				// 组织标签系统:登录后前端调一次拿菜单 + 顶栏模式,详见 org.md
				selfRoute.GET("/menu", controller.GetUserMenu)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				// 批量删除用户:root 专属(级联删除其 token 与用户记录)
				adminRoute.POST("/batch/delete", middleware.RootAuth(), controller.DeleteUserBatch)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}
		toioRoot := router.Group("/toio")
		toioRoot.Use(gzip.Gzip(gzip.DefaultCompression))
		toioRoot.Use(middleware.GlobalAPIRateLimit())
		{
			toioRoot.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			toioRoot.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
		}

		// Subscription billing (plans, purchase, admin management)
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", controller.SubscriptionEpayReturn)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
			// Import/Export options
			optionRoute.GET("/export", controller.ExportOptionsCSV)
			optionRoute.POST("/import", controller.ImportOptionsCSV)
		}

		// Custom OAuth provider management (admin only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		// org.md 全系统级约束:channels 升级为 root only,非 root 不可见所有 channel 接口
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.RootAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/channel-list-by-model", controller.GetChannelsByModelName)
			channelRoute.GET("/channel-list-by-model-newapi", controller.GetChannelsByModelNameNewAPI)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/", controller.AddChannel)
			channelRoute.PUT("/", controller.UpdateChannel)
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			channelRoute.POST("/tag/disabled", controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", controller.EnableTagChannels)
			channelRoute.PUT("/tag", controller.EditTagChannels)
			channelRoute.DELETE("/:id", controller.DeleteChannel)
			channelRoute.POST("/batch", controller.DeleteChannelBatch)
			channelRoute.POST("/fix", controller.FixChannelsAbilities)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", controller.FetchModels)
			channelRoute.POST("/codex/oauth/start", controller.StartCodexOAuth)
			channelRoute.POST("/codex/oauth/complete", controller.CompleteCodexOAuth)
			channelRoute.POST("/:id/codex/oauth/start", controller.StartCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/oauth/complete", controller.CompleteCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/refresh", controller.RefreshCodexChannelCredential)
			channelRoute.GET("/:id/codex/usage", controller.GetCodexChannelUsage)
			channelRoute.POST("/ollama/pull", controller.OllamaPullModel)
			channelRoute.POST("/ollama/pull/stream", controller.OllamaPullModelStream)
			channelRoute.DELETE("/ollama/delete", controller.OllamaDeleteModel)
			channelRoute.GET("/ollama/version/:id", controller.OllamaVersion)
			channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.POST("/copy/:id", controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)
		}
		// org.md 全系统级约束:channels 全部 root only,channel-name-list 也不例外
		// (非 root 用户的 token 编辑表单本就不显示 specific_channel_id 相关字段)
		userCannelRoute := apiRouter.Group("/channel")
		userCannelRoute.Use(middleware.RootAuth())
		{
			userCannelRoute.GET("/channel-name-list", controller.GetNameIdList)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/group", controller.BatchSetTokenGroup)
			tokenRoute.POST("/batch/models", controller.BatchAppendTokenModels)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/error-logs", middleware.AdminAuth(), controller.GetAllErrorLogs)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.GET("/error-logs", middleware.AdminAuth(), controller.GetAllErrorLogs)
		logRoute.GET("/error-logs/:id/body", middleware.AdminAuth(), controller.GetErrorLogBody)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/error-logs", middleware.UserAuth(), controller.GetSelfErrorLogs)
		logRoute.GET("/self/error-logs/:id/body", middleware.UserAuth(), controller.GetSelfErrorLogBody)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)
		dataRoute.GET("/statistics", middleware.MixRouterAuth(), controller.GetQuotaDataStatistics)
		dataRoute.GET("/statistics/export", middleware.MixRouterAuth(), controller.ExportQuotaDataStatistics)
		dataRoute.GET("/channel-statistics", middleware.AdminAuth(), controller.GetChannelQuotaStatistics)
		dataRoute.GET("/model-usage-analysis", middleware.UserAuth(), controller.GetModelUsageAnalysis)
		dataRoute.GET("/channel-monitor", middleware.RootAuth(), controller.GetChannelMonitor)
		dataRoute.GET("/project-names", middleware.MixRouterAuth(), controller.GetDistinctProjectNames)
		dataRoute.GET("/token-list", middleware.MixRouterAuth(), controller.GetTokenListForStatistics)
		// /api/toio/data/* 路由已删除(组织标签系统替代,详见 org.md)。
		// 同源能力通过 /api/data/statistics 等接口提供,数据范围由 ComputeOrgScope 计算。

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		logRootRoute := apiRouter.Group("/log")
		logRootRoute.Use(middleware.RootAuth())
		{
			logRootRoute.GET("/export", controller.ExportLogsCSV)
			logRootRoute.GET("/:id/request", controller.GetLogRequest)
			logRootRoute.GET("/:id/response", controller.GetLogResponse)
			// header 仅 root 可查；同组复用 RootAuth 中间件
			logRootRoute.GET("/:id/header", controller.GetLogHeader)
			logRootRoute.GET("/error-logs/:id/header", controller.GetErrorLogHeader)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		// 组织清单只读接口:root 在用户编辑下拉里用。详见 org.md (constant/org.go 是唯一权威)
		orgRoute := apiRouter.Group("/orgs")
		orgRoute.Use(middleware.RootAuth())
		{
			orgRoute.GET("/", controller.GetOrgs)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		fileRoute := apiRouter.Group("/file")
		fileRoute.Use(middleware.AdminAuth())
		{
			fileRoute.POST("/upload", controller.UploadFile)
		}
		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}

		// UID 预算管理:从 AdminAuth 降到 UserAuth + PageAuth(client_user_quota)。
		// PageAuth 让系统 admin/root bypass 通过(看全局),mt-admin 走 GetUserMenu 命中此页;
		// controller 内部按 ComputeOrgScope 加 WHERE 过滤数据范围。详见 org.md。
		cuQuotaRoute := apiRouter.Group("/cliend_user_quota")
		cuQuotaRoute.Use(middleware.UserAuth(), middleware.PageAuth(service.PageClientUserQuota))
		{
			cuQuotaRoute.GET("/", controller.GetAllCliendUserQuota)
			cuQuotaRoute.GET("/search", controller.SearchCliendUserQuota)
			cuQuotaRoute.GET("/export", controller.ExportCliendUserQuotaCSV)
			cuQuotaRoute.GET("/logs", controller.GetCliendUserQuotaLogs)
			cuQuotaRoute.GET("/project-allocations", controller.GetCliendUserProjectAllocations)
			cuQuotaRoute.GET("/batch-project-budget", controller.GetBatchProjectBudgetSummary)
			cuQuotaRoute.GET("/:id", controller.GetCliendUserQuota)
			cuQuotaRoute.POST("/", controller.CreateCliendUserQuota)
			cuQuotaRoute.PUT("/", controller.UpdateCliendUserQuota)
			cuQuotaRoute.DELETE("/:id", controller.DeleteCliendUserQuota)
		}

		// 项目预算管理:从 AdminAuth 降到 UserAuth + PageAuth(project)。
		// PageAuth 让系统 admin/root bypass,mt-admin 命中此页;controller 按 ComputeOrgScope 过滤。
		projectRoute := apiRouter.Group("/project")
		projectRoute.Use(middleware.UserAuth(), middleware.PageAuth(service.PageProject))
		{
			projectRoute.GET("/dashboard", controller.GetProjectDashboard)
			projectRoute.GET("/:id/allocations", controller.GetProjectAllocations)
			projectRoute.POST("/:id/allocation", controller.CreateOrUpdateAllocation)
			projectRoute.GET("/:id/statistics", controller.GetProjectStatistics)
			projectRoute.PUT("/:id/status", controller.UpdateProjectStatus)
			projectRoute.PUT("/:id/active-plan", controller.SetActivePlan)
			projectRoute.PUT("/:id", controller.UpdateProject)
			// Allocation plan routes
			projectRoute.GET("/:id/plans", controller.GetProjectPlans)
			projectRoute.POST("/:id/plan", controller.CreateProjectPlan)
			projectRoute.PUT("/plan/:planId", controller.UpdateProjectPlan)
			projectRoute.DELETE("/plan/:planId", controller.DeleteProjectPlan)
			projectRoute.GET("/plan/:planId/allocations", controller.GetPlanAllocations)
			projectRoute.POST("/plan/:planId/allocation", controller.CreateOrUpdatePlanAllocation)
			projectRoute.POST("/allocation/:allocationId/clear", controller.ClearAllocationBudget)
		}
		projectsRoute := apiRouter.Group("/projects")
		projectsRoute.Use(middleware.UserAuth(), middleware.PageAuth(service.PageProject))
		{
			projectsRoute.GET("/", controller.GetProjects)
		}
		// Single project creation route
		apiRouter.POST("/project", middleware.UserAuth(), middleware.PageAuth(service.PageProject), controller.CreateProject)

		// Root-only import/export
		adminRootRoute := apiRouter.Group("/admin")
		adminRootRoute.Use(middleware.RootAuth())
		{
			adminRootRoute.GET("/export/users", controller.ExportUsersCSV)
			adminRootRoute.GET("/export/tokens", controller.ExportTokensCSV)
			adminRootRoute.GET("/export/channels", controller.ExportChannelsCSV)
			adminRootRoute.POST("/import/users", controller.ImportUsersCSV)
			adminRootRoute.POST("/import/tokens", controller.ImportTokensCSV)
			adminRootRoute.POST("/import/channels", controller.ImportChannelsCSV)
		}

		// Model Route Config - Root only
		modelRouteConfigRoute := apiRouter.Group("/model_route_config")
		modelRouteConfigRoute.Use(middleware.RootAuth())
		{
			modelRouteConfigRoute.GET("/", controller.GetAllModelRouteConfigs)
			modelRouteConfigRoute.GET("/search", controller.SearchModelRouteConfigs)
			modelRouteConfigRoute.GET("/:id", controller.GetModelRouteConfig)
			modelRouteConfigRoute.POST("/", controller.AddModelRouteConfig)
			modelRouteConfigRoute.PUT("/", controller.UpdateModelRouteConfig)
			modelRouteConfigRoute.DELETE("/:id", controller.DeleteModelRouteConfig)
			modelRouteConfigRoute.POST("/batch/delete", controller.BatchDeleteModelRouteConfigs)
			modelRouteConfigRoute.POST("/status", controller.UpdateModelRouteConfigStatus)
		}

		// Debug Module - Root only
		debugRoute := apiRouter.Group("/debug")
		debugRoute.Use(middleware.RootAuth())
		{
			// 渠道和 Key 信息
			debugRoute.GET("/channels", controller.GetDebugChannels)
			debugRoute.GET("/keys", controller.GetDebugKeys)
			debugRoute.GET("/tags", controller.GetDebugTags)

			// 调试配置
			debugRoute.GET("/configurations", controller.GetDebugConfigurations)
			debugRoute.GET("/configurations/:id", controller.GetDebugConfiguration)
			debugRoute.POST("/configurations", controller.CreateDebugConfiguration)
			debugRoute.PUT("/configurations/:id", controller.UpdateDebugConfiguration)
			debugRoute.DELETE("/configurations/:id", controller.DeleteDebugConfiguration)
			debugRoute.POST("/configurations/:id/execute", controller.ExecuteDebugConfiguration)

			// 直接执行调试请求
			debugRoute.POST("/execute", controller.ExecuteDebugRequest)
			debugRoute.POST("/execute/stream", controller.ExecuteDebugRequestStream)

			// 测试数据（模板的简化版，用于保存调试数据）
			debugRoute.GET("/test-data", controller.GetDebugTestData)
			debugRoute.POST("/test-data", controller.CreateDebugTestData)
			debugRoute.PUT("/test-data/:id", controller.UpdateDebugTestData)
			debugRoute.DELETE("/test-data/:id", controller.DeleteDebugTestData)

			// 调试模板
			debugRoute.GET("/templates", controller.GetDebugTemplates)
			debugRoute.GET("/templates/:id", controller.GetDebugTemplate)
			debugRoute.POST("/templates", controller.CreateDebugTemplate)
			debugRoute.PUT("/templates/:id", controller.UpdateDebugTemplate)
			debugRoute.DELETE("/templates/:id", controller.DeleteDebugTemplate)

			// 调试日志
			debugRoute.GET("/logs", controller.GetDebugLogs)
			debugRoute.GET("/logs/:id", controller.GetDebugLog)
			debugRoute.DELETE("/logs/:id", controller.DeleteDebugLog)
			debugRoute.POST("/logs/batch/tag", controller.BatchTagDebugLogs)
		}

		// 环境同步模块 - Root only
		syncRoute := apiRouter.Group("/sync")
		syncRoute.Use(middleware.RootAuth())
		{
			// 环境管理
			syncRoute.GET("/environments", controller.GetSyncEnvironments)
			syncRoute.GET("/environments/enabled", controller.GetEnabledSyncEnvironments)
			syncRoute.POST("/environments", controller.AddSyncEnvironment)
			syncRoute.PUT("/environments/:id", controller.UpdateSyncEnvironment)
			syncRoute.DELETE("/environments/:id", controller.DeleteSyncEnvironment)
			syncRoute.POST("/environments/:id/test", controller.TestSyncEnvironment)

			// 渠道同步
			syncRoute.POST("/channels/preview", controller.PreviewSyncChannels)
			syncRoute.POST("/channels", controller.SyncChannels)

			// 模型价格同步
			syncRoute.POST("/model-prices", controller.SyncModelPrices)

			// 同步历史
			syncRoute.GET("/logs", controller.GetSyncLogs)
		}

		// Settlement pricing routes
		settlementRoute := apiRouter.Group("/settlement")
		{
			// 结算配置只读 GET:wl-admin 可见(详见 org.md 4.2.2),系统 admin 自动 bypass
			settlementRoute.GET("/config",
				middleware.UserAuth(),
				middleware.PageAuth(service.PageSettlementConfigReadonly),
				controller.GetSettlementConfigs)

			// 结算配置写操作仍然 root only
			adminSettlementConfig := settlementRoute.Group("/config")
			adminSettlementConfig.Use(middleware.RootAuth())
			{
				adminSettlementConfig.POST("", controller.CreateSettlementConfigHandler)
				adminSettlementConfig.PUT("", controller.UpdateSettlementConfigHandler)
				adminSettlementConfig.DELETE("/:id", controller.DeleteSettlementConfigHandler)
				adminSettlementConfig.POST("/batch", controller.BatchImportSettlementConfigs)
			}

			// User query endpoints (UserAuth)
			settlementRoute.GET("/config/self", middleware.UserAuth(), controller.GetSelfSettlementConfigs)
			settlementRoute.GET("/bill/self", middleware.UserAuth(), controller.GetSelfSettlementBill)
			settlementRoute.GET("/bill/self/export", middleware.UserAuth(), controller.SelfExportSettlementBillCSV)
			// 账单页 key 筛选下拉框数据源:root 查全部用户的 key,普通用户只查自己的
			settlementRoute.GET("/bill/tokens", middleware.UserAuth(), controller.GetSettlementBillTokenOptions)

			// 账单 admin 端:wl-admin 也能调,controller 内部按 ComputeOrgScope 过滤 user_id
			adminBill := settlementRoute.Group("/bill/admin")
			adminBill.Use(middleware.UserAuth(), middleware.PageAuth(service.PageBill))
			{
				adminBill.GET("", controller.AdminGetSettlementBill)
				adminBill.GET("/export", controller.AdminExportSettlementBillCSV)
			}
		}
	}
}
