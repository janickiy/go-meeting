# P1: измерения до и после

Дата: 6 октября 2026. Baseline: `86bded31814db9eb748be0c04d7798a6b2212a85`. Методика, ownership, правки и риски: [P1_PERFORMANCE_AUDIT.md](P1_PERFORMANCE_AUDIT.md).

Это локальные наблюдения разных процессов: их нельзя складывать. `not measured` означает отсутствие измерения, причина указана рядом. SFU и RabbitMQ не менялись; для них показан baseline. Для Redis сравниваются старый GET и новый Missing. Эффекты DB batch и индекса измерены отдельно. Парные записи используют одинаковый fixture и Go overlay исходного capture.go.

## 1. Обязательные метрики

| Метрика / сценарий | До | После / пояснение |
| --- | --- | --- |
| RTP allocs/op, payload160 egress | 0 без sinks; 1/176 B при 1 или 2 sinks | Код не менялся; дополнительной payload copy на второй sink нет |
| RTP allocs/op, payload1200 egress | 0 без sinks; 1/1280 B при 1 или 2 sinks | Код не менялся |
| RTP allocs/sec, 10 peers | 197 897, сервер + клиенты | Код не менялся; server-only not measured: один процесс |
| RTP CPU, 10 peers / 5 s | 4,066 CPU s, сервер + клиенты | Код не менялся; server-only not measured |
| Mutex wait/block | SFU10×2: 171,49 ms aggregate mutex; WS replay: 7,47 ms | Locks не менялись; block включает ожидающее время множества горутин |
| Individual mutex hold/frequency | not measured — pprof этого не даёт | Не добавлялись per-packet timestamps без bottleneck |
| Redis reconnect recovery | not measured количественно до правок; recovery code unchanged | 5 CLIENT KILL: 103,385–105,767 ms до readiness и получения события; не полный outage |
| Redis request batches, 500 sessions / 20 scans / 5 s | 10 022 | 42, включая одинаковые 2 setup batches; Redis commands 10 023 → 10 023 |
| Redis page500 latency | 84,976 ms | 0,932 ms, median5 runs, −98,90% |
| Rabbit reconnect recovery | Publisher 2,42–2,51 ms; consumer 1,004–1,011 s | Production без правок; consumer — первая наблюдаемая delivery, возможна redelivery; prolonged outage/herd not measured |
| Rabbit throughput | Serial ≈4262 confirmed commands/s; parallel ≈3038/s | Production без правок; маленькие команды, не FFmpeg jobs/s |
| DB pool wait, 32 clients × 5 pages | 12 908 waits / 8,774 aggregate waiter s | 889 / 2,609 s; peak Open/InUse20/20 в обоих прогонах |
| Queries / recordings page20 | 82 | 6; page1=6 → 6 |
| Queries / history | 6 для одной конференции; 1 timeline20 | 6/1; код уже bounded |
| DB/service recordings20 p95 | 51,312 ms | 4,016 ms, 100 requests/run |
| Gin/JSON recordings20 p95 | 29,690 ms | 17,292 ms final; candidate4,130 ms сохранён, не выбран как final |
| Recording temp bytes | 8 013 114 B / 135 files, sampled peak | 7 806 045 B / 130 files; небольшой разброс не объявляется выигрышем |
| Finalization time, stop→ready | 364,962 / 414,852 ms, две pipelines | 363,403 / 364,156 ms; обычный сценарий сопоставим |
| FFmpeg CPU | ≥0,40 CPU s: /proc sampled FFmpeg-only; все children 5,397 s | ≥0,35 CPU s; все children 5,740 s. Не доказано снижение CPU |
| Disk I/O wait | not measured — Docker VM shared I/O нельзя отнести к конкретной записи этим sampler | Process I/O ниже; это не iowait |
| Recording forced Flush | 697,140 ms, median6, missing VP8 partition head | 0,299 ms, median6; одинаковые output/drop/timing, 0 B/0 alloc |

