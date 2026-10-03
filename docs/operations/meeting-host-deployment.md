# Развёртывание meeting.janickiy.com

Дата проверки: 3 октября 2026. **Исправление Firefox установлено на закрытом
стенде и в локальном Docker. Полная проверка Firefox, включая звук, видео,
чат, вложения и запись, успешна. Публичный production-запуск остаётся отдельным
незавершённым этапом; ограничения доступа не изменены.**

## Границы изменений

Целевой хост: `93.89.223.151`, Ubuntu 22.04, amd64, Docker 29.1.3,
Compose 2.40.3. На хосте работают Apache, другие сайты, почта и VPN.
Развёрнут отдельный Compose-проект `recorder-staging-meeting`; существующие сервисы,
данные и конфигурации доменов не заменены. Для Meetrix добавлен отдельный Apache vhost.
HTTPS доступен оператору через согласованный VPN; остальные посетители получают 403.
Production-проект и его база ещё не запускались.

На начало проверки: около 4 ГиБ RAM, 7.5 ГиБ свободного диска. Стенд ограничивается
небольшим пилотом: до 2 комнат, 4 участников на комнату, одна одновременная запись
длительностью до 1 часа / 512 МиБ; резерв диска 1 ГиБ. Это не подтверждение capacity
для масштабного production. После загрузки артефактов осталось около 4.1 ГиБ диска.

## Исправления инфраструктуры

- Новый layout `hardened-shared-host`: S3-хранилище SeaweedFS 4.48 вместо
  архивированной community-версии MinIO. DNS-служба `minio` и `MINIO_*` остаются
  совместимыми настройками S3-клиента. Используется **новый том `objects`**:
  старые тома MinIO не открываются и локальные пользовательские данные не переносятся.
- S3 требует ключи с первого запуска; IAM API, публичный admin UI, WebDAV,
  Iceberg/Lance и telemetry выключены. Внутренний gRPC control plane не
  публикуется на хосте, доступен только доверенным контейнерам приватной сети.
- Coturn 4.18.0 собирается из проверенного по SHA-256 архива на Alpine 3.24.2.
  SQL/Mongo/Redis/OAuth-интеграции исключены из сборки; TLS сохраняется.
- PostgreSQL сохраняет major 16, Redis — major 7. Системные библиотеки обновлены.
  Старый Go-бинарник gosu заменён совместимым вызовом `/sbin/su-exec`, чтобы
  не распространять устаревший Go runtime. Итоговый PostgreSQL image не содержит старых слоёв gosu.
- Nginx 1.30.5 получил исправленные системные библиотеки.
- Prometheus обновлён до 3.15.0. Grafana 13.2.3 имеет 8 HIGH / 0 CRITICAL
  в bundled datasource plugins и **не включается в новый пакет**. Это не CVE exception.
  Классический release layout сохраняет её как отдельный Compose-файл.
- Reverse proxy слушает только loopback-порт, TLS остаётся на существующем Apache.
  Базы, broker, S3 control plane и metrics не публикуются наружу.
- TURN слушает только публичный IPv4 хоста в host network, от UID 1000,
  без capabilities. Это устраняет Docker hairpin NAT; запрет частных peer IP
  сохранён без исключений для Docker subnet. Общий секрет не публикуется.

## Проверки до публикации

- `go test ./... -count=1`: PASS.
- Новый archive security contract: 9 сценариев PASS, включая отказ при HIGH,
  CRITICAL, смене Image ID/архитектуры и повреждении архива, scan либо SBOM.
- Отдельный проект `recorder-security-acceptance`: новые PostgreSQL, Redis,
  RabbitMQ и SeaweedFS; существующий `go-recorder` не используется тестами.
- Реальные `TestStageFourCompositeRecording`, `TestStageFiveChatFilesRead`,
  `TestStageEightRecordingStorage`: PASS, 32.276s. Подтверждены H264/AAC,
  MP4/preview, audio_only, individual_tracks, screen_focus, приватные вложения,
  подписанные загрузки, race/idempotency и остановка записи.
- Все **13 финальных amd64 образов**: 0 HIGH / 0 CRITICAL, полный SBOM для каждого.
  Grafana не входит в пакет; CVE исключения и `ignore-unfixed` не использованы.
