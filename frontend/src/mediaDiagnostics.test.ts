import { expect, it } from "vitest";
import { safeDiagnosticsReport, summarizeRtcStats } from "./mediaDiagnostics";

function report(rows: [string, Record<string, unknown>][]) {
  return new Map(rows) as unknown as RTCStatsReport;
}

it("reports only measured RTT, loss, bitrate and relay route", () => {
  const first = report([
    ["transport", { type: "transport", selectedCandidatePairId: "pair" }],
    [
      "pair",
      {
        type: "candidate-pair",
        localCandidateId: "candidate",
        currentRoundTripTime: 0.042,
        ip: "192.0.2.10",
      },
    ],
    [
      "candidate",
      {
        type: "local-candidate",
        candidateType: "relay",
        address: "192.0.2.10",
      },
    ],
    [
      "in",
      {
        type: "inbound-rtp",
        timestamp: 1000,
        packetsLost: 2,
        packetsReceived: 98,
        bytesReceived: 100_000,
      },
    ],
    [
      "out",
      {
        type: "outbound-rtp",
        timestamp: 1000,
        bytesSent: 50_000,
        sdp: "secret",
      },
    ],
  ]);
  const one = summarizeRtcStats(first);
  expect(one.summary).toEqual({
    roundTripTimeMs: 42,
    packetLossPercent: 2,
    route: "relay",
  });
  const second = report([
    [
      "in",
      {
        type: "inbound-rtp",
        timestamp: 2000,
        packetsLost: 2,
        packetsReceived: 198,
        bytesReceived: 200_000,
      },
    ],
    ["out", { type: "outbound-rtp", timestamp: 2000, bytesSent: 100_000 }],
  ]);
  const two = summarizeRtcStats(second, one.counters);
  expect(two.summary.inboundKbps).toBe(800);
  expect(two.summary.outboundKbps).toBe(400);
  expect(JSON.stringify(two)).not.toMatch(/192\.0\.2\.10|secret|candidate/);
});

it("omits unknown and invalid measurements instead of inventing metrics", () => {
  const sample = summarizeRtcStats(
    report([
      [
        "in",
        {
          type: "inbound-rtp",
          timestamp: 1000,
          packetsLost: -1,
          packetsReceived: 0,
          bytesReceived: 100,
        },
      ],
      [
        "candidate",
        { type: "local-candidate", candidateType: "host", address: "10.0.0.2" },
      ],
    ]),
  );
  expect(sample.summary).toEqual({});
  const reset = summarizeRtcStats(
    report([
      ["in", { type: "inbound-rtp", timestamp: 2000, bytesReceived: 10 }],
    ]),
    sample.counters,
  );
  expect(reset.summary.inboundKbps).toBeUndefined();
});

it("calls a route direct only after both selected ICE candidates are measured", () => {
  const rows: [string, Record<string, unknown>][] = [
    [
      "pair",
      {
        type: "candidate-pair",
        selected: true,
        state: "succeeded",
        localCandidateId: "local",
        remoteCandidateId: "remote",
      },
    ],
    [
      "local",
      { type: "local-candidate", candidateType: "host", address: "10.0.0.2" },
    ],
    [
      "remote",
      {
        type: "remote-candidate",
        candidateType: "srflx",
        address: "198.51.100.3",
      },
    ],
  ];
  expect(summarizeRtcStats(report(rows)).summary.route).toBe("direct");
  rows.pop();
  expect(summarizeRtcStats(report(rows)).summary.route).toBeUndefined();
  rows.push(["remote", { type: "remote-candidate", candidateType: "relay" }]);
  expect(summarizeRtcStats(report(rows)).summary.route).toBe("relay");
});

it("copies an allowlisted report without IDs, addresses, SDP or tokens", () => {
  const text = safeDiagnosticsReport({
    buildVersion: "1.9.0+stage9",
    realtimeStatus: "Подключено",
    mediaWorkerAvailable: true,
    connectionState: "connected",
    iceState: "completed",
    diagnostics: {
      roundTripTimeMs: 42,
      packetLossPercent: 2,
      route: "direct",
      sdp: "secret-sdp",
      ip: "192.0.2.10",
      token: "secret-token",
    } as never,
    peerId: "private-peer",
  } as never);
  expect(JSON.parse(text)).toEqual({
    report: "meet-media-diagnostics",
    buildVersion: "1.9.0+stage9",
    websocketStatus: "connected",
    mediaWorkerAvailable: true,
    connectionState: "connected",
    iceState: "completed",
    rtc: { roundTripTimeMs: 42, packetLossPercent: 2, route: "direct" },
  });
  expect(text).not.toMatch(/192\.0\.2\.10|secret|private-peer|candidate/);
  expect(
    JSON.parse(
      safeDiagnosticsReport({
        buildVersion: "token=private",
        realtimeStatus: "private-status",
        mediaWorkerAvailable: false,
        connectionState: "a private state",
        iceState: "192.0.2.10",
        diagnostics: null,
      }),
    ),
  ).toMatchObject({
    buildVersion: "unknown",
    websocketStatus: "disconnected",
    mediaWorkerAvailable: false,
    connectionState: "unknown",
    iceState: "unknown",
  });
});
