# Групповые чаты: аудит перед реализацией

Дата: 6 октября 2026 года. База: `82a6312`. Исходники до этого отчёта не изменялись.

## Текущая схема и переиспользование

`conversations` сейчас допускает только `direct`, содержит обязательную уникальную пару зарегистрированных пользователей. Отложенный constraint trigger требует ровно два соответствующих membership. Общие `conversation_messages`, `conversation_attachments` и read cursor уже обслуживаются через адаптер общего `chat.Service`. Сообщения имеют версии, последовательность, UUID запроса и fingerprint для повторов; reply/file/read защищены composite FK одного conversation. Отдельная таблица групповых сообщений не нужна.

`PersonalRepository` проецирует peer, последнее сообщение и unread в фиксированном числе запросов. Сейчас его JOIN, поиск клиента, `personal.Events` и frontend принимают только direct. Global User WebSocket работает через Redis Pub/Sub независимо от конференции и SFU. Presence уже принадлежит пользователю и поддерживает несколько соединений.

Frontend: `PersonalPage`/`personal.css` дают две колонки и мобильный переход; общий `MessageThread` в `ChatPanel` содержит историю, reply/edit/delete/read и composer; `AttachmentUploader` содержит progress, retry и abort. `createChatAPI("conversations")`, объединение страниц по ID/version/BigInt(sequence), Modal/focus и поиск пользователей пригодны для групп. Сейчас send идемпотентен, но визуальной pending строки нет: её нужно добавить в общий компонент.

## Предлагаемая модель

Добавить `type=group` к существующему Conversation. Сохранить ограничения direct условно по типу. Для group: название 1–50 символов, описание до 200, создатель, timestamps, необязательная версия приватного аватара, стабильный UUID. Общие membership получают типизированную роль и состояние активности. Удалённые/вышедшие membership сохраняются: сообщения и файлы имеют FK на их авторов. На активную группу приходится ровно один активный owner; PK не допускает дубликат пользователя.

Создание группы получает clientRequestId для безопасного повтора. До 100 активных участников, включая создателя. Добавляемые аккаунты проверяются сервером; guest запрещён. История общая: добавленный или вернувшийся пользователь видит прежние сообщения. Для первого добавления read cursor начинается с текущей границы, чтобы прежняя история не создавала неожиданный unread. Read cursor сохраняется при выходе и возвращении; только активное членство участвует в unread/list/fanout.

Папки не реализуются. Будущая per-user many-to-many связь Folder → Conversation сможет использовать этот UUID; `folder_id` в Conversation не добавляется. Conference остаётся существующим адаптером общего Message core, без рискованного переноса его физических таблиц.

## Права

| Роль | Права |
|---|---|
| owner | Metadata/аватар, добавление/удаление, назначение admin, передача владения, удаление группы |
| admin | Metadata/аватар, добавление и удаление обычных member; не меняет owner/admin |
| member | Сообщения/файлы/read, информация/участники, выход |

Редактирование и удаление сообщения сохраняют общую политику собственных сообщений. Владелец при других активных участниках сначала передаёт владение. Удаление группы закрывает доступ; это не обещание немедленного физического удаления ранее скачанных данных. Архивация отсутствует в текущем продукте и не добавляется фиктивной кнопкой.

## REST и UI

Сохранить `/personal` и название «Личные». Фильтры страницы: **Все | Личные | Группы | Новые**; «Новые» означает unread. Кнопка «Создать группу» без возвращения ранее удалённой кнопки нового личного сообщения. Строка группы содержит avatar/name/last sender/preview/time/unread. Общий composer и файлы используются без копирования messenger. Создание, информация и управление участниками следуют верхней половине приложенного дизайна; нижняя половина про папки не входит в этап.

Список/detail расширяются group DTO и серверными фильтрами с привязкой cursor к запросу. Добавляются group create, metadata update, members list/add/remove, role change, leave, ownership transfer и delete. Общие message/read/attachment маршруты сохраняются. Group member response использует существующую user presence, без «group online».

Аватар: ограниченный JPEG/PNG, проверка геометрии и нормализация, приватный MinIO, серверный object key и durable cleanup. Файлы группы и avatar читаются через authenticated API с проверкой текущего членства. В браузере Bearer fetch → Blob/ObjectURL с освобождением ресурсов; JWT в URL не передаётся. Existing direct/conference signed flow сохраняет совместимость.

## Realtime

После commit публикуются `message.*`, `conversation.updated`, `conversation.member.*`, `conversation.read.updated` с настоящим типом conversation. Fanout только активным участникам. Исключаемый пользователь получает отдельный служебный revoke без private message. Добавленный сразу получает группу. Global WS проверяет актуальный доступ перед доставкой отложенного private payload; reconnect восстанавливает данные REST. Клиент на revoke отменяет запросы и очищает detail/history/read/avatar/download cache.

## Найденные риски

1. **Доступ по tombstone:** существующая авторизация проверяет только наличие membership. Все операции сообщений/файлов/read/list/fanout должны проверять активность и тип.
2. **Конкурентное удаление:** сначала lock conversation, затем отдельная свежая проверка membership/role. Проверка EXISTS в том же ожидающем SELECT FOR UPDATE может использовать старый snapshot. Все изменения группы используют одинаковый порядок.
3. **Signed URL:** выданный MinIO URL сейчас действует до пяти минут после удаления. Для группы используется authenticated content proxy; ранее скачанные байты отозвать нельзя.
4. **Отложенный WS:** очередь per-user сама по себе не отзывает private payload. Добавляется membership guard.
5. **Аватар:** обычный ready message attachment истекает, а attached требует message_id. Нужен отдельный lifecycle объекта, без скрытого сообщения и публичного bucket.
6. **Rollback:** старый API не проверяет group type/активность. После migration и появления group нельзя откатываться на прежний binary: он может вновь разрешить доступ removed member. Требуется совместимый guarded rollback binary или forward fix. Additive DDL само по себе не делает rollback безопасным.
7. **Повторы:** существующий повтор send снова публикует created. Публиковать created только после нового persist; визуальный optimistic элемент сверять с ответом/realtime без искусственного sequence.
8. **Перечисление/XSS:** сохранить names-only ограниченный поиск аккаунтов, rate limits, ограниченные страницы/размеры; текст выводится React без HTML, SVG/внешние avatar URL запрещены.

## План и проверки

1. Миграция, типы/валидация, общий projection, group repository и REST с role/active guards.
2. Приватные avatar/file endpoints и освобождение объектов/потоков.
3. Типизированный fanout, lifecycle/revoke, global WS delivery guard.
4. UI фильтров/создания/info/roles, общий optimistic send, cache/resource cleanup.
5. Backend create/roles/duplicates/owner/leave/transfer/delete, cross-scope message/file/reply, removed denial, idempotency/races; Redis realtime/multiple connections; frontend filters/send/realtime/mobile/focus.
6. Реальные `go test`, `go test -race`, `go vet`, frontend tests/typecheck/build и browser проверки на изолированных PostgreSQL/Redis/MinIO. Проверять, какие integration tests реально выполняются, а не засчитывать skip.
7. Регрессии direct/conference/global WS/media/SFU/recording. Итоговый отчёт содержит результаты, изменённые файлы и ограничения; папки не начинаются автоматически.
