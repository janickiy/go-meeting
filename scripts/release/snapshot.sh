#!/usr/bin/env bash
# Создаёт отдельный чистый source snapshot текущей рабочей копии для выпуска.
# Индекс, коммиты и незакоммиченные изменения исходного репозитория не изменяются.
set -euo pipefail
source "$(dirname "$0")/common.sh"
[[ $# == 2 && "$1" == --directory && "$2" == /* && ! -e "$2" ]] || fail "Usage: snapshot.sh --directory /absolute/new/private/directory"
snapshot_directory="$2"
umask 077
mkdir -p "$snapshot_directory/source"
origin_commit="$(git -C "$RELEASE_ROOT" rev-parse HEAD)"
origin_dirty=false
[[ -z "$(git -C "$RELEASE_ROOT" status --porcelain)" ]] || origin_dirty=true
# Берутся текущие байты tracked/untracked source; env, ключи и Python cache не публикуются.
git -C "$RELEASE_ROOT" ls-files --cached --others --exclude-standard -z |
  rsync -a --from0 --files-from=- --include='.env*.example' --exclude='.env*' \
    --exclude='secrets/**' --exclude='*.pem' --exclude='*.key' --exclude='*.pyc' \
    --exclude='.DS_Store' "$RELEASE_ROOT/" "$snapshot_directory/source/"
git -C "$snapshot_directory/source" init -q -b codex/deployment-snapshot
git -C "$snapshot_directory/source" add --all
git -C "$snapshot_directory/source" -c user.name='Meetrix release snapshot' \
  -c user.email='release-snapshot@localhost' commit -qm 'Immutable snapshot of approved working tree'
snapshot_commit="$(git -C "$snapshot_directory/source" rev-parse HEAD)"
[[ -z "$(git -C "$snapshot_directory/source" status --porcelain)" ]] || fail "Snapshot is unexpectedly dirty"
jq -n --arg originCommit "$origin_commit" --arg snapshotCommit "$snapshot_commit" \
  --argjson originDirty "$origin_dirty" \
  '{schemaVersion:1,mode:"working-tree-snapshot",originCommit:$originCommit,originDirty:$originDirty,snapshotCommit:$snapshotCommit,originalRepositoryModified:false}' \
  > "$snapshot_directory/provenance.json"
printf 'Created isolated clean source snapshot %s. Original repository unchanged.\n' "$snapshot_commit"
