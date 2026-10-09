import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetModules();
});

describe("бренд MeetSpace", () => {
  it("использует новое имя, слоган и описание по умолчанию", async () => {
    vi.stubEnv("VITE_PRODUCT_NAME", undefined);
    vi.resetModules();
    const brand = await import("./brand");
    expect(brand.PRODUCT_NAME).toBe("MeetSpace");
    expect(brand.PRODUCT_TAGLINE).toBe(
      "Встречи, чаты и совместная работа в одном месте.",
    );
    expect(brand.PRODUCT_DESCRIPTION).toBe(
      "MeetSpace — единое пространство для встреч и общения.",
    );
  });
  it("не восстанавливает старое название из прежнего публичного build-time значения", async () => {
    vi.stubEnv("VITE_PRODUCT_NAME", "Meetrix");
    vi.resetModules();
    expect((await import("./brand")).PRODUCT_NAME).toBe("MeetSpace");
  });
  it("сохраняет намеренно настроенное собственное имя", async () => {
    vi.stubEnv("VITE_PRODUCT_NAME", "  Командное пространство  ");
    vi.resetModules();
    const brand = await import("./brand");
    expect(brand.PRODUCT_NAME).toBe("Командное пространство");
    expect(brand.PRODUCT_DESCRIPTION).toBe(
      "Командное пространство — единое пространство для встреч и общения.",
    );
  });
});
