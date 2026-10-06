# P0-аудит надёжности и жизненного цикла ресурсов

Дата: 6 октября 2026. Базовый commit: `afc33ee31f5140aad6b25059aa7aa0f9b4e511ec`. Область: задание `CODE_REVIEW_AND_OPTIMIZATION.md`, только P0. Все нагрузки выполнялись локально. Production не изменялся.

## 1. Результат

Аудит включает исходные проверки, воспроизведение сбоев до правок, минимальные исправления, повторные нагрузки и профилирование. Подтверждены и исправлены восемь P0-находок: проблемы восстановления Redis, очистки MinIO, жизненного цикла legacy WebRTC/FFmpeg и блокирующего ввода-вывода RabbitMQ. Современный SFU прошёл повторные lifecycle/churn нагрузки без подтверждённой утечки. Это не оценка максимальной вместимости сервера и не подтверждение отсутствия любых дефектов.

Численные результаты и сопоставление каждого исправления: [P0_BEFORE_AFTER.md](P0_BEFORE_AFTER.md).

## 2. Исходное состояние и методика

- Рабочее дерево перед аудитом чистое, Go `go1.26.6 darwin/arm64`, macOS 27.0.1 arm64.
- Начало фиксации окружения: `2026-10-06T01:22:22Z`.
- Реальные Pion/UDP соединения; настоящий локальный PostgreSQL с отдельными временными БД; отдельный Redis контейнер без persistence, доступный только через loopback. Нагрузки не касались production конференций.
- Для FFmpeg используются отдельные тестовые контейнеры существующего arm64 worker image; на macOS FFmpeg отсутствует.
- Измерения относятся к **разным тестовым процессам**, их нельзя складывать в общий RSS или сравнивать как один production baseline.
- Прогрев → нагрузка → GC → cooldown → GC → повтор. Смотрим удерживаемый heap, goroutine profiles, FD и предметные реестры. Пиковый RSS сам по себе не доказывает утечку.
- Измерение FD на macOS: WS через `F_GETFD`, SFU через `lsof`. У SFU одинаковая временная pipe наблюдателя входит в каждый snapshot. Показатели между этими двумя тестами напрямую не сравниваются.
- В `go test ./...` интеграционные проверки без переменных окружения пропускаются. Ниже отдельно указаны реально запущенные интеграционные нагрузки; общий PASS не заменяет их.

Сырые локальные доказательства: `tmp/p0-reliability-20261006/evidence/`. Файл с параметрами тестовых подключений находится вне evidence, не входит в отчёт и не должен публиковаться. Профили и логи в `tmp` не являются отслеживаемыми Git файлами; существенные результаты сохранены в этих двух документах, воспроизводимые тесты — в исходниках.

### Проверки до изменений production-кода

| Проверка | Результат |
| --- | --- |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `go vet ./...` | PASS |
| gofmt, синтаксис release shell scripts | PASS |
| `npm run lint` в frontend | PASS |
| staticcheck v0.7.0 | Один исходный S1024 в `tests/integration/persistent_auth_sessions_test.go:103`: `time.Until` вместо вычитания `time.Now()` |
| govulncheck v1.8.0 | Уязвимых вызываемых путей не найдено; одна уязвимость зависимости вне вызываемого кода |

S1024 — исходное замечание к стилю, оно не исправлялось в рамках P0. Версии зависимостей не менялись.

## 3. Подтверждённые P0

### P0-01. Redis disconnect/Prune переводит realtime Hub в необратимый отказ

- **Severity:** Critical — все новые WS на затронутом API получают отказ до перезапуска.
- **Файлы/функции:** `internal/usecase/realtime/hub.go`: `receive`, `janitor`, `stopSockets`, `ShutdownContext`; `internal/app/api.go`: readiness checks.
- **Путь:** первая ошибка PubSub Receive или Prune → `stopSockets` устанавливает терминальный `closing` → receive завершён → Redis уже здоров, но новые регистрации запрещены. Ping Redis не выявляет отказ Hub. Автопереподключённая SDK subscription может оставаться без читателя до завершения процесса.
- **До правки:** в тестовом Redis убито строго одно PubSub соединение по уникальным ClientName/client ID. Redis Ping успешен, старый WS закрыт, все новые попытки в течение 3 секунд получают 409. Отдельно воспроизведён такой же результат после однократной ошибки Prune.
- **Исправление:** временный `unavailable` отделён от terminal shutdown. Единственный существующий receive worker закрывает старую subscription и выполняет acknowledged Subscribe с timeout 5 s и backoff 100 ms…2 s. Admission открывается только после успеха. Generation fence предотвращает закрытие новой subscription устаревшей ошибкой Prune. Shutdown отменяет retry и закрывает поздний результат Subscribe. Readiness проверяет Hub.
- **Риск:** при восстановлении текущие WS закрываются и клиенты переподключаются; transient error не означает бесшовную доставку неперсистентных событий.
- **Доказательства:** `evidence/ws/redis-disconnect-before.log`, `redis-prune-before.log`, recovery regression tests, before/after resource profiles.

