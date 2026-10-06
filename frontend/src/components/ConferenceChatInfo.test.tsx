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
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type {
  ChatMessage,
  Conference,
  ConferenceChatInfo,
  ConferenceChatMember,
} from "../types";
import { ConferenceChatActions } from "./ConferenceChatActions";
import { ConferenceChatInfoModal } from "./ConferenceChatInfo";

const authState = vi.hoisted(() => ({
  user: {
    id: "user",
    displayName: "Александр",
    guestConferenceId: undefined as string | undefined,
  },
}));
vi.mock("../auth", () => ({ useAuth: () => authState }));
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
  authState.user.guestConferenceId = undefined;
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
  vi.spyOn(api, "createPersonalConversation").mockResolvedValue({
    status: "success",
    item: {
      id: "existing-conversation",
      type: "direct",
      peer: { id: "peer", displayName: "Алиса" },
      createdAt: "2026-10-06T10:00:00Z",
      lastMessageAt: null,
      lastMessageId: null,
      preview: "",
      unreadCount: 0,
    },
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
        online: true,
        isGuest: false,
      } as ConferenceChatMember,
      {
        id: "p2",
        userId: "peer",
        displayName: "Алиса",
        role: "participant",
        status: "left",
        online: false,
        isGuest: false,
      } as ConferenceChatMember,
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
      <MemoryRouter>
        {element}
        <LocationOutput />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return client;
}
function LocationOutput() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}
function trigger() {
  return screen.getByRole("button", {
    name: "Действия с конференцией: Планирование",
  });
}
function member(
  overrides: Partial<ConferenceChatMember> = {},
): ConferenceChatMember {
  return {
    id: "p2",
    conferenceId: conference.id,
    userId: "peer",
    displayName: "Алиса",
    role: "participant",
    status: "left",
    joinedAt: null,
    leftAt: "2026-10-06T10:05:00Z",
    createdAt: "2026-10-06T10:00:00Z",
    updatedAt: "2026-10-06T10:05:00Z",
    online: true,
    isGuest: false,
    ...overrides,
  };
}

