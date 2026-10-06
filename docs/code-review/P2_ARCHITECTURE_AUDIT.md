# P2 — Architecture audit

Дата: 2026-10-06. Baseline: `27594d8` (после P0/P1). Этот документ создан **до первого изменения production-кода P2**. Итоги и фактические изменения: [P2_BEFORE_AFTER.md](P2_BEFORE_AFTER.md).

## 1. Baseline и методика

`go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`, govulncheck и frontend lint прошли. Staticcheck: единственное прежнее замечание S1024 в `tests/integration/persistent_auth_sessions_test.go:103`, относится к P3.

До изменений прошли: P0 WS repeated lifecycle/reconnect после Redis disconnect/transient prune failure (race), P1 recording page batch; SFU 2/3/5 participant tests в общем suite; отдельный профиль 5 peers + screen + recording + audio tap; 6 повторов RTP benchmark; DB workload на 1000 конференций; два isolated Linux recording pod сценария с настоящим FFmpeg и MP4. Дополнительные key API/auth/moderation tests выполнены до первого production edit: выявлен прежний неверный assert 403 для записи авторизованным участником (противоречит уже реализованному требованию пользователя). Удалён только этот assert, добавлен отдельный guest/account recording boundary test; повторный запуск фиксируется отдельно. Базовые подробности — `tmp/p2-architecture-20261006/evidence/baseline-*.{json,log}`. DSN и секреты в отчёт не включены.

AST inventory: 67 production Go packages под `internal/`, тестовые файлы исключены. Полный snapshot — `tmp/p2-architecture-20261006/evidence/inventory-before.json`. LOC — span декларации (включая внутренние комментарии); branches — количество AST if/for/range/case/comm, включая closures, **не cyclomatic complexity**. Public API ниже — функции/методы, структуры и interfaces; imports указаны полностью в snapshot. Inbound — production imports внутри `internal/`, не runtime calls и не test imports. Не путать package-level число функций с количеством методов одного manager.

## 2. Владение состоянием

| Объект | Владелец | Ресурс/граница |
|---|---|---|
| ConferenceParticipant | domain/conferences + postgres operation | Persisted membership, role, admission, kick, media policy version; не уничтожается при закрытии одного WS |
| ParticipantSession | realtime Hub + websocket client | Одна физическая realtime connection, heartbeat/presence/queue; peer-specific disconnect не удаляет другое соединение |
| MediaPeer | SFU manager/room/peer | Pion PC, tracks, RTCP, subscriptions и bounded sinks; cleanup idempotent и вне глобального lock |
| Recording | records + recording repository/outbox + recorder strategy | Durable status/lease/outbox и отдельный capture/FFmpeg/storage lifecycle; compositor не решает admission |

API знает media commands/Track DTO, а не Pion state. P0 join/leave, queue/backpressure и P1 RTP/compositor hot paths сохраняются. Операция, требующая атомарности, выражена методом repository (Start/Moderate/Invite); её транзакцию выполняет Postgres adapter. Перенос транзакции в usecase только ради слоёв не нужен.

## 3. Coupling и крупные компоненты

- `usecase/captions → infrastructure/ffmpeg`: используется только чистая PCM activity policy; перенести к captions owner.
- `usecase/recorder → composite/ffmpeg/local/s3/webrtc`: реальное orchestration coupling; стратегии уже отдельные типы. Разделять весь recorder сейчас опасно для finalize/recovery, сохранить как явный долг P2.
- `usecase/media.HTTPClient`: конкретный HTTP adapter расположен рядом с Controller, но Controller использует Transport interface; перемещение каталога само по себе мало помогает.
- `domain/records → gorm.io/datatypes`: persisted JSON representation; удаление требует отдельного compatibility review.
- `transport/mediaworker → Pion`: Engine contract использует ICECandidateInit; adapter boundary конкретного media worker, а не утечка в HTTP API.
- `app/notifications`: SSE state вместе с HTTP endpoints; оправдан transport lifecycle, но отдельный SSE subscription owner — возможный следующий P2.
- `operations.current`: atomic process singleton metrics; lifecycle один на процесс, изоляция collector registry сохранена. Не вводить DI framework ради замены всех counter calls.

| Компонент | Состояние/контракт до P2 | Решение |
|---|---|---|
| recordings.Service | 5 deps, Repository 7 methods, API/read + polling outbox | C: API service и CommandDispatcher, узкие 4/3 repo contracts |
| realtime.Hub | 18 fields, Repository 5 / Store 9 methods | B: убрать повтор membership policy; lifecycle ownership после P0 не дробить |
| sfu.Manager / peer | 22 / 32 fields, package 37 public callable declarations | Не разделять по LOC: room/peer ownership уже локальный; сохранить hot path |
| recorder.CompositeService | 10 fields, options 11 | Стратегия capture/recovery отдельна; дальнейшее разделение finalization — долг |
| integrations.Service | 6 fields, Repository 22 methods | Email/calendar/push разные причины изменения; split требует отдельного полного job/provider regression |
| mediaworker.Handler | 18 fields, Engine 12 methods | Media-worker auth/lease enforcement не переносить в API; command dispatch долг |

### Крупные функции до изменений

| Функция | LOC | Branches | Params | Решение |
|---|---:|---:|---:|---|
| `Composer.Capture` (internal/infrastructure/composite/capture.go:78) | 326 | 72 | 5 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `Handler.command` (internal/transport/mediaworker/handler.go:530) | 276 | 58 | 3 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `IntegrationRepository.Fanout` (internal/infrastructure/postgres/integration_repository.go:302) | 149 | 36 | 3 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `LoadStageSeven` (internal/config/product.go:52) | 141 | 35 | 1 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `validateSourceOffer` (internal/infrastructure/sfu/peer.go:741) | 121 | 34 | 6 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `ConferenceInvitationRepository.Invite` (internal/infrastructure/postgres/conference_invitation_repository.go:74) | 114 | 30 | 4 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `ConferenceRepository.Moderate` (internal/infrastructure/postgres/moderation_repository.go:28) | 103 | 30 | 5 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `client.read` (internal/transport/websocket/handler.go:650) | 155 | 28 | 0 | E: отделить envelope/dispatch от reader lifecycle |
| `Handler.egress` (internal/transport/mediaworker/control_egress.go:72) | 130 | 26 | 2 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `LiveAudio.Decode` (internal/infrastructure/ffmpeg/live_audio.go:29) | 120 | 26 | 4 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `CompositeService.run` (internal/usecase/recorder/composite.go:430) | 115 | 26 | 3 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `Worker.track` (internal/usecase/captions/worker.go:270) | 132 | 25 | 7 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `Handler.Events` (internal/app/notifications/handler.go:171) | 128 | 25 | 1 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `Manager.OfferSources` (internal/infrastructure/sfu/peer.go:405) | 129 | 24 | 5 | Сохранить, audit; не извлекать произвольные строки ради метрики |
| `ArchiveTracks` (internal/infrastructure/composite/archive.go:52) | 104 | 24 | 3 | Сохранить, audit; не извлекать произвольные строки ради метрики |

## 4. Дублирование, ошибки, state/events

**Реальное:** две идентичные Participant→media-policy projection в usecase/conferences и postgres; три admission/status проверки в Hub; media error sentinel→wire code и обратная классификация по `.Error()`; reader WS смешивает lifecycle и dispatch. **Intentional:** публичный HTTP JSON (`status/message`), internal media (`code`) и WS (`error` envelope + `replyTo`) — разные совместимые контракты. UUID parsing в HTTP и WS имеет разные правила envelope; не объединять в utils. Pagination уже в httpresponse. Redis ключи в Redis adapter; P0 stale session fencing не упрощать. Event constructors уже в domain/realtime.

Typed categories исходно покрывают 6 из 9. Добавить явные RateLimited/Timeout/Internal, сохранив статусы и safe messages существующих ошибок; raw context deadline не менять молча с 500 на 504. Root cause не должен пропадать при safe transport classification. Media codes должны жить в одном typed catalogue, включая legacy wire `error` compatibility. Legacy UDP recorder сравнивает тексты `closed`/`connection refused`; заменить стандартными typed network errors с проверкой.

Conference/recording transitions — domain constants + transactional repository; admission/record permission authoritative на backend. Versioned moderation outbox имеет fencing; durable `record.start` и realtime `recording.starting` имеют разные смыслы. Их не объединять. Retry owner: durable command outbox до подтверждённой доставки; job worker — provider jobs; HTTP media client не делает retry. Rabbit reconnect восстанавливает transport; не добавлять повтор execution на всех слоях. DB commit перед внешним side effect; существующая повторная WS publication at request/delivery сохранена (события idempotent UI).

## 5. Cleanup, DTO, interfaces, config и dead code