### P0-02. RemovePrefix оставляет две горутины на неудачную bulk deletion

- **Severity:** High — повторяемое накопление горутин/удерживаемых ответов хранилища при ошибках очистки.
- **Файл/функция:** `internal/infrastructure/storage/s3/client.go: RemovePrefix`.
- **Путь:** MinIO ListObjects → RemoveObjects → первый per-object error → ранний return без чтения остальных результатов. В закреплённом minio-go v7.0.95 goroutine преобразования результатов и producer блокируются на channel send; отмена ctx эти sends не освобождает.
- **До:** 12 операций, по 8 AccessDenied в bulk response → **+24 SDK goroutines**, несмотря на cancel каждого ctx и cooldown. Pprof показывает `RemoveObjects.func1`, `removeObjects`, `processRemoveMultiObjectsResponse`, `runtime.chansend`.
- **Исправление:** доступные в текущей версии SDK синхронные `ListObjectsIter`/`RemoveObjectsWithIter`, child ctx с cancel. Ранний return завершает iterator producer. Ошибка перечисления передаётся отдельно, поскольку bulk iterator сам не обрабатывает `ObjectInfo.Err`. Батч SDK ограничен 1000 объектами.
- **После:** те же 12 ошибок → **0** оставшихся SDK goroutines, число горутин теста 4 → 4. Проверены успех, отмена, ошибка GET listing и повторный race-прогон.
- **Риск:** ошибка по-прежнему возвращается; частично выполненная deletion требует идемпотентного повторения существующим caller.
- **Доказательства:** `evidence/storage-before.log`, `storage-after.log`, `storage/*growth.txt`, heap/allocs/goroutine profiles.

### P0-03. RemoveRecording не освобождает version listing после ошибки удаления

- **Severity:** High — горутина на каждый неудачный цикл retention.
- **Файл/функция:** `internal/infrastructure/storage/s3/retention.go: RemoveRecording`.
- **Путь:** buffered ListObjects(WithVersions) → ошибка первого RemoveObject → cancel без drain. SDK пытается отправить последний `ctx.Err()` в уже заполненный канал и остаётся навсегда.
- **До:** 12 операций → **+12 ListObjects goroutines** после cancel. Это измерено после предыдущего subtest, поэтому общий счётчик SDK 24 → 36; собственная дельта именно 12.
- **Исправление:** version listing переведён на `ListObjectsIter`. Проверки канонического UUID, bucket и безопасных namespace сохранены.
- **После:** дельта 0, все операции завершаются с исходной ошибкой storage; versioned deletion на успешном пути работает.
- **Риск:** отсутствие файла не становится успехом при иной ошибке хранилища; повтор retention сохраняет прежний контракт.

### P0-04. Late offer возвращает к жизни уже удалённую legacy recording session

- **Severity:** High — live PC вне manager registry, с недетерминированной очисткой.
- **Файлы:** `internal/infrastructure/webrtc/manager.go`, `shutdown.go`; `handleOffer`, `handleTrack`, `stop`, `armTrackWaitTimeout`.
- **Путь:** offer уже получил указатель session; Stop удаляет её из manager, закрывает старый PC и отменяет ctx; offer проверяет только `failed` и регистрирует новый PC. После cancel также мог запускаться track-wait timer.
- **До:** `TestStoppedRecordingRejectsLateOffer`: `manager_active=0`, `stopped_context=context canceled`, `new_peer_created=true`, ошибка offer отсутствует. `TestCancelledRecordingDoesNotArmTrackTimer` также FAIL.
- **Исправление:** stopRequested/terminal stopping fence, проверки session ctx и идентичности PC, запрет новых offers/timers после остановки. Старый PC.Close выполняется вне session mutex. Сохраняется bounded grace 2–8 s для первых tracks уже принятого PC; startDone согласует ещё выполняющийся FFmpeg Start с Stop/Shutdown.
- **После:** late offer отвергается, нового PC нет, timer не создаётся. PID/Wait regression проверяет Stop и Shutdown во время startup, в том числе child, игнорирующий SIGTERM.
- **Риск:** поздние операции получают ошибку завершённой session; штатный fast-stop grace сохранён. Дополнительное окно регистрации процесса после terminal fence найдено при review и закрыто до финальных gates.
- **Evidence:** `recording/legacy-before.txt`, `legacy-after.txt`.

