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
| `middleware/distributor.go` | 保留 main 的渠道约束/插件过滤与 prod 的 Key、特殊渠道、全局模型路由优先级；显式候选先过滤禁用渠道、路径黑白名单、模态和插件约束，无可用候选时按优先级继续回退；将真实请求 Body 与 URL 交给全局路由匹配，并仅为最终采用的规则设置重试次数。 |
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

## Future main merge non-regression contract

本节记录本次冲突表面解决后实际发生过的功能回退。它不是一次性备注，而是后续合并 `main` 时必须保留的项目契约。`main` 中没有对应实现，不代表这里的功能可以删除；除非用户明确批准行为变更，否则必须把这些能力迁移到上游的新结构中，并运行对应锚点测试。

| 保护能力 | 本次合并后出现过的问题 | 后续合并必须保持的行为 | 主要实现与回归锚点 |
|---|---|---|---|
| 固定渠道类型编号与基础常量/DTO | 渠道编号随 `main` 新类型插入而漂移，前后端、任务插件和已有数据库配置可能指向错误 provider。 | prod 已有渠道编号 `0–77` 永久保持；新增 Task Plugin、vLLM、SGLang 使用 `78–80`。前端枚举、后端常量、API type、默认 Base URL、插件绑定和序列化 DTO 必须同步。 | `constant/channel.go`、`common/api_type.go`、`relaykit/dto/channel_settings.go`；`constant/channel_test.go`、`common/api_type_task_plugin_test.go`、`web/src/features/channels/lib/__tests__/channel-type-ids.test.ts`。 |
| 创建/编辑渠道的旧配置 | 请求路径白名单、请求路径黑名单、响应模型名称重写和 Vertex Google bucket 控件从表单消失。 | 四项配置必须能够创建、编辑回填并保存，字段名和后端序列化契约不得改变；bucket 仅在适用的 Vertex 配置中显示。 | `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`；`web/src/features/channels/components/__tests__/channel-configuration.test.tsx`。 |
| 请求路径白名单/黑名单运行时约束 | UI 字段存在但普通渠道选择链路没有执行约束。 | 所有候选来源——普通随机、固定渠道、Key 规则、全局路由、特殊渠道、重试、竞速和 affinity——都必须经过同一请求路径过滤；黑名单优先于白名单。 | `model/channel_constraint.go`、`middleware/distributor.go`；`model/channel_constraint_test.go`。 |
| 跨协议响应模型名称重写 | 只有 OpenAI 响应生效，Anthropic、Gemini Native、Responses、SSE 和 Realtime 被遗漏。 | JSON、SSE、透传和 WebSocket 下行响应均应用当前渠道的映射；支持 `model`、`modelVersion`/`model_version` 和嵌套 Responses 对象；不得改写客户端上行请求，不得因长度变化保留错误的 `Content-Length`。 | `relay/common/model_output_mapping.go`、`controller/model_output_mapping.go`、Realtime handlers；`controller/relay_race_test.go`。 |
| 左侧菜单与系统模块开关 | 账单、错误日志、部署、监控、预算、路由、结算、价格中心等 prod 入口被 `main` 菜单结构覆盖。 | prod 旧入口与 main 新增审计日志、安全中心、任务插件入口并存；默认开关、设置页名称、路由和组织菜单 `pageKeys` 保持一致。 | `web/src/hooks/use-sidebar-data.ts`、`web/src/hooks/use-sidebar-config.ts`、`web/src/features/system-settings/maintenance/*`；`web/src/hooks/__tests__/sidebar-config.test.tsx` 及侧边栏定向测试。 |
| root 创建/编辑 Key 的渠道规则 | root 可配置的渠道倍率、渠道规则和“优先使用 Key 渠道规则”开关丢失。 | 这些控件只对 root 开放，编辑时正确回填，提交时继续写入既有字段；普通账号不可获得该权限。 | `web/src/features/keys/components/api-keys-mutate-drawer.tsx`；`api-keys-mutate-drawer.test.tsx`、`channel-rules-editor-dialog.test.tsx`、`web/src/features/keys/lib/__tests__/channel-rules.test.ts`。 |
| root 按用户筛选 Key | Key 列表只能查看当前用户，root 的“全部用户/指定用户”筛选丢失。 | root 可选择我的 Key、全部用户或指定用户；普通列表与搜索请求都必须携带正确的 `user_id`，非 root 不显示越权入口。 | `web/src/features/keys/components/api-keys-table.tsx`；`api-key-listing.test.tsx`。 |
| Key/全局/特殊渠道规则及三种模式 | Key 维度规则、`/model-route-config` 全局路由和特殊渠道之间的优先级、回退及 order/random/race 行为在重构后不完整；全局路由曾只匹配模型而忽略 Body/URL。 | 固定渠道优先；显式 Key 规则、特殊渠道和全局路由按既有优先级选择并在无可用候选时正确回退。顺序、随机、随机竞速均保持候选边界和重试语义；不支持并发竞速的协议按同一候选计划串行降级。全局规则的模型、Body、URL 已配置维度必须同时匹配。 | `middleware/distributor.go`、`service/channel_select.go`、`controller/relay_race.go`、`model/model_route_config.go`；`service/channel_select_test.go`、`controller/relay_race_test.go`、`model/model_route_config_test.go`、`relay/responses_websocket_test.go`。 |
| 日志列表筛选条件 | Token、用户名、渠道、Client UID、MT Session ID、Trace ID、Traj ID、Session ID 等筛选入口和 URL 状态同步丢失。 | 恢复的筛选条件必须继续支持回填、清空、提交、计数和权限边界；root 用户名筛选与 Token/渠道选项接口必须保留。 | `web/src/features/usage-logs/components/common-logs-filter-bar.tsx`、`web/src/features/usage-logs/api.ts`；`group-filter.test.tsx`、`filter-combobox.test.tsx`、`log-type-filter.test.tsx`。 |
| root 查看日志原始报文 | 日志详情中请求体、响应体和 Header 入口消失。 | 三个入口仅 root 可见；root 即使切换为“仅看自己”仍可用；前端隐藏不能替代后端 `RootAuth`。 | `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`、`router/api-router.go`、`controller/log.go`；`detail-preview.test.tsx`。 |
| 个人资料模型限制 | 账号级模型限制开关、允许模型列表和“限制为已有结算价格模型”操作丢失。 | 用户设置必须完整回填并保存模型限制；令牌单独设置限制时以令牌为准；批量加入结算模型需去重且失败时恢复 UI 状态。 | `web/src/features/profile/components/tabs/notification-tab.tsx`、用户设置 DTO/接口；`web/src/features/profile/__tests__/settings.test.tsx`。 |
| 独立错误日志 | 失败请求只写普通 `logs`，不再写 `error_logs`。 | `SAVE_ERROR_LOG`/错误日志开关启用时，每次失败渠道记录真实渠道快照与耗时；重试成功或竞速胜出时清除不应保留的失败请求体，全部失败时保留最终可审计请求；落库失败必须写后端错误日志。 | `controller/relay.go`、`controller/channel-test.go`、`model/debug.go`；`controller/relay_error_log_test.go`。 |
| 日志渠道链路与竞速胜者 | 渠道列不再直接展示各段耗时，hover 信息和 race winner/loser 标记消失；成功的最后一段耗时曾漏记。 | root 渠道列直接展示每个渠道的耗时，hover 展示渠道名称、ID、耗时；竞速日志明确标记胜出/未胜出；`use_channel` 与 `admin_info.use_channel_time` 一一对应并包含最终成功尝试。 | `service/log_info_generate.go`、`controller/relay.go`、日志列组件；`service/text_quota_test.go`、`retry-chain-display.test.tsx`。 |
| `/v1/responses` 请求/响应日志 | handler 已捕获请求和完整响应，但 HTTP/WebSocket 共用结算函数把两个参数替换为空串，普通 Responses 日志因此没有原始报文。 | `SAVE_REQUEST_RESPONSE=true` 时，普通及音频 HTTP `/v1/responses` 必须把已捕获请求、响应传入消费日志；`/v1/responses/compact` 保持独立计价日志路径。WebSocket 没有 HTTP body 记录器时不得伪造报文。 | `relay/responses_handler.go`、`relay/responses_websocket.go`；`TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse`。 |

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
- 内置任务插件及前端 provider 映射已按固定渠道编号 0–80 校正，避免 Jimeng/Vidu/Doubao/Sora 等插件绑定到旧编号。
- 渠道配置表单完整回归文件：65 个 Vitest 用例通过，覆盖上述四项配置的编辑回填与保存。
- 侧边栏定向回归测试 17 个 Vitest 用例通过，覆盖 prod 旧入口与 main 新入口的并存、模块默认值、独立部署开关及组织菜单 `pageKeys`。
- Key 与日志筛选定向回归：5 个 Vitest 文件共 54 个用例通过；渠道规则解析/序列化的 Bun 测试 15 个用例通过。覆盖 root 渠道规则提交、root 按用户筛选 Key、日志关联标识 URL 状态及下拉框选择/清空行为。
- 本轮涉及的 9 个前端文件通过 `oxlint` 与 `oxfmt --check`；随后再次执行 `bun run build:check`，TypeScript 与生产构建通过。
- 前端完整测试套件曾启动，但在并行运行时出现多组 20 秒级超时并被终止；不将完整前端测试标记为通过。
- 数据库相关合并尚未完成 SQLite/MySQL/PostgreSQL 三数据库实机矩阵，因此不声明数据库兼容性验证完成。
- 本轮渠道路由定向验证：`go test ./model ./service ./middleware ./controller ./relay -count=1` 通过；覆盖 Key 顺序/随机/竞速重试计划、Responses WebSocket 显式路由和全局 Body/URL 匹配。
- 本轮再次执行 `go test ./... -count=1` 时有一个未定位的全量套件失败；上述五个受影响包随后/单独均通过，因此不把这次全量复跑记录为通过，需在资源允许时用 JSON 输出继续定位。
- 日志详情原始报文入口定向测试：`bun run test --run src/features/usage-logs/components/__tests__/detail-preview.test.tsx`，22 个用例通过；后端 `/api/log/:id/request`、`response`、`header` 路由均确认位于 `RootAuth` 路由组。
- 个人资料模型限制定向验证：`bun run test -- src/features/profile/__tests__/settings.test.tsx`，8 个用例通过；相关文件通过 `oxlint`、`oxfmt --check`，并通过 `bun run typecheck`。
- 独立错误日志恢复验证：`go test ./controller ./service -count=1` 与 `go test ./model -count=1` 均通过；定向用例确认 `SAVE_ERROR_LOG=true` 时 `error_logs` 实际产生记录。未执行 MySQL/PostgreSQL 实机矩阵，因此本轮不额外声明跨数据库验证完成。
- 请求链路展示定向验证：`bun run test -- src/features/usage-logs/components/__tests__/retry-chain-display.test.tsx` 通过（2 个用例）；后端渠道耗时定向用例 `TestAppendRelayLogAdminInfoWritesChannelAttemptTimes` 通过。
- 渠道请求路径与响应规则定向验证：`TestChannelSatisfiesFilters` 覆盖普通渠道白名单/黑名单，`TestModelOutputMappingResponseWriterRewritesAllResponseFormats` 覆盖 OpenAI/Anthropic/Gemini Native/Responses/SSE 的统一模型名称重写；受影响的 model/controller/relay/helper/OpenAI/Gemini Realtime/Qwen Realtime 包完整测试在允许 loopback listener 的环境全部通过。
- Responses 请求/响应日志恢复验证：`go test ./relay -count=1` 通过；新增 `TestConsumeResponsesQuotaRecordsCapturedRequestAndResponse` 确认普通 Responses 结算会将捕获的请求体和响应体实际写入消费日志。
