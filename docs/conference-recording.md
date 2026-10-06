# Этап 4 Управление медиа и запись конференций

Документ описывает основу реализации по `CODEX_PROMPT_04.md` и последующие изменения
прав записи и уведомлений. Камера и экран передаются через SFU,
а общая запись формируется отдельным recorder-worker. Обзор текущих контрактов
и режимов записи — в [API.md](API.md); результаты исторических прогонов приведены ниже.

## 1 Реализованные возможности

Участник может независимо включать микрофон и камеру, менять устройства и показывать
экран вместе с камерой. Повторная публикация сохраняет PeerConnection. Владелец
управляет правами, назначает соорганизаторов и исключает участников. Запись может
запустить любой допущенный участник с аккаунтом. В интерфейсе конференции появились
управление медиа, модерация, индикатор
и история записей с подписанными ссылками. Большого изменения дизайна нет.
Доступно подключение без камеры и микрофона: можно слушать встречу и отдельно
включить только нужное устройство или экран, не запрашивая оба устройства сразу.

## 2 Архитектурные решения

```text
Browser ⇄ WebRTC ⇄ media-worker SFU ⇄ другие Browser
                         │ encoded RTP и metadata
                         ▼
                 защищённый HTTP egress
                         ▼
                 recorder-worker spool
                         ▼
           FFmpeg segment composition и audio mix
                         ▼
           concat MP4 → preview → ffprobe → MinIO
```

SFU не запускает FFmpeg и не декодирует медиа. API не проксирует RTP. Передача
в recorder выделена в независимую ограниченную очередь: медленный или сломанный
recorder не тормозит участников. Существующие RabbitMQ, Redis, record tables,
FFmpeg postprocessor и S3-клиент используются повторно.

## 3 Демонстрация экрана

`media.offer.publications` связывает SDP MID с источником `microphone`, `camera`,
`video/screen` или `audio/screen`. Дополнительный `trackId` обозначает поколение
browser capture, но не является серверным ID публикации. Старый протокол без
publications сохраняется для Stage 3. Камера и экран имеют отдельные streamId:
`mediaPeerId` и `mediaPeerId-screen`.

Браузер вызывает `getDisplayMedia()` по нажатию кнопки. Аудио экрана необязательно;
его завершение не останавливает видео. Завершение video/screen, отключение вкладки
или модерация освобождают владение. Лимит `MEDIA_MAX_SCREEN_SHARERS=1` задаётся
конфигурацией; второй владелец получает `screen_sharing_conflict`. Клиент откатывает
отклонённый offer, останавливает захват экрана и сохраняет звонок.

## 4 Матрица прав

| Действие | Owner | Co host | Participant |
| --- | --- | --- | --- |
| Собственные устройства и экран | Да, если разрешены | Да, если разрешены | Да, если разрешены |
| Завершение конференции | Да | Нет | Нет |
| Запуск записи с аккаунтом | Да | Да | Да |
| Остановка записи | Любая запись встречи | Своя запись | Своя запись |
| Mute и остановка экрана другого | Любой не owner | Только participant | Нет |
| Запрет видео другого | Любой не owner | Нет | Нет |
| Исключение другого | Любой не owner | Только participant | Нет |
| Назначение и снятие co_host | Да | Нет | Нет |
| Просмотр записей своей конференции | Да | Да | Да |

Для управления нужны авторизация и активное joined membership. Owner неизменяем;
исключённый участник не получает историю записей. Вышедший, но не исключённый
участник сохраняет доступ к истории. Гости с `guest_conference_id` не управляют записью, даже имея JWT и joined membership.
Остановить запись может её инициатор (`requested_by`) либо owner. При фактическом
начале все участники слышат встроенное английское объявление «Recording has started».
UUID записи объединяет событие и polling, повторная доставка не повторяет звук.

## 5 Семантика модерации