### P0-05. Неожиданный выход FFmpeg не освобождает recording slot

- **Severity:** High — две неудачные legacy записи могут исчерпать default MaxSessions=2.
- **Файлы:** `internal/infrastructure/ffmpeg/segment_recorder.go`, `internal/infrastructure/webrtc/manager.go`; Start/Stop/startFFmpeg/fail.
- **Путь:** первые 250 ms FFmpeg стартует успешно, позже завершается; cmd.Wait результат остаётся в канале без наблюдателя до ручного Stop. Session, PC и slot продолжают считаться активными.
- **До:** helper process выходит с code23 через 500 ms; после ожидания cleanup `manager_active=1`, `peer_state=new`, session ctx не отменён.
- **Исправление:** один cmd.Wait публикует completion через закрытие Done; результат доступен startup/monitor/Stop без конкуренции за единственный error value. Monitor вызывает fail, закрывает PC, отменяет ctx, освобождает registry slot и вызывает bounded failure callback. Stop идемпотентен через sync.Once.
- **После:** regression получает failure, Manager.Active=0, PC Closed, session canceled. Реальные FFmpeg success/error/cancel циклы не оставляют child processes.
- **Риск:** ранний неожиданный exit теперь явно переводит ingest в ошибку; normal Stop отмечен terminal fence и не запускает ложный failure callback.

### P0-06. OnStarted удерживает session mutex во время DB callback

- **Severity:** Critical — блокировка Stop/Shutdown при зависшей операции сохранения состояния.
- **Файл/функция:** `internal/infrastructure/webrtc/manager.go: startFFmpeg`.
- **До:** отдельный baseline reproducer на `git archive afc33ee` блокирует callback; `TryLock=false`, Stop не завершается до внешнего release. Callback передавался `context.Background()` и выполнялся под session mutex. После проверки test освобождает callback и child.
- **Исправление:** callback вызывается вне mutex с session ctx и timeout 5 s; OnFailed также ограничен 5 s. Production callback использует GORM с данным ctx. Условный DB transition starting→recording предотвращает восстановление terminal состояния поздним callback.
- **После:** `TestStartedCallbackDoesNotBlockRecordingStop` PASS под race: Stop завершает процесс, отменяет callback ctx, start goroutine освобождается.
- **Риск:** недоступная БД может получить timeout, который существующий callback логирует; освобождение медиа больше не ждёт session lock, удерживаемый БД.
- **Evidence:** `recording/callback-before.txt`, `legacy-after.txt`.

### P0-07. RabbitMQ cancellation не прерывает заблокированный AMQP write

- **Severity:** Critical — publisher и Close могут зависнуть с удерживаемым transport mutex; worker/control commands не продвигаются.
- **Файл:** `internal/infrastructure/rabbitmq/commands.go`.
- **Путь:** amqp091-go v1.13.0 проверяет ctx перед publish, но не прерывает уже начатый socket write. Heartbeat/read может оставаться работоспособным. Context истёк, запись и ожидание mutex продолжаются. Аналогичный риск есть у AMQP RPC/ack/nack и частичной инициализации.
- **До:** настоящий AMQP handshake с локальным RabbitMQ, затем детерминированно заблокирован write транспортом net.Pipe. Publish игнорирует cancel, Close не прерывает write; stack показывает library flush и net.pipe.write.
- **Исправление:** cancellation handler вызывает CloseDeadline и делает connection непригодным для повторного использования. Close помечает terminal state и закрывает connection до ожидания publish mutex. Startup/declaration/confirm/Qos/Consume/ack/nack получают bounded cancellation, handshake закрывается при cancel. Старый connection закрывается до замены, watcher относится к своему connection.
- **После:** реальные broker regressions PASS: blocked publish cancel ≈0.11 s, confirm/Qos/Consume/ack/nack каждый ≈0.11 s, Close прерывает write ≈1.01 s; reconnect/quarantine/correlation/drain проходят.
- **Риск:** неопределённый результат публикации при разрыве connection требует существующей идемпотентности команд. Повторная доставка допустима. Per-publisher transport mutex сохраняет сериализацию channel операций, но I/O теперь ограничен; глобальной новой блокировки нет.
- **Evidence:** `recording/rabbit-before.txt`, `rabbit-after.txt`.

