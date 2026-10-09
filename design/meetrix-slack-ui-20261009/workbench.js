/* Галерея меняет реальную ширину iframe: responsive правила срабатывают внутри макета. */
(() => {
  const D = window.MeetrixDesign;
  const params = new URLSearchParams(location.search);
  let current =
    D.screens.find((x) => x.id === params.get("screen")) ||
    D.screens.find((x) => x.id === "home");
  let state = params.get("state") || "default";
  let width = Number(params.get("width")) || (innerWidth < 850 ? 390 : 1440);
  let theme = params.get("theme") === "dark" ? "dark" : "light";
  let scale = Number(params.get("scale")) || 100;
  let role = params.get("role") || "owner";
  const iframe = document.getElementById("preview");
  const screenSelect = document.getElementById("screen-select");
  const stateSelect = document.getElementById("state-select");
  const groups = [...new Set(D.screens.map((x) => x.group))];
  document.getElementById("screen-list").innerHTML = groups
    .map(
      (group) =>
        `<section class="wb-nav-group"><h2>${D.esc(group)}</h2>${D.screens
          .filter((x) => x.group === group)
          .map(
            (screen) =>
              `<button data-screen="${screen.id}">${D.esc(screen.title)}</button>`,
          )
          .join("")}</section>`,
    )
    .join("");
  screenSelect.innerHTML = groups
    .map(
      (group) =>
        `<optgroup label="${D.esc(group)}">${D.screens
          .filter((x) => x.group === group)
          .map(
            (screen) =>
              `<option value="${screen.id}">${D.esc(screen.title)}</option>`,
          )
          .join("")}</optgroup>`,
    )
    .join("");
  document.getElementById("screen-count").textContent = D.screens.length;
  function height() {
    return width < 768 ? 844 : width < 1024 ? 1112 : 1000;
  }
  function fit() {
    const canvas = document.getElementById("canvas");
    const padding = innerWidth < 850 ? 20 : innerWidth < 1400 ? 32 : 40;
    const available = canvas.clientWidth - padding - 2;
    const zoom = Math.min(1, available / width);
    iframe.style.width = `${width}px`;
    iframe.style.height = `${height()}px`;
    iframe.style.transform = `scale(${zoom})`;
    const wrap = document.getElementById("frame-wrap");
    wrap.style.width = `${width * zoom}px`;
    wrap.style.height = `${height() * zoom}px`;
    document.getElementById("dimensions").textContent =
      `${width} × ${height()} · ${Math.round(zoom * 100)}% просмотра`;
  }
  function update(navigate = true) {
    const states = current.states || [{ id: "default", label: "Обычное" }];
    if (!states.some((x) => x.id === state)) state = states[0].id;
    screenSelect.value = current.id;
    stateSelect.innerHTML = states
      .map((x) => `<option value="${x.id}">${D.esc(x.label)}</option>`)
      .join("");
    stateSelect.value = state;
    document.getElementById("theme-select").value = theme;
    document.getElementById("scale-select").value = String(scale);
    document.getElementById("role-select").value = role;
    document
      .querySelectorAll("[data-screen]")
      .forEach((x) =>
        x.classList.toggle("active", x.dataset.screen === current.id),
      );
    document
      .querySelectorAll("[data-width]")
      .forEach((x) =>
        x.setAttribute(
          "aria-pressed",
          String(Number(x.dataset.width) === width),
        ),
      );
    document.getElementById("screen-title").textContent = current.title;
    document.getElementById("screen-group").textContent = current.group;
    const device = width < 768 ? "mobile" : width < 1024 ? "tablet" : "desktop";
    const notes = (current.notes || []).filter(
      (x) => !x.includes("вымышлен") && !x.includes("Вымышлен"),
    );
    document.getElementById("inspector").innerHTML =
      `<section><h2>Маршрут и доступ</h2><p><code>${D.esc(current.route || "Компонент")}</code></p><p>${D.esc(current.permissions || "По контексту использования")}</p></section><section><h2>${device === "mobile" ? "Телефон" : device === "tablet" ? "Планшет" : "Компьютер"}</h2><p>${D.esc(current.responsive?.[device] || "Адаптивные правила общей дизайн-системы.")}</p></section><section><h2>Поведение и ограничения</h2><ul>${notes.map((x) => `<li>${D.esc(x)}</li>`).join("")}</ul></section><section><h2>Проверено по коду</h2><details><summary>API и исходные компоненты</summary>${[...(current.api || []), ...(current.sources || [])].map((x) => `<p><code>${D.esc(x)}</code></p>`).join("")}</details></section><section><h2>Система</h2><div class="wb-swatch"><i style="background:#6b3f78"></i>Primary</div><div class="wb-swatch"><i style="background:#f7f5f8;border:1px solid #e7e2e9"></i>Canvas</div><p>Inter · шаг 4 px · контролы от 44 px<br>Светлая и тёмная темы · текст 75–200%</p></section>`;
    const query = new URLSearchParams({
      screen: current.id,
      state,
      theme,
      scale: String(scale),
      role,
    });
    const src = `preview.html?${query}`;
    if (navigate) iframe.src = src;
    document.getElementById("open-screen").href = src;
    history.replaceState({}, "", `?${query}&width=${width}`);
    fit();
  }
  document.addEventListener("click", (event) => {
    const screen = event.target.closest("[data-screen]");
    if (screen) {
      current = D.screens.find((x) => x.id === screen.dataset.screen);
      state = "default";
      update();
    }
    const device = event.target.closest("[data-width]");
    if (device) {
      width = Number(device.dataset.width);
      update();
    }
  });
  screenSelect.addEventListener("change", () => {
    current = D.screens.find((x) => x.id === screenSelect.value);
    state = "default";
    update();
  });
  stateSelect.addEventListener("change", () => {
    state = stateSelect.value;
    update();
  });
  document.getElementById("role-select").addEventListener("change", (event) => {
    role = event.target.value;
    update();
  });
  document
    .getElementById("theme-select")
    .addEventListener("change", (event) => {
      theme = event.target.value;
      update();
    });
  document
    .getElementById("scale-select")
    .addEventListener("change", (event) => {
      scale = Number(event.target.value);
      update();
    });
  window.addEventListener("message", (event) => {
    if (
      event.source !== iframe.contentWindow ||
      event.data?.type !== "meetrix-design-screen"
    )
      return;
    const screen = D.screens.find((x) => x.id === event.data.id);
    if (screen) {
      current = screen;
      state = event.data.state;
      theme = event.data.theme;
      update(false);
    }
  });
  new ResizeObserver(fit).observe(document.getElementById("canvas"));
  update();
})();
