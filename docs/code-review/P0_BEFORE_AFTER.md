# P0: доказательства и измерения до/после

База: `afc33ee31f5140aad6b25059aa7aa0f9b4e511ec`; 6 октября 2026. Контекст и описание исправлений: [P0_RELIABILITY_AUDIT.md](P0_RELIABILITY_AUDIT.md). Evidence root: `tmp/p0-reliability-20261006/evidence/` (локальные, не Git-tracked артефакты). Ниже числа относятся к конкретному workload/process, а не к общей production установке.

## 1. Сбои и результат минимальных исправлений

| Issue | Evidence / before | Root cause / fix | After / regression | Риск |
| --- | --- | --- | --- | --- |
| P0-01 Redis recovery | После разрыва PubSub и отдельно единичного Prune error Redis здоров, новые WS стабильно 409 все 3 s наблюдения | Permanent closing заменён временной недоступностью, один retry owner, generation fence, readiness Hub | Оба real Redis regression PASS, `-race ×3`; shutdown vs resubscribe и stale Prune unit tests PASS | Текущие sockets закрываются; клиенты восстанавливают canonical state |
| P0-02 MinIO bulk cleanup | 12 failed deletions → +24 SDK goroutines; pprof `chansend` в remove producer/converter | Consumer прекращал читать channel; synchronous iterators + cancel, listing error propagation | Те же 12 failures → +0; race×3, success/cancel/GET403 PASS | Partial deletion остаётся ошибкой и может быть повторена |
| P0-03 MinIO retention | 12 failed version deletions → +12 listing goroutines после cancel | Финальная отправка SDK в заполненный channel; ListObjectsIter | 12 failures → +0; safe UUID/bucket checks PASS | Сохраняются ограничения namespace и version deletion |
| P0-04 Legacy Stop/offer/timer | После Stop late offer принят, создан незарегистрированный PC; cancel оставляет wait timer | terminal fence, PC identity/context checks; согласование Start/Stop/Shutdown | Late offer отвергается, timer отсутствует; PID/Wait tests PASS | Grace 2–8 s сохранён, новые offers запрещены |
| P0-05 Unexpected FFmpeg exit | Process code23 после startup, Active=1, PC new, ctx жив | broadcast completion и session monitor/fail cleanup | Active=0, PC Closed, ctx canceled; race PASS | Неожиданный exit корректно завершает запись ошибкой |
| P0-06 Callback lock | На исходном commit заблокированный OnStarted удерживает mutex, Stop ждёт внешнего release | Callback вне mutex, session context +5 s timeout | Stop/ctx/callback goroutines завершаются; race PASS | Ошибки DB callback логируются, медиа очищается |
| P0-07 Rabbit blocked write | Cancel не освобождает write, Close ждёт mutex | Cancellation→CloseDeadline, непригодное соединение уничтожается, bounded AMQP RPC | Publish/RPC cancel ≈0.11 s, Close ≈1.01 s; real broker race PASS | При неопределённом confirm возможна повторная доставка |
| P0-08 Prepare DB failure | AddEvent error после Prepare: Active=1 | ingest.Stop rollback перед возвратом ошибки | Active=0 после обеих повторных попыток; race PASS | Retry заново подготавливает session |

## 2. WebSocket: идентичный workload до и после

Два Hub/API, реальные PostgreSQL/Redis, signed auth, real HTTP Upgrade. Idle → 20 warmup → четыре раунда по 250 connect/state/disconnect. В каждом snapshot: cleanup invariants, 300 ms cooldown, два GC. Нагрузки запускались отдельными тестовыми процессами. Heap/RSS — байты.

