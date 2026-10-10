# P1: измерения до и после

## Повторный аудит — 10 октября 2026

Текущий baseline: `a0781257debba3f911ee180067cc0fad5bb06204` плюс исходные P0/security/UI working-tree изменения. Go1.26.9, Apple M5 Pro18CPU/48GiB, macOS arm64; recording в Linux arm64 Docker VM10CPU. Результаты 6 октября ниже — **исторические**, не before для этой работы. Методика/контракты: [P1_PERFORMANCE_AUDIT.md](P1_PERFORMANCE_AUDIT.md).

### 1. Обязательные метрики текущего прохода

| Метрика | До | После / интерпретация |
| --- | ---: | --- |
| RTP egress allocs/op, payload160 | 0 без sinks; 1/176 B при1 или2 sinks | SFU не менялся; один marshal обслуживает оба sinks |
| RTP egress allocs/op, payload1200 | 0 без sinks; 1/1280 B при1 или2 sinks | SFU не менялся |
| RTP allocations/s, 10 peers | 227 511 mallocs/s; 27.144 MB/s | Current baseline, server+clients; server-only not measured |
| RTP CPU, 10 peers / ~5s | 4.265745 CPU s | Код не менялся; не сравнивать с прежним host/Go |
| Mutex wait/block, 10×2 | 225.66 ms aggregate mutex; 7551.96 aggregate goroutine-s block | Locks не менялись; 99.86% block — select wait, не CPU |
| Individual lock hold/frequency | not measured: pprof не даёт распределение hold/acquire | Per-packet timestamps без hotspot не добавлялись |
| Redis recovery | 105.888–109.492 ms, 5 faults | Код не менялся; CLIENT KILL собственной Pub/Sub при здоровом Redis, readiness + доставка |
| Redis round trips | Missing500:1 batch/500 commands; Online1/20/100:1 batch/1 Lua command | Уже пакетный текущий код; не новая экономия500→1 |
| Rabbit recovery | Publisher2.169–2.436 ms; consumer1.005–1.014 s | Без правок; consumer first observed delivery может быть redelivery |
| Rabbit throughput | Serial median≈3983 confirmed commands/s | Без правок; не FFmpeg jobs/s |
| DB pool wait32×5 | 917 waits / 1.455 aggregate waiter-s;166 ms wall | Pool не менялся; peak20/20, final10/0/10,0 errors |
| SQL/recordings20 | 6 (recordings1 тоже6) | 6; batch уже присутствовал до P1 |
| SQL/history | 6 | 6; backend не удалён вместе с меню |
| SQL/personal unread20 | 2 | 2; меньше работы внутри первого SQL, а не round trips |
| DB unread20 p95, fresh fixture | 30.076 ms | 16.464 ms (−45.3%),100 requests; ограничения ниже |
| DB unread20 matched fresh-page bench | 25.74 ms median6 | 14.88 ms (−42.18%,p=.002); Go memory без значимого изменения |
| DB recordings20 p95 | 4.693 ms | 4.007 ms; этот path не менялся, не заявляем gain |
| Gin recordings20 p95 | 3.935 ms | 6.392 ms; unchanged path/host variability, не скрываем разброс |
| Recording temp bytes | 7 986 655 B / 130 files | 7 928 689 B / 130 files; sampled peak, не доказанный gain |
| Composite method finalization | 102.41 ms median6 | 83.70 ms (−18.27%,p=.002) |
| Audio method finalization | 63.95 ms median6 | 52.41 ms (−18.05%,p=.002) |
| End-to-end stop→ready,2 rooms | 411.60 / 410.44 ms | 412.66 / 410.59 ms; ускорение всего pipeline **не доказано** |
| FFmpeg CPU | ≥0.40 CPU s, sampled | ≥0.39 CPU s; lower bounds, общий CPU saving не доказан |
| Disk I/O wait | not measured | Shared Docker VM iowait нельзя отнести к одной записи этим sampler |

