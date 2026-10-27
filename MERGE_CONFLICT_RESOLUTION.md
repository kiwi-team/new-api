# Merge conflict resolution record

本次合并以“不覆盖已手工解决文件、优先采用 main 的新架构与安全/计费语义、保留 prod 的固定渠道编号和仍在使用的定制能力”为原则。下表覆盖合并索引中全部 111 个冲突文件。

| 文件 | 解决方案 |
|---|---|
| `.gitignore` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `common/api_type.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `common/model.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `common/rate-limit.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `common/str.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `constant/channel.go` | 保留用户手工结果，并校正固定编号：prod 0–77 不变，main 新类型顺延为 78–80；修复基础 URL 边界。 |
| `constant/context_key.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `constant/task.go` | 保留用户手工结果；仅消除重复常量名，同时保留旧 remixGenerate 与新 remix 语义。 |
| `controller/audit.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `controller/channel-test.go` | 保留 main 的渠道健康检查流程；适配恢复后的错误处理参数，使自动健康检查失败仍按 prod 语义把本次耗时和请求体写入独立 `error_logs`。 |
| `controller/channel.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `controller/log.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `controller/option.go` | 保留用户手工结果；仅按新服务签名和导入集合做编译兼容修正。 |
| `controller/pricing.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `controller/relay.go` | 采用 main 的新 relay/任务插件流程，并补回 prod 的 Key 渠道列表、重试次数与渠道竞速入口；同时恢复 `SAVE_ERROR_LOG` 控制的独立 `error_logs` 写入链路，逐次记录失败渠道与真实耗时，重试成功时不保存请求体，全部失败时仅最后一条保存请求体，竞速成功仍清除失败请求体。错误日志落库失败不再静默忽略，而会写后端错误日志。恢复每次渠道尝试的耗时累计，并确保成功的最后一次尝试也与 `use_channel` 一一对应。每次选定渠道后按其配置安装响应模型重写器，重试切换渠道时恢复原 writer，避免不同渠道配置串用。 |
| `controller/secure_verification.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `controller/task.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `controller/token.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `controller/token_test.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `controller/twofa.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `controller/user.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `controller/video_proxy.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `controller/video_proxy_gemini.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `go.mod` | 合并两侧依赖后执行 `go mod tidy`，以实际模块图重建校验和。 |
| `go.sum` | 合并两侧依赖后执行 `go mod tidy`，以实际模块图重建校验和。 |
| `main.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `middleware/auth.go` | 保留用户手工认证合并结果；仅修正 DTO 包别名，保持 main 的会话/审计控制与 prod 的项目/UID 校验。 |
| `middleware/distributor.go` | 保留 main 的渠道约束/插件过滤与 prod 的 Key、特殊渠道、全局模型路由优先级；Key 渠道规则和全局模型路由的显式候选必须在渠道支持模型列表中包含用户请求的完整模型名，并继续过滤禁用渠道、路径黑白名单、模态和插件约束；无可用候选时按既有优先级继续回退，将真实请求 Body 与 URL 交给全局路由匹配，并仅为最终采用的规则设置重试次数。 |
| `middleware/utils.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/ability.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/channel_cache.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/log.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/log_format_test.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/main.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/option.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/task.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/usedata.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `model/user.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/channel/gemini/adaptor.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/channel/openai/adaptor.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/channel/openai/image_stream_test.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/channel/openai/relay_responses.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/channel/task/ali/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/ali/constants.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/doubao/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/doubao/constants.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/gemini/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/gemini/dto.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/hailuo/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/kling/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/sora/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/vertex/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/channel/task/vidu/adaptor.go` | 保留 prod 旧任务/视频适配能力；按 main 的新轮询接口补兼容，不删除现有 provider 功能。 |
| `relay/claude_handler.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/common/relay_info.go` | 保留 main 的 relay 元数据，并增加当前渠道尝试的开始时间，使 relay handler 内部提前生成的成功消费日志也能记录最后一个成功渠道的真实耗时，修复 prod 旧逻辑只能在 handler 返回后补时导致成功日志缺尾段的问题；同时缓存实时 WebSocket 使用的模型名称重写器，避免每帧重复解析配置。 |
| `relay/common/relay_utils.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/compatible_handler.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/gemini_handler.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/helper/price.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/helper/valid_request.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/relay_adaptor.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/relay_task.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `relay/responses_handler.go` | 保留 main 的 Responses 请求准备、协议转换和 HTTP/WebSocket 共用结算入口；恢复 prod 的请求/响应日志参数传递：`SAVE_REQUEST_RESPONSE=true` 时，普通 `/v1/responses` 与音频 Responses 都把已捕获的请求体和完整上游响应传给消费日志，不再在共用结算函数中丢弃。Compact 请求继续使用原有独立计价与日志路径。 |
| `relaykit/dto/channel_settings.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `relaykit/dto/openai_request.go` | 保留用户手工结果；补齐注释边界及音频/视频/cache-control 解析，确保 relaykit 独立构建。 |
| `relaykit/dto/openai_response.go` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |
| `relaykit/relayconvert/internal/oai_chat/to_claude_messages_req.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `router/api-router.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `router/main.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `router/video-router.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `service/channel_select.go` | 新增统一渠道重试计划：恢复 prod 的显式渠道顺序/随机结果，支持 race 分组的串行降级，重试次数限制在候选列表内并正确识别显式 `0`；固定渠道优先，同时为每次显式选择复查状态、路径/插件约束和模态标签。 |
| `service/log_info_generate.go` | 保留 main 的分层日志信息结构，恢复管理员域 `admin_info.use_channel_time`；序列化日志时若当前成功尝试尚未返回，则用当前开始时间补齐临时耗时副本，既覆盖普通重试也覆盖竞速子请求，且不重复写入上下文。 |
| `service/quota.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `service/task_billing.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `service/task_billing_test.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `service/task_polling.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `setting/billing_setting/tiered_billing.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `setting/billing_setting/tiered_billing_test.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `setting/ratio_setting/model_ratio.go` | 冲突区采用 main 的新接口/安全与计费语义，保留不冲突的 prod 定制；根据编译与调用链补齐字段、参数和日志兼容。 |
| `web/.gitignore` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/components/layout/components/app-header.tsx` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/components/profile-dropdown.tsx` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/components/ui/combobox-input.tsx` | 保留 main 的可输入下拉框结构与 API；修正选择或清空后再次被焦点事件打开、继续显示旧搜索词的问题，使恢复的日志筛选器能稳定提交所选值。 |
| `web/src/features/channels/components/channels-columns.tsx` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 采用 main 的分栏式 React 表单结构；恢复 prod 的请求路径白名单、请求路径黑名单、响应模型名称重写以及 Vertex AI Google bucket 控件，并继续使用既有回填与提交序列化。 |
| `web/src/features/channels/constants.ts` | 保留 prod 的固定渠道编号 0–77，并将 main 新增 Task Plugin/vLLM/SGLang 固定为 78/79/80；补齐新 UI 所需导出。 |
| `web/src/features/channels/lib/__tests__/new-api-channel.test.ts` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/features/channels/lib/channel-utils.ts` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/features/keys/components/api-keys-mutate-drawer.tsx` | 保留 main 的抽屉结构、服务端错误处理和提交流程；恢复额度告警阈值，并恢复仅 root 可见的渠道倍率、渠道规则可视化编辑器及“优先使用 Key 渠道规则”开关，沿用已有表单字段和后端序列化契约。 |
| `web/src/features/keys/components/api-keys-table.tsx` | 保留 main 的桌面表格、移动卡片和 React Query 数据流；恢复 root 用户筛选器（我的 Key、全部用户、指定用户），并把所选 `user_id` 同时传入普通列表与搜索请求。 |
| `web/src/features/profile/components/tabs/notification-tab.tsx` | 保留 main 的通知设置结构、完整设置合并与错误处理；恢复 prod 的账号级模型限制开关、允许模型多选和“限制为已有结算价格模型”批量去重入口。模型候选仅在启用限制后加载，批量加载失败会恢复按钮状态并提示错误。 |
| `web/src/features/profile/index.tsx` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/features/system-settings/general/channel-affinity/cache-stats-dialog.tsx` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/features/system-settings/maintenance/config.ts` | 合并两侧侧边栏开关：保留 main 新增的审计日志与安全中心开关，同时恢复 prod 的错误日志、账单、用量分析、部署、监控、预算、结算与价格中心等默认配置。 |
| `web/src/features/system-settings/maintenance/sidebar-modules-section.tsx` | 保留 main 的 React 设置页结构，并恢复 prod 各侧边栏/页内模块开关的名称与说明，使管理员仍可配置所有旧入口。 |
| `web/src/features/system-settings/models/model-ratio-form.tsx` | 采用 main 的 React 结构/API 作为冲突基线；仅补回 prod 仍被调用的兼容字段、筛选 API 或交互能力。 |
| `web/src/features/usage-logs/api.ts` | 保留 main 的日志、统计与任务产物 API；恢复 Token 名称选项和管理员用户名搜索选项接口，并复用现有渠道、Client UID 选项接口。 |
| `web/src/features/usage-logs/components/columns/common-logs-columns.tsx` | 保留 main 的日志表格结构，恢复 prod 的请求链路展示：root 在渠道列直接看到各渠道 ID 与逐段耗时，hover 后看到渠道名称、ID 和耗时；竞速日志使用冠军/未胜出图标及提示明确标记 winner/loser。渠道名称继续受敏感信息开关保护。 |
| `web/src/features/usage-logs/components/common-logs-filter-bar.tsx` | 保留 main 的分组筛选、移动端即时日期提交、日志类型标记和紧凑工具栏；恢复 Token 名称、用户名、渠道下拉筛选，以及 Client UID、MT Session ID、Trace ID、Traj ID、Session ID 条件，并补齐 URL 状态同步、筛选计数和管理员权限边界。 |
| `web/src/features/usage-logs/components/dialogs/details-dialog.tsx` | 保留 main 的详情布局、计费与审计信息；恢复 prod 的原始请求体、响应体和请求 Header 查看入口，并根据当前登录账号的实际角色严格限制为 root 可见（root 切换到“仅看自己”时仍可使用）。复用已有 `LogPayloadDialog`，后端三个接口继续由 `RootAuth` 强制鉴权。 |
| `web/src/hooks/use-sidebar-config.ts` | 使用设置页共享默认值，合并 main 的审计日志/安全中心映射与 prod 的错误日志、用量、预算、路由、监控、结算和价格中心映射，避免旧配置被误判为缺失。 |
| `web/src/hooks/use-sidebar-data.ts` | 在 main 新增的审计日志、安全中心、任务插件入口基础上，恢复 prod 的账单分组、错误日志、模型部署、渠道监控、路由配置、环境同步等入口，并恢复组织菜单所需 `pageKeys`。 |
| `web/src/i18n/locales/_reports/_sync-report.json` | 接受 main 删除：该文件是可再生成的同步报告；已运行 i18n 同步验证。 |
| `web/src/i18n/locales/en.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/i18n/locales/fr.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/i18n/locales/ja.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/i18n/locales/ru.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/i18n/locales/vi.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/i18n/locales/zh-TW.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/i18n/locales/zh.json` | 以 main 翻译集为基线，补回 prod 独有键，再通过 `bun run i18n:sync` 统一排序与完整性。 |
| `web/src/routeTree.gen.ts` | 保留用户已手工解决的工作区版本，不重新选择 ours/theirs；仅在编译必需时做窄范围兼容修正。 |

## Post-merge semantic recovery files

以下文件并非本轮新增冲突项，但为防止“冲突已消除、旧功能却被静默删除”而随对应冲突文件一并修复：

