import { useCallback, useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import {
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  Folder,
  FolderPlus,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import {
  folderAccessDenied,
  invalidateFolders,
  purgeFolder,
  updateFolder,
  updateFolderMapping,
  useFolderRequest,
  useFolderVisibility,
} from "../folders";
import type { FolderItem, FolderItemFilters, PersonalFolder } from "../types";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { ItemActions } from "../components/ItemActions";
import { FolderManageModal } from "../components/FolderModals";
import { FolderPicker } from "../components/FolderPicker";
import {
  FolderItemLabel,
  FolderItemPicker,
  folderItemHref,
  folderItemName,
  folderItemTarget,
} from "../components/FolderItems";
import "./folders-page.css";

type Management = {
  mode: "create" | "rename" | "delete";
  folder?: PersonalFolder;
};
export function FoldersPage() {
  const { id } = useParams();
  const { user } = useAuth();
  return id ? (
    <FolderDetail key={`${user?.id}:${id}`} id={id} />
  ) : (
    <FolderList key={user?.id} />
  );
}
function FolderList() {
  const { user } = useAuth();
  const client = useQueryClient();
  const request = useFolderRequest();
  const visible = useFolderVisibility();
  const gate = useRef(false);
  const [manage, setManage] = useState<Management | null>(null);
  const list = useQuery({
    queryKey: ["folders", user?.id],
    queryFn: ({ signal }) => api.folders(undefined, signal),
    refetchInterval: visible ? 15000 : false,
  });
  const items = list.data?.items || [];
  const order = useMutation({
    mutationFn: (ids: string[]) => api.orderFolders(ids, request.signal()),
    onSuccess: ({ items }) => {
      if (!request.mounted.current || !user) return;
      client.setQueryData(["folders", user.id], { status: "success", items });
      void invalidateFolders(client, user.id);
    },
    onSettled: () => {
      gate.current = false;
    },
  });
  function move(index: number, step: number) {
    if (gate.current) return;
    const next = items.map((folder) => folder.id);
    [next[index], next[index + step]] = [next[index + step], next[index]];
    gate.current = true;
    order.mutate(next);
  }
  return (
    <section className="folders-page" aria-label="Папки">
      <header className="folders-heading">
        <div>
          <p className="eyebrow">ЛИЧНОЕ ПРОСТРАНСТВО</p>
          <h1>Папки</h1>
          <p className="muted">Соберите нужные чаты и встречи в одном месте.</p>
        </div>
        <Button
          disabled={items.length >= 100}
          onClick={() => setManage({ mode: "create" })}
        >
          <Plus size={18} aria-hidden="true" />
          Новая папка
        </Button>
      </header>
      <ErrorNotice error={list.error || order.error} />
      {list.isError && (
        <Button variant="outline" onClick={() => void list.refetch()}>
          Повторить загрузку
        </Button>
      )}
      {list.isPending && <Loading />}
      <div className="folder-list" aria-busy={order.isPending || undefined}>
        {items.map((folder, index) => (
          <div className="folder-row" key={folder.id}>
            <Link to={`/folders/${folder.id}`} className="folder-link">
              <span className="folder-symbol">
                <Folder size={27} aria-hidden="true" />
              </span>
              <span className="folder-item-copy">
                <strong>{folder.name}</strong>
                <small>
                  {folder.conversationCount} чатов · {folder.conferenceCount}{" "}
                  встреч
                </small>
              </span>
              <span
                className="folder-count"
                aria-label={`${folder.itemCount} элементов`}
              >
                {folder.itemCount}
              </span>
            </Link>
            <ItemActions
              label={`Действия с папкой: ${folder.name}`}
              actions={[
                {
                  label: "Переименовать",
                  icon: <Pencil size={18} aria-hidden="true" />,
                  run: () => setManage({ mode: "rename", folder }),
                  disabled: order.isPending,
                },
                {
                  label: "Переместить выше",
                  icon: <ArrowUp size={18} aria-hidden="true" />,
                  run: () => move(index, -1),
                  disabled: index === 0 || order.isPending,
                },
                {
                  label: "Переместить ниже",
                  icon: <ArrowDown size={18} aria-hidden="true" />,
                  run: () => move(index, 1),
                  disabled: index === items.length - 1 || order.isPending,
                },
                {
                  label: "Удалить папку",
                  icon: <Trash2 size={18} aria-hidden="true" />,
                  run: () => setManage({ mode: "delete", folder }),
                  danger: true,
                  disabled: order.isPending,
                },
              ]}
            />
          </div>
        ))}
      </div>
      {!list.isPending && !list.isError && !items.length && (
        <div className="folder-empty">
          <FolderPlus size={48} aria-hidden="true" />
          <h2>У вас пока нет папок</h2>
          <p>Создайте папку и добавьте в неё чаты или встречи.</p>
        </div>
      )}
      {manage && (
        <FolderManageModal {...manage} onClose={() => setManage(null)} />
      )}
    </section>
  );
}
function FolderDetail({ id }: { id: string }) {
  const { user } = useAuth();
  const client = useQueryClient();
  const navigate = useNavigate();
  const request = useFolderRequest();
  const visible = useFolderVisibility();
  const gate = useRef(false);
  const [params, setParams] = useSearchParams();
  const type = (
    ["conversation", "conference"].includes(params.get("type") || "")
      ? params.get("type")
      : "all"
  ) as FolderItemFilters["type"];
  const paramSearch = params.get("search") || "";
  const [text, setText] = useState(paramSearch);
  useEffect(() => setText(paramSearch), [paramSearch]);
  useEffect(() => {
    const timer = setTimeout(() => {
      const next = new URLSearchParams(params);
      if (text.trim()) next.set("search", text.trim());
      else next.delete("search");
      if (next.toString() !== params.toString())
        setParams(next, { replace: true });
    }, 300);
    return () => clearTimeout(timer);
  }, [text, params, setParams]);
  const [manage, setManage] = useState<Management | null>(null),
    [adding, setAdding] = useState(false),
    [mapping, setMapping] = useState<FolderItem | null>(null);
  const unavailable = useCallback(() => {
    if (user) purgeFolder(client, user.id, id);
    setManage(null);
    setAdding(false);
    setMapping(null);
    navigate("/folders", { replace: true });
  }, [user, client, id, navigate]);
  const meta = useQuery({
    queryKey: ["folder", user?.id, id],
    queryFn: ({ signal }) => api.folder(id, signal),
    refetchInterval: visible ? 15000 : false,
    retry: (count, error) => !folderAccessDenied(error) && count < 2,
  });
  const entries = useInfiniteQuery({
    queryKey: [
      "folder-items",
      user?.id,
      id,
      { type, search: params.get("search") || "" },
    ],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.folderItems(
        id,
        { type, search: params.get("search") || "" },
        pageParam,
        signal,
      ),
    getNextPageParam: (page) => page.nextCursor || undefined,
    enabled: !!meta.data && !folderAccessDenied(meta.error),
    refetchInterval: visible ? 15000 : false,
    retry: (count, error) => !folderAccessDenied(error) && count < 2,
  });
  const denied =
    folderAccessDenied(meta.error) || folderAccessDenied(entries.error);
  useEffect(() => {
    if (denied) unavailable();
  }, [denied, unavailable]);
  const remove = useMutation({
    mutationFn: (entry: FolderItem) =>
      api.setFolderItem(id, folderItemTarget(entry), false, request.signal()),
    onSuccess: async ({ item }, entry) => {
      if (!request.mounted.current || !user) return;
      const target = folderItemTarget(entry);
      updateFolder(client, user.id, item, target, false);
      updateFolderMapping(client, user.id, id, target, false);
      await invalidateFolders(client, user.id);
    },
    onError: (error) => {
      if (request.mounted.current && folderAccessDenied(error)) unavailable();
    },
    onSettled: () => {
      gate.current = false;
    },
  });
  const folder = denied ? undefined : meta.data?.item;
  const items = denied
    ? []
    : Array.from(
        new Map(
          (entries.data?.pages.flatMap((page) => page.items) || []).map(
            (entry) => [`${entry.type}:${entry.item.id}`, entry],
          ),
        ).values(),
      );
  return (
    <section className="folders-page" aria-label="Содержимое папки">
      <Link to="/folders" className="folder-back">
        <ArrowLeft size={18} aria-hidden="true" />
        Все папки
      </Link>
      <header className="folders-heading">
        <div>
          <h1>{folder?.name || "Папка"}</h1>
          {folder && (
            <p className="muted">
              {folder.conversationCount} чатов · {folder.conferenceCount} встреч
            </p>
          )}
        </div>
        {folder && (
          <div className="folders-heading-actions">
            <Button onClick={() => setAdding(true)}>
              <Plus size={18} aria-hidden="true" />
              Добавить
            </Button>
            <ItemActions
              label={`Действия с папкой: ${folder.name}`}
              actions={[
                {
                  label: "Переименовать",
                  icon: <Pencil size={18} aria-hidden="true" />,
                  run: () => setManage({ mode: "rename", folder }),
                },
                {
                  label: "Удалить папку",
                  icon: <Trash2 size={18} aria-hidden="true" />,
                  run: () => setManage({ mode: "delete", folder }),
                  danger: true,
                },
              ]}
            />
          </div>
        )}
      </header>
      <ErrorNotice error={meta.error} />
      {meta.isError && !denied && (
        <Button variant="outline" onClick={() => void meta.refetch()}>
          Повторить загрузку
        </Button>
      )}
      {meta.isPending && <Loading />}
      {folder && (
        <>
          <div className="folder-toolbar">
            <div
              className="folder-candidate-filters"
              role="group"
              aria-label="Тип элементов папки"
            >
              {(
                [
                  ["all", "Все"],
                  ["conference", "Встречи"],
                  ["conversation", "Чаты"],
                ] as const
              ).map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={type === value}
                  onClick={() => {
                    const next = new URLSearchParams(params);
                    if (value === "all") next.delete("type");
                    else next.set("type", value);
                    setParams(next);
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
            <label className="folder-detail-search">
              Поиск в папке
              <input
                value={text}
                placeholder="Название или собеседник"
                onChange={(event) => setText(event.target.value)}
              />
            </label>
          </div>
          <ErrorNotice error={entries.error || remove.error} />
          {entries.isError && !denied && (
            <Button variant="outline" onClick={() => void entries.refetch()}>
              Повторить загрузку
            </Button>
          )}
          {entries.isPending && <Loading />}
          {(["conference", "conversation"] as const).map((kind) => {
            const section = items.filter((entry) => entry.type === kind);
            return (
              section.length > 0 && (
                <section
                  className="folder-item-section"
                  key={kind}
                  aria-label={
                    kind === "conference" ? "Встречи в папке" : "Чаты в папке"
                  }
                >
                  <h2>{kind === "conference" ? "Встречи" : "Чаты"}</h2>
                  <div className="folder-list">
                    {section.map((entry) => (
                      <div
                        className="folder-row"
                        key={`${entry.type}:${entry.item.id}`}
                      >
                        <Link
                          className="folder-link"
                          to={folderItemHref(entry)}
                        >
                          <FolderItemLabel entry={entry} />
                        </Link>
                        <ItemActions
                          label={`Действия с элементом: ${folderItemName(entry)}`}
                          actions={[
                            {
                              label: "Добавить в папку",
                              icon: <FolderPlus size={18} aria-hidden="true" />,
                              run: () => setMapping(entry),
                              disabled: remove.isPending,
                            },
                            {
                              label: "Удалить из папки",
                              icon: <Trash2 size={18} aria-hidden="true" />,
                              danger: true,
                              disabled: remove.isPending,
                              run: () => {
                                if (gate.current) return;
                                gate.current = true;
                                remove.mutate(entry);
                              },
                            },
                          ]}
                        />
                      </div>
                    ))}
                  </div>
                </section>
              )
            );
          })}
          {!entries.isPending && !entries.isError && !items.length && (
            <div className="folder-empty">
              <Folder size={48} aria-hidden="true" />
              <h2>
                {params.get("search")
                  ? "Ничего не найдено"
                  : "В этой папке пока ничего нет"}
              </h2>
              <p>
                {params.get("search")
                  ? "Попробуйте другое название."
                  : "Добавьте чаты или встречи с помощью кнопки выше."}
              </p>
            </div>
          )}
          {entries.hasNextPage && (
            <Button
              variant="outline"
              busy={entries.isFetchingNextPage}
              onClick={() => void entries.fetchNextPage()}
            >
              Ещё элементы
            </Button>
          )}
        </>
      )}
      {manage && (
        <FolderManageModal
          {...manage}
          onClose={() => setManage(null)}
          onDeleted={unavailable}
        />
      )}
      {adding && folder && (
        <FolderItemPicker
          folderId={id}
          onClose={() => setAdding(false)}
          onUnavailable={unavailable}
        />
      )}
      {mapping && folder && (
        <FolderPicker
          target={folderItemTarget(mapping)}
          onClose={() => setMapping(null)}
        />
      )}
    </section>
  );
}
