# Этап 3 Серверная передача аудио и видео

Этап добавляет SFU на Pion WebRTC: участники публикуют аудио и видео отдельному
media-worker, который пересылает закодированные RTP-пакеты другим участникам.
API сохраняет авторизацию, конференции, membership и WebSocket. Recorder-worker
остаётся отдельным. Конференционная запись и этап 4 не реализуются этой работой.

## 1 Реализованные возможности

Отдельный executable `cmd/media-worker`, комнаты с ленивым созданием, MediaPeer
для каждого подключения, публикации microphone/camera, подписки и поздний вход.
Добавлены trickle ICE, ограниченные очереди RTP, RTCP/PLI, сериализация переговоров,
остановка треков, cleanup, лимиты, защищённый внутренний transport, media tickets
и Redis routing. Фронтенд содержит минимальную медиа-панель без нового дизайна.
Устройства включаются только по явной кнопке; тесты не используют физическую камеру.

## 2 Архитектура медиа

```text
Browser → same-origin WebSocket → API media controller → protected worker HTTP
Browser ← same-origin WebSocket ← API Hub ← Redis targeted events ← media-worker
Browser ⇄ encrypted WebRTC RTP/RTCP ⇄ media-worker SFU ⇄ other browsers
```

SFU не зависит от PostgreSQL, RabbitMQ, MinIO или FFmpeg. Его engine принимает
проверенную identity через интерфейс; подписи, активная сессия и ownership проверяются
на границе worker. Данные conference CRUD и business permissions не перенесены в SFU.
Room и MediaPeer не переиспользуют recorder ingest sessions.

Глобальные mutex защищают только короткие операции с registry. Pion/Redis и ожидание
закрытия не выполняются под глобальным lock. Peer lifecycle исключает добавление
goroutines после начала Wait; регистрация публикации атомарна относительно close.
У каждого subscriber отдельная bounded RTP queue; медленный subscriber не блокирует
остальных. MID/RID/TWCC extensions publisher не копируются в чужой RTP transport.

## 3 Изменения сигнализации

Сохраняется envelope v1, UUID сообщения, conferenceId, timestamp и replyTo.
Media-команды не содержат client participant/session/connection IDs или targetConnectionId.
API получает binding из аутентифицированной WebSocket-сессии.

| Событие | Направление и данные |
| --- | --- |
| `media.join` | Browser → API, `{}` |
| `media.joined` | Server → browser: mediaPeerId, workerId, maxPeers, videoCapture, iceServers, tracks |
| `media.offer` | Browser → worker через API: mediaPeerId, negotiationId, sdp |
| `media.answer` | Worker → browser через API: те же IDs и sdp, replyTo offer |
| `media.ready` | Browser → worker через API: mediaPeerId, negotiationId после применения answer |
| `media.ice` | Оба направления: mediaPeerId, candidate object либо null |
| `media.renegotiate` | Worker → browser: mediaPeerId, monotonic revision |
| `media.tracks` | Worker → subscriber: mediaPeerId, revision, актуальный массив tracks |
| `media.published` | Worker → publisher: mediaPeerId, track |
| `media.unpublished` | Worker → publisher: mediaPeerId, trackId |
| `media.unpublish` | Browser → worker: mediaPeerId, собственный trackId |
| `media.state` | Worker → browser: mediaPeerId, состояние PeerConnection |
| `media.leave` | Browser → worker: mediaPeerId; `{}` отменяет pending join текущей сессии |
| `media.left` | Server → browser: mediaPeerId |

Браузер — единственный инициатор offer, включая обновления подписок. Он заранее
создаёт по maxPeers−1 receive slots каждого kind. Worker сериализует SDP и
AddTrack/RemoveTrack; полный offer с retirement также сериализован, чтобы старые
изменения не обгоняли новые. Subscription mutations вызывают coalesced revision
notifications. Браузер ждёт соответствующий answer и объединяет следующие revisions.
Server offer отсутствует, поэтому glare не требует случайного выбора владельца.

