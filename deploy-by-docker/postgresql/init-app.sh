#!/bin/sh
set -eu

# The official entrypoint runs this only when initializing an empty data volume.
# psql identifier/literal quoting keeps the configured role/password out of SQL syntax.
psql --set=ON_ERROR_STOP=1 \
    --username "$POSTGRES_USER" \
    --dbname "$POSTGRES_DB" \
    --set=app_user="$POSTGRES_APP_USER" \
    --set=app_password="$POSTGRES_APP_PASSWORD" \
    --set=app_db="$POSTGRES_DB" <<'SQL'
CREATE USER :"app_user" WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD :'app_password';
ALTER DATABASE :"app_db" OWNER TO :"app_user";
REVOKE CONNECT ON DATABASE :"app_db" FROM PUBLIC;
ALTER SCHEMA public OWNER TO :"app_user";
SQL
