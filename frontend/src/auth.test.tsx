import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { configureAuth } from "./api";
import { AuthProvider, useAuth } from "./auth";
import { SESSION_KEY, saveSession } from "./utils";

afterEach(() => {
  saveSession(null);
  configureAuth(null, () => {});
  vi.unstubAllGlobals();
});

function ProfileHarness() {
  const { user, updateProfile } = useAuth();
  return (
    <div>
      <span data-testid="name">{user?.displayName ?? "loading"}</span>
      <button onClick={() => void updateProfile("Новое имя")}>Изменить</button>
    </div>
  );
}

it("refreshes the public user after profile update while keeping the same JWT", async () => {
  const session = {
    token: "stable-profile-token",
    expiresAt: Date.now() + 60_000,
  };
  saveSession(session);
  const before = {
    id: "person-1",
    email: "person@example.test",
    displayName: "Старое имя",
    isAdmin: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
  const fetch = vi.fn(
    async (input: RequestInfo | URL, options?: RequestInit) => {
      expect(input).toBe("/api/v1/auth/me");
      expect(new Headers(options?.headers).get("Authorization")).toBe(
        "Bearer stable-profile-token",
      );
      if (options?.method === "PATCH") {
        expect(JSON.parse(String(options.body))).toEqual({
          displayName: "Новое имя",
        });
        return new Response(
          JSON.stringify({
            status: "success",
            user: { ...before, displayName: "Новое имя" },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ status: "success", user: before }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <ProfileHarness />
      </AuthProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("Старое имя")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Изменить" }));
  expect(await screen.findByText("Новое имя")).toBeInTheDocument();
  expect(JSON.parse(sessionStorage.getItem(SESSION_KEY) || "null")).toEqual(
    session,
  );
  expect(fetch).toHaveBeenCalledTimes(2);
  view.unmount();
  client.clear();
});
