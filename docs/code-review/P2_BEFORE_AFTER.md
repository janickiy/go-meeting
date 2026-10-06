# P2 — результаты архитектурного аудита и рефакторинга

Дата: 2026-10-06. Исходный commit: `27594d8`. Архитектурная карта создана до production edits: [P2_ARCHITECTURE_AUDIT.md](P2_ARCHITECTURE_AUDIT.md). Набор доказательств: `tmp/p2-architecture-20261006/evidence/` (ignored, не содержит DSN в отчётах). Данные измерений ниже относятся к этой локальной машине и тестовым нагрузкам, не к production SLO.

## 1. Executive summary

Выполнены A–F и небольшой дополнительный A2 после заключительного error-path review. Упрощены реальные границы ownership: HTTP-запросы записей отделены от durable command delivery; WS lifecycle — от protocol dispatch; membership/media policy централизованы; captions больше не импортирует FFmpeg для чистой PCM-проверки. Воспроизведены и исправлены два дефекта: goroutine leak при неудачном запуске pprof и раскрытие внутренних ошибок legacy/debug HTTP handlers. Шесть основных batches и A2 проверены тестами, race и относящимися к ним integration checks.

UI, migrations, queue/storage technology и Pion packet path не менялись. Выкладка не выполнялась. P3 не начат. Изменения не закоммичены агентом: часть файлов была staged пользователем во время работы, состояние index сохранено.

## 2. Architecture inventory

Полная карта **67** packages с responsibility/state/inbound/outbound/public callable API/struct/interface metrics находится в audit §7. Snapshot до изменений сохранён отдельно как `architecture-audit-before.md` + SHA256; JSON AST inventory — `inventory-before.json` и `inventory-after.json`. Область: production `internal/**/*.go`, без `_test.go`; внешние cmd/test consumers не включены в inbound.

Четыре сущности разделены: **Participant** — persisted membership; **Session** — одна физическая WS-сессия; **MediaPeer** — один Pion peer; **Recording** — durable state/outbox и отдельный capture lifecycle. Разрыв одной сессии не равен удалению membership и других peers.

## 3. Coupling findings

- Удалён конкретный import `usecase/captions → infrastructure/ffmpeg`; activity policy принадлежит `domain/captions`. Decoder provider остаётся заменяемым.
- `recordings.Service` больше не знает Commander/Locker и timer/retry/logging. App связывает один repository с API service и отдельным dispatcher.
- Media policy projection теперь в `domain/media`; Postgres и moderation usecase не содержат две копии правила. Направление media → conferences допустимо: обратного import нет (media уже зависел от conferences через realtime).
- Сохранены осмысленные interfaces Email/Calendar/STT/AI/Embedding, MediaController.Transport, Engine и repositories. Нет новых универсальных providers/utils/DI frameworks.
- Оставлены явно отмеченные зависимости `usecase/recorder → concrete strategies/storage`, `domain/records → gorm datatypes`, совместное размещение Controller и HTTPClient. Причины и дальнейшие шаги — §17.

## 4. Large service/function findings

`recordings.Service` имел две независимые причины изменения: HTTP reads/requests и delivery loop с Redis/Rabbit/outbox. Теперь Service содержит 3 зависимости и 4 public operations; CommandDispatcher — 4 зависимости и единственный public Run. Это один coherent split, а не набор микросервисов.

`Service.Run` (60 LOC, 17 branch nodes) заменён `CommandDispatcher.Run` (16/5), `dispatchOne` (22/5), `deliver` (25/7). Политика delivery сохранена: tick 250 ms, до 16 commands, timeout 5 s; Claim → lock → command → best-effort WS event → Complete; при delivery/Complete failure — Retry; terminal record только Complete.

`client.read` сокращён с 155 LOC / 28 branch nodes до 88 / 15. Reader владеет heartbeat/deadline/rate/controls/close; messages.go — decode envelope → authorization → media controller или validated legacy forwarding → reply. SDP/ICE validation не требует запуска сокета в unit tests.

Крупнейшая функция всего проекта `Composer.Capture` (326 LOC / 72 branch nodes) **осталась прежней**. Остались также mediaworker.command и IntegrationRepository.Fanout. Их размер зафиксирован, но полный rewrite существенно повышал бы риск для recovery/lease/egress; минимальный LOC не был целью.

## 5. Duplication findings

Две одинаковые projection функции удалены, заменены одной `media.PolicyForParticipant`; три inline проверки admission/status в Hub используют domain membership methods. ParticipantView делегирует Participant, а не задаёт вторую policy.

