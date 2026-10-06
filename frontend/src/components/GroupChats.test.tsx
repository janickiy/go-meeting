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
import type { GroupConversation, GroupMember } from "../types";
import {
  ConversationAvatar,
  GroupCreateModal,
  GroupInfoModal,
} from "./GroupChats";

vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: "alice", displayName: "Алиса" } }),
}));
const initial: GroupConversation = {
  id: "group",
  type: "group",
  name: "Команда",
  description: "План работ",
  createdBy: "alice",
  createdAt: "2026-10-06T10:00:00Z",
  updatedAt: "2026-10-06T10:00:00Z",
  memberCount: 3,
  myRole: "owner",
  avatarVersion: null,
  lastMessageAt: null,
  lastMessageId: null,
  lastSender: null,
  preview: "",
  unreadCount: 0,
};
const members: GroupMember[] = [
  { id: "alice", displayName: "Алиса", role: "owner", online: true },
  { id: "bob", displayName: "Борис", role: "admin", online: false },
  { id: "charlie", displayName: "Светлана", role: "member", online: null },
];
let group: GroupConversation;
const clients: QueryClient[] = [];
function Location() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}
function show(element: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const result = render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Location />
        {element}
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { client, ...result };
}
beforeEach(() => {
  group = { ...initial };
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = vi.fn(() => "blob:test-avatar");
      static revokeObjectURL = vi.fn();
    },
  );
  vi.spyOn(api, "personalConversation").mockImplementation(async () => ({
    status: "success",
    item: { ...group },
  }));
  vi.spyOn(api, "groupMembers").mockResolvedValue({
    status: "success",
    items: members,
  });
  vi.spyOn(api, "createGroup").mockResolvedValue({
    status: "success",
    item: initial,
  });
  vi.spyOn(api, "searchPersonalUsers").mockResolvedValue({
    items: [
      { id: "alice", displayName: "Алиса" },
      { id: "bob", displayName: "Борис" },
    ],
  });
  vi.spyOn(api, "putGroupAvatar").mockResolvedValue({
    status: "success",
    item: { ...initial, avatarVersion: "version" },
  });
  vi.spyOn(api, "transferGroup").mockImplementation(async () => {
    group.myRole = "admin";
    return { status: "success", item: { ...group } };
  });
  vi.spyOn(api, "leaveGroup").mockResolvedValue({ status: "success" });
  vi.spyOn(api, "deleteGroup").mockResolvedValue({ status: "success" });
  vi.spyOn(api, "setGroupRole").mockResolvedValue({
    status: "success",
    item: initial,
  });
  vi.spyOn(api, "removeGroupMember").mockResolvedValue({
    status: "success",
    item: initial,
  });
});
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  onlineManager.setOnline(true);
});

