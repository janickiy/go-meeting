# job recorder

REST API сервиса записи видеопотока на Go + Gin.

API отвечает за управление задачами записи: создает запись в PostgreSQL, публикует команды `record.start` и `record.stop` в централизованный RabbitMQ и возвращает состояние записи через HTTP. Непосредственный захват видеопотока, склейку итогового видео, генерацию preview и загрузку артефактов в MinIO выполняет отдельный `worker`.

## Структура проекта

```text
/project
├── cmd/
│   ├── main/                 # общий entrypoint: serve, migrate ...
│   ├── api/                  # отдельный API binary
│   ├── worker/               # отдельный recorder-worker binary
│   └── cli/                  # отдельный CLI binary
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
│   ├── moduleconfig/         # loader/validator JSON-конфигов модулей
│   └── transport/
│       ├── cli/              # CLI transport wrapper
│       ├── http/             # Gin HTTP routes
│       └── ws/               # WebSocket handlers
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
- `rabbitmq` - централизованный брокер из проекта `infra-main`, подключается через внешнюю Docker-сеть `infra_network`.

HTTP-вызов API -> worker сохранен только для WebRTC signaling endpoint-а, потому что браузерному `SDP offer` нужен синхронный `SDP answer`.

Данные локальных контейнеров проекта хранятся в `dockers/`.

## Настройки

Настройки берутся из `.env`. Если файла нет, создай его из примера:

```bash
cp .env.example .env
```

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

Lock снимается после успешного выполнения worker-команды `record.stop` при вызове `POST /api/v1/records/end`. Если запись не завершили вручную, Redis снимет lock по TTL.

Redis rate limit:

```env
RATE_LIMIT_ENABLED=true
RATE_LIMIT_WINDOW=1m
RATE_LIMIT_DEFAULT_RPM=240
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
User:                 guest
Password:             guest
Exchange:             go-recorder.commands
Queue:                go-recorder.recording.commands
Routing key:          record.commands
Docker network:       infra_network
```

Перед запуском `go-recorder` должен быть поднят централизованный брокер из `/Users/yanicki/htdocs/infra-main`:

```bash
cd /Users/yanicki/htdocs/infra-main
docker compose up -d
```

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

Для локальной разработки в браузере на этой же машине используй:

```env
WEBRTC_NAT_IPS=127.0.0.1
```

Тогда browser ICE candidate будет указывать на host `127.0.0.1:50000`, а Docker пробросит ICE/RTP-пакеты в контейнер worker-а. Основной transport - UDP, TCP используется как fallback, если внешний контур блокирует UDP.

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

Go-тесты лежат в `tests/`: unit-тесты в `tests/unit`, функциональные smoke-тесты нужно добавлять в `tests/functional`. Интеграционный smoke выполняется через Docker Compose, PostgreSQL, Redis, FFmpeg и MinIO.

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
