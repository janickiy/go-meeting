# Групповые чаты: итоговый отчёт

Дата реализации: 6 октября 2026 года; публикация: 7 октября 2026 года. База изменений: `82a63127c6b2b99c8fbb9c54e990584905ede297`.

## Результат

Групповые чаты реализованы поверх существующих личных сообщений. Пользователь может создать группу с названием, описанием и аватаром, добавить зарегистрированных пользователей, обмениваться сообщениями и файлами, отвечать, редактировать и удалять собственные сообщения. Владелец и администраторы управляют участниками согласно серверным правилам. Владелец может передать владение и закрыть группу.

Раздел и маршруты «Личные» / `/personal` сохранены. Добавлены фильтры **Все / Личные / Группы / Новые**. Чаты встреч остаются в конференциях. Папки из нижней половины дизайн-макета в этот этап не входят.

До изменения исходников подготовлен [аудит](GROUP_CHATS_AUDIT.md). Полный контракт и эксплуатационные правила описаны в [GROUP_CHATS.md](../GROUP_CHATS.md).

## Схема и миграции

| Миграция | Изменения |
|---|---|
| `000030_group_conversations` | `conversations.type=direct|group`; nullable direct pair только для group; название, описание, создатель, avatar key/version, версия metadata, soft-delete, идемпотентный ключ создания и fingerprint. Общие membership получают `role` и `left_at`. |
| `000031_conversation_avatar_objects` | Приватный журнал объектов аватаров: pending/active/retired/cleaning/cleaned, cleanup token, ограниченная очистка объектов и завершённых записей. |

Сообщения, вложения, reply, read cursor и message versions используют прежние `conversation_messages`, `conversation_attachments` и membership. `group_messages` не создаётся. Физические таблицы конференций не перенесены.

Уникальная пара direct и ровно два её участника сохранены. Для действующей группы БД требует ровно одного владельца и от 1 до 100 действующих участников. PK membership запрещает дубликаты. Вышедшие строки сохраняются для ссылок из истории. При закрытии группы все membership деактивируются атомарно.

Новый участник видит прежнюю историю; его первый read cursor устанавливается на текущую границу. При возвращении сохраняется прежний cursor, а роль становится `member`.

## Права

| Действие | owner | admin | member |
|---|---|---|---|
| История, сообщения, файлы, read | Да | Да | Да |
| Edit/delete собственного сообщения | Да | Да | Да |
| Название, описание, аватар | Да | Да | Нет |
| Добавление пользователей | Да | Да | Нет |
| Исключение обычного участника | Да | Да | Нет |
| Исключение admin / назначение и снятие admin | Да | Нет | Нет |
| Передача владения / закрытие группы | Да | Нет | Нет |
| Выход | После передачи, если остаются другие | Да | Да |

Передача владения делает прежнего владельца администратором. Выход единственного владельца закрывает группу. Роль не даёт права редактировать чужие сообщения. Guest аккаунты исключены из групп и поиска добавляемых пользователей. Actor, membership и роль определяются backend, а не запросом клиента.

## REST и realtime

Общие `/api/v1/conversations/:id/messages`, read и attachment маршруты переиспользованы. Новые операции: создание группы, изменение metadata, список/добавление/исключение участников, назначение admin, передача владения, выход, закрытие и приватный аватар. Точные пути и DTO — в `docs/GROUP_CHATS.md`.

Создание идемпотентно по `clientRequestId`: 201 для нового, 200 для того же тела, 409 для повторного ключа с другим содержимым. Cursor списка привязан к аккаунту и фильтрам. Список получает название, количество участников, последнего автора, preview и unread общей SQL projection; отдельных detail запросов для каждой строки нет.

Используется существующий Global User WebSocket и Redis Pub/Sub:

- `message.created|updated|deleted`;
- `conversation.updated`;
- `conversation.member.added|updated|removed`;
- `conversation.read.updated`.

