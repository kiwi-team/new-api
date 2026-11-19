# 2026-11-19 main 合入 prod 的冲突处理记录

本次合并的目标是 prod `d376c7538`，来源是 main `3a407cc9b`，共同祖先为 `6cf0d90b5`。开始处理时索引包含 110 个内容冲突。按《MERGE_CONFLICT_RESOLUTION.md》的分支保护约定比较两侧实现，在当前合并中处理，保留原有未跟踪文件。

## main 带来的主要改动

从共同祖先到 main 共 137 个 prod 尚未包含的提交，共涉及 778 个文件。对运行和管理方式影响较大的有以下几项。

| 领域 | 对使用者的影响 |
| --- | --- |
| 插件 | 新增提供方插件、请求处理插件和社区市场。管理员可配置插件适用范围。渠道可通过插件登录、管理凭证、发现模型。原生插件路由可挂在顶层。 |
| 模型与中转 | 新增或更新 Seedream 5.5、Vertex Gemini Omni、Kling、GPT-6、Gemini 思考级别和 DeepSeek 工具历史等适配，修正 Gemini Embeddings 用量。 |
| 定价与看板 | 管理员保存模型价格前可预览变更。前端统一展示不同缓存时长，支持 12 小时缓存写入价格，统计增加缓存读写量。 |
| 授权与渠道 | 加入 Redis 鉴权缓存完整性验证与同槽操作，修复已删除用户的 API Key 状态、OAuth 邀请归属、手动禁用渠道被错误覆盖等问题。 |
| 运维与支付 | 支持通过 `HOST` 配置监听地址、易支付 RSA 签名，更新前端依赖及插件运行时的无 AVX 兼容性。 |

## 冲突处理

### 已核对的取舍

- 比较了 110 个文件与 Git 的 `AUTO_MERGE` 快照，处理前的冲突文本与快照一致。文件内自动合并的代码也纳入编译、协议测试和调用链检查。
- Claude 请求转换保留真实省略号、空工具结果和相邻 assistant 消息中的工具调用。移除 main 的合成占位逻辑。Gemini 同时保留附件兼容字段、思考签名和新的媒体处理字段。工具参数改为原始 JSON 后，相应转换器和断言同步适配。
- 保留 prod 的大文件路由、Key 和全局显式候选、响应模型重写、每次尝试耗时及独立错误日志，并接入 main 的请求插件入口及尝试结束处理。对接流式响应时区分客户端流与仅上游使用的缓冲流。
- 统计事件继续经过 prod 的持久化、去重与按 UID／场景／项目／计划隔离的累计路径。接入看板的缓存读写总量，并保留各时长的原计数。额度修正只更新金额。旧事件新增的零值字段在 JSON 中省略，兼容旧事件校验和。
- 认证缓存同时校验 main 的 MAC 和 prod 的数据库授权版本。令牌 Lua 脚本同步写入渠道规则、倍率和告警字段。令牌权限与余额依然由权威数据库校验。新增的 Redis 测试按该契约检查数据库故障和并发余额变更。
- 前端沿用已有渠道、Key、用户抽屉及定价编辑器，合并渠道插件配置和 prod 的路径、模型重写与流式桥接设置。保留字符计价并加入 12 小时缓存价格展示。七种翻译按共同祖先逐键合并，双方都修改同一译文时保留 prod 的译文。
- Docker 保留构建平台原生编译和目标架构交叉编译，更新 main 指定的镜像版本。以 main 锁文件及双方依赖声明交给 Bun 1.4.4 重新解析，保留 prod 的 `@lobehub/ui`。

### 回归测试发现的跨文件问题

- 渠道类型数值会持久化。prod 的高级自定义渠道是 74，任务插件渠道是 78。把 main 测试和界面中的对应场景接到 prod 常量后，插件登录、余额和模型发现可以沿真实渠道路径测试。渠道测试弹窗合并时还多出一个 Embeddings 选项，TypeSafe 也缺少用于按插件能力筛选的路径，两处均已修正。
- 插件顶层路由曾能占用 prod 控制台路径。补全保留的命名空间，并把告警批量迁移接口设为与单项迁移相同的会话访问规则。错误日志包装器补充 `Unwrap`，让流式响应仍能设置写入截止时间。独立错误日志采用每次尝试捕获的上游请求 ID，避免并发重试串号。
- main 的通用计费校验会拒绝 prod 的 Seedream 5.5 原生图片用量表达式。实际图片入口现在带入已校验的计费事实。Gemini Omni 原生任务保留官方 token 维度，渠道插件的计价维度单列。按请求写入统计时，普通输入、缓存读取和缓存写入继续分别计算。
- Kling 4.0 的两个分支对同一路径有不同约定。保留 prod 的 `/v1/videos` 多模态输入、API Key、按官方时长结算和缺用量待结算行为，沿用已有的 `/v1/responses` 模型范围。main 新增的 Kling 原生提交路径单独接入，保留请求体并在任务状态中记录计费维度。原生与统一视频入口分别有回归覆盖。
- 对比两侧测试约定时，保留 prod 的数据库授权、未缓存输入口径、统一客户端错误类型与固定渠道编号。调整使用不同约定的上游测试夹具，保留对金额、权限、请求内容和路由可见性的断言。
- 前端全量验证发现，已缓存渠道选项时，消费日志列表刷新可能关闭已打开的详情。复现后固定渠道名称索引的查询转换函数，保留弹窗状态。回归覆盖父组件刷新、切换语言以及管理员与普通用户看到的插件信息。

