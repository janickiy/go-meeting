# P2 — результаты архитектурного аудита и рефакторинга

## Текущий проход — 2026-10-10

Статус: **A–F завершены; correctness, race, static и representative performance gates прошли.** Все четыре режима записи проверены с реальными FFmpeg/MinIO; существенной измеренной регрессии нет. Исторические результаты 6 октября сохранены отдельно. Baseline: HEAD `a0781257debba3f911ee180067cc0fad5bb06204` **плюс существовавшие P0/P1/UI изменения**, не чистый checkout. Новые правки не закоммичены и не выложены на сервер. P3 не начат.

### 1. Executive summary

Выбраны ограниченные изменения A–F: укрепление контрактов ошибок; один общий predicate отозванного membership; отдельный владелец публикации файлов записи и её cleanup; перенос peer-presence operation из HTTP; удаление неиспользуемой зависимости конструктора. Уже выполненные ранее разделения Service/CommandDispatcher, WS reader/dispatch и media policy не переделываются. Нет UI/migration/queue/schema/Pion rewrite, универсального framework или новых retries.

### 2. Architecture inventory

Аудит всех **73** production `internal` packages сделан до production edits: [P2_ARCHITECTURE_AUDIT.md](P2_ARCHITECTURE_AUDIT.md). В нём responsibility/API/state карта, границы и намеренно сохранённые связи. Воспроизводимая утилита `go run ./tools/architecture-audit` выдаёт точные imports/inbound, экспортируемые функции/методы/типы/constants/vars, поля структур, repository/provider interfaces и метрики функций. Тесты утилиты проверяют детерминизм, направления связей, исключение tests/cmd и метрики.

Зафиксированные до изменений evidence: `tmp/p2-audit-20261010/architecture-audit-before.md` SHA256 `d15006c7a188945e6f5518e5651eeb481a4e23563772fe45074c205ba0fb4d20`; `inventory-before.json` SHA256 `acb71fbdbc775dbad0ddd4d6989c1c83fd4ae0d7d26fac83044a2403ce76ee0f`. Полные runtime manifests/команды/логи находятся в том же ignored evidence-каталоге; секреты/DSN в отчёт не включены.

### 3. Coupling findings

Нужные границы — durable membership, physical realtime session, Pion MediaPeer и recording lifecycle — сохранены. Выделение publication уменьшает ответственность orchestration, но **не удаляет все infrastructure imports** из recorder: реальные composite/FFmpeg/S3 adapters остаются. Новый владелец имеет два узких контракта: upload/delete и fenced artifact commit. HTTP presence не должен владеть TTL/ACL lookup orchestration. `domain/records→gorm/datatypes`, media HTTPClient рядом с Controller и большой integrations service остаются явно обозначенным долгом, а не скрываются переименованием каталогов.

### 4. Large service/function findings

`CompositeService.run` до P2: 115 LOC / 26 AST branch nodes — capture, state validation, FFmpeg, upload, metadata, fenced save, rollback, events, local retention. Из него выделена coherent artifact operation; orchestration сохраняет transitions/capture/lease/ready/events/local-directory lifetime. Publisher owns attempt token prefix/ZIP/commit state. Это не новый сервис deployment и не per-packet interface.

Крупнейший `Composer.Capture` (326 LOC / 72 nodes) и `mediaworker.Handler.command` (276 / 58) остаются прежними. `conferences.Service` с 14 business methods + observer — coherent facade, не разделяется по одному LOC. `personal.Handler` с 18 handlers сохраняет основной фасад, выделяется лишь самостоятельная presence operation. Итоговые AST числа приведены в §13 после freeze.

### 5. Duplication findings

Два точных kicked/rejected выражения (rejoin и invitation repository) заменены одним `Participant.IsRejectedOrKicked`. 42 комбинации status/admission включают waiting/empty/unknown и противоречивые состояния. `IsAdmitted`/`CanReadHistory` не переписаны: отказ в admission шире revocation. SQL ACL guards внутри операций сохранены, поскольку предотвращают race после предварительной проверки. Legacy/composite upload keys, durable/WS events, stage-specific validation и retries не унифицируются только из-за похожего кода.

### 6. Error-handling changes