Media errors кодируются/декодируются одним typed catalogue; обратное сравнение с human `.Error()` удалено. Legacy UDP retry определяет `ECONNREFUSED` через `errors.Is`, shutdown errors — через typed cancellation/closed/EOF. Pagination, Redis key construction, event constructor, signed URLs и FFmpeg setup просмотрены; различающиеся transport scopes/strategies не сведены в случайный shared helper.

## 6. Error-handling changes

| Category | HTTP |
|---|---:|
| Validation | 422 |
| Unauthenticated | 401 |
| Forbidden | 403 |
| NotFound | 404 |
| Conflict | 409 |
| RateLimited | 429 |
| Unavailable | 503 |
| Timeout (explicit) | 504 |
| Internal / unclassified | 500 |

`apperrors.Wrap` сохраняет category и диагностический root cause (`errors.Is/As`), Error() возвращает только safe message. Явная classification имеет приоритет над cause. Raw context deadline остаётся 500, пока владелец операции не классифицировал его явно: скрытого API change нет. Rate-limit body и headers прежние.

Media wire codes сохранены для всех 9 ошибок, unknown/unsafe strings превращаются в `media_unavailable`. Legacy field `error` принимается только как ранее существовавший machine code. HTTP/media-worker/WS используют соответствующие contract adapters. WS error envelope сохраняет `code` и `replyTo`; новое поле `message` намеренно не добавлено ради совместимости.

**A2 bug fix:** воспроизводящие tests сначала показали SQL/storage details в legacy 500/502 и debug 500. Теперь 5xx bodies содержат `internal server error`; статусы, JSON shape и validation 4xx сохраняются. Это намеренное изменение небезопасного текста ошибки, не новая функция.

Logging ownership: retry logging остаётся у dispatcher, connection failure — у transport/hub, process failure — у worker. Общий mapper не логирует повторно и не сериализует cause. Старый trusted worker HTTP протокол и persisted diagnostics не переводились целиком на новый wire contract (см. §17).

## 7. Cleanup/lifecycle ownership changes

`Profiling` больше не создаёт goroutine, ожидающую process context после failed bind. Cancellation callback регистрируется через `context.AfterFunc`; `defer stop()` снимает её на любом выходе. Reproducer: 20 failed binds оставляли **20** waiters; после fix — **0**. Existing endpoint/profile/shutdown тест подтверждает закрытие listener при cancellation.

В CommandDispatcher один `defer cancel()` на dispatchOne вместо cancellation, разбросанной по веткам. Shared clients закрывает app; освобождение recording lock принадлежит capture worker, а не API/dispatcher. P0 idempotent peer/session/sink cleanup не переписывался.

## 8. State/event changes

Conference/Participant/Recording status transitions, optimistic/version fencing и transactions сохранены. Domain membership остаётся authoritative; гостевой JWT сам по себе не даёт права записи. Start разрешён авторизованному admitted account; остановка — owner/requester, как прежде. Conference availability остаётся отдельным условием при media-policy lookup.

Durable `record.start`/`record.stop` не объединены с realtime `recording.starting`/`recording.stopping`. Порядок, payloads, requestedBy, mode, public status mapping и best-effort WS publication сохранены; outbox completion не предшествует confirmed delivery. Существующий terminal-command recovery semantics и retry cadence проверены отдельно.

## 9. Repository/interface changes

API `Repository`: 7 → 4 methods. Новый `CommandRepository`: Claim/Complete/Retry. Один production Postgres repository реализует оба; DB transaction implementation не перемещалась. `Locker`: 2 → 1 method (Acquire); реальный Release остаётся у владельцев, которым нужен.

`NewConferenceService(repo, reader, events)` теперь не принимает неиспользуемые delivery dependencies; app и все test callers обновлены. `NewCommandDispatcher(repo, commands, locker, events)` создаётся рядом. Это internal Go API change; HTTP routes/schema не зависят от сигнатуры конструктора. Reader batch optimization P1 сохранена, fallback для старых Reader implementations сохранён.

## 10. Config/DI/global-state findings

Config уже разделён по подсистемам и проверяется при запуске; общий loader велик, но не находится в hot path. Новые env/defaults, DI container и mutable global service не добавлены. Constants polling принадлежат dispatcher. `operations.current` остаётся atomic process metrics singleton; его удаление потребовало бы передачи runtime во все hot counters без доказанной пользы. Shared DB/Redis/Rabbit clients создаёт app.

