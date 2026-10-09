import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Link,
  NavLink,
  useLocation,
  useParams,
  useSearchParams,
  useNavigate,
} from "react-router";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  ArrowLeft,
  BellOff,
  Info,
  MessageCircle,
  Plus,
  Search,
} from "lucide-react";
import { api, ApiError, personalChatAPI } from "../api";
import { useAuth } from "../auth";
import { chatTime } from "../chatPresentation";
import { ConversationActions } from "../components/FolderPicker";
import { DirectUserInfoModal } from "../components/DirectConversationActions";
import { MessageThread } from "../components/ChatPanel";
import { Button, ErrorNotice, Loading } from "../components/ui";
import {
  ConversationAvatar,
  GroupCreateModal,
  GroupInfoModal,
} from "../components/GroupChats";
import type { ConversationFilters, PersonalConversation } from "../types";
import { revokePersonalConversation } from "../personalRealtime";
import "./personal.css";

export function PersonalPage() {
  const { id } = useParams();
  const location = useLocation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const filter = ["direct", "group", "unread"].includes(
    params.get("filter") || "",
  )
    ? params.get("filter")!
    : "all";
  const [creating, setCreating] = useState(false);
  const [infoOpen, setInfoOpen] = useState(false);
  const [userInfoOpen, setUserInfoOpen] = useState(false);
  const infoTrigger = useRef<HTMLButtonElement>(null);
  const userInfoTrigger = useRef<HTMLButtonElement>(null);
  const root = useRef<HTMLElement>(null);
  useEffect(() => {
    const viewport = window.visualViewport;
    const measure = () => {
      const element = root.current;
      if (!element || !window.matchMedia("(max-width:760px)").matches) return;
      const visibleHeight =
        (viewport?.height ?? window.innerHeight) + (viewport?.offsetTop ?? 0);
      const keyboard = visibleHeight < window.innerHeight - 100;
      const bottom = keyboard
        ? 12
        : (document.querySelector(".mobile-bottom-nav")?.getBoundingClientRect()
            .height ?? 64) + 20;
      element.style.setProperty(
        "--personal-mobile-height",
        `${Math.max(180, visibleHeight - element.getBoundingClientRect().top - bottom)}px`,
      );
    };
    measure();
    viewport?.addEventListener("resize", measure);
    viewport?.addEventListener("scroll", measure);
    window.addEventListener("resize", measure);
    return () => {
      viewport?.removeEventListener("resize", measure);
      viewport?.removeEventListener("scroll", measure);
      window.removeEventListener("resize", measure);
    };
  }, [id]);
  const { user } = useAuth();
  const threadVersion = useQuery({
    queryKey: ["personal-thread-version", id, user?.id],
    queryFn: () => 0,
    initialData: 0,
    staleTime: Infinity,
    enabled: false,
  });
  const revoke = useCallback(() => {
    if (!id || !user) return;
    revokePersonalConversation(client, id, user.id);
    setInfoOpen(false);
    navigate(`/personal${location.search}`, { replace: true });
  }, [client, id, user, navigate, location.search]);
  const paramSearch = params.get("search") || "";
  const [search, setSearch] = useState(paramSearch);
  useEffect(() => setSearch(paramSearch), [paramSearch]);
  useEffect(() => {
    const timer = setTimeout(() => {
      const next = new URLSearchParams(params);
      if (search.trim()) next.set("search", search.trim());
      else next.delete("search");
      if (next.toString() !== params.toString())
        setParams(next, { replace: true });
    }, 300);
    return () => clearTimeout(timer);
  }, [search, params, setParams]);
  const filters = useMemo<ConversationFilters>(
    () => ({
      ...(filter === "direct" || filter === "group" ? { type: filter } : {}),
      ...(filter === "unread" ? { unreadOnly: true } : {}),
      ...(params.get("search") ? { search: params.get("search")! } : {}),
    }),
    [filter, params],
  );
  const list = useInfiniteQuery({
    queryKey: ["personal-list", user?.id, filters],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.personalConversations(pageParam, signal, 50, filters),
    getNextPageParam: (p) => p.nextCursor || undefined,
    refetchInterval: 30000,
  });
  const detail = useQuery({
    queryKey: ["personal-detail", id, user?.id],
    queryFn: ({ signal }) => api.personalConversation(id!, signal),
    enabled: !!id,
    refetchInterval: (query) =>
      query.state.data?.item.type === "group" ? 15000 : false,
    retry: (failures, error) =>
      !(error instanceof ApiError && [403, 404].includes(error.status)) &&
      failures < 3,
  });
  const conversations = Array.from(
    new Map(
      (list.data?.pages.flatMap((p) => p.items) || []).map((c) => [c.id, c]),
    ).values(),
  );
  const inaccessible =
    detail.error instanceof ApiError &&
    [403, 404].includes(detail.error.status);
  const selected = inaccessible ? undefined : detail.data?.item;
  useEffect(() => {
    if (inaccessible && detail.data?.item.type === "group") revoke();
  }, [inaccessible, detail.data, revoke]);
  useEffect(() => {
    setInfoOpen(false);
    setUserInfoOpen(false);
  }, [id]);
  return (
    <section
      className={`personal-page ${id ? "personal-selected" : ""}`}
      ref={root}
      aria-label="Личные"
    >
      <aside className="personal-sidebar">
        <div className="section-heading">
          <h1>Личные</h1>
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus size={16} aria-hidden="true" />
            Создать группу
          </Button>
        </div>
        <div
          className="personal-filters"
          role="group"
          aria-label="Фильтр переписок"
        >
          {(
            [
              ["all", "Все"],
              ["direct", "Личные"],
              ["group", "Группы"],
              ["unread", "Новые"],
            ] as const
          ).map(([value, label]) => (
            <button
              type="button"
              key={value}
              aria-pressed={filter === value}
              onClick={() => {
                const next = new URLSearchParams(params);
                if (value === "all") next.delete("filter");
                else next.set("filter", value);
                setParams(next);
              }}
            >
              {label}
            </button>
          ))}
        </div>
        <label className="personal-search">
          <Search size={18} />
          <input
            aria-label="Поиск переписок"
            value={search}
            placeholder="Поиск переписок"
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>
        <ErrorNotice error={list.error} />
        {list.isError && (
          <Button variant="outline" onClick={() => void list.refetch()}>
            Повторить загрузку
          </Button>
        )}
        {list.isPending && <Loading />}
        <nav className="personal-conversations" aria-label="Переписки">
          {conversations.map((c) => (
            <div className="personal-conversation-row" key={c.id}>
              <NavLink
                to={`/personal/${c.id}${location.search}`}
                className={({ isActive }) =>
                  `personal-conversation ${isActive ? "selected" : ""}`
                }
              >
                <ConversationAvatar conversation={c} />
                <span className="personal-preview">
                  <strong>{conversationName(c)}</strong>
                  <span>
                    {c.type === "group" && c.preview && c.lastSender
                      ? `${c.lastSender.id === user?.id ? "Вы" : c.lastSender.displayName}: `
                      : ""}
                    {c.preview || "Начните переписку"}
                  </span>
                </span>
                <span className="personal-meta">
                  {c.type === "direct" && c.notificationsEnabled === false && (
                    <BellOff size={14} aria-label="Уведомления отключены" />
                  )}
                  <time>
                    {c.lastMessageAt ? chatTime(c.lastMessageAt) : ""}
                  </time>
                  {c.unreadCount > 0 && (
                    <span className="count-badge">{c.unreadCount}</span>
                  )}
                </span>
              </NavLink>
              <ConversationActions conversation={c} />
            </div>
          ))}
          {!list.isPending && !list.isError && !conversations.length && (
            <p className="compact-empty muted">
              {search
                ? "Переписки не найдены"
                : filter === "group"
                  ? "Пока нет групп. Создайте группу для общения с командой."
                  : filter === "unread"
                    ? "Непрочитанных сообщений нет."
                    : "Пока нет личных переписок."}
            </p>
          )}
        </nav>
        {list.hasNextPage && (
          <Button
            variant="outline"
            busy={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            Ещё переписки
          </Button>
        )}
      </aside>
      <div className="personal-content">
        {!id ? (
          <div className="personal-placeholder">
            <MessageCircle size={48} />
            <h2>Переписка</h2>
            <p>Выберите собеседника из списка переписок.</p>
          </div>
        ) : (
          <>
            <header className="personal-header">
              <Link
                to={`/personal${location.search}`}
                aria-label="Назад к перепискам"
              >
                <ArrowLeft />
              </Link>
              {selected?.type === "direct" ? (
                <button
                  ref={userInfoTrigger}
                  type="button"
                  className="personal-peer-button"
                  aria-label={`Информация о пользователе: ${selected.peer.displayName}`}
                  onClick={() => setUserInfoOpen(true)}
                >
                  <ConversationAvatar conversation={selected} />
                  <span className="personal-header-copy">
                    <strong>{selected.peer.displayName}</strong>
                    {selected.notificationsEnabled === false && (
                      <small>Без уведомлений</small>
                    )}
                  </span>
                </button>
              ) : (
                <>
                  {selected && <ConversationAvatar conversation={selected} />}
                  <div className="personal-header-copy">
                    <strong>
                      {selected ? conversationName(selected) : "Переписка"}
                    </strong>
                    {selected?.type === "group" && (
                      <small>{selected.memberCount} участн.</small>
                    )}
                  </div>
                </>
              )}
              {selected && <ConversationActions conversation={selected} />}
              {selected?.type === "group" && (
                <button
                  ref={infoTrigger}
                  type="button"
                  className="icon-button"
                  aria-label="Информация о группе"
                  onClick={() => setInfoOpen(true)}
                >
                  <Info size={22} />
                </button>
              )}
            </header>
            <ErrorNotice error={detail.error} />
            {detail.isPending ? (
              <Loading />
            ) : (
              selected && (
                <MessageThread
                  key={`${id}:${selected.type === "direct" ? selected.historyClearedThrough || 0 : 0}:${threadVersion.data}`}
                  scopeId={id}
                  transport={personalChatAPI}
                  personal
                  requireAuthenticatedDownloads={selected.type === "group"}
                  onAccessDenied={
                    selected.type === "group" ? revoke : undefined
                  }
                />
              )
            )}
          </>
        )}
      </div>
      {creating && <GroupCreateModal onClose={() => setCreating(false)} />}
      {userInfoOpen && selected?.type === "direct" && (
        <DirectUserInfoModal
          conversation={selected}
          onClose={() => setUserInfoOpen(false)}
          returnFocus={() => userInfoTrigger.current}
        />
      )}
      {infoOpen && selected?.type === "group" && (
        <GroupInfoModal
          group={selected}
          onClose={() => setInfoOpen(false)}
          returnFocus={() => infoTrigger.current}
        />
      )}
    </section>
  );
}

export function conversationName(conversation: PersonalConversation) {
  return conversation.type === "group"
    ? conversation.name
    : conversation.peer.displayName;
}
