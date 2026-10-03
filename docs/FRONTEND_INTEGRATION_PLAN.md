# Frontend: аудит и план интеграции по макету

Дата: 3 октября 2026. Этот отчёт подготовлен **до изменения интерфейса**.
Reference: приложенный пользователем полный mockup из 14 экранов; это визуальное
направление, а не новый backend-контракт.

## 1. Фактическая архитектура

React 19.3, TypeScript 7, Vite 8.3, React Router 8.4, TanStack Query 5.104,
Lucide, локальный Inter. Сохраняем стек без новых библиотек. Маршруты lazy-loaded,
AuthProvider управляет токеном в sessionStorage/памяти; API использует same-origin
`/api/v1`, отмену запросов и нормализованные ошибки. Refresh API отсутствует.
Источники состояния: HTTP Query cache, один conference realtime client, отдельный
ConferenceMediaClient/useMedia для Pion SFU, локальные настройки устройств.

## 2. Переиспользуемые компоненты

Button/PasswordInput/Modal/CopyButton/StatusBadge/Loading/ErrorNotice, Layout,
CreateConference/JoinByLink/ScheduleFields, DeviceSettings/PreJoin,
RealtimePanel, WaitingRoomPanel, ChatPanel/AttachmentUploader, CaptionsPanel,
ReactionsPanel, RecordingPanel/RecordingInsights, AnalyticsPanel,
NotificationBell/IntegrationsSettings. Существующие reducers, permission checks,
подписки, безопасные URL и cleanup сохраняются.

## 3–7. Экран → маршрут → REST → realtime/media

Все REST paths ниже имеют префикс `/api/v1`.

| Макет | Маршруты / компоненты | Серверный контракт и события |
| --- | --- | --- |
| 1. Вход/регистрация | `/login`, `/register`, `/register/success`; AuthPage | POST auth/login, auth/register; GET/PATCH auth/me; POST auth/logout |
| 2. Главная | `/app`, `/meetings`, `/conferences`; Dashboard | GET me/conferences с view/scope/from/to/cursor; POST conferences; без N+1 для списка |
| 3. Prejoin | `/meetings/:id/join`, `/conferences/:id/join` | conference, participants/me, join/admission; enumerateDevices/getUserMedia/AudioContext, setSinkId при поддержке; preview cleanup |
| 4. Комната | `/meetings/:id`, `/conferences/:id` | ws-ticket → WS; conference.state, participant.*; media.join/joined, offer/answer/ICE/tracks/policy; существующий SFU client |
| 5. Чат | панель комнаты и `/history/:id?section=chat` | cursor GET/POST messages, PATCH/DELETE message, chat/read; chat.message.created/updated/deleted, chat.read.updated |
| 6. Screen share | встроенное управление комнаты | getDisplayMedia, серверная эксклюзивность/forced stop, track.ended; native chooser не подменяем HTML-диалогом |
| 7. Участники | panel + WaitingRoomPanel | participants, admission, moderation; participant/admission/media policy events; owner/co-host permissions неизменны |
| 8. Запись/транскрипт | `/history/:id`; RecordingInsights | history, recordings/details с signed URLs, transcript/segments/retry, summary/regenerate; recording.* в комнате |
| 9. Календарь | новый `/calendar` | me/conferences?view=upcoming&from/to, PUT conferences/:id/schedule, POST cancel; существующий ScheduleFields |
| 10. История | `/history`, `/app/recordings` | me/conferences?view=past, pagination → history details |
| 11. Файлы | meeting-scoped панель материалов | attachments init → upload URL PUT → finalize → message; download только через авторизованный API |
| 12. Аналитика | новый `/analytics` с выбором встречи, существующий tab | GET conferences/:id/analytics, только meetingAnalytics capability; нет выдуманных общих KPI |
| 13. Настройки | `/settings`, `/app/settings` | displayName PATCH, email read-only; local devices, notifications/preferences, integrations capabilities/calendars |
| 14. Mobile | те же маршруты | те же права и контракты, drawer/bottom navigation вместо отдельной логики |

Дополнительно сохраняются `/i/:code`, `/search`, `/app/search`, `/notifications`,
`/admin`, calendar callback. Search modes зависят от capabilities; admin — только
isAdmin и серверного ACL. Captions partial/final заменяют сегмент по ID/revision,
а не дублируют его. Реакции используют REST-команду POST /reactions и WS-событие
reaction.created. Notifications SSE остаётся один на session.

