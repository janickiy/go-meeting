import { useEffect, useId, useRef, useState } from "react";
import type { SubmitEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Mail, Search, X } from "lucide-react";
import { api } from "../api";
import type { Conference, InvitationUser } from "../types";
import { inviteLink } from "../utils";
import { Button, CopyLink, ErrorNotice } from "./ui";
import "./conference-invitations.css";

const maxRecipients = 20;

/** Общая форма приглашения используется при создании встречи и внутри комнаты. */
export function ConferenceInviteContent({
  conference,
  canInvite,
  onBusyChange,
}: {
  conference: Conference;
  canInvite: boolean;
  onBusyChange?: (busy: boolean) => void;
}) {
  return (
    <div className="conference-invitations">
      <p className="modal-description">{conference.title}</p>
      <CopyLink value={inviteLink(conference.inviteCode)} />
      <p className="field-hint">По приглашению можно войти без аккаунта.</p>
      {canInvite && (
        <>
          <div className="invitation-form-divider">
            <span>или отправьте приглашение</span>
          </div>
          <ConferenceInvitationForm
            key={conference.id}
            conferenceId={conference.id}
            onBusyChange={onBusyChange}
          />
        </>
      )}
    </div>
  );
}

/** Сохраняет введённых получателей, пока сервер не подтвердит постановку приглашений в очередь. */
export function ConferenceInvitationForm({
  conferenceId,
  onBusyChange,
}: {
  conferenceId: string;
  onBusyChange?: (busy: boolean) => void;
}) {
  const id = useId();
  const [emailsText, setEmailsText] = useState("");
  const [queryText, setQueryText] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<InvitationUser[]>([]);
  const [validation, setValidation] = useState("");
  const submitting = useRef(false);
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(queryText.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [queryText]);
  const query = useQuery({
    queryKey: ["invitation-users", conferenceId, search],
    queryFn: ({ signal }) => api.invitationUsers(conferenceId, search, signal),
    enabled: Array.from(search).length >= 2,
    retry: false,
    staleTime: 30_000,
  });
  const invitation = useMutation({
    mutationFn: (recipients: { emails: string[]; userIds: string[] }) =>
      api.inviteParticipants(conferenceId, recipients),
    onSuccess: () => {
      setEmailsText("");
      setSelected([]);
      setQueryText("");
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  useEffect(() => {
    onBusyChange?.(invitation.isPending);
    return () => onBusyChange?.(false);
  }, [invitation.isPending, onBusyChange]);

  function edit() {
    setValidation("");
    if (!invitation.isPending) invitation.reset();
  }

  function toggle(user: InvitationUser) {
    edit();
    if (selected.some((item) => item.id === user.id)) {
      setSelected(selected.filter((item) => item.id !== user.id));
      return;
    }
    if (selected.length >= maxRecipients) {
      setValidation(
        `За один раз можно пригласить не больше ${maxRecipients} человек.`,
      );
      return;
    }
    setSelected([...selected, user]);
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current) return;
    setValidation("");
    invitation.reset();
    const entered = [
      ...new Set(
        emailsText
          .split(/[\s,;]+/)
          .filter(Boolean)
          .map((email) => email.toLowerCase()),
      ),
    ];
    if (
      entered.some(
        (email) =>
          email.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email),
      )
    ) {
      setValidation("Проверьте адреса email. Например: colleague@example.com.");
      return;
    }
    const selectedEmails = new Set(
      selected.map((user) => user.email.toLowerCase()),
    );
    const emails = entered.filter((email) => !selectedEmails.has(email));
    const total = emails.length + selected.length;
    if (!total) {
      setValidation("Укажите email или выберите пользователя системы.");
      return;
    }
    if (total > maxRecipients) {
      setValidation(
        `За один раз можно пригласить не больше ${maxRecipients} человек.`,
      );
      return;
    }
    submitting.current = true;
    invitation.mutate({ emails, userIds: selected.map((user) => user.id) });
  }

  const currentSearch = queryText.trim() === search;
  const hasSearch = Array.from(queryText.trim()).length >= 2;
  return (
    <form className="conference-invitation-form" onSubmit={submit} noValidate>
      <label className="field" htmlFor={`${id}-emails`}>
        Email участников
        <textarea
          id={`${id}-emails`}
          data-autofocus
          placeholder="colleague@example.com"
          autoComplete="off"
          rows={2}
          maxLength={6000}
          value={emailsText}
          disabled={invitation.isPending}
          aria-describedby={`${id}-email-hint`}
          onChange={(event) => {
            edit();
            setEmailsText(event.target.value);
          }}
        />
      </label>
      <p className="field-hint" id={`${id}-email-hint`}>
        Несколько адресов разделяйте запятой или новой строкой. До{" "}
        {maxRecipients} получателей за один раз.
      </p>
      <label className="field" htmlFor={`${id}-search`}>
        Добавить пользователей системы
        <span className="invitation-search-field">
          <Search size={17} aria-hidden="true" />
          <input
            id={`${id}-search`}
            type="search"
            placeholder="Имя или email"
            autoComplete="off"
            maxLength={100}
            value={queryText}
            disabled={invitation.isPending}
            aria-describedby={`${id}-search-hint`}
            onChange={(event) => setQueryText(event.target.value)}
          />
        </span>
      </label>
      <p className="field-hint" id={`${id}-search-hint`}>
        Введите минимум 2 символа имени или email.
      </p>
      {selected.length > 0 && (
        <ul className="invitation-selected" aria-label="Выбранные пользователи">
          {selected.map((user) => (
            <li key={user.id}>
              <span>{user.displayName || user.email}</span>
              <button
                type="button"
                aria-label={`Убрать ${user.email}`}
                disabled={invitation.isPending}
                onClick={() => toggle(user)}
              >
                <X size={15} aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {hasSearch && (!currentSearch || query.isFetching) && (
        <p className="field-hint" role="status">
          Ищем пользователей…
        </p>
      )}
      {hasSearch && currentSearch && query.isError && (
        <div className="invitation-search-error">
          <ErrorNotice error={query.error} />
          <Button
            type="button"
            variant="outline"
            onClick={() => void query.refetch()}
          >
            Повторить поиск
          </Button>
        </div>
      )}
      {hasSearch &&
        currentSearch &&
        query.isSuccess &&
        !query.isFetching &&
        (query.data.items.length ? (
          <ul
            className="invitation-user-results"
            aria-label="Пользователи системы"
          >
            {query.data.items.map((user) => (
              <li key={user.id}>
                <label>
                  <input
                    type="checkbox"
                    aria-label={
                      user.displayName
                        ? `${user.displayName}, ${user.email}`
                        : user.email
                    }
                    checked={selected.some((item) => item.id === user.id)}
                    disabled={invitation.isPending}
                    onChange={() => toggle(user)}
                  />
                  <span>
                    <strong>{user.displayName || user.email}</strong>
                    {user.displayName && <small>{user.email}</small>}
                  </span>
                </label>
              </li>
            ))}
          </ul>
        ) : (
          <p className="field-hint" role="status">
            Пользователи не найдены. Можно указать email выше.
          </p>
        ))}
      <p className="field-hint">
        Получатели получат письмо со ссылкой на встречу. У зарегистрированных
        пользователей приглашение появится и в личном кабинете.
      </p>
      <ErrorNotice error={invitation.error}>{validation || null}</ErrorNotice>
      {invitation.data && (
        <div className="invitation-result" role="status" aria-live="polite">
          <ul>
            {invitation.data.items.map((item) => (
              <li key={item.id}>
                <strong>{item.email}</strong>
                <span>
                  {item.status === "queued"
                    ? "Приглашение поставлено в очередь"
                    : item.status === "left_chat"
                      ? "Пользователь покинул чат. Он может вернуться по ссылке на встречу."
                      : "Приглашение уже отправлялось"}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
      <Button type="submit" busy={invitation.isPending} className="full-width">
        <Mail size={18} aria-hidden="true" />
        Пригласить участников
      </Button>
    </form>
  );
}
