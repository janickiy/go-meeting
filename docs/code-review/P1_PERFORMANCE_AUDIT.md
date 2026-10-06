# P1: производительность и эффективность

Дата: 6 октября 2026. Базовый commit: `86bded31814db9eb748be0c04d7798a6b2212a85`. Задание: `CODE_REVIEW_AND_OPTIMIZATION_P1.md`. Аудит и нагрузки выполнены локально; развёртывание на удалённом хосте не выполнялось. P2 не реализовывался.

## 1. Результат

Приняты четыре изменения, основанные на измерениях: пакетная проверка присутствия Redis, пакетное чтение карточек записей, индекс для страниц записей и истории конференции, устранение повторного сканирования RTP-буфера при Flush записи. Состав ответов, проверки доступа, срок хранения, publisher confirms, очереди и штатное окно компенсации джиттера сохранены.

SFU, его буферы и блокировки оставлены без оптимизаций: измеренные собственные аллокации не обосновали дополнительную сложность. Числа, разделение эффектов и ограничения сравнения: [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md).

## 2. Окружение и исходные проверки

Apple M5, 10 CPU, 24 GiB RAM; macOS 27.0.1 arm64; Go 1.26.6. Docker 29.4.2, Linux VM: 10 CPU и 16 748 032 000 байт памяти. PostgreSQL 16.14, Redis 7; версии FFmpeg/RabbitMQ и параметры контейнеров зафиксированы в доказательствах записи. GOMAXPROCS/GOGC не менялись.

До production-правок выполнены:

| Проверка | Результат |
| --- | --- |
| `go test -count=1 ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |
| `go vet ./...` | PASS |
| gofmt, синтаксис release shell scripts | PASS |
| frontend `npm run lint` — проверка Prettier | PASS |
| staticcheck v0.7.0 | Только исходный S1024 в `persistent_auth_sessions_test.go:103` |
| govulncheck v1.8.0 | 0 уязвимых вызываемых путей; одна уязвимость зависимости вне вызываемого кода |

Локальные доказательства: `tmp/p1-performance-20261006/evidence/`. Приватные параметры подключений находятся отдельно, в файле с правами 0600, и не входят в отчёт. Профили/логи в `tmp` не отслеживаются Git; ключевые результаты перенесены в эти два документа. Воспроизводимые тесты включены в репозиторий.

## 3. Профили и методика

Сняты CPU, heap, allocs, mutex, block и goroutine profiles: idle; 2/5/10 участников; 5 и 10 участников в двух комнатах; экран с recording/audio tap; Redis reconciliation; DB/service/JSON; две параллельные записи. Дополнительно профилирован повторный WebSocket lifecycle.

SFU использует реальные Pion/ICE/DTLS/SRTP/UDP, но синтетические payload, без браузерного декодирования. Клиенты и сервер находятся в одном процессе, поэтому суммарный CPU/heap не является расходом только сервера. Прогрев 22 s заполняет bounded NACK history; затем 5 s измерения. RTP: audio 160/video 1200 байт, 50 пакетов/s на источник. В screen+recording+audio-tap сценарии encoded egress вычитывают тестовые goroutines; FFmpeg, Opus decode и STT здесь не запускаются. Их расходы не входят в эти SFU показатели; запись измерена отдельно. Профили heap sampling 65 536 байт; mutex fraction 1, block rate 1. Накопительные mutex/block/alloc profiles анализируются с исходным checkpoint, а не как независимые суммы разных subtests.

DB: случайные отдельные базы, миграции и ANALYZE, прогрев 5, 100 запросов на сценарий. Большой набор: 1000 встреч, 20 000 записей, 400 000 сегментов записи, 200 000 событий, 50 000 сообщений и 400 000 фрагментов транскриптов. Счётчик SQL включает Query/Row/Raw, исключает BEGIN/COMMIT. Gin-проверка включает handler/JSON с готовой identity, исключает TLS/сеть/auth verification/MinIO presigning.

Redis измеряется отдельно от SQL на реальных ключах и синтетическом Stale repository: 0/20/500 длительных сессий, 20 проходов за 5 s. Hook считает клиентские request batches; внутренние команды Lua и Pub/Sub handshake не входят в этот счётчик. Запись выполняется в собственном Docker pod на внутренней сети, с настоящими зависимостями и FFmpeg.

Окна тяжёлых нагрузок согласованы, но обычные процессы пользователя и существующие контейнеры продолжали работать. Малые изменения задержки не объявляются эффектом оптимизации. CPU sampling: 100 Hz: отсутствие samples означает недостаточное разрешение, а не нулевой CPU. Нагрузки короткие и не подтверждают максимальную вместимость production.

## 4. RTP allocations и copies

Путь: `TrackRemote.ReadRTP → Manager.receive → bounded subscriber queue → subscription.forward → TrackLocalStaticRTP.WriteRTP → NACK/SRTP/UDP`.

В SFU нет payload clone на каждого подписчика: один неизменяемый packet передаётся всем очередям. Pion создаёт receive buffer/Packet, сохраняет нужные копии для NACK и сериализации/шифрования. Перезаписывание SSRC/PT выполняется на поверхностной копии заголовка Pion. `recordPacket` маршалит RTP один раз и разделяет bytes между записью и audio tap.

Бенчмарк показал 0 alloc/op без egress, 1 alloc/op при одном **или двух** egress: 176 B для payload160, 1280 B для payload1200. Microbenchmark использует KindAudio и синтетические sinks для обоих размеров; второй sink не означает передачу video в production STT. Предполагаемый snapshot allocation в этом пути не подтвердился на Go1.26.6. В receive при 10 участниках snapshot составляет примерно 0,63% sampled allocations; при 2/5 не наблюдался в heap. Пул и reusable snapshot не добавлены.

Per-RTP JSON/Redis/SQL, дорогое debug formatting и payload compression в forwarding отсутствуют. Атомарные packet/byte counters сохранены. Собственный RTP stack, sharding и новые cache не добавлялись.

## 5. Buffer ownership и STT

| Участок | Владелец / срок жизни |
| --- | --- |
| Receive buffer/Packet Pion | Принадлежит packet; payload ссылается на этот buffer и может пережить текущий receive |
| Подготовка RTP в SFU | Producer меняет negotiated extensions до fan-out |
| Очереди подписчиков | Разделяют immutable packet до отправки/отмены; payload не меняется |
| Pion WriteRTP | Собственный временный header/SSRC/PT; shared immutable payload |
| NACK history | Собственные retained copies, до 1024 пакетов на подписчика |
| Recording/audio egress | Один immutable marshaled buffer; асинхронные consumers держат ссылку |
| PCM live STT | Отдельный captions worker/FFmpeg decoder; синхронный callback с ограниченным chunk |

Немедленное переиспользование receive/payload buffer нарушило бы срок жизни данных. STT остаётся вне RTP goroutine. Subscriber queue default128/max4096; recording default2048/max8192; audio tap max256. Overflow не блокирует producer и изолируется на соответствующем downstream. Decoder slots/LiveQueue ограничены; декодирование Opus выполняется отдельным процессом.

## 6. Блокировки и каналы

Изучены manager/room/peer/subscriptions, negotiation, forward barrier, WS registry, routing и recording state. Профили не подтвердили значимого SFU/WS registry contention. В WS replay суммарная mutex delay7,47 ms, около 97% — runtime locks; пользовательский Mutex Unlock около 0,04 ms. Ожидания select/network/timer в block profiles являются ожидаемыми состояниями горутин, не автоматически CPU bottleneck.

Negotiation/WriteRTP barriers обеспечивают согласованность SDP и готовности подписки. Их изменение требует отдельного доказательства выгоды. Длительность каждого lock hold и частота acquisition отдельно не инструментировались: pprof даёт суммарное ожидание, а наблюдавшаяся доля не обосновала production instrumentation/refactor. Mutex→RWMutex, новые shards и замена channels не выполнялись. Queues сохранили P0 bounds/drop/cancellation.

Rabbit publish mutex сериализует публикацию с confirm; measurable parallel wait объясняется этим контрактом. Throughput около 4,2k команд/s не обосновывает усложнение подтверждений.

## 7. Redis

Подтверждённый bottleneck: SQL Session намеренно обновляется при открытии/закрытии, поэтому давно работающие соединения попадают в Stale; Hub каждые 250 ms последовательно GET/decode проверял до 500 маршрутов. При 500 сессиях:10 022 request batches/5 s и 14 127 688 allocated bytes.

`RealtimeStore.Missing` выполняет pipeline EXISTS по тем же route keys, до 500 команд в batch; Hub закрывает только подтверждённо отсутствующие маршруты. Redis/context error отбрасывает частичные ответы. Истечение, LastSeenAt, SQL Close и TTL не изменены. После:42 batches/5 s,4 574 880 allocated bytes. Серверных команд всё ещё 10 023: сокращены round trips и JSON, не количество EXISTS.

Benchmark500:84,976→0,932 ms,9500→2521 alloc/op. Benchstat5 samples, p=.008 для 20/500; пяти samples недостаточно для 95%CI. При 2 строках timing gain статистически не подтверждён. CPU до:300 ms samples/5,02 s; после:0 samples, без заявления о 100% CPU saving.

go-redisv9.19.0 владеет pool/command retries с SDK jitter; app задаёт timeout3 s, MaxRetries2. Hub recovery выполняет один существующий receive worker с acknowledged Subscribe,100 ms…2 s. SDK может выполнить одно немедленное переподключение перед возвратом Receive error; Hub затем закрывает эту subscription. Дополнительного параллельного recovery worker нет. Hub-level jitter и многорепличный prolonged-outage herd не измерены и не изменены.

5 faults CLIENT KILL только собственной Pub/Sub:103,385–105,767 ms до readiness; replacement действительно получает событие. Это post-change observation при здоровом Redis, не restart/partition benchmark. Register/Touch/Unregister/Active/Prune — существующий Lua,1 warm round trip; cold NOSCRIPT может добавить EVAL. Rate limit/ownership также остаются атомарными. Heartbeat control plane default5 s; Redis per RTP нет. Pool hard cap не добавлен без измеренной saturation.

## 8. RabbitMQ

Connection/channel reuse, один consumer loop, publisher confirms и QoS1 сохранены. Payload — маленькие команды/ссылки, без медиа или transcript blobs. Serial confirmed publish229–242 µs/op; parallel298–356 µs/op. Publisher reconnect с topology/confirms2,42–2,51 ms; consumer disconnect→первая наблюдаемая delivery 1,004–1,011 s при существующем фиксированном backoff. Девять faults; наблюдение может включать redelivery, latency отдельной новой команды/p95 этим тестом не измеряется.

Prefetch1 соответствует последовательному handler; повышение только перенесло бы ожидание jobs в процесс. Consumer retry не имеет jitter; длительный multi-worker outage/thundering herd не измерен. Для текущего workload изменение recovery/prefetch/confirm pipeline не оправдано. P0 bounded close/cancel не ослаблены.

## 9. PostgreSQL pool

MaxOpen 20, MaxIdle 10, lifetime 30m; connect timeout5 s, statement_timeout10 s, lock_timeout5 s. Runtime Operations config задаёт MaxOpen/MaxIdle/lifetime из env; DB_QUERY_TIMEOUT отдельно управляет statement_timeout. Connect/lock timeout фиксированы. MaxIdleTime не задан. Чтение передаёт context; тестовые запросы имеют deadline10 s. Pool не увеличен.

32 клиента×5 страниц: WaitCount12 908→889, суммарный WaitDuration8,774→2,609 s, elapsed757→245 ms; peak Open/InUse20/20, final10/0/10, ошибок 0. WaitDuration суммирует ожидания разных горутин и не является wall time.

Четыре DB-процесса API/recorder/product/live с cap20 дают потенциальные 80 соединений. Реальный локальный PG, проверенный SHOW, допускает 100. Конфигурационный `docker-compose.shared-host.yml` задаёт 60: при его применении default budget80 превышает лимит. Это несогласованность комбинации конфигураций, не воспроизведённый отказ локального runtime. Дополнительные реплики увеличивают budget. Remote settings и общий 80-connection saturation не проверялись. Перед capacity tuning требуется суммарная нагрузка всех ролей и резерв для миграций/диагностики; произвольные новые pool limits не установлены.

## 10. N+1 и состав запросов

`recordings.List` выполнял `2+4N` SQL:6 для 1 записи,82 для 20. Новый batch после membership/page query повторно проверяет availability, загружает record/files/segments/events общими запросами и восстанавливает порядок UUID. Тот же DTO и public status, nullable fields, presigning builder; legacy adapters сохраняют fallback. Страница 1/20 теперь 6 запросов.

Остальные пути имеют постоянный budget: meetings20=1; history=6; participants20=4; chat20=6 или 7 с replies/attachments; notifications20=2; transcript20=5; summary=4; analytics=3; admin=3. Проверка чата с непустыми вложениями/ответами даёт 7 SQL и при 1, и при 20 сообщениях. Полный object-storage/UI chat E2E не запускался; DB decoration проверен напрямую.

Список записей сохраняет существующие segments/events metadata: fixture20 карточек содержит 400 сегментов и 200 событий. Двоичные медиа не читаются; transcript page ограничена. Удаление этих коллекций потребует отдельного API contract и не выполнено в этой минимальной правке.

## 11. EXPLAIN и индекс

После ANALYZE сохранён каждый фактически выполненный SQL и EXPLAIN(ANALYZE,BUFFERS). На 20k записей page/history выполняли Seq Scan, отбрасывая 19 980 строк. Candidate по `platform_conference_id, created_at DESC, id DESC`, partial `deleted_at IS NULL` и четырём recording modes, дал Bitmap Index/Heap Scan:588→3 shared buffers, page1,478→0,040 ms, history1,220→0,018 ms.

Миграция `000027_recording_page_index.up.sql` добавляет только этот индекс; старые checksums не менялись. Локальный build34 ms, размер 1 007 616 байт; small in-memory sort без temp spill. Retention deadline остаётся условием запроса. Общие COUNT с низкой selectivity не получили необоснованные индексы.

Миграционный runner транзакционный: обычный CREATE INDEX блокирует запись в таблицу на время построения/транзакции. Локальные 34 ms не определяют production окно. Write/WAL overhead не измерен. Текущий loader читает только up.sql, как миграции 23–26; автоматического down rollback нет. Развёртывание и операторский rollback в этом аудите не выполнялись.

## 12. Временные файлы и disk I/O

Проверен жизненный цикл файлов, включая успешное завершение и recovery:

| Данные | Создание / потребление | Очистка / сохранение |
| --- | --- | --- |
| Encoded IVF/H.264/Ogg sources и chunk manifests | `records/<UUID>/sources`, manifest .partial→atomic rename; composer читает завершённый chunk | Сохраняются при interruption/failure для recovery |
| Rendered MP4 chunks | Отдельный FFmpeg на chunk, фиксированные thread/concurrency limits | Повторно используются recovery/concat; после защищённого ready удаляется каталог, если KEEP_LOCAL=false |
| `concat.txt`, final MP4 и preview JPG | Composite concat использует `-c copy`; preview seek + один frame, SHA256 читает файл потоком | После upload и lease-guarded metadata commit очищается локальная запись |
| MinIO upload staging | FPutObject читает файл, затем StatObject сверяет размер; ключ включает recording и lease token | Неуспешный commit удаляет только generation prefix своего token; общий retention сохранён |
| STT input MP4 / WAV | Отдельный приватный случайный job directory; bounded object reader→io.Copy→disk; FFmpeg имеет byte/duration limits | Cleanup закрывает reader и один раз удаляет job directory; error path также удаляет каталог |
| Individual-tracks archive | CreateTemp в recording directory; `zip.Store`, streaming io.Copy + checksum, без повторного сжатия encoded media | Частичная ошибка удаляет ZIP; после upload defer удаляет staging archive |
| Legacy recorder intermediates | Каталоги `records/<UUID>` и `tmp/<UUID>` | Штатная cleanup удаляет только каталоги соответствующей записи |

Full-media ReadAll / MinIO→full RAM→disk в этих production путях не найден. ReadFile в composer касается manifests. Дисковые chunks нужны для recovery; заменять их pipes без измеренной выгоды и нового failure contract не стали. Segment duration в fixture 2 s; изменение duration, FFmpeg passes или codec settings не обосновано измерениями. Offline/live STT/provider CPU и полные длительные записи отдельно **not measured**: workload записи не запускает STT provider.

Парные две pipelines: Linux arm64, PostgreSQL17, Redis7, RabbitMQ4.2.9, MinIO, FFmpeg6.1.2; 4→6 участников в двух комнатах, screen, H.264/AAC640×360, около 11 s и 1 MB на каждый итоговый MP4. Fixture KeepLocal=true сохраняет sources для проверки; temp peak 8 013 114→7 806 045 B; stop→ready 364,962/414,852→363,403/364,156 ms. Этот timer включает Rabbit/control, FFmpeg, upload/DB и polling. FFmpeg-only `/proc` CPU lower bounds 0,40→0,35 s; all-child CPU 5,397→5,740 s включает другие subprocesses. Shared VM iowait и полные disk totals не измерены, причина и process counters приведены в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md).

Исходный профиль показал дорогой SampleBuilder.tooOld при forced Flush. Изолированный loss case воспроизведён через публичные Push/Pop: VP8 tail без partition head вызывает повторное сканирование пустых RTP slots. Linux6 repeats median697,140 ms→298,791 µs, одинаковые output/drop/timing и 0 B/0 alloc. Guard снимает age limit только внутри Flush, затем восстанавливает 200 ms; normal capture не меняется. Реальные outputs, wrap/reorder/loss и reuse проверены, включая mutation-test восстановления окна джиттера.

Matched normal-case CPU и allocations не показывают общего выигрыша: Go parent 3,964→4,373 CPU s, all-child5,397→5,740 s. Принята только доказанная правка дорогого loss case. Initial profile, парное сравнение и неприменимые попытки сохранены раздельно. Первый after-профиль остановила существующая MinIO bootstrap race BucketExists→MakeBucket; одинаковое предварительное создание disposable bucket в обеих measurement fixtures устраняет этот шум без production-правки. Последний after temp sample не захватил финальную очистку; measured EOFtemp0 не утверждается. Все собственные pod containers/network/anonymous volumes после запусков удалены.

## 13. Принятые оптимизации

| Находка | Доказательство | Минимальная правка | Основной риск / проверка |
| --- | --- | --- | --- |
| Sequential presence GET | 500 RTT/page, JSON allocation/profile | bounded EXISTS pipeline | Partial failure не должен закрыть живую Session; expiry/cancel/error tests |
| Recording page N+1 | 82 SQL для 20, рост пропорционален N | Batch records и existing related loaders | DTO/order/NULL/access/expiry equivalence |
| Full scan platform recordings | EXPLAIN588 buffers/19 980 лишних строк | Один query-specific partial index | DDL lock/write cost; реальные migration ledger/lock tests |
| RTP builder final Flush scan | Профиль tooOld/forced purge | Отключить только age scan во время Flush, восстановить 200 ms | RTP tail/holes/reorder/wrap/quiet reuse equivalence и реальная запись |

Ни одна правка не переносит STT в SFU, не отключает confirms, не увеличивает очереди и не меняет формат записанного медиа. JSON/compression/cache не оптимизировались: профили не обосновали отдельное изменение.

## 14. Before/after

Полная таблица обязательных метрик, отдельные batch/index эффекты и не измеренные показатели — в [P1_BEFORE_AFTER.md](P1_BEFORE_AFTER.md). Нельзя складывать показатели разных процессов или трактовать throughput-normalized workload как одинаковый объём работы.

## 15. Нагрузочные сценарии

Медиа-матрица idle/2/5/10/две комнаты/screen+egress прошла. Счётчики SFU/recording/audio-tap drops равны 0. Forwarded RTP/s считает успешные WriteRTP; downstream sequence gaps и полные сетевые потери отдельно не измерялись. При 10 участниках в двух комнатах измерено ≈17 995 forwarded RTP/s. API/DB: 100 samples на каждый путь и 32 concurrent clients; Redis: 0/20/500 sessions; recording две независимые pipelines. Подробные таблицы и ограничения представлены во втором отчёте.

## 16. P0 regression

Финальные `go test -count=1 ./...` — PASS, 18,208 s, `go test -race -count=1 ./...` — PASS, 22,694 s, 49 пакетов с тестами. `go vet`, gofmt, release shell syntax, frontend Prettier и govulncheck — PASS. Staticcheck: только исходный S1024, новых замечаний нет. Govulncheck: 0 вызываемых уязвимых путей.

Opt-in regression: SFU: 25 lifecycle +25 source churn + smoke2/3/5 + stalled audio tap под race — PASS, 57,317 s; после shutdown/churn maps0, G2/FD6. DB: 10 выбранных tests под race — PASS, 8,763 s: DTO/order/NULL/modes, access, expiry/deletion, cursor/history, retention/retry, content/analytics и migrations. Redis проверяет chunking1001 IDs, expiry/cancel/error и Pub/Sub recovery; реальный WS lifecycle повторён после DB/Redis изменений. Recording Flush equivalence и реальные FFmpeg output tests прошли 6 Linux повторов.

Первый final gate выявил тестовый adapter без нового Missing и временные .go overlay sources внутри module. Исправлены adapter и расширения snapshots; повторные gates успешны. Собственные Docker pods/Redis/network/anonymous volumes удалены; исходные 11 контейнеров продолжают работать.

Повторный WS replay:1020 lifecycles, горутины 15 во всех checkpoints, warmFD14/final15, final localWS/Redis keys/connectedSQL=0. Первый запуск совместил CLI CPU profiler с внутренним profiler старого теста и завершился ошибкой наблюдателя; исправлен режим запуска и выполнен успешный повтор. SFU fixture сначала зарезервировал лишнюю SDP-пару; corrected baseline использует штатные limits. Эти попытки сохранены отдельно, не выдаются за дефекты production.

## 17. Оставшиеся P1

1. Production capacity: длительная одновременная API/media/recording/product/live нагрузка, отдельный server-only SFU process, браузерные кодеки, TURN/TCP/NAT и remote configuration не измерены.
2. При shared-host overlay необходимо распределить суммарный DB budget ниже 60 с maintenance reserve; idle-time policy и Redis hard connection cap требуют combined load, не произвольных значений.
3. Многорепличные Redis/Rabbit outage, jitter и herd recovery не измерены. Serial Hub critical delivery выполняет bounded roster/state I/O; устранение повторных queries/cache требует latency workload и явной invalidation модели.
4. Recording list metadata остаётся пропорциональной числу child segments/events. Для долгих записей нужен отдельный summary projection contract; нынешняя правка сохраняет DTO.
5. Полный FFmpeg CPU/per-recording disk total, длительные composite/individual-track/STT нагрузки и retained temp после ошибок требуют отдельного продолжительного измерения. MinIO bootstrap BucketExists→MakeBucket допускает параллельную race; это исходная reliability находка, не регрессия Flush и не реализованная P1 оптимизация.

## 18. P2 follow-up

P2 записаны отдельно от реализованных P1: исходный staticcheckS1024; единичный profiling bind-failure waiter до process cancel; local-only debug completed-records listing собирает metadata всего bucket до limit; будущая cleanup/TTL политика для сохранённых recovery chunks. Удаление recovery-файлов без подтверждённой политики не выполнялось.

## Воспроизведение

Тесты нагрузки opt-in, обычный `go test ./...` их пропускает. Экспортируйте тестовые env в subprocess без вывода credentials. PostgreSQL DSN должен быть localhost с CREATE DATABASE: fixture создаёт/удаляет собственную случайную БД. Redis использует уникальный namespace. Для записи helper создаёт собственные изолированные PG/Rabbit/MinIO/Redis. Требуются cached Linux arm64 images и worker image с FFmpeg; список проверяется до создания контейнеров. Имя evidence directory должно быть новым.

```sh
go test ./internal/infrastructure/sfu -run '^$' -bench '^BenchmarkP1RecordPacket$' -benchmem -count=6 -benchtime=1s
RECORDER_P1_MEDIA=true RECORDER_P1_PROFILE_DIR="$PWD/tmp/p1/sfu" go test -v ./internal/infrastructure/sfu -run '^TestP1MediaPerformance$' -count=1 -timeout=10m -memprofilerate=65536
RECORDER_P1_REDIS_LOAD=true RECORDER_P1_PROFILE_DIR="$PWD/tmp/p1/redis" go test -v ./tests/integration -run '^TestP1RedisReconciliationLoad$' -count=1 -timeout=1m
go test ./tests/integration -run '^$' -bench '^BenchmarkP1PresencePage/(SequentialGet|Missing)' -benchmem -benchtime=1s -count=5
RECORDER_P1_DB_PERF=true RECORDER_P1_DB_CONFERENCES=1000 RECORDER_P1_DB_PROFILE_DIR="$PWD/tmp/p1/db" go test -v ./tests/integration -run '^TestP1DBWorkloads$' -count=1 -timeout=10m
go test -race ./tests/integration -run '^TestP1RecordingBatchPreservesPage$' -count=1
python3 tools/p1_audit/recording_pod.py --phase before --baseline-ref 86bded3 --output "$PWD/tmp/p1/recording-before"
python3 tools/p1_audit/recording_pod.py --phase after --rabbit --output "$PWD/tmp/p1/recording-after"
```

Before DB требует baseline 86bded3 с measurement harness, без batch/index; для recording baseline используется Go overlay исходного capture.go. Алгоритм sequential GET сохранён только в benchmark для повторного comparison. Benchstat: `golang.org/x/perf/cmd/benchstat@v0.0.0-20260929162123-406019bb8b68`. Команды/настройки recording pod и профильные summaries сохранены в `evidence/recording`, SQL/EXPLAIN — в `evidence/db`, ownership/нагрузка — в `evidence/sfu`.

## 19. Изменённые файлы

Production: `internal/infrastructure/{composite/capture.go,postgres/record_repository.go,redis/realtime.go}`, `internal/usecase/{realtime/hub.go,recorder/conference_reader.go,recordings/service.go}`, новая миграция `database/migrations/000027_recording_page_index.up.sql`.

Измерения/регрессии: `internal/infrastructure/composite/{performance_test.go,flush_performance_test.go}`, `internal/infrastructure/rabbitmq/performance_test.go`, `internal/infrastructure/sfu/{manager_test.go,p1_performance_test.go}`, `internal/infrastructure/redis/realtime_test.go`, `internal/usecase/realtime/reconciliation_test.go`, `internal/transport/websocket/persistent_session_test.go`, `tests/integration/p1_{db_performance,recording_performance,recordings_batch,redis_performance}_test.go`. Изолированный recording pod: `tools/p1_audit/recording_pod.py` и `tools/p1_audit/README.md`. Отчёты: этот документ и `P1_BEFORE_AFTER.md`.

Рабочее дерево содержит правки; commit/release/deploy не создавались. Следующий этап автоматически не запускался.