### 如何复查冲突范围

在合并前运行 `git merge-tree --write-tree --name-only prod main` 可以预演三方合并。退出码 0 表示可以自动合并，1 表示有冲突。该命令保留当前分支、索引与工作区，可随时对两个固定提交重跑。合并开始后，`git diff --name-only --diff-filter=U` 列出待解决文件，`git ls-files -u` 可核对索引中的三个版本。以本次固定提交 `d376c7538` 和 `3a407cc9b` 重跑，退出码为 1，列出 110 个冲突文件。

### 验证结果

- 根模块使用 Go 1.26.10，`go test -p 4 ./...` 和 `go build ./...` 通过。插件最终更改另以 `go test ./plugins -count=1` 验证，包括从 Kling 原生提交状态恢复、等待用量及按实际输入输出时长结算。
- `cd relaykit && GOWORK=off go test ./... && GOWORK=off go build ./...` 通过，覆盖空消息、签名、缓存 usage、相邻工具调用和 golden 转换矩阵。
- `cd web && bun run i18n:sync && bun run build:check` 使用 Bun 1.4.4 通过。类型检查覆盖 1,688 个文件。
- `go test ./model -run '^TestAuthCacheIntegrityDatabaseMatrix$' -count=1 -v` 在独立 SQLite、MySQL 和 PostgreSQL 实例上通过。认证边界核对依据为 [OWASP ASVS 5.0.0](https://asvs.owasp.org/)、[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 和 [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)。

- `go test ./model -run '^TestQuotaDataPersistenceDatabaseMatrix$' -count=1 -v` 在 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 16.15 通过，覆盖同桶多次累加、缓存写入、退款和分组隔离。
- 各设置一次 `TEST_TASK_DB_DIALECT=sqlite|mysql|postgres` 并运行 `go test ./controller -run '^TestTaskBillingAttributionDatabase$' -count=1 -v`，三套数据库通过。各自验证提交、退款及补扣时的 UID、项目、计划和额度归属。
- `go test ./controller -run '^TestProviderChannelManagementDatabaseMatrix$' -count=1 -v`、`go test ./relay -run '^Test(Seedream55NativeFallbackDatabaseMatrix|VolcengineMessages)' -count=1 -v`、`go test ./model -run '^TestMigrationSchemaStability$' -count=1 -v` 在三套数据库通过。Volcengine 测试为主库与日志库分别配置 DSN。
- `go test ./service/alerting -count=1 -v` 连接上述三种数据库及独立 Redis 套接字通过。AWS 渠道测试覆盖 Bedrock SDK 的日志报文与流式最终用量。
- 数据库启动验证使用实际发布标签 `v1.0.0-rc.51`（`c9c6da278`）和合并后的源码分别编译夹具。每种引擎各跑两条路径，一条在发布版执行 `model.InitDB()` 和 `model.InitLogDB()` 后写入样本，另一条从合并版空库开始。两条路径均再用合并版连续启动两次。六组全部通过，复核了用户额度 12345、统计计数 2、token 数 33、消费额度 123 及独立日志库里的原始记录。所有数据库均为本次创建的隔离实例或文件。
- `cd tokenkit && GOWORK=off go build ./... && GOWORK=off go test ./...` 通过。
- `go test ./controller -run '^Test(ErrorLogUpstreamRequestIDDatabaseMatrix|RequestPolicyRoutingDatabaseMatrix|ChannelAffinityRetryPriorities)$' -count=1 -v` 在上述三种引擎通过，覆盖强制路由优先级、重试与上游请求 ID。
- ClickHouse 24.8.14.39 独立日志库通过 `go test ./model -run '^TestClickHouseErrorTimingMigration$' -count=1 -v`。
- 前端按仓库已有的两个测试运行器执行。`cd web` 后，用 `rg -l "from 'node:test'" src` 找出 12 个独立测试文件交给 `bun test`，59 项通过。将这 12 个路径逐一作为 `--exclude` 传给 `bun run test --maxWorkers=3 --testTimeout=60000`，最后一轮全部 261 个 Vitest 文件、3,234 项通过。日志详情文件的 34 项用例另连续单独运行五次，均通过。
- 最终状态为 `prod` 上的合并结果已暂存，`git ls-files -u` 无输出，`git diff --cached --check` 通过。当前保留 `MERGE_HEAD=3a407cc9b`，由维护者决定提交时间。
