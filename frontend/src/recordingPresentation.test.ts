import { describe, expect, it } from "vitest";
import type { ConferenceRecording } from "./types";
import { formatDate } from "./utils";
import {
  formatRecordingDuration,
  formatRecordingSize,
  playableRecordings,
  recordingDate,
  recordingDetailPath,
  recordingMediaFile,
  recordingPreviewFile,
  safeRecordingUrl,
} from "./recordingPresentation";

/** recording создаёт минимальную запись для изолированной проверки представления.
 * @args patch — свойства, отличающиеся от готовой записи с безопасным видео.
 * @return запись без вымышленных дополнительных метаданных.
 */
function recording(
  patch: Partial<ConferenceRecording> = {},
): ConferenceRecording {
  return {
    uuid: "record-1",
    conferenceId: "conference-1",
    mode: "composite",
    status: "ready",
    createdAt: "2026-10-04T12:00:00Z",
    files: [
      { fileType: "final_mp4", url: "https://storage.invalid/final.mp4" },
    ],
    ...patch,
  };
}

describe("безопасные ссылки на файлы записи", () => {
  it("сохраняет подписанный адрес побайтно без нормализации query", () => {
    const signed =
      "https://storage.invalid/bucket/%D0%B7%D0%B0%D0%BF%D0%B8%D1%81%D1%8C.mp4?X-Amz-Credential=access%2f20261004%2Fs3&X-Amz-Signature=aBc012%2B%2F&X-Amz-Date=20261004T120000Z";
    expect(safeRecordingUrl(signed)).toBe(signed);
  });

  it.each([
    "https://storage.invalid/video.mp4?signature=x%2Fy",
    "http://localhost:9000/bucket/video.mp4?signature=test",
    "HTTPS://storage.invalid/video.mp4",
    "/storage/bucket/video.mp4?signature=x%2Fy",
  ])("принимает допустимый HTTP(S) или локальный адрес %s", (url) => {
    expect(safeRecordingUrl(url)).toBe(url);
  });

  it.each([
    undefined,
    "",
    "javascript:alert(1)",
    "data:video/mp4;base64,AAAA",
    "blob:https://meeting.invalid/private",
    "file:///private/video.mp4",
    "ftp://storage.invalid/video.mp4",
    "//foreign.invalid/video.mp4",
    "video.mp4",
    "http:storage.invalid/video.mp4",
    "https://",
    "https://[malformed]/video.mp4",
    "https://storage.invalid:99999/video.mp4",
    "https://user:password@storage.invalid/video.mp4",
    "https://user@storage.invalid/video.mp4",
    "https://%75ser:%70assword@storage.invalid/video.mp4",
    " https://storage.invalid/video.mp4",
    "https://storage.invalid/video.mp4 ",
    "https://storage.invalid/video file.mp4",
    "https://storage.invalid/video\n.mp4",
    "https://storage.invalid/video\r.mp4",
    "https://storage.invalid/video\t.mp4",
    "https://storage.invalid/video\u0000.mp4",
    "https://storage.invalid/video\u007f.mp4",
    "https://storage.invalid\\foreign.invalid/video.mp4",
    "/\\foreign.invalid/video.mp4",
  ])("отвергает неподдерживаемый или неоднозначный адрес %s", (url) => {
    expect(safeRecordingUrl(url)).toBeUndefined();
  });
});

