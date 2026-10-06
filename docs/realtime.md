# Этап 2 Realtime и WebRTC signaling

Документ описывает результат этапа 2 и его исходный диагностический протокол.
Актуальная передача аудио и видео реализована отдельно в [этапе 3 SFU](media-sfu.md).
Прежняя P2P-проверка удалена из интерфейса; relay-события сохранены для совместимости
тестов, но не используются production media. Описанные ниже результаты DataChannel
относятся к состоянию на завершение этапа 2.

На этапе 2 реализован слой подключений конференции поверх API первого этапа.
Пользователи видят онлайн участников и число вкладок, браузеры обмениваются offer,
answer и ICE. Сам слой presence не создаёт conference PeerConnection и не передаёт RTP.

## 1 Реализованные возможности

WebSocket с проверкой JWT или одноразового билета, история ParticipantSession, Redis presence с TTL, межсерверная доставка через Pub/Sub, native ping/pong, переподключение, ограничения сообщений и очередей, завершение соединений при leave/finish/cancel и остановке API.

В Meet после join появляется блок «Связь с участниками». Каждая вкладка показывает собственный connectionId. Дополнительная проверка P2P использует настоящий браузерный WebRTC DataChannel без запроса камеры и микрофона. Это временная проверка одного соединения, не mesh и не видеоконференция.

## 2 Архитектурные решения

ConferenceParticipant — постоянное членство. ParticipantSession — история отдельного подключения. Закрытие вкладки не вызывает REST leave и не удаляет membership. Несколько вкладок имеют разные connectionId и один participantId.

HTTP transport занимается аутентификацией и upgrade, WebSocket client — чтением, единственным write loop и ограниченной очередью. ConferenceHub предоставляет Register, Unregister, Broadcast, SendToParticipant, SendToConnection и GetActiveSessions. Он зависит от интерфейсов repository, store и socket, не от Pion.

Все межсерверные события, включая события своего API, доставляются только через Redis Pub/Sub. Локальной дополнительной доставки нет, поэтому publisher не создаёт дубликат. Локальный registry хранит socket objects с неизменяемой идентичностью сессии. Presence и routing не вычисляются только по нему.

После committed REST-изменения observer уведомляет hub. Ошибка публикации не превращает успешную транзакцию в ложный HTTP failure; проверка membership при signaling и heartbeat обеспечивает резервное закрытие. Миграции теперь применяются в транзакции под PostgreSQL advisory lock, чтобы два запуска API не конкурировали за таблицы и триггеры.

