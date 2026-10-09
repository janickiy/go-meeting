import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { ApiError, api } from "../api";
import { useAuth } from "../auth";
import { Button, ErrorNotice, Loading } from "../components/ui";
import type { AdminSummary } from "../types";
import { useCapabilities } from "../useCapabilities";
import { formatDate } from "../utils";
import "./admin-page.css";

const dependencies = [
  ["postgres", "PostgreSQL"],
  ["redis", "Redis"],
  ["rabbitmq", "RabbitMQ"],
  ["minio", "MinIO"],
] as const;

/** Показывает только агрегаты, доступные пользователю с подтверждённым сервером правом администратора. */
export function AdminPage() {
  const { user } = useAuth();
  const localAdmin = user?.isAdmin === true;
  const summary = useQuery({
    queryKey: ["admin-summary", user?.id],
    queryFn: ({ signal }) => api.adminSummary(signal),
    enabled: localAdmin,
    retry: false,
    refetchInterval: 30_000,
  });
  const capabilities = useCapabilities();
  const serverDenied =
    summary.error instanceof ApiError && summary.error.status === 403;

  if (!localAdmin || serverDenied) {
    return (
      <section className="content-card admin-denied" role="alert">
        <h1>Нет доступа к разделу операций</h1>
        <p>
          {serverDenied
            ? "Административное право изменилось. Обновите страницу, чтобы проверить текущую учётную запись."
            : "Этот раздел доступен только администраторам сервиса."}
        </p>
      </section>
    );
  }

  const item = summary.data?.item;
  return (
    <div className="admin-page">
      <header className="page-heading admin-heading">
        <div>
          <h1>Состояние сервиса</h1>
          <p>Сводка по встречам, обработке записей и зависимостям.</p>
        </div>
        <Button
          variant="outline"
          busy={summary.isFetching}
          onClick={() => void summary.refetch()}
        >
          <RefreshCw size={17} aria-hidden="true" /> Обновить
        </Button>
      </header>
      {summary.isPending ? (
        <Loading />
      ) : summary.isError ? (
        <section className="content-card">
          <ErrorNotice error={summary.error} />
          <Button variant="outline" onClick={() => void summary.refetch()}>
            Попробовать снова
          </Button>
        </section>
      ) : item ? (
        <AdminSummaryView
          item={item}
          buildVersion={capabilities.data?.buildVersion}
        />
      ) : null}
    </div>
  );
}

function AdminSummaryView({
  item,
  buildVersion,
}: {
  item: AdminSummary;
  buildVersion?: string;
}) {
  const metrics = [
    ["Активные встречи", item.activeConferences],
    ["Участники активных встреч", item.joinedParticipants],
    ["Записи в работе", item.activeRecordings],
    ["Задания в очереди и обработке", item.queuedJobs],
    ["Ошибки заданий за 24 часа", item.failedJobs24h],
    ["Ошибки записей за 24 часа", item.failedRecordings24h],
    ["Ошибки расшифровки за 24 часа", item.failedTranscriptions24h],
  ] as const;
  return (
    <>
      <p className="admin-as-of" role="status">
        Данные на <time dateTime={item.asOf}>{formatDate(item.asOf)}</time>
      </p>
      <section aria-labelledby="admin-counts-heading">
        <h2 id="admin-counts-heading">Текущая нагрузка и ошибки</h2>
        <dl className="admin-metrics">
          {metrics.map(([label, count]) => (
            <div className="content-card admin-metric" key={label}>
              <dt>{label}</dt>
              <dd>{new Intl.NumberFormat("ru-RU").format(count)}</dd>
            </div>
          ))}
        </dl>
      </section>
      <section className="content-card" aria-labelledby="admin-health-heading">
        <h2 id="admin-health-heading">Доступность компонентов</h2>
        <ul className="admin-health-list">
          <HealthRow label="API" ready={item.apiReady} />
          <HealthRow label="Media Worker" ready={item.mediaWorkerReady} />
          {dependencies.map(([key, label]) => (
            <HealthRow
              key={key}
              label={label}
              ready={item.dependencies?.[key] === true}
            />
          ))}
        </ul>
      </section>
      <section
        className="content-card"
        aria-labelledby="admin-failures-heading"
      >
        <h2 id="admin-failures-heading">Последние ошибки</h2>
        {item.recentFailures.length ? (
          <div className="admin-table-wrap">
            <table className="admin-failures">
              <thead>
                <tr>
                  <th scope="col">Время</th>
                  <th scope="col">Подсистема</th>
                  <th scope="col">Код</th>
                </tr>
              </thead>
              <tbody>
                {item.recentFailures.map((failure, index) => (
                  <tr key={`${failure.at}-${failure.kind}-${index}`}>
                    <td>
                      <time dateTime={failure.at}>
                        {formatDate(failure.at)}
                      </time>
                    </td>
                    <td>{failure.kind}</td>
                    <td>
                      <code>{failure.code}</code>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="admin-empty">За последние 24 часа ошибок нет.</p>
        )}
      </section>
      {buildVersion && (
        <p className="admin-build">
          Сборка: <code>{buildVersion}</code>
        </p>
      )}
    </>
  );
}

function HealthRow({ label, ready }: { label: string; ready: boolean }) {
  return (
    <li>
      <span>{label}</span>
      <span className={`admin-health-status ${ready ? "is-ready" : "is-down"}`}>
        {ready ? "Готов" : "Недоступен"}
      </span>
    </li>
  );
}
