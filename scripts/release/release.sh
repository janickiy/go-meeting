#!/usr/bin/env bash
# Управляет выпуском закреплённых артефактов без пересборки исходников на сервере.
set -euo pipefail
source "$(dirname "$0")/common.sh"
action="${1:-}"
[[ -n "$action" ]] || fail "Usage: release.sh validate|pull|migrate|drain|resume|deploy|rollback --environment ... --env-file ... --manifest ... --project ..."
shift
parse_release_args "$@"
validate_release

# pull_images получает только digest из манифеста; локальные ID уже должны существовать.
pull_images() {
  if [[ "$RELEASE_ARTIFACT_MODE" == archive ]]; then
    # Архив и все отчёты уже проверены validate_release до загрузки в Docker.
    docker image load --quiet --input "$(dirname "$RELEASE_MANIFEST")/images.tar"
    verify_loaded_images
  elif [[ "$RELEASE_ENVIRONMENT" == local ]]; then
    local service ref
    for service in "${RELEASE_SERVICES[@]}"; do
      ref="$(jq -r --arg service "$service" '.images[$service]' "$RELEASE_MANIFEST")"
      docker image inspect "$ref" >/dev/null || fail "Local artifact missing for $service; load the archived release"
    done
  else compose --profile observability pull --ignore-buildable; fi
}

# start_dependencies допускает первоначальный запуск, но не обновляет базы и хранилища скрытым образом.
start_dependencies() {
  local service id current desired
  for service in postgres redis rabbitmq minio coturn; do
    id="$(compose ps -aq "$service")"
    desired="$(jq -r --arg service "$service" '.images[$service]' "$RELEASE_MANIFEST")"
    if [[ -n "$id" ]]; then
      current="$(docker inspect --format '{{.Config.Image}}' "$id")"
      [[ "$current" == "$desired" ]] || fail "$service image changed: perform its documented maintenance/backup procedure first"
    fi
    compose up -d --no-build --no-deps --no-recreate "$service"
    wait_healthy "$service"
  done
}

# drain_service закрывает приём работы и ждёт нулевой активности до пересоздания контейнера.
drain_service() {
  local service="$1" port="$2" id response start=$SECONDS timeout="${RELEASE_DRAIN_TIMEOUT:-300}"
  [[ "$timeout" =~ ^[1-9][0-9]*$ ]] || fail "Invalid drain timeout"
  id="$(compose ps -q "$service")"
  [[ -n "$id" ]] || return 0
  [[ "$(docker inspect --format '{{.State.Running}}' "$id")" == true ]] || return 0
  compose exec -T "$service" sh -c 'wget -qO- --post-data="" --header="Authorization: Bearer $METRICS_SECRET" "http://127.0.0.1:$1/operations/drain"' sh "$port" >/dev/null || fail "$service cannot enter drain; legacy upgrade requires an explicit maintenance window"
  while ((SECONDS - start < timeout)); do
    response="$(compose exec -T "$service" sh -c 'wget -qO- --header="Authorization: Bearer $METRICS_SECRET" "http://127.0.0.1:$1/operations/drain"' sh "$port")" || fail "Cannot inspect $service drain state"
    if jq -e '.draining == true and .active == 0' <<<"$response" >/dev/null; then return 0; fi
    sleep 2
  done
  fail "$service is still active and remains draining; no forced restart performed"
}

# deploy_apps обновляет службы последовательно; совместимость БД проверяется до вызова.
deploy_apps() {
  local service port id desired current force="${1:-false}"
  for service in "${RELEASE_APPLICATIONS[@]}"; do
    id="$(compose ps -q "$service")"
    desired="$(jq -r --arg service "$service" '.images[$service]' "$RELEASE_MANIFEST")"
    if [[ -n "$id" ]]; then
      current="$(docker inspect --format '{{.Config.Image}}' "$id")"
      if [[ "$force" == true && "$current" != "$desired" ]]; then fail "resume requires the currently installed manifest"; fi
      if [[ "$force" == false && "$current" == "$desired" ]]; then wait_healthy "$service"; continue; fi
    fi
    case "$service" in media-worker) port=8091;; worker) port=8090;; product-worker) port=8092;; live-worker) port=8093;; api) port=8085;; frontend) port=;; esac
    if [[ -n "$port" ]]; then drain_service "$service" "$port"; fi
    if [[ "$force" == true ]]; then compose up -d --no-build --no-deps --force-recreate "$service";
    else compose up -d --no-build --no-deps "$service"; fi
    wait_healthy "$service"
  done
  compose up -d --no-build --no-deps proxy
  wait_healthy proxy
  observability_services=(prometheus)
  if [[ "$RELEASE_LAYOUT" == classic ]]; then observability_services+=(grafana); fi
  compose --profile observability up -d --no-build --no-deps "${observability_services[@]}"
  printf 'Release %s deployed to %s; run smoke and record the observation window.\n' "$RELEASE_VERSION" "$RELEASE_PROJECT"
}

case "$action" in
  validate) printf 'Manifest and Compose valid: %s (%s).\n' "$RELEASE_VERSION" "$RELEASE_ENVIRONMENT";;
  pull) pull_images;;
  migrate) require_release_approval migrate; acquire_release_lock; pull_images; start_dependencies; compose --profile migration run --rm --no-deps migrate;;
  drain)
    require_release_approval drain; acquire_release_lock
    drain_service api 8085
    drain_service media-worker 8091
    drain_service worker 8090
    drain_service product-worker 8092
    drain_service live-worker 8093
    printf 'Application processes drained. Keep the write freeze until backup finishes.\n'
    ;;
  deploy) require_release_approval deploy; acquire_release_lock; pull_images; start_dependencies; deploy_apps;;
  resume) require_release_approval resume; acquire_release_lock; pull_images; start_dependencies; deploy_apps true;;
  rollback) require_release_approval rollback; acquire_release_lock; pull_images; start_dependencies; deploy_apps;;
  *) fail "Unknown release action: $action";;
esac
