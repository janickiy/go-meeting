# Готовность к выпуску: автоматизация и локальная репетиция

Это исторический отчёт локальной репетиции. Последующее исправление инфраструктуры
и проверка закрытого staging на `meeting.janickiy.com` описаны
[в отдельном отчёте](meeting-host-deployment.md). Общий production GO пока не выдан из-за Firefox.

Дата: 3 октября 2026. Область согласована с владельцем: отдельного staging нет;
подготовить автоматизацию и выполнить локальные проверки. Этап 9 не реализовывался повторно.

**Решение для production: NO-GO.** Автоматизация и локальные проверки не подменяют
production-like staging, security approval и испытания внешней сети. Обнаруженные
HIGH/CRITICAL findings не скрыты исключениями. Реального production deploy не было.

## 1. Итоговый аудит

До реализации проверены фактические Compose/CI, миграции, операции завершения,
monitoring, документация и исторические отчёты этапов 6 и 9. Основные пробелы:
сборка на сервере, отсутствие immutable release manifest, повторное исполнение SQL
при startup, отсутствие общей процедуры rollback/restore, неполный drain и отсутствие
безопасной клиентской телеметрии. Их устранение не добавляет продуктовые функции.

Проверки текущего кода:

| Проверка | Результат и границы |
| --- | --- |
| `go test ./...`, `go test -race ./...`, `go vet ./...` | PASS; без opt-in инфраструктурные сценарии пропускаются |
| staticcheck v0.7.0 | PASS |
| govulncheck v1.8.0 | 0 достижимых / 0 импортируемых package findings; 1 module-only deprecated OpenPGP |
| SQL/Redis интеграции | Повторный полный прогон 57 pass / 12 skip; затем три прогона 171 pass / 36 skip |
| Первый SQL/Redis прогон | Две нестабильные notification-проверки упали, затем повторные прогоны прошли; причина не доказана, исправление не заявлено |
| Frontend | lint/typecheck/CSP/build, 157 unit, 6 a11y — PASS |
| Browser tests | Chromium 19 pass / 7 opt-in skip; две release privacy/metadata проверки PASS; отдельный реальный Compose UI smoke PASS |
| Firefox | Не стартует executable Playwright: `Could not find profile folder`; повтор с чистой загрузкой не устранил отказ |
| npm audit, включая dev | 0 vulnerabilities на дату проверки |
| Shell/YAML/JSON/Compose/Prometheus rules | Проверки синтаксиса и конфигурации PASS |
| Image security gate | FAIL; подробности в отдельном отчёте контейнеров |

Точные версии инструментов, license inventory и границы проверок:
[backend security](launch-security-review.md), [контейнеры](container-security-review.md),
[frontend](frontend-release-checks.md).

## 2. Архитектура развёртывания

Сохранена single-host Docker Compose модель. Шесть служб приложения: API,
media-worker, recorder worker, product-worker, live-worker, frontend. Восемь
инфраструктурных: PostgreSQL, Redis, RabbitMQ, MinIO, Coturn, TLS proxy,
Prometheus и Grafana. Миграция — одноразовая команда из API image, не новый демон.

Private control API и зависимости не публикуются в production. HTTPS/WSS проходит
через Nginx; media/TURN имеют отдельные порты. Persistent volumes и spool отделены
от образов. Redis хранит краткоживущее состояние. HA и live migration комнат не
добавлены; последовательное обновление одной реплики требует окна обслуживания.
Полный inventory, зависимости и пределы масштабирования: [ARCHITECTURE](../ARCHITECTURE.md).

## 3. CI и процесс выпуска

Определены validate → test → security → build → integration → package; legacy
`deploy-dev` сохранён только для development. В gates входят Go/race/static/vuln,
frontend/unit/a11y/CSP, PostgreSQL/Redis и Chromium/Firefox. Тяжёлые media/soak —
manual/nightly. CI loopback-прокси позволяют сохранить ограничения изоляции тестов.

Release package требует SemVer tag, registry, закреплённый Trivy, SBOM и отсутствие
HIGH/CRITICAL. Trivy получает image archive, не Docker API socket/credentials;
helper проверен локально, включая корректный отказ при находках.

Для первого выпуска нужен явный `RELEASE_BOOTSTRAP=true`; для последующих —
проверенный предыдущий infrastructure manifest. Фактический GitLab runner, registry
publishing и полный pipeline в этой работе не запускались. Требования к runner и
секретам перечислены в [RELEASE_PROCESS](../RELEASE_PROCESS.md).

## 4. Артефакты и версии

Подготовлены два локальных пакета:

- `v1.0.0-rehearsal.20261003.1` — baseline для проверки отката;
- `v1.0.0-rehearsal.20261003.2` — текущий проверяемый выпуск.