## 2. Redis: одна и та же bounded presence page

Benchmark: 5 повторов по 1 s, реальные route keys собственного Redis7. До: GET и JSON decode каждой Session. После: production Missing, pipeline EXISTS. После измеряется также создание ID slice, как в Hub. Hook считает request batches и команды. Без ошибок request batches соответствуют round trips, а не числу TCP packets.

| Routes/page | medianµs до → после | B/op до → после | allocs/op до → после | Request batches до → после |
| ---: | ---: | ---: | ---: | ---: |
| 2 | 334,823 → 277,786 | 2624 → 760 | 38 → 21 | 2 → 1 |
| 20 | 3426,516 → 179,843 | 26 240 → 7096 | 380 → 117 | 20 → 1 |
| 500 | 84 976,277 → 931,669 | 656 003 → 167 256 | 9500 → 2521 | 500 → 1 |

Benchstat: timing p=.008 для 20/500; p=.095 для 2 routes, здесь timing gain не подтверждён. Для 95% confidence interval нужны минимум 6 samples, здесь 5. Детерминированные counts и большая разница для 20/500 поддерживают правку. Co-tenancy и процессы пользователя делают малые timing comparisons ненадёжными. Pipeline не является transaction и не меняет атомарность определения отсутствия отдельного route.

| Hub5 s / 20 scans | Batches до → после | Allocated bytes до → после | Mallocs до → после |
| ---: | ---: | ---: | ---: |
| 0 sessions | 22 → 22 | 1 256 072 → 1 240 048 | 538 → 499 |
| 20 sessions | 422 → 42 | 1 810 648 → 1 370 608 | 8277 → 2822 |
| 500 sessions | 10 022 → 42 | 14 127 688 → 4 574 880 | 185 161 → 50 913 |

Это TotalAlloc, **не retained heap**. Profiling overhead около 1,2 MB входит в оба прогона. CPU100 Hz: до 500=300 ms samples/5,02 s; после 0 samples означает недостаточное разрешение, а не 0 CPU или 100% gain. Lua Prune и SQL scan frequency сохранены. Ошибки/cancellation не вызывают SQL Close.

## 3. PostgreSQL: точный финальный набор

1000 встреч, 20k записей, 400k сегментов записи, 200k событий, 50k сообщений и 400k фрагментов транскриптов. Прогретый cache, 100 samples. SQL counts включают Query/Row/Raw, исключают BEGIN/COMMIT. Gin run исключает network, auth verification и presigning. Кроме recordings/history эти пути не оптимизировались; изменения их коротких latency tails не выдаются за эффект правки.

| Workload | SQL до→после | p50 ms до→после | p95 ms до→после | p99 ms до→после | B/op до→после |
| --- | ---: | ---: | ---: | ---: | ---: |
|recordings_http_20|82→6|24.444→5.677|29.690→17.292|35.891→21.792|2,699,870→1,983,688|
|recordings_1|6→6|3.286→1.379|5.292→3.125|7.846→3.593|128,301→135,581|
|recordings_20|82→6|27.572→2.700|51.312→4.016|54.605→6.687|1,907,655→1,242,347|
|meetings_20|1→1|0.435→0.445|0.485→0.535|0.634→0.570|52,223→52,272|
|history|6→6|2.990→1.780|3.797→2.486|4.042→3.270|81,254→79,607|
|participants_20|4→4|0.893→0.889|1.441→1.042|1.940→1.177|64,557→64,618|
|chat_20|6→6|1.477→1.541|2.463→2.783|2.904→3.254|95,755→96,139|
|notifications_20|2→2|0.436→0.527|0.499→0.681|0.626→1.340|51,324→51,450|
|transcript_20|5→5|1.042→1.051|1.642→1.885|2.208→2.307|53,648→53,325|
|summary|4→4|0.788→0.781|0.871→0.990|1.414→2.005|42,027→41,286|
|analytics|3→3|0.663→0.667|1.107→1.170|1.901→1.734|47,155→46,427|
|admin|3→3|4.164→4.276|5.234→5.329|7.114→6.398|26,176→24,825|

