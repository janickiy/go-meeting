# Meetrix — отчёт о вёрстке и интеграции frontend

Дата проверки: 3 октября 2026 года.
Источник требований: `CODEX_PROMPT_FRONTEND_INTEGRATION.md` и приложенный макет
из 14 экранов. Имя Meetrix и временные заглушки входа Google/Microsoft явно
согласованы владельцем. Реализован интерфейс поверх существующих контрактов;
возможности, отсутствующие на backend, не имитируются.

## 1. Архитектура frontend

Сохранено приложение React: Router → AuthProvider/QueryClient → защищённый Layout
→ lazy pages. Разделены оболочка кабинета, prejoin и активная тёмная комната.
Media lifecycle, API client, WebSocket reducer и аутентификация переиспользованы.
Подробности: [руководство разработчика](FRONTEND_GUIDE.md) и подготовленный до
реализации [аудит и mapping](FRONTEND_INTEGRATION_PLAN.md).

## 2. Фактически использованный стек

React 19.3, TypeScript 7.0.2, Vite 8.3.1, React Router 8.4, TanStack Query 5.104,
Lucide, локальный Inter; Vitest, Testing Library, axe-core и Playwright.
Проверено с Node 24.19.0. Новые runtime-зависимости не добавлены.

## 3. Маршруты и экраны

Обновлены вход/регистрация/успех, dashboard, создание/приглашение, prejoin,
conference room, история/материалы записи, профиль/настройки. Добавлены `/calendar`
и `/analytics`. Сохранены старые `/app/*`, `/meetings/*`, `/conferences/*`, invitation,
search, notifications, admin и calendar callback. Глобальные файлы/чат не добавлены:
они доступны в контексте встречи, согласно API.

## 4. Макет → компоненты

| Экран макета | Реализация |
| --- | --- |
| Вход/регистрация | AuthPages, центральный Brand, фотофон, disabled social buttons |
| Главная | Layout, Dashboard, реальные server-filtered списки, recent history |
| Prejoin | PreJoinPage, реальные устройства и preview с cleanup |
| Конференция | ConferencePage, RealtimePanel, memoized MediaTile |
| Чат | Существующие ChatPanel/AttachmentUploader в светлой боковой панели |
| Демонстрация экрана | Существующий media controller + системный browser picker |
| Участники/модерация | ParticipantsPanel, WaitingRoomPanel, серверные permissions |
| Запись/транскрипт | HistoryDetailPage, RecordingPanel, RecordingInsights |
| Календарь | CalendarPage, week/month/mobile agenda, ScheduleFields |
| История | RecordingsPage, pagination и поиск по загруженным страницам |
| Файлы | Вложения чата встречи; отдельного cross-meeting API нет |
| Аналитика | AnalyticsPage + AnalyticsPanel для одной выбранной встречи |
| Настройки | SettingsPage: профиль, устройства, уведомления, календарь |
| Mobile | Общие маршруты с drawer/bottom navigation и адаптивной комнатой |

## 5. REST-интеграция

Используются существующие auth, me/conferences, conferences, admission/moderation,
messages/attachments, recordings, transcript, summary, analytics, notifications и
integrations endpoints. Главная не выполняет N+1 запросов записей/превью.
Timeline API требует явного `view`: календарь использует `upcoming`, история —
`past`, аналитика — выбор `past`/`active`. Эта контрактная ошибка обнаружена и
исправлена при ревью новой страницы аналитики, добавлен regression test.

## 6. WebSocket

Новая оболочка не создаёт второй conference WS. Существующие snapshot, revisions,
reconnect, participant/media policy, chat и recording события сохранены. Presence
diagnostics свёрнуты под раскрываемую секцию, а не удалены. Live proof подтвердил
два браузера, две вкладки, presence и reconnect без включения устройств.

## 7. WebRTC

SFU client и протокол не переписаны. Prejoin не начинает скрытый захват устройств.
Memoized tiles не переустанавливают `srcObject` от посторонних UI-событий; cleanup
отвязывает поток. Реальный Go/browser test подтвердил двусторонние audio/video,
демонстрацию экрана, ограничение конкурирующей публикации, модерацию, восстановление
медиа после reconnect и нулевые peers/rooms/tracks/subscriptions после teardown.

## 8. Управление состоянием

Серверные данные остаются в React Query, с user-scoped keys и серверной pagination.
Роль/admission/policy берутся из backend/realtime. Выбранные history/analytics
параметры сохраняются в URL; открытые панели/диалоги локальны. Потоки и
PeerConnection не сериализуются и не переносятся в новый global store.

## 9. Токены и компоненты дизайна

Новые `tokens.css`, `workspace.css`, `brand.ts`, `brand-mark.svg`; scoped CSS для
dashboard, calendar, conference/prejoin, history/settings/analytics. Имя задаётся
`VITE_PRODUCT_NAME` (Docker build arg `PRODUCT_NAME`), default Meetrix. Типографика,
primary/border/text/surface, dark room colors, радиусы и отступы централизованы.
Сохранены общие Button/Modal/ErrorNotice/Loading и совместимые legacy aliases.

## 10. Responsive/mobile

