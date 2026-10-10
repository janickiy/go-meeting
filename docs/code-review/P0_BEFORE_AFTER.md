# P0: доказательства и измерения до/после

## Актуальные измерения — 10 октября 2026

База: `a0781257debba3f911ee180067cc0fad5bb06204`; исходный Go1.26.6, финальный Go1.26.9. [Описание аудита и владельцев ресурсов](P0_RELIABILITY_AUDIT.md). Все новые evidence находятся в `tmp/p0-audit-20261010/`, сохраняются локально и не включены в Git. Раздел от6 октября внизу — **исторический**, его числа не переиспользованы как свежие.

### 1. Каждое исправление: evidence → причина → результат

| Issue / severity | Before / evidence | Root cause / minimal fix | After / regression | Риск |
| --- | --- | --- | --- | --- |
| P0-20261010-01 LiveAudio / High | child exit0/23, RTP channel открыт и пуст; оба вызова Decode висят >3s; stack показывает packet select и cmd.watchCtx | EOF завершал только reader, select не знал об этом. Добавлен outputDone broadcast; существующие Close/join/Wait остаются у owner | race×10:20 дочерних процессов, Decode72.187375–259.958583ms, PID reaped; exit0 успех,23 ошибка | Только ранний EOF; PCM callback сохраняет обязанность соблюдать context |
| P0-20261010-02 attachment cleanup / High |12 AccessDenied cleanup +cancel/GC: G4→16, SDK0→12; pprof ListObjects.func1/chansend | early return оставлял channel producer; ListObjectsIter + конечный ctx.Err | Те же12 failures: G4→4, SDK0→0; success/precancel + S3/chat race×3 PASS | Partial deletion остаётся ошибкой, prefix/keep не изменены |
| Security dependencies / отдельная группа | govulncheck11 symbol advisories, включая известные HTTP2 crash/CPU/memory DoS | Go patch1.26.9 и x/net0.60 + обязательные минимумы; одинаковые версии CI/сборки | scanner0 symbol/0 package; whole tests/race/build PASS | Все11 не объявляются воспроизведёнными application exploits; module-only OpenPGP advisory не используется |

Before production functions воспроизведены с теми же новыми регрессиями через Go overlay; рабочие файлы не откатывались. Early-exit лог: `recording/live-early-exit-go1.26.9-before.log` / `...-after.log`. Storage: `recording/attachment-cleanup-before.log` / `...-after.log`, `recording/storage-before/` / `storage-after/`, `attachment-*-growth.txt`. Независимый review обеих правок не выявил блокирующих замечаний.

### 2. WebSocket: одинаковые workload до и после dependency patch

Реальные отдельные PostgreSQL/Redis, два API/Hub, реальные tickets и HTTP Upgrade.20 warmup +4×250 cycles, cleanup, cooldown300ms и2 GC на checkpoint. Memory/RSS ниже в **байтах**. Это разные test processes, а не production baseline. В WS production-код не менялся: сравнение проверяет отсутствие регрессии, а не доказывает ускорение.

| Workload / phase | Goroutines до / после | Heap после GC до / после | FD до / после | RSS до / после |
| --- | ---: | ---: | ---: | ---: |
| Conference idle |14 /14|3220184 /3315392|11 /11|240877568 /177143808|
| Conference warm |14 /14|3467384 /3511136|14 /13|242384896 /177389568|
| Conference250 |14 /14|3571000 /3608360|14 /14|243548160 /177684480|
| Conference500 |14 /14|3606608 /3623832|14 /14|243924992 /177930240|
| Conference750 |14 /14|3636992 /3635000|14 /14|244039680 /177930240|
| Conference1000 |14 /14|3642768 /3646816|14 /14|244154368 /177979392|
| Account idle |16 /16|3150664 /3148576|13 /13|168411136 /168116224|
| Account warm |22 /22|3291160 /3306440|18 /18|170786816 /170754048|
| Account250 |22 /22|3495272 /3439216|20 /19|171917312 /172032000|
| Account500 |22 /22|3523232 /3474952|20 /19|172392448 /172441600|
| Account750 |22 /22|3576112 /3496376|20 /19|172589056 /172589056|
| Account1000 |22 /22|3605648 /3570104|20 /20|172736512 /172703744|

