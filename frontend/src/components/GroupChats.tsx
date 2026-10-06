import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  onlineManager,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useNavigate } from "react-router";
import {
  ArrowLeft,
  Camera,
  LogOut,
  Pencil,
  ShieldCheck,
  Trash2,
  UserPlus,
  Users,
  X,
} from "lucide-react";
import { api, ApiError } from "../api";
import { useAuth } from "../auth";
import { revokePersonalConversation } from "../personalRealtime";
import { loadGroupAvatar } from "../groupAvatarLoader";
import type {
  GroupConversation,
  GroupMember,
  PersonalConversation,
  PersonalPeer,
} from "../types";
import { initials } from "../utils";
import { Button, ErrorNotice, Loading, Modal } from "./ui";
import "./group-chats.css";

export function ConversationAvatar({
  conversation,
  large = false,
}: {
  conversation: PersonalConversation;
  large?: boolean;
}) {
  const { user } = useAuth();
  const [url, setUrl] = useState<string>();
  const group = conversation.type === "group" ? conversation : undefined;
  useEffect(() => {
    setUrl(undefined);
    if (!group?.avatarVersion) return;
    const controller = new AbortController();
    let objectURL: string | undefined;
    void loadGroupAvatar(group.id, controller.signal)
      .then((blob) => {
        if (controller.signal.aborted) return;
        objectURL = URL.createObjectURL(blob);
        setUrl(objectURL);
      })
      .catch(() => {});
    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [group?.id, group?.avatarVersion, user?.id]);
  return (
    <span
      className={`personal-avatar ${group ? "group-avatar" : ""} ${large ? "group-avatar-large" : ""}`}
      aria-hidden="true"
    >
      {url ? (
        <img src={url} alt="" />
      ) : group ? (
        <Users size={large ? 32 : 20} />
      ) : (
        initials(
          conversation.type === "direct"
            ? conversation.peer.displayName
            : conversation.name,
        )
      )}
    </span>
  );
}

function avatarError(file: File): string | undefined {
  if (!file.size || file.size > 2 * 1024 * 1024)
    return "Выберите изображение размером от 1 байта до 2 МБ.";
  if (!["image/png", "image/jpeg"].includes(file.type))
    return "Для аватара подходят PNG и JPEG.";
}

function AvatarFileField({
  file,
  onChange,
  disabled,
}: {
  file?: File;
  onChange: (file?: File) => void;
  disabled: boolean;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [url, setUrl] = useState<string>();
  const [error, setError] = useState("");
  useEffect(() => {
    if (!file) {
      setUrl(undefined);
      return;
    }
    const object = URL.createObjectURL(file);
    setUrl(object);
    return () => URL.revokeObjectURL(object);
  }, [file]);
  return (
    <div className="group-avatar-field">
      <button
        type="button"
        className="group-avatar-picker"
        disabled={disabled}
        aria-label="Выбрать аватар группы"
        onClick={() => input.current?.click()}
      >
        {url ? <img src={url} alt="" /> : <Camera size={25} />}
      </button>
      <input
        ref={input}
        type="file"
        accept="image/png,image/jpeg"
        hidden
        aria-label="Файл аватара группы"
        disabled={disabled}
        onChange={(event) => {
          const selected = event.target.files?.[0];
          event.target.value = "";
          if (!selected) return;
          const error = avatarError(selected);
          setError(error || "");
          if (!error) onChange(selected);
        }}
      />
      <span>PNG или JPEG, до 2 МБ</span>
      {file && (
        <button
          type="button"
          className="text-link"
          disabled={disabled}
          onClick={() => onChange(undefined)}
        >
          Убрать выбранный аватар
        </button>
      )}
      {error && <ErrorNotice>{error}</ErrorNotice>}
    </div>
  );
}

function GroupUserPicker({
  selected,
  onChange,
  excluded,
  disabled,
  max,
}: {
  selected: PersonalPeer[];
  onChange: (users: PersonalPeer[]) => void;
  excluded: string[];
  disabled: boolean;
  max: number;
}) {
  const { user } = useAuth();
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  const id = useId();
  useEffect(() => {
    const timer = setTimeout(() => setSearch(text.trim()), 300);
    return () => clearTimeout(timer);
  }, [text]);
  const query = useQuery({
    queryKey: ["group-user-search", user?.id, search],
    queryFn: ({ signal }) => api.searchPersonalUsers(search, signal),
    enabled: Array.from(search).length >= 2 && !disabled,
    retry: false,
  });
  const current = text.trim() === search;
  return (
    <div className="group-user-picker">
      <label className="field" htmlFor={id}>
        Найти пользователя
        <input
          id={id}
          type="search"
          value={text}
          maxLength={100}
          disabled={disabled}
          placeholder="Имя пользователя"
          onChange={(event) => setText(event.target.value)}
        />
      </label>
      <p className="field-hint">
        Введите минимум 2 символа. Выбрано: {selected.length} из {max}.
      </p>
      {!!selected.length && (
        <ul className="group-selected-users" aria-label="Выбранные участники">
          {selected.map((person) => (
            <li key={person.id}>
              {person.displayName}
              <button
                type="button"
                aria-label={`Убрать ${person.displayName}`}
                disabled={disabled}
                onClick={() =>
                  onChange(selected.filter((item) => item.id !== person.id))
                }
              >
                <X size={14} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <ErrorNotice error={query.error} />
      {Array.from(text.trim()).length >= 2 &&
        (!current || query.isFetching) && (
          <p role="status">Ищем пользователей…</p>
        )}
      {current && Array.from(search).length >= 2 && !query.isFetching && (
        <ul className="group-search-users" aria-label="Пользователи">
          {(query.data?.items || [])
            .filter((person) => !excluded.includes(person.id))
            .map((person) => {
              const checked = selected.some((item) => item.id === person.id);
              return (
                <li key={person.id}>
                  <label>
                    <span className="avatar avatar-small" aria-hidden="true">
                      {initials(person.displayName)}
                    </span>
                    <span>{person.displayName}</span>
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={
                        disabled || (!checked && selected.length >= max)
                      }
                      onChange={() =>
                        onChange(
                          checked
                            ? selected.filter((item) => item.id !== person.id)
                            : [...selected, person],
                        )
                      }
                    />
                  </label>
                </li>
              );
            })}
        </ul>
      )}
    </div>
  );
}

function invalidateGroup(
  client: ReturnType<typeof useQueryClient>,
  id: string,
) {
  for (const key of [
    "personal-list",
    "personal-summary",
    "personal-detail",
    "group-members",
  ])
    void client.invalidateQueries({
      queryKey:
        key === "personal-detail" || key === "group-members"
          ? [key, id]
          : [key],
    });
}

export function GroupCreateModal({ onClose }: { onClose: () => void }) {
  const { user } = useAuth();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<PersonalPeer[]>([]);
  const [file, setFile] = useState<File>();
  const [validation, setValidation] = useState("");
  const [created, setCreated] = useState<GroupConversation>();
  const saved = useRef<GroupConversation | undefined>(undefined);
  const submitting = useRef(false);
  const request = useRef<{ signature: string; id: string } | undefined>(
    undefined,
  );
  const controller = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);
  const finish = (group: GroupConversation) => {
    client.setQueryData(["personal-detail", group.id, user?.id], {
      status: "success",
      item: group,
    });
    invalidateGroup(client, group.id);
    onClose();
    navigate(`/personal/${group.id}`);
  };
  const create = useMutation({
    mutationFn: async () => {
      controller.current = new AbortController();
      let group = saved.current;
      if (!group) {
        const body = {
          name: name.trim(),
          description: description.trim(),
          memberIds: selected.map((person) => person.id).sort(),
        };
        const signature = JSON.stringify(body);
        if (request.current?.signature !== signature)
          request.current = { signature, id: crypto.randomUUID() };
        group = (
          await api.createGroup({
            ...body,
            clientRequestId: request.current.id,
          })
        ).item;
        saved.current = group;
        if (mounted.current) setCreated(group);
      }
      controller.current.signal.throwIfAborted();
      if (file)
        group = (
          await api.putGroupAvatar(group.id, file, controller.current.signal)
        ).item;
      return group;
    },
    onSuccess: (group) => {
      if (mounted.current) finish(group);
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  return (
    <Modal
      title="Создать группу"
      onClose={() => {
        if (!submitting.current) onClose();
      }}
      className="group-modal"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (submitting.current) return;
          if (
            !created &&
            (!name.trim() ||
              Array.from(name.trim()).length > 50 ||
              Array.from(description.trim()).length > 200)
          ) {
            setValidation(
              "Укажите название до 50 символов и описание до 200 символов.",
            );
            return;
          }
          setValidation("");
          submitting.current = true;
          create.mutate();
        }}
      >
        <AvatarFileField
          file={file}
          onChange={setFile}
          disabled={create.isPending}
        />
        <label className="field">
          Название группы
          <input
            data-autofocus
            value={name}
            maxLength={100}
            disabled={create.isPending || !!created}
            onChange={(event) => setName(event.target.value)}
            placeholder="Команда разработки"
          />
        </label>
        <p className="field-hint">{Array.from(name).length}/50</p>
        <label className="field">
          Описание (необязательно)
          <textarea
            value={description}
            maxLength={400}
            rows={3}
            disabled={create.isPending || !!created}
            onChange={(event) => setDescription(event.target.value)}
          />
        </label>
        <p className="field-hint">{Array.from(description).length}/200</p>
        <h3>Добавить участников</h3>
        <GroupUserPicker
          selected={selected}
          onChange={setSelected}
          excluded={user ? [user.id] : []}
          disabled={create.isPending || !!created}
          max={99}
        />
        {created && create.isError && (
          <p role="status">
            Группа создана. Не удалось загрузить аватар; можно повторить
            загрузку или открыть группу без него.
          </p>
        )}
        <ErrorNotice error={create.error}>{validation || null}</ErrorNotice>
        <div className="group-form-actions">
          <Button
            type="button"
            variant="outline"
            disabled={create.isPending}
            onClick={() => {
              if (!submitting.current) onClose();
            }}
          >
            Отмена
          </Button>
          <Button type="submit" busy={create.isPending}>
            {created ? "Повторить загрузку аватара" : "Создать"}
          </Button>
        </div>
        {created && create.isError && (
          <Button
            type="button"
            variant="outline"
            onClick={() => finish(created)}
          >
            Открыть группу без аватара
          </Button>
        )}
      </form>
    </Modal>
  );
}

type GroupView =
  "info" | "members" | "add" | "edit" | "leave" | "transfer" | "delete";
const roles = { owner: "Владелец", admin: "Администратор", member: "Участник" };
export function GroupInfoModal({
  group,
  onClose,
  returnFocus,
}: {
  group: GroupConversation;
  onClose: () => void;
  returnFocus: () => HTMLElement | null;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [view, setView] = useState<GroupView>("info");
  const [selected, setSelected] = useState<PersonalPeer[]>([]);
  const [search, setSearch] = useState("");
  const [validation, setValidation] = useState("");
  const [name, setName] = useState(group.name);
  const [description, setDescription] = useState(group.description);
  const [file, setFile] = useState<File>();
  const [transferId, setTransferId] = useState("");
  const [removeTarget, setRemoveTarget] = useState<GroupMember>();
  const submitting = useRef(false);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const detail = useQuery({
    queryKey: ["personal-detail", group.id, user?.id],
    queryFn: ({ signal }) => api.personalConversation(group.id, signal),
    initialData: { status: "success", item: group },
  });
  const current = detail.data.item.type === "group" ? detail.data.item : group;
  const owner = current.myRole === "owner",
    manages = owner || current.myRole === "admin";
  const [visible, setVisible] = useState(
    () => document.visibilityState !== "hidden",
  );
  useEffect(() => {
    const onVisibility = () => {
      const visible = document.visibilityState !== "hidden";
      setVisible(visible);
      if (!visible)
        void client.cancelQueries({
          queryKey: ["group-members", group.id, user?.id],
          exact: true,
        });
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, [client, group.id, user?.id]);
  const members = useQuery({
    queryKey: ["group-members", group.id, user?.id],
    queryFn: ({ signal }) => api.groupMembers(group.id, signal),
    enabled: visible,
    retry: false,
    refetchInterval: visible ? 5000 : false,
    refetchIntervalInBackground: false,
  });
  const accessError = [detail.error, members.error].find(
    (error) => error instanceof ApiError && [403, 404].includes(error.status),
  );
  const denialHandled = useRef(false);
  useEffect(() => {
    if (!accessError || denialHandled.current) return;
    denialHandled.current = true;
    if (user) revokePersonalConversation(client, group.id, user.id);
    onClose();
    navigate("/personal", { replace: true });
  }, [accessError, client, group.id, user, onClose, navigate]);
  const people = members.data?.items || [];
  const connected = useSyncExternalStore(onlineManager.subscribe, () =>
    onlineManager.isOnline(),
  );
  const mutation = useMutation({
    mutationFn: async (operation: () => Promise<unknown>) => operation(),
    onSuccess: () => {
      invalidateGroup(client, group.id);
      setRemoveTarget(undefined);
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  const run = (operation: () => Promise<unknown>, next?: GroupView) => {
    if (submitting.current) return;
    submitting.current = true;
    mutation.mutate(async () => {
      const result = await operation();
      if (next) setView(next);
      return result;
    });
  };
  const exit = async (deleting: boolean) => {
    if (deleting) await api.deleteGroup(group.id);
    else await api.leaveGroup(group.id);
    if (user) revokePersonalConversation(client, group.id, user.id);
    onClose();
    navigate("/personal");
  };
  const titles: Record<GroupView, string> = {
    info: "О группе",
    members: "Участники группы",
    add: "Добавить участников",
    edit: "Настройки группы",
    leave: "Выйти из группы",
    transfer: "Передать владение",
    delete: "Удалить группу",
  };
  if (accessError) return <ErrorNotice error={accessError} />;
  return (
    <Modal
      title={titles[view]}
      className="group-modal"
      returnFocus={returnFocus}
      onClose={() => {
        if (submitting.current) return;
        if (removeTarget) setRemoveTarget(undefined);
        else if (view !== "info") setView("info");
        else onClose();
      }}
    >
      {view !== "info" && (
        <Button
          className="group-back"
          variant="outline"
          disabled={mutation.isPending}
          onClick={() => {
            setView("info");
            setRemoveTarget(undefined);
            mutation.reset();
          }}
        >
          <ArrowLeft size={16} />О группе
        </Button>
      )}
      <ErrorNotice error={detail.error || mutation.error}>
        {validation || null}
      </ErrorNotice>
      {view === "info" && (
        <>
          <div className="group-identity">
            <ConversationAvatar conversation={current} large />
            <div>
              <h3>{current.name}</h3>
              <p>{current.memberCount} участн.</p>
              <p>{current.description}</p>
              <small>{roles[current.myRole]}</small>
            </div>
          </div>
          <div className="group-info-actions">
            <Button variant="outline" onClick={() => setView("members")}>
              <Users size={20} />
              Участники ({current.memberCount})
            </Button>
            {manages && (
              <>
                <Button variant="outline" onClick={() => setView("add")}>
                  <UserPlus size={20} />
                  Добавить участников
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setName(current.name);
                    setDescription(current.description);
                    setView("edit");
                  }}
                >
                  <Pencil size={20} />
                  Настройки группы
                </Button>
              </>
            )}
            {owner && (
              <Button variant="outline" onClick={() => setView("transfer")}>
                <ShieldCheck size={20} />
                Передать владение
              </Button>
            )}
            <Button variant="outline" onClick={() => setView("leave")}>
              <LogOut size={20} />
              Выйти из группы
            </Button>
            {owner && (
              <Button variant="danger" onClick={() => setView("delete")}>
                <Trash2 size={20} />
                Удалить группу
              </Button>
            )}
          </div>
        </>
      )}
      {(view === "members" || view === "transfer") && (
        <>
          <label className="field">
            Найти участника
            <input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          <ErrorNotice error={members.error} />
          {members.isPending && <Loading />}
          <ul className="group-members" aria-label="Участники группы">
            {people
              .filter((person) =>
                person.displayName
                  .toLocaleLowerCase("ru")
                  .includes(search.toLocaleLowerCase("ru")),
              )
              .map((person) => (
                <li key={person.id}>
                  <span className="group-member-avatar" aria-hidden="true">
                    <span className="avatar avatar-small">
                      {initials(person.displayName)}
                    </span>
                    {connected &&
                      !members.isPaused &&
                      !members.isError &&
                      person.online === true && (
                        <span className="group-presence-dot" />
                      )}
                  </span>
                  <span className="group-member-name">
                    <strong>
                      {person.displayName}
                      {person.id === user?.id ? " (вы)" : ""}
                    </strong>
                    <small>{roles[person.role]}</small>
                    <small>
                      {connected &&
                      !members.isPaused &&
                      !members.isError &&
                      typeof person.online === "boolean"
                        ? person.online
                          ? "В сети"
                          : "Не в сети"
                        : "Статус недоступен"}
                    </small>
                  </span>
                  {view === "transfer" ? (
                    person.id !== user?.id && (
                      <label className="group-transfer-choice">
                        <input
                          type="radio"
                          name="owner"
                          aria-label={`Передать владение: ${person.displayName}`}
                          checked={transferId === person.id}
                          onChange={() => setTransferId(person.id)}
                        />
                      </label>
                    )
                  ) : (
                    <div className="group-member-controls">
                      {owner && person.id !== user?.id && (
                        <Button
                          variant="outline"
                          disabled={mutation.isPending}
                          onClick={() =>
                            run(() =>
                              api.setGroupRole(
                                group.id,
                                person.id,
                                person.role === "admin" ? "member" : "admin",
                              ),
                            )
                          }
                        >
                          {person.role === "admin"
                            ? "Снять администратора"
                            : "Назначить администратором"}
                        </Button>
                      )}
                      {person.id !== user?.id &&
                        (owner || (manages && person.role === "member")) && (
                          <Button
                            variant="outline"
                            disabled={mutation.isPending}
                            onClick={() => setRemoveTarget(person)}
                          >
                            Удалить участника
                          </Button>
                        )}
                    </div>
                  )}
                </li>
              ))}
          </ul>
          {removeTarget && (
            <div className="group-confirmation">
              <p>
                Удалить {removeTarget.displayName} из группы? Доступ к
                сообщениям и файлам будет закрыт.
              </p>
              <Button
                variant="outline"
                disabled={mutation.isPending}
                onClick={() => setRemoveTarget(undefined)}
              >
                Отмена
              </Button>
              <Button
                variant="danger"
                busy={mutation.isPending}
                onClick={() =>
                  run(() => api.removeGroupMember(group.id, removeTarget.id))
                }
              >
                Подтвердить удаление участника
              </Button>
            </div>
          )}
          {view === "transfer" && (
            <>
              <p className="field-hint">
                Выбранный участник станет владельцем, а вы — администратором.
              </p>
              <Button
                busy={mutation.isPending}
                disabled={!transferId || !owner}
                onClick={() =>
                  run(() => api.transferGroup(group.id, transferId), "info")
                }
              >
                Передать владение
              </Button>
            </>
          )}
        </>
      )}
      {view === "add" && (
        <>
          <GroupUserPicker
            selected={selected}
            onChange={setSelected}
            excluded={people.map((person) => person.id)}
            disabled={mutation.isPending}
            max={Math.max(0, 100 - current.memberCount)}
          />
          <Button
            busy={mutation.isPending}
            disabled={!selected.length || !manages}
            onClick={() =>
              run(async () => {
                const result = await api.addGroupMembers(
                  group.id,
                  selected.map((person) => person.id),
                );
                setSelected([]);
                return result;
              }, "members")
            }
          >
            Добавить выбранных
          </Button>
        </>
      )}
      {view === "edit" && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (
              !name.trim() ||
              Array.from(name.trim()).length > 50 ||
              Array.from(description.trim()).length > 200
            ) {
              setValidation("Название — до 50 символов, описание — до 200.");
              return;
            }
            setValidation("");
            run(async () => {
              await api.updateGroup(group.id, {
                name: name.trim(),
                description: description.trim(),
              });
              if (file) {
                controller.current = new AbortController();
                await api.putGroupAvatar(
                  group.id,
                  file,
                  controller.current.signal,
                );
                setFile(undefined);
              }
            }, "info");
          }}
        >
          <AvatarFileField
            file={file}
            onChange={setFile}
            disabled={mutation.isPending}
          />
          {current.avatarVersion && (
            <Button
              type="button"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => run(() => api.deleteGroupAvatar(group.id))}
            >
              Удалить аватар группы
            </Button>
          )}
          <label className="field">
            Название группы
            <input
              value={name}
              maxLength={100}
              disabled={mutation.isPending}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <label className="field">
            Описание
            <textarea
              rows={3}
              value={description}
              maxLength={400}
              disabled={mutation.isPending}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          <Button type="submit" busy={mutation.isPending} disabled={!manages}>
            Сохранить
          </Button>
        </form>
      )}
      {view === "leave" && (
        <>
          {owner && current.memberCount > 1 ? (
            <>
              <p>Перед выходом передайте владение другому участнику.</p>
              <Button onClick={() => setView("transfer")}>
                Выбрать нового владельца
              </Button>
            </>
          ) : (
            <>
              <p>
                Выйти из группы «{current.name}»? Доступ к сообщениям и файлам
                будет закрыт.
              </p>
              <Button
                variant="danger"
                busy={mutation.isPending}
                onClick={() => run(() => exit(false))}
              >
                Подтвердить выход
              </Button>
            </>
          )}
        </>
      )}
      {view === "delete" && (
        <>
          <p>
            Удалить группу «{current.name}» для всех участников? Доступ к её
            сообщениям и файлам будет закрыт.
          </p>
          <Button
            variant="danger"
            busy={mutation.isPending}
            disabled={!owner}
            onClick={() => run(() => exit(true))}
          >
            Подтвердить удаление группы
          </Button>
        </>
      )}
    </Modal>
  );
}
