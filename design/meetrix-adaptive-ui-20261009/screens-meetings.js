/* Автономные дизайн-макеты. Данные вымышлены; приложение и API не вызываются. */
(() => {
  const D = window.MeetrixDesign;
  const esc = D.esc;
  const icon = (name, size = 20) => D.icon(name, size);
  const button = (label, name, attrs, kind = "primary") =>
    D.button(label, name || "", attrs || "", kind);
  const state = (id, label) => ({ id, label });
  const statusStates = [
    state("default", "Обычное"),
    state("loading", "Загрузка"),
    state("empty", "Пусто"),
    state("error", "Ошибка"),
  ];
  const brand = (target = "home") =>
    `<a class="mt-brand" data-go="${target}" href="#${target}" aria-label="Meetrix — главная"><img src="assets/brand-mark.svg" alt="">Meetrix</a>`;
  const label = (
    title,
    value = "",
    placeholder = "",
    type = "text",
    extra = "",
  ) =>
    `<label class="field mt-field"><span>${esc(title)}</span><input type="${type}" value="${esc(value)}" placeholder="${esc(placeholder)}" ${extra}></label>`;
  const select = (title, values) =>
    `<label class="field mt-field"><span>${esc(title)}</span><select>${values.map((value) => `<option>${esc(value)}</option>`).join("")}</select></label>`;
  const alert = (message, tone = "danger") =>
    `<div class="mt-inline-notice mt-inline-notice-${tone}" role="${tone === "danger" ? "alert" : "status"}">${icon(tone === "danger" ? "CircleAlert" : "Info", 18)}<span>${message}</span></div>`;
  const sectionHead = (title, link = "", target = "", goState = "default") =>
    `<div class="mt-section-head"><h2>${title}</h2>${link ? `<a href="#${target}" data-go="${target}" data-go-state="${goState}">${link}${icon("ArrowUpRight", 16)}</a>` : ""}</div>`;
  const empty = (title, description, name = "CalendarDays", action = "") =>
    `<div class="mt-empty"><span>${icon(name, 28)}</span><h3>${title}</h3><p>${description}</p>${action}</div>`;
  const retry = (text = "Не удалось загрузить встречи") =>
    `<div class="mt-error-card">${alert(`${text}. Проверьте соединение и попробуйте ещё раз.`)}${button("Повторить", "RotateCw", 'data-state="default"', "secondary")}</div>`;
  const upcoming = [
    {
      title: "Дизайн продукта",
      time: "10:00",
      day: "Сегодня",
      date: "9 октября",
      duration: "45 мин",
      status: "Запланирована",
      tone: "info",
      owner: true,
    },
    {
      title: "Планы на следующую неделю",
      time: "14:30",
      day: "Сегодня",
      date: "9 октября",
      duration: "30 мин",
      status: "Запланирована",
      tone: "info",
      owner: false,
    },
    {
      title: "Командная встреча",
      time: "11:00",
      day: "Понедельник",
      date: "12 октября",
      duration: "60 мин",
      status: "Запланирована",
      tone: "info",
      owner: true,
    },
    {
      title: "Обсудить идею",
      time: "—",
      day: "Без даты",
      date: "",
      duration: "",
      status: "Создана",
      tone: "neutral",
      owner: false,
    },
  ];
  const active = [
    {
      title: "Рабочая встреча команды",
      time: "09:30",
      day: "Сегодня",
      date: "9 октября",
      duration: "",
      status: "В эфире",
      tone: "success",
      owner: true,
    },
  ];
  const past = [
    {
      title: "Итоги спринта",
      time: "16:00",
      day: "Вчера",
      date: "8 октября",
      duration: "",
      status: "Завершена",
      tone: "neutral",
      owner: true,
    },
    {
      title: "Демо новой версии",
      time: "12:30",
      day: "Среда",
      date: "7 октября",
      duration: "",
      status: "Завершена",
      tone: "neutral",
      owner: false,
    },
    {
      title: "Обсуждение планов",
      time: "10:00",
      day: "Вторник",
      date: "6 октября",
      duration: "",
      status: "Отменена",
      tone: "neutral",
      owner: true,
    },
  ];
  function meetingRow(meeting, index = 0, controls = false) {
    const historic = ["Завершена", "Отменена"].includes(meeting.status);
    const target = historic ? "history-detail" : "conference";
    const menu = `mt-meeting-menu-${index}`;
    return `<article class="mt-meeting-row"><div class="mt-meeting-date"><strong>${meeting.time}</strong><span>${meeting.day}</span></div><div class="mt-meeting-copy"><a data-go="${target}" href="#${target}"><h3>${meeting.title}</h3></a><p>${[meeting.date, meeting.duration, meeting.owner ? "Вы организатор" : "Вы участник"].filter(Boolean).join(" · ")}</p></div><div class="mt-meeting-status">${D.badge(meeting.status, meeting.tone)}</div><div class="mt-meeting-action">${button(historic ? "Материалы" : "Открыть", historic ? "FolderOpen" : "ArrowRight", `data-go="${target}"`, "secondary")}${controls ? `<button class="icon-button" aria-label="Действия со встречей ${meeting.title}" data-menu="${menu}" aria-expanded="false">${icon("EllipsisVertical")}</button><div class="mt-menu dropdown" id="${menu}" hidden><button data-go="folders">${icon("FolderPlus", 18)}Добавить в папку</button><button data-go="${target}">${icon("Info", 18)}Открыть встречу</button></div>` : ""}</div></article>`;
  }
  function meetingTabs(value = "default") {
    return `<div class="tabs mt-tabs" role="group" aria-label="Категория встреч">${[
      ["default", "Предстоящие"],
      ["active", "Активные"],
      ["past", "Завершённые"],
    ]
      .map(
        ([id, text]) =>
          `<button class="tab ${value === id ? "active" : ""}" data-state="${id}" aria-pressed="${value === id}">${text}</button>`,
      )
      .join("")}</div>`;
  }
  function recentCards() {
    return `<div class="mt-recent-grid">${past
      .slice(0, 2)
      .map(
        (meeting, i) =>
          `<a href="#history-detail" data-go="history-detail" class="mt-recent-card"><div class="mt-recent-symbol mt-recent-symbol-${i}">${icon("Video", 24)}</div><div><h3>${meeting.title}</h3><p>${meeting.date} · Завершена</p><span>Открыть материалы ${icon("ArrowUpRight", 15)}</span></div></a>`,
      )
      .join("")}</div>`;
  }
  function modal(title, body, footer, back = "home", caption = "") {
    return `<div class="mt-modal-stage"><div class="mt-modal-underlay" aria-hidden="true"><h1>Ваши встречи</h1><div class="mt-underlay-line"></div><div class="mt-underlay-line"></div><div class="mt-underlay-line short"></div></div><div class="mt-modal-scrim"></div><section class="mt-modal-panel" role="dialog" aria-modal="true" aria-labelledby="mt-dialog-title"><header class="mt-modal-header"><div>${caption ? `<span class="eyebrow">${caption}</span>` : ""}<h1 id="mt-dialog-title">${title}</h1></div><button class="icon-button" aria-label="Закрыть окно" data-go="${back}">${icon("X")}</button></header><div class="mt-modal-body">${body}</div>${footer ? `<footer class="mt-modal-footer">${footer}</footer>` : ""}</section></div>`;
  }
  const commonResponsive = {
    desktop: "1440: sidebar 240, topbar 72, content padding 40, controls ≥44.",
    tablet:
      "834: navigation rail 76, padding 24, перенос toolbar по содержимому.",
    mobile:
      "390: topbar 64, bottom nav 72 + safe-area, padding 16; карточки одной колонкой.",
  };
  const bareResponsive = {
    desktop:
      "Публичный экран без кабинета: brand header, ограниченная ширина содержимого.",
    tablet: "Одна колонка без sidebar, внешние отступы 24.",
    mobile:
      "390: внешние отступы 16, поля 16px/48px, обычная прокрутка при клавиатуре.",
  };
  function auth(register, selectedState) {
    const error = selectedState === "error";
    const pending = selectedState === "loading";
    return `<main class="mt-auth"><header class="mt-auth-header">${brand("landing")}<a data-go="landing" href="#landing" class="mt-back-link">${icon("ArrowLeft", 17)}На главную</a></header><div class="mt-auth-layout"><aside class="mt-auth-story"><span class="eyebrow">БЛИЖЕ К ВАЖНОМУ</span><h1>Место, где<br>идеи становятся<br><em>общими.</em></h1><p>Встречи, разговоры и материалы.<br>Соберите рабочий день в одном месте.</p><div class="mt-auth-mini"><span class="mt-auth-mini-icon">${icon("Video", 22)}</span><div><strong>Следующая встреча — начало нового.</strong><span>Meetrix · Встречи. Идеи. Результаты.</span></div></div><div class="mt-auth-abstract" aria-hidden="true"><span></span><span></span><span></span><span></span></div></aside><section class="mt-auth-card" aria-labelledby="mt-auth-title"><span class="eyebrow">${register ? "НАЧНЁМ ЗНАКОМСТВО" : "С ВОЗВРАЩЕНИЕМ"}</span><h2 id="mt-auth-title">${register ? "Создайте аккаунт" : "Вход в Meetrix"}</h2><p class="mt-auth-description">${register ? "Проводите встречи и продолжайте общение." : "Ваши встречи и разговоры уже здесь."}</p>${error ? alert(register ? "Аккаунт с этим email уже существует. Войдите или укажите другой адрес." : "Не удалось войти. Проверьте email и пароль.") : ""}${selectedState === "expired" ? alert("Сессия завершилась. Войдите, чтобы продолжить.", "info") : ""}<div class="mt-auth-fields">${label("Email", error ? "anna@example.com" : "", "you@example.com", "email", `autocomplete="email" ${pending ? "disabled" : ""}`)}<label class="field mt-field"><span>Пароль</span><div class="mt-password"><input type="password" placeholder="${register ? "Не менее 8 символов" : "Введите пароль"}" autocomplete="${register ? "new-password" : "current-password"}" ${pending ? "disabled" : ""}><button class="icon-button" type="button" aria-label="Показать пароль" data-toast="Показано действие переключения видимости пароля">${icon("Eye", 19)}</button></div>${register ? "<small>8–128 символов. Цифры и спецсимволы необязательны.</small>" : ""}</label>${register ? label("Ваше имя · необязательно", "", "Как к вам обращаться?", "text", `autocomplete="nickname" maxlength="100" ${pending ? "disabled" : ""}`) : `<button class="mt-text-button mt-recovery" data-state="recovery">Забыли пароль?</button>`}${selectedState === "recovery" ? alert("Восстановление пароля пока недоступно. Обратитесь к администратору сервиса.", "info") : ""}${button(pending ? "Подождите…" : register ? "Зарегистрироваться" : "Войти", pending ? "LoaderCircle" : "ArrowRight", pending ? 'disabled aria-busy="true"' : `data-go="${register ? "register-success" : "home"}"`)}<p class="mt-auth-switch">${register ? "Уже есть аккаунт?" : "Нет аккаунта?"} <a data-go="${register ? "login" : "register"}" href="#${register ? "login" : "register"}">${register ? "Войти" : "Создать аккаунт"}</a></p></div>${register ? "" : `<div class="mt-social-placeholders" aria-label="Будущие способы входа"><span>или</span><button disabled aria-describedby="mt-social-note"><b aria-hidden="true">G</b>Google<small>Позже</small></button><button disabled aria-describedby="mt-social-note"><b aria-hidden="true">⊞</b>Microsoft<small>Позже</small></button><p id="mt-social-note">Вход через Google и Microsoft пока недоступен.</p></div>`}<div class="mt-auth-note">${icon("Link", 17)}<p>Чтобы присоединиться по приглашению,<br>создавать аккаунт необязательно.</p></div></section></div><footer class="mt-auth-footer">Meetrix — одно место для вашей команды.</footer></main>`;
  }
  function home(selectedState) {
    const records =
      selectedState === "active"
        ? active
        : selectedState === "past"
          ? past
          : upcoming.slice(0, 3);
    return `<div class="page mt-home"><div class="mt-welcome"><div><span class="eyebrow">ПЯТНИЦА, 9 ОКТЯБРЯ</span><h1>Доброе утро, Анна<span>.</span></h1><p>Встречи, которые сближают. И всё важное после них.</p></div><div class="mt-date-tile"><span>ОКТ</span><strong>09</strong></div></div><div class="mt-quick-actions"><button class="mt-quick-card mt-quick-primary" data-go="create-meeting"><span>${icon("Plus", 23)}</span><div><strong>Новая встреча</strong><small>Начните разговор сейчас</small></div>${icon("ArrowUpRight", 21)}</button><button class="mt-quick-card" data-go="create-meeting" data-go-state="scheduled"><span>${icon("CalendarDays", 23)}</span><div><strong>Запланировать</strong><small>Выберите удобное время</small></div>${icon("ArrowUpRight", 21)}</button><button class="mt-quick-card" data-go="join-link"><span>${icon("Link", 23)}</span><div><strong>По приглашению</strong><small>Введите ссылку на встречу</small></div>${icon("ArrowUpRight", 21)}</button></div><section class="mt-home-section">${sectionHead("Ваши встречи", "Все встречи", "meetings")}${meetingTabs(selectedState)}<div class="mt-meeting-list">${selectedState === "loading" ? D.skeleton(3) : selectedState === "error" ? retry() : selectedState === "empty" ? empty("Время для первой встречи", "Создайте встречу и поделитесь ссылкой с коллегами.", "Video", button("Создать встречу", "Plus", 'data-go="create-meeting"')) : records.map((meeting, i) => meetingRow(meeting, i)).join("")}</div></section><section class="mt-home-section">${sectionHead("Недавние встречи", "Завершённые", "meetings", "past")}${selectedState === "empty" ? empty("Здесь останется важное", "После завершения встречи здесь появятся доступные вам материалы.", "FolderOpen") : selectedState === "loading" ? D.skeleton(2) : recentCards()}</section><div class="mt-home-tip">${icon("Link", 20)}<p><strong>Встречайтесь без лишних шагов.</strong> По ссылке могут присоединиться и коллеги без аккаунта.</p><button data-go="join-link" class="mt-text-button">Открыть приглашение ${icon("ArrowRight", 16)}</button></div></div>`;
  }
  function meetings(selectedState) {
    const selected = ["active", "past"].includes(selectedState)
      ? selectedState
      : "default";
    const list =
      selected === "active" ? active : selected === "past" ? past : upcoming;
    return `<div class="page mt-meetings">${D.heading("Встречи", "От первого приглашения до общих результатов.", button("Новая встреча", "Plus", 'data-go="create-meeting"'))}<div class="mt-meetings-tools">${meetingTabs(selected)}<div class="mt-search-row"><label class="mt-search">${icon("Search", 19)}<input type="search" aria-label="Поиск среди загруженных встреч" placeholder="Найти среди загруженных…"></label><button class="button button-secondary mt-mobile-filter" data-state="filters">${icon("SlidersHorizontal", 18)}Фильтры</button></div></div><div class="mt-meeting-filters ${selectedState === "filters" ? "mt-filters-open" : ""}">${select("Моё участие", ["Все мои встречи", "Я организатор", "Я участник"])}${label("С даты", "", "", "date")}${label("По дату", "", "", "date")}${selectedState === "filters" ? button("Применить", "", 'data-state="default"') : ""}</div><div class="mt-list-caption"><span>${selected === "active" ? "Встречи в эфире" : selected === "past" ? "Завершённые и отменённые" : "Предстоящие и без даты"}</span><span>Локальное время · Москва</span></div><div class="mt-meeting-list">${selectedState === "loading" ? D.skeleton(4) : selectedState === "error" ? retry() : selectedState === "empty" ? empty("Пока нет встреч", "Создайте встречу или присоединитесь по приглашению.", "CalendarDays", button("Создать встречу", "Plus", 'data-go="create-meeting"')) : selectedState === "no-results" ? empty("Среди загруженных совпадений нет", "Попробуйте другое название или загрузите ещё встречи.", "Search", button("Загрузить ещё", "ChevronDown", 'data-state="default"', "secondary")) : list.map((meeting, i) => meetingRow(meeting, i, true)).join("")}</div>${!["loading", "error", "empty", "no-results"].includes(selectedState) ? `<div class="mt-load-more">${button("Загрузить ещё встречи", "ChevronDown", 'data-toast="Показано действие загрузки следующей страницы"', "secondary")}</div>` : ""}<p class="mt-footnote">Поиск работает по загруженной части списка. Записи встреч доступны в разделе <a data-go="recordings" href="#recordings">«Записи»</a>.</p></div>`;
  }
  function createMeeting(selectedState) {
    const planned = ["scheduled", "error", "loading"].includes(selectedState);
    return modal(
      "Новая встреча",
      `${label("Название встречи", planned ? "Дизайн продукта" : "", "Например, обсуждение проекта", "text", 'maxlength="200"')}<div class="mt-create-options"><div class="mt-switch-row"><span class="mt-option-icon">${icon("CalendarDays", 21)}</span><div><strong>Запланировать встречу</strong><p>Выберите дату и время заранее</p></div><button class="mt-switch ${planned ? "is-on" : ""}" role="switch" aria-checked="${planned}" aria-label="Запланировать встречу" data-state="${planned ? "default" : "scheduled"}"><span></span></button></div>${planned ? `<div class="mt-schedule-fields">${label("Дата и время", "2026-10-12T10:00", "", "datetime-local")}<p class="mt-field-help">Москва · UTC+3</p>${label("Длительность, минуты · необязательно", "45", "", "number", 'min="1" max="1440"')}</div>` : ""}<div class="mt-switch-row"><span class="mt-option-icon">${icon("DoorOpen", 21)}</span><div><strong>Зал ожидания</strong><p>Для участников без допуска</p></div><button class="mt-switch" aria-pressed="false" aria-label="Зал ожидания" data-toggle><span></span></button></div><p class="mt-field-help mt-waiting-policy">Участники с действующей ссылкой-приглашением входят без ожидания.</p></div>${alert("Встречу запускает организатор. Запись можно включить после начала встречи.", "info")}${selectedState === "error" ? alert("Выберите дату и время в будущем. Введённое название сохранено.") : ""}`,
      `${button("Отмена", "", 'data-go="home"', "secondary")}${button(selectedState === "loading" ? "Создаём…" : "Создать встречу", selectedState === "loading" ? "LoaderCircle" : "ArrowRight", selectedState === "loading" ? 'disabled aria-busy="true"' : 'data-go="meeting-created"')}`,
      "home",
      "СОБЕРИТЕ КОМАНДУ",
    );
  }
  function prejoin(selectedState, invite = false) {
    const failed = selectedState === "denied";
    const unavailable = selectedState === "unavailable";
    const scheduled = selectedState === "scheduled";
    const isGuest = invite;
    if (selectedState === "loading")
      return `<main class="mt-prejoin"><header>${brand(isGuest ? "landing" : "home")}</header><div class="mt-prejoin-loading">${D.skeleton(2)}<p>Готовим вход во встречу…</p></div></main>`;
    if (unavailable)
      return `<main class="mt-prejoin"><header>${brand(isGuest ? "landing" : "home")}</header>${empty("Встреча недоступна", "Она могла завершиться или приглашение больше не действует. Попросите организатора прислать новую ссылку.", "VideoOff", button("На главную", "ArrowLeft", `data-go="${isGuest ? "landing" : "home"}"`, "secondary"))}</main>`;
    return `<main class="mt-prejoin"><header>${brand(isGuest ? "landing" : "home")}<button class="icon-button" aria-label="Закрыть подключение" data-go="${isGuest ? "landing" : "home"}">${icon("X")}</button></header><div class="mt-prejoin-heading"><span class="eyebrow">${isGuest ? "ВАС ПРИГЛАСИЛИ" : "ПЕРЕД ПОДКЛЮЧЕНИЕМ"}</span><h1>Дизайн продукта</h1><p>${scheduled ? "12 октября · 10:00 · Москва" : "Убедитесь, что вас хорошо видно и слышно."}</p></div><div class="mt-prejoin-layout"><section class="mt-preview-card"><div class="mt-preview"><div class="mt-preview-top">${D.badge(scheduled ? "Запланирована" : "Встреча открыта", scheduled ? "neutral" : "success")}<span>${icon("LockKeyhole", 14)}Видно только вам</span></div><div class="mt-preview-identity">${D.avatar(isGuest ? "Мария" : "Анна Морозова", "blue", "large")}<span>${icon("VideoOff", 21)}Камера выключена</span></div><div class="mt-preview-name">${isGuest ? "Мария" : "Анна Морозова"} · вы</div></div><div class="mt-device-controls"><button aria-label="Включить микрофон" class="mt-device-control" data-toggle aria-pressed="false">${icon("MicOff", 21)}<span>Микрофон</span></button><button aria-label="Включить камеру" class="mt-device-control" data-toggle aria-pressed="false">${icon("VideoOff", 21)}<span>Камера</span></button><button class="mt-device-control mt-mobile-device-settings" data-state="devices" aria-label="Выбрать устройства">${icon("Settings", 21)}<span>Устройства</span></button></div><div class="mt-sound-level"><span>${icon("Mic", 17)}Уровень микрофона</span><div aria-label="Микрофон выключен" class="mt-meter">${Array.from({ length: 20 }, () => "<i></i>").join("")}</div></div>${failed ? alert("Нет доступа к камере. Разрешите её в настройках браузера или войдите без видео.") : selectedState === "device-lost" ? alert("Сохранённое устройство отключено. Выбрано системное устройство.", "info") : selectedState === "busy" ? alert("Устройство занято другим приложением. Закройте его или выберите другое устройство.", "info") : ""}</section><section class="mt-prejoin-settings"><h2>${isGuest ? "Как вас представить?" : "Всё готово к встрече?"}</h2>${isGuest ? label("Имя на встрече", "Мария", "Ваше имя", "text", 'maxlength="100"') : `<div class="mt-signed-in">${D.avatar("Анна Морозова", "blue")}<div><strong>Анна Морозова</strong><span>anna@example.com</span></div></div>`}<div class="mt-device-selects ${selectedState === "devices" ? "mt-device-selects-open" : ""}">${select("Микрофон", ["Системное устройство", "Внешний микрофон"])}${select("Камера", ["Системное устройство", "Внешняя камера"])}${select("Вывод звука", ["Системное устройство", "Наушники"])}<p class="mt-field-help">Выбор сохраняется в этом браузере.</p></div>${scheduled ? alert("Подключение станет доступно, когда организатор начнёт встречу.", "info") : ""}${selectedState === "error" ? alert("Не удалось подключиться. Проверьте интернет и повторите.") : ""}${button(scheduled ? "Ожидаем начала встречи" : "Подключиться", scheduled ? "Clock3" : "ArrowRight", scheduled ? "disabled" : 'data-go="conference"')}<p class="mt-prejoin-privacy">${icon("ShieldCheck", 16)}Можно войти с выключенными камерой и микрофоном.</p>${!isGuest ? `<a class="mt-prejoin-account-link" data-go="settings-profile" href="#settings-profile">Изменить имя в настройках</a>` : ""}</section></div></main>`;
  }
  function calendar(selectedState) {
    const month = selectedState === "month";
    const grid = month
      ? `<div class="mt-month"><div class="mt-month-weekdays">${["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"].map((x) => `<span>${x}</span>`).join("")}</div><div class="mt-month-days">${Array.from(
          { length: 42 },
          (_, i) => {
            const n = i - 2;
            return `<div class="mt-month-day ${n < 1 || n > 31 ? "muted" : ""} ${n === 9 ? "is-today" : ""}"><button data-toast="Показана дата календаря" aria-label="${n < 1 ? 30 + n : n > 31 ? n - 31 : n} ${n < 1 ? "сентября" : n > 31 ? "ноября" : "октября"}">${n < 1 ? 30 + n : n > 31 ? n - 31 : n}</button>${n === 9 ? '<button class="mt-cal-chip" data-state="details">10:00 Дизайн продукта</button><button class="mt-cal-chip mt-cal-chip-soft" data-state="details">14:30 Планы на неделю</button>' : n === 12 ? '<button class="mt-cal-chip" data-state="details">11:00 Командная встреча</button>' : ""}</div>`;
          },
        ).join("")}</div></div>`
      : `<div class="mt-week"><div class="mt-week-head"><span>GMT+3</span>${["Пн 5", "Вт 6", "Ср 7", "Чт 8", "Пт 9", "Сб 10", "Вс 11"].map((day, i) => `<div class="${i === 4 ? "is-today" : ""}"><span>${day.split(" ")[0]}</span><strong>${day.split(" ")[1]}</strong></div>`).join("")}</div><div class="mt-week-body" role="region" tabindex="0" aria-label="Расписание недели по часам"><div class="mt-hour-labels">${Array.from({ length: 24 }, (_, i) => `<span>${String(i).padStart(2, "0")}:00</span>`).join("")}</div><div class="mt-week-columns">${Array.from({ length: 7 }, (_, day) => `<div>${day === 4 ? '<button class="mt-calendar-block" style="--mt-start:10;--mt-size:1.15" data-state="details"><strong>Дизайн продукта</strong><span>10:00 – 10:45</span></button><button class="mt-calendar-block mt-calendar-block-secondary" style="--mt-start:14.5;--mt-size:1" data-state="details"><strong>Планы на следующую неделю</strong><span>14:30 – 15:00</span></button>' : ""}</div>`).join("")}</div></div></div>`;
    const details =
      selectedState === "details"
        ? `<div class="mt-calendar-details"><div><h3>Дизайн продукта</h3><button class="icon-button" data-state="default" aria-label="Закрыть карточку встречи">${icon("X", 18)}</button></div>${D.badge("Запланирована", "info")}<p>${icon("CalendarDays", 17)}9 октября, 10:00 · 45 мин</p><p class="mt-field-help">Вы организатор · Москва<br>Встреча не начнётся автоматически.</p>${button("Открыть встречу", "ArrowRight", 'data-go="conference"')}${button("Изменить расписание", "CalendarDays", 'data-go="edit-schedule"', "secondary")}${button("Отменить встречу", "", 'data-state="cancel"', "ghost")}</div>`
        : "";
    return `<div class="page mt-calendar">${D.heading("Календарь", "Пусть у каждого разговора будет своё время.", button("Запланировать", "Plus", 'data-go="create-meeting" data-go-state="scheduled"'))}<section class="mt-calendar-surface"><div class="mt-calendar-toolbar"><div class="mt-calendar-period"><h2>${month ? "Октябрь 2026" : "5–11 октября 2026"}</h2><span>Предстоящие встречи · Москва</span></div><div class="mt-calendar-nav">${button("Сегодня", "", 'data-state="default"', "secondary")}<button class="icon-button" aria-label="Предыдущий период" data-toast="Показано действие перехода к предыдущему периоду">${icon("ChevronLeft")}</button><button class="icon-button" aria-label="Следующий период" data-toast="Показано действие перехода к следующему периоду">${icon("ChevronRight")}</button></div><div class="tabs mt-calendar-toggle" role="group" aria-label="Вид календаря"><button class="tab ${!month ? "active" : ""}" data-state="default" aria-pressed="${!month}">Неделя</button><button class="tab ${month ? "active" : ""}" data-state="month" aria-pressed="${month}">Месяц</button></div></div>${
      selectedState === "loading"
        ? D.skeleton(4)
        : selectedState === "error"
          ? retry("Не удалось загрузить календарь")
          : selectedState === "empty"
            ? empty(
                "В этом периоде пока свободно",
                "Запланируйте встречу — она появится здесь.",
                "CalendarDays",
                button(
                  "Запланировать",
                  "Plus",
                  'data-go="create-meeting" data-go-state="scheduled"',
                ),
              )
            : `<div class="mt-calendar-desktop">${grid}</div><div class="mt-calendar-agenda"><div class="mt-day-strip">${["Пн 5", "Вт 6", "Ср 7", "Чт 8", "Пт 9", "Сб 10", "Вс 11"].map((day, i) => `<button class="${i === 4 ? "is-today" : ""}" aria-pressed="${i === 4}" data-toast="Показано действие выбора дня"><span>${day.split(" ")[0]}</span><strong>${day.split(" ")[1]}</strong></button>`).join("")}</div><h3>Пятница, 9 октября</h3>${upcoming
                .slice(0, 2)
                .map(
                  (meeting, i) =>
                    `<button class="mt-agenda-event" data-state="details"><span>${meeting.time}<small>${i === 0 ? "10:45" : "15:00"}</small></span><div><strong>${meeting.title}</strong><p>${meeting.duration}${meeting.owner ? " · Вы организатор" : ""}</p></div>${icon("ChevronRight", 18)}</button>`,
                )
                .join("")}</div>`
    }${details}</section>${selectedState === "cancel" ? modal("Отменить встречу?", '<p class="mt-confirm-copy">Встреча «Дизайн продукта» будет отменена для всех участников. Это действие нельзя отменить.</p>', `${button("Оставить встречу", "", 'data-state="default"', "secondary")}${button("Отменить встречу", "", 'data-state="empty"', "danger")}`, "calendar") : ""}<p class="mt-footnote">Здесь только встречи с датой, которые ещё не начались. Остальные — в <a href="#meetings" data-go="meetings">списке встреч</a>.</p></div>`;
  }
  D.register([
    {
      id: "landing",
      title: "Приветственная страница",
      group: "Начало работы",
      route: "/",
      layout: "bare",
      states: [state("default", "Гость")],
      api: [],
      permissions: "Публичная страница.",
      sources: ["frontend/src/pages/Landing.tsx:13"],
      notes: [
        "Переходы только вход / регистрация / кабинет. Декоративный пример не является реальной встречей.",
      ],
      responsive: bareResponsive,
      render: () =>
        `<main class="mt-landing"><header>${brand("landing")}<div>${button("Войти", "", 'data-go="login"', "ghost")}${button("Создать аккаунт", "ArrowRight", 'data-go="register"')}</div></header><section class="mt-landing-hero"><div><span class="eyebrow">MEETRIX · ВСТРЕЧИ. ИДЕИ. РЕЗУЛЬТАТЫ.</span><h1>Хорошие идеи<br>начинаются<br><em>со встречи.</em></h1><p>Проводите видеовстречи, продолжайте разговоры в чате и возвращайтесь к важным моментам.</p>${button("Начать встречаться", "ArrowRight", 'data-go="register"')}<span class="mt-landing-note">По приглашению можно войти без аккаунта.</span></div><div class="mt-landing-product" aria-label="Иллюстрация интерфейса встречи"><div class="mt-landing-product-head"><span>${icon("Video", 16)}Командная встреча</span>${D.badge("В эфире", "success")}</div><div class="mt-landing-tiles">${["Анна Морозова", "Мария Орлова", "Иван Ким", "Елена Смирнова"].map((name, i) => `<div class="mt-landing-tile mt-landing-tile-${i}">${D.avatar(name, ["blue", "teal", "purple", "slate"][i], "large")}<span>${name} ${icon(i === 1 ? "Mic" : "MicOff", 13)}</span></div>`).join("")}</div><div class="mt-landing-controls">${["Mic", "Video", "MonitorUp", "MessageCircle", "PhoneOff"].map((name) => `<span>${icon(name, 20)}</span>`).join("")}</div></div></section><section class="mt-landing-features">${[
          [
            "Link",
            "Просто присоединиться",
            "Создайте встречу и поделитесь ссылкой.",
          ],
          [
            "MessageCircle",
            "Разговор продолжается",
            "Личные и групповые чаты для рабочих вопросов.",
          ],
          [
            "CirclePlay",
            "Важное остаётся",
            "Записи и материалы завершённых встреч.",
          ],
        ]
          .map(
            ([name, title, text]) =>
              `<article>${icon(name, 23)}<h2>${title}</h2><p>${text}</p></article>`,
          )
          .join(
            "",
          )}</section><footer>Meetrix <span>Одно место для вашей команды.</span></footer></main>`,
    },
    {
      id: "login",
      title: "Вход",
      group: "Начало работы",
      route: "/login",
      layout: "bare",
      states: [
        state("default", "Обычное"),
        state("loading", "Отправка"),
        state("error", "Неверные данные"),
        state("expired", "Сессия истекла"),
        state("recovery", "Восстановление недоступно"),
      ],
      api: ["POST /api/v1/auth/login"],
      permissions: "Публичная форма.",
      sources: [
        "frontend/src/pages/AuthPages.tsx:25",
        "internal/transport/http/platform_routes.go:18",
      ],
      notes: [
        "Google/Microsoft сохранены отключёнными заглушками «Позже»; действующего OAuth нет.",
        "Recovery — только пояснение, не сценарий отправки письма.",
        "После входа сохраняется безопасный next.",
      ],
      responsive: {
        desktop: "Auth split story + card440.",
        tablet: "Одна card440 без бокового блока.",
        mobile:
          "Форма width358, input16px/48px, обычная прокрутка при клавиатуре.",
      },
      render: ({ state: s }) => auth(false, s),
    },
    {
      id: "register",
      title: "Регистрация",
      group: "Начало работы",
      route: "/register",
      layout: "bare",
      states: [
        state("default", "Обычное"),
        state("loading", "Отправка"),
        state("error", "Email занят"),
      ],
      api: ["POST /api/v1/auth/register"],
      permissions: "Публичная форма.",
      sources: [
        "frontend/src/pages/AuthPages.tsx:49",
        "internal/domain/users/user.go:179",
      ],
      notes: [
        "Email; пароль 8–128 Unicode-символов; имя необязательно до100.",
        "Email-верификации нет. После регистрации автоматический вход может не удаться — success сохраняется.",
      ],
      responsive: bareResponsive,
      render: ({ state: s }) => auth(true, s),
    },
    {
      id: "register-success",
      title: "Аккаунт создан",
      group: "Начало работы",
      route: "/register/success",
      layout: "bare",
      states: [
        state("default", "Вход выполнен"),
        state("login-required", "Требуется вход"),
      ],
      api: [],
      permissions: "Результат регистрации.",
      sources: ["frontend/src/pages/AuthPages.tsx:292"],
      notes: ["CTA зависит от фактически восстановленной сессии."],
      responsive: bareResponsive,
      render: ({ state: s }) =>
        `<main class="mt-success-page">${brand(s === "login-required" ? "landing" : "home")}<section class="mt-success-card"><span class="mt-success-mark">${icon("Check", 36)}</span><span class="eyebrow">ПРИЯТНО ПОЗНАКОМИТЬСЯ</span><h1>Вы в Meetrix.</h1><p>Аккаунт создан. Теперь можно собирать команду, встречаться и сохранять важное.</p>${button(s === "login-required" ? "Войти в аккаунт" : "Перейти в приложение", "ArrowRight", `data-go="${s === "login-required" ? "login" : "home"}"`)}</section></main>`,
    },
    {
      id: "home",
      title: "Главная",
      group: "Рабочее пространство",
      route: "/app",
      nav: "home",
      states: [
        ...statusStates,
        state("active", "Активные"),
        state("past", "Завершённые"),
      ],
      api: [
        "GET /api/v1/me/conferences?view=upcoming",
        "GET /api/v1/me/conferences?view=past&status=finished",
      ],
      permissions: "Постоянный аккаунт; только доступные встречи.",
      sources: ["frontend/src/pages/Dashboard.tsx:191"],
      notes: [
        "Быстрые действия: создать, запланировать, присоединиться по ссылке.",
        "Недавние встречи — фактический список истории, не выдуманные превью записей.",
        "История доступна из материалов, в основное меню не возвращается.",
      ],
      responsive: commonResponsive,
      render: ({ state: s }) => home(s),
    },
    {
      id: "meetings",
      title: "Встречи",
      group: "Рабочее пространство",
      route: "/conferences",
      nav: "meetings",
      states: [
        ...statusStates,
        state("active", "Активные"),
        state("past", "Завершённые"),
        state("no-results", "Поиск без совпадений"),
        state("filters", "Мобильные фильтры"),
      ],
      api: ["GET /api/v1/me/conferences?view&scope&from&to&cursor"],
      permissions: "Постоянный аккаунт; серверная фильтрация членства.",
      sources: [
        "frontend/src/pages/Dashboard.tsx:200",
        "internal/domain/conferences/conference.go:155",
      ],
      notes: [
        "Название/даты/статус/длительность реальны; имён организаторов и счётчиков участников в основном View нет.",
        "Поиск только среди загруженных страниц.",
        "Завершённая встреча ведёт к /history/:id.",
      ],
      responsive: commonResponsive,
      render: ({ state: s }) => meetings(s),
    },
    {
      id: "create-meeting",
      title: "Создать встречу",
      group: "Встречи и приглашения",
      route: "/meetings/new",
      nav: "meetings",
      states: [
        state("default", "Без даты"),
        state("scheduled", "Запланированная"),
        state("error", "Ошибка даты"),
        state("loading", "Создание"),
      ],
      api: ["POST /api/v1/conferences"],
      permissions: "Постоянный аккаунт.",
      sources: [
        "frontend/src/components/ConferenceModals.tsx:99",
        "internal/domain/conferences/conference.go:259",
      ],
      notes: [
        "Только title/waitingRoomEnabled/scheduledAt/plannedDurationMin.",
        "Нет описания, повтора, автозаписи или автозапуска.",
        "Валидная invite-ссылка обходит зал ожидания: ограничение отражено прямо в форме.",
      ],
      responsive: {
        desktop: "Modal520 padding32, max-height viewport−48.",
        tablet: "Modal520, прокручиваемое содержимое.",
        mobile:
          "Полноэкранная форма, header56/body16/sticky footer + safe-area.",
      },
      render: ({ state: s }) => createMeeting(s),
    },
    {
      id: "meeting-created",
      title: "Встреча создана",
      group: "Встречи и приглашения",
      route: "/meetings/new · результат",
      nav: "meetings",
      states: [
        state("default", "Обычное"),
        state("copied", "Ссылка скопирована"),
        state("copy-error", "Копирование не удалось"),
      ],
      api: [],
      permissions: "Создавший встречу владелец.",
      sources: ["frontend/src/components/ConferenceModals.tsx:23"],
      notes: [
        "Share не означает запуск. Полученный inviteCode преобразуется в публичную /i/:code ссылку.",
      ],
      responsive: commonResponsive,
      render: ({ state: s }) =>
        modal(
          "Встреча создана",
          `<div class="mt-created-intro"><span class="mt-success-mark">${icon("Check", 28)}</span><h2>Дизайн продукта</h2><p>Пригласите коллег и начните разговор.</p></div><label class="field mt-field"><span>Ссылка-приглашение</span><div class="mt-copy-field"><input readonly value="https://meetrix.example/i/пример" aria-label="Ссылка-приглашение"><button class="icon-button" data-state="copied" aria-label="Скопировать ссылку">${icon(s === "copied" ? "Check" : "Copy", 20)}</button></div></label>${s === "copied" ? alert("Показано состояние: ссылка скопирована.", "success") : s === "copy-error" ? alert("Выделите и скопируйте ссылку вручную. Браузер не разрешил копирование.") : ""}<p class="mt-field-help">По ссылке можно присоединиться без аккаунта.</p><div class="mt-created-actions">${button("Открыть встречу", "Video", 'data-go="conference"')}${button("Пригласить участников", "Mail", 'data-go="invite"', "secondary")}</div>`,
          "",
          "home",
        ),
    },
    {
      id: "join-link",
      title: "Войти по ссылке",
      group: "Встречи и приглашения",
      route: "/app · диалог",
      nav: "meetings",
      states: [
        state("default", "Обычное"),
        state("error", "Некорректная ссылка"),
      ],
      api: ["GET /api/v1/conference-invites/:code"],
      permissions: "Код или ссылка текущей установки.",
      sources: ["frontend/src/components/ConferenceModals.tsx:443"],
      notes: [
        "Принимается same-origin /i/:code либо32-символьный код; результат публичный pre-join.",
      ],
      responsive: commonResponsive,
      render: ({ state: s }) =>
        modal(
          "Присоединиться по ссылке",
          `<p class="mt-modal-description">Попросите организатора поделиться приглашением.</p>${label("Ссылка или код", "", "Вставьте ссылку-приглашение")}${s === "error" ? alert("Нужна ссылка этой установки Meetrix или код из 32 символов.") : ""}`,
          `${button("Отмена", "", 'data-go="home"', "secondary")}${button("Открыть приглашение", "ArrowRight", 'data-go="guest-prejoin"')}`,
          "home",
        ),
    },
    {
      id: "invite",
      title: "Пригласить участников",
      group: "Встречи и приглашения",
      route: "/conferences/:id · диалог",
      nav: "meetings",
      states: [
        state("default", "Обычное"),
        state("loading", "Отправка"),
        state("search-empty", "Нет результатов"),
        state("sent", "Поставлено в очередь"),
        state("error", "Ошибка"),
      ],
      api: [
        "GET /api/v1/conferences/:id/invitation-users?query",
        "POST /api/v1/conferences/:id/invitations",
      ],
      permissions:
        "Владелец created/scheduled/active; co_host только joined/admitted в active; постоянные аккаунты.",
      sources: [
        "frontend/src/components/ConferenceInvitations.tsx:40",
        "internal/infrastructure/postgres/conference_invitation_repository.go:26",
      ],
      notes: [
        "До20 получателей, поиск≥2 символа.",
        "Статус queued не равен доставлено; есть already_invited и left_chat.",
      ],
      responsive: commonResponsive,
      render: ({ state: s }) =>
        modal(
          "Пригласить участников",
          `<p class="mt-modal-description">Дизайн продукта</p><div class="mt-copy-field"><input readonly aria-label="Ссылка на встречу" value="meetrix.example/i/пример"><button class="icon-button" aria-label="Скопировать ссылку" data-toast="Показано состояние копирования ссылки">${icon("Copy")}</button></div><p class="mt-field-help">По приглашению можно войти без аккаунта.</p><div class="mt-form-divider"><span>или отправьте приглашение</span></div><label class="field mt-field"><span>Email участников</span><textarea placeholder="colleague@example.com" rows="2" aria-describedby="mt-recipients-hint"></textarea><small id="mt-recipients-hint">До 20 адресов, через запятую или с новой строки.</small></label>${label("Пользователи Meetrix", "", "Имя или email · от 2 символов", "search")}<div class="mt-selected-person">${D.avatar("Мария Орлова", "teal", "small")}<span>Мария Орлова</span><button class="icon-button" data-toast="Показано действие удаления выбранного получателя" aria-label="Убрать Марию Орлову">${icon("X", 16)}</button></div>${s === "search-empty" ? '<p class="mt-field-help">Пользователи не найдены. Можно указать email выше.</p>' : ""}${s === "sent" ? alert("Показано состояние: приглашение для maria@example.test поставлено в очередь.", "success") : s === "error" ? alert("Не удалось отправить приглашения. Проверьте адреса и повторите.") : ""}`,
          `${button("Назад", "", 'data-go="meeting-created"', "secondary")}${button(s === "loading" ? "Отправляем…" : "Пригласить", "Mail", s === "loading" ? 'disabled aria-busy="true"' : 'data-state="sent"')}`,
          "meeting-created",
        ),
    },
    {
      id: "prejoin",
      title: "Перед входом",
      group: "Встречи и приглашения",
      route: "/meetings/:id/join",
      layout: "bare",
      states: [
        state("default", "Устройства выключены"),
        state("loading", "Загрузка"),
        state("denied", "Доступ запрещён"),
        state("device-lost", "Устройство отключено"),
        state("busy", "Устройство занято"),
        state("devices", "Устройства на мобильном"),
        state("error", "Ошибка подключения"),
        state("unavailable", "Встреча недоступна"),
      ],
      api: [
        "GET /api/v1/conferences/:id",
        "GET /api/v1/conferences/:id/participants/me",
        "POST /api/v1/conferences/:id/join",
      ],
      permissions: "Текущий участник/разрешённая конференция.",
      sources: [
        "frontend/src/pages/PreJoinPage.tsx:55",
        "frontend/src/prejoinDevices.ts:133",
      ],
      notes: [
        "До допуска preview только локальный, SFU не подключён.",
        "Выбор вывода звука только при setSinkId, при отсутствии скрыть.",
        "В прототипе устройства не запрашиваются; кнопки показывают лишь визуальное состояние.",
        "Внутренний вход и публичное приглашение различаются автоматическим включением медиа — сохранить текущую семантику.",
      ],
      responsive: {
        desktop: "Контейнер1080, preview640 + settings360, gap32.",
        tablet: "Preview16:9 сверху, настройки ниже.",
        mobile: "Preview358×201, controls48, устройства раскрываются, Join52.",
      },
      render: ({ state: s }) => prejoin(s, false),
    },
    {
      id: "guest-prejoin",
      title: "Приглашение и вход гостя",
      group: "Встречи и приглашения",
      route: "/i/:code",
      layout: "bare",
      states: [
        state("default", "Гость"),
        state("scheduled", "Встреча ещё не началась"),
        state("denied", "Нет доступа к камере"),
        state("devices", "Выбор устройств"),
        state("unavailable", "Приглашение недоступно"),
      ],
      api: [
        "GET /api/v1/conference-invites/:code",
        "POST /api/v1/conference-invites/:code/guest",
      ],
      permissions:
        "Публично. Гостевой вход только created/active; scoped сессия одной встречи.",
      sources: [
        "frontend/src/pages/InvitePage.tsx:8",
        "internal/infrastructure/postgres/guest_repository.go:31",
      ],
      notes: [
        "Сначала только метаданные, затем явное действие входа создаёт гостя.",
        "Valid invite bypasses waiting room. Нельзя изображать обязательное ожидание допуска гостя по ссылке.",
        "Гостю недоступен кабинет.",
      ],
      responsive: bareResponsive,
      render: ({ state: s }) => prejoin(s, true),
    },
    {
      id: "calendar",
      title: "Календарь",
      group: "Рабочее пространство",
      route: "/calendar",
      nav: "calendar",
      states: [
        ...statusStates,
        state("month", "Месяц"),
        state("details", "Карточка встречи"),
        state("cancel", "Отмена встречи"),
      ],
      api: [
        "GET /api/v1/me/conferences?view=upcoming&from&to",
        "POST /api/v1/conferences/:id/cancel",
      ],
      permissions: "Постоянный аккаунт; изменение только владельцем.",
      sources: [
        "frontend/src/pages/CalendarPage.tsx:227",
        "internal/infrastructure/postgres/conference_product_repository.go:178",
      ],
      notes: [
        "Только будущие created/scheduled, в сетке только существующий scheduledAt.",
        "Нет внешнего calendar sync controls, повторов или drag/drop.",
        "Для частичной страницы сохраняется отдельный action загрузки продолжения.",
        "Mobile agenda использует те же API.",
      ],
      responsive: {
        desktop: "Неделя7дней и24ч scroll / месяц7×6; caption12+, event44+.",
        tablet:
          "При ширине контента<760 agenda, иначе сетка. Toolbar переносится.",
        mobile:
          "Выбор периода+7дней44+, agenda одной колонкой. Не уменьшенная desktop таблица.",
      },
      render: ({ state: s }) => calendar(s),
      afterRender: () => {
        const grid = document.querySelector(".mt-week-body");
        if (grid)
          grid.scrollTop =
            9 *
            3.5 *
            parseFloat(getComputedStyle(document.documentElement).fontSize);
      },
    },
    {
      id: "edit-schedule",
      title: "Изменить расписание",
      group: "Встречи и приглашения",
      route: "/calendar · диалог",
      nav: "calendar",
      states: [state("default", "Обычное"), state("error", "Ошибка")],
      api: ["PUT /api/v1/conferences/:id/schedule"],
      permissions: "Только владелец status=scheduled.",
      sources: [
        "internal/infrastructure/postgres/conference_product_repository.go:111",
        "frontend/src/components/ConferenceModals.tsx:305",
      ],
      notes: [
        "Целое число минут1–1440 либо пусто; время строго в будущем.",
        "Дата в часовом поясе браузера отправляется RFC3339 UTC.",
      ],
      responsive: commonResponsive,
      render: ({ state: s }) =>
        modal(
          "Изменить расписание",
          `<p class="mt-modal-description">Дизайн продукта</p>${label("Дата и время", "2026-10-12T10:00", "", "datetime-local")}<p class="mt-field-help">Часовой пояс: Москва · UTC+3</p>${label("Длительность, минуты · необязательно", "45", "", "number", 'min="1" max="1440"')}${s === "error" ? alert("Встреча уже началась. Расписание больше нельзя изменить.") : alert("Встречу запускает организатор. Она не начнётся автоматически.", "info")}`,
          `${button("Отмена", "", 'data-go="calendar"', "secondary")}${button("Сохранить", "Check", 'data-go="calendar"')}`,
          "calendar",
        ),
    },
  ]);
})();