Оба привязаны к HEAD `c0df1df896637d7f5abcd2f2c9a149087ef57d65` с незакоммиченными
изменениями. Это **не clean production releases**. Каждый пакет содержит точные
image IDs, `images.tar`, `release.json`, SHA256SUMS. Для staging/production нужны
registry digests, чистое происхождение и пройденный image security gate.

Baseline build завершил образы, но запись build metadata прервалась при изменении
скрипта во время исполнения. Metadata восстановлены из согласованных OCI labels
и image IDs без пересборки; неизвестный source fingerprint явно отмечен. Текущий
build завершён штатно, source fingerprint проверен по labels. После его сборки
дополнялись тесты, release-скрипты и отчёты, но не поставляемый runtime-код.

При обновлении повторно используются восемь прежних инфраструктурных image IDs;
новая версия приложения не пересоздаёт MinIO/БД скрытым образом. Изменение
инфраструктуры требует отдельной процедуры maintenance. `/version`, frontend
`/version.json`, CLI version и `recorder_build_info` раскрывают безопасные метаданные.

## 5. Миграции

Добавлен `release_schema_migrations`: имя файла, SHA-256, время и длительность.
Advisory transaction lock сериализует запуск; SQL и ledger атомарны. При
`AUTO_MIGRATE=false` startup проверяет наличие и checksum, но не применяет SQL.
Применённые SQL-файлы больше нельзя редактировать, даже только комментарии.

Проверены fresh/legacy adoption, replay, concurrency, checksum mismatch, rollback
транзакции при ошибке и отказ startup с pending migration. На маленькой локальной
БД: **21 migration, суммарно 186ms SQL duration**. Повторный запуск не изменил
21 строку и исходные duration. Это не замер блокировок на production data.

Application rollback не запускает down migration. Используется Expand → compatible
deploy → backfill/verify → отдельный Contract. Обратная совместимость с настоящим
предыдущим production release ещё должна быть доказана на representative snapshot.

## 6. Окружение репетиции

Проект `recorder-release-rehearsal`, отдельные volumes/network, 14 контейнеров,
новые локальные credentials, временный self-signed сертификат. HTTPS:
`https://localhost:25482`; API доступен только на loopback `28085`, Grafana `23001`,
Prometheus `29090`. Рабочие проекты `go-recorder`, `recorder-stage8` и другие
существовавшие контейнеры не перезапускались и не удалялись.

HTTP/WSS smoke использует process-scoped `SMOKE_CA_FILE`: chain/hostname verification
не отключены, системное доверие не изменено. Browser UI test отдельно использует
явное исключение только для локального self-signed target. Это не внешний staging.

## 7. Smoke, UI, media и запись

На immutable deployed API/frontend через HTTPS проверены совпадение версии,
frontend, login/me, capabilities, ICE config, отказ admin обычному пользователю,
создание/join/cancel тестовой встречи и WebSocket snapshot. Токены/пароли не выводятся.

Реальный Chromium UI smoke baseline: **PASS за 5.3s**, финального current после
повторного deploy: **PASS за 5.0s** — регистрация, prejoin без
преждевременного join, вход ровно один раз, старт/завершение, history, notifications,
admin UI и API403. Firefox не пройден по причине запуска браузера, а не скрыт skip.

Дополнительно Go harness текущего исходного дерева с настоящими изолированными
PostgreSQL/Redis/RabbitMQ/MinIO выполнил:

| Сценарий | Результат |
| --- | --- |
| Composite recording | PASS, 15.57s; H.264/AAC 640×360, 6 chunks, 2 storage artifacts, 0 recordingDrops, auto-stop |
| Chat/files/read | PASS, 1.62s, 6 вложенных проверок |
| Четыре режима recording/storage | PASS, 11.19s |
| Forced TURN | PASS, 8.30s; UDP и TCP, relay-only двусторонний RTP и reconnect |

Recording harness использует host FFmpeg и Go services в тестовом процессе,
**не** полную цепочку deployed API→worker containers. TURN harness — helper в
private Docker network, не браузер/WAN/TURNS. Этим прогонам не приписываются
реальные устройства, screen capture, публичные сертификаты или внешние провайдеры.

## 8. Откат и повторное обновление

Выполнена последовательность baseline → backup → повторная migration → current →
HTTPS/WS smoke → rollback baseline → HTTPS/WS smoke → redeploy current → финальный HTTPS/WS smoke.
Все app images берутся из сохранённых пакетов; старый commit не пересобирается.
Infrastructure IDs и существующая схема остаются прежними. Drain закрывает новую
работу и ожидает active=0; timeout не выполняет скрытый force restart.