- P0: peer/session/room teardown уже idempotent; writer owns WS writes, stop owns cancellation; source buffers bounded. Нужна регрессия, а не новый общий ResourceManager.
- Найден потенциальный leak `operations.Runtime.Profiling`: waiter на ctx создаётся до bind; при bind error удерживается до process cancellation. D: сначала failing test, затем unregisterable cancellation callback, normal shutdown test.
- Record API service не нуждается в Rabbit/Redis и outbox methods. CommandDispatcher получает only Claim/Complete/Retry, Commander и Acquire; неиспользуемый Release в его Locker contract удалить, сам реальный Redis Release сохранить для других владельцев.
- DTO: RecordCard/ParticipantView/public status mapping сохранены; GORM models не выводить наружу новыми endpoints. Provider Email/Calendar/STT/AI/Embedding contracts полезны для adapters/tests.
- Config уже typed per subsystem; stage loaders велики, но работают один раз, defaults имеют validation tests. Poll timings 250ms/16/5s останутся локальной explicit dispatcher policy. Нет нового global config или DI container.
- Giant utils/common не найден. PCM activity helper ошибочно прикреплён к FFmpeg adapter; F переносит pure policy к captions.
- Legacy P2P/recorder/debug/recovery paths имеют routes/callers/tests; НЕ считать dead по возрасту. Staticcheck не показал unreachable private code. Удалять лишь доказанно избыточные projection/error-loop/contract после обновления всех callers.

## 6. План batches (до изменений)

| Batch | Область | Проверка |
|---|---|---|
| A | Typed error categories, safe root cause, media wire mapping; legacy network typed checks | HTTP/media error contract, wrapped causes, media-worker/WS regression, race |
| B | Membership policy/projection authoritative domain owner | Status×admission table, moderation/auth/rejoin/media integration, race |
| C | Recording API service vs durable CommandDispatcher | Ordered fake side effects/failure paths; Postgres transaction/outbox tests; Linux recording later |
| D | Profiling cleanup owner | Воспроизводящий failing test до fix; bind failure/ctx shutdown/race |
| E | WS reader vs protocol decode/dispatch | Envelope/legacy signal safety, WS lifecycle/reconnect integration/race |
| F | Remove unused contract/duplicate helpers, captions activity owner | PCM boundary cases, captions/provider tests/race, actual recording/audio smoke |

Каждый batch — отдельный diff/evidence checkpoint, без giant commit и без автоматического commit. После них полные gates и representative P1 comparison. API schemas, auth/logout policy, UI и deployment не менять. P3 не выполнять автоматически.

## 7. Полный package inventory (snapshot до P2)

Сокращения путей: ниже `internal/` опущен в заголовках; inbound/outbound включают прямые production зависимости. Полный список stdlib/external imports и struct/interface metrics находится в JSON snapshot. Каждый пакет получил ответственность и owner state, а не только LOC.

### `app`

Composition root процессов; создаёт адаптеры, связывает зависимости, отменяет контексты и закрывает клиенты.

- Inbound: нет прямых internal consumers.
- Outbound internal: `app/auth`, `app/captions`, `app/chat`, `app/conferences`, `app/content`, `app/engagement`, `app/integrations`, `app/notifications`, `app/platform`, `app/recordings`, `app/records`, `app/telemetry`, `config`, `domain/content`, `domain/integrations`, `domain/media`, `domain/realtime`, `domain/records`, `infrastructure/contentproviders`, `infrastructure/ffmpeg`, `infrastructure/health`, `infrastructure/liveproviders`, `infrastructure/postgres`, `infrastructure/providers`, `infrastructure/rabbitmq`, `infrastructure/redis`, `infrastructure/security`, `infrastructure/sfu`, `infrastructure/storage/s3`, `infrastructure/webrtc`, `infrastructure/worker`, `operations`, `transport/http`, `transport/http/middleware`, `transport/mediaworker`, `transport/websocket`, `usecase/analytics`, `usecase/auth`, `usecase/captions`, `usecase/chat`, `usecase/conferences`, `usecase/content`, `usecase/integrations`, `usecase/jobs`, `usecase/media`, `usecase/notifications`, `usecase/platform`, `usecase/realtime`, `usecase/recorder`, `usecase/recordings`, `usecase/search`.
- Public callable API (14): `RunAPI`, `RunLiveWorker`, `RunMediaWorker`, `RunMigrations`, `RunProductWorker`, `RunWorker`, `measuredAI.Summarize`, `measuredCalendar.CancelEvent`, `measuredCalendar.CreateEvent`, `measuredCalendar.GetEvent`, `measuredCalendar.UpdateEvent`, `measuredEmail.Send`, `measuredPush.Send`, `measuredSTT.Transcribe`.
- State/contracts (число полей/методов): structures `{"measuredAI": 1, "measuredCalendar": 1, "measuredEmail": 1, "measuredPush": 1, "measuredSTT": 1, "productServices": 3}`; interfaces `{}`.

### `app/auth`

HTTP auth: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/users`, `transport/http/middleware`.
- Public callable API (10): `Handler.Bootstrap`, `Handler.Login`, `Handler.Logout`, `Handler.Me`, `Handler.PersistentSessionsEnabled`, `Handler.Refresh`, `Handler.Register`, `Handler.UpdateProfile`, `Handler.WithSessionCookies`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 4}`; interfaces `{"Service": 4, "persistentService": 3}`.

### `app/captions`

HTTP captions: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `config`, `domain/analytics`, `domain/apperrors`, `transport/http/middleware`, `usecase/captions`.
- Public callable API (5): `Handler.Finals`, `Handler.Read`, `Handler.ReadAnalytics`, `Handler.Reindex`, `Handler.Set`.
- State/contracts (число полей/методов): structures `{"Handler": 5}`; interfaces `{}`.

### `app/chat`

HTTP chat: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/chat`, `transport/http/middleware`, `usecase/chat`.
- Public callable API (11): `Handler.Delete`, `Handler.Download`, `Handler.Edit`, `Handler.FinalizeAttachment`, `Handler.InitAttachment`, `Handler.List`, `Handler.MarkRead`, `Handler.ReadState`, `Handler.Send`, `Handler.Upload`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 1}`; interfaces `{}`.

### `app/conferences`

HTTP conferences: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/conferences`, `transport/http/middleware`, `usecase/conferences`.
- Public callable API (21): `ControlHandler.Media`, `ControlHandler.Moderate`, `GuestHandler.Join`, `Handler.Admission`, `Handler.Create`, `Handler.History`, `Handler.Join`, `Handler.JoinInvite`, `Handler.Leave`, `Handler.List`, `Handler.LookupInvite`, `Handler.Participants`, `Handler.Read`, `Handler.Schedule`, `Handler.Self`, `Handler.Timeline`, `Handler.Transition`, `InvitationHandler.Invite`, `InvitationHandler.Search`, `NewControlHandler`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"ControlHandler": 1, "GuestHandler": 2, "Handler": 1, "InvitationHandler": 1}`; interfaces `{"ControlService": 2, "InvitationService": 2, "Service": 14}`.

### `app/content`

HTTP content: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/content`, `operations`, `transport/http/middleware`.
- Public callable API (7): `Handler.RegenerateSummary`, `Handler.RetryTranscript`, `Handler.Search`, `Handler.Segments`, `Handler.Summary`, `Handler.Transcript`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 1}`; interfaces `{"Service": 6}`.

### `app/engagement`

HTTP engagement: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/ratelimit`, `transport/http/middleware`, `usecase/realtime`.
- Public callable API (2): `Handler.Reaction`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 3}`; interfaces `{"Limiter": 1}`.

### `app/httpresponse`

HTTP httpresponse: parse/validate → usecase → response; request context. Единый JSON error mapper, strict JSON decode и pagination.

- Inbound: `app/auth`, `app/captions`, `app/chat`, `app/conferences`, `app/content`, `app/engagement`, `app/integrations`, `app/notifications`, `app/platform`, `app/recordings`, `transport/http/middleware`, `transport/websocket`.
- Outbound internal: `domain/apperrors`.
- Public callable API (3): `BindJSON`, `Fail`, `Pagination`.
- State/contracts (число полей/методов): structures `{}`; interfaces `{}`.

### `app/integrations`

HTTP integrations: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/integrations`, `transport/http/middleware`, `usecase/integrations`.
- Public callable API (13): `Handler.CalendarMappings`, `Handler.Calendars`, `Handler.Callback`, `Handler.Capabilities`, `Handler.Connect`, `Handler.Devices`, `Handler.Disconnect`, `Handler.MockConnect`, `Handler.Preferences`, `Handler.RegisterDevice`, `Handler.RevokeDevice`, `Handler.SavePreferences`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 1, "oauthCallbackRequest": 2}`; interfaces `{}`.

### `app/notifications`

HTTP notifications: parse/validate → usecase → response; request context. SSE handler дополнительно владеет subscription/keepalive и per-user counters.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/ratelimit`, `domain/realtime`, `transport/http/middleware`, `usecase/notifications`.
- Public callable API (5): `Handler.BeginDrain`, `Handler.Events`, `Handler.List`, `Handler.Read`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 9, "syncCounts": 2}`; interfaces `{"Limiter": 1, "Subscriber": 1, "Verifier": 1, "persistentVerifier": 2}`.

