import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, configureAuth, getAccessToken } from "./api";
import { AuthProvider, useAuth } from "./auth";
import { LOGOUT_KEY, SESSION_KEY, readSession, saveSession } from "./utils";

const person = {
  id: "person-1",
  email: "person@example.test",
  displayName: "Старое имя",
  isAdmin: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function account(token = "persistent-access") {
  return {
    status: "success",
    accessToken: token,
    expiresIn: 3600,
    user: person,
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function Harness() {
  const auth = useAuth();
  return (
    <>
      <span data-testid="name">{auth.user?.displayName ?? "signed out"}</span>
      <span data-testid="loading">{String(auth.loading)}</span>
      <span data-testid="error">{String(auth.startupError)}</span>
      <button onClick={() => void auth.updateProfile("Новое имя")}>
        Изменить
      </button>
      <button onClick={() => void auth.logout()}>Выйти</button>
      <button
        onClick={() => void auth.login("person@example.test", "password-test")}
      >
        Войти
      </button>
    </>
  );
}
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { gcTime: Infinity } },
  });
  client.setQueryData(["meeting-preserved"], "still mounted");
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <AuthProvider>
          <Harness />
        </AuthProvider>
      </QueryClientProvider>,
    ),
  };
}
function stored(token = "legacy-access", expiresAt = Date.now() + 3600_000) {
  saveSession({ token, expiresAt, user: person });
}
async function ready() {
  await waitFor(() =>
    expect(screen.getByTestId("loading")).toHaveTextContent("false"),
  );
  expect(screen.getByTestId("name")).toHaveTextContent(person.displayName);
}
function standardFetch(
  override?: (
    path: string,
    options?: RequestInit,
  ) => Promise<Response> | Response | undefined,
) {
  const fetch = vi.fn(
    async (input: RequestInfo | URL, options?: RequestInit) => {
      const path = String(input);
      const result = override?.(path, options);
      if (result) return result;
      if (path === "/api/v1/auth/me")
        return json({ status: "success", user: person });
      if (path === "/api/v1/auth/session" || path === "/api/v1/auth/login")
        return json(account());
      if (path === "/api/v1/auth/refresh")
        return json(account("renewed-access"));
      if (path === "/api/v1/auth/logout")
        return new Response(null, { status: 204 });
      throw new Error(`Unexpected request ${path}`);
    },
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
afterEach(() => {
  cleanup();
  saveSession(null);
  localStorage.removeItem(LOGOUT_KEY);
  configureAuth(null, () => false);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("migrates the old JWT once and updates the profile without resetting its renewed access token", async () => {
  stored();
  const fetch = standardFetch((path, options) => {
    if (path === "/api/v1/auth/me" && options?.method === "PATCH") {
      expect(new Headers(options.headers).get("Authorization")).toBe(
        "Bearer persistent-access",
      );
      return json({
        status: "success",
        user: { ...person, displayName: "Новое имя" },
      });
    }
  });
  mount();
  await ready();
  fireEvent.click(screen.getByRole("button", { name: "Изменить" }));
  await screen.findByText("Новое имя");
  expect(readSession()?.token).toBe("persistent-access");
  expect(readSession()?.user?.displayName).toBe("Новое имя");
  expect(fetch.mock.calls.map(([path]) => path)).toEqual([
    "/api/v1/auth/me",
    "/api/v1/auth/session",
    "/api/v1/auth/me",
  ]);
});

it("restores an expired access token from the HttpOnly cookie", async () => {
  stored("expired-access", Date.now() - 86_400_000);
  const fetch = standardFetch();
  mount();
  await ready();
  expect(getAccessToken()).toBe("renewed-access");
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][0]).toBe("/api/v1/auth/refresh");
  expect(fetch.mock.calls[0][1]?.credentials).toBe("same-origin");
  expect(
    new Headers(fetch.mock.calls[0][1]?.headers).has("Authorization"),
  ).toBe(false);
  expect(localStorage.getItem(SESSION_KEY)).toBeNull();
});

it("silently renews before one hour and preserves account, loading and query cache", async () => {
  vi.useFakeTimers();
  stored();
  const fetch = standardFetch();
  const view = mount();
  await act(async () => {
    await Promise.resolve();
  });
  expect(screen.getByTestId("loading")).toHaveTextContent("false");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(3600_000);
  });
  expect(screen.getByTestId("name")).toHaveTextContent(person.displayName);
  expect(screen.getByTestId("loading")).toHaveTextContent("false");
  expect(getAccessToken()).toBe("renewed-access");
  expect(view.client.getQueryData(["meeting-preserved"])).toBe("still mounted");
  expect(
    fetch.mock.calls.filter(([path]) => path === "/api/v1/auth/refresh"),
  ).toHaveLength(1);
});

