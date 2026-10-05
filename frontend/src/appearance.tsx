import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { useAuth } from "./auth";

export const TEXT_SIZE_OPTIONS = [75, 90, 100, 110, 125, 150, 200] as const;
export type TextSize = (typeof TEXT_SIZE_OPTIONS)[number];
export type Theme = "light" | "dark";

interface Preferences {
  theme: Theme;
  textSize: TextSize;
}
interface Appearance extends Preferences {
  setTheme: (theme: Theme) => void;
  setTextSize: (textSize: TextSize) => void;
  persistenceError: boolean;
}
interface AccountPreferences {
  userId: string | null;
  preferences: Preferences;
  persistenceError?: boolean;
}

const DEFAULT_PREFERENCES: Preferences = { theme: "light", textSize: 100 };
const Context = createContext<Appearance>({
  ...DEFAULT_PREFERENCES,
  setTheme: () => {},
  setTextSize: () => {},
  persistenceError: false,
});

/**
 * appearanceStorageKey разделяет оформление разных аккаунтов в одном браузере.
 *
 * @args userId — идентификатор авторизованного пользователя.
 * @return ключ локального хранилища без данных сессии.
 */
export function appearanceStorageKey(userId: string): string {
  return `go-recorder.appearance.v1:${encodeURIComponent(userId)}`;
}

/**
 * parsePreferences проверяет версию и значения локальных настроек.
 *
 * @args raw — JSON из локального хранилища либо отсутствие значения.
 * @return допустимое оформление; повреждённые поля заменяются стандартными.
 */
function parsePreferences(raw: string | null): Preferences {
  if (!raw) return DEFAULT_PREFERENCES;
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object" || !("version" in value))
      return DEFAULT_PREFERENCES;
    if (value.version !== 1) return DEFAULT_PREFERENCES;
    const theme = "theme" in value && value.theme === "dark" ? "dark" : "light";
    const textSize =
      "textSize" in value &&
      typeof value.textSize === "number" &&
      TEXT_SIZE_OPTIONS.includes(value.textSize as TextSize)
        ? (value.textSize as TextSize)
        : 100;
    return { theme, textSize };
  } catch {
    return DEFAULT_PREFERENCES;
  }
}

/**
 * readPreferences безопасно загружает оформление только текущего аккаунта.
 *
 * @args userId — идентификатор пользователя либо отсутствие авторизации.
 * @return сохранённые настройки или светлая тема со стандартным текстом.
 */
function readPreferences(userId: string | null): Preferences {
  if (!userId) return DEFAULT_PREFERENCES;
  try {
    return parsePreferences(
      window.localStorage.getItem(appearanceStorageKey(userId)),
    );
  } catch {
    return DEFAULT_PREFERENCES;
  }
}

/**
 * AppearanceProvider применяет оформление без масштабирования видео и интерфейса.
 * Сохранение локально для браузера и аккаунта; после выхода действует стандартное оформление.
 *
 * @args children — содержимое приложения внутри провайдера авторизации.
 * @return контекст с выбором темы и процентного размера текста.
 */
export function AppearanceProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const userId = user && !user.guestConferenceId ? user.id : null;
  const loaded = useMemo(() => readPreferences(userId), [userId]);
  const [account, setAccount] = useState<AccountPreferences>({
    userId,
    preferences: loaded,
  });
  // Старые настройки не показываются даже в первом рендере другого аккаунта.
  const preferences = account.userId === userId ? account.preferences : loaded;
  const persistenceError =
    account.userId === userId && !!account.persistenceError;
  const current = useRef<AccountPreferences>({ userId, preferences });
  current.current = { userId, preferences };

  useLayoutEffect(() => {
    document.documentElement.dataset.theme = preferences.theme;
    document.documentElement.style.setProperty(
      "--text-scale",
      String(preferences.textSize / 100),
    );
  }, [preferences.theme, preferences.textSize]);

  useEffect(() => {
    setAccount({ userId, preferences: loaded });
  }, [userId, loaded]);

  useEffect(() => {
    if (!userId) return;
    /**
     * sync отражает изменения оформления этого аккаунта в соседней вкладке.
     *
     * @args event — событие изменения локального хранилища.
     * @return ничего; обновляет оформление лишь при совпадении аккаунта.
     */
    function sync(event: StorageEvent) {
      if (current.current.userId !== userId) return;
      if (event.key !== null && event.key !== appearanceStorageKey(userId!))
        return;
      const next =
        event.key === null
          ? DEFAULT_PREFERENCES
          : parsePreferences(event.newValue);
      current.current = { userId, preferences: next };
      setAccount(current.current);
    }
    window.addEventListener("storage", sync);
    return () => window.removeEventListener("storage", sync);
  }, [userId]);

  /**
   * update немедленно применяет и сохраняет изменение текущего аккаунта.
   *
   * @args change — проверенный фрагмент пользовательского оформления.
   * @return ничего; при недоступности хранилища выбор остаётся в памяти вкладки.
   */
  const update = useCallback(
    (change: Partial<Preferences>) => {
      // Оставшийся обработчик старого диалога не может изменить новый аккаунт.
      if (!userId || current.current.userId !== userId) return;
      const next = { ...current.current.preferences, ...change };
      let failed = false;
      try {
        window.localStorage.setItem(
          appearanceStorageKey(userId),
          JSON.stringify({ version: 1, ...next }),
        );
      } catch {
        // Ограничения браузера на хранилище не мешают менять оформление.
        failed = true;
      }
      current.current = { userId, preferences: next, persistenceError: failed };
      setAccount(current.current);
    },
    [userId],
  );

  /**
   * setTheme проверяет значение темы перед применением.
   *
   * @args theme — светлая либо тёмная тема.
   * @return ничего; применяет допустимую тему.
   */
  const setTheme = useCallback(
    (theme: Theme) => {
      if (theme === "light" || theme === "dark") update({ theme });
    },
    [update],
  );
  /**
   * setTextSize проверяет процент перед применением.
   *
   * @args textSize — один из предлагаемых размеров текста в процентах.
   * @return ничего; применяет допустимый размер текста.
   */
  const setTextSize = useCallback(
    (textSize: TextSize) => {
      if (TEXT_SIZE_OPTIONS.includes(textSize)) update({ textSize });
    },
    [update],
  );

  return (
    <Context.Provider
      value={{ ...preferences, setTheme, setTextSize, persistenceError }}
    >
      {children}
    </Context.Provider>
  );
}

/**
 * useAppearance возвращает оформление и обработчики текущего аккаунта.
 *
 * @args нет.
 * @return выбранная тема, размер текста и действия для их изменения.
 */
export function useAppearance(): Appearance {
  return useContext(Context);
}
