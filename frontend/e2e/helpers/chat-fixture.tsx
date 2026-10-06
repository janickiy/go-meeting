import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { Mic, Video, MonitorUp, Users, X } from "lucide-react";
import { api } from "../../src/api";
import { AuthProvider, useAuth } from "../../src/auth";
import { ChatPanel } from "../../src/components/ChatPanel";
import { MediaTile } from "../../src/components/RealtimePanel";
import { Button } from "../../src/components/ui";
import { saveSession } from "../../src/utils";
import type {
  ChatAttachment,
  ChatMessage,
  LoginResponse,
  Participant,
  User,
} from "../../src/types";
import "../../src/styles.css";
import "../../src/pages/conference.css";

type SendBody = Parameters<typeof api.sendMessage>[1];
let sent: SendBody[] = [];
let history: ChatMessage[] = [];
const attachments = new Map<string, ChatAttachment>();

const user: User = {
  id: "chat-fixture-self",
  email: "chat@example.test",
  displayName: "Александр",
  createdAt: "2026-10-03T10:00:00Z",
  updatedAt: "2026-10-03T10:00:00Z",
};

const member = {
  id: "chat-fixture-member",
  userId: user.id,
  role: "owner",
  status: "joined",
  admissionState: "admitted",
} as Participant;

/**
 * fixtureMessage создаёт обычное серверное сообщение без искусственных статусов прочтения.
 * @args index — порядковый номер; text — содержимое; own — автор текущий пользователь.
 * @return сообщение для автономной проверки настоящего интерфейса чата.
 */
function fixtureMessage(index: number, text: string, own = false): ChatMessage {
  const date = new Date();
  date.setHours(11, Math.min(index, 59), 0, 0);
  return {
    id: `history-${index}`,
    sequence: String(index),
    conferenceId: "chat-fixture",
    senderId: own ? user.id : "chat-fixture-maria",
    senderName: own ? "Александр" : "Мария",
    text,
    createdAt: date.toISOString(),
    updatedAt: date.toISOString(),
    deletedAt: null,
    version: 1,
    attachments: [],
  };
}

/**
 * Fixture показывает панель в реальных классах комнаты без SFU и физических устройств.
 * @return изолированный макет конференции с настоящими ChatPanel и AuthProvider.
 */
function Fixture() {
  const { user: activeUser, loading } = useAuth();
  if (!activeUser || loading)
    return <p role="status">Подготовка тестового чата…</p>;
  return (
    <main
      className="conference-room-page"
      data-testid="chat-fixture-room"
      style={{ position: "fixed", inset: 0, zIndex: 1000 }}
    >
      <header className="room-header">
        <div className="room-title">
          <h1>Обсуждение проекта</h1>
          <span>Встреча в эфире</span>
        </div>
        <span className="room-clock">00:18:42</span>
        <button className="room-header-action">Запись</button>
        <button className="room-header-action">Пригласить</button>
      </header>
      <div className="conference-stage">
        <div className="conference-stage-main">
          <div className="media-grid">
            <MediaTile name="Александр" local video={false} microphoneEnabled />
            <MediaTile name="Мария" video={false} microphoneEnabled />
          </div>
          <div className="conference-control-bar">
            <Button variant="outline">
              <Mic />
              Микрофон
            </Button>
            <Button variant="outline">
              <Video />
              Камера
            </Button>
            <Button variant="outline">
              <MonitorUp />
              Экран
            </Button>
            <Button variant="outline">
              <Users />
              Участники
            </Button>
            <Button variant="danger">Выйти</Button>
          </div>
        </div>
        <aside className="conference-stage-rail" aria-label="Панели встречи">
          <header className="room-panel-header">
            <strong>Общение во встрече</strong>
            <button className="icon-button" aria-label="Закрыть панель встречи">
              <X size={19} />
            </button>
          </header>
          <div
            className="conference-stage-tabs"
            role="tablist"
            aria-label="Панель встречи"
          >
            <button role="tab" aria-selected="true">
              Чат
            </button>
            <button role="tab" aria-selected="false">
              Участники (2)
            </button>
          </div>
          <div
            className="conference-stage-panel conference-stage-panel-chat"
            role="tabpanel"
          >
            <ChatPanel
              conferenceId="chat-fixture"
              membership={member}
              readOnly={false}
            />
          </div>
        </aside>
      </div>
    </main>
  );
}

