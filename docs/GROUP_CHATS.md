# Групповые чаты

Группы — самостоятельные `conversations.type='group'`, доступные в разделе «Личные». Фильтры **Все / Личные / Группы / Новые** не создают разные хранилища сообщений. Conference chat остаётся в конференции. Папки в этом этапе не реализованы.

## Схема и правила

Миграция **30** расширяет существующий Conversation: имя, описание, создатель, metadata version, nullable private avatar key/version, soft-delete и ключ идемпотентного создания. UUID conversation стабилен. Общие `conversation_messages`, `conversation_attachments`, reply, read cursor и версии сообщений сохраняются. `group_messages` и `folder_id` отсутствуют.

`conversation_members` получает `role=owner|admin|member` и `left_at`. Строки вышедших/исключённых сохраняются для FK авторов истории. PK исключает дубликат пользователя. Partial unique и deferred validation обеспечивают одного активного владельца и максимум 100 активных участников. Ограничения уникальной пары и ровно двух пользователей остаются для direct.

Имя: 1–50 Unicode символов; описание: до 200, с переносами строк. Составные emoji поддерживаются. NUL, неподдержанные управляющие символы и bidi controls запрещены. Только зарегистрированные аккаунты: guest не может создавать группу, становиться её участником или обращаться к групповым сообщениям/файлам.

История общая: новый участник видит существующие сообщения. При первом добавлении его read cursor начинается с текущей границы истории; старые сообщения не увеличивают unread. При возвращении cursor сохраняется, роль восстанавливается как member. Unread/list/fanout учитывают только текущее членство и неудалённую группу.

| Действие | owner | admin | member |
|---|---|---|---|
| Сообщения, файлы, история, read | Да | Да | Да |
| Edit/delete собственного сообщения | Да | Да | Да |
| Имя, описание, аватар | Да | Да | Нет |
| Добавить пользователей | Да | Да | Нет |
| Удалить обычного member | Да | Да | Нет |
| Удалить admin, назначить/снять admin | Да | Нет | Нет |
| Передать владение, удалить группу | Да | Нет | Нет |
| Выйти | После передачи, если есть другие | Да | Да |

Передача превращает прежнего owner в admin. Последний owner при выходе закрывает группу. Удаление группы атомарно закрывает доступ всем и скрывает её из списков. Сообщения и прикреплённые файлы сохраняются в рамках существующей политики истории; soft-delete не является обещанием немедленного физического уничтожения истории. Текущий аватар удалённой группы подлежит очистке.

## REST

Все пути имеют префикс `/api/v1`, требуют Bearer authorization зарегистрированного аккаунта, private/no-store и rate limits. Actor и роль определяются сервером.

| Метод и путь | Назначение / body |
|---|---|
| `GET /conversations` | Общий список; `type=direct|group`, `unreadOnly=true`, `search`, `before`, `limit` |
| `GET /conversations/:id` | Detail текущего участника |
| `POST /conversations/group` | `{clientRequestId,name,description,memberIds}`; создатель добавляется owner |
| `PATCH /conversations/:id` | `{name?,description?}` |
| `GET /conversations/:id/members` | `{status,items:[{id,displayName,role,online}]}`; до 100, owner/admin/member порядок |
| `POST /conversations/:id/members` | `{userIds}`; повтор не создаёт второе membership |
| `DELETE /conversations/:id/members/:userId` | Исключение с сохранением исторической строки |
| `PATCH /conversations/:id/members/:userId` | `{role:"admin"|"member"}` |
| `POST /conversations/:id/ownership` | `{userId}` текущего участника |
| `POST /conversations/:id/leave` | `{}` |
| `DELETE /conversations/:id` | Закрытие группы владельцем |
| `PUT /conversations/:id/avatar` | Raw JPEG/PNG до 2 MiB |
| `DELETE /conversations/:id/avatar` | Очистка ссылки на текущий аватар |
| `GET /conversations/:id/avatar/content` | Приватные байты текущего аватара |
| `GET /conversations/:id/attachments/:attachmentId/content` | Приватные байты прикреплённого файла группы |

Обычные сообщения, reply/edit/delete, history cursor, read и attachment init/upload/finalize/download используют существующие generic `/conversations/:id/...` маршруты. Cross-conversation reply/file/read запрещены. Edit/delete чужих сообщений не разрешаются ролями группы.

