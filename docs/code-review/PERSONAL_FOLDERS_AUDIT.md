# Папки личного пространства: аудит до реализации

Дата: 7 октября 2026 года. HEAD: `82a6312`; актуальная база включает завершённые, ещё не закоммиченные группы (Git tree `9bb01f315a8bdcea2ec47e0953782104d71605b3`). До этого отчёта исходники этапа папок не изменялись.

## Фактическая структура

`Layout` содержит один блок «Рабочее пространство»: Главная, Встречи, Личные, Календарь, Записи, История, условная Аналитика, Настройки и условное Администрирование. `/conferences` и `/meetings` используют Dashboard и один Conference UUID; личные сообщения и группы — `/personal/:id`. Settings открывает существующий account modal. Ранее удалённые Search/Notifications/profile dropdown не возвращаются.

Direct/group используют `conversations.id` и общий Message core; тип и текущая активность membership проверяются через `conversation_members.left_at` и `conversations.deleted_at`. Встречи живут отдельно в `conferences`, их conference chat использует тот же UUID. Папка не получает сообщения и не создаёт второй item для чата встречи. Записи и файлы не добавляются автоматически.

Dashboard/Timeline показывает собственные membership `admitted|waiting`, включая приглашённых до подключения (`left/admitted`), и исключает явный выход из conference chat. `Service.Read` проверки папок заменить не может: по наличию membership он возвращает базовые metadata даже kicked/rejected. Папки используют list-style predicate с дополнительным исключением kicked/rejected и guest аккаунтов. Обычный выход из звонка (`left/admitted`) не равен потере доступа к истории. Waiting допускает базовую карточку, но не participant count/history/invite secrets.

Frontend переиспользует Modal/focus, Button, Loading/ErrorNotice, ConversationAvatar с ограниченной загрузкой, Conference/StatusBadge, cursor transport/auth renewal и существующие переходы в чат/встречу/историю. Контекстное меню встреч уже есть в `ConferenceChatActions`; у direct/group будет отдельная кнопка рядом со ссылкой строки, а не вложенная в anchor.

## Модель и семантика

Миграция **32**:

- `folders(id,user_id,name,name_key,position,created_at,updated_at)`;
- `folder_conversations(folder_id,conversation_id,created_at)`;
- `folder_conferences(folder_id,conference_id,created_at)`.

У каждой связи настоящий FK и составной PK, исключающий дубликат. Удаление папки каскадно удаляет только её связи; физическое удаление target может удалить соответствующие связи, не остальные папки. `folder_id` в conversations/conferences не появляется. Один item допускается в нескольких папках.

Название: trim + NFC, 1–50 Unicode символов, plain text, без управляющих/bidi символов. Сервер формирует NFC Unicode case-fold key; «Работа» и «работа» у одного пользователя считаются дубликатом. Между пользователями названия независимы. Порядок папок сохраняется; максимум 100 папок на аккаунт позволяет выдавать picker одной ограниченной страницей. Создание/удаление/reorder сериализуются по owner row с `NO KEY UPDATE`, совместимым с FK key-share locks.

Порядок папок изменяется доступными кнопками вверх/вниз через единую операцию reorder полного набора ID. Внутри папки — recent activity/date, keyset страницы до 100 элементов; ненужные mapping ranks и drag/drop не добавляются. Потерявшие доступ связи скрываются во всех items/counts/picker запросах. Они могут восстановиться при возвращении доступа; это не grants и не изменение других пользователей.

## API

Все пути `/api/v1`, auth зарегистрированного аккаунта, owner scope, private/no-store и rate limits:

| Метод/путь | Контракт |
|---|---|
| GET `/folders` | До 100 папок с доступными counts; опциональные `itemKind`/`itemId` добавляют `contains` для checkbox без per-folder GET |
| POST `/folders` | `{name}` → `{status,item}` |
| GET `/folders/:id` | Metadata и доступные counts |
| PATCH `/folders/:id` | `{name}` |
| DELETE `/folders/:id` | Только папка и связи |
| PUT `/folders/order` | `{ids:[...]}` полный актуальный набор; сохранённый порядок |
| GET `/folders/:id/items` | `type=all|conversation|conference`, `search`, `before`, `limit`; union страницы |
| PUT/DELETE `/folders/:id/items/:kind/:itemId` | Идемпотентное добавление/удаление связи; enum kind только conversation/conference |
| GET `/folder-items` | Бounded authorized candidates с type/search/cursor и optional `folderId`/`inFolder` для добавления из папки |

Последний общий candidate endpoint нужен для server search встреч: текущий Dashboard ищет только среди уже загруженных страниц. Он переиспользует metadata, не дублирует meeting/message operations. Folder item: `{type:'conversation'|'conference',item:...}`; optional `inFolder` только в кандидате. Folder DTO: id/name/position/timestamps/itemCount/conversationCount/conferenceCount, optional contains. Conference проекция не выдаёт invite secrets; participant count — только при допущенном history access, без имён участников.

Cursor связан с actor, folder/candidate scope, type/search, activity/date, item kind и UUID: ID двух таблиц теоретически может совпадать. Системные Все/Личные/Группы/Новые остаются виртуальными фильтрами messaging.

## Query/index/security план

Индексы: owner/position/id, unique owner/name_key, mapping PK и обратные target/folder. Counts вычисляются по тем же доступным наборам, что items. Metadata проецируется с актуальным membership predicate до LIMIT, без authorize → raw Get gap и отдельных запросов на строку. Mixed detail ограничен keyset страницей. EXPLAIN на representative PostgreSQL fixture проверит доступные наборы, rows/buffers и порядок страниц; маленькая таблица не обязана использовать Index Scan.

Каждый CRUD/map/reorder/picker/cursor ограничен actor. Добавление берёт item shared lock, затем свежую проверку доступа и owner-scoped folder lock в одной транзакции. Не выполняет join/admit/role change. FK сам по себе не grants. Удаление mapping не требует сохранённого доступа к target, но требует владения папкой. При counts/item projection скрываются group tombstones/deleted и meeting kicked/rejected/chat-left; сырой mapping count не выдаётся.

Frontend на group revoke очищает также folder/candidate previews/counts; authoritative 403/404 закрывает и удаляет private cache. Все requests имеют AbortSignal и user-scoped keys. Folder changes можно посылать через существующий per-user Global WS (`folder.*`, только actor, minimal IDs); никаких Pion/conference WS изменений. Внешняя потеря meeting access сверяется REST при фокусе/видимом refresh.

## UI и реализация

Отдельный блок «Личное пространство» → один пункт «Папки»; Settings и условные служебные пункты сохраняются. Защищённые `/folders` и `/folders/:id`; список/create/rename/delete/reorder, detail Все/Встречи/Чаты, существующие карты и переходы, add/remove mapping. Общие dialogs/picker используются из folder UI, conversation menu и meeting menu, с checkbox state, ошибкой и безопасным повтором. Из макета используются белые rounded panels, blue accent и mobile layout; description/color/pin не добавляются без соответствующего контракта.

Последовательность: migration/domain/repository; HTTP/bootstrap/minimal owner WS; frontend/menus/cache; actual PG/Redis/browser/security/performance; full Go/race/vet + frontend tests/typecheck/lint/build; direct/group/conference/meeting/global WS/SFU/recording regression; финальный отчёт. Исходники предыдущего этапа сохраняются. Production публикация не является частью этого промпта.
