# API и протоколы текущего выпуска

Это каноническая точка входа в документацию REST, WebSocket, медиа и диагностики.
Подробные документы ранних этапов сохраняют контракты и историю решений, но их
вступления «ещё не реализовано», команды запуска и результаты тестов относятся
к соответствующему этапу. Текущие источники истины — зарегистрированные маршруты,
валидация конфигурации и документы выпуска, ссылки на которые приведены ниже.

## Контракты по областям

| Область | Документ | Исполняемые маршруты / источник |
| --- | --- | --- |
| Аккаунты, JWT, встречи, приглашения | [auth-conferences-api.md](auth-conferences-api.md) | [platform_routes.go](../internal/transport/http/platform_routes.go) |
| Присутствие, WS tickets, конверт событий и reconnect | [realtime.md](realtime.md) | [WebSocket transport](../internal/transport/websocket), [realtime config](../internal/config/realtime.go) |
| SFU, `media.*`, ICE/TURN, внутреннее управление | [media-sfu.md](media-sfu.md) | [mediaworker transport](../internal/transport/mediaworker), [TURN config](../internal/config/turn.go) |
| Запись, модерация, экран, recorder egress | [conference-recording.md](conference-recording.md) | [recording routes](../internal/transport/http/conference_recording_routes.go), [record models](../internal/domain/records/models.go) |
| Зал ожидания, расписание, чат, вложения, реакции, история, уведомления | [collaboration.md](collaboration.md) | [HTTP routes](../internal/transport/http), [chat routes и лимиты](../internal/transport/http/chat_routes.go) |
| Email/push/calendar, STT, ИИ, поиск | [content-integrations.md](content-integrations.md) | [integration routes](../internal/transport/http/integration_routes.go), [content routes](../internal/transport/http/content_routes.go) |
| Live captions, аналитика, переиндексация | Этот индекс и прикладные типы | [captions routes](../internal/transport/http/captions_routes.go), [captions types](../internal/domain/captions/captions.go), [intelligence config](../internal/config/meeting_intelligence.go) |
| Возможности клиента и admin | [frontend.md](frontend.md), [admin.md](operations/admin.md) | [platform status routes](../internal/transport/http/platform_status_routes.go) |

Продуктовые REST-маршруты начинаются с `/api/v1`. JSON использует camelCase.
Вход возвращает **`accessToken`**, `tokenType`, `expiresIn` и `user`; дальнейшие
запросы используют `Authorization: Bearer …`. Пароль — 8–128 Unicode-символов.
Refresh token, гостевой вход и серверное аннулирование JWT при logout отсутствуют.
Права на материалы проверяются по текущему членству и допуску, а не по наличию UUID.

WebSocket: клиент получает одноразовый билет для разрешённой конференции и
открывает `/api/v1/conferences/{id}/ws`; небраузерный клиент может использовать
Bearer-заголовок. Нельзя журналировать query с билетом. При reconnect клиент
получает свежий снимок, поскольку Redis Pub/Sub не обеспечивает историю доставки.
Уведомления используют авторизованный fetch-SSE `/api/v1/notifications/events`.

Низкоуровневый legacy recorder `/api/v1/records*` не является контрактом записи
платформенной встречи и не даёт доступа к её строкам/артефактам. Для интерфейса
используются `/api/v1/conferences/{id}/recordings*`.

## Возможности, запись и дополнительные результаты

`GET /api/v1/capabilities` требует JWT и возвращает
`{status, capabilities, buildVersion}`. Флаги: `liveCaptions`, `transcription`,
`aiSummary`, `semanticSearch`, `meetingAnalytics`; `recordingModes` сейчас
содержит `composite`, `audio_only`, `individual_tracks`, `screen_focus`.
Семантический поиск дополнительно требует доступного pgvector. Этот ответ не
предоставляет права пользователю и не доказывает готовность каждого провайдера.
Клиент также обрабатывает текущие статусы и ошибки отдельных операций.

Запись создаётся `POST /api/v1/conferences/{id}/recordings` с выбранным `mode`;
запустить её может любой joined/admitted участник с постоянным аккаунтом. Гостям
(`guest_conference_id`) запуск и остановка запрещены. Остановить запись может
инициатор (`requested_by`) или owner. Остановка асинхронная.
Публичные состояния: `starting`, `recording`, `stopping`, `processing`,
`ready`, `failed` (отмена, где применимо, — `cancelled`). Доступные файлы
различаются по режиму; нельзя требовать MP4 и preview от аудиозаписи.
Подписанные URL выдаются после проверки доступа и имеют ограниченный срок.

| Endpoint | Назначение |
| --- | --- |
| `GET/PUT /api/v1/conferences/{id}/captions` | Состояние и управление живым распознаванием согласно `canManage` |
| `GET /api/v1/conferences/{id}/captions/segments` | Сохранённые финальные реплики с курсором |
| `GET /api/v1/conferences/{id}/analytics` | Разрешённые агрегаты встречи |
| `POST /api/v1/conferences/{id}/recordings/{recordingId}/search/reindex` | Разрешённый повтор индексации |
| `GET /api/v1/admin/summary` | Агрегаты для `users.is_admin`; обычному пользователю — `403` |

Внутренние протоколы SFU/recorder используют отдельные service secrets.
Не публикуйте их через пользовательский API или proxy. Описание передачи
содержимого провайдерам: [DATA_FLOWS.md](DATA_FLOWS.md).

## Версия и эксплуатационные endpoints

