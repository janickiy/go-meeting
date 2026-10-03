# Проверка безопасности и зависимостей перед выпуском

Дата проверки: 3 октября 2026, Europe/Moscow. Область: фактический код backend, SQL-миграции, эксплуатационные маршруты, закреплённые Go-зависимости и доступные локальные образы. Это техническая инвентаризация; выбор лицензии проекта и юридические выводы не подменяются этим отчётом.

## Проверки и точные границы результата

| Проверка | Фактический результат |
| --- | --- |
| Go runtime | go1.26.6 darwin/arm64 |
| `go test ./...` | PASS до изменений и после основной реализации |
| `go test -race ./...` | PASS до изменений и после новых migration/drain/version/telemetry тестов |
| `go vet ./...` | PASS |
| staticcheck v0.7.0 | PASS |
| govulncheck v1.8.0 | 0 достижимых уязвимостей, 0 уязвимостей импортируемых пакетов; 1 module-only |
| Обычный integration baseline без env | 6 pass (включая вложенные), 58 skip; это не полный инфраструктурный прогон |
| Новые migration tests на PostgreSQL 16 | PASS: 0.17s и 0.21s; весь целевой пакет 0.704s |
| SQL/Redis suite на изолированных контейнерах | Повторный полный прогон: 57 pass, 12 skip, 0 fail; затем count=3: 171 pass, 36 skip, 0 fail; отдельные 3 Redis ownership/drain теста PASS |
| Первый SQL/Redis прогон | 58 pass, 12 skip, 2 fail (с учётом отдельного Redis package); notification category/fanout. Каждый из двух тестов затем PASS пять раз подряд, повторный полный прогон PASS. Причина первоначальной нестабильности не доказана; не считать её исправленной |
| Recording/chat/storage acceptance | PASS на выделенных release-rehearsal PostgreSQL/Redis/RabbitMQ/MinIO: `TestStageFourCompositeRecording`, `TestStageFiveChatFilesRead`, `TestStageEightRecordingStorage`, суммарно 28.935s |
| Forced TURN | PASS: `TestStageSixForcedTURN` UDP и TCP relay-only RTP + reconnect, 8.30s. Синтетические peers в private Docker network через реальный Coturn; это не WAN/browser/TURNS проверка |
| Контейнерный CVE scan | Trivy 0.75.0: все 14 исходных образов проверены, HIGH/CRITICAL gate FAIL; повторный финальный API 0/0, frontend и Nginx 1.30.5 — 2 HIGH/0 CRITICAL. Точные image IDs, DB timestamp и оговорки в [container-security-review.md](container-security-review.md) |
| Git secret scan | Узкий поиск форматов private key/AWS/GitHub tokens в текущих файлах не нашёл совпадений; это не полный secret scan истории Git |

На первом SQL/Redis прогоне 12 интеграционных сценариев были пропущены по opt-in flags. Позже отдельно выполнены chat files, composite recording и recording storage. Из этого backend-аудита не следует успешное выполнение оставшихся semantic authorization/pgvector, browser controls, frontend DB, browser media, WS reconnect burst, concurrent recordings, browser proof, search corpus и synthetic WebRTC recording. Их состояние сверять с общим launch report. Общий успешный `go test` не заменяет эти сценарии.

В composite acceptance подтверждены H264/AAC 640×360, 11.12s media, 6 chunks, 2 MinIO artifacts, decoded audio/video, пик FFmpeg children = 1, recordingDrops = 0, восстановление истёкшего recorder и автоматическая остановка по завершению конференции. Storage acceptance проверил composite/audio_only/individual_tracks/screen_focus. Chat acceptance включал пагинацию, ограничения доступа и burst: 55 принятых / 25 rate-limited запросов из 80. Все fixture используют отдельные БД, Redis namespaces и MinIO prefixes, а не пользовательские записи.

Эти Go integration tests исполнялись из исходного дерева на host (включая host FFmpeg) с настоящими изолированными инфраструктурными зависимостями. Они не являются полным browser → immutable API → deployed worker E2E. Forced TURN исполнялся в отдельном Linux helper container через Coturn в private network. Результаты deployed smoke, браузерных тестов и backup/restore фиксируются отдельно.

