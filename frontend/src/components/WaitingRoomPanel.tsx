import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Clock3, ShieldCheck } from "lucide-react";
import { api } from "../api";
import type { Participant } from "../types";
import { Button, ErrorNotice } from "./ui";

/**
 * WaitingRoomPanel показывает собственное ожидание либо очередь организатора и разрешённые действия допуска.
 *
 * @args
 *   - объект параметров: conferenceId — идентификатор конференции и области данных; membership — свойство текущего компонента; participants — разрешённый состав участников; active — свойство текущего компонента; closed — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function WaitingRoomPanel({
  conferenceId,
  membership,
  participants,
  active,
  closed = false,
  standalone = false,
}: {
  conferenceId: string;
  membership?: Participant | null;
  participants: Participant[];
  active: boolean;
  closed?: boolean;
  /** Показывает лаконичный экран ожидания без оформления встроенной карточки. */
  standalone?: boolean;
}) {
  const client = useQueryClient();
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @args
     *   - объект параметров: id — идентификатор ресурса или конференции данного запроса; decision — решение admit или reject.
     *
     * @returns вычисленное значение: api.admit(conferenceId, id, decision).
     */
    mutationFn: ({
      id,
      decision,
    }: {
      id: string;
      decision: "admit" | "reject";
    }) => api.admit(conferenceId, id, decision),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
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
        className={
          standalone
            ? "waiting-room waiting-room-standalone"
            : "content-card waiting-room"
        }
        data-testid="waiting-room"
        role="status"
      >
        <span className="meeting-icon">
          <Clock3 size={25} />
        </span>
        {standalone ? (
          <h1>
            {closed
              ? "Встреча завершена"
              : rejected
                ? "Запрос отклонён"
                : membership?.status === "left"
                  ? "Запрос на вход приостановлен"
                  : "Вы в зале ожидания"}
          </h1>
        ) : (
          <h2>
            {closed
              ? "Встреча закрыта"
              : rejected
                ? "Вход во встречу отклонён"
                : membership?.status === "left"
                  ? "Запрос на вход приостановлен"
                  : "Вы в зале ожидания"}
          </h2>
        )}
        <p>
          {closed
            ? "Присоединиться к этой встрече больше нельзя."
            : membership?.status === "left"
              ? "Присоединитесь снова, чтобы организатор увидел ваш запрос."
              : rejected
                ? "Организатор отклонил ваш запрос. Камера, чат и файлы встречи недоступны."
                : standalone && active
                  ? "Организатор увидит ваш запрос. Как только вас допустят, страница обновится."
                  : !active
                    ? "Организатор рассмотрит запрос после начала встречи. Камера, микрофон и чат пока недоступны."
                    : "Организатор скоро рассмотрит ваш запрос. Пока вас не пригласили, камера, микрофон и чат недоступны."}
        </p>
        {!standalone &&
          !rejected &&
          !closed &&
          membership?.status !== "left" && (
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
    /**
     * Обработчик participants.filter проверяет, должен ли элемент войти в отфильтрованный набор.
     *
     * @args
     *   - person — целевое членство участника.
     *
     * @returns логический признак соответствия элемента условию.
     */
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
      {pending.map(
        /**
         * Обработчик pending.map преобразует один элемент набора в представление или данные следующего шага.
         *
         * @args
         *   - person — целевое членство участника.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (person) => (
          <div className="waiting-person" key={person.id}>
            <strong>{person.displayName}</strong>
            <div className="meeting-actions">
              <Button
                disabled={!active || mutation.isPending}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                   */ () =>
                    mutation.mutate({ id: person.id, decision: "admit" })
                }
                aria-label={`Допустить: ${person.displayName}`}
              >
                Допустить
              </Button>
              <Button
                variant="outline"
                disabled={!active || mutation.isPending}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                   */ () =>
                    mutation.mutate({ id: person.id, decision: "reject" })
                }
                aria-label={`Отклонить: ${person.displayName}`}
              >
                Отклонить
              </Button>
            </div>
          </div>
        ),
      )}
    </section>
  );
}
