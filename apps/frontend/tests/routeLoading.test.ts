/// <reference types="node" />

import assert from "node:assert/strict";
import { test } from "node:test";
import { createRouteLoadingTracker } from "../src/composables/useRouteLoading.ts";

function flush(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

test("start() flips loading to true", () => {
  const tracker = createRouteLoadingTracker();
  assert.equal(tracker.loading.value, false);
  tracker.start();
  assert.equal(tracker.loading.value, true);
  tracker.dispose();
});

test("start() resets completing to false when a navigation is in flight", () => {
  const tracker = createRouteLoadingTracker();
  tracker.start();
  tracker.finish();
  assert.equal(tracker.completing.value, true);
  tracker.start();
  assert.equal(tracker.completing.value, false);
  tracker.dispose();
});

test("finish() drains a single pending navigation and enters completing state", async () => {
  const tracker = createRouteLoadingTracker();
  tracker.start();
  tracker.finish();
  assert.equal(tracker.completing.value, true);
  await flush(500);
  assert.equal(tracker.completing.value, false);
  assert.equal(tracker.loading.value, false);
  tracker.dispose();
});

test("multiple start() calls require matching finish() calls to hide", async () => {
  const tracker = createRouteLoadingTracker();
  tracker.start();
  tracker.start();
  tracker.finish();
  assert.equal(tracker.completing.value, false, "still one navigation pending");
  assert.equal(tracker.loading.value, true);
  tracker.finish();
  assert.equal(tracker.completing.value, true);
  await flush(500);
  assert.equal(tracker.loading.value, false);
  tracker.dispose();
});

test("fail() immediately hides the bar and clears pending count", () => {
  const tracker = createRouteLoadingTracker();
  tracker.start();
  tracker.start();
  tracker.fail();
  assert.equal(tracker.loading.value, false);
  assert.equal(tracker.completing.value, false);
  tracker.finish();
  assert.equal(tracker.loading.value, false, "fail() must clear pending count");
  tracker.dispose();
});

test("hides after the minimum visible window when navigation finishes fast", async () => {
  const tracker = createRouteLoadingTracker();
  const t0 = Date.now();
  tracker.start();
  tracker.finish();
  await flush(500);
  const elapsed = Date.now() - t0;
  assert.equal(tracker.loading.value, false);
  assert.ok(elapsed >= 150, `expected min visible hold, got ${elapsed}ms`);
  tracker.dispose();
});
