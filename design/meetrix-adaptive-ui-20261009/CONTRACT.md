# Контракт автономного дизайн-прототипа

Это отдельный дизайн-артефакт, а не реализация рабочего frontend. Все данные вымышлены; сетевых API, авторизации, доступа к камере и микрофону нет. Исходники приложения не меняются.

Модули `screens-*.js` вызывают `window.MeetrixDesign.register([...])` после загрузки `runtime.js`.

Экран:

```js
{
  id: 'personal', title: 'Личные сообщения', group: 'Общение',
  nav: 'personal', route: '/personal', layout: 'app',
  states: [{ id: 'default', label: 'Обычное' }, { id: 'empty', label: 'Пусто' }, { id: 'loading', label: 'Загрузка' }, { id: 'error', label: 'Ошибка' }],
  api: ['GET /api/v1/conversations'], permissions: 'Авторизованный участник',
  sources: ['frontend/src/pages/PersonalPage.tsx:1'],
  notes: ['Конкретные interaction notes и границы данных.'],
  responsive: { desktop: '...', tablet: '...', mobile: '...' },
  render: ({ state, role, theme, width }) => '<section>...</section>'
}
```

Общий shell с sidebar/topbar/bottom nav добавляется автоматически для `layout: 'app'` (значение по умолчанию). `layout: 'bare'` — auth/prejoin/conference. `nav` — home/meetings/personal/calendar/recordings/folders. История/аналитика/admin не появляются в глобальной навигации. Настройки вызываются как modal; отдельные макеты settings показывают modal поверх home.

Общие функции `const D = window.MeetrixDesign`:

- `D.icon('Video', 20)` — имя существующего Lucide PascalCase.
- `D.button('Создать', 'Plus', 'data-go="create-meeting"', 'primary')` — kind primary/secondary/ghost/danger/icon; иконка может быть пустой строкой.
- `D.avatar('Мария Орлова', 'blue', 'large')` — tone blue/teal/purple/slate; size small/large/empty.
- `D.badge('В эфире', 'success')` — tone success/warning/danger/neutral/info.
- `D.input('Имя', 'Введите имя', '', 'text')` — native label/input, уникальные id.
- `D.select('Камера', ['Встроенная камера', 'Внешняя камера'])`.
- `D.heading('Встречи', 'Описание', '<button>...</button>')`.
- `D.notice('Текст', 'info')` — tone info/warning/danger/success.
- `D.empty('Нет сообщений', 'Начните общение.', 'MessageCircle', '<button>...</button>')`.
- `D.skeleton(3)` — повторяющиеся cards, aria-busy.
- `D.esc(text)` — экранирование пользовательского текста.

Общие CSS classes: `page`, `page-narrow`, `card`, `stack`, `row`, `spread`, `grid-two`, `grid-three`, `muted`, `small`, `eyebrow`, `divider`, `field`, `tabs`, `tab active`, `list-row`, `list-main`, `section-title`, `toolbar`, `button`, `button-primary`, `button-secondary`, `button-ghost`, `button-danger`, `icon-button`, `chip`, `form-actions`, `modal-backdrop`, `modal-panel`, `modal-header`, `modal-body`, `modal-footer`, `sr-only`.

Токены: `--primary`, `--primary-hover`, `--primary-soft`, `--bg`, `--surface`, `--surface-alt`, `--text`, `--secondary`, `--muted`, `--border`, `--success`, `--danger`, `--warning`, `--radius`, `--shadow`. Все product стили в `design.css`, тематические в отдельных `screens-*.css`. Тёмная тема через `html[data-theme="dark"]`. Размеры в rem; root 16px. Цвет secondary минимум AA. 44px controls.

Навигация прототипа: `data-go="screen-id"`; состояние: `data-state="empty"`; уведомление о локальном действии: `data-toast="Текст"`; меню: `data-menu="target-id"` + element id/hidden; диалог: `data-dialog="user-info"` (глобальный registry dialogs — root); закрыть `data-close`; переключатель `data-toggle` с `aria-pressed`; чисто визуальный disabled с пояснением рядом. Для внутренних modal/screens предпочтительны отдельные registered screens, все существуют в галерее.

Не показывать успешное реальное сохранение/отправку. Toast локальной демонстрации формулировать «Показано состояние …» либо показывать соответствующее состояние макета. Все рабочие на вид кнопки должны либо переходить к существующему макету, либо менять состояние макета, либо иметь disabled с понятной причиной.

Статические страницы без API. SVG-иконки и font assets будут подготовлены root из существующих зависимостей. Никаких CDN или сторонних картинок. Макеты можно открыть напрямую через index.html; preview.html содержит один адаптивный экран. Root делает browser QA и screenshots.
