import { Link } from "react-router";
import { ArrowRight, CalendarDays, ShieldCheck, Users } from "lucide-react";
import { useAuth } from "../auth";
import { Brand } from "../components/ui";

/**
 * Landing показывает приветственную страницу Meet и переходы к регистрации и входу.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function Landing() {
  const { user } = useAuth();
  return (
    <div className="landing">
      <header className="landing-header">
        <Brand />
        <nav aria-label="Аккаунт">
          {user ? (
            <Link className="button button-primary" to="/app">
              В приложение <ArrowRight size={17} />
            </Link>
          ) : (
            <>
              <Link className="plain-link" to="/login">
                Войти
              </Link>
              <Link className="button button-primary" to="/register">
                Зарегистрироваться
              </Link>
            </>
          )}
        </nav>
      </header>
      <main className="hero">
        <div className="hero-copy">
          <span className="eyebrow">
            <span className="online-dot" />
            Место для ваших встреч
          </span>
          <h1>
            Встречайтесь
            <br />
            без границ<span className="blue-dot">.</span>
          </h1>
          <p className="hero-subtitle">
            Простые и удобные конференции
            <br className="desktop-only" /> для работы, обучения и общения.
          </p>
          <div className="hero-features">
            {[
              {
                Icon: ShieldCheck,
                title: "Защищённый доступ",
                text: "Ваш аккаунт и права участников под контролем",
              },
              {
                Icon: Users,
                title: "Встречи без лишних шагов",
                text: "Создайте конференцию и поделитесь ссылкой",
              },
              {
                Icon: CalendarDays,
                title: "Всё в одном месте",
                text: "Участники, приглашения и история встреч",
              },
            ].map(
              /**
               * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
               *
               * @parameters:
               *   - объект параметров: Icon — свойство текущего компонента; title — название встречи или диалога; text — обычный текст сообщения.
               *
               * @returns преобразованное значение текущего элемента для результирующего набора.
               */ ({ Icon, title, text }) => (
                <div className="hero-feature" key={title}>
                  <span className="feature-icon">
                    <Icon size={23} />
                  </span>
                  <div>
                    <strong>{title}</strong>
                    <p>{text}</p>
                  </div>
                </div>
              ),
            )}
          </div>
          <Link
            to={user ? "/app" : "/register"}
            className="button button-primary hero-cta"
          >
            {user ? "Открыть мой кабинет" : "Начать встречаться"}
            <ArrowRight size={18} />
          </Link>
          <span className="hero-note">
            Управление встречами уже доступно. Видеосвязь — следующий этап.
          </span>
        </div>
        <div className="hero-visual">
          <div className="hero-orbit orbit-one" />
          <div className="hero-orbit orbit-two" />
          <img
            src="/media/meet-laptop.png"
            alt="Ноутбук с четырьмя участниками видеовстречи — иллюстрация"
            width="1536"
            height="1024"
            fetchPriority="high"
          />
          <div className="hero-floating">
            <span className="floating-check">
              <ShieldCheck size={22} />
            </span>
            <span>
              <strong>Ваша следующая встреча</strong>
              <small>начинается здесь</small>
            </span>
          </div>
        </div>
      </main>
      <footer className="landing-footer">
        <span>Meet — быть рядом стало проще.</span>
        <span>Создавайте. Приглашайте. Встречайтесь.</span>
      </footer>
    </div>
  );
}
