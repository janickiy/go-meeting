import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router";
import {
  CalendarDays,
  Clapperboard,
  Home,
  LogOut,
  Menu,
  Settings,
  Search,
  X,
} from "lucide-react";
import { useAuth } from "../auth";
import { initials } from "../utils";
import { Brand } from "./ui";
import { NotificationBell } from "./NotificationBell";
import { useNotificationStream } from "../notifications";

/**
 * Layout собирает основную навигацию, учётную запись и личные уведомления авторизованного приложения.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function Layout() {
  const { user, logout } = useAuth();
  useNotificationStream(user?.id);
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const sidebar = useRef<HTMLElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!open) return;
      const desktop = window.matchMedia("(min-width: 761px)");
      if (desktop.matches) {
        setOpen(false);
        return;
      }
      const overflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      sidebar.current?.querySelector<HTMLButtonElement>("button")?.focus();
      /**
       * resize обновляет адаптивную навигацию при изменении ширины окна.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      const resize = () => {
        if (desktop.matches) setOpen(false);
      };
      /**
       * key обрабатывает клавиатурное действие интерфейса.
       *
       * @args
       *   - event (KeyboardEvent) — проверенный конверт события комнаты.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      function key(event: KeyboardEvent) {
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
        const first = items[0];
        const last = items.at(-1);
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
      desktop.addEventListener("change", resize);
      document.addEventListener("keydown", key);
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        document.body.style.overflow = overflow;
        desktop.removeEventListener("change", resize);
        document.removeEventListener("keydown", key);
        menuButton.current?.focus();
      };
    },
    [open],
  );
  return (
    <div className="app-shell">
      <aside
        ref={sidebar}
        className={`sidebar ${open ? "sidebar-open" : ""}`}
        role={open ? "dialog" : undefined}
        aria-modal={open || undefined}
        aria-label={open ? "Меню Meet" : undefined}
      >
        <div className="sidebar-head">
          <Brand to="/app" />
          <button
            className="icon-button mobile-only"
            aria-label="Закрыть меню"
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: setOpen(false).
               */ () => setOpen(false)
            }
          >
            <X />
          </button>
        </div>
        <nav className="sidebar-nav" aria-label="Основная навигация">
          {[
            { to: "/app", label: "Главная", Icon: Home, end: true },
            {
              to: "/conferences",
              label: "Мои конференции",
              Icon: CalendarDays,
            },
            { to: "/app/recordings", label: "Записи", Icon: Clapperboard },
            { to: "/app/search", label: "Поиск", Icon: Search },
            { to: "/app/settings", label: "Настройки", Icon: Settings },
          ].map(
            /**
             * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
             *
             * @args
             *   - объект параметров: to — верхняя граница фильтра либо локальный путь согласно типу; label — свойство текущего компонента; Icon — свойство текущего компонента; end — свойство текущего компонента.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ ({ to, label, Icon, end }) => (
              <NavLink
                key={to}
                to={to}
                end={end}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленное значение: setOpen(false).
                   */ () => setOpen(false)
                }
                className={
                  /**
                   * className выбирает оформление ссылки по её активности.
                   *
                   * @args
                   *   - объект параметров: isActive — свойство текущего компонента.
                   *
                   * @returns вычисленное значение: isActive ? "nav-item nav-active" : "nav-item".
                   */ ({ isActive }) =>
                    isActive ? "nav-item nav-active" : "nav-item"
                }
              >
                <Icon size={19} />
                {label}
              </NavLink>
            ),
          )}
        </nav>
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
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                setLeaving(true);
                void logout()
                  .catch(
                    /**
                     * Обработчик catch выполняет переданный шаг вызова catch в интерфейсе Meet.
                     *
                     *
                     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                     */ () => {},
                  )
                  .finally(
                    /**
                     * Обработчик finally выполняет переданный шаг вызова finally в интерфейсе Meet.
                     *
                     *
                     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                     */ () => {
                      setLeaving(false);
                      navigate("/login", { replace: true });
                    },
                  );
              }
            }
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
          onClick={
            /**
             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setOpen(false).
             */ () => setOpen(false)
          }
        />
      )}
      <div className="workspace" inert={open}>
        <header className="workspace-top">
          <button
            className="icon-button mobile-only"
            aria-label="Открыть меню"
            ref={menuButton}
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: setOpen(true).
               */ () => setOpen(true)
            }
          >
            <Menu />
          </button>
          <span>Ваше пространство для встреч</span>
          <span className="workspace-account">
            <NotificationBell />
            <span className="online-dot" />
            {user?.displayName || "Личный кабинет"}
          </span>
        </header>
        <main className="workspace-main">
          <Outlet />
        </main>
        <footer className="workspace-footer">
          <span>Meet</span>
          <span>Ближе друг к другу — даже на расстоянии.</span>
        </footer>
      </div>
    </div>
  );
}
