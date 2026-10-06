# Папки личного пространства — итоговый отчёт

Дата: 7 октября 2026 года. Реализован `CODEX_PROMPT_CHAT_FOLDERS.md`.

База сравнения: Git tree `9bb01f315a8bdcea2ec47e0953782104d71605b3` — состояние после реализации групп, до начала папок. HEAD рабочей копии остаётся `82a6312`; незакоммиченные изменения предыдущего этапа сохранены. 7 октября группы и папки опубликованы на `meeting.janickiy.com` отдельным чистым build snapshot; история исходной рабочей копии не изменена. Подробности публикации — в разделе 12.

## 1. Результат

Добавлен отдельный блок **«Личное пространство» → «Папки»**. Папки принадлежат аккаунту и поддерживают личные чаты, группы и встречи. Чат встречи и встреча представлены одним элементом `conferences.id`.

- `/folders`: создание, список со счётчиками, переименование, удаление, сохранённый порядок кнопками вверх/вниз.
- `/folders/:id`: общая выдача, разделы «Встречи» и «Чаты», фильтры «Все / Встречи / Чаты», поиск, следующая страница, добавление и удаление связей.
- «Добавить в папку» в меню чата и встречи: checkbox нескольких папок и создание новой папки в общем диалоге.
- Карточки открывают существующий чат, встречу или историю. В папках нет своего чата, загрузки файлов или медиасоединения.
- Удаление папки или элемента из неё сохраняет исходный чат, встречу и сообщения. Один элемент может находиться в нескольких папках.
- Состав, названия и порядок папок независимы у разных пользователей; папка не предоставляет доступ к своим элементам.

Сценарий Definition of Done выполнен в настоящем браузере с настоящим API: создан «Проект Meetrix», добавлены личный чат, группа и встреча; проверены фильтры, вторая папка с тем же чатом, удаление одной связи, переименование и удаление папки, сохранность чатов и независимость другого пользователя.

## 2. Аудит и документация

До изменения исходников подготовлен [аудит](PERSONAL_FOLDERS_AUDIT.md): фактический sidebar, маршруты и модели, правила доступа к чатам и встречам, схема/FK, API, миграция, безопасность, индексы и план реализации.

Полный контракт и семантика: [PERSONAL_FOLDERS.md](../PERSONAL_FOLDERS.md).

Из дизайн-референса использованы отдельный пункт навигации, светлые панели, синий акцент, список смешанных элементов и диалоги. Виртуальные фильтры сообщений остаются фильтрами, без создания системных папок.

## 3. Схема и миграция

Миграция **000032_personal_folders** добавляет:

| Таблица | Назначение |
|---|---|
| `folders` | Владелец, название и нормализованный ключ, порядок, timestamps |
| `folder_conversations` | Настоящие FK к папке и direct/group conversation, составной PK |
| `folder_conferences` | Настоящие FK к папке и conference, составной PK |

Индексы: owner/position/id, unique owner/name_key, deferrable unique owner/position, PK связей и обратные target/folder. Удаление папки каскадно удаляет только её связи; физическое удаление исходного объекта убирает соответствующие ссылки. Сообщения не копируются.

Название: trim + NFC, 1–50 Unicode символов, без управляющих/bidi символов; Unicode case-fold запрещает «Работа» и «работа» в одном аккаунте. Между аккаунтами совпадения допустимы. Лимит — 100 папок на аккаунт.

Down-миграция отказывается удалять таблицы при наличии пользовательских папок. Проверены отказ с сохранением данных и безопасный down/reapply на пустых таблицах. Для установленной версии с данными предусмотрено исправление следующей миграцией.

## 4. API и синхронизация

Все пути под `/api/v1`, только зарегистрированный аккаунт, owner scope, `private, no-store`, `nosniff`, ограничения частоты.

| Метод | Путь |
|---|---|
| GET / POST | `/folders` |
| GET / PATCH / DELETE | `/folders/:id` |
| PUT | `/folders/order` |
| GET | `/folders/:id/items` |
| PUT / DELETE | `/folders/:id/items/:kind/:itemId` |
| GET | `/folder-items` |

