(() => {
  const D = window.MeetrixDesign;
  const states = [
    { id: "default", label: "Обычное" },
    { id: "loading", label: "Загрузка" },
    { id: "empty", label: "Пусто" },
    { id: "error", label: "Ошибка" },
  ];
  const messagingStates = [
    ...states,
    { id: "reconnect", label: "Переподключение" },
  ];
  const responsive = {
    desktop:
      "1440–1920: список 320 px, переписка от 420 px. Поля 24 px; шапка 72 px; строка ввода закреплена снизу.",
    tablet:
      "768–1023: список 272 px при достаточной ширине; иначе список и диалог раздельно. Глобальная навигация свёрнута.",
    mobile:
      "360–767: отдельный экран списка → отдельный диалог, назад возвращает в список. Элементы касания не менее 44 px; клавиатура не перекрывает ввод.",
  };
  const base = {
    group: "Общение",
    layout: "app",
    nav: "personal",
    states,
    permissions:
      "Зарегистрированная учётная запись; доступ только к своим перепискам.",
    responsive,
    sources: [
      "frontend/src/pages/PersonalPage.tsx:36",
      "internal/transport/http/personal_routes.go:12",
    ],
    notes: [
      "Вымышленные данные показывают поддержанные поля API; действия работают только внутри дизайн-прототипа.",
    ],
    afterRender: wireMessagingDesign,
  };
  function wireMessagingDesign() {
    document
      .querySelectorAll(".msg-action-menu,.msg-emoji-panel")
      .forEach((menu) => menu.classList.add("dropdown"));
    if (D.current?.screen.id === "direct-chat" && D.current.state === "muted") {
      const subtitle = document.querySelector(".msg-peer small");
      if (subtitle) subtitle.textContent = "Без уведомлений";
    }
    const composer = document.querySelector(".msg-compose textarea");
    const messages = document.querySelector(".msg-message-list");
    if (composer) composer.dataset.composer = "";
    if (messages) messages.dataset.messages = "";
    const send = document.querySelector(".msg-compose-row>.button-primary");
    if (send) {
      delete send.dataset.toast;
      send.dataset.action = "send-demo";
    }
    document.querySelectorAll(".msg-emoji-grid button").forEach((button) => {
      delete button.dataset.toast;
      button.dataset.emoji = button.textContent;
    });
    document.querySelectorAll(".msg-tabs .tab").forEach((button) => {
      button.addEventListener("click", () => {
        for (const sibling of button.parentElement.children) {
          sibling.classList.toggle("active", sibling === button);
          sibling.setAttribute("aria-pressed", String(sibling === button));
        }
        const list = document.querySelector(".msg-list-body");
        if (button.closest(".msg-list-pane") && list) {
          list.querySelectorAll(".msg-conversation").forEach((row) => {
            const group = row.dataset.go === "group-chat";
            row.hidden =
              button.textContent === "Личные"
                ? group
                : button.textContent === "Группы"
                  ? !group
                  : button.textContent === "Новые"
                    ? !row.querySelector(".msg-unread")
                    : false;
          });
        }
      });
    });
    document
      .querySelectorAll(".msg-list-pane .msg-search input")
      .forEach((input) => {
        input.addEventListener("input", () =>
          document
            .querySelectorAll(".msg-conversation")
            .forEach(
              (row) =>
                (row.hidden = !row.textContent
                  .toLocaleLowerCase("ru")
                  .includes(input.value.toLocaleLowerCase("ru"))),
            ),
        );
      });
    document
      .querySelectorAll('input[name="design-theme"]')
      .forEach((input, index) => {
        input.checked =
          (document.documentElement.dataset.theme === "dark") === (index === 1);
        input.addEventListener("change", () =>
          D.theme(index === 1 ? "dark" : "light"),
        );
      });
    if (D.current?.screen.id === "settings-appearance") {
      const size = document.querySelector(".msg-settings-content select");
      const currentSize =
        new URLSearchParams(location.search).get("scale") || "100";
      if (size) {
        size.value =
          [...size.options].find(
            (option) => String(parseInt(option.value, 10)) === currentSize,
          )?.value || size.options[0].value;
        size.addEventListener("change", () => {
          const params = new URLSearchParams(location.search);
          params.set("scale", String(parseInt(size.value, 10)));
          history.replaceState({}, "", `${location.pathname}?${params}`);
          D.render();
        });
      }
    }
    if (
      ["direct-clear", "direct-delete", "folder-delete"].includes(
        D.current?.screen.id,
      )
    )
      document
        .querySelector(".msg-dialog-footer button")
        ?.focus({ preventScroll: true });
  }
  const iconLabels = {
    X: "Закрыть",
    ArrowLeft: "Назад",
    Download: "Скачать файл",
    Pencil: "Изменить сообщение",
    MoreHorizontal: "Действия",
    Paperclip: "Прикрепить файл",
    Send: "Отправить сообщение",
  };
  const action = (text, icon, toast, kind = "secondary") =>
    D.button(
      text,
      icon,
      `data-toast="${D.esc(toast)}"${text ? "" : ` aria-label="${iconLabels[icon] || D.esc(toast)}"`}`,
      kind,
    );
  const go = (text, icon, id, kind = "secondary") =>
    D.button(
      text,
      icon,
      `data-go="${id}"${text ? "" : ` aria-label="${iconLabels[icon] || "Открыть"}"`}`,
      kind,
    );
  const more = (id, options) =>
    `<div class="msg-menu-wrap">${D.button("", "MoreHorizontal", `data-menu="${id}" aria-label="Действия"`, "icon")}<div class="msg-action-menu" id="${id}" hidden>${options.map(([label, icon, target, danger]) => go(label, icon, target, danger ? "danger" : "ghost")).join("")}</div></div>`;
  const directMenu = (id) =>
    `<div class="msg-menu-wrap">${D.button("", "MoreHorizontal", `data-menu="${id}" aria-label="Действия с перепиской"`, "icon")}<div class="msg-action-menu" id="${id}" hidden>${go("Информация", "Info", "direct-info", "ghost")}${D.button(D.current?.state === "muted" ? "Включить уведомления" : "Без уведомлений", D.current?.state === "muted" ? "Bell" : "BellOff", `data-state="${D.current?.state === "muted" ? "default" : "muted"}"`, "ghost")}${go("Добавить в папку", "FolderPlus", "folder-add", "ghost")}${go("Очистить историю", "Eraser", "direct-clear", "ghost")}${go("Удалить чат", "Trash2", "direct-delete", "danger")}</div></div>`;
  const error = (title = "Не удалось загрузить данные") =>
    `<div class="msg-state">${D.empty(title, "Проверьте подключение и попробуйте ещё раз.", "WifiOff", D.button("Повторить", "RefreshCw", 'data-state="default"', "primary"))}</div>`;
  const tabs = (labels, active = 0) =>
    `<div class="tabs msg-tabs" role="group" aria-label="Фильтры">${labels.map((label, index) => `<button class="tab ${index === active ? "active" : ""}" aria-pressed="${index === active}" data-toast="Показан фильтр: ${label}">${label}</button>`).join("")}</div>`;
  const people = [
    {
      name: "Мария Орлова",
      text: "Макеты посмотрела, спасибо!",
      time: "12:42",
      tone: "teal",
      n: 2,
      id: "direct-chat",
    },
    {
      name: "Команда продукта",
      text: "Иван: Обсудим завтра на встрече",
      time: "12:28",
      tone: "purple",
      n: 5,
      id: "group-chat",
      group: true,
    },
    {
      name: "Иван Ким",
      text: "Вы: Отлично, до встречи",
      time: "11:15",
      tone: "blue",
      id: "direct-chat",
      muted: true,
    },
    {
      name: "Дизайн-команда",
      text: "Вложение",
      time: "Вчера",
      tone: "teal",
      id: "group-chat",
      group: true,
    },
  ];
  const conversationList = (state, selected) =>
    `<aside class="msg-list-pane"><header class="msg-list-title"><h1>Личные</h1>${D.button("", "SquarePen", 'data-menu="new-chat-menu" aria-label="Новый чат"', "icon")}<div id="new-chat-menu" class="msg-action-menu" hidden>${go("Личный чат", "UserRound", "new-direct", "ghost")}${go("Группа", "Users", "group-create", "ghost")}</div></header><label class="msg-search">${D.icon("Search", 18)}<input type="search" aria-label="Поиск переписок" placeholder="Найти переписку" /></label>${tabs(["Все", "Личные", "Группы", "Новые"])}<div class="msg-list-body">${state === "loading" ? D.skeleton(4) : state === "error" ? error("Переписки недоступны") : state === "empty" ? D.empty("Пока нет переписок", "Начните разговор с коллегой или создайте группу.", "MessageCircle", go("Новый чат", "Plus", "new-direct", "primary")) : people.map((p, index) => `<button class="msg-conversation ${selected === p.id && index < 2 ? "is-selected" : ""}" data-go="${p.id}">${p.group ? `<span class="msg-group-avatar ${p.tone}">${D.icon("Users", 20)}</span>` : D.avatar(p.name, p.tone)}<span class="msg-conversation-copy"><strong>${p.name}</strong><span>${p.text}</span></span><span class="msg-conversation-meta"><time>${p.time}</time>${p.n ? `<span class="msg-unread" aria-label="${p.n} непрочитанных">${p.n}</span>` : p.muted ? D.icon("BellOff", 14) : ""}</span></button>`).join("")}</div><footer class="msg-list-footer">Переписки доступны только участникам</footer></aside>`;
  const fileBubble = `<div class="msg-file"><span class="msg-file-icon">${D.icon("FileText", 24)}</span><div><strong>Структура встречи.pdf</strong><small>PDF · 428 КБ</small></div>${action("", "Download", "Показано получение защищённой ссылки на файл", "icon")}</div>`;
  const message = (name, body, time, own = false, extra = "") => {
    const menuId = `msg-message-menu-${time.replace(":", "")}-${own ? "own" : "peer"}`;
    return `<article class="msg-message ${own ? "is-own" : ""}">${own ? "" : D.avatar(name, "teal", "small")}<div class="msg-message-inner">${own ? "" : `<strong class="msg-sender">${name}</strong>`}<div class="msg-bubble">${body}<footer>${extra}<time>${time}</time></footer></div><div class="msg-message-tools"><div class="msg-menu-wrap">${D.button("", "MoreHorizontal", `data-menu="${menuId}" aria-controls="${menuId}" aria-expanded="false" aria-label="Действия с сообщением: ${D.esc(name)}, ${time}"`, "icon")}<div class="msg-action-menu" id="${menuId}" hidden>${action("Ответить", "Reply", "Показано состояние ответа с цитатой", "ghost")}${own ? action("Изменить", "Pencil", "Показано состояние редактирования своего сообщения", "ghost") + action("Удалить", "Trash2", "Показано подтверждение удаления своего сообщения", "danger") : ""}</div></div></div></div></article>`;
  };
  const composer = () =>
    `<div class="msg-compose"><div class="msg-composer-hint" hidden id="emoji-hint">Смайлик добавлен в макет сообщения</div><div class="msg-compose-row">${action("", "Paperclip", "Показана очередь вложений: до 5 файлов, до 10 МиБ каждый", "icon")}<label class="sr-only" for="msg-input">Сообщение</label><textarea id="msg-input" rows="1" placeholder="Напишите сообщение…"></textarea>${D.button("", "Smile", 'data-menu="msg-emoji-menu" aria-label="Выбрать смайлик"', "icon")}${action("", "Send", "Показано состояние отправки сообщения", "primary")}</div><div id="msg-emoji-menu" class="msg-emoji-panel" hidden><div class="spread"><strong>Смайлики</strong><span class="muted small">64 варианта</span></div><div class="msg-emoji-grid">${"😀 😃 😄 😁 😆 😅 😂 🤣 😊 🙂 🙃 😉 😍 🥰 😘 😋 😛 😜 🤪 😎 🤓 🧐 🤩 🥳 😏 😌 🤔 🤨 😐 😶 😑 😒 🙄 😬 😮 😯 😲 😳 🥺 😢 😭 😤 😠 😡 😱 😴 🤗 🤝 👍 👎 👏 🙌 👋 ✋ 🤞 💪 🙏 ❤️ 🧡 💛 💚 💜 🔥 🎉"
      .split(" ")
      .map(
        (e) =>
          `<button aria-label="Вставить ${e}" data-toast="Показан выбор смайлика ${e}">${e}</button>`,
      )
      .join("")}</div></div></div>`;
  const thread = (state, group) =>
    `<section class="msg-thread"><header class="msg-thread-header">${go("", "ArrowLeft", "personal", "icon")}<button class="msg-peer" data-go="${group ? "group-info" : "direct-info"}">${group ? `<span class="msg-group-avatar purple">${D.icon("Users", 21)}</span>` : D.avatar("Мария Орлова", "teal")}<span><strong>${group ? "Команда продукта" : "Мария Орлова"}</strong><small>${group ? "6 участников" : "Личная переписка"}</small></span></button>${
      group
        ? more("group-menu", [
            ["Информация", "Info", "group-info"],
            ["Добавить в папку", "FolderPlus", "folder-add"],
          ])
        : directMenu("direct-menu")
    }</header>${state === "reconnect" ? `<div class="msg-reconnect">${D.icon("RefreshCw", 16)}Переподключаемся… Сообщения обновятся автоматически.</div>` : ""}<div class="msg-message-list">${state === "loading" ? D.skeleton(4) : state === "error" ? error("Не удалось открыть переписку") : state === "empty" ? D.empty("Начните разговор", group ? "Сообщения будут видны участникам группы." : "Отправьте первое сообщение или прикрепите файл.", "MessageCircle") : `<div class="msg-date">Сегодня</div>${message(group ? "Иван Ким" : "Мария Орлова", "<p>Привет! Подготовила структуру следующей встречи. Посмотри, пожалуйста, когда будет время.</p>", "12:34")}${message("Мария Орлова", fileBubble, "12:35")}${message("Вы", "<blockquote><strong>Мария Орлова</strong>Подготовила структуру следующей встречи.</blockquote><p>Спасибо! Посмотрел — всё понятно. Добавим в конце 10 минут на вопросы? 👍</p>", "12:40", true)}${message("Мария Орлова", "<p>Да, хорошая идея. Обновлю повестку до вечера.</p>", "12:42")}`}</div>${composer()}</section>`;
  const chat = (ctx, group = false, selected = true) =>
    `<section class="msg-workspace ${ctx.width < 900 ? "msg-narrow" : ""} ${selected ? "msg-selected" : ""}">${conversationList(selected ? "default" : ctx.state, selected ? (group ? "group-chat" : "direct-chat") : "")}${selected ? thread(ctx.state, group) : `<section class="msg-welcome"><div class="msg-welcome-symbol">${D.icon("MessagesSquare", 40)}</div><h2>Разговор начинается здесь</h2><p>Выберите переписку или начните новую.<br />Команда всегда рядом.</p>${go("Новый чат", "SquarePen", "new-direct", "primary")}<span class="msg-welcome-caption">Личные разговоры и группы в одном месте</span></section>`}</section>`;
  const modal = (title, body, footer = "", size = "", back = "personal") =>
    `<div class="msg-modal-scene"><div class="msg-modal-underlay" aria-hidden="true"><div class="msg-underlay-heading"></div><div class="msg-underlay-columns"><div></div><div></div></div></div><section class="msg-dialog ${size}" role="dialog" aria-modal="true" aria-label="${title}"><header class="msg-dialog-header"><h1>${title}</h1>${go("", "X", back, "icon")}</header><div class="msg-dialog-body">${body}</div>${footer ? `<footer class="msg-dialog-footer">${footer}</footer>` : ""}</section></div>`;
  const selectablePerson = (name, tone, detail, selected = false) =>
    `<label class="msg-person-select">${D.avatar(name, tone)}<span><strong>${name}</strong><small>${detail}</small></span><input type="checkbox" ${selected ? "checked" : ""} aria-label="Выбрать ${name}" /></label>`;
  const peoplePicker = () =>
    `${D.input("Найти пользователя", "Введите минимум 2 символа", "Ма", "search")}<div class="msg-search-caption">Поиск по имени · выбрано 2 из 99</div><div class="msg-selected-people"><span>Мария Орлова ${D.icon("X", 14)}</span><span>Иван Ким ${D.icon("X", 14)}</span></div><div class="msg-person-options">${selectablePerson("Мария Орлова", "teal", "Пользователь Meetrix", true)}${selectablePerson("Марина Соколова", "purple", "Пользователь Meetrix")}</div>`;
  const settingsSections = [
    ["profile", "Профиль", "UserRound"],
    ["audio", "Аудио", "Mic"],
    ["video", "Видео", "Video"],
    ["notifications", "Уведомления", "Bell"],
    ["appearance", "Оформление", "Palette"],
  ];
  const toggle = (title, caption, checked = true) =>
    `<label class="msg-setting-toggle"><span><strong>${title}</strong>${caption ? `<small>${caption}</small>` : ""}</span><input type="checkbox" role="switch" ${checked ? "checked" : ""} /></label>`;
  const deviceLine = (title, options, icon) =>
    `<div class="msg-device-row">${D.select(title, options)}${action("Проверить", icon, `Показано состояние проверки: ${title}`)}</div>`;
  const settingsBody = (section, state) => {
    if (state === "loading") return D.skeleton(3);
    if (state === "error") return error("Настройки не загрузились");
    if (section === "profile")
      return `<div class="msg-settings-heading"><div><h2>Ваш профиль</h2><p>Как вас видят участники встречи</p></div>${D.avatar("Анна Морозова", "blue", "large")}</div>${D.input("Имя для встреч", "", "Анна Морозова")}<p class="msg-field-hint">От 1 до 100 символов</p><label class="field"><span>Email</span><input value="anna@example.com" readonly /><small>Используется для входа. Изменить адрес пока нельзя.</small></label><div class="msg-settings-save">${action("Сохранить имя", "", "Показано состояние сохранённого имени", "primary")}</div><p class="msg-registration">${D.icon("CalendarDays", 16)}Зарегистрирована 14 сентября 2026</p>`;
    if (section === "audio")
      return `<div class="msg-settings-heading"><div><h2>Аудио</h2><p>Подготовьте звук к следующей встрече</p></div></div><div class="msg-setting-block"><div class="spread"><h3>Микрофон</h3><span class="msg-level" aria-label="Уровень микрофона, пример">${[1, 2, 3, 4, 5, 6, 7, 8].map((i) => `<i class="${i <= 5 ? "lit" : ""}"></i>`).join("")}</span></div>${deviceLine("Устройство ввода", ["Встроенный микрофон", "По умолчанию"], "Mic")}<p class="msg-field-hint">Нажмите «Проверить», затем произнесите несколько слов.</p>${toggle("Входить с выключенным микрофоном", "", false)}${toggle("Шумоподавление", "Доступность зависит от браузера")}</div><div class="msg-setting-block"><h3>Динамик</h3>${deviceLine("Устройство вывода", ["Встроенные динамики", "По умолчанию"], "Volume2")}</div><div class="msg-setting-block"><h3>Звук уведомлений</h3>${deviceLine("Источник звука", ["По умолчанию", "Встроенные динамики"], "Volume2")}</div><p class="msg-field-hint">Выбор устройств сохраняется в этом браузере.</p>`;
    if (section === "video")
      return `<div class="msg-settings-heading"><div><h2>Видео</h2><p>Предпросмотр доступен только вам</p></div></div><div class="msg-camera-preview"><span>${D.icon("VideoOff", 32)}</span><strong>Камера выключена</strong><p>В макете камера не включается</p>${action("Проверить камеру", "Video", "Показано состояние предпросмотра камеры")}</div>${D.select("Камера", ["Встроенная камера", "По умолчанию"])}<div class="msg-setting-block">${toggle("Входить с выключенной камерой", "", true)}${toggle("Видеть себя на звонке", "", true)}${toggle("Скрыть видео участников", "Настройка применяется только к вашему экрану", false)}</div>`;
    if (section === "notifications")
      return `<div class="msg-settings-heading"><div><h2>Уведомления</h2><p>Выберите события и каналы доставки</p></div></div><div class="msg-setting-block"><h3>События</h3>${toggle("Приглашения и изменения встреч", "", true)}${toggle("Напоминания о встречах", "", true)}${toggle("Готовность записи", "", true)}${toggle("Расшифровка и итоги встречи", "События появляются, когда обработка доступна", false)}</div><div class="msg-setting-block"><h3>Каналы доставки</h3>${toggle("Email", "Для автоматических уведомлений", true)}<label class="msg-setting-toggle"><span><strong>Push-уведомления</strong><small>Канал не настроен для этой установки</small></span><input type="checkbox" role="switch" disabled /></label></div><p class="msg-field-hint">Личные приглашения, отправленные организатором, приходят отдельно.</p>${action("Сохранить настройки", "", "Показано состояние сохранённых предпочтений", "primary")}`;
    return `<div class="msg-settings-heading"><div><h2>Оформление</h2><p>Настройте Meetrix под себя</p></div></div><fieldset class="msg-themes"><legend>Тема</legend><label class="msg-theme-card"><span class="msg-theme-preview light"><i></i><b></b><b></b></span><span>${D.icon("Sun", 18)}Светлая тема<input type="radio" name="design-theme" checked data-toast="Показан выбор светлой темы" /></span></label><label class="msg-theme-card"><span class="msg-theme-preview dark"><i></i><b></b><b></b></span><span>${D.icon("Moon", 18)}Тёмная тема<input type="radio" name="design-theme" data-toast="Показан выбор тёмной темы" /></span></label></fieldset><div class="msg-setting-block"><h3>Внешний вид</h3>${D.select("Размер текста", ["100% (по умолчанию)", "75%", "90%", "110%", "125%", "150%", "200%"])}<div class="msg-type-example"><strong>Удобный размер для важных разговоров</strong><p>Все ваши встречи и сообщения — в одном месте.</p></div></div><p class="msg-field-hint">Применяется сразу. Сохраняется для вашего аккаунта в этом браузере.</p>`;
  };
  const settingsRender = (section, ctx) =>
    modal(
      "Настройки аккаунта",
      `<div class="msg-settings-layout"><nav class="msg-settings-nav" aria-label="Разделы настроек">${settingsSections.map(([id, label, icon]) => `<button class="${id === section ? "active" : ""}" data-go="settings-${id}">${D.icon(icon, 19)}${label}${D.icon("ChevronRight", 16)}</button>`).join("")}</nav><div class="msg-settings-content">${ctx.width < 768 ? go("Все настройки", "ArrowLeft", "settings", "ghost") : ""}${settingsBody(section, ctx.state)}</div></div>`,
      "",
      `msg-settings-dialog ${ctx.width < 768 ? "msg-mobile-settings" : ""}`,
      "home",
    );
  const folders = [
    ["Продуктовая команда", "2 встречи · 3 чата", 5, "blue"],
    ["Дизайн и исследования", "2 встречи · 2 чата", 4, "purple"],
    ["Рабочие встречи", "6 встреч · 1 чат", 7, "teal"],
  ];
  const folderRows = () =>
    `<div class="msg-folder-list">${folders
      .map(
        ([name, detail, count, tone], index) =>
          `<article class="msg-folder-row"><button class="msg-folder-link" data-go="folder-detail"><span class="msg-folder-symbol ${tone}">${D.icon("Folder", 26)}</span><span><strong>${name}</strong><small>${detail}</small></span><span class="msg-folder-count">${count}</span></button>${more(
            `folder-row-${index}`,
            [
              ["Переименовать", "Pencil", "folder-create"],
              ["Выше", "ArrowUp", "folders"],
              ["Ниже", "ArrowDown", "folders"],
              ["Удалить папку", "Trash2", "folder-delete", true],
            ],
          )}</article>`,
      )
      .join("")}</div>`;
  const folderBase = {
    ...base,
    group: "Организация",
    nav: "folders",
    permissions:
      "Только владелец личной папки. Папка не предоставляет права на встречу или переписку.",
    api: [
      "GET/POST /api/v1/folders",
      "GET/PATCH/DELETE /api/v1/folders/:id",
      "PUT /api/v1/folders/order",
    ],
    sources: [
      "frontend/src/pages/FoldersPage.tsx:41",
      "internal/domain/folders/folders.go:15",
    ],
    notes: [
      "До 100 папок; имя 1–50 символов NFC, уникальное без учёта регистра. Удаление папки удаляет ссылки, но сохраняет встречи и чаты.",
    ],
    responsive: {
      desktop: "Карточка-список до 1040 px, строки 88 px; действия справа.",
      tablet: "Список на доступную ширину; меню привязано к строке.",
      mobile:
        "Вертикальные строки с иконкой 44 px, метаданные под заголовком; фильтры прокручиваются.",
    },
  };
  const notificationItems = [
    [
      "Video",
      "Вас пригласили на встречу",
      "Откройте встречу, чтобы посмотреть приглашение.",
      "Сегодня, 12:30",
      true,
    ],
    [
      "CirclePlay",
      "Запись встречи готова",
      "Можно вернуться к разговору и посмотреть запись.",
      "Сегодня, 11:15",
      true,
    ],
    [
      "CalendarDays",
      "Скоро начнётся встреча",
      "Проверьте устройства перед подключением.",
      "Сегодня, 09:45",
      false,
    ],
    [
      "CalendarDays",
      "Время встречи изменилось",
      "Актуальное время указано на странице встречи.",
      "Вчера, 17:10",
      false,
    ],
  ];
  D.register([
    {
      ...base,
      id: "personal",
      title: "Личные · список и пустая область",
      route: "/personal",
      api: ["GET /api/v1/conversations"],
      render: (ctx) => chat(ctx, false, false),
    },
    {
      ...base,
      id: "direct-chat",
      title: "Личная переписка",
      route: "/personal/:id",
      states: [...messagingStates, { id: "muted", label: "Без уведомлений" }],
      api: [
        "GET/POST /api/v1/conversations/:id/messages",
        "PATCH/DELETE /api/v1/conversations/:id/messages/:messageId",
        "PATCH /api/v1/conversations/:id/preferences",
      ],
      notes: [
        ...base.notes,
        "Нет typing, peer read ticks, личного online или звонка из чата. Ответ, своё редактирование/удаление, 64 emoji и вложения реализованы.",
        "Reconnecting — новый визуальный статус существующего WebSocket reconnect; hook требует frontend wiring.",
        "4000 символов; до 5 вложений по 10 МиБ: JPG/PNG/WebP/PDF/TXT/CSV.",
      ],
      render: (ctx) => chat(ctx),
    },
    {
      ...base,
      id: "new-direct",
      title: "Новая личная переписка",
      route: "/personal (новая точка входа)",
      api: ["GET /api/v1/users?search=", "POST /api/v1/conversations/direct"],
      notes: [
        "API уже существует и используется из информации об участнике встречи. Меню Новый чат — предлагаемая frontend-точка входа.",
        "Поиск 2–100 символов; до 20 найденных пользователей; только зарегистрированные, кроме себя.",
      ],
      render: (ctx) =>
        modal(
          "Новый чат",
          ctx.state === "loading"
            ? D.skeleton(3)
            : ctx.state === "error"
              ? error("Поиск временно недоступен")
              : `${D.input("Найти пользователя", "Имя пользователя", "Мар", "search")}<p class="msg-field-hint">Введите минимум 2 символа имени.</p>${ctx.state === "empty" ? D.empty("Никого не нашли", "Попробуйте другое имя.", "Search") : `<div class="msg-user-results">${["Мария Орлова", "Марина Соколова"].map((name) => `<button data-go="direct-chat">${D.avatar(name, "teal")}<span><strong>${name}</strong><small>Начать переписку</small></span>${D.icon("ChevronRight", 18)}</button>`).join("")}</div>`}`,
        ),
    },
    {
      ...base,
      id: "direct-info",
      title: "Информация о пользователе",
      route: "/personal/:id (modal)",
      states: [states[0]],
      api: ["GET /api/v1/conversations/:id"],
      notes: [
        "Публичный peer содержит только id и displayName. Email, профильное фото, статус сети и last seen не добавляются.",
      ],
      render: () =>
        modal(
          "Информация о пользователе",
          `<div class="msg-user-identity">${D.avatar("Мария Орлова", "teal", "large")}<h2>Мария Орлова</h2><p class="muted">Участник вашей переписки</p></div><dl class="msg-definition"><dt>Имя</dt><dd>Мария Орлова</dd><dt>Идентификатор пользователя</dt><dd class="msg-user-id">019b4000-71a3-4000-bb00-000000000001</dd></dl>`,
          go("Закрыть", "", "direct-chat"),
          "",
          "direct-chat",
        ),
    },
    ...[
      [
        "direct-clear",
        "Очистить историю?",
        "Переписка будет очищена только у вас. История собеседника сохранится. Отменить очистку нельзя.",
        "Очистить историю",
        "clear-history",
      ],
      [
        "direct-delete",
        "Удалить чат?",
        "Чат исчезнет из вашего списка и папок, а история очистится только у вас. Новый входящий ответ снова вернёт чат в список.",
        "Удалить чат",
        "hide",
      ],
    ].map(([id, title, text, label, endpoint]) => ({
      ...base,
      id,
      title,
      route: "/personal/:id (подтверждение)",
      states: [states[0], states[3]],
      api: [`POST /api/v1/conversations/:id/${endpoint}`],
      notes: [
        "Действие влияет только на свою историю и видимость. Фокус по умолчанию на Отмена. При ошибке диалог остаётся открытым.",
      ],
      render: (ctx) =>
        modal(
          title,
          `<div class="msg-danger-symbol">${D.icon(endpoint === "hide" ? "Trash2" : "Eraser", 26)}</div><h2>Переписка с Марией Орловой</h2><p class="msg-confirm-copy">${text}</p>${ctx.state === "error" ? D.notice("Не удалось выполнить действие. Попробуйте ещё раз.", "danger") : ""}`,
          go("Отмена", "", "direct-chat") +
            action(
              label,
              "",
              "Показано состояние подтверждения действия",
              "danger",
            ),
          "msg-confirm-dialog",
          "direct-chat",
        ),
    })),
    {
      ...base,
      id: "group-chat",
      title: "Групповая переписка",
      route: "/personal/:id type=group",
      states: messagingStates,
      api: [
        "GET /api/v1/conversations/:id",
        "GET/POST /api/v1/conversations/:id/messages",
      ],
      notes: [
        "Сообщения доступны активным участникам. Имя, memberCount, avatar; нет голосовых сообщений, стикеров, pinned или read-by-peer.",
        "Reconnecting требует frontend отображения текущего состояния WS.",
      ],
      render: (ctx) => chat(ctx, true),
    },
    {
      ...base,
      id: "group-create",
      title: "Создание группы",
      route: "/personal (modal)",
      states: [
        states[0],
        states[3],
        { id: "partial", label: "Аватар не загружен" },
      ],
      api: [
        "POST /api/v1/conversations/group",
        "GET /api/v1/users",
        "PUT /api/v1/conversations/:id/avatar",
      ],
      permissions:
        "Зарегистрированный пользователь становится owner созданной группы.",
      notes: [
        "Имя 1–50, описание 0–200 символов. 100 участников включая owner. Аватар PNG/JPEG до 2 МиБ. При ошибке аватара после создания не создавать группу повторно.",
      ],
      render: (ctx) =>
        modal(
          "Новая группа",
          `<div class="msg-group-photo"><button aria-label="Выбрать аватар" data-toast="Показано состояние выбора PNG/JPEG до 2 МиБ">${D.icon("Camera", 24)}</button><div><strong>Аватар группы</strong><small>PNG или JPEG · до 2 МиБ</small></div></div>${D.input("Название группы", "Например, Команда продукта", "Команда продукта")}<p class="msg-field-hint">16 / 50</p><label class="field"><span>Описание <small>необязательно</small></span><textarea rows="2" placeholder="О чём эта группа?">Обсуждаем продукт и готовимся к встречам.</textarea></label><p class="msg-field-hint">41 / 200</p><h3>Участники</h3>${peoplePicker()}${ctx.state === "error" ? D.notice("Не удалось создать группу. Введённые данные сохранены в форме.", "danger") : ctx.state === "partial" ? D.notice("Группа создана, но аватар не загрузился. Повторите загрузку или откройте группу без него.", "warning") + action("Повторить загрузку аватара", "RefreshCw", "Показана повторная загрузка аватара без повторного создания группы") : ""}`,
          go("Отмена", "", "personal") +
            (ctx.state === "partial"
              ? go("Открыть без аватара", "", "group-chat", "primary")
              : go("Создать группу", "", "group-chat", "primary")),
        ),
    },
    {
      ...base,
      id: "group-info",
      title: "Информация и участники группы",
      route: "/personal/:id (modal)",
      api: [
        "GET /api/v1/conversations/:id/members",
        "POST /api/v1/conversations/:id/members",
        "PATCH/DELETE /api/v1/conversations/:id/members/:userId",
        "POST /api/v1/conversations/:id/ownership",
        "POST /api/v1/conversations/:id/leave",
        "DELETE /api/v1/conversations/:id",
      ],
      permissions:
        "member: чтение, выход; admin: метаданные, добавление, удаление member; owner: роли, передача владельца, удаление группы.",
      notes: [
        "Владелец группы с другими участниками перед выходом передаёт владение. Удаление группы для всех подтверждается.",
        "Nullable online есть только у GroupMember: показать Статус недоступен при отсутствии актуальных данных.",
        "Пустое состояние относится к поиску участников; существующая группа не изображается без владельца.",
      ],
      render: (ctx) =>
        modal(
          "Информация о группе",
          ctx.state === "loading"
            ? D.skeleton(4)
            : ctx.state === "error"
              ? error("Информация недоступна")
              : `<div class="msg-group-identity"><span class="msg-group-avatar purple large">${D.icon("Users", 30)}</span><div><h2>Команда продукта</h2><p>Обсуждаем продукт и готовимся к встречам.</p><span class="muted small">6 участников · ${{ owner: "Владелец", admin: "Администратор", member: "Участник" }[ctx.role] || "Участник"}</span></div></div><div class="msg-group-quick">${["owner", "admin"].includes(ctx.role) ? action("Изменить", "Pencil", "Показана форма изменения метаданных") + action("Добавить", "UserPlus", "Показан выбор новых участников существующей группы") : ""}</div><div class="spread"><h3>Участники</h3><span class="muted small">6 / 100</span></div><label class="msg-search">${D.icon("Search", 18)}<input placeholder="Найти участника" aria-label="Поиск участников" value="${ctx.state === "empty" ? "Нет совпадений" : ""}" /></label><div class="msg-members">${
                  ctx.state === "empty"
                    ? D.empty(
                        "Никого не нашли",
                        "Попробуйте другое имя участника.",
                        "Search",
                      )
                    : [
                        [
                          "Анна Морозова",
                          `${{ owner: "Владелец", admin: "Администратор", member: "Участник" }[ctx.role] || "Участник"} · вы`,
                          true,
                        ],
                        [
                          "Мария Орлова",
                          ctx.role === "owner" ? "Администратор" : "Владелец",
                          true,
                        ],
                        ["Иван Ким", "Участник", false],
                        ["Анна Власова", "Участник", null],
                        ["Дмитрий Волков", "Участник", false],
                        ["Ольга Романова", "Участник", true],
                      ]
                        .map(
                          ([name, role, online], i) =>
                            `<div class="msg-member">${D.avatar(name, i % 2 ? "teal" : "blue")}<div><strong>${name}</strong><small>${role}</small></div><span class="msg-presence ${online ? "online" : ""}">${online === null ? "Статус недоступен" : online ? "В сети" : "Не в сети"}</span>${(ctx.role === "owner" && i > 0) || (ctx.role === "admin" && role === "Участник") ? action("", "MoreHorizontal", "Показано меню прав участника по вашей роли", "icon") : ""}</div>`,
                        )
                        .join("")
                }</div><div class="msg-group-danger">${ctx.role === "owner" ? action("Передать владение", "ShieldCheck", "Показано подтверждение передачи владения") + action("Удалить группу", "Trash2", "Показано подтверждение удаления для всех", "danger") : action("Выйти из группы", "LogOut", "Показано подтверждение выхода", "danger")}</div>`,
          "",
          "msg-group-info-dialog",
          "group-chat",
        ),
    },
    {
      ...folderBase,
      id: "folders",
      title: "Личные папки",
      route: "/folders",
      render: (ctx) =>
        `<section class="page msg-folder-page">${D.heading("Папки", "Соберите нужные встречи и переписки в одном месте.", go("Новая папка", "Plus", "folder-create", "primary"))}${ctx.state === "loading" ? D.skeleton(3) : ctx.state === "error" ? error() : ctx.state === "empty" ? `<div class="card msg-spacious-empty">${D.empty("У вас пока нет папок", "Создайте первую папку и добавьте в неё важные встречи или чаты.", "FolderPlus", go("Создать папку", "Plus", "folder-create", "primary"))}</div>` : folderRows()}<p class="msg-private-note">${D.icon("LockKeyhole", 16)}Ваши папки видны только вам</p></section>`,
    },
    {
      ...folderBase,
      id: "folder-detail",
      title: "Содержимое папки",
      route: "/folders/:id",
      api: [
        ...folderBase.api,
        "GET /api/v1/folders/:id/items",
        "GET /api/v1/folder-items",
        "PUT/DELETE /api/v1/folders/:id/items/:kind/:itemId",
      ],
      render: (ctx) =>
        `<section class="page msg-folder-page">${go("Все папки", "ArrowLeft", "folders", "ghost")}${D.heading("Продуктовая команда", ctx.state === "empty" ? "Пока нет элементов" : ctx.state === "default" ? "2 встречи · 3 чата" : "Ваши встречи и переписки", go("Добавить", "Plus", "folder-add", "primary"))}<div class="msg-folder-toolbar">${tabs(["Все", "Встречи", "Чаты"])}<label class="msg-search">${D.icon("Search", 18)}<input placeholder="Найти в папке" aria-label="Поиск в папке" /></label></div>${
          ctx.state === "loading"
            ? D.skeleton(4)
            : ctx.state === "error"
              ? error()
              : ctx.state === "empty"
                ? D.empty(
                    "В этой папке пока ничего нет",
                    "Добавьте существующую встречу или переписку.",
                    "Folder",
                    go("Добавить", "Plus", "folder-add", "primary"),
                  )
                : `<div class="msg-folder-section"><h2>Встречи <span>2</span></h2>${[
                    [
                      "Планирование продукта",
                      "Сегодня, 14:00 · 6 участников",
                      "Запланирована",
                    ],
                    ["Дизайн-ревью", "Вчера, 11:00 · 4 участника", "Завершена"],
                  ]
                    .map(
                      ([name, meta, status], i) =>
                        `<div class="msg-folder-row"><button class="msg-folder-link" data-go="${i ? "history-detail" : "meetings"}"><span class="msg-folder-symbol blue">${D.icon("Video", 24)}</span><span><strong>${name}</strong><small>${meta}</small></span>${D.badge(status, i ? "neutral" : "info")}</button>${action("", "MoreHorizontal", "Показано меню: добавить в папку / удалить из папки", "icon")}</div>`,
                    )
                    .join("")}<h2>Чаты <span>3</span></h2>${people
                    .slice(0, 3)
                    .map(
                      (p) =>
                        `<div class="msg-folder-row"><button class="msg-folder-link" data-go="${p.id}">${D.avatar(p.name, p.tone)}<span><strong>${p.name}</strong><small>${p.group ? "Группа · 6 участников" : "Личная переписка"}</small></span></button>${action("", "MoreHorizontal", "Показано меню ссылок на чат в папке", "icon")}</div>`,
                    )
                    .join("")}</div>`
        }</section>`,
    },
    {
      ...folderBase,
      id: "folder-create",
      title: "Создание / название папки",
      route: "/folders (modal)",
      states: [states[0], states[3]],
      render: (ctx) =>
        modal(
          "Новая папка",
          `${D.input("Название папки", "Например, Проект Meetrix", "Продуктовая команда")}<p class="msg-field-hint">19 / 50</p>${ctx.state === "error" ? D.notice("Папка с таким названием уже существует. Выберите другое.", "danger") : ""}`,
          go("Отмена", "", "folders") +
            go("Создать папку", "", "folders", "primary"),
          "msg-confirm-dialog",
          "folders",
        ),
    },
    {
      ...folderBase,
      id: "folder-delete",
      title: "Удаление папки",
      route: "/folders (modal)",
      states: [states[0]],
      render: () =>
        modal(
          "Удалить папку?",
          `<div class="msg-danger-symbol">${D.icon("FolderMinus", 26)}</div><h2>Продуктовая команда</h2><p class="msg-confirm-copy">Встречи и чаты сохранятся. Будут удалены только ссылки на них в этой папке.</p>`,
          go("Отмена", "", "folders") +
            go("Удалить папку", "", "folders", "danger"),
          "msg-confirm-dialog",
          "folders",
        ),
    },
    {
      ...folderBase,
      id: "folder-add",
      title: "Добавление элементов в папку",
      route: "/folders/:id (modal)",
      api: [
        "GET /api/v1/folder-items",
        "PUT /api/v1/folders/:id/items/:kind/:itemId",
      ],
      render: (ctx) =>
        modal(
          "Добавить в папку",
          `<p class="muted">Продуктовая команда</p>${tabs(["Все", "Встречи", "Чаты"])}${D.input("Поиск", "Название или собеседник", "", "search")}${ctx.state === "loading" ? D.skeleton(3) : ctx.state === "error" ? error() : ctx.state === "empty" ? D.empty("Ничего не найдено", "Попробуйте другое название.", "Search") : `<div class="msg-person-options">${selectablePerson("Мария Орлова", "teal", "Личная переписка", true)}${selectablePerson("Планирование продукта", "blue", "Встреча · сегодня, 14:00")}${selectablePerson("Команда продукта", "purple", "Группа · 6 участников")}</div>`}`,
          go("Готово", "", "folder-detail", "primary"),
          "",
          "folder-detail",
        ),
    },
    {
      ...base,
      group: "Аккаунт",
      nav: "home",
      id: "settings",
      title: "Настройки · категории",
      route: "/app/settings (modal)",
      states: [states[0]],
      api: ["GET /api/v1/auth/me"],
      permissions: "Аккаунт; гости видят только Аудио, Видео, Оформление.",
      render: () =>
        modal(
          "Настройки аккаунта",
          `<nav class="msg-setting-categories">${settingsSections.map(([id, label, icon]) => `<button data-go="settings-${id}"><span>${D.icon(icon, 23)}</span><strong>${label}</strong>${D.icon("ChevronRight", 19)}</button>`).join("")}</nav>`,
          "",
          "msg-category-dialog",
          "home",
        ),
    },
    ...settingsSections.map(([id, label]) => ({
      ...base,
      group: "Аккаунт",
      nav: "home",
      id: `settings-${id}`,
      title: `Настройки · ${label}`,
      route: "/app/settings (modal)",
      states: [states[0], states[1], states[3]],
      api:
        id === "notifications"
          ? [
              "GET/PUT /api/v1/notifications/preferences",
              "GET /api/v1/integrations/capabilities",
            ]
          : id === "profile"
            ? ["PATCH /api/v1/auth/me"]
            : ["Локальные настройки браузера; backend API не требуется"],
      permissions: ["audio", "video", "appearance"].includes(id)
        ? "Аккаунт или гость встречи"
        : "Зарегистрированная учётная запись",
      sources: [
        "frontend/src/pages/AccountPages.tsx:40",
        "frontend/src/components/DeviceSettings.tsx:318",
        "frontend/src/appearance.tsx:14",
      ],
      notes: [
        "Настройки открываются поверх текущей страницы; активная встреча не размонтируется.",
        "Профиль: только имя, email readonly. Нет Интеграций и Безопасности. Оформление light/dark и 75/90/100/110/125/150/200%, локально для аккаунта.",
        "Каналы уведомлений зависят от capabilities; mock/noop не изображать работающими. Доступ к устройствам зависит от браузера и разрешений.",
      ],
      responsive: {
        desktop:
          "Модальное окно до 960 px, навигация216px, контент с отступом32px и собственной прокруткой.",
        tablet:
          "Модальное окно на доступную ширину, навигация180px; форма одной колонкой.",
        mobile:
          "Полноэкранный диалог: категории → подраздел, кнопка назад и закрыть; зона ввода не менее44px.",
      },
      render: (ctx) => settingsRender(id, ctx),
    })),
    {
      ...base,
      group: "Аккаунт",
      nav: "home",
      id: "notifications",
      title: "Уведомления",
      route: "/notifications",
      api: [
        "GET /api/v1/notifications?limit=30&cursor=",
        "POST /api/v1/notifications/:notificationId/read",
        "GET /api/v1/notifications/events",
      ],
      permissions:
        "Только уведомления текущего зарегистрированного пользователя.",
      sources: [
        "frontend/src/pages/NotificationsPage.tsx:17",
        "internal/transport/http/notification_routes.go:14",
      ],
      notes: [
        "Нет массового прочтения/удаления/фильтра unread-only в текущем контракте. Прочитать одно или открыть встречу.",
        "На desktop bell может раскрывать popover с переходом к этому полному списку; mobile сразу открывает страницу.",
        "Личные сообщения имеют отдельный WS toast; не добавлять их в долговечный inbox без backend контракта.",
        "В payload нет названия встречи: под событием статическая подсказка, не вымышленное имя из API.",
      ],
      responsive: {
        desktop:
          "Список до880px; иконка40px, текст и время слева, действие справа.",
        tablet: "Одна колонка; сохранять ясную unread-индикацию.",
        mobile:
          "Иконка32px и гибкий текст, действия под текстом, даты не обрезать; 44px touch.",
      },
      render: (ctx) =>
        `<section class="page page-narrow msg-notification-page">${D.heading("Уведомления", "Приглашения, напоминания и материалы встреч.", ctx.state === "default" ? D.badge("2 новых", "info") : "")}${ctx.state === "loading" ? D.skeleton(4) : ctx.state === "error" ? error("Уведомления недоступны") : ctx.state === "empty" ? `<div class="card msg-spacious-empty">${D.empty("Пока тихо", "Здесь появятся приглашения и новости о ваших встречах.", "Bell")}</div>` : `<div class="msg-notifications">${notificationItems.map(([icon, title, name, time, unread]) => `<article class="msg-notification ${unread ? "is-unread" : ""}"><span class="msg-notification-icon">${D.icon(icon, 22)}</span><div class="msg-notification-copy"><div class="msg-notification-title"><strong>${title}</strong>${unread ? '<span class="msg-unread-dot" aria-label="Непрочитанное"></span>' : ""}</div><p>${name}</p><time>${time}</time><div class="msg-notification-actions">${go("Открыть встречу", "ArrowUpRight", "meetings", "ghost")}${unread ? action("Прочитано", "Check", "Показано прочитанное уведомление", "ghost") : ""}</div></div></article>`).join("")}</div>`}</section>`,
    },
  ]);
})();
