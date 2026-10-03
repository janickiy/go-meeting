/** AudioActivitySource связывает существующий микрофонный поток с плиткой участника.
 * @params id — стабильный ключ подключения; participantId — участник встречи;
 * stream — уже полученный поток, без дополнительного захвата; enabled — разрешён ли его анализ.
 */
export interface AudioActivitySource {
  id: string;
  participantId: string;
  stream: MediaStream | null;
  enabled: boolean;
}

/** normalizeAudioLevel преобразует RMS в воспринимаемую громкость для индикатора.
 * Логарифмическая шкала от 0,01 до 0,25 RMS сохраняет различие тихого голоса,
 * обычной речи и громкого звука; шум ниже нижнего порога не подсвечивается.
 * Это измерение звуковой активности, а не распознавание речи.
 * @args rms — среднеквадратичная амплитуда существующей аудиодорожки.
 * @return ограниченный диапазоном 0–1 уровень; неверный замер даёт ноль.
 */
export function normalizeAudioLevel(rms: number): number {
  if (!Number.isFinite(rms) || rms <= 0.01) return 0;
  return Math.min(1, Math.log(rms / 0.01) / Math.log(25));
}

/** AudioLevelEnvelope отбрасывает одиночные щелчки и сглаживает реальную громкость.
 * Короткое затухание убирает дрожание, но не удерживает полностью жёлтый индикатор
 * между словами: интенсивность непрерывно меняется вместе со звуком.
 * @params level — текущая громкость; startedAt — начало подтверждаемого звука;
 * sampledAt — время последнего замера; quietAt/releaseFrom — начало и уровень затухания.
 */
export class AudioLevelEnvelope {
  level = 0;
  private startedAt: number | null = null;
  private sampledAt: number | null = null;
  private quietAt: number | null = null;
  private releaseFrom = 0;

  /** update подтверждает звук за 50 мс, быстро реагирует на рост громкости
   * и за 100 мс плавно гасит сигнал после первого тихого замера.
   * Отключённая дорожка и некорректные данные гасят индикацию сразу.
   * @args rms — измеренная амплитуда; now — монотонное время в миллисекундах;
   * available — доступна ли включённая и не заглушённая дорожка.
   * @return нормированная громкость, округлённая до 1% для ограниченной частоты обновлений.
   */
  update(rms: number, now: number, available = true): number {
    if (
      !available ||
      !Number.isFinite(rms) ||
      rms < 0 ||
      !Number.isFinite(now) ||
      (this.sampledAt !== null && now < this.sampledAt)
    ) {
      this.reset();
      return 0;
    }
    const elapsed = this.sampledAt === null ? 50 : now - this.sampledAt;
    this.sampledAt = now;
    const target = normalizeAudioLevel(rms);
    // Более высокий порог начала не даёт пограничному фоновому шуму включать индикатор.
    if (target > 0 && (this.level > 0 || rms >= 0.015)) {
      this.quietAt = null;
      if (this.level === 0) {
        this.startedAt ??= now;
        if (now - this.startedAt < 50) return 0;
        this.level = target;
      } else {
        const smoothing =
          1 - Math.exp(-elapsed / (target > this.level ? 30 : 70));
        this.level += (target - this.level) * smoothing;
      }
    } else {
      this.startedAt = null;
      if (this.quietAt === null) {
        this.quietAt = now;
        this.releaseFrom = this.level;
      }
      this.level =
        this.releaseFrom * Math.max(0, 1 - (now - this.quietAt) / 100);
    }
    this.level = Math.round(this.level * 100) / 100;
    return this.level;
  }

  /** reset сбрасывает громкость и накопленное состояние при недоступности источника. */
  reset() {
    this.level = 0;
    this.startedAt = null;
    this.sampledAt = null;
    this.quietAt = null;
    this.releaseFrom = 0;
  }
}

/** Observation хранит граф анализа одной микрофонной дорожки.
 * @params participantId — автор; track — заимствованная дорожка WebRTC;
 * source/analyser — узлы без слышимого вывода; samples — повторно используемый буфер;
 * envelope — сглаженная громкость; unavailable — обработчик потери аудиодорожки.
 */