В payload передаётся фактический тип `direct|group`. Публикация происходит после commit. Повтор отправки не создаёт второе `message.created`. Исключённый получает только минимальное событие удаления своего membership; активные участники получают обновления группы. Несколько соединений аккаунта и разные экземпляры API используют прежнюю инфраструктуру realtime.

Presence принадлежит аккаунту. Список участников выполняет одну batch проверку; при недоступности presence статус неизвестен. Клиент очищает группу и её приватные кэши при removal event либо authoritative REST 403/404, включая случай пропущенного WS события.

## UI и переиспользованный код

- Существующий `PersonalPage`: список/две колонки, маршруты, поиск, cursor, unread и мобильный переход. Добавлены группа в списке, фильтры и кнопка «Создать группу».
- Общий `MessageThread` / `ChatPanel`: история, bubble/composer, reply/edit/delete/read и attachment uploader. Добавлена настоящая pending строка с согласованием REST/WS по автору и `clientRequestId`.
- `GroupChats`: создание с поиском и выбором пользователей, private avatar, информация, роли/участники, metadata, передача владения, выход и удаление. Действия показываются согласно текущей роли.
- `api.ts`, auth renewal, React Query и personal realtime: общий transport, отмена запросов, cache invalidation/revoke и уведомления.
- Backend `chat.Service` и conversation adapter: одна модель сообщений для direct/group, действующие conference chat контракты сохранены.

Интерфейс следует верхней части приложенного дизайна и существующему стилю Meetrix. Проверены desktop и ширины 390/320 px. Ранее удалённые пункты меню и кнопка нового личного сообщения не возвращены.

## Исправленные риски безопасности и ресурсов

1. **Доступ по исторической строке membership:** все group операции используют текущее активное членство. Исключённый не может читать/писать историю, read state, metadata, membership или получать файлы.
2. **Конкурентное исключение:** сначала блокируется conversation, затем выполняется отдельная свежая проверка роли/членства. Чтение истории/read, projection ответа записи, WS выдача и private stream используют согласованный порядок блокировок.
3. **Projection после commit:** сообщения, изменения группы и аватара формируют ответ внутри разрешённой транзакции. Это предотвращает чтение новых данных после отзыва прав и ложный отказ уже выполненной операции. Для смены/удаления аватара отдельно проверен возврат успешного снимка после конкурентного commit исключения администратора.
4. **Отложенные WS сообщения:** проверка перед bounded socket write удерживает shared lock; завершённое исключение закрывает выдачу очереди. Служебный revoke имеет строгий минимальный allowlist полей.
5. **Подписанная ссылка после исключения:** групповые файлы выдаются через authenticated API, а не MinIO capability. Каждый запрос байтов проверяет актуальное членство. Direct/conference download совместимы с прежним signed контрактом.
6. **Аватары и незавершённые загрузки:** только PNG/JPEG до 2 MiB и 2048×2048, decode/re-encode, private bucket, серверный key, запись ledger до PUT, повторная проверка manager при activation. Текущий объект защищён от cleanup; pending/retired и удалённые группы очищаются порциями.
7. **Неограниченный рост ledger:** завершённые записи старше семи дней удаляются максимум по 25 за цикл; текущие и неочищенные сохраняются.
8. **Frontend ресурсы:** AbortSignal для приватных запросов, освобождение Blob/Object URL, отменяемая очередь аватаров максимум на 6 запросов, до двух повторов только HTTP429. После потери доступа удаляются история, read, members, detail и previews из кэшей.
9. **Optimistic correlation:** чужой автор не может погасить pending строку совпавшим UUID запроса.
10. **IDOR/XSS/перечисление:** cross-conversation reply/file/read запрещены; поля и UUID валидируются; поиск аккаунтов ограничен именами, длиной и страницей, без email; React выводит текст без HTML; SVG и внешние avatar URL запрещены; новые пути имеют auth и rate limits.

## Проверки