it("recovers after a suspended tab crosses expiry without waiting for its timer", async () => {
  stored();
  const fetch = standardFetch();
  mount();
  await ready();
  vi.spyOn(Date, "now").mockReturnValue(Date.now() + 3 * 3600_000);
  await act(async () => {
    window.dispatchEvent(new Event("focus"));
  });
  expect(getAccessToken()).toBe("renewed-access");
  expect(screen.getByTestId("name")).toHaveTextContent(person.displayName);
  expect(
    fetch.mock.calls.filter(([path]) => path === "/api/v1/auth/refresh"),
  ).toHaveLength(1);
});

it("shares a single renewal across concurrent 401s and retries each operation once", async () => {
  stored();
  const refresh = deferred<Response>();
  const fetch = standardFetch((path, options) => {
    if (path === "/api/v1/auth/refresh") return refresh.promise;
    if (
      path === "/api/v1/auth/me" &&
      new Headers(options?.headers).get("Authorization") ===
        "Bearer persistent-access"
    )
      return json({ message: "expired" }, 401);
  });
  mount();
  await ready();
  let first!: Promise<unknown>, second!: Promise<unknown>;
  await act(async () => {
    first = api.me();
    second = api.me();
  });
  expect(screen.getByTestId("name")).toHaveTextContent(person.displayName);
  expect(screen.getByTestId("loading")).toHaveTextContent("false");
  expect(
    fetch.mock.calls.filter(([path]) => path === "/api/v1/auth/refresh"),
  ).toHaveLength(1);
  await act(async () => {
    refresh.resolve(json(account("next-access")));
    await Promise.all([first, second]);
  });
  expect(getAccessToken()).toBe("next-access");
  expect(
    fetch.mock.calls.filter(
      ([path, options]) =>
        path === "/api/v1/auth/me" &&
        new Headers(options?.headers).get("Authorization") ===
          "Bearer next-access",
    ),
  ).toHaveLength(2);
});

it.each(["network", "server"])(
  "keeps a cached account on %s failure and renews when online",
  async (failure) => {
    stored("expired-access", Date.now() - 1000);
    let unavailable = true;
    standardFetch((path) => {
      if (path === "/api/v1/auth/refresh" && unavailable) {
        if (failure === "network")
          return Promise.reject(new TypeError("offline"));
        return json({ message: "temporarily unavailable" }, 503);
      }
    });
    const view = mount();
    await ready();
    expect(readSession()?.token).toBe("expired-access");
    expect(screen.getByTestId("error")).toHaveTextContent("false");
    expect(view.client.getQueryData(["meeting-preserved"])).toBe(
      "still mounted",
    );
    unavailable = false;
    await act(async () => {
      window.dispatchEvent(new Event("online"));
    });
    expect(getAccessToken()).toBe("renewed-access");
  },
);

it("clears only after the cookie refresh confirms that the server session is gone", async () => {
  stored("expired-access", Date.now() - 1000);
  standardFetch((path) =>
    path === "/api/v1/auth/refresh"
      ? json({ message: "unauthorized" }, 401)
      : undefined,
  );
  mount();
  await waitFor(() =>
    expect(screen.getByTestId("name")).toHaveTextContent("signed out"),
  );
  expect(getAccessToken()).toBeNull();
  expect(readSession()).toBeNull();
});

it("explicit logout clears immediately, sends captured Bearer and fences a late refresh", async () => {
  stored();
  const refresh = deferred<Response>();
  const logout = deferred<Response>();
  const fetch = standardFetch((path, options) => {
    if (path === "/api/v1/auth/refresh") return refresh.promise;
    if (path === "/api/v1/auth/logout") {
      expect(new Headers(options?.headers).get("Authorization")).toBe(
        "Bearer persistent-access",
      );
      return logout.promise;
    }
    if (
      path === "/api/v1/auth/me" &&
      new Headers(options?.headers).get("Authorization") ===
        "Bearer persistent-access"
    )
      return json({}, 401);
  });
  mount();
  await ready();
  let request!: Promise<unknown>;
  await act(async () => {
    request = api.me().catch(() => {});
  });
  fireEvent.click(screen.getByRole("button", { name: "Выйти" }));
  expect(screen.getByTestId("name")).toHaveTextContent("signed out");
  expect(localStorage.getItem(LOGOUT_KEY)).not.toBeNull();
  expect(readSession()).toBeNull();
  await act(async () => {
    refresh.resolve(json(account("late-access")));
    logout.resolve(new Response(null, { status: 204 }));
    await request;
  });
  expect(getAccessToken()).toBeNull();
  expect(screen.getByTestId("name")).toHaveTextContent("signed out");
  expect(
    fetch.mock.calls.filter(
      ([path, options]) =>
        path === "/api/v1/auth/me" &&
        new Headers(options?.headers).get("Authorization") ===
          "Bearer late-access",
    ),
  ).toHaveLength(0);
});