### P0-08. Частично подготовленная запись занимает slot после ошибки БД

- **Severity:** High — повторные неудачные команды могут занять все recording slots без медиа.
- **Файл/функция:** `internal/usecase/recorder/service.go: WorkerService.handleStart`.
- **Путь:** ingest.Prepare выделяет local session → AddEvent(record.worker.ready) возвращает ошибку → ошибка уходит в Rabbit retry/quarantine без rollback. У prepared session ещё нет track-wait timer, поскольку offer не поступил.
- **До:** fault injection только в repository AddEvent с **настоящим WebRTC Manager** показывает Active=1 после неудачного handleStart.
- **Исправление:** перед возвратом ошибки вызывается ingest.Stop; ErrNoMedia ожидаем, остальные ошибки объединяются с исходной. Local allocation освобождается до retry/quarantine.
- **После:** две последовательные ошибки подготовки оставляют Active=0 после каждой попытки, race PASS.
- **Риск:** готовность worker не объявляется после ошибки БД; повторная команда может заново подготовить session. SQL transaction redesign не выполнялся.
- **Evidence:** `recording/prepare-before.txt`, `prepare-after.txt`, `TestWorkerReadyEventFailureReleasesPreparedSession`.

## 4. Владение горутинами, контекстами и таймерами

Инвентаризация стартовых точек и таймеров: `evidence/production-start-points.txt`; исходный поиск до изменений — `resource-start-points.txt`. Ниже сгруппированы **все семейства** production start points; anonymous closures и `peer.start` входят в владельца, а не считаются отдельными независимыми сервисами.

| Семейство | Owner / start | Stop / context / wait |
| --- | --- | --- |
| `app/api`, `worker`, `media_worker`, `product`, `live` | Контекст процесса; HTTP, maintenance, samplers | HTTP Shutdown/Close, отмена process ctx, bounded drain; worker/live/product ждут свои done; короткие maintenance операции ограничены ctx |
| `operations.Runtime.Run/Profiling`, `app/operations` | Process ctx; probes 5 s / samplers 2–5 s, loopback pprof | ticker.Stop, закрытие listener по cancel; pprof выключен по умолчанию |
| `transport/websocket` | Handler владеет read lifecycle и одним write pump, heartbeat/auth lease | stopOnce/cancel, socket.Close прерывает чтение, ticker/expiry timer Stop, Hub.Unregister и bounded cleanup |
| `usecase/realtime` | Hub: receive, low-priority delivery, janitor | hub ctx, workers WG, socket WG; recovery внутри того же receive worker; shutdown terminal |
| `app/notifications` SSE | Один handler/подписка на активный ответ | ctx/drain, subscription Close, count decrement, keepalive.Stop, write deadline |
| Современный SFU | Manager → peer → publication/subscription; peer.start регистрирует loop до запуска | detach cancels/removes; PC.Close и sender.Stop прерывают RTP/RTCP; peer WG; manager closeWG закрывает shared listeners после peers |
| Media control handler | Room lease watchdog и освобождение owner | room identity/version fence, ctx, ticker.Stop, bounded cleanup |
| Legacy WebRTC | Manager → session → PC/track forward/PLI/wait timer/process monitor | terminal session fence, PC.Close, session cancel, Wait процесса, остановка wait timer; исправления ниже |
| `ffmpeg` | CommandContext; один cmd.Wait; decoder stdout reader | cancel → terminate → kill fallback/WaitDelay; stdin/pipes close; completion broadcast |
| `composite` | Capture: compositor jobs и input reader; bounded slots | child cancel, reader Close, channels close, done wait; temp path принадлежит recording generation |
| `usecase/recorder/composite` | Poller, leased recording run, lease renew, capture monitor | StopAccepting/drain, WG, bounded max duration, poll/renew timers Stop; сохраняет нужные recovery artifacts |
| `rabbitmq` | Publisher connection watcher; один consumer loop | NotifyClose завершает watcher; bounded connection close/cancel; reconnect не добавляет независимого consumer loop |
| `captions.Worker` | Конференции/track slots, lease renewal, decode copy, provider receive | ctx дерева, deadline, cancel/input.Close, channel close, work.Wait; renewDone/eventDone/copyDone |
| `liveproviders` | WebSocket session reader | AfterFunc(ctx) закрывает conn, bounded writes, done/events close; Close ждёт reader |
| `content.summary`, `jobs` | Bounded semaphore для chunks; фиксированный runner | acquire до go, ctx cancellation, WG; polling ticker.Stop |
| `conferences.ControlService` | reconcile batch ≤64, workers ≤8 | закрывает jobs, workers.Wait; операции ≤2 s, query ≤3 s |
| `notifications`, `recordings`, `chat`, `retention` | Process maintenance loops | таймеры Stop; bounded operation ctx; выход по process cancellation |

