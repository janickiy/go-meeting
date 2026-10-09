import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { UnreadMessageCount } from "./UnreadMessageCount";

describe("число непрочитанных сообщений", () => {
  it("не занимает место, если непрочитанных сообщений нет", () => {
    const { container } = render(<UnreadMessageCount count={0} />);
    expect(container).toBeEmptyDOMElement();
  });
  it("показывает точное число и доступную подпись для личных и групповых чатов", () => {
    render(<UnreadMessageCount count={123} />);
    expect(
      screen.getByLabelText("123 непрочитанных сообщений"),
    ).toHaveTextContent("123");
    expect(screen.getByLabelText("123 непрочитанных сообщений")).toHaveClass(
      "message-unread-count",
    );
  });
  it("обновляет число и исчезает при прочтении последнего сообщения", () => {
    const { container, rerender } = render(<UnreadMessageCount count={5} />);
    rerender(<UnreadMessageCount count={2} />);
    expect(
      screen.getByLabelText("2 непрочитанных сообщений"),
    ).toHaveTextContent("2");
    rerender(<UnreadMessageCount count={0} />);
    expect(container).toBeEmptyDOMElement();
  });
});