- Сертификат Let's Encrypt: SAN `meeting.janickiy.com`, действителен до 1 января 2027.
  Установлен узко ограниченный renew hook; ключ доступен Coturn через закрытую копию.
- Все 13 служб запущены; readiness всех Go-сервисов/БД/хранилища успешен.
  Пять Prometheus targets — `up`; OOM и автоматических перезапусков нет.
- HTTPS smoke: версии API/UI совпадают с manifest, регистрация/вход, capabilities,
  ICE, отказ non-admin в доступе к admin, создание/отмена встречи и WS snapshot — PASS.
- Chromium через настоящий опубликованный артефакт, **без static overlay**: два
  участника, SFU-видео, realtime чат, приватное PDF-вложение, подпись скачивания,
  анонимный отказ 403, запись start/stop/ready и просмотр истории — PASS (27s).
- Отдельные полные Chromium-сценарии forced relay: UDP 24.5s, TCP 24.7s, TLS 25.5s — PASS.
  `getStats` подтвердил выбранный local candidate `relay`, transport UDP/TCP/TLS и RTP.
- Публичные `/metrics`, `/health/ready`, `/internal`, `/operations/drain`, `/debug/pprof/` — 404.
- Перед backup: active conferences/records, processing jobs, pending uploads — 0;
  RabbitMQ ready/unacknowledged — 0. Admission закрыт, producer/consumer-процессы остановлены.
- Backup/restore staging: 12 объектов восстановлены в новый бакет, SHA-256 и private
  policy проверены; подписанное чтение успешно, анонимное запрещено. PostgreSQL
  восстановлен в отдельную БД с валидным migration ledger. Основная БД не заменялась.
- После backup выполнен `resume` того же артефакта, все службы снова healthy;
  повтор HTTPS/REST/WS smoke — PASS. Прежние Apache configs совпали по SHA-256,
  все шесть исходных VPN-контейнеров сохранили свои ID и не перезапускались.

Production evidence с общим результатом GO **не создан**: исправление Firefox
и успешная браузерная проверка не заменяют оставшиеся production backup/rollback gates.

## Доставка без внешнего registry

Новый `archive` artifact mode сохраняет те же scan и clean-source gates:
в пакет включены immutable image IDs, SBOM, scan каждого образа и SHA-256
архива. Перед загрузкой проверяются checksum, архитектура и отсутствие
HIGH/CRITICAL, после загрузки — source/version/commit labels приложений.
Production approval и version-bound evidence сохраняются обязательными.

Исходная версия инфраструктуры: `v1.0.0-meeting.20261003.1`, build snapshot commit
`fab52b8ec2acce8c6a07b19883a026333560e9c7`. Текущая версия приложения:
`v1.0.0-meeting.20261003.2`, snapshot commit
`f7913af0e001462060cc49e8a2ad2657a0a1a54c`. Docker при экспорте только amd64
меняет исходный OCI index на platform manifest. Delivery-пакет отдельно
проверяет цепочку **байты OCI manifest → config digest → Trivy ImageID**;
идентификаторы не сравниваются ошибочно с исходным multiarch index.
Deployment tooling имеет отдельный проверяемый checksum и не выдаётся
за source snapshot, использованный для сборки бинарников.
После исходного `package.sh --mode archive` delivery формируется через
`scripts/release/prepare-archive.mjs INPUT NEW_OUTPUT`: исходные результаты scan
и байты архива проверяются до привязки platform manifest. Развёртывать нужно
именно новый delivery, а не первоначальный multiarch-ID manifest. При
переиспользовании инфраструктуры третьим аргументом нужно передать её исходный
manifest: его checksum, Image IDs и отдельное происхождение проверяются.
Образы без RepoTags связываются с scan по проверенным digest manifest/config.

Текущий серверный пакет: `/opt/meetrix/releases/v1.0.0-meeting.20261003.2/delivery-v2`.
Исходники: `/opt/meetrix/releases/v1.0.0-meeting.20261003.2/source`.
Прежний пакет `.1/delivery-v3` сохранён для отката.
Закрытая конфигурация: `/etc/meetrix/staging/.env`; production secrets независимы.
Результаты и журналы: `tmp/infrastructure-hardening-20261003/` в рабочей копии,
на сервере — `/opt/meetrix/evidence/`. Пароли и токены в отчёты не включаются.