Принудительный mute блокирует всё аудио участника, включая audio/screen.
Модераторский запрет видео блокирует камеру и экран; в UI он называется
«Отключить видео». Это предотвращает обход переименованием источника. Собственный
переключатель микрофона или камеры влияет только на соответствующее устройство.

Ограничения записываются в PostgreSQL и действуют на все вкладки. Разрешение
`blocked:false` снимает запрет, но никогда не включает оборудование автоматически.
Браузер освобождает захват; сервер независимо прекращает forwarding и запрещает
повторную публикацию. Физическое происхождение картинки сервер подтвердить не может:
typed sources описывают заявленное назначение, а не аппаратную аттестацию.

Каждое изменение имеет монотонную policy version и audit row. Свежая policy
проверяется на join и offer, возвращается в media.joined и доставляется как
media.policy. Reconciler повторяет неуспешную доставку каждые 2 секунды. При kick
membership/history сохраняются, WS и медиа закрываются; повторный вход запрещён
без отдельной будущей процедуры восстановления. После последней вкладки control
flags очищаются; наличие control flag не доказывает наличие RTP.
Флаги хранятся для каждой WebSocket-сессии и объединяются через OR: открытие пустой
вкладки не выключает индикатор работающей камеры в другой. Обновления содержат
connectionId и возрастающий sequence; закрытая/чужая сессия отвергается, запоздавшее
обновление не перезаписывает новое. Изменение статуса сессии и агрегация используют
тот же порядок блокировок, что и модерация. Снятие запрета не восстанавливает старые
флаги устройств.

## 6 Архитектура общей записи

Recorder получает VP8 и Opus без транскодирования на SFU, депакетизирует их в
короткие IVF/Ogg-фрагменты и сохраняет атомарные manifests. Отдельный layout module
выбирает full frame для одного видео, две плитки для двух, 2×2 для трёх или четырёх
и сетку для большего количества. При screen share экран занимает основную область,
а камеры — дополнительную. Порядок определяется стабильными participant/track IDs.

FFmpeg строит одинаковые H.264/AAC сегменты, затем соединяет их без повторного
перекодирования. Audio mix использует нормализацию, limiter и единые 48 kHz.
Общий clock egress и offsets выравнивают потоки; пустые области и тишина заполняют
пропуски. Смена layout происходит на границах сегментов. Последний неполный сегмент
сохраняется. Это не active-speaker layout и не запись отдельных участников.
Для нерегулярных кадров браузера последнее видеоизображение сохраняется до нового
кадра либо явного track.end; тихие сегменты используют ограниченный GOP pre-roll.
Завершённый источник не продолжается бесконечно. Сбой FFmpeg немедленно прерывает
capture записи, не ожидая ручного stop; число удерживаемых буферов ограничено также
при частой смене track ID.

## 7 Контракт между SFU и recorder

Внутренний `POST /internal/media/egress` защищён отдельным Bearer secret и Redis
ownership lease. Запрос содержит conferenceId, recordingId, route и параметры PLI.
Ответ — упорядоченный NDJSON: hello, track, RTP, track end, ping либо error.
Метаданные содержат codec и typed source; RTP передаётся как base64, capturedAt —
общий timestamp worker. Sequence позволяет обнаружить пропуски контракта.

На SFU очередь ограничена `MEDIA_EGRESS_QUEUE_SIZE`; overflow завершает только
recording output. Пустая комната сохраняется, пока существует recording subscriber.
Внутренние адреса и secrets не доступны браузеру. HTTP write deadline обновляется
на каждой отправке; shutdown закрывает egress до ожидания завершения HTTP.

## 8 Жизненный цикл и восстановление

Публичные состояния: `starting → recording → stopping → processing → ready/failed`.
Существующие `finalizing` и `uploading` отображаются как processing. Для composite
ready выставляется
только после проверки MP4, создания preview, загрузки обоих объектов и транзакции
метаданных. Ошибка записи не завершает конференцию.

