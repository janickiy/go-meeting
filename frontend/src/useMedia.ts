import { useEffect, useRef, useState } from "react";
import { ConferenceMediaClient, emptyMediaView } from "./media";
import type { MediaPolicy } from "./media";
import { api } from "./api";
import type { useRealtime } from "./realtime";

export function useMedia(
  live: ReturnType<typeof useRealtime>,
  conferenceId: string,
  policy: MediaPolicy,
) {
  const [view, setView] = useState(emptyMediaView);
  const [running, setRunning] = useState(false);
  const controller = useRef<ConferenceMediaClient | null>(null);
  const mounted = useRef(true);
  const liveRef = useRef(live);
  const policyRef = useRef(policy);
  // Never reset on media.stop/start: delayed HTTP requests from an earlier
  // capture must not overwrite a newer snapshot on the same WS connection.
  const mediaSequence = useRef(0);
  policyRef.current = policy;
  liveRef.current = live;
  const stop = () => {
    controller.current?.stop();
    controller.current = null;
    setRunning(false);
  };
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.stop();
      controller.current = null;
    };
  }, []);
  useEffect(() => {
    stop();
    setView(emptyMediaView());
  }, [live.state?.connectionId]);
  useEffect(() => {
    controller.current?.setPolicy(policy);
  }, [
    policy.version,
    policy.microphoneBlocked,
    policy.cameraBlocked,
    policy.screenBlocked,
  ]);
  useEffect(() => {
    const connectionId = live.state?.connectionId;
    if (!connectionId) return;
    const snapshot = {
      connectionId,
      sequence: ++mediaSequence.current,
      microphoneEnabled: view.microphoneEnabled,
      cameraEnabled: view.cameraEnabled,
      screenSharing: view.screenSharing,
    };
    const timer = setTimeout(() => {
      void api.setMediaState(conferenceId, snapshot).catch(() => {});
    }, 100);
    return () => clearTimeout(timer);
  }, [
    conferenceId,
    live.state?.connectionId,
    view.microphoneEnabled,
    view.cameraEnabled,
    view.screenSharing,
  ]);
  live.onEvent.current = (event) => controller.current?.handle(event);
  const start = (captureDevices = true) => {
    const connectionId = liveRef.current.state?.connectionId;
    if (!connectionId) return;
    stop();
    const next = new ConferenceMediaClient(
      (type, data) => {
        if (liveRef.current.state?.connectionId !== connectionId)
          throw new Error("connection_changed");
        return liveRef.current.send(type, data);
      },
      (state) => {
        if (mounted.current && controller.current === next) {
          setView(state);
          setRunning(state.active);
        }
      },
    );
    controller.current = next;
    next.setPolicy(policyRef.current);
    setRunning(true);
    void next.start(captureDevices);
  };
  return {
    view,
    running,
    start,
    stop,
    microphone: (enabled: boolean, deviceId?: string) =>
      controller.current?.changeSource("microphone", enabled, deviceId),
    camera: (enabled: boolean, deviceId?: string) =>
      controller.current?.changeSource("camera", enabled, deviceId),
    startScreen: () => controller.current?.startScreen(),
    stopScreen: () => controller.current?.stopScreen(),
  };
}