## 11. Dead code removed / retained

Удалены две старые media policy projection declarations, duplicated media error decoding loop, ненужный Locker.Release contract и старое размещение AudioActive в FFmpeg. Заглушка/alias для старого helper не оставлена: все callers обновлены. `AudioActive` сохранён побайтно по алгоритму и порогу в domain/captions, проверены PCM16LE/negative/full-scale/odd-byte/threshold cases.

Legacy P2P/recording/debug/recovery routes и feature flags не удалены: они зарегистрированы и/или имеют integration callers. Staticcheck не нашёл дополнительных unused private symbols. Это review с доказательствами ссылок, а не удаление по возрасту файла.

## 12. Refactor batches

| Batch | Что изменено | Проверка |
|---|---|---|
| Baseline repair | Один устаревший assert 403 для account recording удалён; добавлен guest/account boundary test | Сначала baseline failure, затем корректные tests до production edits |
| A | Category/root cause/HTTP + media protocol codes + typed UDP errors | Unit/race HTTP/media/SFU control + StageOne/recording authorization |
| B | Membership predicates и media policy projection | 42 status/admission combinations, projection, real Redis/PG moderation/rejoin/media state/race |
| C | Service / CommandDispatcher | 12 ordered delivery/failure cases, cancellation, HTTP concurrent starts, DB outbox fencing/race |
| D | Failed profiling bind cleanup | Failing reproducer → fix → repeated bind and existing normal shutdown/race |
| E | WS decode/dispatch | Spoofed sender, nil/self target, SDP/ICE bounds, envelope reply correlation; real WS repeated/reconnect/race |
| F | Helper ownership/dead-code review | PCM tests + captions/FFmpeg/provider/race; real Linux FFmpeg later |
| A2 | Legacy/debug error disclosure | Failing reproducer → safe 5xx → handler/routes/StageOne tests/race |

После каждого — relevant tests, race, integration и просмотр diff. Checkpoints/logs: `batch-*.json`, `batch-*-tracked.patch`; начальная ошибка импортов A была исправлена до green checkpoint. В C diff-check выявил trailing whitespace только в новом audit document; устранено, повторный diff-check green. Отдельные коммиты агентом не создавались.

## 13. Before / After metrics

LOC считает строки внутри тела declaration, включая комментарии; branch nodes = if/for/range/case/comm AST nodes, включая closures, **не формальная cyclomatic complexity**. Снижение LOC service.go частично связано с переносом и обновлением документации contracts; оно не интерпретируется как ускорение.

| Метрика | До | После |
|---|---:|---:|
| recordings.Service: dependencies | 5 | 3 |
| recordings.Repository methods | 7 | 4 |
| CommandRepository methods | в общем Repository | 3 |
| Locker methods | 2 | 1 |
| recordings.Service public methods | 5 | 4 |
| SFU package public callable declarations | 37 | 37 |
| Production internal packages | 67 | 67 |
| Participant → media policy projections | 2 | 1 |
| Hub inline admission checks | 3 | 0 |
| Application error categories | 6 | 9 |
| client.read LOC | 155 | 88 |
| client.read branch nodes | 28 | 15 |
| captions usecase → FFmpeg concrete import | 1 | 0 |
| profiling goroutines retained / 20 bind failures | 20 | 0 |

## 14. Test / race / static results

- `final-key-api`: PASS; 24.97 s.
- `final-test`: PASS; 16.052 s.
- `final-race`: PASS; 21.304 s.
- `final-vet`: PASS; 0.595 s.
- `final-staticcheck`: S1024 (pre-existing), exit 1; 2.206 s.
- `final-vuln`: PASS; 3.66 s.
- `final-frontend-lint`: PASS; 1.596 s.
- `after-ws-smoke`: PASS; 17.008 s.
- `final-linux-audio-modes`: PASS; 1.246 s.

Единственное замечание staticcheck: `persistent_auth_sessions_test.go:103`, S1024 (`time.Until`); перенесено в P3. Govulncheck: 0 vulnerabilities в вызываемом коде, 1 в required module без достижимых вызовов. Scan относится к текущему dependency graph и времени проверки.

Стандартный `go test ./...` на macOS включает Pion smoke 2/3/5 peers, но integration tests с внешними opt-in зависимостями могут skip. Поэтому отдельно запущены real PG/Redis auth/moderation/outbox + API→WS→SFU и два полных Linux recording pipelines. FFmpeg отсутствует на host: F initially skipped real decoder/modes, они отдельно выполнены в cached Linux image с `--network none`. Не выдаём browser/TURN/real STT/long soak за пройденные проверки: они не входили в representative P2 regression.

