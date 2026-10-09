import { useEffect, useId, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  CalendarDays,
  Clapperboard,
  FileArchive,
  Info,
  Play,
  Search,
  Users,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { useConferences } from "../queries";
import { formatDate } from "../utils";
import type {
  Conference,
  ConferenceHistory,
  ConferenceRecording,
} from "../types";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { RecordingActions } from "../components/RecordingActions";
import {
  formatRecordingDuration,
  formatRecordingSize,
  playableRecordings,
  recordingDate,
  recordingDetailPath,
  recordingMediaFile,
  recordingPreviewFile,
} from "../recordingPresentation";
import "./recordings.css";

const pageSize = 20;

/** RecordingCard показывает одну реальную готовую запись выбранной встречи.
 * @args record — запись; conference — встреча; history — доступная историческая сводка.
 * @return компактная строка с превью, метаданными и поддерживаемыми действиями.
 */
export function RecordingCard({
  record,
  conference,
  history,
}: {
  record: ConferenceRecording;
  conference: Conference;
  history?: ConferenceHistory;
}) {
  const [brokenPreview, setBrokenPreview] = useState<string | null>(null);
  const preview = recordingPreviewFile(record);
  const file = recordingMediaFile(record);
  const path = recordingDetailPath(conference.id, record.uuid);
  const hasPreview = !!preview?.url && brokenPreview !== preview.url;
  const audioOnly = record.mode === "audio_only";
  const tracks = record.mode === "individual_tracks";
  const modeLabel = tracks
    ? "Аудиомикс и дорожки"
    : audioOnly
      ? "Аудиозапись"
      : "Видеозапись";
  return (
    <article className="recordings-row" data-testid="recordings-row">
      <Link
        to={path}
        className={`recordings-thumbnail${hasPreview ? "" : " recordings-thumbnail-placeholder"}${audioOnly ? " recordings-thumbnail-audio" : ""}${tracks ? " recordings-thumbnail-tracks" : ""}`}
        aria-label={`Смотреть запись: ${conference.title}`}
      >
        {hasPreview ? (
          <img
            src={preview!.url}
            alt=""
            loading="lazy"
            onError={() => setBrokenPreview(preview!.url!)}
          />
        ) : (
          <>
            <span className="recordings-thumbnail-label">{modeLabel}</span>
            {audioOnly ? (
              <span className="recordings-thumbnail-wave" aria-hidden="true">
                {Array.from({ length: 19 }, (_, index) => (
                  <i
                    key={index}
                    style={{ height: `${12 + ((index * 13) % 37)}px` }}
                  />
                ))}
              </span>
            ) : tracks ? (
              <span
                className="recordings-thumbnail-tracks-art"
                aria-hidden="true"
              >
                <FileArchive size={25} />
                <span>
                  {[0, 1, 2].map((row) => (
                    <i key={row}>
                      <b />
                      <b />
                      <b />
                      <b />
                      <b />
                      <b />
                    </i>
                  ))}
                </span>
              </span>
            ) : (
              <span className="recordings-thumbnail-art" aria-hidden="true">
                <Clapperboard size={27} />
                <span>
                  <i />
                  <i />
                  <i />
                </span>
              </span>
            )}
          </>
        )}
        {record.durationSec !== undefined && (
          <span className="recordings-thumbnail-duration">
            {formatRecordingDuration(record.durationSec)}
          </span>
        )}
      </Link>
      <div className="recordings-row-info">
        <Link to={path} className="recordings-row-title">
          {conference.title}
        </Link>
        <p className="recordings-meta">
          <span>{recordingDate(record)}</span>
          <span aria-hidden="true">·</span>
          <span>{modeLabel}</span>
        </p>
        <p className="recordings-meta">
          {history && (
            <span>
              <Users size={13} aria-hidden="true" />
              {history.participantCount} участников встречи
            </span>
          )}
          {history && <span aria-hidden="true">·</span>}
          <span>{formatRecordingSize(file?.sizeBytes)}</span>
        </p>
        {history?.owner.displayName && (
          <p className="recordings-row-owner">
            Организатор: {history.owner.displayName}
          </p>
        )}
      </div>
      <div className="recordings-row-actions">
        <Link to={path} className="button button-primary recordings-watch">
          <Play size={15} aria-hidden="true" /> Смотреть
        </Link>
        <RecordingActions
          title={conference.title}
          path={path}
          downloadUrl={file?.url}
        />
      </div>
    </article>
  );
}

/** RecordingsPage открывает записи одной выбранной встречи без глобального каталога.
 * Список встреч читается курсором; файлы запрашиваются только для выбранной встречи,
 * поэтому страница не делает запрос материалов для каждой строки истории.
 * @return адаптивный выбор встречи, страницы её доступных записей и действия просмотра.
 */
export function RecordingsPage() {
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const conferenceId = params.get("conference") || "";
  const [meetingView, setMeetingView] = useState<"past" | "active">("past");
  const conferences = useConferences({ view: meetingView });
  const [search, setSearch] = useState("");
  const [period, setPeriod] = useState("all");
  const [sort, setSort] = useState("newest");
  const selectionId = useId();
  const loadedConferences = [
    ...new Map(
      (conferences.data?.pages.flatMap((page) => page.items) || []).map(
        (item) => [item.id, item],
      ),
    ).values(),
  ];
  const visibleConferences = loadedConferences.filter((item) =>
    item.title
      .toLocaleLowerCase("ru-RU")
      .includes(search.trim().toLocaleLowerCase("ru-RU")),
  );
  const firstConferenceId = loadedConferences[0]?.id;
  useEffect(() => {
    if (conferenceId || !firstConferenceId) return;
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.set("conference", firstConferenceId);
        return next;
      },
      { replace: true },
    );
  }, [conferenceId, firstConferenceId, setParams]);
  const conference = useQuery({
    queryKey: ["recording-conference", user?.id, conferenceId],
    queryFn: ({ signal }) => api.conference(conferenceId, signal),
    enabled: !!user && !!conferenceId,
    retry: false,
  });
  const selected =
    conference.data?.item.id === conferenceId
      ? conference.data.item
      : undefined;
  const records = useInfiniteQuery({
    queryKey: ["conference-recording-pages", user?.id, conferenceId],
    enabled: !!user && !!selected && !conference.isError,
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      api.recordings(conferenceId, signal, {
        limit: pageSize,
        offset: pageParam,
      }),
    getNextPageParam: (last, _pages, offset) =>
      last.items.length === pageSize ? offset + pageSize : undefined,
    retry: false,
  });
  const history = useQuery({
    queryKey: ["recording-conference-history", user?.id, conferenceId],
    queryFn: ({ signal }) => api.history(conferenceId, signal),
    enabled:
      !!user &&
      !!selected &&
      !conference.isError &&
      ["finished", "cancelled"].includes(selected.status),
    retry: false,
  });
  const readable = playableRecordings(
    (records.data?.pages.flatMap((page) => page.items) || []).filter(
      (record) => record.conferenceId === conferenceId,
    ),
  );
  const cutoff =
    period === "all" ? -Infinity : Date.now() - Number(period) * 86_400_000;
  const visible = readable
    .filter(
      (record) =>
        period === "all" ||
        Date.parse(record.startedAt || record.createdAt) >= cutoff,
    )
    .sort((a, b) => {
      const time =
        (Date.parse(a.startedAt || a.createdAt) || 0) -
        (Date.parse(b.startedAt || b.createdAt) || 0);
      return sort === "oldest" ? time : -time;
    });

  /** chooseConference меняет контекст списка, сохраняя независимый серверный кеш.
   * @args id — выбранный идентификатор доступной встречи.
   */
  function chooseConference(id: string) {
    setParams({ conference: id });
    setPeriod("all");
    setSort("newest");
  }

  return (
    <div className="recordings-page">
      <section className="page-heading recordings-heading">
        <div>
          <h1>Записи</h1>
          <p>Записи и материалы выбранной встречи.</p>
        </div>
      </section>
      <section className="recordings-selection" aria-label="Выбор встречи">
        <div
          className="recordings-meeting-views"
          role="group"
          aria-label="Список встреч"
        >
          {(
            [
              ["past", "Завершённые встречи"],
              ["active", "В эфире"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              aria-pressed={meetingView === value}
              onClick={() => {
                setMeetingView(value);
                setParams({});
                setSearch("");
                setPeriod("all");
              }}
            >
              {label}
            </button>
          ))}
        </div>
        <label className="recordings-search-field">
          <span>Поиск по загруженным встречам</span>
          <span className="recordings-search">
            <Search size={17} aria-hidden="true" />
            <input
              type="search"
              aria-label="Поиск по загруженным встречам"
              placeholder="Название встречи"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </span>
        </label>
        <label className="recordings-conference-selector" htmlFor={selectionId}>
          <span>Встреча</span>
          <select
            aria-label="Встреча"
            id={selectionId}
            value={conferenceId}
            onChange={(event) => chooseConference(event.target.value)}
            disabled={!conferenceId && !loadedConferences.length}
          >
            {!conferenceId && <option value="">Выберите встречу</option>}
            {conferenceId &&
              !visibleConferences.some((item) => item.id === conferenceId) && (
                <option value={conferenceId}>
                  {selected?.title || "Выбранная встреча"}
                </option>
              )}
            {visibleConferences.map((item) => (
              <option key={item.id} value={item.id}>
                {item.title}
              </option>
            ))}
          </select>
        </label>
        {conferences.hasNextPage && (
          <Button
            variant="outline"
            busy={conferences.isFetchingNextPage}
            onClick={() => void conferences.fetchNextPage()}
          >
            Ещё встречи
          </Button>
        )}
        {search && !visibleConferences.length && (
          <p className="recordings-selection-note" role="status">
            В загруженных встречах совпадений нет.
          </p>
        )}
        {
          <p className="recordings-selection-note">
            Поиск выполняется по загруженным встречам. Записи загружаются только
            для выбранной встречи.
          </p>
        }
        {conferences.isError && (
          <div className="recordings-error">
            <ErrorNotice>Не удалось загрузить список встреч.</ErrorNotice>
            <Button
              variant="outline"
              onClick={() => void conferences.refetch()}
            >
              Повторить загрузку встреч
            </Button>
          </div>
        )}
      </section>
      {!conferenceId && conferences.isPending && <Loading />}
      {!conferenceId && conferences.isSuccess && !loadedConferences.length && (
        <section className="recordings-empty">
          <Clapperboard size={36} aria-hidden="true" />
          <h2>
            {meetingView === "past"
              ? "Завершённых встреч пока нет"
              : "Встреч в эфире пока нет"}
          </h2>
          <p>После встречи с записью здесь можно будет открыть её материалы.</p>
          <Link className="text-link" to="/meetings">
            Перейти к встречам
          </Link>
        </section>
      )}
      {conferenceId && conference.isPending && <Loading />}
      {conferenceId &&
        (conference.isError || (conference.isSuccess && !selected)) && (
          <section className="recordings-empty">
            <h2>Встреча недоступна</h2>
            <ErrorNotice>
              Не удалось открыть выбранную встречу. Проверьте доступ или
              выберите другую.
            </ErrorNotice>
            <Button variant="outline" onClick={() => void conference.refetch()}>
              Повторить загрузку
            </Button>
          </section>
        )}
      {selected && !conference.isError && (
        <section
          className="recordings-library"
          aria-labelledby="recordings-selected-title"
        >
          <div className="recordings-library-heading">
            <div>
              <h2 id="recordings-selected-title">{selected.title}</h2>
              {history.data?.item.owner.displayName &&
                !history.isError &&
                history.data.item.conference.id === conferenceId && (
                  <p className="recordings-library-context">
                    {formatDate(selected.finishedAt || selected.createdAt)} ·
                    Организатор {history.data.item.owner.displayName}
                  </p>
                )}
            </div>
            <div className="recordings-filters">
              <label>
                <CalendarDays size={16} aria-hidden="true" />
                <select
                  aria-label="Период записей"
                  value={period}
                  onChange={(event) => setPeriod(event.target.value)}
                >
                  <option value="all">Все время</option>
                  <option value="7">Последние 7 дней</option>
                  <option value="30">Последние 30 дней</option>
                  <option value="90">Последние 90 дней</option>
                </select>
              </label>
              <select
                aria-label="Сортировка загруженных записей"
                value={sort}
                onChange={(event) => setSort(event.target.value)}
              >
                <option value="newest">Сначала новые</option>
                <option value="oldest">Сначала старые</option>
              </select>
            </div>
          </div>
          {records.hasNextPage && (
            <p className="recordings-selection-note">
              Период и сортировка применяются к загруженным записям. Нажмите
              «Ещё записи», чтобы расширить список.
            </p>
          )}
          {records.isError ? (
            <div className="recordings-error">
              <ErrorNotice>
                Не удалось загрузить записи этой встречи. Проверьте доступ и
                попробуйте ещё раз.
              </ErrorNotice>
              <Button variant="outline" onClick={() => void records.refetch()}>
                Повторить загрузку записей
              </Button>
            </div>
          ) : records.isPending ? (
            <Loading />
          ) : (
            <>
              <div className="recordings-list">
                {visible.map((record) => (
                  <RecordingCard
                    key={record.uuid}
                    record={record}
                    conference={selected}
                    history={
                      history.isError ||
                      history.data?.item.conference.id !== conferenceId
                        ? undefined
                        : history.data.item
                    }
                  />
                ))}
              </div>
              {!visible.length && (
                <div className="recordings-empty">
                  <Clapperboard size={36} aria-hidden="true" />
                  <h3>
                    {records.hasNextPage
                      ? "Среди загруженных записей совпадений нет"
                      : period !== "all"
                        ? "В выбранном периоде записей нет"
                        : "Доступных записей пока нет"}
                  </h3>
                  <p>
                    {records.hasNextPage
                      ? "На этой странице записей для просмотра нет. Загрузите следующие записи."
                      : "Выберите другую встречу или вернитесь сюда позже."}
                  </p>
                </div>
              )}
            </>
          )}
          {records.hasNextPage && !records.isError && (
            <div className="recordings-pagination">
              <Button
                variant="outline"
                busy={records.isFetchingNextPage}
                onClick={() => void records.fetchNextPage()}
              >
                Ещё записи
              </Button>
            </div>
          )}
          {["finished", "cancelled"].includes(selected.status) && (
            <Link
              className="recordings-history-link"
              to={`/history/${encodeURIComponent(selected.id)}`}
            >
              Открыть историю встречи
            </Link>
          )}
        </section>
      )}
      <p className="recordings-retention">
        <Info size={16} aria-hidden="true" />
        <span>
          Готовые записи доступны 7 дней после завершения записи. Ссылка на файл
          временная; ссылка на страницу не открывает доступ посторонним.
        </span>
      </p>
    </div>
  );
}
