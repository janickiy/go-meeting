import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordingActions } from "./RecordingActions";

afterEach(() => vi.unstubAllGlobals());

describe("действия записи", () => {
  it("сохраняет подписанный адрес скачивания без изменений", () => {
    const url =
      "https://media.example.test/final.mp4?X-Amz-Signature=abc%2Fdef&X-Amz-Date=20261004";
    render(
      <RecordingActions
        title="Встреча"
        path="/recordings/id?conference=room"
        downloadUrl={url}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Действия с записью: Встреча" }),
    );
    expect(screen.getByRole("menuitem", { name: "Скачать" })).toHaveAttribute(
      "href",
      url,
    );
    expect(screen.getByRole("menuitem", { name: "Скачать" })).toHaveAttribute(
      "download",
      "",
    );
    expect(screen.queryByText(/Избранное|Удалить/)).toBeNull();
  });

  it("копирует страницу с проверкой доступа, а не публичный адрес файла", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    const path = "/recordings/id?conference=room";
    render(
      <RecordingActions
        title="Встреча"
        path={path}
        downloadUrl="https://media.example.test/private?signature=secret"
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    fireEvent.click(
      screen.getByRole("menuitem", { name: "Копировать ссылку" }),
    );
    expect(writeText).toHaveBeenCalledWith(
      new URL(path, window.location.origin).href,
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Ссылка скопирована",
    );
  });

  it("при недоступном буфере предлагает ручное копирование", async () => {
    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: vi.fn().mockRejectedValue(new Error("private failure")),
      },
    });
    render(
      <RecordingActions
        title="Встреча"
        path="/recordings/id?conference=room"
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    fireEvent.click(
      screen.getByRole("menuitem", { name: "Копировать ссылку" }),
    );
    expect(
      await screen.findByRole("textbox", { name: "Ссылка на запись" }),
    ).toHaveValue(
      new URL("/recordings/id?conference=room", window.location.origin).href,
    );
    expect(screen.queryByText("private failure")).toBeNull();
  });

  it("не предлагает скачивание небезопасного адреса", () => {
    render(
      <RecordingActions
        title="Встреча"
        path="/recordings/id?conference=room"
        downloadUrl="javascript:alert(1)"
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(screen.queryByRole("menuitem", { name: "Скачать" })).toBeNull();
    expect(
      screen.getByRole("menuitem", { name: "Копировать ссылку" }),
    ).toBeInTheDocument();
  });

  it("открывается стрелкой, перемещает фокус и закрывается Escape с возвратом фокуса", async () => {
    render(
      <RecordingActions
        title="Встреча"
        path="/recordings/id?conference=room"
        downloadUrl="/media/file"
      />,
    );
    const trigger = screen.getByRole("button");
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "Скачать" })).toHaveFocus(),
    );
    fireEvent.keyDown(document.activeElement!, { key: "End" });
    expect(
      screen.getByRole("menuitem", { name: "Копировать ссылку" }),
    ).toHaveFocus();
    fireEvent.keyDown(document.activeElement!, { key: "Home" });
    expect(screen.getByRole("menuitem", { name: "Скачать" })).toHaveFocus();
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it("закрывается при переходе фокуса за границы меню и внешнем нажатии", () => {
    render(
      <>
        <RecordingActions
          title="Встреча"
          path="/recordings/id?conference=room"
        />
        <button>Следующий элемент</button>
      </>,
    );
    const trigger = screen.getByRole("button", {
      name: "Действия с записью: Встреча",
    });
    fireEvent.click(trigger);
    act(() =>
      screen.getByRole("button", { name: "Следующий элемент" }).focus(),
    );
    expect(screen.queryByRole("menu")).toBeNull();
    fireEvent.click(trigger);
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });
});
