import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
  type SubmitEvent,
} from "react";
import { Link, useNavigate } from "react-router";
import {
  onlineManager,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  ArrowLeft,
  Bell,
  Download,
  LogOut,
  MessageCircle,
  Paperclip,
  Pencil,
  Search,
  Star,
  UserPlus,
  Users,
  Video,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { formatBytes } from "../collaboration";
import { safeRecordingUrl } from "../recordingPresentation";
import type {
  ChatAttachment,
  ChatMessage,
  Conference,
  ConferenceChatInfo,
  ConferenceChatMember,
  Participant,
} from "../types";
import { formatDate, initials } from "../utils";
import { ConferenceInviteContent } from "./ConferenceInvitations";
import { Button, CopyLink, ErrorNotice, Loading, Modal } from "./ui";
import "./conference-chat-info.css";

export type ConferenceChatView =
  | "info"
  | "invite"
  | "leave"
  | "search"
  | "participants"
  | "materials"
  | "important"
  | "edit";
const titles: Record<ConferenceChatView, string> = {
  info: "Информация о чате",
  invite: "Добавить участников",
  leave: "Покинуть чат",
  search: "Найти в чате",
  participants: "Участники",
  materials: "Картинки, файлы и ссылки",
  important: "Важные сообщения",
  edit: "Название и описание",
};

/** All panels stay inside the meeting context and retain server access checks. */
export function ConferenceChatInfoModal({
  conference,
  initialView = "info",
  onClose,
  returnFocus,
  onLeft,
}: {
  conference: Conference;
  initialView?: ConferenceChatView;
  onClose: () => void;
  returnFocus?: () => HTMLElement | null;
  onLeft?: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const [view, setView] = useState(initialView);
  const [invitationBusy, setInvitationBusy] = useState(false);
  const [editingBusy, setEditingBusy] = useState(false);
  const [profileBusy, setProfileBusy] = useState(false);
  const [selectedMember, setSelectedMember] =
    useState<ConferenceChatMember | null>(null);
  const key = ["conference-chat-info", conference.id, user?.id];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => api.conferenceChatInfo(conference.id, signal),
    retry: false,
  });
  const info = query.data?.item;
  const canEdit = info?.canEdit === true && user?.id === conference.ownerId;
  const notifications = useMutation({
    mutationFn: (enabled: boolean) =>
      api.setConferenceChatNotifications(conference.id, enabled),
    onSuccess: ({ item }) => {
      client.setQueryData<{ item: ConferenceChatInfo }>(key, (current) =>
        current
          ? {
              ...current,
              item: {
                ...current.item,
                notificationsEnabled: item.notificationsEnabled,
              },
            }
          : current,
      );
      void client.invalidateQueries({
        queryKey: ["conference-chat-preferences", conference.id],
      });
    },
  });
  const leave = useMutation({
    mutationFn: () => api.leaveConferenceChat(conference.id),
    onSuccess: async () => {
      client.removeQueries({ queryKey: ["chat", conference.id] });
      client.removeQueries({ queryKey: ["chat-read", conference.id] });
      await client.invalidateQueries({ queryKey: ["conferences"] });
      for (const prefix of [
        "conference-chat-info",
        "conference-chat-pins",
        "conference-chat-search",
        "conference-chat-materials",
        "conference-chat-members",
        "conference-chat-preferences",
      ])
        client.removeQueries({ queryKey: [prefix, conference.id, user?.id] });
      onLeft?.();
      onClose();
    },
  });
  const busy =
    invitationBusy ||
    editingBusy ||
    profileBusy ||
    leave.isPending ||
    notifications.isPending;
  const link = safeRecordingUrl(info?.inviteUrl);

  return (
    <Modal
      title={selectedMember ? "О пользователе" : titles[view]}
      className="conference-chat-info-modal"
      returnFocus={returnFocus}
      onClose={() => {
        if (!busy) {
          if (selectedMember) setSelectedMember(null);
          else onClose();
        }
      }}
    >
      {view !== "info" && (
        <Button
          variant="outline"
          className="conference-chat-info-back"
          disabled={busy}
          onClick={() => {
            if (selectedMember) setSelectedMember(null);
            else setView("info");
          }}
        >
          <ArrowLeft size={16} aria-hidden="true" />
          {selectedMember ? "Участники" : "Информация о чате"}
        </Button>
      )}
      <ErrorNotice error={query.error || notifications.error} />
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку
        </Button>
      )}
      {!info && query.isPending && <Loading />}
      {info && view === "info" && (
        <>
          <div className="conference-chat-info-identity">
            <span className="conference-chat-info-avatar" aria-hidden="true">
              <Video size={31} />
            </span>
            <div className="conference-chat-info-copy">
              <h3>{info.title}</h3>
              <p>
                {info.description ||
                  "Чат участников встречи. Переписка доступна в конференции."}
              </p>
              <p>Создана {formatDate(conference.createdAt)}</p>
              <Link className="text-link" to={`/meetings/${conference.id}`}>
                Ссылка на видеовстречу
              </Link>
              <p>{info.participantCount} участн.</p>
            </div>
          </div>
          <div className="conference-chat-info-actions">
            <button
              type="button"
              className="conference-chat-info-action"
              role="switch"
              aria-label="Уведомления"
              aria-checked={info.notificationsEnabled}
              disabled={notifications.isPending}
              onClick={() => notifications.mutate(!info.notificationsEnabled)}
            >
              <Bell size={23} aria-hidden="true" />
              Уведомления
              <span className="conference-chat-info-switch" aria-hidden="true">
                <span />
              </span>
            </button>
            <button
              type="button"
              className="conference-chat-info-action"
              onClick={() => setView("search")}
            >
              <Search size={23} aria-hidden="true" />
              Найти в чате
            </button>
            <button
              type="button"
              className="conference-chat-info-action"
              onClick={() => setView("participants")}
            >
              <Users size={23} aria-hidden="true" />
              Участники
              <span className="conference-chat-info-count">
                {info.participantCount}
              </span>
            </button>
            <button
              type="button"
              className="conference-chat-info-action"
              onClick={() => setView("materials")}
            >
              <Paperclip size={23} aria-hidden="true" />
              Картинки, файлы и ссылки
            </button>
            <button
              type="button"
              className="conference-chat-info-action"
              onClick={() => setView("important")}
            >
              <Star size={23} aria-hidden="true" />
              Важные сообщения
            </button>
            {canEdit && (
              <button
                type="button"
                className="conference-chat-info-action"
                onClick={() => setView("edit")}
              >
                <Pencil size={23} aria-hidden="true" />
                Название и описание
              </button>
            )}
            <button
              type="button"
              className="conference-chat-info-action"
              onClick={() => setView("invite")}
            >
              <UserPlus size={23} aria-hidden="true" />
              Добавить участников
            </button>
            <button
              type="button"
              className="conference-chat-info-action conference-chat-danger"
              onClick={() => setView("leave")}
            >
              <LogOut size={23} aria-hidden="true" />
              Покинуть чат
            </button>
          </div>
        </>
      )}
      {info && view === "invite" && (
        <>
          {info.canInvite ? (
            <ConferenceInviteContent
              conference={{ ...conference, title: info.title }}
              canInvite
              onBusyChange={setInvitationBusy}
            />
          ) : (
            <>
              <p className="modal-description">
                Участников добавляет организатор или соорганизатор встречи.
              </p>
              {link && <CopyLink value={link} />}
            </>
          )}
        </>
      )}
      {info && view === "leave" && (
        <div className="conference-chat-info-panel">
          <p className="modal-description">
            Покинуть чат «{info.title}»? Встреча исчезнет из вашего списка,
            уведомления и доступ к её чату отключатся. По ссылке можно
            присоединиться снова.
          </p>
          <ErrorNotice error={leave.error} />
          <Button
            variant="danger"
            busy={leave.isPending}
            onClick={() => leave.mutate()}
          >
            <LogOut size={18} aria-hidden="true" />
            Покинуть чат
          </Button>
          <Button
            variant="outline"
            disabled={leave.isPending}
            onClick={() => setView("info")}
          >
            Отмена
          </Button>
        </div>
      )}
      {info &&
        view === "edit" &&
        (canEdit ? (
          <EditConferenceChat
            conferenceId={conference.id}
            info={info}
            onBusyChange={setEditingBusy}
            onSaved={() => {
              void client.invalidateQueries({ queryKey: key });
              void client.invalidateQueries({ queryKey: ["conferences"] });
              void client.invalidateQueries({ queryKey: ["conference"] });
              setView("info");
            }}
          />
        ) : (
          <p className="conference-chat-info-empty">
            Изменять название и описание может только администратор чата.
          </p>
        ))}
      {info && view === "participants" && (
        <ConferenceChatMembers
          conferenceId={conference.id}
          ownerId={conference.ownerId}
          selectedMember={selectedMember}
          onSelect={setSelectedMember}
          onBusyChange={setProfileBusy}
          onNavigate={onClose}
        />
      )}
      {info && view === "search" && (
        <ConferenceChatSearch conference={conference} onNavigate={onClose} />
      )}
      {info && view === "materials" && (
        <ConferenceChatMaterials conference={conference} onNavigate={onClose} />
      )}
      {info && view === "important" && (
        <ConferenceChatImportant conference={conference} onNavigate={onClose} />
      )}
    </Modal>
  );
}

