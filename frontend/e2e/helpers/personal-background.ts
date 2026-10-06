import type { Route } from "@playwright/test";
// Layout now has a global message stream. Isolated conference/account fixtures
// acknowledge those background requests without using the real API or a fake WS.
export async function personalBackground(route: Route, path: string) {
  if (path === "/conversations") {
    await route.fulfill({
      json: { status: "success", items: [], unreadCount: 0, nextCursor: null },
    });
    return true;
  }
  if (path === "/ws-ticket") {
    await route.fulfill({
      status: 503,
      json: { message: "global stream disabled in isolated UI fixture" },
    });
    return true;
  }
  return false;
}