describe("group lifecycle UI", () => {
  it("purges cached member names when authoritative members access is denied", async () => {
    const onClose = vi.fn();
    const { client } = show(
      <GroupInfoModal
        group={initial}
        onClose={onClose}
        returnFocus={() => null}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Участники (3)" }));
    await screen.findByText("Светлана");
    vi.mocked(api.groupMembers).mockRejectedValue(
      new ApiError(403, "Доступ закрыт"),
    );
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["group-members", "group"] });
    });
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(screen.queryByText("Светлана")).toBeNull();
    expect(
      client.getQueryData(["group-members", "group", "alice"]),
    ).toBeUndefined();
  });
  it("shows only known live presence and clears cached green while offline", async () => {
    show(
      <GroupInfoModal
        group={initial}
        onClose={vi.fn()}
        returnFocus={() => null}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Участники (3)" }));
    await screen.findByText("Светлана");
    expect(screen.getByText("В сети")).toBeInTheDocument();
    expect(screen.getByText("Не в сети")).toBeInTheDocument();
    expect(screen.getByText("Статус недоступен")).toBeInTheDocument();
    expect(document.querySelectorAll(".group-presence-dot")).toHaveLength(1);
    act(() => onlineManager.setOnline(false));
    await waitFor(() =>
      expect(screen.getAllByText("Статус недоступен")).toHaveLength(3),
    );
    expect(document.querySelectorAll(".group-presence-dot")).toHaveLength(0);
  });
  it("creates once, selects registered peers and retries avatar without another group", async () => {
    vi.mocked(api.putGroupAvatar).mockRejectedValueOnce(
      new ApiError(503, "Аватар недоступен"),
    );
    const onClose = vi.fn();
    show(<GroupCreateModal onClose={onClose} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Название группы" }), {
      target: { value: "Команда" },
    });
    fireEvent.change(
      screen.getByRole("searchbox", { name: "Найти пользователя" }),
      { target: { value: "Бор" } },
    );
    const checkbox = await screen.findByRole("checkbox", { name: "Борис" });
    fireEvent.click(checkbox);
    expect(screen.queryByRole("checkbox", { name: "Алиса" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Файл аватара группы"), {
      target: {
        files: [new File(["image"], "avatar.png", { type: "image/png" })],
      },
    });
    const create = screen.getByRole("button", { name: "Создать" });
    act(() => {
      create.click();
      create.click();
    });
    await screen.findByText(/Группа создана/);
    expect(api.createGroup).toHaveBeenCalledTimes(1);
    expect(api.createGroup).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Команда",
        description: "",
        memberIds: ["bob"],
        clientRequestId: expect.any(String),
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Повторить загрузку аватара" }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(api.putGroupAvatar).toHaveBeenCalledTimes(2);
    expect(api.createGroup).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent(
        "/personal/group",
      ),
    );
  });
  it("retries failed creation with stable request identity and retains the form", async () => {
    vi.mocked(api.createGroup).mockRejectedValueOnce(new Error("offline"));
    show(<GroupCreateModal onClose={vi.fn()} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Название группы" }), {
      target: { value: "Команда" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    await screen.findByRole("alert");
    expect(
      screen.getByRole("textbox", { name: "Название группы" }),
    ).toHaveValue("Команда");
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    await waitFor(() => expect(api.createGroup).toHaveBeenCalledTimes(2));
    expect(vi.mocked(api.createGroup).mock.calls[1][0]).toEqual(
      vi.mocked(api.createGroup).mock.calls[0][0],
    );
  });
  it("requires ownership transfer before owner leave", async () => {
    show(
      <GroupInfoModal
        group={initial}
        onClose={vi.fn()}
        returnFocus={() => null}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Выйти из группы" }));
    expect(
      screen.getByText(/Перед выходом передайте владение/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Подтвердить выход" }),
    ).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Выбрать нового владельца" }),
    );
    fireEvent.click(
      await screen.findByRole("radio", { name: "Передать владение: Борис" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Передать владение" }));
    await waitFor(() =>
      expect(api.transferGroup).toHaveBeenCalledWith("group", "bob"),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "О группе" }),
      ).toBeInTheDocument(),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("button", {
          name: "Передать владение",
        }),
      ).toBeNull(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Выйти из группы" }));
    fireEvent.click(screen.getByRole("button", { name: "Подтвердить выход" }));
    await waitFor(() => expect(api.leaveGroup).toHaveBeenCalledWith("group"));
  });
  it.each(["admin", "member"] as const)(
    "restricts %s actions and protects administrators",
    async (role) => {
      group.myRole = role;
      show(
        <GroupInfoModal
          group={{ ...group }}
          onClose={vi.fn()}
          returnFocus={() => null}
        />,
      );
      expect(
        screen.queryByRole("button", { name: "Удалить группу" }),
      ).toBeNull();
      expect(
        screen.queryByRole("button", {
          name: "Передать владение",
        }),
      ).toBeNull();
      expect(!!screen.queryByRole("button", { name: "Настройки группы" })).toBe(
        role === "admin",
      );
      fireEvent.click(screen.getByRole("button", { name: "Участники (3)" }));
      await screen.findByText("Светлана");
      expect(
        screen.queryByRole("button", { name: "Назначить администратором" }),
      ).toBeNull();
      const bob = screen.getByText("Борис").closest("li")!;
      expect(
        within(bob).queryByRole("button", { name: "Удалить участника" }),
      ).toBeNull();
      expect(
        screen.queryAllByRole("button", { name: "Удалить участника" }).length,
      ).toBe(role === "admin" ? 1 : 0);
    },
  );
  it("releases private avatar URLs and aborts on version change/unmount", async () => {
    let signal: AbortSignal | undefined;
    vi.spyOn(api, "groupAvatar").mockImplementation(async (_id, abort) => {
      signal = abort;
      return new Blob(["image"], { type: "image/png" });
    });
    const { unmount } = show(
      <ConversationAvatar conversation={{ ...initial, avatarVersion: "v1" }} />,
    );
    await waitFor(() => expect(URL.createObjectURL).toHaveBeenCalledOnce());
    unmount();
    expect(signal?.aborted).toBe(true);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:test-avatar");
  });
});