Ориентировочное wall time по временным меткам журналов: первичный deploy 42.7s,
повторная migration command 2.0s, current deploy 44.3s, rollback 44.2s,
redeploy 44.3s. Текущий build занял 72.6s с локальным cache. Финальный HTTPS/WS
smoke после повторного deploy — 107ms. Эти величины не являются SLA/RTO и не
включают полное окно осмысленного пользовательского наблюдения.

Два пакета относятся к текущей реализации этапа 10. Этот опыт проверяет механизм
отката, но **не доказывает** совместимость настоящего старого этапа 9 с будущей
схемой. Именно поэтому production требует отдельное compatibility evidence.

## 9. Резервное копирование и восстановление

Перед backup остановлены прикладные писатели через drain; записей/активных комнат
не было. `backup.sh` требует подтверждённый write freeze и общий project lock.
PG subprocess имеет deadline 600s по умолчанию, lock wait dump — 5s. Cross-store
snapshot при продолжающейся записи атомарным не объявляется.

Выполнены dump и восстановление в **новую** БД и **новый** приватный бакет:

- dump PostgreSQL: 146043 bytes; исходная и восстановленная БД имеют 2 users,
  3 conferences, 21 migration ledger entries;
- MinIO: 1 synthetic object, 56 bytes; SHA-256 и ContentType перенесены;
- проверены checksum backup/manifest, source release identity, приватная policy,
  подписанный GET и запрет анонимного чтения;
- SQL restore: 1s; object restore в секундной дискретности: 0s (не нулевая работа);
- source/target manifest hashes и конкретные retained target names сохранены в JSON.

Восстановленные DB/bucket оставлены для проверки. История версий объектов, tags,
произвольная metadata, broker/spool snapshot и recovery secrets не входят в этот
инструмент. Backup нужно шифровать и хранить off-host. Production RPO/RTO не заданы
и не выводятся из маленького теста. Процедуры: [backup/restore](backup-restore.md).

## 10. Мониторинг, оповещения и SLO

Prometheus фактически опрашивает **5/5 application targets**; все 18 правил имеют
health=ok, firing=0 в финальном снимке. Grafana отвечает с healthy database,
проверен provisioned dashboard из 14 панелей и datasource health=OK.
Добавлены версии служб, HTTP/WS, media, recording/jobs, storage/dependency gauges
и безопасные клиентские ошибки. Уровни и длительности правил — начальные предложения.

Внешний receiver оповещений не подключён: наличие правил не означает доставку
пейджинга. Метрики очередей/инфраструктуры интерпретируются только в реально
измеряемых границах; полной host/exporter topology не добавлялось. API availability,
signaling availability, media join success и recording completion предложены как
SLI; численных production SLO на основании локального baseline не назначено.

## 11. Безопасность, зависимости и лицензии

Клиентская телеметрия выключена по умолчанию, использует same-origin endpoint,
allowlist route/code/browser, короткий sanitized stack и ограниченный размер.
Нет raw message, chat/transcript, SDP, signed URLs или credentials. Проверены
frontend privacy cases и server validation/logging; Redis limits ограничивают приём.

Сканирование Trivy выявило HIGH/CRITICAL в MinIO, Coturn, PostgreSQL image utilities,
Redis/monitoring и старом frontend. Nginx обновлён с 1.27 на 1.30.5; финальные
frontend/proxy всё ещё имеют **2 HIGH** в libexpat/pcre2. Финальный API имеет
0 HIGH/CRITICAL. Нулевые результаты прежних worker images не приписываются новым
image IDs. Полная таблица, advisory/version/remediation evidence:
[container-security-review](container-security-review.md).

MinIO upstream архивирован: требуется поддерживаемый канал исправлений или
согласованная отдельная миграция. Массовое обновление major/storage и blanket
CVE exceptions не выполнялись. Локальные manifests имеют `securityScanned=false`
и заведомо не проходят production gate.

Собраны 124 Go module metadata entries, 74 сторонние Go license entries, 162 npm
package-path/license entries. У репозитория отсутствует LICENSE; автоматически
назначать её нельзя. MinIO server AGPLv3; финальный worker FFmpeg 6.1.2 с
GPLv3-or-later, x264/x265. Inventory не является юридическим заключением.

## 12. Сравнение производительности

Frontend main gzip: исторически 80.02KB; новый build с telemetry off 80.23KB,
с telemetry on 81.37KB (около +1.69%). Необъяснённого крупного роста bundle не найдено.

