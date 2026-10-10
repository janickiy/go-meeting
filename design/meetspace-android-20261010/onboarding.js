/* MeetSpace Android design prototype. No network access or credential persistence. */
(() => {
  const M = window.MS;
  const I = (name) => M.icon(name);
  const e = (value) => M.escape(String(value ?? ""));
  const S = () =>
    M.state.onboarding ||
    (M.state.onboarding = {
      server: "https://meet.example.org",
      host: "203.0.113.10",
      port: "22",
      username: "deploy",
      domain: "meet.example.org",
      auth: "password",
      stage: 2,
    });
  const logo = () =>
    '<img src="assets/brand-mark.svg" alt="" width="32" height="32"><span>MeetSpace<span class="on-logo-dot">.</span></span>';
  const demo = (text) =>
    `<div class="on-demo">${I("FlaskConical")}<span><strong>Демо-прототип</strong> · ${text || "Все проверки и результаты — пример сценария."}</span></div>`;
  const progress = (n, label = "Подключение сервера") =>
    `<div class="on-step-label"><span>${e(label)}</span><span>0${n} <span class="on-step-sep">/ 05</span></span></div><div class="on-progress-track">${[1, 2, 3, 4, 5].map((v) => `<i class="${v <= n ? "is-done" : ""}"></i>`).join("")}</div>`;
  const marks = [
    { icon: "Video", label: "Встречи", text: "Когда важен живой разговор" },
    {
      icon: "MessageCircle",
      label: "Сообщения",
      text: "Весь контекст в одном месте",
    },
    {
      icon: "ShieldCheck",
      label: "Ваш сервер",
      text: "Ваше пространство общения",
    },
  ];
  const visual = () =>
    `<div class="on-orbit-art" aria-hidden="true"><div class="on-orbit on-orbit-one"></div><div class="on-orbit on-orbit-two"></div><div class="on-orbit on-orbit-three"></div><div class="on-orbit-line"></div><div class="on-art-label">РАБОТАЕМ ВМЕСТЕ, ГДЕ БЫ МЫ НИ БЫЛИ</div><div class="on-video-tile"><div class="on-person on-person-one"><span>АК</span><i></i></div><div class="on-person on-person-two"><span>МИ</span><i></i></div><div class="on-mini-call"><span>${I("Video")}</span><span>${I("Mic")}</span><span class="on-call-red">${I("Phone")}</span></div><span class="on-art-live"><i></i>Встреча команды</span></div><div class="on-chat-bubble">${I("MessageCircle")}<div>Хорошие идеи<br><strong>начинаются здесь.</strong></div><span>09:41</span></div><div class="on-secure-node">${I("ShieldCheck")}</div><div class="on-star-node">✦</div></div>`;
  const side = (install = false) =>
    `<aside class="on-side"><a class="on-logo" href="#welcome" data-go="welcome">${logo()}</a><div class="on-side-body"><span class="on-kicker">${install ? "ВАШ СЕРВЕР. ВАШИ ПРАВИЛА." : "БЛИЖЕ К КОМАНДЕ."}</span><h2>${install ? "Своё пространство.<br>Полный контроль." : "Быть на одной<br>волне."}</h2><p>${install ? "Подключите удалённый сервер, а мы покажем каждый шаг настройки." : "Встречи, сообщения и идеи —<br>в одном пространстве."}</p>${visual()}</div><div class="on-side-footer"><span>${I("ShieldCheck")} На вашей инфраструктуре</span><span>MeetSpace для Android</span></div></aside>`;
  const frame = (content, opts = {}) =>
    `<div class="on-layout ${opts.welcome ? "on-layout-welcome" : ""}">${side(opts.install)}<main class="on-main"><div class="on-mobile-brand"><a class="on-logo" href="#welcome" data-go="welcome">${logo()}</a><span class="on-platform">ANDROID</span></div>${opts.back ? `<button class="on-back" data-go="${opts.back}">${I("ArrowLeft")} ${opts.backLabel || "Назад"}</button>` : ""}<div class="on-content">${content}</div><div class="on-bottom-note">${I("LockKeyhole")} ${opts.install ? "Данные доступа не сохраняются в макете" : "Ваше общение остаётся в вашем пространстве"}</div></main></div>`;
  const heading = (eyebrow, title, subtitle) =>
    `<div class="on-heading"><span class="on-kicker">${eyebrow}</span><h1>${title}</h1><p>${subtitle}</p></div>`;
  const field = (label, name, value, placeholder, other = "") =>
    `<label class="field on-field"><span>${label}</span><input name="${name}" value="${e(value)}" placeholder="${placeholder}" ${other}></label>`;
  const error = '<div class="on-form-error" role="alert" hidden></div>';
  const showError = (form, text) => {
    const node = form.querySelector(".on-form-error");
    node.textContent = text;
    node.hidden = false;
  };
  const register = (id, title, render, bind = () => {}) => {
    M.screens[id] = { title, group: "Первый запуск", render, bind };
  };

  register("welcome", "Добро пожаловать", () =>
    frame(
      `${heading("ЕДИНОЕ ПРОСТРАНСТВО", "Команда ближе.<br><span>Идей больше.</span>", 'MeetSpace — единое пространство<br class="on-desktop-br"> для встреч и общения.')}<div class="on-welcome-art">${visual()}</div><div class="on-choice-stack"><button class="on-choice on-choice-main" data-go="connect"><span class="on-choice-icon">${I("Link")}</span><span><strong>Подключиться к серверу</strong><small>У меня уже есть адрес MeetSpace</small></span>${I("ArrowUpRight")}</button><button class="on-choice" data-go="deploy"><span class="on-choice-icon">${I("Server")}</span><span><strong>Развернуть свой сервер</strong><small>Создать пространство на своём VPS</small></span>${I("ArrowUpRight")}</button></div><div class="on-feature-strip">${marks.map((x) => `<span>${I(x.icon)}${x.label}</span>`).join("")}</div><button class="on-demo-link" data-go="home">Посмотреть интерфейс ${I("ArrowRight")}</button>`,
      { welcome: true },
    ),
  );

  register(
    "connect",
    "Подключение к серверу",
    () =>
      frame(
        `${heading("01 / ПОДКЛЮЧЕНИЕ", "Где ваша<br>команда?", "Введите адрес сервера MeetSpace. Его можно узнать у администратора команды.")}<form id="on-connect-form" class="on-form" novalidate>${field("Адрес сервера", "server", S().server, "https://meet.example.org", 'type="url" inputmode="url" autocomplete="off" spellcheck="false" required')}<p class="on-field-hint">${I("LockKeyhole")} Защищённое подключение по HTTPS</p>${error}<div id="on-connect-result" class="on-connect-result" hidden><span class="on-success-icon">${I("Check")}</span><div><strong>Сервер найден · демо</strong><p>Пример ответа /version. Возможности сервера определяются после входа.</p></div></div><button class="button primary on-full" type="submit">Проверить адрес ${I("ArrowRight")}</button></form><div class="on-info-card">${I("Info")}<p>Каждый сервер — отдельное пространство. Аккаунт вашей команды работает на её сервере.</p></div>${demo("Проверяется формат адреса. Запрос к серверу не отправляется.")}`,
        { back: "welcome" },
      ),
    () => {
      const form = document.getElementById("on-connect-form");
      let verifiedAddress = "";
      const button = form.querySelector('button[type="submit"]');
      form.elements.server.addEventListener("input", () => {
        verifiedAddress = "";
        document.getElementById("on-connect-result").hidden = true;
        button.innerHTML = `Проверить адрес ${I("ArrowRight")}`;
      });
      form.addEventListener("submit", (evt) => {
        evt.preventDefault();
        let url;
        try {
          url = new URL(form.elements.server.value.trim());
        } catch {
          showError(form, "Введите полный адрес: https://meet.example.org");
          return;
        }
        if (
          url.protocol !== "https:" ||
          !url.hostname.includes(".") ||
          url.username ||
          url.password
        ) {
          showError(
            form,
            "Используйте HTTPS и адрес сервера без логина или пароля.",
          );
          return;
        }
        const normalized = url.href.replace(/\/$/, "");
        if (verifiedAddress === normalized) {
          M.go("login");
          return;
        }
        S().server = normalized;
        verifiedAddress = normalized;
        form.querySelector(".on-form-error").hidden = true;
        document.getElementById("on-connect-result").hidden = false;
        button.innerHTML = `Перейти ко входу ${I("ArrowRight")}`;
      });
    },
  );

  register(
    "login",
    "Вход в пространство",
    () =>
      frame(
        `${heading("02 / ВХОД В ПРОСТРАНСТВО", "С возвращением.", "Ваша команда уже здесь.")}<div class="on-server-pill"><span>${I("Server")} ${e(new URL(S().server).hostname)}</span><button data-go="connect">Изменить</button></div><form id="on-login-form" class="on-form" novalidate>${field("Электронная почта", "email", "anna@example.org", "you@company.ru", 'type="email" autocomplete="off" required')}${field("Пароль", "password", "", "Введите demo", 'type="password" autocomplete="off" required')}<p class="on-field-hint">Для просмотра макета введите пароль <strong>demo</strong>.</p>${error}<button type="submit" class="button primary on-full">Войти в MeetSpace ${I("ArrowRight")}</button></form><button class="on-text-link" data-toast="Восстановление доступа показано как сценарий. Обратитесь к администратору вашего сервера.">Не получается войти?</button>${demo("Вход демонстрационный. Данные не отправляются и не сохраняются.")}`,
        { back: "connect" },
      ),
    () => {
      const form = document.getElementById("on-login-form");
      form.addEventListener("submit", (evt) => {
        evt.preventDefault();
        if (!form.elements.email.validity.valid) {
          showError(form, "Проверьте адрес электронной почты.");
          return;
        }
        if (!form.elements.password.value) {
          showError(form, "Для демонстрации введите demo.");
          return;
        }
        form.elements.password.value = "";
        M.toast("Демонстрационное пространство открыто");
        M.go("home");
      });
    },
  );

  register(
    "deploy",
    "Настройка своего сервера",
    () =>
      frame(
        `${progress(1)}${heading("СОБСТВЕННАЯ ИНФРАСТРУКТУРА", "Начнём с сервера.", "Нужен отдельный удалённый Linux VPS с публичным IP-адресом.")}<form id="on-deploy-form" class="on-form on-form-compact" novalidate><div class="on-field-row">${field("IP-адрес или хост", "host", S().host, "203.0.113.10", 'autocomplete="off" spellcheck="false" required')}${field("Порт SSH", "port", S().port, "22", 'type="number" min="1" max="65535" inputmode="numeric" required')}</div>${field("Пользователь SSH", "username", S().username, "deploy", 'autocomplete="off" spellcheck="false" required')}<div class="on-auth-label">Способ входа</div><div class="on-segmented" role="group" aria-label="Способ входа по SSH"><button type="button" data-on-auth="password" class="${S().auth === "password" ? "is-active" : ""}">${I("LockKeyhole")} Пароль</button><button type="button" data-on-auth="key" class="${S().auth === "key" ? "is-active" : ""}">${I("KeyRound")} SSH-ключ</button></div><div id="on-auth-fields">${S().auth === "key" ? `<label class="on-file-picker">${I("FileKey")}<span><strong id="on-key-name">Выбрать приватный ключ</strong><small>Локальный файл · содержимое не читается</small></span><input name="keyfile" type="file" aria-label="Выбрать приватный ключ"></label>${field("Кодовая фраза · если есть", "passphrase", "", "Необязательно", 'type="password" autocomplete="off"')}` : field("Пароль SSH", "password", "", "Для макета: demo", 'type="password" autocomplete="off"')}<p class="on-field-hint">Используйте демонстрационные данные.</p></div>${field("Домен MeetSpace", "domain", S().domain, "meet.example.org", 'autocomplete="off" spellcheck="false" required')}<p class="on-field-hint">A-запись домена должна указывать на IP сервера.</p>${error}<button type="submit" class="button primary on-full">Продолжить ${I("ArrowRight")}</button></form>${demo("SSH-соединение не устанавливается.")}`,
        { back: "welcome", install: true },
      ),
    () => {
      document.querySelectorAll("[data-on-auth]").forEach((btn) =>
        btn.addEventListener("click", () => {
          const form = document.getElementById("on-deploy-form");
          for (const name of ["host", "port", "username", "domain"])
            S()[name] = form.elements[name].value;
          S().auth = btn.dataset.onAuth;
          M.render();
        }),
      );
      const form = document.getElementById("on-deploy-form");
      const key = form.elements.keyfile;
      if (key)
        key.addEventListener("change", () => {
          document.getElementById("on-key-name").textContent =
            key.files[0]?.name || "Выбрать приватный ключ";
        });
      form.addEventListener("submit", (evt) => {
        evt.preventDefault();
        const data = new FormData(form);
        const host = String(data.get("host") || "").trim(),
          domain = String(data.get("domain") || "").trim();
        if (!/^[a-zA-Z0-9.-]+$/.test(host) || !host.includes(".")) {
          showError(form, "Укажите IP-адрес или полное имя сервера.");
          return;
        }
        if (!form.elements.port.validity.valid) {
          showError(form, "Порт должен быть числом от 1 до 65535.");
          return;
        }
        if (
          !/^[a-zA-Z_][a-zA-Z0-9_-]*$/.test(String(data.get("username") || ""))
        ) {
          showError(form, "Укажите корректное имя пользователя SSH.");
          return;
        }
        if (!/^(?:[a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}$/.test(domain)) {
          showError(form, "Введите доменное имя, например meet.example.org.");
          return;
        }
        if (S().auth === "password" && !data.get("password")) {
          showError(form, "Для макета введите пароль demo.");
          return;
        }
        if (S().auth === "key" && !form.elements.keyfile.files.length) {
          showError(
            form,
            "Выберите демонстрационный файл ключа. Содержимое не будет прочитано.",
          );
          return;
        }
        Object.assign(S(), {
          host,
          domain,
          port: String(data.get("port")),
          username: String(data.get("username")),
          server: "https://" + domain,
        });
        form.reset();
        M.go("fingerprint");
      });
    },
  );

  register(
    "fingerprint",
    "Проверка ключа сервера",
    () =>
      frame(
        `${progress(2)}<div class="on-feature-icon">${I("Fingerprint")}</div>${heading("ИДЕНТИЧНОСТЬ СЕРВЕРА", "Знакомимся<br>безопасно.", "При первом подключении проверьте отпечаток SSH-ключа сервера.")}<div class="on-host-card"><span>${I("Server")} ${e(S().host)}<small>Порт ${e(S().port)}</small></span><div><span class="on-small-label">ED25519 · ПРИМЕР ОТПЕЧАТКА</span><code>SHA256:Z7mKp2R8wQ4bN6vY<br>cJ3xT9aL5hF1uD0eG8sW4rE2nPo</code></div></div><div class="on-info-card">${I("ShieldCheck")}<p>Сравните отпечаток с данными в панели провайдера VPS. При несовпадении прервите подключение.</p></div><label class="on-check"><input id="on-trust" type="checkbox"><span>Я проверил отпечаток и доверяю этому серверу</span></label><button id="on-trust-next" class="button primary on-full" disabled>Подтвердить сервер ${I("ArrowRight")}</button>${demo("Отпечаток приведён для дизайна и не получен от сервера.")}`,
        { back: "deploy", install: true },
      ),
    () => {
      const check = document.getElementById("on-trust"),
        btn = document.getElementById("on-trust-next");
      check.addEventListener("change", () => (btn.disabled = !check.checked));
      btn.addEventListener("click", () => M.go("preflight"));
    },
  );

  register(
    "preflight",
    "Проверка перед установкой",
    () =>
      frame(
        `${progress(3)}${heading("ПЕРЕД УСТАНОВКОЙ", "Всё по плану.", "Ознакомьтесь с примером проверки сервера и планом изменений.")}<div class="on-checklist"><div class="on-checklist-head"><strong>Проверка сервера</strong><span class="on-example-badge">Пример</span></div>${[
          {
            label: "Система и ресурсы",
            text: "Linux · 4 ядра · 8 ГБ RAM · 80 ГБ",
            icon: "Cpu",
          },
          {
            label: "Домен и сеть",
            text: e(S().domain) + " → " + e(S().host),
            icon: "Globe",
          },
          {
            label: "Среда выполнения",
            text: "Docker и Compose · пример готовой среды",
            icon: "Container",
          },
        ]
          .map(
            (x) =>
              `<div class="on-checklist-row"><span class="on-checklist-icon">${I(x.icon)}</span><div><strong>${x.label}</strong><small>${x.text}</small></div><span class="on-row-check">${I("CircleCheck")}</span></div>`,
          )
          .join(
            "",
          )}</div><div class="on-plan"><span class="on-small-label">ЧТО ПОЯВИТСЯ НА СЕРВЕРЕ</span><p>Сервисы MeetSpace, конфигурация HTTPS, медиасервис и постоянное хранилище.</p><div><span>${I("Folder")} /opt/meetspace</span><span>${I("LockKeyhole")} HTTPS</span></div></div><label class="on-check"><input id="on-consent" type="checkbox"><span>Разрешаю установку по показанному плану. В макете изменения демонстрационные.</span></label><button id="on-install-start" class="button primary on-full" disabled>Начать демо-установку ${I("ArrowRight")}</button>${demo("Ресурсы, DNS и доступность портов фактически не проверялись.")}`,
        { back: "fingerprint", install: true },
      ),
    () => {
      const check = document.getElementById("on-consent"),
        btn = document.getElementById("on-install-start");
      check.addEventListener("change", () => (btn.disabled = !check.checked));
      btn.addEventListener("click", () => {
        S().stage = 0;
        M.go("install");
      });
    },
  );

  const stages = [
    ["Подготовка сервера", "Проверка среды и каталога установки"],
    ["Загрузка компонентов", "Получение версии MeetSpace"],
    ["Настройка пространства", "Конфигурация сервисов и HTTPS"],
    ["Запуск сервисов", "Запуск контейнеров и миграций"],
    ["Проверка готовности", "HTTPS, API и медиасоединение"],
  ];
  register(
    "install",
    "Установка MeetSpace",
    () => {
      const stage = Math.max(0, Math.min(4, S().stage));
      return frame(
        `${progress(4)}${heading("СОЗДАЁМ ПРОСТРАНСТВО", "Почти на месте.", "Демонстрация пошаговой установки MeetSpace на ваш сервер.")}<div class="on-install-summary"><span class="on-install-glyph">${I("Server")}</span><div><strong>${e(S().domain)}</strong><small>${e(S().host)}</small></div><span class="on-install-percent">${stage * 20 + 10}<small>%</small></span></div><div class="on-install-track"><i style="width:${stage * 20 + 10}%"></i></div><div class="on-install-stages">${stages.map((x, i) => `<div class="on-install-stage ${i < stage ? "is-complete" : ""} ${i === stage ? "is-current" : ""}"><span class="on-stage-number">${i < stage ? I("Check") : i === stage ? '<i class="on-pulse-dot"></i>' : String(i + 1).padStart(2, "0")}</span><div><strong>${x[0]}</strong><small>${i < stage ? "Завершено · демо" : x[1]}</small></div>${i === stage ? '<span class="on-now-badge">Сейчас</span>' : ""}</div>`).join("")}</div><button class="button primary on-full" id="on-stage-next">${stage === 4 ? "Показать результат" : "Следующий этап"} ${I("ArrowRight")}</button><div class="on-install-actions"><button id="on-simulate-error">Сценарий сбоя</button><button data-go="preflight">Остановить демо</button></div>${demo("Этапы переключаются вручную. Установка на сервер не выполняется.")}`,
        { install: true },
      );
    },
    () => {
      document.getElementById("on-stage-next").addEventListener("click", () => {
        if (S().stage >= 4) M.go("complete");
        else {
          S().stage++;
          M.render();
        }
      });
      document
        .getElementById("on-simulate-error")
        .addEventListener("click", () => M.go("install-error"));
    },
  );

  register(
    "install-error",
    "Восстановление установки",
    () =>
      frame(
        `${progress(4)}<div class="on-feature-icon on-warning-icon">${I("Unplug")}</div>${heading("СЦЕНАРИЙ ВОССТАНОВЛЕНИЯ", "Связь прервалась.", "Устройство потеряло соединение с сервером. Установка может продолжаться в фоне.")}<div class="on-recovery-card"><span>${I("History")}</span><div><strong>Последний известный этап</strong><p>${stages[Math.max(0, Math.min(4, S().stage))][0]}</p><small>Сохранён в текущем сеансе макета</small></div></div><div class="on-info-card">${I("Info")}<p>При восстановлении соединения приложение должно запросить статус установки, прежде чем продолжить.</p></div><button class="button primary on-full" id="on-retry">Повторить подключение ${I("RotateCw")}</button><button class="button secondary on-full on-secondary-gap" data-go="preflight">Вернуться к плану</button>${demo("Ошибка смоделирована. Повтор вернёт к сохранённому этапу.")}`,
        { install: true },
      ),
    () => {
      document.getElementById("on-retry").addEventListener("click", () => {
        M.toast("Демо: соединение восстановлено");
        M.go("install");
      });
    },
  );

  register("complete", "Пространство готово", () =>
    frame(
      `${progress(5)}<div class="on-complete-art" aria-hidden="true"><div class="on-complete-orbit"></div><span>${I("Check")}</span><i></i><b>✦</b></div>${heading("ГОТОВО К ОБЩЕНИЮ", "Ваше пространство<br><span>готово.</span>", "Так выглядит успешное завершение установки. Дальше — вход в MeetSpace.")}<div class="on-result-address"><span class="on-small-label">АДРЕС ВАШЕГО ПРОСТРАНСТВА</span><strong>${e(S().domain)}</strong><span>${I("CircleCheck")} HTTPS · пример результата</span></div><button class="button primary on-full" data-go="login">Перейти в MeetSpace ${I("ArrowRight")}</button><button class="on-text-link" data-go="welcome">На первый экран</button>${demo("Сервер не развёрнут. Это финальный экран сценария установки.")}`,
      { install: true },
    ),
  );
})();
