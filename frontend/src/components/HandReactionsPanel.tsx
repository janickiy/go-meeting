import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Hand } from "lucide-react";
import { api } from "../api";
import type { Items, Participant, RaisedHand, ReactionEmoji } from "../types";
import type { useRealtime } from "../realtime";
import { Button, ErrorNotice } from "./ui";

export const reactionEmoji: ReactionEmoji[] = ["👍", "👏", "❤️", "😂"];
type Bubble = {
  id: string;
  participantId: string;
  emoji: ReactionEmoji;
  until: number;
};
export function HandReactionsPanel({
  conferenceId,
  membership,
  participants,
  live,
}: {
  conferenceId: string;
  membership: Participant;
  participants: Participant[];
  live: ReturnType<typeof useRealtime>;
}) {
  const client = useQueryClient();
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [cooldown, setCooldown] = useState(false);
  const query = useQuery({
    queryKey: ["hands", conferenceId],
    queryFn: ({ signal }) => api.hands(conferenceId, signal),
    refetchInterval: 15000,
  });
  useEffect(
    () =>
      live.subscribe((event) => {
        if (event.type === "hand.raised" || event.type === "hand.lowered") {
          const value = event.data as Partial<RaisedHand> | null;
          if (!value || typeof value.participantId !== "string") return;
          client.setQueryData<Items<RaisedHand>>(
            ["hands", conferenceId],
            (old) => {
              const items = (old?.items || []).filter(
                (hand) => hand.participantId !== value.participantId,
              );
              if (
                event.type === "hand.raised" &&
                typeof value.raisedAt === "string"
              )
                items.push({
                  participantId: value.participantId!,
                  raisedAt: value.raisedAt,
                });
              return { status: "success", items };
            },
          );
        }
        if (event.type === "reaction.created") {
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
        }
      }),
    [live.subscribe, client, conferenceId],
  );
  useEffect(() => {
    if (live.state?.hands)
      client.setQueryData<Items<RaisedHand>>(["hands", conferenceId], {
        status: "success",
        items: live.state.hands,
      });
  }, [live.state?.connectionId, live.state?.hands, conferenceId, client]);
  useEffect(() => {
    const timer = setInterval(
      () =>
        setBubbles((old) =>
          old.some((item) => item.until <= Date.now())
            ? old.filter((item) => item.until > Date.now())
            : old,
        ),
      500,
    );
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    if (!cooldown) return;
    const timer = setTimeout(() => setCooldown(false), 650);
    return () => clearTimeout(timer);
  }, [cooldown]);
  const hands = query.data?.items || [];
  const raised = hands.some((item) => item.participantId === membership.id);
  const hand = useMutation({
    mutationFn: ({
      participantId,
      raised,
    }: {
      participantId: string;
      raised: boolean;
    }) => api.hand(conferenceId, participantId, raised),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["hands", conferenceId] });
    },
  });
  const reaction = useMutation({
    mutationFn: (emoji: ReactionEmoji) => api.reaction(conferenceId, emoji),
  });
  const name = (id: string) =>
    participants.find((person) => person.id === id)?.displayName || "Участник";
  const moderator = ["owner", "co_host"].includes(membership.role);
  return (
    <section
      className="content-card hand-reactions"
      aria-label="Руки и реакции"
    >
      <div className="collaboration-toolbar">
        <Button
          variant={raised ? "primary" : "outline"}
          busy={hand.isPending}
          disabled={!live.state}
          onClick={() =>
            hand.mutate({ participantId: membership.id, raised: !raised })
          }
        >
          <Hand size={18} />
          {raised ? "Опустить руку" : "Поднять руку"}
        </Button>
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
      <ErrorNotice error={query.error || hand.error || reaction.error} />
      {hands.length > 0 && (
        <ul className="raised-hands" aria-label="Поднятые руки">
          {hands.map((item) => (
            <li key={item.participantId}>
              <Hand size={16} />
              <strong>{name(item.participantId)}</strong>
              {moderator && item.participantId !== membership.id && (
                <button
                  className="text-link"
                  disabled={hand.isPending}
                  onClick={() =>
                    hand.mutate({
                      participantId: item.participantId,
                      raised: false,
                    })
                  }
                >
                  Опустить руку: {name(item.participantId)}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
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