Production taxonomy/mappers уже централизованы. A добавляет contract tests, не меняет wire responses. Проверены nine typed media errors, plain/wrapped/root cause/unknown/string-not-code; category-over-cause при HTTP mapping; malformed JSON; WS envelope и replyTo. Внутренние строки не выходят клиенту; новый publication layer сохраняет точные `%w` и raw-error ветки, cleanup error не подменяет исходную ошибку.

### 7. Cleanup/lifecycle ownership

Private `compositeArtifactPublication` — один single-goroutine attempt: Publish один раз, idempotent Close. `CompositeService.run` defer Close сохраняет прежний внешний scope: post-commit ready/events/local cleanup выполняются **до** освобождения ZIP. Close удаляет ZIP, затем делает best-effort rollback собственного token prefix только до commit; timeout 5s независим от отменённого рабочего context. Nil S3 не превращается в typed-nil interface; rollback не вооружается до storage guard. Источники ошибочной записи сохраняются для recovery; чужой token и committed artifacts не удаляются. Shared storage client не закрывается publisher-ом. P0 process/peer/session cleanup не переписан.

### 8. State/event changes

Новых состояний, event names или payloads нет. `SaveCompositeArtifacts` остаётся lease-fenced atomic commit; до него нельзя объявить запись готовой, после него нельзя откатить опубликованные объекты из-за failure чтения/уведомления. Durable recording events и realtime notifications — разные contracts. Recording permission/admission/screen ownership и queue retry ordering не меняются.

### 9. Repository/interface changes

Publication использует только `UploadFile/RemovePrefix` и `SaveCompositeArtifacts`; public CompositeOptions/constructor signature сохраняется. Presence operation использует Get + Online, без generic repo/нового SQL. `AssetService` избавляется от reader, который лишь проверялся на nil, но не сохранялся/не вызывался; committed snapshot возвращается без повторной authorization. Email/Calendar/STT/AI/Embedding adapters и useful repository testing seams сохранены.

### 10. Config/DI/global-state findings

Subsystem configs и explicit app constructors сохранены. Нет DI framework, per-request env parsing, новых flags/таймеров/горутин/retries. `operations.current` — atomic process collector, массовый перенос через параметры не даёт доказанного lifecycle выигрыша. Segment defaults на разных trust boundaries пока не объединяются. Retry owners остаются SDK request, durable outbox delivery, provider job и lease recovery — не дополнительный универсальный retry layer.

### 11. Dead code removed / documented

F удаляет только доказанно неиспользуемый constructor parameter; интерфейс Get становится нужен presence operation. Крупное legacy удаление **отложено**: `app/records` и `infrastructure/worker` отсутствуют в production command graph, но публичные 410 tombstones и живой legacy worker ingest/recovery должны остаться. Старый debug HTML и retired rate flags — кандидаты отдельного compatibility review, не удалены по одному совпадению поиска.

### 12. Refactor batches

| Batch | Действие | Подтверждение |
|---|---|---|
| A | Error HTTP/internal-media/WS contract tests, production mapping unchanged | Focused tests + race PASS; root diff review |
| B | Domain revocation predicate + два call sites | Unit/race PASS; 13 real PG/Redis integration tests under race PASS, без skips; diff review |
| C | Private artifact publication owner | Focused tests/race/vet PASS, 6 real PG/Redis fence/outbox/recording integration tests under race PASS; full recording rerun ниже |
| D | Idempotent attempt cleanup в C seam | All-mode/fault/cancellation/stale lease/ZIP/foreign-prefix/cleanup-failure tests + race PASS; root review усилил failed-delete fake |
| E | Presence application operation | Unit/race PASS; 9 real PG/Redis tests under race PASS без skips |
| F | Unused reader constructor dependency | Unit/race, integration compile и 6 real PG/Redis/MinIO asset tests under race PASS; старый SQL marker fixture исправлен после before-F reproducer |

C/D — один coherent production split с отдельными cleanup инвариантами и проверками. A/B и E/F имеют собственные checkpoints. Старые dirty файлы, включая P1 FFmpeg/SQL оптимизации и staged user file, не сбрасываются. Проверка baseline source manifest показывает отличия только в 12 ожидаемых прежних P2-target Go files и добавленной audit utility; остальные baseline Go sources, включая P0/P1, сохранили hashes (`baseline-source-comparison.log`). Staged список остался прежним. Подробности: `tmp/p2-audit-20261010/batches-errors.md`, `batches-recording.md`, `regression-results.md`.

