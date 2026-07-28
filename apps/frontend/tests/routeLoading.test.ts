import { describe, expect, it } from "vitest";
import { createRouteLoadingTracker } from "../src/composables/useRouteLoading.ts";

function flush(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

describe("createRouteLoadingTracker", () => {
  it("start() flips loading to true", () => {
    const tracker = createRouteLoadingTracker();
    expect(tracker.loading.value).toBe(false);
    tracker.start();
    expect(tracker.loading.value).toBe(true);
    tracker.dispose();
  });

  it("start() resets completing to false when a navigation is in flight", () => {
    const tracker = createRouteLoadingTracker();
    tracker.start();
    tracker.finish();
    expect(tracker.completing.value).toBe(true);
    tracker.start();
    expect(tracker.completing.value).toBe(false);
    tracker.dispose();
  });

  it("finish() drains a single pending navigation and enters completing state", async () => {
    const tracker = createRouteLoadingTracker();
    tracker.start();
    tracker.finish();
    expect(tracker.completing.value).toBe(true);
    await flush(500);
    expect(tracker.completing.value).toBe(false);
    expect(tracker.loading.value).toBe(false);
    tracker.dispose();
  });

  it("multiple start() calls require matching finish() calls to hide", async () => {
    const tracker = createRouteLoadingTracker();
    tracker.start();
    tracker.start();
    tracker.finish();
    expect(tracker.completing.value).toBe(false);
    expect(tracker.loading.value).toBe(true);
    tracker.finish();
    expect(tracker.completing.value).toBe(true);
    await flush(500);
    expect(tracker.loading.value).toBe(false);
    tracker.dispose();
  });

  it("fail() immediately hides the bar and clears pending count", () => {
    const tracker = createRouteLoadingTracker();
    tracker.start();
    tracker.start();
    tracker.fail();
    expect(tracker.loading.value).toBe(false);
    expect(tracker.completing.value).toBe(false);
    tracker.finish();
    expect(tracker.loading.value).toBe(false);
    tracker.dispose();
  });

  it("hides after the minimum visible window when navigation finishes fast", async () => {
    const tracker = createRouteLoadingTracker();
    const t0 = Date.now();
    tracker.start();
    tracker.finish();
    await flush(500);
    const elapsed = Date.now() - t0;
    expect(tracker.loading.value).toBe(false);
    expect(elapsed).toBeGreaterThanOrEqual(150);
    tracker.dispose();
  });
});