Передача subscriber приостанавливается на время offer. После setRemoteDescription
браузер подтверждает `media.ready` с тем же negotiationId; только тогда worker
возобновляет RTP и запрашивает ключевой кадр. Устаревшее подтверждение отклоняется.
Подтверждение включает только подписки из соответствующего answer; новые публикации
ждут следующего offer/answer/ready и не могут начать RTP по старому подтверждению.
Это также предотвращает race early undeclared SSRC в использованной версии Pion,
найденный полным прогоном race detector. Таймаут подтверждения закрывает endpoint.

Remote ICE до SDP буферизуется: максимум 64 сообщения до offer и 128 на PeerConnection
за lifecycle; null означает end-of-candidates и тоже учитывается в лимите.
Malformed SDP, unsupported codecs, ICE и переполнение дают безопасные coded errors.
Медиаконтракт ограничен SDP 48 KiB и ICE 4 KiB, даже если legacy WS config разрешает больше.
Встроенные SDP candidates валидируются до Pion и ограничены 128 на offer;
для совместимости полного SDP допускаются компоненты 1/2, trickle принимает только 1.
Прежний `webrtc.*` relay остаётся для совместимости тестов, но frontend SFU его не использует.

## 4 Внутренний transport API и worker

Используется HTTP POST `/internal/media/{join,offer,ready,ice,unpublish,leave}`. Это короткие
сигнальные операции, не канал RTP. RabbitMQ не используется для ICE или SDP.
Запросы содержат UUID correlation ID одновременно в body и X-Request-ID, binding
и worker route. Bearer `MEDIA_INTERNAL_SECRET` обязателен. Redirects запрещены,
body/response ограничены, default timeout 5 секунд; upstream raw errors не раскрываются.
Duplicate join возвращает тот же endpoint; duplicate last offer с тем же negotiationId
и SDP использует cached answer; изменение SDP под тем же ID отклоняется. Leave идемпотентен.

Media ticket подписывается HS256, имеет issuer `go-recorder-api`, audience
`go-recorder-media`, purpose `media`, UUID jti, conference/participant/session/connection/user
binding, workerId и ownership leaseId. Admission exp — до 45 секунд и не позже JWT expiry.
Отдельный authorizationExpiresAt сохраняет исходный срок JWT для established media.
Ticket не выдаётся браузеру и не помещается в URL. Worker повторно проверяет живую
Redis route перед командами. Истечение сессии или JWT закрывает media.

## 5 Модели Room MediaPeer и Track

Room привязан к conferenceId и содержит peers и published tracks. Пустая room удаляется.
MediaPeer содержит собственный UUID, binding, PeerConnection, publications и subscriptions.
Несколько вкладок одного membership — разные session/connection/mediaPeer IDs.
Собственные треки не возвращаются тому же participantId, включая его другие вкладки.

Track имеет UUID, streamId publisher mediaPeerId, publisher mediaPeerId, participantId,
typed kind и source. Реализованы audio/microphone и video/camera; screen types зарезервированы,
но screen sharing отсутствует. Browser receiver track ID может быть другим после
preallocated transceiver; frontend использует streamId + kind для привязки и server UUID
для lifecycle. Metadata snapshot удаляет исчезнувшие streams, даже если браузер не прислал ended.

## 6 Кодеки и ограничения

SFU регистрирует только Opus 48 kHz stereo и VP8 90 kHz. Пакеты не декодируются и не
перекодируются; Pion переписывает SSRC/PT отдельного sender. RTCP sender читается,
PLI/FIR subscribers превращаются в ограниченные запросы ключевого кадра publisher.
Ключевой кадр запрашивается при подписке и после negotiation.

Default limits: 10 MediaPeers на room, 100 rooms, до двух опубликованных треков
на peer — один audio и один video. Вкладки расходуют лимит endpoints. Frontend запрашивает
не более 1280×720 и 30 fps. Worker передаёт настроенные MEDIA_VIDEO_MAX_WIDTH/HEIGHT/FPS
как `videoCapture` в joined; браузер применяет их к локальному video track до первого offer.
Это capture target, не серверная проверка размеров VP8 кадров от произвольного клиента.
Simulcast/SVC, H264 и bandwidth admission пока отсутствуют.