WebSocket transport использует [Gorilla WebSocket](https://pkg.go.dev/github.com/gorilla/websocket). Ping, pong и close приложения проходят через один writer; reader не пишет application frames.

## 3 Новые и изменённые файлы

- `internal/domain/realtime/realtime.go`: сессия, identity, protocol envelope, snapshot, ICE config и внутренние bus messages.
- `internal/usecase/realtime/hub.go`: hub, routing, snapshots, janitor, registry и shutdown.
- `internal/infrastructure/redis/realtime.go`: atomic Lua presence, leases, tickets и Pub/Sub adapter.
- `internal/infrastructure/postgres/session_repository.go`: authorization, история подключения и восстановление после crash.
- `internal/transport/websocket/handler.go`: endpoints, upgrade, reader/writer, limits и heartbeat.
- `internal/config/realtime.go`, `.env.example`: отдельная конфигурация realtime для API.
- `database/migrations/000009_create_participant_sessions_table.*.sql`: новая таблица и индексы.
- `internal/app/api.go`, `internal/transport/http/router.go`, `internal/infrastructure/security/jwt.go`: подключение слоя, graceful shutdown, безопасные логи и проверенная дата истечения JWT.
- `internal/usecase/conferences/service.go`: observer после REST mutations.
- `frontend/src/realtime.ts`, `frontend/src/components/RealtimePanel.tsx`: presence, reconnect и DataChannel proof; подключение в ConferencePage, типы, API методы и стили.
- `frontend/nginx.conf.template`, `dockers/https/nginx.conf`, `frontend/vite.config.ts`: WS proxy, сохранение Host с портом и исключение ticket query из access logs.
- Unit и integration tests в `internal/config`, `internal/transport/websocket`, `tests/integration/realtime_test.go`, `frontend/src` и `frontend/e2e/realtime.spec.ts`.

## 4 Изменения PostgreSQL и Redis

Таблица `participant_sessions`: UUID id, conference_id, participant_id, user_id, уникальный UUID connection_id, status connected/disconnected, connected_at, disconnected_at, last_seen_at. Composite foreign key гарантирует, что participant, conference и user относятся к одному membership. CHECK constraints проверяют статус и порядок дат. Несколько сессий одного участника разрешены.

PostgreSQL записывается на open/close, не на каждый pong. При закрытии сохраняется последнее подтверждённое время жизни. У неудавшегося upgrade также остаётся закрытая запись истории. Janitor проверяет старые connected записи страницами по 500 и исправляет SQL-only crash между commit и регистрацией в Redis.

Redis namespace по умолчанию `go-recorder:realtime:v1`. Все API одной платформы должны использовать одинаковые PostgreSQL, Redis database и namespace.

| Ключ | Назначение |
| --- | --- |
| `:route:<connectionId>` | JSON сессии с физическим TTL, по умолчанию 75 секунд |
| `:conference:<conferenceId>` | Sorted set активных connectionId и сроков lease |
| `:expiry` и `:metadata` | Индекс истечения и сведения для crash cleanup |
| `:bus` | Pub/Sub канал внутренних событий |
| `:ticket:<sha256>` | Одноразовый билет с TTL до 45 секунд |
| `:ticket-limit:<userId>` | До 30 выдач билета за минуту |

Lua использует Redis TIME: часы разных API не определяют истечение lease. Sorted set содержит индивидуальные сроки, а routing key имеет настоящий Redis TTL. Истёкший routing key никогда не считается online, даже до janitor. Metadata сохраняется до atomic prune, чтобы после crash можно было объявить disconnect и завершить историю. Janitor работает не реже чем раз в 5 секунд, удаляя до 100 истёкших записей за проход; при наличии backlog очистка индексных записей постепенная. Пустые hash и sorted set удаляются самим Redis.

## 5 Протокол WebSocket

Browser subprotocol: `go-recorder.v1`. Native клиент может использовать Bearer header без subprotocol, но JSON version всегда равен 1.

```json
{
  "version": 1,
  "id": "7156348b-df27-49f6-a9b5-cc7e61e17e04",
  "type": "webrtc.offer",
  "conferenceId": "053c3098-a4ac-4438-a087-659c767e19f7",
  "timestamp": "2026-10-01T03:00:00Z",
  "data": {
    "targetConnectionId": "8e6da783-2df4-4112-bb7e-e447f49f6e8b",
    "sdp": "opaque SDP string"
  }
}
```

Client id должен быть UUID; timestamp обязателен. Сервер создаёт новый id и timestamp, добавляет senderConnectionId и senderParticipantId из аутентифицированной сессии. Client не может подделать эти поля. forwarded message и ack/error содержат replyTo исходного id. Входящий replyTo не разрешён.

| Событие | Направление и данные |
| --- | --- |
| `conference.state` | Server → client: connectionId, participantId, status конференции, participants |
| `participant.connected` | Server → conference: актуальный snapshot после подключения |
| `participant.disconnected` | Server → conference: актуальный snapshot после удаления подключения |
| `webrtc.offer`, `webrtc.answer` | Client → target: targetConnectionId и непрозрачный sdp |
| `webrtc.ice` | Client → target: targetConnectionId и candidate object в формате RTCIceCandidateInit |
| `ack` | Server → sender: публикация signaling принята брокером, replyTo |
| `error` | Server → sender: безопасный code и replyTo |

Каждый элемент participants содержит обычный ParticipantView и поля online, connections, connectionIds. Online требует joined membership и хотя бы одного живого routing lease. Presence event несёт snapshot текущего состояния, а не накопительный delta; поздний disconnect старой вкладки не делает переподключённого участника offline.

SDP сервер не разбирает. ICE object ограничивается размером, без интерпретации сетевых адресов. Проверяется, что sender и target активны, принадлежат одной конференции и всё ещё имеют joined membership. Неизвестные поля, несовпадающая conferenceId, self-target и чужая conference не разрешены.

Default limits: входящий frame 64 KiB, SDP 48 KiB, ICE 4 KiB, исходящий data 256 KiB; 20 сообщений в секунду с burst 40. Control frames тоже учитываются в token bucket. Outbound queue содержит максимум 64 сообщения. Переполнение закрывает slow client и запускает обычную очистку; hub не ждёт свободной очереди. Oversized исходящий snapshot закрывает подключение, а не выдаёт неполный roster.

Причины закрытия включают `client_closed`, `presence_timeout`, `server_shutdown`, `authentication_expired`, `authentication_revoked`, `authentication_unavailable`, `membership_closed`, `rate_limited`, `slow_client`, `broker_unavailable` и `outbound_too_large`. Сервер отправляет нативный ping каждую секунду; после пяти секунд без подтверждённого pong физическая сессия отключается. Срок присутствия в Redis также равен пяти секундам и обновляется только подтверждённым pong, а не произвольными сообщениями приложения. Закрытие браузера удаляет присутствие при закрытии сокета, не ожидая тайм-аута. Очистка присутствия выполняется до ожидания отправки закрывающего кадра. При аварии процесса API истёкшие аренды дополнительно удаляются фоновым обработчиком с периодом не более 250 мс; доставка события зависит от доступности хранилища и сети.

Отключение относится к конкретному `connectionId`: если у участника остаётся другая действующая вкладка или устройство, он остаётся онлайн. Сохранённое членство и история сообщений не удаляются при потере связи; после нового подключения приходит актуальный снимок присутствия.

Дедлайн аренды сохраняется как абсолютная серверная отметка `время приёма pong + TTL`: задержка обработки не добавляет ещё пять секунд. Системные часы API и Redis должны быть синхронизированы. Для защиты от положительного сдвига часов API срок дополнительно ограничен `текущее время Redis + TTL`; уже истёкшая аренда не продлевается.

## 6 Endpoints и конфигурация

- `POST /api/v1/conferences/:id/ws-ticket`: Bearer JWT, joined membership, открытая конференция. Ответ 201 `{ticket, expiresAt}`, Cache-Control no-store. Билет привязан к conference и JWT expiry, одноразовый через GETDEL; срок не превышает 45 секунд и остаток JWT.
- `GET /api/v1/conferences/:id/ws?ticket=...`: browser upgrade. JWT в URL не принимается. Можно вместо ticket передать Bearer header в native клиенте. Одновременное использование обоих способов отклоняется.
- `GET /api/v1/webrtc/config`: Bearer JWT, ответ `{iceServers: [...]}`, Cache-Control no-store.

Created и active конференции открыты для realtime, finished и cancelled закрыты. Наличие membership со статусом left недостаточно: сначала нужен REST join. Конференция и права проверяются до 101; после upgrade проводится повторная проверка на race с finish/leave.

Origin по умолчанию должен иметь тот же host и порт, что WS request. HTTPS reverse proxy сохраняет исходный Host, включая порт. `WS_ALLOWED_ORIGINS` позволяет задать точные HTTP(S) origins через запятую. Отсутствующий Origin допустим для native клиента, но аутентификация всё равно обязательна. Cookies не используются.

Все параметры перечислены в `.env.example`. Сумма `WS_PING_INTERVAL + WS_PONG_TIMEOUT` не должна превышать пяти секунд; `WS_SESSION_TTL` должен быть не меньше этой суммы и не больше пяти секунд. Значения по умолчанию — `1s`, `4s`, `5s`. Старые значения `25s/10s/75s` необходимо заменить в действующих файлах окружения, иначе проверка конфигурации остановит запуск. Срок билета в конфигурации — 30–60 секунд. ICE-серверы задаются через `WEBRTC_ICE_SERVERS_JSON`, например:

```dotenv
WEBRTC_ICE_SERVERS_JSON=[{"urls":["stun:stun.example.com:3478"]}]
```

По умолчанию массив пуст: нет неявного обращения к стороннему STUN. TURN username/credential можно передать через env, не в frontend source; это credentials для браузера, поэтому они видны аутентифицированному клиенту. Для production нужна выдача короткоживущих TURN credentials, а не общий постоянный секрет.

Локальная сборка:

```bash
docker compose build api frontend
docker compose up -d --no-deps api frontend
docker compose exec -T https nginx -t
docker compose exec -T https nginx -s reload
```

Не требуется перезапуск PostgreSQL, Redis, RabbitMQ, MinIO или recorder-worker. UI: http://localhost:5173; HTTPS: https://localhost:18482. После join откройте конференцию в двух браузерах/вкладках. В блоке проверки P2P выберите другое connectionId и нажмите «Проверить P2P-соединение».

## 7 Результаты тестов

Проверены `go test ./...`, `go test -race ./...` и `go vet ./...`. Существующего дополнительного Go lint target в проекте нет. Frontend: TypeScript и production build, 26 Vitest checks, 8 существующих Chromium UI tests, реальный Stage 1 API lifecycle и новый двухбраузерный DataChannel proof.

Real PostgreSQL/Redis suite отдельно запущен с race detector. Покрыты missing/invalid/expired auth, истечение JWT во время открытого соединения, outsider, left membership, finished/cancelled, foreign Origin, ticket replay/expiry/conference binding, snapshots, multiple tabs, reconnect, двусторонние offer/answer/ICE без дубликатов, cross-conference rejection, размеры payload, подделка sender, history constraints, dead client, TTL crash, SQL-only crash, потеря соединения с брокером, cleanup registry и завершение hub loops. Unit test проверяет bounded queue overflow и идемпотентный Stop.

```bash
RECORDER_STAGE1_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
RECORDER_STAGE2_TEST_REDIS_ADDR='127.0.0.1:6380' \
go test -race -v ./tests/integration -run '^TestStageTwo' -count=1
```

Для браузерной проверки дополнительно нужны доступные Chromium и совместимый Node:

```bash
RECORDER_FRONTEND_E2E=true \
RECORDER_STAGE1_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
RECORDER_STAGE2_TEST_REDIS_ADDR='127.0.0.1:6380' \
go test -v ./tests/integration -run '^TestStageTwoBrowserProof$' -count=1
```

Тесты создают случайную отдельную БД и уникальный Redis namespace. Удаляются только собственные тестовые данные. Общий Redis не очищается. Пользовательские записи, membership, MinIO и RabbitMQ не затрагиваются. Браузерный тест использует две изолированные Chromium contexts, а не открытый профиль пользователя; разрешения камеры/микрофона не выдаются.

## 8 Нагрузочная проверка

`TestStageTwoLoad100Connections` удержал 100 одновременно живых WebSocket на двух API instances с настоящими PostgreSQL и Redis. В проверенных запусках с race detector открытие заняло около 0,9–1,1 секунды. Все клиенты читали сообщения; после закрытия Redis active sessions и local registry были пустыми.

Это smoke test realtime, не benchmark, не 100 видеопотоков и не оценка SFU. Конкретное время зависит от машины и окружения. Запуск отдельно:

```bash
RECORDER_STAGE1_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
RECORDER_STAGE2_TEST_REDIS_ADDR='127.0.0.1:6380' \
go test -race -v ./tests/integration -run '^TestStageTwoLoad100Connections$' -count=1
```

## 9 Безопасность

JWT не помещается в WebSocket URL. В Redis сохраняется SHA256 имени билета; его случайное значение имеет 256 бит энтропии. Билет одноразовый, scoped и short lived. Для аккаунта с `sid` WebSocket проверяет серверную сессию перед upgrade и при каждом pong/команде; истечение исходного JWT не меняет соединение и медиапривязку. Отзыв закрывает сокет. Гостевые и старые JWT без `sid` сохраняют исходный expiry. Перед upgrade и при работе проверяются joined membership и состояние конференции.

Backend logs содержат структурированные conference_id, participant_id, user_id, connection_id, event_type, но не JWT, SDP, ICE credentials или пароль. HTTP logs используют route template без query string. Recovery не печатает request dump. Nginx access logs исключают query, а WS location подавляет credential-bearing upstream error dumps. Клиентский журнал показывает только названия событий.

Короткий JWT остаётся в sessionStorage текущей вкладки. Секрет постоянной сессии хранится в HttpOnly cookie и обновляет JWT через `/auth/refresh`; logout отзывает сессию вместе с JWT. Подробности: [PERSISTENT_AUTH.md](PERSISTENT_AUTH.md). XSS остаётся существенным риском, поэтому CSP и отказ от недоверенного HTML важны. Для production нужны TLS, закрытая сеть Redis, настройка реальных trusted proxies и отдельные лимиты инфраструктуры.

## 10 Совместимость recorder

RabbitMQ продолжает обслуживать только record.start/record.stop и фоновые задачи записи. Realtime не публикует в RabbitMQ. Existing Pion ingest, FFmpeg, worker HTTP, MinIO и legacy record routes не изменены. Запись и конференция по-прежнему имеют разные signaling endpoints.

Stage 1 PostgreSQL suite проверил прежние REST permissions, транзакции, membership и создание legacy record с независимым conference UUID. Existing Go recorder tests проходят. Новая composite FK применяется только к participant_sessions, не к record.

## 11 Известные ограничения

- Нет SFU, conference audio/video routing, composite recording, screen sharing, chat или waiting room.
- P2P proof ограничен одним DataChannel peer; нестандартный ICE/NAT может потребовать настроенный TURN. Production TURN не развёрнут.
- Redis Pub/Sub не является durable transport. Ack означает принятие публикации, не подтверждение обработки адресатом. При потере брокера текущие sockets закрываются; API instance в fail-closed состоянии требует перезапуска для восстановления realtime. REST остаётся доступен.
- Lua storage рассчитан на обычный Redis, не Redis Cluster. Глобальный канал и полные snapshots подходят для этапа проверки, не являются заявлением о production capacity. Ограничение размера исходящего snapshot явное; пагинированный REST roster сохраняется.
- Общая SQLite/in-memory подмена не используется для интеграционных проверок; без opt-in env real database suite пропускается.
- Проверка браузера проведена в Chromium. Автоматический Firefox на этом Mac по-прежнему блокируется окружением Playwright/macOS; пользовательский Firefox и разрешения ОС не менялись.
- Heavy metrics infrastructure отсутствовала, поэтому новый observability stack не добавлен. Есть structured lifecycle/signaling logs и проверяемый LocalCount.

## 12 Рекомендации для третьего этапа

Следующая отдельная работа — media-worker, Pion Room Manager, server-side PeerConnections и track routing, сначала для двух пользователей, затем multi-user SFU. Hub и connection identity уже могут служить signaling transport, но media-worker должен иметь собственные lifecycle и permissions, не переиспользовать browser recorder session как room.

До production также нужны TURN deployment с временными credentials, reconnect/recovery брокера и health/readiness realtime, целевые media load tests и coalescing/delta presence при больших конференциях. Третий этап не выполняется автоматически.