### Разделение batch/index эффекта

| Сценарий | Baseline p95 ms | Только batch | Batch + candidate index | Финальная миграция |
| --- | ---: | ---: | ---: | ---: |
| recordings20 | 51,312 | 7,384 | 3,923 | 4,016 |
| history | 3,797 | 4,232 | 2,662 | 2,486 |
| Gin/JSON recordings20 | 29,690 | 9,146 | 4,130 | 17,292 |

Final Gin p95 существенно отличается от candidate pass на общем хосте; оба значения сохранены. SQL counts, планы и нормализованные allocations показывают механизм независимо от этой вариативности.

EXPLAIN candidate index: recording page1,478 → 0,040 ms, history1,220 → 0,018 ms; shared buffers588 → 3; устранены 19 980 discarded rows. Размер 1 007 616 B, build34 ms на 20k fixture. Write/WAL overhead и production DDL lock duration **not measured**: обычный transactional CREATE INDEX потребует планирования окна развёртывания.

### Pool: 32 clients / 160 requests

| Metric | Before | batchonly | Final |
| --- | ---: | ---: | ---: |
|Elapsed ms|757|300|245|
|p50 ms|130,574|43,488|34,205|
|p95 ms|233,294|127,358|117,221|
|p99 ms|240,395|137,952|122,127|
|WaitCount delta|12 908|890|889|
|WaitDuration delta,s|8,774|3,152|2,609|
|Peak Open/InUse|20/20|20/20|20/20|
|Final Open/InUse/Idle|10/0/10|10/0/10|10/0/10|
|Errors|0|0|0|

WaitDuration суммирует ожидания всех waiters и не является wall time. Pool caps не повышались. Текущий локальный `max_connections=100` проверен SHOW; четыре пула имеют потенциальный budget80. Только shared-host overlay задаёт 60, поэтому эта комбинация требует отдельного общего budget. Отказ от 80 соединений в текущем runtime не воспроизведён.

DB profile за 3 s после оптимизации завершает больше запросов:540 → 840 ms CPU samples не означает CPU regression/gain за одинаковую работу. Нормализованные recordings20 B/op:1 907 655 → 1 242 347 (−34,9%); SQL82 → 6 (−92,7%). JSON/cache/compression не менялись.

## 4. RTP и медиа: baseline без production-правок

Audio 160/video 1200 B, 50 RTP/s; 22 s NACK warmup и 5 s measurement. Оба конца Pion в одном Go process; allocated bytes/s нельзя приписывать только standalone SFU. Forwarded bytes — payload, без header/SRTP/UDP/RTCP/retransmission. Forwarded RTP/s — успешные серверные WriteRTP, drops — внутренние SFU/egress counters; downstream sequence gaps не измерялись. Screen+recording+audio-tap вычитывает encoded egress тестовыми goroutines, без FFmpeg/Opus decode/STT; полноценная запись измерена отдельно.

| Scenario | forwardedRTP/s | CPU s /≈5wall s | allocatedMB/s(decimal) | mallocs/s | RTP/recording/audio-tap drops |
| --- | ---: | ---: | ---: | ---: | --- |
|idle|0.0|0.004|0.00|3|0/0/0|
|2_peers|200.0|0.407|1.05|7264|0/0/0|
|5_peers|1999.8|0.809|6.46|46421|0/0/0|
|10_peers|8999.0|4.066|26.12|197897|0/0/0|
|5_peers_2_rooms|3999.2|2.186|12.89|92797|0/0/0|
|10_peers_2_rooms|17994.7|5.378|52.29|395875|0/0/0|
|5_peers_screen_recording_audio_tap|2199.7|1.376|7.55|52173|0/0/0|

### Egress microbenchmark: 6 повторов