Start создаёт DB row и outbox в одной транзакции под блокировкой конференции.
Повторный start любого режима возвращает `409 recording is already active`, пока
предыдущая запись находится в starting/recording/degraded/stopping/finalizing/uploading.
Защита распространяется на параллельные запросы; уникальный индекс дополнительно
запрещает вторую незавершённую запись. Запуск разрешён любому допущенному участнику
с аккаунтом; остановка — владельцу конференции либо инициатору записи. Гостям
запуск и остановка запрещены.
Dispatcher получает Redis lock и подтверждение RabbitMQ; stop
не ждёт FFmpeg в HTTP. Finish конференции атомарно добавляет stopping и stop command.
Worker наблюдает DB state, поэтому stop, доставленный другой реплике, не теряется.

События API и recorder содержат `recordingId`, `conferenceId`, публичный `status`,
`mode` и `requestedBy` из сохранённой записи. `recording.starting` означает подготовку;
по `recording.started` все допущенные участники, включая инициатора, слышат
«Recording has started». Текстовая плашка показывается остальным участникам.
Плашка использует существующий WebSocket и дедупликацию по UUID записи. После
входа или переподключения список записей сверяется с API; активная запись также
показывает предупреждение, если событие было пропущено. Уведомления о начале
не добавляются в личный каталог уведомлений и не отправляются по email/push.

У владельца recorder есть DB lease и уникальный incarnation token. Устаревший worker
не может продлить lease, изменить статус или опубликовать метаданные. После сбоя
финализацию закрытых сегментов можно повторить. Оборванный активный capture считается
failed: система не выдаёт неполную запись за непрерывную успешную встречу.

Объекты имеют путь `recordings/{conferenceId}/{recordId}/artifacts/{token}/final.mp4`
и аналогичный `preview.jpg`. Token-подкаталог намеренно дополняет рекомендованный
путь: старый незавершённый upload не способен перезаписать результат нового владельца.
API выдаёт ссылки только на подтверждённое поколение. Bucket остаётся приватным.

## 9 Миграция базы данных

`000010_conference_recording_control.up.sql` добавляет control flags и policy version участникам,
таблицу `conference_moderation_audit`, поля mode/platform_conference_id/recorder lease
в `record` и `recording_outbox`. Частичный unique index запрещает две активные
composite-записи одной конференции. Старые строки остаются mode=legacy, без нового FK.
Миграция добавочная; существующие recordings и данные пользователей не удаляются.
`000011_participant_session_media.up.sql` добавляет три media flags и sequence
в `participant_sessions` для корректной работы нескольких вкладок и защиты
от перестановки HTTP-обновлений.

## 10 API и события WebSocket

Все новые HTTP endpoints требуют JWT:

| Метод и путь относительно `/api/v1/conferences/{id}` | Назначение |
| --- | --- |
| `PUT /participants/me/media` | connectionId, sequence, microphoneEnabled, cameraEnabled, screenSharing |
| `POST /participants/{participantId}/moderation` | action mute/camera/screen/kick/role; blocked либо role |
| `POST /recordings` | Запуск; optional segmentDurationSec от 2 до 30; ответ 202 item |
| `POST /recordings/{recordingId}/stop` | Асинхронная остановка, ответ 202 item |
| `GET /recordings` | История с limit/offset |
| `GET /recordings/{recordingId}` | Метаданные и подписанные URLs |

События: participant.media.updated, participant.role.updated, participant.kicked,
media.policy, recording.starting, recording.started, recording.stopping,
recording.processing, recording.ready, recording.failed. Старые анонимные `/records`
и worker ingest endpoints не читают и не изменяют composite rows; legacy start с ID
платформенной конференции также запрещён.

## 11 Основные файлы

Новые модули: `internal/infrastructure/composite`, `internal/usecase/recordings`,
`internal/app/recordings`; domain media egress и moderation; PostgreSQL repositories
для control, outbox и fenced artifacts; `internal/usecase/recorder/composite.go`;
SFU policy/egress и защищённые worker handlers; `internal/config/composite.go`.