`time.After` в ограниченном shutdown/retry select не создаёт бесконечного polling/busy-loop. Explicit NewTicker/NewTimer проверены на Stop и владельца. Исключение низкого риска для pprof bind failure вынесено в P1, без изменения production.

## 5. Современный Pion/WebRTC

Join учитывается в `closeWG` до создания PC. Ошибки после allocation закрывают PC и отменяют контекст. Detach под mutex помечает closing и удаляет peer, connection, screen owner и последнюю пустую room. Внешние операции Redis и PC.Close выполняются вне manager lock.

RTP loop выходит на ошибке ReadRTP, RTCP — на sender.Stop/Close. Unsubscribe удаляет sender и оба индекса подписки; unpublish останавливает PLI и убирает публикацию. Negotiation сериализует изменения SDP/AddTrack/RemoveTrack, notifications coalesce с revision. ICE/admission watchdog прекращается по peer cancellation.

Проверено 100 циклов 2/3/5 участников, 10 reconnect, 352 PC с прогревом; 9770 пересланных пакетов, 0 drops/failed PCs. После каждой серии и shutdown: rooms/peers/tracks/subscriptions/connections/room closures = 0. Отдельно 100 camera/mic/screen cycles. Дополнительная нагрузка `-race`: 25 lifecycle + 25 churn, PASS.

RTP синтетический, но PeerConnection/ICE/DTLS/SRTP/UDP настоящие. Проверялась доставка актуальных track IDs, не браузерное декодирование или качество изображения.

## 6. WebSocket, SSE и ParticipantSession

Single writer, bounded очереди, ping/pong/read/write deadlines, idempotent Stop и освобождение local/Redis/SQL presence проверены. Медленный клиент отключается при переполнении critical queue; low-priority events имеют отдельную очередь. Critical signaling не отбрасывается незаметно.

1000 connect → auth → initial state → disconnect циклов на двух Hub/API после 20 warmup. Клиенты закрываются немедленно, а не накапливаются в t.Cleanup до завершения нагрузки. После каждой серии: local WS 0, Redis namespace keys 0, SQL ParticipantSessions с открытым подключением 0. Исторические строки завершённых ParticipantSession допустимы и не являются утечкой presence.

SSE: глобально ≤256, на пользователя ≤4; buffer 32; slow write ограничен 5 s, keepalive 15 s; отказ/ctx закрывает subscription и уменьшает счётчики. Сервис не создаёт goroutine на каждый event.

## 7. Очереди и backpressure

| Очередь / map | Ограничение | Overflow / cleanup |
| --- | --- | --- |
| WS critical / low / control | 64 / 8 / 8 в default config | critical/control overflow → disconnect; low drops только разрешённых типов |
| Hub low events / local sockets | 128 / WS max 1000 per process | low drop; admission limit; unregister |
| SFU peer event / negotiation | 128 / 1 | close peer / coalesce с revision |
| RTP subscription | default 128, max 4096 | drop + PLI; cancel/Stop |
| Egress / audio tap | default 2048, max 8192 / max 256 | fail egress, явный error; закрытие потока |
| Peer ICE / SDP | pending 64, total 128 / 49152 bytes | reject/close |
| Rooms, peers, published tracks, screen owners | Config caps | reject admission; detach/unpublish удаляют entries |
| Composite jobs / read frames / FFmpeg slots | 4 / 512 / configured semaphore | bounded blocking с cancellation, overflow/failure заканчивает capture |
| Chat upload / media control admission | 4 / 128 | отказ при перегрузке |
| Caption conferences / sessions / frames | Config caps / LiveQueue | acquire до go; drop + degraded / cancellation |
| AI chunks / search workers | MaxChunks + AIConcurrency / EmbeddingWorkers | bounded input и semaphore, ctx |
| Prometheus custom labels | 64 фиксируемых имён | неизвестные resource labels не создаются сверх cap |

