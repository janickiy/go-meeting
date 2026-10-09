import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderPlus, Info } from "lucide-react";
import { api, ApiError } from "../api";
import { useAuth } from "../auth";
import {
  folderAccessDenied,
  invalidateFolders,
  purgeFolderTarget,
  purgeFolder,
  updateFolder,
  updateFolderMapping,
  useFolderRequest,
} from "../folders";
import type {
  FolderTarget,
  PersonalConversation,
  PersonalFolder,
} from "../types";
import { Button, ErrorNotice, Loading, Modal } from "./ui";
import { FolderNameForm } from "./FolderModals";
import { ItemActions } from "./ItemActions";
import { DirectConversationActions } from "./DirectConversationActions";
import "./folders.css";

/** Each checkbox is an idempotent mapping write; folders never grant access. */
export function FolderPicker({
  target,
  onClose,
  returnFocus,
}: {
  target: FolderTarget;
  onClose: () => void;
  returnFocus?: () => HTMLElement | null;
}) {
  const { user } = useAuth();
  return (
    <FolderPickerContent
      key={`${user?.id}:${target.type}:${target.id}`}
      target={target}
      onClose={onClose}
      returnFocus={returnFocus}
    />
  );
}
function FolderPickerContent({
  target,
  onClose,
  returnFocus,
}: {
  target: FolderTarget;
  onClose: () => void;
  returnFocus?: () => HTMLElement | null;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const request = useFolderRequest();
  const gate = useRef(false);
  const [creating, setCreating] = useState(false);
  const [pending, setPending] = useState<{
    id: string;
    present: boolean;
  } | null>(null);
  const [search, setSearch] = useState("");
  const folders = useQuery({
    queryKey: ["folders", user?.id, target],
    queryFn: ({ signal }) => api.folders(target, signal),
    retry: false,
  });
  const denied = folderAccessDenied(folders.error);
  useEffect(() => {
    if (denied && user) {
      purgeFolderTarget(client, user.id, target);
      onClose();
    }
  }, [denied, client, user, target, onClose]);
  const write = useMutation({
    mutationFn: ({ id, present }: { id: string; present: boolean }) =>
      api.setFolderItem(id, target, present, request.signal()),
    onSuccess: async ({ item }, { present }) => {
      if (!request.mounted.current || !user) return;
      updateFolder(client, user.id, item, target, present);
      updateFolderMapping(client, user.id, item.id, target, present);
      await invalidateFolders(client, user.id);
    },
    onError: (error, { id }) => {
      if (!request.mounted.current || !user) return;
      if (error instanceof ApiError && error.status === 404) {
        purgeFolder(client, user.id, id);
        void folders.refetch();
      } else if (folderAccessDenied(error)) {
        purgeFolderTarget(client, user.id, target);
        void folders.refetch();
      }
    },
    onSettled: () => {
      gate.current = false;
      if (request.mounted.current) setPending(null);
    },
  });
  const select = (id: string, present: boolean) => {
    if (gate.current) return;
    gate.current = true;
    setPending({ id, present });
    write.mutate({ id, present });
  };
  const close = () => {
    if (gate.current) return;
    if (creating) setCreating(false);
    else onClose();
  };
  const added = (folder: PersonalFolder) => {
    setCreating(false);
    gate.current = false;
    select(folder.id, true);
  };
  const items = denied
    ? []
    : folders.data?.items.filter((folder) =>
        folder.name
          .toLocaleLowerCase()
          .includes(search.trim().toLocaleLowerCase()),
      ) || [];
  return (
    <Modal
      title={creating ? "Новая папка" : "Добавить в папку"}
      onClose={close}
      returnFocus={returnFocus}
      className="folder-modal"
    >
      {creating ? (
        <FolderNameForm
          busyGate={gate}
          onSaved={added}
          onCancel={() => setCreating(false)}
        />
      ) : (
        <>
          <p className="folder-hint">
            Можно выбрать несколько папок. Изменения сохраняются сразу.
          </p>
          <label className="field">
            Поиск папки
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Название папки"
            />
          </label>
          <ErrorNotice error={folders.error} />
          {folders.isError && (
            <Button variant="outline" onClick={() => void folders.refetch()}>
              Повторить загрузку
            </Button>
          )}
          {folders.isPending && <Loading />}
          <div
            className="folder-checkboxes"
            aria-busy={write.isPending || undefined}
          >
            {items.map((folder) => (
              <label key={folder.id}>
                <input
                  type="checkbox"
                  checked={
                    pending?.id === folder.id
                      ? pending.present
                      : folder.contains === true
                  }
                  disabled={write.isPending}
                  onChange={(event) => select(folder.id, event.target.checked)}
                />
                <span>
                  <strong>{folder.name}</strong>
                  <small>{folder.itemCount} элементов</small>
                </span>
              </label>
            ))}
            {!folders.isPending && !folders.isError && !items.length && (
              <p className="muted">
                {search ? "Папки не найдены." : "У вас пока нет папок."}
              </p>
            )}
          </div>
          <ErrorNotice error={write.error} />
          <div className="folder-picker-actions">
            <Button
              variant="outline"
              disabled={
                write.isPending || (folders.data?.items.length || 0) >= 100
              }
              onClick={() => setCreating(true)}
            >
              <FolderPlus size={18} aria-hidden="true" />
              Новая папка
            </Button>
            <Button disabled={write.isPending} onClick={close}>
              Готово
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}
export function ConversationActions({
  conversation,
  onGroupInfo,
}: {
  conversation: PersonalConversation;
  onGroupInfo?: () => void;
}) {
  return conversation.type === "direct" ? (
    <DirectConversationActions
      key={conversation.id}
      conversation={conversation}
    />
  ) : (
    <FolderConversationActions
      conversation={conversation}
      onGroupInfo={onGroupInfo}
    />
  );
}

/** Сохраняет прежнее меню папок для групповых переписок. */
function FolderConversationActions({
  conversation,
  onGroupInfo,
}: {
  conversation: PersonalConversation;
  onGroupInfo?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const name =
    conversation.type === "group"
      ? conversation.name
      : conversation.peer.displayName;
  return (
    <>
      <ItemActions
        label={`Действия с перепиской: ${name}`}
        actions={[
          ...(onGroupInfo
            ? [
                {
                  label: "Информация",
                  icon: <Info size={19} aria-hidden="true" />,
                  run: onGroupInfo,
                },
              ]
            : []),
          {
            label: "Добавить в папку",
            icon: <FolderPlus size={19} aria-hidden="true" />,
            run: () => setOpen(true),
          },
        ]}
      />
      {open && (
        <FolderPicker
          target={{ type: "conversation", id: conversation.id }}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