Текущая рабочая копия содержит ранее созданный frontend. `snapshot.sh`
создаёт отдельный чистый source repository с текущими байтами и собственным
commit; provenance явно сохраняет исходный commit и признак dirty исходной
копии. Git index/история исходного проекта не изменяются. Пароли, env и ключи
в source snapshot и Docker build context не включаются.

## Исправленный блокер Firefox

Firefox 155 / Playwright 1.63 на отдельном Linux runner отправляет корректное
initial offer: аудио `m=audio 9`, видео `m=video 0` с `a=bundle-only`, обе секции
в `a=group:BUNDLE`, публикации `microphone` MID 0 и `camera` MID 1.
Прежняя версия сервера отвечала `invalid_media_signal` и закрывала медиасвязь до ICE.

Валидатор раньше безусловно пропускал секции с port 0, затем сравнивал число
активных секций с заявленными публикациями и отклонял видео Firefox.
Port 0 + bundle-only не означает отказ
от дорожки в таком предложении: это определено в
[RFC 8843](https://www.rfc-editor.org/rfc/rfc8843.html#section-6).
Сырые SDP, ICE passwords и токены не журналировались; сохранена только структура секций.

После разрешения пользователя исправлен продуктовый SFU: новый разбор
`sdp_bundle.go` проверяет членство в BUNDLE, ненулевой порт основной секции,
уникальность MID и корректность bundle-only. Секции с нулевым портом без этого
атрибута остаются отключёнными. Проверки кодеков, типов источников, публикаций
и лимитов сохранены. Та же логика применяется при подтверждении SSRC после Ready.

Регрессии включают 15 некорректных структур BUNDLE и проверки отключённых
дорожек, приёмных слотов, кодеков и лимитов. `go test ./...`, race-проверка
SFU и 35 frontend media/component tests успешны. Все 13 образов нового пакета
имеют 0 HIGH / 0 CRITICAL; 7 инфраструктурных Image IDs остались прежними.
Перезапуск PostgreSQL, Redis, RabbitMQ, S3 и TURN не понадобился. Миграции БД
не изменены и повторно не запускались. Конференция пользователя сохранена.

На опубликованной сборке, без static overlay, Firefox прошёл полные сценарии
direct (21.4s) и forced TURN/TCP (22.8s): два участника, декодированные RTP
видеокадры, входящие аудиопакеты и незаглушённый запущенный плеер, чат,
приватное PDF-вложение, запись ready и воспроизведение MP4 в истории.
TURN-проверка подтвердила relay candidate и передачу байтов у обоих участников.
Такой же полный сценарий Chromium с forced TURN/TCP прошёл за 25.5s.
`totalVideoFrames` в используемом Firefox runner оставался нулевым для
WebRTC даже при сотнях декодированных кадров: проверка теперь использует
`getStats().framesDecoded` вместе с воспроизведением настоящего DOM-плеера.
Отказы отдельных `media.ice` сохраняются в отчёте; они не прервали проверенные
потоки. Закрытие WebSocket после завершения тестовой встречи не является
доказательством проблем с её передачей медиа.

Локально пересобран и обновлён только `go-recorder-media-worker-1` из того же
исправленного кода; контейнер healthy, перезапусков по ошибке нет. Остальные
локальные проекты и данные не изменены. На удалённом хосте конфигурации Apache
и ID всех контейнеров других проектов остались прежними.
Подробные результаты: `tmp/firefox-hotfix-20261003/`, серверные журналы:
`/opt/meetrix/evidence/firefox-hotfix-20261003/`.

Также исправлен самостоятельный проверочный Go CLI: для WebSocket используется
clone TLS config с ALPN HTTP/1.1, а не общий config, изменяемый HTTP/2 transport.
Это изменение тестового инструмента, не пересборка опубликованного приложения.

## Оставшиеся шаги

Зашифрованный off-host backup и подтверждение rollback/production evidence,
запуск чистого production-проекта, открытие Apache admission и финальная проверка.
Публичная production-публикация до этого не завершена.
Секреты подключения и конфигурации в этом документе не сохраняются.
