#!/usr/bin/env bash
# Закрепляет образы digest/ID и архивирует локальную репетицию либо публикует registry-релиз.
set -euo pipefail
source "$(dirname "$0")/common.sh"
build= output= mode= security=true infrastructure_manifest=
while (($#)); do
  case "$1" in
    --build) build="$2"; shift 2;; --output) output="$2"; shift 2;;
    --mode) mode="$2"; shift 2;; --skip-security-local) security=false; shift;;
    --infrastructure-manifest) infrastructure_manifest="$2"; shift 2;;
    *) fail "Unknown package argument: $1";;
  esac
done
[[ "$mode" == registry || "$mode" == archive || "$mode" == local ]] || fail "--mode must be registry, archive or local"
[[ "$build" == /* && -f "$build" && "$output" == /* ]] || fail "Absolute --build and --output paths required"
[[ "$mode" == local || "$security" == true ]] || fail "A deployable release cannot skip security gates"
[[ "$mode" == local || "$(jq -r .sourceDirty "$build")" == false ]] || fail "Dirty source cannot be published as a release"
layout="$(jq -r '.deploymentLayout // "classic"' "$build")"
[[ "$layout" == classic || "$layout" == hardened-shared-host ]] || fail "Unsupported release layout"
select_release_services "$layout"
export IMAGE_PLATFORM="$(jq -r '.platform // ""' "$build")"
if [[ -n "$infrastructure_manifest" ]]; then
  [[ "$infrastructure_manifest" == /* && -f "$infrastructure_manifest" ]] || fail "--infrastructure-manifest must name an existing absolute path"
  jq -e --arg mode "$mode" '.schemaVersion == 1 and .artifactMode == $mode and (.images | type == "object")' "$infrastructure_manifest" >/dev/null || fail "Infrastructure manifest mode must match the package mode"
  dependencies=(minio postgres redis rabbitmq coturn proxy prometheus)
  if [[ "$layout" == classic ]]; then dependencies+=(grafana); fi
  for dependency in "${dependencies[@]}"; do
    dependency_ref="$(jq -er --arg service "$dependency" '.images[$service]' "$infrastructure_manifest")" || fail "Infrastructure manifest misses $dependency"
    if [[ "$mode" == local || "$mode" == archive ]]; then
      [[ "$dependency_ref" =~ ^sha256:[a-f0-9]{64}$ ]] || fail "Local infrastructure must use immutable image IDs"
    else
      [[ "$dependency_ref" =~ ^[a-zA-Z0-9][a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$ ]] || fail "Registry infrastructure must use immutable digests"
    fi
  done
fi
mkdir -p "$output"
[[ ! -e "$output/release.json" ]] || fail "Manifest already exists; choose a new package directory"
manifest="$(jq 'del(.registry) | .images={} | .sourceFingerprintVerified=true' "$build")"
if [[ -n "$infrastructure_manifest" ]]; then
  manifest="$(jq --arg sha "$(file_sha256 "$infrastructure_manifest")" --arg version "$(jq -r .version "$infrastructure_manifest")" '.infrastructureReused=true | .infrastructureManifestSHA256=$sha | .infrastructureVersion=$version' <<<"$manifest")"
fi
references=()
for service in "${RELEASE_SERVICES[@]}"; do
  reuse_infrastructure=false
  built_ref=
  case "$service" in
    minio|postgres|redis|rabbitmq|coturn|proxy|prometheus|grafana)
      if [[ -n "$infrastructure_manifest" ]]; then reuse_infrastructure=true; fi;;
  esac
  if [[ "$reuse_infrastructure" == true ]]; then
    ref="$(jq -er --arg service "$service" '.images[$service]' "$infrastructure_manifest")"
  else
  built_ref="$(jq -r --arg service "$service" '.images[$service] // ""' "$build")"
  if [[ -n "$built_ref" ]]; then ref="$built_ref";
  else
  case "$service" in
    postgres) ref=postgres:16-alpine;; redis) ref=redis:7-alpine;;
    rabbitmq) ref=rabbitmq:4.2.9-management;; coturn) ref=coturn/coturn:4.18.0-r0;;
    proxy) ref=nginx:1.30.5-alpine;; prometheus) ref=prom/prometheus:v3.5.0;; grafana) ref=grafana/grafana:12.2.0;;
    *) ref="$(jq -er --arg service "$service" '.images[$service]' "$build")";;
  esac
  fi
  if [[ "$layout" == hardened-shared-host && -z "$built_ref" ]]; then
    case "$service" in prometheus) ref=prom/prometheus:v3.15.0;; grafana) ref=grafana/grafana:13.2.3;; esac
  fi
  fi
  if ! docker image inspect "$ref" >/dev/null 2>&1; then
    if [[ "$reuse_infrastructure" == true ]]; then
      [[ "$mode" == registry ]] || fail "Infrastructure image missing for $service; load its archived release first"
      docker pull "$ref"
    else
      case "$service" in postgres|redis|rabbitmq|coturn|proxy|prometheus|grafana) docker pull "$ref";; *) fail "Built image missing: $service";; esac
    fi
  fi
  if [[ "$reuse_infrastructure" == false && -n "$built_ref" ]]; then
      docker image inspect "$ref" | jq -e --arg version "$(jq -r .version "$build")" --arg commit "$(jq -r .commit "$build")" --arg source "$(jq -r .sourceSHA256 "$build")" '
        .[0].Config.Labels | .["org.opencontainers.image.version"] == $version
        and .["org.opencontainers.image.revision"] == $commit' >/dev/null || fail "Built image provenance mismatch for $service"
      if ! docker image inspect "$ref" | jq -e --arg source "$(jq -r .sourceSHA256 "$build")" '.[0].Config.Labels["io.go-recorder.source-sha256"] == $source and $source != "null"' >/dev/null; then
        [[ "$mode" == local ]] || fail "Source fingerprint mismatch for $service"
        manifest="$(jq '.sourceFingerprintVerified=false' <<<"$manifest")"
      fi
  fi
  if [[ "$security" == true ]]; then
    [[ "${TRIVY_IMAGE:-}" =~ @sha256:[a-f0-9]{64}$ ]] || fail "TRIVY_IMAGE must pin the approved scanner by digest"
    # Критические и высокие находки блокируют пакет; полный SBOM сохраняется вместе с ним.
    bash "$RELEASE_ROOT/scripts/release/scan-image.sh" "$ref" "$service" "$output"
    manifest="$(jq --arg service "$service" --arg scan "$(file_sha256 "$output/$service.scan.json")" --arg sbom "$(file_sha256 "$output/$service.sbom.json")" '.securityEvidence[$service]={scanSHA256:$scan,sbomSHA256:$sbom}' <<<"$manifest")"
  fi
  if [[ "$reuse_infrastructure" == true ]]; then
    # Инфраструктура имеет собственное происхождение и не перепубликуется вместе с приложением.
    immutable="$ref"
  elif [[ "$mode" == registry ]]; then
    if [[ -n "$built_ref" ]]; then
      if docker manifest inspect "$ref" >/dev/null 2>&1; then fail "Release tag already exists in registry: $service; retrieve its previous manifest instead of replacing it"; fi
      docker push "$ref"
    fi
    repo="${ref%:*}"
    immutable="$(docker image inspect "$ref" | jq -er --arg repo "$repo" '.[0].RepoDigests[] | select(startswith($repo+"@"))' | head -1)"
    [[ -n "$immutable" ]] || fail "Registry digest missing for $service"
  else immutable="$(docker image inspect --format '{{.Id}}' "$ref")"; fi
  references+=("$ref")
  manifest="$(jq --arg service "$service" --arg ref "$immutable" '.images[$service]=$ref' <<<"$manifest")"
done
manifest="$(jq --arg mode "$mode" --argjson security "$security" '.artifactMode=$mode | .securityScanned=$security' <<<"$manifest")"
printf '%s\n' "$manifest" > "$output/release.json"
if [[ "$mode" == local || "$mode" == archive ]]; then
  save_options=()
  if [[ -n "$IMAGE_PLATFORM" ]]; then save_options=(--platform "$IMAGE_PLATFORM"); fi
  docker image save "${save_options[@]}" --output "$output/images.tar" "${references[@]}"
  if [[ "$mode" == archive ]]; then
    manifest="$(jq --arg sha "$(file_sha256 "$output/images.tar")" '.archiveSHA256=$sha' <<<"$manifest")"
    printf '%s\n' "$manifest" > "$output/release.json"
  fi
fi
for file in "$output/release.json" "$output/images.tar"; do
  [[ -f "$file" ]] || continue
  printf '%s  %s\n' "$(file_sha256 "$file")" "$(basename "$file")" >> "$output/SHA256SUMS"
done
printf 'Packaged immutable %s artifacts at %s.\n' "$mode" "$output"