| Phase | Goroutines до / после | Heap после GC до / после | FD до / после | RSS до / после |
| --- | ---: | ---: | ---: | ---: |
| Idle | 14 / 14 | 2976056 / 2970984 | 11 / 11 | 168148992 / 167198720 |
| Warmup | 14 / 14 | 3344080 / 3204592 | 15 / 13 | 170917888 / 169738240 |
| 250 | 14 / 14 | 3446848 / 3353680 | 15 / 14 | 172228608 / 82788352 |
| 500 | 14 / 14 | 3464880 / 3397560 | 15 / 14 | 172720128 / 65241088 |
| 750 | 14 / 14 | 3491616 / 3426856 | 15 / 14 | 172998656 / 65814528 |
| 1000 | 14 / 14 | 3492832 / 3421624 | 15 / 14 | 173064192 / 66306048 |

Во **всех** фазах до/после: Local WS=0, Redis namespace keys=0, connected ParticipantSessions=0. Исторические disconnected SQL rows сохраняются намеренно. After heap 750→1000 уменьшается; FD после прогрева постоянны. RSS изменяется из-за reclamation OS/Go; это не доказательство ускорения или экономии памяти исправлением.

Warmup→1000 heap: +148752 bytes до, +217032 после. Sampling pprof не показывает положительных retained-space nodes или новых goroutine stacks; это не точный per-object подсчёт. Idle CPU за 1.01 s — 0 samples, ниже разрешения измерения. Наблюдаемого hot busy loop нет. Aggregate sampled mutex delay 8.07 ms, в основном runtime/GC; block profile в основном select/channel waits.

Evidence: `ws/before/metrics.json`, `ws/after/metrics.json`, `ws/*growth`/profile text outputs, `recovery-race.log`. Обычный lifecycle был устойчив и до исправления; исправление устраняет **аварийный необратимый отказ**, а не обещает ускорение обычного пути.

## 3. Современный SFU: неизменённый production-код

Это baseline → repeated workload, не сравнение двух реализаций: подтверждённого P0 в современном SFU нет, код не менялся. 3 warmup комнаты с 2/3/5 peers; 100 комнат с чередованием 2/3/5 peers, audio+video publication/receive; 10 reconnect с новым media peer. 342 PC workload + 10 warmup = 352. 9770 forwarded RTP packets, 0 dropped, 0 failed PCs.

| Phase | Goroutines | Heap bytes после GC | RSS bytes | FD | Rooms / Peers / Tracks / Subscriptions |
| --- | ---: | ---: | ---: | ---: | --- |
| Idle | 2 | 344080 | 14270464 | 6 | 0 / 0 / 0 / 0 |
| Warmup | 2 | 736792 | 32145408 | 6 | 0 / 0 / 0 / 0 |
| 25 циклов | 2 | 794520 | 35176448 | 6 | 0 / 0 / 0 / 0 |
| 50 | 2 | 778952 | 35520512 | 6 | 0 / 0 / 0 / 0 |
| 75 | 2 | 812968 | 35651584 | 6 | 0 / 0 / 0 / 0 |
| 100 | 2 | 790296 | 36814848 | 6 | 0 / 0 / 0 / 0 |
| Shutdown | 2 | 790392 | 37666816 | 6 | 0 / 0 / 0 / 0 |

Все checkpoints также имеют connections=0, roomClosures=0. Heap после прогрева колеблется, монотонного роста нет; goroutine diff = 0. Idle CPU 0.002219 CPU s за 2.019151 wall s; pprof без samples. Aggregate mutex delay 212.80 ms за всю нагрузку, не latency запроса.

### Camera/mic/screen churn

Два peer остаются в комнате, 100 retire/republish camera/mic и AddTrack→publish/receive→RemoveTrack screen. После последнего restart обычное аудио/видео продолжается до steady +25/+30 s для заполнения bounded 1024-packet NACK buffer.

