import { Suspense, useEffect, useRef, useState } from "react";
import type { MouseEvent } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router";
import {
  BarChart3,
  Bell,
  CalendarDays,
  ChevronDown,
  Home,
  History,
  LogOut,
  Menu,
  CirclePlay,
  Search,
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

/**
 * Layout объединяет навигацию, поиск и личный профиль, не управляя соединениями комнаты.
 * Мобильный drawer удерживает фокус и возвращает его кнопке после закрытия.
 * @return Общая оболочка с лениво загружаемым защищённым маршрутом.
 */
export function Layout() {
  const { user, logout } = useAuth();
  useNotificationStream(user?.guestConferenceId ? undefined : user?.id);
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
  const [search, setSearch] = useState("");
  const sidebar = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const bottomButton = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLElement | null>(null);

  /** Открывает диалог обычным кликом, сохраняя текущую страницу и стандартное открытие ссылок в новой вкладке.
   * @args event — нажатие ссылки настроек в навигации или меню профиля.
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
    const details = event.currentTarget.closest("details");
    settingsOpener.current = open
      ? opener.current || menuButton.current
      : details?.querySelector<HTMLElement>("summary") || event.currentTarget;
    if (details) details.open = false;
    setOpen(false);
    setSettingsOpen(true);
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
      <div className="guest-room-shell">
        <a className="skip-link" href="#workspace-main">
          Перейти к встрече
        </a>
        <main id="workspace-main">
          <Suspense fallback={<Loading />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    );
  const items = [
    { to: "/app", label: "Главная", Icon: Home, end: true },
    { to: "/conferences", label: "Встречи", Icon: Video },
    { to: "/calendar", label: "Календарь", Icon: CalendarDays },
    { to: "/recordings", label: "Записи", Icon: CirclePlay },
    { to: "/history", label: "История", Icon: History },
    ...(capabilities.isSuccess &&
    !capabilities.isError &&
    capabilities.data.capabilities.meetingAnalytics
      ? [{ to: "/analytics", label: "Аналитика", Icon: BarChart3 }]
      : []),
    { to: "/notifications", label: "Уведомления", Icon: Bell },
    { to: "/app/search", label: "Поиск", Icon: Search },
    { to: "/app/settings", label: "Настройки", Icon: Settings },
    ...(user?.isAdmin
      ? [{ to: "/admin", label: "Администрирование", Icon: ShieldCheck }]
      : []),
  ];
  return (
    <>
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
            <form
              className="workspace-search"
              role="search"
              aria-label="Поиск по встречам и материалам"
              onSubmit={(event) => {
                event.preventDefault();
                if (search.trim().length >= 2)
                  navigate(`/search?q=${encodeURIComponent(search.trim())}`);
              }}
            >
              <Search size={17} aria-hidden="true" />
              <input
                aria-label="Найти в пространстве"
                placeholder="Поиск по встречам и материалам"
                minLength={2}
                maxLength={500}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
              <button
                type="submit"
                className="search-submit"
                aria-label="Выполнить поиск"
              >
                ↵
              </button>
            </form>
            <div className="workspace-account">
              <NotificationBell />
              <details className="profile-dropdown">
                <summary aria-label="Меню профиля">
                  <span className="avatar avatar-small">
                    {initials(user?.displayName || user?.email || "")}
                  </span>
                  <span className="topbar-profile-text">
                    <strong>{user?.displayName || "Мой аккаунт"}</strong>
                    <small>{user?.email}</small>
                  </span>
                  <ChevronDown size={15} />
                </summary>
                <div className="profile-dropdown-content">
                  <Link to="/app/settings" onClick={openSettings}>
                    <Settings size={16} />
                    Настройки профиля
                  </Link>
                  <button disabled={leaving} onClick={leaveAccount}>
                    <LogOut size={16} />
                    Завершить сеанс
                  </button>
                </div>
              </details>
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
      {settingsOpen && (
        <AccountSettingsModal
          onClose={() => setSettingsOpen(false)}
          returnFocus={() => settingsOpener.current}
        />
      )}
    </>
  );
}