### 13. Before/After architecture metrics

Методика — AST declaration span и branch node count, не cyclomatic complexity. Сравнение `inventory-before.json` / `inventory-after.json`. Минимизация суммарных LOC не цель: небольшое увеличение production кода и focused tests допустимо ради явного resource owner.

| Area | Before | After |
|---|---|---|
| Production internal packages | 73 | 73 |
| Composite run | 115 LOC / 26 branch nodes | 70 / 16; publication 47 / 10, cleanup 14 / 3 |
| PeerPresence HTTP handler | 50 LOC / 9 branch nodes | 18 / 3; transport-free operation 30 / 7 |
| Largest function | Composer.Capture 326 / 72 | unchanged |
| Revoked membership predicate | 2 inline copies | 1 domain owner, 2 callers |
| Error mapping implementations | Centralized existing mappers | unchanged; expanded contracts |
| Public CompositeService methods | 6 | 6 (плюс прежний constructor) |
| Asset constructor dependencies | ctx/repo/storage/unused reader/events | ctx/repo/storage/events |
| Recorder infrastructure imports | concrete strategies/adapters | intentionally retained; publication independently fakeable |
| Testability | Upload failures reached through concrete storage | isolated mode/fault/resource matrix + real integrations |

Риск C/D — изменение порядка commit/rollback и времени жизни ZIP; его ограничивают lease/fault/lifetime tests и реальные четыре режима записи. Риск E — изменение ACL, deadline или трактовки неизвестного присутствия; исходные HTTP contract tests оставлены без изменений и дополнены operation/integration tests. B/F имеют небольшой scope, но также проверены против admission/authorization races. Производительность не является целью этих извлечений; итог измерений — §15.

### 14. Tests/race results

Fresh baseline full tests/race/vet/staticcheck v0.7.0/govulncheck v1.8.0/frontend lint PASS. Govulncheck: 0 reachable vulnerabilities, 0 in imported packages; 1 advisory in a required but unimported module — не заявляется «во всех зависимостях ноль». Baseline критические сценарии: 1,020 account + 1,020 conference WS lifecycle, reconnect100/race, 23 key API tests/race, SFU25 lifecycle +25 source churn +2/3/5 peers/audio isolation/race, четыре performance сценария, две реальные recording pipelines. Все workload commands PASS без unexpected skips.

Первоначальный baseline runner вернул 1 **после успешных нагрузок** из-за слишком широкого source-manifest guard: разрешённые новые test-only files и audit utility появились в ходе baseline. Production и ранее существовавшие fixtures неизменны; отдельные production manifests совпадают. Это не скрытый workload failure: qualified source-consistency и cleanup receipts сохранены, baseline валиден для неизменённых измерявшихся production paths.

Финальные full test/race/vet/staticcheck/govulncheck/frontend lint gates прошли на production freeze. После последнего test-only fixture repair повторно прошли full test/race/vet/staticcheck (`final-post-fixture-*.log`). Логи остальных — `final-*.log`; source formatting и `git diff --check` также PASS. Первая попытка lint не нашла npm из-за слишком узкого PATH, команда повторена с установленным npm и тем же Node runtime; source для этого не менялся. Недостижимое module advisory: `GO-2026-5932`, unmaintained `golang.org/x/crypto/openpgp`; этот пакет не импортируется (подробный scan сохранён).

Дополнительная asset race проверка нашла **ранее существовавший дефект fixture**: pause callback ожидал `c.last_message_at,c.last_message_id`, тогда как уже в HEAD/baseline projection читает `lm.created_at AS last_message_at,lm.id AS last_message_id`. Из-за этого pause не срабатывал и тест ожидал 15s, хотя production projection оставался внутри authorization transaction. Isolated before-F overlay со старым constructor/reader guard/callers повторил оба timeout (FAIL32.547s); единственная compatibility правка временного overlay — удаление дублирующегося Get interface, который E уже перенёс. Live sources не откатывались. Test-only исправление сократило matcher до стабильного начала projection; все pause/removal/lock-wait/afterCommit/snapshot assertions сохранены. Повтор F: 6 top-level tests PASS8.603s без skips/race warnings, activate0.94s/clear0.96s. Evidence: `batches/F-223611`, `pre-f-repro/`, `batches/F-224029`. API/SQL production не исправлялись под тест.