Список папок может сразу вернуть `contains` для checkbox. Общий endpoint кандидатов поддерживает `inFolder`; отдельного запроса на каждую папку или карточку нет. Detail/candidates поддерживают тип, literal search, cursor и limit 1–100. Cursor связан с аккаунтом, областью, папкой, фильтром и ключом сортировки. Reorder принимает полный актуальный набор ID владельца.

Изменения публикуются после записи через существующий общий user WebSocket: `folder.created`, `folder.updated`, `folder.deleted`, `folder.items.updated`, `folder.reordered`. Payload содержит только ID, получатель — владелец. Проверены две API-инстанции с Redis и два соединения владельца; другой пользователь событий не получил.

React Query keys включают actor ID, запросы отменяются через AbortSignal. Смена аккаунта закрывает старые формы и сбрасывает локальные данные. При потере группового доступа очищаются previews; authoritative ошибки не сохраняют устаревшие карточки. Для внешних изменений встреч предусмотрены refetch при фокусе и каждые 15 секунд на видимой странице. REST остаётся источником данных.

## 5. Безопасность и жизненный цикл

Проверены CRUD/map/reorder/candidate IDOR, курсоры другого аккаунта, гостевые аккаунты, недоступные и удалённые элементы, выход из чата встречи, kicked/rejected/waiting состояния, смена аккаунта и потеря WebSocket-события.

- Чужая и неизвестная папка одинаково возвращают `404`.
- Direct/group видимы только при текущем membership и отсутствии удаления conversation.
- Встречи используют доступ списка: admitted/waiting, без kicked/rejected и явного выхода из conference chat. Обычный выход из звонка и приглашение до подключения не считаются потерей доступа.
- Waiting видит базовую карточку без invite secrets и числа участников. Имена участников в folder DTO не выдаются.
- Один SQL snapshot проверяет доступ до metadata, counts, сортировки и LIMIT. Сохранённая связь не заменяет membership.
- Добавление связи проверяет владельца папки, блокирует target и заново проверяет доступ в транзакции. Установлен порядок блокировок для конкурентного revoke/add; owner `NO KEY UPDATE` совместим с FK key-share.
- Повторное добавление/удаление связи идемпотентно. Потерявший доступ элемент скрывается из карточек, кандидатов, checkbox и счётчиков; старую связь разрешено удалить.
- Скрытая связь может восстановиться после возвращения доступа. Автоматическое восстановление membership или изменение роли не выполняются.

Отдельный независимый просмотр backend проверил predicates, ownership, read snapshots, блокировки и ограниченность запросов; оставшихся дефектов не обнаружено.

## 6. Что исправлено во время реализации

Помимо основного функционала устранены выявленные проверками ошибки:

1. При отказе добавления `403` устаревший preview удаляется сразу, даже если последующий refetch завершился сетевой ошибкой. Удалённая/недоступная папка закрывает диалог и очищает cache.
2. Смена аккаунта сбрасывает формы и picker; старые callbacks не изменяют данные нового пользователя.
3. Удаление связи обновляет checkbox во всех загруженных страницах кандидатов и удаляет карточку только из выбранной папки; соседние папки сохраняются.
4. Переключение фильтра не сбрасывает быстро набираемый поиск. Такой же дефект исправлен в существующем разделе «Личные», добавлена регрессия.
5. «Добавить в папку» размещён до разрушительного действия выхода из чата; сохранено управление меню клавишами.
6. Исправлены размеры круглых аватаров кандидатов, контраст статусов/ссылок и мобильная геометрия. Проверки axe не отключались.
7. Обновлена только тестовая auth-фикстура старых chat-layout сценариев: полный текущий session response и завершение bootstrap. Продуктовая авторизация этой правкой не изменялась.

## 7. Производительность