## 15. P1 performance regression

Замеры до/после последовательные, без параллельного запуска audit tests. Общая машина не изолирована от других пользовательских процессов: проценты latency/CPU описывают наблюдение, а не production guarantee. Контейнеры приложения не перезапускались. RTP benchmark — 6 samples, benchtime 250 ms; media profile — 5 peers, screen + recorder sink + audio tap, 50 packets/s/source, audio 160/video 1200 bytes, 22 s warm-up + 5 s capture. In-process Pion clients входят в measured CPU/allocations, реального FFmpeg/STT в этом профиле нет.

### RTP benchmark

| Payload / sinks | До ns/op (median) | После ns/op (median) | Δ | До → после B/op / allocs |
|---|---:|---:|---:|---|
| payload_160/sinks_0 | 7.70 | 7.60 | -1.3% | 0/0 → 0/0 |
| payload_160/sinks_1 | 138.90 | 141.45 | +1.8% | 176/1 → 176/1 |
| payload_160/sinks_2 | 172.30 | 174.35 | +1.2% | 176/1 → 176/1 |
| payload_1200/sinks_0 | 7.67 | 7.63 | -0.6% | 0/0 → 0/0 |
| payload_1200/sinks_1 | 216.50 | 219.55 | +1.4% | 1280/1 → 1280/1 |
| payload_1200/sinks_2 | 250.20 | 254.85 | +1.9% | 1280/1 → 1280/1 |

### Media profile / lifecycle

| Метрика | До | После |
|---|---:|---:|
| cpu_seconds | 1.851 | 1.784 |
| allocated_bytes_per_second | 7543289.397 | 7542789.865 |
| mallocs_per_second | 52167.659 | 52174.958 |
| forwarded_packets | 11000.000 | 11000.000 |
| dropped_packets | 0.000 | 0.000 |
| recording_drops | 0.000 | 0.000 |
| audio_tap_drops | 0.000 | 0.000 |

baseline cleanup: goroutines=3, FD=6, rooms=0, peers=0, tracks=0, subscriptions=0, recording outputs=0, audio taps=0.

after cleanup: goroutines=3, FD=6, rooms=0, peers=0, tracks=0, subscriptions=0, recording outputs=0, audio taps=0.

WS repeated lifecycle (1020 connections): after cleanup all local sockets, connected sessions and Redis presence keys return to zero. Counts:

| Phase | До goroutines / FD | После goroutines / FD |
|---|---:|---:|
| idle | 14 / 12 | 14 / 11 |
| warm | 14 / 14 | 14 / 15 |
| round-1 | 14 / 14 | 14 / 15 |
| round-2 | 14 / 14 | 14 / 15 |
| round-3 | 14 / 14 | 14 / 15 |
| round-4 | 14 / 14 | 14 / 15 |

### DB and HTTP list reads

PG fixture: 1000 conferences, 20k participants/records, 40k files, 400k segments, 200k record events, 50k messages, 20k notifications/summaries/transcripts, 400k transcript segments. 100 samples/workload. HTTP workload includes Gin+JSON, excludes auth/network/TLS/MinIO signed URLs. Query counts below are per operation.

| Workload | Queries до → после | p95 ms до → после | bytes/op до → после |
|---|---:|---:|---:|
| recordings_http_20 | 6 → 6 | 5.110 → 5.740 | 1967700 → 1898528 |
| recordings_1 | 6 → 6 | 1.595 → 1.476 | 135480 → 135447 |
| recordings_20 | 6 → 6 | 3.904 → 4.058 | 1242276 → 1242205 |
| meetings_20 | 1 → 1 | 0.503 → 0.603 | 52370 → 52309 |
| history | 6 → 6 | 2.317 → 1.968 | 81066 → 82412 |
| participants_20 | 4 → 4 | 1.145 → 2.066 | 64561 → 64519 |
| chat_20 | 6 → 6 | 2.126 → 2.716 | 94254 → 94355 |
| notifications_20 | 2 → 2 | 0.581 → 0.631 | 51405 → 51266 |
| transcript_20 | 5 → 5 | 1.134 → 1.312 | 54412 → 54953 |
| summary | 4 → 4 | 1.866 → 1.485 | 42403 → 40542 |
| analytics | 3 → 3 | 0.819 → 0.753 | 45657 → 47152 |
| admin | 3 → 3 | 4.685 → 4.225 | 25766 → 25034 |

