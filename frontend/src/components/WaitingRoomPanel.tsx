import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Clock3, ShieldCheck } from "lucide-react";
import { api } from "../api";
import type { Participant } from "../types";
import { Button, ErrorNotice } from "./ui";

export function WaitingRoomPanel({
  conferenceId,
  membership,
  participants,
  active,
  closed = false,
}: {
  conferenceId: string;
  membership?: Participant | null;
  participants: Participant[];
  active: boolean;
  closed?: boolean;
}) {
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({
      id,
      decision,
    }: {
      id: string;
      decision: "admit" | "reject";
    }) => api.admit(conferenceId, id, decision),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["membership"] });
    },
  });
  const waiting =
    membership?.admissionState === "waiting" ||
    membership?.status === "waiting";
  const rejected =
    membership?.admissionState === "rejected" ||
    membership?.status === "rejected";
  if (waiting || rejected)
    return (
      <section
        className="content-card waiting-room"
        data-testid="waiting-room"
        role="status"
      >
        <span className="meeting-icon">
          <Clock3 size={25} />
        </span>
        <h2>
          {closed
            ? "Встреча закрыта"
            : rejected
              ? "Вход во встречу отклонён"
              : membership?.status === "left"
                ? "Запрос на вход приостановлен"
                : "Вы в зале ожидания"}
        </h2>
        <p>
          {closed
            ? "Встреча завершена. Допуск в неё больше недоступен."
            : membership?.status === "left"
              ? "Присоединитесь снова, чтобы организатор увидел ваш запрос."
              : rejected
                ? "Организатор отклонил ваш запрос. Камера, чат и файлы встречи недоступны."
                : !active
                  ? "Организатор рассмотрит запрос после начала встречи. Камера, микрофон и чат пока недоступны."
                  : "Организатор скоро рассмотрит ваш запрос. Пока вас не пригласили, камера, микрофон и чат недоступны."}
        </p>
        {!rejected && !closed && membership?.status !== "left" && (
          <p className="field-hint">
            Можно оставить эту страницу открытой. Допуск обновится
            автоматически, в том числе после переподключения.
          </p>
        )}
      </section>
    );
  const moderator =
    membership?.status === "joined" &&
    ["owner", "co_host"].includes(membership.role);
  const pending = participants.filter(
    (person) =>
      person.status === "waiting" &&
      (!person.admissionState || person.admissionState === "waiting"),
  );
  if (!moderator || !pending.length) return null;
  return (
    <section
      className="content-card waiting-room-management"
      aria-label="Зал ожидания"
    >
      <div className="section-heading">
        <h2>
          <ShieldCheck size={20} />
          Ожидают допуска <span className="count-badge">{pending.length}</span>
        </h2>
      </div>
      <ErrorNotice error={mutation.error} />
      {!active && (
        <p className="field-hint">
          Допустить участников можно после начала встречи.
        </p>
      )}
      {pending.map((person) => (
        <div className="waiting-person" key={person.id}>
          <strong>{person.displayName}</strong>
          <div className="meeting-actions">
            <Button
              disabled={!active || mutation.isPending}
              onClick={() =>
                mutation.mutate({ id: person.id, decision: "admit" })
              }
              aria-label={`Допустить: ${person.displayName}`}
            >
              Допустить
            </Button>
            <Button
              variant="outline"
              disabled={!active || mutation.isPending}
              onClick={() =>
                mutation.mutate({ id: person.id, decision: "reject" })
              }
              aria-label={`Отклонить: ${person.displayName}`}
            >
              Отклонить
            </Button>
          </div>
        </div>
      ))}
    </section>
  );
}