Изменены bootstrap API/media-worker/worker, SFU publication lifecycle, RabbitMQ
publisher confirms, Redis lock, legacy guards, realtime disconnect observers,
frontend media client/hook/conference screen. Добавлен `RecordingPanel` и unit,
integration, browser tests Stage 4. Конфигурация описана в `.env.example`.

## 12 Тесты и race проверки

Перед изменениями были выполнены Go test, race и vet. После реализации проверяются
все Go packages, реальные PostgreSQL/Redis race scenarios, RabbitMQ confirms,
FFmpeg/MinIO acceptance и браузерные сценарии. `go test ./...`, `go test -race ./...`
с локальными PostgreSQL/Redis и `go vet ./...` прошли. Frontend: production build,
typecheck и 59 unit tests прошли. Опциональные native интеграции используют
случайную test database, Redis namespace, Rabbit exchange/queue и собственные UUID
объектов, а не существующие данные приложения.

Frontend unit suite покрывает переключение/смену устройства, поздний capture после
выхода, partial screen failure, optional screen audio, устаревшие policy, замену
receiver, запись и права кнопок. Chromium покрывает прежние восемь UI сценариев,
Stage 3 media и Stage 4 control/screen/moderation/reconnect. Firefox Playwright
на этом macOS не стартует с ошибкой `Could not find profile folder`; это не
успешная проверка Firefox, даже при успешной проверке Chromium.

Docker-проверка обнаружила превышение WebSocket burst при исходном ICE gathering
для 40 transceivers. Клиент теперь использует `bundlePolicy: max-bundle`;
лимиты сообщений не повышались. Stage 4 browser regression повторён с production
лимитами 20 сообщений/с и burst 40 и прошёл.

Отдельно проверяются редкие кадры/полностью тихие видео-сегменты и смена разрешения
камеры внутри одного источника. Диагностика сохранённого Chrome-фрагмента выявила
сброс filter graph в Docker FFmpeg 6.1.2 при переходе 480×270 → 640×360;
на локальном FFmpeg 9 этот дефект не воспроизводился. Отключение автоматического
`reinit_filter` для входа сохраняет временную шкалу композиции; тест проверяет
декодированное изображение, а не только количество source/layout entries.

Воспроизводимые команды после настройки локальных test DSN и Redis:

```bash
go test ./...
go test -race ./...
go vet ./...
go test -race ./internal/infrastructure/sfu -run TestSFUMediaSmoke -count=1 -v
RECORDER_STAGE4_RECORDING_E2E=true go test ./tests/integration -run TestStageFourCompositeRecording -count=1 -v
RECORDER_STAGE4_BROWSER_E2E=true go test ./tests/integration -run TestStageFourBrowserControls -count=1 -v
RECORDER_TEST_FFMPEG_DOCKER=go-recorder-worker go test -race ./internal/infrastructure/composite -count=1
cd frontend
npm run build
npm test
npm run test:e2e -- e2e/meet.spec.ts
MEET_STAGE4_DOCKER=true npx playwright test -c playwright.recording-docker.config.ts
```

Integration settings: `RECORDER_STAGE1_TEST_POSTGRES_DSN`,
`RECORDER_STAGE2_TEST_REDIS_ADDR`; для записи также `RECORDER_TEST_FFMPEG`,
`RECORDER_STAGE4_TEST_MINIO_ENDPOINT`, `RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY`,
`RECORDER_STAGE4_TEST_MINIO_SECRET_KEY`, `RECORDER_STAGE4_TEST_RABBIT_URL`.
Test credentials по умолчанию применимы только к локальному dev Compose.
Docker acceptance использует уже запущенный локальный Compose и создаёт отдельные
smoke-аккаунты/конференцию. После сценария cleanup проверяет точные UUID, smoke email
и title, затем удаляет только свои тестовые SQL rows, storage prefix и spool.
Скачанные файлы и отчёт остаются в `frontend/test-results/stage4-docker`.

## 13 Проверка итогового видео