Проверки выполняются на локальных изолированных БД/namespace PostgreSQL и Redis, приватных объектах MinIO и реальном браузере; skip учитывается отдельно от pass. Fixtures не используют production данные. Test API, Vite, временные БД и browser credentials после сценария остановлены/удалены.

| Проверка | Результат |
|---|---|
| Полный `go test -count=1 ./...` | **PASS: 717 тестовых случаев, 57 skip, 0 fail** |
| Полный `go test -race -count=1 ./...` | **PASS: 717 тестовых случаев, 57 skip, 0 fail; DATA RACE нет** |
| `go vet ./...` | **PASS** после всех production/test изменений |
| Frontend `npm test -- --run` | **PASS: 62 файла, 602 теста** после всех frontend изменений |
| Frontend `npm run build` (включая typecheck) / `npm run lint` | **PASS** |
| Group browser, настоящий локальный API | **PASS: 2 сценария**; desktop 1440, mobile 390/320, проверенные create/info/members views — axe без нарушений |
| Existing direct browser regression | **PASS**, настоящий API/Redis/PostgreSQL/MinIO |
| Group core targeted `-race` | **PASS:** создание/constraints/100 участников/roles/owner/HTTP/idempotency/revoke и DTO snapshot, плюс domain validation |
| Assets targeted `-race` | **PASS:** private bytes, replaced/pending/current cleanup, удаление/закрытие, upload reauthorization, stream vs revoke, avatar DTO vs revoke и S3 key isolation |
| Generic history/read/write projection `-race` | **PASS:** 2 read случая и 4 send/edit/delete/mark-read случая |
| Multi-instance realtime / queued delivery | **PASS:** Redis и несколько socket соединений; удалённому запрещены последующие private события; removal сериализован с bounded write |
| Реальная запись / SFU → FFmpeg → MinIO | **PASS для всех четырёх режимов:** composite, audio_only, individual_tracks, screen_focus; проверены конечные artifacts |

Browser сценарии охватывают создание с пользователями, realtime сообщения, reply/edit, private file download, добавление/исключение, роли, изменение metadata, передачу и выход, фильтры, повторы аватара и потерю доступа при пропущенном WS. Unit проверки дополняют удаление кэша и отменяемую очередь загрузки аватаров.

Полные Go прогоны охватили все 88 пакетов; integration пакет завершился за 190,158 секунды без race и 185,726 секунды с `-race`. Реально прошли `TestPersonalMessagesRealtimeAndFiles`, `TestPersonalGlobalSessionLifecycle`, `TestPersonalMigrationPreservesConferenceChat`, `TestConferenceChatActionsAndNotifications`, `TestConferenceChatMembersLivePresenceAndAccess` и `TestStageThreeAuthenticatedSFUMedia`. Пропущенные 57 случаев требуют отдельных external-test настроек PostgreSQL/Redis/RabbitMQ, FFmpeg либо явных opt-in для browser/TURN/soak/load/semantic сценариев. Они не засчитываются как успешные. Group browser и четыре режима записи выполнены отдельно с необходимыми flags/fixtures, хотя их opt-in entrypoints пропущены в общем прогоне.

В первом объединённом recording прогоне три режима прошли; `screen_focus` потерял fixture SFU peers при нагрузке и коротком heartbeat. Отдельный прогон `screen_focus` с тем же production кодом прошёл и создал MP4/preview в MinIO. На хосте нет FFmpeg; использован действующий Docker image worker через временный wrapper с host network и доступом только к test paths. Чужие контейнеры не останавливались.

Промежуточные общие проверки выявили два старых дефекта test fixtures: query budget conference страницы остался 7 после прежнего добавления leave policy/bookmarks (фактически фиксированные 9 для 1 и 20 сообщений), а recorder startup fixture полагался на окно 250 мс. Исходный timing сбой воспроизведён контролируемой задержкой 600 мс; новый test-only barrier прошёл три повтора и `-race`. Исправлены ожидания/синхронизация и cleanup тестов, production media не изменён. Повторные общие прогоны после исправлений завершились успешно.

