import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { useAuth } from "./auth";

/** Получает действующие серверные флаги; неизвестное состояние не выдаётся за доступную функцию. */
export function useCapabilities() {
  const { user } = useAuth();
  return useQuery({
    queryKey: ["product-capabilities", user?.id],
    queryFn: ({ signal }) => api.capabilities(signal),
    enabled: !!user,
    staleTime: 60_000,
  });
}
