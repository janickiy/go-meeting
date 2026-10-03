#!/usr/bin/env bash
set -euo pipefail

DEPLOY_HOST="${DEPLOY_HOST:-}"
DEPLOY_USER="${DEPLOY_USER:-}"
DEPLOY_PORT="${DEPLOY_PORT:-22}"
DEPLOY_PATH="${DEPLOY_PATH:-}"

# CI deploys a source archive without .git, so carry its immutable revision into
# the Docker build explicitly. Never place env contents or credentials in this arg.
BUILD_VERSION="${BUILD_VERSION:-}"
if [[ -z "${BUILD_VERSION}" && -n "${CI_COMMIT_SHA:-}" ]]; then
  BUILD_VERSION="${CI_COMMIT_SHA:0:12}"
fi
if [[ -z "${BUILD_VERSION}" ]]; then
  BUILD_VERSION="$(git rev-parse --short=12 HEAD 2>/dev/null || true)"
fi
if [[ ! "${BUILD_VERSION}" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$ ]]; then
  echo "Set BUILD_VERSION to a short, safe release identifier" >&2
  exit 1
fi

if [[ -z "${DEPLOY_HOST}" || -z "${DEPLOY_USER}" || -z "${DEPLOY_PATH}" ]]; then
  echo "DEPLOY_HOST, DEPLOY_USER and DEPLOY_PATH are required" >&2
  exit 1
fi

ssh_command=(ssh -p "${DEPLOY_PORT}" -o StrictHostKeyChecking=accept-new)
rsync_ssh="ssh -p ${DEPLOY_PORT} -o StrictHostKeyChecking=accept-new"

if [[ -n "${DEPLOY_SSH_PRIVATE_KEY_B64:-}" || -n "${DEPLOY_SSH_PRIVATE_KEY:-}" ]]; then
  eval "$(ssh-agent -s)"
  mkdir -p "${HOME}/.ssh"
  chmod 700 "${HOME}/.ssh"
  if [[ -n "${DEPLOY_SSH_PRIVATE_KEY_B64:-}" ]]; then
    printf '%s' "${DEPLOY_SSH_PRIVATE_KEY_B64}" | base64 -d | tr -d '\r' | ssh-add -
  elif [[ -f "${DEPLOY_SSH_PRIVATE_KEY}" ]]; then
    tr -d '\r' < "${DEPLOY_SSH_PRIVATE_KEY}" | ssh-add -
  else
    printf '%s\n' "${DEPLOY_SSH_PRIVATE_KEY}" | tr -d '\r' | ssh-add -
  fi
elif [[ -n "${DEPLOY_PASSWORD:-}" ]]; then
  export SSHPASS="${DEPLOY_PASSWORD}"
  ssh_command=(sshpass -e ssh -p "${DEPLOY_PORT}" -o StrictHostKeyChecking=accept-new)
  rsync_ssh="sshpass -e ssh -p ${DEPLOY_PORT} -o StrictHostKeyChecking=accept-new"
else
  echo "Set DEPLOY_SSH_PRIVATE_KEY_B64, DEPLOY_SSH_PRIVATE_KEY or DEPLOY_PASSWORD in GitLab CI/CD variables" >&2
  exit 1
fi

remote="${DEPLOY_USER}@${DEPLOY_HOST}"

"${ssh_command[@]}" "${remote}" "mkdir -p '${DEPLOY_PATH}'"

tmp_env=""
cleanup() {
  if [[ -n "${tmp_env}" && -f "${tmp_env}" ]]; then
    rm -f "${tmp_env}"
  fi
}
trap cleanup EXIT

rsync -az --delete \
  --exclude '.git/' \
  --exclude '.idea/' \
  --exclude '.env' \
  --exclude '.DS_Store' \
  --exclude 'dockers/https/certs/' \
  --exclude 'dockers/postgres/data/' \
  --exclude 'dockers/redis/data/' \
  --exclude 'dockers/rabbitmq/data/' \
  --exclude 'dockers/minio/data/' \
  --exclude 'dockers/storage/data/' \
  -e "${rsync_ssh}" \
  ./ "${remote}:${DEPLOY_PATH}/"

if [[ -n "${DEPLOY_ENV_FILE_B64:-}" || -n "${DEPLOY_ENV_FILE:-}" ]]; then
  tmp_env="$(mktemp)"
  if [[ -n "${DEPLOY_ENV_FILE_B64:-}" ]]; then
    printf '%s' "${DEPLOY_ENV_FILE_B64}" | base64 -d > "${tmp_env}"
  elif [[ -f "${DEPLOY_ENV_FILE}" ]]; then
    cp "${DEPLOY_ENV_FILE}" "${tmp_env}"
  else
    printf '%s\n' "${DEPLOY_ENV_FILE}" > "${tmp_env}"
  fi
  rsync -az -e "${rsync_ssh}" "${tmp_env}" "${remote}:${DEPLOY_PATH}/.env"
fi

"${ssh_command[@]}" "${remote}" "cd '${DEPLOY_PATH}' && \
  if [ ! -f .env ]; then cp .env.example .env; fi && \
  BUILD_VERSION='${BUILD_VERSION}' docker compose build api worker media-worker frontend minio && \
  docker compose up -d --remove-orphans && \
  docker compose ps"
