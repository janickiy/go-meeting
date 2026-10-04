import { useEffect, useRef, useState } from "react";
import { Circle, X } from "lucide-react";
import { isAdmitted } from "../collaboration";
import type { ConferenceRecording, Participant, RealtimeEvent } from "../types";
import "./RecordingNotice.css";

const activeStatuses = new Set(["recording", "degraded"]);
const endedStatuses = new Set([
  "stopping",
  "processing",
  "ready",
  "failed",
  "cancelled",
  "ended",
]);
const endedEvents = new Set([
  "recording.stopping",
  "recording.processing",
  "recording.ready",
  "recording.failed",
  "recording.cancelled",
  "recording.ended",
  "recording.stopped",
]);
const modes = new Set([
  "composite",
  "audio_only",
  "individual_tracks",
  "screen_focus",
]);
const maxRemembered = 64;

/**
 * Описывает разрешённые данные текущей комнаты и подписку на уже существующий транспорт.
 * @params conferenceId — текущая встреча; ownerId — пользователь-организатор; userId — текущий пользователь;
 * participants — разрешённый состав с собственным членством; recordings — подтверждённый снимок записей
 * либо undefined до успешной загрузки; subscribe — подписка единственного транспорта комнаты с очисткой.
 */
export interface RecordingNoticeProps {
  conferenceId: string;
  ownerId: string;
  userId: string | null | undefined;
  participants: readonly Participant[];
  recordings?: readonly ConferenceRecording[];
  subscribe: (listener: (event: RealtimeEvent) => void) => () => void;
}

/** Хранит факт уведомления и запрет повторного объявления завершённой записи. */
interface RememberedRecording {
  notified: boolean;
  ended: boolean;
}

/** Связывает видимую плашку с записью, комнатой и инициатором подтверждённого события. */
interface Notice {
  conferenceId: string;
  recordingId: string;
  actorId?: string;
}

/**
 * Проверяет ограниченный непустой идентификатор, не используя его как HTML или адрес ресурса.
 * @args value — недоверенное поле события или снимка.
 * @return true для строкового идентификатора без пробелов длиной не более 128 символов.
 */
function identifier(value: unknown): value is string {
  return typeof value === "string" && /^[A-Za-z0-9_-]{1,128}$/.test(value);
}

/**
 * Проверяет членство в разрешённом составе, включая текущий допуск и статус присоединения.
 * @args participant — проверяемое членство; conferenceId — текущая встреча.
 * @return true, когда участник присоединился и допущен именно в эту встречу.
 */
function joined(participant: Participant, conferenceId: string) {
  return (
    participant.conferenceId === conferenceId &&
    participant.status === "joined" &&
    isAdmitted(participant)
  );
}

/**
 * Объявляет остальным допущенным участникам фактический запуск записи, без новых запросов и соединений.
 * Закрытие запоминается по UUID записи; starting и повторная доставка не создают новых объявлений.
 * @args props — идентификаторы, разрешённый состав, подтверждённые записи и общая подписка комнаты.
 * @return Доступная закрываемая плашка либо отсутствие уведомления.
 */