| 文件 | 处理内容 |
|---|---|
| `web/src/features/usage-logs/components/log-filter-combobox.tsx` | 为日志下拉筛选器补充可访问名称，使恢复后的 Token、用户、渠道与 Client UID 控件可被辅助技术和回归测试稳定识别。 |
| `web/src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx` | 增加 root 创建 Key 时提交渠道倍率、渠道规则和高优先级开关的回归用例。 |
| `web/src/features/keys/components/__tests__/api-key-listing.test.tsx` | 增加 root 选择指定用户后请求携带 `user_id` 的回归用例。 |
| `web/src/features/usage-logs/components/__tests__/group-filter.test.tsx` | 增加五项关联标识筛选从 URL 回填、搜索提交并保留参数的回归用例。 |
| `controller/relay_race.go` | 将 race 分组的 order/random/random_n 候选生成统一到 service，避免实际竞速与不支持协议的串行降级产生不同顺序。 |
| `controller/responses_websocket.go` | 每个有效 `response.create` 都重新执行渠道分配；分配前使用规范化后的 Responses 请求体，控制帧/非法帧仍由原 WebSocket 校验路径处理。 |
| `relay/responses_websocket.go` | Responses WebSocket 接入统一显式路由与重试计划；禁用 affinity 绕过显式规则，复用 distributor 已选首渠道，并将不支持并发竞速的 race 配置安全降级为同一候选序列的串行尝试。适配共用 Responses 结算入口的新日志参数；WebSocket 尚无 HTTP body 记录器，因此明确传空值，不影响其既有结算。 |
| `relay/responses_websocket_test.go` | 增加 Responses WebSocket 按显式渠道序列选择指定重试索引的回归测试。 |
| `relay/chat_completions_via_responses_test.go` | 增加 Responses 消费日志回归，直接验证共用结算入口会把已捕获的 `/v1/responses` 请求体与响应体写入日志，防止后续 HTTP/WebSocket 结算重构再次静默丢弃。 |
| `service/channel_select_test.go` | 覆盖显式列表边界、配置重试次数、零重试、单候选竞速、race 串行降级及固定渠道优先级。 |
| `model/channel.go` | 增加渠道支持模型精确判断；Key 渠道规则和全局模型路由只能把用户请求的完整模型名确实列在渠道 `models` 中的渠道作为候选。 |
| `model/channel_rule_test.go` | 覆盖渠道支持模型的完整名称、大小写及空模型边界，防止退化为关键词或正则匹配。 |
| `middleware/distributor_priority_test.go` | 覆盖显式候选的请求模型过滤开关，确认同关键词但不同完整模型名的渠道不会进入 Key/全局路由候选。 |
| `model/model_route_config.go` | 修复旧逻辑仅检查模型名、忽略 Body/URL 条件的问题；现在模型、Body、URL 三个已配置维度必须同时匹配，并统一使用项目 JSON 包装函数。 |
| `model/model_route_config_test.go` | 覆盖全局模型路由的模型正则、Body 关键词、URL 关键词联合匹配，以及未配置 Body/URL 时的通配语义。 |
| `web/src/features/usage-logs/components/__tests__/detail-preview.test.tsx` | 增加日志原始报文入口权限回归：root 即使处于“仅看自己”视图仍可见，普通管理员和普通用户均不可见。 |
| `web/src/features/profile/__tests__/settings.test.tsx` | 更新完整用户设置断言以覆盖账号级模型限制字段，并增加已有限制回填、结算模型去重追加及最终保存载荷的回归用例。 |
| `controller/relay_error_log_test.go` | 扩展渠道错误回归，除旧 `logs` 错误类型外，同时断言启用 `SAVE_ERROR_LOG` 后会向 `error_logs` 写入渠道快照、模型、状态码与可审计请求体快照。 |
| `service/text_quota_test.go` | 增加渠道耗时日志回归，验证历史失败尝试耗时与 handler 内正在完成的成功尝试耗时会按渠道顺序写入 `admin_info.use_channel_time`。 |
| `web/src/features/usage-logs/components/__tests__/retry-chain-display.test.tsx` | 增加日志渠道列集成回归，验证链路耗时无需点击即可直接显示、hover 展示渠道名称与耗时，并标出竞速胜出请求。 |
| `model/channel_constraint.go` | 将渠道 `MatchPath` 白名单/黑名单检查接回统一 `FilterRequestPath` 约束；普通、固定、Key 规则、全局路由、随机重试、竞速和 affinity 候选均复用同一入口，黑名单继续优先于白名单；Advanced Custom 在通过黑白名单后仍继续校验自身 route/model。 |
| `model/channel_constraint_test.go` | 增加普通渠道路径约束回归，覆盖白名单未命中和“黑名单优先于已命中白名单”。 |
| `relay/common/model_output_mapping.go` | 新增协议无关的响应模型字段重写器，递归处理 OpenAI/Anthropic 的 `model`、Gemini Native 的 `modelVersion`/`model_version`、Responses 嵌套对象及 SSE data；映射键忽略大小写，并以 `RawMessage` 保留大整数等原始 JSON 数值。 |
| `controller/model_output_mapping.go` | 在 Gin 响应 writer 层统一应用模型名称重写，因此使用不同 provider adaptor、原样透传、流式 SSE 与普通 JSON 的渠道均生效；重写时移除旧 `Content-Length`，避免名称长度变化造成响应截断。 |
| `controller/relay_race_test.go` | 增加跨响应格式重写回归，覆盖 Anthropic、Gemini Native、Responses 嵌套响应、SSE、大小写匹配、超出 JavaScript 安全整数范围的数值保真和 Content-Length 清理。 |
| `relay/helper/common.go` | 增加仅用于“上游到客户端”方向的 WebSocket 响应写入函数，在不改写客户端上行请求的前提下应用渠道模型名称映射。 |
| `relay/channel/openai/relay-openai.go` | Realtime 上游响应转发改用统一模型名称重写函数；客户端发往上游的消息继续原样转发。 |
| `relay/channel/openai/relay_realtime.go` | OpenAI Realtime 下行消息接入统一模型名称重写，上行消息保持不变。 |
| `relay/channel/gemini_realtime/handler.go` | Gemini Live 下行消息接入统一模型名称重写，支持原生 `modelVersion` 字段。 |
| `relay/channel/qwen_realtime/handler.go` | Qwen Realtime 下行消息接入统一模型名称重写。 |
| `controller/user.go` | 移除合并后残留的事务外 `Insert`，创建用户只执行一次包含权限写入的事务插入；保留 prod 的 Leader 角色 `5`，并将上游“非标准角色”回归用例改为真正未定义的角色值。 |

## Future main merge non-regression contract

本节记录本次冲突表面解决后实际发生过的功能回退，以及用户明确要求保留的现有行为。它不是一次性备注，而是后续合并 `main` 时必须保留的项目契约。`main` 中没有对应实现，不代表这里的功能可以删除；除非用户明确批准行为变更，否则必须把这些能力迁移到上游的新结构中，并运行对应锚点测试。

2026-10-26 追溯并修复：Key 显式渠道计划优先于会话绑定的行为在 `2d0605383` 合并中丢失。本次按用户最终确认固定为“大文件路由 → Key 渠道规则 → 全局模型路由 → 系统默认规则”，显式计划优先于会话缓存。后续合并必须核对本文末尾“渠道路由优先级回归追溯与修复”的证据及回归测试，不能把候选列表仍存在视为优先级已经保留。

