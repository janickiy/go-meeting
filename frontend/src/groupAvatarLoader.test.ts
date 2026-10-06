import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./api";
import { loadGroupAvatar } from "./groupAvatarLoader";

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("private avatar resource bounds", () => {
  it("limits concurrency to six and cancels queued work before it sends", async () => {
    const resolve: Array<(blob: Blob) => void> = [];
    let concurrent = 0,
      maximum = 0;
    const fetch = vi
      .spyOn(api, "groupAvatar")
      .mockImplementation((_id, signal) => {
        concurrent++;
        maximum = Math.max(maximum, concurrent);
        return new Promise<Blob>((done, reject) => {
          resolve.push(done);
          signal!.addEventListener("abort", () => reject(signal!.reason), {
            once: true,
          });
        }).finally(() => {
          concurrent--;
        });
      });
    const controllers = Array.from({ length: 8 }, () => new AbortController());
    const work = controllers.map((controller, index) =>
      loadGroupAvatar(String(index), controller.signal),
    );
    const outcomes = Promise.allSettled(work);
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(6));
    controllers[7].abort();
    resolve[0](new Blob(["avatar"]));
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(7));
    controllers.forEach((controller) => controller.abort());
    await outcomes;
    expect(maximum).toBe(6);
    expect(fetch.mock.calls.some(([id]) => id === "7")).toBe(false);
  });
  it("retries only429 twice and aborts its retry timer", async () => {
    vi.useFakeTimers();
    const fetch = vi
      .spyOn(api, "groupAvatar")
      .mockRejectedValue(new ApiError(429, "Busy"));
    const controller = new AbortController();
    const bounded = expect(
      loadGroupAvatar("group", controller.signal),
    ).rejects.toMatchObject({ status: 429 });
    await vi.runAllTimersAsync();
    await bounded;
    expect(fetch).toHaveBeenCalledTimes(3);
    fetch.mockClear();
    const canceled = new AbortController();
    const canceledResult = expect(
      loadGroupAvatar("group", canceled.signal),
    ).rejects.toMatchObject({ name: "AbortError" });
    await vi.advanceTimersByTimeAsync(0);
    canceled.abort();
    await canceledResult;
    await vi.runAllTimersAsync();
    expect(fetch).toHaveBeenCalledTimes(1);
    fetch.mockClear().mockRejectedValue(new ApiError(403, "Denied"));
    await expect(
      loadGroupAvatar("group", new AbortController().signal),
    ).rejects.toMatchObject({ status: 403 });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
