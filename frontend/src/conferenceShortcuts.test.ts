import { expect, it } from "vitest";
import { meetingShortcut } from "./conferenceShortcuts";

function dispatch(target: EventTarget, init: KeyboardEventInit) {
  let result: string | null = null;
  const listener = (event: Event) => {
    result = meetingShortcut(event as KeyboardEvent, ["m", "v", "h", "c"]);
  };
  target.addEventListener("keydown", listener, { once: true });
  target.dispatchEvent(
    new KeyboardEvent("keydown", { bubbles: true, ...init }),
  );
  return result;
}

it("accepts plain meeting keys and ignores modifiers, repeats and composition", () => {
  expect(dispatch(window, { key: "M" })).toBe("m");
  expect(dispatch(window, { key: "m", ctrlKey: true })).toBeNull();
  expect(dispatch(window, { key: "v", metaKey: true })).toBeNull();
  expect(dispatch(window, { key: "h", altKey: true })).toBeNull();
  expect(dispatch(window, { key: "c", shiftKey: true })).toBeNull();
  expect(dispatch(window, { key: "m", repeat: true })).toBeNull();
  expect(dispatch(window, { key: "m", isComposing: true })).toBeNull();
  expect(dispatch(window, { key: "x" })).toBeNull();
});

it("never runs in editors, selectors or modal dialogs", () => {
  const targets = [
    document.createElement("input"),
    document.createElement("textarea"),
    document.createElement("select"),
    document.createElement("div"),
    document.createElement("div"),
  ];
  targets[3].setAttribute("contenteditable", "true");
  targets[4].setAttribute("role", "textbox");
  for (const target of targets) {
    document.body.append(target);
    expect(dispatch(target, { key: "m" })).toBeNull();
    target.remove();
  }
  const dialog = document.createElement("div");
  dialog.setAttribute("role", "dialog");
  const button = document.createElement("button");
  dialog.append(button);
  document.body.append(dialog);
  expect(dispatch(button, { key: "h" })).toBeNull();
  dialog.remove();
});