## 8. Реальные gaps

- Google/Microsoft OAuth реализован для календаря, не входа. По последующему явному
  запросу владельца показываем disabled-заглушки «Скоро», без login, redirect или fake-success.
- Нет password-reset, email edit, avatar upload, должности/команды профиля: не делать fake-success.
- Нет cross-meeting файлового каталога, глобального чата и общей аналитики компании.
  Файлы/чат остаются в контексте встречи; отдельная аналитика выбирает одну встречу.
- Нет API произвольного description/автозаписи при создании. Используем title,
  waitingRoom, scheduledAt, plannedDurationMin; запись запускается отдельным действием.
- Co-host не получает owner-only camera/role actions. UI не расширяет серверные права.
- Browser screen picker, системный звук и выбор audio output зависят от браузера.
- AI/transcript/live captions/semantic/calendar UI показывается по эффективным capabilities.
- Timeline API без `view` возвращает upcoming, а не все встречи. Его дата —
  COALESCE(finished_at, scheduled_at, created_at), поэтому календарь явно ограничен
  предстоящими запланированными встречами; история и активные вынесены в свои разделы.
  Аналитика выбирает `past` либо `active` явным фильтром.

## 9. Иерархия компонентов

App → Protected → Layout (Brand/Sidebar/Topbar/Search/Notifications/Profile/MobileNav)
→ lazy pages. ConferencePage → dark stage → existing RealtimePanel/media tiles
+ accessible controls + selected side panel. HistoryDetail → header/tabs → lazy
recording/player/transcript/summary/chat/analytics. Settings → navigation + active
section; настоящие формы переиспользуются. Calendar → date navigation/week-month
grid/meeting links → CreateConference/ScheduleFields.

## 10. План состояния

Серверные сущности остаются в React Query с user-scoped cache, cursor pagination,
AbortSignal и явной invalidation. WS snapshot authoritative при reconnect. Media
objects/PeerConnection не попадают в persisted store; эффекты освобождают tracks,
audio contexts, timers/listeners. Draft/tab/drawer — локальный React state/URL.
Чат сохраняет clientRequestId/idempotency и существующий reconcile. Никакого второго
сокета или нового P2P implementation ради дизайна.

## 11. Токены и branding

Центральные CSS tokens: semantic colors, light surfaces, dark room surfaces,
text/muted/border/primary/danger/success/warning, spacing/radius/shadow/type/z-index.
Legacy aliases позволяют постепенное внедрение без потери текущих состояний.
Breakpoints: mobile ≤760px, tablet ≤1100px, desktop выше. Branding задаётся в одном
модуле с build-time env; владелец явно выбрал имя **Meetrix**. Не копируем
чужие фото/товарные знаки с картинки и не выдаём иллюстрации за реальные данные.

## 12–13. Responsive и accessibility

Desktop: compact sidebar, topbar, light workspace, dark meeting. Mobile: bottom
navigation и accessible drawer, safe-area, controls ≥44px, ограниченная прокрутка
панелей без горизонтального overflow. Семантические headings/forms/tabs/dialogs,
keyboard navigation, focus return/trap, skip link, reduced motion, live statuses,
labelled icon buttons. Существующие axe/component checks остаются обязательными.

## 14. Порядок

1. Baseline checks и mapping report.
2. Tokens/branding/shell/auth; параллельно dashboard/calendar, room/prejoin,
   history/settings/meeting analytics на существующих API.
3. Unit/permission/capability tests, production build, Chromium user journeys.
4. Desktop/mobile screenshots и сверка с reference; исправление layout/contrast.
5. Реальный локальный API smoke в изолированном окружении; browser/media ограничения
   записать честно, не считать skipped full acceptance.
6. Итоговый frontend guide/report, без автоматического перехода к следующему этапу.

## 15. Область файлов

`frontend/src/{App,main,brand}.tsx/ts`, `components/{Layout,ui,...}`, pages Auth,
Dashboard, Calendar, Conference/PreJoin, History/Account, Analytics, отдельные CSS
tokens/shell/page modules; соответствующие component/E2E tests, `.env.example`,
frontend build branding args при необходимости и frontend docs. Backend/media
протоколы не меняются. Рабочие Docker-проекты и существующие незакоммиченные
изменения сохраняются; для visual checks используется отдельный preview.