Остальные batch real integration receipts: B13tests12.221s, E9tests24.937s, C/D6tests7.088s; все под race, без skips. По завершении batch fixtures — 0 тестовых БД/Redis keys; собственные PG/Redis/MinIO удалены по проверенным ID/labels, чужие контейнеры неизменны (`batches/cleanup.json`). Default all-package тесты сами по себе не доказывают opt-in coverage; она подтверждается этими отдельными запусками. Browser E2E, WAN/TURN и длительный capacity soak здесь не выполнялись.

Matched-after suite завершён с exit0 и неизменным полным source manifest на всём интервале. Повторены WS lifecycle/recovery (17.335s), reconnect100/race (3.971s), 23 key API tests/race (41.731s), SFU lifecycle/source churn/2–5 peers/audio isolation/race (88.866s), четыре media performance сценария (91.461s), две реальные записи с декодированием, recovery и auto-stop. Без skips/race warnings; финальные socket/session/key и manager-owned object counts — 0. Ресурсные показатели, source/binary hashes и ограничения сохранены в `tmp/p2-audit-20261010/regression-results.md` и `after/`.

Дополнительно после измерений `TestStageEightRecordingStorage` прошёл за 10.92s на том же frozen integration binary: composite и screen_focus — MP4 + preview; audio_only — audio без preview; individual_tracks — audio + ZIP. Во всех четырёх режимах проверены duplicate-start conflict, ready/mode/storage key, media timeline, подписанное скачивание приватных непустых файлов, два активных peer и отсутствие SFU drops. Evidence: `storage-mode-pod/recording/`; original helper не менялся. Все созданные тестовые контейнеры, сети и volumes удалены; 131 ранее существовавший контейнер, включая остановленные, остался неизменным. Пользовательские данные и рабочие сервисы не затронуты.

### 15. P1 performance preservation

Matched baseline/after: Go1.26.9, Apple M5 Pro/18 logicalCPU/48GiB, Docker29.4.2 Linuxarm64; один immutable FFmpeg6.1.2 worker image; одни fixture shapes; последовательные измерения без других агентских тяжёлых задач. Shared host, не dedicated load lab. RTP benchmark six variants ×6 samples; FFmpeg composite/audio/probe control ×6; четыре real-Pion windows; real decoded recordings/recovery/auto-stop. Никаких новых media hot-path allocations/interfaces.

| Workload | Before | After | Вывод |
|---|---:|---:|---|
| RTP 160B / 1 sink | 133.7 ns/op | 131.0 ns/op | p=.240; 176 B/op, 1 alloc — неизменны |
| RTP 160B / 2 sinks | 166.8 ns/op | 165.3 ns/op | p=.329; 176 B/op, 1 alloc — неизменны |
| RTP 1200B / 1 sink | 229.8 ns/op | 226.6 ns/op | p=.515; 1280 B/op, 1 alloc — неизменны |
| RTP 1200B / 2 sinks | 257.9 ns/op | 256.9 ns/op | p=.784; 1280 B/op, 1 alloc — неизменны |
| FFmpeg composite finalization | 83.21 ms/op | 83.11 ms/op | p=.937; 306 alloc/op → 306 |
| FFmpeg audio finalization | 48.81 ms/op | 49.12 ms/op | p=1.000; 229 alloc/op → 228.5 (медианы) |
| Unchanged duration-probe control | 15.66 ms/op | 15.29 ms/op | p=.065; 62 alloc/op → 62 |
| Real stop→ready, две комнаты | 410.521 / 413.954 ms | 371.706 / 363.208 ms | Одна пара запусков, только описательные данные |

Шесть samples на benchmark variant: значимых различий времени active-sink RTP/finalization и finalization allocations не найдено; RTP B/op и allocs/op точно совпадают. Zero-sink ветки — 7.489→7.272ns и 7.650→7.377ns (p=.002), 0 allocations. Этот небольшой сдвиг **не приписывается P2**: SFU production code не менялся. Composite interval остаётся широким (±14%/±12%): это отсутствие наблюдаемой существенной регрессии, не доказательство точной эквивалентности.

