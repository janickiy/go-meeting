import { expect, test, type Page } from "@playwright/test";
import type { Conference, Participant } from "../src/types";

const stamp = "2026-10-05T10:00:00Z";
const owner = {
  id: "invite-owner",
  email: "owner@example.test",
  displayName: "Александр",
  createdAt: stamp,
  updatedAt: stamp,
};
const contact = {
  id: "invite-contact",
  email: "alice@example.test",
  displayName: "Алиса",
};
const meeting: Conference = {
  id: "invite-meeting",
  ownerId: owner.id,
  title: "Встреча проекта",
  status: "scheduled",
  createdAt: stamp,
  updatedAt: stamp,
  startedAt: null,
  finishedAt: null,
  scheduledAt: "2026-10-06T10:00:00Z",
  inviteCode: "a".repeat(32),
  inviteUrl: "",
};

/** Browser fixtures exercise real views without sending mail or creating production accounts. */
async function fixture(
  page: Page,
  role: "owner" | "participant" | "guest" = "owner",
  failOnce = false,
) {
  const self =
    role === "owner"
      ? owner
      : {
          ...owner,
          id: "invite-member",
          ...(role === "guest" ? { guestConferenceId: meeting.id } : {}),
        };
  const member: Participant = {
    id: "invite-membership",
    userId: self.id,
    conferenceId: meeting.id,
    displayName: self.displayName,
    role,
    status: "joined",
    admissionState: "admitted",
    createdAt: stamp,
    updatedAt: stamp,
    joinedAt: stamp,
    leftAt: null,
  };
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "invitation-browser-fixture",
        expiresAt: Date.now() + 1_800_000,
      }),
    ),
  );
  const writes: { emails: string[]; userIds: string[] }[] = [];
  const searches: string[] = [];
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    const respond = (json: unknown, status = 200) =>
      route.fulfill({ status, json });
    if (path === "/auth/me") return respond({ user: self });
    if (path === "/capabilities")
      return respond({
        status: "success",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: false,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
      });
    if (path === "/notifications/events")
      return route.fulfill({
        contentType: "text/event-stream",
        body: ": fixture\n\n",
      });
    if (path === "/notifications")
      return respond({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path === "/me/conferences")
      return respond({ status: "success", items: [meeting], nextCursor: null });
    if (path === "/conferences" && request.method() === "POST")
      return respond({ status: "success", item: meeting }, 201);
    if (path === `/conferences/${meeting.id}`)
      return respond({ status: "success", item: meeting });
    if (path.endsWith("/participants/me"))
      return respond({ status: "success", item: member });
    if (path.endsWith("/participants"))
      return respond({ status: "success", items: [member] });
    if (
      path.endsWith("/recordings") ||
      path.endsWith("/calendar") ||
      path.endsWith("/messages")
    )
      return respond({ status: "success", items: [], nextCursor: null });
    if (path.endsWith("/chat/read"))
      return respond({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    if (path.endsWith("/invitation-users")) {
      searches.push(url.searchParams.get("query") || "");
      return respond({ status: "success", items: [contact] });
    }
    if (path.endsWith("/invitations") && request.method() === "POST") {
      writes.push(request.postDataJSON());
      if (failOnce && writes.length === 1)
        return respond({ message: "temporarily unavailable" }, 503);
      return respond(
        {
          status: "success",
          items: [
            ...writes.at(-1)!.emails.map((email, index) => ({
              id: `email-${index}`,
              email,
              status: "queued",
            })),
            ...writes.at(-1)!.userIds.map((userId) => ({
              id: userId,
              userId,
              email: contact.email,
              status: "queued",
            })),
          ],
        },
        202,
      );
    }
    return respond({ message: "unavailable fixture route" }, 404);
  });
  return { writes, searches };
}

test("после создания: email и выбранный пользователь отправляются через форму без дублей", async ({
  page,
}, info) => {
  const state = await fixture(page);
  await page.goto("/meetings/new");
  await page
    .getByRole("textbox", { name: "Название конференции" })
    .fill(meeting.title);
  await page
    .getByRole("button", { name: "Создать конференцию", exact: true })
    .click();
  await page.getByRole("button", { name: "Пригласить по email" }).click();
  const dialog = page.getByRole("dialog", { name: "Пригласить участников" });
  await expect(
    dialog.getByRole("textbox", { name: "Email участников" }),
  ).toBeFocused();
  const search = dialog.getByRole("searchbox", {
    name: "Добавить пользователей системы",
  });
  await search.fill("А");
  await expect(
    dialog.getByRole("list", { name: "Пользователи системы" }),
  ).toHaveCount(0);
  await search.fill("Али");
  await dialog
    .getByRole("checkbox", { name: "Алиса, alice@example.test" })
    .check();
  await dialog
    .getByRole("textbox", { name: "Email участников" })
    .fill("ALICE@example.test, outside@example.test\noutside@example.test");
  await page.screenshot({
    path: info.outputPath("postcreate-invitation-desktop.png"),
    fullPage: true,
  });
  await dialog
    .getByRole("button", { name: "Пригласить участников", exact: true })
    .click();
  await expect(
    dialog.getByText("Приглашение поставлено в очередь", { exact: true }),
  ).toHaveCount(2);
  expect(state.searches).toEqual(["Али"]);
  expect(state.writes).toEqual([
    { emails: ["outside@example.test"], userIds: [contact.id] },
  ]);
  await expect(
    dialog.getByRole("textbox", { name: "Email участников" }),
  ).toHaveValue("");
});

test("страница встречи: мобильная форма сохраняет адрес после ошибки и даёт повторить", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const state = await fixture(page, "owner", true);
  await page.goto(`/meetings/${meeting.id}`);
  await page.getByRole("button", { name: "Пригласить участников" }).click();
  const dialog = page.getByRole("dialog", { name: "Пригласить участников" });
  const email = dialog.getByRole("textbox", { name: "Email участников" });
  await email.fill("outside@example.test");
  await dialog
    .getByRole("button", { name: "Пригласить участников", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(email).toHaveValue("outside@example.test");
  await expect(email).toBeEnabled();
  await page.screenshot({
    path: info.outputPath("meeting-invitation-mobile.png"),
    fullPage: true,
  });
  const box = await dialog.boundingBox();
  expect(box!.width).toBeLessThanOrEqual(390);
  expect(box!.height).toBeLessThanOrEqual(844);
  await dialog
    .getByRole("button", { name: "Пригласить участников", exact: true })
    .click();
  await expect(
    dialog.getByText("Приглашение поставлено в очередь", { exact: true }),
  ).toBeVisible();
  expect(state.writes).toHaveLength(2);
});

for (const role of ["participant", "guest"] as const) {
  test(`${role}: на странице встречи доступна ссылка без формы и поиска чужих аккаунтов`, async ({
    page,
  }) => {
    const state = await fixture(page, role);
    await page.goto(`/meetings/${meeting.id}`);
    await expect(
      page.getByRole("button", { name: "Копировать" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Пригласить участников" }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("searchbox", { name: "Добавить пользователей системы" }),
    ).toHaveCount(0);
    expect(state.searches).toHaveLength(0);
    expect(state.writes).toHaveLength(0);
  });
}