### 2. Новая оптимизация: unread filter

**Bottleneck:** correlated COUNT всех непрочитанных сообщений только для ответа «есть ли хотя бы одно». **Evidence:** first SQL EXPLAIN19.339 ms,1019 COUNT subplans,≈48 rows/loop. **Change:** EXISTS с тем же predicate, exact projected/total counts не тронуты. **Risk:** минимальная SQL-замена без schema/API/index/cache изменений; regression сверяет numeric projection, порядок, все cursor pages, clear/read/self/deleted/mute/hidden.

Большой fresh-write fixture:1000groups×20members+19direct,50messages/chat,1019conversations;100requests после5warmups. SQL count2→2, B/op98 740→99 098: экономия Go memory не заявляется. p50 25.387→14.714ms (−42.0%), p95 30.076→16.464ms (−45.3%), p99 32.432→27.127ms (−16.4%). First SQL EXPLAIN19.339→8.306ms; total shared buffer hits58 654→5972 (−89.8%). Planner заменяет COUNT subplan на Hash Semi Join; дополнительных индексов нет.

Visibility map существенно влияет на этот workload. При отдельном VACUUM ANALYZE до timing median6≈11.713→10.520ms (−10.2%), **p=.065: статистически значимое ускорение при95% не подтверждено**; B/op/allocs без значимого изменения. Исходный несогласованный набор с одним существенно более быстрым sample исключён из matched comparison, а не удалён. Изменение visibility map — возможная, но не доказанная причина этого разброса.

Строго matched fresh-page benchmark: autovacuum выключен **только на собственной synthetic table в disposable DB до seeding**; оба варианта читают одинаковые недавно записанные, ещё не all-visible pages. Конкурентных записей в timed loop нет. Исходный COUNT подключён через Go overlay, working tree не откатывался. Чистые6samples по30операций после5warmups: **25.74ms ±13% → 14.88ms ±18%, −42.18%,p=.002**; B/op/allocs статистически без изменения. Предварительный run, пересёкшийся с functional FFmpeg проверкой, отдельно сохранён и повторён. Это обоснование узкой правки для свежих данных, не обещание42% ускорения всех чатов/базы.

### 3. Новая оптимизация: финализация записи

**Bottleneck:** один final.mp4 повторно запускает ffprobe для duration после validateOutput, который уже прочитал и проверил эту duration. **Evidence:** отдельный probe≈16.2ms и62alloc/op; negative control подтверждает два final-file probes. **Change:** reuse локальных validated metadata, без cache/file/codec изменений; audio сохраняет explicit ctx.Err после checksum. **Risk/regression:** одинаковое округление, invalid0/NaN/text отклоняются, legacy Finalize прежний, cancellation test с блокировкой checksum и negative control.

Linux FFmpeg6.1.2, synthetic2s640×36010fps H264/AAC fixture до таймера,6×1s samples, Go allocs только parent (не RSS subprocess). Медиа одинаковое; baseline binary собран до правки, snapshots/overlay сохранены для последующей регрессии и воспроизведения без отката working tree. Benchstat:

| Метод | Median ms до→после | B/op до→после | allocs/op до→после | Значимость времени |
| --- | ---: | ---: | ---: | --- |
| composite | 102.41→83.70 | 293.0→257.1 KiB | 367.5→306 | −18.27%,p=.002,n=6 |
| audio | 63.95→52.41 | 210.9→175.6 KiB | 290.5→228 | −18.05%,p=.002,n=6 |
| duration_probe control | 16.21→16.40 | ≈35.46 KiB, без изменения | 62→62 | p=.240, незначимо |

Это ускорение **метода финализации на коротком fixture**, не18% ускорения конференции или полной записи. End-to-end stop→ready двух pipelines остался≈411ms: таймер включает команды, FFmpeg, upload/DB и polling. Container versions/source hashes и cleanup receipts: `recording/{baseline-isolated,after-isolated}`. Начальный `baseline-current` пересекался с другим workload, считается warmup и не используется.