| Phase | Goroutines | Heap bytes после GC | RSS bytes | FD | Rooms / Peers / Tracks / Subscriptions |
| --- | ---: | ---: | ---: | ---: | --- |
| Idle | 2 | 335456 | 14139392 | 6 | 0 / 0 / 0 / 0 |
| 25, active | 116 | 3243984 | 28573696 | 22 | 1 / 2 / 4 / 4 |
| 50, active | 116 | 4936472 | 32751616 | 22 | 1 / 2 / 4 / 4 |
| 75, active | 116 | 6598936 | 36470784 | 22 | 1 / 2 / 4 / 4 |
| 100, active | 116 | 8261376 | 39976960 | 22 | 1 / 2 / 4 / 4 |
| Steady +25 s | 116 | 10505592 | 44728320 | 22 | 1 / 2 / 4 / 4 |
| Steady +30 s | 116 | 10494232 | 44105728 | 22 | 1 / 2 / 4 / 4 |
| Leave | 2 | 669408 | 50135040 | 6 | 0 / 0 / 0 / 0 |

Heap выходит на плато; steady25→30 падает на 11360 bytes. Pprof sampled heap diff: −64.04 KiB Pion RTP buffer allocations. После leave heap <0.67 MB, хотя RSS≈50 MB. Это пример различия allocator high-water RSS и удерживаемых ресурсов.

Дополнительный `-race`: 25 lifecycle + 25 churn, PASS за 48.810 s. Evidence: `sfu/stress.log`, `churn-steady-final.log`, `race.log`, `final/`, `steady-final/source-churn/`.

## 4. Ошибки MinIO: отдельный процесс

Реальный SDK v7.0.95 и httptest S3 XML responses, по 8 объектов в listing/error response, 12 повторов каждого метода. Каждый ctx отменяется, выполняются GC и ожидание до 1 s оставшихся SDK goroutines. Нет MinIO daemon: эмулируется только S3 endpoint, producer/consumer SDK настоящие.

| Метод | До: goroutines baseline→после failures | После: baseline→после failures | Удержанные SDK goroutines до / после |
| --- | ---: | ---: | ---: |
| RemovePrefix | 4 → 28 | 4 → 4 | +24 / 0 |
| RemoveRecording | 28 → 40 | 4 → 4 | +12 / 0 |

Первый failing subtest намеренно оставляет SDK goroutines до завершения тестового процесса, поэтому второй starts с28. Без этого пояснения нельзя считать все36 утечек результатом одного retention метода.

Heap snapshots для прозрачности: prefix до 755144→967200, после 709000→735752; recording до 1014336→1046760, после 742104→985976 bytes. Это очень короткие тесты, profile recording само аллоцирует память, прогрев HTTP различается; вывод об исправлении основан на точных SDK goroutine counts и исчезновении блокированных стеков, не на сравнении этих heap дельт. RSS/FD данного fault-only fixture — **not measured**, они измеряются в SFU/WS/FFmpeg workloads отдельно.

Pprof diffs: `storage/prefix-before-growth.txt` (+24), `recording-before-growth.txt` (+12), обе соответствующие after growth =0. Проверены успешное удаление, pre-canceled request, GET403 listing error и небезопасные UUID/bucket namespaces. `go test -race -count=3 ./internal/infrastructure/storage/s3` PASS.

## 5. Как повторить

Использовать тестовые PG/Redis и отдельные namespaces. Не передавать production DSN. `RECORDER_STAGE1_TEST_POSTGRES_DSN` и `RECORDER_STAGE2_TEST_REDIS_ADDR` должны быть заданы в окружении; fixtures создают/удаляют собственные временные БД. Redis disconnect test выбирает только собственный ClientName/client ID.