В каждом conference checkpoint: localWS0, connected SQL ParticipantSessions0, Redis namespace keys0. В каждом account checkpoint отдельно проверены handler reservations0, physical leases0, PubSub subscribers0. Поле `localWebSockets` общей snapshot helper относится к conference Hub; оно **не подменяет** отдельные assertions global account handler.

Account warmup вводит6 HTTP keepalive goroutines двух тестовых серверов/клиента; они не принадлежат завершённым WS. FD pool доходит до20, не растёт на каждый socket. Небольшой монотонный рост heap в первых4раундах сам по себе не доказывает ни leak, ни плато; поэтому дополнительно выполнена длинная серия с более точным sampling — ниже. Большая разница RSS двух conference processes не заявляется экономией памяти от патча Go.

Evidence: `realtime/profiles/`, `realtime/profiles-post/`, `realtime/findings.md`; первоначальные и финальные non-race stress logs сохранены отдельно. Warm→end goroutine diffs0; default heap sampling512KiB слишком груб для объяснения нескольких сотенKiB drift.1s CPU profiles0 samples означают только отсутствие наблюдаемого busy loop в этом окне.

### 3. Дополнительная проверка плато WS

Чтобы не объявлять первые4точки плато без доказательства, opt-in tests через временный overlay расширены до12×250 циклов в одном процессе, без изменения production или tracked test sources. `GODEBUG=memprofilerate=16384` повысил точность профиля. Первые20 циклов — прогрев.

Conference поздние раунды7…12: heap3712712 /3724560 /3721872 /3726880 /3730704 /3729808 bytes; G14/FD14, все session/WS/Redis registries0. Это колебания около устойчивого уровня, не линейная память на каждый session.

Account в первой длинной серии rounds7/8/9: heap3658320 /3655680 /3654632 bytes, G22/FD20, leases0; sampled retained delta round7→9 −32KiB. Профиль объясняет ранний рост в основном pgx connection/statement-cache (~98KiB), Redis pooled readers (~37KiB), runtime thread/stack (~49KiB), далее небольшими HTTP/runtime allocations. **Две account серии на macOS не завершились:** обе в round10/cycle225, после2495 успешных циклов с прогревом, получили TCP dial timeout к ephemeral localhost API listener до WebSocket handshake. Ticket POST до этого успешен, FD/leases/goroutines стабильны. Красные логи сохранены; это воспроизводимое ограничение длинного локального прогона, причина не установлена. Они не выдаются за успешные3000 циклов или за доказанную production утечку.

Тот же account test/overlay на **Linux arm64:3020 циклов PASS,8.80s**. Во всех поздних checkpoints G22, FD23 (сround3), account reservations/leases/subscribers/Redis keys0. Heap rounds7…12:3461936 /3460032 /3463648 /3495632 /3503712 /3515680 bytes; небольшой long-tail drift не называется идеально flat heap. RSS **not measured** (snapshot `-1`): минимальный Linux image не поддерживает используемый вариант `ps`. Конференционный macOS long-run и Linux account run не смешиваются в одну memory series. Evidence: `realtime/ws-long.log`, `realtime/ws-long-user-repeat.log`, `realtime/ws-linux-user-long.log`, `realtime/profiles-linux/` и precise pprof diffs. macOS-only timeout остаётся открытой диагностической границей, его причина не приписывается ядру без доказательства.

Linux sampled heap round7→12: net+32.65KiB (`runtime.malg`+48.71KiB, HTTP persistConn+16.05KiB, `UserHandler.run`−32.11KiB). Число goroutines неизменно, idle CPU1s profile0 samples. Не выявлен рост удерживаемой популяции WS; это не обещание zero heap drift навсегда. См. `realtime/linux-user-tail-heap-diff.txt`, `linux-user-goroutine-diff.txt`, `linux-user-idle-cpu.txt`.

### 4. SFU / camera / microphone / screen

100 real Pion/UDP lifecycle с чередованием2/3/5 participants, публикацией/получением audio+video; reconnect с новым peer каждый10-й цикл. Отдельно100 source churn с mic/camera retire/republish и screen AddTrack/RemoveTrack. Production SFU не менялся.

