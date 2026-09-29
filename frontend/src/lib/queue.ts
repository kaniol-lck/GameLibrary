import type { QueueStatus } from "../api/client";

/**
 * Pure helpers for the scrape task list. Kept out of the component so the
 * progress arithmetic and the list capping can be unit tested without a DOM.
 */

export type QueueState = "idle" | "running" | "paused";

/** How many pending titles the panel lists before summarising the rest. */
export const PENDING_LIMIT = 8;

/** How many failures the panel lists before summarising the rest. */
export const FAILURE_LIMIT = 5;

export interface QueueView {
  state: QueueState;
  /** Tasks waiting or in flight. */
  outstanding: number;
  /** Tasks that have finished, successfully or not. */
  finished: number;
  /** Size of the batch: finished plus outstanding. */
  total: number;
  /** Completion percentage, 0-100, over the whole batch. */
  percent: number;
  /** True when there is a batch to show at all. */
  hasBatch: boolean;
  /** Short status line for the collapsed header. */
  summary: string;
}

/** Derives everything the panel displays from a queue snapshot. */
export function queueView(status: QueueStatus): QueueView {
  const pending = status.pending ?? 0;
  const running = status.running ?? 0;
  const completed = status.completed ?? 0;
  const failed = status.failed ?? 0;

  const outstanding = pending + running;
  const finished = completed + failed;
  const total = status.total ?? outstanding + finished;

  const state: QueueState = status.paused ? "paused" : running > 0 ? "running" : "idle";
  const percent = total > 0 ? Math.round((finished / total) * 100) : 0;

  const summary = (() => {
    if (running > 0 && status.paused) return `Pausing… ${finished}/${total}`;
    if (running > 0) return `Scraping ${finished}/${total}`;
    if (pending > 0) return `Queued ${pending}`;
    if (finished > 0) return failed > 0 ? `Done · ${failed} failed` : `Done · ${completed} scraped`;
    return "Task queue empty";
  })();

  return { state, outstanding, finished, total, percent, hasBatch: total > 0, summary };
}

/** True when the panel has anything worth showing. */
export function hasQueueActivity(status: QueueStatus): boolean {
  return (status.total ?? 0) > 0 || (status.pending ?? 0) > 0 || (status.running ?? 0) > 0;
}

export interface CappedList {
  shown: string[];
  hidden: number;
}

/** Truncates a list for display while reporting how many entries were omitted. */
export function capList(items: readonly string[], limit: number): CappedList {
  if (limit <= 0 || items.length <= limit) {
    return { shown: [...items], hidden: 0 };
  }
  return { shown: items.slice(0, limit), hidden: items.length - limit };
}
