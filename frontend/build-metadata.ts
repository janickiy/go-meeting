import type { Plugin } from "vite";

/** Публичные сведения об артефакте; произвольные переменные окружения не включаются. */
export interface BuildMetadata {
  version: string;
  commit: string;
  buildTime: string;
}

/**
 * Проверяет метки сборки до включения в публичный JavaScript и JSON.
 * @args env — явно переданное окружение процесса сборки.
 * @return безопасные метки; неверное непустое значение прерывает сборку.
 */
export function readBuildMetadata(
  env: Record<string, string | undefined>,
): BuildMetadata {
  const version = env.VITE_BUILD_VERSION || "dev";
  const commit = env.VITE_BUILD_COMMIT || "unknown";
  const buildTime = env.VITE_BUILD_TIME || "unknown";
  if (!/^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$/.test(version))
    throw new Error("VITE_BUILD_VERSION must be a public release identifier");
  if (commit !== "unknown" && !/^[a-f0-9]{7,40}$/.test(commit))
    throw new Error("VITE_BUILD_COMMIT must be a Git commit SHA");
  if (
    buildTime !== "unknown" &&
    (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(buildTime) ||
      !Number.isFinite(Date.parse(buildTime)) ||
      new Date(buildTime).toISOString() !== buildTime.replace("Z", ".000Z"))
  )
    throw new Error("VITE_BUILD_TIME must be a UTC timestamp");
  return { version, commit, buildTime };
}

/**
 * Добавляет одинаковые метки в готовый артефакт и локальный сервер Vite.
 * @args metadata — проверенные публичные сведения о сборке.
 * @return плагин выдачи /version.json без кеширования.
 */
export function buildMetadataPlugin(metadata: BuildMetadata): Plugin {
  const source = JSON.stringify(metadata) + "\n";
  return {
    name: "public-build-metadata",
    generateBundle() {
      this.emitFile({ type: "asset", fileName: "version.json", source });
    },
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (request.url?.split("?")[0] !== "/version.json") return next();
        response.setHeader("Content-Type", "application/json");
        response.setHeader("Cache-Control", "no-store");
        response.end(source);
      });
    },
  };
}
