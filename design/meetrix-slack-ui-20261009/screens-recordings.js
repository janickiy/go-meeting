/* Автономный дизайн-макет. Данные вымышлены, соответствуют полям DTO и не запрашиваются по сети. */
(() => {
  "use strict";
  const D = window.MeetrixDesign;
  const E = D.esc;
  const icon = (name, size = 20) => D.icon(name, size);
  const button = (label, name, attrs, kind = "secondary") =>
    D.button(
      kind === "icon" ? "" : label,
      name,
      kind === "icon" && !attrs.includes("aria-label=")
        ? `${attrs} aria-label="${E(label)}"`
        : attrs,
      kind,
    );
  const go = (id, nextState = "default") =>
    `data-go="${id}" data-go-state="${nextState}"`;
  const toast = (text) => `data-toast="${E(text)}"`;
  const stateButton = (label, next, name = "") =>
    button(label, name, `data-state="${next}"`);
  const states = (...items) => items.map(([id, label]) => ({ id, label }));
  const sources = (...items) => items;
  const responseStates = states(
    ["default", "Обычное"],
    ["loading", "Загрузка"],
    ["empty", "Пусто"],
    ["error", "Ошибка"],
    ["expired", "Материалы недоступны"],
  );
  const retention =
    "Готовые записи доступны 7 дней после завершения записи. Ссылка на файл временная; ссылка на страницу не открывает доступ посторонним.";
  const people = [
    { id: "p1", displayName: "Мария Орлова", role: "owner" },
    { id: "p2", displayName: "Денис Волков", role: "co_host" },
    { id: "p3", displayName: "Анна Смирнова", role: "participant" },
    { id: "p4", displayName: "Илья Козлов", role: "participant" },
    { id: "p5", displayName: "Елена Белова", role: "participant" },
    { id: "p6", displayName: "Павел Соколов", role: "participant" },
    { id: "p7", displayName: "Ольга Морозова", role: "participant" },
    { id: "p8", displayName: "Алексей Новиков", role: "participant" },
  ];
  const conference = {
    id: "42a31bbb-79b0-43f0-bc00-ec59d167acb7",
    ownerId: "u1",
    title: "Запуск мобильного приложения",
    status: "finished",
    createdAt: "2026-10-07T08:00:00Z",
    startedAt: "2026-10-08T14:00:00Z",
    finishedAt: "2026-10-08T14:40:00Z",
  };
  const file = (fileType, sizeBytes) => ({
    fileType,
    sizeBytes,
    url: `/fixtures/never-requested/${fileType}`,
  });
  const recordings = [
    {
      uuid: "2b5ecf8c-c008-4cec-b303-908c3b6047cd",
      conferenceId: conference.id,
      requestedBy: "u1",
      mode: "composite",
      status: "ready",
      createdAt: "2026-10-08T14:01:00Z",
      startedAt: "2026-10-08T14:01:00Z",
      endedAt: "2026-10-08T14:19:44Z",
      durationSec: 1124,
      files: [file("final_mp4", 186646528), file("preview_jpg", 184320)],
    },
    {
      uuid: "aabbb7b1-d165-4d0e-8c7e-adb26d03ebd5",
      conferenceId: conference.id,
      requestedBy: "u2",
      mode: "audio_only",
      status: "ready",
      createdAt: "2026-10-08T14:20:00Z",
      startedAt: "2026-10-08T14:20:00Z",
      endedAt: "2026-10-08T14:30:24Z",
      durationSec: 624,
      files: [file("final_audio", 11953766)],
    },
    {
      uuid: "ee24a78b-c2a1-4235-9c0c-b87887207c5f",
      conferenceId: conference.id,
      requestedBy: "u1",
      mode: "individual_tracks",
      status: "ready",
      createdAt: "2026-10-08T14:32:00Z",
      startedAt: "2026-10-08T14:32:00Z",
      endedAt: "2026-10-08T14:38:12Z",
      durationSec: 372,
      files: [file("final_audio", 7112542), file("tracks_archive", 94268236)],
    },
  ];
  const history = {
    conference,
    owner: { id: "u1", displayName: "Мария Орлова" },
    durationSec: 2400,
    participantCount: 8,
    participants: people,
    participantsTruncated: false,
    recordings: { total: 3, ready: 3, processing: 0, failed: 0 },
    chatAvailable: true,
    chatReadOnly: true,
  };
  const capabilities = {
    liveCaptions: false,
    transcription: true,
    aiSummary: true,
    semanticSearch: false,
    meetingAnalytics: true,
    recordingModes: [
      "composite",
      "audio_only",
      "individual_tracks",
      "screen_focus",
    ],
  };
  const durations = (seconds) =>
    `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
  const size = (bytes) =>
    `${(bytes / 1048576).toLocaleString("ru-RU", { maximumFractionDigits: 1 })} МБ`;
  const date = (iso) =>
    new Date(iso).toLocaleString("ru-RU", {
      timeZone: "Europe/Moscow",
      day: "numeric",
      month: "short",
      hour: "2-digit",
      minute: "2-digit",
    });
  const tones = ["blue", "teal", "purple", "slate"];
  function back(id, label) {
    return `<div class="rec-back">${button(label, "ArrowLeft", go(id), "ghost")}</div>`;
  }
  function note(text = retention) {
    return `<p class="rec-footnote">${icon("Info", 16)}<span>${E(text)}</span></p>`;
  }
  function loading(title) {
    return `<div class="page rec-page">${D.heading(title, "Загружаем доступные материалы…")}${D.skeleton(3)}</div>`;
  }
  function error(title, subtitle, retry = "default") {
    return D.empty(
      title,
      subtitle,
      "CloudOff",
      stateButton("Попробовать снова", retry, "RefreshCw"),
    );
  }
  function expired() {
    return D.empty(
      "Материалы записи недоступны",
      "Срок хранения мог закончиться или доступ к встрече изменился. Записи доступны 7 дней после завершения.",
      "FileVideo",
      button("К встречам", "ArrowRight", go("meetings")),
    );
  }
  function menu(id, label, options) {
    return `<div class="rec-menu-wrap">${button(label, "ChevronDown", `data-menu="${id}" aria-haspopup="menu" aria-expanded="false" aria-controls="${id}"`)}<div id="${id}" class="rec-menu dropdown card" hidden role="menu" aria-label="${E(label)}">${options}</div></div>`;
  }
  function actionMenu(id, audio = false) {
    return `<div class="rec-menu-wrap">${button("Действия с записью", "MoreVertical", `data-menu="${id}" aria-label="Действия с записью" aria-haspopup="menu" aria-expanded="false" aria-controls="${id}"`, "icon")}<div id="${id}" class="rec-menu dropdown card" hidden role="menu" aria-label="Действия с записью">${button(audio ? "Скачать аудио" : "Скачать запись", "Download", `${toast("Показано действие скачивания. Файл в прототипе не загружается.")} role="menuitem"`, "ghost")}${button("Копировать ссылку", "Link", `${toast("Показано состояние «Ссылка скопирована». Это только демонстрация.")} role="menuitem"`, "ghost")}<p class="small muted">Для участников с доступом к встрече.</p></div></div>`;
  }
  function thumbnail(record, large = false) {
    const audio = ["audio_only", "individual_tracks"].includes(record.mode);
    const tracks = record.mode === "individual_tracks";
    const label = tracks ? "Аудиомикс и дорожки" : audio ? "Аудиозапись" : "Видеозапись";
    const illustration = tracks
      ? `<div class="rec-editorial-tracks">${icon("FileArchive", 25)}<div>${[1, 2, 3].map(() => '<span><i></i><i></i><i></i><i></i><i></i><i></i></span>').join("")}</div></div>`
      : audio
        ? `<div class="rec-editorial-wave">${Array.from({ length: 19 }, (_, n) => `<i style="--rec-wave:${12 + ((n * 13) % 37)}px"></i>`).join("")}</div>`
        : `<div class="rec-editorial-document"><span>${icon("Clapperboard", 27)}</span><div><i></i><i></i><i></i></div></div>`;
    return `<div class="rec-preview rec-editorial-preview${audio ? " rec-preview-audio" : ""}${tracks ? " rec-preview-tracks" : ""}${large ? " rec-preview-large" : ""}" aria-hidden="true"><span class="rec-editorial-label">${label}</span>${illustration}<span class="rec-preview-time">${durations(record.durationSec)}</span></div>`;
  }
  function recordingCard(record, index) {
    const audio = ["audio_only", "individual_tracks"].includes(record.mode);
    const main = record.files.find((item) =>
      ["final_mp4", "final_audio"].includes(item.fileType),
    );
    return `<article class="rec-record-card card"><button class="rec-preview-button" ${go("recording-player", record.mode === "individual_tracks" ? "tracks" : audio ? "audio" : "default")} aria-label="Открыть запись: ${E(conference.title)}">${thumbnail(record)}</button><div class="rec-record-copy"><button class="rec-title-link" ${go("recording-player", record.mode === "individual_tracks" ? "tracks" : audio ? "audio" : "default")}>${E(conference.title)}</button><p class="rec-meta">${date(record.startedAt)}<span>·</span>${record.mode === "individual_tracks" ? "Аудиомикс и дорожки" : audio ? "Аудиозапись" : "Видеозапись"}</p><p class="rec-meta">${icon("Users", 15)} 8 участников встречи <span>·</span> ${size(main.sizeBytes)}</p><p class="rec-owner">Организатор: Мария Орлова</p></div><div class="rec-record-actions">${button("Смотреть", "Play", go("recording-player", record.mode === "individual_tracks" ? "tracks" : audio ? "audio" : "default"), "ghost")}${actionMenu(`rec-actions-${index}`, audio)}</div></article>`;
  }
  function renderRecordings({ state }) {
    const active = state === "active";
    const title = active ? "Обсуждение следующего релиза" : conference.title;
    if (state === "loading") return loading("Записи");
    const switcher = `<div class="rec-meeting-switch" role="group" aria-label="Список встреч"><button class="rec-switch${!active ? " rec-switch-active" : ""}" data-state="default" aria-pressed="${!active}">Завершённые</button><button class="rec-switch${active ? " rec-switch-active" : ""}" data-state="active" aria-pressed="${active}">В эфире</button></div>`;
    const picker = `<section class="rec-selection card" aria-label="Выбор встречи">${switcher}<div class="rec-picker-grid"><label class="field rec-search"><span>Поиск по загруженным встречам</span><div>${icon("Search", 18)}<input type="search" placeholder="Название встречи" aria-label="Поиск по загруженным встречам" /></div></label><div class="rec-selector"><span class="rec-field-label">Встреча</span>${menu("rec-meeting-menu", title, stateButton(conference.title, "default") + stateButton("Обсуждение следующего релиза", "active"))}</div>${button("Ещё встречи", "Plus", toast("Показано состояние загрузки следующей страницы встреч."), "ghost")}</div><p class="rec-scope small muted">Поиск — только по загруженным встречам. Записи — только для выбранной встречи.</p></section>`;
    const period = menu(
      "rec-period-menu",
      "За всё время",
      [
        "Все время",
        "Последние 7 дней",
        "Последние 30 дней",
        "Последние 90 дней",
      ]
        .map((label) =>
          button(
            label,
            "",
            `data-state="${label === "Все время" ? "default" : "filtered"}" role="menuitem"`,
            "ghost",
          ),
        )
        .join(""),
    );
    const sort = menu(
      "rec-sort-menu",
      state === "oldest" ? "Сначала старые" : "Сначала новые",
      stateButton("Сначала новые", "default") +
        stateButton("Сначала старые", "oldest"),
    );
    let body;
    if (state === "error")
      body = error(
        "Не удалось загрузить записи",
        "Проверьте подключение и попробуйте ещё раз. Доступ к встрече проверяется сервером.",
      );
    else if (state === "expired") body = expired();
    else if (state === "empty" || active)
      body = D.empty(
        "Доступных записей пока нет",
        active
          ? "Встреча продолжается. Готовые записи появятся после завершения обработки."
          : "Выберите другую встречу или вернитесь позже.",
        "Clapperboard",
        button("К встречам", "Video", go("meetings")),
      );
    else if (state === "filtered")
      body = D.empty(
        "В выбранном периоде записей нет",
        "Измените период. Фильтры и сортировка применяются к загруженным записям.",
        "Search",
        stateButton("Сбросить период", "default", "RotateCcw"),
      );
    else
      body = `<div class="rec-list">${(state === "oldest" ? [...recordings].reverse() : recordings).map(recordingCard).join("")}</div>`;
    return `<div class="page rec-page">${D.heading("Записи", "Записи и материалы выбранной встречи.")}${picker}<section class="rec-library" aria-labelledby="rec-library-title"><div class="rec-library-heading"><div><h2 id="rec-library-title">${E(title)}</h2><p class="small muted">${active ? "Активная встреча" : "8 октября · Организатор Мария Орлова"}</p></div><div class="rec-filters">${period}${sort}</div></div>${body}<div class="rec-list-footer">${!["empty", "error", "expired", "active", "filtered"].includes(state) ? button("Ещё записи", "ChevronDown", toast("Показано состояние загрузки следующей страницы записей."), "ghost") : ""}${!active ? button("Материалы встречи", "ArrowUpRight", go("history-detail"), "ghost") : ""}</div></section>${note()}</div>`;
  }
  const poster = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720"><rect width="1280" height="720" fill="#251d2b"/><rect x="84" y="68" width="1112" height="584" rx="10" fill="#f7f4f8"/><path d="M84 126H1196" stroke="#e1d7e6"/><circle cx="118" cy="97" r="5" fill="#bcaec5"/><circle cx="138" cy="97" r="5" fill="#d1c4d8"/><circle cx="158" cy="97" r="5" fill="#e0d6e5"/><g font-family="sans-serif"><text x="140" y="206" fill="#655d69" font-size="22" letter-spacing="3">MEETRIX · ЗАПИСЬ ВСТРЕЧИ</text><text x="136" y="317" fill="#242127" font-size="63" font-weight="600">Запуск мобильного</text><text x="136" y="393" fill="#242127" font-size="63" font-weight="600">приложения</text><text x="140" y="482" fill="#655d69" font-size="25">8 октября 2026 · Команда продукта</text><text x="140" y="600" fill="#655d69" font-size="20">Демонстрационный кадр</text></g><g transform="translate(950 284)"><rect width="166" height="208" rx="10" fill="#ede4f1"/><rect x="32" y="34" width="102" height="78" rx="8" fill="#ffffff" stroke="#d7c8df"/><path d="M71 54L99 73L71 92Z" fill="#6b3f78"/><path d="M33 148H135M33 173H108" stroke="#a18bab" stroke-width="8" stroke-linecap="round"/></g><path d="M140 536H716" stroke="#ded4e3" stroke-width="2"/><path d="M140 536H316" stroke="#6b3f78" stroke-width="4"/></svg>')}`;
  function materialList(record) {
    const labels = {
      final_mp4: ["FileVideo", "Видеозапись MP4"],
      final_audio: ["FileAudio", "Аудиозапись"],
      preview_jpg: ["Image", "Превью записи"],
      tracks_archive: ["FileArchive", "Архив дорожек ZIP"],
    };
    return `<ul class="rec-files">${record.files.map((item) => `<li><span class="rec-file-icon">${icon(labels[item.fileType][0], 21)}</span><div><strong>${labels[item.fileType][1]}</strong><span class="small muted">${size(item.sizeBytes)}</span></div>${button(`Скачать ${labels[item.fileType][1]}`, "Download", `${toast("Показано действие скачивания. Файл в прототипе не загружается.")} aria-label="Скачать ${labels[item.fileType][1]}"`, "icon")}</li>`).join("")}</ul>`;
  }
  function renderPlayer({ state }) {
    if (state === "loading") return loading("Просмотр записи");
    const audio = state === "audio" || state === "tracks";
    const record =
      state === "tracks"
        ? recordings[2]
        : audio
          ? recordings[1]
          : recordings[0];
    const main = record.files[0];
    let player;
    if (state === "expired") player = expired();
    else if (state === "error")
      player = D.empty(
        "Не удалось воспроизвести запись",
        "Ссылка на файл могла устареть. Обновите ссылку и попробуйте ещё раз.",
        "FileVideo",
        stateButton("Обновить ссылку", "default", "RefreshCw"),
      );
    else if (state === "empty")
      player = D.empty(
        "Запись пока недоступна",
        "Материалы ещё не готовы или файл недоступен. Попробуйте обновить запись позже.",
        "FileVideo",
        stateButton("Обновить запись", "default", "RefreshCw"),
      );
    else
      player = audio
        ? `<div class="rec-audio-player card">${icon("AudioLines", 48)}<h2>Аудиозапись встречи</h2><p class="muted">${date(record.startedAt)} · ${durations(record.durationSec)}</p><audio controls preload="none" aria-label="Демонстрационный аудиоплеер без медиафайла"></audio><p class="small muted">Дизайн-макет: медиафайл не загружается.</p></div>`
        : `<div class="rec-video-player"><video controls playsinline preload="none" poster="${poster}" aria-label="Демонстрационный видеоплеер без медиафайла"></video><p class="rec-player-demo">Демонстрационный кадр · воспроизведение в макете отключено</p></div>`;
    const unavailable = ["expired", "error", "empty"].includes(state);
    return `<div class="page rec-page">${back("recordings", "К записям выбранной встречи")}${D.heading(conference.title, unavailable ? "Доступ к материалам записи не подтверждён" : `${date(record.startedAt)} · ${durations(record.durationSec)}`, unavailable ? "" : button("Скачать запись", "Download", toast("Показано действие скачивания. Файл в прототипе не загружается."), "primary"))}<div class="rec-player-layout"><section class="rec-player-main" aria-label="Просмотр записи">${player}</section><aside class="rec-player-sidebar"><section class="card rec-info-card"><h2>О записи</h2>${unavailable ? '<p class="small muted">Информация появится после подтверждения доступа к записи.</p>' : `<dl class="rec-info"><div><dt>Дата</dt><dd>8 октября 2026</dd></div><div><dt>Длительность</dt><dd>${durations(record.durationSec)}</dd></div><div><dt>Размер файла</dt><dd>${size(main.sizeBytes)}</dd></div><div><dt>Организатор</dt><dd>Мария Орлова</dd></div><div><dt>Участники встречи</dt><dd>8</dd></div></dl>`}</section><section class="card rec-info-card"><h2>Материалы записи</h2>${unavailable ? '<p class="small muted">Доступных материалов пока нет.</p>' : materialList(record)}<p class="rec-download-hint">Если файл открылся в браузере, сохраните его через меню плеера или браузера.</p></section></aside></div>${note()}${button("Материалы встречи", "ArrowUpRight", go("history-detail"), "ghost")}</div>`;
  }
  function renderHistory({ state }) {
    if (state === "loading") return loading("История встреч");
    const past = [
      conference,
      {
        ...conference,
        id: "meeting-2",
        title: "Планирование следующего спринта",
        startedAt: "2026-10-07T09:00:00Z",
        finishedAt: "2026-10-07T09:52:00Z",
      },
      {
        ...conference,
        id: "meeting-3",
        title: "Согласование дизайн-системы",
        startedAt: "2026-10-06T12:00:00Z",
        finishedAt: "2026-10-06T12:35:00Z",
      },
    ];
    const content =
      state === "error"
        ? error(
            "История встреч не загрузилась",
            "Проверьте подключение. Повторная загрузка использует тот же доступный список встреч.",
          )
        : state === "empty"
          ? D.empty(
              "Завершённых встреч пока нет",
              "История и доступные материалы появятся после встречи.",
              "CalendarClock",
              button("К встречам", "Video", go("meetings")),
            )
          : state === "expired"
            ? `<div class="rec-inline-expiry">${D.notice("История встречи сохранена, но её записи могут быть недоступны после срока хранения.", "info")}</div>`
            : `<div class="rec-history-list">${past.map((item) => `<button class="rec-history-row" ${go("history-detail")}><span class="rec-history-art">${icon("Video", 25)}</span><span><strong>${E(item.title)}</strong><span class="rec-meta">${date(item.finishedAt)} · ${Math.round((Date.parse(item.finishedAt) - Date.parse(item.startedAt)) / 60000)} мин</span></span><span class="rec-history-open">Материалы ${icon("ChevronRight", 18)}</span></button>`).join("")}</div>`;
    return `<div class="page rec-page">${back("recordings", "К записям")}${D.heading("История встреч", "Завершённые встречи и доступные материалы.")}<section class="card rec-history-card"><div class="rec-section-head"><h2>Завершённые встречи</h2><label class="field rec-search"><span class="sr-only">Поиск по загруженным встречам</span><div>${icon("Search", 18)}<input type="search" placeholder="Поиск по названию" aria-label="Поиск по загруженным встречам" /></div></label></div><p class="small muted">Поиск по загруженным встречам. Откройте встречу, чтобы посмотреть доступные материалы.</p>${content}<div class="rec-list-footer">${button("Ещё встречи", "ChevronDown", toast("Показано состояние загрузки следующей страницы встреч."), "ghost")}${button("Все завершённые встречи", "ArrowUpRight", go("meetings", "past"), "ghost")}</div></section>${note("Вторичный экран: история не возвращается в основную навигацию.")}</div>`;
  }
  const transcriptSegments = [
    {
      id: "s1",
      startMs: 37000,
      endMs: 51000,
      speakerLabel: "Говорящий 1",
      text: "Сегодня согласуем порядок запуска. Сначала проверим сценарий регистрации, затем переход к встрече.",
    },
    {
      id: "s2",
      startMs: 148000,
      endMs: 167000,
      speakerLabel: "Говорящий 2",
      text: "Проверка готова. Осталось уточнить тексты пустых состояний и убедиться, что кнопки доступны с клавиатуры.",
    },
    {
      id: "s3",
      startMs: 294000,
      endMs: 321000,
      speakerLabel: "Говорящий 1",
      text: "Договорились: Мария соберёт обратную связь, Денис завершит проверку до пятницы.",
    },
  ];
  function recordingTiles() {
    return `<div class="rec-history-recordings">${recordings.map(recordingCard).join("")}</div>`;
  }
  function renderHistoryDetail({ state, role }) {
    if (state === "loading") return loading("Материалы встречи");
    if (state === "error")
      return `<div class="page rec-page">${back("history", "К истории встреч")}${error("История встречи недоступна", "Проверьте доступ к встрече или попробуйте ещё раз.")}</div>`;
    const enabled = !["capabilities-off", "capability-error"].includes(state);
    const section = [
      "transcript",
      "summary",
      "processing",
      "content-failed",
    ].includes(state)
      ? state === "summary"
        ? "summary"
        : "transcript"
      : state === "recordings"
        ? "recording"
        : "overview";
    const navItems = [
      ["overview", "Обзор", "default"],
      ["recording", "Записи", "recordings"],
      ...(enabled && capabilities.transcription
        ? [["transcript", "Расшифровка", "transcript"]]
        : []),
      ...(enabled && capabilities.aiSummary
        ? [["summary", "Итоги ИИ", "summary"]]
        : []),
      ...(enabled && capabilities.meetingAnalytics
        ? [["analytics", "Аналитика", "analytics"]]
        : []),
    ];
    const tabs = `<nav class="rec-material-nav" aria-label="Материалы встречи">${navItems.map(([id, label, destination]) => `<button class="rec-material-tab${section === id ? " rec-material-active" : ""}" ${id === "analytics" ? go("analytics") : `data-state="${destination}"`} ${section === id ? 'aria-current="page"' : ""}>${label}</button>`).join("")}</nav>`;
    let body;
    if (state === "expired") body = expired();
    else if (state === "empty")
      body = D.empty(
        "У встречи пока нет материалов",
        "История и участники сохранены. Доступные материалы появятся здесь, если запись велась.",
        "Clapperboard",
        button("К записям", "ArrowRight", go("recordings")),
      );
    else if (section === "recording") body = recordingTiles();
    else if (section === "transcript") {
      const manage = ["owner", "co_host", "cohost"].includes(role);
      body = `<section class="card rec-material-card"><div class="rec-section-head"><h2>Расшифровка встречи</h2>${D.badge("Материал выбранной записи", "neutral")}</div><p class="small muted">Демонстрационный материал из DTO-fixture. Говорящие не подтверждают личность участника.</p>${state === "processing" ? `<div class="rec-processing">${icon("LoaderCircle", 25)}<h3>Обрабатываем материал</h3><p class="muted">Можно закрыть страницу и вернуться позже.</p>${stateButton("Показать готовый материал", "transcript")}</div>` : state === "content-failed" ? `<div class="rec-processing">${D.notice("Обработка не завершена. Повторная попытка доступна организатору или соорганизатору, если её разрешил сервер.", "warning")}${manage ? stateButton("Повторить расшифровку", "processing", "RefreshCw") : '<p class="muted">У вас нет права повторной обработки.</p>'}</div>` : `<ol class="rec-transcript">${transcriptSegments.map((item) => `<li><button class="rec-timestamp" ${toast("Показано состояние перехода к выбранному фрагменту.")} aria-label="Перейти к ${Math.floor(item.startMs / 60000)}:${String(Math.floor(item.startMs / 1000) % 60).padStart(2, "0")}">${Math.floor(item.startMs / 60000)}:${String(Math.floor(item.startMs / 1000) % 60).padStart(2, "0")}</button><div><strong>${E(item.speakerLabel)}</strong><p>${E(item.text)}</p></div></li>`).join("")}</ol>${button("Ещё фрагменты", "ChevronDown", toast("Показано состояние загрузки следующих фрагментов."), "ghost")}`}</section>`;
    } else if (section === "summary") {
      const manage = ["owner", "co_host", "cohost"].includes(role);
      body = `<section class="card rec-material-card"><div class="rec-section-head"><h2>Итоги встречи</h2>${manage ? button("Пересоздать итоги", "RefreshCw", toast("Показано состояние постановки повторной обработки. Реальный запрос не отправляется."), "ghost") : ""}</div>${D.notice("Условный сценарий: функция включена, сервер разрешил текущий результат. Данные вымышлены. Автоматические итоги нужно проверять по записи.", "info")}<div class="rec-summary-body"><h3>Краткое содержание</h3><p>Команда согласовала последовательность запуска приложения и финальную проверку пользовательских сценариев.</p><h3>Ключевые моменты</h3><ul><li>Проверить регистрацию и переход к встрече.</li><li>Уточнить тексты пустых состояний.</li><li>Завершить проверку клавиатурной навигации.</li></ul><h3>Договорённости и задачи</h3><div class="rec-summary-item"><strong>Собрать обратную связь по запуску</strong><p class="small muted">Ответственный: Мария · Срок: не указан</p></div><div class="rec-summary-item"><strong>Завершить финальную проверку</strong><p class="small muted">Ответственный: Денис · Срок: 9 октября 2026</p></div><h3>Темы</h3><div class="rec-topics">${["Запуск", "Пользовательские сценарии", "Доступность"].map((item) => `<span class="chip">${item}</span>`).join("")}</div></div></section>`;
    } else
      body = `<div class="rec-overview-grid"><section class="card rec-material-card"><h2>Обзор встречи</h2><dl class="rec-info"><div><dt>Организатор</dt><dd>${E(history.owner.displayName)}</dd></div><div><dt>Длительность</dt><dd>40 мин</dd></div><div><dt>Участники встречи</dt><dd>${history.participantCount}</dd></div><div><dt>Записи для просмотра</dt><dd>${history.recordings.ready}</dd></div></dl>${button("Открыть записи встречи", "Play", 'data-state="recordings"', "primary")}</section><section class="card rec-material-card"><h2>Участники</h2><ul class="rec-participants">${people.map((person, index) => `<li>${D.avatar(person.displayName, tones[index % 4], "small")}<span>${E(person.displayName)}</span></li>`).join("")}</ul></section></div>`;
    return `<div class="page rec-page">${back("history", "К истории встреч")}${D.heading(conference.title, "8 октября 2026 · 40 мин · 8 участников", button("Чат встречи", "MessageCircle", go("history-chat"), "secondary"))}<p class="rec-secondary-context">Материалы завершённой встречи · чат только для чтения</p>${tabs}${state === "capability-error" ? D.notice("Доступность дополнительных материалов не удалось проверить. Показан обзор встречи.", "warning") : !enabled ? D.notice("Расшифровка, итоги ИИ и аналитика не включены. Записи и обзор доступны.", "info") : section !== "overview" && section !== "recording" ? "" : `<p class="small muted">Дополнительные разделы показаны только в условном сценарии с подтверждёнными возможностями сервера.</p>`}${body}${note()}</div>`;
  }
  const historicalMessages = [
    {
      id: "m1",
      sequence: "1",
      conferenceId: conference.id,
      senderId: "u1",
      senderName: "Мария Орлова",
      text: "Коллеги, начинаем с проверки сценария регистрации. Вопросы можно оставлять здесь.",
      createdAt: "2026-10-08T14:03:00Z",
      updatedAt: "2026-10-08T14:03:00Z",
    },
    {
      id: "m2",
      sequence: "2",
      conferenceId: conference.id,
      senderId: "u2",
      senderName: "Денис Волков",
      text: "Проверил переход к встрече — всё готово. Отдельно пройдём сценарий с клавиатуры.",
      createdAt: "2026-10-08T14:09:00Z",
      updatedAt: "2026-10-08T14:09:00Z",
    },
    {
      id: "m3",
      sequence: "3",
      conferenceId: conference.id,
      senderId: "u3",
      senderName: "Анна Смирнова",
      text: "Добавила замечания по текстам пустых состояний. Обсудим их после основной проверки.",
      createdAt: "2026-10-08T14:14:00Z",
      updatedAt: "2026-10-08T14:14:00Z",
    },
    {
      id: "m4",
      sequence: "4",
      conferenceId: conference.id,
      senderId: "u1",
      senderName: "Мария Орлова",
      text: "Спасибо! Договорённости зафиксированы. Запись и доступные материалы останутся в истории встречи.",
      createdAt: "2026-10-08T14:38:00Z",
      updatedAt: "2026-10-08T14:38:00Z",
    },
  ];
  function renderHistoryChat({ state }) {
    if (state === "loading") return loading("Чат завершённой встречи");
    const content =
      state === "error"
        ? error(
            "Чат встречи не загрузился",
            "Проверьте подключение и сохранённый доступ к встрече.",
          )
        : state === "empty"
          ? D.empty(
              "В чате пока нет сообщений",
              "Встреча завершена. Новые сообщения здесь отправить нельзя.",
              "MessageCircle",
            )
          : `<p class="rec-chat-date">8 октября 2026</p><ol class="rec-chat-messages">${historicalMessages.map((item, index) => `<li>${D.avatar(item.senderName, tones[index % 4], "small")}<div><div class="rec-chat-author"><strong>${E(item.senderName)}</strong><time datetime="${item.createdAt}">${new Date(item.createdAt).toLocaleTimeString("ru-RU", { timeZone: "Europe/Moscow", hour: "2-digit", minute: "2-digit" })}</time></div><p>${E(item.text)}</p></div></li>`).join("")}</ol>`;
    return `<div class="page rec-page">${back("history-detail", "К материалам встречи")}${D.heading(conference.title, "Чат завершённой встречи", D.badge("Только чтение", "neutral"))}<section class="card rec-chat-archive" aria-label="История чата встречи">${D.notice("Встреча завершена. Сообщения сохранены, отправка новых сообщений недоступна.", "info")}${content}<div class="rec-chat-footer">${button("Ранние сообщения", "ChevronUp", toast("Показано состояние загрузки ранних сообщений."), "ghost")}<p class="small muted">История доступна только участникам с сохранённым доступом.</p></div><div class="rec-readonly-composer">${icon("LockKeyhole", 18)}<span>Чат завершённой встречи доступен только для чтения</span></div></section></div>`;
  }
  const analyticsData = {
    enabled: true,
    durationMs: 2400000,
    participantCount: 8,
    recordingAvailable: true,
    transcriptAvailable: true,
    timeline: [
      { atMs: 0, count: 2 },
      { atMs: 240000, count: 6 },
      { atMs: 480000, count: 8 },
      { atMs: 960000, count: 8 },
      { atMs: 1440000, count: 7 },
      { atMs: 1920000, count: 6 },
      { atMs: 2280000, count: 3 },
    ],
    participants: [
      {
        participantId: "p3",
        displayName: "Анна Смирнова",
        participationMs: 2220000,
        speakingMs: 354000,
        observedAudioMs: 1890000,
        screenMs: 0,
        messageCount: 4,
      },
      {
        participantId: "p2",
        displayName: "Денис Волков",
        participationMs: 2400000,
        speakingMs: 486000,
        observedAudioMs: 2220000,
        screenMs: 648000,
        messageCount: 8,
      },
      {
        participantId: "p1",
        displayName: "Мария Орлова",
        participationMs: 2400000,
        speakingMs: 714000,
        observedAudioMs: 2340000,
        screenMs: 924000,
        messageCount: 6,
      },
    ],
  };
  function minutes(ms) {
    return ms ? `${Math.round(ms / 60000)} мин` : "—";
  }
  function renderAnalytics({ state }) {
    if (state === "loading") return loading("Аналитика встречи");
    const notice = D.notice(
      "Условный макет: meetingAnalytics=true и подтверждён доступ участника. Не сводка по всем встречам.",
      "info",
    );
    if (state === "error")
      return `<div class="page rec-page">${back("history-detail", "К материалам встречи")}${D.heading("Аналитика встречи", "Показатели одной выбранной встречи.")}${notice}${error("Показатели не загрузились", "Проверьте подключение и доступ к встрече.")}</div>`;
    if (["empty", "disabled", "expired"].includes(state))
      return `<div class="page rec-page">${back("history-detail", "К материалам встречи")}${D.heading("Аналитика встречи", "Показатели одной выбранной встречи.")}${notice}${D.empty(state === "disabled" ? "Аналитика сейчас недоступна" : "Показатели ещё не собраны", state === "disabled" ? "Функция отключена или её доступность не подтверждена." : "Запись и другие доступные материалы можно открыть отдельно.", "ChartNoAxesCombined", button("К материалам встречи", "ArrowRight", go("history-detail")))}</div>`;
    const total = analyticsData.participants.reduce(
      (sum, person) => sum + person.messageCount,
      0,
    );
    const table = `<div class="rec-analytics-table"><table><caption>Технические показатели участников · по алфавиту</caption><thead><tr><th scope="col">Участник</th><th scope="col">Присутствие</th><th scope="col">Речь ≈</th><th scope="col">Аудио наблюдалось</th><th scope="col">Экран</th><th scope="col">Сообщения</th></tr></thead><tbody>${analyticsData.participants.map((person) => `<tr><th scope="row">${E(person.displayName)}</th><td>${minutes(person.participationMs)}</td><td>${minutes(person.speakingMs)}</td><td>${minutes(person.observedAudioMs)}</td><td>${minutes(person.screenMs)}</td><td>${person.messageCount}</td></tr>`).join("")}</tbody></table></div>`;
    const mobile = `<div class="rec-analytics-people">${analyticsData.participants.map((person) => `<article class="rec-analytics-person"><h3>${E(person.displayName)}</h3><dl class="rec-person-metrics"><div><dt>Присутствие</dt><dd>${minutes(person.participationMs)}</dd></div><div><dt>Речь ≈</dt><dd>${minutes(person.speakingMs)}</dd></div><div><dt>Аудио наблюдалось</dt><dd>${minutes(person.observedAudioMs)}</dd></div><div><dt>Экран</dt><dd>${minutes(person.screenMs)}</dd></div><div><dt>Сообщения</dt><dd>${person.messageCount}</dd></div></dl></article>`).join("")}</div>`;
    return `<div class="page rec-page">${back("history-detail", "К материалам встречи")}${D.heading("Аналитика встречи", "Объективные показатели без оценки продуктивности.")}${notice}<section class="card rec-analytics-selector">${D.select("Статус встречи", ["Завершённые", "Активные"])}${D.select("Встреча для анализа", [conference.title, "Планирование следующего спринта"])}</section><div class="rec-stat-grid">${[
      ["Clock3", "Длительность", "40 мин"],
      ["Users", "Участники", "8"],
      ["MessageCircle", "Сообщения в показанной выборке", total],
    ]
      .map(
        ([name, label, value]) =>
          `<div class="card rec-stat"><span>${icon(name, 20)}${label}</span><strong>${value}</strong></div>`,
      )
      .join(
        "",
      )}</div><div class="rec-analytics-context"><section class="card rec-material-card"><h2>Участники во времени</h2><div class="rec-timeline" aria-hidden="true">${analyticsData.timeline.map((point) => `<span style="--rec-height:${(point.count / 8) * 100}%"></span>`).join("")}</div><div class="rec-chart-axis"><span>00:00</span><span>40:00</span></div><details class="rec-chart-values"><summary>Показать значения графика</summary><ul>${analyticsData.timeline.map((point) => `<li>${durations(Math.floor(point.atMs / 1000))} — ${point.count} участников</li>`).join("")}</ul></details></section><section class="card rec-material-card"><h2>Материалы и точность</h2><dl class="rec-info"><div><dt>Запись</dt><dd>Доступна</dd></div><div><dt>Расшифровка</dt><dd>Доступна</dd></div></dl><p class="small muted">Речь ≈ — оценка по уровню аудио, а не точное распознавание. Шум может учитываться как речь. Пропуски наблюдения не считаются молчанием.</p></section></div><section class="card rec-material-card"><h2>Показатели участников</h2>${table}${mobile}</section></div>`;
  }
  const adminData = {
    asOf: "2026-10-09T01:20:00Z",
    activeConferences: 3,
    joinedParticipants: 18,
    activeRecordings: 2,
    queuedJobs: 4,
    failedJobs24h: 1,
    failedRecordings24h: 0,
    failedTranscriptions24h: 1,
    apiReady: true,
    mediaWorkerReady: true,
    dependencies: { postgres: true, redis: true, rabbitmq: true, minio: true },
    recentFailures: [
      {
        kind: "transcription",
        code: "provider_unavailable",
        at: "2026-10-09T00:15:00Z",
      },
    ],
  };
  function renderAdmin({ state, role }) {
    if (state === "denied" || !["admin", "administrator"].includes(role))
      return `<div class="page rec-page">${D.empty("Нет доступа к разделу операций", "Нужны действующие глобальные права администратора. Роль организатора встречи их не заменяет.", "ShieldAlert", button("На главную", "ArrowLeft", go("home")))}</div>`;
    if (state === "loading") return loading("Состояние сервиса");
    const heading = D.heading(
      "Состояние сервиса",
      "Встречи, обработка записей и доступность компонентов.",
      button(
        "Обновить",
        "RefreshCw",
        toast("Показано состояние обновления сводки. Реального запроса нет."),
      ),
    );
    const notice = D.notice(
      "Условный макет для администратора: только чтение сводки. Нет управления пользователями, переключателями функций или заданиями.",
      "info",
    );
    if (state === "error")
      return `<div class="page rec-page">${heading}${notice}${error("Сводка временно недоступна", "Попробуйте загрузить данные ещё раз. Действующие права проверяет сервер.")}</div>`;
    const metrics = [
      ["Активные встречи", adminData.activeConferences],
      ["Участники активных встреч", adminData.joinedParticipants],
      ["Записи в работе", adminData.activeRecordings],
      ["Задания в очереди и обработке", adminData.queuedJobs],
      ["Ошибки заданий за 24 ч", adminData.failedJobs24h],
      ["Ошибки записей за 24 ч", adminData.failedRecordings24h],
      ["Ошибки расшифровки за 24 ч", adminData.failedTranscriptions24h],
    ];
    const health = [
      ["API", true],
      ["Media Worker", true],
      ["PostgreSQL", true],
      ["Redis", true],
      ["RabbitMQ", true],
      ["MinIO", state !== "dependency-down"],
    ];
    const failures = state === "empty" ? [] : adminData.recentFailures;
    return `<div class="page rec-page">${heading}${notice}<p class="small muted">Данные на ${date(adminData.asOf)} · демонстрационная сводка</p><section><h2 class="section-title">Нагрузка и ошибки</h2><div class="rec-admin-metrics">${metrics.map(([label, value]) => `<div class="card rec-stat"><span>${label}</span><strong>${value}</strong></div>`).join("")}</div></section><section class="card rec-material-card"><h2>Доступность компонентов</h2><ul class="rec-health">${health.map(([label, ready]) => `<li><span>${label}</span>${D.badge(ready ? "Готов" : "Недоступен", ready ? "success" : "danger")}</li>`).join("")}</ul></section><section class="card rec-material-card"><h2>Последние ошибки</h2>${failures.length ? `<ul class="rec-failures">${failures.map((item) => `<li><time datetime="${item.at}">${date(item.at)}</time><span>${E(item.kind)}</span><code>${E(item.code)}</code></li>`).join("")}</ul>` : '<p class="muted">За последние 24 часа ошибок нет.</p>'}</section></div>`;
  }
  const common = {
    layout: "app",
    notes: [
      "Самостоятельный дизайн-прототип; DTO-fixtures вымышлены. Сетевые запросы, сохранение и файлы не выполняются.",
      "44×44px hit targets. Реальный доступ подтверждается backend, а не наличием скопированной ссылки.",
      "7-дневная доступность записей; без поля expiresAt и без выдуманного таймера.",
    ],
    responsive: {
      desktop:
        "1440–1920: общая plum-sidebar, глобальная панель 44 px + header 64 px = 108 px; gutter 32 px. Редакционные превью и компактные строки; единая информационная панель плеера 290 px.",
      tablet:
        "768–1023: общая компактная sidebar; глобальная панель 44 px + header 64 px = 108 px; gutter 24 px. Плеер полной ширины, затем информация и материалы; строки перестраиваются, не уменьшая текст.",
      mobile:
        "360–767: общая панель 44 px + header 64 px = 108 px; gutter 16 px, bottom nav ≥72 px + safe area. Превью и карточки полной ширины, действия от 44×44 px; последовательность player → info → materials.",
    },
  };
  D.register([
    {
      ...common,
      id: "recordings",
      title: "Записи",
      group: "Материалы",
      nav: "recordings",
      route: "/recordings?conference=:id",
      states: [
        ...responseStates,
        ...states(
          ["active", "Встреча в эфире"],
          ["filtered", "Пустой результат фильтра"],
          ["oldest", "Сначала старые"],
        ),
      ],
      api: [
        "GET /api/v1/me/conferences?view=past|active",
        "GET /api/v1/conferences/:id",
        "GET /api/v1/conferences/:id/recordings?limit=20&offset=0",
        "GET /api/v1/conferences/:id/history",
      ],
      permissions:
        "Авторизованный допущенный участник joined/left. Глобального каталога нет.",
      sources: sources(
        "frontend/src/pages/RecordingsPage.tsx:107",
        "frontend/src/components/RecordingActions.tsx:11",
        "internal/infrastructure/postgres/record_retention.go:14",
      ),
      render: renderRecordings,
    },
    {
      ...common,
      id: "recording-player",
      title: "Просмотр записи",
      group: "Материалы",
      nav: "recordings",
      route: "/recordings/:id?conference=:conferenceId",
      states: [
        ...responseStates,
        ...states(
          ["audio", "Аудиозапись"],
          ["tracks", "Аудиомикс и архив дорожек"],
        ),
      ],
      api: [
        "GET /api/v1/conferences/:id",
        "GET /api/v1/conferences/:id/recordings/:recordingId",
        "GET /api/v1/conferences/:id/history",
      ],
      permissions:
        "Сервер подтверждает связь записи и встречи, current admitted joined/left membership.",
      sources: sources(
        "frontend/src/pages/RecordingDetailPage.tsx:45",
        "frontend/src/pages/RecordingDetailPage.tsx:268",
        "internal/usecase/recorder/service.go:882",
        "internal/usecase/recorder/composite.go:509",
      ),
      notes: [
        ...common.notes,
        "Без вкладок AI/расшифровки, избранного и удаления. Нативный медиаэлемент без src честно показывает дизайн, а не реальное воспроизведение.",
        "Архив дорожек показан только в реальном режиме individual_tracks; final_audio + tracks_archive, без вымышленного превью или MP4.",
      ],
      render: renderPlayer,
    },
    {
      ...common,
      id: "history",
      title: "История встреч · вторичный экран",
      group: "Материалы",
      nav: "recordings",
      route: "/history",
      secondary: true,
      restricted: true,
      states: responseStates,
      api: ["GET /api/v1/me/conferences?view=past"],
      permissions: "Только доступные пользователю finished/cancelled встречи.",
      sources: sources(
        "frontend/src/pages/AccountPages.tsx:317",
        "frontend/src/components/Layout.tsx:157",
      ),
      notes: [
        ...common.notes,
        "Нет самостоятельного History пункта в глобальной навигации; вход через материалы встречи.",
      ],
      render: renderHistory,
    },
    {
      ...common,
      id: "history-detail",
      title: "Материалы встречи",
      group: "Материалы",
      nav: "recordings",
      route: "/history/:id",
      secondary: true,
      states: [
        ...responseStates,
        ...states(
          ["recordings", "Записи встречи"],
          ["transcript", "Условная расшифровка"],
          ["summary", "Условные итоги ИИ"],
          ["processing", "Материал обрабатывается"],
          ["content-failed", "Ошибка обработки"],
          ["capabilities-off", "Возможности отключены"],
          ["capability-error", "Флаги не подтверждены"],
        ),
      ],
      api: [
        "GET /api/v1/conferences/:id/history",
        "GET /api/v1/conferences/:id/participants/me",
        "GET /api/v1/capabilities",
        "GET /api/v1/conferences/:id/recordings",
        "GET /api/v1/conferences/:id/recordings/:recordingId/transcript",
        "GET /api/v1/conferences/:id/recordings/:recordingId/summary",
        "POST /api/v1/conferences/:id/recordings/:recordingId/transcript/retry",
        "POST /api/v1/conferences/:id/recordings/:recordingId/summary/regenerate",
      ],
      permissions:
        "Исторический admitted joined/left участник. AI/STT только при подтверждённых flags; reprocess owner/co_host + server canRetry/canRegenerate.",
      sources: sources(
        "frontend/src/pages/HistoryDetailPage.tsx:47",
        "frontend/src/components/RecordingInsights.tsx:429",
        "internal/infrastructure/postgres/conference_product_repository.go:224",
      ),
      notes: [
        ...common.notes,
        "Один уровень material navigation; нет вложенных дублирующих tabs. AI/STT сценарии условные, mock результат обозначен вымышленным.",
        "Чат закрытой встречи только для чтения; участники — historical memberships, не peak attendance.",
      ],
      render: renderHistoryDetail,
    },
    {
      ...common,
      id: "history-chat",
      title: "Чат завершённой встречи · только чтение",
      group: "Материалы",
      nav: "recordings",
      route: "/conferences/:id",
      secondary: true,
      restricted: true,
      states: states(
        ["default", "История сообщений"],
        ["loading", "Загрузка"],
        ["empty", "Нет сообщений"],
        ["error", "Ошибка запроса"],
      ),
      api: [
        "GET /api/v1/conferences/:id",
        "GET /api/v1/conferences/:id/participants/me",
        "GET /api/v1/conferences/:id/messages",
      ],
      permissions:
        "Исторический admitted joined/left участник; finished/cancelled conference; chatReadOnly=true.",
      sources: sources(
        "frontend/src/pages/HistoryDetailPage.tsx:124",
        "frontend/src/components/conference/ConferenceOverview.tsx:130",
        "frontend/src/components/ChatPanel.tsx:50",
      ),
      notes: [
        ...common.notes,
        "Настоящий существующий secondary route, без live media controls, начала записи или editable composer.",
        "История сообщений не получает 7-дневный срок хранения записей. Перечень вымышленных сообщений соответствует ChatMessage DTO.",
      ],
      render: renderHistoryChat,
    },
    {
      ...common,
      id: "analytics",
      title: "Аналитика · условный экран",
      group: "Условные возможности",
      nav: null,
      route: "/analytics?conference=:id",
      restricted: true,
      conditional:
        "meetingAnalytics=true AND current participant can read history",
      states: states(
        ["default", "Возможность подтверждена"],
        ["loading", "Загрузка"],
        ["empty", "Агрегаты не готовы"],
        ["error", "Ошибка"],
        ["disabled", "Возможность выключена"],
      ),
      api: [
        "GET /api/v1/capabilities",
        "GET /api/v1/me/conferences?view=past|active",
        "GET /api/v1/conferences/:id/analytics",
      ],
      permissions:
        "Допущенный joined/left участник + подтверждённый meetingAnalytics. Не роль администратора и не глобальные KPI.",
      sources: sources(
        "frontend/src/pages/AnalyticsPage.tsx:13",
        "frontend/src/components/AnalyticsPanel.tsx:67",
        "internal/infrastructure/postgres/analytics_repository.go:66",
      ),
      notes: [
        ...common.notes,
        "Речь ≈ — приблизительное наблюдение; нет рейтинга/оценки продуктивности. Метрики отдельных участников идут по алфавиту.",
        "Нет пункта аналитики в глобальном меню макета. Вход из условного материала встречи; это не доказательство включённых production flags.",
      ],
      render: renderAnalytics,
    },
    {
      ...common,
      id: "admin",
      title: "Операции · только администратор",
      group: "Условные возможности",
      nav: null,
      route: "/admin",
      restricted: true,
      conditional:
        "Persisted user.isAdmin=true, rechecked on each backend request",
      states: states(
        ["default", "Администратор"],
        ["loading", "Загрузка"],
        ["empty", "Нет последних ошибок"],
        ["error", "Ошибка запроса"],
        ["dependency-down", "Компонент недоступен"],
        ["denied", "Нет прав"],
      ),
      api: ["GET /api/v1/admin/summary", "GET /api/v1/capabilities"],
      permissions:
        "Глобальные действующие административные права; никакой связи с ролью owner/co_host.",
      sources: sources(
        "frontend/src/pages/AdminPage.tsx:18",
        "internal/transport/http/middleware/admin.go:16",
        "internal/transport/http/platform_status_routes.go:17",
      ),
      notes: [
        ...common.notes,
        "Read-only: только обновление сводки, без users/config/job mutation.",
        "Для полноценного состояния выберите роль admin. Участнику/организатору показан отказ. Глобальной admin-навигации нет.",
      ],
      render: renderAdmin,
    },
  ]);
})();
