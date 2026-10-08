import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import type { ConferenceRecording, Items, ModerationAction } from "../../types";

/** Разрешённые переходы встречи и команды собственного членства. */
export type ConferenceCommand =
  "start" | "finish" | "cancel" | "join" | "leave";

/**
 * Выполняет серверные команды встречи без оптимистического изменения прав участников.
 * Подтверждённая остановка записи сразу обновляет общий кеш; после любой команды
 * затронутые запросы сверяются с сервером, включая ответы с ошибками.
 * @args id — встреча; onTransitionSuccess — закрытие подтверждения после успешного перехода.
 * @return Отдельные состояния команд встречи, модерации и остановки записи.
 */
export function useConferenceCommands(
  id: string,
  onTransitionSuccess: () => void,
) {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (action: ConferenceCommand) => {
      if (action === "join" || action === "leave")
        await api.membership(id, action);
      else await api.transition(id, action);
    },
    onSettled: () => {
      for (const key of [
        "conference",
        "conferences",
        "participants",
        "membership",
        "history",
      ])
        void client.invalidateQueries({ queryKey: [key] });
    },
    onSuccess: onTransitionSuccess,
  });
  const moderation = useMutation({
    mutationFn: ({
      participantId,
      action,
    }: {
      participantId: string;
      action: ModerationAction;
    }) => api.moderate(id, participantId, action),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["membership"] });
    },
  });
  const stopRecording = useMutation({
    mutationFn: (recordingId: string) => api.stopRecording(id, recordingId),
    onSuccess: (response) => {
      client.setQueryData<Items<ConferenceRecording>>(
        ["recordings", id],
        (cached) =>
          cached && {
            ...cached,
            items: cached.items.map((item) =>
              item.uuid === response.item.uuid ? response.item : item,
            ),
          },
      );
    },
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["recordings", id] });
    },
  });
  return { mutation, moderation, stopRecording };
}

/** Серверные команды и состояния их выполнения для всех представлений страницы. */
export type ConferenceCommands = ReturnType<typeof useConferenceCommands>;
