export interface User {
  id: string;
  email: string;
  displayName: string | null;
  createdAt: string;
  updatedAt: string;
}
export type ConferenceStatus = "created" | "active" | "finished" | "cancelled";
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
}
export interface Invite {
  id: string;
  title: string;
  status: ConferenceStatus;
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
