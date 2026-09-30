# job recorder

REST API сервиса записи видеопотока на Go + Gin.

API отвечает за управление задачами записи: создает запись в PostgreSQL, публикует команды `record.start` и `record.stop` в локальный RabbitMQ проекта и возвращает состояние записи через HTTP. Непосредственный захват видеопотока, склейку итогового видео, генерацию preview и загрузку артефактов в MinIO выполняет отдельный `worker`.

Этап 1 платформы конференций добавляет пользователей, email/password-аутентификацию,
часовой JWT, конференции, membership участников и права владельца. Контракты,
миграции, ограничения совместимости и примеры запросов: [Stage 1 API](docs/stage-1-api.md).
Новый API требует `JWT_SECRET`; существующий recorder pipeline не изменён.

## Структура проекта

```text
/project
├── cmd/
│   ├── main/                 # общий entrypoint: serve, migrate ...
│   ├── api/                  # отдельный API binary
│   └── worker/               # отдельный recorder-worker binary
├── internal/
│   ├── app/                  # bootstrap: config, DB, repositories, transport
│   ├── config/               # env config
│   ├── domain/               # домен records: DTO, статусы, валидация
│   ├── usecase/              # business use-cases
│   ├── infrastructure/
│   │   ├── postgres/         # DB repo + migrator
│   │   ├── rabbitmq/         # RabbitMQ publisher/consumer команд записи
│   │   ├── worker/           # внутренний HTTP-клиент API -> worker для WebRTC SDP
│   │   └── storage/          # S3/MinIO storage clients
│   └── transport/
│       └── http/             # Gin HTTP routes
├── database/migrations/      # SQL-миграции DB
├── scripts/                  # cron/helper scripts
├── docs/                     # документация и Postman collection
├── tests/                    # unit-tests
├── dockers/                  # Dockerfile, configs и volume-данные контейнеров
├── docker-compose.yml
├── Makefile
└── .env.example
```

## Сервисы

- `api` - Go + Gin REST API.
- `worker` - Go recorder-worker: читает команды из RabbitMQ, поднимает WebRTC ingest через Pion и управляет FFmpeg.
- `postgres` - PostgreSQL 16.
- `redis` - Redis 7, используется для lock-а активной записи по `conferenceId` и HTTP rate limit.
- `minio` - локальное S3-compatible хранилище итоговых артефактов записи.
- `rabbitmq` - собственный брокер RabbitMQ с management UI, запускается этим же Docker Compose в сети `app-network`.

HTTP-вызов API -> worker сохранен только для WebRTC signaling endpoint-а, потому что браузерному `SDP offer` нужен синхронный `SDP answer`.

Данные локальных контейнеров проекта хранятся в `dockers/`.

## Настройки

Настройки берутся из `.env`. Если файла нет, создай его из примера:

```bash
cp .env.example .env
```

Перед запуском API сгенерируй `openssl rand -hex 32` и сохрани результат в
`JWT_SECRET` внутри `.env`. Не коммить этот секрет. Без секрета длиной минимум
32 байта API не запускается; worker и отдельная команда миграций его не требуют.
`HTTP_TRUSTED_PROXIES` по умолчанию пуст: доверие к `X-Forwarded-For` разрешается
только для явно указанных адресов/CIDR реального reverse proxy.

Локальные подключения:

```text
API:        http://localhost:8085
Worker:     http://localhost:8090
PostgreSQL: localhost:5433
Redis:      localhost:6380
RabbitMQ:   localhost:5672 / http://localhost:15672
MinIO API:  http://localhost:9000
MinIO UI:   http://localhost:9001
MinIO public links via HTTPS smoke origin: https://localhost:18482/recordings/...
```

PostgreSQL:

```text
Database: go_recorder
User:     go_recorder
Password: go_recorder_pass
JDBC URL: jdbc:postgresql://localhost:5433/go_recorder
```

Redis lock записи:

```text
Key: record:conference:{conferenceId}:lock
Value: {recordId}
TTL: RECORD_LOCK_TTL, по умолчанию 6h
```

API ставит lock перед созданием записи. Если для этого `conferenceId` уже есть активная запись, повторный `POST /api/v1/records/start` вернет `409 Conflict`:

```json
{
  "status": "failed",
  "message": "recording already started for conferenceId"
}
```

Lock снимает worker после фактической остановки WebRTC/FFmpeg, перед финализацией файлов, либо после ошибки подготовки/приёма потока. Публикация `record.stop` сама по себе lock не снимает. Повторный `end` для готовой/ошибочной записи ничего не меняет; в состоянии `stopping` команду можно отправить повторно после ошибки публикации. TTL остаётся аварийным ограничением и пока не продлевается автоматически.

