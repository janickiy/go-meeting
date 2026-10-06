import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type {
  ChatMessage,
  Conference,
  ConferenceChatInfo,
  Participant,
} from "../types";
import { ConferenceChatActions } from "./ConferenceChatActions";
import { ConferenceChatInfoModal } from "./ConferenceChatInfo";

vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: "user", displayName: "Александр" } }),
}));
const conference: Conference = {
  id: "conference",
  ownerId: "user",
  title: "Планирование",
  status: "active",
  inviteCode: "a".repeat(32),
  inviteUrl: "https://meet.example/i/" + "a".repeat(32),
  createdAt: "2026-10-06T10:00:00Z",
  updatedAt: "2026-10-06T10:00:00Z",
  startedAt: "2026-10-06T10:00:00Z",
  finishedAt: null,
};
const message: ChatMessage = {
  id: "message",
  sequence: "1",
  conferenceId: conference.id,
  senderId: "peer",
  senderName: "Алиса",
  text: "Срок сдачи проекта",
  important: false,
  createdAt: "2026-10-06T10:01:00Z",
  updatedAt: "2026-10-06T10:01:00Z",
  deletedAt: null,
  version: 1,
  attachments: [],
};
let info: ConferenceChatInfo;
let clients: QueryClient[];
beforeEach(() => {
  clients = [];
  info = {
    conferenceId: conference.id,
    title: "Планирование",
    description: "Обсуждение следующего этапа",
    inviteUrl: conference.inviteUrl,
    participantCount: 2,
    notificationsEnabled: true,
    canEdit: true,
    canInvite: true,
  };
  vi.spyOn(api, "conferenceChatInfo").mockImplementation(async () => ({
    status: "success",
    item: { ...info },
  }));
  vi.spyOn(api, "setConferenceChatNotifications").mockImplementation(
    async (_id, enabled) => {
      info.notificationsEnabled = enabled;
      return { status: "success", item: { notificationsEnabled: enabled } };
    },
  );
  vi.spyOn(api, "updateConferenceChatInfo").mockImplementation(
    async (_id, values) => {
      info = { ...info, ...values };
      return { status: "success", item: info };
    },
  );
  vi.spyOn(api, "leaveConferenceChat").mockResolvedValue({ status: "success" });
  vi.spyOn(api, "transition").mockResolvedValue({
    status: "success",
    item: conference,
  });
  vi.spyOn(api, "searchConferenceChat").mockResolvedValue({
    status: "success",
    items: [message],
    nextCursor: null,
  });
  vi.spyOn(api, "conferenceChatPins").mockResolvedValue({
    status: "success",
    items: [message],
    nextCursor: null,
  });
  vi.spyOn(api, "setConferenceChatImportant").mockResolvedValue({
    status: "success",
  });
  vi.spyOn(api, "conferenceChatMaterials").mockResolvedValue({
    status: "success",
    items: [],
    nextCursor: null,
  });
  vi.spyOn(api, "conferenceChatMembers").mockResolvedValue({
    status: "success",
    nextCursor: null,
    items: [
      {
        id: "p1",
        userId: "user",
        displayName: "Александр",
        role: "owner",
        status: "joined",
      } as Participant,
      {
        id: "p2",
        userId: "peer",
        displayName: "Алиса",
        role: "participant",
        status: "left",
      } as Participant,
    ],
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  vi.restoreAllMocks();
});
function show(element = <ConferenceChatActions conference={conference} />) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{element}</MemoryRouter>
    </QueryClientProvider>,
  );
  return client;
}
function trigger() {
  return screen.getByRole("button", {
    name: "Действия с конференцией: Планирование",
  });
}

