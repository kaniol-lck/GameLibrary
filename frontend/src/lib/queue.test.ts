import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { capList, hasQueueActivity, queueView, FAILURE_LIMIT, PENDING_LIMIT } from "./queue.ts";
import type { QueueStatus } from "../api/client";

function status(overrides: Partial<QueueStatus> = {}): QueueStatus {
  return {
    pending: 0,
    running: 0,
    paused: false,
    concurrency: 4,
    runningTitles: [],
    pendingTitles: [],
    completed: 0,
    failed: 0,
    total: 0,
    failures: [],
    ...overrides,
  } as QueueStatus;
}

describe("queueView", () => {
  it("reports an empty queue", () => {
    const view = queueView(status());
    assert.equal(view.state, "idle");
    assert.equal(view.hasBatch, false);
    assert.equal(view.percent, 0);
    assert.equal(view.summary, "Task queue empty");
  });

  it("reports running work with progress over the whole batch", () => {
    const view = queueView(status({ running: 4, pending: 6, completed: 5, failed: 1, total: 16 }));
    assert.equal(view.state, "running");
    assert.equal(view.outstanding, 10);
    assert.equal(view.finished, 6);
    assert.equal(view.percent, 38);
    assert.equal(view.summary, "Scraping 6/16");
  });

  it("distinguishes paused from idle", () => {
    const paused = queueView(
      status({ running: 2, pending: 3, completed: 5, total: 10, paused: true }),
    );
    assert.equal(paused.state, "paused");
    assert.match(paused.summary, /Pausing/);

    const idle = queueView(status({ pending: 3, total: 3, paused: true }));
    assert.equal(idle.state, "paused");
    assert.equal(idle.summary, "Queued 3");
  });

  it("summarises a finished batch, mentioning failures only when there were any", () => {
    assert.equal(queueView(status({ completed: 12, total: 12 })).summary, "Done · 12 scraped");
    assert.equal(
      queueView(status({ completed: 9, failed: 3, total: 12 })).summary,
      "Done · 3 failed",
    );
  });

  it("reaches 100% only when everything has finished", () => {
    assert.equal(queueView(status({ completed: 10, total: 10 })).percent, 100);
    assert.equal(queueView(status({ completed: 9, running: 1, total: 10 })).percent, 90);
  });

  it("survives a snapshot with missing numbers", () => {
    const view = queueView({ paused: false } as QueueStatus);
    assert.equal(view.outstanding, 0);
    assert.equal(view.percent, 0);
    assert.equal(view.hasBatch, false);
  });
});

describe("hasQueueActivity", () => {
  it("is false for an untouched queue", () => {
    assert.equal(hasQueueActivity(status()), false);
  });

  it("is true while work is outstanding", () => {
    assert.equal(hasQueueActivity(status({ pending: 1 })), true);
    assert.equal(hasQueueActivity(status({ running: 1 })), true);
  });

  it("stays true after a batch so its result remains visible", () => {
    assert.equal(hasQueueActivity(status({ completed: 5, total: 5 })), true);
  });
});

describe("capList", () => {
  it("passes a short list through", () => {
    assert.deepEqual(capList(["a", "b"], PENDING_LIMIT), { shown: ["a", "b"], hidden: 0 });
  });

  it("truncates and reports the remainder", () => {
    const items = Array.from({ length: 20 }, (_, i) => `g${i}`);
    assert.deepEqual(capList(items, PENDING_LIMIT), {
      shown: items.slice(0, PENDING_LIMIT),
      hidden: 20 - PENDING_LIMIT,
    });
  });

  it("handles an empty list and a non-positive limit", () => {
    assert.deepEqual(capList([], PENDING_LIMIT), { shown: [], hidden: 0 });
    assert.deepEqual(capList(["a"], 0), { shown: ["a"], hidden: 0 });
  });

  it("uses the documented limits", () => {
    assert.equal(PENDING_LIMIT, 8);
    assert.equal(FAILURE_LIMIT, 5);
  });
});