function EditConferenceChat({
  conferenceId,
  info,
  onSaved,
  onBusyChange,
}: {
  conferenceId: string;
  info: ConferenceChatInfo;
  onSaved: () => void;
  onBusyChange: (busy: boolean) => void;
}) {
  const id = useId();
  const [title, setTitle] = useState(info.title);
  const [description, setDescription] = useState(info.description);
  const [validation, setValidation] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      api.updateConferenceChatInfo(conferenceId, {
        title: title.trim(),
        description: description.trim(),
      }),
    onSuccess: onSaved,
  });
  useEffect(() => {
    onBusyChange(mutation.isPending);
    return () => onBusyChange(false);
  }, [mutation.isPending, onBusyChange]);
  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidation("");
    if (
      !title.trim() ||
      Array.from(title.trim()).length > 200 ||
      Array.from(description.trim()).length > 1000
    ) {
      setValidation(
        "Название — от 1 до 200 символов, описание — до 1000 символов.",
      );
      return;
    }
    mutation.mutate();
  }
  return (
    <form onSubmit={submit} className="conference-chat-info-rename" noValidate>
      <label className="field" htmlFor={`${id}-title`}>
        Название
        <input
          id={`${id}-title`}
          data-autofocus
          value={title}
          maxLength={400}
          disabled={mutation.isPending}
          onChange={(event) => setTitle(event.target.value)}
        />
      </label>
      <label className="field" htmlFor={`${id}-description`}>
        Описание
        <textarea
          id={`${id}-description`}
          value={description}
          rows={4}
          maxLength={1000}
          disabled={mutation.isPending}
          onChange={(event) => setDescription(event.target.value)}
        />
      </label>
      <ErrorNotice error={mutation.error}>
        {validation || undefined}
      </ErrorNotice>
      <Button type="submit" busy={mutation.isPending}>
        Сохранить
      </Button>
    </form>
  );
}

