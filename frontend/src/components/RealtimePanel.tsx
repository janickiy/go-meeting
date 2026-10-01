import { useEffect, useRef, useState } from "react";
import { Radio, RefreshCw, ShieldCheck } from "lucide-react";
import { api } from "../api";
import { useRealtime } from "../realtime";
import { Button, ErrorNotice } from "./ui";
import type { RealtimeEvent, Signal } from "../types";

// Temporary one-peer DataChannel proof. No getUserMedia, no media tracks,
// no automatic mesh. The production SFU belongs to Stage 3.
export function RealtimePanel({ conferenceId }: { conferenceId: string }) {
  const live = useRealtime(conferenceId, true);
  const [proof, setProof] = useState("Не проверено");
  const [target, setTarget] = useState("");
  const [proofError, setProofError] = useState<Error | null>(null);
  const peer = useRef<{
    pc: RTCPeerConnection;
    target: string;
    pending: RTCIceCandidateInit[];
    timer?: ReturnType<typeof setTimeout>;
  } | null>(null);
  const mounted = useRef(true);
  const liveRef = useRef(live);
  liveRef.current = live;
  const stop = () => {
    const old = peer.current;
    peer.current = null;
    if (old) {
      clearTimeout(old.timer);
      old.pc.close();
    }
  };
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      stop();
    };
  }, []);
  useEffect(() => {
    stop();
    setProof("Не проверено");
    setProofError(null);
  }, [live.state?.connectionId]);
  useEffect(() => {
    if (
      peer.current &&
      !live.state?.participants.some((p) =>
        p.connectionIds.includes(peer.current!.target),
      )
    ) {
      stop();
      setProof("Участник отключился");
    }
  }, [live.state]);

  const attachChannel = (channel: RTCDataChannel) => {
    channel.onopen = () => {
      if (mounted.current && channel.readyState === "open")
        channel.send("stage2-ping");
    };
    channel.onmessage = (e) => {
      if (!mounted.current) return;
      if (e.data === "stage2-ping" && channel.readyState === "open")
        channel.send("stage2-pong");
      if (e.data === "stage2-pong" || e.data === "stage2-ping") {
        setProof("P2P DataChannel работает");
        if (peer.current) clearTimeout(peer.current.timer);
      }
    };
    if (channel.readyState === "open") channel.send("stage2-ping");
  };
  const create = async (remote: string) => {
    const connectionId = liveRef.current.state?.connectionId;
    const config = await api.iceConfig();
    if (
      !mounted.current ||
      !connectionId ||
      liveRef.current.state?.connectionId !== connectionId
    )
      throw new Error("Подключение изменилось");
    stop();
    const pc = new RTCPeerConnection(config);
    const entry = {
      pc,
      target: remote,
      pending: [] as RTCIceCandidateInit[],
      timer: undefined as ReturnType<typeof setTimeout> | undefined,
    };
    peer.current = entry;
    pc.onicecandidate = (e) => {
      if (e.candidate && peer.current === entry) {
        try {
          liveRef.current.send("webrtc.ice", {
            targetConnectionId: remote,
            candidate: e.candidate.toJSON(),
          });
        } catch {
          stop();
          setProof("Связь прервана");
        }
      }
    };
    pc.ondatachannel = (e) => attachChannel(e.channel);
    pc.onconnectionstatechange = () => {
      if (
        peer.current === entry &&
        ["failed", "disconnected"].includes(pc.connectionState)
      ) {
        stop();
        setProof("P2P недоступен — проверьте ICE/TURN");
      }
    };
    entry.timer = setTimeout(() => {
      if (peer.current === entry) {
        stop();
        setProof("P2P не установлено — может требоваться TURN");
      }
    }, 20000);
    setProof("Установка P2P…");
    setProofError(null);
    return entry;
  };
  // Serialize async offer/answer/ICE handling; candidates before remote SDP are
  // buffered with a hard cap. Glare picks the lexically lower connection UUID.
  const incoming = useRef(Promise.resolve());
  live.onEvent.current = (event: RealtimeEvent) => {
    if (!event.type.startsWith("webrtc.")) return;
    const own = liveRef.current.state?.connectionId;
    incoming.current = incoming.current
      .then(async () => {
        if (!mounted.current || liveRef.current.state?.connectionId !== own)
          return;
        const signal = event.data as Signal;
        const remote = signal.senderConnectionId;
        if (
          !own ||
          !remote ||
          signal.targetConnectionId !== own ||
          !liveRef.current.state?.participants.some((p) =>
            p.connectionIds.includes(remote),
          )
        )
          return;
        if (event.type === "webrtc.offer") {
          if (peer.current && peer.current.target !== remote) return; // one-peer proof, never mesh
          if (
            peer.current?.pc.signalingState === "have-local-offer" &&
            own < remote
          )
            return;
          const entry = await create(remote);
          await entry.pc.setRemoteDescription({
            type: "offer",
            sdp: signal.sdp,
          });
          await entry.pc.setLocalDescription(await entry.pc.createAnswer());
          if (peer.current === entry)
            liveRef.current.send("webrtc.answer", {
              targetConnectionId: remote,
              sdp: entry.pc.localDescription!.sdp,
            });
        } else {
          const entry = peer.current;
          if (!entry || entry.target !== remote) return;
          if (event.type === "webrtc.answer") {
            await entry.pc.setRemoteDescription({
              type: "answer",
              sdp: signal.sdp,
            });
            for (const candidate of entry.pending.splice(0))
              await entry.pc.addIceCandidate(candidate);
          } else if (event.type === "webrtc.ice" && signal.candidate) {
            if (entry.pc.remoteDescription)
              await entry.pc.addIceCandidate(signal.candidate);
            else if (entry.pending.length < 100)
              entry.pending.push(signal.candidate);
            else throw new Error("Слишком много ICE-кандидатов");
          }
        }
      })
      .catch(() => {
        if (mounted.current) {
          stop();
          setProofError(
            new Error("Проверка signaling не удалась. Повторите подключение."),
          );
        }
      });
  };
  const test = async () => {
    if (!target) return;
    try {
      const entry = await create(target);
      attachChannel(entry.pc.createDataChannel("stage2-proof"));
      await entry.pc.setLocalDescription(await entry.pc.createOffer());
      if (peer.current === entry)
        liveRef.current.send("webrtc.offer", {
          targetConnectionId: target,
          sdp: entry.pc.localDescription!.sdp,
        });
    } catch {
      if (mounted.current) {
        stop();
        setProofError(new Error("Не удалось начать P2P-проверку."));
      }
    }
  };
  const peers =
    live.state?.participants.flatMap((p) =>
      p.connectionIds
        .filter((id) => id !== live.state?.connectionId)
        .map((id) => ({ id, name: p.displayName })),
    ) || [];
  return (
    <section
      className="content-card realtime-panel"
      aria-label="Realtime-подключение"
    >
      <div className="section-heading">
        <h2>
          <Radio size={20} />
          Связь с участниками
        </h2>
        <span className="participant-status">{live.status}</span>
      </div>
      <ErrorNotice error={live.error} />
      {live.state && (
        <>
          <p className="field-hint">
            Подключение этой вкладки:{" "}
            <code data-testid="connection-id">{live.state.connectionId}</code>
          </p>
          <div className="realtime-people">
            {live.state.participants.map((p) => (
              <div
                key={p.id}
                className="realtime-person"
                data-testid={`presence-${p.userId}`}
              >
                <span
                  className={`presence-dot ${p.online ? "online-dot" : ""}`}
                />
                <strong>{p.displayName}</strong>
                <span>
                  {p.online
                    ? `Онлайн · подключений: ${p.connections}`
                    : "Не в сети"}
                </span>
              </div>
            ))}
          </div>
          <details className="signaling-proof">
            <summary>
              <ShieldCheck size={16} />
              Проверка signaling и P2P
            </summary>
            <p className="field-hint">
              Тест между двумя вкладками без камеры и микрофона. Это не
              видеоконференция и не SFU.
            </p>
            <label className="field-label" htmlFor="proof-target">
              Подключение участника
            </label>
            <select
              id="proof-target"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
            >
              <option value="">Выберите подключение</option>
              {peers.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} · {p.id.slice(0, 8)}
                </option>
              ))}
            </select>
            <div className="meeting-actions">
              <Button
                variant="outline"
                disabled={!peers.some((p) => p.id === target)}
                onClick={() => void test()}
              >
                Проверить P2P-соединение
              </Button>
            </div>
            <p role="status">{proof}</p>
            <ErrorNotice error={proofError} />
            <p className="field-hint">
              Последние события: {live.events.join(" · ") || "—"}
            </p>
          </details>
        </>
      )}
      <Button
        variant="secondary"
        onClick={() => {
          stop();
          live.reconnect();
        }}
      >
        <RefreshCw size={16} />
        Переподключиться
      </Button>
    </section>
  );
}