Измерения выполнены на локальном PostgreSQL после ANALYZE: 50 папок владельца + 600 папок других пользователей; 3 000 собственных + 36 000 фоновых связей; 30 групп и 30 встреч. Фоновые membership содержали потерявших доступ пользователей.

| Операция repository | SQL для 1 / 50 элементов | Основной SQL, execution | Shared hit / read |
|---|---:|---:|---:|
| List | 2 / 2 | 2,531 мс | 5 108 / 0 |
| Items с folderId | 4 / 4 | 0,781 мс | 503 / 0 |
| Candidates с folderId | 4 / 4 | 1,105 мс | 290 / 0 |

Registered-account HTTP middleware добавляет ещё один SQL-запрос: REST totals соответственно 3 / 5 / 5. Метрики таблицы относятся к основному запросу EXPLAIN, без middleware, транспорта и остальных SQL. Это наблюдение на прогретой локальной тестовой базе, не production SLA.

Проверены scoped index/bitmap обращения и неизменное количество SQL; N+1 на папки и строки нет. Страницы ограничены keyset pagination; до 100 папок выдаются одним списком для picker. Внутренние индексовые lookup в PostgreSQL-плане не являются отдельными запросами приложения.

Полные планы и счётчики: [folder-query-plans.json](/Users/aleksandranickij/htdocs/go-recorder/tmp/folders-20261007/evidence/folder-query-plans.json).

## 8. Тесты и регрессия

| Проверка | Результат |
|---|---|
| `go test ./... -count=1` | PASS: 735 cases, 56 packages; 35 packages без тестов |
| `go test -race ./... -count=1` | PASS на расширенном наборе: 743 cases, 56 packages; DATA RACE нет |
| Финальный targeted domain/events/integration папок | PASS: 26 cases, 3 packages |
| Отдельный security/query/read/add-revoke запуск с race | PASS |
| `go vet ./...` | PASS |
| Frontend Vitest | PASS: 629 tests / 64 files |
| TypeScript, Prettier, production build | PASS; typecheck/lint повторены после финальных test-fixture правок |
| Папки с настоящим API, Chromium | PASS: DoD + recovery + финальная геометрия, 3 сценария |
| Direct/group с настоящим API, Chromium | PASS: 3 сценария сообщений, файлов, ролей, удаления доступа и reconnect |
| Chat layout, Chromium | PASS: 5 desktop/mobile/landscape сценариев |
| Conference layout + invitations, Chromium | PASS: 15 сценариев, включая гостя и настройки без потери медиасессии |
| Dashboard/calendar, Chromium | PASS: 2 сценария |
| Реальная запись FFmpeg + MinIO | PASS: composite, audio_only, individual_tracks, screen_focus |

Полный non-race запуск предшествовал добавлению восьми concurrency cases; полный race и финальный targeted запуск включают новые тесты. В Go full/race по 58 skips из условных/opt-in тестов; browser harness в обычном targeted запуске также пропущен без fixture-флага. Требуемые браузерные сценарии запущены отдельно с настоящими изолированными API. Четыре режима записи запущены отдельно с FFmpeg и MinIO. Не заявляется проверка всех необязательных внешних провайдеров.

Backend full/race также проверяют существующие meetings, conference chat, global WS, Pion/SFU и recording lifecycle. Внешние browser layout/invitation сценарии используют HTTP/WS fixtures; их не следует считать production end-to-end звонком между реальными устройствами.

Ранний объединённый browser запуск остановлен после выявления устаревшей chat auth-фикстуры; затем пять chat cases и пятнадцать conference cases отдельно прошли. В отчёте приведены успешные отдельные запуски, а не результат остановленного запуска.

Журналы находятся в `/Users/aleksandranickij/htdocs/go-recorder/tmp/folders-20261007/`: `go-all.jsonl`, `go-race.jsonl`, `folders-final.jsonl`, `security-query-race.log`, `frontend-unit-final.log`, `frontend-build.log`, `frontend-typecheck-final.log`, `frontend-lint-final.log`, `frontend-folder-browser.log`, `frontend-folder-browser-final-ui.log`, `personal-group-regression.log`, `chat-layout-regression.log`, `conference-regression.log`, `recording-regression.log`.

