# Эксплуатация Go Recorder — этап 6

Дата проверки: 1–2 октября 2026. Это single-host Compose, не HA-платформа.
Состояние приёмки и фактические числа: [CAPACITY_BASELINE](../CAPACITY_BASELINE.md),
[отчёт этапа](production-hardening-report.md). Этап 7 не запускается автоматически.

## Запуск production

1. Скопировать `.env.production.example` в неотслеживаемый `.env.production`.
   Заполнить все пустые поля. Для каждого JWT/media/internal/metrics/TURN ключа
   сгенерировать отдельное значение `openssl rand -hex 32`. Пароли зависимостей
   также генерировать независимо; образцы не содержат готовых паролей.
2. Указать действительные `WS_ALLOWED_ORIGINS`, `MINIO_PUBLIC_ENDPOINT`,
   `TURN_URLS`, `TURN_REALM`, публичный IPv4 `MEDIA_NAT_IPS` и `TURN_PUBLIC_IP`.
   `example.invalid` — только обозначение, не рабочий адрес.
3. Подготовить действительные HTTPS и TURN TLS сертификаты. В каталогах
   `TLS_CERT_DIR` / `TURN_TLS_CERT_DIR` должны находиться `fullchain.pem` и
   `privkey.pem`. Coturn работает как nobody; ключ должен быть читаем этим
   пользователем, но не всеми пользователями хоста. Не копировать сертификаты
   или `.env.production` внутрь образа. Они исключены из build context.
4. Подготовить `RECORDER_STORAGE_DIR`, доступный UID/GID `1000:1000` с правами
   0750. Production recorder запускается без root entrypoint. По умолчанию
   требуется запас 1 ГиБ; размер запаса не означает лимит размера записи.
5. Выполнить `docker compose --env-file .env.production -f docker-compose.production.yml config --quiet`.
   Затем `docker compose --env-file .env.production -f docker-compose.production.yml up -d --build`.
   Не публиковать вывод `config` без `--quiet`: там присутствуют secrets.
6. Проверить readiness всех трёх служб, приватность бакета и внешнюю конференцию
   с `TURN_FORCE_RELAY=true`. Затем вернуть `TURN_FORCE_RELAY=false` и повторить
   с обеими реальными сетями. Миграции выполняются при старте; развёртывание
   API/recorder лучше производить последовательно, особенно при изменении схемы.

Внутренний транспорт на одном доверенном Compose-хосте — HTTP/AMQP/Redis без
TLS, не опубликованный наружу. Между отдельными хостами нужны TLS/mTLS/VPN и
отдельный security review. Значения CPU/RAM в Compose — начальные защитные
ограничения, а не результат production capacity planning.

## Порты и границы сети

| Назначение | Порт | Доступ |
|---|---|---|
| HTTPS/WSS | 443/TCP | Браузеры → Nginx |
| API | 8085/TCP | Только Compose/LB внутри доверенной сети |
| media-worker control | 8091/TCP | Только API/recorder, Bearer service secret |
| recorder control | 8090/TCP | Только API, Bearer service secret |
| SFU ICE mux | 50010/UDP и TCP по умолчанию | Публичное назначение `MEDIA_NAT_IPS`, прямой RTP |
| TURN | 3478/UDP + TCP | Браузеры → Coturn |
| TURNS | 5349/TCP | TLS fallback, отдельный действительный сертификат |
| TURN relay | 49160–49259/UDP по умолчанию | Coturn → SFU; диапазон должен совпасть с firewall |
| PostgreSQL / Redis / Rabbit / MinIO | 5432 / 6379 / 5672 / 9000 | Не опубликованы в production |
| Metrics / pprof | `/metrics` / loopback `PPROF_PORT` | Метрики требуют Bearer; pprof выключен по умолчанию |

RTP не проходит через HTTP reverse proxy. TCP fallback здесь означает TCP между
браузером и TURN; TURN→SFU обычно остаётся UDP. Не закрывать этот внутренний UDP
маршрут. Перенаправить SFU mux 1:1 через NAT, без случайной смены внешнего порта.
Для `MEDIA_UDP_MIN_PORT/MAX_PORT` вместо mux нужен явный соответствующий диапазон
в Compose/firewall; production пример использует именно один mux.

Coturn production запрещает private/loopback/link-local/multicast peer addresses,
чтобы не стать прокси к внутренним сервисам. Если TURN и SFU на одном хосте и
hairpin NAT недоступен, разрешить **только конкретный** адрес SFU через
`allowed-peer-ip`, а не весь private CIDR. Тестовый стенд имеет отдельные
послабления для loopback и никогда не используется как production конфигурация.

