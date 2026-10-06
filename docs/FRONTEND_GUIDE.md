# Meetrix: устройство и запуск frontend

## Архитектура

Frontend находится в `frontend/`. Стек: React, TypeScript, Vite, React Router,
TanStack Query, Lucide и локальные шрифты Inter. Новых runtime-зависимостей ради
макета не добавлено. Точные версии закреплены в `frontend/package-lock.json`.

`main.tsx` устанавливает провайдеры авторизации, QueryClient, Router и общий
ErrorBoundary. `App.tsx` содержит маршруты и ленивые импорты страниц.
`components/Layout.tsx` отвечает только за общую навигацию, поиск, профиль и
уведомления; создаёт account WebSocket личных сообщений через
`usePersonalRealtime`, не создавая медиасоединение.

Иерархия комнаты: `ConferencePage` → `RealtimePanel` → memoized `MediaTile`,
`ParticipantsPanel`, ленивые `ChatPanel`/`RecordingPanel`, диалоги приглашения и
записи. Prejoin вынесен в `PreJoinPage` и существующие настройки устройств.
`HistoryDetailPage` загружает материалы только выбранной вкладки; player,
расшифровка и итоги используют общий `RecordingInsights`.

## Маршруты

| Путь | Назначение |
| --- | --- |
| `/`, `/login`, `/register`, `/register/success` | Приветствие и email/password авторизация |
| `/app`, `/meetings`, `/conferences` | Главная и фильтрованный список встреч |
| `/personal`, `/personal/:id` | Личные переписки; общий MessageThread/AttachmentUploader |
| `/calendar` | Предстоящие запланированные встречи: неделя, месяц, мобильный список |
| `/conferences/new`, `/meetings/new`; диалог на главной | Создание/планирование и переход по ссылке |
| `/i/:code` | Приглашение с сохранением назначения после входа |
| `/conferences/:id/join`, `/meetings/:id/join` | Prejoin, устройства и допуск |
| `/conferences/:id`, `/meetings/:id` | Комната и управление встречей |
| `/history`, `/app/recordings`, `/history/:id` | Завершённые встречи и материалы |
| `/analytics` | Аналитика одной завершённой или активной встречи |
| `/search`, `/app/search` | Поиск названий встреч, расшифровок и итогов |
| `/settings`, `/app/settings` | Профиль, устройства, уведомления, календарные интеграции |
| `/notifications`, `/admin` | Уведомления и защищённые операции администратора |

Все существующие aliases сохранены. Точный REST/WS mapping находится в
[плане интеграции](FRONTEND_INTEGRATION_PLAN.md). Query-параметры истории сохраняют
выбранную запись, вкладку и временную метку. Аналитика использует `view=past` по
умолчанию, `view=active` — по выбору; отсутствие фильтра нельзя трактовать как «все».

## Данные и жизненный цикл соединений

- `api.ts` — same-origin `/api/v1`, Bearer-токен, AbortSignal, нормализованные ошибки,
  безопасные signed URLs; ответы сервера не подменяются демонстрационными данными.
- `auth.tsx` — токен в памяти/sessionStorage, восстановление сессии и очистка Query
  cache при её смене. Старый ключ `meet.session.v1` сохранён для совместимости.
  Refresh endpoint и OAuth вход не придуманы.
- `queries.ts` — user-scoped query keys, серверные фильтры, cursor/offset pagination.
  Главная получает recent history отдельным ограниченным запросом, не запрашивает
  каждый элемент списка отдельно. Поиск в истории явно ограничен загруженными страницами.
- `realtime.ts` (включая `useRealtime`) — существующие ticket, WS, snapshot/reconnect,
  reconciliation и cleanup. События являются авторитетным состоянием комнаты.
- `media.ts`/`useMedia.ts` — существующий SFU/WebRTC client: admission → signaling
  → publication; SDP/ICE, server policy, exclusive screen sharing и reconnect не
  заменены новой реализацией ради дизайна. MediaStream не сохраняется в Query cache.
- `MediaTile` меняет `srcObject` при изменении самого потока, а не каждого события
  чата/участника; при удалении видео отвязывает поток. Управление tracks остаётся
  у владельца media lifecycle.
- Prejoin получает устройства только после явного действия, освобождает preview
  при выходе. Native `getDisplayMedia` показывает системный выбор окна/экрана.
- `personalRealtime.ts` создаёт account WS с одноразовым билетом, восстановлением
  истории при reconnect, ограниченным окном дедупликации и очисткой при logout.
  Контракты: [PERSONAL_MESSAGES.md](PERSONAL_MESSAGES.md).
- `useNotificationStream` создаёт одну SSE-подписку оболочки. Тяжёлые room/history
  панели загружаются по требованию; transcript/segments/summary не запрашиваются
  заранее, если соответствующая вкладка не открыта.

## Возможности и безопасность

UI скрывает/блокирует AI, transcript, captions, analytics и semantic search
по эффективным `/capabilities`. Новые gated-разделы истории/аналитики, навигация,
поиск и room captions не используют устаревшее разрешение после ошибки обновления.
Календарные интеграции отдельно получают `/integrations/capabilities`; это прежняя
панель с собственными статусами и ошибками. Клиентские проверки не заменяют ACL сервера. Owner/co-host
различаются: роль и выключение чужой камеры остаются owner-only.

