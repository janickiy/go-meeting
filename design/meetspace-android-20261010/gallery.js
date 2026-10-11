(() => {
  "use strict";
  const screens = [
    {
      id: "home",
      label: "Главная",
      group: "workspace",
      title: "Главная, звонки, чаты и профиль.",
      description:
        "Главная сохранена. Звонки, чаты и профиль оформлены по мобильным референсам.",
    },
    {
      id: "calls",
      label: "Звонки",
      group: "workspace",
      title: "Звонки — в отдельной вкладке.",
      description:
        "Новый звонок, подключение по ссылке, планирование и история разговоров.",
    },
    {
      id: "messages",
      label: "Сообщения",
      group: "workspace",
      title: "Разговор, который объединяет.",
      description:
        "Личные диалоги и командные обсуждения. Важное всегда рядом.",
    },
    {
      id: "settings",
      label: "Профиль",
      group: "workspace",
      title: "Ваш профиль и настройки.",
      description:
        "Профиль, уведомления и подключение к серверу вашей команды.",
    },
    {
      id: "thread",
      label: "Диалог",
      group: "workspace",
      title: "От идеи — к общему решению.",
      description:
        "Контекст, файлы и быстрый переход к звонку внутри командного диалога.",
    },
    {
      id: "calendar",
      label: "Календарь",
      group: "workspace",
      title: "Больше ясности в вашем дне.",
      description: "Планируйте встречи и находите время для главного.",
    },
    {
      id: "recordings",
      label: "Записи",
      group: "workspace",
      title: "Хорошие идеи остаются с вами.",
      description:
        "Записи встреч — в библиотеке команды. Найдите нужный разговор и вернитесь к нему.",
    },
    {
      id: "recording",
      label: "Просмотр записи",
      group: "workspace",
      title: "Вернитесь к важному моменту.",
      description:
        "Просмотр записи, информация о встрече и управление доступом.",
    },
    {
      id: "prejoin",
      label: "Перед встречей",
      group: "meeting",
      title: "Минута на подготовку.",
      description:
        "Проверьте камеру и звук, прежде чем присоединиться к команде.",
    },
    {
      id: "call",
      label: "Видеовстреча",
      group: "meeting",
      title: "Ближе, даже на расстоянии.",
      description: "Участники, чат и управление встречей — всё под рукой.",
    },
    {
      id: "welcome",
      label: "Добро пожаловать",
      group: "onboarding",
      title: "Ваше место для совместной работы.",
      description:
        "Знакомство с MeetSpace и выбор способа подключения к команде.",
    },
    {
      id: "connect",
      label: "Подключение",
      group: "onboarding",
      title: "Найдите своё пространство.",
      description:
        "Подключение к существующему серверу по адресу или приглашению.",
    },
    {
      id: "login",
      label: "Вход",
      group: "onboarding",
      title: "Команда уже ждёт вас.",
      description: "Войдите в своё рабочее пространство и продолжите общение.",
    },
    {
      id: "deploy",
      label: "Создание сервера",
      group: "onboarding",
      title: "Ваш сервер. Ваше пространство.",
      description: "Понятная настройка собственного сервера для вашей команды.",
    },
    {
      id: "fingerprint",
      label: "Проверка ключа",
      group: "onboarding",
      title: "Доверие начинается с проверки.",
      description:
        "Подтвердите отпечаток ключа перед первым подключением к серверу.",
    },
    {
      id: "preflight",
      label: "Проверка сервера",
      group: "onboarding",
      title: "Убедимся, что всё готово.",
      description: "Проверка соединения и ресурсов перед установкой MeetSpace.",
    },
    {
      id: "install",
      label: "Установка",
      group: "onboarding",
      title: "Ваше пространство создаётся.",
      description: "Прозрачный ход установки: понятные этапы и текущий статус.",
    },
    {
      id: "complete",
      label: "Всё готово",
      group: "onboarding",
      title: "Добро пожаловать в ваше пространство.",
      description:
        "Демонстрация успешной установки. Сервер фактически не разворачивается.",
    },
    {
      id: "install-error",
      label: "Ошибка установки",
      group: "onboarding",
      title: "Понятный следующий шаг.",
      description:
        "Помогаем разобраться с ошибкой и безопасно продолжить установку.",
    },
  ];
  const params = new URLSearchParams(location.search);
  let current =
    screens.find((s) => s.id === params.get("screen")) || screens[0];
  let theme = params.get("theme") === "dark" ? "dark" : "light";
  let view = ["compare", "phone", "tablet"].includes(params.get("view"))
    ? params.get("view")
    : innerWidth <= 760
      ? "phone"
      : "compare";
  let portrait = params.get("orientation") === "portrait";
  const $ = (id) => document.getElementById(id);
  const frames = [$("phone-preview"), $("tablet-preview")];
  const initialPreview = `preview.html?screen=${current.id}&theme=${theme}`;
  frames.forEach((frame) => {
    if (frame.getAttribute("src") !== initialPreview)
      frame.src = initialPreview;
  });
  const icons = window.MeetrixIcons || {};
  const icon = (name, fallback) => icons[name] || fallback;
  $("rotate-tablet").innerHTML = icon("RotateCcw", "↻");
  $("screen-select").innerHTML = ["workspace", "meeting", "onboarding"]
    .map(
      (group) =>
        `<optgroup label="${{ workspace: "Рабочее пространство", meeting: "Встреча", onboarding: "Первый запуск" }[group]}">${screens
          .filter((s) => s.group === group)
          .map((s) => `<option value="${s.id}">${s.label}</option>`)
          .join("")}</optgroup>`,
    )
    .join("");

  function updateURL() {
    const url = new URL(location.href);
    url.searchParams.set("screen", current.id);
    url.searchParams.set("theme", theme);
    url.searchParams.set("view", view);
    if (portrait) url.searchParams.set("orientation", "portrait");
    else url.searchParams.delete("orientation");
    history.replaceState(null, "", url);
    $("open-screen").href = `preview.html?screen=${current.id}&theme=${theme}`;
  }
  function notifyFrames(except) {
    frames.forEach((frame) => {
      if (frame.contentWindow && frame.contentWindow !== except)
        frame.contentWindow.postMessage(
          { type: "ms:screen", screen: current.id, theme },
          "*",
        );
    });
  }
  function renderNavigation() {
    $("screen-title").textContent = current.title;
    $("screen-description").textContent = current.description;
    $("screen-select").value = current.id;
    $("screen-count").textContent =
      `${String(screens.indexOf(current) + 1).padStart(2, "0")} / ${screens.length}`;
    document.querySelectorAll("[data-group]").forEach((button) => {
      button.classList.toggle("active", button.dataset.group === current.group);
      button.setAttribute(
        "aria-pressed",
        String(button.dataset.group === current.group),
      );
    });
    $("screen-tabs").innerHTML = screens
      .filter((s) => s.group === current.group)
      .map(
        (s) =>
          `<button class="screen-tab${s.id === current.id ? " active" : ""}" data-screen="${s.id}" aria-current="${s.id === current.id ? "page" : "false"}">${s.label}</button>`,
      )
      .join("");
    document.title = `${current.label} · MeetSpace — Android Concept`;
  }
  function navigate(screen, source) {
    const next = screens.find((s) => s.id === screen);
    if (!next) return;
    current = next;
    renderNavigation();
    updateURL();
    notifyFrames(source);
  }
  function applyTheme(options = {}) {
    document.body.dataset.theme = theme;
    $("theme-toggle").innerHTML =
      theme === "light" ? icon("Moon", "☾") : icon("Sun", "☀");
    $("theme-toggle").setAttribute(
      "aria-label",
      theme === "light" ? "Включить тёмную тему" : "Включить светлую тему",
    );
    $("theme-toggle").setAttribute("aria-pressed", String(theme === "dark"));
    updateURL();
    if (options.broadcast !== false) notifyFrames(options.except);
  }
  function sizeDevices() {
    const boardWidth =
      $("device-board").clientWidth - (innerWidth > 760 ? 10 : 0);
    const tabletWidth = portrait ? 834 : 1194;
    const tabletHeight = portrait ? 1194 : 834;
    const phoneOuter = { width: 408, height: 862 };
    const tabletOuter = { width: tabletWidth + 18, height: tabletHeight + 18 };
    const stacked = innerWidth <= 760;
    const gap = innerWidth <= 1120 ? 30 : 46;
    const maxSingleScale = view === "phone" ? 0.91 : 0.93;
    let phoneScale = Math.min(maxSingleScale, boardWidth / phoneOuter.width);
    let tabletScale = Math.min(maxSingleScale, boardWidth / tabletOuter.width);
    if (view === "compare" && !stacked)
      phoneScale = tabletScale = Math.min(
        0.9,
        (boardWidth - gap) / (phoneOuter.width + tabletOuter.width),
      );
    if (stacked && view === "compare")
      phoneScale = Math.min(0.91, boardWidth / phoneOuter.width);
    frames[1].width = tabletWidth;
    frames[1].height = tabletHeight;
    $("tablet-resolution").textContent = `${tabletWidth} × ${tabletHeight}`;
    [
      ["phone", phoneOuter, phoneScale],
      ["tablet", tabletOuter, tabletScale],
    ].forEach(([id, dimensions, scale]) => {
      $(id + "-stage").style.width = `${dimensions.width * scale}px`;
      $(id + "-stage").style.height = `${dimensions.height * scale}px`;
      $(id + "-frame").style.width = `${dimensions.width}px`;
      $(id + "-frame").style.height = `${dimensions.height}px`;
      $(id + "-frame").style.transform = `scale(${scale})`;
    });
  }
  function applyView() {
    $("device-board").dataset.view = view;
    document.querySelectorAll("[data-view]").forEach((button) => {
      if (button.tagName === "BUTTON") {
        button.classList.toggle("active", button.dataset.view === view);
        button.setAttribute(
          "aria-pressed",
          String(button.dataset.view === view),
        );
      }
    });
    $("rotate-tablet").setAttribute("aria-pressed", String(portrait));
    $("rotate-tablet").style.visibility =
      view === "phone" ? "hidden" : "visible";
    sizeDevices();
    updateURL();
  }
  $("screen-select").addEventListener("change", (event) =>
    navigate(event.target.value),
  );
  $("screen-tabs").addEventListener("click", (event) => {
    const button = event.target.closest("[data-screen]");
    if (button) navigate(button.dataset.screen);
  });
  document
    .querySelectorAll("[data-group]")
    .forEach((button) =>
      button.addEventListener("click", () =>
        navigate(screens.find((s) => s.group === button.dataset.group).id),
      ),
    );
  document.querySelectorAll("button[data-view]").forEach((button) =>
    button.addEventListener("click", () => {
      view = button.dataset.view;
      applyView();
    }),
  );
  $("rotate-tablet").addEventListener("click", () => {
    portrait = !portrait;
    applyView();
  });
  $("theme-toggle").addEventListener("click", () => {
    theme = theme === "light" ? "dark" : "light";
    applyTheme();
  });
  frames.forEach((frame) =>
    frame.addEventListener("load", () => notifyFrames()),
  );
  window.addEventListener("message", (event) => {
    if (
      !frames.some((frame) => frame.contentWindow === event.source) ||
      !event.data ||
      typeof event.data !== "object"
    )
      return;
    if (event.data.type === "ms:navigate") {
      const next = screens.find((screen) => screen.id === event.data.screen);
      const nextTheme = ["light", "dark"].includes(event.data.theme)
        ? event.data.theme
        : theme;
      if (next && (next.id !== current.id || nextTheme !== theme)) {
        current = next;
        theme = nextTheme;
        renderNavigation();
        applyTheme({ broadcast: false });
        notifyFrames(event.source);
      }
    }
    if (event.data.type === "ms:ready") notifyFrames();
  });
  window.addEventListener("resize", sizeDevices);
  renderNavigation();
  applyTheme();
  applyView();
})();
