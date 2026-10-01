import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в интерфейсе Meet.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    cleanup();
    sessionStorage.clear();
  },
);