### 4. Current media baseline — без production-правок

| Сценарий | Forwarded RTP/s | Process CPU s / ~5wall s | MB allocated/s | Mallocs/s | Mutex aggregate |
| --- | ---: | ---: | ---: | ---: | ---: |
| idle | 0 | 0.005523 | 0.000243 | 2.60 | 0.289ms |
| 2 peers | 199.98 | 0.360794 | 1.051 | 7264.95 | 0.486ms |
| 5 peers | 1999.51 | 1.232526 | 6.502 | 46506.28 | 11.79ms |
| 10 peers | 9026.27 | 4.265745 | 27.144 | 227511.45 | 127.87ms |
| 5×2 rooms | 3999.11 | 2.182794 | 12.942 | 92903.68 | 17.59ms |
| 10×2 rooms | 14541.36 | 7.756383 | 45.266 | 593018.90 | 225.66ms |
| 5+screen+taps | 2199.88 | 1.327878 | 7.580 | 52231.04 | 15.47ms |

Matrix PASS184.853s; SFU/record/audio queue drops0, cleanup maps0/G3/FD6. При10×2 offeredideal18k/s не достигнут; энд-ту-энд sequence gaps не измерены. Включены клиенты и SFU, не только server. NACK retained copies нужны, application snapshot allocation0.38%/0.22% не оправдывает unsafe pooling. Screen taps в этой матрице не запускают real decoder/FFmpeg/STT.

Egress microbenchmark6samples: payload160/sinks0/1/2=7.375/134.0/167.25ns; payload1200=7.484/227.7/260.35ns. Безsink0B/0alloc, с1/2sink176B/1alloc или1280B/1alloc. Profiles и raw logs: `media/baseline`, `media/benchmark-egress-final.log`; предварительный concurrent log исключён.

### 5. DB/query matrix текущего функционала

100samples/service; latency ms. Только unread20 code изменился. Различия других строк демонстрируют вариативность общего хоста, не эффект оптимизации. Точные SQL/plans/allocs: `data/{db-baseline,db-after}`.

| Путь | SQL до→после | p50 до→после | p95 до→после | p99 до→после |
| --- | ---: | ---: | ---: | ---: |
| recordings HTTP20 | 6→6 | 3.562→4.682 | 3.935→6.392 | 5.012→9.385 |
| recordings1 | 6→6 | 1.571→1.498 | 2.713→1.873 | 3.044→2.526 |
| recordings20 | 6→6 | 2.878→3.013 | 4.693→4.007 | 7.566→13.270 |
| meetings20 | 1→1 | 0.476→0.490 | 0.535→0.565 | 0.655→0.611 |
| history | 6→6 | 2.050→1.927 | 3.163→3.080 | 4.085→3.299 |
| participants20 | 4→4 | 0.972→0.855 | 1.106→1.054 | 2.033→1.183 |
| conference chat20 | 8→8 | 2.072→2.007 | 2.966→3.416 | 4.016→3.614 |
| notifications20 | 2→2 | 0.771→0.600 | 1.040→0.651 | 1.212→1.673 |
| transcript20 | 5→5 | 1.154→1.089 | 1.477→1.312 | 2.401→2.375 |
| summary | 4→4 | 0.846→0.829 | 1.108→0.922 | 1.994→1.657 |
| analytics | 3→3 | 0.704→0.693 | 0.804→0.770 | 1.971→0.876 |
| admin | 3→3 | 3.986→4.193 | 4.316→5.275 | 5.393→11.979 |
| personal HTTP20 | 3→3 | 12.059→12.032 | 12.574→12.847 | 13.509→13.187 |
| personal1 | 2→2 | 11.814→11.383 | 15.282→13.880 | 23.843→17.654 |
| personal20 | 2→2 | 11.661→11.859 | 13.052→13.553 | 18.615→14.018 |
| personal unread20 | 2→2 | 25.387→14.714 | 30.076→16.464 | 32.432→27.127 |
| group chat1 | 10→10 | 2.941→2.806 | 8.337→4.003 | 18.257→4.224 |
| group chat20 | 10→10 | 3.257→2.949 | 4.486→3.812 | 5.484→4.025 |
| personal read state | 6→6 | 1.567→1.614 | 2.012→1.883 | 2.885→2.823 |
| direct chat20 | 9→9 | 2.057→2.315 | 3.209→3.383 | 3.286→3.768 |
| group members20 | 3→3 | 0.948→1.056 | 1.180→1.882 | 2.408→2.332 |

