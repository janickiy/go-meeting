import { Navigate, Outlet, Route, Routes, useLocation } from "react-router";
import { useAuth } from "./auth";
import { Brand, Button, ErrorNotice, Loading } from "./components/ui";
import { Layout } from "./components/Layout";
import { Landing } from "./pages/Landing";
import { AuthPage, RegistrationSuccess } from "./pages/AuthPages";
import { Dashboard } from "./pages/Dashboard";
import { ConferencePage } from "./pages/ConferencePage";
import { InvitePage } from "./pages/InvitePage";
import { RecordingsPage, SettingsPage } from "./pages/AccountPages";
import { Link } from "react-router";

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
      <Route element={<Protected />}>
        <Route element={<Layout />}>
          <Route path="/app" element={<Dashboard />} />
          <Route path="/conferences" element={<Dashboard all />} />
          <Route path="/conferences/new" element={<Dashboard create />} />
          <Route path="/conferences/:id" element={<ConferencePage />} />
          <Route path="/i/:code" element={<InvitePage />} />
          <Route path="/app/settings" element={<SettingsPage />} />
          <Route path="/app/recordings" element={<RecordingsPage />} />
        </Route>
      </Route>
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
