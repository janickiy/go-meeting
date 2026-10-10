import { Link } from "react-router";
import {
  ArrowRight,
  CirclePlay,
  MessageCircle,
  Video,
  ShieldCheck,
} from "lucide-react";
import { useAuth } from "../auth";
import { Brand } from "../components/ui";
import { Copyright } from "../components/Copyright";
import { PRODUCT_DESCRIPTION, PRODUCT_NAME, PRODUCT_TAGLINE } from "../brand";
import { LandingScene } from "./LandingScene";
import "./landing.css";

const features = [
  {
    Icon: Video,
    title: "Встречайтесь",
    text: "Одна ссылка — и вы рядом. Обсуждайте идеи и показывайте экран.",
    tone: "violet",
  },
  {
    Icon: MessageCircle,
    title: "Общайтесь",
    text: "Продолжайте разговор в личных и групповых чатах. Делитесь важным.",
    tone: "green",
  },
  {
    Icon: CirclePlay,
    title: "Возвращайтесь к важному",
    text: "Записи и материалы встреч под рукой, когда нужно освежить детали.",
    tone: "peach",
  },
];

/** Публичная главная: продуктовая иллюстрация не подключается к медиа или встречам. */
export function Landing() {
  const { user } = useAuth();
  return (
    <div className="meetspace-home">
      <a className="skip-link" href="#home-main">
        Перейти к содержимому
      </a>
      <header className="home-header home-container">
        <Brand />
        <nav className="home-account-nav" aria-label="Аккаунт">
          {user ? (
            <Link className="home-button" to="/app">
              В приложение <ArrowRight size={18} aria-hidden="true" />
            </Link>
          ) : (
            <>
              <Link className="home-login" to="/login">
                Войти
              </Link>
              <Link className="home-button" to="/register">
                Создать аккаунт <ArrowRight size={18} aria-hidden="true" />
              </Link>
            </>
          )}
        </nav>
      </header>
      <main id="home-main" className="home-main home-container" tabIndex={-1}>
        <section
          className="home-hero"
          aria-labelledby="home-title"
          aria-describedby="home-description"
        >
          <div className="home-hero-copy">
            <p className="home-eyebrow">
              <span aria-hidden="true" /> Пространство, которое объединяет
            </p>
            <h1 id="home-title">
              Хорошие идеи начинаются <span>с разговора.</span>
            </h1>
            <p className="home-hero-description">{PRODUCT_TAGLINE}</p>
            <p id="home-description" className="sr-only">
              {PRODUCT_DESCRIPTION}
            </p>
            <Link
              className="home-button home-hero-cta"
              to={user ? "/app" : "/register"}
            >
              {user ? "Открыть мой кабинет" : "Начать встречаться"}
              <ArrowRight size={20} aria-hidden="true" />
            </Link>
            <p className="home-guest-note">
              <ShieldCheck size={16} aria-hidden="true" /> По приглашению можно
              войти без аккаунта.
            </p>
          </div>
          <LandingScene />
        </section>
        <section
          id="features"
          className="home-features"
          aria-label={`Возможности ${PRODUCT_NAME}`}
        >
          {features.map(({ Icon, title, text, tone }) => (
            <article
              className={`home-feature home-feature--${tone}`}
              key={title}
            >
              <span className="home-feature-icon">
                <Icon size={23} aria-hidden="true" />
              </span>
              <div>
                <h2>{title}</h2>
                <p>{text}</p>
              </div>
            </article>
          ))}
        </section>
      </main>
      <footer className="home-footer home-container">
        <span className="home-footer-brand">
          <strong>{PRODUCT_NAME}</strong>
          <span>Ближе, где бы вы ни были.</span>
        </span>
        <Copyright />
      </footer>
    </div>
  );
}