### `app/platform`

HTTP platform: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/platform`.
- Public callable API (2): `Handler.Capabilities`, `Handler.Summary`.
- State/contracts (число полей/методов): structures `{"Handler": 2}`; interfaces `{"Service": 2}`.

### `app/recordings`

HTTP recordings: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/records`, `transport/http/middleware`.
- Public callable API (5): `Handler.List`, `Handler.Read`, `Handler.Start`, `Handler.Stop`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 1}`; interfaces `{"Service": 4}`.

### `app/records`

HTTP records: parse/validate → usecase → response; request context. Legacy record endpoints; compatibility error schema сохранена.

- Inbound: `app`, `transport/http`.
- Outbound internal: `domain/records`, `infrastructure/worker`.
- Public callable API (8): `Handler.CountByConference`, `Handler.End`, `Handler.List`, `Handler.Offer`, `Handler.Read`, `Handler.Start`, `NewHandler`, `NewHandlerWithSignaler`.
- State/contracts (число полей/методов): structures `{"Handler": 2}`; interfaces `{"Service": 5, "WorkerSignaler": 1}`.

### `app/telemetry`

HTTP telemetry: parse/validate → usecase → response; request context.

- Inbound: `app`, `transport/http`.
- Outbound internal: нет.
- Public callable API (2): `Handler.Receive`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Frame": 3, "Handler": 3, "Report": 5}`; interfaces `{}`.

### `buildinfo`

Версия сборки, immutable metadata; lifecycle отсутствует.

- Inbound: `operations`, `usecase/platform`.
- Outbound internal: нет.
- Public callable API (3): `Current`, `Handler`, `ValidVersion`.
- State/contracts (число полей/методов): structures `{"Info": 3}`; interfaces `{}`.

### `config`

Typed config подсистем, env parsing и validation при запуске; mutable runtime state отсутствует.

- Inbound: `app`, `app/captions`, `infrastructure/liveproviders`, `infrastructure/postgres`, `infrastructure/redis`, `operations`, `transport/mediaworker`, `transport/websocket`, `usecase/captions`, `usecase/media`, `usecase/platform`, `usecase/recorder`, `usecase/search`.
- Outbound internal: `domain/realtime`.
- Public callable API (12): `Config.IsLocal`, `Load`, `LoadComposite`, `LoadMedia`, `LoadRealtime`, `LoadStageEight`, `LoadStageSeven`, `LoadTURN`, `MediaConfig.Validate`, `RealtimeConfig.Validate`, `TURNConfig.ClientICE`, `ValidateMediaEndpoint`.
- State/contracts (число полей/методов): structures `{"CompositeConfig": 10, "Config": 33, "MediaConfig": 33, "OperationsConfig": 12, "ProviderConfig": 3, "RateLimitConfig": 11, "RealtimeConfig": 17, "SMTPConfig": 7, "StageEightConfig": 21, "StageSevenConfig": 41, "TURNConfig": 4}`; interfaces `{}`.

### `domain/analytics`

Аналитические события и агрегаты; значения без фоновых ресурсов.

- Inbound: `app/captions`, `infrastructure/postgres`, `usecase/analytics`.
- Outbound internal: нет.
- Public callable API (0): types/contracts only.
- State/contracts (число полей/методов): structures `{"Conference": 10, "Interval": 3, "Participant": 7, "Point": 2}`; interfaces `{}`.

### `domain/apperrors`

Категории прикладных ошибок; значения без lifecycle.

- Inbound: `app/auth`, `app/captions`, `app/chat`, `app/conferences`, `app/content`, `app/engagement`, `app/httpresponse`, `app/integrations`, `app/notifications`, `app/recordings`, `domain/chat`, `domain/conferences`, `domain/notifications`, `domain/users`, `infrastructure/postgres`, `infrastructure/redis`, `infrastructure/security`, `transport/http/middleware`, `transport/mediaworker`, `transport/websocket`, `usecase/auth`, `usecase/chat`, `usecase/conferences`, `usecase/content`, `usecase/integrations`, `usecase/realtime`, `usecase/recorder`, `usecase/recordings`.
- Outbound internal: нет.
- Public callable API (3): `Error.Error`, `Error.Unwrap`, `New`.
- State/contracts (число полей/методов): structures `{"Error": 2}`; interfaces `{}`.

### `domain/captions`

Сессии/сегменты субтитров, STT boundary; значения без goroutines.

- Inbound: `infrastructure/liveproviders`, `infrastructure/postgres`, `usecase/captions`.
- Outbound internal: нет.
- Public callable API (0): types/contracts only.
- State/contracts (число полей/методов): structures `{"Caption": 10, "Event": 8, "SessionConfig": 6, "State": 10}`; interfaces `{"LiveTranscriptionProvider": 1, "Session": 3}`.

### `domain/chat`

Сообщения, вложения и validation; значения без lifecycle.

- Inbound: `app/chat`, `infrastructure/postgres`, `usecase/chat`.
- Outbound internal: `domain/apperrors`.
- Public callable API (7): `Attachment.Prefix`, `Attachment.TableName`, `Message.TableName`, `NormalizeInit`, `NormalizeSend`, `NormalizeText`, `UUID`.
- State/contracts (число полей/методов): structures `{"Attachment": 18, "EditRequest": 1, "InitRequest": 4, "Message": 15, "Page": 4, "ReadRequest": 1, "ReadState": 4, "ReplyPreview": 4, "SendRequest": 4}`; interfaces `{}`.

### `domain/conferences`

Conference и Participant membership, admission/role/status policy, публичные views.

- Inbound: `app/conferences`, `domain/realtime`, `infrastructure/postgres`, `transport/http`, `usecase/conferences`, `usecase/integrations`, `usecase/realtime`.
- Outbound internal: `domain/apperrors`, `domain/users`.
- Public callable API (19): `AdmissionRequest.Validate`, `CanModerate`, `CanTransition`, `Conference.TableName`, `Conference.View`, `DecodeTimelineCursor`, `EncodeTimelineCursor`, `Invitation.TableName`, `ModerationRequest.Validate`, `NormalizeTitle`, `Participant.CanAdmit`, `Participant.CanParticipate`, `Participant.CanReadHistory`, `Participant.IsAdmitted`, `Participant.TableName`, `Participant.View`, `TimelineQuery.Validate`, `ValidateInvitationEmail`, `ValidateSchedule`.
- State/contracts (число полей/методов): structures `{"AdmissionRequest": 1, "Conference": 12, "CreateRequest": 4, "HistoryOwner": 2, "HistoryView": 9, "Invitation": 9, "InvitationRequest": 2, "InvitationResult": 4, "InvitationUser": 3, "InviteView": 5, "JoinRequest": 1, "MediaState": 5, "ModerationRequest": 3, "Participant": 20, "ParticipantView": 20, "RecordingSummary": 4, "ScheduleRequest": 2, "TimelineCursor": 2, "TimelinePage": 2, "TimelineQuery": 7, "View": 13}`; interfaces `{}`.

### `domain/content`

Материалы, summary и provider contracts; значения и interfaces.

- Inbound: `app`, `app/content`, `infrastructure/contentproviders`, `infrastructure/ffmpeg`, `infrastructure/postgres`, `usecase/content`, `usecase/search`.
- Outbound internal: нет.
- Public callable API (0): types/contracts only.
- State/contracts (число полей/методов): structures `{"AIRequest": 6, "ActionItem": 4, "Audio": 4, "RecordingSource": 5, "SearchPage": 6, "SearchQuery": 10, "SearchResult": 11, "Segment": 9, "SegmentPage": 4, "Summary": 17, "SummaryOutput": 4, "SummaryState": 4, "Transcript": 12, "TranscriptState": 4, "TranscriptionRequest": 6, "TranscriptionResult": 2}`; interfaces `{"AIProvider": 3, "TranscriptionProvider": 2}`.

### `domain/integrations`

Email/calendar/push contracts и persisted state; без запуска providers.

- Inbound: `app`, `app/integrations`, `infrastructure/postgres`, `infrastructure/providers`, `usecase/integrations`.
- Outbound internal: нет.
- Public callable API (7): `CalendarConnection.TableName`, `CalendarMapping.TableName`, `DefaultPreferences`, `Device.TableName`, `OAuthState.TableName`, `Preferences.Allows`, `Preferences.TableName`.
- State/contracts (число полей/методов): structures `{"CalendarConnection": 11, "CalendarCredentials": 4, "CalendarEvent": 9, "CalendarMapping": 9, "Capabilities": 5, "Device": 10, "DeviceRequest": 2, "EmailMessage": 5, "OAuthState": 7, "Preferences": 8, "Providers": 5, "PushMessage": 5}`; interfaces `{"CalendarProvider": 4, "EmailProvider": 1, "OAuthProvider": 4, "PushProvider": 1}`.

### `domain/jobs`

Durable jobs, classified retry errors, provider/work contracts.