Карты media control `roomGates` удаляются по refcount (включая отмену ожидания); `offers` удаляются при leave/sweep/fence, `leases` — после последнего peer/egress, потери ownership или fence. Sweep работает каждые 2 s, heartbeat каждые 5 s по default config. У кратковременных stale `offers/leases` нет отдельного численного hard cap; применяется периодическая очистка.

Необоснованной неограниченной realtime очереди в проверенных путях не найдено. P1 по полной материализации списка завершённых записей приведён отдельно.

## 8. FD, файлы, HTTP, частичная инициализация

Проверены точки `os.Open/Create/OpenFile/CreateTemp`, UDP/TCP listener/dial, FFmpeg pipes, HTTP response bodies и storage streams. GORM операции владеют строками/транзакциями внутри repository; пул SQL закрывается процессом. Оптимизация SQL не выполнялась.

- JSON/mediaworker тела ограничены middleware/MaxBytesReader до ReadAll; chat attachment ограничен max+1 и upload semaphore.
- Обычные outbound HTTP clients имеют timeout, ответы закрываются через defer. Provider/OAuth/content ответы дополнительно ограничены по размеру.
- Долгие egress/audio HTTP streams используют request lifecycle ctx, header timeout и явный reader/body.Close, а не общий короткий HTTP timeout.
- SMTP DialContext, socket deadline и AfterFunc(conn.Close) ограничивают весь обмен; клиент и соединение закрываются. Аудит не отправлял писем.
- Transcription download: input.Close, output file.Close, bounded copy; возврат Audio передаёт владение reader+temp cleanup caller, который вызывает Cleanup.
- Readiness temp file закрывается и удаляется сразу; failures проверок доступа не оставляют открытый FD.
- В SFU частичная инициализация listeners, failed SDP/ICE, duplicate close и shutdown покрыты существующими и новыми тестами. Ошибки storage и legacy recorder покрыты новыми воспроизводящими тестами.

## 9. Pprof и CPU

Pprof уже существует: default `PPROF_PORT=0`, при включении bind строго `127.0.0.1` внутри процесса/контейнера. Production nginx отклоняет debug routes; port mapping не добавлялся. Heap/allocs/goroutine/mutex/block/CPU доступны. Новый `TestProfilingEndpointsAndShutdown` проверяет реальные HTTP endpoints и освобождение listener при cancel. CPU/trace duration ограничен до 60 s существующим handler.

Нагрузочные тесты включают mutex/block sampling только в тестовом процессе. Heap/allocs/goroutine/mutex/block профили записаны на checkpoints, CPU — на idle-after-load. Block и mutex профили накопительные: время ожидания множества goroutines нельзя трактовать как latency одного запроса.

SFU idle CPU: 0.002219 CPU s за 2.019151 wall s; профиль 100 Hz не получил samples. Это ниже разрешения семплирования, а не утверждение о нулевом CPU. Горутинный diff warmup → 100 media cycles = 0. Активный heap при source churn выходит на плато после заполнения bounded NACK buffers Pion; после leave heap возвращается ниже 0.67 MB, хотя RSS остаётся около 50 MB.

Для MinIO pprof подтвердил именно заблокированные goroutines, а не только изменение RSS; после правки горутинный diff равен 0.

## 10. P1: только последующая работа