Google/Microsoft на входе — **disabled-заглушки «Скоро»**, по решению владельца.
Они не открывают OAuth, не выдают токен и не показывают ложный успешный вход.
OAuth календаря в настройках — отдельная существующая интеграция.

Текст чата и AI выводится средствами React, без нового `dangerouslySetInnerHTML`.
Загрузка вложений сохраняет серверный allowlist и лимит 10 МБ на файл; приватные
медиа скачиваются по выданным сервером ссылкам. Секреты никогда не помещаются
в `VITE_*`, артефакты сборки или console logging. Серверная телеметрия допускает
новые статические пути `/calendar` и `/analytics`, но не идентификаторы встреч.

## Дизайн и адаптивность

`src/brand.ts` — единственный источник имени продукта; `VITE_PRODUCT_NAME` меняет
его во время сборки. Default — Meetrix. `public/brand-mark.svg` — кодовый знак.
`tokens.css` содержит semantic colors, поверхности комнаты, spacing/radius/shadow,
типографику и z-index. Старые `--blue`, `--line`, `--soft` — aliases для совместимости.
`workspace.css` задаёт оболочку/auth, остальные стили ограничены страницами.

Desktop: светлый кабинет с sidebar/topbar, тёмная активная комната без внешней
навигации. Tablet ≤1100px — компактная сетка; mobile ≤760px — drawer, нижняя
навигация и список календаря. Панели комнаты открываются поверх сцены на узком
экране. Формы и таблицы не расширяют страницу за viewport; широкая техническая
таблица аналитики прокручивается внутри своего контейнера.

Есть skip-link, labels, aria-live состояния, keyboard tabs, Escape/focus return,
focus trap диалогов/drawer и reduced motion. Горячие клавиши комнаты не действуют
при наборе текста или открытом диалоге. Автоматические axe-тесты в jsdom не
проверяют реальный контраст; визуальная и ручная клавиатурная проверка дополняет их.

## Настройка, запуск и сборка

Требуется версия Node из `package.json.engines` (проверено Node 24.19.0).
Из каталога `frontend`:

```sh
npm ci
npm run dev
npm run build
npm run preview
```

Dev-сервер слушает только `127.0.0.1:5173`; API proxy по умолчанию направлен на
`http://127.0.0.1:8085`. Изменение порта API: `API_PROXY_TARGET=http://127.0.0.1:28085
npm run dev`. Серверный `WS_ALLOWED_ORIGINS` должен разрешать фактический origin
UI; подменять Origin для обхода защиты нельзя. Камера/микрофон требуют secure
context (HTTPS или разрешённый браузером localhost), а удалённый стенд — TLS/TURN.

Публичные настройки сборки:

| Настройка | Назначение |
| --- | --- |
| `VITE_PRODUCT_NAME` | Имя; образ Docker принимает `--build-arg PRODUCT_NAME=Meetrix` |
| `VITE_BUILD_VERSION`, `VITE_BUILD_COMMIT`, `VITE_BUILD_TIME` | Публичные release metadata |
| `VITE_CLIENT_TELEMETRY_ENABLED` | Существующий явный переключатель безопасной телеметрии |
| `API_PROXY_TARGET` | Только proxy Vite в разработке, не секрет и не production API URL |

`npm run preview` не заменяет production reverse proxy и его API/WS/TLS/CSP.
Для production используются существующий Dockerfile, Nginx, CSP validation и
release automation; редизайн не разворачивает новый образ автоматически.

## Проверки

```sh
npm run lint
npm run typecheck
npm test
npm run test:a11y
npm run test:e2e
npm run test:e2e:all
bash scripts/test-csp-config.sh
```

Обычная Chromium suite содержит test-only API fixtures и сохраняет screenshots
в `test-results/ui`. `calendar-dashboard`, `conference-layout`,
`history-analytics-visual` покрывают новые экраны. Production-код фикстуры не
импортирует. Live SFU проверяется `TestStageFourBrowserControls` с явными
локальными PostgreSQL/Redis test variables; тест создаёт отдельную БД и namespace,
после выхода проверяет отсутствие SFU ресурсов. Opt-in Docker suites нельзя
считать пройденными только потому, что они skipped в обычном запуске.

Новый `playwright.live-smoke.config.ts` проверяет production-сборку на уже
запущенной **изолированной** локальной репетиции `https://localhost:25482`
(API `127.0.0.1:28085`), не на основном Compose. После сборки, из `frontend/`:

```sh
MEET_FRONTEND_LIVE_SMOKE=true MEET_FRONTEND_DIST="$PWD/dist" \
  npx playwright test -c playwright.live-smoke.config.ts
```

Тест накладывает только HTML/assets на origin в отдельном browser context;
API/WS остаются настоящими, deployment не выполняется. Для Chromium выдано
разрешение local-network-access только этому тестовому origin. Создаются
синтетические аккаунты, встреча, сообщения, PDF и MP4; встреча закрывается в
cleanup, данные сохраняются для диагностики. Тест не меняет Docker/env, не
перезапускает сервисы и не удаляет пользовательские данные. Нужен работающий
recorder pipeline и доступная локальная HTTPS/storage конфигурация.

Фактические результаты и браузерные ограничения конкретной проверки указаны
в [итоговом отчёте](FRONTEND_INTEGRATION_REPORT.md). Build target Chrome120 /
Firefox120 / Safari17 — цель компиляции, а не доказательство тестирования устройств.