describe("меню конференции", () => {
  it("не делает запросы для закрытых строк и содержит четыре действия без жалобы", async () => {
    show();
    expect(api.conferenceChatInfo).not.toHaveBeenCalled();
    fireEvent.click(trigger());
    const menu = screen.getByRole("menu");
    expect(within(menu).getAllByRole("menuitem")).toHaveLength(4);
    expect(
      within(menu).getByRole("menuitem", { name: "Информация о чате" }),
    ).toHaveFocus();
    expect(
      await screen.findByRole("menuitem", { name: "Выключить уведомления" }),
    ).toBeEnabled();
    expect(screen.queryByText("Пожаловаться")).not.toBeInTheDocument();
    fireEvent.keyDown(document.activeElement!, { key: "End" });
    expect(
      within(menu).getByRole("menuitem", { name: "Покинуть чат" }),
    ).toHaveFocus();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger()).toHaveFocus();
  });

  it("сохраняет переключение уведомлений на сервере и показывает новый пункт", async () => {
    show();
    fireEvent.click(trigger());
    const off = await screen.findByRole("menuitem", {
      name: "Выключить уведомления",
    });
    await waitFor(() => expect(off).toBeEnabled());
    fireEvent.click(off);
    await waitFor(() =>
      expect(api.setConferenceChatNotifications).toHaveBeenCalledWith(
        conference.id,
        false,
      ),
    );
    expect(
      await screen.findByRole("menuitem", { name: "Включить уведомления" }),
    ).toBeEnabled();
  });

  it("не скрывает строку при отказе сервера покинуть чат", async () => {
    vi.mocked(api.leaveConferenceChat).mockRejectedValue(
      new ApiError(403, "Нет доступа"),
    );
    show();
    fireEvent.click(trigger());
    fireEvent.click(screen.getByRole("menuitem", { name: "Покинуть чат" }));
    const dialog = await screen.findByRole("dialog", { name: "Покинуть чат" });
    fireEvent.click(
      await within(dialog).findByRole("button", { name: "Покинуть чат" }),
    );
    await screen.findByRole("alert");
    expect(dialog).toBeInTheDocument();
    expect(trigger()).toBeInTheDocument();
  });

  it("даёт участнику ссылку и ограничивает отправку приглашений серверной политикой", async () => {
    info.canInvite = false;
    info.canEdit = false;
    show();
    fireEvent.click(trigger());
    await waitFor(() => expect(api.conferenceChatInfo).toHaveBeenCalled());
    expect(
      screen.getByRole("menuitem", { name: "Добавить участников" }),
    ).toBeEnabled();
    fireEvent.click(
      screen.getByRole("menuitem", { name: "Информация о чате" }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Информация о чате",
    });
    expect(
      within(dialog).getByRole("button", { name: "Добавить участников" }),
    ).toBeEnabled();
    expect(
      within(dialog).queryByRole("button", { name: "Название и описание" }),
    ).toBeNull();
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Добавить участников" }),
    );
    const inviteDialog = screen.getByRole("dialog", {
      name: "Добавить участников",
    });
    expect(
      within(inviteDialog).getByText(/Участников добавляет организатор/),
    ).toBeInTheDocument();
    expect(
      within(inviteDialog).getByRole("button", { name: "Копировать" }),
    ).toBeInTheDocument();
    expect(
      within(inviteDialog).queryByRole("textbox", { name: "Email участников" }),
    ).toBeNull();
  });
});