Два изолированных API harness завершены, временные credentials/stop-файлы удалены; их базы, workers и тестовые объекты очищены. Существующие локальные Docker-сервисы оставлены работающими.

## 9. Экраны и доступность

Проверены desktop 1440 и mobile 390/320, отсутствие горизонтального переполнения, читаемые карточки, круглая геометрия аватаров, клавиатура, focus return, семантика меню и диалогов. Axe WCAG 2/2.1 A/AA — без нарушений в проверенных страницах и диалогах.

- [Папка, desktop](/Users/aleksandranickij/htdocs/go-recorder/tmp/folders-20261007/browser-final-ui/folders-live-focused-folde-5252e-able-cards-on-the-final-CSS-chromium/folder-detail-1440.png)
- [Папка, mobile](/Users/aleksandranickij/htdocs/go-recorder/tmp/folders-20261007/browser-final-ui/folders-live-focused-folde-5252e-able-cards-on-the-final-CSS-chromium/folder-detail-320.png)
- [Добавление, desktop](/Users/aleksandranickij/htdocs/go-recorder/tmp/folders-20261007/browser-final-ui/folders-live-focused-folde-5252e-able-cards-on-the-final-CSS-chromium/folder-picker-1440.png)
- [Добавление, mobile](/Users/aleksandranickij/htdocs/go-recorder/tmp/folders-20261007/browser-final-ui/folders-live-focused-folde-5252e-able-cards-on-the-final-CSS-chromium/folder-picker-320.png)

Скриншоты и logs — локальные ignored evidence, не входят в Git-пакет исходников.

## 10. Изменённые файлы этого этапа

46 файлов относительно указанного базового tree, включая этот отчёт. Изменения групп предыдущего этапа не включены в список:

```text
database/migrations/000032_personal_folders.down.sql
database/migrations/000032_personal_folders.up.sql
docs/PERSONAL_FOLDERS.md
docs/code-review/PERSONAL_FOLDERS_AUDIT.md
docs/code-review/PERSONAL_FOLDERS_REPORT.md
frontend/e2e/chat-layout.spec.ts
frontend/e2e/folders-live.spec.ts
frontend/e2e/helpers/chat-fixture.tsx
frontend/src/App.tsx
frontend/src/api.test.ts
frontend/src/api.ts
frontend/src/components/ConferenceChatActions.tsx
frontend/src/components/ConferenceChatInfo.test.tsx
frontend/src/components/FolderItems.tsx
frontend/src/components/FolderModals.tsx
frontend/src/components/FolderPicker.tsx
frontend/src/components/Folders.test.tsx
frontend/src/components/ItemActions.tsx
frontend/src/components/Layout.a11y.test.tsx
frontend/src/components/Layout.tsx
frontend/src/components/folders.css
frontend/src/folders.test.ts
frontend/src/folders.ts
frontend/src/pages/FoldersPage.tsx
frontend/src/pages/PersonalPage.tsx
frontend/src/pages/folders-page.css
frontend/src/pages/personal.css
frontend/src/personalRealtime.ts
frontend/src/types.ts
frontend/src/workspace.css
internal/app/api.go
internal/app/folders/handler.go
internal/domain/folders/folders.go
internal/domain/folders/folders_test.go
internal/infrastructure/postgres/folder_repository.go
internal/transport/http/folder_routes.go
internal/usecase/folders/events.go
internal/usecase/folders/events_test.go
tests/integration/folders_browser_harness_test.go
tests/integration/folders_query_test.go
tests/integration/folders_race_test.go
tests/integration/folders_realtime_test.go
tests/integration/folders_security_test.go
tests/integration/folders_test.go
tests/integration/group_browser_harness_test.go
tests/integration/personal_messages_test.go
```

## 11. Границы и следующие типы

