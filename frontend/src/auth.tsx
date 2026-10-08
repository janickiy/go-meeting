import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiError, configureAuth } from "./api";
import type { User, LoginResponse, Item, Participant } from "./types";
import {
  clearLogoutMarker,
  hasLogoutMarker,
  LOGOUT_KEY,
  markLogout,
  readSession,
  saveSession,
} from "./utils";
import type { Session } from "./utils";

interface Auth {
  user: User | null;
  loading: boolean;
  expired: boolean;
  startupError: boolean;
  login: (email: string, password: string) => Promise<void>;
  updateProfile: (displayName: string) => Promise<User>;
  enterGuest: (
    code: string,
    displayName: string,
  ) => Promise<LoginResponse & Item<Participant>>;
  logout: () => Promise<void>;
  retry: () => void;
}

const Context = createContext<Auth | null>(null);
const RENEW_BEFORE_MS = 60_000;
const RETRY_MS = 30_000;

/** The account persists on the server; an expiring JWT only limits one access token. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const [session, setSession] = useState<Session | null>(readSession);
  const sessionRef = useRef(session);
  const [user, setUser] = useState<User | null>(session?.user ?? null);
  const userRef = useRef(user);
  const [loading, setLoading] = useState(true);
  const [expired, setExpired] = useState(false);
  const [startupError, setStartupError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const generation = useRef(0);
  // Выход из аккаунта запрещает восстановление его cookie, но не отменяет
  // отдельно выданный гостевой токен. Сервер подтвердит его через auth/me.
  const accountRestoreBlocked = useRef(hasLogoutMarker());
  const loggedOut = useRef(
    accountRestoreBlocked.current && !session?.user?.guestConferenceId,
  );
  const renewal = useRef<Promise<boolean> | null>(null);
  const renewalController = useRef<AbortController | null>(null);
  const identityController = useRef<AbortController | null>(null);
  const restoreAllowed = useRef(true);
  const migrationNeeded = useRef(!!session);
  const startupComplete = useRef(false);
  const renewRef = useRef<(usedToken?: string) => Promise<boolean>>(
    async () => false,
  );

  const clear = useCallback(
    (isExpired = false) => {
      sessionRef.current = null;
      userRef.current = null;
      restoreAllowed.current = false;
      startupComplete.current = true;
      configureAuth(null);
      saveSession(null);
      void client.cancelQueries();
      client.clear();
      setSession(null);
      setUser(null);
      setLoading(false);
      setStartupError(false);
      setExpired(isExpired);
    },
    [client],
  );

  /** A renewal never resets queries, loading, or the mounted conference. */
  const applySession = useCallback(
    (result: LoginResponse, expected: number) => {
      if (generation.current !== expected || loggedOut.current) return false;
      if (
        !result.accessToken ||
        !result.user?.id ||
        !Number.isFinite(result.expiresIn) ||
        result.expiresIn <= 0
      )
        throw new ApiError(502, "Сервер вернул некорректный ответ.");
      const next: Session = {
        token: result.accessToken,
        expiresAt: Date.now() + Math.min(result.expiresIn, 3600) * 1000,
        user: result.user,
      };
      sessionRef.current = next;
      userRef.current = result.user;
      restoreAllowed.current = true;
      migrationNeeded.current = false;
      configureAuth(next.token);
      saveSession(next);
      setSession(next);
      setUser(result.user);
      setExpired(false);
      setStartupError(false);
      return true;
    },
    [],
  );

  const renew = useCallback(
    (usedToken?: string): Promise<boolean> => {
      if (loggedOut.current) return Promise.resolve(false);
      if (usedToken && sessionRef.current?.token !== usedToken)
        return Promise.resolve(!!sessionRef.current);
      if (userRef.current?.guestConferenceId) {
        clear(true);
        return Promise.resolve(false);
      }
      if (accountRestoreBlocked.current) return Promise.resolve(false);
      if (renewal.current) return renewal.current;
      const expected = generation.current;
      const controller = new AbortController();
      renewalController.current = controller;
      const current = sessionRef.current;
      const response =
        migrationNeeded.current &&
        userRef.current &&
        current &&
        current.expiresAt > Date.now()
          ? api.establishSession(controller.signal).catch((error: unknown) => {
              if (error instanceof ApiError && error.status === 401)
                return api.refreshSession(controller.signal);
              throw error;
            })
          : api.refreshSession(controller.signal);
      const pending = response
        .then((result) => applySession(result, expected))
        .catch((error: unknown) => {
          if (controller.signal.aborted || generation.current !== expected)
            return false;
          if (error instanceof ApiError && error.status === 401) {
            clear(!!sessionRef.current);
          } else if (!userRef.current) {
            setStartupError(true);
          }
          // A transport failure cannot invalidate an account or its cached token.
          return false;
        })
        .finally(() => {
          if (renewal.current === pending) renewal.current = null;
          if (renewalController.current === controller)
            renewalController.current = null;
        });
      renewal.current = pending;
      return pending;
    },
    [applySession, clear],
  );
  renewRef.current = renew;

  useEffect(() => {
    const expected = generation.current;
    const controller = new AbortController();
    configureAuth(sessionRef.current?.token ?? null, (usedToken) =>
      renewRef.current(usedToken),
    );
    const start = async () => {
      if (!userRef.current) setLoading(true);
      setStartupError(false);
      if (loggedOut.current) {
        clear();
        // Retry an offline logout before ever attempting cookie restoration.
        await api.logout().catch(() => {});
        return;
      }
      const cached = sessionRef.current;
      if (cached && cached.expiresAt > Date.now()) {
        try {
          const result = await api.me(controller.signal);
          if (controller.signal.aborted || generation.current !== expected)
            return;
          // Признак гостя в кеше не даёт права восстановить старый аккаунт:
          // исключение действует только для подтверждённой сервером гостевой сессии.
          if (accountRestoreBlocked.current && !result.user.guestConferenceId) {
            loggedOut.current = true;
            clear();
            await api.logout().catch(() => {});
            return;
          }
          userRef.current = result.user;
          setUser(result.user);
          if (result.user.guestConferenceId) {
            const next = { ...cached, user: result.user };
            sessionRef.current = next;
            saveSession(next);
            setSession(next);
          } else {
            // Migrate users already signed in before persistent cookies existed.
            const account = await api.establishSession(controller.signal);
            applySession(account, expected);
          }
        } catch (error) {
          if (controller.signal.aborted || generation.current !== expected)
            return;
          if (error instanceof ApiError && error.status === 401) {
            // Отозванный гостевой токен означает завершённую сессию,
            // а не недоступность сервера и не повод восстанавливать аккаунт.
            if (restoreAllowed.current) await renewRef.current();
          } else if (!userRef.current) setStartupError(true);
        }
      } else {
        await renewRef.current();
      }
    };
    void start().finally(() => {
      if (!controller.signal.aborted && generation.current === expected) {
        startupComplete.current = true;
        setLoading(false);
      }
    });
    return () => controller.abort();
  }, [attempt, applySession, clear]);

  useEffect(() => {
    const check = () => {
      if (!startupComplete.current) return;
      if (loggedOut.current) {
        if (hasLogoutMarker()) void api.logout().catch(() => {});
        return;
      }
      const current = sessionRef.current;
      if (userRef.current?.guestConferenceId) {
        if (current && current.expiresAt <= Date.now()) clear(true);
        return;
      }
      if (
        restoreAllowed.current &&
        (migrationNeeded.current ||
          !current ||
          current.expiresAt <= Date.now() + RENEW_BEFORE_MS)
      )
        void renewRef.current();
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") check();
    };
    const onStorage = (event: StorageEvent) => {
      if (event.key !== LOGOUT_KEY || event.newValue === null) return;
      accountRestoreBlocked.current = true;
      loggedOut.current = true;
      generation.current++;
      renewalController.current?.abort();
      identityController.current?.abort();
      renewal.current = null;
      clear();
    };
    const interval = window.setInterval(check, RETRY_MS);
    window.addEventListener("focus", check);
    window.addEventListener("online", check);
    window.addEventListener("storage", onStorage);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", check);
      window.removeEventListener("online", check);
      window.removeEventListener("storage", onStorage);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [clear]);

  useEffect(() => {
    if (!session) return;
    const isGuest = !!session.user?.guestConferenceId;
    const timer = window.setTimeout(
      () => {
        if (!startupComplete.current) return;
        if (
          sessionRef.current?.token !== session.token ||
          sessionRef.current.expiresAt !== session.expiresAt
        )
          return;
        if (isGuest) clear(true);
        else void renewRef.current();
      },
      Math.max(
        0,
        session.expiresAt - Date.now() - (isGuest ? 0 : RENEW_BEFORE_MS),
      ),
    );
    return () => window.clearTimeout(timer);
  }, [session, clear]);

  useEffect(
    () => () => {
      generation.current++;
      renewalController.current?.abort();
      identityController.current?.abort();
      renewal.current = null;
    },
    [],
  );

  /** Принимает новую личность только пока запрос принадлежит текущему поколению.
   * @args result — подтверждённая сервером сессия; expected — поколение запроса входа.
   * @return Принята ли сессия; false запрещает вызывающему коду применять старый ответ.
   */
  async function acceptSession(
    result: LoginResponse,
    expected: number,
  ): Promise<boolean> {
    if (generation.current !== expected) return false;
    await client.cancelQueries();
    if (generation.current !== expected) return false;
    client.clear();
    // A new identity fences retries belonging to the previous account.
    configureAuth(null);
    loggedOut.current = false;
    if (!result.user.guestConferenceId) {
      accountRestoreBlocked.current = false;
      clearLogoutMarker();
    }
    if (!applySession(result, expected)) return false;
    startupComplete.current = true;
    setLoading(false);
    return true;
  }

  async function login(email: string, password: string) {
    const expected = ++generation.current;
    identityController.current?.abort();
    const controller = new AbortController();
    identityController.current = controller;
    renewalController.current?.abort();
    renewal.current = null;
    try {
      const result = await api.login(email, password, controller.signal);
      await acceptSession(result, expected);
    } finally {
      if (identityController.current === controller)
        identityController.current = null;
      if (generation.current === expected) setLoading(false);
    }
  }

  async function enterGuest(code: string, displayName: string) {
    const expected = ++generation.current;
    identityController.current?.abort();
    const controller = new AbortController();
    identityController.current = controller;
    renewalController.current?.abort();
    renewal.current = null;
    try {
      const result = await api.joinGuest(code, displayName, controller.signal);
      const accepted = await acceptSession(result, expected);
      if (!accepted || generation.current !== expected)
        throw new ApiError(
          409,
          "Сессия изменилась. Повторите подключение к встрече.",
        );
      return result;
    } finally {
      if (identityController.current === controller)
        identityController.current = null;
    }
  }

  async function updateProfile(displayName: string): Promise<User> {
    const expected = generation.current;
    if (!sessionRef.current?.token)
      throw new ApiError(401, "Войдите в аккаунт.");
    const result = await api.updateProfile(displayName);
    if (generation.current !== expected || !sessionRef.current)
      throw new ApiError(409, "Сессия изменилась. Повторите действие.");
    userRef.current = result.user;
    setUser(result.user);
    const next = { ...sessionRef.current, user: result.user };
    sessionRef.current = next;
    saveSession(next);
    setSession(next);
    return result.user;
  }

  async function logout() {
    const token = sessionRef.current?.token ?? null;
    accountRestoreBlocked.current = true;
    loggedOut.current = true;
    generation.current++;
    renewalController.current?.abort();
    identityController.current?.abort();
    renewal.current = null;
    markLogout();
    clear();
    // The tombstone survives a failed request and a browser restart.
    await api.logout(token).catch(() => {});
  }

  return (
    <Context.Provider
      value={{
        user,
        loading,
        expired,
        startupError,
        login,
        updateProfile,
        enterGuest,
        logout,
        retry: () => setAttempt((value) => value + 1),
      }}
    >
      {children}
    </Context.Provider>
  );
}

export function useAuth() {
  const value = useContext(Context);
  if (!value) throw new Error("AuthProvider is missing");
  return value;
}
