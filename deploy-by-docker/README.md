# new-api Docker Compose 部署

此目录支持本地 PostgreSQL 和已购买的云 PostgreSQL。默认启动 Caddy、new-api、Redis、PostgreSQL 四个服务；外部数据库模式只启动前三个服务。示例域名为 `api.example.com`。

## 准备

```bash
cd deploy-by-docker
./init-env.sh
# 编辑 .env：替换 DOMAIN、ACME_EMAIL 和 NEW_API_TAG。
docker login kimiteam-ap-southeast-3.cr.volces.com
```

项目镜像默认使用 `kimiteam-ap-southeast-3.cr.volces.com/newapi/newapi-full`，标签应与 `../build-image.sh --push` 上传的标签一致。可用 `NEW_API_IMAGE` 更换仓库。镜像架构需要与部署服务器一致。

## 本地 PostgreSQL（默认）

`.env` 中的 `SQL_DSN` 保持空值。Compose 自动合并 `docker-compose.yml` 和 `docker-compose.override.yml`，启动本地 PostgreSQL 并等待数据库健康后启动应用。

```bash
docker compose config --quiet
docker compose pull
docker compose up -d
docker compose ps
```

也可以显式指定两个文件：`docker compose -f docker-compose.yml -f docker-compose.override.yml up -d`。

## 使用已购买的云 PostgreSQL

先在云数据库中准备好数据库和应用账号。账号需要连接数据库、在应用 schema 中创建表和索引、维护其创建对象的权限，以便 new-api 执行数据库迁移；不需要超级用户权限。

在 `.env` 设置完整连接 URL（用户名、密码中的特殊字符需进行 URL 编码）：

```dotenv
SQL_DSN='postgresql://newapi:URL_ENCODED_PASSWORD@postgres.example.com:5432/newapi?sslmode=verify-full'
```

只加载基础文件，跳过本地数据库服务和它的健康检查依赖：

```bash
docker compose -f docker-compose.yml config --quiet
docker compose -f docker-compose.yml pull
docker compose -f docker-compose.yml up -d
docker compose -f docker-compose.yml ps
```

外部模式不使用 `POSTGRES_PASSWORD`、`POSTGRES_APP_PASSWORD` 等本地 PostgreSQL 参数，也不创建本地 PostgreSQL 数据卷。应用通过代理网络的正常出口访问云数据库；需保证服务器到云数据库的网络路由、端口和云侧访问白名单允许连接。

