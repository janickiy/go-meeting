/* Автономный дизайн-прототип. Действия меняют только локальные состояния макетов. */
(() => {
  let fieldSequence = 0;
  let toastTimer;
  let returnFocus;
  let menuTrigger;
  const screens = [];
  const esc = (value = "") =>
    String(value).replace(
      /[&<>"']/g,
      (char) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[char],
    );
  const D = (window.MeetrixDesign = {
    screens,
    esc,
    register(items) {
      for (const item of items) {
        if (screens.some((x) => x.id === item.id))
          throw Error(`Повторный экран ${item.id}`);
        screens.push(item);
      }
    },
    icon(name, size = 20) {
      const markup =
        (window.MeetrixIcons || {})[name] || (window.MeetrixIcons || {}).Circle;
      return markup
        ? markup
            .replace('width="20"', `width="${size}"`)
            .replace('height="20"', `height="${size}"`)
        : "";
    },
    button(text, icon = "", attrs = "", kind = "primary") {
      const label =
        text && !attrs.includes("aria-label")
          ? `aria-label="${esc(text)}"`
          : "";
      return `<button type="button" class="button button-${kind}" ${label} ${attrs}>${icon ? D.icon(icon, 18) : ""}${text ? `<span>${text}</span>` : ""}</button>`;
    },
    avatar(name, tone = "blue", size = "") {
      const initials = String(name)
        .trim()
        .split(/\s+/)
        .slice(0, 2)
        .map((x) => x[0])
        .join("");
      return `<span class="avatar avatar-${tone} ${size ? `avatar-${size}` : ""}" aria-hidden="true">${esc(initials)}</span>`;
    },
    badge(text, tone = "neutral") {
      return `<span class="badge badge-${tone}">${esc(text)}</span>`;
    },
    input(label, placeholder = "", value = "", type = "text") {
      const id = `field-${++fieldSequence}`;
      return `<label class="field" for="${id}">${esc(label)}<input id="${id}" type="${esc(type)}" placeholder="${esc(placeholder)}" value="${esc(value)}"></label>`;
    },
    select(label, options) {
      const id = `field-${++fieldSequence}`;
      return `<label class="field" for="${id}">${esc(label)}<select id="${id}">${options.map((x) => `<option>${esc(typeof x === "object" ? x.label : x)}</option>`).join("")}</select></label>`;
    },
    heading(title, description = "", actions = "") {
      return `<div class="page-heading"><div><h1>${esc(title)}</h1>${description ? `<p>${esc(description)}</p>` : ""}</div>${actions ? `<div class="page-heading-actions">${actions}</div>` : ""}</div>`;
    },
    notice(text, tone = "info") {
      return `<div class="notice notice-${tone}" role="${tone === "danger" ? "alert" : "status"}">${D.icon(tone === "danger" ? "CircleAlert" : tone === "warning" ? "TriangleAlert" : tone === "success" ? "CircleCheck" : "Info", 20)}<span>${text}</span></div>`;
    },
    empty(title, text = "", icon = "Inbox", button = "") {
      return `<div class="empty"><span class="empty-symbol">${D.icon(icon, 28)}</span><h2>${esc(title)}</h2>${text ? `<p>${esc(text)}</p>` : ""}${button}</div>`;
    },
    skeleton(count = 3) {
      return `<div class="skeleton-list" aria-busy="true" aria-label="Загрузка"><span class="sr-only">Загружаем содержимое</span>${Array.from({ length: count }, () => '<div class="skeleton-card" aria-hidden="true"><div class="skeleton skeleton-block"></div><div class="skeleton-lines"><div class="skeleton"></div><div class="skeleton"></div><div class="skeleton"></div></div></div>').join("")}</div>`;
    },
    go(id, state = "default") {
      const screen = screens.find((x) => x.id === id);
      if (!screen) return D.toast(`Макет «${id}» не найден`);
      const params = new URLSearchParams(location.search);
      params.set("screen", id);
      params.set("state", state);
      history.pushState({}, "", `${location.pathname}?${params}`);
      D.render();
    },
    toast(message) {
      const node = document.getElementById("toast");
      if (!node) return;
      node.textContent = message;
      node.hidden = false;
      clearTimeout(toastTimer);
      toastTimer = setTimeout(() => {
        node.hidden = true;
      }, 3500);
    },
    theme(value) {
      document.documentElement.dataset.theme = value;
      const params = new URLSearchParams(location.search);
      params.set("theme", value);
      history.replaceState({}, "", `${location.pathname}?${params}`);
    },
  });
  const nav = [
    ["home", "House", "Главная"],
    ["meetings", "Video", "Встречи"],
    ["personal", "MessageCircle", "Личные"],
    ["calendar", "CalendarDays", "Календарь"],
    ["recordings", "CirclePlay", "Записи"],
  ];
  function sidebar(current, drawer = false) {
    return `<aside class="sidebar" aria-label="Разделы Meetrix"><div class="spread"><a class="brand" href="?screen=home" data-go="home"><img src="assets/brand-mark.svg" alt=""><span>Meetrix</span></a>${drawer ? `<button class="icon-button drawer-close" data-action="close-drawer" aria-label="Закрыть меню">${D.icon("X")}</button>` : ""}</div><div><p class="nav-label">Рабочее пространство</p><nav class="side-nav" aria-label="Основная навигация">${nav.map(([id, icon, label]) => `<button class="nav-link ${current === id ? "active" : ""}" data-go="${id}" ${current === id ? 'aria-current="page"' : ""} aria-label="${label}">${D.icon(icon, 22)}<span class="nav-text">${label}</span></button>`).join("")}</nav><div class="side-divider"></div><p class="nav-label">Личное пространство</p><nav class="side-nav" aria-label="Личное пространство"><button class="nav-link ${current === "folders" ? "active" : ""}" data-go="folders" aria-label="Папки">${D.icon("Folder", 22)}<span class="nav-text">Папки</span></button></nav></div><div class="sidebar-bottom"><button class="nav-link" data-go="settings-profile" aria-label="Настройки аккаунта">${D.icon("Settings", 22)}<span>Настройки</span></button><div class="side-divider"></div><button class="profile-button identity" data-go="settings-profile" aria-label="Профиль Анны Морозовой">${D.avatar("Анна Морозова")}<div><strong>Анна Морозова</strong><small>anna@example.com</small></div></button><button class="nav-link" data-go="login" aria-label="Выйти из аккаунта">${D.icon("LogOut", 20)}<span>Выйти из аккаунта</span></button></div></aside>`;
  }
  function shell(screen, body) {
    const current = screen.nav ?? "home";
    return `<a class="skip-link" href="#workspace-main">К содержимому</a><div class="app-shell">${sidebar(current)}<div class="workspace"><header class="topbar"><span class="topbar-context">${esc(screen.group || "Рабочее пространство")}</span><a href="?screen=home" data-go="home" class="brand"><img src="assets/brand-mark.svg" alt=""><span>Meetrix</span></a><div class="topbar-right"><button class="icon-button notification-button" data-go="notifications" aria-label="Уведомления: 2 непрочитанных">${D.icon("Bell", 22)}<span class="notification-dot"></span></button><button class="profile-button identity" data-go="settings-profile" aria-label="Открыть профиль Анны Морозовой">${D.avatar("Анна Морозова")}<div><strong>Анна Морозова</strong><small>anna@example.com</small></div></button></div></header><main id="workspace-main" tabindex="-1">${body}</main></div></div><nav class="mobile-bottom-nav" aria-label="Мобильная навигация">${nav
      .filter((x) => x[0] !== "calendar")
      .map(
        ([id, icon, label]) =>
          `<button class="${current === id ? "active" : ""}" data-go="${id}" aria-label="${label}" ${current === id ? 'aria-current="page"' : ""}>${D.icon(icon, 22)}<span>${label}</span></button>`,
      )
      .join(
        "",
      )}<button data-action="drawer" aria-label="Все разделы">${D.icon("Ellipsis", 22)}<span>Ещё</span></button></nav><div class="drawer-shade" id="drawer-shade" hidden>${sidebar(current, true)}</div>`;
  }
  D.render = function () {
    const params = new URLSearchParams(location.search);
    const screen =
      screens.find((x) => x.id === params.get("screen")) ||
      screens.find((x) => x.id === "home") ||
      screens[0];
    if (!screen) return;
    fieldSequence = 0;
    const state = params.get("state") || "default";
    const theme = params.get("theme") === "dark" ? "dark" : "light";
    document.documentElement.dataset.theme = theme;
    document.documentElement.dataset.largeText = String(
      Number(params.get("scale") || 100) >= 150,
    );
    document.documentElement.style.fontSize = `${(16 * (Number(params.get("scale")) || 100)) / 100}px`;
    const context = {
      state,
      theme,
      role: params.get("role") || "owner",
      width: innerWidth,
    };
    D.current = { screen, ...context };
    const body = screen.render(context);
    document.getElementById("app").innerHTML =
      screen.layout === "bare" ? body : shell(screen, body);
    document.getElementById("dialog-root").innerHTML = "";
    document.title = `${screen.title} · Meetrix — дизайн`;
    document.body.dataset.screen = screen.id;
    const dialog = document.querySelector('[role="dialog"], .modal-backdrop');
    if (dialog) {
      if (!dialog.hasAttribute("role")) dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      let branch = dialog;
      while (branch.parentElement && branch.id !== "app") {
        for (const sibling of branch.parentElement.children)
          if (sibling !== branch) sibling.inert = true;
        branch = branch.parentElement;
      }
      if (
        !dialog.hasAttribute("aria-label") &&
        !dialog.hasAttribute("aria-labelledby")
      )
        dialog.setAttribute("aria-label", screen.title);
      dialog
        .querySelector('button, input, select, textarea, [tabindex="0"]')
        ?.focus({ preventScroll: true });
    }
    window.scrollTo(0, 0);
    if (window.parent !== window)
      window.parent.postMessage(
        {
          type: "meetrix-design-screen",
          id: screen.id,
          state,
          theme,
          height: document.documentElement.scrollHeight,
        },
        location.origin === "null" ? "*" : location.origin,
      );
    screen.afterRender?.(context);
  };
  D.close = function () {
    const root = document.getElementById("dialog-root");
    if (root?.children.length) {
      root.innerHTML = "";
      returnFocus?.focus();
      return;
    }
    const current = D.current?.screen;
    const targets = {
      "new-direct": "personal",
      "group-create": "personal",
      "group-info": "group-chat",
      "direct-info": "direct-chat",
      "direct-clear": "direct-chat",
      "direct-delete": "direct-chat",
      "folder-create": "folders",
      "create-meeting": "meetings",
      "meeting-created": "meetings",
      "recording-control": "conference",
      "conference-invite": "conference",
      "conference-devices": "conference",
      captions: "conference",
      "leave-meeting": "conference",
      "finish-meeting": "conference",
    };
    D.go(
      current?.id.startsWith("settings")
        ? "home"
        : targets[current?.id] ||
            (current?.nav === "personal" ? "personal" : current?.nav || "home"),
    );
  };
  D.start = function () {
    D.render();
    window.addEventListener("popstate", D.render);
    document.addEventListener("submit", (event) => {
      event.preventDefault();
      const target = event.target.dataset.submitGo;
      if (target) D.go(target);
      else D.toast("Показано состояние формы. Это макет.");
    });
    document.addEventListener("click", (event) => {
      const target = event.target.closest(
        "button,a,[data-go],[data-state],[data-toggle]",
      );
      if (!target || target.disabled) return;
      if (target.dataset.go) {
        event.preventDefault();
        D.go(target.dataset.go, target.dataset.goState || "default");
      } else if (target.dataset.state)
        D.go(D.current.screen.id, target.dataset.state);
      else if (target.hasAttribute("data-close")) D.close();
      else if (target.dataset.toast) D.toast(target.dataset.toast);
      else if (target.dataset.theme) {
        D.theme(target.dataset.theme);
        document.querySelectorAll("[data-theme]").forEach((x) => {
          if (x !== document.documentElement)
            x.setAttribute(
              "aria-pressed",
              String(x.dataset.theme === target.dataset.theme),
            );
        });
      } else if (target.dataset.menu) {
        const menu = document.getElementById(target.dataset.menu);
        if (menu) {
          menuTrigger = target;
          menu.hidden = !menu.hidden;
          target.setAttribute("aria-expanded", String(!menu.hidden));
          if (!menu.hidden) menu.querySelector("button")?.focus();
        }
      } else if (target.hasAttribute("data-toggle")) {
        const pressed = target.getAttribute("aria-pressed") === "true";
        target.setAttribute("aria-pressed", String(!pressed));
        target.classList.toggle("active", !pressed);
      } else if (target.dataset.action === "drawer") {
        returnFocus = target;
        const shade = document.getElementById("drawer-shade");
        shade.hidden = false;
        shade.classList.add("open");
        shade.setAttribute("role", "dialog");
        shade.setAttribute("aria-modal", "true");
        shade.setAttribute("aria-label", "Разделы Meetrix");
        shade.querySelector("button")?.focus();
      } else if (target.dataset.action === "close-drawer") {
        const shade = document.getElementById("drawer-shade");
        shade.hidden = true;
        shade.classList.remove("open");
        returnFocus?.focus();
      } else if (target.dataset.action === "send-demo") {
        const input = document.querySelector("[data-composer]");
        if (input?.value.trim()) {
          const list = document.querySelector("[data-messages]");
          const bubble = document.createElement("div");
          bubble.className = "msg-bubble msg-own";
          bubble.textContent = input.value;
          list?.append(bubble);
          input.value = "";
          bubble.scrollIntoView({ block: "nearest" });
        }
      } else if (target.dataset.emoji) {
        const input = document.querySelector("[data-composer]");
        if (input) {
          const start = input.selectionStart ?? input.value.length;
          const end = input.selectionEnd ?? start;
          input.setRangeText(target.dataset.emoji, start, end, "end");
          input.dispatchEvent(new Event("input", { bubbles: true }));
          const picker = target.closest(".dropdown");
          if (picker) picker.hidden = true;
          menuTrigger?.setAttribute("aria-expanded", "false");
          input.focus();
        }
      }
    });
    document.addEventListener("keydown", (event) => {
      if (
        event.key === "Enter" &&
        !event.shiftKey &&
        !event.isComposing &&
        event.target.matches("[data-composer]")
      ) {
        event.preventDefault();
        document.querySelector('[data-action="send-demo"]')?.click();
      }
      const activeMenu = event.target.closest(".dropdown:not([hidden])");
      if (
        activeMenu &&
        ["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)
      ) {
        const options = [
          ...activeMenu.querySelectorAll("button:not(:disabled)"),
        ];
        const currentIndex = options.indexOf(document.activeElement);
        const index =
          event.key === "Home"
            ? 0
            : event.key === "End"
              ? options.length - 1
              : (currentIndex +
                  (event.key === "ArrowDown" ? 1 : -1) +
                  options.length) %
                options.length;
        event.preventDefault();
        options[index]?.focus();
      }
      const overlay = [
        ...document.querySelectorAll('[role="dialog"],.modal-backdrop'),
      ].find((x) => !x.hidden && x.getClientRects().length);
      if (event.key === "Escape") {
        const menus = [...document.querySelectorAll(".dropdown:not([hidden])")];
        if (menus.length) {
          menus.forEach((x) => (x.hidden = true));
          menuTrigger?.setAttribute("aria-expanded", "false");
          menuTrigger?.focus();
          return;
        }
        const shade = document.getElementById("drawer-shade");
        if (shade && !shade.hidden) {
          shade.hidden = true;
          shade.classList.remove("open");
          returnFocus?.focus();
          return;
        }
        if (overlay) D.close();
      }
      if (event.key === "Tab" && overlay) {
        const focus = [
          ...overlay.querySelectorAll(
            'button:not(:disabled),a[href],input:not(:disabled),select:not(:disabled),textarea:not(:disabled),[tabindex="0"]',
          ),
        ].filter((x) => x.getClientRects().length);
        const first = focus[0],
          last = focus.at(-1);
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
    });
  };
})();