| Метрика | До, Go1.26.6 | После, Go1.26.9 |
| --- | ---: | ---: |
| Idle G / FD |2 /6|2 /6|
| Post100 G / FD |2 /6|2 /6|
| Post100 heap после GC |1149848|1149664|
| Post100 RSS |40386560|39895040|
| Rooms / MediaPeers / tracks / subscriptions после leave |0 /0 /0 /0|0 /0 /0 /0|
| Churn active G / FD |120 /26|120 /26|
| Churn steady25s heap |10908976|10981480|
| Churn steady30s heap |10915440|10968648|
| Churn after-leave G / FD |2 /6|2 /6|
| Churn after-leave heap / RSS |963952 /52805632|931272 /51281920|
| Idle-after-load CPU seconds / wall seconds |0.002638 /2.027819|0.002306 /2.033615|

Во всех пустых checkpoints manager.connections=0 и roomClosures=0. Heap активной комнаты растёт во время заполнения bounded NACK buffers, затем выходит на плато; после leave падает. RSS сохраняет страницы runtime и не означает retained peers. FD SFU измеряется с одинаковой временной pipe `lsof` observer. Raw FD между разными harness не складываются.

Final lifecycle52.52s/churn50.18s PASS; race SFU/legacy/media-worker/FFmpeg PASS. Goroutine diff0, sampled heap runtime/test-observer без подтверждённых retained SFU nodes, короткий idle CPU profile0 samples. Mutex/block profile — накопленные concurrent waits за нагрузку, не latency запроса и не доказательство deadlock. Evidence: `media/review.md`, `media/lifecycle/`, `media/source-churn/`, `media/final/` и root `media-*.log`.

### 5. FFmpeg и temp resources

Изолированный Linux process/container, реальный FFmpeg6.1.2, deterministic lavfi workload, текущая Go1.26.9 test binary.2 GC +100ms cooldown; FD/RSS/children через Linux `/proc`.17 segment starts,14 success finalize,15 cancel/timeout,6 malformed/disk-full,6 live cancel. Это component lifecycle, не100 full-stack conferences.

| Phase | Goroutines | Heap bytes после GC | RSS bytes | FD | FFmpeg children | Temp files / bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Warm |2|1436976|19795968|7|0|0 /0|
| Round1 |2|1457240|17551360|7|0|0 /0|
| Round2 |2|1473096|18235392|7|0|0 /0|
| Round3 |2|1493976|18247680|7|0|0 /0|

Исходный Go1.26.6: warm G2/FD7/heap1434008/RSS17907712; round3 G2/FD7/heap1488152/RSS17190912; children/temp0. Обе серии PASS, исходный process lifecycle был устойчив на этих сценариях; отдельно воспроизведённая ошибка тихого live EOF проверяется новым helper выше.

Final warm→round3 heap +57000 bytes: короткий профилируемый прогон не доказывает абсолютно flat heap; sampled diff указывает runtime thread/timer machinery, не retained recording objects. Goroutine diff0. Отдельные FFmpeg idle CPU/mutex/block profiles **not measured**: есть heap/allocs/goroutine profiles и процессные счётчики, CPU full-stack не заменяет idleCPU.

Evidence: `recording/real-ffmpeg.log`, `recording/real-ffmpeg-go1.26.9.log`, `recording/go1.26.9/`, `recording/*growth-go1.26.9.txt`. Успешные и принадлежащие тесту отменённые temp artifacts очищены. Failed durable chunks пользователя намеренно могут сохраняться для recovery — этот тест не даёт права их удалять.

### 6. Полная запись / broker / storage

`TestStageFourCompositeRecording` PASS15.57s; `TestRecordingRetentionObjectStorage` с versioning PASS0.23s. Реальный путь: temporary PG17/Redis8/Rabbit4.2.9/MinIO, SFU/Pion, host FFmpeg9.0.2.3 participants + screen layouts; H264/AAC640×360, ffprobe11.119479s,1008157bytes,2artifacts.44 video frames и11.25s mixed audio декодированы. Finalize485.758833ms; peakFFmpeg1; drops0; lease expiry2s; finish→auto-stop2.421s; egress409 изолирован от встречи.

Combined CPU1.519316s, peakRSS178126848bytes (macOS units). Это одна acceptance-запись, не throughput benchmark. Repeated100 full-stack recordings **not measured** — повторная процессная нагрузка описана отдельно.