Проверена гипотеза о времени для первоначальных двух notification failures: 100 измерений показали разницу PostgreSQL относительно midpoint часов host от −330µs до +410µs; в 98 измерениях DB timestamp немного предшествовал host request start. Код сравнивает host `participant.created_at` с SQL cutoff job. Это возможный механизм пограничного исключения только что добавленного участника, но не доказанная причина исходных failures (полный assertion output первого запуска не сохранён). Производственное изменение cutoff без воспроизводимого теста не выполнялось.

[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) относится к устаревшему `golang.org/x/crypto/openpgp`, который проект не импортирует. Исправленной версии для этого advisory нет. `x/crypto` требуется для другого кода; удаление всего модуля или механическое повышение версии не устраняет эту module-only запись.

## Go: текущая и доступная версия

Источник inventory: `go list -m -u -json all` (метаданные модулей из Go proxy, без изменения go.mod/go.sum). 124 внешних модуля в графе, 23 прямых зависимости. `latest` здесь ограничен тем же module path: смена major/import path не предлагается автоматически. Полный результат: [go-dependency-inventory.csv](go-dependency-inventory.csv).

| Прямая зависимость | Закреплена | Доступна в module path |
| --- | --- | --- |
| github.com/gin-gonic/gin | v1.10.0 | v1.12.0 |
| github.com/golang-jwt/jwt/v5 | v5.3.1 | v5.3.1 |
| github.com/google/uuid | v1.6.0 | v1.6.0 |
| github.com/gorilla/websocket | v1.5.3 | v1.5.3 |
| github.com/jackc/pgx/v5 | v5.9.2 | v5.11.0 |
| github.com/joho/godotenv | v1.5.1 | v1.5.1 |
| github.com/minio/minio-go/v7 | v7.0.95 | v7.3.0 |
| github.com/pion/ice/v4 | v4.2.5 | v4.4.5 |
| github.com/pion/interceptor | v0.1.45 | v0.1.49 |
| github.com/pion/logging | v0.2.4 | v0.2.4 |
| github.com/pion/rtcp | v1.2.16 | v1.2.19 |
| github.com/pion/rtp | v1.10.2 | v1.10.5 |
| github.com/pion/sdp/v3 | v3.0.18 | v3.0.20 |
| github.com/pion/stun/v3 | v3.1.5 | v3.1.7 |
| github.com/pion/webrtc/v4 | v4.2.12 | v4.2.22 |
| github.com/prometheus/client_golang | v1.23.2 | v1.24.1 |
| github.com/rabbitmq/amqp091-go | v1.13.0 | v1.15.0 |
| github.com/redis/go-redis/v9 | v9.19.0 | v9.22.0 |
| golang.org/x/crypto | v0.56.0 | v0.57.0 |
| golang.org/x/sys | v0.47.0 | v0.48.0 |
| gorm.io/datatypes | v1.2.7 | v1.2.7 |
| gorm.io/driver/postgres | v1.6.0 | v1.6.3 |
| gorm.io/gorm | v1.31.1 | v1.31.2 |

Наличие новой версии само по себе не означает security blocker. Для Pion/ICE/DTLS/SRTP обновления должны сопровождаться двумя peers, forced relay, recording и reconnect regression. Для pgx/GORM — SQL/migration и concurrency suite. Для остальных — changelog, govulncheck и критическая регрессия. В рамках backend-аудита версии Go-модулей не менялись.