```sh
go test ./tests/integration -run '^TestP0WSRecovers' -count=1 -v
go test -race ./tests/integration -run '^TestP0WSRecovers' -count=3 -v
RECORDER_P0_WS_STRESS=true RECORDER_P0_PROFILE_DIR="$PWD/tmp/p0-repeat/ws" \
  go test ./tests/integration -run '^TestP0WSRepeatedLifecycle$' -count=1 -v
RECORDER_P0_MEDIA_CYCLES=100 RECORDER_P0_PROFILE_DIR="$PWD/tmp/p0-repeat/sfu" \
  go test ./internal/infrastructure/sfu -run '^TestP0MediaLifecycle$' -count=1 -v -timeout=10m
RECORDER_P0_MEDIA_CYCLES=100 RECORDER_P0_PROFILE_DIR="$PWD/tmp/p0-repeat/churn" \
  go test ./internal/infrastructure/sfu -run '^TestP0MediaSourceChurn$' -count=1 -v -timeout=5m -memprofilerate=65536
RECORDER_P0_MEDIA_CYCLES=25 \
  go test -race ./internal/infrastructure/sfu -run '^TestP0Media(Lifecycle|SourceChurn)$' -count=1 -timeout=5m
P0_STORAGE_PROFILE_DIR="$PWD/tmp/p0-repeat/storage" \
  go test ./internal/infrastructure/storage/s3 -count=1 -v
go test -race ./internal/infrastructure/storage/s3 ./internal/operations -count=1
go tool pprof -top -base warm-heap.pprof round-4-heap.pprof
go tool pprof -top idle-after-load-cpu.pprof
```

Длинные SFU/WS workloads opt-in и не замедляют обычный `go test`. При интерпретации pprof heap учитывать sampling: ReadMemStats даёт точный общий heap, sampled stacks объясняют владельцев, но не точный размер каждого объекта.

## 6. Реальный FFmpeg после исправления

Тот же отдельный Linux arm64 process после warmup и трёх rounds. ffmpeg 6.1.2, disposable network-none worker container. До исправления этот численный workload **not measured**: исходный recorder failure подтверждён отдельным failing lifecycle reproducer, таблица ниже проверяет отсутствие накопления после правки. Raw evidence: `recording/real-ffmpeg.txt` и `recording-*.pprof`.

| Phase | Goroutines | Heap после GC, bytes | RSS, bytes | FD | Child processes | Temp files / bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Warmup | 2 | 1431512 | 16396288 | 7 | 0 | 0 / 0 |
| Round 1 | 2 | 1462048 | 15966208 | 7 | 0 | 0 / 0 |
| Round 2 | 2 | 1472240 | 15319040 | 7 | 0 | 0 / 0 |
| Round 3 | 2 | 1477792 | 15499264 | 7 | 0 | 0 / 0 |

Счётчики: segment starts17, successful finalize14, cancel/timeout15, malformed/disk-full6, live-decoder cancellation6. Finalize проверяет непустые final video/preview; завершение проверяет ProcessState после Wait. В тесте процессы настоящие, а synthetic lavfi source заменяет RTP input для управляемой длительности. Cleanup каталога fixture не означает удаление recoverable artifacts при всех production ошибках.

До/после legacy failure: late offer `new_peer_created=true` → `false`; unexpected exit Manager.Active1 →0; отменённый track timer retained→absent; callback lock удержан→свободен для Stop. Подробности воспроизводимых failures: `legacy-before.txt`, `callback-before.txt`, `legacy-after.txt`. Prepared slot после AddEvent failure1 →0, включая повтор: `prepare-before.txt`, `prepare-after.txt`.

Дополнительно: 10 existing composite tests с настоящим FFmpeg PASS (`real-composite.txt`), включая sparse-video и resolution-change; real transcription extraction/cleanup PASS. RabbitMQ existing command roundtrip/reconnect/quarantine/drain/correlation и новые blocked-write/RPC/cancel/Close tests PASS под race (`rabbit-after.txt`).

Один full-stack recording acceptance с реальным MinIO upload и проверкой состояний выполнен дополнительно (см. раздел7). Длительный repeated full-stack recording soak и browser e2e — **not measured**; повторная нагрузка17 starts относится к компонентному процессному тесту. Idle CPU/mutex/block FFmpeg — **not measured**; heap/allocs/goroutine profiles сохранены, SFU/WS CPU/mutex/block измерены отдельно.

Повтор FFmpeg на Linux с установленными ffmpeg/ffprobe:

