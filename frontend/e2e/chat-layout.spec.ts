import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

/** openChat загружает автономный макет без записи в API проекта. */
async function openChat(page: Page) {
  // Settle the original application's anonymous startup before mounting a
  // second AuthProvider; a late 401 must not clear the fixture's shared session.
  await page.route("**/api/v1/auth/refresh", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ status: "error", message: "unauthorized" }),
    }),
  );
  await page.addInitScript(() => {
    // HTTP-адрес host.docker.internal тестового Firefox не является secure context.
    // Только эта автономная вкладка получает UUID v4 на CSPRNG; HTTPS-приложение не меняется.
    if (typeof crypto.randomUUID === "function") return;
    Object.defineProperty(crypto, "randomUUID", {
      value: () => {
        const bytes = crypto.getRandomValues(new Uint8Array(16));
        bytes[6] = (bytes[6] & 0x0f) | 0x40;
        bytes[8] = (bytes[8] & 0x3f) | 0x80;
        const hex = Array.from(bytes, (value) =>
          value.toString(16).padStart(2, "0"),
        ).join("");
        return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
      },
    });
  });
  await page.goto("/login");
  await page.waitForLoadState("networkidle");
  await page.evaluate(async () => {
    const fixturePath = "/e2e/helpers/chat-fixture.tsx";
    const fixture = await import(fixturePath);
    fixture.install();
  });
  await expect(page.getByTestId("chat-message-own-short")).toBeVisible();
}

/** getSent читает только тестовый локальный журнал отправок. */
async function getSent(page: Page) {
  return page.evaluate(async () => {
    const fixturePath = "/e2e/helpers/chat-fixture.tsx";
    const fixture = await import(fixturePath);
    return fixture.getSent() as Array<{ text: string; replyTo?: string }>;
  });
}

const sizes = [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
  { name: "landscape", width: 844, height: 390 },
];