1. `ListCompletedRecords` собирает карту всех подходящих objects до применения limit. Для большого бакета это рост времени и памяти; production workload с исчерпанием ресурсов здесь не воспроизводился. Рассмотреть paginated/indexed listing отдельной задачей.
2. `Runtime.Profiling` запускает waiter на process ctx до ListenAndServe. При bind failure остаётся один waiter до process cancel. Запуск однократный на процесс; повторяемого нормального накопления нет. Возможен локальный cleanup при bind failure.
3. RTP allocation/copy, forward-lock contention и заполнение NACK cache — предмет отдельного perf tuning после capacity benchmark. В текущих нагрузках кэш ограничен и освобождается.
4. Исходный staticcheck S1024 в auth integration test — style follow-up, не P0.
5. Multi-hour soak, реальный браузер/кодеки, TURN/TCP/NAT и производственная конфигурация лимитов требуют отдельного планового capacity/interop прогона.

6. Hub janitor выполняет bounded dependency work каждые ≤250 ms, critical bus delivery последовательна и может ждать bounded roster/state I/O. Масштабирование требует отдельного latency/capacity benchmark.
7. Явная настройка `WS_MAX_CONNECTIONS=0` отключает admission cap; default 1000 ограничен. Значения конфигурации в этом аудите не менялись.

8. Failed durable local chunks сохраняются для recovery; отдельная проверка их disk quota/TTL garbage collection не выполнена. Семидневный retention объектов не гарантирует expiry локальных failed files. Автоматическое удаление recovery artifacts не добавлялось.

P1 изменения в этом аудите не выполнялись.

## 11. FFmpeg, запись и артефакты

Все launch families проверены: SegmentRecorder.Start; postprocessor/ffprobe/transcription через runBounded; Composer.Compose; LiveAudio.Decode. CommandContext, graceful terminate, kill fallback и Wait/WaitDelay обеспечивают закрытие child/pipes; stderr/stdout диагностик ограничены (64 KiB/8 KiB в соответствующих путях). Отдельная goroutine владеет cmd.Wait; Done передаёт завершение всем наблюдателям. Прекращение регистрации session синхронизировано с in-flight Start.

В Linux arm64 контейнере с настоящим FFmpeg 6.1.2 и `--network none` выполнены 17 segment starts, 14 успешных finalize, 15 отмен/тайм-аутов, 6 ожидаемых ошибок malformed input/disk-full, 6 live-decoder cancellation. Это счётчики операций, не число полноценных пользовательских конференций. Для воспроизводимого process теста launcher использует lavfi media; real RTP lifecycle измерен отдельно в SFU.

После warmup и каждого из трёх раундов: goroutines=2, FD=7, child processes=0, temp files=0 и temp bytes=0. Heap после GC 1431512 → 1477792 bytes; RSS 16396288 → 15499264 bytes. Временный каталог fixture очищается после каждой операции; это не утверждение, что production recovery artifacts должны удаляться немедленно.

Все 10 существующих composite tests прошли с реальным FFmpeg, включая sparse video, resolution changes, error/cancel. Transcription extraction/cleanup и download/admission failures также прошли. Дополнительно выполнен существующий `TestStageFourCompositeRecording` в отдельном Linux pod: API→PostgreSQL→RabbitMQ→SFU→FFmpeg→MinIO, PASS за 15.38 s. Нагрузка включает трёх участников, screen churn, восстановление expired recording lease, изоляцию намеренного egress409 и auto-stop при завершении конференции. Результат: H264/AAC 640×360, 11.118333 s, 993533 bytes, два артефакта MinIO; содержимое проверено декодированием 44 видео-кадров и смешанного аудио. Это один acceptance workload, не длительный recording soak.

`TestRecordingRetentionObjectStorage` с versioning=true также PASS (0.13 s): проверяет реальное удаление исторических версий и сохранение соседних объектов. Pod использовал отдельные PG17/Redis7/Rabbit4.2.9/MinIO, общую внутреннюю network namespace и internal-only Docker network без host ports. Все собственные контейнеры и anonymous volumes удалены, сеть удалена, существующие сервисы не изменялись. Evidence: `recording/isolated-full-stack-recording.txt`, `isolated-e2e-setup.json`, `isolated-e2e-cleanup.json`.

## 12. Race и регрессии

Исходный полный race suite был зелёным. Подтверждённые ошибки включали semantic lifecycle races (Stop/offer/Start), которые не обязаны вызывать Go data-race warning. После исправлений отдельные race suites покрывают Redis recovery, WS, storage failures, legacy recorder callbacks/Start/Stop/Shutdown, Rabbit transport cancellation и prepare rollback. Современный SFU дополнительно прошёл opt-in race workload.

