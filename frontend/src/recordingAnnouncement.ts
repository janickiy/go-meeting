import recordingStarted from "./assets/recording-started-en.wav";

let audio: HTMLAudioElement | undefined;
let generation = 0;
let playing = false;
let primed = false;
let priming = false;

function player() {
  if (!audio) {
    audio = new Audio(recordingStarted);
    audio.preload = "auto";
  }
  return audio;
}

/** Разрешает звук на пользовательском жесте, в том числе на кнопке входа.
 * Используется тот же элемент, что и для объявления; проба беззвучна. */
export function installRecordingAudioUnlock() {
  const unlock = () => {
    if (primed || priming || playing) return;
    const element = player();
    const token = generation;
    priming = true;
    element.muted = true;
    element.volume = 0;
    void element
      .play()
      .then(
        () => {
          primed = true;
        },
        () => {},
      )
      .finally(() => {
        priming = false;
        if (token !== generation) return;
        element.pause();
        element.currentTime = 0;
        element.muted = false;
        element.volume = 1;
      });
  };
  document.addEventListener("pointerdown", unlock);
  document.addEventListener("keydown", unlock);
  return () => {
    document.removeEventListener("pointerdown", unlock);
    document.removeEventListener("keydown", unlock);
  };
}

/** Произносит фиксированную английскую фразу локальным аудиофайлом.
 * При запрете autoplay повторяет на следующем жесте. Возвращает отмену:
 * выход из комнаты или остановка записи не должны оставлять отложенный звук. */
export function playRecordingAnnouncement(): () => void {
  const element = player();
  const token = ++generation;
  playing = true;
  let pending = false;
  let cancelled = false;
  const clearRetry = () => {
    document.removeEventListener("pointerdown", retry);
    document.removeEventListener("keydown", retry);
  };
  const play = () => {
    if (cancelled || token !== generation) return;
    pending = false;
    element.muted = false;
    element.volume = 1;
    element.currentTime = 0;
    void element.play().then(
      () => {
        if (cancelled || token !== generation) return;
        primed = true;
        clearRetry();
      },
      (error: unknown) => {
        if (cancelled || token !== generation) return;
        if (error instanceof DOMException && error.name === "NotAllowedError") {
          pending = true;
          document.addEventListener("pointerdown", retry);
          document.addEventListener("keydown", retry);
        }
      },
    );
  };
  function retry() {
    if (pending) play();
  }
  play();
  return () => {
    cancelled = true;
    clearRetry();
    if (token !== generation) return;
    generation++;
    playing = false;
    element.pause();
    element.currentTime = 0;
  };
}
