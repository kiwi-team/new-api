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
| Claude 请求禁止合成占位上下文（2026-11-18 用户明确批准） | 先前为真正空消息填 `...`、为非 user 起始历史补虚构 user，且将空工具结果改成 `...`。用户明确要求取消这三类行为，取代此前保留空消息兜底的约定。 | Chat Completions 和 Responses 转 Claude Messages 均不再合成占位文本或首条 user。先完成工具块组装，真正空消息、空对话及非 user 起始历史返回客户端参数错误；空工具结果与工具 ID 保留，真实文本与签名思考不变。不得在合并时恢复旧占位逻辑。 | `internal/oai_chat/to_claude_messages_req.go`、`internal/oai_responses/to_claude_messages_req.go`、`internal/shared/claude/messages.go:ValidateMessages`（均位于 `relaykit/relayconvert/`）；`TestConvertRequestToClaudeDoesNotFabricateContext`、`TestConvertRequestResponsesToClaudeMergesEmptyContentWithTools`、`TestOpenAIChatRequestToClaudeMessagesPreservesToolOnlyContent`、`TestGoldenRequestConversionMatrix`；保留 `reasoning.ClientError` 经宿主转换为 HTTP 400/不重试的处理。 |
| 飞书告警卡片（2026-11-17） | 用户要求按触发/持续/恢复、等级和对象区分彩色卡片，避免原纯文本中数字、场景和监控对象混杂。 | 保留统一飞书目标的原生卡片、状态图标、等级配色、渠道/模型分栏、正确单位与恢复阈值。通知使用不可变事件快照；用户名称及兼容正文为 plain_text；长消息满足大小限制。继续校验原生签名和平台业务码，不能回退为忽略业务失败或改变通用 Webhook 协议。 | `service/alerting/feishu.go`、`render.go`、`targets.go`；已有 `alerting_test.go` 的 `TestAlertFeishuCard*`、`TestAlertDeliveryProtocolsAndBusinessErrors`、`TestLegacyFeishuDeliveryProtocolsAndBusinessErrors`、`TestAlertNotification*` 与 `i18n/TestLocalesRenderEveryKey`。 |
| Root 跨用户配置告警通知（2026-11-17） | 创建个人规则时通知候选被规则属主过滤，Root 无法选择其他用户目标；首批候选无法覆盖超过 200 项的数据。 | 保留账号/Key 规则的归属用户搜索分页；Root 可查询并绑定任意用户的普通及升级目标，跨归属引用由前后端自动设为平台强制管理。普通用户保持属主隔离；平台监控范围仍 owner=0；停用/缺失目标与自动动作权限校验不得绕过。 | `ListAlertTargets`、`alerting.Validate`、`RuleEditor`、`useTargetOptions`；`testAlertRootTargets`、`testAlertRuleDeliveryRequirement` 的三数据库矩阵，`rule-editor.test.tsx` / `rule-form.test.ts` 与共享多选测试。 |
| 渠道模型级非流请求上游流式桥接（2026-11-16） | 用户要求将既有设计落地；客户端仍使用 Chat/Messages 非流式 JSON，规则属于渠道并按原始模型匹配。 | 保留 `setting.model_behaviors`、模型多选入口及全部语言；每次尝试重新判断，不能把 `stream=true`、失败的流状态或响应缓存泄漏到后续渠道。透传只改写 stream，参数覆盖后仍强制流式；完整聚合及转换前不得写客户端 SSE/心跳。保留原始计费用量、缓存和签名，成功只结算一次，日志按客户端非流口径。其他协议尤其 Responses WebSocket 的显式流式状态不得被该规则重置。 | `relay/buffered_stream.go`、`RelayInfo.InitChannelMeta`、`TextHelper`/`ClaudeHelper`、`relay/chat_completions_via_responses.go`、`BufferedStreamSetting`；`relay/buffered_stream_test.go` 的 `TestBuffered*`，`model/channel_settings_test.go` 的模型行为校验与三数据库配置往返，前端 `buffered-stream-setting.test.tsx`；支持边界、命令和验证限制见 `CHANNEL_MODEL_NON_STREAM_TO_STREAM_DESIGN.md` 第 15 节。 |
| Claude thinking 签名透传（2026-11-16） | 请求 `toio202611161040440884120001LbBuC7U` 的上游 thinking 块带有 7412 字符签名；非流式转换只复制 thinking，漏读 signature，最终 JSON 因 `omitempty` 丢失签名。 | Claude 原生响应转 Chat Completions 时，同一 thinking 块的推理内容和签名必须配对保留，签名原样写入 `choices[0].message.signature`；thinking 后接工具调用且文本为空时同样必须返回签名。无签名和纯文本响应继续省略该字段。流式 `signature_delta` 继续写入 `choices[*].delta.signature`，不得变成推理文本或换行。 | `relaykit/relayconvert/internal/claude_messages/to_oai_chat_resp.go` 的 `ResponseClaude2OpenAI`、`StreamResponseClaude2OpenAI`，共享 DTO 与实际 handler 调用链；同目录 `to_oai_chat_resp_signature_test.go` 的 `TestResponseClaude2OpenAIPreservesThinkingSignature`、`TestStreamResponseClaude2OpenAIPassesThroughSignature`、`TestStreamResponseClaude2OpenAIKeepsThinkingDelta`；下述签名合并复核列出命令。 |
| Gemini Omni 官方 token 计费与视频兼容渠道（2026-11-12） | 仅接入 Gemini/Vertex 原生计量时，OpenAI 兼容渠道仍进入 Sora 秒数计量，官方表达式读取 `u("input_tokens")` 得到 nil。原生任务转换成公开视频响应时也曾丢弃上游 `usage`，使级联下游无法精确结算。 | 未显式绑定插件的 Omni 在 Gemini/Vertex 走 Interactions，在 OpenAI/Sora 兼容渠道保留 `/v1/videos`，均按提交时冻结的输入、文本输出、思考和视频输出单价结算；不能用秒数替代实际 token，不能重复收取思考或视频原生音频费用。查询响应透传经过校验的官方用量；缺失或异常用量保留预扣并等待结算，显式零用量结算为零。显式插件绑定与普通 Sora/Veo 继续使用原协议和计量。保留签名视频直链及空白地址回退；退款和补扣仍只调整额度，不重复累计请求或 token。 | `relay/relay_adaptor.go`、`relay/channel/task/{gemini,vertex,sora}/`、`setting/billing_setting/builtin_billing.go`；`relay/relay_task_test.go` 的 `TestGeminiOmniOfficialUsageExpression`、`TestGeminiOmniOfficialUsageSettlement`、`TestGeminiOmniCompatibleSubmitAndUsageRoundTrip`、`TestGeminiOmniNativeSubmitFreezesOfficialExpression`；同时运行 `TestVideoTaskQueryPreservesUpstreamURLDatabaseMatrix`、`TestQuotaDataPersistenceDatabaseMatrix`、`TestTaskBillingAttributionDatabase`、`TestImmediateTaskSettlementDatabase` 的真实三库版本。 |
| 统一告警与渠道模型隔离（2026-11-05） | 请求只在终态统计会遗漏在途变慢；模型 A 的故障不能停掉同渠道正常模型 B。新采集、旧执行方和通知订阅若在合并时被绕过，可能重复处置或让已撤销凭证继续使用。 | 保留独立采集及真实开始/有效输出/终态语义，统计和日志关闭仍可采集；来源必须绑定当前 boot 和连续接受前缀，未知覆盖不得新增隔离或恢复。渠道、渠道模型和 Key 的有效 hold 参与全部新调用准入，人工停用与其他 hold 不被恢复覆盖。在途只通知。旧提醒以共享迁移标记确定唯一执行方，自然周期从明确边界接管，退款不回退最高档位；不得重新扣费或退款。通知目标撤销、凭证版本和精确操作重新认证必须在服务端执行。 | `common/alerting.go`、`service/alerting.go`、`service/alerting/`、`model/alert*.go`、`middleware/alerting.go`、`router/alert_router.go`、`controller/alerting*.go`；`TestAlertingDatabaseMatrix`（三库及真实 Redis）、`TestAlertLifecycleWithoutDebugTiming`、`TestAlertDeliveryProtocolsAndBusinessErrors`、`TestAlertMutationGuardsRunBeforeHandler`、`TestAlertProof*`、`TestProcessChannelErrorSendsFeishuAlerts`，以及下述 2026-11-05 记录中的资金回归和前端入口测试。 |
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
| Gemini 原生/Chat 单一大文件规则（2026-10-14 用户批准变更） | 原模态规则会使小文件按类型跨站，Body 透传和转换又可能跳过附件策略。 | 用户明确批准退役 `OnlyTextChannels`、`VideoChannels`、`OnlyImageChannels`、`NoVideoChannels`，页面和运行时仅保留 `LargeFileChannels`，不继承旧键。`GeminiFileRouting` 默认关闭；按 2026-10-15 用户确认，仅 URL 附件原始字节达到阈值才触发大文件约束；已有 base64 保持 inline，不按大小选路或上传，混合请求只上传大 URL，整个请求体限额继续生效。同日用户批准移除内容识别结果与 MIME 强制一致校验：优先使用声明 MIME，再用响应 MIME，均缺失时才从内容补全；保留媒体分类范围及 base64 校验。固定、Key 高优先级、强制路由、affinity 和重试不得越界；实际 Vertex 凭据绑定 GCS 上传，预扣成功后执行，上传对象按逻辑请求清理，推理提交后不自动重放。保留原生/Chat JSON/SSE、usage、PDF/audio 来源映射、透传检查、请求脱敏和 relaykit 独立性。2026-10-28 用户确认：纯 inlineData/Base64 请求的上游失败使用普通错误解析，保留具体上游消息、错误码和状态码映射，不得统一替换成 `file_inference_failed`；包含 URL、GCS 或 provider 文件引用的请求仍保留错误脱敏，包括未达到大文件阈值的引用及混合请求。 | `service/gemini_file_plan.go`、`service/gemini_file_execute.go`、`middleware/distributor.go`、`model/channel_constraint.go`、Gemini/Chat handler、共享转换器、特殊渠道页面；`TestGeminiInlineFilesStayInlineAndPreserveChatConversion`、`TestGeminiFileMIMEPreservesDeclaredType`、`TestGeminiFileURLMIMEFallback`、`TestGeminiFileURLThreshold`、`TestGeminiInlineFilesStillRespectBodyLimit`、`TestGeminiFileGCSLifecycleAndPermissionDomain`、`TestGeminiFilePassthroughAndOverrides`、`TestGeminiFileRoutingCannotBeBypassedAndRetiresLegacyRules`、`TestGeminiFileChannelConstraints`、`TestGeminiFileRoutingOptionsDatabaseMatrix`、`TestGeminiFilePlanActualAdaptorJSONAndSSE`、`TestGeminiFileEffectiveGenerationBounds`、`TestGeminiInlineFilesPreserveUpstreamErrors`（原生/Chat、JSON/SSE、具体错误与状态码映射、传输失败及远程/混合引用脱敏）、`special-channels.test.ts`、`special-channels-tab.test.tsx`。 |
| Gemini 工具结果嵌套附件（2026-11-02 用户确认） | 文件路由在渠道选择前一律拒绝 `functionResponse.parts`，使官方支持的多模态工具结果连内联图片也无法提交。 | 原生 Gemini 的 `functionResponse.parts[]` 与 `function_response.parts[]` 附件纳入文件计划，保留消息和工具结果顺序、`id`、thought signature、`response` 引用及媒体 `displayName`。内联数据保持 inline，嵌套文本附件支持 `text/plain`；URL 继续按阈值 inline、转发或按实际 Vertex 凭据上传 GCS。普通序列化与 Body 透传均回写到原嵌套位置，并保留 SSRF、base64/MIME 校验、请求体限额、参数覆盖检查、远程引用脱敏与上传清理。 | `service/gemini_file_plan.go`、`service/gemini_file_execute.go`、`relaykit/dto/gemini.go`；`TestGeminiFileFunctionResponseAttachments`、`TestGeminiFileFunctionResponseURLRouting`、`TestGeminiFilePlanRejectsInvalidOrUnsupportedAttachments`、`TestGeminiFileGCSLifecycleAndPermissionDomain`，以及既有 Gemini 原生/Chat JSON/SSE 和上游错误回归。 |
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
| 可灵 Omni 原生渠道协议（2026-10-28 用户确认） | 渠道类型 `50` 被无条件交给 Kling 任务插件，覆盖了原有 Omni Go 适配器；插件未实现 Omni 的 `contents/settings/options`、提交及查询端点。同日真实任务 `task_3oy96xSeQmS0erKu5mxE3Ws7TsbUVmZ0` 暴露另一处解析回归：官方查询返回 `data[]`，旧实现仅识别 `data.result[]`，HTTP 200 连续 20 次解析失败后把上游已成功的任务标失败并退款。 | 原可灵渠道按映射后的 Omni 模型选择 Go 适配器，保留数字平台 `50`；提交走 `/omni-video/{model}`，轮询走 `/tasks?task_ids=`，结果转换继续使用真实上游 ID。查询统一兼容 `data[]` 与 `data.result[]`，状态探测、轮询解析和视频地址/实际时长提取共用同一解码逻辑；回归测试必须包含官方数组形式，不能仅用网关包装形式。保留无参考视频有声、有参考视频无声、4K/1080p、时长与清晰度倍率和渠道模型别名。历史任务缺少上游模型名时回退原始模型名；持久化的显式插件执行快照仍按其原插件协议处理，普通 Kling 插件任务不受影响。解析修复不自动恢复已退款任务，历史恢复须单独对账，不能重复退款。 | `relay/relay_adaptor.go`、`relay/channel/task/kling/adaptor.go`、`relay/channel/task/kling/omni.go`；`TestKlingOmniTasksKeepNativeChannelWithPluginEnabled`、`TestKlingOmniPollingPreservesStoredTaskProtocol`、`TestKlingOmniQueryArrayResponse`（成功、处理中、上游失败及视频结果读取）、`TestGetTaskAdaptorMapsMigratedPlatformsToFactoryPlugins`，并运行下行 Gemini Omni 锚点，防止共用分流逻辑回退。命令：`go test ./relay ./relay/channel/task/kling ./plugins`。 |
| Gemini Omni 与 Veo 任务协议（2026-10-15 回归） | 按渠道类型无条件选择 Google/Vertex Veo 插件，覆盖了现有 Omni Interactions 实现；Vertex 合法的空 Base URL 被插件通用校验拒绝，即使填写地址也会误发 predictLongRunning。 | 未被显式插件绑定的 Gemini/Vertex `gemini-omni*` 请求按映射后的模型选择原生 Interactions；Vertex 继续由项目和区域生成默认地址。原生 Omni 保留数字平台及已有上游 ID 编码，轮询、结果读取和结算根据上游 ID 与插件执行快照区分 Omni/Veo，不能把公开 task_id 当作协议标识。显式绑定的插件和 Veo 仍使用插件，不放宽通用插件 URL 校验。 | `relay/relay_adaptor.go`、`relay/relay_task.go`、`relay/channel/task/gemini/adaptor.go`、`relay/channel/task/vertex/adaptor.go`；`TestGoogleOmniTasksKeepInteractionsWithVeoPluginsEnabled`、`TestGoogleOmniPollingUsesInteractionEndpoint`、`TestGetTaskAdaptorForRequestUsesExactPinnedPlugin`、`TestRelayTaskSubmitMapsBeforeValidateWhenOriginSet`。 |
| 级联视频任务直链（2026-10-15 回归） | OpenAI/Sora 任务迁移到插件后，解析器漏读上游 `video_url` / `result_url` / `url`；查询又把本机 HTTPS `/content` 兜底地址当作已转存结果，国内站始终无法返回海外已有的 S3 地址。 | 成功任务按既有优先级读取并保存上游直链（含签名参数），仅上游没有直链时使用本机内容代理。已完成但仅存本机代理地址的任务允许查询补取，已有直链则不再回源；补取失败、过期或返回旧状态不能撤销成功态或重复结算、退款。 | `plugins/tasks/sora/plugin.js`、`relay/relay_task.go`；`TestVideoTaskQueryPreservesUpstreamURLDatabaseMatrix`（SQLite/MySQL/PostgreSQL）、`TestTryRealtimeFetchDoesNotDoubleRefund`。 |
| `/v1/responses` 请求/响应日志 | handler 已捕获请求和完整响应，但 HTTP/WebSocket 共用结算函数把两个参数替换为空串，普通 Responses 日志因此没有原始报文。 | `SAVE_REQUEST_RESPONSE=true` 时，普通及音频 HTTP `/v1/responses` 必须把已捕获请求、响应传入消费日志；`/v1/responses/compact` 保持独立计价日志路径。WebSocket 没有 HTTP body 记录器时不得伪造报文。 | `relay/responses_handler.go`、`relay/responses_websocket.go`；`TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse`。 |
| 管理员创建用户的原子性与 Leader 角色 | 旧的 `Insert` 与 main 新的 `InsertWithTx` 同时保留，一次请求重复插入用户并因主键冲突失败；main 的新测试还把 prod 的 Leader 角色 `5` 当成非法值。 | 创建用户只在同一个事务中写入用户和管理员权限，成功后再执行缓存/邀请奖励收尾；Leader 角色 `5` 继续是合法角色，未定义角色必须在写库前拒绝。 | `controller/user.go`、`common/constants.go`；`TestAdminUserRoutineEditsDoNotRequireProof`、`TestCreateUserRejectsNonStandardRole`。 |

