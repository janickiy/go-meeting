import { expect, test } from "@playwright/test";

test("frontend exposes only public immutable build metadata", async ({
  request,
}) => {
  const response = await request.get("/version.json");
  expect(response.ok()).toBe(true);
  const data = await response.json();
  expect(Object.keys(data).sort()).toEqual(["buildTime", "commit", "version"]);
  expect(data.version).toMatch(/^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$/);
});

test("opt-in error telemetry removes sensitive content before transport", async ({
  page,
}) => {
  test.skip(
    process.env.VITE_CLIENT_TELEMETRY_ENABLED !== "true",
    "Requires explicit telemetry build opt-in.",
  );
  const events: { body: string; headers: Record<string, string> }[] = [];
  await page.route("**/api/v1/client-errors", async (route) => {
    events.push({
      body: route.request().postData() || "",
      headers: route.request().headers(),
    });
    await route.fulfill({
      status: 202,
      contentType: "application/json",
      body: "{}",
    });
  });
  await page.goto("/");
  await page.evaluate(() => {
    history.pushState({}, "", "/i/private-invite?token=private-token");
    const error = new Error("private-chat private-transcript private-token");
    error.stack = `Error: private-chat\n at privateFunction (${location.origin}/assets/index-abcdefgh.js:4:8)\n at privateFunction (https://storage.example/private?signature=private-token:1:2)`;
    window.dispatchEvent(new ErrorEvent("error", { error }));
  });
  await expect.poll(() => events.length).toBe(1);
  const event = events[0];
  expect(event.body).not.toContain("private");
  expect(event.headers.authorization).toBeUndefined();
  expect(event.headers.cookie).toBeUndefined();
  expect(event.headers.referer).toBeUndefined();
  expect(JSON.parse(event.body)).toMatchObject({
    route: "/i/:code",
    code: "uncaught_error",
    stack: [{ file: "index-abcdefgh.js", line: 4, column: 8 }],
  });
});