| Real-Pion окно (~5s) | CPU seconds before → after | Allocated MB/s before → after |
|---|---:|---:|
| Idle | .004632 → .004594 | .000234 → .000253 |
| 2 peers | .297678 → .285091 | 1.047316 → 1.049427 |
| 5 peers | 1.211816 → 1.043582 | 6.631733 → 6.496963 |
| 5 peers + screen + recording/audio tap | 1.109624 → 1.169247 | 7.559891 → 7.560076 |

SFU/recording/audio-tap drops — 0; после cleanup G3/FD6/owned objects0. Idle allocations 11→14 за окно — малые абсолютные числа, не новые per-packet allocations. Single-pair CPU/stop→ready изменения не доказывают ускорение/замедление; race reconnect burst 383→431ms также не является статистическим latency benchmark. Whole two-pipeline allocations 979,488,336→979,644,856B, post-GC heap4,249,848→4,238,232B, G3→7 в обоих запусках; сюда входят authentication fixtures и клиенты, не только recorder.

Итог: существенной регрессии в измеренной области не выявлено, нового расхода памяти на RTP нет. Ускорение от рефакторинга не заявляется. Полный P1 DB/Redis corpus не повторялся, поскольку запросы/ключи не менялись; exact unread projection/query-budget и presence semantics прошли real API regression. Полные benchstat таблицы: `tmp/p2-audit-20261010/egress-comparison.txt`, `finalization-comparison.txt`; media windows — `media-comparison.json`. Capacity/длительный soak/WAN этими результатами не сертифицируются.

### 16. Public API compatibility

URL/method/status/body/headers/WS events/Redis keys/storage keys/SQL schema сохранены. Важные исключения не «нормализуются»: malformed presence UUID400 против обычной validation422; unknown internal media error400 + media_unavailable против explicit unavailable503; raw deadline500 против explicit timeout504; WS code-only data, replyTo≤36; legacy machine-code `error` field decoding; old records routes410. Presence failure/unknown никогда не выдаётся как достоверное «offline». Internal Go AssetService constructor — единственная намеренная signature simplification; callers обновляются вместе.

### 17. Remaining P2 debt

1. Composer.Capture и media-worker command dispatch требуют отдельного stream/recovery-focused исследования перед следующей декомпозицией.
2. Legacy WorkerService всё ещё объединяет старый lifecycle и artifacts, но его keys/retention/fencing не равны composite; generic объединение сейчас опасно.
3. Integrations service/repository и personal mutations имеют дальнейшие operation-boundary возможности, нужны свои provider/job/transaction regressions.
4. `domain/records` persistence JSON coupling, old public-handler package/debug asset/config reachability — отдельный совместимый cleanup.
5. Unknown media-worker error HTTP400/503 различие документировано, не исправлено молча; изменение требует отдельного reproducer/contract decision.

### 18. P3 follow-up

Сокращение механических длинных комментариев, унификация локальных имён и организация больших файлов/тестовых fixtures — только последующий cosmetic review. P3 автоматически не выполняется. Рабочий Docker/remote deployment не входит в этот запрос.

### 19. Files changed in this P2 pass

- `docs/code-review/P2_ARCHITECTURE_AUDIT.md`, `P2_BEFORE_AFTER.md`: текущий аудит/результаты, old history preserved.
- `tools/architecture-audit/main.go`, `main_test.go`: воспроизводимый source inventory.
- `internal/app/httpresponse/error_contract_test.go`, `internal/transport/mediaworker/error_contract_test.go`, `internal/transport/websocket/error_contract_test.go`: A.
- `internal/domain/conferences/product.go`, `membership_policy_test.go`, `internal/infrastructure/postgres/conference_repository.go`, `conference_invitation_repository.go`: B.
- `internal/usecase/recorder/composite.go`, `composite_artifacts.go`, `composite_artifacts_test.go`: C/D.
- `internal/app/personal/peer_presence_handler.go`, `internal/usecase/personal/peer_presence.go`, `peer_presence_test.go`: E.
- `internal/usecase/personal/assets.go`, `assets_test.go`, `internal/app/api.go`, `tests/integration/group_assets_test.go`, `group_avatar_projection_race_test.go`, `group_browser_harness_test.go`: E/F wiring, committed-snapshot proof и точечная коррекция устаревшего SQL marker теста.
- Существующие P0/P1/UI changes не относятся к этому списку. Полный P2 набор — 23 файла, без staged/unstaged reset и автоматического commit.

---

## Исторический проход — 2026-10-06

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