Create и изменения возвращают `{status,item}`; новое создание — 201, повтор того же clientRequestId/body — 200, другое содержимое с тем же ключом — 409. Leave/delete возвращают `{status}`. Cursor списка привязан к пользователю и фильтрам. Список проецирует name/member count/last sender/preview/unread без отдельного detail SQL на каждую группу; глобальный unread badge охватывает direct и group.

## Realtime и уведомления

Global User WebSocket работает через существующий Redis Pub/Sub, независимо от conference session и SFU:

- `message.created|updated|deleted`: `{conversationId,type,message}`;
- `conversation.updated`: `{conversationId,type}`;
- `conversation.member.added|updated|removed`: `{conversationId,type,userId,role?}`;
- `conversation.read.updated`: `{conversationId,type,state}`.

Запись фиксируется до публикации. Создание посылает один membership update всем активным участникам; добавление/роли посылают lifecycle updates. Исключённый получает минимальный removal event без сообщений/metadata. Повтор send не публикует второе created уведомление. Общая модель message содержит clientRequestId, и pending строка клиента сверяется с UUID запроса **и автором**.

Перед выдачей отложенного WS payload сервер повторно проверяет membership. Shared conversation lock удерживается на время ограниченного socket write; removal берёт exclusive lock. Завершённое исключение не может сопровождаться последующей выдачей очереди старому участнику. REST history/read и DTO записи также формируются под общим порядком блокировок, без чтения новой истории после потери прав.

Redis Pub/Sub остаётся механизмом уведомления, а PostgreSQL — источником состояния. При reconnect или пропуске события клиент обновляет REST данные; authoritative 403/404 закрывает группу и очищает приватные кэши. Сохраняются существующие unread badge и toast личных сообщений. Новой email-рассылки для каждого группового сообщения нет.

Presence относится к аккаунтам, включая их несколько вкладок. Список участников использует одну batch presence проверку; при недоступности Redis `online=null`, интерфейс показывает неизвестный статус. У группы нет выдуманного online статуса.

## Приватные файлы и ресурсы

Message attachments используют прежние ограничения размера/содержимого, lease/finalize и приватный bucket. Для group download выдаётся URL API с `authenticated:true`; это не MinIO bearer capability. Каждый запрос байтов проверяет текущее membership. Direct/conference подписанные ссылки сохраняют прежний контракт.

Браузер использует Bearer fetch → Blob/ObjectURL для файлов и аватаров; JWT не помещается в query. Запросы отменяются при размонтировании/потере доступа, Object URL освобождаются. Загрузка аватаров ограничена общей очередью на 6 запросов; HTTP429 повторяется максимум дважды, ожидание отменяется при unmount. Уже скачанные пользователем байты отозвать невозможно. Начатая авторизованная передача завершается или отменяется до commit удаления участника; stream ограничен 30 секундами и пулом 8 операций на API. Долгий stream может задержать изменения этого conversation до освобождения shared lock.

Аватары поддерживают JPEG/PNG, до 2 MiB и 2048×2048. Сервер декодирует и заново кодирует изображение, удаляя metadata/добавленные байты. SVG, WebP и внешние URL не принимаются. Upload ограничен двумя операциями на API.

Миграция **31** создаёт durable ledger `conversation_avatar_objects`: pending → active → retired/cleaning → cleaned. Объект регистрируется до отправки MinIO; прерванные загрузки, замены и удалённые группы очищаются ограниченными порциями. Cleanup не удаляет текущий аватар и не может активировать уже захваченный для удаления объект. Очищенные записи старше семи дней удаляются ограниченными порциями; неочищенные и текущие ссылки сохраняются.

## Rollback и будущие папки

Не откатывать API на старый direct-only binary после появления group data: его проверка существования membership может разрешить исключённому участнику доступ по tombstone. Нужен rollback binary с active/type guards либо совместимый forward fix.

Down 30 запрещён, если существуют группы; down 31 запрещён при неочищенных объектах/текущих avatar ссылках. Безопасная rehearsal на пустых таблицах использует 31 → 30 → 28 и согласованные migration ledger entries. Существующая conference история не переносится и не меняется.

Будущие папки смогут ссылаться на UUID direct/group conversation через отдельную per-user many-to-many связь. Архивация, закрепление, «пометить непрочитанным», звонки из группы и папки не представлены фиктивными действиями этого этапа.