| 保护能力 | 已发生的问题或保护背景 | 后续合并必须保持的行为 | 主要实现与回归锚点 |
|---|---|---|---|
| Claude 转 Chat Completions 的客户端 usage（2026-10-23 用户确认） | 用户确认现有 `/v1/chat/completions` 返回的 `prompt_tokens` 包含缓存读取和缓存写入，并要求固定为后续合并保护；此项是现有行为确认，不是本次新发现的故障。 | Claude 原生响应转换后，`prompt_tokens = 原始 input_tokens + cache_read_input_tokens + 缓存写入总量`，`cached_tokens` 是其中的缓存读取子集；写入总量取上游汇总与 5m/1h 分项之和的较大值，不重复相加。非流式 JSON 与流式最终 usage 帧保持一致，转换后的 `input_tokens = prompt_tokens`，`total_tokens = prompt_tokens + completion_tokens`，保留 `usage_semantic=openai`、`usage_source=anthropic` 和缓存明细。仅转换对外副本，内部 Claude usage 与原始 `BillingUsage` 不得被覆盖；不能按模型名给已有 OpenAI 口径的上游 usage 再加缓存。 | `relay/channel/claude/relay-claude.go` 的 `HandleClaudeResponseData`、`HandleStreamFinalResponse`、`buildOpenAIStyleUsageFromClaudeUsage`；`relaykit/relayconvert/internal/claude_messages/to_oai_chat_resp.go` 的 `UsageFromClaudeUsage`。测试：`TestBuildOpenAIStyleUsageFromClaudeUsage*`、`TestClaudeStreamFinalUsageEstimatesOnlyUncountedUsage`、`TestConvertResponseProviderToOAIChatUsage`；命令和复核要求见下节。 |
| 异步任务退款与补扣的计费归属（2026-10-23） | 任务提交消费日志有 UID，但创建任务时未持久化 UID、场景等归属；异步退款从空任务属性取值，钱包已退款而 UID 可用额度未恢复。`RecordTaskBillingLog` 还遗漏了将已传入的归属字段写入实际日志行。 | 插入任务前保存原请求的 Token、UID、场景、项目和计划；无 HTTP Context 的差额退款、失败全额退款及补扣均沿用该归属，实际 `logs` 和 `quota_data` 都必须落库。钱包、令牌与 UID 额度分别对账；项目消耗不得进入非项目预算，结算不得重复累计请求或 token 等计数。历史归属修正不得再次退还已到账的钱包或令牌额度。项目分配的已用量必须在提交时记账，退款和补扣仅调整差额；所有分配更新按项目＋计划＋UID 匹配，包括退款归零分支，不能修改同一 UID 的其他计划。已扣费金额不能因预算单位错误或超预算被漏记。 | `controller/relay.go`、`model/task.go`、`service/task_billing.go`、`service/project_budget.go`、`model/project.go`、`model/log.go`、`model/usedata.go`；`controller/relay_task_plugin_test.go` 的 `TestTaskBillingAttributionDatabase`，以及 `TestQuotaDataPersistenceDatabaseMatrix`。受影响合并必须在真实 SQLite/MySQL/PostgreSQL 上验证，不能只检查字段存在或编译通过。 |
| `quota_data` 完整累计与分组隔离（2026-10-23 追溯） | `00a210716`（2026-09-19 合并）相对第一父提交 `2dc19ee6a`，将 `increaseQuotaData` 的 14 项累计替换为上游的 `count/quota/token_used` 三项，并删除 `client_user_id/client_scenairo/project_name/plan_id` 更新条件。后续批次遗漏输入、输出、缓存与耗时统计，且可能更新其他业务分组。`21b5080b3`（2026-10-22）已恢复完整累计与分组匹配。 | 同一完整聚合身份跨批次写入时，14 项计数必须全部累计；用户、模型、小时、Key、渠道、分组、节点、UID、场景、项目、计划维度不得串组。退款仅修正额度，不重复累计请求、token 或耗时。保留 `/api/data/model-usage-analysis` 的聚合、用户范围和人工调整语义；存储迁移不能被视为自动补齐历史漏记数据。 | `model/usedata.go`、`model/quota_data_event.go`、`model/quota_data_migration.go`；`TestQuotaDataPersistenceDatabaseMatrix` 的 `all counters and refunds`、`all dimensions` 子用例，`TestModelUsageAnalysisCanFilterAndIdentifyUsers`、`TestUsageAdjustmentCorrectsWholeDayWithoutMutatingSource`。后续受影响合并须验证 SQLite/MySQL/PostgreSQL 矩阵。 |
| HappyOyster 多模型渠道与 Directing | Adventure 专用前缀若残留在 World 操作或后台轮询中，会把 Directing 请求发送到错误地址；按 MySQL 默认排序规则比较指令 ID 会合并大小写不同的请求。 | 渠道名称为 HappyOyster，编号固定为 `82`，同时提供 Adventure 和 Directing。Directing 原生前缀下保留 16 个接口，包含 simple/scriptlist 创建、暂停/恢复/回溯、过程指令和剧本全量更新。模型权限、临时凭证 scope、资源查询、列表和固定渠道账号均按模型隔离；旧资源及旧临时凭证仍属于 Adventure。`/api/v1/tokens?model=happyoyster-1.0-directing` 选择新模型，省略 model 仍为 Adventure。Directing 按分辨率预扣 180 秒，以原片时长结算，不重复计算合成变体；世界创建和指令注入独立计费。指令以 Travel + clientRequestId 摘要唯一登记，重试不重复执行或扣费；上游结果未知时保留预扣及 pending 记录，需人工对账，不自动退款或重放。暂停响应不能作为已暂停的确认；`rewindToSec=0` 必须保留。 | `constant/happyoyster.go`、`controller/happyoyster_directing.go`、共享 HappyOyster 路由/鉴权/模型/轮询；`TestHappyOysterDirectingLifecycleAndInstructionBilling`、`TestHappyOysterDirectingValidation`、`TestHappyOysterModelAndTemporaryScopeIsolation`、`TestHappyOysterDatabaseMatrix`、`TestHappyOysterDirectingBuiltinBilling`、`TestHappyOysterAdventureRoutesMatchOfficialAPI`、`channel-type-ids.test.ts`。 |
| 固定渠道类型编号与基础常量/DTO | 渠道编号随 `main` 新类型插入而漂移，前后端、任务插件和已有数据库配置可能指向错误 provider。 | prod 已有渠道编号 `0–77` 永久保持；Task Plugin、vLLM、SGLang、TypeSafe、HappyOyster 分别固定使用 `78–82`。前端枚举、后端常量、API type、默认 Base URL、插件绑定和序列化 DTO 必须同步。 | `constant/channel.go`、`common/api_type.go`、`relaykit/dto/channel_settings.go`、`web/src/features/channels/constants.ts`；`constant/channel_test.go`、`common/api_type_task_plugin_test.go`、`web/src/features/channels/lib/__tests__/channel-type-ids.test.ts`。 |
| HappyOyster Adventure 独立模型接入 | `main` 没有这套非 OpenAI 协议时，合并中容易把专用 router/controller、四张资源表、临时凭证鉴权、原生计费或前端渠道类型当成旁支删除，或强行并入通用任务接口；响应包装差异和加密 ID 前缀假设也曾导致上游成功被误报为 502。 | `happyoyster-1.0-adventure` 必须继续在 HappyOyster 渠道类型 `82` 内以独立协议存在，不得强行兼容 OpenAI/通用 Task submit-fetch。保留官方前缀下 11 个 World/Travel 接口以及 `/api/v1/tokens`；同时支持官方信封和百炼 `{output,request_id}` 包装，资源 ID 视为非空 opaque ID。主 Key 与 `st-` 临时 Key 的端点权限、摘要存储、过期/单次 ticket、用户/渠道账号绑定和资源固定路由必须保留；列表继续按网关用户隔离。保留 `happy_oyster_worlds/tickets/travels/client_tokens` 持久化和迁移、后台状态收敛及幂等差额结算。计费继续使用 `world_creations`、`experience_seconds`、`price_scope` 的内置表达式，进入 Travel 按 60/90/120 秒预扣并按实际时长结算。World 创建消费日志必须把完整请求 JSON（含 prompt、URL 或 Base64）和返回给客户端的完整响应分别写入 `logs.request`/`logs.response`，不受 `SAVE_REQUEST_RESPONSE` 开关影响；不得记录 Authorization 或主/临时 API Key。 | `HAPPY_OYSTER_ADVENTURE_INTEGRATION.md`、`router/happyoyster-router.go`、`middleware/happyoyster_auth.go`、`controller/happyoyster.go`、`model/happyoyster.go`、`relay/channel/happyoyster/client.go`、`setting/billing_setting/builtin_billing.go`、渠道常量/前端渠道配置；`TestHappyOysterAdventureRoutesMatchOfficialAPI`、`TestHappyOyster*`、`TestHappyOysterBuiltinBillingUsesProtocolUsage`、`channel-type-ids.test.ts`。 |
| 创建/编辑渠道的旧配置 | 请求路径白名单、请求路径黑名单、响应模型名称重写和 Vertex Google bucket 控件从表单消失。 | 四项配置必须能够创建、编辑回填并保存，字段名和后端序列化契约不得改变；bucket 仅在适用的 Vertex 配置中显示。 | `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`；`web/src/features/channels/components/__tests__/channel-configuration.test.tsx`。 |
| 请求路径白名单/黑名单运行时约束 | UI 字段存在但普通渠道选择链路没有执行约束。 | 所有候选来源——普通随机、固定渠道、Key 规则、全局路由、特殊渠道、重试、竞速和 affinity——都必须经过同一请求路径过滤；黑名单优先于白名单。 | `model/channel_constraint.go`、`middleware/distributor.go`；`model/channel_constraint_test.go`。 |
| 跨协议响应模型名称重写 | 只有 OpenAI 响应生效，Anthropic、Gemini Native、Responses、SSE 和 Realtime 被遗漏。 | JSON、SSE、透传和 WebSocket 下行响应均应用当前渠道的映射；支持 `model`、`modelVersion`/`model_version` 和嵌套 Responses 对象；不得改写客户端上行请求，不得因长度变化保留错误的 `Content-Length`。 | `relay/common/model_output_mapping.go`、`controller/model_output_mapping.go`、Realtime handlers；`controller/relay_race_test.go`。 |
| 左侧菜单与系统模块开关 | 账单、错误日志、部署、监控、预算、路由、结算、价格中心等 prod 入口被 `main` 菜单结构覆盖。 | prod 旧入口与 main 新增审计日志、安全中心、任务插件入口并存；默认开关、设置页名称、路由和组织菜单 `pageKeys` 保持一致。 | `web/src/hooks/use-sidebar-data.ts`、`web/src/hooks/use-sidebar-config.ts`、`web/src/features/system-settings/maintenance/*`；`web/src/hooks/__tests__/sidebar-config.test.tsx` 及侧边栏定向测试。 |
| root 创建/编辑 Key 的渠道规则 | root 可配置的渠道倍率、渠道规则和旧优先级开关曾丢失。 | 渠道倍率、渠道规则及可视化编辑器只对 root 开放，编辑时正确回填，提交时继续写入既有字段；普通账号不可获得该权限。2026-10-26 用户确认固定四级路由顺序后，移除与之冲突的“优先使用 Key 渠道规则”开关，改为展示路由顺序；保留旧字段的存储和序列化兼容性，但不得再让它覆盖大文件路由。 | `web/src/features/keys/components/api-keys-mutate-drawer.tsx`；`api-keys-mutate-drawer.test.tsx`、`channel-rules-editor-dialog.test.tsx`、`web/src/features/keys/lib/__tests__/channel-rules.test.ts`。 |
| root 按用户筛选 Key | Key 列表只能查看当前用户，root 的“全部用户/指定用户”筛选丢失。 | root 可选择我的 Key、全部用户或指定用户；普通列表与搜索请求都必须携带正确的 `user_id`，非 root 不显示越权入口。 | `web/src/features/keys/components/api-keys-table.tsx`；`api-key-listing.test.tsx`。 |
| Gemini 原生/Chat 单一大文件规则（2026-10-14 用户批准变更） | 原模态规则会使小文件按类型跨站，Body 透传和转换又可能跳过附件策略。 | 用户明确批准退役 `OnlyTextChannels`、`VideoChannels`、`OnlyImageChannels`、`NoVideoChannels`，页面和运行时仅保留 `LargeFileChannels`，不继承旧键。`GeminiFileRouting` 默认关闭；按 2026-10-15 用户确认，仅 URL 附件原始字节达到阈值才触发大文件约束；已有 base64 保持 inline，不按大小选路或上传，混合请求只上传大 URL，整个请求体限额继续生效。同日用户批准移除内容识别结果与 MIME 强制一致校验：优先使用声明 MIME，再用响应 MIME，均缺失时才从内容补全；保留媒体分类范围及 base64 校验。固定、Key 高优先级、强制路由、affinity 和重试不得越界；实际 Vertex 凭据绑定 GCS 上传，预扣成功后执行，上传对象按逻辑请求清理，推理提交后不自动重放。保留原生/Chat JSON/SSE、usage、PDF/audio 来源映射、透传检查、请求脱敏和 relaykit 独立性。 | `service/gemini_file_plan.go`、`service/gemini_file_execute.go`、`middleware/distributor.go`、`model/channel_constraint.go`、Gemini/Chat handler、共享转换器、特殊渠道页面；`TestGeminiInlineFilesStayInlineAndPreserveChatConversion`、`TestGeminiFileMIMEPreservesDeclaredType`、`TestGeminiFileURLMIMEFallback`、`TestGeminiFileURLThreshold`、`TestGeminiInlineFilesStillRespectBodyLimit`、`TestGeminiFileGCSLifecycleAndPermissionDomain`、`TestGeminiFilePassthroughAndOverrides`、`TestGeminiFileRoutingCannotBeBypassedAndRetiresLegacyRules`、`TestGeminiFileChannelConstraints`、`TestGeminiFileRoutingOptionsDatabaseMatrix`、`TestGeminiFilePlanActualAdaptorJSONAndSSE`、`TestGeminiFileEffectiveGenerationBounds`、`special-channels.test.ts`、`special-channels-tab.test.tsx`。 |
| Key/全局/特殊渠道规则及三种模式 | Key 维度规则、`/model-route-config` 全局路由和特殊渠道之间的优先级、回退及 order/random/race 行为在重构后不完整；全局路由曾只匹配模型而忽略 Body/URL；Key/全局规则还曾把不支持用户请求模型的显式渠道作为候选并实际发起请求。 | 2026-10-26 用户确认策略优先级固定为：大文件路由 → Key 渠道规则 → 全局模型路由（enforce/fallback）→ 系统默认规则。固定渠道身份约束仍须保留，并通过大文件等全部过滤。大文件路由无兼容候选时直接失败；Key 未匹配或无合格候选时才查全局路由；全局 enforce 匹配后无可用候选仍直接失败，fallback 无候选才进入系统默认。显式候选不得被会话缓存提前拒绝、替换或重排，只有没有显式计划时才应用默认会话亲和；重试耗尽不得跳出已选计划。Key 规则及全局模型路由中的每个显式渠道，必须在自身支持模型列表中包含用户请求参数里的完整模型名，否则在发送请求前排除；不得退化为关键词、子串、正则或仅按规则名匹配。顺序、随机、随机竞速均保持过滤后的候选边界和重试语义；不支持并发竞速的协议按同一候选计划串行降级。全局规则的模型、Body、URL 已配置维度必须同时匹配。 | `middleware/distributor.go`、`model/channel.go`、`service/channel_select.go`、`controller/relay_race.go`、`model/model_route_config.go`；`TestChannelRoutingPriority`、`TestGeminiFileRoutingCannotBeBypassedAndRetiresLegacyRules`、`TestEnforcedModelRouteFailsClosedWhenNoConfiguredChannelIsAvailable`、`TestGetChannelRetryPlanBoundsExplicitRoutes`、`TestSelectResponsesWSChannelHonorsStrictSessionBinding`，以及 `model/channel_rule_test.go`、`controller/relay_race_test.go`、`model/model_route_config_test.go`。 |
| 日志列表筛选条件 | Token、用户名、渠道、Client UID、MT Session ID、Trace ID、Traj ID、Session ID 等筛选入口和 URL 状态同步丢失。 | 恢复的筛选条件必须继续支持回填、清空、提交、计数和权限边界；root 用户名筛选与 Token/渠道选项接口必须保留。 | `web/src/features/usage-logs/components/common-logs-filter-bar.tsx`、`web/src/features/usage-logs/api.ts`；`group-filter.test.tsx`、`filter-combobox.test.tsx`、`log-type-filter.test.tsx`。 |
| Claude Session ID 日志落库 | 合并时 `/v1/messages` 请求校验中的 `metadata.user_id` Session ID 提取被删除，导致 `logs.session_id` 与 `error_logs.session_id` 为空。 | Claude 请求必须在后续校验及 relay 失败前提取 Session ID 并写入请求 Context，使成功消费日志和错误日志均可落库；OpenAI 兼容链路的同类提取也必须保留。 | `relay/helper/valid_request.go`；`relay/helper/max_tokens_bounds_test.go` 中 `claude session id is retained for logging`。 |
| root 查看日志原始报文 | 日志详情中请求体、响应体和 Header 入口消失。 | 三个入口仅 root 可见；root 即使切换为“仅看自己”仍可用；前端隐藏不能替代后端 `RootAuth`。 | `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`、`router/api-router.go`、`controller/log.go`；`detail-preview.test.tsx`。 |
| 个人资料模型限制 | 账号级模型限制开关、允许模型列表和“限制为已有结算价格模型”操作丢失。 | 用户设置必须完整回填并保存模型限制；令牌单独设置限制时以令牌为准；批量加入结算模型需去重且失败时恢复 UI 状态。 | `web/src/features/profile/components/tabs/notification-tab.tsx`、用户设置 DTO/接口；`web/src/features/profile/__tests__/settings.test.tsx`。 |
| 独立错误日志 | 失败请求只写普通 `logs`，不再写 `error_logs`。 | `SAVE_ERROR_LOG`/错误日志开关启用时，每次失败渠道记录真实渠道快照与耗时；重试成功或竞速胜出时清除不应保留的失败请求体，全部失败时保留最终可审计请求；落库失败必须写后端错误日志。 | `controller/relay.go`、`controller/channel-test.go`、`model/debug.go`；`controller/relay_error_log_test.go`。 |
| 日志渠道链路与竞速胜者 | 渠道列不再直接展示各段耗时，hover 信息和 race winner/loser 标记消失；成功的最后一段耗时曾漏记。 | root 渠道列直接展示每个渠道的耗时，hover 展示渠道名称、ID、耗时；竞速日志明确标记胜出/未胜出；`use_channel` 与 `admin_info.use_channel_time` 一一对应并包含最终成功尝试。 | `service/log_info_generate.go`、`controller/relay.go`、日志列组件；`service/text_quota_test.go`、`retry-chain-display.test.tsx`。 |
| 渠道飞书错误告警 | relay 错误处理迁移到 `service.ProcessChannelError` 时，渠道可能欠费提醒和最终失败慢错误预警未迁移，渠道仍会自动禁用但不再发送飞书。 | 欠费提醒继续读取 `feishu_qianfei_webhook_url`/`feishu_qianfei_secret`，在开启自动禁用且错误命中 `AutomaticDisableKeywords` 时发送；普通渠道按渠道限频 1 分钟，海外、阿里、theapi/aliyun 相关渠道限频 1 小时。慢错误提醒继续读取 `slow_error_feishu_webhook_url`/`slow_error_feishu_secret`/`slow_error_threshold_seconds` 和 `ErrorWarningEnvName`，阈值无有效配置时为 180 秒，同渠道限频 10 分钟；仅在整条串行重试链最终失败且最终失败渠道的单次耗时达到阈值时发送，重试后成功、渠道定时测试和阈值以下错误不得误报。两类通知必须保留渠道名称/ID、状态码等诊断信息，并对上游错误中的凭证和敏感信息脱敏。 | `service/relay_error.go`、`controller/relay.go`；`TestProcessChannelErrorSendsFeishuAlerts`。 |
| `/api/log/stat` 额度统计 | `SumUsedQuota` 为避免 GORM 第二次 `Scan` 清空首次结果，将 quota 与 RPM/TPM 拆到独立结构体，但遗漏了将 `quotaResult.Quota` 回填 `stat.Quota`，导致 `/api/log/stat` 和 `/api/log/self/stat` 无论实际用量多少都返回 `quota: 0`。 | quota 查询和 RPM/TPM 查询可以分开 `Scan`，但返回前必须同时填充 `Stat.Quota/Rpm/Tpm`。`quota` 必须是当前筛选范围内消费与负值退款的净额，不得因查询重构、增加速率指标或 GORM 扫描目标调整而默认丢弃。 | `model/log.go`、`controller/log.go`；`TestSumUsedQuotaPreservesQuotaAcrossRateScan`。 |
| `/v1/videos` 任务 ID 兼容字段 | OpenAI Video 新响应只保留 `id`，删除了旧客户端依赖的 `task_id`，导致既有调用方无法读取任务 ID。 | `POST /v1/videos` 与 `GET /v1/videos/:task_id` 的所有内置 provider 和 Task Plugin 响应必须同时返回 `id`、`task_id`，且两者始终等于同一个公开任务 ID；不得返回上游私有任务 ID，也不得以兼容新版接口为由删除 `task_id`。 | `model/task.go`、`relay/channel/task/*/adaptor.go`、`relay/channel/task/jsplugin/adaptor.go`；`TestTaskToOpenAIVideoDoesNotExposeResultURL`、`TestPresentTaskSubmissionUsesHostOpenAIVideoCreateReceipt`、`TestTaskAdaptorPreservesSoraVideoResponseFields`。 |
| Gemini Omni 与 Veo 任务协议（2026-10-15 回归） | 按渠道类型无条件选择 Google/Vertex Veo 插件，覆盖了现有 Omni Interactions 实现；Vertex 合法的空 Base URL 被插件通用校验拒绝，即使填写地址也会误发 predictLongRunning。 | 未被显式插件绑定的 Gemini/Vertex `gemini-omni*` 请求按映射后的模型选择原生 Interactions；Vertex 继续由项目和区域生成默认地址。原生 Omni 保留数字平台及已有上游 ID 编码，轮询、结果读取和结算根据上游 ID 与插件执行快照区分 Omni/Veo，不能把公开 task_id 当作协议标识。显式绑定的插件和 Veo 仍使用插件，不放宽通用插件 URL 校验。 | `relay/relay_adaptor.go`、`relay/relay_task.go`、`relay/channel/task/gemini/adaptor.go`、`relay/channel/task/vertex/adaptor.go`；`TestGoogleOmniTasksKeepInteractionsWithVeoPluginsEnabled`、`TestGoogleOmniPollingUsesInteractionEndpoint`、`TestGetTaskAdaptorForRequestUsesExactPinnedPlugin`、`TestRelayTaskSubmitMapsBeforeValidateWhenOriginSet`。 |
| 级联视频任务直链（2026-10-15 回归） | OpenAI/Sora 任务迁移到插件后，解析器漏读上游 `video_url` / `result_url` / `url`；查询又把本机 HTTPS `/content` 兜底地址当作已转存结果，国内站始终无法返回海外已有的 S3 地址。 | 成功任务按既有优先级读取并保存上游直链（含签名参数），仅上游没有直链时使用本机内容代理。已完成但仅存本机代理地址的任务允许查询补取，已有直链则不再回源；补取失败、过期或返回旧状态不能撤销成功态或重复结算、退款。 | `plugins/tasks/sora/plugin.js`、`relay/relay_task.go`；`TestVideoTaskQueryPreservesUpstreamURLDatabaseMatrix`（SQLite/MySQL/PostgreSQL）、`TestTryRealtimeFetchDoesNotDoubleRefund`。 |
| `/v1/responses` 请求/响应日志 | handler 已捕获请求和完整响应，但 HTTP/WebSocket 共用结算函数把两个参数替换为空串，普通 Responses 日志因此没有原始报文。 | `SAVE_REQUEST_RESPONSE=true` 时，普通及音频 HTTP `/v1/responses` 必须把已捕获请求、响应传入消费日志；`/v1/responses/compact` 保持独立计价日志路径。WebSocket 没有 HTTP body 记录器时不得伪造报文。 | `relay/responses_handler.go`、`relay/responses_websocket.go`；`TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse`。 |
| 管理员创建用户的原子性与 Leader 角色 | 旧的 `Insert` 与 main 新的 `InsertWithTx` 同时保留，一次请求重复插入用户并因主键冲突失败；main 的新测试还把 prod 的 Leader 角色 `5` 当成非法值。 | 创建用户只在同一个事务中写入用户和管理员权限，成功后再执行缓存/邀请奖励收尾；Leader 角色 `5` 继续是合法角色，未定义角色必须在写库前拒绝。 | `controller/user.go`、`common/constants.go`；`TestAdminUserRoutineEditsDoNotRequireProof`、`TestCreateUserRejectsNonStandardRole`。 |

