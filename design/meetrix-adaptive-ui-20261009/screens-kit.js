/* Каталог компонентов — часть передачи дизайна, не новый раздел приложения. */
(() => {
  const D = window.MeetrixDesign;
  const swatches = [
    ["Primary", "var(--primary)"],
    ["Подложка", "var(--bg)"],
    ["Поверхность", "var(--surface)"],
    ["Основной текст", "var(--text)"],
    ["Вторичный текст", "var(--secondary)"],
    ["Граница", "var(--border)"],
    ["Успех", "var(--success)"],
    ["Ошибка", "var(--danger)"],
  ];
  D.register([
    {
      id: "ui-kit",
      title: "Компоненты и состояния",
      group: "Дизайн-система",
      nav: "",
      route: "Дизайн-артефакт; нового route в приложении нет",
      states: [
        { id: "default", label: "Компоненты" },
        { id: "loading", label: "Состояния загрузки" },
        { id: "error", label: "Ошибка и восстановление" },
      ],
      api: [],
      permissions: "Каждый компонент наследует права своего сценария.",
      sources: [
        "frontend/src/components/ui.tsx:1",
        "frontend/src/components/Layout.tsx:1",
        "frontend/src/components/ItemActions.tsx:21",
        "frontend/src/components/EmojiPicker.tsx:15",
      ],
      notes: [
        "Из существующих компонентов сохраняются поведение, права, фокус и клавиатурная навигация. Дизайн задаёт визуальные токены и responsive варианты.",
        "AppShell, Sidebar, Topbar, MobileNavigation; MeetingCard, RecordingCard, ConversationItem, FolderItem, ParticipantTile; формы, диалоги, меню, empty/error/skeleton.",
        "Default, hover и pressed реализованы стилями. Focus видим с клавиатуры. Disabled не единственный способ объяснить недоступность.",
        "Размеры текста в rem, высоты кнопок минимальные. Контент может увеличивать высоту при 200%.",
      ],
      responsive: {
        desktop: "Колонки 2–3; таблицы токенов и наборы компонентов.",
        tablet: "Две колонки при достаточной ширине, формы не уже 280px.",
        mobile:
          "Одна колонка, controls≥44px, labels≥12px. Тема и масштаб доступны в галерее.",
      },
      render: ({ state }) =>
        `<div class="page stack">${D.heading("Один язык интерфейса", "Компоненты Meetrix, собранные в систему.")}${state === "loading" ? D.skeleton(4) : state === "error" ? D.empty("Не удалось загрузить данные", "Проверьте соединение и попробуйте ещё раз.", "WifiOff", D.button("Попробовать снова", "RefreshCw", 'data-state="default"')) : `<section class="card"><div class="section-title"><h2>Цвета</h2><span class="muted small">Светлая / тёмная тема</span></div><div class="kit-swatches">${swatches.map(([label, color]) => `<div><span style="background:${color}"></span><strong>${label}</strong></div>`).join("")}</div></section><div class="grid-two"><section class="card stack"><h2>Действия</h2><div class="row">${D.button("Основное", "Plus", 'data-toast="Основное действие"')}${D.button("Вторичное", "", 'data-toast="Вторичное действие"', "secondary")}${D.button("Текстовое", "", 'data-toast="Текстовое действие"', "ghost")}</div><div class="row">${D.button("Удалить", "Trash2", 'data-toast="Перед удалением открывается подтверждение"', "danger")}${D.button("Сохраняем…", "LoaderCircle", 'disabled aria-busy="true"')}${D.button("Недоступно", "", "disabled", "secondary")}</div><p class="muted small">Кнопка от 44 px. Фокус: кольцо 3 px и отступ 3 px.</p></section><section class="card stack"><h2>Поля и выбор</h2>${D.input("Название встречи", "Введите название", "Обсуждение проекта")}${D.select("Размер текста", ["100%", "125%", "150%", "200%"])}<label class="field">Email<input type="email" value="anna@" aria-invalid="true" aria-describedby="kit-email-error"><span id="kit-email-error" class="field-error">Укажите полный email, например name@example.com</span></label><label class="switch"><input type="checkbox" checked>Подключаться с выключенным микрофоном</label></section><section class="card stack"><h2>Навигация и статусы</h2><div class="tabs" role="group" aria-label="Пример фильтров"><button class="tab active" data-toggle aria-pressed="true">Все</button><button class="tab" data-toggle aria-pressed="false">Личные</button><button class="tab" data-toggle aria-pressed="false">Группы</button></div><div class="row">${D.badge("В эфире", "success")}${D.badge("Ожидание", "warning")}${D.badge("Без уведомлений")}${D.badge("2 новых", "info")}</div><div class="row">${D.avatar("Анна Морозова")}${D.avatar("Мария Орлова", "teal")}${D.avatar("Илья Волков", "purple")}${D.avatar("Команда продукта", "slate")}</div><p class="small muted">Статус обозначается текстом и, где нужно, иконкой.</p></section><section class="card stack"><h2>Сообщения интерфейса</h2>${D.notice("Ваши изменения сохранены.", "success")}${D.notice("Некоторые встречи ещё не загружены.", "warning")}${D.notice("Не удалось загрузить файл. Повторите попытку.", "danger")}${D.notice("Во встрече идёт запись.", "info")}</section></div><section class="card"><h2>Типографика</h2><div class="kit-type"><h1>Встречи, которые сближают</h1><h2>Всё важное в одном месте</h2><p>Основной текст — Inter 16 / 24. Он остаётся читаемым при изменении размера окна.</p><p class="small muted">Вторичный текст — 14 / 21; подписи — от 12 / 18. Заголовок страницы — 28 / 35, на телефоне 24 / 30.</p></div></section><section class="card"><h2>Пространство</h2><div class="kit-spacing">${[4, 8, 12, 16, 20, 24, 32, 40, 48].map((x) => `<div><i style="width:${x}px"></i><span>${x}</span></div>`).join("")}</div><p class="small muted">Радиусы: поле и кнопка 10 px; карточка 14 px; диалог 20 px. Тени только у меню и диалогов.</p></section>`}</div>`,
    },
  ]);
})();
