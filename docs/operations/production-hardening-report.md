# Итог этапа 6: production hardening

Дата: 1–2 октября 2026. Реализация по `CODEX_PROMPT_06.md`; этап 7 не начат.
Код и локальная приёмка подготовлены для контролируемого staging rollout.
Это **не** утверждение, что реальный production уже развёрнут или сертифицирован:
внешние сети/TLS, ёмкость, restore и дополнительные отказные сценарии требуют
проверки на целевой инфраструктуре. Основной локальный Compose не заменялся.

## 1. Findings production-аудита

PG/MinIO хранят durable данные; Redis presence/PubSub/ownership ephemeral;
Rabbit доставляет команды записи. API — auth/control/WS, media-worker — SFU,
recorder — ingest/compositor/FFmpeg/finalization. Каждый dependency/process/disk
в single-host схеме — SPOF. Redis PubSub не replay log, ownership не migration.

До изменений: статический ICE без краткоживущего TURN, нет общего readiness и
закрытого metrics endpoint, недостаточная startup validation, нет общего pool
budget/WS cap, legacy internal recorder HTTP без service auth, неполный shutdown,
FFmpeg output без общего ограничения, dependency vulnerability findings.
Исходные unit/race/vet/format/typecheck прошли до изменения поведения.

## 2. Реализованное усиление

Общий `internal/operations`: cached readiness, irreversible drain, JSON logging,
UUID correlation, закрытые метрики, loopback pprof и disk check. Новый config
разделяет dev/test/prod, проверяет числовые настройки, пароли и независимые ключи,
HTTPS origins/public storage, release/rate-limit. JSON body/WS connections/legacy
sessions/DB pool/FFmpeg output ограничены; internal recorder HTTP защищён Bearer.
Новые функции и структуры снабжены русскими комментариями.

## 3. TURN: проектирование и тесты

Coturn REST: `expiry:random UUID` + HMAC-SHA1 password, TTL 1–60m, default 10m,
shared secret только на серверах. Authenticated `/api/v1/webrtc/config`, no-store;
новый media.joined также выдаёт credentials. Frontend учитывает relay-only policy.
Production Coturn example: UDP/TCP, TLS, quotas, private/multicast peer deny.

Forced relay, выбранная relay pair, двусторонний audio/video RTP и reconnect:
UDP и TCP PASS. Остановка активного Coturn → два failed transport → rooms/peers=0
PASS; после старта снова UDP/TCP PASS. LAN/direct SFU regression PASS.
TCP проверяет control/client→TURN, не гарантирует TCP на TURN→SFU участке.
WAN/UDP-blackhole/TURNS с публичным сертификатом не воспроизведены.

## 4. Health/readiness

API/media/recorder имеют `/health/live` и `/health/ready`; compositor встроен в
recorder. Сбой dependency влияет на ready, не live. PG/Redis/Rabbit/MinIO, media
ownership и recorder disk проверяются согласно роли. Пробы timer-cached с общим
коротким deadline; 100 health запросов в unit-тесте не создают 100 probes.
Тексты ошибок, credentials и private addresses в health response не попадают.

## 5. Graceful shutdown/draining

Drain снимает ready, отклоняет новые HTTP/WS/control, API закрывает hijacked WS,
Hub ожидает sockets/PubSub/workers в контексте. Media очищает PC/rooms/ownership;
recorder завершает HTTP/consumer/composites/legacy ingest с ограниченным ожиданием.
FFmpeg TERM→KILL, child join, закрытые chunks сохраняются для recovery.
Default shutdown 20s, Compose grace 45s. Unit проверяет deadline зависшего socket,
однократный shutdown, SFU races/cleanup; integration проверяет presence cleanup.
Полная финализация активных часовых записей за 20s не гарантируется.
После browser acceptance отдельный Docker SIGTERM/stop для API/media/recorder
завершился с exit=0 у всех трёх процессов; активных записей в момент stop не было.

## 6. Logging/correlation

JSON slog с service/instance/event; HTTP UUID X-Request-ID, media event UUID,
internal request headers и Rabbit request_id до consumer. Legacy log adapter
ограничивает строку и скрывает известные env secrets. SQL logger выключен,
Pion safe logger не пишет SDP/ICE credentials. WS event ID отличается от HTTP
handshake request ID. Пользовательские IDs в bounded logs, не metric labels.
Полного distributed trace/OpenTelemetry в этом этапе нет.

## 7. Metrics/dashboards/alerts

