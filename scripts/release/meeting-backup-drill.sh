#!/usr/bin/env bash
# Выполняет адресную репетицию backup только на закрытом новом staging-проекте.
set -euo pipefail
delivery=/opt/meetrix/releases/v1.0.0-meeting.20261003.1/delivery-v3
source "$delivery/deployment/scripts/release/common.sh"
arguments=(--environment staging --env-file /etc/meetrix/staging/.env --manifest "$delivery/release.json" --project recorder-staging-meeting)
parse_release_args "${arguments[@]}"
validate_release
compose exec -T postgres sh -ec 'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
SELECT 'active_conferences', count(*) FROM conferences WHERE status='active';
SELECT 'active_records', count(*) FROM record WHERE status NOT IN ('ready','partial_ready','failed','cancelled');
SELECT 'processing_jobs', count(*) FROM background_jobs WHERE state='processing';
SELECT 'pending_uploads', count(*) FROM chat_attachments WHERE status='pending';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM conferences WHERE status='active') OR
    EXISTS(SELECT 1 FROM record WHERE status NOT IN ('ready','partial_ready','failed','cancelled')) OR
    EXISTS(SELECT 1 FROM background_jobs WHERE state='processing') OR
    EXISTS(SELECT 1 FROM chat_attachments WHERE status='pending')
 THEN RAISE EXCEPTION 'Active work prevents backup'; END IF;
END $$;
SQL
queues="$(compose exec -T rabbitmq rabbitmqctl -q list_queues name messages_ready messages_unacknowledged --formatter json)"
jq -e 'all(.[]; .messages_ready == 0 and .messages_unacknowledged == 0)' <<<"$queues" >/dev/null || fail 'Nonempty broker queues prevent write freeze'
bash "$delivery/deployment/scripts/release/release.sh" drain "${arguments[@]}"
# Admission уже ограничен оператором; после завершения тестов останавливаем
# только собственные producer/consumer-процессы, не базы и не чужие контейнеры.
compose stop proxy frontend api live-worker product-worker worker media-worker
install -d -m 0700 /opt/meetrix/backups
RELEASE_BACKUP_QUIESCED=1 bash "$delivery/deployment/scripts/release/backup.sh" "${arguments[@]}" --output /opt/meetrix/backups/staging-20261003
bash "$delivery/deployment/scripts/release/restore-check.sh" "${arguments[@]}" --backup /opt/meetrix/backups/staging-20261003
printf 'Closed staging backup/restore complete; application processes remain stopped.\n'
