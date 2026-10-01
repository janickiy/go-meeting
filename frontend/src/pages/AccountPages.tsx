import { CalendarDays, Clapperboard, Mail, UserRound } from "lucide-react";
import { Link } from "react-router";
import { useAuth } from "../auth";
import { formatDate } from "../utils";

export function SettingsPage() {
  const { user } = useAuth();
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ЛИЧНЫЙ КАБИНЕТ</span>
          <h1>Настройки аккаунта</h1>
          <p>Всё, что важно знать о вашем профиле.</p>
        </div>
      </section>
      <section className="content-card account-card">
        <h2>Ваш профиль</h2>
        <div className="account-property">
          <UserRound size={20} />
          <div>
            <span>Имя</span>
            <strong>{user?.displayName || "Не указано"}</strong>
          </div>
        </div>
        <div className="account-property">
          <Mail size={20} />
          <div>
            <span>Email</span>
            <strong>{user?.email}</strong>
          </div>
        </div>
        <div className="account-property">
          <CalendarDays size={20} />
          <div>
            <span>Дата регистрации</span>
            <strong>{user && formatDate(user.createdAt)}</strong>
          </div>
        </div>
        <p className="field-hint">
          Редактирование профиля и смена пароля пока недоступны. Сессия
          действует 1 час. Для выхода используйте кнопку в боковом меню.
        </p>
      </section>
    </>
  );
}
export function RecordingsPage() {
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ЛИЧНЫЙ КАБИНЕТ</span>
          <h1>Записи встреч</h1>
          <p>Важные моменты ваших конференций.</p>
        </div>
      </section>
      <section className="content-card empty-state">
        <span className="empty-icon">
          <Clapperboard size={32} />
        </span>
        <h2>Раздел пока недоступен</h2>
        <p>
          Записи появятся здесь после подключения записи конференций
          <br className="desktop-only" /> и разграничения доступа к файлам.
        </p>
        <p className="field-hint">
          Существующий recorder работает отдельно и не изменён.
        </p>
        <Link className="button button-primary" to="/conferences">
          Мои конференции
        </Link>
      </section>
    </>
  );
}