HTTP count/duration/status, WS active/message class, dependency up, DB pool waits,
SFU rooms/peers/tracks/subscriptions/PC failures/RTP/drops, relay/direct first pair,
recordings/FFmpeg/durations/errors/queue latency, free disk, Go/process resources.
Labels ограничены техническими именами; cardinality unit test PASS.
Metrics требуют отдельный Bearer secret, иначе 404; публичный proxy закрывает их.

Optional production Compose profile: Prometheus + Grafana, 8-panel starter
dashboard, 13 alert rules. Promtool PASS; локально все три scrape targets UP,
Grafana dashboard provisioned и datasource health OK. Alertmanager delivery не
настроен. Waiting/online global aggregates, Rabbit depth и все query error counts
ещё не экспортируются; backlog проверяется broker command в runbook.

## 8. Resource limits/timeouts

Конфигурируемые HTTP body/read, WS physical/message/queues, rooms/peers/tracks,
upload/chat/reaction limits, recordings/FFmpeg/admission/duration/disk reserve.
PG 20/10/30m pool, connect 5s, query 10s, lock 5s; Redis dial/read/write/pool 3s;
AMQP connect/heartbeat/confirms/handler deadline; MinIO bounded transport/retries
и upload 10m. Pprof off by default, loopback only; seconds вне (0,60] отклоняется.
Low disk — admission/readiness, не гарантированная mid-recording quota.

## 9. Security/vulnerability findings

По согласованию пользователя Go поднят до 1.26.6; совместимые patch/minor updates
AMQP 1.13.0, pgx 5.9.2, DTLS 3.1.4, STUN 3.1.5, x/crypto/net/text/sys/sync и
других транзитивных модулей. Major application dependency migration не делалась.
`govulncheck v1.8.0 ./...`: 0 vulnerabilities в вызываемом коде/импортируемых
пакетах. 1 module-only finding GO-2026-5932 относится к неимпортируемому deprecated
OpenPGP; fixed version на момент проверки отсутствует. Это не «все модули без CVE».
Frontend production dependency audit: 0 vulnerabilities.

Auth/ticket/JWT expiry/algorithm, IDOR/permissions/waiting bypass, chat XSS,
upload paths/signed URLs, internal auth и literal FFmpeg arguments проверены
existing+новыми tests. Новый arbitrary-URL fetch/SSRF feature не добавлялся.
Bearer auth не cookie session: ambient-cookie CSRF модель не применяется.
HTTP/WSS production proxy и явный WS origin allowlist обязательны.
Container OS/MinIO/Coturn/FFmpeg image CVE scan и penetration test не выполнены.

## 10. Dependency resilience

Rabbit persistent publish+confirm, reconnect, QoS=1, bounded poison body,
одна повторная доставка → durable `.failed` (7d/10k/64MiB). Original ack после
confirmed quarantine. Correlation/reconnect/quarantine real-broker test PASS.
Duplicate delivery остаётся возможной; domain start/stop idempotency сохранена.

Redis reconnect/lease/presence semantics без обещания replay; PG finite pool/query
timeouts; MinIO upload size verification и best-effort token-prefix cleanup при
незавершённой финализации. Outage cleanup может оставить orphan object, его
нельзя автоматически считать пользовательским мусором.

## 11. Load results

100 HTTP, 100 WS+burst, synthetic SFU 2/5/10 peers в **двух** комнатах,
одна и две параллельные реальные короткие записи PASS.
HTTP p50/p95/p99 0.662/1.984/6.288ms; WS 236/261/272ms (non-race baseline).
SFU 20 peers total: 44244 packets, dropped=0, CPU 4.039s за 3.105s scenario.
Две записи: 15.82s scenario, decoded MP4 audio/video, FFmpeg child peak=2.
Полная таблица измерений/ограничений: [CAPACITY_BASELINE](../CAPACITY_BASELINE.md).

## 12. Soak/leak results

30m host SFU test: 185 циклов, goroutines 2→2, rooms/peers=0; FD host unavailable.
30m Linux Go 1.26.6 / исправленные зависимости: PASS, 183 цикла за 30m1s,
CPU 11.115s, goroutines 2→2, FD 8→8, GC heap 331608→826280 bytes,
rooms/peers=0, RTP 19252 packets / 77008 payload bytes. Бинарник прогона собран
до финальной поправки ICE counters; она отдельно прошла race/unit/TURN tests.
Soak не включает long recording/Redis keys/temp files; это не вся система под
30m production-like нагрузкой. Отдельные cleanup/lease/recording сценарии PASS.

## 13. Failure injection results