Desktop: компактный sidebar/topbar и отдельная полноширинная тёмная комната.
Tablet ≤1100px, mobile ≤760px. На mobile — нижняя навигация, доступное выдвижное
меню, список календаря, переключаемая room-панель. Проверены login/register,
dashboard/create, calendar, prejoin/room/chat, history, settings и analytics;
в браузерных проверках нет горизонтального переполнения всей страницы.

## 11. Accessibility

Сохранены labels, skip-link, aria-live, доступные ошибки и disabled/busy действия.
Проверены клавиатурные вкладки, Escape/focus return, drawer/dialog focus management,
read-only email, role/capability restrictions. Axe/component tests проходят;
jsdom axe не измеряет контраст. Screenshots дополнительно просмотрены визуально.
Это не является полной сертификацией WCAG или проверкой со всеми screen readers.

## 12. Security review

Никаких production fixtures, новых токенов в localStorage, подмены ACL или
небезопасного HTML. Google/Microsoft login — disabled «Скоро», без OAuth/redirect
и без fake success. Приватные файлы используют существующую авторизацию и signed
URLs. Upload allowlist и 10 МБ сохранены. Новые gated-разделы истории/аналитики,
навигация, semantic search и room captions работают fail-closed, включая ошибку
обновления с ранее закешированным разрешением; это не утверждение о переписывании
всех прежних panels. Календарные интеграции используют отдельный контракт
`/integrations/capabilities`. CSP origin validation проходит.
Telemetry allowlist дополнен только `/calendar` и `/analytics`.

Редизайн **не снимает** существующие security/release ограничения из
[отчёта этапа 10](operations/launch-readiness-report.md): внешнего staging нет,
полный production security gate должен проверяться отдельно.

## 13. Тесты, E2E и visual results

| Проверка | Фактический результат |
| --- | --- |
| Prettier + TypeScript + production build | PASS |
| Vitest | 197 тестов, 39 файлов — PASS |
| Основная Chromium UI suite | 24 PASS; 9 внешних/opt-in сценариев skipped в этом запуске |
| Реальный SFU/control browser harness | PASS, 2 браузера, teardown без утечек |
| Реальный presence/reconnect browser harness | PASS, 2 браузера/вкладки |
| Реальные запись/чат/файлы/history player | PASS, отдельный opt-in live smoke с новой production-сборкой |
| Calendar DST, Europe/Berlin | 9 targeted tests — PASS |
| Go regression | `go test ./...` — PASS; opt-in integration tests без env остаются skipped |
| Backend контрактные пакеты без кеша | 9 пакетов — PASS |
| CSP origin validation | PASS |

E2E error-state проверка адаптирована к двум независимым загрузкам dashboard:
обе ошибки должны быть безопасными. Это не подавление ошибки продукта.
Отдельный `frontend-live-smoke.spec.ts` проверил два реальных SFU видеопотока,
realtime чат, PDF upload/download (200 по signed URL, 403 без подписи), запуск/
остановку записи, готовый MP4 и воспроизведение истории с растущим `currentTime`.
Использованы синтетические устройства и аккаунты в отдельном локальном стенде.
Новая production-сборка наложена на HTTPS origin только в изолированном браузере:
API/WS не перехватывались, CSP сохранён, deployment не выполнялся. Для такого
static overlay Chromium потребовал origin-scoped `local-network-access`; глобальные
security checks не выключались. Тестовые встречи завершены, артефакты сохранены.
Это отдельный PASS, а не утверждение о прохождении skipped старых Docker suites.

Локальные просмотренные изображения сохранены в
`tmp/frontend-integration-20261003/screenshots/`: login, dashboard, calendar,
conference/mobile, history, settings, analytics, live-conference и live-history.
JSON smoke и последние live screenshots сохраняются тестом в `test-results/live-frontend`.

## 14. Браузеры

- Chromium Playwright: обычные UI tests и реальные SFU/presence tests PASS.
- Установленный Google Chrome: 13 desktop/mobile/keyboard/auth/calendar/room
  сценариев PASS.
- Firefox Playwright: запуск заблокирован окружением до открытия страницы —
  `Could not find profile folder`. Это не успешная проверка Firefox и не
  доказательство ошибки frontend. Нужен повтор в исправленном runner.
- Edge отсутствует на машине; Safari/iOS и настоящие мобильные устройства
  не проверялись. Компиляционные targets не заменяют browser validation.

## 15. Production build

Build проходит без sourcemaps. Main JS: 262.46 КБ / 81.60 КБ gzip; до редизайна
257.06 КБ / 80.23 КБ gzip. Комната: 86.62 КБ / 23.90 КБ gzip, lazy-loaded;
отдельные тяжёлые панели остаются чанками. Фотофон: около 516 KiB JPEG.
Сборка проверена в изолированной временной копии с Node 24; рабочие контейнеры
пользователя не пересобирались и не перезапускались.

## 16. Изменённые и новые файлы

- Оболочка: `App.tsx`, `main.tsx`, `brand.ts`, `tokens.css`, `workspace.css`,
  `components/Layout.tsx`, `components/ui.tsx`, index.html, Dockerfile, `.env.example`.