Независимый review проверил MinIO iterator cancellation и media-worker maps, затем запись. Найденное при review окно Start/Stop исправлено; повторный review подтвердил ownership и сохранение bounded grace. Новые major dependencies и архитектурные переработки не вводились.


## 13. Итоговые проверки

| Проверка после заморозки кода | Результат |
| --- | --- |
| `go test -count=1 ./...` | PASS, 49 пакетов с тестами |
| `go test -race -count=1 ./...` | PASS, 49 пакетов с тестами; нет data-race reports |
| `go vet ./...` | PASS |
| staticcheck v0.7.0 | Единственный исходный S1024, новых замечаний нет |
| govulncheck v1.8.0 | Уязвимых вызываемых путей не найдено |
| frontend Prettier formatting | PASS (`npm run lint`) |
| gofmt / `bash -n` / `git diff --check` | PASS |
| Real Redis recovery race ×3, unit recovery/shutdown | PASS |
| Existing WS/SSE integration и 100 simultaneous WS | PASS |
| 1020 WS cycles before/after | PASS |
| 100 media cycles +100 churn; дополнительный race25+25 | PASS |
| Storage cleanup + listing error/cancel, race×3 | PASS |
| Legacy late-offer/process/DB callback/start-stop/shutdown, race | PASS |
| Partial worker preparation rollback, race | PASS |
| Real Rabbit blocked write/RPC/Close/reconnect/drain/quarantine, race | PASS |
| Real FFmpeg repeated lifecycle/transcription и 10 composite tests | PASS |
| Local HTTP pprof endpoints + shutdown | PASS |
| Isolated StageFour full-stack recording + real versioned MinIO retention | PASS |

Логи `final-*.log`, `final-gates-summary.json` и scoped evidence содержат результат каждого запуска. Общий go test не включает opt-in e2e по умолчанию; реально выполненные opt-in workloads перечислены отдельно.

## 14. Оставшиеся P0 и границы вывода

Подтверждённых неисправленных P0 в исследованных путях после этих исправлений и регрессий не осталось. Это заключение ограничено воспроизведёнными сценариями. Короткий локальный audit не доказывает бессрочную production стабильность. Не измерялись multi-hour soak, реальный браузер/камеры/TURN, sustained capacity и полный успешный live-caption/PCM pipeline. В найденные P1 автоматически не переходили.

Pprof по-прежнему выключен по умолчанию и не публикуется наружу. Схема БД, UI, зависимости и production установка не изменялись. Изменения оставлены в рабочем дереве для review; commit/deploy этим аудитом не выполнялись. Временный WS Redis и отдельный recording pod полностью удалены после проверок.

## 15. Изменённые файлы

Production (9):

- `internal/app/api.go` — readiness Hub.
- `internal/usecase/realtime/hub.go` — bounded Redis recovery/shutdown.
- `internal/infrastructure/storage/s3/client.go` — bulk iterator cleanup и listing errors.
- `internal/infrastructure/storage/s3/retention.go` — version iterator cleanup.
- `internal/infrastructure/ffmpeg/segment_recorder.go` — broadcast completion/Wait, idempotent Stop.
- `internal/infrastructure/webrtc/manager.go` — lifecycle fences, process monitor, callbacks, in-flight startup ownership.
- `internal/infrastructure/webrtc/shutdown.go` — terminal fence и ожидание pending startup.
- `internal/infrastructure/rabbitmq/commands.go` — bounded network cancellation/close/RPC.
- `internal/usecase/recorder/service.go` — rollback failed preparation.

Регрессионные/нагрузочные тесты (9):

- `internal/usecase/realtime/recovery_test.go`
- `tests/integration/p0_ws_lifecycle_test.go`
- `internal/infrastructure/sfu/p0_lifecycle_test.go`
- `internal/infrastructure/storage/s3/lifecycle_test.go`
- `internal/operations/profiling_lifecycle_test.go`
- `internal/infrastructure/ffmpeg/resource_lifecycle_test.go`
- `internal/infrastructure/webrtc/recording_lifecycle_test.go`
- `internal/infrastructure/rabbitmq/publish_lifecycle_test.go`
- `internal/usecase/recorder/start_lifecycle_test.go`

Документы: этот отчёт и `docs/code-review/P0_BEFORE_AFTER.md`. Сырые профили и fault logs сохранены локально в evidence; параметры подключений в документы не включены.