Native E2E использует настоящие VP8/Opus RTP, API/WS/HTTP, PostgreSQL, Redis,
RabbitMQ, FFmpeg и MinIO. Два участника начинают передачу, включается запись,
появляется экран, третий участник входит и выходит, экран останавливается, сетка
возвращается. Проверяются duplicate start, async stop, active conference во время
обработки, ready, два подписанных скачивания, отдельный failure и finish auto-stop.

Проверка не ограничена существованием файла: ffprobe подтверждает MP4/H.264/AAC,
размер и положительную длительность обеих дорожек, допустимую разницу до 350 мс.
Проверяются manifests со screen/grid и тремя камерами, превью, DB rows и MinIO.
Допуск сравнивает длительности дорожек и не является доказательством lip sync
для любых сетевых условий или длительных сессий.

Native race acceptance после последних исправлений: H.264 640×360, AAC,
video 11.280 с, audio 11.316708 с, разница 36.7 мс, размер 1 075 171 байт.
Проверены 45 декодированных кадров с частотой
4 кадра/с: периодических чёрных провалов нет. В аудио не обнаружено провалов ≥200 мс.
Последний неполный сегмент сохранён. Для непрерывного декодирования на стыках
сегментов используются ограниченный encoded GOP pre-roll и Opus pre-roll, а не
ожидание следующего ключевого кадра с чёрной областью. Проверки failure isolation,
восстановления обработки и finish auto-stop также прошли.

Полный Docker E2E в Chromium прошёл за 30.1 с: owner login/start, второй участник,
запись, screen share, поздний третий участник, moderator mute, остановка экрана,
возврат сетки, stop → ready и finish с закрытием медиа во всех трёх браузерах.
Повторный start вернул ту же запись, а конференция оставалась active при processing.
Проверены подписанные скачивания; без JWT API вернул 401, без подписи MinIO — 403.

Итог: MP4 H.264/AAC 1280×720, video 23.000 с / audio 23.022333 с, разница 22.3 мс,
936 016 байт; JPEG preview 17 031 байт. RGB-проверка не обнаружила полностью пустых
интервалов; проверка финальной сетки подтвердила три заполненных camera tile и одно
ожидаемо пустое место сетки 2×2. Результат также просмотрен визуально. Проверяются
именно декодированные изображения: простого наличия трёх source entries недостаточно.

Локальные артефакты последнего запуска: каталог
`frontend/test-results/stage4-docker/stage-four-recording-real--e0c62-ces-private-MP4-and-preview`
с `acceptance.json`, `conference.mp4`, `preview.jpg` и кадрами QA. Каталог исключён
из Git и заменяется следующим запуском теста. Smoke-данные приложения очищены.

## 14 Регрессия на нескольких участниках

`TestSFUMediaSmoke` повторён с race detector для 2, 3 и 5 peers: все прошли,
dropped=0, после cleanup число goroutines возвращается к исходному значению 3.
Отдельный authenticated integration повторяет join/leave/reconnect через две API
реплики, Redis routing и защищённый worker. Старый двухтрековый контракт сохраняется.

## 15 Наблюдения по ресурсам

Короткий SFU smoke с race detector показал CPU time около 0.281/0.499/1.084 с
для 2/3/5 участников за 1.25/1.32/1.53 с сценария соответственно. Go heap во время
измерения — примерно 2.8/3.6/4.8 МБ. Это синтетические RTP и один тестовый процесс,
а не измерение production capacity.

Recording acceptance измеряет combined native process SFU, recorder и test clients,
Go heap/goroutines, CPU/RSS, дочерние FFmpeg, финализацию и recording drops.
Такие значения нельзя выдавать за раздельный расход контейнеров или устойчивую
нагрузку. Для эксплуатации нужны отдельные длительные измерения целевых workers.

В последнем native race acceptance: CPU time процесса 3.941 с (без дочерних FFmpeg),
RSS peak 583.2 МБ с накладными расходами race detector, Go heap 16.1 МБ,
goroutines 214 перед stop, наблюдаемое пиковое число FFmpeg 1, recording drops 0,
finalization 0.524 с. Это один короткий
тест с искусственными источниками, а не production benchmark.

