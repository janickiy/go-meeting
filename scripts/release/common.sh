#!/usr/bin/env bash
# Общие проверки релиза. Файлы конфигурации и манифест никогда не выполняются как shell-код.
set -euo pipefail

RELEASE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASE_SERVICES=(api media-worker worker product-worker live-worker frontend minio postgres redis rabbitmq coturn proxy prometheus grafana)
RELEASE_APPLICATIONS=(media-worker worker product-worker live-worker api frontend)

# fail завершает операцию с безопасным сообщением, без содержимого конфигурации.
fail() { printf '%s\n' "$*" >&2; exit 1; }

# file_sha256 возвращает контрольную сумму файла на Linux и macOS.
# @args $1 — путь к файлу; @return SHA-256 без имени файла.
file_sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d ' ' -f 1;
  else shasum -a 256 "$1" | cut -d ' ' -f 1; fi
}

# parse_release_args разбирает явную цель операции; дополнительные параметры вызывающий скрипт снимает заранее.
parse_release_args() {
  RELEASE_ENVIRONMENT= RELEASE_ENV_FILE= RELEASE_MANIFEST= RELEASE_PROJECT= RELEASE_EVIDENCE=
  while (($#)); do
    (($# >= 2)) || fail "Missing value for $1"
    case "$1" in
      --environment) RELEASE_ENVIRONMENT="$2";;
      --env-file) RELEASE_ENV_FILE="$2";;
      --manifest) RELEASE_MANIFEST="$2";;
      --project) RELEASE_PROJECT="$2";;
      --evidence) RELEASE_EVIDENCE="$2";;
      *) fail "Unknown release argument: $1";;
    esac
    shift 2
  done
}

# compose вызывает только зафиксированный набор Compose-файлов и явно заданный проект.
compose() {
  local options=(-f "$RELEASE_ROOT/docker-compose.production.yml")
  if [[ "$RELEASE_ENVIRONMENT" == local ]]; then options+=(-f "$RELEASE_ROOT/docker-compose.release-local.yml"); fi
  docker compose --project-name "$RELEASE_PROJECT" --env-file "$RELEASE_ENV_FILE" "${options[@]}" "$@"
}

