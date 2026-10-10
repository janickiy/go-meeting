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
    ["meetings", "Video", "Встречи"],
    ["messages", "MessageCircle", "Личные"],
    ["calendar", "CalendarDays", "Календарь"],
    ["recordings", "CirclePlay", "Записи"],
    ["settings", "Settings", "Настройки"],
  ];
  const nav = (active, compact = false) => {
    const items = compact
      ? [...navItems.slice(0, 4), ["more", "Ellipsis", "Ещё"]]
      : navItems;
    return items
      .map(
        ([id, icon, label]) =>
          `<button class="nav-item ${active === id || (id === "more" && ["settings", "recordings"].includes(active)) ? "active" : ""}" ${id === "more" ? 'data-action="more"' : `data-go="${id}"`} ${active === id ? 'aria-current="page"' : ""}>${I(icon)}<span>${label}</span>${id === "messages" ? '<i class="nav-unread"></i>' : ""}</button>`,
      )
      .join("");
  };
  const status = () =>
    `<div class="statusbar"><span>9:41</span><div class="status-indicators">${I("Signal")}${I("Wifi")}<span class="battery" aria-label="Заряд батареи"></span></div></div>`;
  const shell = (content, active = "home", extra = "") =>
    `<div class="shell ${M.state.screen === "thread" ? "thread-shell" : M.state.screen === "home" ? "home-shell" : ""}">${status()}<div class="app-layout"><aside class="rail"><div class="rail-logo"><img class="rail-brand" src="assets/brand-mark.svg" alt=""/><strong>Meet<span>Space</span></strong></div><p class="rail-tagline">Встречи, чаты и совместная работа<br/><b>в одном месте.</b></p><div class="rail-section-label">Рабочее пространство</div><nav aria-label="Главная навигация">${nav(active)}<button class="nav-item folders-nav" data-action="folders">${I("Folder")}<span>Папки</span></button></nav><button class="profile-nav icon-button" data-go="settings" aria-label="Мой профиль">${avatar("АС", "blue")}<span class="rail-profile-name">Александр<small>alex@example.org</small></span></button></aside><div class="app-main"><header class="app-header"><div class="workspace-label"><img class="workspace-icon" src="assets/brand-mark.svg" alt=""/><div><h2 class="mobile-wordmark">MeetSpace</h2><h2 class="desktop-section-title">${M.screens[M.state.screen]?.title || "MeetSpace"}</h2><div class="workspace-sub"><i class="dot"></i> Пространство команды</div></div></div><div class="header-actions"><button class="icon-button bell" data-action="notifications" aria-label="Уведомления">${I("Bell")}</button><button class="header-profile" data-go="settings" aria-label="Мой профиль">${avatar("АС", "blue")}<span>Александр</span></button></div></header><main class="content ${extra}">${content}</main><nav class="bottom-nav" aria-label="Главная навигация">${nav(active, true)}</nav></div></div></div>`;
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
      initials: "П",
      name: "Команда продукта",
      preview: "Мария: собрала всё в одном файле",
      time: "09:38",
      tone: "",
      unread: 3,
    },
    {
      id: "anna",
      initials: "АК",
      name: "Анна Кузнецова",
      preview: "Да, до встречи!",
      time: "09:24",
      tone: "orange",
      unread: 1,
    },
    {
      id: "dev",
      initials: "Р",
      name: "Разработка",
      preview: "Дмитрий: обновление готово",
      time: "Вчера",
      tone: "blue",
      unread: 0,
    },
    {
      id: "max",
      initials: "МП",
      name: "Максим Петров",
      preview: "Вы: Спасибо, посмотрю сегодня",
      time: "Вчера",
      tone: "mint",
      unread: 0,
    },
    {
      id: "design",
      initials: "Д",
      name: "Дизайн-команда",
      preview: "Новая версия макетов в папке",
      time: "Пт",
      tone: "",
      unread: 0,
    },
  ];
  const meetingRow = (m) =>
    `<button class="agenda-row" data-action="meeting" data-title="${E(m.title)}" data-time="${m.time}" data-date="${m.date || "2026-10-12"}"><span class="agenda-time">${m.time}<small>${m.end}</small></span><span class="agenda-mark ${m.tone}"></span><span class="agenda-detail"><h3>${E(m.title)}</h3><p>${m.meta}</p></span>${I("ChevronRight")}</button>`;
  const homeConversation = (c) =>
    `<button class="home-chat-row" data-action="chat" data-chat="${c.id}" data-search="${E(c.name.toLocaleLowerCase("ru"))}" data-personal="${["anna", "max"].includes(c.id)}" data-unread="${Boolean(c.unread)}">${avatar(E(c.initials), c.tone)}<span class="home-chat-copy"><strong>${E(c.name)}</strong><span>${E(c.preview)}</span></span><span class="home-chat-meta"><time>${c.time}</time>${c.unread ? `<span class="unread">${c.unread}</span>` : I("CheckCheck")}</span></button>`;
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
  const conversationList = () =>
    `<aside class="conversation-list"><div class="page-title"><h1>Чаты</h1><button class="icon-button soft" data-action="newChat" aria-label="Новая беседа">${I("SquarePen")}</button></div><label class="search-box">${I("Search")}<input id="chat-search" type="search" placeholder="Найти человека или чат" aria-label="Поиск чатов"/></label><div class="chips"><button class="chip ${M.state.filter === "all" ? "active" : ""}" data-filter="all">Все <small>5</small></button><button class="chip ${M.state.filter === "unread" ? "active" : ""}" data-filter="unread">Непрочитанные <small>2</small></button></div><div class="eyebrow">Закреплённые</div><div id="conversation-results">${contacts
      .filter((c) => M.state.filter !== "unread" || c.unread)
      .map(
        (c) =>
          `<button class="conversation ${M.state.chat === c.id ? "active" : ""}" data-action="chat" data-chat="${c.id}" data-search="${E(c.name.toLowerCase())}">${avatar(E(c.initials), c.tone)}<span class="message-text"><h3>${E(c.name)}</h3><p>${E(c.preview)}</p></span><span class="conversation-meta">${c.time}${c.unread ? `<span class="unread">${c.unread}</span>` : I("CheckCheck")}</span></button>`,
      )
      .join(
        "",
      )}</div><p id="chat-empty" class="empty-state" hidden>Такого чата пока нет</p></aside>`;
  const thread = () => {
    const c = contacts.find((c) => c.id === M.state.chat) || contacts[0];
    const isGroup =
      ["team", "dev", "design"].includes(c.id) || c.id.startsWith("group");
    return `<section class="thread"><header class="thread-header"><button class="icon-button phone-back" data-go="messages" aria-label="Назад к чатам">${I("ArrowLeft")}</button>${avatar(E(c.initials), c.tone)}<div><h3>${E(c.name)}</h3><p>${isGroup ? "5 участников · 3 в сети" : "В сети"}</p></div><span class="spacer"></span><button class="icon-button" data-go="prejoin" aria-label="Начать видеовстречу">${I("Video")}</button><button class="icon-button" data-action="chatInfo" aria-label="Информация о чате">${I("Ellipsis")}</button></header><div class="thread-banner">${I("Pin")}<span>Встречаемся в 10:00 · Дизайн-синхронизация</span></div><div class="thread-log" id="thread-log"><div class="chat-date">Сегодня, 12 октября</div><div class="chat-message">${avatar(isGroup ? "МП" : E(c.initials), "mint")}<div><div class="bubble"><h4>${isGroup ? "Мария Полякова" : E(c.name)}</h4>Доброе утро! Собрала материалы к встрече. Посмотрите, когда будет минутка.<div class="chat-file">${I("FileText")}<div><strong>Обзор продукта.pdf</strong><span>2,4 МБ · PDF</span></div><button class="icon-button" data-action="attachment" aria-label="Открыть файл">${I("ArrowDown")}</button></div><small>09:32</small></div><button class="chat-reaction" data-action="reaction" aria-label="Отметить сообщение полезным">${I("Check")}<span>${M.state.reacted ? "3" : "2"}</span></button></div></div><div class="chat-message mine"><div class="bubble">Спасибо! Посмотрю перед созвоном. Есть пара идей по навигации.<small>09:34 · ✓✓</small></div></div><div class="chat-message">${avatar("АК", "orange")}<div class="bubble"><h4>${isGroup ? "Анна Кузнецова" : E(c.name)}</h4>Отлично, давайте начнём с них. До встречи!<small>09:38</small></div></div>${M.state.messages
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
    document.getElementById("chat-search")?.addEventListener("input", (e) => {
      const query = e.target.value.toLowerCase().trim();
      let count = 0;
      document.querySelectorAll(".conversation").forEach((c) => {
        const show = c.dataset.search.includes(query);
        c.hidden = !show;
        c.classList.toggle("hide", !show);
        if (show) count++;
      });
      document.getElementById("chat-empty").hidden = count > 0;
    });
    document.getElementById("message-form")?.addEventListener("submit", (e) => {
      e.preventDefault();
      const input = document.getElementById("message-input");
      const text = input.value.trim();
      if (!text) return;
      M.state.messages.push({ chat: M.state.chat, text });
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
        `<div class="prejoin-layout"><div class="camera-preview">${avatar("АС", "blue")}<p>${M.state.camera ? "Предпросмотр камеры в макете" : "Камера выключена"}</p><div class="camera-controls"><button class="icon-button ${M.state.mic ? "" : "off"}" data-action="mic" aria-label="${M.state.mic ? "Выключить" : "Включить"} микрофон" aria-pressed="${M.state.mic}">${I(M.state.mic ? "Mic" : "MicOff")}</button><button class="icon-button ${M.state.camera ? "" : "off"}" data-action="camera" aria-label="${M.state.camera ? "Выключить" : "Включить"} камеру" aria-pressed="${M.state.camera}">${I(M.state.camera ? "Video" : "VideoOff")}</button></div></div><div class="prejoin-info"><span class="badge blue">${I("Video")} Ваша следующая встреча</span><h1>${E(M.state.meetingTitle || "Дизайн-синхронизация")}</h1><p>${Number((M.state.meetingMeta?.date || "2026-10-12").slice(-2))} октября · ${E(M.state.meetingMeta?.time || "10:00")} — ${E(M.state.meetingMeta?.end || "10:45")}<br/>Организатор: ${E(M.state.meetingMeta?.owner || "Анна Кузнецова")}</p><label class="field">Ваше имя<input id="participant-name" value="${E(M.state.participantName || "Александр Соколов")}" autocomplete="name"/></label><div class="device-line">${I("Mic")}Встроенный микрофон <span class="spacer"></span><button class="icon-button" data-action="devices" aria-label="Настроить устройства">${I("Settings")}</button></div><button class="button primary" data-action="enterCall">Присоединиться ${I("ArrowRight")}</button><button class="button quiet" data-go="home">Вернуться на главную</button><p style="font-size:10px;text-align:center;margin-top:14px">Демоэкраны · камера и микрофон устройства не используются</p></div></div>`,
        "home",
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
  const settingsTabs = [
    ["profile", "User", "Профиль"],
    ["devices", "Mic", "Устройства"],
    ["notifications", "Bell", "Уведомления"],
    ["appearance", "Sun", "Оформление"],
  ];
  const settingToggle = (title, desc, checked = true) =>
    `<label class="setting-row"><span><h3>${title}</h3><p>${desc}</p></span><input class="toggle" type="checkbox" ${checked ? "checked" : ""} aria-label="${title}"/></label>`;
  const settingsContent = () => {
    if (M.state.tab === "appearance")
      return `<h2>Оформление</h2><p class="muted" style="margin-top:8px;font-size:12px">Подберите комфортную тему.</p><div class="setting-row"><span><h3>Тёмная тема</h3><p>Применяется ко всему пространству</p></span><input id="theme-toggle" type="checkbox" class="toggle" ${M.state.theme === "dark" ? "checked" : ""} aria-label="Тёмная тема"/></div><div class="setting-row"><span><h3>Размер текста</h3><p>Масштаб в прототипе</p></span><select id="text-size" aria-label="Размер текста"><option value="100">100%</option><option value="115">115%</option><option value="130">130%</option></select></div>`;
    if (M.state.tab === "notifications")
      return `<h2>Уведомления</h2>${settingToggle("Новые сообщения", "Личные и групповые беседы")}${settingToggle("Напоминания о встречах", "За 10 минут до начала")}${settingToggle("Звуки уведомлений", "Когда приложение открыто", false)}<p class="muted" style="margin-top:18px;font-size:11px">Параметры демонстрируются в текущем макете.</p>`;
    if (M.state.tab === "devices")
      return `<h2>Аудио и видео</h2><div class="stack" style="margin-top:24px"><label class="field">Микрофон<select><option>Встроенный микрофон</option><option>Гарнитура Bluetooth (пример)</option></select></label><label class="field">Динамики<select><option>Динамики устройства</option><option>Гарнитура Bluetooth (пример)</option></select></label>${settingToggle("Входить с выключенной камерой", "Можно включить перед встречей")}<button class="button secondary" data-go="prejoin">Открыть предпросмотр</button></div>`;
    return `<div class="profile-block">${avatar("АС", "blue", "lg")}<div><h2>${E(M.state.profileName || "Александр Соколов")}</h2><p>Участник команды продукта</p></div></div><form class="stack" id="profile-form"><label class="field">Имя и фамилия<input name="name" value="${E(M.state.profileName || "Александр Соколов")}" required/></label><label class="field">Электронная почта<input type="email" name="email" value="${E(M.state.profileEmail || "alex@example.org")}" required/></label><button class="button primary">Сохранить изменения</button></form><div class="server-card"><div class="row">${I("Server")}<h3>Сервер команды</h3><span class="spacer"></span><i class="dot" style="color:var(--green)"></i></div><p>${E(serverAddress())}</p><button class="text-action" data-action="servers">Управлять подключением ${I("ArrowRight")}</button></div><button class="text-action" data-action="logout" style="color:var(--danger);margin-top:12px">Выйти из аккаунта</button>`;
  };
  M.screens.settings = {
    title: "Профиль и настройки",
    group: "Рабочее пространство",
    render: () =>
      shell(
        `<div class="page-title"><div><h1>Ваш профиль</h1><p>Пространство, в котором удобно вам.</p></div></div><div class="settings-layout"><nav class="settings-menu card" aria-label="Разделы настроек">${settingsTabs.map(([id, icon, name]) => `<button data-settings-tab="${id}" class="${M.state.tab === id ? "active" : ""}">${I(icon)}${name}</button>`).join("")}</nav><section class="settings-panel card">${settingsContent()}</section></div>`,
        "settings",
      ),
    bind: () => {
      document.querySelectorAll("[data-settings-tab]").forEach(
        (b) =>
          (b.onclick = () => {
            M.state.tab = b.dataset.settingsTab;
            M.render();
          }),
      );
      document
        .getElementById("profile-form")
        ?.addEventListener("submit", (e) => {
          e.preventDefault();
          M.state.profileName = new FormData(e.target).get("name");
          M.state.profileEmail = new FormData(e.target).get("email");
          M.render();
          M.toast("Изменения сохранены в макете");
        });
      document
        .getElementById("theme-toggle")
        ?.addEventListener("change", (e) => {
          M.state.theme = e.target.checked ? "dark" : "light";
          M.render();
        });
      document.getElementById("text-size")?.addEventListener("change", (e) => {
        document.querySelector(".settings-panel").style.fontSize =
          `${e.target.value}%`;
        document
          .querySelectorAll(
            ".settings-panel h2,.settings-panel h3,.settings-panel p",
          )
          .forEach(
            (el) =>
              (el.style.fontSize =
                (Number(e.target.value) / 100) *
                  (el.tagName === "H2" ? 19 : el.tagName === "H3" ? 13 : 12) +
                "px"),
          );
      });
    },
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
            M.go("home");
            M.toast("Встреча добавлена в демонстрационный список");
          } else M.go("prejoin");
        }),
    );
  M.actions = {
    more: () =>
      M.modal(
        "Личное пространство",
        `<div class="stack"><button class="button secondary" data-go="recordings">${I("CirclePlay")}Записи встреч</button><button class="button secondary" data-action="folders">${I("Folder")}Папки</button><button class="button secondary" data-go="settings">${I("Settings")}Профиль и настройки</button><button class="button quiet" data-go="welcome">${I("Server")}Подключение к серверу</button></div>`,
      ),
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
    chatInfo: () =>
      M.modal(
        "О беседе",
        `<div class="stack"><p>Команда продукта · 5 участников</p>${contacts
          .slice(1, 4)
          .map(
            (c) =>
              `<div class="row">${avatar(E(c.initials), c.tone)}<span>${E(c.name)}</span></div>`,
          )
          .join(
            "",
          )}<button class="button secondary" data-go="prejoin">${I("Video")}Начать встречу</button></div>`,
      ),
    newChat: () =>
      M.modal(
        "Новая беседа",
        `<div class="stack"><p>Выберите коллегу для начала разговора.</p>${contacts
          .filter((c) => ["anna", "max"].includes(c.id))
          .map(
            (c) =>
              `<button class="home-message" data-action="chat" data-chat="${c.id}">${avatar(E(c.initials), c.tone)}${E(c.name)}${I("ArrowRight")}</button>`,
          )
          .join(
            "",
          )}<button class="button secondary" data-action="newGroup">${I("Users")}Создать группу</button></div>`,
      ),
    newGroup: () =>
      M.modal(
        "Создать группу",
        `<form id="group-form" class="stack"><label class="field">Название группы<input name="name" required placeholder="Команда проекта" maxlength="50"/></label><p>В демогруппу войдут Анна и Максим.</p><button class="button primary">Создать группу</button></form>`,
        () =>
          (document.getElementById("group-form").onsubmit = (e) => {
            e.preventDefault();
            const name = new FormData(e.target).get("name");
            const c = {
              id: "group" + contacts.length,
              initials: name.slice(0, 1),
              name,
              preview: "Группа создана",
              time: "09:41",
              tone: "blue",
              unread: 0,
            };
            contacts.push(c);
            M.state.chat = c.id;
            M.go("thread");
          }),
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
        `<div class="stack"><p>Остальные участники смогут продолжить разговор.</p><button class="button danger" data-go="home">Выйти из встречи</button><button class="button secondary" data-close>Остаться</button></div>`,
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
})();
