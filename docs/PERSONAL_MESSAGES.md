# Личные сообщения

Реализовано 6 октября 2026. Отдельный раздел **«Личные»**, без общего раздела
«Чаты» и вкладок «Личные / Встречи». Чаты встреч открываются только в конференциях,
в том числе для чтения завершённой встречи.

## Модель и совместимость

Выбран compatibility adapter. Существующие `chat_messages`, `chat_attachments`,
`chat_read_states`, REST конференций и события `chat.*` сохраняются.
Общие `domain/chat`, `usecase/chat`, SQL-операции `ChatRepository`, HTTP handler,
React `MessageThread` и `AttachmentUploader` обслуживают оба scope.
`NewDirectChatRepository` меняет только таблицы и проверку прав.

Миграция `000028_personal_conversations` добавляет:

| Таблица | Назначение |
| --- | --- |
| `conversations` | `type=direct`, каноническая пара зарегистрированных пользователей, указатель последнего сообщения |
| `conversation_members` | Ровно два участника, курсор прочтения и sequence |
| `conversation_messages` | Общая модель сообщения, ключ повтора, версия, reply, мягкое удаление |
| `conversation_attachments` | Общая модель приватного вложения, upload lease, статус и cleanup |

`user_low_id < user_high_id` и UNIQUE пары обеспечивают одну беседу A↔B.
GetOrCreate использует INSERT ON CONFLICT внутри транзакции PostgreSQL.
Отложенные constraint triggers запрещают неполную пару, третьего участника и
гостевые аккаунты. Composite FK ограничивают отправителя, reply, read cursor,
последнее сообщение и вложения одним scope. Переписка с собой запрещена.

Миграции применяет существующий runner под advisory lock. Up не переносит и
не удаляет данные конференций. Down удаляет **данные личных сообщений** вместе
с новыми таблицами; перед таким откатом нужна копия этих данных. Проверка up/down/up
на изолированной БД подтверждает сохранение старых сообщений, вложений и read state.

## REST

Все пути ниже имеют префикс `/api/v1`, требуют Bearer JWT зарегистрированного
аккаунта и возвращают `Cache-Control: private, no-store`. Гостевого JWT недостаточно.
Backend проверяет членство в каждой операции, независимо от UI.

| Метод и путь | Контракт |
| --- | --- |
| `GET /users?search=…` | `{status,items:[{id,displayName}]}`; имя, 2–100 Unicode-символов, максимум 20 результатов, без гостей и самого пользователя |
| `GET /conversations?limit=50&before=…` | `{items,nextCursor?,unreadCount}`; limit 1–100, курсор привязан к аккаунту |
| `POST /conversations/direct` | `{userId}` → `{status,item}`; 201 при создании, 200 для существующей пары |
| `GET /conversations/{id}` | `{status,item}`; safe peer summary, preview, время, unread |
| `GET /conversations/{id}/messages?limit=50&before=…` | Общая cursor pagination истории: `{status,items,nextCursor,unreadCount,lastReadMessageId}` |
| `POST /conversations/{id}/messages` | `{clientRequestId,text,replyTo?,attachmentIds?}` → сохранённое сообщение |
| `PATCH /conversations/{id}/messages/{messageId}` | `{text}`; только своё сообщение |
| `DELETE /conversations/{id}/messages/{messageId}` | Мягкое удаление; только своё сообщение, роль модератора отсутствует |
| `POST /conversations/{id}/read` | `{messageId}`; монотонное продвижение курсора |
| `GET /conversations/{id}/chat/read` | Состояние прочтения и unread |
| `PUT /conversations/{id}/chat/read` | Alias общей операции mark-read |
| `POST /conversations/{id}/attachments/init` | Общая инициализация файла и серверный upload URL |
| `PUT /conversations/{id}/attachments/{attachmentId}/content` | Двоичные данные, bounded upload |
| `POST /conversations/{id}/attachments/{attachmentId}/finalize` | Проверка фактически загруженного файла; повтор безопасен |
| `GET /conversations/{id}/attachments/{attachmentId}/download` | Временный подписанный URL после проверки прав |