- Страницы: AuthPages, Landing, Dashboard, CalendarPage, ConferencePage,
  PreJoinPage, AccountPages, HistoryDetailPage, AnalyticsPage, InvitePage,
  SearchPage (fail-closed) и соответствующие CSS.
- Компоненты: ConferenceModals, RealtimePanel, ParticipantsPanel,
  RecordingInsights, AnalyticsPanel.
- Tests: component tests этих страниц, Layout.a11y; новые calendar-dashboard,
  conference-layout, history-analytics-visual, live-smoke; адаптированы старые
  media/realtime/composite-recording/collaboration/meet E2E selectors.
- Backend: только статические пути в `internal/app/telemetry/handler.go`.
- Документация: этот отчёт, FRONTEND_GUIDE, FRONTEND_INTEGRATION_PLAN, USER_GUIDE,
  ссылки и актуальные сведения в README и docs/frontend.md.
- Assets: `frontend/public/brand-mark.svg`, `frontend/public/media/auth-landscape.jpg`.

Посторонние изменения пользователя и staged файл Python cache не изменялись.
Два старых временных Go probe в ignored `tmp/` помечены `go:build ignore`, чтобы
не конфликтовать двумя `main` при `go test ./...`; их запуск по явному файлу сохранён.

## 17. Обнаруженные backend gaps

Нет login OAuth, password-reset, avatar upload, редактирования email/должности/
команды. Нет глобального каталога файлов/чата и организационных KPI, preview
aggregate записей, произвольного description/auto-record-on-create.
Календарь не имеет отдельного all-status calendar endpoint: timeline date меняет
семантику по статусу. Поэтому отображается именно предстоящее расписание.

## 18. Отклонения от картинки и причины

Нет фальшивых портретов участников, видео/thumbnail записей, графиков и фиктивных
кнопок успешных интеграций. При выключенной камере используются инициалы, без
имитации active-speaker. Screen picker остаётся системным. Вместо общего files
раздела — вложения встречи; вместо агрегатов компании — аналитика выбранной
встречи. Сохранены понятные подписи действий и сервисные состояния реального API.

Фотофон создан инструментом **imagegen**, режим **generate** (не редактирование
референса), затем оптимизирован в JPEG. Сохранённый asset:
`frontend/public/media/auth-landscape.jpg`.
Использованный prompt:

> Use case: photorealistic-natural. Asset type: decorative background photo for a conferencing product login screen. Primary request: tranquil alpine lake reflecting dramatic sharp mountain peaks, deep evergreen forest framing the lake, soft mist and blue early-morning light, natural photographic detail. Wide landscape composition, coherent uninterrupted scene with usable calm space for a login card overlay near the center. Elegant muted blue and slate colors, not oversaturated, realistic mountains and water. No people, no buildings, no text, no logos, no user-interface elements, no watermark. Output just the scenery.

## 19. Известные ограничения

Полная acceptance matrix на внешнем staging, TURN-only сетях, Firefox/Edge/Safari
и реальных mobile devices не подтверждена. Старые opt-in Docker suites с restart/
cleanup не запускались на рабочем пользовательском проекте. AI/STT/calendar/
captions требуют включённых реальных провайдеров; отсутствие capability честно
отображается. Поддержка system audio/output-device зависит от браузера. Новая
вёрстка готова в исходниках/сборке, но это не автоматическая публикация релиза.

## 20. Рекомендуемые заключительные проверки

1. Повторить Firefox после исправления runner и проверить Safari/iOS/Edge.
2. Пройти полный сценарий записи/расшифровки/AI с целевыми провайдерами на staging.
3. Проверить TURN-only, смену сети, физические камеры/микрофоны и screen reader.
4. После согласования backend API отдельно реализовать социальный вход, recovery,
   глобальные файлы/чат и календарь истории, если эти функции остаются в roadmap.
5. Выполнить существующий security/release gate перед выкладкой образа.

## Дополнение: обновление локального Docker по отдельному запросу

3 октября 2026, после завершения реализации, владелец отдельно разрешил обновить
локальный запуск. В Compose-проекте `go-recorder` пересобраны и запущены frontend,
API, media-worker, recorder-worker, product-worker и live-worker. HTTPS proxy
прошёл проверку конфигурации и плавную перезагрузку. Остальные Docker-проекты,
контейнеры PostgreSQL/Redis/RabbitMQ/MinIO и их тома не пересоздавались.

До миграций создана и проверена резервная копия PostgreSQL:
`tmp/docker-update-20261003.6h2C7n/database-before-update.dump` (доступ только владельцу).
Применены необходимые миграции, журнал содержит 21 запись. До и после обновления:
1 пользователь, 6 прежних записей; данные не удалялись. Внешние провайдеры не включались.

Проверены `health/ready` API (200, ready), frontend health, HTTP/HTTPS форма входа
(200), новый заголовок Meetrix и disabled Google/Microsoft. Защищённый endpoint
capabilities теперь присутствует (401 без токена вместо прежнего 404).
Локальные адреса: `http://localhost:5173/login`, `https://localhost:18482/login`.
Это локальное обновление, не production promotion.