describe("информация о чате", () => {
  it("показывает сведения, счетчик, рабочие действия и возвращает фокус", async () => {
    show();
    fireEvent.click(trigger());
    fireEvent.click(
      screen.getByRole("menuitem", { name: "Информация о чате" }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Информация о чате",
    });
    await within(dialog).findByText("Обсуждение следующего этапа");
    expect(
      within(dialog).getByRole("link", { name: "Ссылка на видеовстречу" }),
    ).toHaveAttribute("href", "/meetings/conference");
    expect(
      within(dialog).getByRole("switch", { name: "Уведомления" }),
    ).toHaveAttribute("aria-checked", "true");
    for (const label of [
      "Найти в чате",
      "Участники",
      "Картинки, файлы и ссылки",
      "Важные сообщения",
      "Покинуть чат",
    ])
      expect(
        within(dialog).getByRole("button", { name: new RegExp(label) }),
      ).toBeEnabled();
    expect(within(dialog).queryByText("Пожаловаться")).toBeNull();
    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(trigger()).toHaveFocus();
  });

  it("подключает существующую форму приглашений вместо фиктивного действия", async () => {
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="invite"
        onClose={() => {}}
      />,
    );
    expect(
      await screen.findByRole("textbox", { name: "Email участников" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("searchbox", { name: "Добавить пользователей системы" }),
    ).toBeInTheDocument();
  });

  it("подтверждает выход и обновляет список только после успешного ответа", async () => {
    let complete!: (value: { status: string }) => void;
    vi.mocked(api.leaveConferenceChat).mockReturnValue(
      new Promise((resolve) => {
        complete = resolve;
      }),
    );
    const onClose = vi.fn();
    const onLeft = vi.fn();
    const client = show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="leave"
        onClose={onClose}
        onLeft={onLeft}
      />,
    );
    const invalidate = vi.spyOn(client, "invalidateQueries");
    fireEvent.click(
      await screen.findByRole("button", { name: "Покинуть чат" }),
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(onLeft).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Покинуть чат" }),
      ).toBeDisabled(),
    );
    await act(async () => complete({ status: "success" }));
    await waitFor(() => expect(onLeft).toHaveBeenCalledOnce());
    expect(onClose).toHaveBeenCalledOnce();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["conferences"] });
    expect(api.transition).not.toHaveBeenCalled();
  });

  it("редактирует название и описание, проверяя пустое название", async () => {
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="edit"
        onClose={() => {}}
      />,
    );
    const title = await screen.findByRole("textbox", { name: "Название" });
    expect(screen.getByRole("textbox", { name: "Описание" })).toHaveAttribute(
      "maxlength",
      "1000",
    );
    fireEvent.change(title, { target: { value: " " } });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Название");
    expect(api.updateConferenceChatInfo).not.toHaveBeenCalled();
    fireEvent.change(title, { target: { value: " Новое название " } });
    fireEvent.change(screen.getByRole("textbox", { name: "Описание" }), {
      target: { value: " Новый этап " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    await waitFor(() =>
      expect(api.updateConferenceChatInfo).toHaveBeenCalledWith(conference.id, {
        title: "Новое название",
        description: "Новый этап",
      }),
    );
    expect(
      await screen.findByRole("heading", { name: "Новое название" }),
    ).toBeInTheDocument();
  });

  it("читает фактических участников и фильтрует загруженный состав", async () => {
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="participants"
        onClose={() => {}}
      />,
    );
    await screen.findByText("Алиса");
    expect(screen.getByText("Александр (вы)")).toBeInTheDocument();
    expect(screen.getByText("Участник · Вышел из встречи")).toBeInTheDocument();
    fireEvent.change(
      screen.getByRole("searchbox", { name: "Найти участника" }),
      { target: { value: "Али" } },
    );
    expect(screen.queryByText("Александр (вы)")).toBeNull();
    expect(api.conferenceChatMembers).toHaveBeenCalledWith(
      conference.id,
      undefined,
      expect.any(AbortSignal),
    );
  });

  it("ищет по серверной истории и ведёт к сообщению в завершённой конференции", async () => {
    const onClose = vi.fn();
    show(
      <ConferenceChatInfoModal
        conference={{ ...conference, status: "finished" }}
        initialView="search"
        onClose={onClose}
      />,
    );
    const input = await screen.findByRole("searchbox", {
      name: "Поиск сообщений",
    });
    fireEvent.change(input, { target: { value: "С" } });
    expect(api.searchConferenceChat).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: " Срок " } });
    await screen.findByText("Срок сдачи проекта");
    expect(api.searchConferenceChat).toHaveBeenCalledWith(
      conference.id,
      "Срок",
      undefined,
      expect.any(AbortSignal),
    );
    const link = screen.getByRole("link", { name: "Открыть в чате" });
    expect(link).toHaveAttribute(
      "href",
      "/meetings/conference?chat=1&message=message",
    );
    fireEvent.click(link);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("важные сообщения являются личными серверными закладками", async () => {
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="important"
        onClose={() => {}}
      />,
    );
    const remove = await screen.findByRole("button", {
      name: "Убрать из важных",
    });
    expect(remove).toHaveAttribute("aria-pressed", "true");
    vi.mocked(api.conferenceChatPins).mockResolvedValue({
      status: "success",
      items: [],
      nextCursor: null,
    });
    fireEvent.click(remove);
    await waitFor(() =>
      expect(api.setConferenceChatImportant).toHaveBeenCalledWith(
        conference.id,
        message.id,
        false,
      ),
    );
    expect(
      await screen.findByText(/Важных сообщений пока нет/),
    ).toBeInTheDocument();
  });

  it("показывает материалы по категориям и скачивает только после разрешения API", async () => {
    const attachment = {
      id: "file",
      filename: "План.pdf",
      mimeType: "application/pdf",
      size: 1024,
      status: "attached",
    };
    vi.mocked(api.conferenceChatMaterials).mockImplementation(
      async (_id, kind) => ({
        status: "success",
        items: kind === "file" ? [{ message, attachment }] : [],
        nextCursor: null,
      }),
    );
    const signed = "https://media.example.test/private?signature=abc%2Fdef";
    vi.spyOn(api, "attachmentDownload").mockResolvedValue({
      url: signed,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    });
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="materials"
        onClose={() => {}}
      />,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Файлы" }));
    const file = await screen.findByRole("button", { name: /План.pdf/ });
    expect(api.attachmentDownload).not.toHaveBeenCalled();
    fireEvent.click(file);
    expect(
      await screen.findByRole("link", { name: "Скачать План.pdf" }),
    ).toHaveAttribute("href", signed);
    expect(api.attachmentDownload).toHaveBeenCalledWith(
      conference.id,
      attachment.id,
    );
  });

  it("не создаёт исполняемые ссылки из материалов", async () => {
    vi.mocked(api.conferenceChatMaterials).mockResolvedValue({
      status: "success",
      items: [{ message, url: "javascript:alert(1)" }],
      nextCursor: null,
    });
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="materials"
        onClose={() => {}}
      />,
    );
    await screen.findByText(message.text);
    expect(document.querySelector('a[href^="javascript:"]')).toBeNull();
  });
});
