import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, personalChatAPI } from "../api";
import { updateDirectConversation } from "../directConversations";
import type { ChatMessage, DirectConversation } from "../types";
import { PersonalPage } from "../pages/PersonalPage";
import { ConversationActions } from "./FolderPicker";
import { DirectConversationActions } from "./DirectConversationActions";

const authIdentity = vi.hoisted(() => ({ id: "alice" }));
vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: authIdentity.id, displayName: "Участник" } }),
}));
const initial: DirectConversation = {
  id: "direct",
  type: "direct",
  peer: { id: "bob", displayName: "Борис" },
  createdAt: "2026-10-08T10:00:00Z",
  lastMessageAt: "2026-10-08T10:00:00Z",
  lastMessageId: "message",
  preview: "Прежняя переписка",
  unreadCount: 2,
};
const message: ChatMessage = {
  id: "message",
  sequence: "9",
  conversationId: "direct",
  senderId: "bob",
  senderName: "Борис",
  text: "Прежняя переписка",
  createdAt: "2026-10-08T10:00:00Z",
  updatedAt: "2026-10-08T10:00:00Z",
  deletedAt: null,
  version: 1,
  attachments: [],
};
const clients: QueryClient[] = [];
let current: DirectConversation;
function Location() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}
function Actions() {
  const query = useQuery({
    queryKey: ["personal-detail", "direct", "alice"],
    queryFn: () => api.personalConversation("direct"),
    initialData: { status: "success", item: initial },
  });
  return <ConversationActions conversation={query.data.item} />;
}
function show(page = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  client.setQueryData(["personal-list", "alice", {}], {
    pages: [{ items: [initial], unreadCount: 2 }],
    pageParams: [undefined],
  });
  client.setQueryData(["folder-items", "alice", "folder", {}], {
    pages: [{ items: [{ type: "conversation", item: initial }] }],
    pageParams: [undefined],
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/personal/direct"]}>
        <Location />
        {page ? (
          <Routes>
            <Route path="/personal/:id?" element={<PersonalPage />} />
          </Routes>
        ) : (
          <Actions />
        )}
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return client;
}
async function openMenu(header = false) {
  const root = header
    ? document.querySelector(".personal-header")!
    : document.body;
  fireEvent.click(
    within(root as HTMLElement).getByRole("button", {
      name: "Действия с перепиской: Борис",
    }),
  );
  return await screen.findByRole("menu", {
    name: "Действия с перепиской: Борис",
  });
}
beforeEach(() => {
  authIdentity.id = "alice";
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  current = { ...initial };
  vi.spyOn(api, "personalConversation").mockImplementation(async () => ({
    status: "success",
    item: current,
  }));
  vi.spyOn(api, "personalConversations").mockImplementation(async () => ({
    status: "success",
    items: [current],
    unreadCount: current.unreadCount,
    nextCursor: null,
  }));
  vi.spyOn(api, "folders").mockResolvedValue({ status: "success", items: [] });
  vi.spyOn(api, "setPersonalConversationNotifications").mockImplementation(
    async (_id, enabled) => {
      current = { ...current, notificationsEnabled: enabled };
      return { status: "success", item: current };
    },
  );
  vi.spyOn(api, "clearPersonalConversationHistory").mockImplementation(
    async () => {
      current = {
        ...current,
        unreadCount: 0,
        preview: "",
        lastMessageAt: null,
        lastMessageId: null,
        historyClearedThrough: 9,
      };
      return { status: "success", item: current };
    },
  );
  vi.spyOn(api, "hidePersonalConversation").mockResolvedValue({
    status: "success",
    hidden: true,
    historyClearedThrough: 10,
  });
  vi.spyOn(personalChatAPI, "messages").mockImplementation(async () => ({
    status: "success",
    items: current.historyClearedThrough ? [] : [message],
    unreadCount: current.unreadCount,
    nextCursor: null,
    lastReadMessageId: null,
  }));
  vi.spyOn(personalChatAPI, "chatRead").mockResolvedValue({
    status: "success",
    item: { unreadCount: 0, lastReadMessageId: null },
  });
  vi.spyOn(personalChatAPI, "markChatRead").mockResolvedValue({
    status: "success",
    item: { unreadCount: 0, lastReadMessageId: "message" },
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
describe("действия личной переписки", () => {
  it.each(["mute", "clear", "hide"] as const)(
    "игнорирует ответ %s прежней учётной записи после переключения без удаления компонента",
    async (action) => {
      let finish!: () => void;
      const leaked = {
        ...initial,
        preview: "Секрет старой учётной записи",
        historyClearedThrough: 9,
      };
      if (action === "mute")
        vi.mocked(api.setPersonalConversationNotifications).mockImplementation(
          () =>
            new Promise((resolve) => {
              finish = () => resolve({ status: "success", item: leaked });
            }),
        );
      if (action === "clear")
        vi.mocked(api.clearPersonalConversationHistory).mockImplementation(
          () =>
            new Promise((resolve) => {
              finish = () => resolve({ status: "success", item: leaked });
            }),
        );
      if (action === "hide")
        vi.mocked(api.hidePersonalConversation).mockImplementation(
          () =>
            new Promise((resolve) => {
              finish = () =>
                resolve({
                  status: "success",
                  hidden: true,
                  historyClearedThrough: 9,
                });
            }),
        );
      const client = new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      });
      clients.push(client);
      const retained = {
        status: "success",
        item: { ...initial, preview: "История новой учётной записи" },
      };
      client.setQueryData(["personal-detail", "direct", "charlie"], retained);
      client.setQueryData(["personal-thread-version", "direct", "charlie"], 7);
      const shell = () => (
        <QueryClientProvider client={client}>
          <MemoryRouter initialEntries={["/personal/direct"]}>
            <Location />
            <DirectConversationActions conversation={initial} />
          </MemoryRouter>
        </QueryClientProvider>
      );
      const view = render(shell());
      const labels = {
        mute: "Без уведомлений",
        clear: "Очистить историю",
        hide: "Удалить чат",
      };
      fireEvent.click(
        within(await openMenu()).getByRole("menuitem", {
          name: labels[action],
        }),
      );
      if (action !== "mute")
        fireEvent.click(
          within(await screen.findByRole("dialog")).getByRole("button", {
            name: labels[action],
          }),
        );
      await waitFor(() => expect(finish).toBeTypeOf("function"));
      const signal =
        action === "mute"
          ? vi.mocked(api.setPersonalConversationNotifications).mock.calls[0][2]
          : action === "clear"
            ? vi.mocked(api.clearPersonalConversationHistory).mock.calls[0][1]
            : vi.mocked(api.hidePersonalConversation).mock.calls[0][1];
      authIdentity.id = "charlie";
      view.rerender(shell());
      await waitFor(() => expect(signal?.aborted).toBe(true));
      await act(async () => finish());
      expect(
        client.getQueryData(["personal-detail", "direct", "charlie"]),
      ).toEqual(retained);
      expect(
        client.getQueryData(["personal-thread-version", "direct", "charlie"]),
      ).toBe(7);
      expect(
        client.getQueryData(["personal-history-cutoff", "direct", "charlie"]),
      ).toBeUndefined();
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/personal/direct",
      );
    },
  );
  it("отказ прежней операции не очищает кеш и не меняет страницу новой учётной записи", async () => {
    let reject!: (error: Error) => void;
    vi.mocked(api.clearPersonalConversationHistory).mockImplementation(
      () =>
        new Promise((_resolve, fail) => {
          reject = fail;
        }),
    );
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    clients.push(client);
    const retained = { status: "success", item: initial };
    client.setQueryData(["personal-detail", "direct", "charlie"], retained);
    const shell = () => (
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/personal/direct"]}>
          <Location />
          <DirectConversationActions conversation={initial} />
        </MemoryRouter>
      </QueryClientProvider>
    );
    const view = render(shell());
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    fireEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Очистить историю",
      }),
    );
    await waitFor(() => expect(reject).toBeTypeOf("function"));
    authIdentity.id = "charlie";
    view.rerender(shell());
    await act(async () =>
      reject(new ApiError(403, "Отказ для старой учётной записи")),
    );
    expect(
      client.getQueryData(["personal-detail", "direct", "charlie"]),
    ).toEqual(retained);
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/personal/direct",
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });
  it("завершение прежней операции не снимает блокировку нового подтверждения", async () => {
    let first!: () => void;
    let second!: () => void;
    vi.mocked(api.clearPersonalConversationHistory)
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            first = () =>
              resolve({
                status: "success",
                item: { ...initial, historyClearedThrough: 9 },
              });
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            second = () =>
              resolve({
                status: "success",
                item: { ...initial, historyClearedThrough: 10 },
              });
          }),
      );
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    clients.push(client);
    const shell = () => (
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <DirectConversationActions conversation={initial} />
        </MemoryRouter>
      </QueryClientProvider>
    );
    const view = render(shell());
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    fireEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Очистить историю",
      }),
    );
    await waitFor(() => expect(first).toBeTypeOf("function"));
    authIdentity.id = "charlie";
    view.rerender(shell());
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Очистить историю" }),
    );
    await waitFor(() => expect(second).toBeTypeOf("function"));
    await act(async () => first());
    fireEvent.keyDown(document, { key: "Escape" });
    expect(dialog).toBeInTheDocument();
    await act(async () => second());
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
  it("сохраняет добавление в папку и показывает все новые пункты", async () => {
    show();
    const menu = await openMenu();
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Информация",
      "Без уведомлений",
      "Добавить в папку",
      "Очистить историю",
      "Удалить чат",
    ]);
    fireEvent.click(
      within(menu).getByRole("menuitem", { name: "Добавить в папку" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Добавить в папку" }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("У вас пока нет папок."),
    ).toBeInTheDocument();
  });
  it("открывает одно стандартное окно через меню и восстанавливает фокус", async () => {
    show();
    const trigger = screen.getByRole("button", {
      name: "Действия с перепиской: Борис",
    });
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    const menu = await screen.findByRole("menu");
    await waitFor(() =>
      expect(
        within(menu).getByRole("menuitem", { name: "Информация" }),
      ).toHaveFocus(),
    );
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Информация" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Информация о пользователе",
    });
    expect(
      within(dialog).getByText("Идентификатор пользователя"),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("bob")).toBeInTheDocument();
    expect(within(dialog).queryByText("В сети")).toBeNull();
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(trigger).toHaveFocus());
  });
  it("открывает информацию при нажатии на имя или аватар в заголовке", async () => {
    show(true);
    const trigger = await screen.findByRole("button", {
      name: "Информация о пользователе: Борис",
    });
    fireEvent.click(trigger.querySelector(".personal-avatar")!);
    const dialog = await screen.findByRole("dialog", {
      name: "Информация о пользователе",
    });
    expect(
      within(dialog).getByRole("heading", { name: "Борис" }),
    ).toBeInTheDocument();
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Закрыть окно" }),
    );
    await waitFor(() => expect(trigger).toHaveFocus());
    fireEvent.click(within(trigger).getByText("Борис"));
    expect(
      await screen.findByRole("dialog", { name: "Информация о пользователе" }),
    ).toBeInTheDocument();
  });
  it("сохраняет отключение и включение уведомлений в API, списке и папках", async () => {
    const client = show();
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Без уведомлений",
      }),
    );
    await waitFor(() =>
      expect(api.setPersonalConversationNotifications).toHaveBeenCalledWith(
        "direct",
        false,
        expect.any(AbortSignal),
      ),
    );
    await waitFor(() =>
      expect(
        client.getQueryData<{ item: DirectConversation }>([
          "personal-detail",
          "direct",
          "alice",
        ])?.item.notificationsEnabled,
      ).toBe(false),
    );
    const menu = await openMenu();
    fireEvent.click(
      within(menu).getByRole("menuitem", { name: "Включить уведомления" }),
    );
    await waitFor(() =>
      expect(api.setPersonalConversationNotifications).toHaveBeenLastCalledWith(
        "direct",
        true,
        expect.any(AbortSignal),
      ),
    );
  });
  it("не очищает историю без подтверждения и убирает историю, ответ и черновик после очистки", async () => {
    const client = show(true);
    await screen.findByRole("button", {
      name: "Информация о пользователе: Борис",
    });
    await screen.findByTestId("chat-message-message");
    fireEvent.change(screen.getByRole("textbox", { name: "Сообщение" }), {
      target: { value: "Черновик" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Ответить" }));
    fireEvent.click(
      within(await openMenu(true)).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Очистить историю?",
    });
    expect(api.clearPersonalConversationHistory).not.toHaveBeenCalled();
    expect(within(dialog).getByText(/только у вас/)).toBeInTheDocument();
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Очистить историю" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("textbox", { name: "Сообщение" })).toHaveValue("");
    expect(screen.queryByTestId("chat-message-message")).toBeNull();
    expect(screen.queryByRole("button", { name: "Отменить ответ" })).toBeNull();
    expect(
      client.getQueryData(["personal-thread-version", "direct", "alice"]),
    ).toBe(1);
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/personal/direct",
    );
  });
  it("показывает ошибку очистки и сохраняет переписку до успешного ответа", async () => {
    vi.mocked(api.clearPersonalConversationHistory).mockRejectedValueOnce(
      new ApiError(503, "Временно недоступно"),
    );
    show();
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Очистить историю" }),
    );
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Временно недоступно",
    );
    expect(dialog).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
  it("освобождает уже выданную локальную ссылку вложения после очистки", async () => {
    const file = {
      id: "file",
      filename: "old.pdf",
      mimeType: "application/pdf",
      size: 3,
      status: "ready",
    };
    vi.mocked(personalChatAPI.messages).mockResolvedValueOnce({
      status: "success",
      items: [{ ...message, attachments: [file] }],
      unreadCount: 0,
      lastReadMessageId: null,
      nextCursor: null,
    });
    vi.spyOn(personalChatAPI, "attachmentDownload").mockResolvedValue({
      url: "/api/v1/conversations/direct/attachments/file/content",
      authenticated: true,
      expiresAt: "2099-01-01T00:00:00Z",
    });
    vi.spyOn(personalChatAPI, "attachmentContent").mockResolvedValue(
      new Blob(["pdf"], { type: "application/pdf" }),
    );
    vi.stubGlobal(
      "URL",
      class extends URL {
        static createObjectURL = vi.fn(() => "blob:old-file");
        static revokeObjectURL = vi.fn();
      },
    );
    show(true);
    fireEvent.click(
      await screen.findByRole("button", { name: "Получить ссылку" }),
    );
    expect(
      await screen.findByRole("link", { name: "Скачать файл" }),
    ).toHaveAttribute("href", "blob:old-file");
    fireEvent.click(
      within(await openMenu(true)).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Очистить историю" }),
    );
    await waitFor(() =>
      expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:old-file"),
    );
    expect(screen.queryByRole("link", { name: "Скачать файл" })).toBeNull();
  });
  it("не возвращает загрузку файла, если её инициализация завершилась после очистки", async () => {
    const file = {
      id: "upload",
      filename: "new.txt",
      mimeType: "text/plain",
      size: 3,
      status: "pending",
    };
    let finish!: (value: {
      status: string;
      item: typeof file;
      uploadUrl: string;
    }) => void;
    vi.spyOn(personalChatAPI, "initAttachment").mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const finalize = vi.spyOn(personalChatAPI, "finalizeAttachment");
    const xhr = vi.fn();
    vi.stubGlobal("XMLHttpRequest", xhr);
    show(true);
    await screen.findByRole("button", {
      name: "Информация о пользователе: Борис",
    });
    fireEvent.change(screen.getByLabelText("Выбрать файлы для сообщения"), {
      target: { files: [new File(["new"], "new.txt", { type: "text/plain" })] },
    });
    await waitFor(() =>
      expect(personalChatAPI.initAttachment).toHaveBeenCalledOnce(),
    );
    fireEvent.click(
      within(await openMenu(true)).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Очистить историю" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await act(async () =>
      finish({
        status: "success",
        item: file,
        uploadUrl: "/api/v1/conversations/direct/attachments/upload/content",
      }),
    );
    expect(xhr).not.toHaveBeenCalled();
    expect(finalize).not.toHaveBeenCalled();
    expect(screen.queryByText("new.txt")).toBeNull();
  });
  it("удаляет только подтверждённый чат из своего списка и возвращает к перепискам", async () => {
    const client = show(true);
    await screen.findByRole("button", {
      name: "Информация о пользователе: Борис",
    });
    vi.mocked(api.personalConversations).mockResolvedValue({
      status: "success",
      items: [],
      unreadCount: 0,
      nextCursor: null,
    });
    vi.mocked(api.personalConversation).mockRejectedValue(
      new ApiError(404, "Чат скрыт"),
    );
    fireEvent.click(
      within(await openMenu(true)).getByRole("menuitem", {
        name: "Удалить чат",
      }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Удалить чат?" });
    expect(api.hidePersonalConversation).not.toHaveBeenCalled();
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Удалить чат" }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(/^\/personal$/),
    );
    expect(
      client.getQueryData(["personal-detail", "direct", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-chat", "direct", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-history-cutoff", "direct", "alice"]),
    ).toBe(10);
    expect(screen.queryByText("Борис")).toBeNull();
  });
  it("защищает подтверждение от повторных кликов и закрытия во время записи", async () => {
    let finish!: (value: { status: string; item: DirectConversation }) => void;
    vi.mocked(api.clearPersonalConversationHistory).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    show();
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", {
      name: "Очистить историю",
    });
    act(() => {
      fireEvent.click(confirm);
      fireEvent.click(confirm);
    });
    await waitFor(() =>
      expect(api.clearPersonalConversationHistory).toHaveBeenCalledOnce(),
    );
    await waitFor(() => expect(confirm).toBeDisabled());
    fireEvent.keyDown(document, { key: "Escape" });
    expect(dialog).toBeInTheDocument();
    act(() =>
      finish({
        status: "success",
        item: {
          ...initial,
          historyClearedThrough: 9,
          unreadCount: 0,
          preview: "",
        },
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
  it("не восстанавливает прежнюю историю при позднем HTTP-ответе после более новой очистки в другой вкладке", async () => {
    let finish!: (value: { status: string; item: DirectConversation }) => void;
    vi.mocked(api.clearPersonalConversationHistory).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const client = show();
    fireEvent.click(
      within(await openMenu()).getByRole("menuitem", {
        name: "Очистить историю",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Очистить историю" }),
    );
    await waitFor(() =>
      expect(api.clearPersonalConversationHistory).toHaveBeenCalledOnce(),
    );
    current = {
      ...initial,
      historyClearedThrough: 20,
      unreadCount: 0,
      preview: "",
    };
    act(() => updateDirectConversation(client, "alice", current));
    act(() =>
      finish({
        status: "success",
        item: {
          ...initial,
          historyClearedThrough: 9,
          unreadCount: 1,
          preview: "Позднее прежнее сообщение",
        },
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      client.getQueryData<{ item: DirectConversation }>([
        "personal-detail",
        "direct",
        "alice",
      ])?.item.historyClearedThrough,
    ).toBe(20);
    expect(
      client.getQueryData<{ item: DirectConversation }>([
        "personal-detail",
        "direct",
        "alice",
      ])?.item.preview,
    ).toBe("");
    expect(
      client.getQueryData(["personal-thread-version", "direct", "alice"]),
    ).toBe(1);
  });
});
