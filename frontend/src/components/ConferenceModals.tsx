import { useState } from "react";
import type { SubmitEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  CalendarDays,
  DoorOpen,
  Info,
  Mail,
  Video,
} from "lucide-react";
import { api } from "../api";
import type { Conference } from "../types";
import { inviteCode, inviteLink } from "../utils";
import { Button, CopyLink, ErrorNotice, Modal, SuccessMark } from "./ui";
import { ScheduleFields } from "./ScheduleFields";
import { localSchedule, toLocalInput } from "../collaboration";
import { PRODUCT_NAME } from "../brand";
import { ConferenceInviteContent } from "./ConferenceInvitations";
import "./meeting-modals.css";

/**
 * ShareConference показывает результат создания встречи и действия копирования ссылки и перехода в комнату.
 *
 * @args
 *   - объект параметров: conference — свойство текущего компонента; onClose — обработчик закрытия формы или диалога.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function ShareConference({
  conference,
  onClose,
}: {
  conference: Conference;
  onClose: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в конференциях, расписании и истории.
   *
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ () => void;
}) {
  const [inviting, setInviting] = useState(false);
  const [invitationBusy, setInvitationBusy] = useState(false);
  const link = inviteLink(conference.inviteCode);
  if (inviting)
    return (
      <Modal
        key="invite"
        title="Пригласить участников"
        className="meeting-design-modal"
        onClose={() => {
          if (!invitationBusy) onClose();
        }}
      >
        <ConferenceInviteContent
          conference={conference}
          canInvite
          onBusyChange={setInvitationBusy}
        />
        <Button
          variant="secondary"
          disabled={invitationBusy}
          className="full-width"
          onClick={() => setInviting(false)}
        >
          Назад
        </Button>
      </Modal>
    );
  return (
    <Modal
      key="share"
      title="Встреча создана"
      onClose={onClose}
      className="meeting-design-modal meeting-share-modal"
    >
      <div className="share-content">
        <SuccessMark />
        <h3>{conference.title}</h3>
        <p className="share-subtitle">Пригласите коллег и начните разговор.</p>
        <label className="field">Ссылка-приглашение</label>
        <CopyLink value={link} />
        <p className="field-hint">
          По ссылке можно присоединиться без аккаунта.
        </p>
        <Link
          className="button button-primary full-width"
          to={`/conferences/${conference.id}`}
        >
          <Video size={19} />
          Открыть встречу
        </Link>
        <Button
          variant="secondary"
          className="full-width"
          onClick={() => setInviting(true)}
        >
          <Mail size={19} />
          Пригласить участников
        </Button>
      </div>
    </Modal>
  );
}
/**
 * CreateConference управляет формой создания встречи, локальным расписанием и залом ожидания.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function CreateConference() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const client = useQueryClient();
  const [title, setTitle] = useState("");
  const [validation, setValidation] = useState("");
  const [created, setCreated] = useState<Conference | null>(null);
  const [waitingRoom, setWaitingRoom] = useState(false);
  const [planned, setPlanned] = useState(params.get("scheduled") === "1");
  const [scheduledAt, setScheduledAt] = useState(
    /**
     * Обработчик useState выполняет переданный шаг вызова useState в конференциях, расписании и истории.
     *
     *
     * @returns вычисленное значение: toLocalInput(new Date(Date.now() + 3600000).toISOString()).
     */ () => {
      const selected = localSchedule(params.get("at") || "");
      return selected
        ? toLocalInput(selected)
        : toLocalInput(new Date(Date.now() + 3600000).toISOString());
    },
  );
  const [duration, setDuration] = useState("");
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */
    mutationFn: () =>
      api.create({
        title: title.trim(),
        waitingRoomEnabled: waitingRoom,
        ...(planned
          ? {
              scheduledAt: localSchedule(scheduledAt),
              plannedDurationMin: duration ? Number(duration) : null,
            }
          : {}),
      }),
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     * @args
     *   - объект параметров: item — элемент списка, который обрабатывает текущий шаг.
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSuccess: ({ item }) => {
      void client.invalidateQueries({ queryKey: ["conferences"] });
      setCreated(item);
    },
  });
  /**
   * close закрывает форму или соединение с предусмотренной очисткой.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  function close() {
    if (!mutation.isPending)
      navigate(params.get("returnTo") === "calendar" ? "/calendar" : "/app");
  }
  /**
   * submit проверяет поля формы, отправляет изменение и показывает результат либо ошибку.
   *
   * @args
   *   - event (SubmitEvent<HTMLFormElement>) — событие отправки формы.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidation("");
    if (!title.trim() || Array.from(title.trim()).length > 200) {
      setValidation("Название должно содержать от 1 до 200 символов.");
      return;
    }
    if (
      planned &&
      (!localSchedule(scheduledAt) ||
        new Date(localSchedule(scheduledAt)!).getTime() <= Date.now())
    ) {
      setValidation("Выберите корректную дату и время в будущем.");
      return;
    }
    if (
      planned &&
      duration &&
      (!Number.isInteger(Number(duration)) ||
        Number(duration) < 1 ||
        Number(duration) > 1440)
    ) {
      setValidation("Длительность — целое число от 1 до 1440 минут.");
      return;
    }
    mutation.mutate();
  }
  if (created)
    return (
      <ShareConference key={created.id} conference={created} onClose={close} />
    );
  return (
    <Modal
      title="Новая встреча"
      onClose={close}
      className="meeting-design-modal meeting-create-modal"
    >
      <form onSubmit={submit} noValidate>
        <label className="field" htmlFor="conference-title">
          Название встречи
          <input
            id="conference-title"
            data-autofocus
            placeholder="Например, обсуждение проекта"
            value={title}
            maxLength={200}
            onChange={(event) => setTitle(event.target.value)}
            disabled={mutation.isPending}
          />
        </label>
        <div className="meeting-create-options">
          <label className="meeting-option-row">
            <span className="meeting-option-icon">
              <CalendarDays size={21} aria-hidden="true" />
            </span>
            <span className="meeting-option-copy">
              <strong>Запланировать встречу</strong>
              <small>Выберите дату и время заранее</small>
            </span>
            <input
              className="meeting-switch"
              type="checkbox"
              checked={planned}
              onChange={(event) => setPlanned(event.target.checked)}
              disabled={mutation.isPending}
            />
          </label>
          {planned && (
            <ScheduleFields
              value={scheduledAt}
              onChange={setScheduledAt}
              duration={duration}
              onDuration={setDuration}
              disabled={mutation.isPending}
            />
          )}
          <label className="meeting-option-row">
            <span className="meeting-option-icon">
              <DoorOpen size={21} aria-hidden="true" />
            </span>
            <span className="meeting-option-copy">
              <strong>Зал ожидания</strong>
              <small>Для участников без допуска</small>
            </span>
            <input
              className="meeting-switch"
              type="checkbox"
              checked={waitingRoom}
              onChange={(event) => setWaitingRoom(event.target.checked)}
              disabled={mutation.isPending}
            />
          </label>
          <p className="field-hint meeting-waiting-policy">
            Участники с действующей ссылкой-приглашением входят без ожидания.
          </p>
        </div>
        <p className="meeting-inline-note">
          <Info size={18} aria-hidden="true" />
          <span>
            Встречу запускает организатор. Запись можно включить после начала
            встречи.
          </span>
        </p>
        <ErrorNotice error={mutation.error}>{validation || null}</ErrorNotice>
        <footer className="meeting-dialog-footer">
          <Button
            type="button"
            variant="secondary"
            onClick={close}
            disabled={mutation.isPending}
          >
            Отмена
          </Button>
          <Button type="submit" busy={mutation.isPending}>
            Создать встречу <ArrowRight size={18} aria-hidden="true" />
          </Button>
        </footer>
      </form>
    </Modal>
  );
}
/**
 * EditSchedule редактирует однозначное время и длительность ещё запланированной встречи.
 *
 * @args
 *   - объект параметров: conference — свойство текущего компонента; onClose — обработчик закрытия формы или диалога.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function EditSchedule({
  conference,
  onClose,
}: {
  conference: Conference;
  onClose: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в конференциях, расписании и истории.
   *
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ () => void;
}) {
  const client = useQueryClient();
  const [date, setDate] = useState(
    /**
     * Обработчик useState выполняет переданный шаг вызова useState в конференциях, расписании и истории.
     *
     *
     * @returns вычисленное значение: toLocalInput(conference.scheduledAt || "").
     */ () => toLocalInput(conference.scheduledAt || ""),
  );
  const [duration, setDuration] = useState(
    String(conference.plannedDurationMin || ""),
  );
  const [validation, setValidation] = useState("");
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     *
     * @returns вычисленное значение: api.schedule( conference.id, localSchedule(date)!, duration ? Number(duration) : null, ).
     */
    mutationFn: () =>
      api.schedule(
        conference.id,
        localSchedule(date)!,
        duration ? Number(duration) : null,
      ),
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["conference"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      onClose();
    },
  });
  return (
    <Modal
      title="Изменить расписание"
      className="meeting-design-modal"
      onClose={
        /**
         * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ () => {
          if (!mutation.isPending) onClose();
        }
      }
    >
      <form
        onSubmit={
          /**
           * onSubmit обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           * @args
           *   - event — проверенный конверт события комнаты.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ (event) => {
            event.preventDefault();
            setValidation("");
            const utc = localSchedule(date);
            if (!utc || new Date(utc).getTime() <= Date.now()) {
              setValidation("Укажите время в будущем.");
              return;
            }
            if (
              duration &&
              (!Number.isInteger(Number(duration)) ||
                Number(duration) < 1 ||
                Number(duration) > 1440)
            ) {
              setValidation("Длительность — от 1 до 1440 минут.");
              return;
            }
            mutation.mutate();
          }
        }
      >
        <ScheduleFields
          value={date}
          onChange={setDate}
          duration={duration}
          onDuration={setDuration}
          disabled={mutation.isPending}
        />
        <ErrorNotice error={mutation.error}>{validation || null}</ErrorNotice>
        <footer className="meeting-dialog-footer">
          <Button
            type="button"
            variant="secondary"
            onClick={onClose}
            disabled={mutation.isPending}
          >
            Отмена
          </Button>
          <Button type="submit" busy={mutation.isPending}>
            Сохранить расписание
          </Button>
        </footer>
      </form>
    </Modal>
  );
}
/**
 * JoinByLink проверяет введённое приглашение и выполняет авторизованное присоединение.
 *
 * @args
 *   - объект параметров: onClose — обработчик закрытия формы или диалога.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function JoinByLink({
  onClose,
}: {
  onClose: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в конференциях, расписании и истории.
   *
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ () => void;
}) {
  const navigate = useNavigate();
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  /**
   * submit проверяет поля формы, отправляет изменение и показывает результат либо ошибку.
   *
   * @args
   *   - event (SubmitEvent<HTMLFormElement>) — событие отправки формы.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    const code = inviteCode(value);
    if (!code) {
      setError(
        `Введите ссылку-приглашение этого ${PRODUCT_NAME} или код из 32 символов.`,
      );
      return;
    }
    navigate(`/i/${code}`);
  }
  return (
    <Modal
      title="Присоединиться по ссылке"
      onClose={onClose}
      className="meeting-design-modal"
    >
      <p className="modal-description">
        Попросите организатора поделиться приглашением.
      </p>
      <form onSubmit={submit}>
        <label className="field" htmlFor="invite-link">
          Ссылка или код приглашения
          <input
            id="invite-link"
            data-autofocus
            placeholder={`${window.location.origin}/i/…`}
            value={value}
            onChange={
              /**
               * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               * @args
               *   - event — проверенный конверт события комнаты.
               *
               * @returns вычисленное значение: setValue(event.target.value).
               */ (event) => setValue(event.target.value)
            }
          />
        </label>
        <ErrorNotice>{error || null}</ErrorNotice>
        <footer className="meeting-dialog-footer">
          <Button type="button" variant="secondary" onClick={onClose}>
            Отмена
          </Button>
          <Button type="submit">
            Открыть приглашение <ArrowRight size={18} />
          </Button>
        </footer>
      </form>
    </Modal>
  );
}
