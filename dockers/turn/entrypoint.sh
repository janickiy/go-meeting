#!/bin/sh
# Передаёт только доверенную серверную конфигурацию; значения secrets не выводятся.
set -eu
: "${TURN_SHARED_SECRET:?TURN_SHARED_SECRET required}"
: "${TURN_PUBLIC_IP:?TURN_PUBLIC_IP required}"
: "${TURN_REALM:?TURN_REALM required}"
exec turnserver -c /etc/coturn/turnserver.conf \
  --static-auth-secret="$TURN_SHARED_SECRET" \
  --external-ip="$TURN_PUBLIC_IP" --realm="$TURN_REALM" \
  --min-port="${TURN_MIN_PORT:-49160}" --max-port="${TURN_MAX_PORT:-49259}"
