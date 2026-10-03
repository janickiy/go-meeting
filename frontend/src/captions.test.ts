import { describe, expect, it } from "vitest";
import { mergeCaptions } from "./captions";
import type { Caption } from "./types";
const base = {
  id: "one",
  text: "черновик",
  sequence: 1,
  revision: 1,
  startMs: 0,
  endMs: 500,
  final: false,
} as Caption;
describe("Версии субтитров", () => {
  it("заменяет partial финалом, игнорирует перестановку и повторную доставку", () => {
    const final = {
      ...base,
      text: "готово",
      sequence: 2,
      revision: 2,
      final: true,
    };
    expect(mergeCaptions([base], [final, base, final])).toEqual([final]);
    expect(
      mergeCaptions([final], [{ ...base, sequence: 5, revision: 5 }]),
    ).toEqual([final]);
    const current = [final];
    expect(mergeCaptions(current, [base, final])).toBe(current);
  });
  it("разрешает final той же ревизии, но запрещает откат sequence", () => {
    expect(mergeCaptions([base], [{ ...base, final: true }])[0].final).toBe(
      true,
    );
    expect(
      mergeCaptions([base], [{ ...base, sequence: 0, revision: 2 }]),
    ).toEqual([base]);
  });
  it("ограничивает память и не интерпретирует текст как HTML", () => {
    const rows = Array.from({ length: 300 }, (_, i) => ({
      ...base,
      id: String(i),
      startMs: i,
      endMs: i + 500,
      text: "<script>test</script>",
    }));
    const result = mergeCaptions([], rows);
    expect(result).toHaveLength(200);
    expect(result[0].startMs).toBe(100);
    expect(result[0].text).toBe("<script>test</script>");
  });
});