Раздельные наблюдения финального Docker E2E: 9 выборок `docker stats` за короткий
сценарий; между выборками возможны более высокие пики. CPU 100% соответствует
примерно одному полностью занятому ядру, память — показателю контейнера MemUsage.

| Контейнер | Максимум CPU в выборках | Максимум памяти в выборках |
| --- | --- | --- |
| media-worker, только SFU | 8.81% | 46.39 MiB |
| recorder-worker вместе с FFmpeg | 99.95% | 153.6 MiB |

Stop → API ready занял 2.555 с. Отдельный опрос процессов каждые 250 мс дал
104 выборки, наблюдаемый максимум — один FFmpeg. Go goroutines и dropped
packets для этого Docker-прогона отдельно не измерялись; их native/SFU результаты
приведены выше. Это baseline на локальном Docker Desktop, не гарантия ёмкости
сервера и не длительный нагрузочный тест.

## 16 Известные ограничения

MVP записывает один composite на конференцию, с фиксированным разрешением/FPS и
изменением layout на границах сегментов. Source attribution заявляется клиентом.
После остановки экрана его основная область может оставаться пустой до следующей
границы сегмента (по умолчанию до 5 секунд); живые камеры продолжают отображаться.
В тестах используются искусственные устройства; ручная проверка реального Firefox,
диалога выбора экрана, системного аудио, TURN и нестабильной сети остаётся необходимой.
Screen audio зависит от браузера и выбранного источника.
Синхронизация использует RTP timestamps и время получения; точного выравнивания
часов независимых отправителей по RTCP Sender Reports пока нет.

История в UI показывает первую страницу recording API; это не общая личная библиотека.
Live capture не восстанавливается бесшовно после смерти recorder. Для восстановления
обработки нужны сохранённые закрытые сегменты на доступном новой реплике диске.
Файлы S3 завершённых записей удаляются автоматически через 7 суток после
`ended_at` (для старых задач без этой отметки — `stopped_at`, затем `updated_at`).
API выполняет очистку при старте и раз в минуту; удаляются все поколения артефактов
в пространстве записи, включая неподтверждённые поколения. Ошибки повторяются,
активные задачи и записи с действующей арендой не удаляются. Доступ к истёкшей
записи закрывается при чтении до завершения физической очистки.
Retention для локальных failed spools пока не автоматизирован.
После успешной fenced-публикации локальные файлы удаляются, если
`RECORDING_KEEP_LOCAL=false` (по умолчанию). `RECORDING_MAX_MIB` ограничивает объём
входящего RTP, а не весь диск: pre-roll, промежуточные и итоговые файлы увеличивают
потребление. Диск требует отдельной квоты и мониторинга.

После kick API больше не выдаёт ссылки участнику, но уже выданный presigned URL
действует до истечения своего срока (до 24 часов). Немедленный отзыв таких ссылок
потребует отдельного авторизованного download proxy либо иной схемы доставки.

## 17 Риски перед эксплуатацией

Нужны мониторинг FFmpeg backlog, диска, записи в failed, worker leases и очередь
outbox; нагрузочное тестирование длинных встреч; retention policy; отказоустойчивое
хранилище spool и production TLS для внутреннего egress. Настройки по умолчанию:
до 2 active recordings на recorder, 1 composition FFmpeg, 1280×720/30 FPS,
лимит входных данных 10 GiB и длительность до 4 часов. Эти лимиты не заменяют sizing.

## 18 Рекомендации для следующего этапа

До расширения продукта проверить реальные Chrome/Firefox/Safari, TURN и длительные
записи, закрепить monitoring/retention и показатели целевого сервера. Затем отдельным
заданием реализовывать waiting room, chat, attachments, read state, reactions,
notifications и scheduling, сохраняя текущую авторизацию и независимость SFU.
Работа над этими функциями в Stage 4 не начиналась.
