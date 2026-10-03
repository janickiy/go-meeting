#!/usr/bin/env bash
# Проверяет локальный или удалённый Docker-образ через архив, не передавая сканеру доступ к Docker API.
# Docker CLI сохраняет DOCKER_HOST/DOCKER_TLS_*; docker cp работает и с TLS-защищённым DinD.
set -euo pipefail
source "$(dirname "$0")/common.sh"
ref="${1:-}" service="${2:-}" output="${3:-}"
[[ -n "$ref" && "$service" =~ ^[a-z][a-z0-9-]*$ && "$output" == /* && -d "$output" ]] || fail "Usage: scan-image.sh IMAGE SERVICE ABSOLUTE_OUTPUT_DIRECTORY"
[[ "${TRIVY_IMAGE:-}" =~ @sha256:[a-f0-9]{64}$ ]] || fail "TRIVY_IMAGE must pin the approved scanner by digest"
scan_archive= scan_container=

# cleanup_scan удаляет только созданные этой проверкой контейнер и временный архив.
cleanup_scan() {
  if [[ -n "$scan_container" ]]; then docker rm -f "$scan_container" >/dev/null 2>&1 || true; fi
  if [[ -n "$scan_archive" ]]; then rm -f "$scan_archive"; fi
}
trap cleanup_scan EXIT
scan_archive="$(mktemp "${TMPDIR:-/tmp}/recorder-image-scan.XXXXXX")"
save_options=()
if [[ -n "${IMAGE_PLATFORM:-}" ]]; then
  [[ "$IMAGE_PLATFORM" == linux/amd64 || "$IMAGE_PLATFORM" == linux/arm64 ]] || fail "Invalid scan platform"
  save_options=(--platform "$IMAGE_PLATFORM")
fi
docker image save "${save_options[@]}" --output "$scan_archive" "$ref"
# Повторно используется только публичная база уязвимостей; данные приложения остаются во временном контейнере.
docker volume create recorder-release-scanner-cache >/dev/null
scan_container="$(docker create --mount type=volume,source=recorder-release-scanner-cache,target=/root/.cache/trivy \
  --entrypoint /bin/sh "$TRIVY_IMAGE" -ec '
    trivy image --quiet --input /tmp/release-image.tar --format cyclonedx --output /tmp/release.sbom.json
    trivy image --quiet --input /tmp/release-image.tar --severity HIGH,CRITICAL --exit-code 1 --format json --output /tmp/release.scan.json
  ')"
docker cp "$scan_archive" "$scan_container:/tmp/release-image.tar"
docker start -a "$scan_container" || true
scan_status="$(docker inspect --format '{{.State.ExitCode}}' "$scan_container")"
docker cp "$scan_container:/tmp/release.sbom.json" "$output/$service.sbom.json" || true
docker cp "$scan_container:/tmp/release.scan.json" "$output/$service.scan.json" || true
[[ "$scan_status" == 0 ]] || fail "Image security gate failed for $service; inspect its scan artifact"
jq -e '.bomFormat == "CycloneDX"' "$output/$service.sbom.json" >/dev/null || fail "Missing SBOM for $service"
jq -e 'type == "object" and has("SchemaVersion")' "$output/$service.scan.json" >/dev/null || fail "Missing vulnerability report for $service"