| Payload / sinks | Median ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| payload160 / 0 | 7,61 | 0 | 0 |
| payload160 / 1 | 142,0 | 176 | 1 |
| payload160 / 2 | 176,5 | 176 | 1 |
| payload1200 / 0 | 8,56 | 0 | 0 |
| payload1200 / 1 | 219,4 | 1280 | 1 |
| payload1200 / 2 | 275,0 | 1280 | 1 |

Оба payload размера проверяются с KindAudio и синтетическими sinks. Второй sink не является реальным video+STT сценарием. Одно marshaling обеспечивает оба egress. Receive snapshot при 10×2 составляет 0,31% общих allocation bytes; изменение с дополнительными правилами retention/ownership отклонено по acceptance rule. NACK retained copies сохранены. После каждой медиа-серии maps0, G3/FD6; post-GC heap0,37–1,85 MB. Удержанный RSS не равен утечке heap.

Mutex interval sums: idle31,83 µs;2 peers427,88 µs;5 peers7,35 ms;10 peers109,02 ms;5×2 rooms19,47 ms;10×2 rooms171,49 ms; screen+egress12,66 ms. В 10×2 readRTCP132,73 ms, forward5,27 ms; room registry не доминирует. Individual hold/frequency not measured: timestamp на каждом packet lock изменил бы короткий critical section. Сумма block waits по множеству goroutines не является request latency или CPU time.

## 5. Recording / FFmpeg / temp

Две отдельные full-stack pipelines в disposable Linux arm64 pod: PG17, Redis7, RabbitMQ4.2.9, MinIO и FFmpeg6.1.2. В двух комнатах 4→6 участников, screen, recovery/failure isolation/autostop; H.264/AAC, 640×360. Before использует исходный capture.go через Go overlay, after — Flush guard. Перед обеими параллельными нагрузками fixture одинаково создаёт свой bucket. KeepLocal=true сохраняет sources/layouts для validation до fixture cleanup; production default KeepLocal=false. Выходные артефакты по ≈11 s, каждый итоговый MP4 ≈1 MB, два MP4 на прогон.

| Метрика | Matched before | After |
| --- | ---: | ---: |
| Wall s | 15,926 | 16,133 |
| Go parent CPU s | 3,964 | 4,373 |
| Все children CPU s | 5,397 | 5,740 |
| Sampled FFmpeg-only CPU s, lower bound | ≥0,40 | ≥0,35 |
| Sampled FFmpeg PID count / max simultaneously | 6 / 2 | 9 / 2 |
| Peak temp B / file count | 8 013 114 / 135 | 7 806 045 / 130 |
| Stop→ready ms, pipeline1 / pipeline2 | 364,962 / 414,852 | 363,403 / 364,156 |
| Child input / output block count | 0 / 10 842 | 0 / 10 898 |
| Sampled FFmpeg physical read / write B, lower bounds | 0 / 8192 | 0 / 20 480 |
| Sampled FFmpeg rchar / wchar B, lower bounds | 1 170 211 / 96 | 3 843 390 / 493 496 |
| Largest sampled FFmpeg RSS KiB / FD count | 80 620 / 11 | 78 572 / 11 |
| Go allocated B, cumulative workload | 975 304 880 | 972 675 192 |
| Go heap after GC B | 4 231 672 | 4 262 600 |
| Go parent peak RSS KiB | 360 996 | 368 260 |
| Goroutines before / after GC | 3 / 7 | 3 / 7 |

`finalization` в fixture измеряется от POST /stop до наблюдения StatusReady: включает control/Rabbit, FFmpeg, upload, DB и polling; чистый composer-only finalization отдельно not measured.