type Observation = {
  participantId: string;
  track: MediaStreamTrack;
  source: MediaStreamAudioSourceNode;
  analyser: AnalyserNode;
  samples: Float32Array<ArrayBuffer>;
  envelope: AudioLevelEnvelope;
  unavailable: () => void;
};

/** AudioActivityMonitor анализирует микрофоны в одном AudioContext на всю комнату.
 * Аудио не сохраняется и не отправляется на сервер; микрофон заново не запрашивается.
 * Нулевое усиление исключает повторное воспроизведение и эхо.
 * @params onChange — получатель изменившейся громкости участников;
 * observations — анализируемые подключения; context/output — общий неслышимый граф;
 * timer — периодический замер; levels — последнее опубликованное состояние.
 */
export class AudioActivityMonitor {
  private context: AudioContext | null = null;
  private output: GainNode | null = null;
  private observations = new Map<string, Observation>();
  private timer: ReturnType<typeof setInterval> | null = null;
  private levels = new Map<string, number>();

  /** constructor сохраняет получателя изменений; браузерные ресурсы создаются лениво.
   * @args onChange — вызывается только при изменении положительных уровней громкости.
   */
  constructor(
    private readonly onChange: (levels: ReadonlyMap<string, number>) => void,
  ) {}

  /** resume возобновляет анализ после разрешённого пользовательского действия.
   * Ошибка autoplay не влияет на само WebRTC-соединение.
   */
  private resume = () => {
    if (this.context?.state === "suspended")
      void this.context.resume().catch(() => {});
  };

  /** contextStateChanged сразу гасит все индикаторы при остановке общего аудиоконтекста. */
  private contextStateChanged = () => {
    if (this.context?.state === "running") return;
    for (const item of this.observations.values()) item.envelope.reset();
    this.publish();
  };

  /** prepare создаёт один общий контекст и неслышимый выход.
   * @return true, если Web Audio доступен; false оставляет обычные плитки без подсветки.
   */
  private prepare(): boolean {
    if (this.context) return true;
    if (typeof AudioContext === "undefined") return false;
    try {
      this.context = new AudioContext();
      this.output = this.context.createGain();
      this.output.gain.value = 0;
      this.output.connect(this.context.destination);
      this.context.addEventListener?.("statechange", this.contextStateChanged);
      document.addEventListener("pointerdown", this.resume);
      document.addEventListener("keydown", this.resume);
      this.resume();
      return true;
    } catch {
      if (this.context) void this.context.close().catch(() => {});
      this.context = null;
      this.output = null;
      return false;
    }
  }

  /** setSources согласует дорожки без пересоздания графа при обновлении панелей.
   * @args sources — только микрофонные источники; экранные потоки отфильтрованы вызывающим кодом.
   */
  setSources(sources: readonly AudioActivitySource[]) {
    const requested = new Map<
      string,
      { source: AudioActivitySource; track: MediaStreamTrack }
    >();
    for (const source of sources) {
      if (!source.enabled || !source.stream) continue;
      const track = source.stream
        .getAudioTracks?.()
        .find((track) => track.readyState === "live");
      if (track) requested.set(source.id, { source, track });
    }
    for (const [id, item] of this.observations) {
      const wanted = requested.get(id);
      if (
        !wanted ||
        wanted.track !== item.track ||
        wanted.source.participantId !== item.participantId
      ) {
        this.disconnect(item);
        this.observations.delete(id);
      }
    }
    for (const [id, { source: input, track }] of requested) {
      if (this.observations.has(id) || !this.prepare()) continue;
      let source: MediaStreamAudioSourceNode | undefined;
      let analyser: AnalyserNode | undefined;
      try {
        source = this.context!.createMediaStreamSource(
          new MediaStream([track]),
        );
        analyser = this.context!.createAnalyser();
        analyser.fftSize = 512;
        source.connect(analyser);
        analyser.connect(this.output!);
        const item: Observation = {
          participantId: input.participantId,
          track,
          source,
          analyser,
          samples: new Float32Array(analyser.fftSize),
          envelope: new AudioLevelEnvelope(),
          unavailable: () => {
            item.envelope.reset();
            if (item.track.readyState === "ended") {
              this.disconnect(item);
              this.observations.delete(id);
              this.updateTimer();
            }
            this.publish();
          },
        };
        track.addEventListener?.("mute", item.unavailable);
        track.addEventListener?.("ended", item.unavailable);
        this.observations.set(id, item);
        // Только новая дорожка получает первый замер; перерисовка не продвигает сглаживание.
        this.measure(item, performance.now());
      } catch {
        source?.disconnect();
        analyser?.disconnect();
      }
    }
    for (const item of this.observations.values())
      if (!this.available(item)) item.envelope.reset();
    this.updateTimer();
    this.publish();
  }

