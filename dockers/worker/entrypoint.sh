#!/bin/sh
set -e

mkdir -p /storage/records /storage/tmp
chown -R app:app /storage

exec su-exec app:app /app/go-recorder-worker
