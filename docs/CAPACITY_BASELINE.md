# Локальный baseline производительности — этап 6

Измерения: 1–2 октября 2026, Europe/Moscow. Это проверка работоспособности и
ресурсов на машине разработки, **не production capacity и не SLA**.

## Среда и воспроизводимость

- Хост: Apple M5 Pro, ARM64, 18 CPU cores, 48 ГиБ RAM (51539607552 bytes).
- Docker Desktop: 10 vCPU, 16748032000 bytes RAM (~15.6 ГиБ), Linux ARM64.
- Исходный commit: `b4409b4bb2b31337782943f43c1aa150d38095b8` + незакоммиченные
  изменения этапа 6 и предшествующие русские комментарии. Результаты не привязаны
  к чистому immutable release artifact.
- Go 1.26.6 после согласованного обновления; первый host soak выполнен на 1.26.5
  до финального dependency patch. Итоговый Linux soak отдельно указан ниже.
- Изолированный Compose project `recorder-stage6`, отдельные PG/Redis/Rabbit/MinIO
  volumes, test-only credentials, SFU mux 50020. Обычный dev project оставался
  запущен; фоновые процессы и ресурсы машины не изолированы для benchmark.
- PG 16, Redis 7, RabbitMQ 4.2.9, Coturn 4.18.0-r0. MinIO — существовавший local
  image, не новый production security-audited release. API/media/recorder собраны
  Dockerfile-ами проекта. Prod лимиты не использовались как измеренная capacity.
- В Go harness замер включает сервер и синтетические клиенты в одном процессе.
  CPU ниже — накопленное CPU time процесса, не процент загрузки сервера.

Повторение: `make stage6-up`, `make stage6-load`, `make stage6-soak`.
Отдельные integration нагрузки включаются env `RECORDER_WS_LOAD=true` /
`RECORDER_RECORDING_LOAD=true` с PG `127.0.0.1:15433`, Redis `127.0.0.1:16380`,
MinIO `127.0.0.1:19000`; см. [runbook](operations/README.md). TURN harness должен
работать внутри `recorder-stage6_default`, чтобы Coturn мог достигнуть Pion peers.

## HTTP / WebSocket

| Сценарий | Объём | Время | p50 / p95 / p99 | Ошибки |
|---|---|---|---|---|
| Authenticated `GET /api/v1/auth/me` | 100 запросов, concurrency 10 | 10.447ms | 0.662 / 1.984 / 6.288ms | 0, все 200 |
| WS reconnect burst, 2 API instances | 100 физических сокетов | 276ms | 236 / 261 / 272ms | 0, presence=100 |
| Chat burst | 80 сообщений | не использовалось как throughput benchmark | critical snapshot 6.83ms | 55 accepted, 25 expected rate-limit |

HTTP сценарий короткий, не измеряет steady-state RPS, тяжёлые mutations или
WAN latency. WS burst включает полный connect/auth/presence path. Его процесс:
heap 3068856→178929472 bytes, goroutines 14→314 во время нагрузки, CPU 1.028s.
После закрытия всех соединений Redis presence пуст; долгосрочный WS heap/FD soak
не выполнен. FD на macOS этим harness не измерен. Physical cap возвращает 503,
персональные rate limits — 429; это разные механизмы.

Повтор WS на финальном коде с race instrumentation: 100 sockets, burst 345ms,
p50/p95/p99 289/325/335ms, CPU 4.822s, cleanup PASS. Эти числа не смешиваются с
non-race baseline: race detector существенно меняет стоимость выполнения.

## SFU: две комнаты, 2/5/10 участников в каждой

Каждый peer публикует audio+video, принимает все остальные дорожки. RTP payload
синтетический (4 bytes), без полноценного encoder/decoder и без реального video
bitrate. Сеть ниже — полезные RTP bytes из SFU counter, **не wire bandwidth**.

| Участников / комнату | Время | CPU time | Heap до→во время, bytes | Goroutines до→во время | RTP packets / payload bytes | Dropped |
|---:|---:|---:|---|---|---|---:|
| 2 | 2.231s | 0.219s | 481024→5326824 | 3→231 | 840 / 3360 | 0 |
| 5 | 2.531s | 0.808s | 2908600→19462352 | 3→753 | 8896 / 35584 | 0 |
| 10 | 3.105s | 4.039s | 13226624→90342576 | 3→2103 | 44244 / 176976 | 0 |

Все subscriptions и receivers появились; после выхода rooms=0. FD на host
недоступен (`-1`), network packet loss не инъецировался. Размер all-to-all
подписок растёт квадратично, CPU/heap нельзя экстраполировать линейно на сотни peers.