### 6. Redis, Rabbit и recording resources

Redis control benchmarks повторены6раз на реальных test keys: pipelineMissing500=1batch/500commands; accountOnline1/20/100=1batch/1command. Cold NOSCRIPT может добавить EVAL; client hook не считает Lua-internal calls/PubSub handshake. Сравнение старого sequential Get с уже имеющимся Missing — reference algorithm, не новая production-правка.

Current Missing2/20/500 median0.180/0.187/0.961ms, accountOnline1/20/100=0.179/0.205/0.391ms. Reconciliation20scans/5s на500сессий:42batches/10023commands (включаяsetup),4 634 448 TotalAlloc bytes/51085allocations; synthetic Stale repository не измеряет SQL. CPUprofile10ms samples/5.08s не означает нулевого CPU. Counts/memory — per-window MemStats, не ошибочная разность cumulative profiles разных subtests.

Rabbit baseline3samples serial median251.073µs/op, parallel247.607µs/op, confirms включены; publisher reconnect median2.190ms. Consumer9faults1.005–1.014s. QoS1, connection/channel reuse не менялись; долгий outage/multi-worker herd не измерялся. Повтор после правок записи служит regression, не Rabbit before/after optimization.

Запись: две реальные параллельные pipelines с4→6peers, screen, FFmpeg/MinIO/PG/Rabbit, H264/AAC640×360, около11s каждый итоговый файл. Fixtures имеют отдельные Composer/Service; это не capacity-тест одного worker с default concurrency1. Обе decoded/recovered-closed-segment/auto-stop проверки PASS:44 decoded frames при проверке4fps и≈11.26s mixed audio. Recording drops0. KeepLocal=true удерживает источники до проверки/fixture cleanup.

| Whole-process / sampled resource | До | После |
| --- | ---: | ---: |
| Wall | 16.028s | 16.041s |
| Go parent CPU | 3.2716s | 3.5144s |
| Все waited children CPU | 5.2662s | 5.3033s |
| Parent peak RSS | 370160KiB | 361264KiB |
| Total allocated bytes | 975950320 | 980753952 |
| Heap after GC | 4223624B | 4245440B |
| G before/afterGC | 3/7 | 3/7 |
| Observed peak FFmpeg children | 2 | 2 |
| Observed peak FD / last sample | 154/9 | 150/9 |
| Sampled peak temp bytes/files | 7986655/130 | 7928689/130 |
| Final sample temp bytes/files | 0/0 | 0/0 |
| Parent logical rchar/wchar | 53062496/27366541 | 51827286/27143320 |
| Parent `/proc` write_bytes | 10936708 | 10839811 |
| Child input/output blocks | 0/11172 | 0/11067 |

Один full-stack pair не доказывает экономию CPU/RSS/I/O. Самплер≈0.3s пропускает короткие subprocess: FFmpeg40/39ticks — lower bounds0.40/0.39s, не весь FFmpeg CPU. RUSAGE_CHILDREN включает helpers. read_bytes=0 означает warm cache, не отсутствие чтений; block counters не равны physical-storage traffic всей системы. Disk iowait/полные totals, длительный STT и server-only recording CPU **not measured**.