| 日志导出协议与权限（2026-10-18） | 两侧同时注册 `GET /api/log/export`，Gin 启动 panic；直接接受 main 的管理员导出会泄露本分支仅 root 可见的渠道/原始报文。 | 保留 root CSV `/api/log/export`；管理员 NDJSON 使用 `/api/log/export/stream`，用户使用 `/api/log/self/export`。列表、流式导出共用过滤器，保留 UID/组织范围及用户历史天数；普通管理员清除 request/response/usage/header 和渠道链路，root 保留完整数据。 | `router/api-router.go`、`controller/log.go`、`model/log.go`、日志导出前端 API；`TestLogExportDatabaseMatrix`、`TestLogExportRules`、`TestAccessTokenRouteRulesCoverEveryDashboardRoute`。 |
| 结构化日志与额度（2026-10-18） | main 把日志内容从字符串改为 `common.Message`，直接覆盖会丢失本分支显式额度、Realtime 分模态用量和 TTS 字符说明。 | 内容使用固定英文键与参数，并在七语言前端翻译；`RecordLog` 保留可选显式额度；音频/文本共用结算入口保留 HTTP 报文；Realtime 保留真实分模态用量，不以日志重构改变扣费。 | `model/log.go`、`service/quota.go`、`service/text_quota.go`、`controller/task_video.go`、`controller/happyoyster.go`；`TestLogExportDatabaseMatrix`、`TestTiered*`、`TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse`。 |
| 日志 JSON 跨库（2026-10-18） | `extra/header` 固定 `jsonb` 无法在 MySQL 建表，PG 专用 JSON 运算符不能直接用于所有日志库；ClickHouse 建表漏掉分支日志列。 | PostgreSQL 保留原 jsonb 列；SQLite 使用 text，MySQL 使用 longtext，ClickHouse 使用 Nullable(String)。JSON API 仍输出字符串，空值仍为 NULL；consume/error 列表和导出共用按方言的精确 JSON 筛选。ClickHouse 已有 logs 表以幂等 ADD COLUMN 补齐分支字段。 | `model/log.go`、`model/error_log.go`、`model/main.go`；`TestLogExportDatabaseMatrix` 的 fresh/upgrade、报文/JSON 筛选及重复迁移用例。 |
| 内存限流令牌隔离（2026-10-18） | main 将限流主体改为用户/分组并改用成功请求预留，可能覆盖本分支同用户不同 Key 的独立额度。 | 内存主体同时包含用户、已配置独立额度的分组和 token_id；保留 main 的成功请求预留、失败流释放及标准 429 响应；Redis 保持既有用户/分组粒度。 | `middleware/model-rate-limit.go`；`TestModelMemoryRateLimitIsolatesTokensWithinConfiguredGroup`、`TestModelMemoryRateLimitReservesConcurrentSuccessAdmission`、`TestModelRateLimitCountsConfiguredGroupsSeparately`。 |
| 自定义管理路由的个人令牌边界（2026-10-18） | main 新增全路由权限声明检查，自定义管理路由未声明；简单开放任意 token 会扩大新权限令牌授权，改为 session-only 又会中断旧令牌。 | 自定义管理路由显式声明 legacy 规则：浏览器会话不变，旧令牌仍受原截止日期和角色校验，新 scoped token 拒绝；不能用 `any` 规则代替未定义的授权。 | `middleware/access_token_routes.go`、`middleware/auth.go`；`TestUserAuthAppliesAccessTokenRouteRules`、`TestAccessTokenRouteRulesCoverEveryDashboardRoute`。 |