建议使用 `sslmode=verify-full` 校验服务端证书和主机名。若云厂商提供专用 CA，将 PEM 证书保存为 `postgresql/certs/root.crt`，URL 再追加 `&sslrootcert=/etc/new-api/postgresql/root.crt`。该目录只读挂载到应用，证书文件不纳入 Git；使用公共 CA 且系统信任链可验证时通常不需要单独提供文件。参见 [PostgreSQL TLS 说明](https://www.postgresql.org/docs/18/libpq-ssl.html)。

若已有本地部署，切换前先用原配置执行 `docker compose down`（不要加 `-v`）保留原数据卷，再修改连接配置。修改 `SQL_DSN` 不会迁移数据；需要保留旧数据时，先用 `pg_dump` / `pg_restore` 将其迁入云数据库，并保留原 `SESSION_SECRET`、`CRYPTO_SECRET`。

外部模式的更新、重启、日志和停止命令也应始终带 `-f docker-compose.yml`，例如 `docker compose -f docker-compose.yml down`。不要直接执行默认 `docker compose up`，否则会同时加载本地数据库配置。

## 主数据库与独立日志数据库

两个连接分别通过部署目录的 `.env` 配置，并传入 new-api 容器：

| 变量 | 用途 | 留空时的行为 |
| --- | --- | --- |
| `SQL_DSN` | 用户、渠道、令牌、任务等主数据 | 使用本地 PostgreSQL 地址；本地模式需加载默认覆盖文件 |
| `LOG_SQL_DSN` | 应用请求日志等独立日志存储 | 复用主数据库，不新建日志库连接 |

例如两个云 PostgreSQL 数据库：

```dotenv
SQL_DSN='postgresql://newapi:URL_ENCODED_PASSWORD@main-postgres.example.com:5432/newapi?sslmode=verify-full'
LOG_SQL_DSN='postgresql://newapi_logs:URL_ENCODED_LOG_PASSWORD@log-postgres.example.com:5432/newapi_logs?sslmode=verify-full'
```

可以是同一实例中的两个数据库，也可以是不同实例；还支持本地主库配合外部日志库。主库和日志库的选择互不替代，是否启动本地 PostgreSQL 仍由 Compose 文件选择决定。

只有 `LOG_SQL_DSN` 留空时才复用主库；显式配置的日志库连接失败会阻止应用正常启动，不会静默改写到主库。修改连接后使用对应模式的 `docker compose up -d new-api` 重新创建应用容器，让新环境变量生效。

独立日志数据库必须先创建，日志账号需要建表、索引和维护自身对象的迁移权限。现有本地初始化脚本仅创建 `POSTGRES_DB`，不会依据 `LOG_SQL_DSN` 自动创建其他数据库。若两个云库使用不同 CA，可分别将 `root.crt`、`log-root.crt` 放在 `postgresql/certs/`，并在各自 URL 中设置对应的 `sslrootcert` 容器路径。

在新环境同时指定两个连接即可分别初始化表；在已有环境中修改连接不会迁移历史日志。切换日志库前应备份并迁移需要保留的日志数据。

## 域名和运行工具

将真实域名的 A/AAAA 记录解析到服务器，并开放 TCP 80、TCP 443 和可选的 UDP 443。Caddy 自动申请、续期 HTTPS 证书并将 HTTP 重定向到 HTTPS；示例域名不能用于申请你自己的证书。

首次访问 `https://你的域名` 完成 new-api 初始化并设置管理员账号。

Podman 可使用 `podman login` 和 `podman-compose`；本地模式可显式使用 `podman-compose -f docker-compose.yml -f docker-compose.override.yml up -d`，外部模式使用 `podman-compose -f docker-compose.yml up -d`。检查配置时将 `up -d` 换成 `config >/dev/null`。macOS 上先启动 `podman machine`。

## 文件和持久化

- `docker-compose.yml`：Caddy、new-api、Redis 的基础配置。
- `docker-compose.override.yml`：可选的本地 PostgreSQL、数据卷及应用启动依赖；默认自动加载。
- `.env.example` / `init-env.sh`：生成权限为 600 的 `.env`，包含五个独立随机密码/密钥；重复执行保留现有文件。
- `caddy/Caddyfile`：HTTPS、SSE 即时刷新、WebSocket 及反向代理。
- `redis/redis.conf`：AOF/RDB 持久化，512 MiB 内存上限，禁止自动驱逐键；可按服务器容量调整上限。
- `postgresql/`：本地数据库配置、SCRAM 认证、首次初始化脚本，以及可选的云数据库 CA 证书目录。

数据保存在本项目的命名卷中：`caddy_data`、`caddy_config`、`new_api_data`、`new_api_logs`、`redis_data`、`postgres_data`，卷名由 Compose 项目名前缀隔离；`postgres_data` 仅在本地模式创建。PostgreSQL 18 的卷挂载点为 `/var/lib/postgresql`。

`docker compose down` 保留数据卷；`docker compose down -v` 会删除本项目数据。备份 PostgreSQL、Redis、应用数据卷和 `.env`。保留 `SESSION_SECRET`、`CRYPTO_SECRET`，重建容器时继续使用；更换 `CRYPTO_SECRET` 会影响已有加密数据的解密。

只有 Caddy 发布端口。应用通过独立代理网络访问上游和云数据库，Redis/本地 PostgreSQL 位于内部数据库网络。`PROXY_SUBNET`、`CADDY_IP`、`NEW_API_IP` 应保持匹配，两个服务使用不同地址；网段冲突时同时修改。应用只信任该 Caddy IP 的代理头。

PostgreSQL 的管理员密码、应用用户和应用密码只在**空数据卷初始化时**生效；修改 `.env` 不会修改已有数据库账号。修改已有账号应先在数据库执行对应的 `ALTER ROLE`，再同步连接配置。生成的十六进制密码可直接用于连接 URL；手动使用特殊字符时需正确进行 URL 编码。

## 更新和查看状态

```bash
# 修改 .env 中的 NEW_API_TAG 后更新应用。
docker compose pull new-api
docker compose up -d new-api
docker compose logs --tail=100 -f new-api
```

以上命令用于默认本地模式；云数据库模式在每个 `docker compose` 后加 `-f docker-compose.yml`。

修改挂载的服务配置文件后，重启对应服务以重新读取配置；本地 PostgreSQL 的配置文件不会应用到云数据库。

依赖镜像固定为 2026-10-30 核实的稳定补丁版本，避免重建时自动跨大版本：

| 服务 | 镜像 | 发布来源 |
| --- | --- | --- |
| Caddy | `caddy:2.11.7-alpine` | [Caddy 2.11.7](https://github.com/caddyserver/caddy/releases/tag/v2.11.7) |
| Redis | `redis:8.10.3-alpine` | [Redis 8.10.3](https://github.com/redis/redis/releases/tag/8.10.3) |
| PostgreSQL | `postgres:18.6-alpine` | [PostgreSQL 18.6](https://www.postgresql.org/docs/18/release-18-6.html) |

## HTTPS 与会话配置

按 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 和 [CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) 的适用部署要求，配置启用了 HTTPS、HSTS、Secure Refresh Cookie、精确 HTTPS Origin 和限定来源的代理信任。

参考 [ASVS 5.0.0 V12](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x21-V12-Secure-Communication.md) 的外部 HTTPS 要求（12.1.1、12.2.1、12.2.2）。本地服务的信任边界为单台主机：内部 HTTP、Redis、本地 PostgreSQL 未启用 TLS，未覆盖 V12.3 的 L2 服务间加密要求；云数据库连接应启用上述 TLS 验证。

`DOMAIN` 必须与实际访问域名一致。公网证书签发、DNS、防火墙及线上认证流程需要在替换真实域名后验证；此部署模板不代表完成整个应用的 ASVS 合规验证。

## 验证记录（2026-10-30）

- 使用 Podman 5.8.2、podman-compose 1.6.0 在独立项目中启动四个真实服务，全部健康；临时容器和数据卷已清理。
- 默认自动合并和显式双文件合并均保留本地四服务及 Redis/PostgreSQL 健康依赖；只加载基础文件时仅有三个服务，不依赖本地 PostgreSQL 密码、容器或数据卷。
- 使用独立的 TLS-only PostgreSQL 18.6 模拟外部数据服务，应用通过挂载 CA 和 `sslmode=verify-full` 连接，重复启动和 Caddy HTTPS 访问正常；错误主机名、错误 CA 均被真实应用拒绝，未回退到明文连接。实际云厂商的地址、权限、路由和证书仍需部署时确认。
- 验证 `LOG_SQL_DSN` 留空时在主库初始化日志存储；同时配置两个连接时，真实应用分别打开主库/日志库的 TLS 连接，日志库初始化日志和审计表、不创建用户等主数据表。错误日志库 CA/主机名会导致启动失败，不会回退主库。
- 验证了 PostgreSQL 专用账号权限、Redis 密码认证、Redis/PostgreSQL 重启后数据保留，以及应用在已有数据卷上再次启动成功。
- 使用 `localhost` 和受信任的临时本地 CA 验证 HTTPS、HTTP 重定向、HSTS；不可信 Origin 的刷新请求返回 403，可信 Origin 但无会话凭据返回 401。
- 随机密码初始化生成五个独立的值，每个为 64 个十六进制字符（256 位随机数据），`.env` 权限为 600；已有配置保持不变，随机数生成失败时不发布 `.env`。
- 数据库回归矩阵运行于真实 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 18.6，三个引擎均通过，没有跳过数据库。

以下 Go 验证命令从仓库根目录运行；`TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 已在测试环境中指向独立的临时空数据库：

```bash
GOWORK=off go test ./model -run '^TestQuotaDataPersistenceDatabaseMatrix$' -count=1 -v
# 外部 PostgreSQL TLS 验证时，TEST_POSTGRES_DSN 使用 verify-full 和临时 CA。
GOWORK=off go test ./model -run '^TestQuotaDataPersistenceDatabaseMatrix$/^postgres$' -count=1 -v
GOWORK=off go test ./common ./middleware -run 'TestInitSessionCookieSettings|TestSessionCookieOriginGuard|TestConfigureTrustedProxies' -count=1
```