42 `/proc` samples на прогон, интервал примерно 0,3 s; короткие FFmpeg-процессы могут целиком попасть между probes. Сумма максимальных CPU ticks по замеченным FFmpeg PID даёт **нижнюю границу**, не полный FFmpeg CPU. `RUSAGE_CHILDREN` включает FFmpeg, ffprobe и helper processes. Read/write counters не включают MinIO/Rabbit/PG I/O; physical reads0 могут обслуживаться page cache и не означают отсутствия чтения. I/O wait и полный per-recording disk total **not measured**: общая VM и неполная выборка процессов. CPU/alloc/temp разброс короткого normal-case прогона не доказывает выигрыш.

Последний sampled temp: before0; after4 178 670 B/69 files. После завершения after sampling не захватил конец очистки — measured EOFtemp0 не утверждается. Собственные контейнеры и их anonymous volumes затем удалены; cleanup фиксирует отсутствие остатков.

### Воспроизводимый bottleneck принудительного Flush

Один VP8 tail с отсутствующим partition head оставляет окно SampleBuilder в состоянии, где `tooOld` снова сканирует 65 535 пустых slots при принудительном purge. Шесть Linux повторов: baseline667,926–713,029 ms, median697,140 ms; guard294,957–325,958 µs, median298,791 µs (≈2333×). Измеряется только Flush, подготовка builder исключена из таймера; в обоих вариантах 0 B/0 alloc.

Изменение отключает age scan только на время принудительного Flush и сразу возвращает штатные 200 ms. Проверены одинаковые encoded samples/drop/timing и 9 RTP traces ×3 close/reuse cycles, включая sequence/timestamp wrap и неполные кадры. Mutation без восстановления окна джиттера ломает тест resume до следующего Flush. Реальные FFmpeg decode/regression tests прошли 6 повторов.

Это устранение дорогого патологического RTP loss case. В matched full-stack normal-case нет подтверждённого общего CPU gain. Первоначальный профиль с 41,37% Go CPU samples в tooOld сохранён как находка, но не подменяет парное сравнение.

## 6. Регрессии и очистка

Финальные `go test -count=1 ./...` (18,208 s), `go test -race -count=1 ./...` (22,694 s), `go vet`, gofmt, release shell syntax, frontend Prettier и govulncheck — PASS; 49 пакетов с тестами. Govulncheck: 0 вызываемых уязвимых путей, одна уязвимость требуемого модуля вне вызываемого кода. Staticcheck сохраняет только исходный S1024 в persistent_auth_sessions_test.go:103; новых замечаний нет. Opt-in integration/media нагрузки запускались отдельно.

Первый финальный запуск обнаружил недостающий Missing в тестовом WebSocket adapter и Go overlay snapshots с расширением .go внутри module. Adapter обновлён, snapshots переименованы в .go.txt; повторная сборка/race/vet успешны. Неуспешные gate logs сохранены отдельно.

Реальный WS replay1020 lifecycles: G15 стабилен, warmFD14/final15; warm heap4 395 120/final4 625 144 B; active WS, Redis keys и connected SQL0. SFU `-race`:25 lifecycle +25 churn + smoke2/3/5 + stalled audio tap — PASS, 57,317 s. Shutdown G2/FD6/heap797 624; после churn G2/FD6/heap796 152, maps0/drops0. DB: 10 selected tests `-race` — PASS, 8,763 s, включая DTO/modes/access/expiry/cursor/retention/migrations. Chat с replies/attachments: page1 и 20 обе 7 SQL.

Обычный `go test` не подменяет opt-in нагрузки. Непригодные попытки observer/fixture сохранены отдельно: двойной CPU profiler, лишняя SDP slot pair, устаревший Redis port, сначала пропущенный Row callback. Их результаты не используются как финальные доказательства.

## 7. Локальные артефакты

`tmp/p1-performance-20261006/evidence/`: environment.json, initial/final gates, Redis before/after/recovery/benchstat, DB SQL/plans/metrics/profiles, SFU baseline/regression, парные recording runs и Rabbit confirmed commands. SHA256 manifest включает логи, JSON и profiles. Private env/DSN исключены. Исходники тестов и benchmark находятся в рабочем дереве; команды воспроизведения находятся в основном отчёте.
