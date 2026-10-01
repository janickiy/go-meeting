import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Info, Link as LinkIcon, Mail, Video } from "lucide-react";
import { api } from "../api";
import type { Conference } from "../types";
import { inviteCode, inviteLink } from "../utils";
import { Button, CopyLink, ErrorNotice, Modal, SuccessMark } from "./ui";
import { ScheduleFields } from "./ScheduleFields";
import { localSchedule, toLocalInput } from "../collaboration";

export function ShareConference({
  conference,
  onClose,
}: {
  conference: Conference;
  onClose: () => void;
}) {
  const link = inviteLink(conference.inviteCode);
  return (
    <Modal title="Конференция создана!" onClose={onClose} wide>
      <div className="share-content">
        <SuccessMark />
        <p className="share-subtitle">
          Поделитесь ссылкой, чтобы пригласить участников.
        </p>
        <CopyLink value={link} />
        <Link
          className="button button-primary full-width"
          to={`/conferences/${conference.id}`}
        >
          <Video size={19} />
          Перейти в конференцию
        </Link>
        <a
          className="button button-secondary full-width"
          href={`mailto:?subject=${encodeURIComponent(`Приглашение: ${conference.title}`)}&body=${encodeURIComponent(`Присоединяйтесь к конференции «${conference.title}» в Meet:\n${link}\nДля входа понадобится аккаунт Meet.`)}`}
          title="Открыть черновик в вашем почтовом приложении"
        >
          <Mail size={19} />
          Пригласить по email
        </a>
        <p className="info-line">
          <Info size={20} />
          По ссылке может присоединиться любой пользователь с аккаунтом Meet.
        </p>
      </div>
    </Modal>
  );
}
export function CreateConference() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [title, setTitle] = useState("");
  const [validation, setValidation] = useState("");
  const [created, setCreated] = useState<Conference | null>(null);
  const [waitingRoom, setWaitingRoom] = useState(false);
  const [planned, setPlanned] = useState(false);
  const [scheduledAt, setScheduledAt] = useState(() =>
    toLocalInput(new Date(Date.now() + 3600000).toISOString()),
  );
  const [duration, setDuration] = useState("");
  const mutation = useMutation({
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
    onSuccess: ({ item }) => {
      void client.invalidateQueries({ queryKey: ["conferences"] });
      setCreated(item);
    },
  });
  function close() {
    if (!mutation.isPending) navigate("/app");
  }
  function submit(event: FormEvent) {
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
    <Modal title="Новая конференция" onClose={close}>
      <form onSubmit={submit} noValidate>
        <label className="field" htmlFor="conference-title">
          Название конференции
          <input
            id="conference-title"
            data-autofocus
            placeholder="Обсуждение проекта"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            disabled={mutation.isPending}
          />
        </label>
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={waitingRoom}
            onChange={(event) => setWaitingRoom(event.target.checked)}
            disabled={mutation.isPending}
          />
          <span>
            <strong>Зал ожидания</strong>
            <small>Организатор приглашает участников войти во встречу.</small>
          </span>
        </label>
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={planned}
            onChange={(event) => setPlanned(event.target.checked)}
            disabled={mutation.isPending}
          />
          <span>
            <strong>Запланировать встречу</strong>
            <small>Выбрать дату и время заранее.</small>
          </span>
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
        <p className="field-hint">
          Запись можно включить после начала встречи. Встречу запускает
          организатор — она не начнётся автоматически.
        </p>
        <ErrorNotice error={mutation.error}>{validation || null}</ErrorNotice>
        <Button type="submit" busy={mutation.isPending} className="full-width">
          Создать конференцию
        </Button>
        <Button
          type="button"
          variant="secondary"
          onClick={close}
          disabled={mutation.isPending}
          className="full-width"
        >
          Отмена
        </Button>
      </form>
    </Modal>
  );
}
export function EditSchedule({
  conference,
  onClose,
}: {
  conference: Conference;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [date, setDate] = useState(() =>
    toLocalInput(conference.scheduledAt || ""),
  );
  const [duration, setDuration] = useState(
    String(conference.plannedDurationMin || ""),
  );
  const [validation, setValidation] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      api.schedule(
        conference.id,
        localSchedule(date)!,
        duration ? Number(duration) : null,
      ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["conference"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      onClose();
    },
  });
  return (
    <Modal
      title="Изменить расписание"
      onClose={() => {
        if (!mutation.isPending) onClose();
      }}
    >
      <form
        onSubmit={(event) => {
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
        }}
      >
        <ScheduleFields
          value={date}
          onChange={setDate}
          duration={duration}
          onDuration={setDuration}
          disabled={mutation.isPending}
        />
        <ErrorNotice error={mutation.error}>{validation || null}</ErrorNotice>
        <Button type="submit" busy={mutation.isPending}>
          Сохранить расписание
        </Button>
      </form>
    </Modal>
  );
}
export function JoinByLink({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate();
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  function submit(event: FormEvent) {
    event.preventDefault();
    const code = inviteCode(value);
    if (!code) {
      setError("Введите ссылку-приглашение этого Meet или код из 32 символов.");
      return;
    }
    navigate(`/i/${code}`);
  }
  return (
    <Modal title="Присоединиться по ссылке" onClose={onClose}>
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
            onChange={(event) => setValue(event.target.value)}
          />
        </label>
        <ErrorNotice>{error || null}</ErrorNotice>
        <Button className="full-width" type="submit">
          <LinkIcon size={18} />
          Открыть приглашение
        </Button>
      </form>
    </Modal>
  );
}