for (const size of sizes) {
  test(`правый чат и палитра не обрезаются: ${size.name}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    await openChat(page);
    const rail = page.locator(".conference-stage-rail");
    const composer = page.locator(".chat-composer");
    const log = page.getByRole("log", { name: "Сообщения встречи" });
    const initial = await composer.boundingBox();
    expect(initial).not.toBeNull();
    const ownShort = page.getByTestId("chat-message-own-short");
    const ownArticleBox = (await ownShort.boundingBox())!;
    const ownBubbleBox = (await ownShort
      .locator(".chat-message-bubble")
      .boundingBox())!;
    expect(
      ownArticleBox.x +
        ownArticleBox.width -
        ownBubbleBox.x -
        ownBubbleBox.width,
      "Свой короткий пузырь должен оставаться у правого края на любом экране",
    ).toBeLessThan(2);
    const railBox = (await rail.boundingBox())!;
    expect(railBox.x).toBeGreaterThanOrEqual(0);
    expect(railBox.y).toBeGreaterThanOrEqual(0);
    expect(railBox.x + railBox.width).toBeLessThanOrEqual(size.width + 1);
    expect(railBox.y + railBox.height).toBeLessThanOrEqual(size.height + 1);
    expect(initial!.y + initial!.height).toBeLessThanOrEqual(
      railBox.y + railBox.height,
    );
    expect(
      railBox.y + railBox.height - initial!.y - initial!.height,
    ).toBeLessThan(4);

    await log.evaluate((element) => {
      element.scrollTop = 0;
    });
    expect((await composer.boundingBox())!.y).toBeCloseTo(initial!.y, 1);
    await log.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(size.width);
    await page.screenshot({
      path: info.outputPath(`chat-${size.name}.png`),
      fullPage: true,
    });

    await page.getByRole("button", { name: "Добавить смайлик" }).click();
    const picker = page.getByRole("dialog", { name: "Смайлики" });
    await expect(picker).toBeVisible();
    await expect(
      picker
        .getByRole("group", { name: "Выберите смайлик" })
        .getByRole("button"),
    ).toHaveCount(64);
    const popupBox = (await picker.boundingBox())!;
    expect(popupBox.x).toBeGreaterThanOrEqual(railBox.x);
    expect(popupBox.y).toBeGreaterThanOrEqual(railBox.y);
    expect(popupBox.x + popupBox.width).toBeLessThanOrEqual(
      railBox.x + railBox.width,
    );
    expect(popupBox.y + popupBox.height).toBeLessThanOrEqual(
      railBox.y + railBox.height,
    );
    const headingVisible = await picker
      .locator(".emoji-picker-heading")
      .evaluate((heading) => {
        const box = heading.getBoundingClientRect();
        const top = document.elementFromPoint(
          box.x + 8,
          box.y + box.height / 2,
        );
        return !!top && heading.contains(top);
      });
    expect(
      headingVisible,
      "Заголовок палитры не должен обрезаться контейнерами чата",
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`chat-picker-${size.name}.png`),
      fullPage: true,
    });
    await picker.getByRole("button", { name: "Улыбка", exact: true }).click();
    await expect(page.getByLabel("Сообщение", { exact: true })).toHaveValue(
      "😀",
    );
    await expect(page.getByLabel("Сообщение", { exact: true })).toBeFocused();
    await page.getByRole("button", { name: "Добавить смайлик" }).click();
    await page
      .getByRole("dialog", { name: "Смайлики" })
      .getByRole("button", { name: "Улыбающийся кот", exact: true })
      .click();
    await expect(page.getByLabel("Сообщение", { exact: true })).toHaveValue(
      "😀😺",
    );
  });
}

test("landscape: ответ, многострочный текст и 5 готовых файлов не закрывают управление", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 844, height: 390 });
  await openChat(page);
  await page
    .getByTestId("chat-message-incoming-short")
    .getByRole("button", { name: "Ответить", exact: true })
    .click();
  await page.getByLabel("Выбрать файлы для сообщения").setInputFiles(
    Array.from({ length: 5 }, (_, index) => ({
      name: `Материалы-проекта-длинное-название-${index + 1}.pdf`,
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4 isolated-layout-fixture"),
    })),
  );
  await expect(
    page.getByText("Готов к отправке", { exact: false }),
  ).toHaveCount(5);
  const textarea = page.getByLabel("Сообщение", { exact: true });
  await textarea.fill(
    "Первая строка\nВторая строка\nТретья строка\nЧетвёртая строка\nПятая строка\nШестая строка",
  );
  const details = page.locator(".chat-composer-details");
  const metrics = await details.evaluate((element) => ({
    height: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(
    metrics.height,
    "Прокручиваемая очередь должна оставаться доступной",
  ).toBeGreaterThanOrEqual(24);
  expect(metrics.scrollHeight).toBeGreaterThan(metrics.height);
  await details.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect(
    page.getByRole("button", {
      name: "Убрать файл Материалы-проекта-длинное-название-5.pdf",
    }),
  ).toBeInViewport();
  const rail = (await page.locator(".conference-stage-rail").boundingBox())!;
  const send = page.getByRole("button", { name: "Отправить", exact: true });
  const sendBox = (await send.boundingBox())!;
  expect(sendBox.y + sendBox.height).toBeLessThanOrEqual(rail.y + rail.height);
  await expect(send).toBeEnabled();
  await page.getByRole("button", { name: "Добавить смайлик" }).click();
  const picker = page.getByRole("dialog", { name: "Смайлики" });
  const headingVisible = await picker
    .locator(".emoji-picker-heading")
    .evaluate((heading) => {
      const box = heading.getBoundingClientRect();
      const hit = document.elementFromPoint(box.x + 8, box.y + box.height / 2);
      return !!hit && heading.contains(hit);
    });
  expect(
    headingVisible,
    "Заголовок палитры должен быть доступен при заполненном черновике",
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("chat-landscape-five-files.png"),
    fullPage: true,
  });
  await picker.getByRole("button", { name: "Улыбка", exact: true }).click();
  await send.click();
  await expect(page.getByTestId("chat-message-sent-1")).toContainText(
    "Материалы-проекта-длинное-название-5.pdf",
  );
  await expect(
    page.locator(".chat-upload-details .upload-list li"),
  ).toHaveCount(0);
});

test("пузыри слева/справа, выделение и клавиши отправки сохраняют семантику чата", async ({
  page,
}, info) => {
  await openChat(page);
  const own = page.getByTestId("chat-message-own-short");
  const incoming = page.getByTestId("chat-message-incoming-short");
  const ownBox = (await own.boundingBox())!;
  const incomingBox = (await incoming.boundingBox())!;
  expect(ownBox.x).toBeGreaterThan(incomingBox.x);
  expect(
    await own.evaluate((element) => getComputedStyle(element).alignSelf),
  ).toBe("flex-end");
  expect(
    await incoming.evaluate((element) => getComputedStyle(element).alignSelf),
  ).toBe("flex-start");
  await expect(incoming.locator(".chat-message-avatar")).toBeVisible();
  await expect(
    page.getByTestId("chat-message-own-reply").locator("blockquote"),
  ).toHaveText("МарияТеперь можно использовать любой фон!");

  const textarea = page.getByLabel("Сообщение", { exact: true });
  await textarea.fill("A🙂BC");
  await textarea.evaluate((element: HTMLTextAreaElement) => {
    element.focus();
    element.setSelectionRange(3, 4);
    element.dispatchEvent(new Event("select", { bubbles: true }));
  });
  await page.getByRole("button", { name: "Добавить смайлик" }).click();
  await page
    .getByRole("dialog", { name: "Смайлики" })
    .getByRole("button", { name: "Улыбка", exact: true })
    .click();
  await expect(textarea).toHaveValue("A🙂😀C");
  expect(
    await textarea.evaluate(
      (element: HTMLTextAreaElement) => element.selectionStart,
    ),
  ).toBe(5);
  await textarea.press("Shift+Enter");
  await expect(textarea).toHaveValue("A🙂😀\nC");
  expect(await getSent(page)).toHaveLength(0);
  await textarea.press("Enter");
  await expect(page.getByTestId("chat-message-sent-1")).toBeVisible();
  expect(await getSent(page)).toEqual([
    expect.objectContaining({ text: "A🙂😀\nC" }),
  ]);
  await expect(textarea).toHaveValue("");

  await incoming.getByRole("button", { name: "Ответить", exact: true }).click();
  await expect(page.getByText("Ответ: Мария", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Добавить смайлик" }).click();
  await page
    .getByRole("dialog", { name: "Смайлики" })
    .getByRole("button", { name: "Огонь", exact: true })
    .click();
  await textarea.press("Enter");
  await expect(
    page.getByTestId("chat-message-sent-2").locator("blockquote"),
  ).toHaveText("МарияДа, договорились.");
  expect((await getSent(page))[1]).toEqual(
    expect.objectContaining({ text: "🔥", replyTo: "incoming-short" }),
  );
  await page.screenshot({
    path: info.outputPath("chat-reply-and-emoji.png"),
    fullPage: true,
  });
});