## 7 ICE STUN TURN и адреса

Настройки SFU независимы от recorder порта 50000. Base Compose использует UDP mux
50010 и optional engine TCP mux 50010. Internal signaling — 8091, только Docker network.
`MEDIA_NAT_IPS` должен содержать IP, достижимый браузером. Для Docker Desktop/Firefox
укажите LAN IP Mac, а не IP контейнера/127.0.0.1. Текущая локальная настройка —
192.168.1.35; при смене сети её нужно обновить. На сервере нужен корректный public/NAT IP
и проброс тех же портов без изменения номера. Для других компьютеров требуется HTTPS
origin с доверенным сертификатом, иначе getUserMedia будет недоступен.

`MEDIA_ICE_SERVERS_JSON` задаёт массив STUN/TURN; при отсутствии используется
`WEBRTC_ICE_SERVERS_JSON`. Явный `[]` отключает fallback. Неявного public STUN нет.
TURN deployment не добавлен. Credentials передаются только через env и видимы
аутентифицированному браузеру; перед production нужна выдача короткоживущих TURN credentials.

## 8 Маршрутизация и ownership

Redis namespace SFU по умолчанию `go-recorder:media:v1`, отдельно от realtime.
Worker heartbeat регистрирует identity/endpoint с TTL; API выбирает healthy worker
и атомарно закрепляет conference. Существующий живой owner сохраняется независимо
от API instance. Mapping содержит lease UUID; renew/release сравнивают worker и lease.
Старый owner не может продлить или удалить новую lease. Каждый replica должен иметь
уникальный MEDIA_WORKER_ID и собственный endpoint; общий ID двух живых процессов запрещён.

Worker регулярно проверяет активную WS route и исходный JWT expiry. Hub disconnect
дополнительно вызывает короткий internal leave. Потерянный HTTP cleanup исправляется sweep.
Ownership loss, Redis outage и истечение локального срока владения останавливают rooms;
новый join требует повторной проверки и действующей lease. Это минимальный routing,
не распределённый scheduler и не бесшовная миграция активной room.

Наши структурированные logs содержат только IDs, тип события, kind/source и состояния.
Pion использует безопасный logger для транспортов, interceptors и UDP/TCP mux:
сырые сообщения, строки форматирования и аргументы отбрасываются без их преобразования.
Сохраняются только ограниченное имя компонента и уровень события; raw SDP, candidates,
JWT/tickets и ICE/TURN credentials не журналируются.

## 9 Новые и изменённые файлы

Новые: `cmd/media-worker`, `internal/app/media_worker.go`, `internal/domain/media`,
`internal/config/media.go`, `internal/infrastructure/security/media_ticket.go`,
`internal/infrastructure/redis/media.go`, `internal/infrastructure/sfu`,
`internal/transport/mediaworker`, `internal/usecase/media`, соответствующие tests,
`tests/integration/sfu_media_test.go`, `media_browser_test.go`,
`frontend/src/media.ts`, `useMedia.ts`, `media.test.ts`, `frontend/e2e/media.spec.ts`,
Dockerfile worker и optional Compose overrides.

Изменены API bootstrap, WS handler, Hub disconnect/session validation, frontend
RealtimePanel/ConferencePage/realtime types/styles/Playwright config, Nginx media CSP, Compose, env example,
README, Makefile, CI build/deploy targets и документация. Миграции PostgreSQL этапом 3 не добавляются.

## 10 Docker и запуск

В production задайте отдельные случайные MEDIA_TICKET_SECRET и MEDIA_INTERNAL_SECRET
не короче 32 байт. В local/dev пустые значения безопасно выводятся разными HMAC purposes
из существующего JWT_SECRET; hardcoded общего секрета нет. Не сохраняйте секреты в Git.

```bash
docker compose up -d --build media-worker api frontend https
docker compose ps media-worker api frontend
```

