#!/usr/bin/env bash
# Обновляет копию сертификата только этого проекта после успешного renew.
# @args RENEWED_LINEAGE — каталог сертификата, переданный Certbot.
set -euo pipefail
[[ ${RENEWED_LINEAGE:-} == /etc/letsencrypt/live/meeting.janickiy.com ]] || exit 0
install -m 0644 -o root -g 1000 "$RENEWED_LINEAGE/fullchain.pem" /etc/meetrix/certs/fullchain.pem
install -m 0640 -o root -g 1000 "$RENEWED_LINEAGE/privkey.pem" /etc/meetrix/certs/privkey.pem
apache2ctl configtest
systemctl reload apache2
for project in recorder-staging-meeting recorder-production-meeting; do
  id="$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter 'label=com.docker.compose.service=coturn')"
  if [[ -n "$id" ]]; then docker restart "$id" >/dev/null; fi
done