- Inbound: `infrastructure/contentproviders`, `infrastructure/ffmpeg`, `infrastructure/postgres`, `infrastructure/providers`, `operations`, `usecase/analytics`, `usecase/content`, `usecase/integrations`, `usecase/jobs`, `usecase/search`.
- Outbound internal: нет.
- Public callable API (1): `Error.Error`.
- State/contracts (число полей/методов): structures `{"Count": 3, "Error": 3, "Job": 13}`; interfaces `{"Repository": 3}`.

### `domain/media`

MediaPeer binding, tracks/publications/policy, internal command/result; не frontend DTO.

- Inbound: `app`, `infrastructure/composite`, `infrastructure/ffmpeg`, `infrastructure/liveproviders`, `infrastructure/postgres`, `infrastructure/redis`, `infrastructure/security`, `infrastructure/sfu`, `transport/mediaworker`, `transport/websocket`, `usecase/captions`, `usecase/conferences`, `usecase/media`, `usecase/recorder`.
- Outbound internal: `domain/realtime`.
- Public callable API (3): `ErrorCode`, `ParticipantPolicy.Allows`, `SourceKind`.
- State/contracts (число полей/методов): structures `{"Binding": 6, "Command": 13, "EgressFrame": 7, "EgressRequest": 6, "EgressTrack": 6, "ParticipantPolicy": 5, "PeerView": 5, "Publication": 3, "Result": 11, "Route": 3, "Signal": 6, "Track": 6, "VideoCaptureTarget": 3, "Worker": 3}`; interfaces `{"EgressSubscription": 6}`.

### `domain/notifications`

Notification values, cursor и validation; без SSE resources.

- Inbound: `infrastructure/postgres`, `usecase/integrations`, `usecase/notifications`.
- Outbound internal: `domain/apperrors`.
- Public callable API (3): `DecodeCursor`, `EncodeCursor`, `Notification.TableName`.
- State/contracts (число полей/методов): structures `{"Cursor": 3, "Notification": 9, "Page": 3, "Payload": 8}`; interfaces `{}`.

### `domain/platform`

Capability snapshot; значения.

- Inbound: `app/platform`, `infrastructure/postgres`, `usecase/platform`.
- Outbound internal: нет.
- Public callable API (0): types/contracts only.
- State/contracts (число полей/методов): structures `{"Capabilities": 6, "Failure": 3, "Summary": 12}`; interfaces `{}`.

### `domain/ratelimit`

Bucket/decision и limiter contract; без конкретного Redis.

- Inbound: `app/engagement`, `app/notifications`, `infrastructure/redis`, `transport/http/middleware`, `transport/websocket`.
- Outbound internal: нет.
- Public callable API (0): types/contracts only.
- State/contracts (число полей/методов): structures `{"Result": 5}`; interfaces `{}`.

### `domain/realtime`

ParticipantSession, presence и WS envelope; lifecycle соединения находится в transport/hub.

- Inbound: `app`, `app/notifications`, `config`, `domain/media`, `infrastructure/postgres`, `infrastructure/redis`, `transport/mediaworker`, `transport/websocket`, `usecase/captions`, `usecase/chat`, `usecase/conferences`, `usecase/media`, `usecase/notifications`, `usecase/realtime`, `usecase/recordings`.
- Outbound internal: `domain/conferences`.
- Public callable API (4): `AllowedReaction`, `Event`, `LowPriorityEvent`, `Session.TableName`.
- State/contracts (число полей/методов): structures `{"Bus": 6, "Envelope": 7, "ICEConfig": 3, "ICEServer": 3, "Identity": 3, "Presence": 4, "Session": 9, "Signal": 5, "State": 4}`; interfaces `{"Subscription": 2}`.

### `domain/records`

Recording statuses/transitions/cards/outbox; GORM datatypes остаются persistence coupling.

- Inbound: `app`, `app/recordings`, `app/records`, `infrastructure/postgres`, `infrastructure/rabbitmq`, `infrastructure/storage/s3`, `infrastructure/webrtc`, `infrastructure/worker`, `usecase/platform`, `usecase/recorder`, `usecase/recordings`.
- Outbound internal: нет.
- Public callable API (17): `IsComposite`, `IsSupportedRecordStatus`, `IsSupportedVideoQuality`, `IsTerminalStatus`, `NormalizeStartRequest`, `OutboxCommand.TableName`, `PublicStatus`, `Record.TableName`, `RecordEvent.TableName`, `RecordFile.TableName`, `RecordSegment.TableName`, `ValidConferenceMode`, `ValidateConferenceIDs`, `ValidateEndRequest`, `ValidateRecordStatusFilter`, `ValidateStartRequest`, `VideoSettingsForQuality`.
- State/contracts (число полей/методов): structures `{"Command": 4, "ConferenceRecordItem": 9, "ConferenceRecordSummary": 3, "ConferenceStartRequest": 2, "EndRequest": 2, "ICEServer": 1, "OutboxCommand": 8, "Record": 28, "RecordCard": 4, "RecordDetails": 4, "RecordEvent": 9, "RecordFile": 17, "RecordFileView": 2, "RecordSegment": 15, "RecordSegmentView": 3, "Response": 2, "StartRequest": 6, "StartResponse": 5, "VideoSettings": 5, "WebRTCAnswerResponse": 2, "WebRTCInfo": 3, "WebRTCOfferRequest": 2}`; interfaces `{}`.

### `domain/search`

Search query/results, embedding contract; значения.

- Inbound: `infrastructure/contentproviders`, `infrastructure/postgres`, `usecase/search`.
- Outbound internal: нет.
- Public callable API (0): types/contracts only.
- State/contracts (число полей/методов): structures `{"Chunk": 15, "EmbeddingRequest": 5}`; interfaces `{"EmbeddingProvider": 1}`.

### `domain/users`

User/session/validation; persistent auth policy.

- Inbound: `app/auth`, `domain/conferences`, `infrastructure/postgres`, `usecase/auth`, `usecase/conferences`.
- Outbound internal: `domain/apperrors`.
- Public callable API (7): `NormalizeDisplayName`, `NormalizeEmail`, `NormalizeRegister`, `User.ParticipantName`, `User.TableName`, `User.View`, `ValidateEmail`.
- State/contracts (число полей/методов): structures `{"AuthSession": 5, "LoginRequest": 2, "LoginResponse": 6, "RegisterRequest": 3, "UpdateProfileRequest": 1, "User": 8, "View": 7}`; interfaces `{}`.

### `infrastructure/composite`

RTP source writers, local files, compositor/FFmpeg execution; Capture владеет созданными writers/processes.

- Inbound: `usecase/recorder`.
- Outbound internal: `domain/media`, `operations`.
- Public callable API (14): `ArchiveTracks`, `Composer.Arguments`, `Composer.Capture`, `Composer.Compose`, `Composer.Recover`, `GridLayout`, `NewComposer`, `OnlyEgressEnded`, `TimelineOrigin`, `archiveReader.Read`, `boundedLog.String`, `boundedLog.Write`, `sourceWriter.Close`, `sourceWriter.WriteRTP`.
- State/contracts (число полей/методов): structures `{"CaptureOptions": 5, "Chunk": 6, "Composer": 6, "Layout": 4, "Source": 4, "Tile": 5, "TrackFragment": 7, "TrackManifest": 2, "archiveReader": 2, "boundedLog": 2, "encodedSample": 4, "sourceState": 5, "sourceWriter": 12}`; interfaces `{}`.

### `infrastructure/contentproviders`

HTTP STT/AI/embedding providers; bounded clients и request bodies.

- Inbound: `app`.
- Outbound internal: `domain/content`, `domain/jobs`, `domain/search`.
- Public callable API (9): `NewAIProvider`, `NewEmbeddingProvider`, `NewTranscriptionProvider`, `embeddings.Embed`, `intelligence.Model`, `intelligence.Name`, `intelligence.Summarize`, `transcription.Name`, `transcription.Transcribe`.
- State/contracts (число полей/методов): structures `{"embeddings": 1, "intelligence": 1, "provider": 5, "transcription": 1}`; interfaces `{}`.

### `infrastructure/ffmpeg`

FFmpeg/ffprobe subprocesses, pipes/temp files, audio decode; отмена процесса владельцем операции.

- Inbound: `app`, `infrastructure/webrtc`, `usecase/captions`, `usecase/recorder`.
- Outbound internal: `domain/content`, `domain/jobs`, `domain/media`, `operations`.
- Public callable API (17): `AudioActive`, `LiveAudio.Decode`, `NewPostProcessor`, `NewSegmentRecorder`, `NewTranscriptionAudio`, `PostProcessor.Finalize`, `PostProcessor.FinalizeAudio`, `PostProcessor.FinalizeComposite`, `PostProcessor.ValidateOutput`, `SegmentProcess.Done`, `SegmentProcess.Stderr`, `SegmentProcess.Stop`, `SegmentProcess.WaitErr`, `SegmentRecorder.Start`, `TranscriptionAudio.Open`, `logTail.String`, `logTail.Write`.
- State/contracts (число полей/методов): structures `{"LiveAudio": 1, "OutputValidation": 8, "PostProcessor": 2, "RTPTrack": 7, "Result": 8, "Segment": 5, "SegmentProcess": 8, "SegmentRecorder": 2, "TranscriptionAudio": 6, "logTail": 2}`; interfaces `{"RecordingObjectReader": 1}`.