### Claude → Chat Completions usage 合并复核

- 示例：Claude 普通输入 `100`、缓存读取 `30`、缓存写入 `50`、输出 `20`，对外必须返回 `prompt_tokens=180`、`prompt_tokens_details.cached_tokens=30`、`prompt_tokens_details.cache_write_tokens=50`、`total_tokens=200`；内部原始普通输入仍为 `100`。
- 复核非流式 `HandleClaudeResponseData` 和流式 `HandleStreamFinalResponse` 均实际调用共享转换，且保留现有是否输出 usage 的控制。AWS Bedrock、Vertex 等复用 Claude handler 的路径同样受保护。仅保留转换函数或通过编译不能替代调用链检查；迁移 handler 时补充实际 JSON/SSE 输出的回归验证。
- 后续受影响合并必须运行下列现有锚点测试；涉及 `relaykit/` 或其公共 API 的改动还须在该模块内执行 `GOWORK=off go build ./...`。

```sh
# 仓库根目录：客户端总量、缓存写入汇总/分项、纯缓存输入的流式处理
go test ./relay/channel/claude -run '^(TestBuildOpenAIStyleUsageFromClaudeUsage.*|TestClaudeStreamFinalUsageEstimatesOnlyUncountedUsage)$' -count=1
# relaykit/ 目录：对外总量、缓存明细与原始 BillingUsage 同时保留
GOWORK=off go test ./relayconvert -run '^TestConvertResponseProviderToOAIChatUsage$' -count=1
```

2026-10-23 本次分析已执行第二条 `relaykit` 测试并通过；本次仅记录既有行为，未修改业务代码。第一条为后续受影响合并的必跑命令，不在本次已执行验证范围内。

### Required merge review procedure

1. 合并前先按上表定位受 `main` 修改影响的文件，并比较行为，不只比较是否仍能编译。
2. 冲突处理中禁止对上表相关文件直接整文件选择 `theirs/main`；应以 `main` 新架构为载体逐项移植受保护能力。
3. 合并后检查前端入口、后端鉴权、请求序列化、运行时执行和日志落库五个层面，避免只恢复 UI 或只恢复字段。
4. 至少运行受影响行列出的锚点测试。涉及多个路由能力时运行 `go test ./constant ./model ./service ./middleware ./controller ./relay -count=1`；涉及前端时运行相应 Vitest 文件并执行 `bun run build:check`。
5. 若 `main` 的架构使原实现无法原样保留，允许等价迁移，但必须新增/更新可观察行为回归测试，并在本节记录新的实现锚点。未经用户明确同意，不得以“上游已删除”作为删除这些能力的理由。

## Verification

- `go test ./... -count=1`: 通过（在允许 loopback listener 的环境执行，全量 Go 测试通过）。
- `cd relaykit && GOWORK=off GOCACHE=/tmp/new-api-relaykit-cache go build ./...`: 通过。
- `cd web && bun run build:check`: 通过（TypeScript + production build）。
- `cd web && bun run i18n:sync`: 通过。
- 渠道编号/新渠道定向测试：9 个 Vitest 用例及 6 个后端编号对照用例通过。
- 任务适配器兼容：保留 main 的 JS 插件优先路由，同时为 prod 旧任务适配器增加接口桥接；实时查询、退款防重、公开任务 ID 回归测试通过。
- 计费与流式响应兼容：退款日志按 main 的负数语义断言；Gemini 流首帧携带上游 usage，相关定向测试及全量测试通过。
- 内置任务插件及前端 provider 映射已按当时的固定渠道编号 0–80 校正；后续新增 TypeSafe `81` 与 HappyOyster Adventure `82` 也已纳入本文件的非回归契约和前后端编号测试。
- 渠道配置表单完整回归文件：65 个 Vitest 用例通过，覆盖上述四项配置的编辑回填与保存。
- 侧边栏定向回归测试 17 个 Vitest 用例通过，覆盖 prod 旧入口与 main 新入口的并存、模块默认值、独立部署开关及组织菜单 `pageKeys`。
- Key 与日志筛选定向回归：5 个 Vitest 文件共 54 个用例通过；渠道规则解析/序列化的 Bun 测试 15 个用例通过。覆盖 root 渠道规则提交、root 按用户筛选 Key、日志关联标识 URL 状态及下拉框选择/清空行为。
- 本轮涉及的 9 个前端文件通过 `oxlint` 与 `oxfmt --check`；随后再次执行 `bun run build:check`，TypeScript 与生产构建通过。
- 前端完整测试套件曾启动，但在并行运行时出现多组 20 秒级超时并被终止；不将完整前端测试标记为通过。
- 数据库相关合并尚未完成 SQLite/MySQL/PostgreSQL 三数据库实机矩阵，因此不声明数据库兼容性验证完成。
- 本轮渠道路由定向验证：`go test ./model ./service ./middleware ./controller ./relay -count=1` 通过；覆盖 Key 顺序/随机/竞速重试计划、Responses WebSocket 显式路由和全局 Body/URL 匹配。
- Key/全局模型路由支持模型过滤定向验证：`go test ./model -run 'Test(ChannelSupportsRequestedModel|SelectChannelRuleGroupCandidates|FlattenChannelRuleGroups|ModelRouteConfig)'`、`go test ./middleware -run 'Test(ShouldSpecialChannelsOverrideKeyRules|FilterExplicitChannelCandidatesRequiresRequestedModelWhenEnabled)'` 和 `go test ./service -run TestGetChannelRetryPlanBoundsExplicitRoutes` 通过；确认只有支持模型列表包含用户请求完整模型名的显式渠道会进入候选，同关键词但不同模型名会被排除，全部排除后的回退及最终异常语义保持不变。
- 本轮再次执行 `go test ./... -count=1` 时有一个未定位的全量套件失败；上述五个受影响包随后/单独均通过，因此不把这次全量复跑记录为通过，需在资源允许时用 JSON 输出继续定位。
- 日志详情原始报文入口定向测试：`bun run test --run src/features/usage-logs/components/__tests__/detail-preview.test.tsx`，22 个用例通过；后端 `/api/log/:id/request`、`response`、`header` 路由均确认位于 `RootAuth` 路由组。
- 个人资料模型限制定向验证：`bun run test -- src/features/profile/__tests__/settings.test.tsx`，8 个用例通过；相关文件通过 `oxlint`、`oxfmt --check`，并通过 `bun run typecheck`。
- 独立错误日志恢复验证：`go test ./controller ./service -count=1` 与 `go test ./model -count=1` 均通过；定向用例确认 `SAVE_ERROR_LOG=true` 时 `error_logs` 实际产生记录。未执行 MySQL/PostgreSQL 实机矩阵，因此本轮不额外声明跨数据库验证完成。
- 渠道飞书错误告警恢复验证：`go test ./service -run 'TestProcessChannelError(SendsFeishuAlerts|MasksDisableReasonAndNotification)$' -count=1`、`go test -race ./service -run 'TestProcessChannelErrorSendsFeishuAlerts$' -count=1` 与 `go test ./service ./controller ./relay -count=1` 通过；覆盖欠费关键词告警及按渠道限频、慢错误最终失败/阈值边界、环境名/RequestId 内容和错误信息脱敏。
- 日志 quota 统计回归验证：`go test ./model -run 'TestSumUsedQuotaPreservesQuotaAcrossRateScan$' -count=1` 和 `go test ./model -count=1` 通过。使用隔离的临时 `logs` 表直接调用 `SumUsedQuota`，在 SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 18.3 上均得到 `quota=850 rpm=1 tpm=30`；验证命令形式为 `go run /private/tmp/verify_log_stat.go <sqlite|mysql|postgres> <isolated-test-dsn>`，临时程序和 MySQL 容器已在验证后清理。
- 请求链路展示定向验证：`bun run test -- src/features/usage-logs/components/__tests__/retry-chain-display.test.tsx` 通过（2 个用例）；后端渠道耗时定向用例 `TestAppendRelayLogAdminInfoWritesChannelAttemptTimes` 通过。
- 渠道请求路径与响应规则定向验证：`TestChannelSatisfiesFilters` 覆盖普通渠道白名单/黑名单，`TestModelOutputMappingResponseWriterRewritesAllResponseFormats` 覆盖 OpenAI/Anthropic/Gemini Native/Responses/SSE 的统一模型名称重写；受影响的 model/controller/relay/helper/OpenAI/Gemini Realtime/Qwen Realtime 包完整测试在允许 loopback listener 的环境全部通过。
- Responses 请求/响应日志恢复验证：`go test ./relay -count=1` 通过；新增 `TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse` 确认普通 Responses 结算会将捕获的请求体和响应体实际写入消费日志。
- HappyOyster Adventure 非回归验证：`go test ./controller -run 'TestHappyOyster' -count=1`、`go test ./router -run 'TestHappyOysterAdventureRoutesMatchOfficialAPI' -count=1`、`go test ./setting/billing_setting -run 'TestHappyOysterBuiltinBillingUsesProtocolUsage' -count=1` 以及 `cd web && bun test src/features/channels/lib/__tests__/channel-type-ids.test.ts` 通过；覆盖响应包装/opaque ID、临时 Key 端点边界、World 完整请求响应日志、11 个官方路由、原生计费表达式和渠道编号 `82`。

## 2026-09-29 merge main 冲突修复

### 合并范围与判断基线

- 当前分支基线为 `16a13b853`，本次合入的 `main` 为 `789c97019`；开始处理时索引中有 40 个未解决文件。
- 先复核本文件的 “Future main merge non-regression contract”，再逐个比较 stage 2（当前分支）和 stage 3（main），没有对受保护文件整文件采用 `theirs`。
- 涉及计费、Task Plugin、认证和前端 UI 的冲突，分别先核对 `.agents/rules/billing.md`、`docs/plugin-api/v1.md`、OWASP Authentication/Session/OAuth 指南、`web/AGENTS.md` 及项目 UI/i18n 约定。

### 主要冲突决策与理由