Изолированный стенд: Redis/Rabbit/PG/MinIO stop/start, media/recorder restart,
AMQP reconnect/poison, FFmpeg output/cancel/KILL, active TURN outage/recovery PASS.
Подробные времена, user effects, risks/operator actions:
[FAILURE_TESTS](FAILURE_TESTS.md). Dependency stop/start выполнялся без активных
комнат/записей, не доказывает zero-loss. Lease fencing/egress loss/closed chunk
recovery проверены настоящим recording E2E. Disk-full/kill -9/mid-upload outage
и controllable packet loss оставлены обязательной staging failure matrix.

## 14. CI/Docker changes

CI: format/unit/race/vet/govulncheck, frontend typecheck/test/build/audit,
бинарники; SFU load/30m soak manual/scheduled. Source extraction в test/build
идёт в выделенный временный каталог, не стирает общий workspace.
Docker multi-stage Go 1.26.6, runtime minimal Alpine, CA certs, non-root apps,
prod recorder UID1000/direct entrypoint, init/signals/limits/readiness checks.
Отдельные production и isolated stage6 Compose, Nginx TLS/WSS и Coturn configs.
API/media/worker images локально пересобраны, readiness 200. Production Compose
syntax и Nginx syntax PASS. Remote CI pipeline и image publishing не запускались;
полный Docker/TURN/browser acceptance сейчас локальный opt-in, не каждый commit.

## 15. Operations docs

[README/runbook](README.md): deploy/env/ports/NAT/firewall, HTTPS/WSS/TURNS,
health/drain, logs/metrics/pprof, FFmpeg/disk/MinIO, Rabbit backlog, Redis/DB,
backup/restore/rollback и safe tests. Secrets/config/certs вне Git/build context.
Backup scope PG+MinIO+secrets; Redis leases ephemeral; restore порядок и
согласованность record/object описаны. Restore drill/RPO/RTO ещё не измерены.

## 16. Capacity baseline

[Отдельный отчёт](../CAPACITY_BASELINE.md): hardware/Docker/date/base commit/config,
CPU/heap/goroutines/FD/network payload/latencies/FFmpeg и bottlenecks.
Dev результаты не объявлены production concurrency. Следующий запуск — на
фиксированном release image, с реальными bitrate/codecs/WAN и Prometheus capture.

## 17. Stage 3–5 regression

Authenticated API→WS→internal HTTP→Redis→SFU PASS; browser media/controls/reconnect
PASS. Moderation/outbox/session ordering/last disconnect PASS. Actual SFU recording,
grid/screen/audio/preview/finalization/recovery/fencing PASS, включая 2 concurrent.
Stage 5 chat/files/permissions/idempotency/notifications/scheduling/history PASS;
полный Docker HTTPS Chromium waiting/chat/upload/recording/history flow PASS.
Frontend 74 tests + typecheck + production build PASS; go unit/race/vet/format PASS.
Опциональные integration suites пропускаются без явных test env: их PASS выше
получен отдельными enabled запусками, не только обычным `go test ./...`.

## 18. Известные production ограничения

- Single-host SPOF; HA/multi-region/live room migration отсутствуют.
- WAN/mobile/corporate network, blocked UDP, public TURNS certificate и длительный
  TURN allocation refresh требуют проверки. In-place ICE restart/token refresh
  не реализован; recreated PC получает новые credentials.
- Нет измеренной production capacity, image-level CVE gate, restore drill и RPO/RTO.
- Пока нет global waiting/online/Rabbit-depth metrics и alert delivery.
- Длинная запись/disk-full/kill -9/storage outage during upload не покрыты нагрузкой.
- Локальный browser acceptance Chromium; полная Firefox/Safari/WAN matrix не
  заменяется этими тестами. Внутренний cleartext transport допустим только в
  изолированной доверенной сети; multi-host требует TLS/mTLS/VPN review.
- Основной ранее запущенный dev stack не обновлён автоматически; production не
  развёрнут. Isolated stage6 stack после проверок остановлен, volumes/images
  сохранены. Его можно повторно поднять `make stage6-up`; это не рабочие данные.

## 19. Рекомендованные приоритеты этапа 7

Сначала review этой приёмки и закрытие эксплуатационных проверок: реальный
HTTPS/TURNS/WAN, долговременная запись/refresh, restore drill, image scan,
production capacity/failure matrix и недостающая telemetry/alert delivery.
Затем, только по отдельному решению: email/push → external calendar →
transcription → AI summaries/search. Этап 7 автоматически не запускается.