Поддержаны direct/group/conference. Записи и файлы автоматически не добавляются. Цвет, описание, закрепление, ручной порядок элементов и drag/drop не входят в контракт этапа. Для нового типа элементов нужны отдельная FK-модель и собственная проверка доступа.

При потере внешнего meeting доступа UI сверяется с REST при фокусе/видимом 15-секундном refresh; мгновенные сообщения о таких изменениях зависят от существующих событий встречи. Сервер скрывает недоступные данные уже при следующем запросе. Скрытые исторические связи сохраняются до ручного удаления или физического удаления target.

## 12. Публикация — 7 октября 2026, 00:51 МСК

На [meeting.janickiy.com](https://meeting.janickiy.com/folders) установлен совместный выпуск групп и папок **`v1.0.0-meeting.20261007-folders.1`**. Build snapshot: `889caa37c795d50d75d46356366377c6f77816b1`; manifest SHA-256: `b3a2343aa296fe77d44ad13961d890c366aaeb3488833cd1da736a6611c4913b`.

Обновлены только API и frontend. Установщик подтвердил штатное завершение прежних процессов, сохранение остальных 17 контейнеров и защищённых конфигураций. Compose, SMTP и proxy не менялись. Миграции 30–32 применены к серверной базе; прежние SQL-файлы сохранены побайтно, ledger содержит ровно 32 миграции.

Повторная проверка после установки: все 19 контейнеров работают, 11 healthcheck healthy; API/frontend возвращают точные version/commit. Публичные version/health запросы успешны, анонимный доступ к папкам и личным чатам закрыт (`401`), служебные monitoring/operations пути — `404`.

Браузерная проверка реального опубликованного сайта с настоящим API и временными тестовыми аккаунтами: **PASS, 33 проверки**, включая девять axe WCAG проверок без нарушений. Проверены папка с личным чатом, группой и неактивной встречей, поиск/фильтры, обновление второй открытой страницы, один чат в двух папках, сохранность переписки после удаления связи, отказ в чужой папке и отзыв группового доступа со скрытием карточки/счётчика. Desktop 1440 и mobile 390/320 px — без горизонтального переполнения и browser runtime errors. Сообщения, email-приглашения, uploads и звонки тестом не запускались. Скриншоты опубликованного интерфейса просмотрены.

После smoke точной проверкой ID и принадлежности в одной транзакции удалены только его четыре временных аккаунта, четыре переписки, две неактивные встречи и три папки. Fingerprints всех 41 исходных таблиц после очистки совпали с состоянием до установки; четыре новые таблицы снова пусты. Browser contexts закрыты, временные credentials удалены.

После завершения API до миграций снят `pg_dump` на локальный компьютер. Копия восстановлена в отдельную временную базу: fingerprints всех 41 прежних таблиц совпали с источником. Точный бинарник нового выпуска применил миграции на копии без изменения прежних строк; повторный запуск не изменил ledger и данные. Рабочую базу резервной копией не заменяли. Это проверка восстановления PostgreSQL, без заявления об общей резервной копии объектного хранилища.

Оба amd64 образа проверены свежими Trivy/SBOM: **0 HIGH / 0 CRITICAL**. Восемь проверок установщика прошли. Source/image архивы проверены и переданы через память сервера. После повторной проверки свободно **1 193 668 608 байт**, резерв 1 ГиБ соблюдён.

После применения новой схемы автоматического возврата к старому API нет: прежний direct-only бинарник несовместим с отзывом группового доступа. Дальнейшие исправления выполняются совместимым выпуском; down-миграции защищают пользовательские группы, аватары и папки от удаления.

Evidence: `tmp/folders-deploy-20261007/evidence/`, включая `deployment.json`, `restore-rehearsal.json`, `installer-safety.json`, `final-artifact.json`, `post-deployment-checks.json`, `public-health.json`, `prod-smoke-result.json` и `cleanup-smoke-result.json`. Подробная история: [meeting-host-deployment.md](../operations/meeting-host-deployment.md). Этот раздел отчёта добавлен после публикации и не является частью уже собранных образов.
