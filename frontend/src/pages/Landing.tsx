import { Link } from "react-router";
import {
  ArrowRight,
  CirclePlay,
  Link as LinkIcon,
  MessageCircle,
} from "lucide-react";
import { useAuth } from "../auth";
import { Brand } from "../components/ui";
import { PRODUCT_NAME } from "../brand";
import "./meetings-design.css";

/**
 * Landing показывает приветственную страницу Meet и переходы к регистрации и входу.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function Landing() {
  const { user } = useAuth();
  return (
    <div className="landing landing-design-page">
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
                Создать аккаунт <ArrowRight size={17} aria-hidden="true" />
              </Link>
            </>
          )}
        </nav>
      </header>
      <main className="hero">
        <div className="hero-copy">
          <span className="eyebrow">
            {PRODUCT_NAME.toUpperCase()} · ВСТРЕЧИ. ИДЕИ. РЕЗУЛЬТАТЫ.
          </span>
          <h1>
            Работайте вместе.
            <br />
            <em>Где бы вы ни были.</em>
          </h1>
          <p className="hero-subtitle">
            Встречи, разговоры и важные материалы — в одном спокойном
            пространстве для вашей команды.
          </p>
          <Link
            to={user ? "/app" : "/register"}
            className="button button-primary hero-cta"
          >
            {user ? "Открыть мой кабинет" : "Начать встречаться"}
            <ArrowRight size={18} />
          </Link>
          <span className="hero-note">
            По приглашению можно войти без аккаунта.
          </span>
        </div>
        <div className="hero-visual">
          <img
            src="/media/meet-laptop.png"
            alt="Ноутбук с четырьмя участниками видеовстречи — иллюстрация"
            width="1536"
            height="1024"
            fetchPriority="high"
          />
        </div>
      </main>
      <section
        className="landing-design-features"
        aria-label="Возможности Meetrix"
      >
        {[
          {
            Icon: LinkIcon,
            title: "Просто присоединиться",
            text: "Создайте встречу и поделитесь ссылкой.",
          },
          {
            Icon: MessageCircle,
            title: "Разговор продолжается",
            text: "Личные и групповые чаты для рабочих вопросов.",
          },
          {
            Icon: CirclePlay,
            title: "Важное остаётся",
            text: "Записи и материалы завершённых встреч.",
          },
        ].map(({ Icon, title, text }) => (
          <article key={title}>
            <Icon size={24} aria-hidden="true" />
            <h2>{title}</h2>
            <p>{text}</p>
          </article>
        ))}
      </section>
      <footer className="landing-footer">
        <strong>{PRODUCT_NAME}</strong>
        <span>Одно место для вашей команды.</span>
      </footer>
    </div>
  );
}
