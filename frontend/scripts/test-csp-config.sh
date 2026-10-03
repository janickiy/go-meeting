#!/bin/sh
set -eu

script=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)/15-csp-origins.envsh

check_valid() {
    expected=$1
    shift
    actual=$(env -i PATH="$PATH" "$@" sh -c '. "$1"; printf "%s|%s|%s|%s" "$CSP_WS_ORIGIN" "$CSP_EXTRA_WS_ORIGIN" "$CSP_EXTRA_WS_ORIGIN_2" "$CSP_STORAGE_ORIGIN"' sh "$script")
    [ "$actual" = "$expected" ] || {
        echo "CSP source mismatch" >&2
        exit 1
    }
}

check_invalid() {
    if env -i PATH="$PATH" "$@" sh -c '. "$1"' sh "$script" >/dev/null 2>&1; then
        echo "Unsafe CSP origin was accepted" >&2
        exit 1
    fi
}

check_valid 'wss://meet.example.invalid|||https://media.example.invalid:9443' \
    APP_ENV=production PUBLIC_FRONTEND_URL=https://meet.example.invalid \
    MINIO_PUBLIC_ENDPOINT=https://media.example.invalid:9443
check_valid 'wss://localhost:18482|ws://localhost:5173|ws://127.0.0.1:5173|http://localhost:9000' \
    APP_ENV=local PUBLIC_FRONTEND_URL=https://localhost:18482 \
    MINIO_PUBLIC_ENDPOINT=http://localhost:9000 CSP_EXTRA_WS_ORIGIN=ws://localhost:5173 \
    CSP_EXTRA_WS_ORIGIN_2=ws://127.0.0.1:5173
check_invalid APP_ENV=production PUBLIC_FRONTEND_URL=https://meet.example.invalid \
    'MINIO_PUBLIC_ENDPOINT=https://media.example.invalid;script-src-unsafe-inline'
check_invalid APP_ENV=production PUBLIC_FRONTEND_URL=https://meet.example.invalid \
    MINIO_PUBLIC_ENDPOINT=https://media.example.invalid/path
check_invalid APP_ENV=production PUBLIC_FRONTEND_URL=https://meet.example.invalid \
    MINIO_PUBLIC_ENDPOINT=https://user:password@media.example.invalid
check_invalid APP_ENV=production PUBLIC_FRONTEND_URL=http://meet.example.invalid \
    MINIO_PUBLIC_ENDPOINT=https://media.example.invalid
check_invalid APP_ENV=production PUBLIC_FRONTEND_URL=https://meet.example.invalid \
    MINIO_PUBLIC_ENDPOINT=https://media.example.invalid CSP_EXTRA_WS_ORIGIN=ws://wildcard.example.invalid
check_invalid APP_ENV=local PUBLIC_FRONTEND_URL=http://localhost:5173 \
    MINIO_PUBLIC_ENDPOINT=http://media.example.invalid
check_invalid APP_ENV=local PUBLIC_FRONTEND_URL=https://localhost:18482 \
    'CSP_EXTRA_WS_ORIGIN_2=ws://127.0.0.1:5173;script-src-unsafe-inline'

echo "CSP origin validation passed"
