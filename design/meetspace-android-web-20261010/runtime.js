/* Standalone UI prototype. All fixtures and interactions stay in memory. */
window.MS = {
  screens: {},
  state: {
    screen: "home",
    theme: "light",
    mic: true,
    camera: false,
    chat: "team",
    messages: [],
    filter: "all",
    day: 12,
    tab: "profile",
  },
  icon(name) {
    return window.MeetrixIcons[name] || window.MeetrixIcons.CircleCheck || "";
  },
  escape(value) {
    return String(value).replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  },
  go(id) {
    if (!this.screens[id]) return;
    this.closeModal();
    this.state.screen = id;
    this.render();
  },
  render() {
    document.documentElement.dataset.theme = this.state.theme;
    document.body.dataset.screen = this.state.screen;
    const screen = this.screens[this.state.screen] || this.screens.home;
    document.getElementById("app").innerHTML = screen.render();
    screen.bind?.();
    const url = new URL(location.href);
    url.searchParams.set("screen", this.state.screen);
    url.searchParams.set("theme", this.state.theme);
    history.replaceState(null, "", url);
    if (window.parent !== window)
      window.parent.postMessage(
        {
          type: "ms:navigate",
          screen: this.state.screen,
          theme: this.state.theme,
        },
        location.origin === "null" ? "*" : location.origin,
      );
  },
  toast(message) {
    const toast = document.getElementById("toast");
    toast.textContent = message;
    toast.hidden = false;
    clearTimeout(this.toastTimer);
    this.toastTimer = setTimeout(() => {
      toast.hidden = true;
    }, 3500);
  },
  modal(title, content, bind) {
    this.opener = document.activeElement;
    document.getElementById("dialog-root").innerHTML =
      `<div class="modal-backdrop"><section class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-head"><h2 id="modal-title">${title}</h2><button class="icon-button" data-close aria-label="Закрыть">${this.icon("X")}</button></div>${content}</section></div>`;
    document.getElementById("app").inert = true;
    document.querySelector(".modal input, .modal button")?.focus();
    bind?.();
  },
  closeModal() {
    document.getElementById("dialog-root").innerHTML = "";
    document.getElementById("app").inert = false;
    if (this.opener?.isConnected) this.opener.focus();
    this.opener = null;
  },
  start() {
    const params = new URLSearchParams(location.search);
    this.state.screen = this.screens[params.get("screen")]
      ? params.get("screen")
      : "home";
    this.state.theme = params.get("theme") === "dark" ? "dark" : "light";
    document.addEventListener("click", (e) => {
      const target = e.target.closest("button,a");
      if (!target) return;
      if (target.dataset.go) {
        e.preventDefault();
        this.go(target.dataset.go);
      }
      if (target.dataset.toast) this.toast(target.dataset.toast);
      if (target.hasAttribute("data-close")) this.closeModal();
      if (target.dataset.action)
        this.actions?.[target.dataset.action]?.(target);
    });
    document.addEventListener("keydown", (e) => {
      const modal = document.querySelector(".modal");
      if (!modal) return;
      if (e.key === "Escape") this.closeModal();
      if (e.key === "Tab") {
        const all = [
          ...modal.querySelectorAll(
            "button:not(:disabled),input:not(:disabled),select,textarea,a[href]",
          ),
        ].filter((el) => el.offsetParent !== null);
        const first = all[0],
          last = all.at(-1);
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        }
        if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    });
    window.addEventListener("message", (e) => {
      if (
        e.source !== window.parent ||
        (location.origin !== "null" && e.origin !== location.origin)
      )
        return;
      if (e.data?.type !== "ms:screen") return;
      const next = this.screens[e.data.screen]
        ? e.data.screen
        : this.state.screen;
      const theme = e.data.theme === "dark" ? "dark" : "light";
      if (next !== this.state.screen || theme !== this.state.theme) {
        this.closeModal();
        this.state.screen = next;
        this.state.theme = theme;
        this.state.themeMode = theme;
        this.render();
      }
    });
    this.render();
  },
};
