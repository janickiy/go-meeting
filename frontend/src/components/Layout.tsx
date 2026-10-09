import { MessageCircle } from "lucide-react";
import { usePersonalRealtime } from "../personalRealtime";
import "../pages/personal.css";
import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import type { MouseEvent } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router";
import {
  BarChart3,
  CalendarDays,
  Home,
  Folder,
  LogOut,
  Menu,
  CirclePlay,
  Settings,
  ShieldCheck,
  Video,
  X,
} from "lucide-react";
import { useAuth } from "../auth";
import { initials } from "../utils";
import { Brand, Loading } from "./ui";
import { NotificationBell } from "./NotificationBell";
import { useNotificationStream } from "../notifications";
import { useCapabilities } from "../useCapabilities";
import { PRODUCT_NAME, PRODUCT_TAGLINE } from "../brand";
import { AccountSettingsModal } from "./AccountSettingsModal";
import { AccountSettingsContext } from "./AccountSettingsContext";

/**
 * Layout объединяет навигацию, поиск и личный профиль, не управляя соединениями комнаты.
 * Мобильный drawer удерживает фокус и возвращает его кнопке после закрытия.
 * @return Общая оболочка с лениво загружаемым защищённым маршрутом.
 */
export function Layout() {
  const { user, logout } = useAuth();
  useNotificationStream(user?.guestConferenceId ? undefined : user?.id);
  const personal = usePersonalRealtime(
    user?.guestConferenceId ? undefined : user?.id,
  );
  const capabilities = useCapabilities();
  const navigate = useNavigate();
  const location = useLocation();
  const [open, setOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const settingsOpener = useRef<HTMLElement | null>(null);
  const settingsRouteOpen = ["/app/settings", "/settings"].includes(
    location.pathname,
  );
  const [leaving, setLeaving] = useState(false);
  const sidebar = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const bottomButton = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLElement | null>(null);

  const showAccountSettings = useCallback((element: HTMLElement) => {
    settingsOpener.current = element;
    setSettingsOpen(true);
  }, []);
  const settingsDialog = settingsOpen && (
    <AccountSettingsModal
      onClose={() => setSettingsOpen(false)}
      returnFocus={() => settingsOpener.current}
    />
  );

  /** Открывает диалог обычным кликом, сохраняя текущую страницу и стандартное открытие ссылок в новой вкладке.
   * @args event — нажатие ссылки настроек в навигации.
   * @return Значение не возвращается; диалог получает фокус, мобильное меню закрывается.
   */
  function openSettings(event: MouseEvent<HTMLAnchorElement>) {
    if (
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    )
      return;
    event.preventDefault();
    settingsOpener.current = open
      ? opener.current || menuButton.current
      : event.currentTarget;
    setOpen(false);
    showAccountSettings(settingsOpener.current!);
  }
  useEffect(() => {
    if (!open) return;
    const desktop = window.matchMedia("(min-width: 761px)");
    if (desktop.matches) {
      setOpen(false);
      return;
    }
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    sidebar.current?.querySelector<HTMLButtonElement>("button")?.focus();
    const resize = () => {
      if (desktop.matches) setOpen(false);
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setOpen(false);
      }
      if (event.key !== "Tab") return;
      const items = Array.from(
        sidebar.current?.querySelectorAll<HTMLElement>(
          "a[href], button:not(:disabled)",
        ) || [],
      );
      const first = items[0],
        last = items.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    desktop.addEventListener("change", resize);
    document.addEventListener("keydown", key);
    return () => {
      document.body.style.overflow = overflow;
      desktop.removeEventListener("change", resize);
      document.removeEventListener("keydown", key);
      (opener.current || menuButton.current)?.focus();
    };
  }, [open]);

  /** Завершает локальную сессию и возвращает к форме входа даже при сетевом отказе logout. */
  const leaveAccount = () => {
    setLeaving(true);
    void logout()
      .catch(() => {})
      .finally(() => {
        setLeaving(false);
        navigate("/login", { replace: true });
      });
  };
  if (user?.guestConferenceId)
    return (
      <AccountSettingsContext.Provider value={showAccountSettings}>
        <div className="guest-room-shell" inert={settingsOpen}>
          <a className="skip-link" href="#workspace-main">
            Перейти к встрече
          </a>
          <main id="workspace-main">
            <Suspense fallback={<Loading />}>
              <Outlet />
            </Suspense>
          </main>
        </div>
        {settingsDialog}
      </AccountSettingsContext.Provider>
    );
  const items = [
    { to: "/app", label: "Главная", Icon: Home, end: true },
    { to: "/conferences", label: "Встречи", Icon: Video },
    { to: "/personal", label: "Личные", Icon: MessageCircle },
    { to: "/calendar", label: "Календарь", Icon: CalendarDays },
    { to: "/recordings", label: "Записи", Icon: CirclePlay },
    ...(capabilities.isSuccess &&
    !capabilities.isError &&
    capabilities.data.capabilities.meetingAnalytics
      ? [{ to: "/analytics", label: "Аналитика", Icon: BarChart3 }]
      : []),
  ];
  const serviceItems = [
    { to: "/app/settings", label: "Настройки", Icon: Settings },
    ...(user?.isAdmin
      ? [{ to: "/admin", label: "Администрирование", Icon: ShieldCheck }]
      : []),
  ];
  return (
    <AccountSettingsContext.Provider value={showAccountSettings}>
      {personal.notice && (
        <div className="personal-notice" role="status">
          <Link
            to={`/personal/${personal.notice.id}`}
            onClick={personal.dismiss}
          >
            <strong>{personal.notice.name}</strong>
            <span>{personal.notice.text}</span>
          </Link>
          <button
            className="icon-button"
            aria-label="Закрыть уведомление о сообщении"
            onClick={personal.dismiss}
          >
            <X size={18} />
          </button>
        </div>
      )}
      <div className="app-shell" inert={settingsOpen || settingsRouteOpen}>
        <a className="skip-link" href="#workspace-main">
          Перейти к содержимому
        </a>
        <aside
          id="app-sidebar"
          ref={sidebar}
          className={`sidebar ${open ? "sidebar-open" : ""}`}
          role={open ? "dialog" : undefined}
          aria-modal={open || undefined}
          aria-label={open ? `Меню ${PRODUCT_NAME}` : undefined}
        >
          <div className="sidebar-head">
            <Brand to="/app" />
            <button
              className="icon-button mobile-only"
              aria-label="Закрыть меню"
              onClick={() => setOpen(false)}
            >
              <X />
            </button>
          </div>
          <p className="sidebar-caption">РАБОЧЕЕ ПРОСТРАНСТВО</p>
          <nav className="sidebar-nav" aria-label="Основная навигация">
            {items.map(({ to, label, Icon, ...props }) => (
              <NavLink
                key={to}
                to={to}
                {...props}
                onClick={(event) =>
                  to === "/app/settings" ? openSettings(event) : setOpen(false)
                }
                className={({ isActive }) =>
                  isActive ? "nav-item nav-active" : "nav-item"
                }
              >
                <Icon size={18} aria-hidden="true" />
                {label}
                {to === "/personal" && personal.unread > 0 && (
                  <span
                    className="count-badge"
                    aria-label={`${personal.unread} непрочитанных личных сообщений`}
                  >
                    {personal.unread}
                  </span>
                )}
              </NavLink>
            ))}
          </nav>
          <p className="sidebar-caption sidebar-personal-caption">
            ЛИЧНОЕ ПРОСТРАНСТВО
          </p>
          <nav className="sidebar-nav" aria-label="Личное пространство">
            <NavLink
              to="/folders"
              onClick={() => setOpen(false)}
              className={({ isActive }) =>
                isActive ? "nav-item nav-active" : "nav-item"
              }
            >
              <Folder size={18} aria-hidden="true" />
              Папки
            </NavLink>
          </nav>
          <nav
            className="sidebar-nav sidebar-service-nav"
            aria-label="Настройки приложения"
          >
            {serviceItems.map(({ to, label, Icon }) => (
              <NavLink
                key={to}
                to={to}
                onClick={(event) =>
                  to === "/app/settings" ? openSettings(event) : setOpen(false)
                }
                className={({ isActive }) =>
                  isActive ? "nav-item nav-active" : "nav-item"
                }
              >
                <Icon size={18} aria-hidden="true" />
                {label}
              </NavLink>
            ))}
          </nav>
          <div className="sidebar-context-note">
            <ShieldCheck size={18} />
            <p>
              Ваши встречи и материалы.
              <br />
              Доступ — под вашим контролем.
            </p>
          </div>
          <div className="sidebar-bottom">
            <div className="profile">
              <span className="avatar avatar-small">
                {initials(user?.displayName || user?.email || "")}
              </span>
              <div>
                <strong>{user?.displayName || "Мой аккаунт"}</strong>
                <span>{user?.email}</span>
              </div>
            </div>
            <button
              className="logout-button"
              disabled={leaving}
              onClick={leaveAccount}
            >
              <LogOut size={16} />
              {leaving ? "Выходим…" : "Выйти из аккаунта"}
            </button>
          </div>
        </aside>
        {open && (
          <button
            className="sidebar-shade"
            aria-label="Скрыть меню"
            onClick={() => setOpen(false)}
          />
        )}
        <div className="workspace" inert={open}>
          <header className="workspace-top">
            <button
              className="icon-button mobile-only"
              aria-label="Открыть меню"
              aria-controls="app-sidebar"
              aria-expanded={open}
              ref={menuButton}
              onClick={() => {
                opener.current = menuButton.current;
                setOpen(true);
              }}
            >
              <Menu aria-hidden="true" />
            </button>
            <div className="workspace-account">
              <NotificationBell />
              <div className="topbar-profile">
                <span className="avatar avatar-small">
                  {initials(user?.displayName || user?.email || "")}
                </span>
                <span className="topbar-profile-text">
                  <strong>{user?.displayName || "Мой аккаунт"}</strong>
                  <small>{user?.email}</small>
                </span>
              </div>
            </div>
          </header>
          <main id="workspace-main" className="workspace-main" tabIndex={-1}>
            <Suspense fallback={<Loading />}>
              <Outlet />
            </Suspense>
          </main>
          <footer className="workspace-footer">
            <span>{PRODUCT_NAME}</span>
            <span>{PRODUCT_TAGLINE}</span>
          </footer>
          <nav className="mobile-bottom-nav" aria-label="Быстрая навигация">
            <NavLink to="/app" end aria-label="Главная — быстрая навигация">
              <Home size={20} />
              Главная
            </NavLink>
            <NavLink
              to="/personal"
              aria-label={`Личные — быстрая навигация${personal.unread ? `, непрочитанных: ${personal.unread}` : ""}`}
            >
              <MessageCircle size={20} />
              Личные
            </NavLink>
            <NavLink to="/calendar" aria-label="Календарь — быстрая навигация">
              <CalendarDays size={20} />
              Календарь
            </NavLink>
            <NavLink to="/recordings" aria-label="Записи — быстрая навигация">
              <CirclePlay size={20} />
              Записи
            </NavLink>
            <button
              ref={bottomButton}
              aria-label="Все разделы"
              aria-expanded={open}
              aria-controls="app-sidebar"
              onClick={() => {
                opener.current = bottomButton.current;
                setOpen(true);
              }}
            >
              <Menu size={20} />
              Ещё
            </button>
          </nav>
        </div>
      </div>
      {settingsDialog}
    </AccountSettingsContext.Provider>
  );
}