export function RecordingNotice(props: RecordingNoticeProps) {
  const { conferenceId, userId, recordings, subscribe } = props;
  const latest = useRef(props);
  latest.current = props;
  const memory = useRef(new Map<string, RememberedRecording>());
  const visible = useRef<Notice | null>(null);
  const [notice, setNotice] = useState<Notice | null>(null);
  const eligible =
    !!userId &&
    props.participants.some(
      (person) => person.userId === userId && joined(person, conferenceId),
    );

  /**
   * Запоминает состояние UUID, ограничивая память последними 64 различными записями.
   * @args recordingId — UUID записи; state — подтверждённый факт уведомления или завершения.
   * @return Значение не возвращается; старейшая запись вытесняется при превышении лимита.
   */
  function remember(recordingId: string, state: RememberedRecording) {
    memory.current.set(recordingId, state);
    if (memory.current.size > maxRemembered) {
      const activeIds = new Set(
        latest.current.recordings
          ?.filter(
            (item) =>
              item.conferenceId === latest.current.conferenceId &&
              activeStatuses.has(item.status),
          )
          .map((item) => item.uuid),
      );
      // История не должна вытеснить дедуп активного UUID и повторно открыть закрытую пользователем плашку.
      const oldest =
        Array.from(memory.current.keys()).find(
          (id) => !activeIds.has(id) && id !== visible.current?.recordingId,
        ) ?? memory.current.keys().next().value;
      if (oldest !== undefined) memory.current.delete(oldest);
    }
  }

  /**
   * Убирает плашку только нужного UUID и блокирует его запоздалое повторное начало.
   * @args recordingId — завершённая или останавливаемая запись текущей комнаты.
   * @return Значение не возвращается; остальные уведомления не изменяются.
   */
  function end(recordingId: string) {
    remember(recordingId, {
      notified: memory.current.get(recordingId)?.notified ?? false,
      ended: true,
    });
    if (visible.current?.recordingId === recordingId) {
      visible.current = null;
      setNotice(null);
    }
  }

  /**
   * Показывает одно объявление на UUID, исключая инициатора и повторы после ручного закрытия.
   * @args recordingId — подтверждённая активная запись; actorId — инициатор события, без него используется опрос.
   * @return Значение не возвращается; недоступное либо уже показанное объявление игнорируется.
   */
  function announce(recordingId: string, actorId?: string) {
    const context = latest.current;
    if (
      !context.userId ||
      !context.participants.some(
        (person) =>
          person.userId === context.userId &&
          joined(person, context.conferenceId),
      )
    )
      return;
    const previous = memory.current.get(recordingId);
    if (previous?.notified || previous?.ended) return;
    remember(recordingId, { notified: true, ended: false });
    if (
      actorId ? actorId === context.userId : context.ownerId === context.userId
    )
      return;
    const next = { conferenceId: context.conferenceId, recordingId, actorId };
    visible.current = next;
    setNotice(next);
  }

  useEffect(() => {
    memory.current.clear();
    visible.current = null;
    setNotice(null);
  }, [conferenceId, userId]);

  useEffect(() => {
    if (!eligible) {
      visible.current = null;
      setNotice(null);
    }
    if (recordings === undefined) return;
    const confirmed = recordings.filter(
      (item) => identifier(item.uuid) && item.conferenceId === conferenceId,
    );
    for (const item of confirmed) {
      if (endedStatuses.has(item.status)) end(item.uuid);
    }
    // Пустой снимок может завершить запрос, начатый до события: отсутствие UUID не доказывает остановку.
    const active = confirmed.find(
      (item) =>
        activeStatuses.has(item.status) &&
        !memory.current.get(item.uuid)?.ended,
    );
    if (active) announce(active.uuid);
  }, [conferenceId, userId, recordings, eligible]);

  useEffect(() => {
    let disposed = false;
    const unsubscribe = subscribe((event) => {
      if (
        disposed ||
        !event ||
        event.version !== 1 ||
        event.conferenceId !== conferenceId ||
        !event.data ||
        typeof event.data !== "object" ||
        Array.isArray(event.data)
      )
        return;
      const data = event.data as Record<string, unknown>;
      if (
        data.conferenceId !== conferenceId ||
        !identifier(data.recordingId) ||
        typeof data.status !== "string"
      )
        return;
      if (endedEvents.has(event.type) && endedStatuses.has(data.status)) {
        end(data.recordingId);
        return;
      }
      if (
        event.type !== "recording.started" ||
        !activeStatuses.has(data.status) ||
        !identifier(data.requestedBy) ||
        typeof data.mode !== "string" ||
        !modes.has(data.mode)
      )
        return;
      const confirmed = latest.current.recordings?.find(
        (item) =>
          item.uuid === data.recordingId && item.conferenceId === conferenceId,
      );
      if (confirmed && endedStatuses.has(confirmed.status)) {
        end(data.recordingId);
        return;
      }
      announce(data.recordingId, data.requestedBy);
    });
    return () => {
      disposed = true;
      unsubscribe();
    };
  }, [conferenceId, userId, subscribe]);

  if (!notice || notice.conferenceId !== conferenceId || !eligible) return null;
  const initiator = props.participants.find(
    (person) =>
      person.userId === notice.actorId && joined(person, conferenceId),
  );
  const name =
    typeof initiator?.displayName === "string"
      ? initiator.displayName.trim()
      : "";
  const text = notice.actorId
    ? name
      ? `Началась запись встречи. Инициатор: ${name}.`
      : "Организатор начал запись встречи."
    : "Во встрече идёт запись.";
  return (
    <div
      className="room-recording-notice"
      role="status"
      aria-live="polite"
      aria-atomic="true"
      data-recording-id={notice.recordingId}
    >
      <Circle
        className="room-recording-notice-mark"
        size={13}
        fill="currentColor"
        aria-hidden="true"
      />
      <span>{text}</span>
      <button
        type="button"
        className="room-recording-notice-dismiss"
        aria-label="Скрыть уведомление о записи"
        onClick={() => {
          visible.current = null;
          setNotice(null);
        }}
      >
        <X size={19} aria-hidden="true" />
      </button>
    </div>
  );
}
