/* Meeting fixtures and local filters for the web-inspired second concept. */
(() => {
  const M = window.MS,
    I = (name) => M.icon(name),
    E = (value) => M.escape(value);
  const state = () =>
    M.state.meetings ||
    (M.state.meetings = {
      tab: "upcoming",
      query: "",
      participation: "all",
      from: "",
      to: "",
    });
  const fixtures = [
    {
      id: "design",
      title: "Дизайн-синхронизация",
      date: "2026-10-12",
      time: "10:00",
      end: "10:45",
      team: "Команда продукта",
      owner: "Анна Кузнецова",
      mine: false,
      people: 5,
      status: "upcoming",
      next: true,
    },
    {
      id: "release",
      title: "Обсуждение новой версии",
      date: "2026-10-12",
      time: "13:30",
      end: "14:00",
      team: "Разработка",
      owner: "Вы",
      mine: true,
      people: 8,
      status: "upcoming",
    },
    {
      id: "weekly",
      title: "Планы на неделю",
      date: "2026-10-13",
      time: "11:00",
      end: "11:30",
      team: "Команда продукта",
      owner: "Вы",
      mine: true,
      people: 4,
      status: "upcoming",
    },
    {
      id: "active",
      title: "Рабочая комната команды",
      date: "2026-10-12",
      time: "09:30",
      end: "10:30",
      team: "Разработка",
      owner: "Максим Петров",
      mine: false,
      people: 3,
      status: "active",
    },
    {
      id: "review",
      title: "Продукт. Следующая глава",
      date: "2026-10-09",
      time: "14:00",
      end: "14:45",
      team: "Команда продукта",
      owner: "Вы",
      mine: true,
      people: 6,
      status: "past",
    },
    {
      id: "demo",
      title: "Демо новой версии",
      date: "2026-10-08",
      time: "11:00",
      end: "12:00",
      team: "Разработка",
      owner: "Анна Кузнецова",
      mine: false,
      people: 8,
      status: "past",
    },
  ];
  const matched = () => {
    const all = [...fixtures];
    if (M.state.created)
      all.push({
        ...M.state.created,
        id: "created",
        owner: "Вы",
        mine: true,
        people: 1,
        team: "Личная встреча",
        status: "upcoming",
      });
    const s = state();
    return all.filter(
      (m) =>
        m.status === s.tab &&
        m.title.toLowerCase().includes(s.query.toLowerCase()) &&
        (s.participation !== "mine" || m.mine) &&
        (!s.from || m.date >= s.from) &&
        (!s.to || m.date <= s.to),
    );
  };
  const participantIcons = () =>
    '<div class="avatars"><span class="avatar sm">АК</span><span class="avatar mint sm">МП</span><span class="avatar orange sm">ДВ</span><span class="avatar blue sm">+2</span></div>';
  const entry = (m) =>
    `<article class="meeting-entry ${m.next ? "next" : ""}"><div class="meeting-date">${Number(m.date.slice(-2))}<span>окт</span></div><div class="meeting-entry-main"><span class="badge ${m.status === "active" ? "live" : ""}">${m.status === "active" ? "● Встреча в эфире" : m.status === "past" ? "Завершена" : m.next ? "Через 19 минут" : m.date === "2026-10-12" ? "Сегодня" : "Завтра"}</span><h2>${E(m.title)}</h2><p>${I("Clock")}${E(m.time)} — ${E(m.end)} <span>·</span> ${m.people} участников</p><div class="meeting-owner">${I("User")} ${E(m.owner)} <span>·</span> ${E(m.team)}</div></div><div class="meeting-entry-side">${participantIcons()}<button class="button ${m.next || m.status === "active" ? "primary" : "secondary"}" data-meeting-open="${m.id}">${I(m.status === "past" ? "CirclePlay" : "Video")}${m.status === "past" ? "Запись" : m.next || m.status === "active" ? "Подключиться" : "Открыть"}</button><button class="icon-button" data-meeting-info="${m.id}" aria-label="Подробнее о встрече ${E(m.title)}">${I("Ellipsis")}</button></div></article>`;
  const empty = () =>
    `<div class="meetings-empty"><div class="meetings-empty-icon">${I("CalendarDays")}</div><h2>Здесь пока нет встреч</h2><p>Измените условия поиска или создайте новую встречу и пригласите команду.</p><button class="button quiet" data-action="newMeeting">${I("Plus")} Создать конференцию</button><button class="text-action" id="reset-meeting-filters">Сбросить фильтры</button></div>`;
  const filterFields = () =>
    `<label class="field">Моё участие<select name="participation"><option value="all" ${state().participation === "all" ? "selected" : ""}>Все мои встречи</option><option value="mine" ${state().participation === "mine" ? "selected" : ""}>Я организатор</option></select></label><label class="field">С даты<input name="from" type="date" value="${E(state().from)}" aria-label="С даты"/></label><label class="field">По дату<input name="to" type="date" value="${E(state().to)}" aria-label="По дату"/></label>`;
  const reset = () => {
    Object.assign(state(), {
      query: "",
      participation: "all",
      from: "",
      to: "",
    });
    M.render();
  };
  function updateResults() {
    document.getElementById("meetings-results").innerHTML =
      matched().map(entry).join("") || empty();
    document.getElementById("meeting-result-count").textContent =
      `Найдено встреч: ${matched().length}`;
    document
      .getElementById("reset-meeting-filters")
      ?.addEventListener("click", reset);
  }
  function openMeeting(id, info = false) {
    const m = matched().find((m) => m.id === id);
    if (!m) return;
    if (m.status === "past" && !info) {
      M.state.record = m.id === "review" ? 0 : 3;
      M.go("recording");
      return;
    }
    M.state.meetingTitle = m.title;
    M.state.meetingMeta = m;
    if (!info) {
      M.go("prejoin");
      return;
    }
    M.modal(
      E(m.title),
      `<div class="stack"><span class="badge blue">${E(m.date)} · ${E(m.time)} — ${E(m.end)}</span><p>Организатор: ${E(m.owner)}<br/>${E(m.team)} · ${m.people} участников</p><button class="button primary" data-go="prejoin">${I("Video")}Подключиться</button><button class="button secondary" data-action="copyInvite">${I("Link")}Пригласить по ссылке</button></div>`,
    );
  }
  M.screens.meetings = {
    title: "Встречи",
    group: "Рабочее пространство",
    render: () =>
      M.shell(
        `<div class="meetings-page"><div class="meetings-heading"><div><h1>Встречи</h1><p>Ваши разговоры, планы<br class="phone-only"/> и общие результаты.</p></div><button class="button primary" data-action="newMeeting" aria-label="Новая встреча">${I("Plus")}<span>Новая встреча</span></button></div><div class="meetings-tools"><div class="meeting-tabs" role="tablist" aria-label="Статус встреч">${[
          ["upcoming", "Предстоящие"],
          ["active", "Активные"],
          ["past", "Завершённые"],
        ]
          .map(
            ([id, label]) =>
              `<button class="meeting-tab" id="meeting-tab-${id}" role="tab" aria-controls="meetings-results" aria-selected="${state().tab === id}" data-meeting-tab="${id}">${label}</button>`,
          )
          .join(
            "",
          )}</div><label class="search-box">${I("Search")}<input id="meeting-search" type="search" placeholder="Найти встречу" aria-label="Найти встречу" value="${E(state().query)}"/></label></div><div class="meetings-filters">${filterFields()}</div><div class="meetings-mobile-filter"><button id="open-meeting-filters">${I("SlidersHorizontal")} ${state().participation === "mine" ? "Я организатор" : "Все мои встречи"} ${I("ChevronDown")}</button><button id="open-date-filters">${I("CalendarDays")} ${state().from || state().to ? "Даты выбраны" : "Период"}</button></div><div class="meetings-meta"><span id="meeting-result-count">Найдено встреч: ${matched().length}</span><span class="row">${I("Clock")}Москва · UTC+3</span></div><div class="meetings-entries" id="meetings-results" role="tabpanel" aria-labelledby="meeting-tab-${state().tab}" aria-live="polite">${matched().map(entry).join("") || empty()}</div><div class="meetings-footnote">${I("ShieldCheck")}Ваши встречи остаются на сервере команды.</div></div>`,
        "meetings",
      ),
    bind: () => {
      document.querySelectorAll("[data-meeting-tab]").forEach(
        (b) =>
          (b.onclick = () => {
            state().tab = b.dataset.meetingTab;
            M.render();
          }),
      );
      document.getElementById("meeting-search").oninput = (e) => {
        state().query = e.target.value;
        updateResults();
      };
      document
        .querySelectorAll(".meetings-filters input,.meetings-filters select")
        .forEach(
          (input) =>
            (input.onchange = () => {
              state()[input.name] = input.value;
              updateResults();
            }),
        );
      document.getElementById("meetings-results").onclick = (e) => {
        const btn = e.target.closest("button");
        if (btn?.dataset.meetingOpen) openMeeting(btn.dataset.meetingOpen);
        if (btn?.dataset.meetingInfo)
          openMeeting(btn.dataset.meetingInfo, true);
      };
      const openFilters = () =>
        M.modal(
          "Фильтры встреч",
          `<form id="meeting-filter-form" class="stack">${filterFields()}<button class="button primary">Применить</button><button type="button" id="reset-modal-filters" class="button quiet">Сбросить</button></form>`,
          () => {
            document.getElementById("meeting-filter-form").onsubmit = (e) => {
              e.preventDefault();
              Object.assign(
                state(),
                Object.fromEntries(new FormData(e.target)),
              );
              M.closeModal();
              M.render();
            };
            document.getElementById("reset-modal-filters").onclick = () => {
              M.closeModal();
              reset();
            };
          },
        );
      document.getElementById("open-meeting-filters").onclick = openFilters;
      document.getElementById("open-date-filters").onclick = openFilters;
      document
        .getElementById("reset-meeting-filters")
        ?.addEventListener("click", reset);
    },
  };
})();
