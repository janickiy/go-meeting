import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { api } from "../api";
import type { Participant, ReactionEmoji } from "../types";
import type { useRealtime } from "../realtime";
import { ErrorNotice } from "./ui";

export const reactionEmoji: ReactionEmoji[] = ["👍", "👏", "❤️", "😂"];

/** Bubble хранит реакцию до истечения короткого срока показа.
 * @params id — идентификатор события; participantId — автор; emoji — реакция;
 * until — время удаления из интерфейса в миллисекундах.
 */
type Bubble = {
  id: string;
  participantId: string;
  emoji: ReactionEmoji;
  until: number;
};

/** ReactionsPanel отправляет emoji-реакции и показывает не более шести временных уведомлений.
 * Подписка не заменяет обработчики медиа; таймеры и подписка освобождаются при закрытии.
 * @args conferenceId — встреча; participants — доступные имена участников;
 * live — общее realtime-соединение встречи.
 * @return панель реакций с состоянием отправки и ошибкой запроса.
 */
export function ReactionsPanel({
  conferenceId,
  participants,
  live,
}: {
  conferenceId: string;
  participants: Participant[];
  live: ReturnType<typeof useRealtime>;
}) {
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [cooldown, setCooldown] = useState(false);
  useEffect(() => {
    setBubbles([]);
    return live.subscribe((event) => {
      if (event.type !== "reaction.created") return;
      const value = event.data as {
        participantId?: string;
        emoji?: ReactionEmoji;
      } | null;
      if (
        !value ||
        typeof value.participantId !== "string" ||
        !reactionEmoji.includes(value.emoji!)
      )
        return;
      setBubbles((old) =>
        [
          ...old.filter(
            (item) => item.id !== event.id && item.until > Date.now(),
          ),
          {
            id: event.id,
            participantId: value.participantId!,
            emoji: value.emoji!,
            until: Date.now() + 3500,
          },
        ].slice(-6),
      );
    });
  }, [live.subscribe, conferenceId]);
  useEffect(() => {
    const timer = setInterval(() => {
      setBubbles((old) =>
        old.some((item) => item.until <= Date.now())
          ? old.filter((item) => item.until > Date.now())
          : old,
      );
    }, 500);
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    if (!cooldown) return;
    const timer = setTimeout(() => setCooldown(false), 650);
    return () => clearTimeout(timer);
  }, [cooldown]);
  const reaction = useMutation({
    mutationFn: (emoji: ReactionEmoji) => api.reaction(conferenceId, emoji),
  });

  /** name выбирает доступное имя автора, не раскрывая сведения вне состава встречи.
   * @args id — идентификатор участника.
   * @return имя из текущего состава либо нейтральная подпись.
   */
  const name = (id: string) =>
    participants.find((person) => person.id === id)?.displayName || "Участник";

  return (
    <section className="content-card reactions-panel" aria-label="Реакции">
      <div className="collaboration-toolbar">
        <div className="reaction-buttons" aria-label="Отправить реакцию">
          {reactionEmoji.map((emoji) => (
            <button
              key={emoji}
              type="button"
              aria-label={`Реакция ${emoji}`}
              disabled={!live.state || cooldown || reaction.isPending}
              onClick={() => {
                setCooldown(true);
                reaction.mutate(emoji);
              }}
            >
              {emoji}
            </button>
          ))}
        </div>
      </div>
      <ErrorNotice error={reaction.error} />
      <div className="reaction-bubbles" aria-live="polite" aria-atomic="false">
        {bubbles.map((bubble) => (
          <span key={bubble.id} className="reaction-bubble">
            <span aria-hidden="true">{bubble.emoji}</span>
            {name(bubble.participantId)}
          </span>
        ))}
      </div>
    </section>
  );
}
