#!/usr/bin/env bash
# Сохраняет согласованную логическую копию после явно подтверждённой остановки записывающих операций.
set -euo pipefail
source "$(dirname "$0")/common.sh"
backup_output= args=()
while (($#)); do
  (($# >= 2)) || fail "Missing argument value"
  if [[ "$1" == --output ]]; then backup_output="$2"; else args+=("$1" "$2"); fi
  shift 2
done
parse_release_args "${args[@]}"
validate_release
[[ "$backup_output" == /* && ! -e "$backup_output" ]] || fail "--output must be a new absolute directory"
[[ "${RELEASE_BACKUP_QUIESCED:-}" == 1 ]] || fail "Quiesce application writes, then set RELEASE_BACKUP_QUIESCED=1; a live cross-store snapshot is not atomic"
database_timeout="${RELEASE_DATABASE_TIMEOUT:-600}"
[[ "$database_timeout" =~ ^[1-9][0-9]*$ ]] || fail "Invalid database timeout"
acquire_release_lock
umask 077
mkdir -p "$backup_output"
cp "$RELEASE_MANIFEST" "$backup_output/release.json"
compose exec -T postgres timeout "$database_timeout" sh -ec 'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --no-owner --no-acl --lock-wait-timeout=5s' > "$backup_output/postgres.dump"
[[ -s "$backup_output/postgres.dump" ]] || fail "PostgreSQL backup is empty"
compose run --rm --no-deps --user "$(id -u):$(id -g)" \
  --entrypoint /app/object-backup -v "$backup_output:/backup" api backup --dir /backup/objects
jq -n --arg version "$RELEASE_VERSION" --arg commit "$RELEASE_COMMIT" \
  --arg releaseSHA "$RELEASE_MANIFEST_SHA" --arg databaseSHA "$(file_sha256 "$backup_output/postgres.dump")" \
  --arg objectsSHA "$(file_sha256 "$backup_output/objects/manifest.json")" \
  --arg createdAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{schemaVersion:1,version:$version,commit:$commit,releaseManifestSHA256:$releaseSHA,databaseSHA256:$databaseSHA,objectsManifestSHA256:$objectsSHA,createdAt:$createdAt,scope:"quiesced PostgreSQL and current object versions",restoreVerified:false}' > "$backup_output/backup.json"
printf 'Backup complete. Keep the private directory encrypted off-host; restore verification is still required.\n'
