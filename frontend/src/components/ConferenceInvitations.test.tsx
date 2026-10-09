import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type { Conference } from "../types";
import {
  ConferenceInviteContent,
  ConferenceInvitationForm,
} from "./ConferenceInvitations";
import { ShareConference } from "./ConferenceModals";

const registered = {
  id: "registered-user",
  email: "alice@example.com",
  displayName: "Алиса",
};
const meeting = {
  id: "meeting",
  title: "Обсуждение проекта",
  inviteCode: "a".repeat(32),
  status: "scheduled",
} as Conference;
let clients: QueryClient[];
beforeEach(() => {
  clients = [];
  vi.spyOn(api, "invitationUsers").mockResolvedValue({
    status: "success",
    items: [registered],
  });
  vi.spyOn(api, "inviteParticipants").mockResolvedValue({
    status: "success",
    items: [
      {
        id: "invitation",
        email: registered.email,
        userId: registered.id,
        status: "queued",
      },
    ],
  });
});
afterEach(() => clients.forEach((client) => client.clear()));

function show(
  element = <ConferenceInvitationForm conferenceId={meeting.id} />,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{element}</MemoryRouter>
    </QueryClientProvider>,
  );
}

it("открывает серверную форму из результата создания вместо mailto и объясняет вход гостей", () => {
  show(<ShareConference conference={meeting} onClose={() => {}} />);
  const button = screen.getByRole("button", { name: "Пригласить участников" });
  expect(button).not.toHaveAttribute("href");
  expect(
    screen.getByText(/По ссылке можно присоединиться без аккаунта/),
  ).toBeInTheDocument();
  fireEvent.click(button);
  expect(
    screen.getByRole("dialog", { name: "Пригласить участников" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("textbox", { name: "Email участников" }),
  ).toHaveFocus();
});

it("короткий поиск не раскрывает список; выбранные пользователи и email отправляются без дублей", async () => {
  show();
  const search = screen.getByRole("searchbox", {
    name: "Добавить пользователей системы",
  });
  fireEvent.change(search, { target: { value: "а" } });
  expect(api.invitationUsers).not.toHaveBeenCalled();
  fireEvent.change(search, { target: { value: " Али " } });
  const checkbox = await screen.findByRole("checkbox", {
    name: /Алиса, alice@example.com/,
  });
  expect(api.invitationUsers).toHaveBeenCalledWith(
    meeting.id,
    "Али",
    expect.any(AbortSignal),
  );
  fireEvent.click(checkbox);
  fireEvent.change(screen.getByRole("textbox", { name: "Email участников" }), {
    target: {
      value: "ALICE@example.com, outside@example.com\noutside@example.com",
    },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Пригласить участников" }),
  );
  await waitFor(() =>
    expect(api.inviteParticipants).toHaveBeenCalledWith(meeting.id, {
      emails: ["outside@example.com"],
      userIds: [registered.id],
    }),
  );
  expect(
    await screen.findByText("Приглашение поставлено в очередь"),
  ).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Email участников" })).toHaveValue(
    "",
  );
  expect(
    screen.queryByRole("list", { name: "Выбранные пользователи" }),
  ).not.toBeInTheDocument();
});

it("отказ оставляет email редактируемым и даёт повторить запрос", async () => {
  vi.mocked(api.inviteParticipants).mockRejectedValueOnce(
    new ApiError(503, "Почта временно недоступна."),
  );
  show();
  const email = screen.getByRole("textbox", { name: "Email участников" });
  fireEvent.change(email, { target: { value: "outside@example.com" } });
  fireEvent.click(
    screen.getByRole("button", { name: "Пригласить участников" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Почта временно недоступна.",
  );
  expect(email).toHaveValue("outside@example.com");
  expect(email).not.toBeDisabled();
  fireEvent.click(
    screen.getByRole("button", { name: "Пригласить участников" }),
  );
  await screen.findByText("Приглашение поставлено в очередь");
  expect(api.inviteParticipants).toHaveBeenCalledTimes(2);
});

it("не отправляет пустые, некорректные или слишком большие списки", () => {
  show();
  const button = screen.getByRole("button", { name: "Пригласить участников" });
  const email = screen.getByRole("textbox", { name: "Email участников" });
  fireEvent.click(button);
  expect(screen.getByRole("alert")).toHaveTextContent(
    "Укажите email или выберите пользователя системы.",
  );
  fireEvent.change(email, { target: { value: "wrong-address" } });
  fireEvent.click(button);
  expect(screen.getByRole("alert")).toHaveTextContent(
    "Проверьте адреса email.",
  );
  fireEvent.change(email, {
    target: {
      value: Array.from(
        { length: 21 },
        (_, index) => `person${index}@example.com`,
      ).join(","),
    },
  });
  fireEvent.click(button);
  expect(screen.getByRole("alert")).toHaveTextContent("не больше 20 человек");
  expect(api.inviteParticipants).not.toHaveBeenCalled();
});

it("показывает пустой поиск и позволяет приглашать произвольный email", async () => {
  vi.mocked(api.invitationUsers).mockResolvedValue({
    status: "success",
    items: [],
  });
  show();
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "zz" } });
  expect(
    await screen.findByText(/Пользователи не найдены/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("textbox", { name: "Email участников" }),
  ).not.toBeDisabled();
});

it("показывает ошибку поиска отдельно и позволяет повторить его", async () => {
  vi.mocked(api.invitationUsers).mockRejectedValueOnce(
    new ApiError(500, "Не удалось выполнить поиск."),
  );
  show();
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "al" } });
  const retry = await screen.findByRole("button", { name: "Повторить поиск" });
  fireEvent.click(retry);
  expect(
    await screen.findByRole("checkbox", { name: /Алиса/ }),
  ).toBeInTheDocument();
  expect(api.invitationUsers).toHaveBeenCalledTimes(2);
});

it("обычным участникам доступна ссылка, форма и поиск не запускаются", () => {
  show(<ConferenceInviteContent conference={meeting} canInvite={false} />);
  expect(
    screen.getByRole("button", { name: "Копировать" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("textbox", { name: "Email участников" }),
  ).not.toBeInTheDocument();
  expect(api.invitationUsers).not.toHaveBeenCalled();
  expect(api.inviteParticipants).not.toHaveBeenCalled();
});

it("показывает уже отправлявшееся приглашение и блокирует повторные клики во время запроса", async () => {
  let finish!: (
    response: Awaited<ReturnType<typeof api.inviteParticipants>>,
  ) => void;
  vi.mocked(api.inviteParticipants).mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  show();
  fireEvent.change(screen.getByRole("textbox", { name: "Email участников" }), {
    target: { value: registered.email },
  });
  const button = screen.getByRole("button", { name: "Пригласить участников" });
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(api.inviteParticipants).toHaveBeenCalledTimes(1));
  expect(button).toBeDisabled();
  finish({
    status: "success",
    items: [{ id: "old", email: registered.email, status: "already_invited" }],
  });
  expect(
    await screen.findByText("Приглашение уже отправлялось"),
  ).toBeInTheDocument();
});