UI: http://localhost:5173 или https://localhost:18482. Internal media HTTP/metrics
не опубликованы на host/gateway. Health endpoint возвращает только готовность,
management/metrics требуют internal secret. Новый Docker image non-root и без FFmpeg.
PostgreSQL/RabbitMQ/MinIO/recorder-worker не требуется пересоздавать для обновления SFU.
В локальном окружении обновлены только media-worker, API и frontend. Проверены
frontend/API HTTP 200, worker health 204, отказ metrics без internal secret (401)
и HTTPS API health 200. При проверке локального self-signed HTTPS использовался curl
без проверки доверия сертификату; доверие сертификату в браузере этим не подтверждается.

Base Compose публикует TCP mux; для UDP-only или диапазона используйте один override
(нужен Compose с поддержкой `!override`, проверено на установленном Compose):

```bash
docker compose -f docker-compose.yml -f docker-compose.media-udp.yml up -d --build
docker compose -f docker-compose.yml -f docker-compose.media-range.yml up -d --build
```

Range override использует MEDIA_RANGE_MIN/MAX, по умолчанию 50100–50200, отключает mux
и TCP. Не пытайтесь только выставить MEDIA_TCP_PORT=0 в base Compose: нужно убрать
TCP mapping через override. Firewall должен разрешать выбранный UDP порт/диапазон.

## 11 Проверки и race detector

Проверены существующие тесты до изменений. Новые suites покрывают room/peer/track
lifecycle, дубли, limits, ранний ICE, codec/SDP validation, concurrent negotiation,
publication-vs-leave, join-vs-shutdown, tickets, routing/fencing и protected HTTP.
Добавлены регрессии delayed answer/late publisher, недостаточных receive slots,
отсутствующего Ready и отсутствия утечки ICE credentials через upstream logging.
Real PostgreSQL/Redis integration проверяет authenticated API → WS → ticket → worker HTTP
→ SFU, двусторонние A/V RTP, третьего позднего участника, stop, disconnect/reconnect
с сохранением membership, invalid/cross-conference ticket и empty-room cleanup.

```bash
go test ./...
go test -race ./...
go vet ./...
RECORDER_STAGE1_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
RECORDER_STAGE2_TEST_REDIS_ADDR=127.0.0.1:6380 \
go test -race ./tests/integration ./internal/infrastructure/redis -count=1
```

44 frontend unit tests, 8 обычных UI E2E и production build проходят. Изолированный Chromium с fake devices
проверил реальное декодирование удалённого видео (videoWidth > 0), наличие живого audio
и video, новый mediaPeerId после WS reconnect, explicit restart, stop и нулевые ресурсы
engine после teardown. Это не проверка физической камеры или субъективного качества звука.

```bash
RECORDER_STAGE3_BROWSER_E2E=true \
RECORDER_STAGE1_TEST_POSTGRES_DSN='postgres://go_recorder:go_recorder_pass@127.0.0.1:5433/go_recorder?sslmode=disable' \
RECORDER_STAGE2_TEST_REDIS_ADDR=127.0.0.1:6380 \
go test ./tests/integration -run '^TestStageThreeBrowserMedia$' -count=1 -v
```

Нужны Node 24 и установленный Chromium. Vite использует 5175; одновременно запускайте
только один browser harness. Fixture создаёт случайную отдельную БД и Redis namespace,
удаляет только свои данные. Общая БД, пользовательские записи и browser profile не затрагиваются.

## 12 Smoke проверки двух трёх и пяти участников

`TestSFUMediaSmoke` проверяет двусторонние audio/video для 2, 3, 5 Pion endpoints,
включая late join, остановку publisher/track, новые media endpoints и полный cleanup.
Дополнительный test повторяет создание/удаление room. Во всех зафиксированных
no-race прогонах dropped=0, публикации и subscriptions удалены после закрытия.

```bash
go test ./internal/infrastructure/sfu -run '^TestSFUMediaSmoke$' -count=1 -v
```

## 13 Наблюдения CPU памяти и goroutines

Один no-race локальный прогон, нагрузка около секунды после negotiation:

| Endpoints | Время сценария | CPU процесса | Goroutines во время медиа | Heap во время медиа | Forwarded packets |
| --- | --- | --- | --- | --- | --- |
| 2 | 1.14 с | 122 мс | 117 | 2.03 MiB | 207 |
| 3 | 1.20 с | 206 мс | 192 | 3.21 MiB | 636 |
| 5 | 1.32 с | 431 мс | 378 | 7.44 MiB | 2188 |

