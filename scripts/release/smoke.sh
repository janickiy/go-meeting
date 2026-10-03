#!/usr/bin/env bash
# Запускает проверку артефакта с отдельным тестовым аккаунтом и конечным временем ожидания.
set -euo pipefail
source "$(dirname "$0")/common.sh"
smoke_url= smoke_binary= exercise=false args=()
while (($#)); do
  case "$1" in
    --exercise) exercise=true; shift; continue;;
    --base-url|--binary) (($# >= 2)) || fail "Missing argument value";;
    *) (($# >= 2)) || fail "Missing argument value";;
  esac
  case "$1" in
    --base-url) smoke_url="$2";;
    --binary) smoke_binary="$2";;
    *) args+=("$1" "$2");;
  esac
  shift 2
done
parse_release_args "${args[@]}"
validate_release
[[ "$smoke_binary" == /* && -x "$smoke_binary" ]] || fail "--binary must point to the reviewed release-smoke artifact; do not rebuild during release"
[[ -n "$smoke_url" && -n "${SMOKE_EMAIL:-}" && -n "${SMOKE_PASSWORD:-}" ]] || fail "Public origin and external dedicated smoke account are required"
options=(--environment "$RELEASE_ENVIRONMENT" --base-url "$smoke_url" --version "$RELEASE_VERSION" --commit "$RELEASE_COMMIT")
if [[ "$exercise" == true ]]; then options+=(--exercise); fi
"$smoke_binary" "${options[@]}"