### `infrastructure/health`

HTTP dependency health checks; request-scoped resources.

- Inbound: `app`.
- Outbound internal: нет.
- Public callable API (3): `HTTPReady.Close`, `HTTPReady.Ready`, `NewHTTPReady`.
- State/contracts (число полей/методов): structures `{"HTTPReady": 2}`; interfaces `{}`.

### `infrastructure/liveproviders`

Media egress и streaming STT adapters; HTTP/WS stream закрывает consumer.

- Inbound: `app`.
- Outbound internal: `config`, `domain/captions`, `domain/media`.
- Public callable API (9): `NewTap`, `Provider.StartSession`, `Tap.Open`, `mockSession.Close`, `mockSession.Events`, `mockSession.WriteAudio`, `wsSession.Close`, `wsSession.Events`, `wsSession.WriteAudio`.
- State/contracts (число полей/методов): structures `{"Provider": 4, "Tap": 3, "mockSession": 7, "wsSession": 8}`; interfaces `{"Registry": 1}`.

### `infrastructure/postgres`

SQL persistence и атомарные operation transactions/outbox; repositories используют общий DB pool от app.

- Inbound: `app`.
- Outbound internal: `config`, `domain/analytics`, `domain/apperrors`, `domain/captions`, `domain/chat`, `domain/conferences`, `domain/content`, `domain/integrations`, `domain/jobs`, `domain/media`, `domain/notifications`, `domain/platform`, `domain/realtime`, `domain/records`, `domain/search`, `domain/users`, `usecase/captions`, `usecase/integrations`, `usecase/search`.
- Public callable API (172): `AnalyticsRepository.Read`, `AnalyticsRepository.Save`, `AnalyticsRepository.Source`, `AnalyticsRepository.Tick`, `AuthSessionRepository.AuthorizeLegacy`, `AuthSessionRepository.Bootstrap`, `AuthSessionRepository.Create`, `AuthSessionRepository.GetByHash`, `AuthSessionRepository.GetByID`, `AuthSessionRepository.RevokeByHash`, `AuthSessionRepository.RevokeByID`, `AuthSessionRepository.RevokeLegacy`, `CaptionsRepository.Claim`, `CaptionsRepository.Finals`, `CaptionsRepository.Finish`, `CaptionsRepository.Observe`, `CaptionsRepository.Read`, `CaptionsRepository.Renew`, `CaptionsRepository.SaveFinal`, `CaptionsRepository.Set`, `CaptionsRepository.Speaker`, `CaptionsRepository.Status`, `ChatRepository.AbortUpload`, `ChatRepository.AttachmentForFinalize`, `ChatRepository.ClaimUpload`, `ChatRepository.CleanupCandidates`, `ChatRepository.CompleteCleanup`, `ChatRepository.CompleteUpload`, `ChatRepository.Delete`, `ChatRepository.DownloadAttachment`, `ChatRepository.Edit`, `ChatRepository.FinalizeAttachment`, `ChatRepository.InitAttachment`, `ChatRepository.List`, `ChatRepository.MarkRead`, `ChatRepository.ReadState`, `ChatRepository.Send`, `ConferenceInvitationRepository.Invite`, `ConferenceInvitationRepository.SearchInvitationUsers`, `ConferenceRecordingRepository.Accessible`, `ConferenceRecordingRepository.ClaimCommand`, `ConferenceRecordingRepository.CompleteCommand`, `ConferenceRecordingRepository.List`, `ConferenceRecordingRepository.RetryCommand`, `ConferenceRecordingRepository.Start`, `ConferenceRecordingRepository.StartMode`, `ConferenceRecordingRepository.Stop`, `ConferenceRepository.ClearDisconnectedMedia`, `ConferenceRepository.Create`, `ConferenceRepository.DecideAdmission`, `ConferenceRepository.Get`, `ConferenceRepository.GetByInvite`, `ConferenceRepository.History`, `ConferenceRepository.Join`, `ConferenceRepository.Leave`, `ConferenceRepository.ListForUser`, `ConferenceRepository.MediaPolicy`, `ConferenceRepository.Membership`, `ConferenceRepository.Moderate`, `ConferenceRepository.Participants`, `ConferenceRepository.ParticipantsVisible`, `ConferenceRepository.ReconcileParticipants`, `ConferenceRepository.Timeline`, `ConferenceRepository.Transition`, `ConferenceRepository.UpdateMediaState`, `ConferenceRepository.UpdateSchedule`, `Connect`, `ContentRepository.FailJob`, `ContentRepository.QueueSummary`, `ContentRepository.QueueTranscript`, `ContentRepository.SaveSummary`, `ContentRepository.SaveTranscript`, `ContentRepository.Search`, `ContentRepository.Segments`, `ContentRepository.StartSummary`, `ContentRepository.StartTranscript`, `ContentRepository.Summary`, `ContentRepository.Transcript`, `GuestRepository.JoinGuest`, `IntegrationRepository.AcquireCalendar`, `IntegrationRepository.CalendarMappings`, `IntegrationRepository.CompleteDelivery`, `IntegrationRepository.CompleteInvitation`, `IntegrationRepository.Conference`, `IntegrationRepository.Connections`, `IntegrationRepository.Delivery`, `IntegrationRepository.Devices`, `IntegrationRepository.FailCalendars`, `IntegrationRepository.Fanout`, `IntegrationRepository.InvitationDelivery`, `IntegrationRepository.MappingsForSync`, `IntegrationRepository.Preferences`, `IntegrationRepository.RefreshConnection`, `IntegrationRepository.RevokeConnection`, `IntegrationRepository.RevokeDevice`, `IntegrationRepository.SaveConnection`, `IntegrationRepository.SaveDevice`, `IntegrationRepository.SaveMapping`, `IntegrationRepository.SaveOAuthState`, `IntegrationRepository.SavePreferences`, `IntegrationRepository.ScheduleCalendarSync`, `IntegrationRepository.ScheduleReminders`, `IntegrationRepository.TakeOAuthState`, `JobRepository.Claim`, `JobRepository.Counts`, `JobRepository.Finish`, `NewAnalyticsRepository`, `NewAuthSessionRepository`, `NewCaptionsRepository`, `NewChatRepository`, `NewConferenceInvitationRepository`, `NewConferenceRecordingRepository`, `NewConferenceRepository`, `NewContentRepository`, `NewGuestRepository`, `NewIntegrationRepository`, `NewJobRepository`, `NewNotificationRepository`, `NewPlatformRepository`, `NewRecordRepository`, `NewSearchRepository`, `NewSessionRepository`, `NewUserRepository`, `NotificationRepository.DisableLegacyReminders`, `NotificationRepository.Generate`, `NotificationRepository.List`, `NotificationRepository.Pending`, `NotificationRepository.Published`, `NotificationRepository.Read`, `ParticipantPolicy`, `PlatformRepository.IsAdmin`, `PlatformRepository.Summary`, `RecordRepository.AddEvent`, `RecordRepository.ClaimComposite`, `RecordRepository.Create`, `RecordRepository.ExpireNextRecording`, `RecordRepository.FindByUUID`, `RecordRepository.FindDetailsByUUID`, `RecordRepository.FindDetailsByUUIDs`, `RecordRepository.LegacyConferenceAllowed`, `RecordRepository.List`, `RecordRepository.ListActiveComposite`, `RecordRepository.ListDetails`, `RecordRepository.ListSummaryDetailsByConferenceIDs`, `RecordRepository.MarkFailed`, `RecordRepository.MarkFinalizing`, `RecordRepository.MarkRecording`, `RecordRepository.MarkStopping`, `RecordRepository.MarkUploading`, `RecordRepository.ReleaseComposite`, `RecordRepository.RenewComposite`, `RecordRepository.SaveCompositeArtifacts`, `RecordRepository.SaveFinalArtifacts`, `RecordRepository.TransitionComposite`, `RunMigrations`, `RunStartupMigrations`, `SearchRepository.Available`, `SearchRepository.Cached`, `SearchRepository.Fail`, `SearchRepository.Reindex`, `SearchRepository.Save`, `SearchRepository.Search`, `SearchRepository.Source`, `SessionRepository.Authorize`, `SessionRepository.Close`, `SessionRepository.Open`, `SessionRepository.Roster`, `SessionRepository.Stale`, `UserRepository.Create`, `UserRepository.GetByEmail`, `UserRepository.GetByID`, `UserRepository.UpdateDisplayName`.
- State/contracts (число полей/методов): structures `{"AnalyticsRepository": 1, "AuthSessionRepository": 1, "CaptionsRepository": 1, "ChatRepository": 1, "ConferenceInvitationRepository": 1, "ConferenceRecordingRepository": 1, "ConferenceRepository": 1, "ContentRepository": 1, "GuestRepository": 1, "IntegrationRepository": 1, "JobRepository": 1, "NotificationRepository": 2, "PlatformRepository": 1, "RecordRepository": 1, "SearchRepository": 1, "SessionRepository": 1, "UserRepository": 1, "authLegacyToken": 4, "authSessionToken": 2, "migration": 3}`; interfaces `{}`.