После cleanup goroutines вернулись к 3. Heap — моментальный HeapAlloc, не RSS и не
доказательство отсутствия утечек сам по себе. CPU включает тестовые Pion endpoints
в том же процессе, не только SFU. Синтетический RTP payload около четырёх байт:
это baseline корректности, не 720p load test и не production capacity. Numbers меняются
при race detector и параллельных процессах. Worker counters считают payload bytes.

## 14 Совместимость recorder

Пакеты recorder ingest, FFmpeg, record.start/stop, RabbitMQ consumer, MinIO и storage
не изменены. Recorder и SFU используют разные sockets/ports/PeerConnection lifecycles.
Существующий тестовый набор recorder продолжает проверяться вместе с проектом.
Синтетическая запись полного live Docker pipeline в этой работе не запускается:
это создало бы новые артефакты в пользовательском хранилище. Stage 3 не объявляет
legacy public recorder routes новым защищённым conference recording API.

## 15 Известные ограничения

- Firefox на этом Mac не проверен автоматизацией: прежняя macOS/Playwright проблема
  с isolated profile сохраняется. Разрешения ОС и пользовательский профиль не менялись.
- После retirement source для повторной камеры/микрофона нужен новый MediaPeer;
  reusing того же TrackRemote не поддерживается до отдельной реализации controls.
- Redis Pub/Sub не durable. При потере брокера API Hub этапа 2 закрывает WS и требует
  перезапуска instance для восстановления realtime; REST остаётся доступным.
- Нет simulcast/SVC, congestion policy, H264, screen sharing, recording, chat/moderation
  или production UI. 10 endpoints — конфигурационный target, не подтверждённая capacity.
- Redis Lua рассчитан на обычный Redis, не Redis Cluster. Worker identity должен быть
  уникальным. Lease fencing не обеспечивает бесшовную миграцию существующей room.
- Internal HTTP защищён secret и изолированной сетью; перед выходом за её границы нужен
  TLS/mTLS, firewall и rotation. Production TURN не развёрнут.

## 16 Риски перед следующим этапом

Нужны ручные Firefox/Safari проверки и real-device test, сетевые сценарии NAT/TURN,
длительный 720p тест на 5–10 endpoints с bandwidth/packet loss/CPU/RSS, защита от
bandwidth flooding, TURN credential service, broker recovery/readiness и operational
alerts. UI пока development-oriented; autoplay может потребовать отдельного жеста
пользователя в другом браузере. Эти работы не подменяются коротким synthetic baseline.

Ручная проверка: два аккаунта входят по одной ссылке, нажимают включение устройств,
проверяют звук/видео в обе стороны; третий входит позже; затем stop, закрытие вкладки,
повторный WS вход и явное включение медиа. Проверить, что devices освобождаются,
старые tiles исчезают и membership не удаляется при транспортном reconnect.

## 17 Предлагаемый контракт записи следующего этапа

Рекомендуемая граница: `Room → dedicated recording subscriber → recorder-worker`.
SFU предоставляет internal authenticated подписку на выбранные source IDs и поток
encoded RTP с codec parameters, SSRC/PT, clock rate, conference/participant/mediaPeer/track
binding и lifecycle publish/unpublish/end. Recorder отвечает за timestamps/synchronization,
файлы, сегменты, FFmpeg при необходимости и загрузку; SFU не становится FFmpeg process manager.

Команда start содержит recordId, conferenceId, ownership lease/fencing generation,
разрешённые source kinds и idempotency key. Ответ — recording subscriber ID и принятый
track snapshot. События track added/removed, worker lost, record stopped/failed имеют
sequence/correlation; повторный stop идемпотентен. Нужны явные backpressure/drop policy,
срок lease, reconnect semantics и правило сохранения membership при отказе recorder.
RabbitMQ можно сохранить для start/stop orchestration, но не для RTP. Composite, screen,
moderation и controls проектируются и реализуются только по отдельному запросу.