**Проверка вариативности:** в первом after run participants p95 вырос с 1.145 до 2.066 ms при прежних 4 SQL queries. Выполнены ещё две чередующиеся пары baseline/after на тех же размерах данных. Baseline восстановлен через Go overlay из `27594d8` (включая старые constructors/test callers), рабочие файлы не откатывались. Полный manifest: `baseline-overlay/overlay.json`, результаты: `db-repeat-checks.json`.

| Workload | Repeat 1: baseline → after p95 ms | Repeat 2: baseline → after p95 ms |
|---|---:|---:|
| recordings_http_20 | 6.440 → 5.668 | 5.835 → 6.081 |
| recordings_1 | 1.490 → 1.525 | 1.922 → 2.371 |
| recordings_20 | 4.069 → 4.175 | 3.577 → 3.198 |
| meetings_20 | 0.526 → 0.475 | 0.707 → 0.470 |
| history | 2.873 → 1.870 | 2.941 → 2.681 |
| participants_20 | 0.990 → 0.938 | 1.917 → 2.397 |
| chat_20 | 2.182 → 2.439 | 1.817 → 2.466 |
| notifications_20 | 1.312 → 1.292 | 0.604 → 0.563 |
| transcript_20 | 1.738 → 1.885 | 1.881 → 1.351 |
| summary | 0.998 → 0.833 | 0.934 → 0.886 |
| analytics | 0.735 → 0.742 | 0.705 → 0.772 |
| admin | 4.374 → 8.767 | 4.742 → 5.465 |

### Recording / FFmpeg

Два независимых full-stack pipelines одновременно: actual Postgres + Redis + RabbitMQ + MinIO + Pion + FFmpeg, H264/AAC MP4. Проверяются decoded video/audio, screen content, recovery из закрытого сегмента и auto-stop при завершении conference. Harness P1 запускается с `--phase after` **и до, и после P2**: `before` наложил бы старый P0 compositor и испортил baseline.

- baseline: `{"decoded_content_rooms": 2, "recovered_closed_segment_rooms": 2, "conference_auto_stop_rooms": 2}`.
- after: `{"decoded_content_rooms": 2, "recovered_closed_segment_rooms": 2, "conference_auto_stop_rooms": 2}`.

| Process sample metric | До | После |
|---|---:|---:|
| peak_go_rss_kb | 367632 | 372364 |
| peak_go_fds | 151 | 155 |
| peak_ffmpeg_count | 2 | 2 |
| peak_ffmpeg_rss_kb_sum | 160920 | 155784 |
| peak_temp_bytes | 7788775 | 8029601 |
| peak_temp_files | 135 | 134 |
| last_temp_bytes | 0 | 0 |
| last_temp_files | 0 | 0 |
| go_ticks_final | 406 | 420 |

Process CPU/RSS/FD/temp-file samples и pprof сохранены в `baseline-recording/recording/` и `after-recording/recording/`; wall time тестового harness включает setup/compile/cleanup и не трактуется как latency одного кадра. RTP allocation counters не выросли; SFU/compositor production код P2 не менялся. RTT этим harness не измеряется. Процессные peaks снимались примерно раз в 300 ms, поэтому не являются точными максимумами; short-lived FFmpeg CPU не оценивался по этим редким samples.

**Итог сравнения и пределы:** material regression медиа/WS/recording не обнаружена: RTP median shift от −1.3% до +1.9%, B/op и allocs/op без изменений; media throughput 11000 packets / 5 s и drops=0; steady cleanup counts совпадают; recording Go peak RSS +1.3%, sampled CPU ticks +3.4%, временные файлы возвращаются к нулю. DB query counts сохранены во всех повторах; p95 варьируется в обе стороны, включая unchanged paths. Для этих DB tail latencies строгая эквивалентность не доказана: нужны изолированный host и более длительная серия, если требуется performance SLO. Рост participants на 80% не подтвердился в том же масштабе: в повторах −5% и +25%; у unchanged chat/admin tail latency также колеблется. Эти результаты не позволяют приписать задержку рефакторингу или гарантировать DB p95 SLO. Production soak в P2 не запускался.

Временная инфраструктура очищена: собственный Redis удалён с volume, DB size перед удалением 0; созданных audit containers/networks/volumes не осталось. Проверены ID/start times всех **11** исходных контейнеров: без изменений. Подтверждение: `final-cleanup.json` и cleanup.json обоих recording pods.

