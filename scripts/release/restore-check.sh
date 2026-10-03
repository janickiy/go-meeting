#!/usr/bin/env bash
# Проверяет восстановление только в отдельную БД и новый бакет локального или staging-стенда.
set -euo pipefail
source "$(dirname "$0")/common.sh"
backup_input= args=()
while (($#)); do
  (($# >= 2)) || fail "Missing argument value"
  if [[ "$1" == --backup ]]; then backup_input="$2"; else args+=("$1" "$2"); fi
  shift 2
done
parse_release_args "${args[@]}"
validate_release
[[ "$RELEASE_ENVIRONMENT" != production ]] || fail "Restore drills must use a separate local/staging target"
[[ "$backup_input" == /* && -f "$backup_input/backup.json" ]] || fail "An absolute complete --backup directory is required"
[[ -f "$backup_input/release.json" ]] || fail "Source release manifest is missing from backup"
jq -e --arg database "$(file_sha256 "$backup_input/postgres.dump")" \
  --arg objects "$(file_sha256 "$backup_input/objects/manifest.json")" \
  --arg release "$(file_sha256 "$backup_input/release.json")" \
  --arg version "$(jq -er .version "$backup_input/release.json")" \
  --arg commit "$(jq -er .commit "$backup_input/release.json")" \
  '.schemaVersion == 1 and .databaseSHA256 == $database and .objectsManifestSHA256 == $objects and .releaseManifestSHA256 == $release and .version == $version and .commit == $commit' \
  "$backup_input/backup.json" >/dev/null || fail "Backup checksums do not match"
database_timeout="${RELEASE_DATABASE_TIMEOUT:-600}"
[[ "$database_timeout" =~ ^[1-9][0-9]*$ ]] || fail "Invalid database timeout"
acquire_release_lock
run_suffix="$(date -u +%Y%m%d%H%M%S)_$$"
restore_database="restore_check_$run_suffix"
restore_bucket="restore-check-${run_suffix//_/-}"
[[ "$restore_database" =~ ^restore_check_[0-9]+_[0-9]+$ ]] || fail "Invalid isolated database name"
start=$SECONDS
compose exec -T postgres timeout "$database_timeout" sh -ec 'PGPASSWORD="$POSTGRES_PASSWORD" exec createdb -U "$POSTGRES_USER" --template=template0 "$1"' sh "$restore_database"
compose exec -T postgres timeout "$database_timeout" sh -ec 'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_restore -U "$POSTGRES_USER" -d "$1" --exit-on-error --single-transaction --no-owner --no-acl' sh "$restore_database" < "$backup_input/postgres.dump"
database_seconds=$((SECONDS - start))
# Повторный дамп проверяет, что восстановленный каталог читается целиком, а не только принимает соединение.
compose exec -T postgres timeout "$database_timeout" sh -ec 'PGPASSWORD="$POSTGRES_PASSWORD" exec pg_dump -U "$POSTGRES_USER" -d "$1" --format=custom --no-owner --no-acl --lock-wait-timeout=5s' sh "$restore_database" > /dev/null
ledger_rows="$(compose exec -T postgres timeout "$database_timeout" sh -ec 'PGPASSWORD="$POSTGRES_PASSWORD" exec psql -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -c "SELECT count(*) FROM release_schema_migrations"' sh "$restore_database")"
[[ "$ledger_rows" =~ ^[0-9]+$ && "$ledger_rows" -gt 0 ]] || fail "Restored migration ledger is missing or empty"
start=$SECONDS
compose run --rm --no-deps --user "$(id -u):$(id -g)" \
  --entrypoint /app/object-backup -v "$backup_input:/backup:ro" api restore \
  --dir /backup/objects --bucket "$restore_bucket"
object_seconds=$((SECONDS - start))
umask 077
jq -n --arg testedAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg database "$restore_database" --arg bucket "$restore_bucket" \
  --arg sourceManifestSHA256 "$(file_sha256 "$backup_input/release.json")" --arg targetManifestSHA256 "$RELEASE_MANIFEST_SHA" \
  --arg backupSHA256 "$(file_sha256 "$backup_input/backup.json")" --argjson objectCount "$(jq '.objects|length' "$backup_input/objects/manifest.json")" \
  --argjson migrations "$ledger_rows" --argjson databaseSeconds "$database_seconds" --argjson objectSeconds "$object_seconds" \
  '{testedAt:$testedAt,backupSHA256:$backupSHA256,sourceManifestSHA256:$sourceManifestSHA256,targetManifestSHA256:$targetManifestSHA256,database:$database,bucket:$bucket,migrations:$migrations,objectCount:$objectCount,databaseSeconds:$databaseSeconds,objectSeconds:$objectSeconds,checksumsVerified:true,bucketPolicyVerified:true,signedAccessVerified:($objectCount>0),productionRPO:null,productionRTO:null}' \
  > "$backup_input/restore-check-$run_suffix.json"
printf 'Restore verified in isolated database %s and bucket %s. Targets retained for inspection.\n' "$restore_database" "$restore_bucket"
