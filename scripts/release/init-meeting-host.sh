#!/usr/bin/env bash
# Создаёт независимые secrets для нового staging и production на согласованном хосте.
# @args $1 — абсолютный путь к проверенному .env.production.example.
set -euo pipefail
[[ $(id -u) == 0 && $(hostname) == mail.janickiy.com && ${1:-} == /* && -f $1 ]] || { printf 'Unexpected host or template\n' >&2; exit 1; }
umask 077
install -d -m 0700 /etc/meetrix /etc/meetrix/staging /etc/meetrix/production
install -d -m 0750 -o root -g 1000 /etc/meetrix/certs
for target in staging production; do
  directory="/etc/meetrix/$target"
  [[ ! -e "$directory/.env" ]] || { printf 'Configuration already exists: %s\n' "$target" >&2; exit 1; }
  install -d -m 0700 "$directory/secrets"
  install -d -m 0750 -o 1000 -g 1000 "/opt/meetrix/storage-$target"
  metrics_secret="$(openssl rand -hex 32)"
  printf '%s' "$metrics_secret" > "$directory/secrets/metrics_token"
  chmod 0644 "$directory/secrets/metrics_token"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" != *=* || "$line" == \#* ]]; then printf '%s\n' "$line"; continue; fi
    key="${line%%=*}"; value="${line#*=}"
    case "$key" in
      JWT_SECRET|MEDIA_TICKET_SECRET|MEDIA_INTERNAL_SECRET|WORKER_INTERNAL_SECRET|TURN_SHARED_SECRET|POSTGRES_PASSWORD|REDIS_PASSWORD|RABBIT_MQ_PASSWORD|MINIO_ROOT_PASSWORD) value="$(openssl rand -hex 32)";;
      METRICS_SECRET) value="$metrics_secret";;
      PROVIDER_TOKEN_ENCRYPTION_KEY) value="$(openssl rand -base64 32)";;
      POSTGRES_USER|POSTGRES_DB|RABBIT_MQ_USER|MINIO_ROOT_USER) value="meetrix_$target";;
      PUBLIC_FRONTEND_URL|MINIO_PUBLIC_ENDPOINT|WS_ALLOWED_ORIGINS) value=https://meeting.janickiy.com;;
      MEDIA_NAT_IPS|TURN_PUBLIC_IP) value=93.89.223.151;;
      HTTP_TRUSTED_PROXIES) value=172.31.242.0/24;;
      TURN_URLS) value='turn:meeting.janickiy.com:3478?transport=udp,turn:meeting.janickiy.com:3478?transport=tcp,turns:meeting.janickiy.com:5349?transport=tcp';;
      TURN_REALM) value=meeting.janickiy.com;;
      TURN_MIN_PORT) value=49160;; TURN_MAX_PORT) value=49199;;
      MEDIA_MAX_ROOMS) value=2;; MEDIA_MAX_PEERS) value=4;;
      WS_MAX_CONNECTIONS) value=50;; DB_MAX_OPEN) value=10;; DB_MAX_IDLE) value=2;;
      RECORDING_MAX_ACTIVE) value=1;; RECORDING_MAX_DURATION) value=1h;; RECORDING_MAX_MIB) value=512;;
      MEDIA_WORKER_ID) value="media-meeting-$target";; WORKER_ID) value="recorder-meeting-$target";;
      RECORDER_STORAGE_DIR) value="/opt/meetrix/storage-$target";;
      TLS_CERT_DIR|TURN_TLS_CERT_DIR) value=/etc/meetrix/certs;;
    esac
    printf '%s=%s\n' "$key" "$value"
  done < "$1" > "$directory/.env"
  printf 'METRICS_TOKEN_FILE=%s/secrets/metrics_token\nREVERSE_PROXY_PORT=19080\n' "$directory" >> "$directory/.env"
done
printf 'Independent private staging and production configuration created.\n'
