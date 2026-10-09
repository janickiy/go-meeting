import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
import { MemoryRouter } from "react-router";
import { api, ApiError } from "../api";
import { CHAT_EMOJIS } from "../emoji";
import type { ChatMessage, Participant } from "../types";
import { ChatPanel, MessageThread } from "./ChatPanel";

vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: "user", displayName: "Александр" } }),
}));

const member = {
  id: "self",
  role: "owner",
  status: "joined",
  admissionState: "admitted",
} as Participant;

const message: ChatMessage = {
  id: "message",
  sequence: "1",
  conferenceId: "room",
  senderId: "user",
  senderName: "Александр",
  text: "Первое сообщение",
  createdAt: "2026-10-01T10:00:00Z",
  updatedAt: "2026-10-01T10:00:00Z",
  deletedAt: null,
  version: 1,
  attachments: [],
};

const clients: QueryClient[] = [];

/**
 * showChat монтирует чат с отдельным кешем, чтобы проверки не влияли друг на друга.
 * @args readOnly — разрешён ли только просмотр истории.
 * @return результат монтирования для проверки структуры панели.
 */
function showChat(
  readOnly = false,
  focusMessageId?: string,
  onLatest?: () => void,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ChatPanel
          conferenceId="room"
          membership={member}
          readOnly={readOnly}
          focusMessageId={focusMessageId}
          onLatest={onLatest}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/**
 * chooseEmoji выбирает смайлик через настоящую палитру, а не вызывает обработчик напрямую.
 * @args label — доступное русское имя кнопки выбранного смайлика.
 */
function chooseEmoji(label = CHAT_EMOJIS[0].label) {
  fireEvent.click(screen.getByRole("button", { name: "Добавить смайлик" }));
  const picker = screen.getByRole("dialog", { name: "Смайлики" });
  fireEvent.click(within(picker).getByRole("button", { name: label }));
}

beforeEach(() => {
  vi.spyOn(api, "messages").mockResolvedValue({
    status: "success",
    items: [message],
    nextCursor: null,
    unreadCount: 0,
    lastReadMessageId: null,
  });
  vi.spyOn(api, "chatRead").mockResolvedValue({
    status: "success",
    item: { lastReadMessageId: null, unreadCount: 0 },
  });
  vi.spyOn(api, "markChatRead").mockResolvedValue({
    status: "success",
    item: { lastReadMessageId: message.id, unreadCount: 0 },
  });
});

afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.restoreAllMocks();
});

