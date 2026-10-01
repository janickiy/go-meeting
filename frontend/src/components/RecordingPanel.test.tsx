import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RecordingPanel } from "./RecordingPanel";
import { api } from "../api";
import type { Conference, Participant, ConferenceRecording } from "../types";

const conference = { id: "room", status: "active" } as Conference;
const membership = {
  id: "member",
  role: "participant",
  status: "joined",
} as Participant;
const row = {
  uuid: "record",
  mode: "composite",
  conferenceId: "room",
  status: "recording",
  createdAt: "2026-10-01T10:00:00Z",
  files: [],
} as ConferenceRecording;
function show(role: Participant["role"], items: ConferenceRecording[]) {
  vi.spyOn(api, "recordings").mockResolvedValue({ status: "success", items });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RecordingPanel
        conference={conference}
        membership={{ ...membership, role }}
      />
    </QueryClientProvider>,
  );
  return client;
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
describe("conference recording controls", () => {
  it("shows active recording to participants but exposes no recording command", async () => {
    const client = show("participant", [row]);
    expect(await screen.findByTestId("recording-indicator")).toHaveTextContent(
      "Идёт запись",
    );
    expect(screen.queryByRole("button", { name: /запись/ })).toBeNull();
    client.clear();
  });
  it("allows owner stop and does not duplicate start while recording", async () => {
    const client = show("owner", [row]);
    expect(
      await screen.findByRole("button", { name: "Остановить запись" }),
    ).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Начать запись" })).toBeNull();
    client.clear();
  });
  it("keeps cohost recording permission owner-only", async () => {
    const client = show("co_host", []);
    await screen.findByText(/Записей пока нет/);
    expect(screen.queryByRole("button", { name: "Начать запись" })).toBeNull();
    client.clear();
  });
});
