#!/bin/sh
# Совместимость официального entrypoint PostgreSQL с минимальной утилитой su-exec.
# @args $1 — пользователь либо UID:GID; последующие аргументы — запускаемая команда.
set -eu
exec /sbin/su-exec "$@"
