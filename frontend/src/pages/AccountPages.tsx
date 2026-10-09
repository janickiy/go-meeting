import { useEffect, useId, useRef, useState } from "react";
import type { FormEvent, KeyboardEvent } from "react";
import {
  Bell,
  ArrowLeft,
  CalendarDays,
  ChevronRight,
  Clapperboard,
  Moon,
  Search,
  Palette,
  Mic,
  Video,
  Sun,
  UserRound,
} from "lucide-react";
import { Link } from "react-router";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import { useConferences } from "../queries";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { IntegrationsSettings } from "../components/IntegrationsSettings";
import { AudioSettings, VideoSettings } from "../components/DeviceSettings";
import { TEXT_SIZE_OPTIONS, useAppearance } from "../appearance";
import type { TextSize } from "../appearance";
import "./history-notifications.css";

/** Показывает вкладки профиля, устройств, уведомлений и оформления внутри диалога аккаунта.
 * @return Содержимое настроек без интеграций и неподдерживаемого раздела безопасности.
 */
export function SettingsPage() {
  const { user, updateProfile } = useAuth();
  const [name, setName] = useState(user?.displayName || "");
  const [saving, setSaving] = useState(false);
  const [profileError, setProfileError] = useState<unknown>();
  const [profileValidation, setProfileValidation] = useState("");
  const [saved, setSaved] = useState(false);
  const appearance = useAppearance();
  const guest = Boolean(user?.guestConferenceId);
  const tabs = [
    ...(!guest ? [{ id: "profile", label: "Профиль", Icon: UserRound }] : []),
    { id: "audio", label: "Аудио", Icon: Mic },
    { id: "video", label: "Видео", Icon: Video },
    ...(!guest
      ? [{ id: "notifications", label: "Уведомления", Icon: Bell }]
      : []),
    { id: "appearance", label: "Оформление", Icon: Palette },
  ] as const;
  const [activeSection, setActiveSection] = useState<string>(() => {
    const section = window.location.hash;
    return ["#audio-video", "#audio-settings"].includes(section)
      ? "audio"
      : section === "#video-settings"
        ? "video"
        : section === "#notification-settings" && !guest
          ? "notifications"
          : section === "#appearance-settings"
            ? "appearance"
            : guest
              ? "audio"
              : "profile";
  });
  const prefix = useId();
  const tabButtons = useRef<(HTMLButtonElement | null)[]>([]);
  const [horizontalTabs, setHorizontalTabs] = useState(
    () => window.matchMedia?.("(max-width: 767px)").matches ?? false,
  );
  const [mobileSectionOpen, setMobileSectionOpen] = useState(() =>
    Boolean(window.location.hash),
  );
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => setName(user?.displayName || ""), [user?.displayName]);
  useEffect(() => {
    const media = window.matchMedia?.("(max-width: 767px)");
    if (!media) return;
    const update = () => setHorizontalTabs(media.matches);
    media.addEventListener?.("change", update);
    return () => media.removeEventListener?.("change", update);
  }, []);

  /** Переключает доступные вкладки стрелками, Home и End, не меняя адрес фоновой страницы.
   * @args event — клавиатурное событие кнопки; index — позиция текущей вкладки.
   * @return Значение не возвращается; выбранная вкладка получает фокус.
   */
  function changeTab(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === "ArrowRight" || event.key === "ArrowDown")
      next = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp")
      next = (index - 1 + tabs.length) % tabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = tabs.length - 1;
    else return;
    event.preventDefault();
    setActiveSection(tabs[next].id);
    tabButtons.current[next]?.focus();
  }

  /** Проверяет имя и сохраняет его через существующий контракт профиля.
   * @args event — отправка формы, которую обрабатывает приложение.
   * @return Завершение запроса; ошибки и подтверждение показаны рядом с полем.
   */
  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = name.trim();
    if (
      !value ||
      [...value].length > 100 ||
      /[\u0000-\u001f\u007f]/u.test(value)
    ) {
      setProfileValidation(
        "Укажите имя от 1 до 100 символов без управляющих знаков.",
      );
      setSaved(false);
      return;
    }
    setSaving(true);
    setProfileValidation("");
    setProfileError(undefined);
    setSaved(false);
    try {
      await updateProfile(value);
      setSaved(true);
    } catch (error) {
      setProfileError(error);
    } finally {
      setSaving(false);
    }
  }
  return (
    <div
      className={`account-dialog-layout ${mobileSectionOpen ? "account-mobile-section" : "account-mobile-categories"}`}
    >
      <div
        className="account-dialog-tabs"
        role="tablist"
        aria-label="Разделы настроек"
        aria-orientation="vertical"
      >
        {tabs.map(({ id, label, Icon }, index) => (
          <button
            type="button"
            key={id}
            role="tab"
            id={`${prefix}-${id}-tab`}
            aria-controls={`${prefix}-${id}-panel`}
            aria-selected={activeSection === id}
            tabIndex={activeSection === id ? 0 : -1}
            ref={(node) => {
              tabButtons.current[index] = node;
            }}
            onClick={() => {
              setActiveSection(id);
              setMobileSectionOpen(true);
              if (horizontalTabs)
                requestAnimationFrame(() => panel.current?.focus());
            }}
            onKeyDown={(event) => changeTab(event, index)}
          >
            <Icon size={18} aria-hidden="true" />
            <span>{label}</span>
            <ChevronRight
              className="account-tab-chevron"
              size={18}
              aria-hidden="true"
            />
          </button>
        ))}
      </div>
      <div
        className={`account-dialog-panel settings-section-content ${["audio", "video"].includes(activeSection) ? "account-media-panel" : ""}`}
        role="tabpanel"
        id={`${prefix}-${activeSection}-panel`}
        aria-labelledby={`${prefix}-${activeSection}-tab`}
        tabIndex={0}
        ref={panel}
      >
        <button
          className="account-settings-back"
          type="button"
          onClick={() => {
            setMobileSectionOpen(false);
            requestAnimationFrame(() =>
              tabButtons.current[
                tabs.findIndex((tab) => tab.id === activeSection)
              ]?.focus(),
            );
          }}
        >
          <ArrowLeft size={18} aria-hidden="true" /> Все настройки
        </button>
        {activeSection === "profile" && (
          <section
            className="content-card account-card"
            id="profile-settings"
            aria-labelledby="profile-heading"
          >
            <div className="profile-section-heading">
              <div>
                <h2 id="profile-heading">Ваш профиль</h2>
                <p className="field-hint">Как вас видят участники встречи.</p>
              </div>
              <span className="profile-initials" aria-hidden="true">
                {(user?.displayName || user?.email || "?")
                  .trim()
                  .slice(0, 1)
                  .toLocaleUpperCase("ru-RU")}
              </span>
            </div>
            <form onSubmit={(event) => void saveProfile(event)}>
              <label className="field">
                <span>Имя для встреч</span>
                <input
                  autoComplete="name"
                  value={name}
                  onChange={(event) => {
                    setName(event.target.value);
                    setSaved(false);
                    setProfileValidation("");
                  }}
                  required
                  aria-describedby="profile-name-hint"
                />
              </label>
              <p id="profile-name-hint" className="field-hint">
                От 1 до 100 символов.
              </p>
              <label className="field">
                <span>Email</span>
                <input
                  type="email"
                  value={user?.email || ""}
                  readOnly
                  aria-describedby="profile-email-hint"
                />
              </label>
              <p id="profile-email-hint" className="field-hint">
                Адрес используется для входа. Изменение email пока недоступно.
              </p>
              <ErrorNotice>{profileValidation || null}</ErrorNotice>
              <ErrorNotice error={profileError} />
              {saved && <p role="status">Имя сохранено.</p>}
              <div className="profile-save-row">
                <Button
                  type="submit"
                  busy={saving}
                  disabled={name.trim() === (user?.displayName || "")}
                >
                  Сохранить имя
                </Button>
              </div>
            </form>
            <p className="profile-created">
              <CalendarDays size={16} aria-hidden="true" /> Регистрация:{" "}
              {user && formatDate(user.createdAt)}
            </p>
          </section>
        )}
        {activeSection === "audio" && <AudioSettings />}
        {activeSection === "video" && <VideoSettings />}
        {activeSection === "notifications" && (
          <IntegrationsSettings notificationsOnly />
        )}
        {activeSection === "appearance" && (
          <div className="appearance-settings" id="appearance-settings">
            <div className="account-settings-section-heading">
              <h2>Оформление</h2>
              <p>Настройте Meetrix под себя</p>
            </div>
            <fieldset className="appearance-theme-block">
              <legend>Тема</legend>
              <div className="appearance-theme-options">
                {(["light", "dark"] as const).map((theme) => (
                  <label
                    key={theme}
                    className={`appearance-theme-option ${appearance.theme === theme ? "appearance-theme-selected" : ""}`}
                  >
                    <span
                      className={`appearance-theme-preview appearance-preview-${theme}`}
                      aria-hidden="true"
                    >
                      <span />
                      <span />
                      <span />
                    </span>
                    <span className="appearance-theme-name">
                      {theme === "light" ? (
                        <Sun size={18} aria-hidden="true" />
                      ) : (
                        <Moon size={18} aria-hidden="true" />
                      )}
                      {theme === "light" ? "Светлая тема" : "Тёмная тема"}
                    </span>
                    <input
                      type="radio"
                      name={`${prefix}-theme`}
                      value={theme}
                      checked={appearance.theme === theme}
                      onChange={() => appearance.setTheme(theme)}
                    />
                  </label>
                ))}
              </div>
            </fieldset>
            <section
              className="appearance-text-block"
              aria-labelledby={`${prefix}-text-heading`}
            >
              <h2 id={`${prefix}-text-heading`}>Внешний вид</h2>
              <label className="field appearance-text-size">
                <span>Размер текста</span>
                <select
                  value={appearance.textSize}
                  onChange={(event) => {
                    const value = Number(event.target.value) as TextSize;
                    if (TEXT_SIZE_OPTIONS.includes(value))
                      appearance.setTextSize(value);
                  }}
                >
                  {TEXT_SIZE_OPTIONS.map((value) => (
                    <option key={value} value={value}>
                      {value}%{value === 100 ? " (по умолчанию)" : ""}
                    </option>
                  ))}
                </select>
              </label>
              <div className="appearance-text-example">
                <strong>Удобный размер для важных разговоров</strong>
                <p>Все ваши встречи и сообщения — в одном месте.</p>
              </div>
            </section>
            <p className="field-hint">
              Оформление применяется сразу и сохраняется для вашего аккаунта в
              этом браузере.
            </p>
            {appearance.persistenceError && (
              <p role="status">
                Браузер не разрешил сохранить настройки. Оформление действует до
                перезагрузки страницы.
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/** Читает историю из общего серверного списка без дополнительных запросов для каждой строки.
 * @return Список доступных завершённых встреч с локальным поиском по загруженным страницам.
 */
export function HistoryPage() {
  const query = useConferences({ view: "past" });
  const [search, setSearch] = useState("");
  const conferences = query.data?.pages.flatMap((page) => page.items) || [];
  const visible = conferences.filter((item) =>
    item.title
      .toLocaleLowerCase("ru-RU")
      .includes(search.trim().toLocaleLowerCase("ru-RU")),
  );
  return (
    <>
      <section className="page-heading">
        <div>
          <h1>История встреч</h1>
          <p>Завершённые встречи, записи и материалы — в одном месте.</p>
        </div>
      </section>
      <section
        className="content-card history-library"
        aria-labelledby="history-list-heading"
      >
        <div className="history-library-toolbar">
          <h2 id="history-list-heading">Завершённые встречи</h2>
          <label className="history-search">
            <Search size={17} aria-hidden="true" />
            <input
              type="search"
              aria-label="Поиск по загруженным встречам"
              placeholder="Поиск по названию"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
        </div>
        <p className="field-hint">
          Откройте встречу, чтобы увидеть доступные записи, расшифровку, итоги и
          аналитику.
          {search && query.hasNextPage
            ? " Поиск выполняется по загруженным встречам; загрузите следующие страницы, чтобы расширить результат."
            : ""}
        </p>
        <ErrorNotice error={query.error} />
        {query.isPending ? (
          <Loading />
        ) : (
          <div className="history-library-list">
            {visible.map((item) => (
              <Link
                className="history-library-row"
                key={item.id}
                to={`/history/${item.id}`}
              >
                <span className="history-row-art" aria-hidden="true">
                  <Clapperboard size={24} />
                </span>
                <div>
                  <strong>{item.title}</strong>
                  <p>
                    {formatDate(item.finishedAt || item.createdAt)}
                    {item.startedAt && item.finishedAt
                      ? ` · ${Math.max(0, Math.round((Date.parse(item.finishedAt) - Date.parse(item.startedAt)) / 60000))} мин`
                      : ""}
                  </p>
                </div>
                <span className="history-row-open">
                  Открыть историю <ChevronRight size={16} aria-hidden="true" />
                </span>
              </Link>
            ))}
          </div>
        )}
        {!query.isPending && !query.isError && !visible.length && (
          <div className="history-empty">
            <Clapperboard size={32} aria-hidden="true" />
            <h3>
              {search ? "Совпадений не найдено" : "Завершённых встреч пока нет"}
            </h3>
            <p className="muted">
              {search
                ? "Измените запрос или загрузите ещё встречи."
                : "После встречи здесь появится её история и доступные материалы."}
            </p>
          </div>
        )}
        <div className="history-library-footer">
          {query.hasNextPage && (
            <Button
              variant="outline"
              busy={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              Ещё встречи
            </Button>
          )}
          <Link className="text-link" to="/meetings?view=past">
            Вся история встреч
          </Link>
        </div>
      </section>
    </>
  );
}