`HTTP_TRUSTED_PROXIES` пустой по умолчанию: forwarded IP не доверяются. Для
production задать только фактический IP/CIDR Nginx/LB, иначе IP rate limits могут
считать весь трафик одним proxy-клиентом. При внешнем LB нужно согласовать цепочку
доверия; Nginx пример сам устанавливает `X-Forwarded-For=$remote_addr`.

## TURN и ICE

`GET /api/v1/webrtc/config` доступен только с пользовательским JWT. Ответ содержит
`iceServers`, `iceTransportPolicy` и Unix `expiresAt`, без shared secret. Username
— `expiry:random UUID`; password — Base64(HMAC-SHA1(shared secret, username)),
совместимый с [Coturn REST](https://github.com/coturn/coturn/blob/master/README.turnserver).
Это не алгоритм хеширования паролей пользователей. TTL по умолчанию 10 минут,
допустимый диапазон 1–60 минут. Часы API/media-worker/Coturn должны быть
синхронизированы. Для нового/recreated PeerConnection браузер получает новые
credentials через `media.joined`. Автоматического ICE restart/обновления TURN
credentials внутри уже живого PeerConnection пока нет; долгие встречи нужно
проверить через полный TURN allocation refresh на целевой сети.

Candidate `host` — локальный адрес, `srflx` — адрес после STUN/NAT, `relay` —
выделенный Coturn адрес. При диагностике смотреть **выбранную** пару, а не только
наличие relay кандидата. `TURN_FORCE_RELAY=true` применяется к новым соединениям.
Shared secret никогда не передаётся браузеру. Для ротации ключа согласовать
перекрытие старого/нового секрета на Coturn и TTL на API, иначе новые подключения
или refresh могут прекратиться. Синхронная ротация без перекрытия — операция с
возможным разрывом соединения, не бесшовная.

При `401/438` проверить часы, общий ключ, realm, TTL и transport URI. При timeout
проверить DNS, 3478/5349, relay диапазон, public address, Docker NAT и разрешённый
маршрут relay→SFU. TURNS требует доверенную цепочку и соответствие имени
сертификата. Принудительный relay проверен локально по UDP/TCP; WAN, мобильная и
корпоративная сети, реальный TLS сертификат проверяются оператором отдельно.

## Health и завершение

`/health/live` отвечает 200, пока процесс жив; сбой БД не делает liveness ложным.
`/health/ready` отвечает 200 или 503. Пробы выполняются каждые 2 секунды с общим
дедлайном 1 секунда, а не по каждому запросу health. Тексты ошибок/адреса/ключи
не публикуются. Readiness API: PG, Redis, Rabbit, MinIO; media: Redis и lease
ownership; recorder/compositor: PG, Redis, Rabbit consumer, MinIO и локальный диск.
Compositor встроен в recorder, отдельного процесса/health порта у него нет.

Примеры, не выводящие secrets:

```sh
docker compose --env-file .env.production -f docker-compose.production.yml exec -T api wget -qO- http://127.0.0.1:8085/health/ready
docker compose --env-file .env.production -f docker-compose.production.yml exec -T media-worker wget -qO- http://127.0.0.1:8091/health/ready
docker compose --env-file .env.production -f docker-compose.production.yml exec -T worker wget -qO- http://127.0.0.1:8090/health/ready
```

Перед restart убрать экземпляр из внешнего LB и дождаться отсутствия новой
нагрузки. SIGTERM устанавливает draining/readiness=false, API перестаёт принимать
новые запросы и закрывает hijacked WS, media освобождает комнаты/PC/leases,
recorder завершает обработку в ограниченное время. `SHUTDOWN_TIMEOUT=20s`, Compose
grace period 45s. После дедлайна процесс может завершиться без полной финализации,
закрытые фрагменты сохраняются. Live migration комнат между SFU не реализована:
при смене media-worker требуется reconnect, повторное получение ticket и новые PC.
Redis PubSub ephemeral; клиент после reconnect получает HTTP snapshot/history.

## Диагностика и метрики

JSON логи имеют `service`, `instance_id`, `event_type`; HTTP получает UUID
`X-Request-ID`, внутренние media команды и Rabbit сообщения сохраняют correlation
где применимо. WS media использует UUID конверта как correlation ID; это не
тождественно ID HTTP handshake. Entity IDs допустимы в ограниченных логах,
**не** в metric labels. SQL query logging выключен, чтобы не писать текст чата.
Не включать dump SDP, JWT, tickets, upload/download query или тела чата.

`/metrics` без `Authorization: Bearer METRICS_SECRET` возвращает 404. Публичный
production Nginx закрывает metrics/internal/debug/health. Включение стека:

```sh
docker compose --env-file .env.production -f docker-compose.production.yml --profile observability up -d prometheus grafana
```

Предварительно положить token в `METRICS_TOKEN_FILE`, совпадающий с METRICS_SECRET,
и отдельный пароль Grafana в `GRAFANA_PASSWORD_FILE`. Файлы вне Git, доступны
пользователю контейнера; не сохранять значения в dashboard или Prometheus YAML.
UI доступны только на loopback 19090/13001, использовать SSH tunnel.
`dockers/observability/` содержит starter dashboard и alert rules. Порог DB 18
соответствует pool 20; при изменении пула скорректировать alert. Общие CPU/RAM
пороги требуют настройки под роль службы. Доставка alert через Alertmanager
не настроена; необходимо подключить свой канал уведомлений.

Основные серии: `recorder_http_requests_total{route,method,status}` (401/403 auth,
429 limits), `recorder_http_duration_seconds`, `recorder_ws_active`,
`recorder_ws_messages_total{class}`, `recorder_dependency_up{dependency}`,
`recorder_events_total{event}`, `recorder_work_duration_seconds{operation}`,
`recorder_state{resource}`, Go/process collectors. Route — шаблон, не URL с ID.
State содержит DB pool, media rooms/peers/tracks/subscriptions/RTP, активные
recordings/FFmpeg, disk free и последнюю queue latency. Счётчики SFU представлены
монотонными snapshot gauges, при рестарте обнуляются; использовать `increase`
с пониманием reset. Probe failure count — число неудачных проб, не число всех
ошибок запросов зависимости. `media_relay_total` / `media_direct_total` учитывают
первую выбранную пару физического подключения: TURN на любой стороне — relay,
host/srflx/prflx — direct. Это не статистика переключений пары внутри одного PC.
Waiting/online глобально и Rabbit queue depth пока не экспортируются; queue depth
смотреть в брокере.
Такие ограничения перечислены в отчёте, не выдаются за реализованные метрики.

Pprof: `PPROF_PORT=0` по умолчанию; ненулевой порт слушает `127.0.0.1` **внутри**
контейнера. Не добавлять port mapping или публичный proxy маршрут. Временный
admin доступ через `docker compose exec`/внутренний namespace. Сервер отклоняет
`seconds` вне (0,60], включая NaN/Inf; запросы ограничивать 30–60 секундами.
Выключить порт после диагностики.

## Лимиты и хранилище

WS physical limit 1000/процесс, queue/message/SDP/ICE limits — `WS_*`. SFU:
`MEDIA_MAX_PEERS/ROOMS/PUBLISHED_TRACKS/AUDIO_TRACKS/VIDEO_TRACKS/SCREEN_SHARERS`,
очереди RTP и deadlines — `MEDIA_*`. Recorder: `RECORDING_MAX_ACTIVE`,
`RECORDING_FFMPEG_CONCURRENCY`, `MAX_DURATION`, `MAX_MIB`, `DISK_RESERVE_MIB`.
Legacy ingest также ограничен MaxActive. JSON max 1 МиБ, API read timeout 60s;
upload route имеет отдельный лимит 10 МиБ и собственный deadline. SSE/WS имеют
отдельные heartbeat/write deadlines, поэтому общий HTTP WriteTimeout не используется.

PG pool 20 open/10 idle/30m lifetime. Подключение 5s, query timeout 10s,
lock timeout 5s. Длинные миграции выполнять отдельно с осознанным
`DB_QUERY_TIMEOUT`, не увеличивая лимит всех пользовательских запросов без review.
Redis dial/read/write/pool 3s, context cancellation; Rabbit heartbeat 5s,
connect bounded, publisher confirms/reconnect, consumer QoS=1/reconnect.
Команда: одна повторная доставка, затем durable `.failed` queue (7 суток,
10000 сообщений и 64 МиБ). Повторный старт/стоп должен оставаться идемпотентным.
Общие business ошибки тоже попадают в retry/quarantine; не переотправлять DLQ
вслепую. Сначала исправить причину и проверить состояние record в PG.

Для backlog alert подключить собственный Rabbit management/exporter в закрытой
сети: следить за messages_ready/unacknowledged, consumers=0 и возрастом команды.
Начальный рекомендационный порог messages_ready>100 в течение 5m требует
пересмотра по длительности записей и нагрузке; он не включён как «работающий»
Prometheus rule без соответствующей серии/exporter.

MinIO операции имеют сетевые deadlines, bounded SDK retry, upload deadline 10m,
проверку размера сохранённого объекта. При частичной composite финализации
незакоммиченный token-prefix удаляется best effort (5s). При outage очистка может
не пройти; сверять такие orphan objects с PG перед удалением. Повторный upload
одного object key идемпотентен, ready ставится только после подтверждения артефактов.
Бакет приватный; не включать anonymous access. Ссылки имеют TTL и проверку доступа.

FFmpeg вызывается напрямую, без shell. Новые bounded wrappers сохраняют до
64 КиБ вывода. Cancel: SIGTERM, через 3s SIGKILL; legacy normal stop сначала
отправляет `q`, затем TERM/KILL с ограниченным ожиданием. Контейнер `init: true`
помогает reap дочерних процессов. Codecs: VP8/Opus ingest, H.264/AAC MP4 и JPEG
preview; runtime FFmpeg должен содержать libvpx/libx264/libopus/AAC.
Low disk снимает readiness, но не является полноценной квотой записи в процессе:
следить за диском и заранее прекращать admission. Приложение не удаляет чужие
или неопознанные каталоги при запуске; завершённые chunks нужны для восстановления.

## Аварии и восстановление

См. [FAILURE_TESTS](FAILURE_TESTS.md) для фактически выполненной матрицы и рисков.

При recording failed не подменять статус success. Проверить worker readiness,
lease/token, свободный диск, retained chunks, FFmpeg stderr, MinIO и outbox.
Перезапуск может финализировать завершённые фрагменты, но не восстановит потерянный
RTP или незакрытый хвостовой chunk. Не копировать файлы между record/token
каталогами без проверки ownership. Для Rabbit backlog:

```sh
docker compose --env-file .env.production -f docker-compose.production.yml exec -T rabbitmq rabbitmqctl list_queues name messages_ready messages_unacknowledged consumers
```

PG saturation: посмотреть pool metrics/lock waits, `pg_stat_activity` в закрытом
admin-сеансе без вывода SQL с приватным контентом, active long transactions;
не начинать с массового увеличения max_connections. Redis outage: восстановить
Redis, дождаться новых leases, клиенты reconnect+snapshot; PubSub не replay.
MinIO outage: восстановить storage/диск/credentials, сверить PG status и objects.

## Backup, restore и rollback

Сохранять PostgreSQL (pg_dump/custom-format + проверка restore), MinIO objects и
metadata, а также secrets/config/TLS вне Git в защищённом хранилище. Redis presence,
ownership и PubSub ephemeral; не восстанавливать старые realtime leases из backup.
Rabbit durable commands сохранять вместе с PG или учитывать duplicate replay.
Для согласованного backup остановить новые записи, дождаться финализации,
приостановить пользовательские изменения либо использовать согласованный snapshot.

Порядок restore: PG schema/data → MinIO → secrets/TLS → Redis/Rabbit → media-worker
→ recorder → API/proxy. Проверить соответствие file object keys, token и record
status, затем тестовую загрузку/запись. Тестовый restore на отдельном сервере
обязателен: наличие dump не доказывает его пригодность. Сроки RPO/RTO не измерены.

Rollback: сохранить старые image digests/config, drain, остановить новую версию,
проверить обратную совместимость миграций, поднять прежние images. Не выполнять
автоматический down migration с удалением колонок/данных. При несовместимой схеме
нужен restore согласованного PG+MinIO snapshot и решение о потерях после snapshot.

## Повторяемые тесты

Изолированный стенд: `make stage6-up`; он не использует volumes работающего dev
проекта. `docker-compose.integration.yml` имеет заведомо test-only credentials и порты
15433/16380/15682/19000/18085/18090/18091, SFU 50020. `make stage6-failure` проверяет
точный Compose project label перед остановками. Не менять prefix на production.
`make stage6-load` — HTTP и synthetic SFU; `make stage6-soak` — 30m opt-in.
WS/reconnect и два full recorder конвейера включаются `RECORDER_WS_LOAD=true` и
`RECORDER_RECORDING_LOAD=true` с явными локальными integration env.

TURN test выполняется в сети `recorder-stage6_default` с
`RECORDER_TEST_TURN_HOST=coturn:3478`,
`RECORDER_TEST_TURN_SECRET` равным тестовому shared secret:
`go test -v ./internal/infrastructure/sfu -run '^TestStageSixForcedTURN$'`.
Не печатать настоящий shared secret и не использовать его вместо test-only ключа.
Browser profile требует локальные dev сертификаты; доверие Node задаётся
`NODE_EXTRA_CA_CERTS`, не глобальным отключением TLS проверки.
При browser cleanup обязательно задать `COMPOSE_PROJECT_NAME=recorder-stage6`,
`COMPOSE_FILE` к тестовому Compose и локальный MinIO public endpoint; иначе
защитный marker check откажет в очистке. Тестовые записи не удалять по широкому
prefix из рабочего проекта.

В обычном CI: format, unit, race, vet, govulncheck, frontend tests/build/audit,
бинарники. Heavy SFU/30m soak — manual/scheduled. Полный Docker/TURN/browser
acceptance запускается отдельно с доступом к Docker и тестовыми зависимостями.
