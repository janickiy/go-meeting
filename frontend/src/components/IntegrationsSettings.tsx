import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useAuth } from "../auth";
import type {
  IntegrationCapabilities,
  NotificationPreferences,
} from "../types";
import { formatDate } from "../utils";
import { Button, ErrorNotice, Loading } from "./ui";

const modes = {
  noop: "Не настроено",
  mock: "Тестовый режим",
  http: "Подключён провайдер",
  smtp: "Настроена отправка почты",
};
const preferenceLabels: Record<keyof NotificationPreferences, string> = {
  invitation: "Приглашения и изменения встреч",
  reminder: "Напоминания о встречах",
  recording: "Готовность записи",
  summary: "Расшифровка и итоги встречи",
  email: "Email",
  push: "Push-уведомления",
};

/**
 * Отображает настройки каналов и подключения календаря без сбора токенов.
 * @return Карточки уведомлений и интеграций текущего пользователя.
 */
export function IntegrationsSettings() {
  const { user } = useAuth();
  const client = useQueryClient();
  const capabilities = useQuery({
    queryKey: ["integration-capabilities", user?.id],
    queryFn: ({ signal }) => api.integrationCapabilities(signal),
  });
  const preferences = useQuery({
    queryKey: ["notification-preferences", user?.id],
    queryFn: ({ signal }) => api.notificationPreferences(signal),
  });
  const calendars = useQuery({
    queryKey: ["calendars", user?.id],
    queryFn: ({ signal }) => api.calendars(signal),
  });
  const calendarAction = useMutation({
    mutationFn: async (action: string) => {
      if (action === "mock") return api.connectMockCalendar();
      if (action === "connect") {
        const response = await api.calendarConnect("generic");
        const url = new URL(response.authUrl);
        if (url.protocol !== "https:")
          throw new Error("Сервер вернул небезопасный адрес авторизации.");
        window.location.assign(url.href);
        return;
      }
      return api.disconnectCalendar(action);
    },
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: ["calendars", user?.id] }),
  });
  return (
    <>
      <section
        className="content-card settings-section"
        id="notification-settings"
      >
        <h2>Уведомления</h2>
        <p className="field-hint">
          Выберите события и разрешённые внешние каналы. Email и push управляют
          автоматическими уведомлениями. Приглашения, отправленные
          организатором, приходят на email отдельно.
        </p>
        <ErrorNotice error={preferences.error || capabilities.error} />
        {preferences.isPending || capabilities.isPending ? (
          <Loading />
        ) : (
          preferences.data &&
          capabilities.data && (
            <PreferencesForm
              key={user?.id}
              initial={preferences.data}
              capabilities={capabilities.data}
            />
          )
        )}
      </section>
      <section
        className="content-card settings-section"
        id="integration-settings"
      >
        <h2>Календарь</h2>
        <p className="field-hint">
          Созданные вами запланированные встречи синхронизируются автоматически.
          Смена времени и отмена передаются в подключённый календарь.
        </p>
        <ErrorNotice error={calendars.error || calendarAction.error} />
        {capabilities.data && (
          <p>
            Интеграция: <strong>{modes[capabilities.data.calendar]}</strong>
          </p>
        )}
        {capabilities.data?.calendar === "mock" && (
          <p className="demo-notice">
            Тестовый календарь не создаёт события во внешнем сервисе.
          </p>
        )}
        {calendars.isPending ? (
          <Loading />
        ) : (
          <ul className="integration-list">
            {calendars.data?.items.map((connection) => (
              <li key={connection.id}>
                <div>
                  <strong>
                    {connection.provider === "mock"
                      ? "Тестовый календарь"
                      : "Внешний календарь"}
                  </strong>
                  <p className="field-hint">
                    {connection.status === "connected"
                      ? "Подключён"
                      : "Отключён"}{" "}
                    · {connection.calendarId}
                  </p>
                </div>
                {connection.status === "connected" && (
                  <Button
                    variant="outline"
                    busy={calendarAction.isPending}
                    onClick={() => calendarAction.mutate(connection.id)}
                  >
                    Отключить календарь
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
        {!calendars.isPending &&
          !calendars.data?.items.some(
            (connection) => connection.status === "connected",
          ) && <p className="muted">Нет подключённых календарей.</p>}
        <div className="meeting-actions">
          {capabilities.data?.calendar === "mock" &&
            capabilities.data.mockConnectAllowed && (
              <Button
                variant="outline"
                busy={calendarAction.isPending}
                onClick={() => calendarAction.mutate("mock")}
              >
                Подключить тестовый календарь
              </Button>
            )}
          {capabilities.data?.calendar === "http" &&
            capabilities.data.calendarOAuthConfigured && (
              <Button
                busy={calendarAction.isPending}
                onClick={() => calendarAction.mutate("connect")}
              >
                Подключить календарь
              </Button>
            )}
        </div>
        {capabilities.data?.calendar === "noop" ||
        (capabilities.data?.calendar === "http" &&
          !capabilities.data.calendarOAuthConfigured) ? (
          <p className="field-hint">
            Подключение станет доступно после настройки интеграции
            администратором.
          </p>
        ) : null}
        <p className="field-hint">
          Разрешение выдаётся на странице календарного сервиса. Пароли и токены
          провайдера вводить здесь не нужно.
        </p>
      </section>
    </>
  );
}

/**
 * Сохраняет независимые предпочтения событий и каналов после явного действия.
 * @args initial — сохранённые значения; capabilities — доступность провайдеров.
 * @return Форма с безопасными состояниями выключенных и тестовых каналов.
 */
function PreferencesForm({
  initial,
  capabilities,
}: {
  initial: NotificationPreferences;
  capabilities: IntegrationCapabilities;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const [draft, setDraft] = useState(initial);
  const save = useMutation({
    mutationFn: () => api.saveNotificationPreferences(draft),
    onSuccess: (data) =>
      client.setQueryData(["notification-preferences", user?.id], data),
  });
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        save.mutate();
      }}
    >
      <fieldset className="preferences-fieldset">
        <legend>События</legend>
        {(["invitation", "reminder", "recording", "summary"] as const).map(
          (name) => (
            <label className="preference-option" key={name}>
              <input
                type="checkbox"
                checked={draft[name]}
                onChange={(event) =>
                  setDraft({ ...draft, [name]: event.target.checked })
                }
              />
              {preferenceLabels[name]}
            </label>
          ),
        )}
      </fieldset>
      <fieldset className="preferences-fieldset">
        <legend>Каналы доставки</legend>
        {(["email", "push"] as const).map((name) => (
          <label className="preference-option" key={name}>
            <input
              type="checkbox"
              disabled={capabilities[name] === "noop"}
              checked={draft[name]}
              onChange={(event) =>
                setDraft({ ...draft, [name]: event.target.checked })
              }
            />
            {preferenceLabels[name]}{" "}
            <span className="field-hint">{modes[capabilities[name]]}</span>
          </label>
        ))}
      </fieldset>
      <p className="field-hint">
        Push требует зарегистрированного поддерживаемого устройства. Этот
        браузер не запрашивает разрешение, если доставка не настроена.
      </p>
      {(capabilities.email === "mock" || capabilities.push === "mock") && (
        <p className="demo-notice">
          Тестовый канал не отправляет реальные сообщения.
        </p>
      )}
      <ErrorNotice error={save.error} />
      <Button busy={save.isPending} type="submit">
        Сохранить настройки
      </Button>
      {save.isSuccess && <p role="status">Настройки сохранены.</p>}
    </form>
  );
}

/**
 * Завершает OAuth только в действующей сессии и сразу удаляет код из адреса.
 * @return Безопасное подтверждение либо предложение повторить подключение.
 */
export function CalendarCallback() {
  const { provider = "" } = useParams();
  const auth = useAuth();
  const client = useQueryClient();
  const [parameters] = useState(
    () => new URLSearchParams(window.location.search),
  );
  const request = useRef<Promise<unknown> | null>(null);
  const [state, setState] = useState<"pending" | "success" | "error">(
    "pending",
  );
  useEffect(() => {
    window.history.replaceState(
      window.history.state,
      "",
      window.location.pathname,
    );
  }, []);
  useEffect(() => {
    if (auth.loading) return;
    let active = true;
    const code = parameters.get("code"),
      token = parameters.get("state");
    if (
      !auth.user ||
      !code ||
      !token ||
      parameters.has("error") ||
      provider !== "generic"
    ) {
      setState("error");
      return;
    }
    request.current ||= api.calendarCallback(provider, code, token);
    request.current.then(
      () => {
        if (active) {
          setState("success");
          void client.invalidateQueries({
            queryKey: ["calendars", auth.user?.id],
          });
        }
      },
      () => {
        if (active) setState("error");
      },
    );
    return () => {
      active = false;
    };
  }, [auth.loading, auth.user, provider, parameters, client]);
  return (
    <div className="page-center">
      <section className="content-card">
        <h1>Подключение календаря</h1>
        {state === "pending" ? (
          <Loading />
        ) : state === "success" ? (
          <p role="status">Календарь подключён.</p>
        ) : (
          <ErrorNotice>
            Не удалось завершить подключение. Войдите в аккаунт и начните
            подключение заново в настройках.
          </ErrorNotice>
        )}
        <Link className="text-link" to="/app/settings">
          Перейти в настройки
        </Link>
      </section>
    </div>
  );
}

/**
 * Показывает состояние автоматической синхронизации организатору встречи.
 * @args conferenceId, userId — идентификаторы встречи и пользователя.
 * @return Небольшая карточка состояния без внешних секретов.
 */
export function ConferenceCalendarStatus({
  conferenceId,
  userId,
}: {
  conferenceId: string;
  userId: string;
}) {
  const query = useQuery({
    queryKey: ["calendar-sync", userId, conferenceId],
    queryFn: ({ signal }) => api.calendarSync(conferenceId, signal),
    refetchInterval: 15_000,
    retry: false,
  });
  const labels: Record<string, string> = {
    pending: "Ожидает синхронизации",
    synced: "Синхронизировано",
    cancelled: "Отменено в календаре",
    failed: "Ошибка синхронизации",
  };
  if (!query.data?.items.length && !query.isError) return null;
  return (
    <section className="content-card">
      <h2>Календарь встречи</h2>
      <ErrorNotice error={query.error} />
      {!query.isError &&
        query.data?.items.map((item) => (
          <p key={`${item.provider}:${item.externalCalendarId}`}>
            <strong>
              {item.provider === "mock"
                ? "Тестовый календарь"
                : "Внешний календарь"}
            </strong>{" "}
            · {labels[item.syncStatus] || "Статус обновляется"}
            {item.lastSyncedAt ? ` · ${formatDate(item.lastSyncedAt)}` : ""}
          </p>
        ))}
    </section>
  );
}
