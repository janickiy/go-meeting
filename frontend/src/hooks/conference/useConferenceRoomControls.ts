import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { meetingShortcut } from "../../conferenceShortcuts";

/** Названия переключаемых панелей активной комнаты. */
export type ConferenceStagePanel = "chat" | "participants" | "captions";

/**
 * Хранит состояние панелей, диалогов и клавиатуры независимо от активного представления.
 * Вкладки остаются смонтированными при скрытии; фокус возвращается на открывшую панель кнопку.
 * @args id — встреча для фокуса поля чата; activeMeeting — включает горячие клавиши;
 * captionsEnabled — серверное разрешение вкладки субтитров.
 * @return Состояние интерфейса, ссылки на элементы и операции переключения панелей.
 */
export function useConferenceRoomControls(
  id: string,
  activeMeeting: boolean,
  captionsEnabled: boolean,
) {
  const [params, setParams] = useSearchParams();
  const focusMessageId = params.get("message") || undefined;
  const showChat = params.get("chat") === "1";
  const [stagePanel, setStagePanel] = useState<ConferenceStagePanel>("chat");
  const [panelOpen, setPanelOpen] = useState(
    () => showChat || typeof window === "undefined" || window.innerWidth > 900,
  );
  const panelTrigger = useRef<HTMLButtonElement | null>(null);
  const [reconnectTarget, setReconnectTarget] = useState<HTMLDivElement | null>(
    null,
  );
  const [utility, setUtility] = useState<"recording" | "invite" | null>(null);
  const [invitationBusy, setInvitationBusy] = useState(false);
  const [confirm, setConfirm] = useState<"finish" | "cancel" | null>(null);
  const [editingSchedule, setEditingSchedule] = useState(false);

  useEffect(() => {
    if (showChat) {
      setPanelOpen(true);
      setStagePanel("chat");
    }
  }, [showChat, focusMessageId]);
  useEffect(() => {
    if (!captionsEnabled && stagePanel === "captions") setStagePanel("chat");
  }, [captionsEnabled, stagePanel]);
  useEffect(() => {
    if (!activeMeeting) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        event.key === "Escape" &&
        !document.querySelector('[role="dialog"]')
      ) {
        setPanelOpen(false);
        panelTrigger.current?.focus();
        return;
      }
      if (meetingShortcut(event, ["c"]) === "c") {
        event.preventDefault();
        setStagePanel("chat");
        setPanelOpen(true);
        window.setTimeout(
          () => document.getElementById(`chat-text-${id}`)?.focus(),
          0,
        );
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [activeMeeting, id]);

  /** Убирает фокус старого сообщения из адреса, сохраняя остальные параметры страницы. */
  const latestMessages = () => {
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.delete("message");
        return next;
      },
      { replace: true },
    );
  };

  return {
    focusMessageId,
    latestMessages,
    stagePanel,
    setStagePanel,
    panelOpen,
    setPanelOpen,
    panelTrigger,
    reconnectTarget,
    setReconnectTarget,
    utility,
    setUtility,
    invitationBusy,
    setInvitationBusy,
    confirm,
    setConfirm,
    editingSchedule,
    setEditingSchedule,
  };
}

/** Состояние панелей и диалогов, сохраняемое корневой страницей при смене статуса встречи. */
export type ConferenceRoomControls = ReturnType<
  typeof useConferenceRoomControls
>;
