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

/**
 * Auth описывает пользователя, восстановление сессии и действия React-авторизации.
 *
 * Состав:
 *   - user — публичные сведения пользователя.
 *   - loading — признак незавершённого действия, ограничивающий повторную отправку.
 *   - expired — поле или операция этого контракта.
 *   - startupError — поле или операция этого контракта.
 *   - login — поле или операция этого контракта.
 *   - logout — поле или операция этого контракта.
 *   - retry — номер повторной попытки операции.
 */
interface Auth {
  user: User | null;
  loading: boolean;
  expired: boolean;
  startupError: boolean;
  login: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в клиентской авторизации.
   *
   * @parameters:
   *   - email (string) — адрес электронной почты.
   *   - password (string) — пароль из формы; не предназначен для журналирования.
   *
   * @returns Promise<void> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
   */ (email: string, password: string) => Promise<void>;
  logout: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в клиентской авторизации.
   *
   *
   * @returns Promise<void> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
   */ () => Promise<void>;
  retry: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в клиентской авторизации.
   *
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ () => void;
}
const Context = createContext<Auth | null>(null);
/**
 * AuthProvider восстанавливает клиентскую сессию, проверяет пользователя и предоставляет операции авторизации через React-контекст.
 *
 * @parameters:
 *   - объект параметров: children — вложенное содержимое компонента или диалога.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
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
    /**
     * Обработчик useCallback выполняет действие с текущими зависимостями React-компонента.
     *
     * @parameters:
     *   - isExpired — признак истечения срока текущей сессии (по умолчанию false).
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
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
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      const current = sessionRef.current;
      configureAuth(
        current?.token || null,
        /**
         * Обработчик configureAuth выполняет переданный шаг вызова configureAuth в клиентской авторизации.
         *
         * @parameters:
         *   - usedToken — токен конкретного запроса, который получил отказ авторизации.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ (usedToken) => {
          if (usedToken === sessionRef.current?.token) clear(true);
        },
      );
      if (!current) return;
      const controller = new AbortController();
      setLoading(true);
      setStartupError(false);
      api
        .me(controller.signal)
        .then(
          /**
           * Обработчик then выполняет переданный шаг вызова then в клиентской авторизации.
           *
           * @parameters:
           *   - объект параметров: user — публичные сведения пользователя.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ ({ user }) => {
            if (
              !controller.signal.aborted &&
              sessionRef.current?.token === current.token
            )
              setUser(user);
          },
        )
        .catch(
          /**
           * Обработчик catch выполняет переданный шаг вызова catch в клиентской авторизации.
           *
           * @parameters:
           *   - error (unknown) — пойманная ошибка API или сети.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ (error: unknown) => {
            if (controller.signal.aborted) return;
            if (error instanceof ApiError && error.status === 401) clear(true);
            else setStartupError(true);
          },
        )
        .finally(
          /**
           * Обработчик finally выполняет переданный шаг вызова finally в клиентской авторизации.
           *
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ () => {
            if (!controller.signal.aborted) setLoading(false);
          },
        );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: controller.abort().
       */
      return () => controller.abort();
    },
    [session?.token, attempt, clear],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!session) return;
      const timer = window.setTimeout(
        /**
         * Обработчик window.setTimeout выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns вычисленное значение: clear(true).
         */
        () => clear(true),
        Math.max(0, session.expiresAt - Date.now()),
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: window.clearTimeout(timer).
       */
      return () => window.clearTimeout(timer);
    },
    [session, clear],
  );
  /**
   * login отправляет учётные данные и получает токен и сведения пользователя.
   *
   * @parameters:
   *   - email (string) — адрес электронной почты.
   *   - password (string) — пароль из формы; не предназначен для журналирования.
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
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
  /**
   * logout отправляет запрос завершения авторизации.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
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
        /**
         * retry решает, допустим ли повтор запроса с учётом ошибки и числа отказов.
         *
         *
         * @returns вычисленное значение: setAttempt( (value) => value + 1, ).
         */
        retry: () =>
          setAttempt(
            /**
             * Обработчик setAttempt вычисляет следующее React-состояние из предыдущего значения.
             *
             * @parameters:
             *   - value — значение для проверки, преобразования или отображения.
             *
             * @returns следующее состояние, рассчитанное из предыдущего значения.
             */ (value) => value + 1,
          ),
      }}
    >
      {children}
    </Context.Provider>
  );
}
/**
 * useAuth возвращает авторизацию текущего React-контекста и сообщает об использовании вне провайдера.
 *
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useAuth() {
  const value = useContext(Context);
  if (!value) throw new Error("AuthProvider is missing");
  return value;
}