[Официальный список Go](https://go.dev/dl/?mode=json) на дату проверки предлагает 1.27.1 как текущий stable. Проверенный проект остаётся на 1.26.6; переход на другую ветку требует отдельного совместимого прогона, не включается автоматически в release automation.

## Базовые образы и бинарные компоненты

Это снимок исходного состояния перед релизными изменениями. Итоговый manifest обязан перечислять конкретные digest всех поставляемых образов.

| Компонент | Исходная фиксация | Security / рекомендация |
| --- | --- | --- |
| Go builder | golang:1.26.6-alpine3.23 | Фиксировать digest, сканировать итоговый бинарник и runtime |
| Runtime | alpine:3.22 | Пакеты apk при rebuild могут измениться; SBOM и scan каждого digest обязательны |
| Frontend builder | node:24-alpine | Major tag изменяем; сохранить resolved digest |
| Proxy/frontend | До аудита nginx:1.27-alpine; новый candidate nginx:1.30.5-alpine | Manifest candidate проверен, Docker/Compose ссылки обновлены. Исправленные upstream версии не заменяют image CVE scan и HTTP/WS/TLS regression |
| PostgreSQL | postgres:16-alpine | Сохранить major 16; проверить minor/digest, расширения и restore |
| Redis | redis:7-alpine | Сохранить совместимый major до отдельной проверки; scan/digest |
| RabbitMQ | rabbitmq:4.2.9-management | Проверить security advisory и scan exact digest; management не публиковать наружу |
| Coturn | coturn/coturn:4.18.0-r0 | Нужны image scan и TURN abuse/relay smoke |
| MinIO server | RELEASE.2025-10-15T17-29-55Z, сборка из исходников | Upstream архивирован; требуется явное решение о поддержке исправлений |
| Prometheus / Grafana | v3.5.0 / 12.2.0 | Проверить exact image scan и не публиковать admin UI без защиты |
| FFmpeg в финальном worker image | 6.1.2 | GPLv3+ сборка с x264/x265; проверена в `.2` image ID ниже |

[Официальные Nginx advisories](https://nginx.org/en/security_advisories.html) указывают, например, CVE-2026-42533 для map/regex в 0.9.6–1.31.2; исправленные ветки начинаются с 1.30.4/1.31.3. Новейшие перечисленные исправленные maintenance варианты — 1.30.5+/1.31.6+. Применимость каждой CVE зависит от модулей и конфигурации; не заявляется подтверждённая эксплуатация проекта. До production необходимо выбрать доступный patched image, проверить конфигурацию, TLS/WSS и скан digest. Эта работа не подменяется зелёным govulncheck.

[Upstream MinIO](https://github.com/minio/minio/blob/RELEASE.2025-10-15T17-29-55Z/LICENSE) сообщает archived/read-only с 25 апреля 2026. Закрепление старого server release не обеспечивает дальнейшие исправления. Оператору нужен документированный поддерживаемый канал обновлений или отдельный план замены; существующие данные автоматически не переносятся.

## Лицензии

`go-licenses v1.6.0 report --ignore github.com/janickiy/go-recorder ./...` завершился с кодом 0; 74 строки сторонних пакетов сохранены в [go-license-inventory.csv](go-license-inventory.csv). Найдены MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0 и MPL-2.0. MPL-2.0 относится в том числе к транзитивному MySQL driver; direct PostgreSQL usage не означает отсутствия MySQL-зависимостей в графе GORM.

Полный запуск без `--ignore` сообщает, что у самого репозитория не найден LICENSE. Это не основание автоматически назначать проекту лицензию. Инструмент предупреждает, что assembly/non-Go источники не позволяют полностью вывести дополнительные зависимости. CSV не включает OS packages, FFmpeg codecs, npm, контейнерные серверы и внешние сервисы.

MinIO server использует [AGPLv3](https://github.com/minio/minio/blob/RELEASE.2025-10-15T17-29-55Z/LICENSE), MinIO Go SDK — Apache-2.0. Эти компоненты нельзя объединять под одной лицензией только по имени поставщика.

Проверен существующий локальный `go-recorder-worker`, image ID `sha256:eabbc91a0169d5dca70a623a63c9700a0f397397e5e3f00695433edccda992f2`: `ffmpeg -version` = 6.1.2, configure содержит `--enable-gpl --enable-version3 --enable-libx264 --enable-libx265`; `ffmpeg -L` сообщает GPLv3 or later. Сам проект вызывает отдельный процесс FFmpeg. Для поставляемого релизного digest повторить inventory, сохранить configure flags, лицензии и соответствующие исходники/уведомления. [FFmpeg поясняет](https://ffmpeg.org/legal.html), что включение GPL-компонентов меняет лицензию конкретной сборки. Патентные и юридические выводы этот технический inventory не делает.

Повторный inventory финального `local-recorder-release/worker:v1.0.0-rehearsal.20261003.2`, image ID `sha256:1b8eff209aeae5b5bf0ddb9f581535b61bf6cb5119a12103c316f75b717b3428`, подтвердил FFmpeg 6.1.2, gcc 14.2.0, те же GPL/version3/x264/x265 flags и GPLv3-or-later в `ffmpeg -L`. Проверка не назначает лицензию коду самого проекта и не заменяет комплект лицензионных материалов для распространения.

## Проверенные security границы кода

- Production config отклоняет placeholders/default credentials, требует независимые 32+ byte ключи, пароли зависимостей 16+ byte, HTTPS origins, HTTPS MinIO public origin, rate limits и GIN release.
- Debug routes регистрируются только в local mode. pprof слушает loopback; metrics и drain требуют отдельный METRICS_SECRET, неверный секрет получает 404.
- JWT ограничен HS256, issuer/audience/expiry/issued-at; WebSocket проверяет origin и одноразовые tickets.
- OAuth использует PKCE/state, доверенные HTTPS endpoints, запрет redirects, ограниченный размер ответа; токены не включаются в публичный DTO.
- Chat attachments и STT требуют приватных buckets. Presigned URL остаётся ограниченным bearer-доступом до истечения — мгновенный отзыв не обещается.
- FFmpeg запускается через argv без shell, ограниченные пути/время/размеры; это не замена image CVE scan и изоляции процесса.
- Публичный `/version` содержит только version/commit/buildTime. Подозрительные ldflags значения заменяются безопасными dev/unknown; пути, переменные окружения и compiler flags не выдаются.
- `POST /operations/drain` необратим до перезапуска. API перестаёт принимать новые прикладные HTTP-запросы, считает незавершённые HTTP и WS. SFU перестаёт принимать все новые joins (включая reconnect), но обслуживает текущие SDP/ICE/leave и продлевает ownership. Runtime готовность становится false. Активные комнаты не мигрируют.
- Recorder откладывает AMQP starts без карантина, продолжает stops и активные записи. Новые SQL composite claims запрещены; текущая финализация и загрузка учитываются. Перед drain остановить создание новых записей: при непрерывных starts повторная доставка может задерживать legacy stop; timeout должен прервать обновление, а не принудительно убить запись. Composite stop хранится в SQL и наблюдается действующим владельцем независимо от очереди. Product/live workers прекращают новые claims и учитывают уже начавшийся SQL-захват.

## Миграции и откат

`release_schema_migrations` хранит имя, SHA-256 точных байтов, applied_at и duration_ms. Один execution owner вызывает `recorder-migrate migrate`; advisory transaction lock 748239105 сериализует исполнителей, lock wait ограничен 15s, весь проход 5m, SQL и журнал атомарны. При отсутствии журнала существующие исторические идемпотентные миграции исполняются один раз: запись не создаётся без успешного SQL. На копии крупной БД нужно отдельно измерить время блокировок — маленькая тестовая БД не representative production benchmark.

`AUTO_MIGRATE=false` только читает журнал: неприменённая миграция или изменённый checksum останавливает startup. Команда migrate выполняется явно независимо от этой настройки. Уже применённые файлы, включая комментарии, больше не редактировать; исправления делать следующей миграцией.

Проверки на изолированной PostgreSQL подтвердили: legacy SQL действительно выполняется; повторные и параллельные запуски не повторяют SQL; изменённые суммы отклоняются; ошибочный второй файл откатывает первый и журнал; advisory lock не обходится; выключенный automigrate не создаёт pending table.

Записи новых миграций, которых нет в старом артефакте, разрешены при проверке startup: это технически позволяет совместимый application rollback, но не доказывает совместимость schema. Expand/contract и релизные rollback notes остаются обязательны. Production down migration автоматически не запускается.

## Воспроизведение

```sh
go version
go test ./...
go test -race ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -show verbose ./...
go list -m -u -json all
go run github.com/google/go-licenses@v1.6.0 report --ignore github.com/janickiy/go-recorder ./...
# Только выделенные локальные PostgreSQL/Redis; тесты создают и удаляют собственные БД/namespace.
go test -count=1 -v ./tests/integration -run '^TestReleaseMigration'
go test -count=1 -v ./internal/infrastructure/redis -run '^TestDrainingWorker'
docker run --rm --entrypoint ffmpeg VERIFIED_WORKER_IMAGE -version
docker run --rm --entrypoint ffmpeg VERIFIED_WORKER_IMAGE -L
```

Для SQL/Redis использовать `RECORDER_STAGE1_TEST_POSTGRES_DSN` (CREATE DATABASE, localhost), `RECORDER_STAGE2_TEST_REDIS_ADDR` и при включённом Redis AUTH — `RECORDER_STAGE2_TEST_REDIS_PASSWORD`. Значения секретов не сохранять в выводе/отчётах. Полный image gate по фактическим findings: **FAIL / production NO-GO**. Staging/production rehearsal и общая рекомендация фиксируются в финальном launch report; локальная репетиция с явным bypass не снимает security blocker.
