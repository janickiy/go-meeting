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
import type { User } from "./types";
import { readSession, saveSession } from "./utils";
import type { Session } from "./utils";

interface Auth {
  user: User | null;
  loading: boolean;
  expired: boolean;
  startupError: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  retry: () => void;
}
const Context = createContext<Auth | null>(null);
export function AuthProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const [session, setSession] = useState<Session | null>(readSession);
  const sessionRef = useRef(session);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(!!session);
  const [expired, setExpired] = useState(false);
  const [startupError, setStartupError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const clear = useCallback(
    (isExpired = false) => {
      sessionRef.current = null;
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
  useEffect(() => {
    const current = sessionRef.current;
    configureAuth(current?.token || null, (usedToken) => {
      if (usedToken === sessionRef.current?.token) clear(true);
    });
    if (!current) return;
    const controller = new AbortController();
    setLoading(true);
    setStartupError(false);
    api
      .me(controller.signal)
      .then(({ user }) => {
        if (
          !controller.signal.aborted &&
          sessionRef.current?.token === current.token
        )
          setUser(user);
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.status === 401) clear(true);
        else setStartupError(true);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [session?.token, attempt, clear]);
  useEffect(() => {
    if (!session) return;
    const timer = window.setTimeout(
      () => clear(true),
      Math.max(0, session.expiresAt - Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [session, clear]);
  async function login(email: string, password: string) {
    const result = await api.login(email, password);
    await client.cancelQueries();
    client.clear();
    const next = {
      token: result.accessToken,
      expiresAt: Date.now() + Math.min(result.expiresIn, 3600) * 1000,
    };
    sessionRef.current = next;
    configureAuth(next.token);
    saveSession(next);
    setSession(next);
    setUser(result.user);
    setExpired(false);
    setStartupError(false);
  }
  async function logout() {
    try {
      await api.logout();
    } finally {
      clear();
    }
  }
  return (
    <Context.Provider
      value={{
        user,
        loading,
        expired,
        startupError,
        login,
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
