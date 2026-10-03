import { describe, expect, it } from "vitest";
import { readBuildMetadata } from "../build-metadata";
import { appBuild } from "./buildInfo";

describe("публичная версия frontend", () => {
  it("содержит только три разрешённых поля и не меняется после сборки", () => {
    expect(Object.keys(appBuild).sort()).toEqual([
      "buildTime",
      "commit",
      "version",
    ]);
    expect(Object.isFrozen(appBuild)).toBe(true);
    expect(readBuildMetadata({ TOKEN: "secret" })).toEqual({
      version: "dev",
      commit: "unknown",
      buildTime: "unknown",
    });
  });

  it("принимает SemVer, commit SHA и UTC-время артефакта", () => {
    expect(
      readBuildMetadata({
        VITE_BUILD_VERSION: "v1.2.3-rc.1",
        VITE_BUILD_COMMIT: "a".repeat(40),
        VITE_BUILD_TIME: "2026-10-03T12:00:00Z",
      }),
    ).toEqual({
      version: "v1.2.3-rc.1",
      commit: "a".repeat(40),
      buildTime: "2026-10-03T12:00:00Z",
    });
  });

  it("останавливает сборку при попытке включить URL или неверную дату", () => {
    expect(() =>
      readBuildMetadata({ VITE_BUILD_VERSION: "https://private?token=secret" }),
    ).toThrow();
    expect(() =>
      readBuildMetadata({ VITE_BUILD_COMMIT: "private-data" }),
    ).toThrow();
    expect(() =>
      readBuildMetadata({ VITE_BUILD_TIME: "2026-02-31T12:00:00Z" }),
    ).toThrow();
  });
});
