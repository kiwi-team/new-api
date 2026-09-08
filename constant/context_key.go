package constant

type ContextKey string

const (
	ContextKeyCompletionsResponses            = "completions_responses"
	ContextKeyTokenCountMeta       ContextKey = "token_count_meta"
	ContextKeyPromptTokens         ContextKey = "prompt_tokens"
	ContextKeyEstimatedTokens      ContextKey = "estimated_tokens"

	ContextKeyOriginalModel    ContextKey = "original_model"
	ContextKeyRequestStartTime ContextKey = "request_start_time"
	// ContextKeyUseChannelTime 累积每个被尝试渠道的耗时（毫秒），与 use_channel 一一对应（含成功的最后一个渠道）
	ContextKeyUseChannelTime ContextKey = "use_channel_time"

	/* token related keys */
	ContextKeyTokenUnlimited                ContextKey = "token_unlimited_quota"
	ContextKeyTokenKey                      ContextKey = "token_key"
	ContextKeyTokenId                       ContextKey = "token_id"
	ContextKeyTokenGroup                    ContextKey = "token_group"
	ContextKeyTokenSpecificChannelId        ContextKey = "specific_channel_id"
	ContextKeyTokenModelLimitEnabled        ContextKey = "token_model_limit_enabled"
	ContextKeyTokenModelLimit               ContextKey = "token_model_limit"
	ContextKeyTokenAllowIps                 ContextKey = "token_allow_ips"
	ContextKeyTokenCrossGroupRetry          ContextKey = "token_cross_group_retry"
	ContextKeyTokenAutoGroups               ContextKey = "token_auto_groups"
	ContextKeyTokenChannelRulesHighPriority ContextKey = "token_channel_rules_high_priority"

	/* channel related keys */
	ContextKeyChannelRatio                      ContextKey = "channel_ratio"
	ContextKeyChannelId                         ContextKey = "channel_id"
	ContextKeyChannelName                       ContextKey = "channel_name"
	ContextKeyChannelCreateTime                 ContextKey = "channel_create_time"
	ContextKeyChannelBaseUrl                    ContextKey = "base_url"
	ContextKeyChannelType                       ContextKey = "channel_type"
	ContextKeyChannelSetting                    ContextKey = "channel_setting"
	ContextKeyChannelOtherSetting               ContextKey = "channel_other_setting"
	ContextKeyChannelParamOverride              ContextKey = "param_override"
	ContextKeyChannelHeaderOverride             ContextKey = "header_override"
	ContextKeyChannelOrganization               ContextKey = "channel_organization"
	ContextKeyChannelAutoBan                    ContextKey = "auto_ban"
	ContextKeyChannelEnableAutoDisabledChannels ContextKey = "auto_enable_auto_disabled_channels"
	ContextKeyChannelModelMapping               ContextKey = "model_mapping"
	ContextKeyChannelStatusCodeMapping          ContextKey = "status_code_mapping"
	ContextKeyChannelIsMultiKey                 ContextKey = "channel_is_multi_key"
	ContextKeyChannelMultiKeyIndex              ContextKey = "channel_multi_key_index"
	ContextKeyChannelKey                        ContextKey = "channel_key"

	ContextKeyAutoGroup           ContextKey = "auto_group"
	ContextKeyAutoGroupIndex      ContextKey = "auto_group_index"
	ContextKeyAutoGroupRetryIndex ContextKey = "auto_group_retry_index"

	/* user related keys */
	ContextKeyUserId          ContextKey = "id"
	ContextKeyUserSetting     ContextKey = "user_setting"
	ContextKeyUserQuota       ContextKey = "user_quota"
	ContextKeyUserStatus      ContextKey = "user_status"
	ContextKeyUserEmail       ContextKey = "user_email"
	ContextKeyUserGroup       ContextKey = "user_group"
	ContextKeyUsingGroup      ContextKey = "group"
	ContextKeyUserName        ContextKey = "username"
	ContextKeyClientUserId    ContextKey = "client_user_id"
	ContextKeyClientScenairo  ContextKey = "client_scenairo"
	ContextKeyExtra           ContextKey = "extra"
	ContextKeyHeader          ContextKey = "header"
	ContextKeyClaudeSessionId ContextKey = "claude_session_id"

	/* project related keys */
	ContextKeyProjectName         ContextKey = "project_name"
	ContextKeyProjectId           ContextKey = "project_id"
	ContextKeyProjectPlanId       ContextKey = "project_plan_id"
	ContextKeyProjectAllocationId ContextKey = "project_allocation_id"

	ContextKeyLocalCountTokens ContextKey = "local_count_tokens"

	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"

	/*response related keys*/
	ContextKeyAudioUrl       ContextKey = "audio_url"
	ContextKeyTTSCount       ContextKey = "tts_word_count"
	ContextKeySdkResponseStr ContextKey = "sdk_response_str"
	// ContextKeyFileSourcesToCleanup stores file sources that need cleanup when request ends
	ContextKeyFileSourcesToCleanup ContextKey = "file_sources_to_cleanup"

	// ContextKeyAdminRejectReason stores an admin-only reject/block reason extracted from upstream responses.
	// It is not returned to end users, but can be persisted into consume/error logs for debugging.
	ContextKeyAdminRejectReason ContextKey = "admin_reject_reason"

	// ContextKeyLanguage stores the user's language preference for i18n
	ContextKeyLanguage ContextKey = "language"

	// ContextKeyRequestBodyReadTime stores the duration (in milliseconds) to fully read the request body from the client
	ContextKeyRequestBodyReadTime ContextKey = "request_body_read_time_ms"
	// ContextKeyRequestArrivalTime stores the time when the request first arrived (before body is read)
	ContextKeyRequestArrivalTime ContextKey = "request_arrival_time"

	// ContextKeyFalNanoBananaResolution stores the requested nano-banana-2 output
	// resolution (e.g. "1K"/"2K"/"4K"), used to reverse-engineer token usage from
	// fal's per-image price at DoResponse time.
	ContextKeyFalNanoBananaResolution ContextKey = "fal_nano_banana_resolution"
	// ContextKeyFalNanoBananaWebSearch records whether fal's web-search add-on was
	// requested for a nano-banana-2 call (adds a one-time fee).
	ContextKeyFalNanoBananaWebSearch ContextKey = "fal_nano_banana_web_search"
	ContextKeyIsStream               ContextKey = "is_stream"

	// ContextKeyAuditLogged marks that the current request has already recorded
	// a manage/operation audit log inside the handler. When set, the admin-audit
	// fallback in authHelper (finishAdminAudit) skips its record to avoid
	// duplicate entries.
	ContextKeyAuditLogged ContextKey = "audit_logged"
)
