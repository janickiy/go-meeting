import { useRef, useState } from "react";
import type { FormEvent } from "react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { api, errorMessage } from "../api";
import { useAuth } from "../auth";
import {
  Brand,
  Button,
  ErrorNotice,
  PasswordInput,
  SuccessMark,
} from "../components/ui";
import { passwordLength, safeNext, utf8Bytes } from "../utils";

export function AuthPage({ register = false }: { register?: boolean }) {
  const auth = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const next = safeNext(params.get("next"));
  const nextQuery = next === "/app" ? "" : `?next=${encodeURIComponent(next)}`;
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [recovery, setRecovery] = useState(false);
  const submitted = useRef(false);
  // Do not interrupt the registration success transition after automatic login.
  if (auth.user && !auth.loading && !submitted.current)
    return <Navigate to={next} replace />;
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    if (
      !/^[^\s@]+@[^\s@]+$/.test(email.trim()) ||
      utf8Bytes(email.trim()) > 254
    ) {
      setError("Введите корректный email.");
      return;
    }
    const size = passwordLength(password);
    if (!password || size > 128 || (register && size < 8)) {
      setError(
        register
          ? "Пароль должен содержать от 8 до 128 символов."
          : "Введите пароль длиной не более 128 символов.",
      );
      return;
    }
    if (register && Array.from(name.trim()).length > 100) {
      setError("Имя не должно превышать 100 символов.");
      return;
    }
    submitted.current = true;
    setBusy(true);
    try {
      if (register) {
        await api.register(email.trim(), password, name);
        try {
          await auth.login(email.trim(), password);
        } catch {
          /* Registration succeeded; the success page will offer login if needed. */
        }
        setPassword("");
        navigate(`/register/success${nextQuery}`, { replace: true });
      } else {
        await auth.login(email.trim(), password);
        setPassword("");
        navigate(next, { replace: true });
      }
    } catch (error) {
      submitted.current = false;
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="auth-page">
      <Link className="auth-back" to="/">
        <ArrowLeft size={16} />
        На главную
      </Link>
      <div className={`auth-card ${register ? "register-card" : ""}`}>
        <Brand />
        <div className="auth-heading">
          <h1>{register ? "Создайте аккаунт" : "Вход в аккаунт"}</h1>
          <p>
            {register
              ? "Начните проводить встречи уже сегодня"
              : "Рады видеть вас снова!"}
          </p>
        </div>
        <ErrorNotice>
          {error ||
            (!register && auth.expired
              ? "Сессия завершилась. Войдите снова, чтобы продолжить."
              : null)}
        </ErrorNotice>
        <form onSubmit={submit} noValidate>
          <label className="field" htmlFor="email">
            Email
            <input
              id="email"
              type="email"
              autoComplete="email"
              placeholder="you@example.com"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              disabled={busy}
            />
          </label>
          <label className="field" htmlFor="password">
            Пароль
            <PasswordInput
              id="password"
              placeholder={register ? "Минимум 8 символов" : "Введите пароль"}
              autoComplete={register ? "new-password" : "current-password"}
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              disabled={busy}
            />
          </label>
          {register ? (
            <>
              <p className="field-hint password-hint">
                От 8 до 128 символов. Цифры и спецсимволы необязательны.
              </p>
              <label className="field" htmlFor="displayName">
                Как к вам обращаться?{" "}
                <span className="muted">(необязательно)</span>
                <input
                  id="displayName"
                  autoComplete="nickname"
                  placeholder="Александр"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  disabled={busy}
                />
              </label>
            </>
          ) : (
            <div className="recovery">
              <button
                type="button"
                className="text-button"
                onClick={() => setRecovery(!recovery)}
              >
                Забыли пароль?
              </button>
              {recovery && (
                <p className="field-hint" role="status">
                  Восстановление пароля пока недоступно. Обратитесь к
                  администратору сервиса.
                </p>
              )}
            </div>
          )}
          <Button type="submit" busy={busy} className="full-width">
            {register ? "Зарегистрироваться" : "Войти"}
          </Button>
        </form>
        {register ? (
          <>
            <p className="auth-note">
              Email нужен только для входа. Гостевой доступ
              <br />и подтверждение email пока не поддерживаются.
            </p>
            <p className="auth-switch">
              Уже есть аккаунт? <Link to={`/login${nextQuery}`}>Войти</Link>
            </p>
          </>
        ) : (
          <>
            <div className="divider">
              <span>или</span>
            </div>
            <p className="auth-switch">
              Нет аккаунта?{" "}
              <Link to={`/register${nextQuery}`}>Зарегистрироваться</Link>
            </p>
          </>
        )}
      </div>
      <p className="auth-footer">Одно место. Все ваши встречи.</p>
    </div>
  );
}
export function RegistrationSuccess() {
  const { user } = useAuth();
  const [params] = useSearchParams();
  const next = safeNext(params.get("next"));
  return (
    <div className="auth-page">
      <div className="auth-card registration-success">
        <Brand />
        <SuccessMark />
        <h1>Аккаунт создан!</h1>
        <p>
          Добро пожаловать в Meet.
          <br />
          Теперь вы можете создавать
          <br />и управлять своими конференциями.
        </p>
        <Link
          className="button button-primary full-width"
          to={user ? next : `/login?next=${encodeURIComponent(next)}`}
        >
          {user ? "Перейти в приложение" : "Войти в приложение"}
          <ArrowRight size={17} />
        </Link>
      </div>
    </div>
  );
}
