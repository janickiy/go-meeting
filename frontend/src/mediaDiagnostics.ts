/** Only these aggregate values may leave the WebRTC stats parser.
 * Loss is cumulative inbound RTP; bitrate needs two samples. No quality score is inferred.
 */
export interface RtcDiagnostics {
  roundTripTimeMs?: number;
  packetLossPercent?: number;
  outboundKbps?: number;
  inboundKbps?: number;
  route?: "relay" | "direct";
}

export interface RtcCounters {
  timestampMs: number;
  bytesSent?: number;
  bytesReceived?: number;
}

type StatsEntry = {
  id?: string;
  type?: string;
  timestamp?: number;
  selectedCandidatePairId?: string;
  selected?: boolean;
  nominated?: boolean;
  state?: string;
  localCandidateId?: string;
  remoteCandidateId?: string;
  candidateType?: string;
  currentRoundTripTime?: number;
  roundTripTime?: number;
  packetsLost?: number;
  packetsReceived?: number;
  bytesSent?: number;
  bytesReceived?: number;
};

function measured(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function bitrate(current?: number, previous?: number, seconds?: number) {
  if (
    !measured(current) ||
    !measured(previous) ||
    !measured(seconds) ||
    seconds === 0 ||
    current < previous
  )
    return undefined;
  return Math.round(((current - previous) * 8) / seconds / 1000);
}

/** Parse browser stats into an allowlisted snapshot; candidate addresses and raw IDs stay here. */
export function summarizeRtcStats(
  report: RTCStatsReport,
  previous: RtcCounters | null = null,
): { summary: RtcDiagnostics; counters: RtcCounters | null } {
  const entries = new Map<string, StatsEntry>();
  report.forEach((value, key) => entries.set(key, value as StatsEntry));
  const values = [...entries.values()];
  const summary: RtcDiagnostics = {};
  const transport = values.find(
    (entry) =>
      entry.type === "transport" &&
      typeof entry.selectedCandidatePairId === "string",
  );
  const pair = transport?.selectedCandidatePairId
    ? entries.get(transport.selectedCandidatePairId)
    : values.find(
        (entry) =>
          entry.type === "candidate-pair" &&
          entry.state === "succeeded" &&
          (entry.selected || entry.nominated),
      );
  if (pair?.type === "candidate-pair") {
    if (measured(pair.currentRoundTripTime))
      summary.roundTripTimeMs = Math.round(pair.currentRoundTripTime * 1000);
    const local = pair.localCandidateId
      ? entries.get(pair.localCandidateId)
      : undefined;
    const remote = pair.remoteCandidateId
      ? entries.get(pair.remoteCandidateId)
      : undefined;
    const localType =
      local?.type === "local-candidate" ? local.candidateType : undefined;
    const remoteType =
      remote?.type === "remote-candidate" ? remote.candidateType : undefined;
    const direct = ["host", "srflx", "prflx"];
    if (localType === "relay" || remoteType === "relay")
      summary.route = "relay";
    else if (
      direct.includes(localType || "") &&
      direct.includes(remoteType || "")
    )
      summary.route = "direct";
  }

  let lost = 0;
  let received = 0;
  let hasLoss = false;
  let sentBytes: number | undefined;
  let receivedBytes: number | undefined;
  let timestampMs: number | undefined;
  for (const entry of values) {
    if (
      entry.type === "remote-inbound-rtp" &&
      summary.roundTripTimeMs === undefined &&
      measured(entry.roundTripTime)
    )
      summary.roundTripTimeMs = Math.round(entry.roundTripTime * 1000);
    if (entry.type === "inbound-rtp") {
      if (measured(entry.packetsLost) && measured(entry.packetsReceived)) {
        lost += entry.packetsLost;
        received += entry.packetsReceived;
        hasLoss = true;
      }
      if (measured(entry.bytesReceived))
        receivedBytes = (receivedBytes || 0) + entry.bytesReceived;
    }
    if (entry.type === "outbound-rtp" && measured(entry.bytesSent))
      sentBytes = (sentBytes || 0) + entry.bytesSent;
    if (
      ["inbound-rtp", "outbound-rtp"].includes(entry.type || "") &&
      measured(entry.timestamp)
    )
      timestampMs = Math.max(timestampMs ?? 0, entry.timestamp);
  }
  if (hasLoss && lost + received > 0)
    summary.packetLossPercent =
      Math.round((lost / (lost + received)) * 1000) / 10;
  const counters =
    timestampMs === undefined
      ? null
      : { timestampMs, bytesSent: sentBytes, bytesReceived: receivedBytes };
  if (counters && previous) {
    const seconds = (counters.timestampMs - previous.timestampMs) / 1000;
    summary.outboundKbps = bitrate(
      counters.bytesSent,
      previous.bytesSent,
      seconds,
    );
    summary.inboundKbps = bitrate(
      counters.bytesReceived,
      previous.bytesReceived,
      seconds,
    );
  }
  return { summary, counters };
}

const peerStates = new Set([
  "new",
  "connecting",
  "connected",
  "disconnected",
  "failed",
  "closed",
]);
const iceStates = new Set([
  "new",
  "checking",
  "connected",
  "completed",
  "disconnected",
  "failed",
  "closed",
]);

/** Build a redacted report from explicitly listed fields, never by spreading browser data. */
export function safeDiagnosticsReport(input: {
  buildVersion?: string;
  realtimeStatus: string;
  mediaWorkerAvailable: boolean;
  connectionState: string;
  iceState: string;
  diagnostics: RtcDiagnostics | null;
}): string {
  const summary = input.diagnostics;
  const rtc: RtcDiagnostics = {};
  if (summary) {
    if (measured(summary.roundTripTimeMs))
      rtc.roundTripTimeMs = summary.roundTripTimeMs;
    if (measured(summary.packetLossPercent) && summary.packetLossPercent <= 100)
      rtc.packetLossPercent = summary.packetLossPercent;
    if (measured(summary.outboundKbps)) rtc.outboundKbps = summary.outboundKbps;
    if (measured(summary.inboundKbps)) rtc.inboundKbps = summary.inboundKbps;
    if (summary.route === "relay" || summary.route === "direct")
      rtc.route = summary.route;
  }
  return JSON.stringify(
    {
      report: "meet-media-diagnostics",
      buildVersion:
        input.buildVersion && /^[A-Za-z0-9._+-]{1,80}$/.test(input.buildVersion)
          ? input.buildVersion
          : "unknown",
      websocketStatus:
        input.realtimeStatus === "Подключено"
          ? "connected"
          : input.realtimeStatus === "Подключение…"
            ? "connecting"
            : input.realtimeStatus === "Переподключение…"
              ? "reconnecting"
              : "disconnected",
      mediaWorkerAvailable: input.mediaWorkerAvailable === true,
      connectionState: peerStates.has(input.connectionState)
        ? input.connectionState
        : "unknown",
      iceState: iceStates.has(input.iceState) ? input.iceState : "unknown",
      rtc,
    },
    null,
    2,
  );
}