describe("файлы и доступность записи", () => {
  it("предпочитает безопасное видео аудио независимо от порядка файлов", () => {
    const audio = { fileType: "final_audio", url: "/storage/audio.mp4" };
    const video = { fileType: "final_mp4", url: "/storage/video.mp4" };
    expect(recordingMediaFile(recording({ files: [audio, video] }))).toBe(
      video,
    );
  });

  it("пропускает небезопасное видео и выбирает настоящее аудио", () => {
    const audio = { fileType: "final_audio", url: "/storage/audio.mp4" };
    expect(
      recordingMediaFile(
        recording({
          files: [{ fileType: "final_mp4", url: "javascript:alert(1)" }, audio],
        }),
      ),
    ).toBe(audio);
    expect(recordingMediaFile(recording({ files: [] }))).toBeUndefined();
  });

  it("берёт только безопасное реальное превью и не заменяет его другим файлом", () => {
    const preview = { fileType: "preview_jpg", url: "/storage/preview.jpg" };
    expect(
      recordingPreviewFile(
        recording({
          files: [
            { fileType: "preview_jpg", url: "data:image/jpeg;base64,AA" },
            { fileType: "final_mp4", url: "/storage/video.mp4" },
            preview,
          ],
        }),
      ),
    ).toBe(preview);
    expect(recordingPreviewFile(recording())).toBeUndefined();
  });

  it("оставляет последнюю версию UUID перед фильтрацией состояния", () => {
    const oldReady = recording({ uuid: "record-a" });
    const latestFailed = recording({ uuid: "record-a", status: "failed" });
    const oldProcessing = recording({ uuid: "record-b", status: "processing" });
    const latestReady = recording({ uuid: "record-b", durationSec: 123 });
    const unsafe = recording({
      uuid: "record-c",
      files: [{ fileType: "final_mp4", url: "javascript:alert(1)" }],
    });
    const noMedia = recording({
      uuid: "record-d",
      files: [{ fileType: "preview_jpg", url: "/storage/preview.jpg" }],
    });
    expect(
      playableRecordings([
        oldReady,
        oldProcessing,
        latestFailed,
        latestReady,
        unsafe,
        noMedia,
      ]),
    ).toEqual([latestReady]);
    expect(playableRecordings([])).toEqual([]);
  });
});

describe("метаданные записи", () => {
  it.each([
    [0, "0:00"],
    [0.5, "0:00"],
    [59.99, "0:59"],
    [60, "1:00"],
    [3599, "59:59"],
    [3600, "1:00:00"],
    [3661.99, "1:01:01"],
  ])(
    "показывает измеренную длительность %s без округления вверх",
    (value, expected) => {
      expect(formatRecordingDuration(value as number)).toBe(expected);
    },
  );

  it.each([undefined, null, -1, NaN, Infinity, -Infinity])(
    "не подменяет неизвестную длительность %s нулём",
    (value) => {
      expect(formatRecordingDuration(value)).toBe("—");
    },
  );

  it.each([
    [0, "0 Б"],
    [0.5, "0,5 Б"],
    [1, "1 Б"],
    [1024, "1 КБ"],
    [1536, "1,5 КБ"],
    [1024 ** 2, "1 МБ"],
    [1024 ** 3, "1 ГБ"],
    [1024 ** 4, "1 ТБ"],
  ])("форматирует известный размер %s", (value, expected) => {
    expect(formatRecordingSize(value as number)).toBe(expected);
  });

  it.each([undefined, null, -1, NaN, Infinity, -Infinity])(
    "не выдумывает отсутствующий размер %s",
    (value) => {
      expect(formatRecordingSize(value)).toBe("—");
    },
  );

  it("выбирает время начала записи, затем корректную дату создания", () => {
    const startedAt = "2026-10-04T12:34:56Z";
    expect(recordingDate(recording({ startedAt }))).toBe(formatDate(startedAt));
    const fallback = recording({ startedAt: "invalid-date" });
    expect(recordingDate(fallback)).toBe(formatDate(fallback.createdAt));
    expect(
      recordingDate(recording({ startedAt: "invalid", createdAt: "invalid" })),
    ).toBe("Дата не указана");
  });

  it("кодирует UUID и контекст встречи без внедрения дополнительных параметров", () => {
    const conferenceId = "room/a?x=1&other=2 + кириллица";
    const recordingId = "record/a?x=1&other=2#fragment";
    const path = recordingDetailPath(conferenceId, recordingId);
    const url = new URL(path, "https://meeting.invalid");
    expect(url.origin).toBe("https://meeting.invalid");
    expect(url.pathname).toBe(`/recordings/${encodeURIComponent(recordingId)}`);
    expect([...url.searchParams.keys()]).toEqual(["conference"]);
    expect(url.searchParams.get("conference")).toBe(conferenceId);
    expect(url.hash).toBe("");
  });
});