В baseline alloc profile640MiB/67.67% — Argon2 создания fixture аккаунтов,≈38% sampled Go CPU также Argon2; security hashing не ослаблялся. G3→7 — snapshot инфраструктуры/профилировщика, не plateau/leak diagnosis. Независимый P0 lifecycle:17 starts,14 finalizations,15 cancel/timeouts,6 expected input/disk failures,6 live decoders; все3rounds G2/FD7/children0/temp0. P0 early decoder exit, S3 cleanup и STT input/limit cleanup проверки сохранены и проходят.

Доказательства: `recording/final-focused-race.log`, `p0-final-regression.log`, `p0-regression/`, `recording-{before,after}-*-top.txt`. Full-stack cleanup receipts подтверждают неизменность прежних контейнеров и0 собственных container/network/volume leftovers. Standalone fixture containers были network-none/auto-remove; реальных записей не удалялось.

### 7. Финальные проверки и доказательства

Исходные и окончательные проверки выполнены раздельно, без test cache:

| Gate | Baseline | После правок |
| --- | --- | --- |
| `go test -count=1 ./...` | PASS | PASS |
| `go test -race -count=1 ./...` | PASS | PASS |
| `go vet ./...` | PASS | PASS |
| staticcheck v0.7.0 | PASS | PASS |
| govulncheck v1.8.0 | PASS,0 called/imported findings | PASS,0 called/imported findings |
| frontend `npm run lint` | PASS | PASS |
| gofmt / release shell syntax / diff whitespace | PASS | PASS |

Govulncheck сохраняет одну module-only advisory вне импортируемых/вызываемых пакетов; это не утверждение «все зависимости неуязвимы». Whole tests без внешних env пропускают opt-in нагрузки: ниже отдельно фиксируются их реальные запуски.

Performance matrix, current full DB before/after, real Redis recovery, два full-stack recording runs, Rabbit confirms/reconnect и новые FFmpeg/count semantics regression PASS. Свежие account1020 + conference1020 WS lifecycle, Redis subscription kill и transient-prune recovery PASS (package20.124s). Recording P0 smoke и scoped race PASS, детали выше.

SFU P0 race smoke PASS92.639s:25lifecycle+25source churn,2/3/5peers,stalled audio2rooms×2peers. После leave G2/FD6/maps0, heap≈1.13MB. Stalled tap даёт ожидаемые2overflow при0 SFU/recording drops. Четыре SDK EOF teardown warnings без assertion/race failures. Evidence: `media/regression-race.log`, `media/regression/`.

DB/Redis scoped security race PASS:integration41.396s,Redis1.853s. Проверены unread DTO/query budget/cursor, recording batch1/20, clear/mute/hide, group revoke/projection locks, presenceTTL/leases. Browser/MinIO-only cases этого набора SKIP и не считаются пройденными; real MinIO recording охвачен отдельным full-stack. Evidence: `data/security-regression-race.log`, `ws-regression.log`, `ws-regression-profiles/`.

Cleanup: own PG/Redis удалены после проверки отсутствия fixture DB и Redis keys; recording pods/networks/anonymous volumes удалены с проверкой labels/IDs. Receipt `data/existing-services-after-cleanup.json`:30 ранее существующих контейнеров running, их ID/name/StartedAt неизменны, оба data-audit IDs отсутствуют. Только синтетические данные аудита удалены; их можно воспроизвести harness. Пользовательские сервисы/записи не затронуты.

Gate logs: `baseline-*.log`, `final-*.log` в evidence root. Новые production-правки проверены независимым вторым review: SQL predicate/DTO/cursor equivalent; FFmpeg validated metadata reuse/cancel/legacy behavior безопасны в прежнем контракте.

Новые evidence находятся только в `tmp/p1-audit-20261010/`; никаких remote/deployment операций не выполнялось. Исторические оптимизации Redis batch, recordings batch/index и forced Flush ниже не пересчитываются в достижения этого прохода.

---

## Исторические измерения — 6 октября 2026 (не текущий baseline)

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
