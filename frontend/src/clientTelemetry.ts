import { appBuild, clientTelemetryEnabled } from "./buildInfo";

/** Коды описывают источник ошибки и никогда не содержат пользовательский текст. */
export type ClientErrorCode =
  "uncaught_error" | "unhandled_rejection" | "react_render_error";
export type BrowserFamily =
  "chromium" | "firefox" | "safari" | "edge" | "other";

/** Разрешённые поля сообщения; исходные ошибки и HTTP-запросы не сериализуются. */
export interface ClientErrorEvent {
  version: string;
  route: string;
  code: ClientErrorCode;
  browser: BrowserFamily;
  stack: { file: string; line: number; column: number }[];
}

const staticRoutes = new Set([
  "/",
  "/login",
  "/register",
  "/register/success",
  "/app",
  "/calendar",
  "/analytics",
  "/meetings",
  "/meetings/new",
  "/conferences",
  "/conferences/new",
  "/app/settings",
  "/settings",
  "/app/recordings",
  "/history",
  "/notifications",
  "/app/search",
  "/search",
  "/admin",
]);

/**
 * Удаляет идентификаторы, приглашения и параметры из маршрута.
 * @args pathname — путь текущей страницы; неизвестные пути не сохраняются.
 * @return фиксированный шаблон маршрута либо unknown.
 */
export function telemetryRoute(pathname: string): string {
  const path = pathname.split(/[?#]/, 1)[0];
  if (staticRoutes.has(path)) return path;
  for (const collection of ["meetings", "conferences"]) {
    if (new RegExp(`^/${collection}/[^/]+/join$`).test(path))
      return `/${collection}/:id/join`;
    if (new RegExp(`^/${collection}/[^/]+$`).test(path))
      return `/${collection}/:id`;
  }
  if (/^\/i\/[^/]+$/.test(path)) return "/i/:code";
  if (/^\/history\/[^/]+$/.test(path)) return "/history/:id";
  if (/^\/app\/settings\/calendar\/[^/]+\/callback$/.test(path))
    return "/app/settings/calendar/:provider/callback";
  return "unknown";
}

/**
 * Сводит User-Agent к семейству, исключая точную версию и характеристики устройства.
 * @args userAgent — строка браузера, используемая только локально.
 * @return одно из пяти разрешённых значений.
 */
export function browserFamily(userAgent: string): BrowserFamily {
  if (/Edg\//.test(userAgent)) return "edge";
  if (/Firefox\/|FxiOS\//.test(userAgent)) return "firefox";
  if (/Chrome\/|Chromium\/|CriOS\//.test(userAgent)) return "chromium";
  if (/Safari\//.test(userAgent)) return "safari";
  return "other";
}

/**
 * Оставляет не более пяти координат в JS-артефактах того же origin.
 * @args error — произвольный отказ; origin — origin самого приложения.
 * @return кадры без текста ошибки, функций, хоста, query, hash и внешних URL.
 */
export function sanitizedStack(
  error: unknown,
  origin: string,
): ClientErrorEvent["stack"] {
  const frames: ClientErrorEvent["stack"] = [];
  try {
    if (!(error instanceof Error) || typeof error.stack !== "string")
      return frames;
    for (const row of error.stack.slice(0, 16_384).split("\n")) {
      // Только форма реального кадра Chrome или Firefox, без первой строки сообщения.
      if (!/^\s*at\s/.test(row) && !/^[^\s@]*@https?:\/\//.test(row)) continue;
      const match = row.match(/(https?:\/\/[^\s()]+):(\d+):(\d+)\)?$/);
      if (!match) continue;
      const url = new URL(match[1]);
      const asset = url.pathname.match(
        /^\/assets\/([A-Za-z0-9_-]+-[A-Za-z0-9_-]{8,}\.js)$/,
      );
      const line = Number(match[2]);
      const column = Number(match[3]);
      if (
        url.origin !== origin ||
        url.username ||
        url.password ||
        url.search ||
        url.hash ||
        !asset ||
        asset[1].length > 160 ||
        !Number.isSafeInteger(line) ||
        !Number.isSafeInteger(column) ||
        line < 1 ||
        line > 10_000_000 ||
        column < 1 ||
        column > 10_000_000
      )
        continue;
      frames.push({ file: asset[1], line, column });
      if (frames.length === 5) break;
    }
  } catch {
    // Нестандартный объект ошибки не должен вызывать рекурсивный отказ телеметрии.
  }
  return frames;
}

/** Параметры адаптера позволяют проверить ограничения без реальной отправки данных. */
interface ReporterOptions {
  enabled: boolean;
  version: string;
  origin: string;
  pathname: () => string;
  userAgent: string;
  fetch: typeof fetch;
  now?: () => number;
}

/**
 * Создаёт ограниченный по частоте отправитель на фиксированный same-origin адрес.
 * @args options — публичные метки и браузерные зависимости без токенов авторизации.
 * @return обработчик, который не выбрасывает ошибки и не сохраняет очередь на диске.
 */
export function createClientErrorReporter(options: ReporterOptions) {
  const now = options.now || Date.now;
  const recent = new Map<string, number>();
  let windowStarted = now();
  let inWindow = 0;
  let total = 0;
  return (code: ClientErrorCode, error: unknown): void => {
    if (!options.enabled || total >= 20) return;
    try {
      const timestamp = now();
      if (timestamp - windowStarted >= 60_000) {
        windowStarted = timestamp;
        inWindow = 0;
      }
      if (inWindow >= 5) return;
      const event: ClientErrorEvent = {
        version: /^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$/.test(options.version)
          ? options.version
          : "unknown",
        route: telemetryRoute(options.pathname()),
        code,
        browser: browserFamily(options.userAgent),
        stack: sanitizedStack(error, options.origin),
      };
      const body = JSON.stringify(event);
      const last = recent.get(body);
      if (last !== undefined && timestamp - last < 10_000) return;
      recent.set(body, timestamp);
      inWindow++;
      total++;
      void options
        .fetch("/api/v1/client-errors", {
          method: "POST",
          body,
          headers: { "Content-Type": "application/json" },
          credentials: "omit",
          mode: "same-origin",
          redirect: "error",
          referrerPolicy: "no-referrer",
          cache: "no-store",
          keepalive: true,
        })
        .catch(() => {});
    } catch {
      // Ошибка самого адаптера не влияет на пользовательские действия.
    }
  };
}

let reporter: ReturnType<typeof createClientErrorReporter> | undefined;

/**
 * Передаёт ошибку в единственный адаптер текущей страницы при включённом флаге сборки.
 * @args code — фиксированный источник; error — исходный объект только для извлечения координат.
 */
export function reportClientError(code: ClientErrorCode, error: unknown): void {
  reporter?.(code, error);
}

/**
 * Подключает глобальные ошибки один раз до первого рендера React.
 * @return функция удаления слушателей, используемая при обновлении модулей разработки.
 */
export function installClientTelemetry(): () => void {
  if (!clientTelemetryEnabled || reporter) return () => {};
  reporter = createClientErrorReporter({
    enabled: true,
    version: appBuild.version,
    origin: window.location.origin,
    pathname: () => window.location.pathname,
    userAgent: navigator.userAgent,
    fetch: window.fetch.bind(window),
  });
  const onError = (event: ErrorEvent) =>
    reportClientError("uncaught_error", event.error);
  const onRejection = (event: PromiseRejectionEvent) =>
    reportClientError("unhandled_rejection", event.reason);
  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);
  return () => {
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
    reporter = undefined;
  };
}
