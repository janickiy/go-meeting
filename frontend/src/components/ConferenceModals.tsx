import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CircleHelp, Info, Link as LinkIcon, Mail, Video } from "lucide-react";
import { api } from "../api";
import type { Conference } from "../types";
import { inviteCode, inviteLink } from "../utils";
import { Button, CopyLink, ErrorNotice, Modal, SuccessMark } from "./ui";

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
  const mutation = useMutation({
    mutationFn: () => api.create(title.trim()),
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
        <label className="field unavailable-field" htmlFor="description">
          Описание <span className="muted">(пока недоступно)</span>
          <textarea
            id="description"
            placeholder="О чём будете встречаться?"
            disabled
            rows={3}
          />
        </label>
        <div className="unavailable-feature">
          <span className="disabled-switch" aria-hidden="true" />
          <div>
            <strong>Запись встречи</strong>
            <p>Автоматическая запись будет добавлена отдельно.</p>
          </div>
          <CircleHelp size={17} />
        </div>
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