Email, password hash, роли и сессии не входят в peer DTO и не участвуют в поиске.
Символы `%`, `_`, `\` экранируются в ILIKE. Поиск ограничен 30 запросами/минуту
на аккаунт; создание — 30, список/детали — 120, mark-read — 30.
Общие лимиты на scope/аккаунт: отправка 60/мин, edit/delete 30, upload init/content
10, finalize 20, download 60. Общий HTTP middleware сохраняет существующие ограничения.

## Сообщения, прочтение и список

Текст — до 4000 Unicode-символов. `clientRequestId` — UUID, уникальный для пары
scope/отправитель/запрос. Повтор того же нормализованного содержимого возвращает
тот же ID; изменение содержимого с тем же ключом вызывает conflict.
Reply разрешён только внутри своей переписки. Edit/delete сохраняют общие
семантики версии и tombstone. История ограничена страницами до 100 сообщений.

Read cursor хранится как ID и sequence. Запоздалый запрос не уменьшает sequence.
Unread считает чужие неудалённые сообщения выше этого курсора. Изменение read
публикуется всем вкладкам читающего аккаунта; публичных delivery receipts нет.

Список сортируется по `COALESCE(last_message_at,created_at) DESC,id DESC`:
пустые беседы имеют время создания. Metadata последнего сообщения обновляется
в транзакции отправки. Страница использует одну SQL projection с join и
индексированным unread подзапросом, общий badge — второй агрегатный SQL.
Отдельных запросов на каждого собеседника нет. При активной переписке страницы
могут менять порядок; frontend объединяет их по ID и обновляет после событий.

## Приватные файлы

Переиспользованы существующие init → upload → finalize → attach, проверка
содержимого/MIME, upload lease, idempotency и cleanup. До пяти файлов по 10 МиБ.
Object key формирует сервер:
`attachments/direct/{conversationId}/{attachmentId}/{uploadToken}`.
Префикс конференций `attachments/{conferenceId}/{attachmentId}/` сохранён.

Метаданные и download доступны только собеседникам; upload/finalize требуют
владельца файла. Cross-scope reply/attachment ID запрещены и приложением, и FK.
Бакет приватный; download URL действует пять минут. Подписанный URL — временная
bearer capability: обладатель ссылки сможет использовать её до истечения срока.
В JSON не выдаются object key, upload lease/token и request fingerprint.
Незавершённые/лишние объекты убирает общий bounded cleanup, отдельный цикл
личных вложений завершается по context API. Прикреплённые сообщения не имеют
нового срока автоматического удаления в рамках этой задачи.

## Account WebSocket

1. `POST /api/v1/ws-ticket` с действующим Bearer JWT → 201 `{ticket,expiresAt}`.
2. `GET /api/v1/ws?ticket=…` с разрешённым Origin → upgrade.

Билет случайный, одноразовый, TTL из `WS_TICKET_TTL` (по умолчанию 45 секунд),
отдельный Redis namespace `WS_REDIS_NAMESPACE:user-ws`. До 30 выдач/минуту.
JWT, Authorization query и дополнительные query параметры не принимаются.
Nginx пишет `$uri` без ticket query и ограничивает upload обоих namespace.

Серверный поток не принимает application commands: mutations идут через REST.
Формат — существующий envelope `{version,id,type,timestamp,data}`.
Conference envelope/event contracts не меняются.

| Событие | `data` |
| --- | --- |
| `message.created`, `message.updated`, `message.deleted` | `{conversationId,type:"direct",message}` |
| `conversation.updated` | `{conversationId,type:"direct"}` при создании пары |
| `conversation.read.updated` | `{conversationId,type:"direct",state}` для читающего аккаунта |
| `user.presence` | `{userId,online,connections}` для собственных вкладок |
| Существующие notification events | Существующий персональный контракт без изменений |

Последовательность: REST → usecase → PostgreSQL COMMIT → lookup двух участников
→ Redis персональные каналы → WS на каждом API. Используется общий
`RedisNotificationBus`; глобального broadcast всех DM всем аккаунтам нет.
Каждая вкладка имеет acknowledged Pub/Sub subscription. Redis не хранит историю
событий и не обещает exactly-once. После reconnect frontend перечитывает
PostgreSQL; потеря события не удаляет сообщение. Уведомления frontend по-прежнему
обрабатывает через SSE, поэтому account WS не создаёт дубликаты notification toast.

Outbound queue ограничена `WS_QUEUE_SIZE` (64 по умолчанию), размер —
`WS_MAX_OUTBOUND_BYTES` (262144), deadline — `WS_WRITE_TIMEOUT`.
Медленный клиент с переполненной очередью закрывается и восстанавливает историю.
На account stream действует `WS_MAX_CONNECTIONS` отдельно от conference stream.
Native ping/pong поддерживает lease. Команды клиента отклоняются закрытием сокета.

Presence: Redis sorted set `WS_REDIS_NAMESPACE:user-presence:{userId}` содержит
lease каждого физического соединения. Lua использует Redis TIME, атомарно
удаляет истёкшие lease, обновляет/удаляет текущий и задаёт физический TTL.
Закрытие одной вкладки оставляет аккаунт online при наличии другой.
`WS_SESSION_TTL` по умолчанию пять секунд; аварийный остаток истекает сам.
Индикатор присутствия собеседника в UI пока не выводится.

Раз в пять секунд сервер проверяет существующую durable session и аккаунт.
JWT с session ID может истечь во время работы WS; отзыв сессии/logout закрывает
его при следующей проверке. Legacy JWT ограничен своим expiresAt.
Drain прекращает upgrade, отменяет открытые connections; Shutdown ждёт cleanup.
Cleanup закрывает socket и Pub/Sub, останавливает goroutines/tickers, снимает lease
и освобождает connection reservation. Этот путь не использует Pion/media-worker.

## Frontend

`/personal` — список; `/personal/:id` — переписка. На desktop два столбца, на
телефоне один экран и стрелка назад. VisualViewport resize/scroll учитывает
видимую высоту, composer использует safe area. Нижняя навигация содержит «Личные».
Поиск переписок локальный по загруженным страницам; поиск нового собеседника —
серверный с debounce 300 мс. Аватар — инициалы, отдельного avatar storage нет.

`api.ts` создаёт общий chat transport по namespace. `ChatPanel` конференции —
обёртка того же `MessageThread`, различия передаются props. Общие message bubble,
reply/edit/delete, composer, файлы, история, error/retry не скопированы.
Query keys разделены по scope, ID беседы и аккаунту. Ошибка отправки не очищает
черновик и не изображает подтверждённую отправку; повтор использует тот же request ID.

`usePersonalRealtime` в Layout: один account WS на вкладку зарегистрированного
пользователя, fresh ticket при reconnect, exponential backoff с jitter до ~30 с,
очистка при logout/unmount. Окно дедупликации ограничено 256 ID/версий; история
рисуется по серверным ID. Badge перечитывается по событиям и каждые 30 секунд.
Toast нового чужого сообщения подавлен в открытой видимой переписке.
Push и email на каждое DM не добавлены.

При открытии выполняется прокрутка вниз, старые страницы сохраняют позицию.
Входящее сообщение прокручивает вниз только рядом с концом; иначе доступна
кнопка «Новые сообщения». Общего раздела встречных чатов или вкладки чата в истории нет.

## Проверка

Проверки и измерения: [аудит до реализации](code-review/DIRECT_MESSAGES_AUDIT.md)
и [отчёт реализации](code-review/DIRECT_MESSAGES_REPORT.md).
Real PostgreSQL/Redis/MinIO integration opt-in: `RECORDER_DIRECT_E2E=true`.
Чат конференций с файлами: `RECORDER_STAGE5_CHAT_E2E=true`.
Используйте изолированные тестовые сервисы и переменные окружения, принятые
integration harness; не запускайте разрушительные DB fixtures в production БД.