| 范围 | 处理结果 | 理由 |
|---|---|---|
| 渠道选择与重试 | 在 `middleware/distributor.go`、`service/channel_select.go`、`controller/relay.go` 上保留 main 的统一请求策略、约束和性能结果，同时接回固定渠道、Key 规则、特殊渠道、全局模型路由、顺序/随机/race 计划以及逐次错误日志。显式候选继续校验状态、完整模型名、路径、标签和插件约束。 | 统一策略是 main 的新执行骨架；显式路由、竞速、日志和固定编号属于既有生产契约，任一侧单独覆盖都会造成可编译但静默回退。 |
| 固定渠道编号 | 前后端继续保持既有 `0–77`，New API 为 `76`；Task Plugin、vLLM、SGLang、TypeSafe、HappyOyster Adventure 继续为 `78–82`。同步修正插件绑定测试和 provider 映射。 | 已有数据库记录、前端表单和任务插件都持久化数值编号，按 main 插入顺序重排会把旧渠道解释为其他 provider。 |
| 阿里云旧图片/音频能力 | 保留 main 的适配器改造，同时恢复 `relay/channel/ali/image.go`、`image_wan.go`、图片 DTO、Qwen 设置和图片倍率。 | main 已迁移部分能力到 Task Plugin，但当前分支仍有直接调用与已有渠道配置；上游删除文件不能作为移除生产能力的依据。 |
| OpenAI/Gemini 流式响应 | 合并 main 的 Responses outcome/usage/response-model 处理与当前分支 Ring/Ling、图片解析、finish reason、上游请求 ID 逻辑；增量 JSON 继续使用项目 wrapper/流式解析，不引入业务代码直接 marshal/unmarshal。 | 两侧分别承载计费健康语义和 provider 兼容语义，必须并存。 |
| Responses WebSocket | 保留统一请求 runner、渠道策略、流式结算、显式重试、路由 Body 和上游请求 ID；补齐中间件阶段拒绝的健康采样，AccessDenied/429 不污染模型健康；初始上游拒绝继续保留协议错误类型并退款。 | main 新测试要求 HTTP/SSE/WS 具有相同健康与计费分类；当前分支的全局错误 envelope 会改写 type，因此仅在 WS 协议转发帧恢复上游 type，不改变普通 HTTP 全局行为。 |
| 模型请求限流 | 保留当前分支按 `(uid, token_id)` 隔离，采用 main 的 reservation 机制与 `StreamStatus.ResponseFailed()` 成功判定，并修复 Redis 总限流拒绝后继续执行的问题。 | 避免不同 Token 相互占用额度，同时保证 failed/incomplete 流不会错误消耗“成功请求”额度。 |
| Task Plugin | Doubao 合并 main 的图片任务和 `1.1.0` 元数据，保留固定渠道类型 `57`；Sora 合并 `1.1.0` 协议并保留类型 `58`。数值 usage description 继续表达“计费主体 + 单价”，单位放在 `unit`。 | 兼顾 main 的协议扩展和现有渠道绑定，避免元数据版本或类型编号漂移。 |
| 计费与日志 | `relay/image_handler.go` 保留响应记录、SDK 响应和结算后的图片数量；任务轮询合并 main 的性能收敛与当前分支的结算、脱敏、物化流程；日志合并 response model、billing source、任务同步状态与重试链/竞速结果。 | 预扣、结算、退款及 billable quantity 必须来源一致，日志还需保持现有审计字段和敏感信息边界。 |
| OAuth | 采用 main 的安全流程并保留 legacy binding 迁移：继续执行 state/会话校验、账号绑定权限和安全审计，不恢复可绕过服务端校验的旧分支。 | 与 OWASP 的服务端校验、会话绑定、重放防护和敏感信息不落日志要求一致。 |
| 前端 UI 与 i18n | 以 main 的 React/shadcn 组合结构为基础，复用现有 MultiSelect、Combobox、表格与按钮组件，补回 API 地址复制、root Key 操作、日志链路/响应模型、用户组织列及固定 provider 映射。七个 locale 以 main 新 key 为全集、当前分支同 key 翻译优先，再执行同步工具。 | 避免复制已有交互组件；同时防止只恢复英文或因整文件选边而丢失任一侧翻译。 |
| relaykit | 保持模块独立，reasoning 冲突按 main 的“诊断并采用更具体控制”语义处理；保留当前分支 `video_tokens` DTO，并更新转换 golden。 | relaykit 不能依赖宿主；推理字段冲突的新语义已有专门 diagnostics 回归，而视频 token 是现有 usage 契约。 |
| 数据库测试夹具 | 请求策略 SQLite 夹具显式迁移 `ModelRouteConfig`；未修改生产 schema 或迁移逻辑。 | 合并后的 distributor 会查询该表，测试夹具必须声明自身依赖，不能靠生产查询吞掉“表不存在”。 |

### 验证结果

- 冲突标记扫描通过；所有 40 个冲突文件均已形成单一工作区结果。
- `go test ./controller -count=1`：通过（在允许 loopback listener 的环境执行）。
- `go test ./middleware ./relay ./service ./plugins ./pkg/jsplugin -count=1`：通过。
- `cd relaykit && GOWORK=off GOCACHE=/tmp/new-api-relaykit-cache go build ./...`：通过。
- `cd relaykit && GOWORK=off GOCACHE=/tmp/new-api-relaykit-cache go test ./... -count=1`：通过。
- `cd web && bun run typecheck`：通过。
- `cd web && bun run test -- channel-configuration provider-selection api-addresses detail-preview model-badge option-hint option-icon`：7 个文件、335 个用例通过。
- `cd web && bun run build`：通过；`bun run i18n:sync` 与 `bun run format:plugins:check`：通过。
- `cd web && bun run lint`：未通过；完整 lint 在大量本次冲突范围外的既有文件中报告规则存量问题（例如 `src/lib/utils.ts`、旧 `src/helpers/*`、旧 `src/components/table/*`）。TypeScript、定向测试和生产构建均已单独通过，本轮未扩大范围批量改写旧代码。
- `TestRequestPolicyRoutingDatabaseMatrix` 的 SQLite 3.50.4 用例通过；MySQL/PostgreSQL 因未配置 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 被测试显式跳过。本轮未修改生产数据库 schema，且不声明三数据库实机矩阵已完成。

## 2026-10-13 HappyOyster Directing 接入验证

