import { useState } from "react";
import { Link, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  CalendarDays,
  Check,
  Info,
  LogIn,
  LogOut,
  Play,
  Square,
  Users,
  Video,
  X,
} from "lucide-react";
import { api } from "../api";
import { RealtimePanel } from "../components/RealtimePanel";
import { RecordingPanel } from "../components/RecordingPanel";
import type { ModerationAction } from "../types";
import { useAuth } from "../auth";
import { useConference, useParticipants, useMembership } from "../queries";
import { isAdmitted } from "../collaboration";
import { useRealtime } from "../realtime";
import { WaitingRoomPanel } from "../components/WaitingRoomPanel";
import { ChatPanel } from "../components/ChatPanel";
import { HandReactionsPanel } from "../components/HandReactionsPanel";
import { EditSchedule } from "../components/ConferenceModals";
import { formatDate, initials, inviteLink } from "../utils";
import {
  Button,
  CopyLink,
  ErrorNotice,
  Loading,
  Modal,
  StatusBadge,
} from "../components/ui";

/**
 * ConferencePage координирует сведения встречи, членство, единственный WebSocket, допуск, медиа, чат, запись и историю.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function ConferencePage() {
  const { id = "" } = useParams();
  const { user } = useAuth();
  const client = useQueryClient();
  const query = useConference(id);
  const self = useMembership(id);
  const membership = self.data || undefined;
  const admitted = isAdmitted(membership);
  const participants = useParticipants(id, admitted);
  const people =
    participants.data?.pages.flatMap(
      /**
       * Обработчик flatMap преобразует текущий элемент в данные или представление результирующего списка.
       *
       * @parameters:
       *   - page — изолированная страница Playwright.
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ (page) => page.items,
    ) || [];
  const closed = ["finished", "cancelled"].includes(
    query.data?.item.status || "",
  );
  const live = useRealtime(
    id,
    admitted && membership?.status === "joined" && !closed,
  );
  const history = useQuery({
    queryKey: ["history", user?.id, id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @parameters:
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.history(id, signal).
     */
    queryFn: ({ signal }) => api.history(id, signal),
    enabled: admitted && closed,
  });
  const [confirm, setConfirm] = useState<"finish" | "cancel" | null>(null);
  const [editingSchedule, setEditingSchedule] = useState(false);
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @parameters:
     *   - action ("start" | "finish" | "cancel" | "join" | "leave") — разрешённое действие управления либо асинхронная операция.
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */
    mutationFn: (action: "start" | "finish" | "cancel" | "join" | "leave") =>
      action === "join" || action === "leave"
        ? api.membership(id, action).then(
            /**
             * Обработчик then выполняет переданный шаг вызова then в конференциях, расписании и истории.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          )
        : api.transition(id, action).then(
            /**
             * Обработчик then выполняет переданный шаг вызова then в конференциях, расписании и истории.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          ),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["conference"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["membership"] });
      void client.invalidateQueries({ queryKey: ["history"] });
    },
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns вычисленное значение: setConfirm(null).
     */
    onSuccess: () => setConfirm(null),
  });
  const moderation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @parameters:
     *   - объект параметров: participantId — идентификатор членства целевого участника; action — разрешённое действие управления либо асинхронная операция.
     *
     * @returns вычисленное значение: api.moderate(id, participantId, action).
     */
    mutationFn: ({
      participantId,
      action,
    }: {
      participantId: string;
      action: ModerationAction;
    }) => api.moderate(id, participantId, action),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["membership"] });
    },
  });
  if (query.isPending) return <Loading />;
  if (query.isError || !query.data)
    return (
      <div className="content-card">
        <ErrorNotice error={query.error || new Error()} />
        <Link to="/conferences" className="text-link">
          <ArrowLeft size={16} />К моим конференциям
        </Link>
      </div>
    );
  const conference = query.data.item;
  const owner = conference.ownerId === user?.id;
  const roleNames = {
    owner: "Организатор",
    co_host: "Соорганизатор",
    participant: "Участник",
    guest: "Гость",
  };
  const presence = {
    joined: "Присоединился",
    left: "Вышел",
    waiting: "Ожидает",
    rejected: "Отклонён",
    kicked: "Исключён",
  };
  return (
    <>
      <Link className="back-link" to="/conferences">
        <ArrowLeft size={17} />
        Мои конференции
      </Link>
      <section className="page-heading conference-heading">
        <div>
          <span className="eyebrow">КОНФЕРЕНЦИЯ</span>
          <h1>{conference.title}</h1>
          <p>
            <CalendarDays size={15} />
            Создана {formatDate(conference.createdAt)}
          </p>
          {conference.scheduledAt && (
            <p>
              Запланирована: {formatDate(conference.scheduledAt)}
              {conference.plannedDurationMin
                ? ` · ${conference.plannedDurationMin} мин`
                : ""}
            </p>
          )}
        </div>
        <StatusBadge status={conference.status} />
      </section>
      <ErrorNotice error={mutation.error} />
      <ErrorNotice error={moderation.error} />
      <ErrorNotice error={self.error} />
      <div className="conference-grid">
        <section className="content-card meeting-card">
          <div className="meeting-card-symbol">
            <Video size={35} />
          </div>
          <h2>
            {closed
              ? "Встреча закрыта"
              : conference.status === "active"
                ? "Конференция началась"
                : "Всё готово к встрече"}
          </h2>
          <p className="muted">
            {closed
              ? "Участники и история конференции сохранены."
              : owner
                ? "Управляйте встречей и приглашайте участников."
                : "Присоединитесь к встрече, когда будете готовы."}
          </p>
          <div className="meeting-actions">
            {!closed &&
              conference.status !== "scheduled" &&
              !["kicked", "waiting", "rejected"].includes(
                membership?.status || "",
              ) && (
                <Button
                  busy={mutation.isPending}
                  variant={
                    membership?.status === "joined" ? "secondary" : "primary"
                  }
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: mutation.mutate( membership?.status === "joined" ? "leave" : "join", ).
                     */ () =>
                      mutation.mutate(
                        membership?.status === "joined" ? "leave" : "join",
                      )
                  }
                  disabled={self.isPending || self.isError}
                >
                  {membership?.status === "joined" ? (
                    <>
                      <LogOut size={18} />
                      Покинуть конференцию
                    </>
                  ) : (
                    <>
                      <LogIn size={18} />
                      Присоединиться
                    </>
                  )}
                </Button>
              )}
            {owner && ["created", "scheduled"].includes(conference.status) && (
              <>
                <Button
                  variant="outline"
                  busy={mutation.isPending}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: mutation.mutate("start").
                     */ () => mutation.mutate("start")
                  }
                >
                  <Play size={17} />
                  Начать конференцию
                </Button>
                <Button
                  variant="secondary"
                  disabled={mutation.isPending}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: setConfirm("cancel").
                     */ () => setConfirm("cancel")
                  }
                >
                  <X size={17} />
                  Отменить конференцию
                </Button>
                {conference.status === "scheduled" && (
                  <Button
                    variant="secondary"
                    onClick={
                      /**
                       * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       *
                       * @returns вычисленное значение: setEditingSchedule(true).
                       */ () => setEditingSchedule(true)
                    }
                    disabled={mutation.isPending}
                  >
                    Изменить расписание
                  </Button>
                )}
              </>
            )}
            {owner && conference.status === "active" && (
              <Button
                variant="danger"
                disabled={mutation.isPending}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленное значение: setConfirm("finish").
                   */ () => setConfirm("finish")
                }
              >
                <Square size={16} />
                Завершить конференцию
              </Button>
            )}
          </div>
          {membership?.status === "joined" && !closed && (
            <p className="membership-note">
              <Check size={15} />
              Вы присоединились к конференции
            </p>
          )}
          {conference.status === "scheduled" &&
            (admitted || membership?.admissionState === "waiting") && (
              <p className="membership-note">
                <Check size={15} />
                Встреча добавлена в ваш список.{" "}
                {admitted
                  ? "Войти можно после её начала."
                  : "Организатор рассмотрит запрос на вход после начала встречи."}
              </p>
            )}
          <div className="video-notice">
            <Info size={18} />
            <span>
              После присоединения можно включить камеру и микрофон в блоке
              медиасвязи. Организатор может включить общую запись встречи.
            </span>
          </div>
          {conference.startedAt && (
            <p className="field-hint">
              Начало: {formatDate(conference.startedAt)}
            </p>
          )}
          {conference.finishedAt && (
            <p className="field-hint">
              Завершение: {formatDate(conference.finishedAt)}
            </p>
          )}
        </section>
        {admitted && (
          <aside className="content-card invitation-card">
            <span className="eyebrow">ПРИГЛАСИТЕ КОЛЛЕГ</span>
            <h2>
              Встреча начинается
              <br />с приглашения
            </h2>
            <p className="muted">
              Отправьте ссылку тем, с кем хотите встретиться.
            </p>
            <CopyLink value={inviteLink(conference.inviteCode)} />
            <div className="invite-code">
              <span>Код приглашения</span>
              <code>{conference.inviteCode}</code>
            </div>
            <p className="field-hint">
              Для присоединения нужен аккаунт Meet.
              {closed ? " Эта конференция уже закрыта." : ""}
            </p>
          </aside>
        )}
      </div>
      <WaitingRoomPanel
        conferenceId={id}
        membership={membership}
        participants={people}
        active={conference.status === "active"}
        closed={closed}
      />
      {admitted && membership?.status === "joined" && !closed && (
        <RealtimePanel conferenceId={id} membership={membership} live={live} />
      )}
      {admitted &&
        membership?.status === "joined" &&
        conference.status === "active" && (
          <HandReactionsPanel
            conferenceId={id}
            membership={membership}
            participants={people}
            live={live}
          />
        )}
      {membership?.status === "kicked" && (
        <ErrorNotice>
          Организатор исключил вас из конференции. Повторное присоединение
          недоступно.
        </ErrorNotice>
      )}
      <RecordingPanel conference={conference} membership={membership} />
      {admitted && membership && (
        <ChatPanel
          conferenceId={id}
          membership={membership}
          readOnly={
            conference.status !== "active" || membership.status !== "joined"
          }
          readOnlyReason={
            closed
              ? undefined
              : "Присоединитесь к активной встрече, чтобы отправлять сообщения."
          }
        />
      )}
      {admitted && closed && (
        <section
          className="content-card history-summary"
          aria-label="История встречи"
        >
          <h2>Итоги встречи</h2>
          <ErrorNotice error={history.error} />
          {history.isPending ? (
            <Loading />
          ) : (
            history.data && (
              <dl>
                <div>
                  <dt>Организатор</dt>
                  <dd>
                    {history.data.item.owner.displayName ||
                      "Организатор встречи"}
                  </dd>
                </div>
                <div>
                  <dt>Длительность</dt>
                  <dd>
                    {history.data.item.durationSec === null
                      ? "—"
                      : `${Math.max(1, Math.round(history.data.item.durationSec / 60))} мин`}
                  </dd>
                </div>
                <div>
                  <dt>Участников</dt>
                  <dd>{history.data.item.participantCount}</dd>
                </div>
                <div>
                  <dt>Записи</dt>
                  <dd>
                    {history.data.item.recordings.ready} готово ·{" "}
                    {history.data.item.recordings.processing} обрабатывается
                  </dd>
                </div>
              </dl>
            )
          )}
        </section>
      )}
      {admitted && (
        <section className="content-card participants-card">
          <div className="section-heading">
            <h2>
              <Users size={20} />
              Участники{" "}
              <span className="count-badge">
                {people.length}
                {participants.hasNextPage ? "+" : ""}
              </span>
            </h2>
            <span className="muted small">Обновляется автоматически</span>
          </div>
          <ErrorNotice error={participants.error} />
          {participants.isPending ? (
            <Loading />
          ) : (
            <div className="participant-list">
              {people.map(
                /**
                 * Обработчик people.map преобразует один элемент набора в представление или данные следующего шага.
                 *
                 * @parameters:
                 *   - person — целевое членство участника.
                 *
                 * @returns преобразованное значение текущего элемента для результирующего набора.
                 */ (person) => (
                  <div className="participant-row" key={person.id}>
                    <span
                      className={`avatar ${person.role === "owner" ? "avatar-owner" : ""}`}
                    >
                      {initials(person.displayName)}
                    </span>
                    {!closed &&
                      membership?.status === "joined" &&
                      person.id !== membership.id &&
                      person.role !== "owner" &&
                      isAdmitted(person) &&
                      person.status !== "kicked" &&
                      (membership.role === "owner" ||
                        (membership.role === "co_host" &&
                          person.role === "participant")) && (
                        <div
                          className="participant-controls"
                          aria-label={`Управление: ${person.displayName}`}
                        >
                          {(
                            [
                              ["mute", "микрофон", !!person.microphoneBlocked],
                              ["camera", "видео", !!person.cameraBlocked],
                              ["screen", "экран", !!person.screenBlocked],
                            ] as const
                          )
                            .filter(
                              /**
                               * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
                               *
                               * @parameters:
                               *   - [action] — элементы записи набора, извлечённые по указанным позициям.
                               *
                               * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                               */
                              ([action]) =>
                                action !== "camera" ||
                                membership.role === "owner",
                            )
                            .map(
                              /**
                               * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                               *
                               * @parameters:
                               *   - [action, label, blocked] — элементы записи набора, извлечённые по указанным позициям.
                               *
                               * @returns преобразованное значение текущего элемента для результирующего набора.
                               */ ([action, label, blocked]) => (
                                <Button
                                  key={action}
                                  variant="secondary"
                                  disabled={moderation.isPending}
                                  onClick={
                                    /**
                                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                     *
                                     *
                                     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                                     */ () =>
                                      moderation.mutate({
                                        participantId: person.id,
                                        action: { action, blocked: !blocked },
                                      })
                                  }
                                >
                                  {blocked ? "Разрешить" : "Отключить"} {label}
                                </Button>
                              ),
                            )}
                          {membership.role === "owner" && (
                            <Button
                              variant="outline"
                              disabled={moderation.isPending}
                              onClick={
                                /**
                                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                 *
                                 *
                                 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                                 */ () =>
                                  moderation.mutate({
                                    participantId: person.id,
                                    action: {
                                      action: "role",
                                      role:
                                        person.role === "co_host"
                                          ? "participant"
                                          : "co_host",
                                    },
                                  })
                              }
                            >
                              {person.role === "co_host"
                                ? "Убрать соорганизатора"
                                : "Назначить соорганизатором"}
                            </Button>
                          )}
                          <Button
                            variant="danger"
                            disabled={moderation.isPending}
                            onClick={
                              /**
                               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                               *
                               *
                               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                               */ () => {
                                if (
                                  window.confirm(
                                    `Исключить ${person.displayName}? Повторное присоединение будет запрещено.`,
                                  )
                                )
                                  moderation.mutate({
                                    participantId: person.id,
                                    action: { action: "kick" },
                                  });
                              }
                            }
                          >
                            Исключить
                          </Button>
                        </div>
                      )}
                    <div className="participant-name">
                      <strong>
                        {person.displayName}
                        {person.userId === user?.id && (
                          <span className="muted"> (вы)</span>
                        )}
                      </strong>
                      <span>{roleNames[person.role]}</span>
                      {person.status === "joined" && (
                        <span className="participant-media-status">
                          {person.microphoneBlocked
                            ? "Звук запрещён"
                            : person.microphoneEnabled
                              ? "Микрофон включён"
                              : "Микрофон выключен"}
                          {" · "}
                          {person.cameraBlocked
                            ? "Видео запрещено"
                            : person.cameraEnabled
                              ? "Камера включена"
                              : "Камера выключена"}
                          {person.screenSharing ? " · Показывает экран" : ""}
                        </span>
                      )}
                    </div>
                    <span
                      className={`participant-status ${person.status === "joined" ? "participant-joined" : ""}`}
                    >
                      <span className="presence-dot" />
                      {person.joinedAt ||
                      ["waiting", "rejected", "kicked"].includes(person.status)
                        ? presence[person.status]
                        : "Ещё не присоединялся"}
                    </span>
                  </div>
                ),
              )}
            </div>
          )}
          {participants.hasNextPage && (
            <Button
              variant="outline"
              busy={participants.isFetchingNextPage}
              onClick={
                /**
                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                 *
                 *
                 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                 */ () => {
                  void participants.fetchNextPage();
                }
              }
            >
              Загрузить ещё участников
            </Button>
          )}
          <p className="field-hint">
            Статус отражает join/leave, а не подключение к видеосвязи.
          </p>
        </section>
      )}
      {editingSchedule && (
        <EditSchedule
          conference={conference}
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setEditingSchedule(false).
             */ () => setEditingSchedule(false)
          }
        />
      )}
      {confirm && (
        <Modal
          title={
            confirm === "finish"
              ? "Завершить конференцию?"
              : "Отменить конференцию?"
          }
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              if (!mutation.isPending) setConfirm(null);
            }
          }
        >
          <p className="modal-description">
            После этого участники не смогут присоединиться. Это действие нельзя
            отменить.
            {confirm === "finish" &&
              " Текущая запись остановится и будет обработана в фоне."}
          </p>
          <ErrorNotice error={mutation.error} />
          <Button
            variant="danger"
            className="full-width"
            busy={mutation.isPending}
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: mutation.mutate(confirm).
               */ () => mutation.mutate(confirm)
            }
          >
            {confirm === "finish" ? "Да, завершить" : "Да, отменить"}
          </Button>
          <Button
            variant="secondary"
            className="full-width"
            disabled={mutation.isPending}
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: setConfirm(null).
               */ () => setConfirm(null)
            }
          >
            Вернуться к встрече
          </Button>
        </Modal>
      )}
    </>
  );
}