# validate_release проверяет формат, изоляцию окружения и отсутствие изменяемых image tags.
validate_release() {
  command -v jq >/dev/null || fail "jq is required"
  command -v docker >/dev/null || fail "Docker is required"
  case "$RELEASE_ENVIRONMENT:$RELEASE_PROJECT" in
    local:recorder-release-rehearsal|local:recorder-release-rehearsal-*) export RELEASE_APP_ENV=test;;
    staging:recorder-staging|staging:recorder-staging-*|production:recorder-production|production:recorder-production-*) export RELEASE_APP_ENV=production;;
    *) fail "Explicit environment and matching isolated project are required";;
  esac
  [[ "$RELEASE_PROJECT" =~ ^[a-z0-9][a-z0-9-]+$ ]] || fail "Invalid project name"
  [[ "$RELEASE_ENV_FILE" == /* && -f "$RELEASE_ENV_FILE" ]] || fail "--env-file must name an existing absolute path"
  [[ "$RELEASE_MANIFEST" == /* && -f "$RELEASE_MANIFEST" ]] || fail "--manifest must name an existing absolute path"
  jq -e '.schemaVersion == 1 and (.version | test("^v[0-9]+\\.[0-9]+\\.[0-9]+(-[A-Za-z0-9.-]+)?$")) and (.commit | test("^[a-f0-9]{40}$")) and (.builtAt | type == "string") and (.images | type == "object")' "$RELEASE_MANIFEST" >/dev/null || fail "Invalid release manifest"
  RELEASE_VERSION="$(jq -r .version "$RELEASE_MANIFEST")"
  RELEASE_COMMIT="$(jq -r .commit "$RELEASE_MANIFEST")"
  RELEASE_MANIFEST_SHA="$(file_sha256 "$RELEASE_MANIFEST")"
  if [[ "$RELEASE_ENVIRONMENT" != local ]]; then
    jq -e '.sourceDirty == false and .sourceFingerprintVerified == true and .securityScanned == true and .artifactMode == "registry"' "$RELEASE_MANIFEST" >/dev/null || fail "Staging/production require a scanned registry release from clean source"
  fi
  local service ref variable
  for service in "${RELEASE_SERVICES[@]}"; do
    ref="$(jq -er --arg service "$service" '.images[$service]' "$RELEASE_MANIFEST")" || fail "Missing image: $service"
    if [[ "$RELEASE_ENVIRONMENT" == local && "$ref" =~ ^sha256:[a-f0-9]{64}$ ]]; then :
    elif [[ "$ref" =~ ^[a-zA-Z0-9][a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$ ]]; then :
    else fail "An immutable image reference is required for $service"; fi
    variable="$(printf '%s_IMAGE' "$service" | tr '[:lower:]-' '[:upper:]_')"
    export "$variable=$ref"
  done
  export RELEASE_ENV_FILE
  export BUILD_VERSION="$RELEASE_VERSION"
  compose --profile observability --profile migration config --quiet
  if [[ "$RELEASE_ENVIRONMENT" != local ]]; then
    # Вывод полной конфигурации содержит секреты: читаем только публичные origins в процессе.
    compose config --format json | jq -e '
      [.services.frontend.environment.PUBLIC_FRONTEND_URL, .services.frontend.environment.MINIO_PUBLIC_ENDPOINT]
      | all(.[]; test("^https://") and (test("localhost|127\\.0\\.0\\.1|example\\.invalid|\\[::1\\]") | not))' >/dev/null || fail "Staging/production require real HTTPS origins"
  fi
}

# require_release_approval блокирует production-изменение до привязанного к артефакту решения оператора.
require_release_approval() {
  local action="${1:-deploy}"
  if [[ "$RELEASE_ENVIRONMENT" == production ]]; then
    [[ "${RELEASE_PRODUCTION_APPROVAL:-}" == "$RELEASE_VERSION" ]] || fail "Set RELEASE_PRODUCTION_APPROVAL to the reviewed release version"
    [[ "$RELEASE_EVIDENCE" == /* && -f "$RELEASE_EVIDENCE" ]] || fail "Production requires an absolute --evidence file"
    jq -e --arg version "$RELEASE_VERSION" --arg commit "$RELEASE_COMMIT" --arg digest "$RELEASE_MANIFEST_SHA" --arg action "$action" '
      .version == $version and .commit == $commit and .manifestSHA256 == $digest
      and .migrationCompatible == true and .backupVerified == true and .rollbackReady == true
      and (if $action == "rollback" then .applicationRollbackCompatible == true
           else .stagingSmokePassed == true and .mediaSmokePassed == true and .securityPassed == true end)
      and (.approvedBy | type == "string" and length > 0)
      and (.evidenceReferences | type == "array" and length > 0)' "$RELEASE_EVIDENCE" >/dev/null || fail "Release evidence does not satisfy production gates"
  fi
}

# acquire_release_lock сериализует операции над одним проектом на выбранном Docker-хосте.
# Каталог блокировок для удалённых окружений должен быть общим для всех операторов/CI.
acquire_release_lock() {
  local directory="${RELEASE_LOCK_DIR:-$RELEASE_ROOT/release-artifacts/locks}"
  [[ "$directory" == /* ]] || fail "RELEASE_LOCK_DIR must be absolute"
  mkdir -p "$directory"
  RELEASE_LOCK_PATH="$directory/$RELEASE_PROJECT.lock"
  mkdir "$RELEASE_LOCK_PATH" 2>/dev/null || fail "Another operation or stale lock exists for $RELEASE_PROJECT; inspect before removing its lock"
  trap 'rmdir "$RELEASE_LOCK_PATH"' EXIT
}

# wait_healthy ожидает контейнер и его readiness, не перезапуская его при ошибке.
# @args $1 — имя службы; тайм-аут задаёт RELEASE_HEALTH_TIMEOUT в секундах.
wait_healthy() {
  local service="$1" id state start=$SECONDS timeout="${RELEASE_HEALTH_TIMEOUT:-180}"
  [[ "$timeout" =~ ^[1-9][0-9]*$ ]] || fail "Invalid health timeout"
  while ((SECONDS - start < timeout)); do
    id="$(compose ps -q "$service")"
    if [[ -n "$id" ]]; then
      state="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$id")"
      [[ "$state" == healthy || "$state" == running ]] && return 0
      [[ "$state" == exited || "$state" == dead ]] && fail "$service stopped before readiness"
    fi
    sleep 2
  done
  fail "$service did not become ready; inspect private logs before retry"
}