| 日志导出协议与权限（2026-10-18） | 两侧同时注册 `GET /api/log/export`，Gin 启动 panic；直接接受 main 的管理员导出会泄露本分支仅 root 可见的渠道/原始报文。 | 保留 root CSV `/api/log/export`；管理员 NDJSON 使用 `/api/log/export/stream`，用户使用 `/api/log/self/export`。列表、流式导出共用过滤器，保留 UID/组织范围及用户历史天数；普通管理员清除 request/response/usage/header 和渠道链路，root 保留完整数据。 | `router/api-router.go`、`controller/log.go`、`model/log.go`、日志导出前端 API；`TestLogExportDatabaseMatrix`、`TestLogExportRules`、`TestAccessTokenRouteRulesCoverEveryDashboardRoute`。 |
| 结构化日志与额度（2026-10-18） | main 把日志内容从字符串改为 `common.Message`，直接覆盖会丢失本分支显式额度、Realtime 分模态用量和 TTS 字符说明。 | 内容使用固定英文键与参数，并在七语言前端翻译；`RecordLog` 保留可选显式额度；音频/文本共用结算入口保留 HTTP 报文；Realtime 保留真实分模态用量，不以日志重构改变扣费。 | `model/log.go`、`service/quota.go`、`service/text_quota.go`、`controller/task_video.go`、`controller/happyoyster.go`；`TestLogExportDatabaseMatrix`、`TestTiered*`、`TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse`。 |
| 日志 JSON 跨库（2026-10-18） | `extra/header` 固定 `jsonb` 无法在 MySQL 建表，PG 专用 JSON 运算符不能直接用于所有日志库；ClickHouse 建表漏掉分支日志列。 | PostgreSQL 保留原 jsonb 列；SQLite 使用 text，MySQL 使用 longtext，ClickHouse 使用 Nullable(String)。JSON API 仍输出字符串，空值仍为 NULL；consume/error 列表和导出共用按方言的精确 JSON 筛选。ClickHouse 已有 logs 表以幂等 ADD COLUMN 补齐分支字段。 | `model/log.go`、`model/error_log.go`、`model/main.go`；`TestLogExportDatabaseMatrix` 的 fresh/upgrade、报文/JSON 筛选及重复迁移用例。 |
| 内存限流令牌隔离（2026-10-18） | main 将限流主体改为用户/分组并改用成功请求预留，可能覆盖本分支同用户不同 Key 的独立额度。 | 内存主体同时包含用户、已配置独立额度的分组和 token_id；保留 main 的成功请求预留、失败流释放及标准 429 响应；Redis 保持既有用户/分组粒度。 | `middleware/model-rate-limit.go`；`TestModelMemoryRateLimitIsolatesTokensWithinConfiguredGroup`、`TestModelMemoryRateLimitReservesConcurrentSuccessAdmission`、`TestModelRateLimitCountsConfiguredGroupsSeparately`。 |
| 自定义管理路由的个人令牌边界（2026-10-18） | main 新增全路由权限声明检查，自定义管理路由未声明；简单开放任意 token 会扩大新权限令牌授权，改为 session-only 又会中断旧令牌。 | 自定义管理路由显式声明 legacy 规则：浏览器会话不变，旧令牌仍受原截止日期和角色校验，新 scoped token 拒绝；不能用 `any` 规则代替未定义的授权。 | `middleware/access_token_routes.go`、`middleware/auth.go`；`TestUserAuthAppliesAccessTokenRouteRules`、`TestAccessTokenRouteRulesCoverEveryDashboardRoute`。 |

### Gemini 文件请求数据库原文（2026-11-04 用户确认）

- 用户明确要求数据库中的 request 保存原始数据、不脱敏。对上表 Gemini 文件请求的请求脱敏约束作此变更：启用 `SAVE_REQUEST_RESPONSE=true` 时，原生 Gemini 和 Chat 文件请求的 `logs.request` 必须保留客户端原始 JSON，包括正文、工具结果中的嵌套附件、Base64、完整 URL 和未建模字段；不得替换为附件摘要或经过渠道转换、参数覆盖后的请求体。
- `error_logs.body` 同样保留已缓存的原始 JSON，继续遵循错误日志开关和 `includeBody`，不得为了记录鉴权/过载拒绝而额外读取未校验的请求体。普通 debug 输出仍可使用附件摘要；上游错误信息、请求头和凭据的既有脱敏边界不变。
- 实现锚点：`relay/gemini_handler.go`、`relay/compatible_handler.go`、`service/request_error_log.go`。回归集中在 `relay/gemini_file_error_test.go` 的 `TestGeminiFileRequestLoggingDatabaseMatrix`，覆盖原生/Chat、JSON/SSE、Body 透传、嵌套图片、URL、参数覆盖、未知字段、错误请求及关闭记录；主库与日志库分别连接真实 SQLite/MySQL/PostgreSQL。
- 实测发现 MySQL 的原 `logs.request` 为最大 64 KiB 的 TEXT，`error_logs.body` 因空串 default 被 GORM 推断为 VARCHAR(191)。移除这两个字段的类型/默认值限制，使 MySQL 使用 LONGTEXT，PostgreSQL/SQLite 使用 TEXT；保持 Go 字段类型和 JSON 接口为 string。测试请求包含超过 64 KiB 的 Base64 图片，验证完整落库。
- 三库实测：SQLite **3.50.4**、MySQL **8.4.13**、PostgreSQL **16.15**。每种数据库均验证 fresh、从本地最新发布 tag `v1.0.0-rc.47` 的 logs schema 升级（该 tag 尚无 request/error_logs），以及本分支变更前 TEXT/VARCHAR schema 升级；连续两次日志迁移，第二次不重复 ALTER，保留既有请求正文、错误正文、消费记录和索引列/唯一性。MySQL/PostgreSQL 的主库使用 `gemini_logs`，独立日志库使用 `gemini_logs_separate`，均为临时测试容器。
- 最终命令（事先设置真实测试实例的 `TEST_MYSQL_DSN`、`TEST_MYSQL_LOG_DSN`、`TEST_POSTGRES_DSN`、`TEST_POSTGRES_LOG_DSN`）：`QUOTA_DATA_SPOOL_DIR="$(mktemp -d /tmp/new-api-gemini-log-final.XXXXXX)" GIN_MODE=release go test ./model ./service ./controller ./relay ./relay/channel/gemini -run '^(TestGemini.*|TestRequestErrorLoggingDatabase|TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata|TestExplicitChannelRoute.*|TestLog.*|TestFormatUserLogs.*|TestLegacyLog.*|TestClickHouseLog.*)$' -count=1 -v`。五个包全部通过，新增矩阵 90 个 HTTP/落库用例通过；测试输出在 `/tmp/new-api-gemini-log-final.log`。未对生产数据库执行迁移。
- 已经只存附件摘要的历史记录不能通过本次代码变更恢复原文。

### 显式渠道列表的故障切换（2026-11-04 用户确认）

- 全局模型路由、大文件处理渠道列表和 Key 渠道列表生成的显式候选计划，应独立于系统默认选路的 HTTP 重试状态码规则。当前渠道返回 400、408、504、524 或其他失败状态时，只要仍有候选与重试次数，就继续尝试计划内下一渠道；不得在列表耗尽后回到系统默认渠道，也不得忽略零重试配置。
- 保留固定渠道身份、严格会话、请求校验失败、禁止重试标记、无效响应体及任务已受理等限制。默认选路继续使用 `AutomaticRetryStatusCodes` 和原有 504/524 排除规则。HTTP 响应开始输出后不能切换渠道；客户端 WebSocket 握手完成不代表上游请求已输出，上游握手失败仍可按显式计划切换。
- 对上表 Gemini 文件请求“推理提交后不自动重放”的约束作如下明确例外：显式列表内收到失败 HTTP 响应后，原生 Gemini 和 Chat 两条 handler 均解除该次提交屏障，保留错误脱敏并允许下一候选。传输失败导致执行结果未知、成功 HTTP 响应之后的处理失败，以及已受理的异步任务，仍不得自动重放。上传对象继续按逻辑请求和实际渠道凭据管理及清理。
- 实现锚点：`service/relay_error.go`、`service/gemini_file_execute.go`、`relay/gemini_handler.go`、`relay/compatible_handler.go`、`controller/relay.go`。回归集中在 `controller/channel_pin_retry_test.go` 的 `TestExplicitChannelRoutesIgnoreRetryStatusCodes` 和 `TestExplicitChannelRouteRetriesUpstreamRejections`，覆盖状态码配置为空、次数边界、固定/会话限制、已输出响应、任务已受理，以及原生/Chat 的无附件、inline 和远程引用实际 A→B 请求、列表全失败及传输结果未知。
- 后续受影响合并至少运行：`go test ./service ./controller ./relay ./middleware ./setting/operation_setting -count=1`，同时保留 `TestGetChannelRetryPlanBoundsExplicitRoutes`、`TestChannelRoutingPriority`、`TestGeminiFileRoutingCannotBeBypassedAndRetiresLegacyRules` 和 `TestGeminiFileSubmittedAttemptStopsRetryAndRecordsFailure`。
- 本轮验证：`QUOTA_DATA_SPOOL_DIR="$(mktemp -d /tmp/new-api-route-suite.XXXXXX)" go test ./service ./controller ./relay ./middleware ./setting/operation_setting -skip '^TestTaskBillingAttributionDatabase$' -count=1` 五个包通过；任务归因锚点在独立 spool 目录运行 `go test ./controller -run '^TestTaskBillingAttributionDatabase$' -count=1` 通过（SQLite 3.50.4）。最终代码的 `go test ./controller ./relay ./service -run 'TestExplicitChannelRoute|TestGeminiInlineFilesPreserveUpstreamErrors|TestGeminiFileSubmittedAttemptStopsRetryAndRecordsFailure' -count=1` 通过。
- 整组运行仍有既有测试隔离问题：先前用例写入共享 quota spool 的事件被 `TestTaskBillingAttributionDatabase` 回放，使其按用户查询到其他模型的统计桶；在干净 spool 目录排除本次新增用例运行 `go test ./controller -skip '^TestExplicitChannelRoute' -count=1` 同样复现。新增 HTTP 回归已独立 spool，并关闭其无关的统计导出及批量记账，未修改任务归因断言或生产记账逻辑。

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

