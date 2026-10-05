import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  AppearanceProvider,
  appearanceStorageKey,
  TEXT_SIZE_OPTIONS,
  useAppearance,
} from "./appearance";
import type { User } from "./types";

const auth = vi.hoisted(() => ({ user: null as User | null }));
vi.mock("./auth", () => ({ useAuth: () => auth }));

const first: User = {
  id: "person/first",
  email: "first@example.test",
  displayName: "Первый",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
const second: User = {
  ...first,
  id: "person-second",
  email: "second@example.test",
};

function Harness({
  capture,
}: {
  capture?: (value: ReturnType<typeof useAppearance>) => void;
}) {
  const value = useAppearance();
  capture?.(value);
  return (
    <>
      <output>
        {value.theme}:{value.textSize}:{String(value.persistenceError)}
      </output>
      <button onClick={() => value.setTheme("dark")}>Тёмная</button>
      <button onClick={() => value.setTheme("light")}>Светлая</button>
      <button onClick={() => value.setTextSize(125)}>125%</button>
    </>
  );
}

function tree(capture?: (value: ReturnType<typeof useAppearance>) => void) {
  return (
    <AppearanceProvider>
      <Harness capture={capture} />
    </AppearanceProvider>
  );
}

beforeEach(() => {
  auth.user = first;
  localStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  delete document.documentElement.dataset.theme;
  document.documentElement.style.removeProperty("--text-scale");
});

it("uses light and 100% by default without altering root font size", () => {
  document.documentElement.style.fontSize = "16px";
  render(tree());
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  expect(document.documentElement.dataset.theme).toBe("light");
  expect(document.documentElement.style.getPropertyValue("--text-scale")).toBe(
    "1",
  );
  expect(document.documentElement.style.fontSize).toBe("16px");
  document.documentElement.style.removeProperty("font-size");
  expect(TEXT_SIZE_OPTIONS).toEqual([75, 90, 100, 110, 125, 150, 200]);
  expect(localStorage.length).toBe(0);
});

it("applies both preferences immediately and restores them after remount", () => {
  const view = render(tree());
  fireEvent.click(screen.getByRole("button", { name: "Тёмная" }));
  fireEvent.click(screen.getByRole("button", { name: "125%" }));
  expect(document.documentElement.dataset.theme).toBe("dark");
  expect(document.documentElement.style.getPropertyValue("--text-scale")).toBe(
    "1.25",
  );
  expect(
    JSON.parse(localStorage.getItem(appearanceStorageKey(first.id))!),
  ).toEqual({ version: 1, theme: "dark", textSize: 125 });
  view.unmount();
  render(tree());
  expect(screen.getByRole("status")).toHaveTextContent("dark:125:false");
});

it("isolates accounts, resets anonymous styling, and rejects old handlers", () => {
  let oldActions: ReturnType<typeof useAppearance> | undefined;
  const view = render(
    tree((value) => {
      oldActions = value;
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Тёмная" }));
  fireEvent.click(screen.getByRole("button", { name: "125%" }));
  const firstActions = oldActions!;
  auth.user = second;
  view.rerender(tree());
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  act(() => firstActions.setTheme("dark"));
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  expect(localStorage.getItem(appearanceStorageKey(second.id))).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "125%" }));
  auth.user = null;
  view.rerender(tree());
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  expect(document.documentElement.dataset.theme).toBe("light");
  expect(document.documentElement.style.getPropertyValue("--text-scale")).toBe(
    "1",
  );
  auth.user = first;
  view.rerender(tree());
  expect(screen.getByRole("status")).toHaveTextContent("dark:125:false");
});

it("does not read or save account preferences for guests or anonymous visitors", () => {
  localStorage.setItem(
    appearanceStorageKey(first.id),
    JSON.stringify({ version: 1, theme: "dark", textSize: 200 }),
  );
  auth.user = { ...first, guestConferenceId: "guest-conference" };
  const view = render(tree());
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  fireEvent.click(screen.getByRole("button", { name: "Тёмная" }));
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  auth.user = null;
  view.rerender(tree());
  fireEvent.click(screen.getByRole("button", { name: "125%" }));
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  expect(
    JSON.parse(localStorage.getItem(appearanceStorageKey(first.id))!).textSize,
  ).toBe(200);
});

it.each([
  "bad json",
  "null",
  "[]",
  JSON.stringify({ version: 2, theme: "dark", textSize: 125 }),
  JSON.stringify({ version: 1, theme: "sepia", textSize: "125" }),
])("falls back safely for invalid stored values %s", (raw) => {
  localStorage.setItem(appearanceStorageKey(first.id), raw);
  render(tree());
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
});

it("validates individual values and ignores unsupported setter inputs", () => {
  localStorage.setItem(
    appearanceStorageKey(first.id),
    JSON.stringify({ version: 1, theme: "dark", textSize: 999 }),
  );
  let actions: ReturnType<typeof useAppearance> | undefined;
  render(
    tree((value) => {
      actions = value;
    }),
  );
  expect(screen.getByRole("status")).toHaveTextContent("dark:100:false");
  act(() => {
    actions!.setTheme("sepia" as "dark");
    actions!.setTextSize(999 as 125);
  });
  expect(screen.getByRole("status")).toHaveTextContent("dark:100:false");
});

it("syncs only the current account across tabs and handles cleared storage", () => {
  render(tree());
  act(() =>
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: appearanceStorageKey(second.id),
        newValue: JSON.stringify({ version: 1, theme: "dark", textSize: 150 }),
      }),
    ),
  );
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  act(() =>
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: appearanceStorageKey(first.id),
        newValue: JSON.stringify({ version: 1, theme: "dark", textSize: 150 }),
      }),
    ),
  );
  expect(screen.getByRole("status")).toHaveTextContent("dark:150:false");
  act(() => window.dispatchEvent(new StorageEvent("storage", { key: null })));
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
});

it("tolerates unavailable storage and reports failed persistence without losing the selection", () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("Unavailable");
  });
  const write = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementation(() => {
      throw new Error("Quota");
    });
  render(tree());
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  fireEvent.click(screen.getByRole("button", { name: "Тёмная" }));
  expect(screen.getByRole("status")).toHaveTextContent("dark:100:true");
  expect(document.documentElement.dataset.theme).toBe("dark");
  write.mockRestore();
  fireEvent.click(screen.getByRole("button", { name: "125%" }));
  expect(screen.getByRole("status")).toHaveTextContent("dark:125:false");
});

it("provides safe defaults outside the application provider", () => {
  render(<Harness />);
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
  fireEvent.click(screen.getByRole("button", { name: "Тёмная" }));
  expect(screen.getByRole("status")).toHaveTextContent("light:100:false");
});
