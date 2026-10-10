import toneURL from "./assets/device-tone.wav";

/** Local playback only. No recording or device data is sent to a server. */
export function playDeviceTone(
  sinkId: string,
  done: (error?: unknown) => void,
  volume = 1,
) {
  const audio = new Audio(toneURL);
  audio.volume = Math.min(1, Math.max(0, volume));
  let stopped = false;
  const finish = (error?: unknown) => {
    if (stopped) return;
    stopped = true;
    audio.pause();
    audio.removeAttribute("src");
    audio.load();
    done(error);
  };
  audio.onended = () => finish();
  audio.onerror = () => finish(new Error("audio_playback_failed"));
  void (async () => {
    try {
      if (sinkId && "setSinkId" in audio) await audio.setSinkId(sinkId);
      if (!stopped) await audio.play();
    } catch (error) {
      finish(error);
    }
  })();
  return () => finish();
}