### Claude thinking signature 合并复核（2026-11-16）

- 故障证据：请求 `toio202611161040440884120001LbBuC7U`，响应 `msg_011CfWT0X6ccGs92nCd82Ios`。原始内容为带签名的 thinking 和 `exec` 工具调用，文本为空。修复前使用该原始响应复现：上游签名长 7412 字符，转换后签名长度为 0，最终 JSON 不含 `signature`；推理内容与工具调用仍正常返回。此处记录已确认的字段遗漏，不推断其由某次历史合并引入，也不保存签名正文。
- 修复契约：`ResponseClaude2OpenAI` 从与 `thinkingContent` 相同的 thinking 块读取 `thinkingSignature`，写入 `choice.Message.Signature`。签名作为不透明字符串逐字保留，不截断、不解码、不混入 `reasoning_content`；带工具调用、空文本的响应同样适用。无签名的 thinking 和纯文本继续省略 `signature`。保留流式 `signature_delta` → `choices[*].delta.signature` 的现有透传。
- 实现与调用链：`relaykit/relayconvert/internal/claude_messages/to_oai_chat_resp.go`、`relaykit/relayconvert/response_compat.go`、`relaykit/relayconvert/response_registry.go:convertClaudeMessagesResponseToOAIChat`、`relay/channel/claude/relay-claude.go:HandleClaudeResponseData`；AWS SDK 的非流式 Claude 路径也复用该 handler。保留 `relaykit/dto/claude.go` 的 `ClaudeMediaMessage.Signature`、`relaykit/dto/openai_request.go` 的 `Message.Signature` 和流式 DTO 的签名字段。后续迁移必须核对实际 JSON/SSE 调用链，不能只保留 DTO 或未被调用的转换函数；不得因 main 缺少实现而删除此行为。
- 回归锚点：现有 `to_oai_chat_resp_signature_test.go` 中的 `TestResponseClaude2OpenAIPreservesThinkingSignature` 覆盖签名 thinking＋工具调用、签名 thinking＋文本、无签名 thinking、纯文本，并断言序列化后的 `choices[0].message`；同时保留 `TestStreamResponseClaude2OpenAIPassesThroughSignature` 和 `TestStreamResponseClaude2OpenAIKeepsThinkingDelta`。非流式用例已先在旧实现上因缺少签名失败，再通过修复。
- 本次修复验证：用户提供的原始响应经修复后序列化，7412 字符签名与上游逐字一致，推理内容和工具调用均保留；`relaykit/` 内 `GOWORK=off go test ./...` 与 `GOWORK=off go build ./...` 均通过。签名修复不改变 usage 或内部计费快照。
- 请求回放补充（2026-11-17）：用户提供的 `/v1/chat/completions` 请求在 assistant 消息中同时回传 `content: null`、`tool_calls`、55 字符 `reasoning_content` 和 1876 字符 `signature`，但转换后的 Anthropic 请求只有占位文本和 `tool_use`。`to_claude_messages_req.go` 原先仅在纯字符串且 `ToolCalls == nil` 的分支恢复 thinking 块，工具调用与数组内容分支均遗漏此字段；不推断其由哪次历史合并引入，不保存签名正文。
- 请求侧契约与回归：`OpenAIChatRequestToClaudeMessages` 必须将 assistant 的非空推理内容及其签名配对还原为 `content[]` 中的 `thinking` 块，并置于文本和 `tool_use` 之前；纯文本、数组内容，以及带工具调用的 null/空文本均适用。签名逐字保留，不为已有签名思考的空文本插入 `...`。现有 `to_claude_messages_req_signature_test.go` 的 `TestOpenAIChatRequestToClaudeMessagesRestoresSignedThinkingBlock` 通过实际序列化覆盖上述分支，同时校验工具参数和 `tool_result`；`TestOpenAIChatRequestToClaudeMessagesKeepsStringContentWithoutSignature` 保留无签名消息的行为。
- 请求侧修复验证：新增分支用例在旧实现上失败，修复后通过。原始请求经宿主 `claude.Adaptor.ConvertOpenAIRequest`、`RemoveDisabledFields` 和 `WriteJson` 后，55 字符推理及 1876 字符签名均逐字一致，内容块为 `thinking`、`tool_use`。`relaykit/` 内 `GOWORK=off go test ./...`、`GOWORK=off go build ./...`，以及根目录 `go test ./relay/channel/claude -run '^(TestConvert.*|TestBuildOpenAIStyleUsageFromClaudeUsage.*|TestClaudeStreamFinalUsageEstimatesOnlyUncountedUsage)$' -count=1` 均通过。

- 纯工具调用初步修复（2026-11-18，后续策略已更新）：最初修复了 Chat Completions 的空文本＋工具调用，以及 Responses 的空消息随后合并工具块时多插入 `...` 的问题；当时保留了真正空消息和起始 user 的占位兜底。对应原始回归先在旧实现上失败，修复后通过，并更新 `openai_to_claude.golden.json`。这项历史记录不再授权保留占位兜底，当前合同以下述用户明确批准的最终策略为准。
- 用户批准取消三类占位（2026-11-18）：用户明确要求去掉“真正空消息补 `...`”“首条不是 user 时补虚构 user”“空工具返回值改成 `...`”三类行为。Chat Completions 与 Responses 的相关合成代码均删除，包括只含 system/instructions 时制造 user 的逻辑；不会删除客户端真实的 `...`。空工具结果保留为空并保留工具调用 ID，不丢弃 `tool_result` 块。
- 当前实现：两条路径均在工具块组装后调用 `internal/shared/claude/messages.go:ValidateMessages`；无消息、真正空消息及首条不是 user 的历史以 `reasoning.ClientError` 明确失败，由宿主已有错误处理返回 HTTP 400 并跳过重试。Responses 的既有尾部 assistant prefill 处理保留；空 assistant 随后合并普通/自定义工具调用，以及空 user 随后合并工具结果，仍能正常转换。此处仅改变这三类合成行为，不撤销已受保护的签名和工具调用内容。
- 当前回归：`request_registry_test.go:TestConvertRequestToClaudeDoesNotFabricateContext` 在两条实际注册转换路径验证空对话、null/空字符串/空数组消息、空工具列表、system-only、assistant/tool-call 起始错误，以及空字符串/null/空数组工具结果；新增用例先在旧实现上失败。`TestConvertRequestResponsesToClaudeMergesEmptyContentWithTools` 取代旧名 `TestConvertRequestResponsesToClaudeDefersEmptyContentFallback`，保留工具块合并和真实省略号文本覆盖。原先断言空消息生成占位文本的用例由新客户端错误用例替代，不再固定旧兜底策略。
- 最终验证（2026-11-18）：`relaykit/` 内 `GOWORK=off go test ./...`、`GOWORK=off go build ./...` 均通过；新参数/空结果矩阵覆盖 12 种情况×两条路径，原有签名、工具合并与 golden 测试同时通过。仓库根目录 `go test ./relay/channel/claude -run '^TestConvert' -count=1`、`go test ./relay -run '^TestUnknownModelModifierIsBadRequestWithoutRetry$' -count=1` 通过，后者复核同一 typed client error 分支的 HTTP 400/不重试映射。两份目标请求转换器已无 `"..."` 合成代码；客户端实际输入的省略号仍由序列化回归覆盖。`git diff --check` 通过。

后续涉及上述 DTO、转换器或调用链的合并必须在 `relaykit/` 目录运行：

```sh
GOWORK=off go test ./relayconvert/internal/claude_messages -run '^(TestResponseClaude2OpenAIPreservesThinkingSignature|TestStreamResponseClaude2OpenAIPassesThroughSignature|TestStreamResponseClaude2OpenAIKeepsThinkingDelta)$' -count=1
GOWORK=off go test ./relayconvert/internal/oai_chat -run '^TestOpenAIChatRequestToClaudeMessages(RestoresSignedThinkingBlock|KeepsStringContentWithoutSignature|PreservesToolOnlyContent)$' -count=1
GOWORK=off go test ./relayconvert -run '^(TestGoldenRequestConversionMatrix|TestConvertRequestResponsesToClaudeMergesEmptyContentWithTools|TestConvertRequestToClaudeDoesNotFabricateContext)$' -count=1
GOWORK=off go build ./...
```

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

## 2026-11-05 统一告警实现与合并保护

- 采集放在宿主层；计费提交事实沿用稳定事件 ID，统计开启时与原 14 项统计累计在同一事务桥接，统计关闭时走独立入口。真实逻辑请求和实际渠道尝试分别记录；重试、竞速 loser、恢复探测和 WebSocket 会话不能混成生产最终请求。来源协议 v2 绑定实例的确切 boot，缓冲进度、缺失序号、失联或时钟跳变不能被新心跳掩盖。
- SQL 保存权威事实、状态、执行租约和路由控制；独立 Redis Stream、PEL 接管、SQL fallback、本机同步 spool、outbox 和版本缓存保留部署命名空间与边界。不得把已有统计 Stream 直接增加消费组后仍随另一消费者 `XDEL`，不得裁剪未接受输入来消除积压。坏输入隔离保留证据并阻止“完整”判断。
- 模型隔离复用完整路由身份，人工状态、父渠道状态、各有效 hold 和最终准入检查保持独立。Root 显式切到 scoped 后，旧单次错误保护只能作用确定的模型，不能扩大为整渠道；旧整体保护、多密钥和人工状态仍保留。恢复使用独立样本、明确延迟上限、连续次数/观察时长及当前凭证摘要，不能通过三次小探测宣称全负载恢复。
- Token 累计、账号档位/日报、Key 自然小时/日、UID/项目预算与原余额/渠道通知均有兼容导入和单一投递执行方。事件适配保留原阈值、冷却、UID/项目/计划和既有检查点，不把提醒重新当成计费或退款。自然周期旧标记不含阈值时保持 ambiguous，从下一边界接管。切换/回退与旧发送共享主库租约；未完成投递须排空，旧设置不能修改已接管的身份。
- UI 继续复用共享 Dialog、ConfirmDialog、StaticDataTable、MultiSelect、表单和权限校验。账号、渠道及 Key 入口携带稳定 ID，显示实时名称；任何归属参数都不能扩大后端授权。七种界面语言与后端通知语言保持分工。
- 验证采用隔离空库：SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15，以及官方旧镜像报告的 MySQL 5.7.8-rc；Redis 8.10.2 使用专属本机 socket。已有用户/令牌升级夹具来自本地发布标签 v1.0.0-rc.47 的模型，迁移两次并核对额度、计数与凭证唯一性；这不是完整生产库升级演练。未修改 ORM/驱动版本，未操作业务数据库。
- 后续合并命令（DSN 必须指向隔离空库，Redis 使用独立测试实例）：`ALERTING_TEST_MYSQL_DSN=... ALERTING_TEST_POSTGRES_DSN=... ALERTING_TEST_REDIS_SOCKET=... go test ./service/alerting -count=1 -v`；最低 MySQL 单独运行 `go test ./service/alerting -run TestAlertingDatabaseMatrix/mysql -count=1 -v`；`go test -race ./service/alerting`；`go test ./controller ./service ./relay ./relay/channel/volcengine ./middleware ./router ./common -count=1`。必须同时运行 `TestQuotaDataPersistenceDatabaseMatrix` 和 `TestTaskBillingAttributionDatabase` 的真实三库版本，不能以告警测试替代资金保护。
- 批量导入保护：`controller/alerting_migrations.go` 的每批 1–10 项限制、Root 校验、逐项结果与稳定迁移身份不能绕过原导入租约/事务，也不能自动切换执行方。`TestAlertingDatabaseMatrix` 的 `batch migration is retryable and preserves legacy delivery` 在真实三库验证重复导入、失败隔离、提醒进度与资金/Key 状态不变；前端 `migration-import.test.tsx` 验证配置筛选、确认、分批与失败结果。
- 前端锚点：`web/src/features/alerts/components/__tests__/rule-editor.test.tsx`、`related-alerts-link.test.tsx`、`web/src/features/alerts/lib/__tests__/rule-form.test.ts`、Key 的现有测试，以及账号 step-up、渠道表格回归。执行 `bun run typecheck`、定向 oxlint/oxfmt 与 `bun run build`。Key 的 `channel-rules.test.ts` 使用 Node runner，单独 `bun test`，其余使用 Vitest。
- 权限变更按 OWASP ASVS 5.0.0、Authentication、Session Management 和 SSRF Cheat Sheets 的相关控制核对：服务端归属/角色、PAT 降权、Origin/body 限制、精确单次操作 proof、主库凭证撤销、目标加密与 DNS/连接时校验、撤销订阅后的投递重新授权。不得把本次定向检查描述为完整 ASVS 认证。没有使用真实通知目标或生产上游发送测试消息，也未部署。
- 完整 `go vet` 仍受本次未修改的 `controller/cliend_user_quota.go` 和 `service/token_counter.go` 的既有 unreachable code 影响；相关包可用 `go vet -unreachable=false` 复核其他检查。原始事实的长期归档、生产容量和一分钟目标按技术文档第 16 节记录，不以通过单元测试或构建代替现场验收。