function showMembers(onClose = vi.fn()) {
  return show(
    <ConferenceChatInfoModal
      conference={conference}
      initialView="participants"
      onClose={onClose}
    />,
  );
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

  it.each(["info", "edit"] as const)(
    "не разрешает участнику редактирование в представлении %s даже при устаревшем canEdit",
    async (initialView) => {
      show(
        <ConferenceChatInfoModal
          conference={{ ...conference, ownerId: "other-owner" }}
          initialView={initialView}
          onClose={() => {}}
        />,
      );
      await waitFor(() => expect(api.conferenceChatInfo).toHaveBeenCalled());
      if (initialView === "edit") {
        expect(
          await screen.findByText(
            "Изменять название и описание может только администратор чата.",
          ),
        ).toBeInTheDocument();
      } else {
        await screen.findByRole("heading", { name: "Планирование" });
      }
      expect(
        screen.queryByRole("button", { name: "Название и описание" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("textbox", { name: "Название" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("textbox", { name: "Описание" }),
      ).not.toBeInTheDocument();
      expect(api.updateConferenceChatInfo).not.toHaveBeenCalled();
    },
  );

  it("разделяет администратора и участников, сохраняя фильтр состава", async () => {
    vi.mocked(api.conferenceChatMembers).mockResolvedValue({
      status: "success",
      nextCursor: null,
      items: [
        {
          id: "p1",
          userId: "user",
          displayName: "Александр",
          role: "owner",
          status: "joined",
          online: false,
          isGuest: false,
        } as ConferenceChatMember,
        {
          id: "p2",
          userId: "peer",
          displayName: "Алиса",
          role: "participant",
          status: "left",
          online: true,
          isGuest: false,
        } as ConferenceChatMember,
        {
          id: "p3",
          userId: "cohost",
          displayName: "Борис",
          role: "co_host",
          status: "joined",
          online: false,
          isGuest: false,
        } as ConferenceChatMember,
      ],
    });
    show(
      <ConferenceChatInfoModal
        conference={conference}
        initialView="participants"
        onClose={() => {}}
      />,
    );
    await screen.findByText("Алиса");
    const administrators = within(
      screen.getByRole("region", { name: "Администраторы" }),
    );
    const participants = within(
      screen.getByRole("region", { name: "Участники" }),
    );
    expect(administrators.getByText("Александр (вы)")).toBeInTheDocument();
    expect(
      administrators.getByText("Администратор · Во встрече"),
    ).toBeInTheDocument();
    expect(administrators.queryByText("Алиса")).not.toBeInTheDocument();
    expect(administrators.queryByText("Борис")).not.toBeInTheDocument();
    expect(participants.queryByText("Александр (вы)")).not.toBeInTheDocument();
    expect(participants.getByText("Алиса")).toBeInTheDocument();
    expect(participants.getByText("Борис")).toBeInTheDocument();
    expect(
      participants.getByText("Участник · Вышел из встречи"),
    ).toBeInTheDocument();
    fireEvent.change(
      screen.getByRole("searchbox", { name: "Найти участника" }),
      { target: { value: "Али" } },
    );
    expect(screen.queryByText("Александр (вы)")).toBeNull();
    expect(screen.queryByText("Борис")).toBeNull();
    expect(participants.getByText("Алиса")).toBeInTheDocument();
    expect(api.conferenceChatMembers).toHaveBeenCalledWith(
      conference.id,
      undefined,
      expect.any(AbortSignal),
    );
  });

  it("показывает серверное присутствие независимо от участия во встрече", async () => {
    vi.mocked(api.conferenceChatMembers).mockResolvedValue({
      status: "success",
      nextCursor: null,
      items: [
        member({
          id: "p1",
          userId: "user",
          displayName: "Александр",
          role: "owner",
          status: "joined",
          online: false,
        }),
        member({ status: "left", online: true }),
        member({
          id: "p3",
          userId: "boris",
          displayName: "Борис",
          status: "joined",
          online: null,
        }),
      ],
    });
    showMembers();
    const online = await screen.findByRole("button", {
      name: "О пользователе: Алиса",
    });
    expect(within(online).getByText("В сети")).toBeInTheDocument();
    expect(
      online.querySelector(".conference-chat-member-online-dot"),
    ).not.toBeNull();
    const offline = screen.getByRole("button", {
      name: "О пользователе: Александр",
    });
    expect(within(offline).getByText("Не в сети")).toBeInTheDocument();
    expect(
      offline.querySelector(".conference-chat-member-online-dot"),
    ).toBeNull();
    const unknown = screen.getByRole("button", {
      name: "О пользователе: Борис",
    });
    expect(within(unknown).getByText("Статус недоступен")).toBeInTheDocument();
    expect(
      unknown.querySelector(".conference-chat-member-online-dot"),
    ).toBeNull();
  });

  it("открывает карточку пользователя и возвращает фильтр, страницы и фокус по Escape", async () => {
    vi.mocked(api.conferenceChatMembers)
      .mockResolvedValueOnce({
        status: "success",
        items: [member({ id: "p3", userId: "boris", displayName: "Борис" })],
        nextCursor: "page2",
      })
      .mockResolvedValue({
        status: "success",
        items: [member()],
        nextCursor: null,
      });
    const onClose = vi.fn();
    showMembers(onClose);
    fireEvent.click(
      await screen.findByRole("button", { name: "Ещё участники" }),
    );
    const row = await screen.findByRole("button", {
      name: "О пользователе: Алиса",
    });
    const filter = screen.getByRole("searchbox", { name: "Найти участника" });
    fireEvent.change(filter, { target: { value: "Али" } });
    fireEvent.click(row);
    const card = screen.getByRole("dialog", { name: "О пользователе" });
    const profile = within(card).getByRole("region", {
      name: "Профиль: Алиса",
    });
    expect(profile).toHaveFocus();
    expect(
      within(profile).getByRole("heading", { name: "Алиса" }),
    ).toBeInTheDocument();
    expect(within(profile).getByText("В сети")).toBeInTheDocument();
    expect(
      within(profile).queryByRole("button", { name: /Звонок|Видеозвонок/ }),
    ).toBeNull();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    await screen.findByRole("dialog", { name: "Участники" });
    expect(filter).toHaveValue("Али");
    expect(row).toHaveFocus();
    expect(onClose).not.toHaveBeenCalled();
    expect(api.conferenceChatMembers).toHaveBeenCalledTimes(2);
    expect(api.conferenceChatMembers).toHaveBeenLastCalledWith(
      conference.id,
      "page2",
      expect.any(AbortSignal),
    );
  });

  it("открывает существующую личную переписку после серверного ответа", async () => {
    const onClose = vi.fn();
    const client = showMembers(onClose);
    const invalidate = vi.spyOn(client, "invalidateQueries");
    fireEvent.click(
      await screen.findByRole("button", { name: "О пользователе: Алиса" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Сообщение" }));
    await waitFor(() =>
      expect(api.createPersonalConversation).toHaveBeenCalledWith("peer"),
    );
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/personal/existing-conversation",
      ),
    );
    expect(onClose).toHaveBeenCalledOnce();
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["personal-list", "user"],
    });
    expect(
      client.getQueryData(["personal-detail", "existing-conversation", "user"]),
    ).toMatchObject({ item: { id: "existing-conversation" } });
  });

  it.each([
    ["себя", { userId: "user" }, false],
    ["гостя с аккаунтом", { userId: "guest-user", isGuest: true }, false],
    ["участника от имени гостя", {}, true],
  ] as const)(
    "не предлагает личное сообщение для %s",
    async (_label, values, guestActor) => {
      if (guestActor) authState.user.guestConferenceId = conference.id;
      vi.mocked(api.conferenceChatMembers).mockResolvedValue({
        status: "success",
        items: [member(values)],
        nextCursor: null,
      });
      showMembers();
      fireEvent.click(
        await screen.findByRole("button", { name: "О пользователе: Алиса" }),
      );
      expect(
        screen.getByRole("dialog", { name: "О пользователе" }),
      ).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Сообщение" })).toBeNull();
      expect(api.createPersonalConversation).not.toHaveBeenCalled();
    },
  );

  it("сохраняет карточку при отказе создать переписку и не навигирует заранее", async () => {
    let reject!: (error: unknown) => void;
    vi.mocked(api.createPersonalConversation).mockReturnValue(
      new Promise((_resolve, rejectRequest) => {
        reject = rejectRequest;
      }),
    );
    const onClose = vi.fn();
    showMembers(onClose);
    fireEvent.click(
      await screen.findByRole("button", { name: "О пользователе: Алиса" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Сообщение" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Сообщение" })).toBeDisabled(),
    );
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(
      screen.getByRole("dialog", { name: "О пользователе" }),
    ).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/$/);
    await act(async () =>
      reject(new ApiError(403, "Личные сообщения недоступны")),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Личные сообщения недоступны",
    );
    expect(screen.getByRole("button", { name: "Сообщение" })).toBeEnabled();
    expect(
      screen.getByRole("dialog", { name: "О пользователе" }),
    ).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("отправляет один запрос при немедленных повторных кликах и разрешает повтор после отказа", async () => {
    let reject!: (error: unknown) => void;
    vi.mocked(api.createPersonalConversation).mockImplementation(
      () =>
        new Promise((_resolve, rejectRequest) => {
          reject = rejectRequest;
        }),
    );
    showMembers();
    fireEvent.click(
      await screen.findByRole("button", { name: "О пользователе: Алиса" }),
    );
    const button = screen.getByRole("button", { name: "Сообщение" });
    act(() => {
      button.click();
      button.click();
    });
    await waitFor(() =>
      expect(api.createPersonalConversation).toHaveBeenCalledTimes(1),
    );
    await act(async () => reject(new ApiError(403, "Нет доступа")));
    expect(await screen.findByRole("alert")).toHaveTextContent("Нет доступа");
    expect(button).toBeEnabled();
    act(() => {
      button.click();
      button.click();
    });
    await waitFor(() =>
      expect(api.createPersonalConversation).toHaveBeenCalledTimes(2),
    );
    await act(async () => reject(new ApiError(403, "Нет доступа")));
  });

  it("сохраняет состав и доступ к карточкам при неизвестном присутствии или ошибке обновления", async () => {
    vi.mocked(api.conferenceChatMembers).mockResolvedValue({
      status: "success",
      items: [member()],
      nextCursor: null,
    });
    const client = showMembers();
    const row = await screen.findByRole("button", {
      name: "О пользователе: Алиса",
    });
    vi.mocked(api.conferenceChatMembers).mockResolvedValue({
      status: "success",
      items: [member({ online: null })],
      nextCursor: null,
    });
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["conference-chat-members"] });
    });
    expect(
      await within(row).findByText("Статус недоступен"),
    ).toBeInTheDocument();
    expect(row.querySelector(".conference-chat-member-online-dot")).toBeNull();
    fireEvent.click(row);
    const profile = screen.getByRole("region", { name: "Профиль: Алиса" });
    expect(within(profile).getByText("Статус недоступен")).toBeInTheDocument();
    expect(
      within(profile).getByRole("button", { name: "Сообщение" }),
    ).toBeEnabled();
    vi.mocked(api.conferenceChatMembers).mockResolvedValue({
      status: "success",
      items: [member({ online: false })],
      nextCursor: null,
    });
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["conference-chat-members"] });
    });
    expect(await within(profile).findByText("Не в сети")).toBeInTheDocument();
    vi.mocked(api.conferenceChatMembers).mockRejectedValue(
      new ApiError(503, "Сервис временно недоступен"),
    );
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["conference-chat-members"] });
    });
    expect(
      await within(profile).findByText("Статус недоступен"),
    ).toBeInTheDocument();
    expect(
      profile.querySelector(".conference-chat-member-online-dot"),
    ).toBeNull();
    expect(
      within(profile).getByRole("button", { name: "Повторить загрузку" }),
    ).toBeInTheDocument();
  });

  it("обновляет присутствие только в видимом открытом списке и освобождает polling после закрытия", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const visibility = vi
      .spyOn(document, "visibilityState", "get")
      .mockReturnValue("visible");
    try {
      showMembers();
      await screen.findByRole("button", { name: "О пользователе: Алиса" });
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(1);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(2);
      visibility.mockReturnValue("hidden");
      fireEvent(document, new Event("visibilitychange"));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30000);
      });
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(2);
      visibility.mockReturnValue("visible");
      fireEvent(document, new Event("visibilitychange"));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(3);
      let pendingSignal!: AbortSignal;
      vi.mocked(api.conferenceChatMembers).mockImplementationOnce(
        (_id, _after, signal) => {
          pendingSignal = signal!;
          return new Promise(() => {});
        },
      );
      await act(async () => {
        await vi.advanceTimersByTimeAsync(5000);
      });
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(4);
      expect(pendingSignal.aborted).toBe(false);
      visibility.mockReturnValue("hidden");
      fireEvent(document, new Event("visibilitychange"));
      expect(pendingSignal.aborted).toBe(true);
      cleanup();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30000);
      });
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(4);
    } finally {
      vi.useRealTimers();
    }
  });

  it("скрывает кешированный онлайн-статус списка и карточки при остановке запросов без сети", async () => {
    vi.mocked(api.conferenceChatMembers).mockResolvedValue({
      status: "success",
      items: [member({ online: true })],
      nextCursor: null,
    });
    const client = showMembers();
    const row = await screen.findByRole("button", {
      name: "О пользователе: Алиса",
    });
    expect(
      row.querySelector(".conference-chat-member-online-dot"),
    ).not.toBeNull();
    try {
      act(() => {
        onlineManager.setOnline(false);
        void client.invalidateQueries({
          queryKey: ["conference-chat-members"],
        });
      });
      await waitFor(() =>
        expect(
          client.getQueryState([
            "conference-chat-members",
            conference.id,
            "user",
          ])?.fetchStatus,
        ).toBe("paused"),
      );
      expect(within(row).getByText("Статус недоступен")).toBeInTheDocument();
      expect(
        row.querySelector(".conference-chat-member-online-dot"),
      ).toBeNull();
      expect(api.conferenceChatMembers).toHaveBeenCalledTimes(1);
      fireEvent.click(row);
      const profile = screen.getByRole("region", { name: "Профиль: Алиса" });
      expect(
        within(profile).getByText("Статус недоступен"),
      ).toBeInTheDocument();
      expect(
        profile.querySelector(".conference-chat-member-online-dot"),
      ).toBeNull();
      act(() => onlineManager.setOnline(true));
      expect(await within(profile).findByText("В сети")).toBeInTheDocument();
      await waitFor(() =>
        expect(api.conferenceChatMembers).toHaveBeenCalledTimes(2),
      );
    } finally {
      onlineManager.setOnline(true);
    }
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