/**
 * install подменяет только тестовые API-методы локальными данными и монтирует макет.
 * Подмены существуют лишь в тестовой вкладке и никогда не попадают в production-сборку.
 */
export function install() {
  document.getElementById("root")?.setAttribute("hidden", "");
  sent = [];
  attachments.clear();
  history = Array.from({ length: 32 }, (_, index) =>
    fixtureMessage(
      index + 1,
      index % 3 === 0
        ? "Обсудим план проекта и подготовим материалы к следующей встрече."
        : "Да, согласуем детали и отправим обновлённый документ.",
      index % 4 === 0,
    ),
  );
  history.push(
    {
      ...fixtureMessage(33, "Теперь можно использовать любой фон!"),
      id: "incoming-final",
    },
    {
      ...fixtureMessage(34, "Отлично, выглядит здорово! 😀", true),
      id: "own-reply",
      replyTo: "incoming-final",
      replyPreview: {
        id: "incoming-final",
        senderName: "Мария",
        text: "Теперь можно использовать любой фон!",
        deleted: false,
      },
    },
    { ...fixtureMessage(35, "Да, договорились."), id: "incoming-short" },
    { ...fixtureMessage(36, "До встречи 👍", true), id: "own-short" },
  );
  api.me = async () => ({ user });
  // Current AuthProvider establishes/renews persistent sessions even when the
  // cached JWT is valid. Keep both paths local to this isolated fixture.
  const session = async (signal?: AbortSignal): Promise<LoginResponse> => {
    signal?.throwIfAborted();
    return {
      accessToken: "isolated-ui-fixture",
      tokenType: "Bearer",
      expiresIn: 1800,
      user,
    };
  };
  api.refreshSession = session;
  api.establishSession = session;
  api.messages = async () => ({
    status: "success",
    items: [...history],
    nextCursor: null,
    unreadCount: 0,
    lastReadMessageId: null,
  });
  api.chatRead = async () => ({
    status: "success",
    item: { lastReadMessageId: null, unreadCount: 0 },
  });
  api.markChatRead = async (_conferenceId, messageId) => ({
    status: "success",
    item: { lastReadMessageId: messageId, unreadCount: 0 },
  });
  // Готовое вложение проверяет настоящую очередь UI, но не передачу байтов и не MinIO.
  api.initAttachment = async (_conferenceId, body) => {
    const item: ChatAttachment = {
      id: body.clientRequestId,
      filename: body.filename,
      mimeType: body.mimeType,
      size: body.size,
      status: "ready",
    };
    attachments.set(item.id, item);
    return { status: "success", item, uploadUrl: "/unused-fixture-upload" };
  };
  api.finalizeAttachment = async (_conferenceId, id) => ({
    status: "success",
    item: attachments.get(id)!,
  });
  api.sendMessage = async (_conferenceId, body) => {
    sent.push({ ...body });
    const item = fixtureMessage(history.length + 1, body.text, true);
    item.id = `sent-${sent.length}`;
    item.attachments = (body.attachmentIds || []).map((id) =>
      attachments.get(id)!,
    );
    if (body.replyTo) {
      const original = history.find((message) => message.id === body.replyTo)!;
      item.replyTo = original.id;
      item.replyPreview = {
        id: original.id,
        text: original.text,
        senderName: original.senderName,
        deleted: false,
      };
    }
    history.push(item);
    return { status: "success", item };
  };
  saveSession({
    token: "isolated-ui-fixture",
    expiresAt: Date.now() + 1800000,
    user,
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const container = document.createElement("div");
  document.body.append(container);
  createRoot(container).render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AuthProvider>
          <Fixture />
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** getSent возвращает копии локальных отправок для проверки тела запроса в браузере. */
export function getSent(): SendBody[] {
  return sent.map((body) => ({ ...body }));
}
