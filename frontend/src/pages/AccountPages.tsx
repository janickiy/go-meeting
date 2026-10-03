import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { CalendarDays, Clapperboard, Mail, UserRound } from "lucide-react";
import { Link } from "react-router";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import { useConferences } from "../queries";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { IntegrationsSettings } from "../components/IntegrationsSettings";
import { DeviceSettings } from "../components/DeviceSettings";

/**
 * SettingsPage показывает доступные сведения и настройки текущей учётной записи.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function SettingsPage() {
  const { user, updateProfile } = useAuth();
  const [name, setName] = useState(user?.displayName || "");
  const [saving, setSaving] = useState(false);
  const [profileError, setProfileError] = useState<unknown>();
  const [profileValidation, setProfileValidation] = useState("");
  const [saved, setSaved] = useState(false);
  useEffect(() => setName(user?.displayName || ""), [user?.displayName]);
  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const value = name.trim();
    if (
      !value ||
      [...value].length > 100 ||
      /[\u0000-\u001f\u007f]/u.test(value)
    ) {
      setProfileValidation(
        "Укажите имя от 1 до 100 символов без управляющих знаков.",
      );
      setSaved(false);
      return;
    }
    setSaving(true);
    setProfileValidation("");
    setProfileError(undefined);
    setSaved(false);
    try {
      await updateProfile(value);
      setSaved(true);
    } catch (error) {
      setProfileError(error);
    } finally {
      setSaving(false);
    }
  }
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ЛИЧНЫЙ КАБИНЕТ</span>
          <h1>Настройки аккаунта</h1>
          <p>Всё, что важно знать о вашем профиле.</p>
        </div>
      </section>
      <nav className="settings-links" aria-label="Разделы настроек">
        <a className="text-link" href="#audio-video">
          Аудио и видео
        </a>
        <a className="text-link" href="#notification-settings">
          Уведомления
        </a>
        <a className="text-link" href="#integration-settings">
          Интеграции
        </a>
      </nav>
      <section className="content-card account-card">
        <h2>Ваш профиль</h2>
        <form onSubmit={(event) => void saveProfile(event)}>
          <label className="field">
            <span>
              <UserRound size={18} aria-hidden="true" /> Имя для встреч
            </span>
            <input
              autoComplete="name"
              value={name}
              onChange={(event) => {
                setName(event.target.value);
                setSaved(false);
                setProfileValidation("");
              }}
              required
              aria-describedby="profile-name-hint"
            />
          </label>
          <p id="profile-name-hint" className="field-hint">
            Имя будет видно другим участникам новых встреч.
          </p>
          <ErrorNotice>{profileValidation || null}</ErrorNotice>
          <ErrorNotice error={profileError} />
          {saved && <p role="status">Имя сохранено.</p>}
          <Button
            type="submit"
            busy={saving}
            disabled={name.trim() === (user?.displayName || "")}
          >
            Сохранить имя
          </Button>
        </form>
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
          Сессия действует 1 час. Для выхода используйте кнопку в боковом меню.
        </p>
      </section>
      <DeviceSettings />
      <IntegrationsSettings />
    </>
  );
}
/**
 * RecordingsPage собирает завершённые встречи для отдельной истории и материалов.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function RecordingsPage() {
  const query = useConferences({ view: "past" });
  const conferences =
    query.data?.pages.flatMap(
      /**
       * Обработчик flatMap преобразует текущий элемент в данные или представление результирующего списка.
       *
       * @args
       *   - page — изолированная страница Playwright.
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ (page) => page.items,
    ) || [];
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ЛИЧНЫЙ КАБИНЕТ</span>
          <h1>История встреч</h1>
          <p>Завершённые встречи, записи и материалы.</p>
        </div>
      </section>
      <section className="content-card">
        <span className="empty-icon">
          <Clapperboard size={32} />
        </span>
        <h2>Завершённые встречи</h2>
        <p className="field-hint">
          Откройте встречу, чтобы увидеть записи, расшифровку, итоги и
          аналитику.
        </p>
        <ErrorNotice error={query.error} />
        {query.isPending ? (
          <Loading />
        ) : (
          <div className="conference-list">
            {conferences.map(
              /**
               * Обработчик conferences.map преобразует один элемент набора в представление или данные следующего шага.
               *
               * @args
               *   - item — элемент списка, который обрабатывает текущий шаг.
               *
               * @returns преобразованное значение текущего элемента для результирующего набора.
               */ (item) => (
                <Link
                  className="conference-row"
                  key={item.id}
                  to={`/history/${item.id}`}
                >
                  <Clapperboard size={22} />
                  <div>
                    <strong>{item.title}</strong>
                    <p className="field-hint">
                      {formatDate(item.finishedAt || item.createdAt)}
                    </p>
                  </div>
                  <span className="text-link">Открыть историю</span>
                </Link>
              ),
            )}
          </div>
        )}
        {!query.isPending && !query.isError && !conferences.length && (
          <p className="compact-empty muted">Завершённых встреч пока нет.</p>
        )}
        {query.hasNextPage && (
          <Button
            variant="outline"
            busy={query.isFetchingNextPage}
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: void query.fetchNextPage().
               */ () => void query.fetchNextPage()
            }
          >
            Ещё встречи
          </Button>
        )}
        <Link className="text-link" to="/meetings?view=past">
          Вся история встреч
        </Link>
      </section>
    </>
  );
}