### `infrastructure/providers`

SMTP/calendar/push/OAuth providers; сетевые операции, classified failures.

- Inbound: `app`.
- Outbound internal: `domain/integrations`, `domain/jobs`.
- Public callable API (14): `CalendarAdapter.CancelEvent`, `CalendarAdapter.CreateEvent`, `CalendarAdapter.GetEvent`, `CalendarAdapter.UpdateEvent`, `EmailAdapter.Send`, `NewIntegrations`, `NewOAuth`, `NewSMTPEmail`, `OAuth.AuthorizeURL`, `OAuth.Exchange`, `OAuth.Refresh`, `OAuth.Revoke`, `PushAdapter.Send`, `SMTPEmail.Send`.
- State/contracts (число полей/методов): structures `{"AdapterConfig": 5, "CalendarAdapter": 1, "EmailAdapter": 1, "IntegrationConfig": 6, "OAuth": 2, "OAuthConfig": 9, "PushAdapter": 1, "SMTPConfig": 7, "SMTPEmail": 3, "calendarInput": 2, "gateway": 4, "tokenResponse": 5}`; interfaces `{}`.

### `infrastructure/rabbitmq`

Durable publisher/consumer, confirms/reconnect; клиент владеет AMQP connection/channel.

- Inbound: `app`.
- Outbound internal: `domain/records`, `operations`.
- Public callable API (11): `Consumer.Active`, `Consumer.BeginDrain`, `Consumer.Check`, `Consumer.Close`, `Consumer.Consume`, `NewConsumer`, `NewPublisher`, `Publisher.Check`, `Publisher.Close`, `Publisher.StartRecord`, `Publisher.StopRecord`.
- State/contracts (число полей/методов): structures `{"Consumer": 12, "Options": 6, "Publisher": 9}`; interfaces `{}`.

### `infrastructure/redis`

Presence/pubsub/lease/ratelimit/locks; общий client от app, subscription закрывает владелец hub.

- Inbound: `app`.
- Outbound internal: `config`, `domain/apperrors`, `domain/media`, `domain/ratelimit`, `domain/realtime`.
- Public callable API (31): `ConferenceLock.Acquire`, `ConferenceLock.Release`, `MediaRegistry.Claim`, `MediaRegistry.GetOwner`, `MediaRegistry.RegisterWorker`, `MediaRegistry.Release`, `MediaRegistry.RemoveWorker`, `MediaRegistry.Renew`, `MediaRegistry.Workers`, `NewClient`, `NewConferenceLock`, `NewMediaRegistry`, `NewNotificationBus`, `NewRateLimiter`, `NewRealtimeStore`, `NotificationBus.Publish`, `NotificationBus.Subscribe`, `RateLimiter.Allow`, `RealtimeStore.Active`, `RealtimeStore.ConsumeTicket`, `RealtimeStore.Get`, `RealtimeStore.Missing`, `RealtimeStore.Prune`, `RealtimeStore.Publish`, `RealtimeStore.Register`, `RealtimeStore.SaveTicket`, `RealtimeStore.Subscribe`, `RealtimeStore.Touch`, `RealtimeStore.Unregister`, `realtimeSubscription.Close`, `realtimeSubscription.Receive`.
- State/contracts (число полей/методов): structures `{"ConferenceLock": 2, "MediaRegistry": 2, "NotificationBus": 2, "RateLimiter": 1, "RealtimeStore": 2, "realtimeSubscription": 1}`; interfaces `{}`.

### `infrastructure/security`

JWT/media tickets, encryption/Argon2; ключи и immutable options.

- Inbound: `app`.
- Outbound internal: `domain/apperrors`, `domain/media`.
- Public callable API (17): `GenerateInviteCode`, `MediaTickets.Issue`, `MediaTickets.Verify`, `NewMediaTickets`, `NewProviderTokens`, `NewTokenService`, `PasswordHasher.Hash`, `PasswordHasher.Verify`, `ProviderTokens.Decrypt`, `ProviderTokens.Encrypt`, `TokenService.Issue`, `TokenService.IssueGuest`, `TokenService.IssueSession`, `TokenService.Verify`, `TokenService.VerifyAuthorization`, `TokenService.VerifySession`, `TokenService.VerifyWithExpiry`.
- State/contracts (число полей/методов): structures `{"MediaTickets": 3, "PasswordHasher": 0, "ProviderTokens": 1, "TokenService": 2, "mediaClaims": 4, "sessionClaims": 3}`; interfaces `{}`.

### `infrastructure/sfu`

Room/MediaPeer/tracks/subscriptions/Pion; manager peer teardown, bounded recorder/audio tap sinks.

- Inbound: `app`.
- Outbound internal: `domain/media`.
- Public callable API (37): `Manager.Bindings`, `Manager.CloseConference`, `Manager.HasConference`, `Manager.ICE`, `Manager.Join`, `Manager.Leave`, `Manager.LeaveConnection`, `Manager.Offer`, `Manager.OfferSources`, `Manager.ParticipantPolicy`, `Manager.PeerBinding`, `Manager.Ready`, `Manager.SetPolicy`, `Manager.Shutdown`, `Manager.Snapshot`, `Manager.SubscribeAudio`, `Manager.SubscribeRecording`, `Manager.Tracks`, `Manager.Unpublish`, `NewManager`, `egress.Close`, `egress.Done`, `egress.Err`, `egress.Frames`, `egress.Keyframes`, `egress.Ping`, `safePionLogger.Debug`, `safePionLogger.Debugf`, `safePionLogger.Error`, `safePionLogger.Errorf`, `safePionLogger.Info`, `safePionLogger.Infof`, `safePionLogger.Trace`, `safePionLogger.Tracef`, `safePionLogger.Warn`, `safePionLogger.Warnf`, `safePionLoggerFactory.NewLogger`.
- State/contracts (число полей/методов): structures `{"Manager": 22, "Options": 21, "Stats": 15, "egress": 12, "outboundEvent": 2, "peer": 32, "publishedTrack": 13, "receiver": 3, "room": 7, "safePionLogger": 2, "safePionLoggerFactory": 1, "subscription": 9}`; interfaces `{}`.

### `infrastructure/storage/local`

Создание локальных каталогов записи; fs helper, lifecycle каталога у recorder.

- Inbound: `usecase/recorder`.
- Outbound internal: нет.
- Public callable API (1): `RemoveEmptyTrees`.
- State/contracts (число полей/методов): structures `{}`; interfaces `{}`.

### `infrastructure/storage/s3`

MinIO/S3 adapter и signed URLs; HTTP client, streaming bodies, object operations.

- Inbound: `app`, `transport/http`, `usecase/recorder`.
- Outbound internal: `domain/records`.
- Public callable API (17): `Client.AttachmentDownloadURL`, `Client.Bucket`, `Client.Check`, `Client.CheckAttachmentPrivacy`, `Client.CleanAttachmentObjects`, `Client.ListCompletedRecords`, `Client.OpenRecording`, `Client.PresignedGetURL`, `Client.PutAttachment`, `Client.RemovePrefix`, `Client.RemoveRecording`, `Client.SetPublicEndpoint`, `Client.StatAttachment`, `Client.UploadFile`, `NewClient`, `limitedRecording.Close`, `limitedRecording.Read`.
- State/contracts (число полей/методов): structures `{"Client": 7, "CompletedRecord": 9, "UploadedObject": 3, "limitedRecording": 2}`; interfaces `{}`.

### `infrastructure/webrtc`

Legacy recorder Pion session и UDP→FFmpeg; session владеет PC/context/ports.

- Inbound: `app`, `usecase/recorder`.
- Outbound internal: `domain/records`, `infrastructure/ffmpeg`.
- Public callable API (6): `Manager.Active`, `Manager.HandleOffer`, `Manager.Prepare`, `Manager.Shutdown`, `Manager.Stop`, `NewManager`.
- State/contracts (число полей/методов): structures `{"Manager": 12, "Options": 9, "session": 22}`; interfaces `{}`.

### `infrastructure/worker`

HTTP adapter legacy worker; client и bounded request/response.

- Inbound: `app`, `app/records`.
- Outbound internal: `domain/records`, `operations`.
- Public callable API (5): `Client.Offer`, `Client.SetSecret`, `Client.StartRecord`, `Client.StopRecord`, `NewClient`.
- State/contracts (число полей/методов): structures `{"Client": 3}`; interfaces `{}`.

### `operations`

