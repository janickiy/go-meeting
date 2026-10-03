# Развёртывание Meet

Текущая платформа — один Docker Compose-хост. Реального staging на момент
подготовки этапа 10 нет; локальная репетиция проверяет автоматизацию, но не
заменяет внешние DNS/TLS/TURN, firewall, реальные устройства и production capacity.
Состав сервисов и ограничения: [ARCHITECTURE.md](ARCHITECTURE.md).

## Окружения

| Среда | Конфигурация / проект | Назначение и границы |
| --- | --- | --- |
| Development | `docker-compose.yml`, локальный `.env` | Разработка, локальная сборка и отладка; тестовые пароли/публикации портов не переносятся в production |
| Integration test | `docker-compose.integration.yml`, `recorder-stage6` и явные test env | Изолированные проверки с тестовыми данными; не обновлять уже работающий проект без отдельного указания |
| Local release rehearsal | Production Compose + `docker-compose.release-local.yml`; `recorder-release-rehearsal` или такой префикс с суффиксом | Отдельные тома/порты, `APP_ENV=test`, loopback, временный сертификат; допускаются локальные image ID и помеченный dirty source |
| Staging | Production Compose; `recorder-staging` или суффикс | Реальные HTTPS/WSS/TURNS origins, отдельные секреты/данные, immutable registry digests, `APP_ENV=production`; сейчас требует предоставления инфраструктуры |
| Production | Production Compose; `recorder-production` или суффикс | Собственные DNS/TLS/хранилища, проверенный манифест, evidence и явное решение оператора |

У каждой среды свои PostgreSQL, Redis namespaces, RabbitMQ, MinIO bucket/данные,
provider credentials и worker identities. Нельзя направлять локальные тесты в
production-зависимости. Внешние email/push/calendar/STT/AI/embeddings по умолчанию
выключены; mock запрещён для staging/production. Матрица флагов и лимитов —
[API.md](API.md), шаблон — [`.env.production.example`](../.env.production.example).

Семантический поиск требует pgvector. Текущий package использует обычный
PostgreSQL 16 image, поэтому одного переключения `EMBEDDINGS_ENABLED` недостаточно:
нужен отдельно согласованный совместимый image/extension и проверка миграции.
Не подменяйте закреплённый образ БД во время application deploy ради этой функции.

## Подготовка хоста

Нужны Docker Engine/Desktop и Compose с поддержкой `!override`, Bash, `jq`,
`openssl`, `ripgrep`, утилита SHA-256; для сборки операторских бинарников —
поддерживаемая версия Go из `go.mod`. Скрипты вызываются через `bash`, права
исполнения у файла не предполагаются. Команды ниже выполняются из корня репозитория.

Для staging/production заранее подготовить:

- DNS, сертификаты HTTPS и TURNS, публичный SFU IP и совпадающие firewall/NAT
  порты из [архитектуры](ARCHITECTURE.md); один открытый порт не доказывает relay.
- Абсолютный env-файл вне Git, права `0600`, секреты из внешнего защищённого
  источника; не выводить `docker compose config` без `--quiet` и не включать `set -x`.
- `TLS_CERT_DIR` и `TURN_TLS_CERT_DIR` с `fullchain.pem`/`privkey.pem`.
  Ключ Coturn должен читаться его пользователем, но не быть общедоступным.
- `RECORDER_STORAGE_DIR` с владельцем `1000:1000` и правами `0750`, запасом места;
  директории объектов и БД на диске с проверенным резервированием.
- `METRICS_TOKEN_FILE` и `GRAFANA_PASSWORD_FILE` вне Git с правами чтения для
  соответствующих контейнеров; значение metrics token совпадает с `METRICS_SECRET`.
- Доступ к registry, чистый проверенный release manifest и предыдущий manifest.
  Все операторы одного Docker-хоста используют общий абсолютный `RELEASE_LOCK_DIR`.
  Локальный lock-каталог одного ноутбука не защищает от другого оператора.
- Выделенный обычный smoke-аккаунт и внешние `SMOKE_EMAIL`/`SMOKE_PASSWORD`;
  скрипт не создаёт аккаунт автоматически и отклоняет admin-аккаунт.

Production Compose не публикует БД, Redis, RabbitMQ и MinIO console. Prometheus и
Grafana доступны на loopback; организуйте защищённый доступ оператора. Внутренние
HTTP/AMQP/Redis предполагают доверенную сеть одного хоста; перенос на разные
хосты требует отдельного решения TLS/mTLS/VPN.

## Локальная репетиция без изменения существующих проектов

Выберите новые абсолютные каталоги, которых ещё нет. Init не изменяет проекты
`go-recorder` и `recorder-stage8` и не содержит команды удаления их данных.

```bash
LOCAL_RELEASE_DIR=/absolute/private/meet-rehearsal
bash scripts/release/init-local.sh --directory "$LOCAL_RELEASE_DIR"
```

Получатся `.env.local`, временный TLS-сертификат, секреты и storage. HTTP API
публикуется только на `127.0.0.1:28085`, HTTPS UI — `https://localhost:25482`,
SFU — `50220`, TURN — `23478` и relay `50300–50340`. Это локальная схема с
разрешённым loopback и без TURNS; её параметры нельзя считать production-шаблоном.
Init использует доступную контейнеру папку spool; для production обязательны
ограниченные права, указанные выше.

