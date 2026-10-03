import { describe, expect, it, vi } from "vitest";
import {
  chatDayKey,
  chatDayLabel,
  chatTime,
  insertChatEmoji,
} from "./chatPresentation";

describe("даты сообщений", () => {
  it("группирует сообщения по локальной календарной дате", () => {
    expect(chatDayKey(new Date(2026, 9, 3, 0, 5).toISOString())).toBe(
      "2026-10-03",
    );
    expect(chatDayKey(new Date(2026, 9, 3, 23, 55).toISOString())).toBe(
      "2026-10-03",
    );
    expect(chatDayKey(new Date(2026, 9, 4, 0, 5).toISOString())).toBe(
      "2026-10-04",
    );
  });

  it("подписывает сегодня, вчера и день другого года", () => {
    const now = new Date(2026, 9, 3, 21, 30);
    expect(chatDayLabel(new Date(2026, 9, 3, 9).toISOString(), now)).toBe(
      "Сегодня",
    );
    expect(chatDayLabel(new Date(2026, 9, 2, 23).toISOString(), now)).toBe(
      "Вчера",
    );
    expect(chatDayLabel(new Date(2026, 8, 30, 12).toISOString(), now)).toBe(
      "30 сентября",
    );
    expect(chatDayLabel(new Date(2025, 11, 31, 12).toISOString(), now)).toBe(
      "31 декабря 2025 г.",
    );
  });

  it("по умолчанию использует текущую дату", () => {
    const now = new Date(2026, 9, 3, 21, 30);
    vi.useFakeTimers();
    try {
      vi.setSystemTime(now);
      expect(chatDayLabel(now.toISOString())).toBe("Сегодня");
    } finally {
      vi.useRealTimers();
    }
  });

  it.each([
    [new Date(2026, 0, 1, 0, 15), new Date(2025, 11, 31, 23, 45)],
    [new Date(2026, 2, 30, 0, 15), new Date(2026, 2, 29, 12)],
    [new Date(2026, 9, 25, 23, 30), new Date(2026, 9, 24, 12)],
  ])(
    "считает вчера календарно на границе года и перевода часов",
    (now, yesterday) => {
      expect(chatDayLabel(yesterday.toISOString(), now)).toBe("Вчера");
      expect(chatDayLabel(now.toISOString(), now)).toBe("Сегодня");
    },
  );

  it("показывает время 00:05 и 23:59 без секунд", () => {
    expect(chatTime(new Date(2026, 9, 3, 0, 5, 59).toISOString())).toBe(
      "00:05",
    );
    expect(chatTime(new Date(2026, 9, 3, 23, 59, 1).toISOString())).toBe(
      "23:59",
    );
  });

  it.each(["", "invalid", "2026-99-99T00:00:00Z"])(
    "не выводит ошибочную дату %s",
    (value) => {
      expect(chatDayKey(value)).toBe("");
      expect(chatDayLabel(value)).toBe("");
      expect(chatTime(value)).toBe("");
    },
  );

  it("показывает полную дату при неизвестной текущей дате", () => {
    expect(
      chatDayLabel(new Date(2026, 9, 3, 12).toISOString(), new Date(NaN)),
    ).toBe("3 октября 2026 г.");
  });
});

describe("вставка смайликов", () => {
  it("вставляет смайлик на месте курсора и возвращает UTF-16 позицию", () => {
    expect(insertChatEmoji("Привет мир", "😀", 7, 7)).toEqual({
      text: "Привет 😀мир",
      caret: 9,
    });
    expect(insertChatEmoji("", "❤️", 0, 0)).toEqual({
      text: "❤️",
      caret: 2,
    });
  });

  it("заменяет выделенный текст, в том числе целую суррогатную пару", () => {
    expect(insertChatEmoji("Привет мир", "👋", 7, 10)).toEqual({
      text: "Привет 👋",
      caret: 9,
    });
    expect(insertChatEmoji("а😀б", "🔥", 1, 3)).toEqual({
      text: "а🔥б",
      caret: 3,
    });
    expect(insertChatEmoji("а😀б", "👍", 3, 3)).toEqual({
      text: "а😀👍б",
      caret: 5,
    });
  });

  it("безопасно ограничивает и упорядочивает позиции выделения", () => {
    expect(insertChatEmoji("abc", "😀", -10, 99)).toEqual({
      text: "😀",
      caret: 2,
    });
    expect(insertChatEmoji("abcd", "😀", 3, 1)).toEqual({
      text: "a😀d",
      caret: 3,
    });
    expect(insertChatEmoji("abc", "😀", NaN, NaN)).toEqual({
      text: "😀abc",
      caret: 2,
    });
    expect(insertChatEmoji("abc", "😀", Infinity, Infinity)).toEqual({
      text: "abc😀",
      caret: 5,
    });
    expect(insertChatEmoji("abc", "😀", 1.9, 1.9)).toEqual({
      text: "a😀bc",
      caret: 3,
    });
  });

  it("не разрывает суррогатную пару ошибочной позицией курсора или выделения", () => {
    expect(insertChatEmoji("а😀б", "👍", 2, 2)).toEqual({
      text: "а👍😀б",
      caret: 3,
    });
    expect(insertChatEmoji("а😀б", "👍", 2, 3)).toEqual({
      text: "а👍б",
      caret: 3,
    });
    expect(insertChatEmoji("а😀б", "👍", 1, 2)).toEqual({
      text: "а👍б",
      caret: 3,
    });
  });

  it("разрешает ровно 4000 Unicode-символов независимо от UTF-16 длины", () => {
    const text = "😀".repeat(3999);
    const result = insertChatEmoji(text, "😀", text.length, text.length);
    expect(result?.text).toBe("😀".repeat(4000));
    expect(result?.caret).toBe(8000);
    expect(insertChatEmoji("я".repeat(4000), "😀", 4000, 4000)).toBeNull();
    expect(insertChatEmoji("😀".repeat(4000), "😀", 8000, 8000)).toBeNull();
    expect(insertChatEmoji("😀".repeat(4000), "👋", 0, 2)?.text).toBe(
      "👋" + "😀".repeat(3999),
    );
  });

  it("считает всю ZWJ-последовательность и вариационные селекторы как руны сервера", () => {
    const family = "👨‍👩‍👧‍👦";
    expect(Array.from(family)).toHaveLength(7);
    const allowed = "я".repeat(3993);
    expect(
      insertChatEmoji(allowed, family, allowed.length, allowed.length),
    ).toEqual({
      text: allowed + family,
      caret: allowed.length + family.length,
    });
    const exceeded = "я".repeat(3994);
    expect(
      insertChatEmoji(exceeded, family, exceeded.length, exceeded.length),
    ).toBeNull();
    expect(insertChatEmoji("я".repeat(3998), "❤️", 3998, 3998)?.text).toBe(
      "я".repeat(3998) + "❤️",
    );
    expect(insertChatEmoji("я".repeat(3999), "❤️", 3999, 3999)).toBeNull();
  });

  it("применяет лимит к обрезанному тексту, не удаляя пробелы из черновика", () => {
    const text = "  " + "я".repeat(3999);
    const result = insertChatEmoji(text, "😀", text.length, text.length);
    expect(result?.text).toBe(text + "😀");
    expect(result?.caret).toBe(text.length + 2);
    const oversized = "я".repeat(4001);
    expect(insertChatEmoji(oversized, "😀", 0, 2)?.text).toBe(
      "😀" + "я".repeat(3999),
    );
  });
});