const roles: Record<Participant["role"], string> = {
  owner: "Организатор",
  co_host: "Соорганизатор",
  participant: "Участник",
  guest: "Гость",
};
const statuses: Record<Participant["status"], string> = {
  joined: "Во встрече",
  left: "Вышел из встречи",
  waiting: "Ожидает допуска",
  rejected: "Запрос отклонён",
  kicked: "Исключён",
};

function ConferenceChatMembers({
  conferenceId,
  ownerId,
  selectedMember,
  onSelect,
  onBusyChange,
  onNavigate,
}: {
  conferenceId: string;
  ownerId: string;
  selectedMember: ConferenceChatMember | null;
  onSelect: (member: ConferenceChatMember) => void;
  onBusyChange: (busy: boolean) => void;
  onNavigate: () => void;
}) {
  const { user } = useAuth();
  const [search, setSearch] = useState("");
  const client = useQueryClient();
  const connected = useSyncExternalStore(onlineManager.subscribe, () =>
    onlineManager.isOnline(),
  );
  const presenceId = useId();
  const [visible, setVisible] = useState(
    () => document.visibilityState !== "hidden",
  );
  const memberList = useRef<HTMLDivElement>(null);
  const memberTrigger = useRef<HTMLButtonElement | null>(null);
  const profileWasOpen = useRef(false);
  useEffect(() => {
    const visibility = () => {
      const next = document.visibilityState !== "hidden";
      setVisible(next);
      if (!next)
        void client.cancelQueries({
          queryKey: ["conference-chat-members", conferenceId, user?.id],
          exact: true,
        });
    };
    document.addEventListener("visibilitychange", visibility);
    return () => document.removeEventListener("visibilitychange", visibility);
  }, [client, conferenceId, user?.id]);
  useEffect(() => {
    if (selectedMember) {
      profileWasOpen.current = true;
    } else if (profileWasOpen.current) {
      profileWasOpen.current = false;
      const trigger = memberTrigger.current;
      if (trigger?.isConnected) trigger.focus();
      else
        memberList.current?.querySelector<HTMLInputElement>("input")?.focus();
    }
  }, [selectedMember]);
  const query = useInfiniteQuery({
    queryKey: ["conference-chat-members", conferenceId, user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.conferenceChatMembers(conferenceId, pageParam, signal),
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
    staleTime: 5000,
    enabled: visible,
    refetchInterval: visible ? 5000 : false,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  });
  const allMembers = Array.from(
    new Map(
      (query.data?.pages.flatMap((page) => page.items) || []).map((member) => [
        member.id,
        member,
      ]),
    ).values(),
  );
  const presenceKnown = connected && !query.isPaused && !query.isError;
  const members = allMembers.filter((item) =>
    item.displayName
      .toLocaleLowerCase("ru")
      .includes(search.toLocaleLowerCase("ru")),
  );
  const currentProfile = selectedMember
    ? allMembers.find((member) => member.id === selectedMember.id)
    : undefined;
  const groups = [
    {
      title: "Администраторы",
      items: members.filter((item) => item.userId === ownerId),
    },
    {
      title: "Участники",
      items: members.filter((item) => item.userId !== ownerId),
    },
  ];
  return (
    <>
      {selectedMember && (
        <ConferenceChatUserProfile
          member={currentProfile || selectedMember}
          presenceKnown={!!currentProfile && presenceKnown}
          presenceError={query.error}
          onRefresh={() => void query.refetch()}
          onBusyChange={onBusyChange}
          onNavigate={onNavigate}
        />
      )}
      <div hidden={!!selectedMember} ref={memberList}>
        <div className="conference-chat-info-panel">
          <label className="field">
            Найти участника
            <input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Имя участника"
            />
          </label>
          <ErrorNotice error={query.error} />
          {query.isPending ? (
            <Loading />
          ) : (
            groups.map((group) => (
              <section
                key={group.title}
                className="conference-chat-member-group"
                aria-label={group.title}
              >
                <h3>{group.title}</h3>
                <ul
                  className="conference-chat-info-list"
                  aria-label={`${group.title} чата`}
                >
                  {group.items.map((item) => (
                    <li key={item.id} className="conference-chat-info-member">
                      <button
                        type="button"
                        className="conference-chat-member-button"
                        aria-label={`О пользователе: ${item.displayName}`}
                        aria-describedby={`${presenceId}-${item.id}`}
                        onClick={(event) => {
                          memberTrigger.current = event.currentTarget;
                          onSelect(item);
                        }}
                      >
                        <span
                          className="conference-chat-member-avatar"
                          aria-hidden="true"
                        >
                          <span className="avatar avatar-small">
                            {initials(item.displayName)}
                          </span>
                          {presenceKnown && item.online === true && (
                            <span className="conference-chat-member-online-dot" />
                          )}
                        </span>
                        <span className="conference-chat-member-copy">
                          <strong>
                            {item.displayName}
                            {item.userId === user?.id ? " (вы)" : ""}
                          </strong>
                          <small>
                            {item.userId === ownerId
                              ? "Администратор"
                              : roles[item.role]}{" "}
                            · {statuses[item.status]}
                          </small>
                          <small
                            id={`${presenceId}-${item.id}`}
                            className={
                              presenceKnown && item.online === true
                                ? "conference-chat-member-online"
                                : undefined
                            }
                          >
                            {memberPresenceLabel(item, presenceKnown)}
                          </small>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            ))
          )}
          {!query.isPending && !query.isError && !members.length && (
            <p className="conference-chat-info-empty">Участники не найдены.</p>
          )}
          {query.isError && (
            <Button variant="outline" onClick={() => void query.refetch()}>
              Повторить загрузку
            </Button>
          )}
          {query.hasNextPage && (
            <Button
              variant="outline"
              busy={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              Ещё участники
            </Button>
          )}
        </div>
      </div>
    </>
  );
}

function memberPresenceLabel(member: ConferenceChatMember, known: boolean) {
  if (!known || typeof member.online !== "boolean") return "Статус недоступен";
  return member.online ? "В сети" : "Не в сети";
}

function ConferenceChatUserProfile({
  member,
  presenceKnown,
  presenceError,
  onRefresh,
  onBusyChange,
  onNavigate,
}: {
  member: ConferenceChatMember;
  presenceKnown: boolean;
  presenceError: unknown;
  onRefresh: () => void;
  onBusyChange: (busy: boolean) => void;
  onNavigate: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const navigate = useNavigate();
  const root = useRef<HTMLDivElement>(null);
  const mounted = useRef(false);
  const directPending = useRef(false);
  const canMessage =
    !!user &&
    !user.guestConferenceId &&
    !!member.userId &&
    member.userId !== user.id &&
    member.isGuest === false;
  const direct = useMutation({
    mutationFn: () => api.createPersonalConversation(member.userId!),
    onSuccess: ({ item }) => {
      if (!mounted.current) return;
      client.setQueryData(["personal-detail", item.id, user?.id], {
        status: "success",
        item,
      });
      void client.invalidateQueries({ queryKey: ["personal-list", user?.id] });
      onNavigate();
      navigate(`/personal/${encodeURIComponent(item.id)}`);
    },
    onSettled: () => {
      directPending.current = false;
    },
  });
  useEffect(() => {
    mounted.current = true;
    root.current?.focus();
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    onBusyChange(direct.isPending);
    return () => onBusyChange(false);
  }, [direct.isPending, onBusyChange]);
  const online = presenceKnown && member.online === true;
  return (
    <div
      ref={root}
      tabIndex={-1}
      role="region"
      aria-label={`Профиль: ${member.displayName}`}
      className="conference-chat-user-profile"
    >
      <span
        className="conference-chat-member-avatar conference-chat-user-avatar"
        aria-hidden="true"
      >
        <span className="avatar">{initials(member.displayName)}</span>
        {online && <span className="conference-chat-member-online-dot" />}
      </span>
      <h3>{member.displayName}</h3>
      <p className={online ? "conference-chat-member-online" : "muted"}>
        {memberPresenceLabel(member, presenceKnown)}
      </p>
      <ErrorNotice error={direct.error} />
      <ErrorNotice error={presenceError} />
      {!!presenceError && (
        <Button variant="outline" onClick={onRefresh}>
          Повторить загрузку
        </Button>
      )}
      {canMessage && (
        <Button
          className="conference-chat-user-message"
          variant="outline"
          busy={direct.isPending}
          onClick={() => {
            if (directPending.current) return;
            directPending.current = true;
            direct.mutate();
          }}
        >
          <MessageCircle size={25} aria-hidden="true" />
          Сообщение
        </Button>
      )}
    </div>
  );
}

function ConferenceChatSearch({
  conference,
  onNavigate,
}: {
  conference: Conference;
  onNavigate: () => void;
}) {
  const { user } = useAuth();
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(text.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [text]);
  const query = useInfiniteQuery({
    queryKey: ["conference-chat-search", conference.id, user?.id, search],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.searchConferenceChat(conference.id, search, pageParam, signal),
    enabled: Array.from(search).length >= 2,
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
  });
  const current = text.trim() === search;
  const messages = query.data?.pages.flatMap((page) => page.items) || [];
  return (
    <div className="conference-chat-info-panel">
      <label className="field">
        Поиск сообщений
        <input
          type="search"
          data-autofocus
          value={text}
          maxLength={200}
          placeholder="Введите текст сообщения"
          onChange={(event) => setText(event.target.value)}
        />
      </label>
      {Array.from(text.trim()).length < 2 ? (
        <p className="field-hint">Введите минимум 2 символа.</p>
      ) : !current || query.isPending ? (
        <Loading />
      ) : (
        <>
          <ErrorNotice error={query.error} />
          <ConferenceChatMessageList
            conference={conference}
            messages={messages}
            onNavigate={onNavigate}
          />
          {!messages.length && !query.isError && (
            <p className="conference-chat-info-empty">Сообщения не найдены.</p>
          )}
          {query.isError && (
            <Button variant="outline" onClick={() => void query.refetch()}>
              Повторить поиск
            </Button>
          )}
          {query.hasNextPage && (
            <Button
              variant="outline"
              busy={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              Ещё сообщения
            </Button>
          )}
        </>
      )}
    </div>
  );
}

function ConferenceChatImportant({
  conference,
  onNavigate,
}: {
  conference: Conference;
  onNavigate: () => void;
}) {
  const { user } = useAuth();
  const query = useInfiniteQuery({
    queryKey: ["conference-chat-pins", conference.id, user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.conferenceChatPins(conference.id, pageParam, signal),
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
  });
  const messages = query.data?.pages.flatMap((page) => page.items) || [];
  return (
    <div className="conference-chat-info-panel">
      <p className="field-hint">
        Сообщения, которые вы отметили важными. Эти отметки видны только вам.
      </p>
      <ErrorNotice error={query.error} />
      {query.isPending ? (
        <Loading />
      ) : (
        <ConferenceChatMessageList
          conference={conference}
          messages={messages}
          important
          onNavigate={onNavigate}
        />
      )}
      {!query.isPending && !query.isError && !messages.length && (
        <p className="conference-chat-info-empty">
          Важных сообщений пока нет. Отметьте сообщение звёздочкой в чате
          встречи.
        </p>
      )}
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку
        </Button>
      )}
      {query.hasNextPage && (
        <Button
          variant="outline"
          busy={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          Ещё важные сообщения
        </Button>
      )}
    </div>
  );
}

function ConferenceChatMaterials({
  conference,
  onNavigate,
}: {
  conference: Conference;
  onNavigate: () => void;
}) {
  const { user } = useAuth();
  const [kind, setKind] = useState<"image" | "file" | "link">("image");
  const query = useInfiniteQuery({
    queryKey: ["conference-chat-materials", conference.id, user?.id, kind],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.conferenceChatMaterials(conference.id, kind, pageParam, signal),
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
  });
  const items = query.data?.pages.flatMap((page) => page.items) || [];
  return (
    <div className="conference-chat-info-panel">
      <div className="tabs" aria-label="Вид материалов">
        {(
          [
            ["image", "Картинки"],
            ["file", "Файлы"],
            ["link", "Ссылки"],
          ] as const
        ).map(([value, label]) => (
          <button
            type="button"
            key={value}
            className={`tab ${kind === value ? "tab-active" : ""}`}
            aria-pressed={kind === value}
            onClick={() => setKind(value)}
          >
            {label}
          </button>
        ))}
      </div>
      <ErrorNotice error={query.error} />
      {query.isPending ? (
        <Loading />
      ) : (
        <ul className="conference-chat-info-list">
          {items.map((item, index) => (
            <li
              key={`${item.message.id}-${item.attachment?.id || item.url || index}`}
            >
              <ConferenceChatMessageCard
                conference={conference}
                message={item.message}
                attachment={item.attachment}
                url={item.url}
                onNavigate={onNavigate}
              />
            </li>
          ))}
        </ul>
      )}
      {!query.isPending && !query.isError && !items.length && (
        <p className="conference-chat-info-empty">
          В этом чате таких материалов пока нет.
        </p>
      )}
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку
        </Button>
      )}
      {query.hasNextPage && (
        <Button
          variant="outline"
          busy={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          Ещё материалы
        </Button>
      )}
    </div>
  );
}

function ConferenceChatMessageList({
  conference,
  messages,
  important = false,
  onNavigate,
}: {
  conference: Conference;
  messages: ChatMessage[];
  important?: boolean;
  onNavigate: () => void;
}) {
  return (
    <ul className="conference-chat-info-list">
      {messages.map((message) => (
        <li key={message.id}>
          <ConferenceChatMessageCard
            conference={conference}
            message={message}
            important={important || message.important}
            onNavigate={onNavigate}
          />
        </li>
      ))}
    </ul>
  );
}

function ConferenceChatMessageCard({
  conference,
  message,
  attachment,
  url,
  important,
  onNavigate,
}: {
  conference: Conference;
  message: ChatMessage;
  attachment?: ChatAttachment;
  url?: string;
  important?: boolean;
  onNavigate: () => void;
}) {
  const client = useQueryClient();
  const { user } = useAuth();
  const marked = important ?? !!message.important;
  const pin = useMutation({
    mutationFn: () =>
      api.setConferenceChatImportant(conference.id, message.id, !marked),
    onSuccess: () => {
      void client.invalidateQueries({
        queryKey: ["conference-chat-pins", conference.id, user?.id],
      });
      void client.invalidateQueries({
        queryKey: ["conference-chat-search", conference.id, user?.id],
      });
      void client.invalidateQueries({
        queryKey: ["conference-chat-materials", conference.id, user?.id],
      });
      void client.invalidateQueries({
        queryKey: ["chat", conference.id, user?.id],
      });
    },
  });
  const safeURL = safeRecordingUrl(url);
  const path = `/meetings/${conference.id}?chat=1&message=${encodeURIComponent(message.id)}`;
  const files = attachment ? [attachment] : message.attachments;
  return (
    <article className="conference-chat-info-message">
      <strong>{message.senderName}</strong>
      <small>{formatDate(message.createdAt)}</small>
      <p>{message.deletedAt ? "Сообщение удалено" : message.text}</p>
      {safeURL && (
        <div className="conference-chat-info-message-links">
          <a href={safeURL} target="_blank" rel="noopener noreferrer">
            {safeURL}
          </a>
        </div>
      )}
      {!message.deletedAt && files.length > 0 && (
        <div className="conference-chat-info-files">
          {files.map((file) => (
            <ConferenceChatFile
              key={file.id}
              conferenceId={conference.id}
              file={file}
            />
          ))}
        </div>
      )}
      <Link className="text-link" to={path} onClick={onNavigate}>
        Открыть в чате
      </Link>
      {!message.deletedAt && (
        <Button
          variant="outline"
          className="conference-chat-info-bookmark"
          aria-pressed={marked}
          busy={pin.isPending}
          onClick={() => pin.mutate()}
        >
          <Star
            size={16}
            fill={marked ? "currentColor" : "none"}
            aria-hidden="true"
          />
          {marked ? "Убрать из важных" : "Отметить важным"}
        </Button>
      )}
      <ErrorNotice error={pin.error} />
    </article>
  );
}

function ConferenceChatFile({
  conferenceId,
  file,
}: {
  conferenceId: string;
  file: ChatAttachment;
}) {
  const [download, setDownload] = useState<{
    url: string;
    expiresAt: string;
  } | null>(null);
  const request = useMutation({
    mutationFn: () => api.attachmentDownload(conferenceId, file.id),
    onSuccess: (result) => setDownload(result),
  });
  useEffect(() => {
    if (!download) return;
    const timer = window.setTimeout(
      () => setDownload(null),
      Math.max(0, Date.parse(download.expiresAt) - Date.now() - 5000),
    );
    return () => window.clearTimeout(timer);
  }, [download]);
  const url = safeRecordingUrl(download?.url);
  return (
    <div>
      <Button
        variant="outline"
        busy={request.isPending}
        onClick={() => request.mutate()}
      >
        <Download size={16} aria-hidden="true" />
        {file.filename} · {formatBytes(file.size)}
      </Button>
      {url && (
        <a
          className="text-link"
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          download={file.filename}
        >
          Скачать {file.filename}
        </a>
      )}
      {url &&
        file.mimeType.startsWith("image/") &&
        !file.mimeType.includes("svg") && (
          <img
            className="conference-chat-info-image"
            src={url}
            alt={file.filename}
            loading="lazy"
          />
        )}
      <ErrorNotice error={request.error} />
    </div>
  );
}
