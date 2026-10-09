window.MeetrixSourceAudits = {
  meetings: {
    scope:
      "Дизайн-аудит по локальному актуальному коду: landing, авторизация, главная, встречи, создание, приглашения, календарь, pre-join. Производственный код, данные и сервер не изменялись. Проверка статическая, поведение на опубликованном сервере не проверялось.",
    referenceRoot: "/Users/aleksandranickij/htdocs/go-recorder/",
    prompt:
      "/Users/aleksandranickij/Documents/PROMPT_MEETRIX_ADAPTIVE_UI_UX_DESIGN.md",
    routes: [
      {
        paths: ["/"],
        access: "Публичный",
        screen: "Приветственная страница",
        evidence: [
          "frontend/src/App.tsx:203",
          "frontend/src/pages/Landing.tsx:13",
        ],
        actions: [
          "Войти → /login",
          "Зарегистрироваться → /register",
          "Авторизованный пользователь → /app",
        ],
      },
      {
        paths: ["/login", "/register", "/register/success"],
        access: "Публичный",
        screen: "Вход, регистрация, результат регистрации",
        evidence: [
          "frontend/src/App.tsx:204",
          "frontend/src/pages/AuthPages.tsx:25",
          "internal/transport/http/platform_routes.go:18",
        ],
        actions: [
          "POST /api/v1/auth/login",
          "POST /api/v1/auth/register",
          "Безопасный next сохраняет возвращение к исходному маршруту",
          "Успех регистрации пытается авторизовать, иначе предлагает вход",
        ],
      },
      {
        paths: ["/app"],
        access: "Постоянный аккаунт",
        screen: "Главная",
        evidence: ["frontend/src/pages/Dashboard.tsx:191"],
        actions: [
          "GET /api/v1/me/conferences с view/scope/from/to/cursor",
          "Новая встреча",
          "Запланировать",
          "Присоединиться по ссылке",
          "До 4 встреч выбранной категории",
          "До 3 последних завершённых встреч",
        ],
      },
      {
        paths: ["/meetings", "/conferences"],
        access: "Постоянный аккаунт",
        screen: "Встречи",
        evidence: [
          "frontend/src/App.tsx:234",
          "frontend/src/pages/Dashboard.tsx:200",
          "frontend/src/pages/Dashboard.tsx:245",
        ],
        actions: [
          "Категории Предстоящие / Активные / Завершённые",
          "Все мои / Я организатор / Я участник",
          "Диапазон дат",
          "Поиск только среди загруженных, не глобальный API-поиск",
          "Курсорная загрузка ещё",
          "Завершённая встреча → /history/:id",
        ],
      },
      {
        paths: ["/meetings/new", "/conferences/new"],
        access: "Постоянный аккаунт",
        screen: "Создание модальным окном над главной",
        evidence: [
          "frontend/src/components/ConferenceModals.tsx:99",
          "internal/domain/conferences/conference.go:259",
        ],
        actions: [
          "POST /api/v1/conferences",
          "?scheduled=1 предвыбирает расписание",
          "?returnTo=calendar возвращает в календарь",
          "?at=локальное-время предзаполняет дату",
        ],
      },
      {
        paths: ["/calendar"],
        access: "Постоянный аккаунт",
        screen: "Календарь",
        evidence: [
          "frontend/src/pages/CalendarPage.tsx:227",
          "internal/infrastructure/postgres/conference_product_repository.go:178",
        ],
        actions: [
          "Неделя / месяц",
          "Сегодня / предыдущий / следующий период",
          "Только upcoming с scheduledAt",
          "Просмотр карточки встречи",
          "Владелец scheduled-встречи меняет расписание",
          "Владелец отменяет с подтверждением",
          "Без drag-and-drop и повторяющихся встреч",
        ],
      },
      {
        paths: ["/i/:code"],
        access: "Публичный",
        screen: "Публичное приглашение и pre-join",
        evidence: [
          "frontend/src/pages/InvitePage.tsx:8",
          "internal/transport/http/guest_routes.go:10",
          "internal/usecase/conferences/guests.go:31",
        ],
        actions: [
          "GET /api/v1/conference-invites/:code",
          "POST /api/v1/conference-invites/:code/guest для гостя",
          "POST /api/v1/conference-invites/:code/join для аккаунта",
          "Имя гостя",
          "Предпросмотр камеры/микрофона",
          "Настройки устройств",
        ],
      },
      {
        paths: ["/meetings/:id/join", "/conferences/:id/join"],
        access: "Аккаунт либо scoped guest соответствующей встречи",
        screen: "Проверка перед входом из комнаты",
        evidence: [
          "frontend/src/App.tsx:160",
          "frontend/src/pages/PreJoinPage.tsx:922",
        ],
        actions: [
          "Проверить микрофон и камеру",
          "Выбрать устройства",
          "Войти без включённых устройств",
          "POST /api/v1/conferences/:id/join либо приглашение",
        ],
      },
    ],
    constraints: [
      {
        id: "auth-form",
        rule: "Вход: email и пароль, регистрация: email, пароль 8–128 Unicode-символов, необязательное имя до 100 символов. Подтверждения email, действующего восстановления пароля и OAuth-входа нет. Google/Microsoft — только отключённые заглушки по прежнему запросу пользователя; в новой основной форме лучше убрать как несуществующие действия, либо явно оставить disabled «Позже». Не рисовать отправку письма восстановления.",
        evidence: [
          "frontend/src/pages/AuthPages.tsx:49",
          "frontend/src/pages/AuthPages.tsx:165",
          "frontend/src/pages/AuthPages.tsx:191",
          "frontend/src/pages/AuthPages.tsx:233",
          "internal/domain/users/user.go:179",
        ],
      },
      {
        id: "meeting-fields",
        rule: "Создание поддерживает только title (1–200), waitingRoomEnabled, scheduledAt в будущем, plannedDurationMin (необязательно, целое 1–1440). Нет description, recurrence, автозаписи, выбора участников сразу в create payload, обоев, настроек качества. Запуск встречи ручной, планирование не означает автозапуск.",
        evidence: [
          "internal/domain/conferences/conference.go:259",
          "internal/domain/conferences/product.go:77",
          "frontend/src/components/ConferenceModals.tsx:129",
          "frontend/src/components/ConferenceModals.tsx:276",
        ],
      },
      {
        id: "meeting-card-metadata",
        rule: "Основной Conference View имеет ownerId, но не имя организатора и не participantCount. В карточках списка разрешены название, состояние, известная дата/время, плановая длительность и признак «Вы организатор» через ownerId. Не выдумывать имена и счётчики; отдельные API истории дают больше данных только на детальном экране.",
        evidence: [
          "internal/domain/conferences/conference.go:155",
          "frontend/src/pages/Dashboard.tsx:63",
          "frontend/src/types.ts:58",
        ],
      },
      {
        id: "search",
        rule: "Поиск встреч локальный среди загруженных страниц; интерфейс должен сообщать это явно. Нет серверного q/title в TimelineQuery. Кнопка «Загрузить ещё» остаётся доступной при отсутствии совпадений в загруженной части.",
        evidence: [
          "frontend/src/pages/Dashboard.tsx:245",
          "frontend/src/pages/Dashboard.tsx:325",
          "frontend/src/pages/Dashboard.tsx:410",
          "internal/domain/conferences/product.go:102",
        ],
      },
      {
        id: "email-invitations",
        rule: "Ссылка копируется, email и найденные зарегистрированные пользователи отправляются через один API. До 20 адресатов, поиск по имени/email от 2 до 100 символов. Право отправки: постоянный аккаунт-владелец в created/scheduled/active, либо joined/admitted co_host в active. После ответа показывать «Приглашение поставлено в очередь», «Уже отправлялось», «Пользователь покинул чат» — не обещать доставку SMTP. Не показывать успешную внешнюю доставку без подтверждения.",
        evidence: [
          "frontend/src/components/ConferenceInvitations.tsx:11",
          "frontend/src/components/ConferenceInvitations.tsx:265",
          "internal/usecase/conferences/invitations.go:21",
          "internal/infrastructure/postgres/conference_invitation_repository.go:26",
          "internal/infrastructure/postgres/conference_invitation_repository.go:159",
        ],
      },
      {
        id: "guest",
        rule: "Гость по приглашению действительно работает и не требует регистрации. Сессия scoped на одну встречу, без кабинета/списков/личных переписок. Только created и active доступны гостю. На scheduled — ожидание старта; авторизованный пользователь может добавить встречу в свой список. finished/cancelled — завершена/недоступна. Гостевое имя нормализуется сервером; UI предел 100. Камера/микрофон до допуска локальны.",
        evidence: [
          "internal/infrastructure/postgres/guest_repository.go:31",
          "internal/usecase/conferences/guests.go:50",
          "frontend/src/App.tsx:160",
          "frontend/src/pages/PreJoinPage.tsx:547",
          "frontend/src/pages/PreJoinPage.tsx:749",
          "frontend/src/components/Layout.tsx:141",
        ],
      },
      {
        id: "waiting-room-limitation",
        rule: "Валидная ссылка приглашения даёт немедленный допуск и обходит зал ожидания, включая ранее waiting-членство. Rejected/kicked не допускаются повторно. В текущем Join новая membership без invite запрещена до ветки создания waiting, поэтому waiting-room новый сценарий фактически недостижим стандартным входом. Макет waiting оставить только как существующее серверное состояние старого/созданного другим путём членства, документировать gap; не рисовать гостя по валидной ссылке ожидающим обязательного согласования.",
        evidence: [
          "internal/infrastructure/postgres/conference_repository.go:235",
          "internal/infrastructure/postgres/conference_repository.go:255",
          "internal/infrastructure/postgres/conference_repository.go:264",
          "internal/infrastructure/postgres/conference_product_repository.go:25",
        ],
      },
      {
        id: "calendar-limitation",
        rule: "Текущий календарь запрашивает только upcoming, затем отбрасывает записи без scheduledAt. Поэтому active/finished/cancelled в основной сетке не показывать; не подменять createdAt запланированной датой. UI week/month меняет локальную группировку, не внешний календарь. Результат частичный при hasNextPage — нужен заметный индикатор и загрузка.",
        evidence: [
          "frontend/src/pages/CalendarPage.tsx:234",
          "frontend/src/pages/CalendarPage.tsx:74",
          "frontend/src/pages/CalendarPage.tsx:507",
          "internal/infrastructure/postgres/conference_product_repository.go:178",
        ],
      },
      {
        id: "capabilities",
        rule: "Auth/Home/Create/Calendar/guest не имеют отдельного флага доступности. Глобальные capabilities: liveCaptions/transcription/aiSummary/semanticSearch/meetingAnalytics/recordingModes. Аналитика навигации только при успешном capability true; неизвестное/ошибка не означает включено. На Home не добавлять общие AI-сводки, глобальный поиск, статистику. Analytics и admin — условные существующие маршруты, не обязательная основная IA.",
        evidence: [
          "internal/domain/platform/platform.go:7",
          "frontend/src/useCapabilities.ts:6",
          "frontend/src/components/Layout.tsx:157",
        ],
      },
    ],
    auditFindings: [
      {
        priority: "high",
        title: "Регистрация сообщает неверное отсутствие гостевого входа",
        evidence: [
          "frontend/src/pages/AuthPages.tsx:220",
          "internal/transport/http/guest_routes.go:10",
        ],
        design:
          "Удалить устаревшее сообщение; допустима нейтральная подпись «Для участия по приглашению аккаунт не обязателен». Подтверждение email не обещать.",
      },
      {
        priority: "high",
        title: "Зал ожидания описан как обычный вход, но invite обходит его",
        evidence: [
          "frontend/src/components/ConferenceModals.tsx:241",
          "internal/infrastructure/postgres/conference_repository.go:235",
        ],
        design:
          "Не делать безопасность приглашения зависимой от предполагаемого допуска. В дизайн-аннотации зафиксировать gap, опцию не представлять как защиту каждого входа по ссылке.",
      },
      {
        priority: "medium",
        title: "Различные визуальные и медиа-сценарии двух pre-join",
        evidence: [
          "frontend/src/pages/PreJoinPage.tsx:489",
          "frontend/src/pages/PreJoinPage.tsx:673",
          "frontend/src/pages/PreJoinPage.tsx:922",
        ],
        design:
          "Общий компонент preview + controls + identity + devices. Публичный вход автопредпросмотр и очередь включения медиа; внутренний вход проверка вручную и последующее включение в комнате. На макете пояснить различия, не менять семантику скрыто.",
      },
      {
        priority: "medium",
        title: "Слишком мелкие элементы в календаре и меню встречи",
        evidence: [
          "frontend/src/pages/calendar.css:226",
          "frontend/src/pages/calendar.css:45",
          "frontend/src/pages/dashboard.css:85",
        ],
        design:
          "44×44 минимум для add-day (сейчас 26), view-switch (36), meeting overflow (38). Calendar caption/time сейчас 10px: увеличить до 12–13px, основные строки 14–16px.",
      },
      {
        priority: "medium",
        title: "Адаптация календаря переключается лишь на 600px",
        evidence: [
          "frontend/src/pages/calendar.css:76",
          "frontend/src/pages/calendar.css:184",
          "frontend/src/pages/calendar.css:307",
        ],
        design:
          "При 601–767 остаётся сетка min-width 760/770. В дизайне до 767 compact calendar + agenda; 768–1023 agenda по умолчанию или grid при реальной ширине контента >=760. Никакого узкого семиколоночного desktop на телефоне.",
      },
      {
        priority: "medium",
        title: "Создание на телефоне остаётся тесным центральным диалогом",
        evidence: [
          "frontend/src/styles.css:1698",
          "frontend/src/styles.css:2396",
        ],
        design:
          "390px: full-screen dialog 100dvh, header 56px, поля 16px, sticky submit/footer safe-area, scroll только формы. 768/1440: modal 520px max-width, padding32.",
      },
      {
        priority: "medium",
        title: "Форма показывает общую ошибку без связи с конкретным полем",
        evidence: [
          "frontend/src/pages/AuthPages.tsx:113",
          "frontend/src/pages/AuthPages.tsx:119",
          "frontend/src/components/ConferenceModals.tsx:280",
        ],
        design:
          "На макетах нужны inline field error, aria-invalid/describedby и фокус первого неверного поля при submit; серверная ошибка отдельным alert. Существующий ErrorNotice role=alert сохранить.",
      },
      {
        priority: "medium",
        title: "UI изменения расписания шире серверного правила",
        evidence: [
          "frontend/src/pages/CalendarPage.tsx:137",
          "internal/infrastructure/postgres/conference_product_repository.go:114",
        ],
        design:
          "Изменить расписание только status=scheduled и owner. created не имеет даты и обычно не попадает в календарь, но reusable permissions должны соответствовать backend.",
      },
      {
        priority: "low",
        title: "Смешение названий «Конференция», «Встреча», «История встреч»",
        evidence: [
          "frontend/src/pages/Dashboard.tsx:257",
          "frontend/src/pages/Dashboard.tsx:279",
          "frontend/src/components/ConferenceModals.tsx:203",
        ],
        design:
          "Единый пользовательский термин «Встреча», категория «Завершённые». Маршруты не менять. История остаётся материалами и deep-link, пункт меню не возвращать после явного удаления пользователем.",
      },
      {
        priority: "positive",
        title: "Базовые состояния и клавиатура уже присутствуют",
        evidence: [
          "frontend/src/pages/Dashboard.tsx:33",
          "frontend/src/pages/Dashboard.tsx:228",
          "frontend/src/components/ui.tsx:202",
          "frontend/src/pages/CalendarPage.tsx:355",
          "frontend/src/prejoinDevices.ts:133",
        ],
        design:
          "Сохранить skeleton/empty/retry/pagination, ARIA-вкладки со стрелками/Home/End, modal focus trap/Escape/restore, безопасные сообщения устройств и fallback системного устройства.",
      },
    ],
    proposedIA: {
      desktop: [
        "Главная /app",
        "Встречи /conferences (+ /meetings alias)",
        "Личные /personal",
        "Календарь /calendar",
        "Записи /recordings",
        "Личное пространство: Папки /folders",
        "Настройки — модальное окно; прямые /settings и /app/settings поддерживаются",
      ],
      mobile: [
        "Главная",
        "Встречи",
        "Личные",
        "Записи",
        "Ещё: Календарь, Папки, Настройки; профиль/выход",
      ],
      conditional: [
        "Аналитика только при capability meetingAnalytics",
        "Администрирование только isAdmin",
      ],
      deepLinksNotMainNav: ["/history", "/history/:id"],
      guestShell:
        "Только текущая встреча и допустимые настройки устройств/оформления; без навигации кабинета.",
      evidence: [
        "frontend/src/components/Layout.tsx:141",
        "frontend/src/components/Layout.tsx:157",
        "frontend/src/App.tsx:225",
      ],
    },
    mockupSpecifications: [
      {
        screen: "Landing",
        desktop1440:
          "Контейнер1200, header72, основной блок 2 колонки 560/560 с gap80; h1 48/56, body18/28, CTA48. Визуальный справа — схематичный компонент встречи, не новая декоративная иллюстрация.",
        tablet834:
          "Контейнер770, header64, hero текст/превью вертикально, h1 40/48.",
        mobile390:
          "Padding16, header56; бренд и Войти; h1 36/42; CTA на всю ширину; только реальные преимущества и компактное preview.",
        states: ["Гость: Войти/Зарегистрироваться", "Аккаунт: В приложение"],
      },
      {
        screen: "Login",
        desktop1440:
          "Спокойный светлый фон, brand слева в header, центр card440 padding32. Email, password с show/hide, Войти48, ссылка регистрации. Ошибка над формой, поле с ошибкой подчёркнуто текстом.",
        tablet834: "Card440 по центру, без декоративного сайдбара.",
        mobile390:
          "Одна колонка width358, padding16, поля48/текст16; не масштабировать форму. При клавиатуре footer прокручивается.",
        states: [
          "Пустая форма",
          "Неверные данные",
          "Сессия истекла",
          "Запрос в работе",
          "Сервер недоступен",
          "Восстановление: только информационное сообщение, не форма отправки",
          "Google/Microsoft disabled only if retained",
        ],
      },
      {
        screen: "Registration + Success",
        desktop1440:
          "Card480 padding32; Email, пароль с подписью 8–128 символов, необязательное имя; один primary48. Success тот же card с нейтральной иконкой и одним CTA.",
        tablet834: "Card480, окружающие поля32.",
        mobile390:
          "Полноширинная форма358, поля48, вертикальный gap20, CTA48, success card без лишней высоты.",
        states: [
          "Валидация поля",
          "Email занят",
          "Отправка",
          "Регистрация успешна и авторизован",
          "Регистрация успешна, требуется вход",
        ],
      },
      {
        screen: "Home",
        desktop1440:
          "Shell sidebar240/topbar64. Content max1120 padding32. Заголовок+дата. Три quick actions48. Список встреч4 строк80–96 с tabs; недавние встречи3 cards по1/3. Не рисовать аналитические KPI/фиктивные записи.",
        tablet834:
          "Icon sidebar72 либо drawer, topbar64, content padding24; actions wrap; recent cards в2 колонки с переносом.",
        mobile390:
          "Topbar56+bottomNav64+safeArea. Padding16. Новая встреча full-width48, ниже Запланировать/По ссылке по1/2. Tabs44, строки-карточки118; recent vertical.",
        states: [
          "Skeleton3 строки",
          "Нет будущих встреч + Создать",
          "Нет активных",
          "Нет завершённых",
          "Загрузка не удалась + Повторить",
          "Данные сохранены при фоновом обновлении",
        ],
      },
      {
        screen: "Meetings",
        desktop1440:
          "Header Встречи + Новая встреча. Tabs44. Поиск среди загруженных + scope select + даты. Карточка/строка с заголовком, известной датой, длительностью, состоянием, ролью «Вы организатор» при ownerId; menu44 и открыть. Нет выдуманных аватаров/количеств.",
        tablet834:
          "Фильтры в2 строки; date range одним блоком; карточки одной колонкой.",
        mobile390:
          "Tabs горизонтальные44 с прокруткой при text-scale; поиск48; Фильтры открывает sheet с теми же полями; cards полноширинные; Загрузить ещё48 после списка даже при локальном no-results.",
        states: [
          "Каждая категория пустая",
          "Ничего среди загруженных + Загрузить ещё",
          "Ошибка + повтор",
          "Пагинация pending",
          "Активная встреча",
          "Отменена",
          "Завершена → материалы",
        ],
      },
      {
        screen: "Create Meeting",
        desktop1440:
          "Modal520 padding32, header24/32. Title48. Два checkbox/switch rows56: Зал ожидания, Запланировать. Если planned: datetime48, timezone caption13, duration48. Текст ручного старта. Footer primary48/cancel44.",
        tablet834: "Тот же modal520, max-height viewport-48 со scroll body.",
        mobile390:
          "Fullscreen sheet width390, header56 close44, body padding16 gap20, footer sticky primary48 + cancel44, safe-area. Без описания и автозаписи.",
        states: [
          "Немедленная",
          "Запланированная",
          "Invalid title/date/duration",
          "Pending и повторный submit disabled",
          "Сервер отказал, поля сохранены",
          "Успех → share",
        ],
      },
      {
        screen: "Share / Invite participants / Join by link",
        desktop1440:
          "Modal520. Share: создана+link readonly+Copy44, Открыть встречу48, Пригласить. Invite: link, email textarea, поиск2+, chips выбранных и results checkbox rows56, результат по адресату; footer48. Join-by-link: одно поле и Открыть приглашение.",
        tablet834:
          "Modal520, scroll body, результат очереди читабельными строками.",
        mobile390:
          "Fullscreen dialog, link+copy в одной строке при вместимости, поля16px, chips removable44, результаты search full-width, sticky footer.",
        states: [
          "Clipboard success/error/manual selection",
          "Поиск loading/empty/error",
          "Лимит20",
          "Некорректный email",
          "Queue success—not delivered",
          "already_invited",
          "left_chat",
          "403/409 безопасное пояснение",
        ],
      },
      {
        screen: "Calendar",
        desktop1440:
          "Shell240/top64. Content padding32. Toolbar Сегодня/Prev/Next/title/Неделя-Месяц и Создать. Неделя: 7 дней, время56, header64, hour slot56, 24ч scroll. Month:7 колонок, day min112; add44. Карточка встречи modal480; только реальные planned upcoming.",
        tablet834:
          "Shell72, content padding24, agenda default если inner<760; можно switch Месяц/Неделя, компактный выбор периода. Недельная сетка только при достаточной ширине, без обрезанного текста.",
        mobile390:
          "Topbar/bottomNav, компактный date/week strip, подпись выбранного периода, grouped agenda cards; размер дня44. Не показывать min-width770 таблицу. Кнопка Новая48. Sheet карточки с реальными действиями.",
        states: [
          "Skeleton agenda/grid",
          "Нет встреч в периоде",
          "Ошибка + Повторить",
          "Показана часть периода + Загрузить ещё",
          "Owner scheduled edit/cancel",
          "Member read-only",
          "Cancel confirmation danger",
        ],
      },
      {
        screen: "Pre-join / public invitation",
        desktop1440:
          "Унифицированная тёмная preview surface: container1080, preview640×360 + settings360 с gap32. Header meeting title+status. Mic/camera controls48, audio meter. Слева имя гостя или аккаунт; справа devices dropdown48, main Join48. Public guest без app shell.",
        tablet834:
          "Container740, preview16:9, под ним controls48, затем identity/devices в одной/двух колонках по доступной ширине, Join48.",
        mobile390:
          "Preview358×201 сверху, mic/camera48 под ним; имя48, devices disclosure44→sheet, громкость meter, Join52; safe-area. Preview не перекрывает ошибки и клавиатуру. Войти без устройств возможно той же Join при muted/off.",
        states: [
          "Preview off initials",
          "Permission pending",
          "Permission denied",
          "Устройство занято",
          "Устройства нет",
          "Устройство отключено → системное",
          "Playback blocked → Запустить preview",
          "Небезопасное соединение",
          "Встреча ещё scheduled",
          "Closed/rejected/kicked",
          "Identity/session changed",
          "Loading membership",
          "API unavailable retry",
        ],
      },
    ],
    interactionAndA11y: [
      "Текст body16/24, secondary14/20, caption12/16, поля16px на мобильном; учитывать сохранённый процент текста 80–150/поддерживаемый список без fixed-height clipping.",
      "Минимум44×44 для каждого клика; для плотных desktop календарей допускается декоративный26px icon внутри44px target, не уменьшать hit area.",
      "Focus-visible2px primary +2px offset; modal focus trap/Escape/возврат инициатору; при pending остаётся доступное объяснение загрузки, повтор не отправляет запрос.",
      "Tabs реальные ARIA tabs со стрелками/Home/End, switch views aria-pressed; иконки декоративны либо имеют aria-label.",
      "Валидация inline aria-invalid/describedby, summary role=alert; skeleton role=status, контентный регион aria-busy; не озвучивать каждую выборку audio meter.",
      "Статусы сопровождаются текстом, не только цветом. Reduce-motion убирает декоративные переходы, не скрывает состояние.",
      "Не выдавать pending приглашения за доставленное письмо; копирование имеет idle/copied/error с возможностью выбрать URL вручную.",
      "Desktop1440:sidebar240/topbar64/padding32; laptop1024–1439:sidebar220/padding24; tablet768–1023:rail72 или drawer/padding24; mobile360–767:top56/bottom64/padding16+safe-area. Border1/radius12 карточки/16диалоги, spacing4/8/12/16/20/24/32. Это предложенные дизайн-параметры, не утверждение о текущем CSS.",
    ],
    implementationPriorityForLater: [
      "P1: исправить copy гостевого входа и согласовать ограничения waiting room; унифицировать layout двух pre-join; обозначить фактические права/состояния",
      "P1: mobile shell и fullscreen forms, минимум44px, читаемость и inline errors",
      "P2: dashboard/meetings единые карточки и фильтры; поиск честно локальный",
      "P2: calendar agenda на всей mobile ширине, промежуточный tablet layout, pagination visibility",
      "P3: invitation results и reusable components. Автоматически к реализации не переходить.",
    ],
  },
  messaging: {
    scope:
      "Аудит кода для дизайна: личные и групповые чаты, папки, настройки, уведомления. Ни frontend, ни backend не изменялись.",
    sourceRoot: "/Users/aleksandranickij/htdocs/go-recorder",
    brief:
      "/Users/aleksandranickij/Documents/PROMPT_MEETRIX_ADAPTIVE_UI_UX_DESIGN.md",
    contractRule:
      "Фактический код имеет приоритет над примерами брифа. История не возвращается в навигацию; Интеграции и Безопасность не возвращаются в настройки.",
    routes: [
      {
        path: "/personal",
        screen: "Список личных и групповых переписок; пустая выбранная область",
        auth: "Зарегистрированная учётная запись",
        source: "frontend/src/App.tsx:230",
      },
      {
        path: "/personal/:id",
        screen: "Личный или групповой чат по type из API",
        auth: "Активный участник диалога",
        source: "frontend/src/App.tsx:231",
      },
      {
        path: "/folders",
        screen: "Все личные папки",
        auth: "Зарегистрированная учётная запись",
        source: "frontend/src/App.tsx:228",
      },
      {
        path: "/folders/:id",
        screen: "Встречи и переписки в своей папке",
        auth: "Владелец папки",
        source: "frontend/src/App.tsx:229",
      },
      {
        path: "/notifications",
        screen: "Полный список уведомлений",
        auth: "Зарегистрированная учётная запись",
        source: "frontend/src/App.tsx:249",
      },
      {
        path: "/app/settings | /settings",
        screen:
          "Модальное окно Настройки аккаунта; обычный клик Настройки открывает поверх текущего экрана",
        source: "frontend/src/components/Layout.tsx:56",
      },
      {
        path: "/history | /history/:id",
        screen:
          "Совместимые прямые маршруты материалов встреч; отсутствуют в основном меню",
        source: "frontend/src/App.tsx:247",
      },
    ],
    navigation: {
      actualWorkspace: [
        "Главная /app",
        "Встречи /conferences",
        "Личные /personal",
        "Календарь /calendar",
        "Записи /recordings",
      ],
      actualConditional: [
        "Аналитика /analytics только capabilities.meetingAnalytics",
        "Администрирование /admin только user.isAdmin",
      ],
      personalSpace: ["Папки /folders"],
      service: [
        "Настройки как модальный сценарий",
        "Уведомления из topbar",
        "Профиль и выход",
      ],
      source: "frontend/src/components/Layout.tsx:157",
      proposed:
        "Сохранить текущие разделы. В Личные заменить единственную CTA Создать группу меню Новый чат с вариантами Личный/Группа; личный сценарий уже поддержан API, но новая точка входа требует frontend wiring. Не добавлять Историю в Ещё.",
    },
    personalChat: {
      list: {
        fields: [
          "id",
          "type direct/group",
          "displayName или name",
          "initials или приватный group avatar",
          "preview",
          "lastMessageAt",
          "unreadCount",
          "direct notificationsEnabled",
          "group lastSender",
        ],
        filters: [
          "Все",
          "Личные type=direct",
          "Группы type=group",
          "Новые unreadOnly=true",
        ],
        search:
          "По имени собеседника или названию группы; 0–100 символов; задержка 300 мс; URL search/filter сохраняются при переходах",
        pagination:
          "cursor before; frontend запрашивает 50; кнопка Ещё переписки; сервер максимум 100",
        sources: [
          "frontend/src/pages/PersonalPage.tsx:89",
          "frontend/src/pages/PersonalPage.tsx:164",
          "internal/domain/personal/groups.go:134",
          "internal/infrastructure/postgres/personal_repository.go:152",
        ],
      },
      directStart: {
        api: [
          "GET /api/v1/users?search=...",
          "POST /api/v1/conversations/direct {userId}",
        ],
        currentEntry:
          "Написать участнику из информации о чате встречи; отдельной CTA Новый личный чат в PersonalPage сейчас нет",
        searchConstraints:
          "Имя 2–100 символов; до 20 результатов; без самого себя и гостей",
        sources: [
          "frontend/src/components/ConferenceChatInfo.tsx:690",
          "frontend/src/api.ts:1105",
          "internal/infrastructure/postgres/personal_repository.go:224",
        ],
      },
      directActions: [
        {
          label: "Информация",
          effect:
            "Стандартный Modal; имя, инициалы, UUID. Клик имени/аватара header открывает тот же сценарий.",
          limits:
            "API не даёт email, фото или присутствие собеседника; не рисовать эти поля",
          source: "frontend/src/components/DirectConversationActions.tsx:44",
        },
        {
          label: "Без уведомлений / Включить уведомления",
          api: "PATCH /api/v1/conversations/:id/preferences {notificationsEnabled}",
          effect:
            "Личная настройка; не меняет настройки собеседника. BellOff в списке и подпись в header.",
          source: "frontend/src/components/DirectConversationActions.tsx:274",
        },
        {
          label: "Добавить в папку",
          effect:
            "Открывает универсальный FolderPicker; множественное членство",
          source: "frontend/src/components/DirectConversationActions.tsx:287",
        },
        {
          label: "Очистить историю",
          api: "POST /api/v1/conversations/:id/clear-history",
          effect:
            "Необратимая очистка видимой истории только для себя. Диалог остаётся. Подтверждение, отмена по умолчанию.",
          source: "frontend/src/components/DirectConversationActions.tsx:325",
        },
        {
          label: "Удалить чат",
          api: "POST /api/v1/conversations/:id/hide",
          effect:
            "Скрывает у себя из списка/папок и очищает свою историю. Новый входящий ответ возвращает диалог. История собеседника не меняется.",
          source: "frontend/src/components/DirectConversationActions.tsx:330",
        },
      ],
      messages: {
        supported: [
          "Текст",
          "Unicode emoji: 64 существующих варианта",
          "Ответ с цитатой",
          "Редактировать своё сообщение",
          "Удалить своё сообщение с подтверждением",
          "Пометка изменено",
          "Удалённое сообщение как tombstone",
          "Разделители по датам",
          "Вложения",
          "Непрочитанные счётчики и личный курсор чтения",
          "Пагинация старых сообщений",
          "Отправка с pending/повтором при ошибке",
        ],
        unsupported: [
          "Голосовые сообщения",
          "Звонок из личного чата",
          "Стикер-паки/GIF",
          "Статус печатает",
          "Галочки прочитано собеседником",
          "Закрепление/Важное в личных и групповых чатах",
          "Модерация чужих сообщений в группах через текущий UI",
          "Форвард сообщений",
          "Email/last seen для собеседника",
        ],
        textLimit:
          "4000 Unicode codepoints; можно отправить только файл без текста",
        attachments:
          "До 5 файлов, каждый от 1 байта до 10 МиБ; JPG/JPEG/PNG/WebP/PDF/TXT/CSV. Прогресс, ошибка, повтор, отмена/убрать из очереди. Ссылка скачивания через backend, для group авторизованный content endpoint.",
        api: "/api/v1/conversations/:id/messages (GET/POST), /messages/:messageId (PATCH/DELETE), /read (POST), /chat/read (GET), /attachments/init (POST), /attachments/:id/content (PUT), /attachments/:id/finalize (POST), /attachments/:id/download (GET)",
        sources: [
          "frontend/src/components/ChatPanel.tsx:104",
          "frontend/src/components/ChatPanel.tsx:430",
          "frontend/src/components/ChatPanel.tsx:724",
          "frontend/src/components/AttachmentUploader.tsx:16",
          "frontend/src/emoji.ts:2",
          "internal/transport/http/chat_routes.go:40",
        ],
      },
      realtime: {
        transport:
          "userWSTicket → /api/v1/ws; message create/update/delete, read, membership, prefs, clear/hide, folder events; REST fallback/refetch",
        retry:
          "Exponential 1–30 seconds + jitter; refresh on reconnect; notices for messages outside active conversation; 6 second dismiss timer",
        gap: "Hook returns unread/notice/dismiss only. A visible connecting/reconnecting/failed indicator requires frontend connection-state plumbing; no backend protocol change needed. Do not imply already-present delivery/read receipts.",
        source: "frontend/src/personalRealtime.ts:206",
      },
      states: [
        "Первичная загрузка списка/истории",
        "Нет переписок",
        "Нет групп",
        "Нет непрочитанных",
        "Поиск ничего не нашёл",
        "Диалог не выбран",
        "Пустой диалог",
        "Отправляется",
        "Отправка не удалась + повтор",
        "Файл загружается/готов/ошибка/повтор",
        "Доступ отозван → возврат в список",
        "Сохранение действия",
        "Ошибка действия",
        "Подтверждение очистки/удаления",
        "Офлайн/переподключение — дизайн состояния с frontend gap",
      ],
    },
    groups: {
      fields: [
        "Название 1–50 символов",
        "Описание 0–200 символов",
        "Аватар PNG/JPEG до 2 МиБ",
        "memberCount максимум 100 вместе с владельцем",
        "myRole owner/admin/member",
        "Список участников: id/displayName/role/nullable online",
      ],
      create:
        "Зарегистрированный пользователь; можно создать без приглашённых, выбрать до 99 пользователей; поиск минимум 2 символа. clientRequestId защищает повтор. Если группа создана, но avatar upload не удался, Повторить загрузку / Открыть без аватара.",
      actionsByRole: {
        member: [
          "Открыть информацию и список участников",
          "Поиск среди участников",
          "Читать/отправлять сообщения",
          "Выйти из группы",
          "Добавить в свою папку",
        ],
        admin: [
          "Все действия member",
          "Добавить участников до общего лимита 100",
          "Изменить название/описание/аватар",
          "Убрать обычного member",
        ],
        owner: [
          "Все действия admin",
          "Назначить/снять admin",
          "Убрать admin/member",
          "Передать владение участнику (сам становится admin)",
          "Удалить группу для всех",
        ],
      },
      leaveConstraint:
        "Владелец при memberCount>1 сначала передаёт владение; единственный владелец может выйти с удалением группы. Удаление/выход — отдельное подтверждение.",
      presence:
        "Только в member list, только при online===true/false и доступной свежей сети; null/error/offline query → Статус недоступен. Не экстраполировать на direct peer.",
      api: [
        "POST /api/v1/conversations/group",
        "PATCH/DELETE /api/v1/conversations/:id",
        "GET/POST /api/v1/conversations/:id/members",
        "PATCH/DELETE /api/v1/conversations/:id/members/:userId",
        "POST /api/v1/conversations/:id/ownership",
        "POST /api/v1/conversations/:id/leave",
        "PUT/DELETE /api/v1/conversations/:id/avatar",
        "GET /api/v1/conversations/:id/avatar/content",
      ],
      sources: [
        "internal/domain/personal/groups.go:18",
        "internal/infrastructure/postgres/group_repository.go:37",
        "internal/infrastructure/postgres/group_repository.go:214",
        "internal/infrastructure/postgres/group_repository.go:253",
        "internal/infrastructure/postgres/group_repository.go:289",
        "internal/infrastructure/postgres/group_repository.go:323",
        "frontend/src/components/GroupChats.tsx:283",
        "frontend/src/components/GroupChats.tsx:665",
      ],
      states: [
        "Создание",
        "Валидация",
        "Поиск участников загрузка/пусто/ошибка",
        "Лимит участников",
        "Частичный успех: группа создана, аватар не загружен",
        "Информация загрузка/ошибка",
        "Участник удалён/права изменены",
        "Подтверждение исключения/выхода/передачи/удаления",
        "Потеря доступа",
      ],
    },
    folders: {
      kind: "Универсальные личные папки со ссылками на доступные встречи и direct/group чаты; не группы чатов и не совместные папки",
      fields: [
        "name 1–50 Unicode codepoints NFC",
        "position",
        "itemCount",
        "conversationCount",
        "conferenceCount",
      ],
      limits:
        "До 100 папок. Имена уникальны с case folding. Поиск элементов 0–100 символов. Элемент может входить в несколько папок. Папка не выдаёт приглашение или права на исходный объект.",
      listActions: [
        "Создать",
        "Открыть",
        "Переименовать",
        "Переместить выше/ниже",
        "Удалить папку (подтверждение; встречи/чаты сохраняются)",
      ],
      detailActions: [
        "Назад к папкам",
        "Фильтры Все/Встречи/Чаты",
        "Поиск по названию/собеседнику",
        "Добавить существующие элементы",
        "Открыть исходный объект",
        "Добавить элемент в другую папку",
        "Удалить ссылку из текущей папки",
        "Ещё элементы по cursor",
      ],
      api: [
        "GET/POST /api/v1/folders",
        "PUT /api/v1/folders/order",
        "GET/PATCH/DELETE /api/v1/folders/:id",
        "GET /api/v1/folders/:id/items",
        "GET /api/v1/folder-items",
        "PUT/DELETE /api/v1/folders/:id/items/:kind/:itemId",
      ],
      states: [
        "Загрузка",
        "Пустой список",
        "Пустая папка",
        "Поиск без результата",
        "Папка недоступна → список",
        "Элемент потерял доступ → скрыт",
        "Дубликат имени",
        "Достигнут лимит 100",
        "Сохранение/ошибка",
        "Перестановка",
        "Удаление/подтверждение",
      ],
      sources: [
        "internal/domain/folders/folders.go:15",
        "internal/transport/http/folder_routes.go:30",
        "frontend/src/pages/FoldersPage.tsx:45",
        "frontend/src/components/FolderModals.tsx:71",
        "frontend/src/components/FolderModals.tsx:204",
      ],
    },
    settings: {
      presentation:
        "Настройки аккаунта в standard Modal над активной страницей. Desktop nav | panel, гостям только Аудио/Видео/Оформление. Нельзя размонтировать активную встречу при открытии.",
      sections: {
        Профиль: {
          fields: [
            "Имя 1–100 символов, редактируется",
            "Email readonly",
            "Дата регистрации",
            "Аватар только инициалы",
          ],
          action:
            "Сохранить имя; pending/error/saved; email/photo/password менять нельзя",
        },
        Аудио: {
          fields: [
            "Микрофон select",
            "Проверить/остановить + уровень RMS",
            "Подключаться с выключенным микрофоном",
            "Шумоподавление при browser support",
            "Динамик select + Проверить",
            "Источник звука уведомлений select + Проверить",
          ],
          limits:
            "Устройства/настройки сохраняются в браузере; HTTPS; output select только setSinkId/selectAudioOutput support; permission denied/no device/busy/removed/loading states",
        },
        Видео: {
          fields: [
            "Предпросмотр камеры",
            "Камера select",
            "Подключаться с выключенной камерой",
            "Видеть себя на звонке",
            "Скрыть видео участников",
          ],
          limits:
            "Нет размытия/виртуальных фонов, ручного разрешения, beauty filters. Preview owns stream and stops on close/switch",
        },
        Уведомления: {
          events: [
            "Приглашения и изменения встреч",
            "Напоминания о встречах",
            "Готовность записи",
            "Расшифровка и итоги встречи",
          ],
          channels: ["Email", "Push"],
          limits:
            "Состояния каналов из integration capabilities: noop disabled, mock явно тестовый, http/smtp доступны. Push настройка не означает регистрацию web-push устройства. Приглашения организатора приходят отдельно от автоматических уведомлений.",
        },
        Оформление: {
          themes: ["Светлая по умолчанию", "Тёмная"],
          textPercent: [75, 90, 100, 110, 125, 150, 200],
          storage:
            "Мгновенно локально по аккаунту в этом браузере; нет синхронизации между устройствами; storage error сохраняет только до reload",
        },
      },
      excluded: [
        "Интеграции",
        "Безопасность",
        "Изменить email",
        "Загрузить фото профиля",
        "Изменить пароль",
        "Язык",
        "Часовой пояс",
        "Системная auto theme",
      ],
      sources: [
        "frontend/src/pages/AccountPages.tsx:40",
        "frontend/src/pages/AccountPages.tsx:100",
        "frontend/src/pages/AccountPages.tsx:297",
        "frontend/src/components/DeviceSettings.tsx:318",
        "frontend/src/components/DeviceSettings.tsx:495",
        "frontend/src/components/IntegrationsSettings.tsx:199",
        "frontend/src/appearance.tsx:14",
      ],
    },
    notifications: {
      surfaces:
        "Topbar bell с unread badge (99+), сейчас стандартный wide Modal; отдельная страница /notifications существует",
      fields: [
        "Тип события → человекочитаемый заголовок",
        "Дата/время",
        "readAt/unread style",
        "Ссылка Открыть встречу только если conferenceId есть",
      ],
      actions: [
        "Открыть встречу + пометить прочитанным",
        "Отметить одно как прочитанное",
        "Ещё уведомления cursor 30",
        "Повторить загрузку",
      ],
      api: [
        "GET /api/v1/notifications?limit=30&cursor=...",
        "POST /api/v1/notifications/:notificationId/read",
        "GET /api/v1/notifications/events (SSE)",
        "GET/PUT /api/v1/notifications/preferences",
      ],
      limits:
        "Нет отметить все, удалить, фильтра Все/Непрочитанные в текущем контракте списка. Личные message notices — отдельный WS toast, не автоматически долговечные rows notification inbox.",
      states: [
        "Непрочитанное",
        "Прочитанное",
        "Пусто",
        "Загрузка",
        "Ошибка + повтор",
        "Mark-read pending/error",
        "Следующая страница",
      ],
      sources: [
        "frontend/src/components/NotificationBell.tsx:48",
        "frontend/src/pages/NotificationsPage.tsx:17",
        "internal/transport/http/notification_routes.go:14",
        "internal/transport/http/integration_routes.go:45",
      ],
    },
    uxFindings: [
      {
        id: "MSG-01",
        severity: "medium",
        finding:
          "Личные переписки нельзя начать из главного экрана Личные: единственная CTA создаёт группу; existing direct API используется только из участника встречи.",
        design:
          "Меню Новый чат → Личный / Группа; единый поиск людей с тем же GET users. Отметить как новую frontend-точку входа, а не новый backend.",
        source: "frontend/src/pages/PersonalPage.tsx:162",
      },
      {
        id: "MSG-02",
        severity: "medium",
        finding:
          "На tablet 768 двухколоночная personal-page сохраняет 260–340px list до breakpoint 760; после shell и внутренних полей чат становится узким.",
        design:
          "На 768 tablet свернуть глобальный sidebar до icon rail и ограничить list 272px; при оставшейся ширине conversation <420px показывать list/detail раздельно. Mobile всегда single pane.",
        source: "frontend/src/pages/personal.css:3",
      },
      {
        id: "MSG-03",
        severity: "medium",
        finding:
          "Действия сообщения имеют min-height26px и текст10px; group role controls36px, delete-chip около22px. Не достигают brief target44px.",
        design:
          "44×44px все touch actions; message actions compact desktop menu и mobile context sheet, с клавиатурной альтернативой long press.",
        sources: [
          "frontend/src/styles.css:346",
          "frontend/src/components/group-chats.css:183",
          "frontend/src/components/group-chats.css:74",
        ],
      },
      {
        id: "MSG-04",
        severity: "medium",
        finding:
          "В личных/групповых стилях font-size12/13/24px фиксированы и не умножены на --text-scale; общий body scale не масштабирует явно заданный размер.",
        design:
          "Все типографические токены учитывают 75–200%; карточки/кнопки увеличиваются по контенту; проверить 200% без потери actions.",
        sources: [
          "frontend/src/pages/personal.css:25",
          "frontend/src/pages/personal.css:79",
          "frontend/src/components/group-chats.css:209",
          "frontend/src/appearance.css:2",
        ],
      },
      {
        id: "MSG-05",
        severity: "medium",
        finding:
          "WS reconnection silent, hook не возвращает connection status; интерфейс может выглядеть актуальным при восстановлении.",
        design:
          "Неблокирующая строка Переподключаемся и offline state композера с retry; требуется frontend instrumentation.",
        source: "frontend/src/personalRealtime.ts:401",
      },
      {
        id: "MSG-06",
        severity: "low",
        finding:
          "Group user picker после поиска без результатов показывает пустой ul; нет ясного пустого состояния.",
        design:
          "Никого не нашли + изменить имя. Аналогичное empty для фильтра участников.",
        source: "frontend/src/components/GroupChats.tsx:227",
      },
      {
        id: "SET-01",
        severity: "medium",
        finding:
          "В mobile settings остаются горизонтальные tabs внутри 600px modal; brief требует category list → subsection и full screen формы.",
        design:
          "Mobile fullscreen settings, категории списком, subsection с back. Desktop modal сохраняется. Не изменять фактические настройки.",
        source: "frontend/src/components/account-settings-modal.css:251",
      },
      {
        id: "STATE-01",
        severity: "low",
        finding:
          "Главные списки применяют Loading spinner, без стабильного skeleton каркаса.",
        design:
          "Skeleton list rows/message placeholders/notification rows одинаковой высоты; содержимое с aria-busy и один status announcement.",
        sources: [
          "frontend/src/pages/PersonalPage.tsx:207",
          "frontend/src/pages/FoldersPage.tsx:109",
          "frontend/src/pages/NotificationsPage.tsx:55",
        ],
      },
    ],
    reuse: [
      {
        component: "Modal",
        keep: "dialog role/aria-modal, label, Escape, focus trap/restore, safe autofocus",
        source: "frontend/src/components/ui.tsx:182",
      },
      {
        component: "ItemActions",
        keep: "Arrow/Home/End/Escape keyboard menu, focus restoration, viewport placement",
        source: "frontend/src/components/ItemActions.tsx:21",
      },
      {
        component: "MessageThread",
        keep: "Single shared behavior for meeting/direct/group, permissions remain contextual",
        source: "frontend/src/components/ChatPanel.tsx:84",
      },
      {
        component: "EmojiPicker",
        keep: "64 labels, arrow-grid navigation, Escape, focus return",
        source: "frontend/src/components/EmojiPicker.tsx:15",
      },
      {
        component: "FolderPicker/FolderItems/FolderManageModal",
        keep: "Reused across meetings and conversations; own references only",
        source: "frontend/src/components/FolderPicker.tsx:24",
      },
      {
        component: "Settings tabs",
        keep: "aria tabs, roving focus, Arrow/Home/End; add mobile category-list presentation",
        source: "frontend/src/pages/AccountPages.tsx:83",
      },
    ],
    implementationReadyDesignRecommendations: {
      desktop1440:
        "Global sidebar240; topbar72; content padding32. Messaging list320 and conversation flexible with minimum420; header72; composer pinned min64; message text14–16 and metadata12. Group info drawer360 or modal560; settings modal960 nav216/content. Notifications popover400 and page link.",
      laptop1024:
        "Sidebar208 or collapsed80 depending remaining content; padding24. Messaging list272, body minimum420. Reduce spacing, never text below14 for body. All actions44.",
      tablet768:
        "Icon rail72 and topbar64; padding16; messaging single pane when content cannot accommodate list272+thread420. Folder rows stack metadata. Dialog max720 and 24px padding; group/info as inset sheet.",
      mobile390:
        "Topbar56, bottom-nav64 + safe area. List screen then conversation fullscreen; hide redundant bottom nav during active conversation to expose composer and safe-area. Conversation top56, composer min56 plus queued uploads; respects visualViewport keyboard. Group create and settings fullscreen. Action menu bottom sheet with44–48px rows; notification full page.",
      messageStyle:
        "Incoming bubble neutral surface, own bubble pale blue/turquoise under brand tokens; max-width desktop70% / mobile86%; radius16 except tail corner4; gap4 grouped /16 new sender. No fake read ticks; pending/error only from existing request state.",
      states:
        "Create separate loading/empty/no-result/error/reconnect artboards for major screens. Replace technical failure strings with user-facing status and next action; do not imply server data exists.",
      accessibility:
        "44×44 target minimum, focus ring3px offset2, contrast4.5:1 text/3:1 controls, non-color unread and mute cues, pointer+keyboard menus, aria-live polite only status changes, focus restore and cancel-first destructive dialogs, reduced motion, test long Cyrillic names and200% text.",
      priority: [
        "P1 messaging mobile pane/composer/touch sizes",
        "P1 permissions-correct menus/destructive confirmations",
        "P1 unify scale-aware typography",
        "P2 new direct-chat entry backed by existing API",
        "P2 folders/notifications responsive list states",
        "P2 settings category navigation and device errors",
        "P2 explicit realtime reconnect state plumbing",
      ],
    },
  },
  recordings: {
    schemaVersion: 1,
    kind: "meetrix-ui-ux-source-audit",
    auditedAtLocalDate: "2026-10-09",
    timezone: "Europe/Moscow",
    sourceCommit: "120280c39ba267441e108063f7c21d152167b1c3",
    instructionFile:
      "/Users/aleksandranickij/Documents/PROMPT_MEETRIX_ADAPTIVE_UI_UX_DESIGN.md",
    scope: [
      "recordings",
      "recording-player",
      "secondary-history",
      "meeting-analytics",
      "read-only-admin",
    ],
    method:
      "Read-only inspection of current local frontend, API routes, use cases, repositories and CSS. No runtime capability flags or remote account data asserted; no application implementation or publication.",
    explicitConstraints: {
      historyNavigation:
        "Removed from desktop/mobile primary navigation. Keep /history and /history/:id as secondary routes reached from meeting materials/recordings; do not restore a primary History item.",
      excluded: [
        "global recordings catalog",
        "recording favorites",
        "recording deletion UI",
        "recording rename",
        "public sharing/permission grant",
        "fake aggregate analytics",
        "admin user management or feature toggle editing",
      ],
      recordingPlayer:
        "Simple standalone video/audio player with information and actual files; no AI/transcript/top-level recording tabs.",
      noMutations:
        "Only this audit JSON is created; application/backend/source/Git index/remote host unchanged.",
    },
    navigation: {
      primaryRecordings: "/recordings",
      aliases: ["/app/recordings"],
      player: "/recordings/:recordingId?conference=:conferenceId",
      history: ["/history", "/history/:conferenceId"],
      analytics: "/analytics?view=past|active&conference=:conferenceId",
      admin: "/admin",
      references: [
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/App.tsx:225",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/App.tsx:244",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/Layout.tsx:157",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:428",
      ],
      auth: "All routes are inside Protected/Layout. Anonymous sessions go through authentication; guest sessions are redirected to their conference rather than account sections.",
      authReference:
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/App.tsx:110",
      capabilityNav:
        "Analytics only after successful current meetingAnalytics=true; Admin only user.isAdmin=true, with backend enforcement on every request.",
    },
    dataContracts: {
      conference: {
        fields: [
          "id",
          "ownerId",
          "title",
          "status",
          "createdAt",
          "startedAt|null",
          "finishedAt|null",
          "scheduledAt?|null",
          "plannedDurationMin?|null",
        ],
        titleSemantics:
          "Recording display title is the meeting title, not a separately editable recording title.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/types.ts:58",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:69",
        ],
      },
      conferenceRecording: {
        fields: [
          "uuid",
          "conferenceId",
          "requestedBy?|null",
          "mode",
          "status",
          "createdAt",
          "startedAt?",
          "endedAt?",
          "durationSec?",
          "errorMessage?",
          "files[].fileType",
          "files[].url?",
          "files[].sizeBytes?",
        ],
        modes: ["composite", "audio_only", "individual_tracks", "screen_focus"],
        userVisibleMetadata: [
          "meeting title",
          "recording start date with creation-date fallback",
          "real duration if supplied",
          "real file size if supplied",
          "real preview if supplied",
          "optional historical organizer name",
          "optional historical participant count",
        ],
        statusesNotToExposeAsRawTechnicalBadges: [
          "starting",
          "recording",
          "degraded",
          "stopping",
          "processing",
          "ready",
          "failed",
          "cancelled",
          "partial_ready (backend-only discrepancy)",
        ],
        actualMaterialTypes: [
          "final_mp4",
          "final_audio",
          "preview_jpg",
          "tracks_archive",
        ],
        unavailableFields: [
          "independent recording name",
          "favorite flag",
          "delete permission",
          "view count",
          "explicit expiresAt",
          "thumbnails not generated by backend",
          "video subtitle/VTT track",
        ],
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/types.ts:173",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:28",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/recordingPresentation.ts:38",
        ],
      },
      history: {
        fields: [
          "conference",
          "owner.id",
          "owner.displayName|null",
          "durationSec|null",
          "participantCount",
          "participants[]",
          "participantsTruncated",
          "recordings.total",
          "recordings.ready",
          "recordings.processing",
          "recordings.failed",
          "chatAvailable",
          "chatReadOnly",
        ],
        limitations:
          "Only finished/cancelled meetings; participant list capped at 100. Owners/co-hosts can see broader historical membership than ordinary participants, whose view filters admission_state=admitted. Count is historical memberships, not peak simultaneous attendance.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/types.ts:365",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_product_repository.go:224",
        ],
      },
      analytics: {
        fields: [
          "enabled",
          "durationMs",
          "participantCount",
          "recordingAvailable",
          "transcriptAvailable",
          "timeline[].atMs",
          "timeline[].count",
          "participants[].participantId",
          "participants[].displayName",
          "participants[].participationMs",
          "participants[].speakingMs",
          "participants[].observedAudioMs",
          "participants[].screenMs",
          "participants[].messageCount",
        ],
        backendExtraFields: [
          "conferenceId",
          "approximateSpeaking",
          "updatedAt",
        ],
        limitations:
          "One meeting only; participant array capped at 500, timeline capped to about 600 points. Speaking is an approximate audio-level observation, not a transcript-derived ranking or productivity measure. Missing observation must not be called silence.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/types.ts:675",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/domain/analytics/analytics.go:29",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/analytics_repository.go:96",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/usecase/analytics/analytics.go:27",
        ],
      },
      admin: {
        fields: [
          "asOf",
          "activeConferences",
          "joinedParticipants",
          "activeRecordings",
          "queuedJobs",
          "failedJobs24h",
          "failedRecordings24h",
          "failedTranscriptions24h",
          "apiReady",
          "mediaWorkerReady",
          "dependencies.postgres",
          "dependencies.redis",
          "dependencies.rabbitmq",
          "dependencies.minio",
          "recentFailures[].kind",
          "recentFailures[].code",
          "recentFailures[].at",
        ],
        optionalBuildVersion:
          "From /capabilities; read-only public build identity.",
        limitations:
          "Read-only service operations summary. No users table, user-role editor, queue/job controls, resource restarts or feature settings API on this page.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/types.ts:710",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AdminPage.tsx:88",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/transport/http/platform_status_routes.go:17",
        ],
      },
    },
    permissions: [
      {
        id: "read-materials",
        rule: "Authenticated current conference member with admitted/legacy-compatible admission and status joined or left. Waiting/rejected/kicked members cannot read history, recording list/detail, transcript, summary or analytics.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/domain/conferences/product.go:16",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/domain/conferences/product.go:30",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_recording_repository.go:233",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/content_repository.go:28",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/analytics_repository.go:66",
        ],
      },
      {
        id: "read-history",
        rule: "read-materials plus conference status finished or cancelled.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_product_repository.go:246",
      },
      {
        id: "start-recording",
        rule: "Any admitted joined permanent account in active conference, when no unfinished recording exists. Guest accounts forbidden; not organizer-only.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_recording_repository.go:51",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_recording_repository.go:74",
        ],
      },
      {
        id: "stop-recording",
        rule: "Permanent account with historical read access AND conference owner with owner role, OR original recording requestedBy. Co-host role alone is insufficient. Current frontend only offers stop while that account is joined.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_recording_repository.go:132",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_recording_repository.go:145",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingPanel.tsx:171",
        ],
      },
      {
        id: "reprocess-content",
        rule: "Owner/co-host and server enabled/canRetry or enabled/canRegenerate. Backend enforces readiness, provider mode, maximum generations and cooldown. Do not assume permission from role alone.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/content_repository.go:41",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/usecase/content/service.go:54",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/usecase/content/service.go:74",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:429",
        ],
      },
      {
        id: "read-admin",
        rule: "Global persisted administrator privilege, not conference owner/co-host and not stale JWT. RequireAdmin queries current IsAdmin for every /admin request.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/internal/transport/http/middleware/admin.go:16",
      },
    ],
    capabilities: {
      endpoint: "GET /api/v1/capabilities",
      fields: [
        "liveCaptions",
        "transcription",
        "aiSummary",
        "semanticSearch",
        "meetingAnalytics",
        "recordingModes",
      ],
      exactFlags: {
        liveCaptions: "StageEight.LiveEnabled / LIVE_STT_ENABLED",
        transcription: "StageSeven.STTEnabled / TRANSCRIPTION_ENABLED",
        aiSummary: "StageSeven.AIEnabled / AI_SUMMARY_ENABLED",
        meetingAnalytics:
          "StageEight.AnalyticsEnabled / MEETING_ANALYTICS_ENABLED",
        semanticSearch: "EmbeddingsEnabled and available pgvector",
        recordingModes:
          "Backend returns composite/audio_only/individual_tracks/screen_focus",
      },
      designPolicy:
        "Hide conditional material sections/nav until current successful flag confirmation. Render disabled/unavailable/error states, not fabricated working AI/analytics. Per-content endpoint enabled/canRetry/canRegenerate/providerMode remains authoritative beyond global flags. Mark mock material as demo.",
      runtimeFlags:
        "Not inspected; feature-enabled mockups must be explicitly conditional scenarios.",
      references: [
        "/Users/aleksandranickij/htdocs/go-recorder/internal/usecase/platform/service.go:41",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/useCapabilities.ts:5",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:47",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AnalyticsPage.tsx:24",
      ],
    },
    retentionAndLinks: {
      availability:
        "Terminal ready/partial_ready/failed/cancelled recordings are hidden after 7 days from COALESCE(ended_at,stopped_at,updated_at,created_at), and deleted_at is always excluded.",
      expiryAppliesTo: [
        "conference recording list",
        "conference recording detail",
        "history recording counts",
      ],
      availabilityReferences: [
        "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/record_retention.go:12",
        "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_recording_repository.go:194",
        "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_product_repository.go:275",
      ],
      signedFileLinks:
        "Backend produces private presigned object GET links valid for 24 hours. UI does not have an explicit expiry field. A signed file URL is not the same as a permission-protected page URL; copy action currently copies the page.",
      linkReferences: [
        "/Users/aleksandranickij/htdocs/go-recorder/internal/usecase/recorder/service.go:802",
        "/Users/aleksandranickij/htdocs/go-recorder/internal/usecase/recorder/service.go:882",
        "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingActions.tsx:43",
      ],
      designConstraint:
        "Do not claim permanent storage, 30/90-day archive retention, public sharing, or show a precise expiry countdown unsupported by DTO.",
    },
    screens: [
      {
        id: "recordings-selected-meeting",
        name: "Записи",
        routes: [
          "/recordings?conference=:id",
          "/app/recordings?conference=:id",
        ],
        ia: "Primary workspace section; content is recordings for one selected meeting, not an all-recordings feed.",
        dataFlow: [
          "GET /api/v1/me/conferences?view=past|active&limit=20&cursor=...",
          "GET /api/v1/conferences/:id",
          "GET /api/v1/conferences/:id/recordings?limit=20&offset=...",
          "Optional GET /api/v1/conferences/:id/history only when finished/cancelled",
        ],
        actualControls: [
          "past/active meeting list switch",
          "title search limited to loaded meetings",
          "meeting selector, auto-first selection if no conference query",
          "load more meetings via cursor",
          "period all/7/30/90 days applied locally to loaded recording start/created dates",
          "sort newest/oldest applied locally to loaded recordings",
          "load more records using limit/offset",
          "watch via thumbnail/title/button",
          "download real main file",
          "copy protected recording page URL",
          "secondary meeting history link",
        ],
        recordsShown:
          "Dedupe uuid and only status=ready plus safe final_mp4/final_audio URL. Never show raw technical status badges.",
        metadata: [
          "meeting title",
          "recording date",
          "duration overlay if known",
          "file size if known",
          "organizer and historical participant count only if optional history available",
        ],
        statesExisting: [
          "loading meeting list",
          "empty past/active list with meetings CTA",
          "no local title matches",
          "meeting inaccessible with retry",
          "records loading",
          "records failed with retry",
          "no records in loaded pages/period",
          "next-page busy",
          "missing/broken preview icon fallback",
          "copy success and manual clipboard fallback",
        ],
        stateGaps: [
          "No explicit successful-state refresh of recordings, although selected active meeting can gain a completed record; current recordings query has no polling interval.",
          "Optional history failure silently removes organizer/participant metadata.",
          "No distinct expired-retention explanation; empty ready-only list can represent processing-only or missing-file records.",
        ],
        actions: [
          "select-meeting",
          "search-loaded-meetings",
          "filter-loaded-records",
          "sort-loaded-records",
          "more-meetings",
          "more-records",
          "open-player",
          "download-file",
          "copy-private-page-link",
          "open-history",
          "retry-meetings",
          "retry-records",
        ],
        currentResponsive:
          "Wide rows with 144px preview. <=1100 hides file size; <=600 keeps tiny 104px preview row, hides Watch and organizer and reduces metadata to 10px.",
        proposedResponsive: {
          desktop1440:
            "Meeting selector/search in 2-column control area; real recording rows/cards with preview, date, optional duration/size and Watch + action menu.",
          tablet834:
            "Compact controls above full-width rows; two controls per row where they fit, otherwise stacked.",
          mobile390:
            "Single-column full-width 16:9 preview cards, legible metadata, visible Watch + 44px action menu; selected-meeting context remains explicit.",
        },
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:107",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:141",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:174",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:303",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:542",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:597",
        ],
      },
      {
        id: "recording-player",
        name: "Просмотр записи",
        routes: ["/recordings/:recordingId?conference=:conferenceId"],
        ia: "Child of selected-meeting Recordings; Back preserves conference query.",
        dataFlow: [
          "GET conference",
          "GET conference-scoped recording after conference check",
          "Optional GET history after ready record identity check and closed meeting",
        ],
        actualControls: [
          "Back to selected-meeting recordings",
          "native video/audio controls",
          "main-file download when ready and real URL exists",
          "material downloads for final_mp4/final_audio/preview_jpg/tracks_archive",
          "manual availability retry",
          "manual signed link refresh after playback error",
        ],
        metadata: [
          "meeting title",
          "recording date",
          "duration",
          "main file size",
          "optional organizer",
          "optional historical participant count",
          "actual material type and size",
        ],
        statesExisting: [
          "missing conference/id context",
          "signed-out fallback (Protected usually redirects first)",
          "loading",
          "access/identity/API failure",
          "record not ready or no safe main URL",
          "media playback error",
          "refresh busy",
          "no materials",
          "audio-only player",
          "preview missing",
        ],
        stateGaps: [
          "No separate retention-expired explanation.",
          "Optional history failure silent.",
          "No media buffering/loading skeleton specific to browser metadata.",
          "No subtitle track in this player; do not invent caption toggle using absent VTT.",
        ],
        actions: [
          "back-to-selected-recordings",
          "native-playback",
          "download-file",
          "refresh-signed-link",
        ],
        prohibitedUI: [
          "AI/transcript tabs",
          "favorite",
          "delete",
          "rename",
          "public sharing",
          "fake quality selector or generated chapters/bookmarks",
        ],
        proposedResponsive: {
          desktop1440:
            "Back + meeting title/date/duration, wide player, 290px information/material rail.",
          tablet834: "Player full width then 2-column info/material panels.",
          mobile390:
            "Back/title then edge-aligned player, information, actual materials; 44px downloads. Native controls remain keyboard accessible.",
        },
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:45",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:109",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:143",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:214",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:268",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:320",
        ],
      },
      {
        id: "history-list-secondary",
        name: "История встреч",
        routes: ["/history"],
        ia: "Secondary existing route; not a primary nav/bottom nav/More item unless user explicitly reverses removal.",
        dataFlow: ["GET /api/v1/me/conferences?view=past&limit=20&cursor=..."],
        actualControls: [
          "local title search on loaded past meetings",
          "open meeting history",
          "load more meetings",
          "link to /meetings?view=past",
        ],
        metadata: [
          "meeting title",
          "finishedAt or createdAt",
          "meeting duration calculated only when startedAt and finishedAt exist",
        ],
        statesExisting: [
          "loading",
          "query error notice",
          "empty past meetings",
          "no loaded search matches",
          "load-more busy",
        ],
        stateGaps: [
          "No explicit retry button for failed list query.",
          "Unconditional hint advertises transcript/AI/analytics even when disabled.",
        ],
        actions: [
          "search-loaded-meetings",
          "more-meetings",
          "open-history",
          "open-past-meetings",
        ],
        proposedResponsive: {
          desktop1440:
            "Meeting-history rows, not recording-preview catalog; concise date/duration and Open materials.",
          tablet834: "Readable row with title/date and contextual action.",
          mobile390:
            "Stacked meeting rows/cards, 44px whole-row navigation, title/date/duration; no fake participant counts or recording total without history requests.",
        },
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AccountPages.tsx:314",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AccountPages.tsx:351",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AccountPages.tsx:358",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/history-notifications.css:370",
        ],
      },
      {
        id: "history-detail-secondary",
        name: "Материалы завершённой встречи",
        routes: [
          "/history/:conferenceId?section=overview|recording|transcript|summary|analytics&recording=:recordingId&tab=transcript|summary&t=:milliseconds&segment=:segmentId",
        ],
        ia: "Secondary meeting-material route; not standalone recording player. Overview and recording are unconditional; other sections capability-gated.",
        dataFlow: [
          "GET conference history",
          "GET current membership",
          "GET capabilities",
          "Lazy RecordingPanel only for recording/transcript/summary",
          "Lazy AnalyticsPanel only for analytics",
        ],
        actualControls: [
          "Back to history",
          "read-only conference chat link when chatAvailable",
          "overview/recording/conditional transcript/summary/analytics material navigation",
          "recording selector",
          "legacy native audio/video material player",
          "download main file and track archive",
          "conditional transcript create/retry, summary create/regenerate",
          "timestamp seek and deep-link query",
          "load more transcript segments",
        ],
        metadata: [
          "closed meeting title/date/duration",
          "historical participants/count/truncation",
          "organizer",
          "ready/processing recording counts",
          "real transcript segments and summary structures only if present",
        ],
        statesExisting: [
          "loading history/membership",
          "error/no access",
          "capability error",
          "requested disabled section falls back to overview",
          "truncated participants",
          "no records",
          "not-ready materials",
          "nullable content",
          "queued/processing/failed/ready content",
          "mock provider disclosure",
          "playback error with signed-link refresh",
          "empty transcript speech/action items",
          "mutation busy/failure",
        ],
        stateGaps: [
          "Current RecordingPanel embeds RecordingInsights tabs in all recording/transcript/summary sections, creating duplicate nested navigation and showing transcript controls within the top Recording section.",
          "RecordingPanel fetches only first default 20 records without load-more.",
          "Closed-meeting recording completion has no periodic list refresh.",
          "History failure lacks retry action; only Back exists.",
        ],
        actions: [
          "open-history-section",
          "open-readonly-conference-chat",
          "select-history-recording",
          "download-file",
          "native-playback",
          "refresh-signed-link",
          "retry-transcript",
          "regenerate-summary",
          "seek-transcript-timestamp",
          "more-transcript-segments",
        ],
        proposedResponsive: {
          desktop1440:
            "Meeting overview + one material navigation level, independently conditional content sections. Keep simplified standalone recording player separate.",
          tablet834:
            "Scrollable accessible material nav or select; full-width content.",
          mobile390:
            "Meeting header + overview/material list, full-width chosen material; avoid nested simultaneous tab bars.",
        },
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:24",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:47",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:74",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:124",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:231",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:35",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:113",
        ],
      },
      {
        id: "analytics-conditional",
        name: "Аналитика одной встречи",
        routes: [
          "/analytics?view=past|active&conference=:conferenceId",
          "/history/:id?section=analytics",
        ],
        ia: "Capability-gated existing screen, not team-wide/global dashboard.",
        dataFlow: [
          "GET capabilities",
          "GET user conference page past/active",
          "GET /api/v1/conferences/:id/analytics; 30s polling for selected active meeting",
        ],
        actualControls: [
          "past/active selector",
          "meeting selector",
          "load more meetings",
          "show chart values via details/summary",
        ],
        metrics: [
          "duration",
          "participant count",
          "message total computed from returned participant rows",
          "participants-over-time",
          "per-person presence/approximate speaking/observed audio/screen time/message count",
          "recording/transcript availability",
        ],
        statesExisting: [
          "capability pending/error/disabled",
          "conference list loading/error/empty",
          "analytics query loading/error",
          "aggregation not ready/disabled",
          "no timeline",
          "no participant metrics",
        ],
        stateGaps: [
          "Error notices have no explicit Retry control for analytics data/list/capabilities.",
          "Participant table stays minimum 690px horizontal scroll on mobile; not a card layout.",
          "Message KPI sums up to 500 returned participant rows; do not claim unrestricted full-meeting total for larger meetings.",
        ],
        actions: [
          "select-analytics-meeting",
          "select-analytics-view",
          "more-meetings",
          "show-chart-values",
        ],
        prohibitedUI: [
          "productivity score",
          "speaker ranking",
          "engagement/attendance score",
          "global/team trends",
          "date-range aggregate reports",
          "export/report action absent in page/API",
        ],
        proposedResponsive: {
          desktop1440:
            "Three factual metrics, participant timeline + materials/accuracy information, alphabetical participant table.",
          tablet834:
            "Stack chart and context; compact grid metrics; table in marked scroll region if retained.",
          mobile390:
            "Stack metrics and chart with text values; alphabetical participant metric cards using same data. Approximation/observation disclosure stays visible.",
        },
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AnalyticsPage.tsx:13",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AnalyticsPage.tsx:50",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/AnalyticsPanel.tsx:20",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/AnalyticsPanel.tsx:67",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/AnalyticsPanel.tsx:154",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/analytics.css:105",
        ],
      },
      {
        id: "admin-read-only",
        name: "Состояние сервиса",
        routes: ["/admin"],
        ia: "Administrator-only service section. This is operations read-only, not product account management.",
        dataFlow: [
          "GET /api/v1/admin/summary; 30s polling for local isAdmin",
          "GET /api/v1/capabilities for optional buildVersion",
        ],
        actualControls: [
          "manual summary refresh",
          "retry after summary query error",
        ],
        statesExisting: [
          "local no admin",
          "403 server revocation",
          "loading",
          "error with retry",
          "healthy/dependency unavailable",
          "no recent failures",
          "refresh busy",
          "snapshot timestamp",
        ],
        actions: ["refresh-admin-summary"],
        prohibitedUI: [
          "user management",
          "role assignment",
          "feature flag switches",
          "worker/API restarts",
          "retry/delete queue jobs",
          "log download absent in API",
        ],
        proposedResponsive: {
          desktop1440:
            "Operational counters, availability list and recent failures table with neutral labels.",
          tablet834: "Two-column counters, compact availability groups.",
          mobile390:
            "Single-column counters and availability; convert recent failures to small cards, retain time/subsystem/code without raw stacks.",
        },
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AdminPage.tsx:18",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AdminPage.tsx:88",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AdminPage.tsx:127",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/admin-page.css:117",
        ],
      },
    ],
    actionMapping: [
      {
        id: "select-meeting",
        effect:
          "Update conference query; reset period/sort; load only selected meeting.",
        api: "GET conference + conference recording page",
        permission: "read-materials",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:194",
      },
      {
        id: "search-loaded-meetings",
        effect: "Local title substring filter, not API global search.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:124",
      },
      {
        id: "filter-loaded-records",
        effect:
          "Local recording startedAt/createdAt cutoff; only loaded pages.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:179",
      },
      {
        id: "sort-loaded-records",
        effect: "Local date ascending/descending; not backend sort switch.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:187",
      },
      {
        id: "more-meetings",
        effect: "GET next cursor page, limit 20.",
        api: "GET /api/v1/me/conferences",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/queries.ts:15",
      },
      {
        id: "more-records",
        effect:
          "GET next 20 via offset, until response fewer than 20; no total returned.",
        api: "GET /api/v1/conferences/:id/recordings?limit=20&offset=:offset",
        permission: "read-materials",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:151",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/app/recordings/handler.go:175",
        ],
      },
      {
        id: "open-player",
        effect:
          "Navigate to recordingDetailPath with BOTH recording UUID and conference query.",
        api: "GET /api/v1/conferences/:id/recordings/:recordingId",
        permission: "read-materials",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/recordingPresentation.ts:116",
      },
      {
        id: "download-file",
        effect:
          "Navigate/download exact backend-provided safe signed URL. Same read authorization produced URL; does not grant meeting access.",
        api: "Private presigned object GET, no dedicated download mutation",
        permission: "read-materials",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingActions.tsx:123",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:380",
        ],
      },
      {
        id: "copy-private-page-link",
        effect:
          "Clipboard protected page URL; fallback input on clipboard failure, success status. No public grant action.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingActions.tsx:43",
      },
      {
        id: "open-history",
        effect:
          "Navigate secondary meeting materials; completed/cancelled context only.",
        api: "GET /api/v1/conferences/:id/history",
        permission: "read-history",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:428",
      },
      {
        id: "retry-meetings",
        effect: "Refetch existing meeting query, no data mutation.",
        api: "GET /api/v1/me/conferences",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:291",
      },
      {
        id: "retry-records",
        effect: "Refetch existing recording query.",
        api: "GET /api/v1/conferences/:id/recordings",
        permission: "read-materials",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:369",
      },
      {
        id: "back-to-selected-recordings",
        effect: "Navigate /recordings?conference=:conferenceId.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:141",
      },
      {
        id: "native-playback",
        effect:
          "Browser-native audio/video controls; no autoplay, preload metadata; inline video on standalone player.",
        api: "Private media GET from actual signed URL",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:273",
      },
      {
        id: "refresh-signed-link",
        effect:
          "Manual access recheck and real recording refetch; replaces media revision after valid identities. No automatic retry loop.",
        api: "GET conference then GET conference-scoped recording",
        permission: "read-materials",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:143",
      },
      {
        id: "open-past-meetings",
        effect: "Navigate /meetings?view=past.",
        api: "GET /api/v1/me/conferences?view=past",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AccountPages.tsx:411",
      },
      {
        id: "open-history-section",
        effect:
          "Set section query; preserve selected recording/timestamp; invalid/disabled section falls back overview.",
        api: "Lazy per-material GET only",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:63",
      },
      {
        id: "open-readonly-conference-chat",
        effect:
          "Navigate /conferences/:id; history response chatReadOnly=true; no restarting/joining closed media implied.",
        api: "Existing conference/chat GET flow",
        permission: "read-history",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:124",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_product_repository.go:254",
        ],
      },
      {
        id: "select-history-recording",
        effect:
          "Set recording query, drop seek segment/time; ready loaded recording selection.",
        api: "GET recording",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:44",
      },
      {
        id: "retry-transcript",
        effect:
          "Queue actual permitted processing; server response 202, enabled+canRetry authoritative; owner/co-host only.",
        api: "POST /api/v1/conferences/:id/recordings/:recordingId/transcript/retry",
        permission: "reprocess-content",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:429",
      },
      {
        id: "regenerate-summary",
        effect:
          "Queue actual permitted current-generation summary; enabled+canRegenerate authoritative.",
        api: "POST /api/v1/conferences/:id/recordings/:recordingId/summary/regenerate",
        permission: "reprocess-content",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:533",
      },
      {
        id: "seek-transcript-timestamp",
        effect:
          "Seek actual media at segment.startMs; store t/segment query, no autoplay.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:212",
      },
      {
        id: "more-transcript-segments",
        effect:
          "GET next actual page; limit100 offset; page total comes from transcript API.",
        api: "GET /api/v1/conferences/:id/recordings/:recordingId/transcript/segments",
        permission: "read-materials",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:151",
      },
      {
        id: "select-analytics-meeting",
        effect: "Set selected conference query; show only its aggregate.",
        api: "GET /api/v1/conferences/:id/analytics",
        permission: "read-materials",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AnalyticsPage.tsx:76",
      },
      {
        id: "select-analytics-view",
        effect: "Set view past/active, clear old conference query.",
        api: "GET /api/v1/me/conferences?view=past|active",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AnalyticsPage.tsx:61",
      },
      {
        id: "show-chart-values",
        effect: "Accessible textual timeline values via details/summary.",
        api: null,
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/AnalyticsPanel.tsx:137",
      },
      {
        id: "refresh-admin-summary",
        effect:
          "Refetch read-only summary; local admin gate and server recheck.",
        api: "GET /api/v1/admin/summary",
        permission: "read-admin",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AdminPage.tsx:54",
      },
    ],
    designFindings: [
      {
        id: "retention-uncommunicated",
        priority: "P0",
        finding:
          "Current UI does not explain 7-day recording availability; 30/90-day local date filters could imply retention that does not exist.",
        decision:
          "Use neutral supported retention explanation; no exact expiry timer without DTO. Do not draw permanent library.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/record_retention.go:14",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:347",
        ],
      },
      {
        id: "loaded-only-search-sort",
        priority: "P0",
        finding:
          "Search/filter/sort scope is loaded data, not server-wide content. Record list has no total.",
        decision:
          "Preserve explicit selected-meeting context, 'по загруженным' wording and Load more. No global record count/catalog.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:285",
      },
      {
        id: "mobile-recording-card",
        priority: "P1",
        finding:
          "<=600 recording row hides Watch/organizer, metadata10px and preview104px; differs from full-width adaptive card requested.",
        decision:
          "Full-width preview card + explicit Watch and menu; unknown metadata omitted, not fabricated.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:597",
      },
      {
        id: "touch-targets",
        priority: "P1",
        finding:
          "Recording list action trigger is36px desktop/32px mobile; view switches min40px and Watch38px. Some desktop material downloads38px. Prompt requires44x44.",
        decision:
          "44x44 minimum hit areas across widths with visible focus; menu retains keyboard Escape/arrows/Home/End and focus return.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:39",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:235",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:246",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/recordings.css:630",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingActions.tsx:57",
        ],
      },
      {
        id: "history-nested-tabs",
        priority: "P1",
        finding:
          "Top History sections and RecordingInsights inner tabs duplicate section selection; top recording also gets insights.",
        decision:
          "One material navigation level in meeting history; standalone recording player remains no tabs.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:231",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:339",
        ],
      },
      {
        id: "history-first-page-recordings",
        priority: "P1",
        finding:
          "Legacy History RecordingPanel uses default20 without load-more; mature history list may omit later ready recordings.",
        decision:
          "Document gap; do not mock complete historical recording count/list unless current API pages are explicitly loaded in implementation.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingPanel.tsx:65",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/app/httpresponse/response.go:88",
        ],
      },
      {
        id: "partial-ready-contract",
        priority: "P1",
        finding:
          "Backend PublicStatus leaves partial_ready unchanged and history ready count includes it, but ConferenceRecording TS union omits it and playableRecordings accepts only ready.",
        decision:
          "Do not draw a ready Watch action for partial_ready; describe unavailable/non-playable materials neutrally. API/contract reconciliation is a separate implementation gap.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/domain/records/conference.go:64",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_product_repository.go:275",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/types.ts:178",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/recordingPresentation.ts:136",
        ],
      },
      {
        id: "missing-explicit-retries",
        priority: "P1",
        finding:
          "History list/detail and analytics show failure without an explicit retry; recording list has retries but no normal-state refresh.",
        decision:
          "Design consistent Retry and refresh states that re-use existing GET APIs, not new backend operations.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AccountPages.tsx:358",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/HistoryDetailPage.tsx:75",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/AnalyticsPanel.tsx:54",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingsPage.tsx:151",
        ],
      },
      {
        id: "capability-copy",
        priority: "P1",
        finding:
          "History list unconditionally mentions transcript/AI/analytics; actual capabilities can be off/unverified.",
        decision:
          "General 'доступные материалы' wording or confirmed capability-specific hint; never fake enabled results.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/AccountPages.tsx:351",
      },
      {
        id: "analytics-mobile-table",
        priority: "P1",
        finding:
          "Current analytics table min-width690px; horizontal scroll is supported but requested mobile design should not mechanically shrink desktop.",
        decision:
          "Alphabetical participant cards preserving all actual six metrics; text-accessible timeline; no leaderboard.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/analytics.css:105",
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/AnalyticsPanel.tsx:33",
        ],
      },
      {
        id: "no-subtitle-track",
        priority: "P2",
        finding:
          "Standalone recording video has no <track> source or caption file DTO; transcript text is a separate conditional history material.",
        decision:
          "Do not invent player CC or transcript tab. Preserve native keyboard playback controls and meaningful media labels.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:288",
      },
      {
        id: "history-count-not-attendance",
        priority: "P2",
        finding:
          "Historical member count is role-filtered and capped list, not peak attendance; cancelled meetings may have no duration.",
        decision:
          "Label 'участники встречи', indicate partial list, keep null duration dash; never invent attendance chart from this count.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/internal/infrastructure/postgres/conference_product_repository.go:255",
      },
      {
        id: "source-actions-in-summary",
        priority: "P2",
        finding:
          "Summary task source links only appear when matching transcript segments have already been loaded; direct summary tab does not preload transcript segments.",
        decision:
          "Do not draw every task with a working source chip unconditionally; show only actual loaded matches.",
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingInsights.tsx:568",
      },
      {
        id: "legacy-retired-api",
        priority: "P0",
        finding:
          "Old /records and debug recording APIs explicitly return410; no conference recording DELETE/favorite/catalog endpoint is registered.",
        decision:
          "Exclude deletion/favorites/global catalog; do not resurrect legacy API.",
        references: [
          "/Users/aleksandranickij/htdocs/go-recorder/internal/transport/http/retired_recording_routes.go:17",
          "/Users/aleksandranickij/htdocs/go-recorder/internal/transport/http/conference_recording_routes.go:14",
        ],
      },
    ],
    reusableComponents: [
      {
        name: "SelectedMeetingControl",
        usedBy: ["recordings-selected-meeting", "analytics-conditional"],
        states: [
          "loading",
          "empty",
          "selected",
          "not-in-loaded-page",
          "disabled",
          "error",
          "next-page-busy",
        ],
      },
      {
        name: "RecordingCard",
        usedBy: ["recordings-selected-meeting"],
        states: [
          "real-preview",
          "preview-unavailable",
          "known/unknown-duration",
          "known/unknown-size",
          "copy-success",
          "clipboard-fallback",
          "menu-open",
        ],
      },
      {
        name: "PrivateRecordingActions",
        usedBy: ["recordings-selected-meeting"],
        states: [
          "default",
          "keyboard-focused",
          "menu-open",
          "download-available",
          "copy-success",
          "manual-copy",
        ],
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/components/RecordingActions.tsx:11",
      },
      {
        name: "SimpleMediaPlayer",
        usedBy: ["recording-player"],
        states: [
          "video",
          "audio",
          "metadata-loading",
          "unavailable",
          "playback-error",
          "refreshing",
        ],
        reference:
          "/Users/aleksandranickij/htdocs/go-recorder/frontend/src/pages/RecordingDetailPage.tsx:268",
      },
      {
        name: "RecordingInformation",
        usedBy: ["recording-player"],
        states: ["known-metadata", "optional-history-missing", "no-materials"],
      },
      {
        name: "MeetingMaterialNavigation",
        usedBy: ["history-detail-secondary"],
        states: [
          "overview",
          "recording",
          "conditional-transcript",
          "conditional-summary",
          "conditional-analytics",
          "requested-disabled",
        ],
      },
      {
        name: "CapabilityGate",
        usedBy: ["history-detail-secondary", "analytics-conditional"],
        states: ["pending", "enabled", "disabled", "unverified/error"],
      },
      {
        name: "FactualMetricsGrid",
        usedBy: ["analytics-conditional", "admin-read-only"],
        states: ["loading", "known-data", "unavailable"],
        restriction:
          "Do not interchange user meeting metrics with global operations metrics.",
      },
      {
        name: "ParticipantMetricsCard",
        usedBy: ["analytics-conditional"],
        states: ["populated", "no-observed-audio", "empty"],
        restriction: "Alphabetical, never ranked.",
      },
      {
        name: "ReadOnlyServiceHealth",
        usedBy: ["admin-read-only"],
        states: ["ready", "unavailable", "refreshing", "error"],
      },
      {
        name: "Skeleton/EmptyState/ErrorState",
        usedBy: ["all"],
        states: [
          "first-load",
          "empty-source",
          "empty-filter",
          "forbidden",
          "network/API-error",
          "unavailable-media",
          "retry-busy",
        ],
      },
    ],
    implementationPriorityIfUserLaterAuthorizes: [
      "P0: Lock selected-meeting IA, secondary History and unsupported-action exclusions; truthful retention/capability/data-scope copy.",
      "P1: Mobile full-width recording cards, 44px targets, one-level History material navigation, consistent retry/refresh.",
      "P1: Analytics mobile participant cards and conditional unavailable/permission screens.",
      "P2: Optional metadata disclosure and advanced state consistency; no backend implementation automatically.",
    ],
    validation: {
      performed: [
        "Route inspection",
        "API/DTO inspection",
        "permission inspection",
        "capability conditions",
        "actual material/link/retention behavior",
        "loading/error/empty states",
        "CSS responsive and touch target source audit",
      ],
      notPerformed: [
        "Live server capability read",
        "runtime screenshot/browser accessibility check",
        "mockup rendering",
        "frontend/backend changes",
        "Git index change",
        "remote deployment",
      ],
      designViewportTargets: [
        { kind: "desktop", width: 1440 },
        { kind: "tablet", width: 834 },
        { kind: "mobile", width: 390 },
      ],
      finalRule:
        "Every displayed field/action must trace to this current source/API mapping; optional and capability-conditioned content stays conditional.",
    },
  },
};