Rabbit real-broker race PASS7.577s:3 reconnect ≈1.02/1.01/1.01s, blocked publish cancel≈0.10s, RPC≈0.11s, Close≈1.01s, drain/quarantine/handshake cancellation. Новых дублированных consumer/накопления не наблюдалось. S3 deletion fault повторяется12раз, before G4→16/heap883480→959024; after G4→4/heap883480→863712. Fault-only RSS/FD **not measured**, этот fixture оценивает SDK goroutine ownership.

### 7. Сводка обязательных метрик и ограничений

| Metric | Before | After |
| --- | ---: | ---: |
| Idle goroutines (conference WS / account WS / media) |14 /16 /2|14 /16 /2|
| Post-WS goroutines (conference / account,1000) |14 /22|14 /22|
| Post-media goroutines (100) |2|2|
| Heap after GC (conference1000 / media100) |3642768 /1149848|3646816 /1149664|
| Open FD (conference1000 / account1000 / media100) |14 /20 /6|14 /20 /6|
| Active Rooms / MediaPeers после leave |0 /0|0 /0|
| Active WS / connected ParticipantSessions |0 /0|0 /0|
| FFmpeg after component stop |0|0|
| Temp after component cleanup, files/bytes |0 /0|0 /0|
| SDK goroutines retained после12attachmentfailures |12|0|
| LiveAudio helper after child exit |Decode stuck>3s|Decode<260ms, PID reaped|
| Race detector, whole repo |PASS|PASS|

Production RSS/heap/FD/rooms/sessions/FFmpeg/temp **not measured under workload**: аудит не нагружал рабочую установку. Browser devices, TURN/WAN, многодневный soak и maximum capacity **not measured**. Нельзя переносить local loopback результаты на эти условия или складывать метрики разных процессов.

### 8. Повторяемые команды и файлы

Из корня репозитория, с новыми изолированными PostgreSQL/Redis и test-only connection variables из существующих integration fixtures:

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go test -race ./internal/infrastructure/ffmpeg -run '^TestLiveAudioReapsEarlyExitWithoutMorePackets$' -count=10
go test -race ./internal/infrastructure/storage/s3 ./internal/usecase/chat -count=3
RECORDER_P0_MEDIA_CYCLES=100 RECORDER_P0_PROFILE_DIR="$PWD/tmp/p0-repeat/media" go test -v ./internal/infrastructure/sfu -run '^TestP0Media(Lifecycle|SourceChurn)$' -count=1 -timeout=10m
RECORDER_P0_WS_STRESS=true RECORDER_P0_PROFILE_DIR="$PWD/tmp/p0-repeat/ws" go test -v ./tests/integration -run '^TestP0(UserWSRepeatedLifecycle|WSRepeatedLifecycle)$' -count=1 -timeout=10m
RECORDER_P0_FFMPEG_STRESS=true RECORDER_P0_EVIDENCE_DIR="$PWD/tmp/p0-repeat/ffmpeg" go test -v ./internal/infrastructure/ffmpeg -run '^TestP0RealFFmpegResourceCycles$' -count=1 -timeout=10m
```

Реальные имена test connection env и дополнительные integration prerequisites находятся в `tests/integration/realtime_test.go`, `tests/integration/composite_recording_test.go`, `tests/integration/recording_retention_test.go` и scope evidence reports. FFmpeg resource workload рассчитан на Linux `/proc`, запускается в отдельном контейнере с реальным FFmpeg. Без настройки test dependencies opt-in workload должен skip/fail, а не обращаться к рабочим данным. Для longer WS сохраняются overlay и log в `realtime/`; baseline и finalprofiles не перезаписываются.

Все финальные стандартные gates PASS: `final-test.log`, `final-race.log`, `final-vet.log`, `final-staticcheck.log`, `final-frontend-lint.log`, `final-govulncheck.log`, `final-mod-verify.log`, `final-build*.log`. Reconnect fixture correction не скрывает data race: первоначальный отказ с presence_timeout сохранён и объяснён в audit §10. Финальный diff содержит только две production lifecycle правки, тесты, security patch/build consistency и эти отчёты; ранее существовавшие user edits не включены в выводы.

---

## Исторические измерения — 6 октября 2026 (не свежий прогон)

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
