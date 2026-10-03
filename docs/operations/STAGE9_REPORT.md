# Этап 9 — Product UX и release hardening

Состояние: реализация в рабочем дереве. Это отчёт о локальной проверке, а не разрешение на production выпуск. Исходные требования: `CODEX_PROMPT_09.md` и `TECHNICAL_SPECIFICATION.md`, предоставленные заказчиком. Предыдущие архитектурные и эксплуатационные решения сверены с `docs/` и доступной историей Git.

## 1. Product architecture

Существующий React/TypeScript/Vite клиент использует REST для устойчивых данных, WebSocket для состояния комнаты, локальный `MediaClient` для `RTCPeerConnection` и TanStack Query для серверного кеша. Stage 9 расширяет эти слои без смены SFU/recorder протокола. Новый `/api/v1/capabilities` сообщает клиенту безопасные признаки функций; серверные права по прежнему остаются источником истины.

## 2. Screens/routes

Сохранены `/app` и `/conferences/*`; добавлены продуктовые `/meetings`, `/meetings/new`, `/meetings/:id`, `/meetings/:id/join`, `/history`, `/history/:id`, `/search`, `/notifications`, `/settings`, `/admin`. Для существующих ссылок остаются старые адреса. Активное приглашение ведёт на Pre-Join; запланированное сначала добавляется в личные встречи.

## 3. Design/component system

Повторно используются существующие `Layout`, формы, карточки, модальные окна и компоненты данных. Для активной встречи добавлен media-first stage с панелями чата, участников и субтитров. Маршруты загружаются отдельными чанками, а навигационная оболочка сохраняется при ожидании чанка.

## 4. Prejoin/device UX

Pre-Join показывает имя, локальный preview, независимую проверку микрофона с meter, выбор входа/выхода при поддержке браузером, ошибки разрешений и отсутствие устройства. Предпочтения хранятся локально по пользователю; исчезнувший `deviceId` заменяется системным. Preview tracks останавливаются при входе и размонтировании. До явного Join нет запроса на присоединение, а до серверного допуска нет публикации в SFU.

## 5. Conference UX

Основное медиа поднято над вторичными сведениями; доступны mic/camera/screen, рука, чат, участники, запись и модерация по существующим server-side правам. Запись видна всем допущенным; доступные режимы берутся из capabilities. Клавиши M/V/H/C действуют только в активной встрече и отключены при вводе/диалогах.

## 6. Reconnect/error model

Интерфейс отдельно показывает offline/WS/media/ICE состояния и явное переподключение. После восстановления устройства автоматически не включаются. Server snapshots и действующие media tickets остаются механизмом согласования, локальный preview не обходит waiting room.

## 7. Accessibility results

Добавлены skip link, управление мобильным меню, обозначенные состояния вкладок и клавиатурная навигация, видимый focus, touch targets и объявления финальных субтитров без потока частичных повторов. `npm run test:a11y` проходит 6 автоматизированных сценариев. Полная ручная проверка WCAG 2.1 AA со screen reader на целевых устройствах ещё требуется.

## 8. Responsive/browser results

CSS учитывает узкие экраны, горизонтальную прокрутку вкладок, touch targets, safe areas и reduced motion. Playwright сценарии проходят в Chromium и Firefox с mock API и на локальном Docker Compose; отдельный мобильный smoke проверяет 390×844 без горизонтального переполнения. Селекторы устройств дополнительно проверены по скриншоту на этой ширине. Физические камеры, Safari, Edge и мобильные устройства остаются отдельным release gate.

## 9. Admin/diagnostics

Миграция `000021` добавляет deny-by-default `users.is_admin`; `/admin/summary` каждый раз проверяет право в PostgreSQL и возвращает ограниченные агрегаты/коды ошибок без личного содержания. `/admin` скрывает данные после 403. Диагностика комнаты показывает состояния связи и агрегаты WebRTC; копируемый отчёт должен содержать только разрешённые поля, без SDP, IP, токенов и `deviceId`.

## 10. Security/CSP review