Process metrics/readiness/drain/pprof; владеет collectors, atomic current и HTTP profiling server.

- Inbound: `app`, `app/content`, `infrastructure/composite`, `infrastructure/ffmpeg`, `infrastructure/rabbitmq`, `infrastructure/worker`, `transport/http`, `transport/websocket`, `usecase/analytics`, `usecase/captions`, `usecase/recorder`, `usecase/search`.
- Outbound internal: `buildinfo`, `config`, `domain/jobs`.
- Public callable API (30): `Authorized`, `DiskCheck`, `Event`, `FFmpegActive`, `ID`, `LogWriter.Write`, `New`, `Observe`, `Product`, `ProductQueue`, `ProviderCall`, `Runtime.ConfigureDrain`, `Runtime.DependencyStatuses`, `Runtime.Drain`, `Runtime.DrainStatus`, `Runtime.Live`, `Runtime.Metrics`, `Runtime.Middleware`, `Runtime.Profiling`, `Runtime.Readiness`, `Runtime.Ready`, `Runtime.Register`, `Runtime.RegisterGin`, `Runtime.Run`, `Search`, `State`, `ValidateStorage`, `WSActive`, `WSMessage`, `WithID`.
- State/contracts (число полей/методов): structures `{"LogWriter": 0, "Runtime": 27, "correlationKey": 0}`; interfaces `{}`.

### `transport/http`

Route composition, HTTP server/debug pages; server lifecycle в app.

- Inbound: `app`.
- Outbound internal: `app/auth`, `app/captions`, `app/chat`, `app/conferences`, `app/content`, `app/engagement`, `app/integrations`, `app/notifications`, `app/platform`, `app/recordings`, `app/records`, `app/telemetry`, `domain/conferences`, `infrastructure/storage/s3`, `operations`, `transport/http/middleware`.
- Public callable API (16): `NewRouter`, `RegisterCaptionRoutes`, `RegisterChatRoutes`, `RegisterClientErrorRoutes`, `RegisterConferenceRecordingRoutes`, `RegisterContentRoutes`, `RegisterControlRoutes`, `RegisterDebugRoutes`, `RegisterEngagementRoutes`, `RegisterGuestRoutes`, `RegisterIntegrationRoutes`, `RegisterInvitationRoutes`, `RegisterNotificationRoutes`, `RegisterPlatformRoutes`, `RegisterPlatformStatusRoutes`, `RegisterRecordRoutes`.
- State/contracts (число полей/методов): structures `{}`; interfaces `{"CompletedRecordsLister": 1}`.

### `transport/http/middleware`

Authentication/origin/limits/request guards; request lifecycle.

- Inbound: `app`, `app/auth`, `app/captions`, `app/chat`, `app/conferences`, `app/content`, `app/engagement`, `app/integrations`, `app/notifications`, `app/recordings`, `transport/http`, `transport/websocket`.
- Outbound internal: `app/httpresponse`, `domain/apperrors`, `domain/ratelimit`.
- Public callable API (8): `Authenticate`, `ClientIPKey`, `GuestRequestAllowed`, `JSONFieldKey`, `PathParamKey`, `RateLimit`, `RequireAdmin`, `UserID`.
- State/contracts (число полей/методов): structures `{"RateLimitConfig": 2, "Rule": 6}`; interfaces `{"AdminChecker": 1, "AuthorizationVerifier": 1, "Limiter": 1, "SessionVerifier": 1, "TokenVerifier": 1}`.

### `transport/mediaworker`

Internal media control/egress HTTP server, Engine/Registry/Ticket interfaces; lease/egress lifecycle.

- Inbound: `app`.
- Outbound internal: `config`, `domain/apperrors`, `domain/media`, `domain/realtime`.
- Public callable API (7): `Handler.ActiveRooms`, `Handler.BeginDrain`, `Handler.Ready`, `Handler.Routes`, `Handler.Start`, `Handler.Stop`, `NewHandler`.
- State/contracts (число полей/методов): structures `{"Handler": 18, "offerCache": 4, "roomGate": 2, "roomLease": 2}`; interfaces `{"Engine": 12, "Registry": 5, "Sessions": 1, "Tickets": 1}`.

### `transport/websocket`

Upgrade/ticket/origin, client reader/writer/heartbeat/control queue + mixed signal dispatch (кандидат E).

- Inbound: `app`.
- Outbound internal: `app/httpresponse`, `config`, `domain/apperrors`, `domain/media`, `domain/ratelimit`, `domain/realtime`, `operations`, `transport/http/middleware`, `usecase/realtime`.
- Public callable API (8): `Handler.Connect`, `Handler.ICE`, `Handler.RegisterRoutes`, `Handler.SetMedia`, `Handler.Ticket`, `NewHandler`, `client.Offer`, `client.Stop`.
- State/contracts (число полей/методов): structures `{"Handler": 7, "bucket": 4, "client": 11, "control": 2}`; interfaces `{"Limiter": 1, "MediaController": 1, "PersistentVerifier": 2, "Tickets": 2, "Verifier": 1}`.

### `usecase/analytics`

Агрегация product events; repository/job boundary, operation context.

- Inbound: `app`.
- Outbound internal: `domain/analytics`, `domain/jobs`, `operations`.
- Public callable API (2): `Aggregate`, `Service.Handle`.
- State/contracts (число полей/методов): structures `{"Service": 2}`; interfaces `{"Repository": 4}`.

### `usecase/auth`

Login/guest/persistent session/logout; repo/password/token interfaces, без HTTP cookies.

- Inbound: `app`.
- Outbound internal: `domain/apperrors`, `domain/users`.
- Public callable API (14): `NewService`, `Service.AuthorizeSession`, `Service.Bootstrap`, `Service.Login`, `Service.Logout`, `Service.Me`, `Service.Refresh`, `Service.Register`, `Service.UpdateProfile`, `Service.Verify`, `Service.VerifyAuthorization`, `Service.VerifySession`, `Service.VerifyWithExpiry`, `Service.WithSessions`.
- State/contracts (число полей/методов): structures `{"Service": 5}`; interfaces `{"SessionRepository": 8, "passwordHasher": 2, "sessionTokens": 2, "tokenIssuer": 1, "userRepository": 4}`.

### `usecase/captions`

Сессии live captions, track workers/STT/audio windows; Worker владеет cancellation и inflight.

- Inbound: `app`, `app/captions`, `infrastructure/postgres`.
- Outbound internal: `config`, `domain/captions`, `domain/media`, `domain/realtime`, `infrastructure/ffmpeg`, `operations`.
- Public callable API (5): `Newer`, `ValidEvent`, `Worker.Active`, `Worker.BeginDrain`, `Worker.Run`.
- State/contracts (число полей/методов): structures `{"Lease": 4, "Worker": 13, "activityMeter": 2, "activitySlot": 3, "activityWindow": 2, "statusStamp": 2}`; interfaces `{"AudioTap": 1, "Decoder": 1, "Events": 1, "Repository": 10}`.

### `usecase/chat`

Chat/attachments операции, auth, image processing; repo/store/event boundaries.

- Inbound: `app`, `app/chat`.
- Outbound internal: `domain/apperrors`, `domain/chat`, `domain/realtime`.
- Public callable API (14): `NewService`, `Service.Cleanup`, `Service.Delete`, `Service.Download`, `Service.Edit`, `Service.FinalizeAttachment`, `Service.InitAttachment`, `Service.List`, `Service.MarkRead`, `Service.ReadState`, `Service.Run`, `Service.Send`, `Service.Upload`, `ValidateContent`.
- State/contracts (число полей/методов): structures `{"Service": 4}`; interfaces `{"Events": 2, "Repository": 15, "Storage": 5}`.

### `usecase/conferences`

Conference CRUD/join/moderation/invitations; DB operation → enforcement → events, без Pion.

- Inbound: `app`, `app/conferences`.
- Outbound internal: `domain/apperrors`, `domain/conferences`, `domain/media`, `domain/realtime`, `domain/users`.
- Public callable API (24): `ControlService.Disconnected`, `ControlService.Moderate`, `ControlService.Run`, `ControlService.UpdateMediaState`, `GuestService.Join`, `InvitationService.Invite`, `InvitationService.Search`, `NewControlService`, `NewService`, `Service.Admission`, `Service.Create`, `Service.History`, `Service.Join`, `Service.JoinInvite`, `Service.Leave`, `Service.List`, `Service.LookupInvite`, `Service.Participants`, `Service.Read`, `Service.Schedule`, `Service.Self`, `Service.SetObserver`, `Service.Timeline`, `Service.Transition`.
- State/contracts (число полей/методов): structures `{"ControlService": 3, "GuestService": 3, "GuestSession": 2, "InvitationService": 1, "Service": 4}`; interfaces `{"ControlEvents": 2, "ControlRepository": 3, "GuestRepository": 1, "GuestTokens": 1, "InvitationRepository": 2, "PolicyController": 1, "productRepository": 4, "repository": 9, "userRepository": 1}`.