После финального redeploy повторён прежний loopback HTTP harness: 100 запросов
`GET /api/v1/auth/me`, concurrency 10, все 200, elapsed 10.90ms. p50/p95/p99:
**0.778 / 1.883 / 4.990ms** против исторических **0.662 / 1.984 / 6.288ms**.
Крупной регрессии p95/p99 в этом коротком прогоне не видно, но это не статистически
надёжное доказательство ускорения или steady-state capacity; ресурсы хоста не изолированы.

Исторический [CAPACITY_BASELINE](../CAPACITY_BASELINE.md) содержит HTTP 100/10,
WS burst, synthetic SFU и короткие записи; это не production capacity. Новые
recording/relay smoke подтверждают работоспособность, но их wall time не равно
отдельной latency финализации. Длительный soak, WAN bitrate/packet loss, SFU RAM/CPU
под реальной нагрузкой и performance gate на выделенном staging не повторялись.

## 13. Документация

Канонические [USER_GUIDE](../USER_GUIDE.md), [ARCHITECTURE](../ARCHITECTURE.md),
[DATA_FLOWS](../DATA_FLOWS.md), [API/WebSocket](../API.md),
[DEPLOYMENT](../DEPLOYMENT.md), [RELEASE_PROCESS](../RELEASE_PROCESS.md),
[ROLLBACK](../ROLLBACK.md), [RELEASE_NOTES_TEMPLATE](../RELEASE_NOTES_TEMPLATE.md),
[runbook](README.md), [backup/restore](backup-restore.md) актуализированы.
Администрирование, capabilities, ограничения, provider data flows и ротация
секретов описывают существующие компоненты. Исторические отчёты помечены как
исторические; прежний production rebuild/auto-migrate workflow не рекомендуется.

## 14. Launch checklist

[PRODUCTION_LAUNCH_CHECKLIST](../PRODUCTION_LAUNCH_CHECKLIST.md) намеренно не отмечен
как выполненный. Локальные результаты этого отчёта не подтверждают DNS/firewall,
public TLS/TURNS, on-call ownership, реальные provider credentials и business approval.

## 15. Блокеры

1. Security gate FAIL; исправление/обоснованный triage exact image findings и повторные сканы.
2. Нет отдельного staging и полного artifact-based browser/media/recording/WAN/TURNS rehearsal.
3. Нет clean signed-off registry release и фактически выполненного GitLab pipeline.
4. Firefox E2E не запускается в текущем окружении; обязательный CI gate остаётся.
5. Исходная нестабильность двух notification tests не объяснена окончательно; возможный clock skew — гипотеза, не исправление.
6. Не подтверждены representative migration/rollback compatibility, off-host recovery,
   on-call/alert delivery, production capacity и согласованные RPO/RTO.

## 16. Известные ограничения

Single-host SPOF, окно обслуживания при замене одной реплики, отсутствие живой
миграции SFU rooms; feature flags глобальные, account/cohort targeting и traffic
canary не изобретены. Recorder requeue новых starts может задерживать legacy stop:
нужно остановить producers/pending starts и завершить записи до drain. Composite
stop хранится в SQL. Backup восстанавливает только текущие object versions.
Локальные test accounts, cancelled/finished meetings, restore DB/bucket и образы
сохранены; автоматической очистки чужих данных или Docker prune не было.

## 17. Критерии GO / NO-GO

GO возможен только после закрытия обязательных пунктов checklist и явного решения
ответственного. Конкретный manifest должен иметь чистый source, immutable digests,
полный зелёный CI, допустимые security результаты, real staging auth/WS/media/recording,
backup/restore и проверенную совместимость предыдущего приложения с новой схемой.
Approval/evidence привязаны к version/commit/SHA-256 manifest. Нельзя поставить
фиктивные true, чтобы пройти CLI guard. Текущие пакеты остаются **local-only / NO-GO**.

## 18. Наблюдение после будущего запуска

Предложение оператору: первые 15 минут непосредственной проверки и последующие
30 минут наблюдения при достаточном объёме реальных операций; при малом трафике
продлить окно. Сохранить counts и denominators, error/latency trends, auth/WS/ICE,
TURN, recording completion, очереди, ресурсы и provider jobs. Системная регрессия
auth/SFU/recording или несовместимость схемы — остановка продвижения и проверенный
rollback/forward fix; необязательную функцию сначала можно отключить флагом.
Эти интервалы — предложение, не состоявшееся production наблюдение или SLA.

Локальные приватные артефакты проверки находятся в
`tmp/launch-rehearsal-20261003/`: packages, build/deploy/smoke logs, backup/restore JSON,
scanner evidence и тестовые credentials. Каталог исключён из Git и не заменяет
защищённое долговременное хранилище evidence. Не публиковать его целиком: backup и
credentials являются закрытыми данными даже в тестовом окружении.