- 渠道编号仍为 82；前端复用既有渠道配置组件，按 i18n 技能流程补齐七种语言（及 zh-CN 镜像），不新增 UI 组件。既有渠道的 models 列表不被自动修改，使用 Directing 时须在该渠道启用 `happyoyster-1.0-directing`。
- `go test ./controller ./router ./setting/billing_setting -run 'TestHappyOyster|TestValidateHappyOyster' -count=1`、对应 `go test -race`、`go build ./...` 均通过。覆盖模型/用户/临时凭证 scope 隔离、两个协议前缀、脚本与零值校验、票据单次使用、并发及已完成指令重试、明确失败退款、未知结果保留预扣、后台按原片结算及日志不含票据/RTC 凭证。
- 三库实测：SQLite **3.50.4**、MySQL **8.0.46**、PostgreSQL **16.15**。执行 `HAPPYOYSTER_TEST_MYSQL_DSN='root:happyoyster-test-only@tcp(127.0.0.1:36465)/happyoyster_test?parseTime=true&clientFoundRows=true' HAPPYOYSTER_TEST_POSTGRES_DSN='host=127.0.0.1 port=33887 user=postgres password=happyoyster-test-only dbname=happyoyster_test sslmode=disable' go test ./controller -run TestHappyOysterDatabaseMatrix -count=1 -v`，六个 fresh/upgrade 子用例全部通过，每个子用例连续执行两次迁移。端口和口令仅用于本次临时测试容器。覆盖旧 Adventure 字段/计费快照/凭证 scope 保留、模型间相同 opaque ID、模型内唯一性及大小写不同的指令 ID。升级 fixture 重建本分支变更前四张表；当前最新本地 release tag `v1.0.0-rc.41` 不含 HappyOyster 表，其受影响表的迁移起点由 fresh 用例覆盖。未测试最低支持版本或生产数据库副本，不将定向迁移测试宣称为完整应用升级演练；日志数据库 schema 未变更。
- `cd web && bun run typecheck`、`bun test src/features/channels/lib/__tests__/channel-type-ids.test.ts`（7 个用例）、`bun run build`、`bun run i18n:sync` 均通过。修改的 TypeScript 文件通过 oxlint；sync 脚本只有既存 warning。全仓库 `bun run format:check` 因范围外既有格式问题失败，未批量修改无关文件。
- 鉴权检查参考 [OWASP ASVS 5.0.0](https://owasp.org/projects/asvs)、[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 和 [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)。本次控制点为服务端模型/资源/端点权限、临时凭证摘要存储与原令牌校验、单次票据、防重放以及凭证不写日志；不声称完成整个应用的 ASVS 合规认证。
- 尚未调用真实付费上游；价格取自 [Directing 官方美元价格](https://www.alibabacloud.com/help/en/model-studio/happyoyster-1-0-directing)。参考图分类按请求中的 firstFrameImage/inputImages/主体 refImage 判断，最终仍须真实账单核对。`update-script` 当前不单独收费；Directing 使用原生 REST + RTC，不支持 OpenAI StreamOptions。网络中断、异常成功响应等无法确定上游结果时保留预扣并要求人工对账，不自动重放计费动作。

## 2026-10-18 merge main 冲突修复

### 范围与原则

- 起点 HEAD `ca378c068c35f9406f76678da31aa94d5606e55b`，合入 main `6cf0d90b5ff27f901697dddd61e0c28aba8efefc`；接手时 90 个未合并文件、303 个文本冲突块。
- 按本文件既有非回归契约、2026-09-29 合并记录和 2026-10-13 HappyOyster 记录处理。保留 main 的消息国际化、流式日志导出、安全校验、tokenkit 计数和 UI 拆分，同时把本分支路由、计费、日志、组织范围与配置入口移植到新接口。
- 原冲突内容备份到工作区外 `/tmp/new-api-merge-20261018/original/`。本轮仅解决并暂存冲突，不创建 merge commit，不处理用户原有未跟踪文件。

### 解决方案与原因

| 范围 | 最终方案及原因 |
|---|---|
| 公共上下文、初始化、pprof、入口 | 合并上下文键和两侧环境配置；保留本分支静态限流、告警环境、pprof 阈值/冷却与堆栈，并采用 main 的服务端 LogText 和退出信号处理，避免诊断配置回退。 |
| 用户、MFA、令牌、设置 | 保留 verified email 唯一性校验和服务端角色限制，同时保留注册 UID、Leader 角色及账号模型限制；保存设置从原设置合并。Token 合入 `AllowUserQuotaQuery`，并保留渠道规则、高优先级、倍率和告警字段；Redis Lua 参数与 TTL 索引同步调整，避免新增字段被当作 TTL。 |
| 普通日志、审计和导出 | 采用 main 的结构化消息及 `LogFilter`，保留原始报文、Header、session/project/client UID、渠道链路、首字时间及显式 quota。查询与导出统一筛选和组织范围，root CSV 与新 NDJSON 地址分开注册，后端清理 root 专属字段。quota 与 RPM/TPM 的独立 Scan 继续正确回填。 |
| 日志数据库 | 三库实测发现原有 PG-only jsonb 建表及筛选。引入字符串型 `LogJSON`，按方言选择存储类型，保持 PostgreSQL jsonb 不降级；错误日志复用同一 JSON 筛选逻辑。ClickHouse 迁移补齐自定义消费日志字段，Scanner/Valuer 保留 NULL；查询不把从主库补充的 channel_name 当作 ClickHouse 物理列。完整启动还发现 AiSearchLog.Extra 的 TEXT 默认值被 MySQL 拒绝，移除该无效默认值，应用写入仍由 Go 零值保存空串；不更换 GORM/driver 版本。 |
| 配额、充值、兑换、任务 | 改用 `common.Message` 与参数，保留业务额度、异步任务补扣/退款、原始用量、project/plan/session 和原始报文。文本、音频、Realtime、TTS、视频及 HappyOyster 的旧字符串日志迁移到固定英文键，新增 19 个键的七语言翻译，避免只修类型编译而丢计费说明。 |
| 路由与协议 | 保留 TypeSafe/Decisions、显式渠道顺序/随机/竞速和 Responses WS 每次分配；合入 DeepSeek tools、错误地址脱敏、WebSocket close code/reason。保留原生 Gemini Omni 与 Veo 插件分流及公开 video task_id/直链逻辑；原有完整测试继续作为锚点。 |
| Gemini 与 tokenkit | 采用 tokenkit 计数和 main 的工具/多模态 function response 支持；保留大 URL 文件路由及 file plan，计数阶段不提前下载或重复上传。统一 DTO 中 MIME 内部提示不输出到上游；schema 清理同时保留 enum 的 Allowed values 说明。 |
| 阿里、Vidu 与插件 | 阿里新增 Responses 地址与原 ASR/图像路由共存；插件固定渠道绑定合并新增 xAI。Vidu 保留 Q4 支持，但 metadata 不能覆盖已计价的 model/duration/resolution，避免预扣参数与实际请求分离。插件描述仍按 API v1 的计费主体+单价约定审查。 |
| 限流 | 保留 main 的并发成功请求预留、失败流释放与 429 错误；内存 bucket 同时按用户、已配置分组和 token_id 隔离。删去已无调用的旧 key 构造函数，增加同用户同组不同令牌回归。 |
| 前端组件 | 复用现有 Key drawer、MultiSelect、日志筛选/报文弹窗；保留 root Key 配置、账号模型限制与关联日志筛选。日志维护页采用 main 的 `HistoryLogCleanup`/`ServerLogFiles`，保留用户历史天数控制。日期组件保留整日起止模式，并合入秒级时间编辑。没有新建重复业务组件。 |
| 前端测试与依赖 | Drawer/通知页测试接入实际所需 Router/QueryClient provider；同时断言 main 新字段和本分支字段。Bun lock 由安装重新解析，保留本分支 lobehub/ui，采用 main 的 icons 版本约束。 |
| 国际化 | YAML 和后端常量取有效并集，保留分支旧消息；前端三方合并时 main 单独修改采用 main、本分支明确修改及独有键保留，再通过指定脚本补键与 i18n:sync。保留项目身份和版权文本。 |

自动合并未标记冲突的修复包括：重复日志导出路由；旧日志调用签名；缺失 fmt 导入/新 LogDebug 格式参数；自定义路由权限声明；ClickHouse 自定义日志列；日志 JSON 跨库类型与运算符、错误日志列表的 `SELECT error_logs.*` 绕过 `Omit(body)`（改为先查日志，再批量补充主库令牌名称）。Video 路由测试补齐本分支 ModelRouteConfig 表与 i18n 初始化，避免测试依赖其他文件的执行顺序。WebSocket 脱敏测试的代理失败改为受控拨号错误，仍验证真实选定的代理地址，避免依赖本机 DNS/代理网络的错误文本。

### 原始冲突文件清单

- common/：`common/gin.go`、`common/init.go`、`common/pprof.go`。
- controller/：`controller/access_token_audit_test.go`、`controller/checkin.go`、`controller/log.go`、`controller/misc.go`、`controller/performance.go`、`controller/relay.go`、`controller/relay_error_log_test.go`、`controller/responses_websocket.go`、`controller/token_test.go`、`controller/twofa.go`、`controller/user.go`、`controller/user_manage_test.go`。
- i18n/：`i18n/keys.go`、`i18n/locales/en.yaml`、`i18n/locales/zh-CN.yaml`、`i18n/locales/zh-TW.yaml`。
- middleware/：`middleware/model-rate-limit.go`。
- model/：`model/log.go`、`model/redemption.go`、`model/subscription.go`、`model/token.go`、`model/token_auto_groups_cache_test.go`、`model/token_cache.go`、`model/topup.go`、`model/usedata.go`、`model/user.go`。
- plugins/：`plugins/builtin_plugins_test.go`、`plugins/tasks/vidu/plugin.js`、`plugins/vidu_responses_test.go`。
- relay/：`relay/audio_handler.go`、`relay/channel/ali/adaptor.go`、`relay/channel/ali/adaptor_test.go`、`relay/channel/gemini/relay-gemini.go`、`relay/channel/moonshot/adaptor_test.go`、`relay/channel/openai/relay-openai.go`、`relay/common/relay_info.go`、`relay/compatible_handler.go`、`relay/constant/relay_mode.go`、`relay/constant/relay_mode_test.go`、`relay/gemini_handler.go`、`relay/helper/valid_request.go`、`relay/responses_handler.go`、`relay/responses_websocket.go`。
- relaykit/：`relaykit/dto/gemini.go`、`relaykit/dto/openai_request.go`、`relaykit/dto/user_settings.go`、`relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go`、`relaykit/relayconvert/internal/shared/gemini/request.go`、`relaykit/relayconvert/internal/shared/gemini/schema.go`。
- service/：`service/channel.go`、`service/log_info_generate.go`、`service/quota.go`、`service/relay_error.go`、`service/relay_error_test.go`、`service/task_billing.go`、`service/text_quota.go`、`service/text_quota_test.go`、`service/token_counter.go`。
- setting/：`setting/billing_setting/builtin_billing.go`。
- web/：`web/bun.lock`、`web/package.json`、`web/src/components/datetime-picker.tsx`、`web/src/features/channels/types.ts`、`web/src/features/keys/components/api-keys-mutate-drawer.tsx`、`web/src/features/keys/lib/__tests__/auto-group-form.test.ts`、`web/src/features/keys/types.ts`、`web/src/features/profile/__tests__/settings.test.tsx`、`web/src/features/profile/components/tabs/notification-tab.tsx`、`web/src/features/profile/lib/user-settings.ts`、`web/src/features/profile/types.ts`、`web/src/features/system-settings/maintenance/log-settings-section.tsx`、`web/src/features/system-settings/operations/index.tsx`、`web/src/features/system-settings/operations/section-registry.tsx`、`web/src/features/system-settings/types.ts`、`web/src/features/usage-logs/api.ts`、`web/src/features/usage-logs/components/__tests__/detail-preview.test.tsx`、`web/src/features/usage-logs/components/common-logs-filter-bar.tsx`、`web/src/features/usage-logs/components/dialogs/details-dialog.tsx`、`web/src/i18n/locales/en.json`、`web/src/i18n/locales/fr.json`、`web/src/i18n/locales/ja.json`、`web/src/i18n/locales/ru.json`、`web/src/i18n/locales/vi.json`、`web/src/i18n/locales/zh-TW.json`、`web/src/i18n/locales/zh.json`。
- 根目录：`go.mod`、`main.go`。

### 验证与限制

运行环境：Go **1.26.0 darwin/arm64**、Bun **1.3.1**。实机数据库：SQLite **3.50.4**、MySQL **8.0.46**、PostgreSQL **16.15**、ClickHouse **25.8.33.6**。MySQL/PostgreSQL/ClickHouse 使用本轮独立 Podman 容器与隔离测试数据库；未操作既有应用数据库，验证后移除本轮容器。

数据库命令的连接变量（口令仅用于本轮临时测试实例）：

```sh
export TEST_MYSQL_DSN='root:merge-test-only@tcp(127.0.0.1:33618)/new_api_merge?parseTime=true&clientFoundRows=true'
export AUDIT_MYSQL_DSN="$TEST_MYSQL_DSN"
export HAPPYOYSTER_TEST_MYSQL_DSN="$TEST_MYSQL_DSN"
export TEST_POSTGRES_DSN='postgres://postgres:merge-test-only@127.0.0.1:33619/new_api_merge?sslmode=disable'
export AUDIT_POSTGRES_DSN="$TEST_POSTGRES_DSN"
export HAPPYOYSTER_TEST_POSTGRES_DSN="$TEST_POSTGRES_DSN"
# 独立日志库的专项测试另外设置：
export AUDIT_CLICKHOUSE_DSN='clickhouse://merge_test:merge-test-only@127.0.0.1:33620/new_api_merge'
```

| 验证 | 实际命令与结果 |
|---|---|
| 根模块完整回归 | 设置上述 MySQL/PostgreSQL 六个变量，`go test ./... -count=1` 最终退出 0；60 个有测试的 package 通过。覆盖保护契约中的渠道/规则、计费、Gemini、AWS SDK 报文、Responses、HappyOyster、任务插件、视频直链、用户/MFA/会话和审计。全量日志保留于 `/tmp/new-api-merge-20261018/go-test-final.log`。 |
| 数据库启动、审计与独立日志库 | `go test ./controller -run 'TestLogExportDatabaseMatrix\|TestAuditDatabaseMatrix\|TestIndependentAuditLogStores' -count=1 -v` 中，`TestAuditDatabaseMatrix` 的三库 fresh/upgrade 和 `TestIndependentAuditLogStores` 的 MySQL/PostgreSQL/ClickHouse fresh/upgrade 均通过。启动至少重复两次，验证历史用户/日志、权限、凭证唯一性、审计 JSON/角色及 TTL 隔离。该批命令当时在日志导出 ClickHouse 列选择失败；修复后按下一行重新验证，不把该批次整体记为成功。 |
| 日志与令牌升级最终矩阵 | 设置全部七个连接变量后，`go test ./controller -run '^TestLogExportDatabaseMatrix$' -count=1 -v` 退出 0，四种引擎的 8 个 fresh/upgrade 子用例全部通过。升级 fixture 来自本地最新 release tag `v1.0.0-rc.45` 的 Log/Token 类型及 ClickHouse 建表语句；迁移后验证历史内容/额度、索引、令牌唯一性、默认 quota-query 开关，再重复迁移。另验证 extra/header 字符串与 NULL、消费/错误日志 JSON 筛选、列表不返回错误 body、root 完整报文与管理员清理、`RecordLog` 显式额度。日志：`/tmp/new-api-merge-20261018/db-logs-latest.log`。 |
| 构建及模块独立性 | 根目录 `GOWORK=off go build ./...`、`cd relaykit && GOWORK=off go build ./...`、`cd tokenkit && GOWORK=off go build ./...` 全部退出 0；`cd relaykit && GOWORK=off go test ./...` 通过。 |
| 前端类型及生产构建 | `cd web && bun run build:check` 通过，Rsbuild 2.1.6 构建完成；最终测试改动后再次 `bun run typecheck` 通过。 |
| 前端交互回归 | `bun run test channel-configuration api-keys-mutate-drawer api-key-listing channel-rules-editor-dialog group-filter filter-combobox log-type-filter detail-preview retry-chain-display settings.test.tsx sidebar-config special-channels datetime-picker log-settings export-dialog export.test.ts --maxWorkers=2`：24 个文件，336/337 用例首次通过，日志详情的一个语言切换用例超时。随后 `bun run test detail-preview --maxWorkers=1` 的 33 个用例全部通过；新增管理员流式地址断言后，`bun run test export-dialog --maxWorkers=1` 的 7 个用例全部通过。没有修改超时阈值或删除断言。 |
| Key/渠道固定编号 | `bun run test auto-group-form --maxWorkers=2` 的 13 个用例通过；`bun test src/features/channels/lib/__tests__/channel-type-ids.test.ts src/features/keys/lib/__tests__/channel-rules.test.ts` 的 22 个用例通过。 |
| 国际化、lint 与格式 | `bun run i18n:sync` 通过，七语言各 7938 个键并同步 zh-CN 镜像，临时补键脚本已删除。受影响 TS/TSX 文件 oxlint 无 error；保留一条已有 JSON 序列化 fixture 的 structuredClone 建议，因为测试明确验证 undefined 字段在传输中被省略。按项目版权保护方式检查修改文件格式通过；Vidu 插件的 oxlint 无 error（保留既有 caught-error cause warning）、oxfmt 检查通过。Go 修改文件已 gofmt。 |
| 合并状态 | 冲突标记扫描与 `git diff --check` / `git diff --cached --check` 通过；原 90 个冲突文件全部暂存解决，未创建 merge commit。 |

认证相关判断参考 [OWASP ASVS 5.0.0](https://owasp.org/projects/asvs)、[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 和 [Multifactor Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html)。核对的控制点是服务端角色/证明与资源边界、验证过的邮箱唯一性、会话撤销、旧令牌截止日期、新令牌授权失败关闭及审计不含可用凭证；对应回归随全量测试通过。本记录不声明整个应用已通过 ASVS 合规认证。

验证范围：升级测试使用发布版本的 schema fixture 和代表性数据，不是完整生产数据库副本恢复演练；未运行数据库最低支持版本或真实付费上游调用。ClickHouse 专项覆盖消费日志和审计日志，不宣称本轮扩展了它对所有其他日志模型的支持。测试中间失败及修复原因已记录，不将首次失败或跳过项计为通过。

## 2026-10-23 quota_data 历史合并回归追溯

- `37034124e`（2025-06-24）已为数据库增量更新加入 `prompt_tokens/completion_tokens`；合并前的 `2dc19ee6a` 中，`increaseQuotaData` 已完整更新 14 项计数，并按 UID、场景、项目、计划等完整维度匹配。
- `git diff 00a210716^1 00a210716 -- model/usedata.go` 明确显示：2026-09-19 的合并删除了 11 项累计和上述四个更新条件。对照第二父提交 `5caafd3d8`，合并结果采用了上游三项累计的实现，未保留分支扩展；这是合并造成的功能回退。
- 2026-10-19 的 `316ac12cc` 合并前，该缺陷已经存在；2026-10-22 的 `21b5080b3` 恢复完整累计和分组匹配。上述日期为 Git 提交日期，不代表线上部署时间。
- 当前持久化事件实现继续保留全部计数。现有回归 `testQuotaDataCounters` 对同一分组分别记录并落库两次，断言所有计数为两次之和，再验证退款只修改额度；由 `TestQuotaDataPersistenceDatabaseMatrix` 调用。
- 本次会话已执行 `go test ./model -run '^TestQuotaDataPersistenceDatabaseMatrix$/^sqlite$/^all_counters_and_refunds$' -count=1 -v`，在 SQLite **3.50.4** 通过。本次追溯仅补充记录，不改生产代码或数据库；没有重新执行 MySQL/PostgreSQL 矩阵，也未核实生产部署版本或回填历史数据。

## 2026-10-23 异步任务退款归属修复与合并保护

本节记录同日另一次实际修复及验证，不改变上一节历史追溯的验证范围。后续合并必须保留完整归属链路，不能将“钱包已退款”视为所有预算均已恢复。

### 故障与修复位置

- 本地请求 `toio20261021034353302932000iEgJWFRU` 的 `omnihuman-2.0` 任务预扣 **$21.000000**，成功后实际费用 **$0.777514**，差额退款 **$20.222486** 已回到账户钱包；原消费属于 UID `12345678a`，退款日志和统计却属于空 UID，导致该 UID 仍承担完整预扣额。
- `controller/relay.go` 的 `executeTaskSubmissionWith` 在任务插入前，从原请求保存 `Properties.TokenId/TokenName/ClientUserId/ClientScenairo/ProjectName/ProjectId/PlanId`。仅在 `LogTaskConsumption` 中读取 Context 并记录提交日志不足以支持异步结算。
- `service/task_billing.go` 的 `RefundTaskQuota`、`RecalculateTaskQuota` 必须继续从持久化任务取得原归属，传递到 `RecordTaskBillingLog`；轮询请求的账号、UID 或节点上下文不能替代提交快照。
- `model/log.go` 的 `RecordTaskBillingLog` 必须同时把 `ClientUserId/ClientScenairo/ProjectName/PlanId` 写入实际 `Log` 和统计事件。不得只恢复参数或结构体声明而漏掉赋值。
- 项目分配曾存在独立的更新缺陷：提交未增加 `project_allocations.used_quota`，退款却会减少；增减 SQL 未限定计划，可能改动同一项目、同一 UID 的多个计划。`LogTaskConsumption` 现按提交结算金额记录项目用量，`TrackProjectConsumption` 与异步差额结算都将已保存的 `PlanId` 传入更新函数，SQL 的正常更新及退款归零均匹配项目、计划和 UID。HappyOyster 使用同一模型函数，其调用也必须传入已保存的计划。
- `allocated_quota` 是美元，`used_quota` 和差额是内部额度单位。提交后记录的是已经扣除的费用，不能用旧的 `used_quota + delta <= allocated_quota` 判断漏记消费；最终补扣超过预算也必须如实记录。请求前仍由 `ValidateProjectRequest` 按统一单位校验剩余额度。立即完成的任务记录最终金额，立即失败记录零，后台差额不能再次增加请求次数。
- UID 非项目可用额度依赖原 UID 的月度非项目净消耗，退款必须以负值冲减原 UID；`cliend_user_quota.used_quota` 也需保持一致。项目与计划维度必须保留，项目支出不消耗个人预算。结算只调整额度，不再增加请求、token、缓存或耗时计数。

### 已修复的历史记录与重放边界

这笔本地记录已完成一次归属修正：补齐任务 `110` 的属性和退款日志 `1144` 的归属，将退款统计桶 `4041` 的负额度合并到原统计桶 `4040`，保留其完整桶标识及其他计数；最终净额度为 `388757`（**$0.777514**），请求次数仍为 `1`。UID 已用额度减少 `10111243`（**$20.222486**），并对对应月度消耗缓存执行失效操作。账户钱包与 API 令牌余额未再次调整；修复脚本重复执行未重复冲减。

上述请求未带项目；本次补充的项目／计划修复不自动回填其他历史 `project_allocations.used_quota`。这是一笔已完成的本地数据修复记录，不是后续部署时应自动重放的退款指令。代码修复不会自动补齐其他历史任务；若另行修复旧数据，须先用原消费日志与任务结算结果确定归属和金额，备份后在事务中校验旧值、完整桶身份及全部计数，处理桶合并和 UID 汇总，完成后使相关缓存失效。已经退还的钱包和令牌额度不得再次退款。

### 回归锚点与已执行验证

`controller/relay_task_plugin_test.go` 的 `TestTaskBillingAttributionDatabase` 经真实提交持久化后重新读出任务，在没有原 HTTP Context 的情况下验证个人/项目两类归属的差额退款、失败全额退款和补扣；检查钱包、令牌、UID 汇总、月度非项目消耗、实际日志、统计请求次数及重复结算。项目用例从已用量为零开始，并预置同一项目＋UID 的另一个计划，验证提交增记、差额退款、失败全退、补扣、超预算实际费用、重复结算、项目总用量和其他计划不变；另外覆盖退款归零、缺失计划及计划 0 的隔离。`TestImmediateTaskSettlementDatabase` 同时验证立即完成与立即失败时项目只记录最终金额。配套 `TestQuotaDataPersistenceDatabaseMatrix` 保护全部 14 项累计与 UID、场景、项目、计划隔离。

以下命令在本次修复阶段实际执行并通过。MySQL/PostgreSQL DSN 指向隔离的临时测试数据库，未对业务数据库运行测试迁移；测试后已清理临时数据库和 MySQL 容器。

从仓库根目录执行，MySQL/PostgreSQL 测试事先设置对应的 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN`：

```bash
TEST_TASK_DB_DIALECT=sqlite go test ./controller -run '^(TestTaskBillingAttributionDatabase|TestImmediateTaskSettlementDatabase|TestExecuteTaskSubmission.*)$' -count=1 -v
TEST_TASK_DB_DIALECT=mysql go test ./controller -run '^(TestTaskBillingAttributionDatabase|TestImmediateTaskSettlementDatabase|TestExecuteTaskSubmission.*)$' -count=1 -v
TEST_TASK_DB_DIALECT=postgres go test ./controller -run '^(TestTaskBillingAttributionDatabase|TestImmediateTaskSettlementDatabase|TestExecuteTaskSubmission.*)$' -count=1 -v
go test ./model -run '^TestQuotaDataPersistenceDatabaseMatrix$' -count=1 -v
```

前三条分别在 SQLite **3.50.4**、MySQL **8.4.13**、PostgreSQL **18.3** 上通过。最后一条同时设置两个 DSN，在上述三个实际引擎上全部通过，包含 `all counters and refunds`、`all dimensions`。

后续涉及该链路的合并必须重跑上述锚点，确认三种数据库均实际执行；缺少 DSN 导致的跳过不能视为矩阵通过。可按新架构调整实现位置，但不得删除这些可观察行为及其回归覆盖。

## 2026-10-26 渠道路由优先级回归追溯与修复

- 用户最终明确确认：大文件路由最高，其次是 Key 渠道规则、全局模型路由、系统默认规则。根据模型匹配到的 Key 候选仍按其组顺序、组内选择方式、竞速及重试计划执行；会话缓存不能提前拒绝这条计划、替换候选或改变组顺序。渠道状态、模型支持、路径及协议能力等必要约束仍须校验，固定渠道的身份约束也必须保留。
- `2d0605383`（2026-10-07，`同步代码`）的第一父提交 `16a13b853` 中，`middleware/distributor.go` 先执行 `if len(channelIds) > 0` 并选择显式计划的首个渠道；只有后面的 `else if shouldSelectChannel` 才读取会话绑定。即使缓存存在旧绑定，也不会覆盖已生成的显式计划。
- `git diff 2d0605383^1 2d0605383 -- middleware/distributor.go service/channel_select.go` 显示：合并删除了上述优先分支，改为统一调用 `SelectChannelForRequest`，并把候选传入 `RetryParam.ChannelIds`。新函数却在检查显式列表之前执行 `GetPreferredChannelByAffinity`；不可用的 strict 绑定直接返回 `strict_session_binding_unavailable`，可用绑定则可能成为列表外的首选渠道。原列表的生成、保存及重试计划仍在，但这不等于保留了原执行优先级。
- `316ac12cc`（2026-10-19）继续保留这条执行顺序；`17fc23c4d`（`use redis`）没有修改 `middleware/distributor.go` 或 `service/channel_select.go`。这些是 Git 提交事实，不代表已经核实线上部署版本或某个生产请求的实际缓存渠道。
- 修复前普通 HTTP relay 在进入实际调用后仍可能按显式列表重新选渠，但分发器中的 strict 错误发生得更早，无法由该逻辑补救；Responses WebSocket 使用同一 `SelectChannelForRequest`，也受影响。
- 修复 `middleware/distributor.go`：先采用大文件候选，否则按 Key 规则生成计划，只有没有可用计划且未固定渠道时才解析全局 enforce/fallback；只有最终采用的规则设置重试次数。大文件候选为空和有效 enforce 命中后候选为空仍直接失败；显式列表耗尽不能退回其他策略。全局 enforce 不再压过大文件或 Key 计划，也不负责判断附件大小。
- 修复 `service/channel_select.go`：存在显式 `RetryParam.ChannelIds` 时跳过默认会话亲和；无显式计划时保留原 strict/prefer 行为。Redis 是否开启不改变上述顺序，不需要以清理会话缓存替代优先级修复。
- 前端复用已有表单说明展示固定四级顺序，移除与新顺序冲突的 Key 高优先级开关，并更新大文件说明和七种界面语言；旧 `channel_rules_high_priority` 字段仍兼容读写，不再参与优先级计算。本次未引入新 UI 组件或交互封装。
- 在已有 `middleware/distributor_priority_test.go` 中增加 `TestChannelRoutingPriority`，覆盖旧绑定不可用、旧绑定可用但在列表外、旧绑定位于后续组、显式渠道与普通分组匹配条件不同，以及 order/random/race 的组顺序和重试边界。扩展大文件用例验证优先于兼容或不兼容的 Key/enforce 候选、无候选不回退、固定渠道和 base64 规则仍有效。扩展 `TestSelectResponsesWSChannelHonorsStrictSessionBinding` 覆盖显式计划与旧 strict 绑定并存，并保留默认会话用例。
- 验证命令：`go test ./middleware -run 'Test(ChannelRoutingPriority|EnforcedModelRouteFailsClosedWhenNoConfiguredChannelIsAvailable|FilterExplicitChannelCandidatesRequiresRequestedModelWhenEnabled|GeminiFileRoutingCannotBeBypassedAndRetiresLegacyRules)$' -count=1`；`go test ./service ./model ./relay ./controller -run '^(TestGetChannelRetryPlanBoundsExplicitRoutes|TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty|TestSelectChannelRuleGroupCandidates|TestFlattenChannelRuleGroups|TestChannelSupportsRequestedModel|TestModelRouteConfig.*|TestSelectResponsesWSChannel.*|TestShouldRetryHonorsPinRetryMode|TestSameChannelPinsMergeToStricterRetryMode|TestSelectRaceGroupChannelsStrategies|TestRaceProtocolAllowlist|TestRequestPolicyRoutingDatabaseMatrix)$' -count=1`。定向回归通过；路由集成使用 SQLite，未验证真实 MySQL/PostgreSQL。本次没有数据库结构、SQL 或驱动变更。
- 前端验证：在 `web/` 执行 `bun run test src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx src/features/model-route-config --maxWorkers=2`，4 个文件、35 个用例通过；`bun run typecheck`、`bun run build`、改动文件的 oxlint/oxfmt 检查通过。Key 抽屉测试已先验证旧开关导致失败，再修改实现。未修改线上配置或缓存，未部署线上。
