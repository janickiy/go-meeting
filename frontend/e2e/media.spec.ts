import { expect, test } from "@playwright/test";

test.skip(
  !process.env.MEET_MEDIA_CONFERENCE,
  "requires the isolated Stage 3 media harness",
);
test("two browsers exchange audio/video through the SFU and recreate media after reconnect", async ({
  browser,
}, info) => {
  test.skip(
    info.project.name !== "chromium",
    "isolated fake devices are configured for Chromium only",
  );
  test.setTimeout(90000);
  const contexts = await Promise.all([
    browser.newContext(),
    browser.newContext(),
  ]);
  const [alice, bob] = await Promise.all(
    contexts.map((context) => context.newPage()),
  );
  const base = process.env.MEET_LIVE_TEST_URL || "http://127.0.0.1:5175";
  const conference = process.env.MEET_MEDIA_CONFERENCE!;
  const password = process.env.MEET_MEDIA_PASSWORD || "stage-one-test-password";
  const diagnostics: Record<string, unknown>[][] = [[], []];
  for (const [index, page] of [alice, bob].entries()) {
    // The isolated media harness focuses on live controls; storage/recording
    // routes are covered by the real RabbitMQ/MinIO recording integration test.
    await page.route("**/api/v1/conferences/*/recordings", (route) =>
      route.fulfill({ json: { status: "success", items: [] } }),
    );
    if (process.env.MEET_STAGE4)
      await page.addInitScript(() => {
        navigator.mediaDevices.getDisplayMedia = async () => {
          const canvas = document.createElement("canvas");
          canvas.width = 640;
          canvas.height = 360;
          const paint = () => {
            const ctx = canvas.getContext("2d")!;
            ctx.fillStyle = "#1766eb";
            ctx.fillRect(0, 0, 640, 360);
            ctx.fillStyle = "white";
            ctx.font = "36px sans-serif";
            ctx.fillText(`Screen ${Date.now()}`, 25, 170);
          };
          paint();
          const timer = setInterval(paint, 60);
          const stream = canvas.captureStream(15);
          const track = stream.getVideoTracks()[0];
          track.addEventListener("ended", () => clearInterval(timer));
          (window as unknown as { __endShare: () => void }).__endShare = () => {
            track.stop();
            track.dispatchEvent(new Event("ended"));
          };
          return stream;
        };
      });
    await page.addInitScript(() => {
      const target = window as unknown as { __mediaTrackTrace: unknown[] };
      target.__mediaTrackTrace = [];
      const Original = window.RTCPeerConnection;
      window.RTCPeerConnection = new Proxy(Original, {
        construct: (Type, args: [RTCConfiguration?]) => {
          const pc = new Type(...args);
          pc.addEventListener("track", (event) =>
            target.__mediaTrackTrace.push({
              id: event.track.id,
              kind: event.track.kind,
              mid: event.transceiver.mid,
              streams: event.streams.map((stream) => stream.id),
            }),
          );
          return pc;
        },
      });
    });
    page.on("websocket", (socket) =>
      socket.on("framereceived", ({ payload }) => {
        try {
          const event = JSON.parse(String(payload));
          if (event.type?.startsWith("media.") || event.type === "error")
            diagnostics[index].push({
              type: event.type,
              mediaPeerId: event.data?.mediaPeerId,
              code: event.data?.code,
              revision: event.data?.revision,
              tracks: event.data?.tracks?.map(
                (t: { id: string; mediaPeerId: string; kind: string }) => ({
                  id: t.id,
                  mediaPeerId: t.mediaPeerId,
                  kind: t.kind,
                }),
              ),
            });
        } catch {
          /* Native websocket control frames are not JSON. */
        }
      }),
    );
  }
  try {
    for (const [page, email] of [
      [alice, process.env.MEET_MEDIA_ALICE_EMAIL || "owner@stage3.example"],
      [bob, process.env.MEET_MEDIA_BOB_EMAIL || "member@stage3.example"],
    ] as const) {
      await page.goto(`${base}/login`);
      await page.getByLabel("Email").fill(email);
      await page.getByLabel("Пароль", { exact: true }).fill(password);
      await page.getByRole("button", { name: "Войти", exact: true }).click();
      await expect(page).toHaveURL(/\/app/);
      await page.goto(`${base}/conferences/${conference}`);
      await expect(page.getByTestId("connection-id")).toBeVisible();
      await expect(page.getByTestId("local-media")).toHaveCount(0);
      await page
        .getByRole("button", {
          name: "Включить камеру и микрофон",
          exact: true,
        })
        .click();
      await expect(page.getByTestId("local-media")).toBeVisible();
    }
    for (const page of [alice, bob]) {
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
            page
              .getByTestId("remote-media")
              .locator("video")
              .evaluate((node) => {
                const video = node as HTMLVideoElement;
                const stream = video.srcObject as MediaStream | null;
                return (
                  video.videoWidth > 0 &&
                  Boolean(
                    stream
                      ?.getAudioTracks()
                      .some((t) => t.readyState === "live"),
                  ) &&
                  Boolean(
                    stream
                      ?.getVideoTracks()
                      .some((t) => t.readyState === "live"),
                  )
                );
              }),
          { timeout: 30000 },
        )
        .toBe(true);
    }
    if (process.env.MEET_STAGE4) {
      await bob
        .getByRole("button", { name: "Выключить камеру", exact: true })
        .click();
      await expect(
        alice.getByTestId("remote-media").locator("video"),
      ).toHaveCount(0);
      await bob
        .getByRole("button", { name: "Выключить микрофон", exact: true })
        .click();
      await expect(alice.getByTestId("remote-media")).toHaveCount(0);
      await bob
        .getByRole("button", { name: "Включить камеру", exact: true })
        .click();
      await bob
        .getByRole("button", { name: "Включить микрофон", exact: true })
        .click();
      await expect(
        alice.getByTestId("remote-media").locator("video"),
      ).toHaveCount(1);
      await bob.getByLabel("Выбор камеры").selectOption({ index: 1 });
      await expect
        .poll(() =>
          alice
            .getByTestId("remote-media")
            .locator("video")
            .evaluate((v) => (v as HTMLVideoElement).videoWidth),
        )
        .toBeGreaterThan(0);
      await bob
        .getByRole("button", { name: "Показать экран", exact: true })
        .click();
      await expect(alice.getByTestId("remote-media")).toHaveCount(2);
      await expect(alice.locator(".media-tile-screen video")).toHaveCount(1);
      await expect
        .poll(() =>
          alice
            .locator(".media-tile-screen video")
            .evaluate((v) => (v as HTMLVideoElement).videoWidth),
        )
        .toBeGreaterThan(0);
      await alice.screenshot({
        path: info.outputPath("stage4-screen.png"),
        fullPage: true,
      });
      await alice
        .getByRole("button", { name: "Показать экран", exact: true })
        .click();
      await expect(
        alice.getByText("Экран уже показывает другой участник", {
          exact: false,
        }),
      ).toBeVisible();
      await expect(alice.getByTestId("local-media")).toHaveCount(1);
      await expect(alice.getByTestId("media-status")).toHaveText(
        "Медиасвязь подключена",
      );
      await bob.evaluate(() =>
        (window as unknown as { __endShare: () => void }).__endShare(),
      );
      await expect(alice.getByTestId("remote-media")).toHaveCount(1);
      await bob
        .getByRole("button", { name: "Показать экран", exact: true })
        .click();
      await expect(alice.getByTestId("remote-media")).toHaveCount(2);
      await alice
        .getByRole("button", { name: "Отключить экран", exact: true })
        .click();
      await expect(alice.getByTestId("remote-media")).toHaveCount(1);
      await expect(
        bob.getByRole("button", { name: "Показать экран", exact: true }),
      ).toBeDisabled();
      await alice
        .getByRole("button", { name: "Разрешить экран", exact: true })
        .click();
      await expect(
        bob.getByRole("button", { name: "Показать экран", exact: true }),
      ).toBeEnabled();
      await expect(alice.getByTestId("remote-media")).toHaveCount(1);
      await alice
        .getByRole("button", { name: "Отключить микрофон", exact: true })
        .click();
      await expect(
        bob.getByRole("button", { name: "Включить микрофон", exact: true }),
      ).toBeDisabled();
      await alice
        .getByRole("button", { name: "Разрешить микрофон", exact: true })
        .click();
      await expect(
        bob.getByRole("button", { name: "Включить микрофон", exact: true }),
      ).toBeEnabled();
      await bob
        .getByRole("button", { name: "Включить микрофон", exact: true })
        .click();
    }
    await bob.getByText("Состояние медиасвязи", { exact: true }).click();
    const previousPeer = await bob.getByTestId("media-peer-id").textContent();
    await bob
      .getByRole("button", { name: "Переподключиться", exact: true })
      .click();
    await expect(bob.getByTestId("local-media")).toHaveCount(0);
    await expect(bob.getByTestId("media-status")).toHaveText(
      "Камера и микрофон выключены",
    );
    await expect(alice.getByTestId("remote-media")).toHaveCount(0, {
      timeout: 10000,
    });
    await bob
      .getByRole("button", { name: "Включить камеру и микрофон", exact: true })
      .click();
    await expect(bob.getByTestId("media-status")).toHaveText(
      "Медиасвязь подключена",
      { timeout: 30000 },
    );
    if (!(await bob.getByTestId("media-peer-id").isVisible()))
      await bob.getByText("Состояние медиасвязи", { exact: true }).click();
    await expect(bob.getByTestId("media-peer-id")).not.toHaveText(
      previousPeer!,
    );
    await expect(alice.getByTestId("remote-media")).toHaveCount(1, {
      timeout: 30000,
    });
    await bob
      .getByRole("button", { name: "Отключить медиа", exact: true })
      .click();
    await expect(bob.getByTestId("local-media")).toHaveCount(0);
    await expect(alice.getByTestId("remote-media")).toHaveCount(0, {
      timeout: 10000,
    });
    await expect(bob.getByTestId("connection-id")).toBeVisible();
    if (process.env.MEET_STAGE4) {
      await bob
        .getByRole("button", {
          name: "Подключиться без камеры и микрофона",
          exact: true,
        })
        .click();
      await expect(bob.getByTestId("media-status")).toHaveText(
        "Медиасвязь подключена",
      );
      await expect(bob.getByTestId("local-media")).toHaveCount(0);
      await expect(bob.getByTestId("remote-media")).toHaveCount(1);
      await bob
        .getByRole("button", { name: "Включить микрофон", exact: true })
        .click();
      await expect(
        alice.getByTestId("remote-media").locator("audio"),
      ).toHaveCount(1);
      await expect(
        alice.getByTestId("remote-media").locator("video"),
      ).toHaveCount(0);
      await bob
        .getByRole("button", { name: "Отключить медиа", exact: true })
        .click();
    }
  } catch (error) {
    console.error("Safe media diagnostics:", JSON.stringify(diagnostics));
    console.error(
      "Safe received tracks:",
      JSON.stringify(
        await Promise.all(
          [alice, bob].map((page) =>
            page.evaluate(
              () =>
                (window as unknown as { __mediaTrackTrace: unknown[] })
                  .__mediaTrackTrace,
            ),
          ),
        ),
      ),
    );
    throw error;
  } finally {
    await Promise.all(contexts.map((context) => context.close()));
  }
});
