import { lazy, Suspense } from "react";
import {
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router";
import { useAuth } from "./auth";
import { Brand, Button, ErrorNotice, Loading } from "./components/ui";
import { Layout } from "./components/Layout";
import { Landing } from "./pages/Landing";
import { AuthPage, RegistrationSuccess } from "./pages/AuthPages";
import { Link } from "react-router";
import { CalendarCallback } from "./components/IntegrationsSettings";
import { AccountSettingsModal } from "./components/AccountSettingsModal";

const PersonalPage = lazy(() =>
  import("./pages/PersonalPage").then((module) => ({
    default: module.PersonalPage,
  })),
);

const Dashboard = lazy(() =>
  import("./pages/Dashboard").then((module) => ({ default: module.Dashboard })),
);
const CalendarPage = lazy(() =>
  import("./pages/CalendarPage").then((module) => ({
    default: module.CalendarPage,
  })),
);
const AnalyticsPage = lazy(() =>
  import("./pages/AnalyticsPage").then((module) => ({
    default: module.AnalyticsPage,
  })),
);
const ConferencePage = lazy(() =>
  import("./pages/ConferencePage").then((module) => ({
    default: module.ConferencePage,
  })),
);
const InvitePage = lazy(() =>
  import("./pages/InvitePage").then((module) => ({
    default: module.InvitePage,
  })),
);
const RecordingsPage = lazy(() =>
  import("./pages/RecordingsPage").then((module) => ({
    default: module.RecordingsPage,
  })),
);
const HistoryPage = lazy(() =>
  import("./pages/AccountPages").then((module) => ({
    default: module.HistoryPage,
  })),
);
const RecordingDetailPage = lazy(() =>
  import("./pages/RecordingDetailPage").then((module) => ({
    default: module.RecordingDetailPage,
  })),
);
const HistoryDetailPage = lazy(() =>
  import("./pages/HistoryDetailPage").then((module) => ({
    default: module.HistoryDetailPage,
  })),
);
const NotificationsPage = lazy(() =>
  import("./pages/NotificationsPage").then((module) => ({
    default: module.NotificationsPage,
  })),
);
const AdminPage = lazy(() =>
  import("./pages/AdminPage").then((module) => ({ default: module.AdminPage })),
);
const PreJoinPage = lazy(() =>
  import("./pages/PreJoinPage").then((module) => ({
    default: module.PreJoinPage,
  })),
);

/** Поддерживает прямые ссылки на настройки: за диалогом находится главная страница.
 * @return Диалог аккаунта; при закрытии прямой адрес заменяется адресом кабинета.
 */
function SettingsRoute() {
  const navigate = useNavigate();
  return (
    <>
      <Dashboard />
      <AccountSettingsModal
        onClose={() => navigate("/app", { replace: true })}
        returnFocus={() => document.getElementById("workspace-main")}
      />
    </>
  );
}

/**
 * Protected проверяет восстановленную авторизацию и допускает защищённые страницы либо перенаправляет на вход.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
function Protected() {
  const auth = useAuth();
  const location = useLocation();
  if (auth.loading)
    return (
      <div className="page-center">
        <Loading />
      </div>
    );
  if (auth.startupError)
    return (
      <div className="page-center">
        <div className="content-card">
          <h1>Не удалось проверить сессию</h1>
          <ErrorNotice>
            Сервер недоступен. Ваш токен не удалён — попробуйте снова.
          </ErrorNotice>
          <Button onClick={auth.retry}>Попробовать снова</Button>
          <Button
            variant="secondary"
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                void auth.logout().catch(
                  /**
                   * Обработчик catch выполняет переданный шаг вызова catch в интерфейсе Meet.
                   *
                   *
                   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                   */ () => {},
                );
              }
            }
          >
            Выйти
          </Button>
        </div>
      </div>
    );
  if (!auth.user)
    return (
      <Navigate
        to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`}
        replace
      />
    );
  if (
    auth.user.guestConferenceId &&
    !["/conferences/", "/meetings/"].some(
      (prefix) =>
        location.pathname === prefix + auth.user!.guestConferenceId ||
        location.pathname === prefix + auth.user!.guestConferenceId + "/join",
    )
  )
    return (
      <Navigate to={`/conferences/${auth.user.guestConferenceId}`} replace />
    );
  return <Outlet />;
}
/**
 * NotFound показывает страницу неизвестного маршрута с переходом к приложению.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
function NotFound() {
  return (
    <div className="page-center">
      <div className="content-card not-found">
        <Brand />
        <span className="not-found-number">404</span>
        <h1>Кажется, мы разминулись</h1>
        <p>Такой страницы не существует.</p>
        <Link className="button button-primary" to="/">
          На главную
        </Link>
      </div>
    </div>
  );
}
/**
 * App собирает публичные и защищённые маршруты Meet и провайдеры приложения.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<Landing />} />
      <Route path="/login" element={<AuthPage />} />
      <Route path="/register" element={<AuthPage register />} />
      <Route path="/register/success" element={<RegistrationSuccess />} />
      <Route
        path="/i/:code"
        element={
          <Suspense
            fallback={
              <div className="page-center">
                <Loading />
              </div>
            }
          >
            <InvitePage />
          </Suspense>
        }
      />
      <Route
        path="/app/settings/calendar/:provider/callback"
        element={<CalendarCallback />}
      />
      <Route element={<Protected />}>
        <Route element={<Layout />}>
          <Route path="/app" element={<Dashboard />} />
          <Route path="/personal" element={<PersonalPage />} />
          <Route path="/personal/:id" element={<PersonalPage />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/meetings" element={<Dashboard all />} />
          <Route path="/meetings/new" element={<Dashboard create />} />
          <Route path="/meetings/:id" element={<ConferencePage />} />
          <Route path="/meetings/:id/join" element={<PreJoinPage />} />
          <Route path="/conferences" element={<Dashboard all />} />
          <Route path="/conferences/new" element={<Dashboard create />} />
          <Route path="/conferences/:id" element={<ConferencePage />} />
          <Route path="/conferences/:id/join" element={<PreJoinPage />} />
          <Route path="/app/settings" element={<SettingsRoute />} />
          <Route path="/settings" element={<SettingsRoute />} />
          <Route path="/app/recordings" element={<RecordingsPage />} />
          <Route path="/recordings" element={<RecordingsPage />} />
          <Route path="/recordings/:id" element={<RecordingDetailPage />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/history/:id" element={<HistoryDetailPage />} />
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/app/search" element={<Navigate to="/app" replace />} />
          <Route path="/search" element={<Navigate to="/app" replace />} />
          <Route path="/admin" element={<AdminPage />} />
        </Route>
      </Route>
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
