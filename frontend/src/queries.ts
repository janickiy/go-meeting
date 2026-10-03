import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { useAuth } from "./auth";
import { ApiError } from "./api";
import type { ConferenceFilters } from "./types";

/**
 * useConferences читает бесконечный серверный список встреч и изолирует его кеш по пользователю и фильтрам.
 *
 * @args
 *   - filters (ConferenceFilters) — серверные фильтры списка встреч (по умолчанию {}).
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useConferences(filters: ConferenceFilters = {}) {
  const { user } = useAuth();
  return useInfiniteQuery({
    queryKey: ["conferences", user?.id, filters],
    enabled: !!user,
    initialPageParam: undefined as string | undefined,
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: pageParam — свойство текущего компонента; signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.myConferences(filters, pageParam, signal).
     */
    queryFn: ({ pageParam, signal }) =>
      api.myConferences(filters, pageParam, signal),
    /**
     * getNextPageParam извлекает курсор продолжения серверной страницы.
     *
     * @args
     *   - last — последняя загруженная страница, по которой определяется продолжение.
     *
     * @returns вычисленное значение: last.nextCursor || undefined.
     */
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
  });
}
/**
 * useConference читает и обновляет сведения одной доступной конференции.
 *
 * @args
 *   - id (string) — идентификатор ресурса или конференции данного запроса.
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useConference(id: string) {
  const { user } = useAuth();
  return useQuery({
    queryKey: ["conference", user?.id, id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.conference(id, signal).
     */
    queryFn: ({ signal }) => api.conference(id, signal),
    refetchInterval: 10000,
  });
}
/**
 * useParticipants читает состав встречи только при включённом запросе и обновляет его по принятому интервалу.
 *
 * @args
 *   - id (string) — идентификатор ресурса или конференции данного запроса.
 *   - enabled — разрешает выполнение запроса или подключение при выполненных условиях доступа (по умолчанию true).
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useParticipants(id: string, enabled = true) {
  const { user } = useAuth();
  return useInfiniteQuery({
    queryKey: ["participants", user?.id, id],
    enabled,
    initialPageParam: 0,
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: pageParam — свойство текущего компонента; signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.participants(id, pageParam, signal).
     */
    queryFn: ({ pageParam, signal }) => api.participants(id, pageParam, signal),
    /**
     * getNextPageParam извлекает курсор продолжения серверной страницы.
     *
     * @args
     *   - last — последняя загруженная страница, по которой определяется продолжение.
     *   - _pages — входное значение _pages текущего шага обработки.
     *   - offset — смещение страницы списка.
     *
     * @returns вычисленное значение: last.items.length === 100 ? offset + 100 : undefined.
     */
    getNextPageParam: (last, _pages, offset) =>
      last.items.length === 100 ? offset + 100 : undefined,
    refetchInterval: 10000,
  });
}
/**
 * useMembership читает собственное членство, чтобы ожидание не требовало доступа к общей комнате.
 *
 * @args
 *   - id (string) — идентификатор ресурса или конференции данного запроса.
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useMembership(
  id: string,
  refetchInterval: number | false = 3000,
) {
  const { user } = useAuth();
  return useQuery({
    queryKey: ["membership", user?.id, id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns Promise, который после завершения операции возвращает: вычисленное значение: (await api.myMembership(id, signal)).item; null.
     */
    queryFn: async ({ signal }) => {
      try {
        return (await api.myMembership(id, signal)).item;
      } catch (error) {
        if (error instanceof ApiError && [403, 404].includes(error.status))
          return null;
        throw error;
      }
    },
    refetchInterval,
  });
}
