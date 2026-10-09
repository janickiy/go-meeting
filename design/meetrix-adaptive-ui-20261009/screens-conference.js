/* Макеты встречи показывают уже существующие WebRTC, чат, запись и модерацию. */
(() => {
  const D = window.MeetrixDesign;
  const base = {
    group: "Встреча",
    nav: "meetings",
    layout: "bare",
    route: "/conferences/:id",
    api: [
      "GET /api/v1/conferences/:id",
      "GET /api/v1/conferences/:id/participants/me",
      "POST /api/v1/conferences/:id/ws-ticket",
      "WebSocket: presence, media, chat, recording events",
    ],
    permissions:
      "Допущенный участник активной встречи; доступные команды зависят от роли и состояния подключения.",
    sources: [
      "frontend/src/components/conference/ActiveConferenceRoom.tsx:49",
      "frontend/src/components/conference/ConferenceRoomSidebar.tsx:34",
      "frontend/src/components/RealtimePanel.tsx:322",
      "frontend/src/components/ParticipantsPanel.tsx:92",
    ],
    responsive: {
      desktop:
        "Тёмная сцена на всю высоту; две плитки 16:9 одна над другой по центру. Панель чата/участников 360px. Контролы вне прокрутки видео.",
      tablet:
        "Панель шириной 360px поверх сцены; после закрытия фокус возвращается к кнопке. Видео не сжимается в узкие колонки.",
      mobile:
        "Две широкие плитки вертикально, управление внизу. Чат/участники — отдельный полноэкранный слой с кнопкой возврата.",
    },
    notes: [
      "Офлайн-участники исчезают из плиток и списка через пять секунд потери присутствия; отдельная вкладка того же участника сохраняет онлайн.",
      "Рамка и заполнение фона микрофона получают реальный уровень звука из useSpeakingParticipants; таймерная декоративная пульсация не используется.",
      "При демонстрации экран заменяет все видеоплитки; чат остаётся доступен.",
      "Клавиши M/V/C действуют вне полей ввода и диалогов; после переподключения камера и микрофон не включаются автоматически.",
      "Все имена и сообщения в макетах вымышлены.",
    ],
  };
  const states = [
    { id: "default", label: "Подключено" },
    { id: "speaking-soft", label: "Тихая речь" },
    { id: "speaking-loud", label: "Громкая речь" },
    { id: "reconnect", label: "Восстановление связи" },
    { id: "error", label: "Медиасвязь недоступна" },
    { id: "empty", label: "Вы одни во встрече" },
    { id: "recording", label: "Идёт запись" },
    { id: "loading", label: "Подключение" },
  ];
  const control = (name, icon, attrs = "", active = false, danger = false) =>
    `<button class="cf-control ${active ? "active" : ""} ${danger ? "danger" : ""}" aria-label="${name}" title="${name}" ${attrs}>${D.icon(icon, 22)}<span>${name}</span></button>`;
  function tile(name, local, level = 0) {
    return `<div class="cf-tile ${level ? "speaking" : ""}" style="--volume:${level};--volume-percent:${level * 100}%" aria-label="${name}${level ? ", говорит" : ""}"><div class="cf-person">${D.avatar(name, local ? "slate" : "purple", "large")}<span>Камера выключена</span></div><div class="cf-name"><span>${name}${local ? " · вы" : ""}</span>${local ? D.icon("ShieldCheck", 17) : ""}<span class="cf-mic ${level ? "level" : ""}" aria-label="${level ? "Говорит" : "Микрофон выключен"}">${D.icon(level ? "Mic" : "MicOff", 18)}</span></div></div>`;
  }
  function chat() {
    return `<div class="cf-chat"><div class="cf-day">Сегодня</div><div class="cf-message"><div class="cf-sender">${D.avatar("Илья Волков", "purple", "small")}<span>Илья Волков</span></div><div class="cf-bubble">Предлагаю начать с обновлений по проекту.<time>10:04</time></div></div><div class="cf-message own"><div class="cf-bubble"><div class="cf-quote">Илья Волков<br><span>Начать с обновлений по проекту</span></div>Да, давайте. Подготовила заметки 👌<time>10:05</time></div></div><div class="cf-message"><div class="cf-sender">${D.avatar("Илья Волков", "purple", "small")}<span>Илья Волков</span></div><div class="cf-bubble cf-file">${D.icon("FileText", 22)}<span><strong>Заметки.pdf</strong><small>240 КБ</small></span><button class="icon-button" data-toast="В макете показано действие скачивания" aria-label="Скачать Заметки.pdf">${D.icon("Download", 18)}</button></div></div></div><div class="cf-composer"><button class="icon-button" aria-label="Прикрепить файл" data-toast="До 5 файлов, каждый до 10 МиБ">${D.icon("Paperclip")}</button><textarea aria-label="Сообщение в чат встречи" placeholder="Сообщение…" data-composer rows="1"></textarea><button class="icon-button" data-menu="cf-emoji" aria-expanded="false" aria-label="Выбрать смайлик">${D.icon("Smile")}</button><button class="cf-send" data-toast="Сообщение показано в макете; в рабочую встречу ничего не отправлено" aria-label="Отправить сообщение">${D.icon("Send", 20)}</button><div class="cf-emoji dropdown" id="cf-emoji" hidden>${"😀 😃 😄 😁 😆 😅 😂 🙂 🙃 😉 😊 😇 🥰 😍 🤩 😘 😋 😛 😎 🤓 🧐 🤔 🤗 🤭 🤫 😐 😴 😮 😲 😢 😭 😤 😡 😳 🥳 😌 😏 😬 🤝 👍 👎 👏 🙌 🙏 💪 👌 ✌️ 🤞 ❤️ 💙 💚 💛 💜 🔥 ✨ 🎉 🎊 ✅ 🚀 💡 ⭐ ☀️ 🌈 ☕"
      .split(" ")
      .map(
        (x) =>
          `<button data-emoji="${x}" aria-label="Добавить ${x}">${x}</button>`,
      )
      .join("")}</div></div>`;
  }
  function participants(moderator = true, alone = false) {
    if (alone)
      return `<div class="cf-participants"><p class="eyebrow">В сети · 1</p><div class="list-row">${D.avatar("Анна Морозова")}<div class="list-main"><strong>Анна Морозова · вы</strong><p>${moderator ? "Организатор" : "Участник"}</p></div></div><p class="muted small">Вы пока одни во встрече.</p>${D.button("Пригласить участников", "UserPlus", 'data-go="conference-invite"', "secondary")}</div>`;
    return `<div class="cf-participants"><label class="search">${D.icon("Search", 18)}<input placeholder="Найти участника" aria-label="Найти участника"></label><p class="eyebrow">В сети · 2</p><div class="list-row">${D.avatar("Анна Морозова")}<div class="list-main"><strong>Анна Морозова · вы</strong><p>Организатор</p></div>${D.icon("Mic", 18)}</div><div class="list-row">${D.avatar("Илья Волков", "purple")}<div class="list-main"><strong>Илья Волков</strong><p>Участник</p></div>${D.icon("MicOff", 18)}${
      moderator
        ? `<div class="menu-anchor"><button class="icon-button" aria-label="Управление Ильёй Волковым" data-menu="cf-person-menu">${D.icon("EllipsisVertical", 18)}</button><div id="cf-person-menu" class="dropdown" hidden>${[
            ["MicOff", "Отключить микрофон"],
            ["VideoOff", "Отключить камеру"],
            ["MonitorOff", "Отключить экран"],
            ["ShieldCheck", "Назначить соорганизатором"],
            ["LogOut", "Исключить участника"],
          ]
            .map(
              ([i, t]) =>
                `<button data-toast="Показано действие: ${t.toLowerCase()}">${D.icon(i, 18)}${t}</button>`,
            )
            .join("")}</div></div>`
        : ""
    }</div>${moderator ? `<div class="cf-waiting-card"><div class="spread"><h3>Ожидает допуска</h3>${D.badge("1", "warning")}</div><div class="list-row">${D.avatar("Олег Соколов", "teal")}<div class="list-main"><strong>Олег Соколов</strong></div></div><div class="row">${D.button("Допустить", "Check", 'data-toast="Показано состояние допуска участника"', "primary")}${D.button("Отклонить", "", 'data-toast="Показано состояние отклонения запроса"', "secondary")}</div></div>` : ""}${D.button("Пригласить участников", "UserPlus", 'data-go="conference-invite"', "secondary")}${moderator ? D.button("Завершить встречу", "Square", 'data-go="finish-meeting"', "ghost") : ""}</div>`;
  }
  function screenContent() {
    return `<div class="cf-shared"><div class="cf-shared-document"><div class="cf-document-brand">${D.icon("FileText", 24)} Проект «Горизонт»</div><span class="eyebrow">Рабочая встреча команды</span><h2>План на следующую неделю</h2><div class="cf-document-line"></div><p>Обсудить обратную связь</p><p>Согласовать приоритеты</p><p>Определить следующие шаги</p><div class="cf-doc-tags"><span>Продукт</span><span>Дизайн</span><span>Разработка</span></div></div><div class="cf-name"><span>Экран · Илья Волков</span>${D.icon("Monitor", 18)}</div></div>`;
  }
  function room({ state, role }, panel = "", sharing = false) {
    const recording = state === "recording";
    const level =
      state === "speaking-loud"
        ? 0.9
        : state === "speaking-soft"
          ? 0.3
          : state === "default"
            ? 0.55
            : 0;
    return `<main class="cf-room ${panel ? "has-panel" : ""}" aria-label="Встреча команды"><header class="cf-header"><button class="cf-header-icon" aria-label="К встречам" data-go="meetings">${D.icon("ArrowLeft")}</button><div class="cf-title"><h1>Обсуждение проекта</h1><span>${state === "loading" ? "Подключаемся…" : "Встреча в эфире"}</span></div><span class="cf-clock">24:18</span>${recording ? `<span class="cf-rec">${D.icon("Circle", 9)} Запись</span>${role === "owner" ? D.button("Остановить", "Square", 'data-state="default"', "danger") : ""}` : ""}${recording ? "" : D.button("Запись", "Circle", 'data-go="recording-control"', "secondary")}${D.button("Пригласить", "Link", 'data-go="conference-invite"', "secondary")}</header>${recording ? `<div class="cf-status recording" role="status">${D.icon("Circle", 12)} Идёт запись встречи. Все участники уведомлены.</div>` : ""}${state === "reconnect" ? `<div class="cf-status" role="status">${D.icon("WifiOff", 18)} Восстанавливаем соединение. Ваши устройства выключены.</div>` : state === "error" ? `<div class="cf-status error" role="alert">${D.icon("CircleAlert", 18)} Не удалось подключить звук и видео. ${D.button("Повторить", "RefreshCw", 'data-state="loading"', "secondary")}</div>` : ""}<div class="cf-content"><section class="cf-stage" aria-label="Видео участников"><div class="cf-tiles ${sharing ? "sharing" : ""} ${state === "empty" ? "single" : ""}">${state === "loading" ? `<div class="cf-connecting">${D.icon("LoaderCircle", 32)}<h2>Подключаемся к встрече</h2><p>Настраиваем звук и видео</p></div>` : sharing ? screenContent() : `${tile("Анна Морозова", true, level)}${state === "empty" ? "" : tile("Илья Волков", false)}`}</div>${state === "empty" ? '<p class="cf-alone">Вы первые во встрече. Пригласите коллег по ссылке.</p>' : ""}</section>${panel ? `<aside class="cf-aside" aria-label="${panel === "chat" ? "Чат встречи" : "Участники встречи"}"><header><h2>${panel === "chat" ? "Чат встречи" : "Участники"}</h2><button class="icon-button" aria-label="Закрыть панель" data-go="conference">${D.icon("X")}</button></header><div class="cf-panel-tabs"><button class="${panel === "chat" ? "active" : ""}" data-go="conference-chat">Чат</button><button class="${panel === "participants" ? "active" : ""}" data-go="conference-participants">Участники · ${state === "empty" ? 1 : 2}</button></div>${panel === "chat" ? chat() : participants(role === "owner", state === "empty")}</aside>` : ""}</div><footer class="cf-footer"><div class="cf-controls">${control("Микрофон", "Mic", 'data-toggle aria-pressed="true"', true)}${control("Камера", "VideoOff", 'data-toggle aria-pressed="false"')}${control(sharing ? "Остановить экран" : "Экран", sharing ? "ScreenShareOff" : "MonitorUp", `data-go="${sharing ? "conference" : "conference-screen"}"`, sharing)}${control("Устройства", "Settings2", 'data-go="conference-devices"')}${control("Участники", "Users", 'data-go="conference-participants"', panel === "participants")}${control("Чат", "MessageCircle", 'data-go="conference-chat"', panel === "chat")}<div class="menu-anchor">${control("Ещё", "Ellipsis", 'data-menu="cf-more" aria-expanded="false"')}<div class="dropdown cf-more" id="cf-more" hidden><button data-go="conference-screen">${D.icon("MonitorUp", 18)}Экран</button><button data-go="conference-participants">${D.icon("Users", 18)}Участники</button><button data-go="conference-invite">${D.icon("Link", 18)}Пригласить</button><button data-go="recording-control">${D.icon("Circle", 18)}Запись</button><button data-go="conference-devices">${D.icon("Settings2", 18)}Устройства</button><button data-state="reconnect">${D.icon("RefreshCw", 18)}Переподключиться</button><button data-toast="Реакция 👍 показана в макете">👍 Реакция</button></div></div>${control("Выйти", "LogOut", 'data-go="leave-meeting"', false, true)}</div><div class="cf-footer-meta"><span>${D.icon("ShieldCheck", 14)} Доступ по приглашению</span><div><button data-toast="Реакция 👍 показана в макете" aria-label="Показать реакцию нравится">👍</button><button data-toast="Реакция 👏 показана в макете" aria-label="Показать аплодисменты">👏</button><button data-toast="Реакция ❤️ показана в макете" aria-label="Показать сердце">❤️</button><button data-toast="Реакция 😂 показана в макете" aria-label="Показать смех">😂</button></div></div></footer></main>`;
  }
  function modal(title, body, footer = "") {
    return `<div class="modal-backdrop" role="dialog" aria-modal="true" aria-label="${title}"><section class="modal-panel"><header class="modal-header"><h2>${title}</h2><button class="icon-button" data-go="conference" aria-label="Закрыть">${D.icon("X")}</button></header><div class="modal-body stack">${body}</div>${footer ? `<footer class="modal-footer">${footer}</footer>` : ""}</section></div>`;
  }
  D.register([
    {
      ...base,
      id: "captions",
      title: "Субтитры · при включённой функции",
      group: "Условные разделы",
      permissions:
        "Только при liveCaptions=true. Изменение распознавания — по canManage. Скрытие текста локальное.",
      api: [
        "GET /api/v1/conferences/:id/captions",
        "PUT /api/v1/conferences/:id/captions",
        "GET /api/v1/conferences/:id/captions/segments",
      ],
      sources: ["frontend/src/components/CaptionsPanel.tsx:160"],
      states: [
        { id: "default", label: "Распознавание включено" },
        { id: "off", label: "Выключено" },
        { id: "unavailable", label: "Отключено на сервере" },
        { id: "error", label: "Ошибка распознавания" },
      ],
      notes: [
        "Этот экран не добавляет функцию: компонент уже существует и скрыт при выключенной серверной возможности.",
        "Показывать текст — настройка этого устройства. Она не выключает передачу звука.",
        "Перед включением требуется явно сообщить о передаче звука сервису распознавания.",
      ],
      render: (ctx) =>
        room({ ...ctx, state: "default" }) +
        modal(
          "Субтитры",
          ctx.state === "unavailable"
            ? D.notice("Живое распознавание отключено администратором.", "info")
            : `${D.notice("Звук участников передаётся сервису распознавания. Скрытие текста на этом устройстве не отключает распознавание.", "info")}${ctx.state === "error" ? D.notice("Распознавание временно недоступно. Уже полученный текст сохранён.", "warning") : ""}<label class="row"><input type="checkbox" checked> Показывать текст на этом устройстве</label>${ctx.role === "owner" ? D.select("Язык распознавания", ["Автоматически", "Русский", "English"]) : ""}${ctx.state === "off" ? '<p class="muted">Распознавание выключено.</p>' : '<div class="card stack"><p><strong>Илья Волков</strong> <span class="muted small">10:04</span><br>Предлагаю начать с обновлений по проекту.</p><p><strong>Анна Морозова</strong> <span class="muted small">10:05</span><br>Да, давайте. Подготовила заметки.</p><p class="muted small">Живой черновик · текст может уточняться</p></div>'}`,
          ctx.state !== "unavailable" && ctx.role === "owner"
            ? D.button(
                ctx.state === "off"
                  ? "Включить распознавание"
                  : "Отключить распознавание",
                "Captions",
                `data-state="${ctx.state === "off" ? "default" : "off"}"`,
                "secondary",
              )
            : "",
        ),
    },
    {
      ...base,
      id: "conference",
      title: "Встреча · два участника",
      states,
      render: (ctx) => room(ctx),
    },
    {
      ...base,
      id: "conference-chat",
      title: "Встреча · чат",
      states: [
        ...states,
        { id: "upload-error", label: "Ошибка загрузки файла" },
      ],
      render: (ctx) =>
        room(ctx, "chat") +
        (ctx.state === "upload-error"
          ? '<div class="cf-upload-error" role="alert">Файл не загружен. <button class="button button-ghost" data-state="default">Повторить</button></div>'
          : ""),
    },
    {
      ...base,
      id: "conference-participants",
      title: "Встреча · участники",
      states: [
        { id: "default", label: "Организатор" },
        { id: "empty", label: "Вы одни" },
      ],
      render: (ctx) => room(ctx, "participants"),
    },
    {
      ...base,
      id: "conference-screen",
      title: "Демонстрация экрана",
      states: [
        { id: "default", label: "Экран занимает сцену" },
        { id: "recording", label: "Демонстрация и запись" },
      ],
      render: (ctx) => room(ctx, "", true),
    },
    {
      ...base,
      id: "recording-control",
      title: "Управление записью",
      api: [
        "POST /api/v1/conferences/:id/recordings",
        "POST /api/v1/conferences/:id/recordings/:recordingId/stop",
        "GET /api/v1/capabilities",
      ],
      states: [
        { id: "default", label: "Запуск" },
        { id: "busy", label: "Уже запущена другим участником" },
        { id: "error", label: "Ошибка запуска" },
        { id: "guest", label: "Гость" },
      ],
      notes: [
        ...base.notes,
        "Одна запись на встречу. Старт: допущенный участник с аккаунтом, active/joined. Стоп: инициатор или owner. При успехе modal закрывается, в шапке остаётся стоп и индикатор. Список файлов в этом modal отсутствует.",
        "Режимы берутся из capabilities, не закрепляются дизайном как всегда включённые.",
      ],
      render: (ctx) =>
        room({ ...ctx, state: "default" }) +
        modal(
          "Запись встречи",
          ctx.state === "guest" || ctx.role === "guest"
            ? D.notice(
                "Чтобы начать запись, войдите в аккаунт. Гости могут участвовать во встрече.",
                "info",
              )
            : ctx.state === "busy"
              ? D.notice(
                  "Илья Волков уже начал запись. Новую можно запустить после завершения текущей обработки.",
                  "info",
                )
              : `${ctx.state === "error" ? D.notice("Не удалось начать запись. Проверьте соединение и повторите.", "danger") : ""}${D.select("Режим записи", ["Общая видеозапись", "Только аудио", "Отдельные дорожки + аудиомикс", "Фокус на экране"])}<p class="muted small">Все участники получат уведомление о начале записи.</p>`,
          ctx.state === "guest" || ctx.role === "guest" || ctx.state === "busy"
            ? D.button("Понятно", "", 'data-go="conference"', "secondary")
            : D.button(
                "Начать запись",
                "Circle",
                'data-go="conference" data-go-state="recording"',
              ),
        ),
    },
    {
      ...base,
      id: "conference-invite",
      title: "Пригласить во встречу",
      states: [
        { id: "default", label: "Ссылка и поиск" },
        { id: "error", label: "Не удалось пригласить" },
      ],
      api: [
        "GET /api/v1/conferences/:id/invitation-users?query=…",
        "POST /api/v1/conferences/:id/invitations",
      ],
      render: (ctx) =>
        room(ctx) +
        modal(
          "Пригласить участников",
          `${ctx.state === "error" ? D.notice("Приглашение не отправлено. Попробуйте ещё раз.", "danger") : ""}<div class="stack"><h3>Поделитесь ссылкой</h3><div class="cf-invite-link"><span>meeting.janickiy.com/i/…</span>${D.button("Копировать", "Copy", 'data-toast="В макете показано копирование ссылки"', "secondary")}</div><p class="muted small">По действующей ссылке можно присоединиться, в том числе без аккаунта.</p><div class="divider"></div>${D.input("Пригласить пользователя", "Имя или email")}<p class="muted small">Введите не менее двух символов</p></div>`,
        ),
    },
    {
      ...base,
      id: "conference-devices",
      title: "Устройства во встрече",
      states: [
        { id: "default", label: "Устройства доступны" },
        { id: "error", label: "Устройство недоступно" },
      ],
      api: [
        "Browser: enumerateDevices, getUserMedia, setSinkId when supported",
      ],
      render: (ctx) =>
        room(ctx) +
        modal(
          "Звук и видео",
          `${ctx.state === "error" ? D.notice("Микрофон занят или отключён. Выберите другое устройство.", "danger") : ""}${D.select("Микрофон", ["Встроенный микрофон"])}<div class="cf-level-meter" aria-label="Уровень микрофона"><i></i><i></i><i></i><i></i><i></i><i></i><i></i><i></i><i></i><i></i></div>${D.select("Динамики", ["Устройство по умолчанию"])}${D.button("Проверить звук", "Volume2", 'data-toast="В макете показана проверка звука"', "secondary")}${D.select("Камера", ["Встроенная камера"])}`,
          D.button("Готово", "", 'data-go="conference"'),
        ),
    },
    {
      ...base,
      id: "waiting-room",
      title: "Ожидание допуска",
      states: [
        { id: "default", label: "Ожидание" },
        { id: "rejected", label: "Запрос отклонён" },
        { id: "closed", label: "Встреча завершена" },
      ],
      permissions:
        "Существующее членство waiting; не обязательный этап входа по валидной invite-ссылке.",
      notes: [
        "В backend действующий inviteCode обходит зал ожидания. Макет waiting показывается только для фактического waiting membership.",
        "Допуск/отказ индивидуальный; действия «Допустить всех» в API/UI нет. В ожидании нет медиасвязи и чата.",
      ],
      sources: [
        "frontend/src/components/WaitingRoomPanel.tsx:64",
        "internal/infrastructure/postgres/conference_repository.go:235",
      ],
      api: [
        "GET /api/v1/conferences/:id/participants/me",
        "POST /api/v1/conferences/:id/participants/:participantId/admission",
      ],
      render: ({ state }) =>
        `<main class="cf-waiting-page"><a class="brand" data-go="home" href="?screen=home"><img src="assets/brand-mark.svg" alt="">Meetrix</a><section class="cf-waiting-content">${D.empty(state === "rejected" ? "Запрос отклонён" : state === "closed" ? "Встреча завершена" : "Вы в зале ожидания", state === "rejected" ? "Организатор не допустил вас к этой встрече." : state === "closed" ? "Присоединиться к этой встрече больше нельзя." : "Организатор увидит ваш запрос. Как только вас допустят, страница обновится.", "Clock3", D.button("К встречам", "ArrowLeft", 'data-go="meetings"', "secondary"))}<div class="card"><h3>Обсуждение проекта</h3><p class="muted small">Имя во встрече: Анна Морозова</p></div>${state === "default" ? '<p class="muted small">Камера, микрофон и чат будут доступны после допуска.</p>' : ""}</section></main>`,
    },
    {
      ...base,
      id: "leave-meeting",
      title: "Выйти из встречи",
      states: [{ id: "default", label: "Подтверждение" }],
      render: (ctx) =>
        room(ctx) +
        modal(
          "Выйти из встречи?",
          "<p>Вы сможете присоединиться снова, пока встреча продолжается.</p>",
          D.button("Остаться", "", 'data-go="conference"', "secondary") +
            D.button("Выйти", "LogOut", 'data-go="meetings"', "danger"),
        ),
    },
    {
      ...base,
      id: "finish-meeting",
      title: "Завершить для всех",
      states: [{ id: "default", label: "Подтверждение организатора" }],
      permissions: "Только owner конференции.",
      api: ["POST /api/v1/conferences/:id/finish"],
      render: (ctx) =>
        room(ctx) +
        modal(
          "Завершить встречу для всех?",
          "<p>Участники будут отключены. Запись остановится и сохранится после обработки.</p>",
          D.button(
            "Продолжить встречу",
            "",
            'data-go="conference"',
            "secondary",
          ) + D.button("Завершить", "Square", 'data-go="meetings"', "danger"),
        ),
    },
  ]);
})();
