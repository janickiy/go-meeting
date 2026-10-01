import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { useAuth } from "./auth";

export function useConferences() {
  const { user } = useAuth();
  return useInfiniteQuery({
    queryKey: ["conferences", user?.id],
    enabled: !!user,
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => api.conferences(pageParam, signal),
    getNextPageParam: (last, _pages, offset) =>
      last.items.length === 20 ? offset + 20 : undefined,
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
export function useParticipants(id: string) {
  const { user } = useAuth();
  return useInfiniteQuery({
    queryKey: ["participants", user?.id, id],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => api.participants(id, pageParam, signal),
    getNextPageParam: (last, _pages, offset) =>
      last.items.length === 100 ? offset + 100 : undefined,
    refetchInterval: 10000,
  });
}
