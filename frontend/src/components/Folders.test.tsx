import { useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type {
  Conference,
  FolderItem,
  PersonalConversation,
  PersonalFolder,
} from "../types";
import { FolderManageModal } from "./FolderModals";
import { FolderPicker } from "./FolderPicker";
import { FolderItemPicker } from "./FolderItems";
import { FoldersPage } from "../pages/FoldersPage";
import { ItemActions } from "./ItemActions";
const authState = vi.hoisted(() => ({ id: "alice" }));
vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: authState.id, displayName: "Алиса" } }),
}));
const initial: PersonalFolder = {
  id: "work",
  name: "Работа",
  position: 0,
  createdAt: "2026-10-07T10:00:00Z",
  updatedAt: "2026-10-07T10:00:00Z",
  itemCount: 0,
  conversationCount: 0,
  conferenceCount: 0,
};
const direct: PersonalConversation = {
  id: "same",
  type: "direct",
  peer: { id: "bob", displayName: "Борис" },
  createdAt: initial.createdAt,
  lastMessageAt: null,
  lastMessageId: null,
  preview: "Доступная переписка",
  unreadCount: 0,
};
const meeting: Conference = {
  id: "same",
  ownerId: "alice",
  title: "План встречи",
  inviteCode: "",
  inviteUrl: "",
  status: "finished",
  createdAt: initial.createdAt,
  updatedAt: initial.createdAt,
  startedAt: null,
  finishedAt: initial.createdAt,
};
const entries: FolderItem[] = [
  { type: "conversation", item: direct },
  { type: "conference", item: meeting },
];
const clients: QueryClient[] = [];
function Location() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}
function show(element: React.ReactNode, path = "/folders") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const result = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Location />
        {element}
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { client, ...result };
}
function page(path = "/folders") {
  return show(
    <Routes>
      <Route path="/folders" element={<FoldersPage />} />
      <Route path="/folders/:id" element={<FoldersPage />} />
    </Routes>,
    path,
  );
}
beforeEach(() => {
  authState.id = "alice";
  vi.spyOn(api, "folders").mockResolvedValue({
    status: "success",
    items: [initial],
  });
  vi.spyOn(api, "folder").mockResolvedValue({
    status: "success",
    item: initial,
  });
  vi.spyOn(api, "createFolder").mockResolvedValue({
    status: "success",
    item: initial,
  });
  vi.spyOn(api, "renameFolder").mockResolvedValue({
    status: "success",
    item: initial,
  });
  vi.spyOn(api, "deleteFolder").mockResolvedValue({ status: "success" });
  vi.spyOn(api, "orderFolders").mockResolvedValue({
    status: "success",
    items: [initial],
  });
  vi.spyOn(api, "folderItems").mockResolvedValue({ items: entries });
  vi.spyOn(api, "folderCandidates").mockResolvedValue({ items: entries });
  vi.spyOn(api, "setFolderItem").mockResolvedValue({
    status: "success",
    item: { ...initial, itemCount: 1 },
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});
describe("folder UI", () => {
  it("normalizes the name and blocks an immediate second submit and Escape while saving", async () => {
    let resolve!: (value: Awaited<ReturnType<typeof api.createFolder>>) => void;
    vi.mocked(api.createFolder).mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const close = vi.fn();
    show(<FolderManageModal mode="create" onClose={close} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Название папки" }), {
      target: { value: "  Cafe\u0301  " },
    });
    const form = screen
      .getByRole("button", { name: "Создать" })
      .closest("form")!;
    act(() => {
      fireEvent.submit(form);
      fireEvent.submit(form);
    });
    await waitFor(() => expect(api.createFolder).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.createFolder).mock.calls[0][0]).toBe("Café");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(close).not.toHaveBeenCalled();
    await act(async () => resolve({ status: "success", item: initial }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
  });
  it("rejects empty, oversized and bidi names without a network write, and retains input after a duplicate response", async () => {
    vi.mocked(api.createFolder).mockRejectedValue(
      new ApiError(409, "Папка с таким названием уже существует"),
    );
    show(<FolderManageModal mode="create" onClose={vi.fn()} />);
    const input = screen.getByRole("textbox", { name: "Название папки" });
    for (const name of ["  ", "x".repeat(51), "Работа\u202e"]) {
      fireEvent.change(input, { target: { value: name } });
      fireEvent.click(screen.getByRole("button", { name: "Создать" }));
      expect(screen.getByRole("alert")).toHaveTextContent("от 1 до 50");
    }
    expect(api.createFolder).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: "Работа" } });
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    await screen.findByText("Папка с таким названием уже существует");
    expect(input).toHaveValue("Работа");
  });
  it("saves independent checkboxes and rolls a failed mapping back without altering other folders", async () => {
    const second = {
      ...initial,
      id: "personal",
      name: "Личное",
      contains: true,
    };
    let selected = false;
    vi.mocked(api.folders).mockImplementation(async () => ({
      status: "success",
      items: [{ ...initial, contains: selected }, second],
    }));
    vi.mocked(api.setFolderItem)
      .mockImplementationOnce(async () => {
        selected = true;
        return { status: "success", item: { ...initial, itemCount: 1 } };
      })
      .mockRejectedValueOnce(new Error("network"));
    show(
      <FolderPicker
        target={{ type: "conversation", id: "chat" }}
        onClose={vi.fn()}
      />,
    );
    const work = await screen.findByRole("checkbox", { name: /Работа/ });
    expect(screen.getByRole("checkbox", { name: /Личное/ })).toBeChecked();
    fireEvent.click(work);
    await waitFor(() => expect(work).not.toBeDisabled());
    expect(work).toBeChecked();
    expect(api.setFolderItem).toHaveBeenNthCalledWith(
      1,
      "work",
      { type: "conversation", id: "chat" },
      true,
      expect.any(AbortSignal),
    );
    fireEvent.click(work);
    await screen.findByRole("alert");
    expect(work).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Личное/ })).toBeChecked();
  });
  it("creates once inside the picker and retries a failed add without creating a duplicate folder", async () => {
    let created = false;
    vi.mocked(api.folders).mockImplementation(async () => ({
      status: "success",
      items: created ? [initial] : [],
    }));
    vi.mocked(api.createFolder).mockImplementation(async () => {
      created = true;
      return { status: "success", item: initial };
    });
    vi.mocked(api.setFolderItem)
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({ status: "success", item: initial });
    show(
      <FolderPicker
        target={{ type: "conference", id: "meeting" }}
        onClose={vi.fn()}
      />,
    );
    await screen.findByText("У вас пока нет папок.");
    fireEvent.click(screen.getByRole("button", { name: "Новая папка" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Название папки" }), {
      target: { value: "Работа" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    await screen.findByRole("alert");
    const checkbox = await screen.findByRole("checkbox", { name: /Работа/ });
    expect(checkbox).not.toBeChecked();
    fireEvent.click(checkbox);
    await waitFor(() => expect(api.setFolderItem).toHaveBeenCalledTimes(2));
    expect(api.createFolder).toHaveBeenCalledTimes(1);
  });
  it("aborts a mapping when its owning picker unmounts", async () => {
    vi.mocked(api.setFolderItem).mockImplementation(
      () => new Promise(() => {}),
    );
    const view = show(
      <FolderPicker
        target={{ type: "conversation", id: "chat" }}
        onClose={vi.fn()}
      />,
    );
    fireEvent.click(await screen.findByRole("checkbox", { name: /Работа/ }));
    await waitFor(() => expect(api.setFolderItem).toHaveBeenCalledOnce());
    const signal = vi.mocked(api.setFolderItem).mock.calls[0][3]!;
    view.unmount();
    expect(signal.aborted).toBe(true);
  });
  it("separates two item kinds with the same UUID and links to the existing chat/history pages", async () => {
    page("/folders/work");
    expect(await screen.findByRole("link", { name: /Борис/ })).toHaveAttribute(
      "href",
      "/personal/same",
    );
    expect(screen.getByRole("link", { name: /План встречи/ })).toHaveAttribute(
      "href",
      "/history/same",
    );
    expect(
      screen.getAllByRole("button", { name: /Действия с элементом/ }),
    ).toHaveLength(2);
    fireEvent.click(
      screen.getByRole("button", { name: "Действия с элементом: Борис" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Удалить из папки" }));
    await waitFor(() =>
      expect(api.setFolderItem).toHaveBeenCalledWith(
        "work",
        { type: "conversation", id: "same" },
        false,
        expect.any(AbortSignal),
      ),
    );
  });
  it("sends the complete folder order and deletes only folder mappings", async () => {
    const second = { ...initial, id: "personal", name: "Личное", position: 1 };
    vi.mocked(api.folders).mockResolvedValue({
      status: "success",
      items: [initial, second],
    });
    page();
    fireEvent.click(
      await screen.findByRole("button", { name: "Действия с папкой: Личное" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Переместить выше" }));
    await waitFor(() =>
      expect(api.orderFolders).toHaveBeenCalledWith(
        ["personal", "work"],
        expect.any(AbortSignal),
      ),
    );
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    fireEvent.click(
      screen.getByRole("button", { name: "Действия с папкой: Работа" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Удалить папку" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("Чаты и встречи сохранятся");
    fireEvent.click(
      within(dialog).getByRole("button", {
        name: "Удалить папку",
      }),
    );
    await waitFor(() =>
      expect(api.deleteFolder).toHaveBeenCalledWith(
        "work",
        expect.any(AbortSignal),
      ),
    );
    expect(api.setFolderItem).not.toHaveBeenCalled();
  });
  it("uses the candidate cursor only within its server filter and preserves membership checkboxes", async () => {
    vi.mocked(api.folderCandidates).mockImplementation(
      async (_, filters, before) =>
        filters.type === "all"
          ? {
              items: [{ ...entries[0], inFolder: true }],
              nextCursor: before ? undefined : "bound-cursor",
            }
          : { items: [entries[1]] },
    );
    show(
      <FolderItemPicker
        folderId="work"
        onClose={vi.fn()}
        onUnavailable={vi.fn()}
      />,
    );
    expect(
      await screen.findByRole("checkbox", { name: "Борис" }),
    ).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Ещё элементы" }));
    await waitFor(() =>
      expect(api.folderCandidates).toHaveBeenCalledWith(
        "work",
        { type: "all", search: "" },
        "bound-cursor",
        expect.any(AbortSignal),
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Встречи" }));
    await screen.findByRole("checkbox", { name: "План встречи" });
    expect(api.folderCandidates).toHaveBeenLastCalledWith(
      "work",
      { type: "conference", search: "" },
      undefined,
      expect.any(AbortSignal),
    );
  });
  it("revokes cached folder metadata and rows immediately after an authoritative denial", async () => {
    const { client } = page("/folders/work");
    await screen.findByRole("link", { name: /Борис/ });
    vi.mocked(api.folder).mockRejectedValue(new ApiError(403, "Доступ закрыт"));
    await act(async () => {
      await client.refetchQueries({ queryKey: ["folder", "alice", "work"] });
    });
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(/^\/folders$/),
    );
    expect(screen.queryByText("Доступная переписка")).toBeNull();
    expect(
      client.getQueryData([
        "folder-items",
        "alice",
        "work",
        { type: "all", search: "" },
      ]),
    ).toBeUndefined();
  });
  it("purges a denied target in an already cached folder picker", async () => {
    const close = vi.fn();
    const { client } = show(
      <FolderPicker
        target={{ type: "conversation", id: "chat" }}
        onClose={close}
      />,
    );
    await screen.findByRole("checkbox", { name: /Работа/ });
    vi.mocked(api.folders).mockRejectedValue(new ApiError(403, "Нет доступа"));
    await act(async () => {
      await client.refetchQueries({ queryKey: ["folders", "alice"] });
    });
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(screen.queryByRole("checkbox")).toBeNull();
  });
  it("supports keyboard menu navigation and restores focus after Escape", async () => {
    show(
      <ItemActions
        label="Действия"
        actions={[
          { label: "Первое", run: vi.fn() },
          { label: "Второе", run: vi.fn() },
        ]}
      />,
    );
    const button = screen.getByRole("button", { name: "Действия" });
    fireEvent.keyDown(button, { key: "ArrowDown" });
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "Первое" })).toHaveFocus(),
    );
    fireEvent.keyDown(document.activeElement!, { key: "End" });
    expect(screen.getByRole("menuitem", { name: "Второе" })).toHaveFocus();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(button).toHaveFocus();
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
describe("folder denial and account lifecycle", () => {
  it("purges a denied candidate preview even if its recovery refetch cannot connect", async () => {
    vi.mocked(api.setFolderItem).mockRejectedValue(
      new ApiError(403, "Доступ закрыт"),
    );
    const { client } = show(
      <FolderItemPicker
        folderId="work"
        onClose={vi.fn()}
        onUnavailable={vi.fn()}
      />,
    );
    await screen.findByRole("checkbox", { name: "Борис" });
    vi.mocked(api.folderCandidates).mockRejectedValue(
      new Error("offline recovery"),
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "Борис" }));
    await waitFor(() =>
      expect(screen.queryByRole("checkbox", { name: "Борис" })).toBeNull(),
    );
    expect(
      screen.getByRole("checkbox", { name: "План встречи" }),
    ).toBeInTheDocument();
    expect(
      JSON.stringify(
        client.getQueryData([
          "folder-candidates",
          "alice",
          "work",
          { type: "all", search: "" },
        ]),
      ),
    ).not.toContain("Доступная переписка");
  });
  it("treats candidate mapping404 as a deleted folder without retaining its open picker", async () => {
    vi.mocked(api.setFolderItem).mockRejectedValue(
      new ApiError(404, "Папка не найдена"),
    );
    const unavailable = vi.fn();
    show(
      <FolderItemPicker
        folderId="work"
        onClose={vi.fn()}
        onUnavailable={unavailable}
      />,
    );
    fireEvent.click(await screen.findByRole("checkbox", { name: "Борис" }));
    await waitFor(() => expect(unavailable).toHaveBeenCalledOnce());
  });
  it("clears a selected folder after DELETE mapping404 rather than waiting for the next poll", async () => {
    vi.mocked(api.setFolderItem).mockRejectedValue(
      new ApiError(404, "Папка не найдена"),
    );
    page("/folders/work");
    fireEvent.click(
      await screen.findByRole("button", {
        name: "Действия с элементом: Борис",
      }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Удалить из папки" }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(/^\/folders$/),
    );
    expect(screen.queryByText("Доступная переписка")).toBeNull();
  });
  it("closes the old account's rename dialog and aborts its write on an in-place account switch", async () => {
    let resolve!: (
      result: Awaited<ReturnType<typeof api.renameFolder>>,
    ) => void;
    vi.mocked(api.renameFolder).mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    vi.mocked(api.folders).mockImplementation(async () => ({
      status: "success",
      items:
        authState.id === "alice"
          ? [initial]
          : [{ ...initial, id: "bob-folder", name: "Личное Бориса" }],
    }));
    const tree = (
      <Routes>
        <Route path="/folders" element={<FoldersPage />} />
      </Routes>
    );
    const view = show(tree);
    fireEvent.click(
      await screen.findByRole("button", { name: "Действия с папкой: Работа" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Переименовать" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Название папки" }), {
      target: { value: "Приватное имя" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() => expect(api.renameFolder).toHaveBeenCalledOnce());
    const signal = vi.mocked(api.renameFolder).mock.calls[0][2]!;
    authState.id = "bob";
    view.rerender(
      <QueryClientProvider client={view.client}>
        <MemoryRouter initialEntries={["/folders"]}>
          <Location />
          <Routes>
            <Route path="/folders" element={<FoldersPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await screen.findByText("Личное Бориса");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText("Работа")).toBeNull();
    expect(signal.aborted).toBe(true);
    await act(async () =>
      resolve({
        status: "success",
        item: { ...initial, name: "Приватное имя" },
      }),
    );
    expect(screen.queryByText("Приватное имя")).toBeNull();
  });
  it("resets a menu picker form and aborts its old create request when the actor changes", async () => {
    let resolve!: (
      result: Awaited<ReturnType<typeof api.createFolder>>,
    ) => void;
    vi.mocked(api.createFolder).mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const tree = (
      <FolderPicker
        target={{ type: "conference", id: "meeting" }}
        onClose={vi.fn()}
      />
    );
    const view = show(tree);
    await screen.findByRole("checkbox", { name: /Работа/ });
    fireEvent.click(screen.getByRole("button", { name: "Новая папка" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Название папки" }), {
      target: { value: "Приватная папка" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    await waitFor(() => expect(api.createFolder).toHaveBeenCalledOnce());
    const signal = vi.mocked(api.createFolder).mock.calls[0][1]!;
    authState.id = "bob";
    view.rerender(
      <QueryClientProvider client={view.client}>
        <MemoryRouter>
          <FolderPicker
            target={{ type: "conference", id: "meeting" }}
            onClose={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await screen.findByRole("dialog", { name: "Добавить в папку" });
    expect(
      screen.queryByRole("textbox", { name: "Название папки" }),
    ).toBeNull();
    expect(signal.aborted).toBe(true);
    await act(async () =>
      resolve({
        status: "success",
        item: { ...initial, name: "Приватная папка" },
      }),
    );
    expect(api.setFolderItem).not.toHaveBeenCalled();
    expect(screen.queryByText("Приватная папка")).toBeNull();
  });
});
it("preserves a new search draft when a type-only URL transition completes", async () => {
  vi.mocked(api.folderItems).mockImplementation(async (_, filters) => ({
    items: filters.search ? [entries[0]] : entries,
  }));
  page("/folders/work");
  await screen.findByRole("link", { name: /Борис/ });
  act(() => {
    fireEvent.click(screen.getByRole("button", { name: "Чаты" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Поиск в папке" }), {
      target: { value: "Борис" },
    });
  });
  await waitFor(() =>
    expect(api.folderItems).toHaveBeenCalledWith(
      "work",
      { type: "conversation", search: "Борис" },
      undefined,
      expect.any(AbortSignal),
    ),
  );
  expect(screen.getByRole("textbox", { name: "Поиск в папке" })).toHaveValue(
    "Борис",
  );
});
it.each(["rename", "delete"] as const)(
  "closes a deleted folder's %s dialog and purges metadata immediately on404",
  async (mode) => {
    const onDeleted = vi.fn();
    const method = mode === "rename" ? api.renameFolder : api.deleteFolder;
    vi.mocked(method).mockRejectedValue(new ApiError(404, "Папка не найдена"));
    function Manager() {
      const [open, setOpen] = useState(true);
      return open ? (
        <FolderManageModal
          mode={mode}
          folder={initial}
          onClose={() => setOpen(false)}
          onDeleted={onDeleted}
        />
      ) : (
        <p>Закрыто</p>
      );
    }
    const { client } = show(<Manager />);
    client.setQueryData(["folder", "alice", "work"], { item: initial });
    client.setQueryData(["folder-items", "alice", "work"], {
      pages: [{ items: entries }],
    });
    fireEvent.click(
      screen.getByRole("button", {
        name: mode === "rename" ? "Сохранить" : "Удалить папку",
      }),
    );
    await screen.findByText("Закрыто");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onDeleted).toHaveBeenCalledOnce();
    expect(client.getQueryData(["folder", "alice", "work"])).toBeUndefined();
    expect(
      client.getQueryData(["folder-items", "alice", "work"]),
    ).toBeUndefined();
  },
);
it("retains a confirmed candidate mapping when its follow-up refresh is offline", async () => {
  const { client } = show(
    <FolderItemPicker
      folderId="work"
      onClose={vi.fn()}
      onUnavailable={vi.fn()}
    />,
  );
  await screen.findByRole("checkbox", { name: "Борис" });
  vi.mocked(api.folderCandidates).mockRejectedValue(
    new Error("offline recovery"),
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "Борис" }));
  await waitFor(() =>
    expect(screen.getByRole("checkbox", { name: "Борис" })).not.toBeDisabled(),
  );
  expect(screen.getByRole("checkbox", { name: "Борис" })).toBeChecked();
  expect(
    client.getQueryData<{ pages: { items: FolderItem[] }[] }>([
      "folder-candidates",
      "alice",
      "work",
      { type: "all", search: "" },
    ])?.pages[0].items[0].inFolder,
  ).toBe(true);
});
