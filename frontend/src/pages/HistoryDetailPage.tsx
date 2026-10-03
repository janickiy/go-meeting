import { useId } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { api } from "../api";
import { isAdmitted } from "../collaboration";
import { AnalyticsPanel } from "../components/AnalyticsPanel";
import { ChatPanel } from "../components/ChatPanel";
import { RecordingPanel } from "../components/RecordingPanel";
import { ErrorNotice, Loading } from "../components/ui";
import { useMembership } from "../queries";
import { formatDate } from "../utils";
import "./history-notifications.css";

const sections = [
  ["overview", "Обзор"],
  ["recording", "Запись"],
  ["transcript", "Расшифровка"],
  ["summary", "Итоги ИИ"],
  ["chat", "Чат"],
  ["analytics", "Аналитика"],
] as const;
type Section = (typeof sections)[number][0];

/** Материалы завершённой встречи загружаются только при открытии нужной вкладки. */
export function HistoryDetailPage() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const tabId = useId();
  const requested = params.get("section");
  const section: Section = sections.some(([value]) => value === requested)
    ? (requested as Section)
    : "overview";
  const history = useQuery({
    queryKey: ["history", id],
    queryFn: ({ signal }) => api.history(id, signal),
    enabled: !!id,
    retry: false,
  });
  const membership = useMembership(id, false);

  function openSection(nextSection: Section) {
    const next = new URLSearchParams(params);
    next.set("section", nextSection);
    if (nextSection === "summary" || nextSection === "transcript")
      next.set("tab", nextSection);
    setParams(next, { replace: true });
  }

  if (history.isPending || membership.isPending) return <Loading />;
  if (history.isError || membership.isError || !history.data)
    return (
      <section className="content-card">
        <h1>История встречи недоступна</h1>
        <ErrorNotice error={history.error || membership.error} />
        <Link className="text-link" to="/history">
          Вернуться к истории
        </Link>
      </section>
    );
  const item = history.data.item;
  if (!isAdmitted(membership.data))
    return (
      <section className="content-card">
        <h1>История встречи недоступна</h1>
        <p>Для просмотра материалов требуется доступ участника.</p>
        <Link className="text-link" to="/history">
          Вернуться к истории
        </Link>
      </section>
    );

  return (
    <>
      <Link className="back-link" to="/history">
        <ArrowLeft size={17} />
        История встреч
      </Link>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ИСТОРИЯ ВСТРЕЧИ</span>
          <h1>{item.conference.title}</h1>
          <p>
            {formatDate(
              item.conference.finishedAt || item.conference.createdAt,
            )}
          </p>
        </div>
      </section>
      <div
        className="insight-tabs history-tabs"
        role="tablist"
        aria-label="Материалы встречи"
      >
        {sections.map(([value, label], index) => (
          <button
            key={value}
            type="button"
            role="tab"
            id={`${tabId}-${value}`}
            aria-controls={`${tabId}-panel`}
            aria-selected={section === value}
            tabIndex={section === value ? 0 : -1}
            onClick={() => openSection(value)}
            onKeyDown={(event) => {
              if (
                !["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)
              )
                return;
              event.preventDefault();
              const target =
                event.key === "Home"
                  ? 0
                  : event.key === "End"
                    ? sections.length - 1
                    : (index +
                        (event.key === "ArrowRight" ? 1 : -1) +
                        sections.length) %
                      sections.length;
              const next = sections[target][0];
              openSection(next);
              document.getElementById(`${tabId}-${next}`)?.focus();
            }}
          >
            {label}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id={`${tabId}-panel`}
        aria-labelledby={`${tabId}-${section}`}
      >
        {section === "overview" && (
          <section
            className="content-card history-summary"
            aria-label="Обзор встречи"
          >
            <h2>Обзор</h2>
            <dl>
              <div>
                <dt>Организатор</dt>
                <dd>{item.owner.displayName || "Организатор встречи"}</dd>
              </div>
              <div>
                <dt>Длительность</dt>
                <dd>
                  {item.durationSec === null
                    ? "—"
                    : `${Math.max(1, Math.round(item.durationSec / 60))} мин`}
                </dd>
              </div>
              <div>
                <dt>Участников</dt>
                <dd>{item.participantCount}</dd>
              </div>
              <div>
                <dt>Записи</dt>
                <dd>
                  {item.recordings.ready} готово · {item.recordings.processing}{" "}
                  обрабатывается
                </dd>
              </div>
            </dl>
            {item.participants.length > 0 && (
              <>
                <h3>Участники</h3>
                <ul>
                  {item.participants.map((person) => (
                    <li key={person.id}>{person.displayName}</li>
                  ))}
                </ul>
                {item.participantsTruncated && (
                  <p className="field-hint">Показана часть участников.</p>
                )}
              </>
            )}
          </section>
        )}
        {["recording", "transcript", "summary"].includes(section) && (
          <RecordingPanel
            conference={item.conference}
            membership={membership.data || undefined}
            showInsights
          />
        )}
        {section === "chat" && item.chatAvailable && membership.data && (
          <ChatPanel conferenceId={id} membership={membership.data} readOnly />
        )}
        {section === "chat" && !item.chatAvailable && (
          <p className="content-card">Чат для этой встречи недоступен.</p>
        )}
        {section === "analytics" && (
          <AnalyticsPanel conferenceId={id} active={false} />
        )}
      </div>
    </>
  );
}