Redis rate limit:

```env
RATE_LIMIT_ENABLED=true
RATE_LIMIT_WINDOW=1m
RATE_LIMIT_DEFAULT_RPM=240
RATE_LIMIT_AUTH_LOGIN_IP_RPM=10
RATE_LIMIT_AUTH_REGISTER_IP_RPM=5
RATE_LIMIT_RECORD_START_CONFERENCE_RPM=12
RATE_LIMIT_RECORD_START_IP_RPM=40
RATE_LIMIT_RECORD_END_RECORD_RPM=40
RATE_LIMIT_RECORD_END_IP_RPM=120
RATE_LIMIT_WEBRTC_OFFER_RECORD_RPM=40
RATE_LIMIT_WEBRTC_OFFER_IP_RPM=120
RATE_LIMIT_RECORD_LIST_IP_RPM=240
RATE_LIMIT_RECORD_READ_IP_RPM=480
```

Rate limit работает через Redis sliding window и проверяется до вызова handler-а. Если лимит исчерпан, API возвращает `429 Too Many Requests`:

```json
{
  "status": "failed",
  "message": "rate limit exceeded"
}
```

Отключить rate limit локально можно так:

```env
RATE_LIMIT_ENABLED=false
```

MinIO:

Контейнер MinIO собирается локально из закрепленного официального релиза
`RELEASE.2025-10-15T17-29-55Z` через `dockers/minio/Dockerfile`: ранее используемый
`minio/minio:latest` недоступен в реестре. Первая сборка скачивает исходники и
Go-зависимости. Источник: [официальный релиз MinIO](https://github.com/minio/minio/releases/tag/RELEASE.2025-10-15T17-29-55Z).

```text
Internal endpoint: minio:9000
Public endpoint:   https://localhost:18482
Bucket:            recordings
User:              go_recorder
Password:          go_recorder_pass
Console:           http://localhost:9001
```

Для удаленного HTTPS-окружения `MINIO_PUBLIC_ENDPOINT` должен указывать на тот же origin, где открывается smoke page, например `https://192.168.90.201:18482`. Nginx проксирует `/recordings/...` в MinIO, поэтому preview/final presigned links открываются без `localhost:9000` и без mixed content.

RabbitMQ:

```text
Host в Docker network: rabbitmq:5672
Host с машины:        localhost:5672
Management UI:        http://localhost:15672
User:                 go_recorder
Password:             go_recorder_pass
Exchange:             go-recorder.commands
Queue:                go-recorder.recording.commands
Routing key:          record.commands
Docker network:       app-network
Data:                 ./dockers/rabbitmq/data
```

RabbitMQ запускается автоматически вместе с проектом. API, worker и их debug-варианты ждут успешного healthcheck брокера. Общий брокер и внешняя сеть `infra_network` больше не требуются. Существующая внешняя сеть gateway (`gateway_default` или `GATEWAY_NETWORK`) по-прежнему нужна для gateway-интеграции API и worker.

AMQP и management UI публикуются только на `127.0.0.1`. Если порты заняты другим брокером, поменяй `RABBIT_MQ_HOST_PORT` и `RABBIT_MQ_MANAGEMENT_HOST_PORT` в `.env`; внутри контейнеров адрес остается `rabbitmq:5672`.

Compose явно задает `RABBIT_MQ_HOST=rabbitmq`, `RABBIT_MQ_PORT=5672` и пустой `RABBIT_MQ_DSN` для API и worker, чтобы старый `.env` не направил их в общий брокер. `RABBIT_MQ_USER`, `RABBIT_MQ_PASSWORD` и `RABBIT_MQ_VHOST` одинаково передаются брокеру и клиентам. Для запуска Go-приложения вне Docker укажи `RABBIT_MQ_HOST=127.0.0.1` и опубликованный порт в `RABBIT_MQ_PORT`; явный `RABBIT_MQ_DSN` вне Compose по-прежнему поддерживается.

Логин и пароль выше предназначены только для локальной разработки. Перед первым запуском на сервере задай собственные значения. Пользователь и vhost создаются только при инициализации пустого хранилища RabbitMQ: изменение `.env` не меняет пароль и права в уже существующем брокере. Данные сохраняются при пересоздании контейнера; deploy script исключает `dockers/rabbitmq/data/` из синхронизации с удалением.

### Переход с общего брокера

1. До переключения останови прием новых записей, заверши активные записи и дождись обработки всех команд старой очереди, включая неподтвержденные сообщения.
2. В `.env` задай логин, пароль и vhost нового брокера. Удали устаревшие `RABBIT_MQ_NETWORK` и `RABBIT_MQ_DSN`, выстави `RABBIT_MQ_HOST=rabbitmq` и `RABBIT_MQ_PORT=5672`. При конфликте host-портов измени новые переменные публикации портов.
3. Запусти локальный брокер и пересоздай API/worker:

```bash
docker compose up -d --wait rabbitmq
docker compose up -d --build api worker
docker compose exec -T rabbitmq rabbitmq-diagnostics -q check_port_connectivity
```

Старая очередь и ее сообщения автоматически не переносятся. Общий брокер других проектов останавливать или очищать не нужно.

Проверка обмена командами с запущенным локальным брокером (для стандартных локальных настроек):

```bash
RABBITMQ_TEST_URL='amqp://go_recorder:go_recorder_pass@127.0.0.1:5672/%2F' \
  go test -race -count=1 -run TestCommandRoundTrip ./internal/infrastructure/rabbitmq
```

Тест создает собственные exchange/queue и удаляет их после проверки; рабочую очередь записей не затрагивает. Без `RABBITMQ_TEST_URL` он пропускается. При других порте, учетных данных или vhost укажи соответствующий URL.

Параметры контейнера: [RabbitMQ Docker Official Image](https://hub.docker.com/_/rabbitmq/).

Recorder worker:

```text
HTTP:      8090
ICE UDP:   50000/udp
ICE TCP:   50000/tcp fallback
Storage:   /storage
FFmpeg:    ffmpeg
MinIO:     minio:9000 / bucket recordings
WebRTC:    Pion WebRTC v4
Go:        1.24
```

Если браузер подключается не с той же машины, где запущен Docker, задай внешний адрес worker-а в `.env`:

```env
WEBRTC_NAT_IPS=203.0.113.10
```

Для Firefox используй LAN-адрес компьютера с Docker, даже если страница открыта
на `localhost`. Например (замени адрес на свой):

```env
WEBRTC_NAT_IPS=192.168.1.35
```

Firefox по умолчанию отбрасывает loopback ICE-кандидаты (`127.0.0.1`), поэтому
при таком адресе SDP-обмен может пройти, а медиапоток так и не подключится.
Это поведение описано в [Mozilla Bugzilla](https://bugzilla.mozilla.org/show_bug.cgi?id=1973521).
На macOS адрес Wi-Fi-интерфейса можно узнать командой `ipconfig getifaddr en0`.
После изменения `.env` пересоздай worker: `docker compose up -d worker`.
Саму страницу на этом компьютере оставь на `http://localhost:8085`: менять
адрес страницы на незащищенный LAN HTTP не нужно.

Docker пробрасывает ICE/RTP на порт `50000` worker-а. Основной транспорт — UDP,
TCP используется как fallback. Если LAN-адрес изменился, обнови `WEBRTC_NAT_IPS`.

При ошибке WebRTC worker закрывает ingest-сессию, сохраняет `failed`, снимает
Redis-блокировку только этой записи и добавляет событие `record.ingest.failed`.
Тестовая страница при статусе `failed` останавливает локальные media tracks.

Сквозной тест без камеры и микрофона доступен в `tests/integration`:
задай `RECORDER_TEST_URL=http://127.0.0.1:8085` и `RECORDER_TEST_IVF` — путь к
искусственному VP8-видео в IVF (25 fps, не менее 6 секунд), затем выполни
`go test -count=1 -v ./tests/integration`. Он создает отдельную проверочную
запись и оставляет ее готовые артефакты в локальном MinIO. Без этих переменных
сквозной тест пропускается.

## Запуск

Через Docker Compose:

```bash
cd /Users/yanicki/htdocs/go-recorder
docker compose up -d
```

Или через `Makefile`:

```bash
make up
```

Для автономного локального запуска без отдельного gateway в `.env` используй:

```env
WORKER_INTERNAL_URL=http://worker:8090
MINIO_PUBLIC_ENDPOINT=http://localhost:9000
```

Внешняя Docker-сеть `gateway_default` должна существовать даже без процесса gateway;
при первом запуске ее можно создать через `docker network create gateway_default`.
Для HTTPS-контейнера предварительно создай локальные сертификаты командой
`HTTPS_CERT_IP=127.0.0.1 sh scripts/generate-https-certs.sh` (не запускай ее поверх
сертификатов, которые нужно сохранить). Доверие к локальному CA в систему автоматически не добавляется.

При этих настройках открой [локальную тестовую страницу записи](http://localhost:8085/debug/webrtc-smoke?gatewayBase=http%3A%2F%2Flocalhost%3A8085).
Параметр `gatewayBase` направляет браузер напрямую в API. Для HTTPS-страницы
нужно вернуть `MINIO_PUBLIC_ENDPOINT=https://localhost:18482` и настроить доверие к локальному сертификату.

Пересборка API и worker:

```bash
docker compose build api worker
docker compose up -d api worker
```

Полный restart через `Makefile`:

```bash
make restart
```

## Миграции

Миграции лежат в `database/migrations` и применяются автоматически при старте API.

Запуск миграций отдельной командой:

```bash
go run ./cmd/main migrate
```

## API

Базовый URL прямого API:

```text
http://localhost:8085
```

Публичная точка входа через gateway для локального окружения:

```text
http://127.0.0.1:18080
```

REST API версионирован префиксом `/api/v1`.

Основные endpoints:

- `POST /api/v1/records/start`
- `POST /api/v1/records/end`
- `POST /api/v1/records/{recordId}/webrtc/offer`
- `GET /api/v1/records`
- `GET /api/v1/records/count-by-conference`
- `GET /api/v1/records/{recordId}`

При работе через gateway используются те же публичные API endpoints, но с gateway base URL:

```text
http://127.0.0.1:18080/api/v1/records
```

Для внутренних обращений API к recorder-worker через gateway добавлены отдельные worker routes:

- `GET /api/v1/record-worker/health`
- `POST /api/v1/record-worker/records/{recordId}/start`
- `POST /api/v1/record-worker/records/{recordId}/stop`
- `POST /api/v1/record-worker/records/{recordId}/webrtc/offer`

Эти worker endpoints нужны для сервисного взаимодействия и smoke/debug-проверок. Клиентский frontend обычно вызывает публичные endpoints `/api/v1/records/*`, а API уже обращается к worker через gateway.

Debug endpoint для списка завершенных записей из MinIO:

- `GET /debug/records/completed?limit=50`

Legacy-префикс `/api/records` удален. Клиенты должны использовать только версионированные endpoints `/api/v1/records/*`.

## Проверка

Healthcheck:

```bash
docker compose exec -T api wget -qO- http://127.0.0.1:8085/health
docker compose exec -T worker wget -qO- http://127.0.0.1:8090/health
```

Проверка таблиц PostgreSQL:

```bash
docker compose exec -T postgres psql -U go_recorder -d go_recorder -c '\dt'
```

Проверка MinIO:

```bash
docker compose ps minio
```

Проверка количества записей:

```bash
docker compose exec -T postgres psql -U go_recorder -d go_recorder -c 'select count(*) from record;'
```

Запуск записи из контейнера API:

```bash
docker compose exec -T api wget -qO- \
  --header 'Content-Type: application/json' \
  --post-data '{"conferenceId":"11111111-1111-1111-1111-111111111111","quality":"720p","qualityMode":"auto","minQuality":"360p","segmentDurationSec":5}' \
  http://127.0.0.1:8085/api/v1/records/start
```

Ответ `POST /api/v1/records/start`:

```json
{
  "recordId": "uuid",
  "status": "starting",
  "message": "Record job accepted"
}
```

Сегменты записи должны лежать в локальном volume worker-а:

```text
dockers/storage/data/records/{recordId}/
```

При вызове `POST /api/v1/records/end` API переводит запись в `stopping` и публикует `record.stop` в RabbitMQ. Worker получает команду из очереди и выполняет post-processing:

- сканирует непустые `segment_*.*` в `/storage/records/{recordId}`;
- создает `concat.txt`;
- склеивает сегменты в единый `final.mp4` через FFmpeg;
- генерирует `preview.jpg`;
- загружает `final.mp4` и `preview.jpg` в MinIO bucket `recordings`;
- сохраняет артефакты в `record_file`;
- обновляет `record.storage_bucket`, `record.storage_object_key`, `record.preview_object_key`, `record.size_bytes`, `record.duration_sec`;
- переводит запись в статус `ready`.

Ключи объектов в MinIO:

```text
records/{recordId}/final.mp4
records/{recordId}/preview.jpg
```

Если вызвать `POST /api/v1/records/end` до появления сегментов или если FFmpeg не смог собрать итоговый файл, запись переводится в `failed`.

### Post-processing smoke

Сценарий проверки MinIO/post-processing:

1. Пересобери и перезапусти сервисы:

```bash
docker compose build api worker
docker compose up -d --remove-orphans
```

3. Запусти запись через `POST /api/v1/records/start`.
4. Убедись, что в `dockers/storage/data/records/{recordId}/` появились сегменты `segment_*`.
5. Вызови `POST /api/v1/records/end` с телом:

```json
{
  "recordId": "{recordId}",
  "reason": "client_stop"
}
```

6. Проверь статус записи: успешный сценарий должен дать `ready`.
7. Проверь MinIO bucket `recordings`: должны появиться `final.mp4` и `preview.jpg`.

Временные файлы записи создаются локально и очищаются после успешной загрузки артефактов в MinIO:

```text
dockers/storage/data/records/{recordId}/
```

## Тесты

```bash
make test-run
```

Через Docker:

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.24-alpine3.22 go test ./...
```

Тесты находятся рядом с реализацией в `internal/`, в `tests/unit` и `tests/integration`.
Обычный `go test ./...` не требует запущенных сервисов; внешние интеграционные проверки включаются переменными окружения.

Проверка переходов статуса и количества SQL-запросов на локальной PostgreSQL:

```bash
RECORDER_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
  go test -race -count=1 -v ./internal/infrastructure/postgres
```

Для нестандартных локальных настроек укажи свои учётные данные и порт. Миграции должны быть применены.
Тестовые строки создаются внутри транзакций и откатываются; существующие записи не меняются.
Сквозной WebRTC-тест в `tests/integration` дополнительно проверяет повторную остановку готовой записи.

Результаты ревизии кода и оставшиеся ограничения: [docs/code-review-2026-09-30.md](docs/code-review-2026-09-30.md).

## GitLab CI/CD

Pipeline описан в `.gitlab-ci.yml`. Список variables для подключения deploy к
серверу: [docs/gitlab-ci-variables.md](docs/gitlab-ci-variables.md).

## Debug через Delve

В проект добавлены debug-образы для API и worker-а на базе [Delve](https://github.com/go-delve/delve). Debug-бинарники собираются с флагом:

```bash
-gcflags="all=-N -l"
```

Это отключает оптимизации и inline, чтобы breakpoint-ы и просмотр переменных работали корректно.

Порты по умолчанию:

```text
API HTTP:      8085
API Delve:     2345
Worker HTTP:   8090
Worker Delve:  2346
WebRTC UDP:    50000/udp
WebRTC TCP:    50000/tcp
```

Переменные в `.env`:

```env
DLV_API_PORT=2345
DLV_WORKER_PORT=2346
```

### Debug API

Команда остановит обычный контейнер `api` и запустит `api-debug`:

```bash
make debug-api
```

После запуска API доступен как обычно:

```text
http://localhost:8085
```

Delve слушает:

```text
localhost:2345
```

Подключение из CLI:

```bash
dlv connect localhost:2345
```

Пример CLI-сессии:

```bash
(dlv) break internal/app/records/start_handler.go:14
(dlv) continue
```

После этого вызови endpoint:

```bash
curl -X POST http://localhost:8085/api/v1/records/start \
  -H 'Content-Type: application/json' \
  -d '{"conferenceId":"11111111-1111-4111-8111-111111111111","qualityMode":"auto","segmentDurationSec":5}'
```

### Debug worker

Команда остановит обычный контейнер `worker` и запустит `worker-debug`. Для Docker-сети ему добавлен alias `worker`, поэтому API продолжит обращаться к `http://worker:8090`.

```bash
make debug-worker
```

Delve слушает:

```text
localhost:2346
```

Подключение из CLI:

```bash
dlv connect localhost:2346
```

Пример breakpoint-а на обработку stop-команды:

```bash
(dlv) break internal/usecase/recorder/service.go:200
(dlv) continue
```

### Debug API и worker одновременно

```bash
make debug-both
```

Подключения:

```text
API:    localhost:2345
Worker: localhost:2346
```

Остановить debug-контейнеры:

```bash
make debug-stop
```

### Подключение из GoLand / PhpStorm с Go plugin

1. Запусти `make debug-api` или `make debug-worker`.
2. Открой Run/Debug Configurations.
3. Добавь Go Remote.
4. Для API укажи `Host: localhost`, `Port: 2345`.
5. Для worker укажи `Host: localhost`, `Port: 2346`.
6. Поставь breakpoint в локальном файле проекта.
7. Нажми Debug и вызови нужный HTTP endpoint или WebRTC smoke flow.

## Postman

Коллекция Postman:

```text
docs/postman/job-recorder.postman_collection.json
```

Переменные коллекции:

```text
baseUrl = http://localhost:8085
recordId = UUID записи после /api/v1/records/start
```
