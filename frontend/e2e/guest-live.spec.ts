import { expect, test, type BrowserContext } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";

const origin = process.env.MEET_GUEST_LIVE_ORIGIN;
test.skip(
  !origin,
  "Set MEET_GUEST_LIVE_ORIGIN for an explicitly selected test host",
);

test("default camera and microphone, immediate guest admission, real media and chat", async ({
  browser,
  request,
}, info) => {
  test.setTimeout(120_000);
  const namespace = `guest-smoke-${randomUUID()}`;
  const password = randomUUID();
  let token = "",
    room = "";
  const contexts: BrowserContext[] = [];
  async function api(
    path: string,
    method = "GET",
    data?: unknown,
    authenticated = true,
  ) {
    const response = await request.fetch(`${origin}/api/v1${path}`, {
      method,
      data,
      headers: authenticated ? { Authorization: `Bearer ${token}` } : {},
    });
    expect(response.status(), `${method} ${path}`).toBeLessThan(300);
    return response.json();
  }
  try {
    await api(
      "/auth/register",
      "POST",
      {
        email: `${namespace}@example.test`,
        password,
        displayName: "Проверка организатора",
      },
      false,
    );
    token = (
      await api(
        "/auth/login",
        "POST",
        { email: `${namespace}@example.test`, password },
        false,
      )
    ).accessToken;
    const created = await api("/conferences", "POST", {
      title: namespace,
      waitingRoomEnabled: true,
    });
    room = created.item.id;
    await writeFile(
      info.outputPath("fixture.json"),
      JSON.stringify({ room, namespace }),
    );
    await api(`/conferences/${room}/join`, "POST", {});
    await api(`/conferences/${room}/start`, "POST", {});
    const ownerContext = await browser.newContext({
      permissions: ["camera", "microphone"],
    });
    const guestContext = await browser.newContext({
      permissions: ["camera", "microphone"],
    });
    contexts.push(ownerContext, guestContext);
    await ownerContext.addInitScript(
      (value) =>
        sessionStorage.setItem(
          "meet.session.v1",
          JSON.stringify({ token: value, expiresAt: Date.now() + 3600000 }),
        ),
      token,
    );
    for (const context of contexts)
      await context.addInitScript(() => {
        const Native = window.RTCPeerConnection;
        const peers: RTCPeerConnection[] = [];
        (window as unknown as { smokePeers: RTCPeerConnection[] }).smokePeers =
          peers;
        window.RTCPeerConnection = class extends Native {
          constructor(config?: RTCConfiguration) {
            super(config);
            peers.push(this);
          }
        };
      });
    const owner = await ownerContext.newPage();
    await owner.goto(`${origin}/conferences/${room}`);
    const guest = await guestContext.newPage();
    await guest.goto(`${origin}/i/${created.item.inviteCode}`);
    await expect(guest.getByLabel("Имя на встрече")).toHaveValue("Гость");
    await guest.getByLabel("Имя на встрече").fill("Гость проверки");
    await expect(
      guest.getByRole("button", { name: "Выключить микрофон", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      guest.getByRole("button", { name: "Выключить камеру", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect
      .poll(() =>
        guest
          .locator(".prejoin-video-frame video")
          .evaluate((video: HTMLVideoElement) => video.videoWidth),
      )
      .toBeGreaterThan(0);
    await guest.screenshot({
      path: info.outputPath("guest-camera-preview.png"),
      fullPage: true,
    });
    await guest
      .getByRole("button", { name: "Подключиться", exact: true })
      .click();
    await expect(guest).toHaveURL(`${origin}/conferences/${room}`);
    await expect(guest.getByTestId("waiting-room")).toHaveCount(0);
    const members = await api(`/conferences/${room}/participants`);
    const member = members.items.find(
      (item: { displayName: string }) => item.displayName === "Гость проверки",
    );
    expect(member.status).toBe("joined");
    expect(member.admissionState).toBe("admitted");
    const start = owner.getByRole("button", {
      name: "Включить камеру и микрофон",
      exact: true,
    });
    await expect(start).toBeEnabled({ timeout: 15000 });
    await start.click();
    for (const page of [owner, guest]) {
      await expect(page.getByTestId("media-status")).toHaveText(
        "Медиасвязь подключена",
        { timeout: 30000 },
      );
      await expect(page.getByTestId("remote-media")).toHaveCount(1, {
        timeout: 30000,
      });
      await expect
        .poll(
          () =>
            page.evaluate(async () => {
              const peers = (
                window as unknown as { smokePeers: RTCPeerConnection[] }
              ).smokePeers;
              let audio = false,
                video = false;
              for (const peer of peers)
                for (const stat of (await peer.getStats()).values()) {
                  if (stat.type !== "inbound-rtp") continue;
                  if (stat.kind === "video" && stat.framesDecoded > 5)
                    video = true;
                  if (stat.kind === "audio" && stat.packetsReceived > 10)
                    audio = true;
                }
              return audio && video;
            }),
          { timeout: 20000 },
        )
        .toBe(true);
    }
    await guest
      .getByLabel("Сообщение", { exact: true })
      .fill("Сообщение гостя");
    await guest.getByRole("button", { name: "Отправить", exact: true }).click();
    await expect(owner.getByRole("log")).toContainText("Сообщение гостя");
    const guestToken = await guest.evaluate(
      () => JSON.parse(sessionStorage.getItem("meet.session.v1") || "{}").token,
    );
    expect(
      (
        await request.get(`${origin}/api/v1/me/conferences`, {
          headers: { Authorization: `Bearer ${guestToken}` },
        })
      ).status(),
    ).toBe(403);
    await guest.screenshot({
      path: info.outputPath("guest-in-meeting.png"),
      fullPage: true,
    });
  } finally {
    if (room)
      await api(`/conferences/${room}/finish`, "POST", {}).catch(() => {});
    for (const context of contexts) await context.close();
  }
});
