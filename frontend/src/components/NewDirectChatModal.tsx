import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Search } from "lucide-react";
import { useNavigate } from "react-router";
import { api } from "../api";
import { useAuth } from "../auth";
import { initials } from "../utils";
import { Button, ErrorNotice, Modal } from "./ui";
import { MessagingSkeleton } from "./MessagingSkeleton";
import "./new-direct-chat-modal.css";

/**
 * Находит зарегистрированного собеседника и открывает существующую либо новую личную переписку.
 * @args onClose — закрывает окно поиска, сохраняя текущую страницу.
 * @return Диалог поиска с результатами реального API и обработкой ошибки создания.
 */
export function NewDirectChatModal({ onClose }: { onClose: () => void }) {
  const { user } = useAuth();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const mounted = useRef(true);
  const activeActor = useRef(user?.id);
  activeActor.current = user?.id;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    const timer = setTimeout(() => setQuery(search.trim()), 300);
    return () => clearTimeout(timer);
  }, [search]);
  const valid =
    Array.from(query).length >= 2 && Array.from(query).length <= 100;
  const users = useQuery({
    queryKey: ["personal-user-search", user?.id, query],
    queryFn: ({ signal }) => api.searchPersonalUsers(query, signal),
    enabled: valid,
  });
  const create = useMutation({
    mutationFn: ({ peerId }: { peerId: string; actorId?: string }) =>
      api.createPersonalConversation(peerId),
    onSuccess: ({ item }, { actorId }) => {
      if (!mounted.current || actorId !== activeActor.current) return;
      void client.invalidateQueries({ queryKey: ["personal-list", actorId] });
      client.setQueryData(["personal-detail", item.id, actorId], { item });
      onClose();
      navigate(`/personal/${item.id}`);
    },
  });
  const results =
    users.data?.items.filter((peer) => peer.id !== user?.id) || [];
  return (
    <Modal title="Новый чат" onClose={onClose} className="new-direct-modal">
      <label className="field">
        Найти пользователя
        <input
          type="search"
          data-autofocus
          value={search}
          maxLength={100}
          placeholder="Имя пользователя"
          onChange={(event) => setSearch(event.target.value)}
        />
      </label>
      <p className="field-hint">Введите минимум 2 символа имени.</p>
      <ErrorNotice error={users.error || create.error} />
      {users.isError && (
        <Button variant="outline" onClick={() => void users.refetch()}>
          Повторить поиск
        </Button>
      )}
      {valid && users.isPending && <MessagingSkeleton />}
      {valid && !users.isPending && !users.isError && results.length === 0 && (
        <div className="personal-search-empty">
          <Search size={32} />
          <h3>Никого не нашли</h3>
          <p>Попробуйте другое имя.</p>
        </div>
      )}
      <div className="personal-user-results">
        {results.map((peer) => (
          <button
            type="button"
            key={peer.id}
            disabled={create.isPending}
            onClick={() =>
              create.mutate({ peerId: peer.id, actorId: user?.id })
            }
          >
            <span className="personal-avatar" aria-hidden="true">
              {initials(peer.displayName)}
            </span>
            <span>
              <strong>{peer.displayName}</strong>
              <small>Начать переписку</small>
            </span>
            <ChevronRight size={18} aria-hidden="true" />
          </button>
        ))}
      </div>
    </Modal>
  );
}
