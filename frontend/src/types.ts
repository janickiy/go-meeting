export interface User {
  id: string;
  email: string;
  displayName: string | null;
  createdAt: string;
  updatedAt: string;
}
export type ConferenceStatus =
  "created" | "scheduled" | "active" | "finished" | "cancelled";
export interface Conference {
  id: string;
  ownerId: string;
  title: string;
  inviteCode: string;
  inviteUrl: string;
  status: ConferenceStatus;
  createdAt: string;
  updatedAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  waitingRoomEnabled?: boolean;
  scheduledAt?: string | null;
  plannedDurationMin?: number | null;
}
export interface Participant {
  id: string;
  conferenceId: string;
  userId: string | null;
  displayName: string;
  role: "owner" | "co_host" | "participant" | "guest";
  status: "joined" | "left" | "waiting" | "rejected" | "kicked";
  joinedAt: string | null;
  leftAt: string | null;
  createdAt: string;
  updatedAt: string;
  microphoneEnabled?: boolean;
  cameraEnabled?: boolean;
  screenSharing?: boolean;
  microphoneBlocked?: boolean;
  cameraBlocked?: boolean;
  screenBlocked?: boolean;
  mediaPolicyVersion?: number;
  admissionState?: "waiting" | "admitted" | "rejected" | "kicked";
  admissionDecidedAt?: string | null;
  admissionVersion?: number;
}
export interface ParticipantMediaState {
  connectionId: string;
  sequence: number;
  microphoneEnabled: boolean;
  cameraEnabled: boolean;
  screenSharing: boolean;
}
export interface ModerationAction {
  action: "mute" | "camera" | "screen" | "kick" | "role";
  blocked?: boolean;
  role?: "co_host" | "participant";
}
export interface ConferenceRecording {
  uuid: string;
  conferenceId: string;
  mode: "composite";
  status:
    | "starting"
    | "recording"
    | "stopping"
    | "processing"
    | "ready"
    | "failed"
    | "cancelled";
  createdAt: string;
  startedAt?: string;
  endedAt?: string;
  durationSec?: number;
  errorMessage?: string;
  files: { fileType: string; url?: string; sizeBytes?: number }[];
}
export interface Invite {
  id: string;
  title: string;
  status: ConferenceStatus;
  scheduledAt?: string | null;
  waitingRoomEnabled?: boolean;
}
export interface LoginResponse {
  accessToken: string;
  tokenType: "Bearer";
  expiresIn: number;
  user: User;
}
export type Item<T> = { status: string; item: T };
export type Items<T> = { status: string; items: T[] };

export interface PresenceParticipant extends Participant {
  online: boolean;
  connections: number;
  connectionIds: string[];
}
export interface RealtimeState {
  connectionId: string;
  participantId: string;
  status: ConferenceStatus;
  participants: PresenceParticipant[];
  hands?: RaisedHand[];
}
export interface RealtimeEvent {
  version: 1;
  id: string;
  type: string;
  conferenceId: string;
  timestamp: string;
  data: unknown;
  replyTo?: string;
}
export interface Signal {
  targetConnectionId: string;
  senderConnectionId?: string;
  senderParticipantId?: string;
  sdp?: string;
  candidate?: RTCIceCandidateInit;
}

export interface CursorItems<T> extends Items<T> {
  nextCursor: string | null;
}
export interface ConferenceFilters {
  view?: "upcoming" | "active" | "past";
  scope?: "all" | "owned" | "participating";
  from?: string;
  to?: string;
  status?: ConferenceStatus;
}
export interface ConferenceInput {
  title: string;
  waitingRoomEnabled?: boolean;
  scheduledAt?: string | null;
  plannedDurationMin?: number | null;
}
export interface ConferenceHistory {
  conference: Conference;
  owner: { id: string; displayName: string | null };
  durationSec: number | null;
  participantCount: number;
  participants: Participant[];
  participantsTruncated: boolean;
  recordings: {
    total: number;
    ready: number;
    processing: number;
    failed: number;
  };
  chatAvailable: boolean;
  chatReadOnly: boolean;
}
export interface ChatAttachment {
  id: string;
  filename: string;
  mimeType: string;
  size: number;
  status: string;
}
export interface ChatMessage {
  id: string;
  sequence: string;
  conferenceId: string;
  senderId: string;
  senderName: string;
  text: string;
  replyTo?: string | null;
  replyPreview?: {
    id: string;
    text: string;
    senderName: string;
    deleted: boolean;
  } | null;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
  version: number;
  attachments: ChatAttachment[];
}
export interface ChatPage extends CursorItems<ChatMessage> {
  unreadCount: number;
  lastReadMessageId: string | null;
}
export interface ChatReadState {
  lastReadMessageId: string | null;
  unreadCount: number;
}
export interface RaisedHand {
  participantId: string;
  raisedAt: string;
}
export type ReactionEmoji = "👍" | "👏" | "❤️" | "😂";
export interface Notification {
  id: string;
  userId: string;
  type: string;
  version: 1;
  payload: {
    conferenceId: string;
    recordingId?: string;
    scheduledAt?: string;
    admissionState?: string;
  };
  createdAt: string;
  readAt: string | null;
}
export interface NotificationsPage extends CursorItems<Notification> {
  unreadCount: number;
}
