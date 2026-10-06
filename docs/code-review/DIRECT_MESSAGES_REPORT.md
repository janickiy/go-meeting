# Личные сообщения: отчёт реализации

Дата: 6 октября 2026. Основа: `2618053`. Реализация и проверка выполнены локально;
на `meeting.janickiy.com` этот выпуск в рамках текущей задачи не устанавливался.

Требование пользователя заменяет IA исходного промпта: самостоятельный раздел
**«Личные»**, без «Чаты» и вкладок «Личные / Встречи». Чаты встреч доступны только
в конференциях. Реализованы текст, ответы, изменение/удаление своих сообщений,
приватные вложения, история, непрочитанные и доставка вне комнаты.

## 1. Найденная архитектура

Аудит до кода: [DIRECT_MESSAGES_AUDIT.md](DIRECT_MESSAGES_AUDIT.md).
Conference chat уже имел постоянные сообщения, sequence/read cursor,
идемпотентные запросы, file upload lease, приватный MinIO, общий usecase и
React ChatPanel. Conference WS привязан к участию во встрече. Для доставки
личных сообщений вне неё используется существующий персональный Redis bus.

## 2. Выбранная стратегия

Вариант B — compatibility adapter. Существующие conference таблицы и события
сохраняются. SQL-операции ChatRepository, domain, application service, HTTP-handler
и UI общие; адаптер меняет storage scope, права и publisher. Второй независимый
messenger с копией бизнес-логики не создан. Новых runtime-зависимостей нет.

## 3. База и миграции

Аддитивная миграция 28: `conversations`, `conversation_members`,
`conversation_messages`, `conversation_attachments`. Включены индексы членства,
активности, истории, unread и cleanup. Composite FK связывают отправителя,
reply, read cursor и вложения с одной беседой. Deferred triggers проверяют ровно
двух зарегистрированных участников. Исправлены выявленные при проверке
PL/pgSQL-обращение к полям разных trigger records и явные имена циклических FK.

Up/down/up проверены на изолированной БД: старые conference messages,
attachments/read state сравниваются до и после, данные и object prefix сохранены.
Down удаляет новые личные данные и требует их копии, если уже использовался.

## 4. Одна переписка на пару

Каноническая упорядоченная пара UUID, CHECK и UNIQUE в PostgreSQL, INSERT
ON CONFLICT в транзакции. 24 одновременных A→B/B→A запроса вернули один ID.
Повторный выбор пользователя открывает ту же переписку. Self/guest/третье
членство и неполная пара отклоняются.

## 5. REST