## 2026-11-13 告警周期、恢复与旧飞书投递修复

- 旧 `SendFeishuNotifyContext` 必须使用飞书机器人正文中的秒级 `timestamp` 和 `sign`，不能用通用 Webhook 签名头替代。HTTP 2xx 仍须校验业务 `code`（兼容旧 `StatusCode`）；错误码、空响应、非法 JSON 和超过 64 KiB 的响应均为发送失败，调用方不能推进告警检查点。通用 Webhook 的原签名协议保持不变。
- `once_per_period` 按已接受指标窗口所属的自然小时、日或月去重，不能按执行时钟把上一周期尚未推进的数据当作新周期消费。`once` 和 `once_per_period` 只抑制重复触发，不能跳过未确认提醒和升级；确认或关闭后停止升级。每次恢复和新故障都重新建立恢复计时，不能继承历史 `RecoveryAt`。Realtime 查询必须包含 `base_url`，以便自定义上游的样本匹配渠道配置版本，并识别运行期间的地址变更。
- 回归集中在 `service/alerting/alerting_test.go`：`TestLegacyFeishuDeliveryProtocolsAndBusinessErrors`，以及 `TestAlertingDatabaseMatrix` 的 `natural periods follow accepted input`、`recurrent incidents require fresh recovery`、`one-shot incidents retain acknowledgement escalation`、`realtime custom endpoints retain metric coverage`。这些用例已先在修复前验证失败。旧渠道通知测试的成功响应夹具使用真实飞书成功协议。
- 真实隔离实例验证通过：SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15、Redis 8.10.2。设置隔离库的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN` 和 `ALERTING_TEST_REDIS_SOCKET` 后执行 `go test -race ./service/alerting -count=1 -v`；设置 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 后执行 `go test ./model -run '^TestQuotaDataPersistenceDatabaseMatrix$' -count=1 -v`；分别使用 `TEST_TASK_DB_DIALECT=mysql` / `postgres` 执行 `go test ./controller -run '^TestTaskBillingAttributionDatabase$' -count=1 -v`，SQLite 任务归属由 controller 全量测试覆盖。告警矩阵沿用发布版本用户/令牌升级夹具及重复迁移检查，未变更模式、驱动或计费累计实现。
- `go build ./...` 和 `go vet -unreachable=false ./service/alerting ./service` 通过。`go test ./service ./controller ./relay ./middleware ./router ./common -count=1` 中除 router 外均通过。router 的既存 `TestAccessTokenRouteRulesCoverEveryDashboardRoute` 报告 `POST /api/alerts/migrations/batch` 缺少权限声明；以本次生产改动之前的文件构造临时 Go overlay，单独执行该用例仍同样失败。本次未修改该权限流程。

### 旧告警迁移跳过网络校验（用户明确要求）

- 导入既有 Token、账号消费/日报、Key 小时/日及旧通知订阅时，只读取并加密复制原地址与凭据，离线生成脱敏地址提示，不做 DNS、IP 网段、连通性或新目标地址策略检查。内网、Fake-IP、暂不可解析的域名和原有 HTTP/自定义端口地址不能因此阻断迁移。导入例外只存在于服务端旧配置导入入口，不增加可由客户端提交的跳过校验字段；非 `ENV=dev` 环境下，新建或替换通知地址及实际投递仍执行原网络校验。开发环境例外见下文。
- 原归属/Root 限制、迁移租约、稳定身份、重复导入幂等、凭据加密、检查点和单一执行方契约继续保留；导入不发送消息，不自动切换执行方，也不修改资金或 Key 状态。
- 回归沿用 `TestAlertingDatabaseMatrix` 中的批量导入、账号档位/日报、Key 自然周期、旧事件适配和通知目标管理用例：覆盖内网与保留地址导入、不可用 DNS 下的凭据完整复制、失败项隔离和重复导入，并验证新建目标及实际投递的校验仍生效。新增预期已先在旧实现上验证失败。
- 本次验证通过：SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15 和独立 Redis 8.10.2。设置隔离实例的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET` 后执行 `go test -race ./service/alerting -count=1 -v`，三库和 Redis 用例均执行、无跳过；`go vet -unreachable=false ./service/alerting`、`go build ./...` 与 `git diff --check` 通过。未更改数据库模式、驱动或既有计费检查点语义。

## 2026-11-14 告警规则归属用户搜索

- 规则编辑器的归属用户使用可搜索列表，显示显示名称、账号与 ID，通过既有管理员用户接口在完整用户集合上搜索，再分页返回；不能改回仅加载前 200 人后在浏览器过滤。每页 50 人，支持继续加载。用户名、显示名称及数字 ID 均支持部分匹配；`SearchUsers` 保留原精确数字 ID 匹配、筛选条件与敏感字段排除，ID 文本转换使用 MySQL/SQLite 的 `CHAR` 和 PostgreSQL 的 `TEXT`。
- 已有规则按确切用户 ID 单独回显名称，不要求用户出现在当前页或当前搜索结果中；切换归属清空原用户的 Key、通知/升级目标、订阅和旧预览。兼容迁移规则仍只读，非平台管理者不显示跨用户选择入口。
- 用户选择器现位于 `web/src/features/users/components/user-select.tsx`（`UserSelect`），由告警和创建 API Key 共用。它只组合业务数据，复用共享 `ui/combobox`、`ErrorState`、`LoadingState` 和 `useDebounce`；已有 `combobox-input` 的本地过滤/自定义输入不能满足远程搜索分页，因此使用共享 Combobox 的组合接口，关闭其重复本地过滤。保留搜索防抖、错误重试、空态、键盘选择及详情加载期间的已选名称，未添加通用下拉替代组件。
- 回归在已有 `model/user_pagination_test.go` 与告警 `rule-editor.test.tsx`：真实 205 人数据验证显示名称、精确/部分 ID、筛选与分页；前端覆盖远程搜索、跨 200 人加载、编辑回显、迁移只读、键盘及失败重试。新增交互和 ID 模糊匹配预期均已先验证旧实现失败。
- 隔离三库验证通过：SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15。设置 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 后运行 `go test ./model -run '^(TestSearchUsersDatabaseMatrix|TestGetAllUsersSortsBeforePagination|TestSearchUsersSortsBeforePagination)$' -count=1 -v`，三库均执行；`go test ./model -count=1` 与 `go build ./...` 通过。`web/` 下 `bun run test src/features/alerts/components/__tests__ src/features/alerts/lib/__tests__`（25 用例）、`bun run typecheck`、改动文件的 oxlint/oxfmt 检查、`bun run build` 和 `bun run i18n:sync` 通过，新文案覆盖七种界面语言。

### 创建 API Key 的用户选择（同日扩展）