Для локального smoke доверяйте только созданному сертификату в рамках процесса:
`export SMOKE_CA_FILE="$LOCAL_RELEASE_DIR/certs/fullchain.pem"`. Этот PEM добавляется
к корням доверия HTTP/WS smoke-клиента, имя хоста и цепочка продолжают проверяться;
системное доверие не меняется. После проверки — `unset SMOKE_CA_FILE`. Для настоящего
staging/production нужны действительные доверенные сертификаты, не отключение TLS.

После [сборки и упаковки](RELEASE_PROCESS.md) укажите существующий манифест:

```bash
LOCAL_MANIFEST=/absolute/artifacts/package/release.json
bash scripts/release/release.sh validate --environment local --project recorder-release-rehearsal --env-file "$LOCAL_RELEASE_DIR/.env.local" --manifest "$LOCAL_MANIFEST"
bash scripts/release/release.sh migrate --environment local --project recorder-release-rehearsal --env-file "$LOCAL_RELEASE_DIR/.env.local" --manifest "$LOCAL_MANIFEST"
bash scripts/release/release.sh deploy --environment local --project recorder-release-rehearsal --env-file "$LOCAL_RELEASE_DIR/.env.local" --manifest "$LOCAL_MANIFEST"
```

Скрипты принимают только разрешённый проект для выбранной среды. `migrate`
поднимает зависимости и запускает явную migration-задачу. `deploy` не выполняет
DDL и не собирает образы. Если зависимости уже существуют, смена их image
запрещается: обновление PostgreSQL/Redis/RabbitMQ/MinIO/Coturn — отдельное окно.

## Staging и production

Перед выполнением команд замените все пути и адреса на проверенные реальные
значения. `validate` проверяет формат/Compose, но не доказывает доступность
DNS, сертификата, TURN, registry и восстановимость данных.

```bash
STAGING_CONFIG=/srv/meet/config/staging.env
RELEASE_MANIFEST=/srv/meet/releases/v1.2.3/release.json
bash scripts/release/release.sh validate --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
bash scripts/release/release.sh pull --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
bash scripts/release/release.sh migrate --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
bash scripts/release/release.sh deploy --environment staging --project recorder-staging --env-file "$STAGING_CONFIG" --manifest "$RELEASE_MANIFEST"
```

Для обновления существующего окружения перед `migrate` обязателен согласованный
[backup с write freeze](operations/backup-restore.md) и проверка миграций на копии.
Production использует те же операции с `--environment production`, проектом
`recorder-production`, собственным env и дополнительными approval/evidence
из [RELEASE_PROCESS.md](RELEASE_PROCESS.md). Нельзя подменять `production` на
`local` ради обхода gate.

Обновление приложений идёт последовательно:
перед ним ограничить новые старты записей, дождаться уже ожидающих `record.start`
и штатного завершения активных записей. `active=0` у процесса не означает пустую
очередь команд. Ограничение requeue и порядок проверки recorder drain описаны в
[runbook](operations/README.md#health-и-завершение).

Порядок замены:
`media-worker → worker → product-worker → live-worker → api → frontend → proxy`,
затем observability. Сначала выполняется drain, ожидается нулевая активность,
после запуска проверяется health. При тайм-ауте процесс остаётся draining,
принудительного restart нет. Это управляемая замена single-host сервисов с
возможным перерывом, а не обещание zero-downtime или canary traffic splitting.

Для возобновления после общего drain/backup существует `release.sh resume`
с тем же установленным manifest: он пересоздаёт процессы с теми же образами.
Если выпуск остановился посередине и образовалась смесь версий, сначала
сверьте фактические image ID каждого сервиса и процедуру [отката](ROLLBACK.md).
Не используйте `resume` с произвольным манифестом.

## Ротация секретов

Записывайте изменения в защищённый env/secret store, не в образ или Git.
Планируйте перезапуск потребителей и проверку `/version`, readiness, auth,
WS/медиа и доставки после каждой группы изменений.

| Секрет | Последствия и порядок |
| --- | --- |
| `JWT_SECRET` | Новый ключ отвергает старые JWT; потребители должны согласованно получить его. Logout отдельного пользователя не заменяет эту операцию |
| Media ticket/internal, recorder internal | Обновить API и соответствующие workers согласованно; старые tickets/вызовы могут перестать работать, понадобится reconnect |
| `METRICS_SECRET` | Согласованно обновить приложения и `METRICS_TOKEN_FILE`, затем проверить scrape; drain также использует этот secret |
| `TURN_SHARED_SECRET` | Согласовать Coturn и выдачу credentials в приложении, учесть TTL старых credentials и проверить новые relay-соединения |
| Пароли PostgreSQL/Redis/RabbitMQ/MinIO | Сначала запланировать изменение учётной записи самой зависимости, затем всех клиентов; изменение env не меняет пароль в существующем persistent storage автоматически |
| `PROVIDER_TOKEN_ENCRYPTION_KEY` | Не заменять вслепую: существующий ciphertext станет нечитаемым. Автоматического re-encryption/key ring нет; нужен отдельный план миграции токенов либо отключения и повторного подключения, с сохранением ключа для разрешённого восстановления backup |
| Provider/OAuth credentials | Ротация у провайдера, затем в потребителях; учесть повторы jobs, refresh/revoke и готовность внешнего gateway |

Наличие инструкции не означает выполненную ротацию или проверенный production
release. Итоговое решение принимается по [чеклисту запуска](PRODUCTION_LAUNCH_CHECKLIST.md).
