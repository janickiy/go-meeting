import { useEffect, useRef, useState, type RefObject } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useAuth } from "../auth";
import {
  folderAccessDenied,
  invalidateFolders,
  purgeFolder,
  useFolderRequest,
} from "../folders";
import type { PersonalFolder } from "../types";
import { Button, ErrorNotice, Modal } from "./ui";
import "./folders.css";

export function FolderNameForm({
  folder,
  onSaved,
  onCancel,
  busyGate,
  onUnavailable,
}: {
  folder?: PersonalFolder;
  onSaved: (folder: PersonalFolder) => void;
  onCancel: () => void;
  busyGate?: RefObject<boolean>;
  onUnavailable?: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const request = useFolderRequest();
  const [name, setName] = useState(folder?.name || ""),
    [validation, setValidation] = useState("");
  const ownGate = useRef(false),
    input = useRef<HTMLInputElement>(null);
  const gate = busyGate || ownGate;
  useEffect(() => input.current?.focus(), []);
  const mutation = useMutation({
    mutationFn: () =>
      folder
        ? api.renameFolder(
            folder.id,
            name.trim().normalize("NFC"),
            request.signal(),
          )
        : api.createFolder(name.trim().normalize("NFC"), request.signal()),
    onSuccess: ({ item }) => {
      if (!request.mounted.current || !user) return;
      void invalidateFolders(client, user.id);
      gate.current = false;
      onSaved(item);
    },
    onError: (error) => {
      if (folder && request.mounted.current && folderAccessDenied(error))
        onUnavailable?.();
    },
    onSettled: (_, error) => {
      if (error) gate.current = false;
    },
  });
  return (
    <form
      className="folder-name-form"
      onSubmit={(event) => {
        event.preventDefault();
        if (gate.current) return;
        const normalized = name.trim().normalize("NFC");
        if (
          !normalized ||
          Array.from(normalized).length > 50 ||
          /[\p{Cc}\u202a-\u202e\u2066-\u2069]/u.test(normalized)
        ) {
          setValidation(
            "Укажите название от 1 до 50 символов без управляющих символов.",
          );
          return;
        }
        setValidation("");
        gate.current = true;
        mutation.mutate();
      }}
    >
      <label className="field">
        Название папки
        <input
          ref={input}
          data-autofocus
          value={name}
          maxLength={100}
          disabled={mutation.isPending}
          onChange={(event) => setName(event.target.value)}
          placeholder="Проект Meetrix"
        />
      </label>
      <p className="folder-hint">
        {Array.from(name.normalize("NFC")).length}/50
      </p>
      <ErrorNotice error={mutation.error}>{validation || null}</ErrorNotice>
      <div className="folder-form-actions">
        <Button
          type="button"
          variant="outline"
          disabled={mutation.isPending}
          onClick={() => {
            if (!gate.current) onCancel();
          }}
        >
          Отмена
        </Button>
        <Button type="submit" busy={mutation.isPending}>
          {folder ? "Сохранить" : "Создать"}
        </Button>
      </div>
    </form>
  );
}

export function FolderManageModal({
  mode,
  folder,
  onClose,
  onDeleted,
  returnFocus,
}: {
  mode: "create" | "rename" | "delete";
  folder?: PersonalFolder;
  onClose: () => void;
  onDeleted?: () => void;
  returnFocus?: () => HTMLElement | null;
}) {
  const { user } = useAuth();
  return (
    <FolderManageModalContent
      key={`${user?.id}:${mode}:${folder?.id || "new"}`}
      mode={mode}
      folder={folder}
      onClose={onClose}
      onDeleted={onDeleted}
      returnFocus={returnFocus}
    />
  );
}
function FolderManageModalContent({
  mode,
  folder,
  onClose,
  onDeleted,
  returnFocus,
}: {
  mode: "create" | "rename" | "delete";
  folder?: PersonalFolder;
  onClose: () => void;
  onDeleted?: () => void;
  returnFocus?: () => HTMLElement | null;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const request = useFolderRequest();
  const gate = useRef(false);
  const unavailable = () => {
    if (!request.mounted.current || !user || !folder) return;
    purgeFolder(client, user.id, folder.id);
    void invalidateFolders(client, user.id);
    onClose();
    onDeleted?.();
  };
  const remove = useMutation({
    mutationFn: () => api.deleteFolder(folder!.id, request.signal()),
    onSuccess: () => {
      if (!request.mounted.current || !user) return;
      purgeFolder(client, user.id, folder!.id);
      void invalidateFolders(client, user.id);
      onClose();
      onDeleted?.();
    },
    onError: (error) => {
      if (folderAccessDenied(error)) unavailable();
    },
    onSettled: () => {
      gate.current = false;
    },
  });
  return (
    <Modal
      title={
        mode === "create"
          ? "Новая папка"
          : mode === "rename"
            ? "Переименовать папку"
            : "Удалить папку"
      }
      className="folder-modal"
      returnFocus={returnFocus}
      onClose={() => {
        if (!gate.current) onClose();
      }}
    >
      {mode !== "delete" ? (
        <FolderNameForm
          folder={folder}
          busyGate={gate}
          onSaved={() => onClose()}
          onCancel={onClose}
          onUnavailable={unavailable}
        />
      ) : (
        <>
          <p>
            Удалить папку «{folder?.name}»? Чаты и встречи сохранятся. Будут
            удалены только ссылки на них в этой папке.
          </p>
          <ErrorNotice error={remove.error} />
          <div className="folder-form-actions">
            <Button
              variant="outline"
              disabled={remove.isPending}
              onClick={() => {
                if (!gate.current) onClose();
              }}
            >
              Отмена
            </Button>
            <Button
              variant="danger"
              busy={remove.isPending}
              onClick={() => {
                if (!gate.current) {
                  gate.current = true;
                  remove.mutate();
                }
              }}
            >
              Удалить папку
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}
