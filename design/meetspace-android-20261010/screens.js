(() => {
  const M = window.MS,
    I = (name) => M.icon(name),
    E = (value) => M.escape(value);
  const avatar = (text, tone = "", size = "") =>
    `<span class="avatar ${tone} ${size}">${text}</span>`;
  const avatars = () =>
    `<div class="avatars">${avatar("АК", "", "sm")}${avatar("МП", "mint", "sm")}${avatar("ДВ", "orange", "sm")}${avatar("+2", "blue", "sm")}</div>`;
  const navItems = [
    ["home", "Home", "Главная"],
    ["calls", "Video", "Звонки"],
    ["messages", "MessagesSquare", "Чаты"],
    ["settings", "UserRound", "Профиль"],
  ];
  const nav = (active) => {
    const selected = ["messages", "thread"].includes(active)
      ? "messages"
      : active === "settings"
        ? "settings"
        : active === "home"
          ? "home"
          : "calls";
    return navItems
      .map(
        ([id, icon, label]) =>
          `<button class="nav-item ${selected === id ? "active" : ""}" data-go="${id}" aria-label="${label}" ${selected === id ? 'aria-current="page"' : ""}>${id === "settings" ? '<span class="nav-profile-avatar" aria-hidden="true">АС<i></i></span>' : I(icon)}<span>${label}</span>${id === "messages" ? '<i class="nav-unread"></i>' : ""}</button>`,
      )
      .join("");
  };
  const status = () =>
    `<div class="statusbar"><span>9:41</span><div class="status-indicators">${I("Signal")}${I("Wifi")}<span class="battery" aria-label="Заряд батареи"></span></div></div>`;
  const shell = (content, active = "home", extra = "") => {
    const screen = M.state.screen;
    const title =
      {
        home: "Главная",
        calls: "Звонки",
        messages: "Чаты",
        thread: "Чаты",
        settings: "Профиль",
      }[screen] ||
      M.screens[screen]?.title ||
      "MeetSpace";
    const search = ["calls", "messages"].includes(screen);
    return `<div class="shell tm-shell screen-${screen} ${screen === "thread" ? "thread-shell" : ""}">${status()}<div class="app-layout"><div class="app-main"><header class="app-header"><div class="tm-heading"><img src="assets/brand-mark.svg" class="tm-desktop-brand" alt="MeetSpace"/><h1>${E(title)}</h1></div><div class="header-actions">${search ? `<button class="icon-button" data-action="screenSearch" aria-label="${screen === "calls" ? "Поиск звонков" : "Поиск чатов"}" aria-expanded="${Boolean(M.state.searchOpen?.[screen])}">${I("Search")}</button>` : screen === "home" ? `<button class="icon-button" data-action="notifications" aria-label="Уведомления">${I("Bell")}</button>` : `<button class="icon-button" data-go="${active === "settings" ? "settings" : "calls"}" aria-label="Назад к ${active === "settings" ? "профилю" : "звонкам"}">${I("ArrowLeft")}</button>`}</div></header><div class="tm-screen-stage"><main class="content ${extra}">${content}</main>${search ? `<button class="tm-fab ${screen === "messages" ? "chat-fab" : ""}" data-action="${screen === "calls" ? "newMeeting" : "newChat"}" aria-label="${screen === "calls" ? "Новый звонок" : "Новый чат"}">${I("Plus")}</button>` : ""}</div><nav class="bottom-nav" aria-label="Главная навигация">${nav(active)}</nav></div></div></div>`;
  };
  M.shell = shell;
  const serverAddress = () =>
    M.state.onboarding?.server || "https://meet.example.org";
  const meetings = [
    {
      time: "10:00",
      end: "10:45",
      title: "Дизайн-синхронизация",
      meta: "Команда продукта · 5 участников",
      tone: "",
      name: "Дизайн-синхронизация",
    },
    {
      time: "13:30",
      end: "14:00",
      title: "Обсуждение новой версии",
      meta: "Разработка · 8 участников",
      tone: "lilac",
      name: "Обсуждение новой версии",
    },
    {
      time: "16:00",
      end: "16:30",
      title: "Планы на неделю",
      meta: "Команда · 4 участника",
      tone: "mint",
      name: "Планы на неделю",
    },
  ];
  const contacts = [
    {
      id: "team",
      kind: "group",
      members: ["self", "maria", "anna", "max", "dmitry"],
      owner: "anna",
      initials: "П",
      name: "Команда продукта",
      preview: "Мария: собрала всё в одном файле",
      time: "09:38",
      tone: "",
      unread: 3,
    },
    {
      id: "anna",
      kind: "direct",
      initials: "АК",
      name: "Анна Кузнецова",
      preview: "Да, до встречи!",
      time: "09:24",
      tone: "orange",
      unread: 1,
    },
    {
      id: "dev",
      kind: "group",
      members: ["self", "dmitry", "anna", "max"],
      owner: "max",
      initials: "Р",
      name: "Разработка",
      preview: "Дмитрий: обновление готово",
      time: "Вчера",
      tone: "blue",
      unread: 0,
    },
    {
      id: "max",
      kind: "direct",
      initials: "МП",
      name: "Максим Петров",
      preview: "Вы: Спасибо, посмотрю сегодня",
      time: "Вчера",
      tone: "mint",
      unread: 0,
    },
    {
      id: "design",
      kind: "group",
      members: ["self", "anna", "maria"],
      owner: "self",
      initials: "Д",
      name: "Дизайн-команда",
      preview: "Анна: новая версия макетов в папке",
      time: "Пт",
      tone: "",
      unread: 0,
    },
  ];
  const memberDirectory = () => [
    {
      id: "self",
      name: M.state.profileName || "Александр Соколов",
      initials: "АС",
      tone: "blue",
    },
    ...contacts.filter((c) => c.kind === "direct"),
    { id: "maria", name: "Мария Полякова", initials: "МП", tone: "mint" },
    { id: "dmitry", name: "Дмитрий Волков", initials: "ДВ", tone: "" },
  ];
  const member = (id) => memberDirectory().find((person) => person.id === id);
  const groupCount = (c) => {
    const n = c.members.length;
    const word =
      n % 100 >= 11 && n % 100 <= 14
        ? "участников"
        : n % 10 === 1
          ? "участник"
          : n % 10 >= 2 && n % 10 <= 4
            ? "участника"
            : "участников";
    return `${n} ${word}`;
  };
  const chatAvatar = (c) =>
    `<span class="chat-avatar ${c.kind === "group" ? "is-group" : ""}">${avatar(E(c.initials), c.tone)}${c.kind === "group" ? `<span class="group-avatar-badge" role="img" aria-label="Групповой чат">${I("Users")}</span>` : ""}</span>`;
  const groupMeta = (c) =>
    c.kind === "group"
      ? `<span class="chat-group-meta">Группа · ${groupCount(c)}</span>`
      : "";
  const meetingRow = (m) =>
    `<button class="agenda-row" data-action="meeting" data-title="${E(m.title)}" data-time="${m.time}" data-date="${m.date || "2026-10-12"}"><span class="agenda-time">${m.time}<small>${m.end}</small></span><span class="agenda-mark ${m.tone}"></span><span class="agenda-detail"><h3>${E(m.title)}</h3><p>${m.meta}</p></span>${I("ChevronRight")}</button>`;
  const homeConversation = (c) =>
    `<button class="home-chat-row" data-action="chat" data-chat="${c.id}" data-kind="${c.kind}" data-search="${E(c.name.toLocaleLowerCase("ru"))}" data-personal="${c.kind === "direct"}" data-unread="${Boolean(c.unread)}">${chatAvatar(c)}<span class="home-chat-copy"><strong>${E(c.name)}</strong>${groupMeta(c)}<span>${E(c.preview)}</span></span><span class="home-chat-meta"><time>${c.time}</time>${c.unread ? `<span class="unread">${c.unread}</span>` : I("CheckCheck")}</span></button>`;
  const homeAction = (action, icon, title, note) =>
    `<button class="home-action-card" data-action="${action}"><span class="home-action-icon">${I(icon)}</span><span class="home-action-title">${title}</span><span class="home-action-note">${note}</span>${I("ArrowUpRight")}</button>`;
  M.screens.home = {
    title: "Главная",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `
      <div class="home-workspace">
        <aside class="home-chats" aria-label="Недавние чаты">
          <div class="home-chat-profile"><button class="home-person" data-go="settings" aria-label="Профиль Александра"><span class="home-online-avatar">${avatar("АС")}<i></i></span><span><strong>Александр</strong><small>В сети</small></span></button><button class="icon-button home-compose" data-action="newChat" aria-label="Новый чат">${I("Plus")}</button></div>
          <div class="home-chat-heading"><h2>Ваши чаты</h2><span>${contacts.length}</span></div>
          <label class="search-box home-chat-search">${I("Search")}<input id="home-chat-search" type="search" placeholder="Найти человека или чат" aria-label="Найти человека или чат" value="${E(M.state.homeQuery || "")}"/></label>
          <div class="home-chat-filters" role="group" aria-label="Фильтры чатов">${[
            ["all", "Все"],
            ["personal", "Личные"],
            ["unread", "Новые"],
          ]
            .map(
              ([id, label]) =>
                `<button data-home-filter="${id}" aria-pressed="${(M.state.homeFilter || "all") === id}">${label}${id === "unread" ? `<span>${contacts.filter((c) => c.unread).length}</span>` : ""}</button>`,
            )
            .join("")}</div>
          <div class="home-chat-list">${contacts.map(homeConversation).join("")}<p class="home-chat-empty" id="home-chat-empty" hidden>Чаты не найдены.<br/>Попробуйте другое имя.</p></div>
          <button class="home-chat-folder" data-action="folders">${I("Folder")}<span>Папки команды</span>${I("ChevronRight")}</button>
        </aside>
        <section class="home-start" aria-labelledby="home-title">
          <div class="home-start-inner">
            <div class="home-welcome"><div class="home-brand-name">Meet<span>Space</span></div><h1 id="home-title">Чаты и видеовстречи</h1><p>Будьте рядом, где бы вы ни находились.</p></div>
            <div class="home-action-grid">
              ${homeAction("newMeeting", "Video", "Новый звонок", "Начните разговор сейчас")}
              ${homeAction("newChat", "MessageCircle", "Новый чат", "Напишите коллеге или команде")}
              ${homeAction("join", "Link", "Подключиться<br/> к звонку", "По ссылке-приглашению")}
              ${homeAction("schedule", "CalendarDays", "Запланировать<br/> встречу", "Выберите удобное время")}
            </div>
            <article class="home-invite"><div class="home-invite-art" aria-hidden="true"><span class="invite-person back">${I("UserRound")}</span><span class="invite-person front">${I("UserRound")}</span><span class="invite-plus">${I("Plus")}</span></div><div class="home-invite-copy"><h2>Встречаться проще вместе</h2><p>Пригласите коллег в MeetSpace по ссылке.</p><button class="button primary" data-action="copyHomeInvite">${I("Copy")}Скопировать ссылку</button></div></article>
          </div>
          <footer class="home-start-footer"><span>${I("ShieldCheck")} Пространство вашей команды</span><button data-go="calendar">Календарь ${I("ArrowUpRight")}</button></footer>
        </section>
      </div>`,
        "home",
        "flush home-content",
      ),
    bind: () => {
      const filterChats = () => {
        const query = (M.state.homeQuery || "").trim().toLocaleLowerCase("ru");
        const filter = M.state.homeFilter || "all";
        let count = 0;
        document.querySelectorAll(".home-chat-row").forEach((row) => {
          const visible =
            row.dataset.search.includes(query) &&
            (filter === "all" ||
              (filter === "personal"
                ? row.dataset.personal === "true"
                : row.dataset.unread === "true"));
          row.hidden = !visible;
          if (visible) count++;
        });
        document.getElementById("home-chat-empty").hidden = count > 0;
      };
      document
        .getElementById("home-chat-search")
        .addEventListener("input", (e) => {
          M.state.homeQuery = e.target.value;
          filterChats();
        });
      document.querySelectorAll("[data-home-filter]").forEach(
        (button) =>
          (button.onclick = () => {
            M.state.homeFilter = button.dataset.homeFilter;
            document
              .querySelectorAll("[data-home-filter]")
              .forEach((item) =>
                item.setAttribute("aria-pressed", String(item === button)),
              );
            filterChats();
          }),
      );
      filterChats();
    },
  };
  const callHistory = [
    {
      id: "review",
      date: "9 октября",
      title: "Продукт. Следующая глава",
      duration: "45 мин",
      time: "14:00",
      tone: "blue",
      record: 0,
    },
    {
      id: "demo",
      date: "8 октября",
      title: "Демо новой версии",
      duration: "1 ч",
      time: "11:00",
      tone: "mint",
      record: 3,
    },
    {
      id: "anna",
      date: "8 октября",
      title: "Анна Кузнецова",
      duration: "12 мин",
      time: "09:30",
      tone: "orange",
    },
    {
      id: "planning",
      date: "7 октября",
      title: "Планы команды",
      duration: "32 мин",
      time: "16:00",
      tone: "",
    },
    {
      id: "design",
      date: "6 октября",
      title: "Обсуждение дизайна",
      duration: "25 мин",
      time: "13:15",
      tone: "blue",
    },
  ];
  const callResults = () => {
    const rows = callHistory.filter((c) =>
      c.title
        .toLocaleLowerCase("ru")
        .includes((M.state.callQuery || "").trim().toLocaleLowerCase("ru")),
    );
    let previous = "";
    return (
      rows
        .map((c) => {
          const heading =
            previous !== c.date ? `<h2 class="call-date">${c.date}</h2>` : "";
          previous = c.date;
          return `${heading}<button class="call-history-row" data-action="historyCall" data-call="${c.id}">${avatar(I("Video"), c.tone)}<span class="call-history-copy"><strong>${E(c.title)}</strong><span>${I("ArrowUpRight")}${c.duration}</span></span><time>${c.time}</time></button>`;
        })
        .join("") ||
      '<div class="tm-empty">Звонки не найдены.<br/>Попробуйте другое название.</div>'
    );
  };
  M.screens.calls = {
    title: "Звонки",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="calls-layout"><section class="calls-start" aria-label="Начать звонок"><div class="call-actions"><button class="call-action call-action-main" data-action="newMeeting">${I("Video")}<span>Новый звонок</span></button><button class="call-action" data-action="join">${I("UserRound")}<span>Подключиться</span></button><button class="call-action" data-action="schedule">${I("CalendarDays")}<span>Запланировать</span></button></div><div class="call-shortcuts"><button data-go="calendar">${I("CalendarDays")}Календарь${I("ChevronRight")}</button><button data-go="recordings">${I("CirclePlay")}Записи${I("ChevronRight")}</button></div>${M.state.created ? `<button class="call-upcoming" data-go="calendar">${I("CalendarDays")}<span><small>Запланировано · ${E(M.state.created.date)} · ${E(M.state.created.time)}</small><strong>${E(M.state.created.title)}</strong></span>${I("ChevronRight")}</button>` : ""}<div class="calls-tablet-note"><img src="assets/brand-mark.svg" alt=""/><h2>Разговор начинается здесь</h2><p>Созвонитесь с командой сейчас<br/>или выберите удобное время.</p><button class="button secondary" data-action="copyHomeInvite">${I("Link")}Пригласить по ссылке</button></div></section><section class="call-history" aria-label="История звонков"><div class="call-history-heading"><h2>Недавние звонки</h2><span>${callHistory.length}</span></div><label class="search-box tm-search" ${M.state.searchOpen?.calls ? "" : "hidden"}>${I("Search")}<input id="call-search" type="search" aria-label="Найти звонок" placeholder="Найти звонок" value="${E(M.state.callQuery || "")}"/></label><div id="call-results">${callResults()}</div></section></div>`,
        "calls",
        "calls-content",
      ),
    bind: () =>
      document.getElementById("call-search")?.addEventListener("input", (e) => {
        M.state.callQuery = e.target.value;
        document.getElementById("call-results").innerHTML = callResults();
      }),
  };
  const conversationList = () =>
    `<aside class="conversation-list"><div class="chat-filters" role="group" aria-label="Фильтры чатов">${[
      ["all", "Все"],
      ["personal", "Личные"],
      ["unread", "Новые"],
    ]
      .map(
        ([id, label]) =>
          `<button data-filter="${id}" aria-pressed="${M.state.filter === id}" class="${M.state.filter === id ? "active" : ""}">${label}${id === "unread" ? `<span>${contacts.filter((c) => c.unread).length}</span>` : ""}</button>`,
      )
      .join(
        "",
      )}<button data-action="folders">Папки ${I("ChevronDown")}</button></div><label class="search-box tm-search" ${M.state.searchOpen?.messages ? "" : "hidden"}>${I("Search")}<input id="chat-search" type="search" placeholder="Найти человека или чат" aria-label="Найти чат" value="${E(M.state.chatQuery || "")}"/></label>${!M.state.inviteDismissed ? `<div class="chat-invite-note"><button data-action="copyHomeInvite"><span><strong>Пригласите свою команду</strong><small>Общайтесь и встречайтесь в MeetSpace</small></span>${I("UserPlus")}</button><button class="dismiss-invite" data-action="dismissInvite" aria-label="Скрыть приглашение">${I("X")}</button></div>` : ""}<div id="conversation-results">${contacts
      .filter((c) =>
        M.state.filter === "personal"
          ? c.kind === "direct"
          : M.state.filter !== "unread" || c.unread,
      )
      .map(
        (c) =>
          `<button class="conversation ${M.state.screen === "thread" && M.state.chat === c.id ? "active" : ""}" data-action="chat" data-chat="${c.id}" data-kind="${c.kind}" data-search="${E(c.name.toLocaleLowerCase("ru"))}">${chatAvatar(c)}<span class="message-text"><h3>${E(c.name)}</h3>${groupMeta(c)}<p>${E(c.preview)}</p></span><span class="conversation-meta">${c.time}${c.unread ? `<span class="unread">${c.unread}</span>` : I("CheckCheck")}</span></button>`,
      )
      .join(
        "",
      )}</div><p id="chat-empty" class="empty-state" hidden>Такого чата пока нет</p></aside>`;
  const thread = () => {
    const c = contacts.find((c) => c.id === M.state.chat) || contacts[0];
    const isGroup = c.kind === "group";
    const firstAuthor = isGroup ? member(c.members[1]) : c;
    const secondAuthor = isGroup ? member(c.members[2] || c.members[1]) : c;
    return `<section class="thread"><header class="thread-header"><button class="icon-button phone-back" data-go="messages" aria-label="Назад к чатам">${I("ArrowLeft")}</button>${chatAvatar(c)}<div><h3>${E(c.name)}</h3><p>${isGroup ? `Группа · ${groupCount(c)}` : "В сети"}</p></div><span class="spacer"></span><button class="icon-button" data-action="chatCall" aria-label="Начать видеовстречу">${I("Video")}</button><button class="icon-button" data-action="chatInfo" aria-label="Информация о чате">${I("Ellipsis")}</button></header>${c.created ? "" : `<div class="thread-banner">${I("Pin")}<span>Встречаемся в 10:00 · Дизайн-синхронизация</span></div>`}<div class="thread-log" id="thread-log">${c.created ? `<div class="chat-date">Вы создали группу</div>${M.state.messages.some((message) => message.chat === c.id) ? "" : `<div class="group-welcome">${chatAvatar(c)}<h3>${E(c.name)}</h3><p>${groupCount(c)} · Всё готово к общению.<br/>Напишите первое сообщение.</p></div>`}` : `<div class="chat-date">Сегодня, 12 октября</div><div class="chat-message">${avatar(E(firstAuthor.initials), firstAuthor.tone)}<div><div class="bubble"><h4>${E(firstAuthor.name)}</h4>Доброе утро! Собрала материалы к встрече. Посмотрите, когда будет минутка.<div class="chat-file">${I("FileText")}<div><strong>Обзор продукта.pdf</strong><span>2,4 МБ · PDF</span></div><button class="icon-button" data-action="attachment" aria-label="Открыть файл">${I("ArrowDown")}</button></div><small>09:32</small></div><button class="chat-reaction" data-action="reaction" aria-label="Отметить сообщение полезным">${I("Check")}<span>${M.state.reacted ? "3" : "2"}</span></button></div></div><div class="chat-message mine"><div class="bubble">Спасибо! Посмотрю перед созвоном. Есть пара идей по навигации.<small>09:34 · ✓✓</small></div></div><div class="chat-message">${avatar(E(secondAuthor.initials), secondAuthor.tone)}<div class="bubble"><h4>${E(secondAuthor.name)}</h4>Отлично, давайте начнём с них. До встречи!<small>09:38</small></div></div>`}${M.state.messages
      .filter((m) => m.chat === c.id)
      .map(
        (m) =>
          `<div class="chat-message mine"><div class="bubble">${E(m.text)}<small>09:41 · ✓✓</small></div></div>`,
      )
      .join(
        "",
      )}</div><form class="composer" id="message-form"><button class="icon-button" type="button" data-action="attach" aria-label="Прикрепить файл">${I("Paperclip")}</button><input id="message-input" autocomplete="off" aria-label="Сообщение" placeholder="Написать сообщение…"/><button class="icon-button send-button" aria-label="Отправить сообщение">${I("ArrowUp")}</button></form></section>`;
  };
  const bindMessages = () => {
    document.querySelectorAll("[data-filter]").forEach(
      (b) =>
        (b.onclick = () => {
          M.state.filter = b.dataset.filter;
          M.render();
        }),
    );
    const filterChatRows = () => {
      const query = (M.state.chatQuery || "").toLocaleLowerCase("ru").trim();
      let count = 0;
      document.querySelectorAll(".conversation").forEach((c) => {
        const show = c.dataset.search.includes(query);
        c.hidden = !show;
        c.classList.toggle("hide", !show);
        if (show) count++;
      });
      document.getElementById("chat-empty").hidden = count > 0;
    };
    document.getElementById("chat-search")?.addEventListener("input", (e) => {
      M.state.chatQuery = e.target.value;
      filterChatRows();
    });
    filterChatRows();
    document.getElementById("message-form")?.addEventListener("submit", (e) => {
      e.preventDefault();
      const input = document.getElementById("message-input");
      const text = input.value.trim();
      if (!text) return;
      M.state.messages.push({ chat: M.state.chat, text });
      const current = contacts.find((c) => c.id === M.state.chat);
      if (current) {
        current.preview = `Вы: ${text}`;
        current.time = "Сейчас";
      }
      M.render();
      document.getElementById("thread-log").scrollTop = 999999;
      document.getElementById("message-input").focus();
    });
  };
  M.screens.messages = {
    title: "Чаты",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="messages-layout">${conversationList()}${thread()}</div>`,
        "messages",
        "flush",
      ),
    bind: bindMessages,
  };
  M.screens.thread = {
    title: "Беседа команды",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="messages-layout show-thread">${conversationList()}${thread()}</div>`,
        "messages",
        "flush",
      ),
    bind: bindMessages,
  };
  M.screens.calendar = {
    title: "Календарь",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="page-title"><div><h1>Календарь</h1><p>Время для важных разговоров.</p></div><button class="icon-button soft" data-action="schedule" aria-label="Запланировать встречу">${I("Plus")}</button></div><div class="calendar-layout"><section><div class="calendar-toolbar"><h2>Октябрь 2026</h2><div class="calendar-controls"><button class="text-action" data-action="today">Сегодня</button><button class="icon-button" data-action="prevWeek" aria-label="Предыдущая неделя">${I("ChevronLeft")}</button><button class="icon-button" data-action="nextWeek" aria-label="Следующая неделя">${I("ChevronRight")}</button></div></div><div class="week-strip">${[
          "Пн",
          "Вт",
          "Ср",
          "Чт",
          "Пт",
          "Сб",
          "Вс",
        ]
          .map((d, i) => {
            const day = 12 + i + (M.state.week || 0);
            return `<button class="day-button ${M.state.day === day ? "active" : ""}" data-day="${day}" aria-pressed="${M.state.day === day}"><small>${d}</small>${day}<i style="opacity:${i < 5 ? 1 : 0}"></i></button>`;
          })
          .join(
            "",
          )}</div><div class="section-heading"><h3>${M.state.day} октября <span class="muted">· ${M.state.day === 12 ? (scheduledForDay() ? "4 встречи" : "3 встречи") : scheduledForDay() ? "1 встреча" : "Нет встреч"}</span></h3></div><div class="timeline">${[
          "09:00",
          "10:00",
          "11:00",
          "12:00",
          "13:00",
          "14:00",
          "15:00",
          "16:00",
          "17:00",
          "18:00",
        ]
          .map(
            (time) =>
              `<div class="time-label">${time}</div><div class="time-slot">${
                M.state.day === 12 && ["10:00", "13:00", "16:00"].includes(time)
                  ? (() => {
                      const m =
                        meetings[["10:00", "13:00", "16:00"].indexOf(time)];
                      return `<button class="event ${m.tone}" data-action="meeting" data-title="${m.title}" data-time="${m.time}" data-date="${m.date || "2026-10-12"}"><h3>${m.title}</h3><p>${m.time} — ${m.end} · ${m.meta.split(" · ")[0]}</p></button>`;
                    })()
                  : ""
              }${scheduledForDay() && scheduledForDay().time.slice(0, 2) === time.slice(0, 2) ? `<button class="event mint" data-action="meeting" data-title="${E(scheduledForDay().title)}" data-time="${scheduledForDay().time}" data-date="${scheduledForDay().date}"><h3>${E(scheduledForDay().title)}</h3><p>${scheduledForDay().time} · Вы — организатор</p></button>` : ""}</div>`,
          )
          .join(
            "",
          )}</div></section><aside class="calendar-side card"><div class="eyebrow">На этой неделе</div><h2 style="margin-top:12px">Хороший ритм</h2><p>Все встречи команды в вашем часовом поясе.</p><div class="row"><span class="agenda-mark"></span>Команда продукта</div><div class="row"><span class="agenda-mark lilac"></span>Разработка</div><div class="row"><span class="agenda-mark mint"></span>Личные встречи</div><div class="team-note">${I("Clock")}Москва · UTC+3</div></aside></div>`,
        "calendar",
      ),
    bind: () =>
      document.querySelectorAll("[data-day]").forEach(
        (b) =>
          (b.onclick = () => {
            M.state.day = Number(b.dataset.day);
            M.render();
          }),
      ),
  };
  const scheduledForDay = () =>
    M.state.created && Number(M.state.created.date.slice(-2)) === M.state.day
      ? M.state.created
      : null;
  const records = [
    {
      title: "Продукт. Следующая глава",
      date: "9 октября",
      duration: "42:18",
      tag: "Команда продукта",
      tone: "",
      cover: "product",
      size: "128 МБ",
    },
    {
      title: "Дизайн-ревью мобильного приложения",
      date: "8 октября",
      duration: "31:06",
      tag: "Дизайн-команда",
      tone: "lilac",
      cover: "design",
      size: "96 МБ",
    },
    {
      title: "Планы на октябрь",
      date: "5 октября",
      duration: "28:42",
      tag: "Команда",
      tone: "mint",
      cover: "plan",
      size: "87 МБ",
    },
    {
      title: "Демо новой версии",
      date: "2 октября",
      duration: "54:12",
      tag: "Разработка",
      tone: "dark",
      cover: "demo",
      size: "168 МБ",
    },
  ];
  const cover = (r) =>
    `<div class="recording-cover ${r.tone}"><div class="slide-mini"><small>${r.cover === "design" ? "DESIGN REVIEW" : "MEETSPACE / TEAM"}</small><strong>${r.cover === "design" ? "Меньше шума.<br/>Больше смысла." : r.cover === "plan" ? "Октябрь.<br/>Всё по плану." : r.cover === "demo" ? "Сделано<br/>вместе." : "Продукт.<br/>Следующая глава."}</strong><div class="slide-lines"><i></i><i></i><i></i></div></div><span class="play-overlay">${I("Play")}</span><span class="duration">${r.duration}</span></div>`;
  M.screens.recordings = {
    title: "Записи",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="page-title"><div><h1>Записи</h1><p>К важному разговору можно вернуться.</p></div><span class="badge desktop-only">4 записи</span></div><label class="search-box recordings-search">${I("Search")}<input id="record-search" type="search" aria-label="Найти запись" placeholder="Найти запись встречи"/></label><div class="chips"><button class="chip active" data-record-filter="all">Все записи</button><button class="chip" data-record-filter="team">Команда продукта</button></div><div class="recording-grid">${records.map((r, i) => `<button class="recording-card" data-action="record" data-record="${i}" data-title="${r.title.toLowerCase()}" data-team="${r.tag}">${cover(r)}<div class="recording-meta"><h3>${r.title}</h3><p>${r.date} · ${r.tag}</p></div></button>`).join("")}</div><div id="record-empty" class="empty-state" hidden>Записи не найдены</div>`,
        "recordings",
      ),
    bind: () => {
      let filter = "all";
      const apply = () => {
        const q = document
          .getElementById("record-search")
          .value.toLowerCase()
          .trim();
        let n = 0;
        document.querySelectorAll(".recording-card").forEach((c) => {
          const show =
            c.dataset.title.includes(q) &&
            (filter === "all" || c.dataset.team === "Команда продукта");
          c.classList.toggle("hide", !show);
          if (show) n++;
        });
        document.getElementById("record-empty").hidden = n > 0;
      };
      document.getElementById("record-search").oninput = apply;
      document.querySelectorAll("[data-record-filter]").forEach(
        (b) =>
          (b.onclick = () => {
            filter = b.dataset.recordFilter;
            document
              .querySelectorAll("[data-record-filter]")
              .forEach((x) => x.classList.toggle("active", x === b));
            apply();
          }),
      );
    },
  };
  const presentation = () =>
    `<div class="presentation"><div class="pres-top"><span>MEETSPACE / PRODUCT</span><span>2026</span></div><h2>В одной<br/>команде.<br/>На одной волне.</h2><p>Следующая глава нашего продукта</p><div class="pres-circles"></div><div class="pres-footer">ОКТЯБРЬ — ДЕКАБРЬ / 01</div></div>`;
  M.screens.recording = {
    title: "Просмотр записи",
    group: "Рабочее пространство",
    render: () => {
      const r = records[M.state.record || 0];
      return shell(
        `<div class="recording-player"><button class="text-action row" data-go="recordings">${I("ArrowLeft")} Все записи</button><div class="page-title"><div><h1>${r.title}</h1><p>${r.date} 2026 · ${r.tag}</p></div></div><div class="player-stage">${presentation()}<button class="play-overlay" data-action="play" aria-label="Запустить демо проигрывателя">${I(M.state.playing ? "Pause" : "Play")}</button></div><div class="player-controls"><button class="icon-button" data-action="play" aria-label="Воспроизвести или приостановить">${I(M.state.playing ? "Pause" : "Play")}</button><span id="play-time">00:00</span><input id="play-seek" type="range" min="0" max="100" value="0" aria-label="Позиция в записи"/><span>${r.duration}</span></div><p class="muted" style="font-size:11px">Демонстрация интерфейса плеера · без видеофайла</p><div class="detail-list"><div><span>Организатор</span><strong>Александр Соколов</strong></div><div><span>Размер записи</span><strong>${r.size} · MP4</strong></div><div><span>Доступ</span><strong>Участники встречи</strong></div></div><div class="row" style="margin-top:20px;flex-wrap:wrap"><button class="button primary" data-action="download">${I("Download")} Скачать</button><button class="button secondary" data-action="recordLink">${I("Link")} Поделиться</button></div></div>`,
        "recordings",
      );
    },
    bind: () =>
      (document.getElementById("play-seek").oninput = (e) => {
        document.getElementById("play-time").textContent = `${String(
          Math.floor(
            (Number(e.target.value) *
              records[M.state.record || 0].duration
                .split(":")
                .map(Number)
                .reduce((m, v) => m * 60 + v, 0)) /
              100 /
              60,
          ),
        ).padStart(2, "0")}:${String(
          Math.floor(
            (Number(e.target.value) *
              records[M.state.record || 0].duration
                .split(":")
                .map(Number)
                .reduce((m, v) => m * 60 + v, 0)) /
              100,
          ) % 60,
        ).padStart(2, "0")}`;
      }),
  };
  M.screens.prejoin = {
    title: "Перед встречей",
    group: "Встреча",
    render: () =>
      shell(
        `<div class="prejoin-layout"><div class="camera-preview">${avatar("АС", "blue")}<p>${M.state.camera ? "Предпросмотр камеры в макете" : "Камера выключена"}</p><div class="camera-controls"><button class="icon-button ${M.state.mic ? "" : "off"}" data-action="mic" aria-label="${M.state.mic ? "Выключить" : "Включить"} микрофон" aria-pressed="${M.state.mic}">${I(M.state.mic ? "Mic" : "MicOff")}</button><button class="icon-button ${M.state.camera ? "" : "off"}" data-action="camera" aria-label="${M.state.camera ? "Выключить" : "Включить"} камеру" aria-pressed="${M.state.camera}">${I(M.state.camera ? "Video" : "VideoOff")}</button></div></div><div class="prejoin-info"><span class="badge blue">${I("Video")} Ваша следующая встреча</span><h1>${E(M.state.meetingTitle || "Дизайн-синхронизация")}</h1><p>12 октября · 10:00 — 10:45<br/>Организатор: Анна Кузнецова</p><label class="field">Ваше имя<input id="participant-name" value="${E(M.state.participantName || "Александр Соколов")}" autocomplete="name"/></label><div class="device-line">${I("Mic")}Встроенный микрофон <span class="spacer"></span><button class="icon-button" data-action="devices" aria-label="Настроить устройства">${I("Settings")}</button></div><button class="button primary" data-action="enterCall">Присоединиться ${I("ArrowRight")}</button><button class="button quiet" data-go="calls">Вернуться к звонкам</button><p style="font-size:10px;text-align:center;margin-top:14px">Демоэкраны · камера и микрофон устройства не используются</p></div></div>`,
        "calls",
      ),
  };
  const callControl = (label, icon, action, cls = "") =>
    `<button class="call-control ${cls}" data-action="${action}" aria-label="${label}"><span class="control-disc">${I(icon)}</span><span>${label}</span></button>`;
  M.screens.call = {
    title: "Видеовстреча",
    group: "Встреча",
    render: () =>
      `<main class="call-screen">${status()}<header class="call-header"><div><h1>${E(M.state.meetingTitle || "Дизайн-синхронизация")}</h1><p>12:48 <span style="color:#60718c">/</span> 5 участников</p></div><span class="spacer"></span><span class="badge"><i class="record-dot"></i> Идёт запись</span><button class="icon-button" data-action="callInfo" aria-label="Информация о встрече">${I("Ellipsis")}</button></header><div class="call-body"><div class="call-stage"><section class="share-stage">${presentation()}<div class="sharing-label">${I("Monitor")} Анна Кузнецова демонстрирует экран</div></section><div class="participant-strip"><div class="participant speaking">${avatar("АК", "orange")}<span class="person-name">Анна Кузнецова</span>${I("AudioLines")}</div><div class="participant">${avatar("МП", "mint")}<span class="person-name">Мария Полякова</span>${I("MicOff")}</div><div class="participant">${avatar("ДВ")}<span class="person-name">Дмитрий Волков</span>${I("MicOff")}</div><div class="participant">${avatar("АС", "blue")}<span class="person-name">Вы${M.state.camera ? " · камера в демо" : ""}</span>${I(M.state.mic ? "Mic" : "MicOff")}</div></div></div><aside class="call-side" ${M.state.callChat ? "" : "hidden"}><div class="row"><h2>Чат встречи</h2><span class="spacer"></span><button class="icon-button" data-action="callChat" aria-label="Закрыть чат">${I("X")}</button></div><div class="chat-date">Сегодня</div><div class="chat-message"><div class="bubble"><h4>Мария Полякова</h4>Хорошо видно, можем начинать!<small>10:02</small></div></div><div class="chat-message"><div class="bubble"><h4>Дмитрий Волков</h4>Давайте обсудим сценарий первого входа.<small>10:08</small></div></div>${(M.state.callMessages || []).map((text) => `<div class="chat-message"><div class="bubble">${E(text)}<small>Вы · сейчас</small></div></div>`).join("")}<form id="call-message-form"><label><span class="muted" style="font-size:11px">Сообщение участникам</span><input placeholder="Написать…" aria-label="Сообщение участникам"/><button class="button primary wide" style="margin-top:10px">Отправить</button></label></form></aside></div><footer class="call-toolbar">${callControl(M.state.mic ? "Микрофон" : "Без звука", M.state.mic ? "Mic" : "MicOff", "mic", M.state.mic ? "" : "off")}${callControl("Камера", M.state.camera ? "Video" : "VideoOff", "camera", M.state.camera ? "" : "off")}${callControl("Экран", "MonitorUp", "shareScreen", "share-control")}${callControl("Участники", "Users", "participants")}${callControl("Чат", "MessageCircle", "callChat", M.state.callChat ? "selected" : "")}${callControl("Выйти", "PhoneOff", "leaveCall", "leave")}</footer></main>`,
    bind: () => {
      document
        .getElementById("call-message-form")
        ?.addEventListener("submit", (e) => {
          e.preventDefault();
          const input = e.target.querySelector("input");
          if (!input.value.trim()) return;
          const msg = document.createElement("div");
          msg.className = "chat-message";
          msg.innerHTML = `<div class="bubble">${E(input.value)}<small>Вы · сейчас</small></div>`;
          e.target.before(msg);
          (M.state.callMessages || (M.state.callMessages = [])).push(
            input.value,
          );
          input.value = "";
        });
    },
  };
  const profileRow = (icon, label, action, value = "") =>
    `<button class="profile-option" data-action="${action}">${I(icon)}<span>${label}</span>${value ? `<small>${E(value)}</small>` : ""}${I("ChevronRight")}</button>`;
  M.screens.settings = {
    title: "Профиль",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="profile-layout"><section class="profile-hero"><button class="profile-identity" data-action="editProfile" aria-label="Редактировать профиль"><span class="profile-portrait">${avatar("АС", "blue")}<i class="presence-dot ${M.state.presence === "Не беспокоить" ? "busy" : M.state.presence === "Нет на месте" ? "away" : ""}"></i></span><h1>${E(M.state.profileName || "Александр Соколов")}${I("ChevronRight")}</h1><span>${E(M.state.profileEmail || "alex@example.org")}</span></button><button class="profile-presence" data-action="presence">${E(M.state.presence || "В сети")}${I("ChevronDown")}</button><button class="profile-server-summary" data-action="servers">${I("Server")}<span><strong>Пространство команды</strong><small>${E(new URL(serverAddress()).host)}</small></span>${I("ChevronRight")}</button></section><div class="profile-groups"><section class="profile-group">${profileRow("Share2", "Поделиться контактом", "shareContact")}</section><section class="profile-group"><label class="profile-option">${I("Bell")}<span>Уведомления</span><input id="profile-notifications" class="toggle" type="checkbox" aria-label="Уведомления" ${M.state.notificationsEnabled !== false ? "checked" : ""}/></label>${profileRow("Folder", "Папки с чатами", "folders")}${profileRow("SunMoon", "Тема оформления", "appearance", M.state.themeMode === "system" ? "Системная" : M.state.theme === "dark" ? "Тёмная" : "Светлая")}${profileRow("Video", "Настройки звонков", "devices")}${profileRow("Server", "Подключение к серверу", "servers")}${profileRow("LockKeyhole", "Конфиденциальность", "privacy")}</section><section class="profile-group">${profileRow("LogOut", "Выйти из аккаунта", "logout")}</section><p class="profile-app-note">MeetSpace · Пространство вашей команды</p></div></div>`,
        "settings",
        "profile-content",
      ),
    bind: () =>
      document
        .getElementById("profile-notifications")
        ?.addEventListener("change", (e) => {
          M.state.notificationsEnabled = e.target.checked;
          M.toast(
            e.target.checked
              ? "Уведомления включены в макете"
              : "Уведомления выключены в макете",
          );
        }),
  };
  const meetingForm = (scheduled) =>
    M.modal(
      scheduled ? "Запланировать встречу" : "Новая встреча",
      `<form id="meeting-form" class="stack"><label class="field">Название<input name="title" placeholder="Например, обсуждение проекта" required maxlength="80"/></label>${scheduled ? '<div class="two-fields"><label class="field">Дата · 5–25 октября<input name="date" type="date" value="2026-10-12" min="2026-10-05" max="2026-10-25" required/></label><label class="field">Время<input name="time" type="time" value="17:00" min="09:00" max="18:00" required/></label></div>' : ""}<label class="field">Описание<textarea placeholder="Что будем обсуждать?"></textarea></label><button class="button primary">${scheduled ? "Запланировать" : "Создать и подключиться"}</button></form>`,
      () =>
        (document.getElementById("meeting-form").onsubmit = (e) => {
          e.preventDefault();
          const data = new FormData(e.target);
          M.state.meetingTitle = data.get("title");
          if (scheduled) {
            const date = data.get("date");
            M.state.created = {
              date,
              title: data.get("title"),
              time: data.get("time"),
              end: "30 мин",
              meta: `${date === "2026-10-12" ? "Сегодня" : date} · Вы — организатор`,
              tone: "mint",
            };
            M.go(M.state.screen === "calls" ? "calls" : "home");
            M.toast("Встреча добавлена в демонстрационный список");
          } else M.go("prejoin");
        }),
    );
  M.actions = {
    newMeeting: () => meetingForm(false),
    schedule: () => meetingForm(true),
    join: () =>
      M.modal(
        "Войти по ссылке",
        `<form id="join-form" class="stack"><p>Вставьте ссылку, которую прислал организатор встречи.</p><label class="field">Ссылка на встречу<input name="url" type="url" placeholder="https://meet.example.org/…" required/></label><button class="button primary">Продолжить ${I("ArrowRight")}</button></form>`,
        () =>
          (document.getElementById("join-form").onsubmit = (e) => {
            e.preventDefault();
            const url = new FormData(e.target).get("url");
            if (!String(url).startsWith("https://")) {
              M.toast("Нужна ссылка с HTTPS");
              return;
            }
            M.state.meetingTitle = "Встреча по приглашению";
            M.go("prejoin");
          }),
      ),
    meeting: (b) =>
      M.modal(
        E(b.dataset.title),
        `<div class="stack"><span class="badge blue">${I("CalendarDays")}${E(b.dataset.date || "2026-10-12")} · ${E(b.dataset.time)}</span><p>Команда продукта обсуждает планы и новые идеи. Подключитесь перед началом, чтобы проверить звук.</p><div class="row">${avatars()}<span class="muted">5 участников</span></div><button class="button primary" id="meeting-join">${I("Video")} Подключиться</button><button class="button secondary" data-action="copyInvite">${I("Link")} Пригласить по ссылке</button></div>`,
        () =>
          (document.getElementById("meeting-join").onclick = () => {
            M.state.meetingTitle = b.dataset.title;
            M.go("prejoin");
          }),
      ),
    copyHomeInvite: async () => {
      const link = `${serverAddress().replace(/\/$/, "")}/room/design`;
      try {
        await navigator.clipboard.writeText(link);
        M.toast("Демо-ссылка на встречу скопирована");
      } catch {
        M.modal(
          "Ссылка-приглашение",
          `<div class="stack"><p>Скопируйте демонстрационную ссылку на встречу.</p><label class="field">Ссылка<input id="home-invite-link" value="${E(link)}" readonly/></label><button class="button secondary" data-close>Готово</button></div>`,
          () => document.getElementById("home-invite-link").select(),
        );
      }
    },
    copyInvite: () => M.toast("Демо-ссылка: meet.example.org/room/design"),
    notifications: () =>
      M.modal(
        "Уведомления",
        `<div class="notification-item">${avatar(I("Video"), "blue")}<div><h3>Скоро начнём</h3><p>Дизайн-синхронизация в 10:00</p><small>Через 19 минут</small></div></div><div class="notification-item">${avatar(I("MessageCircle"))}<div><h3>Вас ждут в команде</h3><p>Мария добавила файл в чат продукта</p><small>3 минуты назад</small></div></div><button class="button quiet wide" data-go="messages">Открыть чаты</button>`,
      ),
    folders: () =>
      M.modal(
        "Команда продукта",
        `<div class="stack"><p>Встречи и беседы, собранные в одной папке.</p>${meetings.map(meetingRow).join("")}<button class="button secondary" data-action="chat" data-chat="team">${I("MessageCircle")}Команда продукта</button></div>`,
      ),
    chat: (b) => {
      M.state.chat = b.dataset.chat;
      M.go("thread");
    },
    chatInfo: () => {
      const c = contacts.find((c) => c.id === M.state.chat) || contacts[0];
      const isGroup = c.kind === "group";
      M.modal(
        isGroup ? "О группе" : "О контакте",
        `<div class="stack"><div class="group-info-heading">${chatAvatar(c)}<div><h3>${E(c.name)}</h3><p>${isGroup ? `Группа · ${groupCount(c)}` : "Личная переписка"}</p></div></div>${
          isGroup
            ? `<h3 class="group-members-title">Участники</h3><div class="group-info-members">${c.members
                .map((id) => {
                  const person = member(id);
                  return `<div class="group-info-member">${avatar(E(person.initials), person.tone)}<div><strong>${E(person.name)}</strong><small>${id === "self" ? "Вы" : ""}${id === c.owner ? `${id === "self" ? " · " : ""}Администратор` : ""}</small></div></div>`;
                })
                .join("")}</div>`
            : ""
        }<button class="button secondary" data-action="chatCall">${I("Video")}${isGroup ? "Позвонить группе" : "Позвонить"}</button></div>`,
      );
    },
    chatCall: () => {
      const c = contacts.find((c) => c.id === M.state.chat) || contacts[0];
      M.state.meetingTitle =
        c.kind === "group" ? `Звонок группы «${c.name}»` : c.name;
      M.state.meetingMeta = null;
      M.go("prejoin");
    },
    newChat: () =>
      M.modal(
        "Новая беседа",
        `<div class="stack"><button class="button secondary create-group-choice" data-action="newGroup">${I("Users")}Создать группу</button><p>Или начните личную переписку.</p>${contacts
          .filter((c) => c.kind === "direct")
          .map(
            (c) =>
              `<button class="home-message" data-action="chat" data-chat="${c.id}">${avatar(E(c.initials), c.tone)}${E(c.name)}${I("ArrowRight")}</button>`,
          )
          .join("")}</div>`,
      ),
    newGroup: () =>
      M.modal(
        "Создать группу",
        `<form id="group-form" class="stack"><label class="field">Название группы<input name="name" required placeholder="Например, Команда проекта" maxlength="50"/></label><fieldset class="group-member-picker"><legend>Участники</legend>${contacts
          .filter((c) => c.kind === "direct")
          .map(
            (c) =>
              `<label class="group-member-option">${avatar(E(c.initials), c.tone)}<span>${E(c.name)}</span><input type="checkbox" name="members" value="${c.id}" checked aria-label="Добавить ${E(c.name)}"/></label>`,
          )
          .join(
            "",
          )}</fieldset><p class="group-create-note">Вы будете администратором группы. Она появится во вкладке «Все».</p><p id="group-members-error" class="field-error" role="alert" hidden>Выберите хотя бы одного участника.</p><button class="button primary">Создать группу</button></form>`,
        () => {
          document.getElementById("group-form").onsubmit = (e) => {
            e.preventDefault();
            const data = new FormData(e.target),
              name = String(data.get("name")).trim();
            const input = e.target.elements.name;
            input.setCustomValidity(name ? "" : "Введите название группы");
            if (!input.reportValidity()) return;
            const selected = data
              .getAll("members")
              .filter((id) =>
                contacts.some((c) => c.id === id && c.kind === "direct"),
              );
            document.getElementById("group-members-error").hidden =
              selected.length > 0;
            if (!selected.length) return;
            const c = {
              id: "group" + contacts.length,
              kind: "group",
              members: ["self", ...selected],
              owner: "self",
              created: true,
              initials: name.slice(0, 1).toLocaleUpperCase("ru"),
              name,
              preview: "Вы создали группу",
              time: "Сейчас",
              tone: "blue",
              unread: 0,
            };
            contacts.unshift(c);
            M.state.chat = c.id;
            M.state.filter = "all";
            M.state.chatQuery = "";
            M.state.homeFilter = "all";
            M.state.homeQuery = "";
            M.go("thread");
          };
          document.querySelector('#group-form input[name="name"]').oninput = (
            e,
          ) => e.target.setCustomValidity("");
          document
            .querySelectorAll('#group-form input[name="members"]')
            .forEach(
              (input) =>
                (input.onchange = () => {
                  if (
                    document.querySelector(
                      '#group-form input[name="members"]:checked',
                    )
                  )
                    document.getElementById("group-members-error").hidden =
                      true;
                }),
            );
        },
      ),
    reaction: () => {
      M.state.reacted = !M.state.reacted;
      M.render();
    },
    attachment: () =>
      M.modal(
        "Обзор продукта.pdf",
        `<div class="stack"><span class="badge blue">PDF · 2,4 МБ</span><h3>Материалы к дизайн-синхронизации</h3><p>Обсуждаем навигацию, первый запуск и встречу на планшете.</p><p>Демонстрационная карточка вложения. Реальный файл к макету не подключён.</p><button class="button secondary" data-close>Закрыть</button></div>`,
      ),
    attach: () =>
      M.modal(
        "Добавить вложение",
        `<div class="stack"><p>В макете можно выбрать файл: на сервер он не отправляется.</p><label class="field">Файл<input id="attachment-file" type="file"/></label><div id="attachment-summary" class="muted"></div><button class="button primary" id="attach-confirm" disabled>Добавить в беседу</button></div>`,
        () => {
          let filename = "";
          document.getElementById("attachment-file").onchange = (e) => {
            filename = e.target.files[0]?.name || "";
            document.getElementById("attachment-summary").textContent =
              filename;
            document.getElementById("attach-confirm").disabled = !filename;
          };
          document.getElementById("attach-confirm").onclick = () => {
            M.state.messages.push({
              chat: M.state.chat,
              text: `Вложение в макете: ${filename}`,
            });
            M.go("thread");
          };
        },
      ),
    today: () => {
      M.state.week = 0;
      M.state.day = 12;
      M.render();
    },
    prevWeek: () => {
      M.state.week = Math.max(-7, (M.state.week || 0) - 7);
      M.state.day = 12 + M.state.week;
      M.render();
    },
    nextWeek: () => {
      M.state.week = Math.min(7, (M.state.week || 0) + 7);
      M.state.day = 12 + M.state.week;
      M.render();
    },
    record: (b) => {
      M.state.record = Number(b.dataset.record);
      M.state.playing = false;
      M.go("recording");
    },
    play: () => {
      M.state.playing = !M.state.playing;
      document
        .querySelectorAll("[data-action=play]")
        .forEach((b) => (b.innerHTML = I(M.state.playing ? "Pause" : "Play")));
      M.toast(
        M.state.playing
          ? "Демо воспроизведения. Позицию можно менять на шкале."
          : "Воспроизведение приостановлено",
      );
    },
    download: () =>
      M.modal(
        "Скачать запись",
        `<div class="stack"><p>В приложении здесь сохраняется MP4 на устройство. В дизайн-макете используется демонстрационная обложка без видеофайла.</p><button class="button secondary" data-close>Понятно</button></div>`,
      ),
    recordLink: () =>
      M.modal(
        "Поделиться записью",
        `<div class="stack"><p>Доступ получат участники встречи.</p><label class="field">Демонстрационная ссылка<input value="https://meet.example.org/recordings/demo" readonly/></label><button class="button secondary" data-close>Готово</button></div>`,
      ),
    mic: () => {
      M.state.mic = !M.state.mic;
      M.render();
    },
    camera: () => {
      M.state.camera = !M.state.camera;
      M.render();
    },
    devices: () =>
      M.modal(
        "Устройства",
        `<div class="stack"><label class="field">Микрофон<select><option>Встроенный микрофон</option><option>Гарнитура (пример)</option></select></label><label class="field">Камера<select><option>Фронтальная камера</option><option>Основная камера</option></select></label><p>Выбор устройств показан для демонстрации интерфейса.</p><button class="button primary" data-close>Готово</button></div>`,
      ),
    enterCall: () => {
      const name = document.getElementById("participant-name").value.trim();
      if (!name) {
        M.toast("Введите ваше имя");
        return;
      }
      M.state.participantName = name;
      M.go("call");
    },
    callChat: () => {
      M.state.callChat = !M.state.callChat;
      M.render();
    },
    participants: () =>
      M.modal(
        "Участники · 5",
        `<div class="stack">${[
          ["АК", "Анна Кузнецова", "Организатор", "orange"],
          ["МП", "Мария Полякова", "Участник", "mint"],
          ["ДВ", "Дмитрий Волков", "Участник", ""],
          ["МП", "Максим Петров", "Участник", "mint"],
          ["АС", "Вы", "Участник", "blue"],
        ]
          .map(
            ([a, n, r, t]) =>
              `<div class="row">${avatar(a, t)}<div><h3>${n}</h3><p>${r}</p></div><span class="spacer"></span>${I("MicOff")}</div>`,
          )
          .join("")}</div>`,
      ),
    shareScreen: () =>
      M.modal(
        "Демонстрация экрана",
        `<div class="stack"><p>В Android-приложении системный диалог запросит разрешение на показ экрана. Этот макет демонстрирует экран Анны.</p><button class="button primary" data-close>Понятно</button></div>`,
      ),
    callInfo: () =>
      M.modal(
        "О встрече",
        `<div class="stack"><p>Дизайн-синхронизация · 5 участников</p><p>В макете показано состояние «Идёт запись». В реальной встрече запись запускает организатор, а участники видят уведомление.</p><button class="button secondary" data-action="copyInvite">${I("Link")}Ссылка приглашения</button></div>`,
      ),
    leaveCall: () =>
      M.modal(
        "Выйти из встречи?",
        `<div class="stack"><p>Остальные участники смогут продолжить разговор.</p><button class="button danger" data-go="calls">Выйти из встречи</button><button class="button secondary" data-close>Остаться</button></div>`,
      ),
    servers: () =>
      M.modal(
        "Ваши серверы",
        `<div class="stack"><div class="row">${avatar(I("Server"), "blue")}<div><h3>Команда продукта</h3><p>${E(new URL(serverAddress()).host)}</p></div><span class="badge green">Выбран</span></div><button class="button secondary" data-go="connect">${I("Plus")}Добавить сервер</button><button class="button quiet" data-go="welcome">Настроить своё пространство</button></div>`,
      ),
    logout: () =>
      M.modal(
        "Выйти из аккаунта?",
        `<div class="stack"><p>Сервер команды останется в списке подключений.</p><button class="button danger" data-go="login">Выйти</button><button class="button secondary" data-close>Остаться</button></div>`,
      ),
  };

  Object.assign(M.actions, {
    screenSearch: () => {
      const screen = M.state.screen;
      M.state.searchOpen ||= {};
      M.state.searchOpen[screen] = !M.state.searchOpen[screen];
      if (!M.state.searchOpen[screen])
        M.state[screen === "calls" ? "callQuery" : "chatQuery"] = "";
      M.render();
      if (M.state.searchOpen[screen])
        document
          .getElementById(screen === "calls" ? "call-search" : "chat-search")
          ?.focus();
    },
    dismissInvite: () => {
      M.state.inviteDismissed = true;
      M.render();
    },
    historyCall: (button) => {
      const call = callHistory.find((c) => c.id === button.dataset.call);
      if (!call) return;
      M.modal(
        E(call.title),
        `<div class="stack"><p>${call.date} · ${call.time} · ${call.duration}</p><button class="button primary" id="repeat-call">${I("Video")}Позвонить снова</button>${call.record !== undefined ? '<button class="button secondary" id="history-record">Открыть запись</button>' : ""}</div>`,
        () => {
          document.getElementById("repeat-call").onclick = () => {
            M.state.meetingTitle = call.title;
            M.state.meetingMeta = null;
            M.go("prejoin");
          };
          document
            .getElementById("history-record")
            ?.addEventListener("click", () => {
              M.state.record = call.record;
              M.go("recording");
            });
        },
      );
    },
    editProfile: () =>
      M.modal(
        "Редактировать профиль",
        `<form id="profile-form" class="stack"><label class="field">Имя и фамилия<input name="name" value="${E(M.state.profileName || "Александр Соколов")}" maxlength="60" required/></label><label class="field">Электронная почта<input type="email" name="email" value="${E(M.state.profileEmail || "alex@example.org")}" required/></label><button class="button primary">Сохранить изменения</button></form>`,
        () => {
          document.getElementById("profile-form").onsubmit = (e) => {
            e.preventDefault();
            const data = new FormData(e.target);
            if (!String(data.get("name")).trim()) return;
            M.state.profileName = String(data.get("name")).trim();
            M.state.profileEmail = data.get("email");
            M.closeModal();
            M.render();
            M.toast("Изменения сохранены в макете");
          };
        },
      ),
    presence: () =>
      M.modal(
        "Ваш статус",
        `<div class="stack">${["В сети", "Не беспокоить", "Нет на месте"].map((label) => `<button class="button secondary" data-action="setPresence" data-presence="${label}" aria-pressed="${(M.state.presence || "В сети") === label}">${label}${(M.state.presence || "В сети") === label ? I("Check") : ""}</button>`).join("")}</div>`,
      ),
    setPresence: (b) => {
      M.state.presence = b.dataset.presence;
      M.closeModal();
      M.render();
    },
    appearance: () =>
      M.modal(
        "Тема оформления",
        `<div class="stack">${[
          ["light", "Светлая"],
          ["dark", "Тёмная"],
          ["system", "Системная"],
        ]
          .map(
            ([id, label]) =>
              `<button class="button secondary" data-action="setTheme" data-theme-value="${id}" aria-pressed="${(M.state.themeMode === "system" ? "system" : M.state.theme) === id}">${I(id === "light" ? "Sun" : id === "dark" ? "Moon" : "Monitor")}${label}${(M.state.themeMode === "system" ? "system" : M.state.theme) === id ? I("Check") : ""}</button>`,
          )
          .join("")}</div>`,
      ),
    setTheme: (b) => {
      M.state.themeMode = b.dataset.themeValue;
      M.state.theme =
        M.state.themeMode === "system"
          ? matchMedia("(prefers-color-scheme: dark)").matches
            ? "dark"
            : "light"
          : M.state.themeMode;
      M.closeModal();
      M.render();
    },
    shareContact: () =>
      M.modal(
        "Поделиться контактом",
        `<div class="stack"><h3>${E(M.state.profileName || "Александр Соколов")}</h3><label class="field">Электронная почта<input id="contact-email" value="${E(M.state.profileEmail || "alex@example.org")}" readonly/></label><button class="button primary" data-action="copyContact">${I("Copy")}Скопировать почту</button></div>`,
      ),
    copyContact: async () => {
      try {
        await navigator.clipboard.writeText(
          M.state.profileEmail || "alex@example.org",
        );
        M.toast("Почта скопирована");
      } catch {
        document.getElementById("contact-email")?.select();
        M.toast("Выделите и скопируйте адрес");
      }
    },
    privacy: () =>
      M.modal(
        "Конфиденциальность",
        `<div class="stack"><p>Встречи, сообщения и записи находятся на сервере вашей команды.</p><div class="server-card"><strong>Сервер</strong><p>${E(serverAddress())}</p></div><p>Камерой и микрофоном можно управлять перед подключением и во время звонка.</p><button class="button secondary" data-action="devices">Настройки звонков</button></div>`,
      ),
  });
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (e) => {
    if (M.state.themeMode === "system") {
      M.state.theme = e.matches ? "dark" : "light";
      M.render();
    }
  });
})();