- 创建 Key 的“为用户创建”复用上述远程搜索和分页，每页 50 人，支持显示名称、账号和部分 ID，超过 200 人时仍搜索完整用户集合。“我的账号”为独立选项，搜索或加载失败不改变已选归属；切回此选项后提交不带 `user_id`，指定用户仍发送确切 ID。归属选择继续只向 Root 的新建界面提供，编辑和普通用户的行为不变。
- 回归扩展已有 `api-keys-mutate-drawer.test.tsx`，覆盖第 205 人的名称和部分 ID 查询、跨 200 人分页、无匹配结果、切回本人、加载失败与重试，以及实际创建请求的归属。测试卸载组件后再恢复请求 mock，避免清空缓存时触发真实网络请求。
- 已检查 [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 和当前稳定版 ASVS 5.0.0；本次仅改用户选择与读取，沿用管理员用户查询接口、服务端 `AddToken` 的 Root/本人权限验证及既有会话机制，不增加凭据读取或授权绕过入口。`go test ./controller -run '^TestAddTokenOwnerAuthorization$' -count=1` 通过，覆盖 Root 代建、本人创建、普通用户跨用户拒绝和不存在用户拒绝。
- `web/` 下 `bun run test src/features/keys/components/__tests__ src/features/keys/lib/__tests__/auto-group-form.test.ts src/features/alerts/components/__tests__ src/features/alerts/lib/__tests__`（101 用例）、`bun test src/features/keys/lib/__tests__/channel-rules.test.ts`（15 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run build`、`bun run i18n:sync` 通过。渠道规则旧测试使用 `node:test`，单独通过 Bun 运行。本次复用已有翻译键，未修改数据库行为。

### 本地开发跳过通知目标网络校验（同日，用户明确要求）

- 仅进程环境变量精确为 `ENV=dev` 时，通知目标的保存及投递前检查跳过 DNS 预解析和目标 IP 网络访问规则，实际 HTTP 连接也使用普通拨号，不再拒绝 Clash Fake-IP、内网及回环地址。只跳过前置解析，实际连接仍需要正常解析和连通性；保留 HTTPS/443、URL 格式、类型、必填凭据、TLS 证书验证、超时和禁止重定向。
- 未设置 `ENV` 或值为 `production`、`DEV`、`development` 等时，继续检查全部 A/AAAA 地址，并在连接时固定已验证 IP。客户端请求不能控制该例外。`.env.example` 记录本地开关；实际 `.env` 已是 `ENV=dev`。
- 回归沿用 `service/alerting/alerting_test.go`，新增 `TestAlertTargetDevelopmentNetworkValidation`：覆盖精确环境值、内网/元数据/回环/Fake-IP 的 IPv4/IPv6、连接阶段策略、不可用 DNS 下的校验和投递，以及开发模式下的 URL/凭据限制。连接测试使用取消的上下文，避免真实访问内网或绑定固定端口。生产安全与数据库矩阵用例显式设置环境，避免依赖开发者 shell 的 `ENV`。新增开发模式预期已先在旧实现上验证失败。
- 验证通过：定向 `go test -race ./service/alerting -run '^(TestAlertTargetDevelopmentNetworkValidation|TestAlertTargetCredentialBinding|TestAlertTargetValidationResponse|TestAlertDeliveryProtocolsAndBusinessErrors)$' -count=1 -v`、`ENV=dev go test -race ./service/alerting -count=1 -v`、`go build ./...`、`go vet -unreachable=false ./service/alerting` 与 `git diff --check`。整体回归本轮执行 SQLite 3.50.4；未配置独立 MySQL/PostgreSQL/Redis 测试实例，对应用例跳过。本次不修改数据库模式、查询、驱动或计费逻辑。

### 通知目标测试冷却提示（同日）

- 同一通知目标仍限制每 60 秒测试一次。`controller/alerting_operations.go` 的原子更新未命中时，重新读取当前目标的版本与测试时间：版本一致且处于冷却期返回 HTTP 429 和 `Retry-After` 剩余秒数，提示“同一通知目标每 60 秒只能测试一次，请稍后重试。”；真实版本变更继续返回 HTTP 409 和原刷新提示。冷却请求不得延长冷却时间或重复创建测试事件、任务与审计记录。
- 回归扩展已有 `TestAlertingDatabaseMatrix`，覆盖冷却过期后接受测试、连续测试限频、拒绝请求不延长冷却/重复入队，以及读取目标后发生并发测试或并发编辑时的不同响应。并发变更使用查询后的受控数据库更新模拟，不使用 sleep 或时序竞争。原实现已复现 HTTP 409 与预期 HTTP 429 不符。新增提示通过翻译脚本和 `i18n:sync` 覆盖七种界面语言。
- 本次新读取查询及原事务在真实 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15 上均验证通过。设置隔离实例的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET`（独立 Redis 8.10.2）后执行 `ENV=dev go test -race ./service/alerting -count=1 -v`，三库和 Redis 均执行、无跳过；`go build ./...`、`go vet -unreachable=false ./controller ./service/alerting` 通过。前端 `bun run test src/lib/__tests__/server-error-notifications.test.ts src/features/alerts/components/__tests__ src/features/alerts/lib/__tests__`（46 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run i18n:sync`、`bun run build` 通过。未修改数据库模式、驱动或生产数据。

### 渠道余额通知迁移缺少旧地址（同日）

- `channel_arrears/site` 从旧配置 `feishu_qianfei_webhook_url` 读取渠道余额告警地址，与新通知目标及 UID/慢错误地址分别保存。本地该项缺失，导致没有通知目标的规则被通用校验拒绝。本次按用户此前指定的飞书机器人地址补齐这一项本地配置，在旧通知租约下写入并检查接管后的配置保护；迁移预览确认 `DestinationPresent=true`。地址凭据未写入仓库或日志。
- `ImportLegacyNotice` 在没有任何目标时返回 `migration_destination_missing`，HTTP 409 提示“请先配置旧告警的通知地址，再执行迁移。”，覆盖七种界面语言。保留原始缺项拒绝行为及 UID 预算仅配置邮件的兼容路径；已配置的旧地址仍离线加密导入，不增加网络校验或自动接管。
- 在已有 `TestAlertingDatabaseMatrix` 中增加渠道余额迁移用例，复现旧实现的笼统 HTTP 400；覆盖地址缺失/空白时的明确提示和不落迁移/目标，配置后通过原 HTTP 导入入口导入 Fake-IP 地址及签名凭据，保留 `legacy/observing` 执行状态，重复导入复用同一规则和目标。
- 验证通过：独立 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15 和 Redis 8.10.2。设置隔离实例的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET` 后运行 `ENV=dev go test -race ./service/alerting -count=1 -v`，三库和 Redis 用例均执行、无跳过。`go build ./...`、`go vet -unreachable=false ./controller ./service/alerting`、前端告警及错误提示回归（46 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run i18n:sync` 和 `git diff --check` 通过。数据库模式与驱动未修改。

### 告警条件字段分组（同日）

- 规则编辑器复用 `ui/field` 的 `FieldSet`、`FieldLegend`、`FieldGroup`：指标、比较运算符和阈值在“告警条件”区域相邻排列，窄屏下指标独占一行，运算符与阈值同排；持续时长、恢复阈值及指标特有字段置于其下。窗口、聚合方式、样本数、协议、流式筛选和时区归入“统计设置”，避免无关字段穿插比较条件。比较运算符复用已有专用翻译键，不能使用表示操作人的 `Operator`。
- 已有 `rule-editor.test.tsx` 增加分组、键盘焦点顺序及实际预览参数回归：调整比较符、阈值、恢复阈值、窗口、聚合和样本数后，验证提交值正确；保留迁移规则只读与预览失效用例。新增分组预期先在旧布局上验证失败。`bun run test src/features/alerts/components/__tests__/rule-editor.test.tsx`（11 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run build`、`bun run i18n:sync` 和 `git diff --check` 通过；两项分组标题覆盖七种界面语言。

### 迁移规则的空列表回显（同日）

- Go 的 nil slice 会将 `actions`、通知目标、升级目标和模型列表编码为 `null`，空的对象/渠道 ID 选择器还可能被省略。前端 `RuleSpec` 类型必须反映这些响应值；`ruleDefaults` 在读取动作、映射通知/升级目标时使用空列表语义，并保留已配置的动作、目标、模型及对象 ID。不能恢复为直接读取 `actions[0]` 或对可能为空的响应字段直接调用 `map`。
- 预览结果同样可能含 `actions:null`，保存时按无动作处理；启用且确有动作的规则仍走原二次验证条件。修复仅发生在前端回显与空值判断，不需要改写已有规则数据或更改服务端 JSON 合同。
- 复用已有表单及编辑器测试：`rule-form.test.ts` 覆盖 null 集合、省略选择器及完整配置往返；`rule-editor.test.tsx` 覆盖兼容账号通知、账号日报、账号消费里程碑、Key 消费里程碑四类响应的打开、修改、预览及保存，并模拟服务端预览返回空动作。旧实现先复现 `null.map`、`null[0]`，随后单独复现保存阶段的空动作访问错误。
- `web/` 下 `bun run test src/features/alerts/components/__tests__ src/features/alerts/lib/__tests__`（33 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run build` 和 `git diff --check` 通过。

### 本地模板规则初始化记录（同日）

- 按用户要求检查六类模板的实际规则，按归属、监控类型、指标、触发方式及在途截止类型匹配，而非仅比较名称。本地 Root 的 API Key 消费档位（规则 1）及账号每日汇总（规则 6）已存在，保留原配置和启用状态。
- 在本地 PostgreSQL 18.3 补建平台规则 17–20：渠道模型延迟、渠道模型失败率、等待首个输出、输出停滞。配置复用前端 `ruleTemplates`、`ruleDefaults`、`buildRule`，经 `alerting.SaveRule` 校验和持久化；归属平台（owner 0）、创建人为 Root（用户 1）、全部暂停且无自动动作，通知目标复用本地平台飞书目标 16。监控范围为当前 44 个可用渠道，模型选择为 `all_configured` 且包含这些渠道后续新增的模型；新增渠道需要另行选择。
- 初始化在归属锁和事务内执行。回读确认四条规则均为 revision 1、各有一条版本记录、没有投递任务，原有规则的配置、启用状态和版本保持一致；再次执行创建数为 0，六类均匹配已有记录。这是已完成的本地数据初始化记录，不是应用启动时应自动重放的种子逻辑。平台规则需切换告警中心顶部“我的告警”至“全部账号”视图查看。

## 2026-11-15 告警比例阈值与聚合方式

- 成功率、失败率、超时率、慢请求率及预算剩余比例的阈值与恢复阈值，在表单内按百分比输入并显示 `%` 后缀；`80` 表示 80%，取值 0～100，最多两位小数。前端提交时转换为整数基点，编辑旧规则时从基点还原，服务端和持久化单位保持不变。指标默认值及失败率模板同步使用百分比，不能只修改标签而漏掉保存或回显换算。
- 规则编辑器复用 `ui/input-group` 和 `ui/field` 展示单位与字段错误；请求量、金额等非比例指标不显示 `%`。聚合方式仅对实际使用聚合计算的 `duration_ms`、`ttft_ms` 显示，其他指标隐藏。
- 已有表单和编辑器测试覆盖五类比例的读写往返、0/100 边界、小数精度、越界拒绝、旧渠道规则回显与预览、模板阈值保持，以及指标切换时的单位和聚合显示。新增预期先在旧实现中复现 11 个失败；`bun run test src/features/alerts --reporter=dot`（52 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run build`、七种语言翻译与别名校验、`bun run i18n:sync` 和 `git diff --check` 通过。
- 比例指标的异常生命周期规则还须在前端校验恢复阈值与告警阈值的关系，与既有服务端校验一致：`lt` 使用恢复阈值 ≥ 告警阈值，`lte` 使用 >，`gt` 使用 ≤，`gte` 使用 <。例如成功率 < 100% 时不能用 99% 作为恢复阈值，否则两种条件重叠。字段下显示具体关系错误，不能一律显示百分比取值范围错误；一次性规则不增加恢复关系限制。
- 关联校验在既有测试文件中先复现 5 个失败，覆盖四种比较符的边界，以及 100%/99% 阻止预览、改为 100%/100% 后提交整数基点的交互。`bun run test src/features/alerts --reporter=dot`（62 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run build`、`bun run i18n:sync`、七种语言与别名校验、`git diff --check` 通过；此次只补前端校验，服务端限制未放宽。

### 通知目标缺项的明确提示（同日）

- 既有服务端规则要求 `notify.target_ids` 和 `actions` 至少一项非空；渠道成功率 95%/恢复 99% 合法，但两项均为空仍必须拒绝。该分支改为保留 `errors.Is(err, ErrInvalid)` 语义的专用错误，HTTP 400 返回七种语言可翻译的明确原因及 `field: notify.target_ids`，不能通过允许无通知、无动作的规则绕过校验。
- 前端在通知目标字段下显示同一要求；在途规则始终仅通知，不能用残留但不会提交的动作值满足校验。复用既有 `MultiSelect`、`ui/field`，仅兼容扩展多选组件的 `aria-invalid`、`aria-describedby` 传递，保留搜索和多选交互。切换归属用户后仍清除原通知目标，必须重新选择该用户可用的目标。
- 在已有 `service/alerting/alerting_test.go` 中通过真实预览处理器复现渠道 49、95%/99%、空目标的 HTTP 400，验证明确提示、补选平台通知目标后 HTTP 200，以及保留纯自动动作规则。新增前端预期和旧接口提示均先在旧实现中复现失败。完整隔离矩阵验证 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15、Redis 8.10.2：设置独立空库的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN` 和独立 Redis 的 `ALERTING_TEST_REDIS_SOCKET` 后执行 `ENV=dev go test -race ./service/alerting -count=1 -v`，全部通过、无跳过；重复迁移和发布版本升级夹具沿用原矩阵。测试容器和 Redis 实例已清理。
- `web/` 下执行 `bun run test src/features/alerts src/components/multi-select/__tests__ --reporter=dot`（82 用例）、`bun run typecheck`、改动文件 oxlint/oxfmt、`bun run i18n:sync`、`bun run build` 均通过，随后根目录 `go build ./...` 通过；后端嵌入前端产物，不能与清理产物的前端构建并行。七种语言与中文别名、保护版权头、`git diff --check` 均已校验。

### 平台规则的通知目标归属（同日）

> 2026-11-17 用户明确要求 Root 可选择所有用户创建的通知目标及升级通知目标，下面历史记录中的“Root 只能选择平台目标”限制已被替代。当前合同是普通用户仅限自身目标，Root 可跨用户引用且跨归属规则自动由平台强制管理；具体回归见文末“Root 跨用户配置通知目标”。平台监控对象的 owner=0 和自动动作授权边界仍需保留。

- 本地渠道 49 的规则归属平台（owner 0），通知目标 6 属于用户 1，目标 16 属于平台。创建规则时表单内部仍可能保存当前用户 owner 1；若使用它筛选渠道规则的通知目标，会错误展示并提交个人目标，随后被服务端拒绝。前端统一根据监控范围推导实际规则归属，渠道、渠道模型、系统、UID 预算和项目预算五类规则只选择平台目标；普通通知和升级通知使用同一策略，预览、回放、保存都提交实际归属。
- 切换个人/平台监控范围时清理原目标与升级目标；模板及渠道入口同样使用实际归属。保留 Root 强制管理的个人规则引用平台目标的既有能力，取消强制管理时移除不再可用的目标。预览、回放、保存前检查已选目标，避免旧选择、删除或停用状态导致笼统错误；暂停规则保留服务端允许引用停用目标的语义。
- 服务端保持原属主授权边界，分别标记 `notify.target_ids` 和 `acknowledgement.escalation_target_ids` 的错误。不存在与越权目标统一 HTTP 403 和相同静态提示，不返回资源元数据；属于规则但已停用的目标在启用规则时返回明确 HTTP 400。错误仍保留 `ErrForbidden`/`ErrInvalid` 分类，数据库故障不伪装成目标权限错误。登录、JWT、Session 以及自动动作的二次验证条件保持原实现。
- 在已有后端矩阵中验证预览、回放、新建、编辑四个真实 HTTP 入口：平台目标成功；Root 自己的个人目标、其他用户目标、缺失目标均拒绝；升级目标使用相同边界；停用目标给出字段提示。前端先复现 7 个失败，覆盖五类范围切换、创建到保存全流程、旧选择拒绝，并补齐模板、渠道入口及 Root 强制个人规则的回归。
- 完整验证通过：SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15、Redis 8.10.2。设置隔离空库的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN` 和独立 `ALERTING_TEST_REDIS_SOCKET` 后运行 `ENV=dev go test -race ./service/alerting -count=1 -v`，无失败、无跳过，沿用发布版本升级与重复迁移夹具；测试实例已清理。`bun run test src/features/alerts src/components/multi-select/__tests__ --reporter=dot`（100 用例），编辑器最终定向回归（30 用例），`bun run typecheck`、改动文件 oxlint/oxfmt、七种语言与中文别名校验、`bun run i18n:sync`、`bun run build`、随后 `go build ./...` 和 `git diff --check` 均通过。
- 已查阅 [OWASP ASVS 5.0.0](https://asvs.owasp.org/)、[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 及 [Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)。本次适用控制为服务端逐请求属主授权、默认拒绝、资源不可枚举及静态拒绝提示；验证范围为上述告警接口，不作整站 ASVS 合规声明。

### 告警通知的百分比与时区显示（同日）

- 共用 `RenderNotification` 将成功率、失败率、超时率、慢请求率和预算剩余比例的基点值转换为百分比正文；9500 显示为 95%，1 显示为 0.01%，使用十进制换算保留精度。比例恢复通知显示恢复阈值；请求量、耗时、金额等指标保留各自单位。持久化证据和规则条件仍使用原始单位，不能通过更改通知显示改变告警判定。
- 英文、简体中文、繁体中文的比例指标名称移除基点/万分比说明；窗口时间使用规则实际配置的时区和 RFC3339 时差，避免 UTC 时间旁标注 Asia/Shanghai。缺省或无法识别的时区使用 UTC，并同步显示 UTC 标签。补齐既有告警语言键在 `i18n/keys.go` 中的声明，保留三语言完整性检查。
- 在已有 `service/alerting/alerting_test.go` 中先复现 9 个失败，覆盖用户事件的三语言正文、五类比例、小数和 0/100 边界、恢复阈值，以及非比例单位保持。`go test -race ./service/alerting -run '^TestAlert(NotificationPercentageAndTimezone|DeliveryProtocolsAndBusinessErrors)$' -count=1 -v`、`go test ./i18n -count=1`、`go build ./...`、gofmt 与 `git diff --check` 均通过。本次为通知渲染与语言目录变更，无数据库模式、查询或规则持久化变更。

## 2026-11-16 usapi-gemini 本地监控与投递验证

- 按用户要求在本地 PostgreSQL 为渠道 49（usapi-gemini）创建 16 条平台规则（22–37），名称以 `[本地验证]` 开头，通知目标为平台飞书目标 17。全部经 `alerting.SaveRule` 的 Root 属主校验、归属锁、版本和审计流程创建；不增加启动时自动重放的种子逻辑。既有规则 21 保持 revision 3 和启用状态。
- 常规 13 条规则使用 60 秒滚动窗口、最少 1 个有效样本、0 秒持续满足时间，并设为 `once`，每条本轮只发一条消息；恢复通知和确认升级关闭。首包等待、输出停滞、在途总时长三条使用 10 秒截止、至少 1 个超限请求、0 秒持续时间，按系统约束使用 `incident`，持续异常的重复间隔为 60 秒。规则均仅通知，`actions` 为空。耗时阈值和慢请求界限为 1 ms，属于快速本地验证参数。

| 规则 ID | 范围 | 条件 | 验证事件 ID |
| --- | --- | --- | --- |
| 22 | 渠道 | 成功率 < 100% | 55 |
| 23 | 渠道 | 失败率 > 0% | 56 |
| 24 | 渠道 | 请求量 ≥ 1 | 48 |
| 25 | 渠道 | 超时次数 ≥ 1 | 57 |
| 26 | 渠道 | 超时率 > 0% | 58 |
| 27 | 渠道 | 连续失败次数 ≥ 1 | 59 |
| 28 | 渠道 | 请求耗时 p95 > 1 ms | 49 |
| 29 | 渠道 | 慢请求次数 ≥ 1 | 50 |
| 30 | 渠道 | 慢请求比例 > 0% | 51 |
| 31 | gemini-3.8-flash | 成功率 < 100% | 60 |
| 32 | gemini-3.8-flash | 失败率 > 0% | 61 |
| 33 | gemini-3.8-flash | 请求耗时 p95 > 1 ms | 52 |
| 34 | gemini-3.8-flash | 首包耗时 p95 > 1 ms | 53 |
| 35 | gemini-3.8-flash | 10 秒仍无首个有效输出 | 62 |
| 36 | gemini-3.8-flash | 输出停滞 10 秒 | 47 |
| 37 | gemini-3.8-flash | 请求在途超过 10 秒 | 46 |

- 通过实际 `/v1/chat/completions` 请求验证，沿用用户提供的 API Key、模型和 UID，使用既有管理员渠道固定机制确保请求命中渠道 49；输出上限收小到 64 token。原配置请求返回网关 `500/do_request_failed`。为稳定覆盖首包、输出停滞和超时，仅对带本轮测试标记的请求使用临时本地上游代理，返回原生 Gemini SSE、延迟首包/后续输出，以及受控 504；普通请求转发原上游。这里的受控故障是告警链路验证证据，不代表真实提供方发生了同样的超时或停滞。
- 初次检查发现旧进程因笔记本休眠触发 `clock_changed`，暂停了评估/发送并把样本标为 unsupported。通过 Air 重启本地进程建立新的健康 boot 后重新开始本轮规则验证，没有修改时钟保护或把旧 unsupported 样本改成 production。35 的首包等待验证将受控首包延迟增加到 35 秒，避开采集水位与评估周期边界；已验证的 36/37 临时暂停以避免额外通知，结束后恢复启用。
- 2026-11-16 09:50:52–09:54:27（Asia/Shanghai），上述 16 条规则各有且仅有 1 条飞书通知任务，全部 `succeeded`，每条均为 1 次发送尝试。最终回读确认 16 条规则全部启用；原上游地址已按原值恢复。恢复后的固定渠道请求生成 production/failure 样本，其配置版本与最初原配置请求一致，且不再经过临时代理；随后关闭代理并清理临时程序和配置备份。
- 后续重做常规一次性验证可暂停再启用对应规则以建立新的判定周期，仍需等待 60 秒统计窗口预热。三条在途规则继续按异常生命周期运行。此记录为已完成的本地配置与真实飞书投递回执，不涉及数据库模式、计费实现或生产渠道配置变更。


### 2026-11-16：告警规则列表完整性、分页与筛选

- `controller.ListAlertRules` 在显式传入 `page` 时返回 `data: {items, total, page, page_size}`，未传 `page` 的旧调用仍返回数组并支持 `before_id`。支持服务端名称（忽略大小写、按字面匹配）或精确 ID 搜索、启用/暂停、监控对象、指标、P0–P3 级别及管理方式组合筛选；筛选先于分页，不能退回前端过滤第一页。页大小 1–200，页码越界回到最后有效页，空集保留第 1 页。
- 配置存储仍为 TEXT；配置字段通过只读 ID/config 的 200 条批次筛选，最终只查询当前页对应行与运行状态，避免数据库方言 JSON 运算符或损坏的旧配置中断整个列表。不引入 schema、迁移或规则写入行为变化。
- `RulesList` 复用 `RulesTable`/`StaticDataTable`、`useDataTable`、`DataTablePagination` 和 `NativeSelect`。默认每页 20 条，可切换页大小、跳页及查看总数；筛选变化重置页码，保留账号/渠道/Key 关联范围，错误显示重试而非空列表。规则名称下显示 ID。详情权限读取对应规则，不依赖当前列表页。
- 有 `catalog.read_all` 权限时默认全部账号，避免平台 owner=0 的规则被“我的告警”隐藏；普通用户不发送 `scope=all`。切换账号范围仍重置分页，服务端原所有权校验同时约束列表和总数。新增 UI 文案通过 i18n 技能脚本补齐七种语言并执行 `bun run i18n:sync`。
- 回归锚点：`service/alerting/alerting_test.go::testAlertRuleList`（纳入已有数据库矩阵）；`web/src/features/alerts/components/__tests__/rules-list.test.tsx`。先验证原接口因数组响应无法解析分页数据而失败，修复后覆盖 205 条记录完整翻页、末页配置组合筛选、字面百分号、ID、空集、非法参数、旧游标兼容、外部 owner 拒绝与 Root 总数；UI 覆盖超过 200 条后的页面访问、页大小切换、筛选重置、范围保留、错误重试、默认全部/个人边界。
- 实测数据库：SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15，MySQL/PostgreSQL 与 Redis 均为独立临时实例。设置 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET` 后运行 `ENV=dev go test -race ./service/alerting -count=1 -v`：全部通过，无跳过（27.866s）。未使用本地业务数据库执行测试或改写业务规则。
- 前端验证命令：在 `web/` 运行 `bun run test src/features/alerts --reporter=dot`（89 项通过）、`bun run typecheck`、对本次 TSX 文件运行 `bunx --no-install oxlint -c .oxlintrc.json`，并执行 `bun run build`；随后在根目录执行 `go build ./...`。分页组件使用现有实现，不新增通用分页封装。


### 2026-11-16：本地消费、慢错误与疑似欠费提醒

- 用户确认生产示例在本地映射到 Root User（UserID=1）、key2（KeyID=2）和 usapi-gemini（ChannelID=49）。复用并启用规则 6（日汇总，Asia/Shanghai 每日 00:10）、5（累计新增每 $1000）、4（请求耗时达到 180 秒后失败）、16（疑似欠费），新增规则 38（Key 每个自然小时消耗达到 $150，一小时至多提醒一次）。两个生产渠道欠费例子映射到同一条本地规则 16。所有规则名以 `[本地]` 开头。
- 规则 4、16 保留 `legacy_notice` 事件适配器，并通过原迁移租约/CAS 切换为 unified/active，仅接收渠道 49。规则 5、6 的旧累计状态含已发送/含糊标记，保持 legacy 执行及原有效时间、累计基线和通知检查点，继续由原消费统计执行器发送；不擅自确认含糊历史，不重置账单或补造历史消费。`ENABLE_UID_FEISHU_ALERTS=true`、`ALERTING_ENABLED=true`、非 shadow 已核对。原生规则 38 使用统一引擎，无处置动作。
- 复用平台飞书目标 17，账号兼容规则仍引用同地址的既有个人目标 5、6。通知 URL 和密钥未写入仓库/记录。`slow_error_threshold_seconds=180`、`ErrorWarningEnvName=本地开发环境` 经既有设置写入与保护路径保存，重载本地后端后再接管渠道提醒。最终核对规则 4/16 revision=3、5/6 revision=2、38 revision=1，均 enabled。配置前后 Root 钱包及使用量、Key 额度/状态/AuthVersion 均未变化；未发送演示消息，未写入生产示例中的金额或日期。
- `LegacyNotice.channel_id` 把渠道身份放入持久化通知快照。仅慢错误/疑似欠费允许配置 `scope.channel_ids`，Root/owner=0 边界不变；校验存在性、排序去重，事件入队前执行范围过滤，缺失渠道元数据时不能绕过范围。没有渠道筛选的旧规则继续接收全站事件。渠道关联规则查询也包含这两类有渠道范围的兼容规则。
- `ProcessChannelError` 将疑似欠费通知与自动停用判定解耦，不修改停用策略或开关。402、明确的余额不足、阿里 Model Studio Access Denied 文档特征及既有关键词可发“可能欠费”提醒；普通 403 权限失败不会仅凭状态码被识别为欠费。无有效渠道 ID 的本地请求错误不发渠道提醒。原渠道级冷却和单执行器语义保留：慢错误 10 分钟，疑似欠费通常 1 分钟，阿里等既有特殊条件 1 小时。
- 飞书原生消息补充 UserID/KeyID、实际达到的 USD 档位；兼容渠道消息保留环境、渠道、状态码、RequestId、掩码错误，并增加规则名、含时区的触发时间和事件详情链接。英语/简体/繁体后端翻译与键目录同步，不把原始错误或凭据引入新模板。保留“@管理员”的旧文本语义，不伪造管理员的飞书身份或新增群全员提醒。
- 回归集中在既有 `service/alerting/alerting_test.go`：通过实际 `ProcessChannelError` 覆盖关闭自动停用时的 402 与指定阿里 403、其他渠道排除、普通 403 排除、秘密掩码及渠道持续启用；同时覆盖未知渠道/不允许的事件筛选拒绝、慢错误的匹配/排除、缺失元数据不能绕过、关联分页查询以及三语言身份/金额/时间/事件链接。原停用开关耦合通过 Go overlay 复现失败，消息增强预期也已在旧模板上复现失败；测试不回退运行中的开发服务。三库测试使用不同渠道身份，隔离进程级通知冷却，避免前一数据库抑制后一数据库的真实源事件。
- 实测 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15 及独立 Redis，配置临时实例的 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET` 后执行 `ENV=dev go test -race ./service/alerting -count=1 -v`：全部通过、无跳过（28.775s）。`go test ./service -run 'TestProcessChannelErrorSendsFeishuAlerts' -count=1`、`go test ./i18n -count=1`、`go build ./...`、`go vet -unreachable=false ./service/alerting ./service` 与 `git diff --check` 通过。没有 schema 或依赖变化；本地配置使用受限临时程序调用原 SaveRule/TargetVault/迁移域函数，临时程序和隔离测试实例在验证后删除。


### 2026-11-16：告警列表视口裁切与分页不可见

- 用户截图中的问题是页面布局：`AuthenticatedLayout`/`SidebarInset` 限定可用高度且隐藏溢出，告警页的原生 `<main>` 未建立收缩和滚动边界。真实 Chrome 中，810px 高视口的分页起点落在 y=1594，分页逻辑存在但无法从页面内访问；不能只用接口分页或 DOM 已渲染的测试判定完整显示。
- `AlertsPage` 接入既有 `SectionPageLayout`，保留统一标题/操作栏，内容区通过共享布局的 `min-h-0 flex-1 overflow-auto` 滚动；`RulesList` 用 `PageFooterPortal` 将既有 `DataTablePagination` 放到固定页脚。其他游标列表页签也复用该页脚，切换页签时移除上一页签的分页。没有新增通用滚动、分页或页面布局组件。
- 窄屏使用既有分页组件的 compact 模式，保留总数、当前页与前后页操作。规则名称显式覆盖表格单元格继承的 `white-space: nowrap`，在 12rem 最小宽度、18rem 最大宽度内换行，窄屏可通过表格原有横向滚动查看其他列。翻页/切换页大小后，在新数据就绪时滚动到表格起点；首次进入及定时刷新不主动改变阅读位置。
- 回归扩展 `web/src/features/alerts/components/__tests__/rules-list.test.tsx`，先复现分页仍在受裁切页签中、名称不换行、翻页不复位等失败，再验证完整一页、独立分页页脚、低宽度分页、切换页签不残留旧分页、长名称及翻页滚动。`web/` 下执行 `bun run test src/features/alerts --reporter=dot`：6 个文件、94 项通过；`bun run typecheck`、对本次四个 TSX 文件执行 `bunx --no-install oxlint -c .oxlintrc.json`、`bun run build`、根目录 `go build ./...` 和 `git diff --check` 通过。版权头原样保留，复用现有翻译键；未修改后台、数据库或本地告警配置。
- 浏览器验证使用隔离 Rsbuild 测试入口，加载实际 `AlertsPage`、共享 Header/SidebarInset 和项目 CSS，接口边界返回 38 条明确 fixture，使用系统 Google Chrome 与 Playwright；不读取浏览器登录资料、不向真实接口写入数据、不触发飞书消息。验证 1335×810、1280×600、390×667、320×568、844×390 五种尺寸：分页完全位于视口、滚动末行后页脚不移动、末行可滚到页脚上方、长名称换行、标题低于全局导航、页面无横向溢出、翻页显示新页第一行、最后一行及最后页禁用状态正确，均通过且无页面异常。修复后分页桌面 y=766/556，移动 y=613/514，横屏低高度 y=346，均含在对应视口内。

### 2026-11-16：调用量基线与跨渠道模型告警

- 保护行为：`request_started_count` 按真实生产调用的开始时间计数，包含未结束调用，metadata/progress 按 source 去重；`request_count` 继续使用终态口径。健康完整空窗口必须得到 0，采集失联或覆盖缺失不能当成 0。渠道与渠道模型使用实际尝试（含重试）；新 `model` 范围按客户端原始名称跨渠道统计网关请求，重试不能放大逻辑调用量。`controller/relay.go` 绑定父请求 metadata 时必须保留 `OriginModelName`，不能使用子 attempt 的渠道映射身份覆盖它。
- `request_volume_change` 支持手动次数基线和不包含当前窗口的历史平均基线；历史归一化到当前窗口，低基线/历史覆盖不足明确暖机。UI 使用百分比，上涨允许超过 100%，下降最大 100%；恢复阈值严格小于触发阈值。基线在 pending/firing 期间冻结并持久化，恢复必须以最初冻结值判断，不能因历史均值随异常下跌而误恢复；服务重启和历史覆盖变化不能丢失已冻结上下文。新流量指标只支持 incident 和通知。
- 权限与存储：跨渠道模型规则继续受 Root、owner=0、platform_enforced 边界保护，模型列表明确且有界，个人权限不能提升为平台规则。新增参数保存在现有配置 JSON；`AlertRuleState.Baseline` 使用每窗口调用数×10000，`PeriodKey` 保存冻结历史区间，仅限新流量 incident，不修改累计档位和周期规则。事件快照固化实际调用数、基线、变化基点、窗口，通知与事件详情显示百分比；预览/回放没有外部副作用。技术合同与完整配置示例见 `UNIFIED_ALERTING_DEVELOPMENT_DESIGN.md` 第 4.6 节。
- 实现锚点：`model/alert.go`、`service/alerting/traffic.go`、`metrics.go`、`rules.go`、`engine.go`、`replay.go`、`render.go`、`controller/alerting.go`、`controller/relay.go`；前端 `rule-form.ts`、`templates.ts`、`rule-editor.tsx`、`rule-preview.tsx`、`rule-replay.tsx`、`evidence-summary.tsx`。界面复用已有 Dialog、Field、InputGroup、MultiSelect、NativeSelect、StaticDataTable，没有引入替代通用组件。新增 28 个文案键覆盖七种界面语言，后端通知覆盖 en/zh-CN/zh-TW。
- 回归锚点：`TestAlertingDatabaseMatrix/*/traffic_baselines_and_zero_volume`，覆盖手动/历史基线、增长超过 100%、去重、在途计数、跨渠道请求去重与权限、零调用、历史排除当前/其他渠道/探测、冻结基线、恢复持续时间、重启、回放不产生事件及非法参数。矩阵保留已有 fresh 和代表性 released-core 升级、重复迁移检查；本次未增加数据库列或索引。
- 真实数据库验证：SQLite **3.50.4**（项目 `glebarez/sqlite` 驱动）、MySQL **8.4.13**、PostgreSQL **16.15**。只使用临时 `codex-traffic-mysql`/`codex-traffic-postgres` 容器和独立 Redis socket；环境变量指向对应一次性测试库后，运行 `go test -race ./service/alerting -count=1 -v`，三库全部通过，没有跳过 MySQL/PostgreSQL。服务端其他验证：`go test ./i18n ./common ./middleware ./controller -count=1`、`go vet ./service/alerting ./model`、`go build ./...` 通过。额外执行 `go vet ./service/alerting ./controller ./model` 时，controller 包被原有 `controller/cliend_user_quota.go:41:2: unreachable code` 阻止；该文件未修改，未将该项报告为通过。
- 前端验证：`bun run test src/features/alerts --reporter=dot`（107 项）、`bun run typecheck`、对上述受影响前端文件执行 `bunx --no-install oxlint -c .oxlintrc.json`、`bun run i18n:sync`、`bun run build` 通过。版权头保持原样。隔离浏览器入口加载真实编辑器和项目样式，在 1440×1000、390×844、320×568 检查历史/手动基线切换、跨渠道模型选择、150% 增长阈值序列化、60% 预览、隐藏无用聚合和最少样本控件，无横向溢出或页面异常；接口只返回 fixture，无真实通知或业务数据变更。

### 2026-11-16：告警字段说明与点击示例

- 保护行为：持续满足时长、恢复持续时长、恢复阈值以及调用量的指标、运算符、阈值、方向、基线和窗口字段应保留就近可点击的问号说明；不能仅依赖悬停，也不能把按钮嵌入输入框的 label。按钮必须为 `type=button`，点击不提交表单或修改配置；支持键盘打开、Escape 关闭并返回焦点，展开状态与可访问名称一致。
- 说明须忠实于规则语义：统计窗口与持续时长独立，0 秒在下一次匹配评估时处理；条件中断/数据不可用会重启计时，恢复时长不等于自动重新启用对象。恢复变化阈值使用冻结基线，不是已恢复流量比例。基线 200、触发变化 50%、恢复变化 20% 的上涨例子为 300 次触发、≤240 次满足恢复；下降例子为 ≤100 次触发、≥160 次满足恢复，均需恢复持续时长。方向切换须更新示例；历史平均说明须保留 120 次/30 分钟折算 20 次/5 分钟及排除当前窗口的示例。
- 实现与复用：`web/src/features/alerts/components/rule-field-help.tsx` 仅组合已有业务组件 `LearnMore` 和 `PopoverTitle`，没有新增独立问号/弹层实现；`rule-editor.tsx` 保留标签与输入关联。弹层高度同时受 Base UI `--available-height` 与视口约束，允许内部滚动，不能仅给一个大于锚点可用空间的视口最大高度。20 个新说明键通过项目翻译脚本写入七种界面语言，版权头保持原样。技术说明同步到 `UNIFIED_ALERTING_DEVELOPMENT_DESIGN.md` 第 4.6 节。
- 验证：`bun run test src/features/alerts --reporter=dot`（109 项）、`bun run typecheck`、对 `rule-editor.tsx`/`rule-field-help.tsx`/`__tests__/rule-editor.test.tsx` 执行 `bunx --no-install oxlint -c .oxlintrc.json`、`bun run i18n:sync`、`bun run build`、`go build ./...` 通过；交互测试覆盖定义与示例、上涨/下降切换、历史归一化说明、键盘展开、Escape 和焦点返回、点击不发送 POST。未改动后端行为、数据库或本地规则配置。

### 2026-11-16：告警复查修复与规则状态颜色

- 保护行为：改规则名称不能重置未展示的通知恢复开关、最少替代渠道数、独立探测模型/次数/观察时长、确认提醒/升级时间、报告回补/等待参数或 `quota_unit`。恢复探测模型与被监控模型分别保存；说明展示保存的实际探测要求，移除动作时不保留 `probe` 模式。
- 处置任务：仅名称、级别、分类、通知和确认配置变化时，同一归属、同一 condition generation、启用状态下，保存事务为仍有效的 `pending/retry_wait/running` 自动动作更新授权 revision，写审计和 outbox。事件与去重身份、原有效期、可领取时间、尝试次数不变；旧执行租约失效。执行策略/条件/权限变化、暂停和过期/终态任务不沿用授权；执行前仍按当前版本、权限和指标复核。不能直接放宽 `engine.runJob` 或 `CommitAlertHold` 的版本检查。
- 迁移和预览：旧事件通知回退只被 `pending/running/retry_wait` 阻止，不能被不可重试的 `expired/canceled` 永久锁住。24 小时滚动预览按接受水位读取完整窗口，不由虚构生效时间裁成预热；真实来源覆盖检查和正式规则启用预热仍保留，累计/在途预览的原起点不变。
- 状态展示复用共享 `StatusBadge`，保留文字并区分启用绿/暂停灰、P0/P1 红/P2 黄/P3 蓝、数据完整绿/预热和部分数据黄/过期与错误红/无数据和未知灰；暂停操作黄、恢复操作绿。复用原有表格和按钮，未新增替代通用组件。探针说明的一个新键按 `i18n-translate` 脚本同步七种界面语言，数字格式使用 `toIntlLocale` 和共享 `formatNumber`，版权头原样保留。
- 实现锚点：`model/alert_store.go:reauthorizeAlertActions`、`controller/alerting.go:PreviewAlertRule`、`service/alerting/legacy_notices.go:SwitchLegacyNotice`，前端 `rule-form.ts`、`rule-editor.tsx`、`rules-table.tsx`、`labels.ts`。回归扩展于现有 `service/alerting/alerting_test.go` 与现有两份前端表单/编辑器测试文件，没有另建散落测试文件。
- 数据库验证：一次性空库的 SQLite **3.50.4**、MySQL **8.4.13**（本机 33685）、PostgreSQL **16.15**（本机 34055），独立 Redis socket `/tmp/codex-alert-fix-1116-redis.sock`。设置 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET` 后运行 `go test -race ./service/alerting -count=1 -v`，三库全部通过，无跳过；沿用 fresh alert 表、代表 released-core 升级与连续两次迁移检查，无新增表结构或索引。最终权限断言与预览改动后，补跑 `go test -race ./service/alerting -run 'TestAlertingDatabaseMatrix/(sqlite|mysql|postgres)/(presentation_edits|rule_preview|legacy_event)' -count=1 -v`，三库均通过，覆盖真实处置、重试、旧租约和版本失效、到期/取消不复活、外用户拒绝、无安全证明拒绝、终态迁移回退和 24 小时窗口。
- 权限核对参照 [ASVS 5.0.0](https://asvs.owasp.org/) 的 `v5.0.0-8.2.1`、`v5.0.0-8.2.2`、`v5.0.0-8.3.1`、`v5.0.0-8.3.2`，以及 [Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)、[Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 指南。新增测试验证未授权主体、缺失证明、当前修订版本与旧 worker 失效；没有修改会话/凭证机制，不声明完整 ASVS 合规。
- 其他检查：告警前端 **112 项**、`bun run typecheck`、六个受影响 TS/TSX 文件定向 oxlint、`bun run i18n:sync`、`bun run build`、`go build ./...`、`go vet ./service/alerting ./model`、`go test ./i18n ./common ./middleware ./controller -count=1` 和 `git diff --check` 通过。隔离 Chrome 入口使用真实组件与项目样式，检查浅/深色主题、1440×1000/390×844/320×568、七种界面语言及非法语言降级，状态颜色可区分、无页面异常或文档横向溢出；编辑器预览保留自定义策略。仅 fixture 接口，无业务数据修改或实际飞书推送；临时测试设施用后清理。
- 前端复跑记录：一轮执行耗时 2107.57 秒，期间出现长时间停顿并有 3 项失败；未改变实现、断言或超时配置，随后完整重跑为 6 文件、112 项全部通过（12.26 秒），首次执行也全部通过（15.71 秒）。不将中断的那一轮报告为通过。

### 2026-11-16：事件摘要的百分比展示

- 保护行为：异常生命周期、触发历史、事件详情摘要中的比例指标必须显示百分比，已有历史记录同样适用。`request_volume_change=50000 | samples=6` 显示为“调用量变化：500%｜有效样本数：6”，不能把 50000 当作调用数量或 50000%。触发、重复提醒与恢复事件均转换，数量指标保留原单位。
- 实现锚点：`web/src/features/alerts/lib/event-summary.ts:formatAlertEventSummary` 复用 `formatAlertMetricValue`、`alertLabel` 和共享数字格式化，仅识别摘要末尾的指标/样本段；规则名、对象名、旧通知和已经格式化的摘要保留。三个入口在渲染时转换并响应语言切换，原始事件与审计证据不修改，无数据库变更。继续使用已有 Dialog、StaticDataTable 和七种语言的翻译键，未新增通用组件或翻译键，版权头原样保留。
- 验证：先新增实际 `IncidentDetailDialog` 回归用例，确认旧实现找不到 `500%` 展示而失败，再接入修复。`bun run test src/features/alerts` 为 **7 文件、129 项通过**，新增覆盖历史 trigger/repeat/recovery、语言切换、七种界面语言和非法语言降级、精度大于 JavaScript 安全整数、规则名包含类似字段、数量/旧文本/已格式化摘要不转换。`bun run typecheck`、五个受影响 TS/TSX 文件定向 oxlint、保留版权头的格式检查、`bun run build` 与 `git diff --check` 通过。
- 浏览器使用实际编辑器、共享样式、隔离 API fixture 和系统 Chrome，在 1440×1000、390×844、320×568、844×390 验证弹层可见、视口内定位、长内容滚动、中文→法语切换、Escape/Enter 操作，无页面错误或横向溢出；所有场景 POST 数均为 0。320×568 的法语弹层 y=293、高约 258，内容超长时内部滚动，保持在可见区域。


### 2026-11-16：告警金额统一展示美元

- 保护行为：异常生命周期、触发历史、事件详情摘要以及结构化金额证据/报告中，`spend_net` 与 `balance_remaining` 应展示 USD。历史 `spend_net=19770000 | samples=3` 展示为“净消耗：$39.54｜有效样本数：3”。换算单位缺省采用系统标准 500000，事件详情与证据优先使用配置快照的 `quota_unit`；不依赖全站人民币、Tokens 或自定义币种展示选项。原始事件、计费与资金数据不修改。
- 实现锚点：`web/src/features/alerts/lib/event-summary.ts:formatAlertMoney` 被摘要和 `EvidenceSummary` 共用，`EventDetailDialog` 将事件配置的换算单位传给摘要。共享 `lib/currency.ts` 增加 `formatUSD`：原有货币封装随全站设置换汇，无法表达固定 USD，因此兼容增加显式 USD 入口，继续复用原有货币格式化实现，不改变其他调用方。普通数额以本地化美元显示；大整数或浮点不能保持八位小数时使用 BigInt 精确结果与 USD 标识，保留负净额和最小额度精度。
- 验证：先扩展实际异常详情回归，确认旧实现无法显示 `$39.54` 后实施修复；`bun run test src/features/alerts` **7 文件、141 项通过**，覆盖金额/余额、历史事件、零值、单额度、退款、安全整数边界及超出边界、保存的换算单位、七种语言/非法语言降级与语言切换、全站 CNY/Tokens/自定义币种不改变告警美元。`bun run typecheck`、六个受影响 TS/TSX 文件定向 oxlint、保留版权头的格式检查、`bun run build` 与 `git diff --check` 通过。复用已有 UI 和翻译键，没有数据库或接口查询变更、业务数据修改或实际通知推送。


### 2026-11-16：线上旧告警迁移页的空列表兼容

- 事故与保护行为：打开「旧告警迁移」时，未配置旧 Key 告警的实例可能由接口返回 `legacy_tokens: null`，旧前端直接 `.map()` 导致整页错误。`migrations` 与 `legacy_quota_items` 也须兼容 `null` 和旧响应中的缺失字段；某一集合为空不能丢弃其他候选/已导入记录或破坏去重，真正请求失败仍应显示可重试错误。
- 实现锚点：`web/src/features/alerts/components/migrations-panel.tsx` 明确接口集合的 nullable/optional 类型，加载成功后分别归一化为空数组，使映射、展开、查找和表格渲染共用同一份有效集合。沿用原有 StaticDataTable、ErrorState、导入/接管/回退与安全验证流程，没有新增通用组件、翻译键、接口或数据库改动。
- 回归锚点：扩展现有 `components/__tests__/migration-import.test.tsx`，先以 `legacy_tokens: null` 在旧实现上复现相同 `Cannot read properties of null (reading 'map')`（原第 130 行），再验证三个集合分别为空、同时为空、字段缺失、标准空数组、正常候选与迁移记录保留及过滤、请求错误与重试恢复。迁移文件 11 项通过，`bun run test src/features/alerts` **7 文件、148 项通过**；`bun run typecheck`、两个受影响 TSX 文件的定向 oxlint、保留版权头的格式检查、`bun run build`、`go build ./...` 与 `git diff --check` 通过。未连接线上管理接口或改变实际迁移执行方；线上需要发布包含此修复的新版本。
- 浏览器验证：隔离 Chrome 通过实际 `AlertsPage` 点击中文「旧告警迁移」，依次模拟全部集合为 null、旧 Key 为 null 但保留已导入账号记录、其他集合为 null 但保留可导入 Key，均正常显示；页面异常为 0、写请求为 0。仅使用本地 fixture，不连接线上业务接口，临时服务已清理。


## Root 跨用户配置通知目标（2026-11-17）

- 用户明确要求 Root 创建规则时可选择归属用户，并可使用所有用户创建的通知及升级目标。以此替代 2026-11-15 记录中 Root 只能为平台范围选择平台目标的限制；普通用户仍只能绑定自己的目标，平台监控范围仍要求 Root、owner=0 和 platform_enforced。
- 账号/Key 规则复用已有 `UserSelect` 的名称/ID 服务端查询与分页。两类通知选择器复用 `MultiSelect`，兼容扩展远程查询回调及分页/重试页脚；现有组件缺少这两个接入点，未另建通用选择器。Root 查询全用户目标，标注属主与目标 ID；每页最多 200 项，搜索可命中未加载目标，已选目标按每批最多 20 个 ID 回填并在搜索变化后保留名称。
- Root 选择跨归属通知或升级目标时，前端自动启用平台强制管理，服务端规范化同样强制该管理模式，不能通过直接请求保留可由普通用户编辑的跨用户引用。取消平台强制管理时移除跨归属引用。启用规则继续拒绝停用、缺失及删除目标。普通用户无法通过数字查询或 IDs 回填跨归属目标，也不能借助规则修改继承 Root 权限。
- 回归锚点：已有 `service/alerting/alerting_test.go` 的 `testAlertRootTargets` 和 `testAlertRuleDeliveryRequirement`，覆盖 205 个目标分页、名称/ID 查询、受限回填、凭证不返回、普通用户隔离、Root 跨归属保存自动强制管理，以及预览/回放/新建/编辑四个 HTTP 入口；已有 `rule-editor.test.tsx` / `rule-form.test.ts` 和共享多选组件用例覆盖对应界面流程、超过 200 用户和目标、超出单批的已选目标回填、查询失败重试及停用目标拒绝。
- 数据库验证使用可销毁空库与独立 Redis：SQLite 3.50.4、MySQL 8.4.13（37005）、PostgreSQL 16.15（41837）、Redis 8.10.2（Unix socket）。设置 `ALERTING_TEST_MYSQL_DSN`、`ALERTING_TEST_POSTGRES_DSN`、`ALERTING_TEST_REDIS_SOCKET` 后执行 `go test -race ./service/alerting -count=1 -v`，三库全部通过，无跳过；包含已有代表发布版本升级与重复迁移夹具。本次没有模型或迁移改动，也未操作业务库或发送真实飞书通知。
- 权限核对参考 [OWASP ASVS 5.0.0](https://asvs.owasp.org/) 的 `v5.0.0-8.2.1`、`v5.0.0-8.2.2`、`v5.0.0-8.3.1`，以及 [Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)、[Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 指南。适用控制为服务端角色/属主授权、跨用户引用仅 Root 批准、普通用户不可继承平台能力及凭证不返回；没有修改登录、会话或自动动作重新认证机制，不作整站 ASVS 合规声明。
- 可销毁 MySQL/PostgreSQL 容器、独立 Redis 和浏览器夹具服务已清理。
- 最终校验：`web/` 下 `bun run test src/features/alerts src/components/multi-select/__tests__`（11 文件、167 用例）全部通过；`bun run typecheck`、改动文件 `bun x oxlint -c .oxlintrc.json ...`、`bun run i18n:sync` 和七语言逐键检查、`bun run build` 通过，随后根目录 `go build ./...` 通过。`go vet ./service/alerting ./model`、`go test ./service -run '^TestAlertProof' -count=1` 和 `go test ./router -run '^TestAlertMutationGuardsRunBeforeHandler$' -count=1` 通过。基于完整 RuleEditor 的隔离浏览器夹具在 1440/390 像素宽度验证 Root 选择用户 245、跨用户普通/升级目标、分页和预览/保存实际请求，均无页面错误和横向溢出；未连接真实通知服务。

### 切回自定义告警模板恢复归属用户（2026-11-17）

- 复现：Root 由自定义规则切到渠道模型模板，归属用户按平台范围隐藏；再切回“自定义规则”时，原处理器因未找到预设而跳过重置，导致表面选中自定义、实际仍保留渠道模型范围和参数。普通用户由消耗档位切回自定义时也保留消耗指标。
- 修复：自定义选项与预设共用现有模板切换处理器；切回时加载普通 API Key 默认表单及当前账号归属，清除模板名称、对象/通知选择和旧预览、回放结果。归属用户继续依据实际监控范围显示，复用既有 NativeSelect、UserSelect 和表单重置，不另建选择器或增加同步 effect。
- 回归：已有 `rule-editor.test.tsx` 新增两个用户可见流程；修复前稳定复现 Root 找不到归属用户、普通用户保留消耗指标两个失败。覆盖 Root 在平台/账号模板与自定义之间反复切换、归属用户重新选择及预览请求的数据归属，以及普通用户恢复默认指标并保持原归属权限。
- 验证：`bun run test src/features/alerts src/components/multi-select/__tests__ --maxWorkers=2`（11 文件、169 用例）、`bun run typecheck`、两个改动 TSX 文件的 oxlint、保护版权头与 `git diff --check` 通过；`bun run build` 后顺序执行根目录 `go build ./...` 通过。隔离浏览器夹具在 1440/390 像素宽度分别验证三次模板往返、归属用户恢复、改选用户 245 以及 API Key 预览/保存实际请求，无页面错误或横向溢出；夹具服务已清理。本次仅修改前端模板切换和回归测试，无数据库改动。

## 飞书告警消息卡片（2026-11-17）

- 用户明确要求美化飞书告警，因此告警中心的 `feishu` 目标由 text 升级为 interactive 原生卡片。P0/P1 红色、P2 橙色、P3 蓝色；标题附带等级色点和场景图标。触发 🚨、持续 ⏳、恢复 ✅、探测 🧪、升级 📣、待确认 ⏰、消费档位 💰、汇总 📊、修订 📝、数据缺失 ⚠️、服务异常 🛠️、测试 🔔 各有表达；恢复为绿色、升级为紫色、数据缺失为黄色。
- 卡片拆分渠道/模型、账号/API Key、系统/预算等对象身份，并显示当前指标、比较阈值、样本、数据完整性、本地时区窗口与事件时间。USD/百分比/毫秒沿用共用格式化；修复原非比例恢复通知仍显示触发阈值的问题。汇总隐藏无意义的触发阈值，保留报告明细和补发/部分数据说明。探测事件中的在途安全检查不能冒充原失败率/耗时指标；恢复卡片提醒以详情确认最终可用状态。
- 使用不可变规则和证据快照，不查询实时凭证或原始请求正文。动态名称、旧告警正文均作为 plain_text；超长 Unicode 文本截短，必要时缩减正文给 20 KiB 限制及签名留出空间。详情按钮仅使用已配置站点的 HTTP(S) URL 和事件 ID，缺失/非法站点地址不生成无效按钮。测试覆盖报告、流量基线、旧渠道消息、特殊字符和超长文本。
- 原生 Feishu timestamp/sign、HTTP/业务码校验、网络访问规则、速率和重试继续走既有路径。通用 Webhook v1/v2、企业微信等保留原 payload 契约；未由统一目标接管的旧 `SendFeishuNotifyContext` 保持原文本协议与加签回归。
- 新增回归全部集中在已有 `service/alerting/alerting_test.go`：先复现 text 类型不符合卡片、卡片缺失等失败，再验证 15 种事件/等级组合、三种语言、对象与单位、方向正确的恢复比较符、汇总/流量细节、探测语义及大小限制。三语言补齐 29 个卡片文案键，键目录及语言完整性测试通过。
- 验证命令：`go test -race ./service/alerting ./i18n -count=1`、`go vet ./service/alerting ./i18n`、`go test -race ./service -run 'TestProcessChannelError(SendsFeishuAlerts|MasksDisableReasonAndNotification)$' -count=1`、`go build ./...`、gofmt 与 `git diff --check`。本次不改变数据库模式、查询或规则判定；完整包测试执行其 SQLite 路径，未配置的 MySQL/PostgreSQL/Redis 矩阵分支沿既有规则跳过，不表述为本次重跑三库。
- 协议参考 [飞书自定义机器人](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)、[卡片按钮](https://open.feishu.cn/document/ukTMukTMukTM/uYzM3QjL2MzN04iNzcDN/component-list/interactive-components)、[卡片结构官方示例](https://open.feishu.cn/community/articles/7271149634339422210)。通过实际 Deliver 序列化和内存 HTTP transport 生成 8 张示例卡片；本地示意在 1440/390 宽度无横向溢出。没有向真实群发送演示消息，最终排版以飞书客户端为准。

### 规则详情的指标单位与空样本（2026-11-17）

- 复现：规则详情的状态表直接展示 `last_value`，导致成功率 10000 显示为原始基点，消耗金额显示内部额度单位。修复前新增详情回归分别无法找到 `100%` 和 `$19.77`。
- 保护行为：详情复用既有百分比和 USD 格式化，成功率/失败率等比例的 10000 为 100%、1234 为 12.34%；保存的 `quota_unit` 决定美元换算。数量、样本的 int64 字符串保持精确，不能统一转为 JS Number。比例有有效样本时 0 显示 0%；无分母的请求比例显示 `—`。调用量下降至零和预算比例仍可在当前样本为零时有有效百分比，不能一并隐藏；调用量基线不完整时继续显示 `—`。
- 详情、预览、回放、事件证据及摘要共用带样本上下文的 `formatAlertMetricValue`；后端判定、数据状态和持久化单位不变。继续复用 `StaticDataTable`、`Dialog`、`formatAlertMoney` 和 `toIntlLocale`，未新增 UI 组件或文案键。
- 验证：`web/` 下 `bun run test src/features/alerts src/components/multi-select/__tests__ --maxWorkers=2`（12 文件、192 用例）、`bun run typecheck`、10 个改动 TS/TSX 文件的定向 oxlint 与 `bun run build` 通过。回归覆盖详情实际单元格、真实零值/无样本、预览/回放、预算与零流量例外、七种界面语言及非法语言降级、运行时切换语言、USD 与超安全整数精度。隔离浏览器在 1440×1100 和 390×844 验证百分比、无样本、语言切换，无页面异常或文档横向溢出；只使用夹具数据。
- 前端构建完成后顺序执行根目录 `go build ./...` 通过；`git diff --check` 和全部 10 个 TS/TSX 文件版权头检查通过，临时浏览器服务已清理。无数据库或规则配置变更。