TURN: отдельный реальный Coturn test, forced relay UDP и TCP-control,
двусторонний audio/video RTP, selected local relay candidate и reconnect PASS.
Повтор после outage: 4.12s UDP / 4.15s TCP. Это тест локальной Docker сети, не
корпоративного firewall, LTE/WAN или валидного внешнего TURNS сертификата.

## Запись SFU→composer/FFmpeg→MinIO

Это настоящий MP4 decode/verification, не только проверка существования файла.
Два независимых тона 440/660Hz исключают фазовое взаимное гашение в тестовой смеси;
RMS/проверки звука не ослаблялись. Проверяются grid/screen transitions,
закрытые chunks, preview/final object и отсутствие ложного ready при lease loss.

| Сценарий | Результат |
|---|---|
| Одна запись | 11.120729s, 640×360 H.264/AAC, 1008473 bytes, 6 chunks, 2 объекта |
| Финализация одной | 417.777ms; decoded 44 video frames при проверке 4fps, audio 11.25s |
| Parent test resources | CPU 1.43947s, sampled peak RSS 176455680 bytes, recording drops=0 |
| Две параллельные | обе PASS за 15.82s; duration 11.141328 / 11.119938s |
| Файлы параллельных | 1018173 / 1012997 bytes; 45 / 44 decoded frames, audio 11.26 / 11.25s |
| Shared test resources | CPU ~5.02s, sampled peak RSS 317341696 bytes, peak FFmpeg children=2, drops=0 |

CPU parent test не включает CPU всех дочерних FFmpeg; их число снималось дискретно.
В одиночном прогоне sampler не поймал короткие процессы (0), что **не** означает
отсутствие FFmpeg. Записи короткие, нельзя оценить из них часовые записи, дисковые
квоты, 1080p, число concurrent production encoders или throughput MinIO.

## Soak / утечки

Первый host прогон: 30m1s, 185 циклов join/RTP/leave, CPU 13.116s, goroutines
2→2, heap после GC 470896→1501920 bytes, rooms=0/peers=0,
19804 RTP packets / 79216 payload bytes. FD unavailable.

Контрольный Linux короткий прогон (12s, 18 циклов): goroutines 2→2, FD 8→8,
heap 325968→1184368 bytes, CPU 0.955s, 1880 packets / 7520 bytes.

30m Linux на Go 1.26.6 / исправленных зависимостях: **PASS** за 30m1s,
183 цикла, CPU 11.115s, goroutines **2→2**, FD **8→8**, heap после GC
331608→826280 bytes, 19252 RTP packets / 77008 payload bytes, rooms=0/peers=0.
Во время циклов снимки FD оставались 8. Бинарник этого длительного прогона
собран до последней поправки счётчиков ICE; эта поправка отдельно проверена
race/unit и настоящими TURN outage/recovery тестами.

Тест сравнивает ресурсы после GC/shutdown и ограничивает остаточный рост goroutine
и FD. Heap warm-up не равен доказанной утечке; allocation profile при больших
нагрузках всё ещё нужен. Soak покрывает SFU synthetic RTP, **не** 30m записи,
Redis keys/temp disk/browser/TURN allocation lifetime. Redis/temp cleanup проверены
отдельными короткими интеграционными сценариями.

## Сбои, ограничения и безопасные выводы

Матрица readiness/recovery и пользовательский эффект: [FAILURE_TESTS](operations/FAILURE_TESTS.md).
Эти времена включают Docker/poll и не являются RTO/SLA.

Подтверждено локально: 100 WS + burst, 2 комнаты по 10 peers с маленьким RTP,
2 коротких composite записи, forced TURN relay/reconnect и отсутствие очевидной
остаточной SFU goroutine/FD утечки в выполненных прогонах.
Не подтверждено: maximum safe production concurrency, WAN bitrate/loss/jitter,
TURNS с публичным сертификатом, HA, recovery при kill -9 или disk-full,
длительная запись и нагрузка БД/MinIO на production data scale.

DB pool, readiness и FFmpeg gauges доступны, но синхронного time-series capture
всех зависимостей во время baseline нет. Rabbit depth смотрится в broker, не
придуман в отчёте. Следующий capacity run должен использовать production-like
сервер, фиксированный image digest, реальные кодеки/битрейты, более длинный
steady state и отдельные server/client processes с Prometheus capture.

Проверка observability отдельно: Prometheus 3/3 targets UP, все 13 rules health=ok,
Grafana 12.2.0 dashboard provisioned и datasource health OK. После browser E2E
idle-ish app snapshot: API 262.3 МиБ, worker 23.18 МиБ, media 23.23 МиБ; это
после нагрузки/до принудительного GC, не leak proof и не steady-state budget.
