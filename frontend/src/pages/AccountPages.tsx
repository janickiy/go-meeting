import { CalendarDays, Clapperboard, Mail, UserRound } from "lucide-react";
import { Link } from "react-router";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import { useConferences } from "../queries";
import { Button, ErrorNotice, Loading } from "../components/ui";

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
  const query = useConferences({ view: "past" });
  const conferences = query.data?.pages.flatMap((page) => page.items) || [];
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ЛИЧНЫЙ КАБИНЕТ</span>
          <h1>Записи встреч</h1>
          <p>Важные моменты ваших конференций.</p>
        </div>
      </section>
      <section className="content-card">
        <span className="empty-icon">
          <Clapperboard size={32} />
        </span>
        <h2>Записи в истории встреч</h2>
        <p className="field-hint">
          Откройте завершённую встречу, чтобы увидеть её записи, статус
          обработки и приватные ссылки на скачивание.
        </p>
        <ErrorNotice error={query.error} />
        {query.isPending ? (
          <Loading />
        ) : (
          <div className="conference-list">
            {conferences.map((item) => (
              <Link
                className="conference-row"
                key={item.id}
                to={`/conferences/${item.id}`}
              >
                <Clapperboard size={22} />
                <div>
                  <strong>{item.title}</strong>
                  <p className="field-hint">
                    {formatDate(item.finishedAt || item.createdAt)}
                  </p>
                </div>
                <span className="text-link">История и записи</span>
              </Link>
            ))}
          </div>
        )}
        {!query.isPending && !query.isError && !conferences.length && (
          <p className="compact-empty muted">Завершённых встреч пока нет.</p>
        )}
        {query.hasNextPage && (
          <Button
            variant="outline"
            busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Ещё встречи
          </Button>
        )}
        <Link className="text-link" to="/conferences?view=past">
          Вся история встреч
        </Link>
      </section>
    </>
  );
}
