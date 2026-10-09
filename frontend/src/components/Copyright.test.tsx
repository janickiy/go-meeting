import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Copyright } from "./Copyright";

afterEach(cleanup);

it("показывает точный копирайт и безопасную ссылку на сайт автора", () => {
  const { container } = render(<Copyright />);
  expect(container).toHaveTextContent(
    "© 2026 Яницкий Александр. Все права защищены.",
  );
  const link = screen.getByRole("link", { name: "Яницкий Александр" });
  expect(link).toHaveAttribute("href", "https://janickiy.com/");
  expect(link).toHaveAttribute("target", "_blank");
  expect(link).toHaveAttribute("rel", "noopener noreferrer");
});