### `usecase/content`

Content jobs, summaries/Q&A; provider interfaces и leases, job-scoped state.

- Inbound: `app`.
- Outbound internal: `domain/apperrors`, `domain/content`, `domain/jobs`.
- Public callable API (14): `ChunkSegments`, `DefaultConfig`, `NewService`, `Service.FailJob`, `Service.Handle`, `Service.RegenerateSummary`, `Service.RetryTranscript`, `Service.Search`, `Service.Segments`, `Service.SetSearch`, `Service.Summary`, `Service.Transcript`, `ValidateSummary`, `ValidateTranscript`.
- State/contracts (число полей/методов): structures `{"Config": 13, "Service": 6}`; interfaces `{"AudioSource": 1, "Repository": 11}`.

### `usecase/integrations`

Invitations/calendar/email/push jobs/preferences; providers и fanout orchestration.

- Inbound: `app`, `app/integrations`, `infrastructure/postgres`.
- Outbound internal: `domain/apperrors`, `domain/conferences`, `domain/integrations`, `domain/jobs`, `domain/notifications`.
- Public callable API (18): `NewService`, `RenderInvitationEmail`, `Service.CalendarMappings`, `Service.Capabilities`, `Service.Connections`, `Service.Devices`, `Service.Disconnect`, `Service.FailJob`, `Service.Handle`, `Service.MockConnect`, `Service.OAuthCallback`, `Service.OAuthStart`, `Service.Preferences`, `Service.RegisterDevice`, `Service.RenderEmail`, `Service.RevokeDevice`, `Service.SavePreferences`, `Service.Tick`.
- State/contracts (число полей/методов): structures `{"ConferenceSnapshot": 8, "Delivery": 4, "InvitationDelivery": 4, "Options": 4, "Service": 6, "eventPayload": 3, "templateData": 4}`; interfaces `{"Repository": 22, "TokenCipher": 2, "invitationDeliveryRepository": 2}`.

### `usecase/jobs`

Leased job execution/backoff; worker context и retry ownership.

- Inbound: `app`.
- Outbound internal: `domain/jobs`.
- Public callable API (5): `Backoff`, `New`, `Runner.Active`, `Runner.BeginDrain`, `Runner.Run`.
- State/contracts (число полей/методов): structures `{"Handler": 2, "Pool": 5, "Runner": 7}`; interfaces `{}`.

### `usecase/media`

Signaling controller/route ownership; Transport boundary; HTTPClient здесь же — residual adapter placement.

- Inbound: `app`.
- Outbound internal: `config`, `domain/media`, `domain/realtime`.
- Public callable API (9): `Controller.CloseConference`, `Controller.Disconnected`, `Controller.Handle`, `Controller.SetParticipantPolicy`, `Controller.SetPolicyProvider`, `HTTPClient.Call`, `HTTPClient.Close`, `NewController`, `NewHTTPClient`.
- State/contracts (число полей/методов): structures `{"Controller": 10, "HTTPClient": 2, "endpoint": 3}`; interfaces `{"PolicyProvider": 1, "Publisher": 1, "Registry": 3, "Sessions": 1, "Tickets": 1, "Transport": 1}`.

### `usecase/notifications`

List/read/read-all и event publication; repository/event boundaries.

- Inbound: `app`, `app/notifications`.
- Outbound internal: `domain/notifications`, `domain/realtime`.
- Public callable API (5): `NewService`, `Service.List`, `Service.Read`, `Service.Run`, `Service.Tick`.
- State/contracts (число полей/методов): structures `{"Service": 2}`; interfaces `{"Publisher": 1, "Repository": 5}`.

### `usecase/platform`

Capabilities from config/build/providers; без соединений.

- Inbound: `app`.
- Outbound internal: `buildinfo`, `config`, `domain/platform`, `domain/records`.
- Public callable API (3): `BuildVersion`, `Service.Capabilities`, `Service.Summary`.
- State/contracts (число полей/методов): structures `{"Service": 7}`; interfaces `{"ReadyProbe": 1, "Repository": 1, "VectorAvailability": 1}`.

### `usecase/realtime`

Hub owns connection map, room subscription/reconcile/prune loops; membership читает через Repository.

- Inbound: `app`, `app/engagement`, `transport/websocket`.
- Outbound internal: `domain/apperrors`, `domain/conferences`, `domain/realtime`.
- Public callable API (22): `DisconnectObservers.Disconnected`, `Engagement.Authorize`, `Engagement.Reaction`, `Hub.Abort`, `Hub.Authorize`, `Hub.Broadcast`, `Hub.Check`, `Hub.ConferenceChanged`, `Hub.GetActiveSessions`, `Hub.LocalCount`, `Hub.Prepare`, `Hub.Register`, `Hub.SendToConnection`, `Hub.SendToParticipant`, `Hub.SetDisconnectObserver`, `Hub.Shutdown`, `Hub.ShutdownContext`, `Hub.Touch`, `Hub.Unregister`, `Hub.ValidateSession`, `NewEngagement`, `NewHub`.
- State/contracts (число полей/методов): structures `{"Engagement": 2, "Hub": 18, "localSocket": 3}`; interfaces `{"DisconnectObserver": 1, "Repository": 5, "Socket": 2, "Store": 9}`.

### `usecase/recorder`

API record read, worker strategies/composite capture/recovery/finalization; несколько типов, конкретные adapters.

- Inbound: `app`.
- Outbound internal: `config`, `domain/apperrors`, `domain/media`, `domain/records`, `infrastructure/composite`, `infrastructure/ffmpeg`, `infrastructure/storage/local`, `infrastructure/storage/s3`, `infrastructure/webrtc`, `operations`.
- Public callable API (25): `CompositeService.Active`, `CompositeService.BeginDrain`, `CompositeService.HandleCommand`, `CompositeService.Start`, `CompositeService.Wait`, `CompositeService.WaitContext`, `FailIngest`, `NewCompositeService`, `NewService`, `NewWorkerService`, `RunRetention`, `Service.CountByConference`, `Service.List`, `Service.Read`, `Service.ReadComposite`, `Service.ReadComposites`, `Service.Start`, `Service.Stop`, `SweepRetention`, `WorkerService.HandleCommand`, `WorkerService.HandleOffer`, `WorkerService.SetComposite`, `WorkerService.ValidateLegacyRecord`, `activityReader.Close`, `activityReader.Read`.
- State/contracts (число полей/методов): structures `{"CompositeOptions": 11, "CompositeService": 10, "Service": 4, "WorkerService": 10, "activityReader": 2, "recordCommandLock": 2}`; interfaces `{"CompositeRegistry": 1, "CompositeRepository": 9, "RecordingRemover": 1, "RetentionRepository": 1, "apiRepository": 7, "conferenceLocker": 2, "conferenceReleaser": 1, "ingestFailureRepository": 3, "mediaIngest": 3, "workerCommander": 2, "workerRepository": 4}`.

### `usecase/recordings`

Conference recording API + durable command dispatcher в одном Service (кандидат C).

- Inbound: `app`.
- Outbound internal: `domain/apperrors`, `domain/realtime`, `domain/records`.
- Public callable API (6): `NewConferenceService`, `Service.List`, `Service.Read`, `Service.Run`, `Service.Start`, `Service.Stop`.
- State/contracts (число полей/методов): structures `{"Service": 5}`; interfaces `{"Commander": 2, "Events": 1, "Locker": 2, "Reader": 1, "Repository": 7}`.

### `usecase/search`

Search/index/embedding jobs; provider и repo boundary.

- Inbound: `app`, `infrastructure/postgres`.
- Outbound internal: `config`, `domain/content`, `domain/jobs`, `domain/search`, `operations`.
- Public callable API (8): `Chunks`, `Hash`, `ModelKey`, `New`, `Service.FailJob`, `Service.Handle`, `Service.Search`, `ValidVectors`.
- State/contracts (число полей/методов): structures `{"Service": 4}`; interfaces `{"Repository": 6}`.

## 8. Итоги выполнения (добавлены после refactor, не часть исходного snapshot)

A–F реализованы и проверены; итоговые метрики, scope тестов, совместимость и остающийся P2/P3 debt: [P2_BEFORE_AFTER.md](P2_BEFORE_AFTER.md). Baseline audit сохранён до правок отдельно с SHA256.

При заключительном error-path review дополнительно обнаружено раскрытие diagnostic error text в legacy recording 5xx и local debug storage response. A2: сначала два failing reproducer tests, затем sanitizer с сохранением status/schema; современный mapper предоставляет единый safe internal message. Raw text errors trusted legacy worker и persisted legacy diagnostics остаются отдельной областью для будущего compatibility review.

Profiling bind-failure leak подтверждён тестом (+20 waiters до, 0 после); normal profiling shutdown проходит. Нет изменения Pion/RTP/compositor hot path. Для captions PCM policy перенесена без изменения алгоритма, а не объявлена obsolete. Старые endpoints не удалены без доказательства unreachable.
