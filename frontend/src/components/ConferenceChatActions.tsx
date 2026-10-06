import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bell,
  BellOff,
  Info,
  FolderPlus,
  LogOut,
  MoreVertical,
  UserPlus,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import type { Conference, ConferenceChatInfo } from "../types";
import { FolderPicker } from "./FolderPicker";
import { ErrorNotice } from "./ui";
import {
  ConferenceChatInfoModal,
  type ConferenceChatView,
} from "./ConferenceChatInfo";
import "./conference-chat-info.css";

/** Meeting actions are fetched only after opening this row's menu. */
export function ConferenceChatActions({
  conference,
}: {
  conference: Conference;
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [folderOpen, setFolderOpen] = useState(false);
  const [view, setView] = useState<ConferenceChatView | null>(null);

  useEffect(() => {
    if (!open) return;
    root.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    function closeOutside(event: PointerEvent) {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [open]);

  function navigateMenu(event: KeyboardEvent<HTMLDivElement>) {
    if (view || folderOpen) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      trigger.current?.focus();
      return;
    }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    if (!open) {
      setOpen(true);
      return;
    }
    const items = [
      ...(root.current?.querySelectorAll<HTMLElement>(
        '[role="menuitem"]:not(:disabled)',
      ) || []),
    ];
    const current = items.indexOf(document.activeElement as HTMLElement);
    const next =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? items.length - 1
          : (current + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
            items.length;
    items[next]?.focus();
  }

  return (
    <div
      className="conference-chat-actions"
      ref={root}
      onKeyDown={navigateMenu}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        type="button"
        ref={trigger}
        className="icon-button conference-chat-actions-trigger"
        aria-label={`Действия с конференцией: ${conference.title}`}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        onClick={() => setOpen(!open)}
      >
        <MoreVertical size={20} aria-hidden="true" />
      </button>
      {open && (
        <ConferenceChatMenu
          id={id}
          conference={conference}
          anchor={() => trigger.current}
          onFolder={() => {
            setOpen(false);
            setFolderOpen(true);
          }}
          onSelect={(next) => {
            setOpen(false);
            setView(next);
          }}
        />
      )}
      {folderOpen && (
        <FolderPicker
          target={{ type: "conference", id: conference.id }}
          onClose={() => setFolderOpen(false)}
          returnFocus={() => trigger.current}
        />
      )}
      {view && (
        <ConferenceChatInfoModal
          conference={conference}
          initialView={view}
          returnFocus={() => trigger.current}
          onClose={() => setView(null)}
        />
      )}
    </div>
  );
}

function ConferenceChatMenu({
  id,
  conference,
  anchor,
  onSelect,
  onFolder,
}: {
  id: string;
  conference: Conference;
  anchor: () => HTMLElement | null;
  onSelect: (view: ConferenceChatView) => void;
  onFolder: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const key = ["conference-chat-info", conference.id, user?.id];
  const info = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => api.conferenceChatInfo(conference.id, signal),
    retry: false,
  });
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
  const enabled = info.data?.item.notificationsEnabled;
  const menu = useRef<HTMLDivElement>(null);
  const focused = useRef(false);
  const [position, setPosition] = useState<{
    left: number;
    top: number;
  } | null>(null);
  useLayoutEffect(() => {
    function placeMenu() {
      const bounds = anchor()?.getBoundingClientRect();
      const popup = menu.current?.getBoundingClientRect();
      if (!bounds || !popup) return;
      const left = Math.max(
        16,
        Math.min(
          bounds.right - popup.width,
          window.innerWidth - popup.width - 16,
        ),
      );
      const below = bounds.bottom + 8;
      const top =
        below + popup.height <= window.innerHeight - 16
          ? below
          : Math.max(16, bounds.top - popup.height - 8);
      setPosition({ left, top });
    }
    placeMenu();
    window.addEventListener("resize", placeMenu);
    window.addEventListener("scroll", placeMenu, true);
    return () => {
      window.removeEventListener("resize", placeMenu);
      window.removeEventListener("scroll", placeMenu, true);
    };
  }, [anchor, info.error, info.isFetching, notifications.error]);
  useLayoutEffect(() => {
    if (!position || focused.current) return;
    menu.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    focused.current = true;
  }, [position]);
  return (
    <div
      ref={menu}
      style={position || { visibility: "hidden" }}
      id={id}
      role="menu"
      aria-label="Действия с конференцией"
      className="conference-chat-menu"
    >
      <button
        role="menuitem"
        tabIndex={-1}
        type="button"
        onClick={() => onSelect("info")}
      >
        <Info size={19} aria-hidden="true" />
        Информация о чате
      </button>
      <button role="menuitem" tabIndex={-1} type="button" onClick={onFolder}>
        <FolderPlus size={19} aria-hidden="true" />
        Добавить в папку
      </button>
      <button
        role="menuitem"
        tabIndex={-1}
        type="button"
        disabled={enabled === undefined || notifications.isPending}
        onClick={() => notifications.mutate(!enabled)}
      >
        {enabled ? (
          <BellOff size={19} aria-hidden="true" />
        ) : (
          <Bell size={19} aria-hidden="true" />
        )}
        {enabled ? "Выключить уведомления" : "Включить уведомления"}
      </button>
      <button
        role="menuitem"
        tabIndex={-1}
        type="button"
        onClick={() => onSelect("invite")}
      >
        <UserPlus size={19} aria-hidden="true" />
        Добавить участников
      </button>
      <button
        role="menuitem"
        tabIndex={-1}
        type="button"
        className="conference-chat-danger"
        onClick={() => onSelect("leave")}
      >
        <LogOut size={19} aria-hidden="true" />
        Покинуть чат
      </button>
      {info.isFetching && !info.data && (
        <p className="field-hint" role="status">
          Загружаем настройки…
        </p>
      )}
      <ErrorNotice error={notifications.error || info.error} />
    </div>
  );
}