`GET /users?search=…`, GET/POST direct conversations, GET одной беседы;
messages GET/POST/PATCH/DELETE, POST read и совместимые GET/PUT chat/read;
общие init/content/finalize/download attachments. Полные тела и ограничения:
[PERSONAL_MESSAGES.md](../PERSONAL_MESSAGES.md#rest).
Каждый маршрут требует registered account и серверную проверку membership.
Safe peer DTO содержит ID/имя; email, password hash и session metadata исключены.

## 6. Account WebSocket

POST `/api/v1/ws-ticket` → GET `/api/v1/ws?ticket=…`. Одноразовый случайный
билет с TTL и отдельным namespace, проверка Origin, registered account,
reservation limit, bounded outbound queue, writer deadline и ping/pong.
Long-lived JWT не передаётся в URL. Обновлены все Nginx-конфигурации proxy/upload;
access log не содержит query билета. Incoming application commands не разрешены.

Durable session проверяется каждые пять секунд: истечение короткого JWT не
выбивает постоянный аккаунт, logout отзывает WS. Проверено живым соединением
с JWT на две секунды: оно продолжило доставку после истечения и закрылось при logout.
Drain/Shutdown закрывают connections/subscriptions, ждут goroutines и снимают lease.

## 7. Redis и несколько API

Используется `RedisNotificationBus`, персональные каналы двух собеседников.
Событие публикуется после PostgreSQL COMMIT. В проверке получатель подключался
к двум API, не входя в conference WS; обе вкладки получили сообщения.
Третий пользователь события не получил. Offline-сообщение осталось в истории.
Presence использует отдельные leases физических account WS и Redis TIME;
последняя вкладка снимает online, аварийный остаток истекает по TTL.

## 8. Message/read/unread

Общие validation, request fingerprint, clientRequestId и версии. 12 конкурентных
повторов дали один ID, другой текст с тем же ключом — conflict. Reply только
внутри беседы; edit/delete только собственные. Проверена гонка edit/delete без
восстановления удалённого сообщения. 20 конкурентных mark-read не уменьшили
курсор. Собственные/удалённые сообщения не входят в unread. Read event обновляет
все вкладки читающего. Pagination истории и списка проверена без пропуска
и повторов на стабильном наборе данных.

## 9. Файлы и безопасность

Общие MIME/content checks, 10 МиБ, пять файлов, upload lease, finalize и cleanup.
Новый приватный prefix `attachments/direct/...`, старый conference prefix сохранён.
Проверены реальная загрузка/скачивание MinIO, двойной finalize, чужой файл,
чужой scope и signed download. Метаданные не содержат object key/token/fingerprint.
Signed URL действует пять минут и остаётся временной bearer capability до истечения.

## 10. Frontend и IA

`/personal` и `/personal/:id`, явный пункт «Личные» в боковой и мобильной навигации.
Desktop: список и переписка; mobile: список → переписка → назад.
Модальное «Новое сообщение» ищет registered users по имени с debounce.
Список показывает инициалы, имя, preview, время и unread; глобальная навигация —
общий badge. Есть in-app toast вне открытой видимой беседы.
Чат удалён из вкладок истории; вместо него ссылка на конференцию.

Мобильная высота учитывает VisualViewport и safe area. Проверены composer bounds
при 390×844 и уменьшенной высоте 390×500, отсутствие horizontal overflow при
320×640. Снимок проверен визуально. После перезагрузки история не дублируется.

## 11. Переиспользованный код

`domain/chat`, неизменённый `usecase/chat/Service`, общие ChatRepository/file
операции, app/chat.Handler и маршруты с namespace, API transport factory,
MessageThread из ChatPanel, AttachmentUploader, reply/edit/delete/composer/
date separators и file presentation. Исправлен выявленный при обобщении GORM
implicit select вычисляемого SenderName. Query keys разделены по scope/аккаунту.
Добавлены сохранение scroll при старой истории и индикатор новых сообщений.

## 12. Проверки качества

| Проверка | Результат |
| --- | --- |
| `go test -count=1 ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |
| `go vet ./...` | PASS |
| staticcheck v0.7.0 | PASS, без замечаний |
| govulncheck v1.8.0 | 0 вызываемых/импортируемых уязвимостей; 1 в required module вне вызываемого кода, как до изменений |
| Frontend lint / typecheck / build | PASS |
| Vitest | 60 файлов, 545 тестов PASS |
| Общий Playwright Chromium | 99 PASS, 11 opt-in SKIP |
| Личные сообщения, реальный API/PG/Redis/MinIO, два browser account contexts | 1 сценарий PASS: search/create/send/reply/edit/delete/file/unread/reconnect/mobile |
| Real integration `TestPersonal*` и `TestStageFiveChatFilesRead`, с race | PASS |
| Расширенная real API regression: conference/auth/invitations/recording/SFU | PASS |
| Изолированный полный recording stack | Две комнаты: decoded=2, recovery=2, auto-stop=2; PASS |

Opt-in сценарии не представлены как запущенные в общем Playwright: личный live
сценарий запускался отдельно. Real интеграции используют изолированные БД,
отдельные Redis/MinIO, собственные test resources.

Исправлены только test infrastructure причины промежуточных failures:
фоновые DM-запросы добавлены в UI fixtures; auth session bootstrap исключён
из счётчика продуктовых mutations read-only recordings; image assertion ждёт
загрузки изображения; RabbitMQ fixture ждёт AMQP listener, а не только Erlang ping.
Production API/WS не подменены моками. Готовые evidence logs находятся локально
в ignored `tmp/direct-messages-20261006/evidence/`.

## 13. Регрессии конференций

Чат с файлами, reply/edit/delete/read и burst-limit прошёл real StageFive тест.
Авторизованный SFU доставил медиа; после cleanup rooms/peers/tracks равны нулю.
Проверены StageFour media state/order/close, account boundary записи,
meeting invitations и persistent auth. Две реальные параллельные записи
декодированы, recovery закрытых сегментов и auto-stop при закрытии комнаты работают.
Existing notification UI и transport tests зелёные. Production media-worker/Pion
код не изменялся. Все 11 исходных Docker containers сохранили ID и StartedAt. Временные
Redis/MinIO и recording pod удалены вместе с их собственными томами. Перед
удалением Redis активных Pub/Sub channels не осталось; единственный клиент —
проверочный redis-cli. Итог: created container leftovers=0.

## 14. Производительность и N+1

Репозиторий списка бесед выполняет два SQL-запроса независимо от числа строк — одна bounded
projection с индексированным коррелированным unread и один total aggregate.
Дополнительные проверки auth/account имеют постоянное число запросов.
Нет запросов к БД по одному на каждый peer. Измерены 25 бесед × 100 сообщений:
EXPLAIN ANALYZE BUFFERS представительного SQL списка/unread использует
`conversation_messages_unread`,
в локальной проверке execution ~0,17–0,18 мс. Это измерение малой тестовой базы,
не прогноз производственной ёмкости. Поиск limit 20, страницы limit 100,
cleanup batches 100, WS queue 64, frontend dedup window 256.
Нет глобального broadcast DM, Pion или media sessions в этом пути.

## 15. Проверка безопасности

Проверены неавторизованный запрос, guest JWT, outsider read/send/edit/delete,
cross-scope reply/files, forged IDs, приватность поиска и escape wildcards.
Search min/max length, limit 20 и Redis rate limit; account ticket rate limit
действует между API. Проверены Origin rejection и повторное использование ticket.
Текст и displayName выводятся React как текст, не через HTML injection.
Backend ownership/membership и DB constraints авторитетны; UI не выдаёт прав.
Секреты и токены не внесены в исходники, docs или report.

## 16. Новые и изменённые файлы

Список файлов текущего diff приведён ниже. Test fixtures и harness отделены от
исполняемого приложения. Большое сокращение строк ChatRepository связано
с удалением шаблонных комментариев при выделении общих операций.

- `database/migrations/000028_personal_conversations.down.sql`
- `database/migrations/000028_personal_conversations.up.sql`
- `dockers/https/nginx.conf`
- `dockers/production/nginx.conf`
- `dockers/production/routes.conf`
- `docs/API.md`
- `docs/ARCHITECTURE.md`
- `docs/FRONTEND_GUIDE.md`
- `docs/PERSONAL_MESSAGES.md`
- `docs/USER_GUIDE.md`
- `docs/code-review/DIRECT_MESSAGES_AUDIT.md`
- `docs/code-review/DIRECT_MESSAGES_REPORT.md`
- `docs/frontend.md`
- `docs/realtime.md`
- `frontend/e2e/account-settings-modal.spec.ts`
- `frontend/e2e/calendar-dashboard.spec.ts`
- `frontend/e2e/conference-invitations.spec.ts`
- `frontend/e2e/conference-layout.spec.ts`
- `frontend/e2e/helpers/personal-background.ts`
- `frontend/e2e/history-analytics-visual.spec.ts`
- `frontend/e2e/meet.spec.ts`
- `frontend/e2e/persistent-auth.spec.ts`
- `frontend/e2e/personal-live.spec.ts`
- `frontend/e2e/presence-layout.spec.ts`
- `frontend/e2e/recording-content.spec.ts`
- `frontend/e2e/recording-controls.spec.ts`
- `frontend/e2e/recording-notifications.spec.ts`
- `frontend/e2e/recordings-layout.spec.ts`
- `frontend/e2e/user-journeys.spec.ts`
- `frontend/nginx.conf.template`
- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `frontend/src/components/AttachmentUploader.tsx`
- `frontend/src/components/ChatPanel.tsx`
- `frontend/src/components/Layout.a11y.test.tsx`
- `frontend/src/components/Layout.tsx`
- `frontend/src/pages/HistoryDetailPage.test.tsx`
- `frontend/src/pages/HistoryDetailPage.tsx`
- `frontend/src/pages/PersonalPage.tsx`
- `frontend/src/pages/personal.css`
- `frontend/src/personalRealtime.test.ts`
- `frontend/src/personalRealtime.ts`
- `frontend/src/types.ts`
- `frontend/src/workspace.css`
- `internal/app/api.go`
- `internal/app/chat/handler.go`
- `internal/app/personal/handler.go`
- `internal/domain/chat/chat.go`
- `internal/domain/personal/personal.go`
- `internal/infrastructure/postgres/chat_attachment_repository.go`
- `internal/infrastructure/postgres/chat_repository.go`
- `internal/infrastructure/postgres/chat_scope.go`
- `internal/infrastructure/postgres/personal_repository.go`
- `internal/infrastructure/redis/user_presence.go`
- `internal/infrastructure/storage/s3/attachments.go`
- `internal/infrastructure/storage/s3/personal_attachments_test.go`
- `internal/transport/http/chat_routes.go`
- `internal/transport/http/personal_routes.go`
- `internal/transport/websocket/user_handler.go`
- `internal/usecase/personal/events.go`
- `tests/integration/personal_browser_harness_test.go`
- `tests/integration/personal_messages_test.go`
- `tools/p1_audit/recording_pod.py`

## 17. Совместимость и установка

Conference REST paths/event names, storage tables и object prefix сохранены.
Завершённая встреча сохраняет read-only chat через её conference page.
Есть отдельный direct scope, поэтому guest/public meeting access не открывает DM.
История/SSE уведомления и постоянная авторизация сохраняют свои контракты.

Релиз требует обычного применения миграции 28 при старте API, сборки frontend
и обновления Nginx конфигурации. Миграция проверена в disposable test DB;
рабочая локальная и удалённая БД не изменялись. Коммит и публикация не выполнялись.

## 18. Ограничения

Redis Pub/Sub — best effort, без outbox/replay и exactly-once. История PostgreSQL
восстанавливается при reconnect; fallback polling составляет 15 с для открытых
сообщений и 30 с для badge/list. Непрочитанные считаются индексированным запросом,
без отдельного счётчика, что требует нового измерения при большой истории.
Поиск переписок фильтрует загруженные страницы. Directory ищет имя, без email.
Аватаров и индикатора online собеседника нет; account leases реализованы.
DM push/email и отдельные настройки DM уведомлений не добавлены.
Soft delete не является физическим удалением из БД. Новый retention для DM не задан.
Изменение порядка бесед во время cursor pagination возможно; frontend dedup по ID.
Проверены desktop браузер и мобильные viewport; native iOS keyboard не проверялся.
Производственный rolling rollout/большой load test этой задачей не подтверждены.

## 19. Дальнейшие работы

При отдельной задаче: блокировка пользователей и preferences DM;
при подтверждённой необходимости — typing/delivery receipts, outbox/replay,
индексирование/счётчики на больших данных, online indicator и group conversations.
В этом выпуске эти возможности не начинались.