```sh
RECORDER_P0_FFMPEG_STRESS=true RECORDER_P0_EVIDENCE_DIR="$PWD/tmp/p0-repeat/recording" \
  go test ./internal/infrastructure/ffmpeg -run '^TestP0RealFFmpegResourceCycles$' -count=1 -v -timeout=5m
go test ./internal/infrastructure/composite -count=1 -v
go test -race ./internal/infrastructure/webrtc ./internal/usecase/recorder -count=1
```

Для безопасного контейнерного повтора собрать `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c` нужного пакета в отдельный output, затем запустить test binary в disposable worker image с `--network none`, смонтировав только binary/evidence. Не запускать эту нагрузку внутри действующего worker процесса.

## 7. Сквозная запись и настоящее версионированное хранилище

Последняя проверка без новых source changes: существующий `TestStageFourCompositeRecording` в disposable Linux pod, API→PostgreSQL→RabbitMQ→SFU→FFmpeg→MinIO. Redis7/PG17/Rabbit4.2.9/MinIO выделены только для этого прогона; internal-only network, host ports отсутствуют. Обычные сервисы не перезапускались. Тест PASS за15.38s.

Проверены three-participant/screen lifecycle, сохранение и декодирование записи, expired lease recovery, намеренный egress409 failure isolation, auto-stop по завершению конференции. Основной результат:

| Показатель | Значение после исправлений |
| --- | ---: |
| Запись | H264/AAC, 640×360 |
| Длительность по ffprobe | 11.118333 s |
| Размер final.mp4 | 993533 bytes |
| Finalization | 371.241125 ms |
| MinIO artifacts | 2 |
| Валидировано декодированных видео-кадров | 44 при4fps |
| Валидировано mixed audio | 11.26 s |
| Recording drops | 0 |
| Peak direct FFmpeg child count | 1 |
| Combined test-process CPU | 2.264094 s |
| Combined peak RSS | 169340 Linux Rusage units (KiB) |
| Восстановленный expired lease segment | 2 s |
| Conference-finish auto-stop recording | 2.428 s |

Combined CPU/RSS включает тестовый SFU/recorder/clients и не является production capacity benchmark или before/after оптимизацией. Для этого acceptance baseline before fix — **not measured**; числа характеризуют финальную regression validation. Исходные ошибки отдельно воспроизведены до правок.

`TestRecordingRetentionObjectStorage`, `RECORDER_RETENTION_S3_VERSIONING=true`: PASS0.13s, проверены удаление старых версий и сохранение соседних объектов. Evidence: `recording/isolated-full-stack-recording.txt`, `isolated-e2e-setup.json`, `isolated-e2e-cleanup.json`. Экспортированные тестовые media находятся в `recording/stage4-artifacts/`.

Cleanup: own container leftovers=0, own network leftovers=0, pre-existing containers unchanged=true. Контейнеры удалены с anonymous volumes; WS audit Redis также удалён после своей нагрузки. Итоговые global tests/race/vet и остальные gates см. раздел13 основного отчёта; staticcheck сохраняет только исходный S1024.

Повтор в изолированной среде с установленными ffmpeg/ffprobe и loopback адресами тестовых зависимостей:

```sh
RECORDER_STAGE4_RECORDING_E2E=true RECORDER_TEST_FFMPEG=ffmpeg \
  go test ./tests/integration -run '^TestStageFourCompositeRecording$' -count=1 -v -timeout=5m
RECORDER_RETENTION_S3_VERSIONING=true \
  go test ./tests/integration -run '^TestRecordingRetentionObjectStorage$' -count=1 -v
```

Дополнительно задать `RECORDER_STAGE4_TEST_MINIO_ENDPOINT`, `RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY`, `RECORDER_STAGE4_TEST_MINIO_SECRET_KEY`, `RECORDER_STAGE4_TEST_RABBIT_URL` и отдельные `RECORDER_RETENTION_S3_*` значения вместе с тестовыми PG/Redis env из раздела5. Никогда не использовать production credentials или DSN для этих нагрузок.
