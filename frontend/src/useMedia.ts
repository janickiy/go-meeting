import { useEffect, useRef, useState } from "react";
import { ConferenceMediaClient, emptyMediaView } from "./media";
import type { useRealtime } from "./realtime";

export function useMedia(live: ReturnType<typeof useRealtime>) {
  const [view, setView] = useState(emptyMediaView);
  const [running, setRunning] = useState(false);
  const controller = useRef<ConferenceMediaClient | null>(null);
  const mounted = useRef(true);
  const liveRef = useRef(live);
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
  live.onEvent.current = (event) => controller.current?.handle(event);
  const start = () => {
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
    setRunning(true);
    void next.start();
  };
  return { view, running, start, stop };
}
