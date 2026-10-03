import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  Bell,
  CalendarDays,
  ChevronRight,
  Clapperboard,
  Link2,
  Mail,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  UserRound,
} from "lucide-react";
import { Link } from "react-router";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import { useConferences } from "../queries";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { IntegrationsSettings } from "../components/IntegrationsSettings";
import { DeviceSettings } from "../components/DeviceSettings";
import "./history-notifications.css";

/** Показывает реальные настройки профиля, устройств и подключений без неподдерживаемых полей.
 * @return Страница с доступной навигацией к разделам и формой обновления имени.
 */
export function SettingsPage() {
  const { user, updateProfile } = useAuth();
  const [name, setName] = useState(user?.displayName || "");
  const [saving, setSaving] = useState(false);
  const [profileError, setProfileError] = useState<unknown>();
  const [profileValidation, setProfileValidation] = useState("");
  const [saved, setSaved] = useState(false);
  const [activeSection, setActiveSection] = useState(
    () => window.location.hash || "#profile-settings",
  );
  useEffect(() => setName(user?.displayName || ""), [user?.displayName]);
  useEffect(() => {
    const updateSection = () =>
      setActiveSection(window.location.hash || "#profile-settings");
    window.addEventListener("hashchange", updateSection);
    return () => window.removeEventListener("hashchange", updateSection);
  }, []);

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
    <>
      <section className="page-heading">
        <div>
          <h1>Настройки аккаунта</h1>
          <p>Ваш профиль, устройства и предпочтения.</p>
        </div>
      </section>
      <div className="account-settings-layout">
        <nav className="settings-section-nav" aria-label="Разделы настроек">
          <a
            href="#profile-settings"
            aria-current={
              activeSection === "#profile-settings" ? "location" : undefined
            }
          >
            <UserRound size={18} aria-hidden="true" /> Профиль
          </a>
          <a
            href="#audio-video"
            aria-current={
              activeSection === "#audio-video" ? "location" : undefined
            }
          >
            <SlidersHorizontal size={18} aria-hidden="true" /> Аудио и видео
          </a>
          <a
            href="#notification-settings"
            aria-current={
              activeSection === "#notification-settings"
                ? "location"
                : undefined
            }
          >
            <Bell size={18} aria-hidden="true" /> Уведомления
          </a>
          <a
            href="#integration-settings"
            aria-current={
              activeSection === "#integration-settings" ? "location" : undefined
            }
          >
            <Link2 size={18} aria-hidden="true" /> Интеграции
          </a>
          <a
            href="#account-security"
            aria-current={
              activeSection === "#account-security" ? "location" : undefined
            }
          >
            <ShieldCheck size={18} aria-hidden="true" /> Безопасность
          </a>
        </nav>
        <div className="settings-section-content">
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
                <span>
                  <UserRound size={17} aria-hidden="true" /> Имя для встреч
                </span>
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
                Имя будет видно другим участникам новых встреч.
              </p>
              <label className="field">
                <span>
                  <Mail size={17} aria-hidden="true" /> Email
                </span>
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
          <DeviceSettings />
          <IntegrationsSettings />
          <section
            className="content-card settings-section"
            id="account-security"
          >
            <h2>Безопасность</h2>
            <p className="field-hint">
              Сессия действует 1 час. Для выхода используйте меню профиля. Смена
              пароля и управление активными сессиями пока недоступны.
            </p>
          </section>
        </div>
      </div>
    </>
  );
}

/** Читает историю из общего серверного списка без дополнительных запросов для каждой строки.
 * @return Список доступных завершённых встреч с локальным поиском по загруженным страницам.
 */
export function RecordingsPage() {
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
