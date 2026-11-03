#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
env_file="$script_dir/.env"

if [[ -e "$env_file" ]]; then
    printf '.env 已存在，保留现有配置和密钥。\n'
    exit 0
fi

command -v openssl >/dev/null 2>&1 || {
    printf '错误：需要 openssl 生成随机密码。\n' >&2
    exit 1
}

umask 077
env_tmp="$(mktemp "$script_dir/.env.tmp.XXXXXX")"
trap 'rm -f -- "$env_tmp"' EXIT

while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
        POSTGRES_PASSWORD=|POSTGRES_APP_PASSWORD=|REDIS_PASSWORD=|SESSION_SECRET=|CRYPTO_SECRET=)
            secret="$(openssl rand -hex 32)"
            printf '%s=%s\n' "${line%%=*}" "$secret"
            ;;
        *) printf '%s\n' "$line" ;;
    esac
done < "$script_dir/.env.example" > "$env_tmp"

# Publish without overwriting an existing .env, even if another init ran concurrently.
ln "$env_tmp" "$env_file"
printf '.env 已生成（权限 600）。请修改 DOMAIN、ACME_EMAIL 和 NEW_API_TAG 后启动。\n'
printf '使用云 PostgreSQL 时，另设 SQL_DSN，并仅加载 docker-compose.yml。\n'
printf '需要独立日志库时设置 LOG_SQL_DSN；留空复用主库。\n'