Логи и browser screenshots: `tmp/group-chats-20261006/` (локальные, игнорируются Git). Основные evidence: `go-test-verified.jsonl`, `go-race-verified.jsonl`, `go-vet-final.log`, `frontend-unit-final.log`, `frontend-build.log`, `frontend-lint.log`, `frontend-group-browser.log`, `core-targeted-race.log`, `evidence/group-assets-final-race.log`, `evidence/group-read-projection-race.log`, `recording-regression.log`, `recording-screen-regression.log`.

## Совместимость, ограничения и публикация

- `conversation_id` стабилен. Следующий этап реализовал папки через отдельную per-user many-to-many связь с direct/group; `folder_id` в conversation не добавлен. Контракт описан в [PERSONAL_FOLDERS.md](../PERSONAL_FOLDERS.md).
- Максимум 100 активных участников. Текст и файлы работают в рамках существующих ограничений общего чата.
- История видна новым участникам. Закрытие группы отзывает доступ и скрывает её, сохраняя исторические сообщения/вложения по текущей политике. Уже скачанные байты отозвать невозможно.
- Авторизованный поток завершается или отменяется до commit исключения, с лимитом 30 секунд и 8 потоков на API. Долгий поток может задержать изменения того же conversation до освобождения shared lock. Upload аватара ограничен двумя операциями на API.
- Redis Pub/Sub не является durable outbox. REST остаётся источником состояния; после reconnect/пропуска события клиент сверяет доступ и данные. Новой email-рассылки для каждого сообщения нет.
- Архивация, закрепление, ручная отметка unread и групповые звонки не имеют поддержки текущего backend и не представлены фиктивными кнопками. Папки реализованы отдельным следующим этапом и опубликованы вместе с группами.
- После появления group data нельзя откатывать API на прежний direct-only binary: старая проверка наличия membership может вновь открыть доступ исключённым пользователям. Нужен совместимый binary с active/type guards либо forward fix. Down30/31 защищены от удаления действующих group/asset данных.
- 7 октября группы и папки опубликованы на `meeting.janickiy.com`; история исходной рабочей копии, Compose и SMTP сохранены. Серверная база обновлена миграциями 30–32 после проверки резервной копии.

## Публикация — 7 октября 2026, 00:51 МСК

API/frontend установлены в версии **`v1.0.0-meeting.20261007-folders.1`**, snapshot `889caa37c795d50d75d46356366377c6f77816b1`. Manifest SHA-256: `b3a2343aa296fe77d44ad13961d890c366aaeb3488833cd1da736a6611c4913b`. Публикация включает описанные группы и последующий этап папок; общий финальный набор регрессий приведён в [отчёте папок](PERSONAL_FOLDERS_REPORT.md).

До миграций API штатно завершён и PostgreSQL сохранён вне VPS. В отдельной временной базе восстановлены 29 миграций и сверены 41 прежняя таблица с источником; новый бинарник применил миграции 30–32 с сохранением данных, повторный запуск ничего не изменил. Рабочую базу резервной копией не заменяли. Остальные 17 контейнеров, защищённые конфигурации, Compose, SMTP и proxy сохранены. После повторной проверки свободно 1 193 668 608 байт; резерв 1 ГиБ соблюдён.

Все 19 контейнеров работают, 11 healthcheck healthy. API/frontend version и commit совпали с manifest; публичные version/health запросы успешны, анонимные folder/conversation API возвращают 401, закрытые служебные пути — 404. Новый production звонок между реальными устройствами в рамках публикации не выполнялся.

На реальном опубликованном сайте с настоящим API прошли 33 проверки групп/папок, включая девять axe WCAG проверок без нарушений. Проверены информация и роли группы, добавление доступной группы в папку, исключение участника с отказом в чтении и скрытием группы/счётчика из его папки. Desktop 1440 и mobile 390/320 px — без горизонтального переполнения, browser runtime errors отсутствуют. Использовались временные тестовые аккаунты; сообщения, email, uploads и звонки в этом smoke не запускались. Evidence — `prod-smoke-result.json` и скриншоты рядом.

