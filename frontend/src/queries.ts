import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { useAuth } from "./auth";
import { ApiError } from "./api";
import type { ConferenceFilters } from "./types";

export function useConferences(filters: ConferenceFilters = {}) {
  const { user } = useAuth();
  return useInfiniteQuery({
    queryKey: ["conferences", user?.id, filters],
    enabled: !!user,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.myConferences(filters, pageParam, signal),
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
  });
}
export function useConference(id: string) {
  const { user } = useAuth();
  return useQuery({
    queryKey: ["conference", user?.id, id],
    queryFn: ({ signal }) => api.conference(id, signal),
    refetchInterval: 10000,
  });
}
export function useParticipants(id: string, enabled = true) {
  const { user } = useAuth();
  return useInfiniteQuery({
    queryKey: ["participants", user?.id, id],
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => api.participants(id, pageParam, signal),
    getNextPageParam: (last, _pages, offset) =>
      last.items.length === 100 ? offset + 100 : undefined,
    refetchInterval: 10000,
  });
}
export function useMembership(id: string) {
  const { user } = useAuth();
  return useQuery({
    queryKey: ["membership", user?.id, id],
    queryFn: async ({ signal }) => {
      try {
        return (await api.myMembership(id, signal)).item;
      } catch (error) {
        if (error instanceof ApiError && [403, 404].includes(error.status))
          return null;
        throw error;
      }
    },
    refetchInterval: 3000,
  });
}