| Endpoint | Доступ и ответ |
| --- | --- |
| `GET /version` | Публичные метки API: `{version,commit,buildTime}`; `Cache-Control: no-store` |
| `GET /version.json` | Публичные метки frontend-артефакта с теми же полями; ответ требует повторной проверки кеша |
| `GET /health/live` | Внутри сети сервиса; жив ли процесс |
| `GET /health/ready` | Внутри сети сервиса; готовность зависимостей и отсутствие drain |
| `GET /metrics` | Внутри сети, Bearer `METRICS_SECRET`; Prometheus |
| `GET/POST /operations/drain` | Внутри сети, Bearer `METRICS_SECRET`; чтение/включение drain, ответ `{draining,active,ready}` |

`dev`/`unknown` обозначают неполные локальные метки, а не валидированный релиз.
Frontend и API могут обновляться отдельно, поэтому smoke сверяет обе версии
с манифестом выпуска. Публичный production proxy запрещает health, metrics,
operations, internal и debug; эти проверки выполняются через закрытый доступ.
Снятие drain обратным HTTP-запросом не реализовано.

## Ошибки frontend

`POST /api/v1/client-errors` работает без JWT, чтобы учитывать отказы до входа.
Приём включается серверным `CLIENT_TELEMETRY_ENABLED`; отправка — флагом сборки
frontend. По умолчанию оба выключены. Только `Content-Type: application/json`,
один JSON-объект до 8192 байт, неизвестные поля запрещены.

```json
{
  "version": "v1.2.3",
  "route": "/meetings/:id",
  "code": "react_render_error",
  "browser": "firefox",
  "stack": [{"file": "index-abcdefgh.js", "line": 42, "column": 8}]
}
```

`code`: `uncaught_error`, `unhandled_rejection`, `react_render_error`.
`browser`: `chromium`, `firefox`, `safari`, `edge`, `other`.
`route` — фиксированный шаблон известного маршрута либо `unknown`, никогда не
реальный UUID/код приглашения. `version` соответствует
`^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$`. `stack` — максимум пять кадров: basename
собственного JS-файла до 160 байт, целые `line`/`column` от 1 до 10 000 000.
Имена функций, исходные сообщения, внешние URL, query/hash, SDP и пользовательский
текст не принимаются. Полный allowlist —
[telemetry/handler.go](../internal/app/telemetry/handler.go).

Успех — `202` без тела. Отклонение: `400` неверная схема, `413` слишком большое
тело, `415` неверный Content-Type; выключенный приём — `404` после middleware.
Redis limiter допускает 30 запросов/IP/минуту и 300 запросов/минуту суммарно;
при превышении — `429`, при недоступности limiter запрос не проходит.
Клиент дополнительно ограничивает поток пятью событиями/минуту и двадцатью за
страницу, не отправляет credentials/Referer и не следует перенаправлениям.
Счётчик: `recorder_client_errors_total{code,browser,route}`.

## Текущие ограничения по умолчанию

Это значения реализации, а не гарантии производительности. Операторские env
могут менять часть из них; актуальный файл окружения выпуска хранится вне Git.

| Область | Значение по умолчанию | Источник |
| --- | --- | --- |
| Login / register | 10 / 5 запросов за минуту на IP | [config.go](../internal/config/config.go), `RATE_LIMIT_AUTH_*` |
| Входящие WS сообщения | 20/секунду, burst 40, до 64 КиБ на сообщение | [realtime.go](../internal/config/realtime.go), `WS_*` |
| Билет WS | До 45 секунд, одноразовый | [realtime.go](../internal/config/realtime.go) |
| SFU | 10 peers на комнату, 100 комнат, 1 screen sharer | [media.go](../internal/config/media.go), `MEDIA_MAX_*` |
| Запись | 2 активные, 1 FFmpeg-конвейер, 4 часа, 10 ГиБ | [composite.go](../internal/config/composite.go), `RECORDING_*` |
| Чат: чтение / отправка / правка | 120 / 60 / 30 в минуту на user+conference | [chat_routes.go](../internal/transport/http/chat_routes.go) |
| Вложения | 10 МиБ/файл, 5/сообщение; upload init/content по 10/минуту | [collaboration.md](collaboration.md), [chat routes](../internal/transport/http/chat_routes.go) |
| Поиск | 30 запросов/минуту на пользователя | [content_routes.go](../internal/transport/http/content_routes.go) |
| OAuth start / callback | 5 / 10 запросов/минуту на пользователя | [integration_routes.go](../internal/transport/http/integration_routes.go) |
| STT записи | 2 часа, 1 ГиБ исходного видео, 256 МиБ извлечённого аудио | [product.go](../internal/config/product.go) |
| Live STT | 2 конференции, 10 сессий, до 2 часов | [meeting_intelligence.go](../internal/config/meeting_intelligence.go) |

Обработка более длинной записи не следует автоматически из увеличения одного
лимита recorder: бюджеты STT, temporary storage и провайдера отдельные.
Не игнорируйте `429` и `Retry-After`; повторять изменяющие операции следует
только с предусмотренной контрактом идемпотентностью.

Старые Postman-коллекции в `docs/postman/` относятся прежде всего к legacy
recorder и не являются полным перечнем продуктовых маршрутов. Исторические
[Stage 6](operations/production-hardening-report.md) и
[Stage 9](operations/PRODUCT_UX_RELEASE_REPORT.md) отчёты не заменяют регрессию
и smoke конкретного текущего артефакта.
