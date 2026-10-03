import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Hand } from "lucide-react";
import { api } from "../api";
import type { Items, Participant, RaisedHand, ReactionEmoji } from "../types";
import type { useRealtime } from "../realtime";
import { Button, ErrorNotice } from "./ui";

export const reactionEmoji: ReactionEmoji[] = ["👍", "👏", "❤️", "😂"];
/**
 * Bubble описывает временную реакцию, автора и время удаления из интерфейса.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - participantId — идентификатор членства целевого участника.
 *   - emoji — одна из четырёх допустимых реакций.
 *   - until — поле или операция этого контракта.
 */
type Bubble = {
  id: string;
  participantId: string;
  emoji: ReactionEmoji;
  until: number;
};
/**
 * HandReactionsPanel показывает поднятые руки и ограниченные временные реакции, восстанавливая состояние по снимкам.
 *
 * @args
 *   - объект параметров: conferenceId — идентификатор конференции и области данных; membership — свойство текущего компонента; participants — разрешённый состав участников; live — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function HandReactionsPanel({
  conferenceId,
  membership,
  participants,
  live,
  handShortcutToken = 0,
}: {
  conferenceId: string;
  membership: Participant;
  participants: Participant[];
  live: ReturnType<typeof useRealtime>;
  handShortcutToken?: number;
}) {
  const client = useQueryClient();
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [cooldown, setCooldown] = useState(false);
  const query = useQuery({
    queryKey: ["hands", conferenceId],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.hands(conferenceId, signal).
     */
    queryFn: ({ signal }) => api.hands(conferenceId, signal),
    refetchInterval: 15000,
  });
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */
    () =>
      live.subscribe(
        /**
         * Обработчик live.subscribe выполняет переданный шаг вызова live.subscribe в интерфейсе Meet.
         *
         * @args
         *   - event — проверенный конверт события комнаты.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ (event) => {
          if (event.type === "hand.raised" || event.type === "hand.lowered") {
            const value = event.data as Partial<RaisedHand> | null;
            if (!value || typeof value.participantId !== "string") return;
            client.setQueryData<Items<RaisedHand>>(
              ["hands", conferenceId],
              /**
               * Обработчик client.setQueryData выполняет переданный шаг вызова client.setQueryData в интерфейсе Meet.
               *
               * @args
               *   - old — предыдущее состояние перед вычислением нового.
               *
               * @returns объект с данными, собранными в текущей операции.
               */
              (old) => {
                const items = (old?.items || []).filter(
                  /**
                   * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
                   *
                   * @args
                   *   - hand — участник с поднятой рукой и время её поднятия.
                   *
                   * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                   */
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
            setBubbles(
              /**
               * Обработчик setBubbles вычисляет следующее React-состояние из предыдущего значения.
               *
               * @args
               *   - old — предыдущее состояние перед вычислением нового.
               *
               * @returns следующее состояние, рассчитанное из предыдущего значения.
               */ (old) =>
                [
                  ...old.filter(
                    /**
                     * Обработчик old.filter проверяет, должен ли элемент войти в отфильтрованный набор.
                     *
                     * @args
                     *   - item — элемент списка, который обрабатывает текущий шаг.
                     *
                     * @returns логический признак соответствия элемента условию.
                     */
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
        },
      ),
    [live.subscribe, client, conferenceId],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (live.state?.hands)
        client.setQueryData<Items<RaisedHand>>(["hands", conferenceId], {
          status: "success",
          items: live.state.hands,
        });
    },
    [live.state?.connectionId, live.state?.hands, conferenceId, client],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      const timer = setInterval(
        /**
         * Обработчик setInterval выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */
        () =>
          setBubbles(
            /**
             * Обработчик setBubbles вычисляет следующее React-состояние из предыдущего значения.
             *
             * @args
             *   - old — предыдущее состояние перед вычислением нового.
             *
             * @returns следующее состояние, рассчитанное из предыдущего значения.
             */ (old) =>
              old.some(
                /**
                 * Обработчик old.some проверяет условие поиска элемента или соответствия элементов набора.
                 *
                 * @args
                 *   - item — элемент списка, который обрабатывает текущий шаг.
                 *
                 * @returns логический признак соответствия элемента условию.
                 */ (item) => item.until <= Date.now(),
              )
                ? old.filter(
                    /**
                     * Обработчик old.filter проверяет, должен ли элемент войти в отфильтрованный набор.
                     *
                     * @args
                     *   - item — элемент списка, который обрабатывает текущий шаг.
                     *
                     * @returns логический признак соответствия элемента условию.
                     */ (item) => item.until > Date.now(),
                  )
                : old,
          ),
        500,
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: clearInterval(timer).
       */
      return () => clearInterval(timer);
    },
    [],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!cooldown) return;
      const timer = setTimeout(
        /**
         * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */ () => setCooldown(false),
        650,
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: clearTimeout(timer).
       */
      return () => clearTimeout(timer);
    },
    [cooldown],
  );
  const hands = query.data?.items || [];
  const raised = hands.some(
    /**
     * Обработчик hands.some проверяет условие поиска элемента или соответствия элементов набора.
     *
     * @args
     *   - item — элемент списка, который обрабатывает текущий шаг.
     *
     * @returns логический признак соответствия элемента условию.
     */ (item) => item.participantId === membership.id,
  );
  const hand = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @args
     *   - объект параметров: participantId — идентификатор членства целевого участника; raised — true поднимает руку, false опускает её.
     *
     * @returns вычисленное значение: api.hand(conferenceId, participantId, raised).
     */
    mutationFn: ({
      participantId,
      raised,
    }: {
      participantId: string;
      raised: boolean;
    }) => api.hand(conferenceId, participantId, raised),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["hands", conferenceId] });
    },
  });
  const lastShortcutToken = useRef(handShortcutToken);
  useEffect(() => {
    if (lastShortcutToken.current === handShortcutToken) return;
    lastShortcutToken.current = handShortcutToken;
    if (!live.state || hand.isPending || query.isPending) return;
    hand.mutate({ participantId: membership.id, raised: !raised });
  }, [
    handShortcutToken,
    hand,
    live.state,
    membership.id,
    query.isPending,
    raised,
  ]);
  const reaction = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @args
     *   - emoji (ReactionEmoji) — одна из четырёх допустимых реакций.
     *
     * @returns вычисленное значение: api.reaction(conferenceId, emoji).
     */
    mutationFn: (emoji: ReactionEmoji) => api.reaction(conferenceId, emoji),
  });
  /**
   * name выбирает отображаемое имя участника реакции или поднятой руки.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *
   * @returns вычисленное значение: participants.find( (person) => person.id === id, )?.displayName || "Участник".
   */
  const name = (id: string) =>
    participants.find(
      /**
       * Обработчик participants.find проверяет условие поиска элемента или соответствия элементов набора.
       *
       * @args
       *   - person — целевое членство участника.
       *
       * @returns логический признак соответствия элемента условию.
       */ (person) => person.id === id,
    )?.displayName || "Участник";
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
          aria-keyshortcuts="H"
          onClick={
            /**
             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
             */ () =>
              hand.mutate({ participantId: membership.id, raised: !raised })
          }
        >
          <Hand size={18} />
          {raised ? "Опустить руку" : "Поднять руку"}
        </Button>
        <div className="reaction-buttons" aria-label="Отправить реакцию">
          {reactionEmoji.map(
            /**
             * Обработчик reactionEmoji.map преобразует один элемент набора в представление или данные следующего шага.
             *
             * @args
             *   - emoji — одна из четырёх допустимых реакций.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ (emoji) => (
              <button
                key={emoji}
                type="button"
                aria-label={`Реакция ${emoji}`}
                disabled={!live.state || cooldown || reaction.isPending}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                   */ () => {
                    setCooldown(true);
                    reaction.mutate(emoji);
                  }
                }
              >
                {emoji}
              </button>
            ),
          )}
        </div>
      </div>
      <ErrorNotice error={query.error || hand.error || reaction.error} />
      {hands.length > 0 && (
        <ul className="raised-hands" aria-label="Поднятые руки">
          {hands.map(
            /**
             * Обработчик hands.map преобразует один элемент набора в представление или данные следующего шага.
             *
             * @args
             *   - item — элемент списка, который обрабатывает текущий шаг.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ (item) => (
              <li key={item.participantId}>
                <Hand size={16} />
                <strong>{name(item.participantId)}</strong>
                {moderator && item.participantId !== membership.id && (
                  <button
                    className="text-link"
                    disabled={hand.isPending}
                    onClick={
                      /**
                       * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       *
                       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                       */ () =>
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
            ),
          )}
        </ul>
      )}
      <div className="reaction-bubbles" aria-live="polite" aria-atomic="false">
        {bubbles.map(
          /**
           * Обработчик bubbles.map преобразует один элемент набора в представление или данные следующего шага.
           *
           * @args
           *   - bubble — временная реакция, отображаемая до истечения её срока.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (bubble) => (
            <span key={bubble.id} className="reaction-bubble">
              <span aria-hidden="true">{bubble.emoji}</span>
              {name(bubble.participantId)}
            </span>
          ),
        )}
      </div>
    </section>
  );
}
