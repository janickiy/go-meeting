import { beforeEach, describe, expect, it } from "vitest";
import {
  availableDeviceId,
  captureErrorMessage,
  hasDevicePreferences,
  readDevicePreferences,
  saveDevicePreferences,
} from "./prejoinDevices";

beforeEach(() => localStorage.clear());

describe("prejoin device preferences", () => {
  it("keeps choices per account and rejects malformed stored values", () => {
    const selection = {
      ...readDevicePreferences("alice"),
      audioInputId: "microphone-2",
      videoInputId: "camera-1",
      audioOutputId: "speaker-1",
      microphoneEnabled: true,
      cameraEnabled: false,
    };
    saveDevicePreferences("alice", selection);
    expect(hasDevicePreferences("alice")).toBe(true);
    expect(readDevicePreferences("alice")).toEqual(selection);
    expect(readDevicePreferences("bob").audioInputId).toBe("");
    localStorage.setItem(
      "meet.devices.v1:bob",
      JSON.stringify({ audioInputId: 42, videoInputId: "x".repeat(513) }),
    );
    expect(readDevicePreferences("bob").audioInputId).toBe("");
    expect(readDevicePreferences("bob").videoInputId).toBe("");
  });

  it("uses the browser default when a previously selected device disappears", () => {
    const devices = [
      { kind: "audioinput", deviceId: "default" },
      { kind: "audioinput", deviceId: "microphone-1" },
    ] as MediaDeviceInfo[];
    expect(availableDeviceId(devices, "audioinput", "microphone-1")).toBe(
      "microphone-1",
    );
    expect(availableDeviceId(devices, "audioinput", "microphone-2")).toBe("");
    expect(
      availableDeviceId(
        [{ kind: "audioinput", deviceId: "default" }] as MediaDeviceInfo[],
        "audioinput",
        "microphone-2",
      ),
    ).toBe("microphone-2");
  });

  it("explains common permission and busy-device failures", () => {
    expect(
      captureErrorMessage(new DOMException("", "NotAllowedError"), "video"),
    ).toContain("Разрешите камеру");
    expect(
      captureErrorMessage(new DOMException("", "NotReadableError"), "audio"),
    ).toContain("занято");
  });
});