  /** available проверяет возможность анализа без повторного чтения аудиосэмплов.
   * @args item — наблюдаемая микрофонная дорожка.
   * @return доступен ли реальный сигнал включённого микрофона.
   */
  private available(item: Observation): boolean {
    return (
      this.context?.state === "running" &&
      item.track.enabled &&
      !item.track.muted &&
      item.track.readyState === "live"
    );
  }

  /** measure измеряет RMS одной дорожки и обновляет её огибающую.
   * @args item — узлы наблюдаемой дорожки; now — единое время текущего замера.
   */
  private measure(item: Observation, now: number) {
    if (!this.available(item)) {
      item.envelope.reset();
      return;
    }
    try {
      item.analyser.getFloatTimeDomainData(item.samples);
      let square = 0;
      for (const value of item.samples) square += value * value;
      item.envelope.update(Math.sqrt(square / item.samples.length), now);
    } catch {
      item.envelope.reset();
    }
  }

  /** sample измеряет громкость не чаще 20 раз в секунду.
   * Выключение, mute и конец дорожки гасят индикатор без обычного затухания.
   */
  private sample() {
    const now = performance.now();
    for (const [id, item] of this.observations) {
      if (item.track.readyState === "ended") {
        this.disconnect(item);
        this.observations.delete(id);
        continue;
      }
      this.measure(item, now);
    }
    this.updateTimer();
    this.publish();
  }

  /** updateTimer включает замеры только при наличии наблюдаемых микрофонов. */
  private updateTimer() {
    if (this.observations.size && this.timer === null)
      this.timer = setInterval(() => this.sample(), 50);
    if (!this.observations.size && this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  /** publish объединяет дорожки участника по максимальной громкости и сообщает только изменения. */
  private publish() {
    const levels = new Map<string, number>();
    for (const item of this.observations.values()) {
      if (item.envelope.level > 0)
        levels.set(
          item.participantId,
          Math.max(levels.get(item.participantId) ?? 0, item.envelope.level),
        );
    }
    if (
      levels.size === this.levels.size &&
      [...levels].every(([id, level]) => this.levels.get(id) === level)
    )
      return;
    this.levels = levels;
    this.onChange(new Map(levels));
  }

  /** disconnect освобождает только собственные узлы и обработчики наблюдения.
   * @args item — наблюдаемая дорожка, которая больше не нужна анализатору.
   */
  private disconnect(item: Observation) {
    item.track.removeEventListener?.("mute", item.unavailable);
    item.track.removeEventListener?.("ended", item.unavailable);
    item.source.disconnect();
    item.analyser.disconnect();
  }

  /** stop освобождает таймер, обработчики и Web Audio-граф.
   * Заимствованные дорожки не останавливаются: ими управляет клиент WebRTC.
   */
  stop() {
    if (this.timer !== null) clearInterval(this.timer);
    this.timer = null;
    for (const item of this.observations.values()) this.disconnect(item);
    this.observations.clear();
    document.removeEventListener("pointerdown", this.resume);
    document.removeEventListener("keydown", this.resume);
    this.output?.disconnect();
    this.context?.removeEventListener?.(
      "statechange",
      this.contextStateChanged,
    );
    if (this.context) void this.context.close().catch(() => {});
    this.context = null;
    this.output = null;
    this.levels.clear();
  }
}