Чат, расшифровки, поиск и AI выводятся как React text, без доверенного HTML. Ссылки на материалы остаются авторизованными/signed; `Referrer-Policy: no-referrer` и `nosniff` заданы прокси. CSP без `unsafe-inline`/`unsafe-eval`; внешний storage origin разрешается только через явно заданный и проверенный HTTPS origin, локальный HTTP storage — только по точному loopback origin, WebSocket origin — по точному адресу. CSP-валидатор включён в CI, заголовок проверен на запущенном Nginx. Права admin и допуск к материалам проверяются сервером; приглашение само по себе не даёт доступ к медиа.

## 11. E2E/visual/a11y tests

Новые browser tests покрывают Pre-Join, историю и глубокие ссылки, уведомления, запрет admin и мобильную ширину: mock suite — 34 passed, 14 opt-in skipped в Chromium и Firefox. Локальный Compose smoke с реальными API/PostgreSQL — 2/2, прежний двухпользовательский live сценарий — 2/2. При live проверены отсутствие Join-запроса до явного входа, `403` для обычного пользователя на admin API и безопасный `buildVersion`. Восемь скриншотов обоих браузеров сохраняются в игнорируемом `frontend/test-results/stage9-visual/`; ссылка и код приглашения замаскированы. `npm run test:a11y` — 6 passed. Реальные устройства, TURN relay и запись с FFmpeg остаются отдельными opt-in проверками.

## 12. Frontend performance/build

После разделения маршрутов основной JS чанк уменьшился с 386.16 КБ (112.42 КБ gzip) до 256.25 КБ (80.01 КБ gzip); конференция вынесена в чанк 77.27 КБ (21.88 КБ gzip). Summary запрашивается при открытии вкладки. Это размер сборки, а не измеренные LCP/INP, CPU или media latency.

## 13. Modified/New files

Основные области: `frontend/src/App.tsx`, `pages/PreJoinPage.tsx`, `pages/ConferencePage.tsx`, `pages/HistoryDetailPage.tsx`, `pages/NotificationsPage.tsx`, `pages/AdminPage.tsx`, `pages/AccountPages.tsx`, `pages/SearchPage.tsx`; устройства и диагностика в `frontend/src/media.ts`, `useMedia.ts`, `prejoinDevices.ts`; backend в `internal/app/platform/`, `internal/usecase/platform/`, `internal/infrastructure/postgres/platform_repository.go`, `database/migrations/000021_admin_capability.up.sql`. Документация: `docs/frontend.md`, `docs/operations/admin.md`, `docs/operations/STAGE9_RELEASE_CHECKLIST.md`.

## 14. Backend changes (if any)

Добавлены `GET /api/v1/capabilities`, `GET /api/v1/admin/summary`, строго ограниченный `PATCH /api/v1/auth/me` для имени. Admin право хранится отдельно от conference owner/co-host и проверяется на каждый запрос. В API bootstrapping подключены безопасный media-worker ready probe и агрегатный repository.

## 15. Stage 3–8 regression

`go test ./...`, `go test -race ./...`, `go vet ./...`, `staticcheck` проходят. Изолированный прогон интеграции этапов 3–9 с PostgreSQL/Redis: 37 passed, 12 skipped; пропущенные требуют FFmpeg, pgvector, MinIO, browser либо явный load opt-in. `govulncheck` не нашёл достижимых уязвимостей; одна запись относится к неиспользуемому `openpgp` в зависимом `golang.org/x/crypto`.

## 16. Release checklist

Операционный чеклист находится в [STAGE9_RELEASE_CHECKLIST.md](STAGE9_RELEASE_CHECKLIST.md): миграция/backup, секреты и флаги, TURN/TURNS, health/observability, два браузерных пользователя, keyboard/screen reader, откат и go/no-go. Каждый пункт фиксируется на конкретном image digest и окружении.

## 17. Known limitations

Локальные unit/mock browser проверки не подтверждают реальную производительность медиа, WAN/TURN, TURNS, Safari/Edge/mobile hardware, долгую запись, восстановление backup и production observability. `000021` создаёт обычные индексы внутри общей транзакции автоматических миграций; на больших таблицах нужен rehearsal и окно выпуска. Полная локализация и измеренный performance budget не реализованы.

## 18. Recommended Stage 10 priorities

Проверить полный сценарий двух пользователей на staging с внешним TURN/TURNS и целевыми браузерами; измерить media и frontend performance; провести restore drill и rollout/rollback rehearsal; закрепить образы и метрики релиза. Stage 10 не начат.
