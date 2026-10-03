#!/usr/bin/env bash
# Создаёт только изолированную конфигурацию репетиции; её секреты и сертификат непригодны для production.
set -euo pipefail
source "$(dirname "$0")/common.sh"
[[ $# == 2 && "$1" == --directory && "$2" == /* && ! -e "$2" ]] || fail "Usage: init-local.sh --directory /absolute/new/private/directory"
rehearsal_dir="$2"
umask 077
mkdir -p "$rehearsal_dir/certs" "$rehearsal_dir/storage" "$rehearsal_dir/secrets"
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 7 \
  -subj '/CN=localhost' -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' \
  -keyout "$rehearsal_dir/certs/privkey.pem" -out "$rehearsal_dir/certs/fullchain.pem" >/dev/null 2>&1
metrics_secret="$(openssl rand -hex 32)"
printf '%s' "$metrics_secret" > "$rehearsal_dir/secrets/metrics_token"
openssl rand -hex 32 > "$rehearsal_dir/secrets/grafana_password"
while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" != *=* || "$line" == \#* ]]; then printf '%s\n' "$line"; continue; fi
  key="${line%%=*}"
  value="${line#*=}"
  case "$key" in
    APP_ENV) value=test;;
    JWT_SECRET|MEDIA_TICKET_SECRET|MEDIA_INTERNAL_SECRET|WORKER_INTERNAL_SECRET|TURN_SHARED_SECRET|POSTGRES_PASSWORD|REDIS_PASSWORD|RABBIT_MQ_PASSWORD|MINIO_ROOT_PASSWORD) value="$(openssl rand -hex 32)";;
    METRICS_SECRET) value="$metrics_secret";;
    PROVIDER_TOKEN_ENCRYPTION_KEY) value="$(openssl rand -base64 32)";;
    POSTGRES_USER|POSTGRES_DB|RABBIT_MQ_USER|MINIO_ROOT_USER) value=release_rehearsal;;
    PUBLIC_FRONTEND_URL|MINIO_PUBLIC_ENDPOINT|WS_ALLOWED_ORIGINS) value=https://localhost:25482;;
    MEDIA_NAT_IPS|TURN_PUBLIC_IP) value=127.0.0.1;;
    MEDIA_UDP_PORT|MEDIA_TCP_PORT) value=50220;;
    TURN_MIN_PORT) value=50300;; TURN_MAX_PORT) value=50340;;
    TURN_URLS) value='turn:127.0.0.1:23478?transport=udp,turn:127.0.0.1:23478?transport=tcp';;
    TURN_REALM) value=release.local;;
    MEDIA_WORKER_ID) value=media-release-rehearsal;;
    WORKER_ID) value=recorder-release-rehearsal;;
    RECORDER_STORAGE_DIR) value="$rehearsal_dir/storage";;
    TLS_CERT_DIR|TURN_TLS_CERT_DIR) value="$rehearsal_dir/certs";;
  esac
  printf '%s=%s\n' "$key" "$value"
done < "$RELEASE_ROOT/.env.production.example" > "$rehearsal_dir/.env.local"
printf 'METRICS_TOKEN_FILE=%s/secrets/metrics_token\nGRAFANA_PASSWORD_FILE=%s/secrets/grafana_password\n' "$rehearsal_dir" "$rehearsal_dir" >> "$rehearsal_dir/.env.local"
# Только файлы привязанных секретов читаются непривилегированными контейнерами; родитель остаётся 0700.
chmod 0644 "$rehearsal_dir/secrets/metrics_token" "$rehearsal_dir/secrets/grafana_password" "$rehearsal_dir/certs/fullchain.pem"
# UID рекордера равен 1000. В локальной Docker Desktop bind mount требует записи из контейнера.
chmod 0777 "$rehearsal_dir/storage"
printf 'Created private local rehearsal configuration. Trust only its temporary certificate for local tests.\n'