it("synchronizes explicit logout between tabs without storing an access token in localStorage", async () => {
  stored();
  standardFetch();
  mount();
  await ready();
  await act(async () =>
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: LOGOUT_KEY,
        newValue: "logout-from-another-tab",
      }),
    ),
  );
  expect(screen.getByTestId("name")).toHaveTextContent("signed out");
  expect(getAccessToken()).toBeNull();
  expect(readSession()).toBeNull();
  expect(localStorage.getItem(SESSION_KEY)).toBeNull();
});

it("retries an offline logout on startup and never restores the stale cookie until explicit login", async () => {
  localStorage.setItem(LOGOUT_KEY, "offline-logout");
  stored();
  const fetch = standardFetch();
  mount();
  await waitFor(() =>
    expect(screen.getByTestId("loading")).toHaveTextContent("false"),
  );
  expect(screen.getByTestId("name")).toHaveTextContent("signed out");
  expect(fetch.mock.calls.map(([path]) => path)).toEqual([
    "/api/v1/auth/logout",
  ]);
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));
  await ready();
  expect(localStorage.getItem(LOGOUT_KEY)).toBeNull();
  expect(getAccessToken()).toBe("persistent-access");
});

it("keeps a scoped guest token separate from persistent account restoration", async () => {
  const guest = { ...person, email: "", guestConferenceId: "guest-room" };
  saveSession({ token: "scoped-guest", expiresAt: Date.now() + 3600_000 });
  const fetch = standardFetch((path) =>
    path === "/api/v1/auth/me" ? json({ user: guest }) : undefined,
  );
  mount();
  await ready();
  expect(fetch.mock.calls.map(([path]) => path)).toEqual(["/api/v1/auth/me"]);
  expect(readSession()?.user?.guestConferenceId).toBe("guest-room");
});

it("retries legacy cookie migration after a transport error without logging out", async () => {
  stored();
  let unavailable = true;
  const fetch = standardFetch((path) => {
    if (path === "/api/v1/auth/session" && unavailable)
      return Promise.reject(new TypeError("offline"));
  });
  mount();
  await ready();
  expect(getAccessToken()).toBe("legacy-access");
  unavailable = false;
  await act(async () => {
    window.dispatchEvent(new Event("online"));
  });
  expect(getAccessToken()).toBe("persistent-access");
  expect(
    fetch.mock.calls.filter(([path]) => path === "/api/v1/auth/refresh"),
  ).toHaveLength(0);
});

it("aborts a pending login on logout and discards it before canceling newer queries", async () => {
  const login = deferred<Response>();
  let signal: AbortSignal | undefined;
  standardFetch((path, options) => {
    if (path === "/api/v1/auth/login") {
      signal = options?.signal as AbortSignal;
      return login.promise;
    }
    if (path === "/api/v1/auth/refresh") return json({}, 401);
  });
  const view = mount();
  const cancel = vi.spyOn(view.client, "cancelQueries");
  await waitFor(() =>
    expect(screen.getByTestId("loading")).toHaveTextContent("false"),
  );
  fireEvent.click(screen.getByRole("button", { name: "Войти" }));
  await waitFor(() => expect(signal).toBeDefined());
  fireEvent.click(screen.getByRole("button", { name: "Выйти" }));
  expect(signal?.aborted).toBe(true);
  const canceledBeforeResponse = cancel.mock.calls.length;
  view.client.setQueryData(["after-logout"], "retain this newer query");
  await act(async () => {
    login.resolve(json(account("late-login-access")));
  });
  expect(getAccessToken()).toBeNull();
  expect(screen.getByTestId("name")).toHaveTextContent("signed out");
  expect(cancel).toHaveBeenCalledTimes(canceledBeforeResponse);
  expect(view.client.getQueryData(["after-logout"])).toBe(
    "retain this newer query",
  );
  expect(localStorage.getItem(LOGOUT_KEY)).not.toBeNull();
});