describe("обновлённая панель чата", () => {
  it("закрывает cached group history on authoritative access denial without a WebSocket event", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    clients.push(client);
    const denied = vi.fn();
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <MessageThread
            scopeId="group"
            transport={api}
            personal
            onAccessDenied={denied}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await screen.findByText(message.text);
    vi.mocked(api.messages).mockRejectedValue(
      new ApiError(403, "Доступ закрыт"),
    );
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["personal-chat", "group"] });
    });
    await waitFor(() => expect(denied).toHaveBeenCalledOnce());
    expect(screen.queryByText(message.text)).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Сообщение" })).toBeNull();
  });
  it("показывает не менее 40 смайликов и заменяет выделение с UTF-16 курсором", async () => {
    const send = vi.spyOn(api, "sendMessage").mockResolvedValue({
      status: "success",
      item: message,
    });
    showChat();
    await screen.findByText(message.text);
    const textarea = screen.getByLabelText("Сообщение") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "A🙂BC" } });
    textarea.focus();
    textarea.setSelectionRange(3, 4);
    fireEvent.select(textarea);

    fireEvent.click(screen.getByRole("button", { name: "Добавить смайлик" }));
    const picker = screen.getByRole("dialog", { name: "Смайлики" });
    const choices = within(picker).getByRole("group", {
      name: "Выберите смайлик",
    });
    expect(
      within(choices).getAllByRole("button").length,
    ).toBeGreaterThanOrEqual(40);
    fireEvent.click(
      within(choices).getByRole("button", {
        name: CHAT_EMOJIS[0].label,
      }),
    );

    const expected = `A🙂${CHAT_EMOJIS[0].emoji}C`;
    await waitFor(() => {
      expect(textarea).toHaveValue(expected);
      expect(textarea).toHaveFocus();
      expect(textarea.selectionStart).toBe(3 + CHAT_EMOJIS[0].emoji.length);
      expect(textarea.selectionEnd).toBe(textarea.selectionStart);
    });
    expect(screen.queryByRole("dialog", { name: "Смайлики" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Отправить" }));
    await waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        "room",
        expect.objectContaining({ text: expected }),
      ),
    );
  });

  it("сохраняет смайлик и ключ повторной отправки после сетевой ошибки", async () => {
    const send = vi
      .spyOn(api, "sendMessage")
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ status: "success", item: message });
    showChat();
    await screen.findByText(message.text);
    const textarea = screen.getByLabelText("Сообщение") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "Привет " } });
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
    fireEvent.select(textarea);
    chooseEmoji();
    const expected = `Привет ${CHAT_EMOJIS[0].emoji}`;
    await waitFor(() => expect(textarea).toHaveValue(expected));
    fireEvent.click(screen.getByRole("button", { name: "Отправить" }));
    const retry = await screen.findByRole("button", {
      name: "Повторить отправку",
    });
    expect(textarea).toHaveValue(expected);
    fireEvent.click(retry);
    await waitFor(() => expect(send).toHaveBeenCalledTimes(2));
    expect(send.mock.calls[0][1].text).toBe(expected);
    expect(send.mock.calls[1][1]).toEqual(send.mock.calls[0][1]);
    expect(send.mock.calls[0][1].clientRequestId).toBeTruthy();
    await waitFor(() => expect(textarea).toHaveValue(""));
  });

  it("блокирует черновик, смайлики и вложения до завершения отправки", async () => {
    let complete!: (
      result: Awaited<ReturnType<typeof api.sendMessage>>,
    ) => void;
    const pending = new Promise<Awaited<ReturnType<typeof api.sendMessage>>>(
      (resolve) => {
        complete = resolve;
      },
    );
    const send = vi.spyOn(api, "sendMessage").mockReturnValue(pending);
    showChat();
    await screen.findByText(message.text);
    const textarea = screen.getByLabelText("Сообщение");
    fireEvent.change(textarea, { target: { value: "Отправляю 🙂" } });
    fireEvent.click(screen.getByRole("button", { name: "Отправить" }));
    await waitFor(() => expect(send).toHaveBeenCalledTimes(1));
    await waitFor(() => {
      expect(textarea).toBeDisabled();
      expect(
        screen.getByRole("button", { name: "Добавить смайлик" }),
      ).toBeDisabled();
      expect(
        screen.getByRole("button", { name: "Прикрепить файл" }),
      ).toBeDisabled();
      expect(
        screen.getByLabelText("Выбрать файлы для сообщения"),
      ).toBeDisabled();
    });
    complete({ status: "success", item: message });
    await waitFor(() => expect(textarea).toHaveValue(""));
    expect(textarea).toBeEnabled();
  });

  it("показывает pending без выдуманного sequence и сверяет раннее realtime подтверждение", async () => {
    const page = {
      status: "success" as const,
      items: [] as ChatMessage[],
      nextCursor: null,
      unreadCount: 0,
      lastReadMessageId: null,
    };
    vi.mocked(api.messages).mockResolvedValue(page);
    let resolve!: (result: Awaited<ReturnType<typeof api.sendMessage>>) => void;
    const send = vi.spyOn(api, "sendMessage").mockReturnValue(
      new Promise((complete) => {
        resolve = complete;
      }),
    );
    showChat();
    await screen.findByRole("log");
    fireEvent.change(screen.getByRole("textbox", { name: "Сообщение" }), {
      target: { value: "Новое сообщение" },
    });
    const button = screen.getByRole("button", { name: "Отправить" });
    act(() => {
      button.click();
      button.click();
    });
    await waitFor(() => expect(send).toHaveBeenCalledTimes(1));
    expect(screen.getByText("Отправляется…")).toBeInTheDocument();
    const confirmed = {
      ...message,
      id: "confirmed",
      text: "Новое сообщение",
      clientRequestId: send.mock.calls[0][1].clientRequestId,
    };
    const peerMessage = {
      ...confirmed,
      id: "peer",
      senderId: "bob",
      senderName: "Борис",
      text: "Другой автор",
    };
    vi.mocked(api.messages).mockResolvedValue({
      ...page,
      items: [peerMessage],
    });
    await act(async () => {
      await clients.at(-1)!.invalidateQueries({ queryKey: ["chat", "room"] });
    });
    expect(screen.getByText("Отправляется…")).toBeInTheDocument();
    vi.mocked(api.messages).mockResolvedValue({ ...page, items: [confirmed] });
    await act(async () => {
      await clients.at(-1)!.invalidateQueries({ queryKey: ["chat", "room"] });
    });
    await waitFor(() =>
      expect(
        screen.getByRole("log").querySelectorAll(".chat-text"),
      ).toHaveLength(1),
    );
    expect(screen.queryByText("Отправляется…")).toBeNull();
    await act(async () => resolve({ status: "success", item: confirmed }));
    await waitFor(() =>
      expect(screen.getAllByTestId("chat-message-confirmed")).toHaveLength(1),
    );
    expect(screen.getByRole("textbox", { name: "Сообщение" })).toHaveValue("");
  });

  it("не добавляет смайлик сверх серверного лимита в 4000 Unicode-символов", async () => {
    showChat();
    await screen.findByText(message.text);
    const textarea = screen.getByLabelText("Сообщение") as HTMLTextAreaElement;
    const fullDraft = "Я".repeat(4000);
    fireEvent.change(textarea, { target: { value: fullDraft } });
    textarea.setSelectionRange(fullDraft.length, fullDraft.length);
    fireEvent.select(textarea);
    chooseEmoji();
    expect(textarea).toHaveValue(fullDraft);
    expect(screen.getByRole("alert")).toHaveTextContent(/4000/);
    expect(textarea).not.toHaveAttribute("maxlength");
  });

  it("в истории только для чтения не показывает редактор и выбор смайликов", async () => {
    showChat(true);
    expect(await screen.findByText(message.text)).toBeVisible();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Добавить смайлик" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Отправить" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Ответить" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Изменить" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Удалить" })).toBeNull();
  });

  it("сохраняет личную отметку важного сообщения и в завершённой встрече", async () => {
    const save = vi
      .spyOn(api, "setConferenceChatImportant")
      .mockImplementation(async () => {
        vi.mocked(api.messages).mockResolvedValue({
          status: "success",
          items: [{ ...message, important: true }],
          nextCursor: null,
          unreadCount: 0,
          lastReadMessageId: null,
        });
        return { status: "success" };
      });
    showChat(true);
    fireEvent.click(
      await screen.findByRole("button", { name: /Действия с сообщением:/ }),
    );
    expect(
      screen.queryByRole("menuitem", { name: /Ответить|Изменить|Удалить/ }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "В важные" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith("room", message.id, true),
    );
    fireEvent.click(
      screen.getByRole("button", { name: /Действия с сообщением:/ }),
    );
    expect(
      await screen.findByRole("menuitemcheckbox", { name: "Убрать из важных" }),
    ).toHaveAttribute("aria-checked", "true");
    expect(screen.queryByRole("button", { name: "Удалить" })).toBeNull();
  });

  it("показывает ошибку сохранения отметки, сохраняя подтверждённое состояние", async () => {
    vi.spyOn(api, "setConferenceChatImportant").mockRejectedValue(
      new Error("Не удалось сохранить отметку"),
    );
    showChat();
    fireEvent.click(
      await screen.findByRole("button", { name: /Действия с сообщением:/ }),
    );
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "В важные" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Не удалось связаться с сервером",
    );
    fireEvent.click(
      screen.getByRole("button", { name: /Действия с сообщением:/ }),
    );
    expect(
      screen.getByRole("menuitemcheckbox", { name: "В важные" }),
    ).toHaveAttribute("aria-checked", "false");
  });

  it("открывает точный контекст сообщения без обхода страниц всей истории", async () => {
    const context = vi
      .spyOn(api, "conferenceChatMessageContext")
      .mockResolvedValue({
        status: "success",
        items: [message],
        nextCursor: "older-cursor",
        unreadCount: 0,
        lastReadMessageId: null,
      });
    showChat(true, message.id);
    expect(
      await screen.findByTestId(`chat-message-${message.id}`),
    ).toHaveAttribute("aria-current", "true");
    expect(context).toHaveBeenCalledWith(
      "room",
      message.id,
      expect.any(AbortSignal),
    );
    expect(api.messages).not.toHaveBeenCalled();
  });
  it("после отправки из старого контекста открывает последние сообщения", async () => {
    vi.spyOn(api, "conferenceChatMessageContext").mockResolvedValue({
      status: "success",
      items: [message],
      nextCursor: null,
      unreadCount: 0,
      lastReadMessageId: null,
    });
    vi.spyOn(api, "sendMessage").mockResolvedValue({
      status: "success",
      item: message,
    });
    const onLatest = vi.fn();
    showChat(false, message.id, onLatest);
    await screen.findByTestId(`chat-message-${message.id}`);
    fireEvent.change(screen.getByRole("textbox", { name: "Сообщение" }), {
      target: { value: "Новое сообщение" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Отправить" }));
    await waitFor(() => expect(onLatest).toHaveBeenCalledOnce());
  });

  it("разделяет локальные дни, отличает свои пузыри и выводит время HH:mm", async () => {
    const today = new Date();
    today.setHours(9, 12, 0, 0);
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);
    const history = [
      {
        ...message,
        id: "incoming-one",
        sequence: "1",
        senderId: "other",
        senderName: "Мария",
        text: "Вчерашнее первое",
        createdAt: yesterday.toISOString(),
        updatedAt: yesterday.toISOString(),
      },
      {
        ...message,
        id: "incoming-two",
        sequence: "2",
        senderId: "other",
        senderName: "Мария",
        text: "Вчерашнее второе",
        createdAt: yesterday.toISOString(),
        updatedAt: yesterday.toISOString(),
      },
      {
        ...message,
        id: "own-today",
        sequence: "3",
        text: "Сегодняшнее своё",
        createdAt: today.toISOString(),
        updatedAt: today.toISOString(),
      },
    ];
    vi.mocked(api.messages).mockResolvedValue({
      status: "success",
      items: history,
      nextCursor: null,
      unreadCount: 0,
      lastReadMessageId: null,
    });
    const { container } = showChat();
    const own = await screen.findByTestId("chat-message-own-today");
    const incoming = screen.getByTestId("chat-message-incoming-one");
    expect(own).toHaveClass("chat-message-own");
    expect(incoming).not.toHaveClass("chat-message-own");
    expect(
      Array.from(container.querySelectorAll(".chat-day-label"), (element) =>
        element.textContent?.trim(),
      ),
    ).toEqual(["Вчера", "Сегодня"]);
    expect(within(own).getByText("09:12", { exact: true })).toBeVisible();
    expect(own.querySelector("time")).toHaveAttribute(
      "datetime",
      today.toISOString(),
    );
  });
});