## 16. Public API compatibility

Preserved: routes, JSON DTO fields, membership/record permission, public recording statuses, HTTP 401/403/404/409/422/429/503, rate-limit headers, internal media codes, WS replyTo/ack/event payloads, ticket/heartbeat/session limits, outbox fencing/retries и retention. New typed Timeout не переклассифицирует raw context errors автоматически.

Два explicit fixes: callback leak при failed pprof bind (внешний контракт тот же); safe legacy/debug 5xx text вместо diagnostics (HTTP status/schema прежние). Internal constructors/helper package изменены, все callers найдены и обновлены. Старые endpoints/recovery paths сохранены. Данных и DB migrations нет.

## 17. Remaining P2 debt

1. `recorder` всё ещё связывает concrete capture/storage strategies; следующий bounded шаг — отдельный finalization/storage adapter с tests на partial upload/recovery, без изменения RTP path.
2. `integrations.Repository` имеет 22 methods; отдельно рассмотреть calendar link/preferences vs delivery jobs. Сначала полный calendar/email/push retry regression.
3. `mediaworker.Handler.command` (276 LOC / 58 branch nodes), `Composer.Capture` (326/72) и Fanout остаются крупными; нужен operation-specific extraction, а не механическое разбиение по 20 строк.
4. `domain/records` использует GORM datatypes; `usecase/media` содержит concrete HTTP adapter рядом с Controller. Никаких показанных текущими тестами lifecycle bugs от этого не найдено; перенос требует цели и compatibility review.
5. Old trusted recording-worker HTTP text errors и persisted legacy diagnostic events остаются отдельным контрактом; перед его полной унификацией нужны end-to-end tests старого ingest/record-event API. Современный media-worker уже использует structured codes; публичные legacy 5xx теперь sanitized.
6. Metrics singleton и SSE per-user state остаются; будущий split оправдан только доказанной lifecycle/test isolation проблемой. Новые абстракции ради названий не введены.

## 18. P3 follow-up

Только отдельным следующим заданием: S1024 (`time.Until`); устаревшие автоматически сгенерированные комментарии (например, описания Participant как bytes/io.Writer); consistency импортов/naming и документации contracts. Массовое форматирование, переименование и косметический rewrite в P2 не выполнялись.

## 19. Files changed

- `docs/code-review/P2_ARCHITECTURE_AUDIT.md`
- `docs/code-review/P2_BEFORE_AFTER.md`
- `internal/app/api.go`
- `internal/app/httpresponse/response.go`
- `internal/app/httpresponse/response_test.go`
- `internal/app/records/response.go`
- `internal/app/records/response_test.go`
- `internal/domain/apperrors/errors.go`
- `internal/domain/apperrors/errors_test.go`
- `internal/domain/captions/audio.go`
- `internal/domain/captions/audio_test.go`
- `internal/domain/conferences/membership_policy_test.go`
- `internal/domain/conferences/product.go`
- `internal/domain/media/errors.go`
- `internal/domain/media/errors_test.go`
- `internal/domain/media/media.go`
- `internal/domain/media/membership_policy_test.go`
- `internal/infrastructure/ffmpeg/live_audio.go`
- `internal/infrastructure/postgres/moderation_repository.go`
- `internal/infrastructure/webrtc/manager.go`
- `internal/operations/profiling_lifecycle_test.go`
- `internal/operations/runtime.go`
- `internal/transport/http/debug.go`
- `internal/transport/http/debug_test.go`
- `internal/transport/http/middleware/rate_limit.go`
- `internal/transport/websocket/handler.go`
- `internal/transport/websocket/messages.go`
- `internal/transport/websocket/messages_test.go`
- `internal/usecase/captions/worker.go`
- `internal/usecase/conferences/control.go`
- `internal/usecase/media/client.go`
- `internal/usecase/media/client_test.go`
- `internal/usecase/realtime/hub.go`
- `internal/usecase/recordings/dispatcher.go`
- `internal/usecase/recordings/dispatcher_test.go`
- `internal/usecase/recordings/service.go`
- `internal/usecase/recordings/start_test.go`
- `tests/integration/composite_recording_test.go`
- `tests/integration/conference_control_test.go`
- `tests/integration/live_audio_recording_modes_test.go`
- `tests/integration/p1_db_performance_test.go`
- `tests/integration/p1_recordings_batch_test.go`