Точные временные объекты smoke удалены после проверки ID/владельцев в одной транзакции: четыре аккаунта, четыре переписки, две неактивные встречи и три папки. Все 41 исходная таблица после очистки совпали по fingerprints с состоянием до установки; четыре новые таблицы пусты. Browser contexts закрыты, credentials удалены. Evidence — `cleanup-smoke-result.json`.

Свежие Trivy/SBOM для обоих amd64 образов: 0 HIGH / 0 CRITICAL; восемь проверок установщика — PASS. После попытки новой миграции предусмотрено только совместимое исправление следующей версией, без автоматического возврата direct-only API.

Evidence: `tmp/folders-deploy-20261007/evidence/`; подробности — [история установки](../operations/meeting-host-deployment.md). Сведения о публикации добавлены в отчёт после сборки и не меняют provenance установленных образов.

## Изменённые файлы

Полный список исходников, миграций, тестов и документации (60 файлов). Локальные test logs/screenshots и временные credentials не входят в исходники.

```text
database/migrations/000030_group_conversations.down.sql
database/migrations/000030_group_conversations.up.sql
database/migrations/000031_conversation_avatar_objects.down.sql
database/migrations/000031_conversation_avatar_objects.up.sql
docs/GROUP_CHATS.md
docs/code-review/GROUP_CHATS_AUDIT.md
docs/code-review/GROUP_CHATS_REPORT.md
frontend/e2e/group-live.spec.ts
frontend/src/api.test.ts
frontend/src/api.ts
frontend/src/components/ChatPanel.redesign.test.tsx
frontend/src/components/ChatPanel.tsx
frontend/src/components/GroupChats.test.tsx
frontend/src/components/GroupChats.tsx
frontend/src/components/group-chats.css
frontend/src/groupAvatarLoader.test.ts
frontend/src/groupAvatarLoader.ts
frontend/src/pages/PersonalPage.tsx
frontend/src/personalRealtime.test.ts
frontend/src/personalRealtime.ts
frontend/src/types.ts
internal/app/api.go
internal/app/chat/group_downloads.go
internal/app/chat/handler.go
internal/app/personal/assets_handler.go
internal/app/personal/group_handler.go
internal/app/personal/handler.go
internal/domain/personal/groups.go
internal/domain/personal/groups_test.go
internal/domain/personal/personal.go
internal/infrastructure/postgres/chat_repository.go
internal/infrastructure/postgres/chat_scope.go
internal/infrastructure/postgres/group_repository.go
internal/infrastructure/postgres/personal_assets_repository.go
internal/infrastructure/postgres/personal_delivery.go
internal/infrastructure/postgres/personal_repository.go
internal/infrastructure/storage/s3/personal_assets.go
internal/infrastructure/storage/s3/personal_assets_test.go
internal/infrastructure/webrtc/recording_lifecycle_test.go
internal/transport/http/personal_asset_routes.go
internal/transport/http/personal_routes.go
internal/transport/websocket/user_delivery.go
internal/transport/websocket/user_delivery_test.go
internal/transport/websocket/user_handler.go
internal/usecase/chat/send_retry_test.go
internal/usecase/chat/service.go
internal/usecase/personal/assets.go
internal/usecase/personal/assets_test.go
internal/usecase/personal/events.go
internal/usecase/personal/events_test.go
tests/integration/group_assets_test.go
tests/integration/group_avatar_projection_race_test.go
tests/integration/group_browser_harness_test.go
tests/integration/group_chats_test.go
tests/integration/group_delivery_race_test.go
tests/integration/group_projection_race_test.go
tests/integration/group_read_race_test.go
tests/integration/group_realtime_test.go
tests/integration/p1_recordings_batch_test.go
tests/integration/personal_messages_test.go
```
