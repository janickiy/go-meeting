import { useEffect, useRef, useState } from "react";
import { Link, NavLink, useNavigate, useParams } from "react-router";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ArrowLeft, MessageCircle, Plus, Search } from "lucide-react";
import { api, personalChatAPI } from "../api";
import { useAuth } from "../auth";
import { initials } from "../utils";
import { chatTime } from "../chatPresentation";
import { MessageThread } from "../components/ChatPanel";
import { Button, ErrorNotice, Loading, Modal } from "../components/ui";
import "./personal.css";

export function PersonalPage() {
  const { id } = useParams();
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
  const navigate = useNavigate();
  const client = useQueryClient();
  const [search, setSearch] = useState("");
  const [newMessage, setNewMessage] = useState(false);
  const list = useInfiniteQuery({
    queryKey: ["personal-list", user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.personalConversations(pageParam, signal),
    getNextPageParam: (p) => p.nextCursor || undefined,
    refetchInterval: 30000,
  });
  const detail = useQuery({
    queryKey: ["personal-detail", id, user?.id],
    queryFn: ({ signal }) => api.personalConversation(id!, signal),
    enabled: !!id,
  });
  const conversations = Array.from(
    new Map(
      (list.data?.pages.flatMap((p) => p.items) || []).map((c) => [c.id, c]),
    ).values(),
  ).filter((c) =>
    c.peer.displayName.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  );
  return (
    <section
      className={`personal-page ${id ? "personal-selected" : ""}`}
      ref={root}
      aria-label="Личные"
    >
      <aside className="personal-sidebar">
        <div className="section-heading">
          <h1>Личные</h1>
          <Button
            onClick={() => setNewMessage(true)}
            aria-label="Новое сообщение"
          >
            <Plus size={18} />
            <span>Новое сообщение</span>
          </Button>
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
            <NavLink
              key={c.id}
              to={`/personal/${c.id}`}
              className={({ isActive }) =>
                `personal-conversation ${isActive ? "selected" : ""}`
              }
            >
              <span className="personal-avatar" aria-hidden="true">
                {initials(c.peer.displayName)}
              </span>
              <span className="personal-preview">
                <strong>{c.peer.displayName}</strong>
                <span>{c.preview || "Начните переписку"}</span>
              </span>
              <span className="personal-meta">
                <time>{c.lastMessageAt ? chatTime(c.lastMessageAt) : ""}</time>
                {c.unreadCount > 0 && (
                  <span className="count-badge">{c.unreadCount}</span>
                )}
              </span>
            </NavLink>
          ))}
          {!list.isPending && !list.isError && !conversations.length && (
            <p className="compact-empty muted">
              {search
                ? "Переписки не найдены"
                : "Выберите пользователя, чтобы написать первое сообщение."}
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
            <h2>Личная переписка</h2>
            <p>Выберите собеседника или создайте новое сообщение.</p>
          </div>
        ) : (
          <>
            <header className="personal-header">
              <Link to="/personal" aria-label="Назад к перепискам">
                <ArrowLeft />
              </Link>
              <strong>
                {detail.data?.item.peer.displayName || "Личная переписка"}
              </strong>
            </header>
            <ErrorNotice error={detail.error} />
            {detail.isPending ? (
              <Loading />
            ) : (
              detail.data && (
                <MessageThread
                  key={id}
                  scopeId={id}
                  transport={personalChatAPI}
                  personal
                />
              )
            )}
          </>
        )}
      </div>
      {newMessage && (
        <NewMessage
          onClose={() => setNewMessage(false)}
          onOpen={(id) => {
            setNewMessage(false);
            void client.invalidateQueries({ queryKey: ["personal-list"] });
            navigate(`/personal/${id}`);
          }}
        />
      )}
    </section>
  );
}
function NewMessage({
  onClose,
  onOpen,
}: {
  onClose: () => void;
  onOpen: (id: string) => void;
}) {
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setSearch(text.trim()), 300);
    return () => clearTimeout(timer);
  }, [text]);
  const query = useQuery({
    queryKey: ["personal-users", search],
    queryFn: ({ signal }) => api.searchPersonalUsers(search, signal),
    enabled: Array.from(search).length >= 2 && Array.from(search).length <= 100,
  });
  const create = useMutation({
    mutationFn: api.createPersonalConversation,
    onSuccess: (r) => onOpen(r.item.id),
  });
  return (
    <Modal
      title="Новое сообщение"
      onClose={() => {
        if (!create.isPending) onClose();
      }}
    >
      <label className="field">
        Имя пользователя
        <input
          data-autofocus
          aria-label="Имя пользователя"
          value={text}
          maxLength={100}
          placeholder="Введите минимум 2 символа"
          onChange={(e) => setText(e.target.value)}
        />
      </label>
      <ErrorNotice error={query.error || create.error} />
      {search.length < 2 ? (
        <p className="field-hint">Найдите собеседника по имени.</p>
      ) : query.isFetching ? (
        <Loading />
      ) : (
        <div className="personal-user-results">
          {query.data?.items.map((peer) => (
            <Button
              key={peer.id}
              variant="outline"
              busy={create.isPending && create.variables === peer.id}
              disabled={create.isPending}
              onClick={() => create.mutate(peer.id)}
            >
              <span className="personal-avatar">
                {initials(peer.displayName)}
              </span>
              {peer.displayName}
            </Button>
          ))}
          {query.isSuccess && !query.data.items.length && (
            <p>Пользователи не найдены.</p>
          )}
        </div>
      )}
    </Modal>
  );
}
