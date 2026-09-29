import { useState } from "react";
import { api, type QueueStatus } from "../api/client";
import { errorMessage } from "../lib/format";
import { capList, queueView, FAILURE_LIMIT, PENDING_LIMIT } from "../lib/queue";

export interface QueuePanelProps {
  status: QueueStatus;
  /** Re-runs the scrape for one game, used by the retry button on a failure. */
  onRetry?: (gameId: string) => void;
}

/**
 * The scrape task list.
 *
 * This replaces a popup that only appeared while work was outstanding and showed a
 * single running title, the pending count and an unbounded list of names. It now
 * reports real progress, shows every task that is running at once (scraping runs
 * several games in parallel), keeps the outcome visible after the batch finishes,
 * and lists recent failures with the reason so a game that could not be matched is
 * actionable rather than silent.
 */
export default function QueuePanel({ status, onRetry }: QueuePanelProps) {
  const [expanded, setExpanded] = useState(false);
  const [error, setError] = useState("");

  const view = queueView(status);
  const runningTitles = status.runningTitles ?? [];
  const pending = capList(status.pendingTitles ?? [], PENDING_LIMIT);

  const failures = status.failures ?? [];
  const shownFailures = failures.slice(0, FAILURE_LIMIT);
  const hiddenFailures = failures.length - shownFailures.length;

  const finished = (status.completed ?? 0) + (status.failed ?? 0);

  const run = async (action: () => Promise<unknown>) => {
    setError("");
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const stateIcon =
    view.state === "paused" ? "\u23F8" : view.state === "running" ? "\u25C9" : "\u2713";

  return (
    <section className={`queue-panel queue-panel-${view.state}`} aria-label="Scrape queue">
      <button
        type="button"
        className="queue-panel-header"
        onClick={() => setExpanded((value) => !value)}
        aria-expanded={expanded}
      >
        <span className="queue-panel-icon" aria-hidden="true">
          {stateIcon}
        </span>
        <span className="queue-panel-summary" aria-live="polite">
          {view.summary}
        </span>
        <span className="queue-panel-arrow" aria-hidden="true">
          {expanded ? "\u25BE" : "\u25B8"}
        </span>
      </button>

      {view.hasBatch && (
        <div
          className="queue-progress"
          role="progressbar"
          aria-valuenow={view.percent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label="Scrape progress"
        >
          <div
            className={`queue-progress-bar${view.state === "paused" ? " queue-progress-paused" : ""}`}
            style={{ width: `${view.percent}%` }}
          />
        </div>
      )}

      {expanded && (
        <div className="queue-panel-body">
          <div className="queue-counts">
            <span>{status.completed ?? 0} scraped</span>
            {(status.failed ?? 0) > 0 && (
              <span className="queue-count-failed">{status.failed} failed</span>
            )}
            {view.outstanding > 0 && <span>{view.outstanding} left</span>}
            {(status.concurrency ?? 1) > 1 && (
              <span className="queue-count-workers">{status.concurrency} at a time</span>
            )}
          </div>

          {runningTitles.length > 0 && (
            <div className="queue-section">
              <div className="queue-section-title">Running</div>
              <ul className="queue-list">
                {runningTitles.map((title, index) => (
                  <li key={`run-${title}-${index}`} className="queue-item queue-item-running">
                    <span className="queue-spinner" aria-hidden="true" />
                    <span className="queue-item-title" title={title}>
                      {title}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {pending.shown.length > 0 && (
            <div className="queue-section">
              <div className="queue-section-title">Waiting</div>
              <ul className="queue-list">
                {pending.shown.map((title, index) => (
                  <li key={`wait-${title}-${index}`} className="queue-item">
                    <span className="queue-dot" aria-hidden="true" />
                    <span className="queue-item-title" title={title}>
                      {title}
                    </span>
                  </li>
                ))}
              </ul>
              {pending.hidden > 0 && <div className="queue-more">and {pending.hidden} more…</div>}
            </div>
          )}

          {shownFailures.length > 0 && (
            <div className="queue-section">
              <div className="queue-section-title">Failed</div>
              <ul className="queue-list">
                {shownFailures.map((failure, index) => (
                  <li
                    key={`fail-${failure.gameId}-${index}`}
                    className="queue-item queue-item-failed"
                  >
                    <span className="queue-item-body">
                      <span className="queue-item-title" title={failure.title ?? failure.gameId}>
                        {failure.title || failure.gameId}
                      </span>
                      {failure.error && <span className="queue-item-error">{failure.error}</span>}
                    </span>
                    {onRetry && (
                      <button
                        type="button"
                        className="queue-retry"
                        onClick={() => onRetry(failure.gameId)}
                        title="Scrape this game again"
                        aria-label={`Scrape ${failure.title ?? failure.gameId} again`}
                      >
                        {"\u21BB"}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
              {hiddenFailures > 0 && <div className="queue-more">and {hiddenFailures} more…</div>}
            </div>
          )}

          {error && <div className="queue-error">{error}</div>}

          <div className="queue-actions">
            <button
              type="button"
              onClick={() => void run(status.paused ? api.queueResume : api.queuePause)}
              aria-label={status.paused ? "Resume the scrape queue" : "Pause the scrape queue"}
            >
              {status.paused ? "Resume" : "Pause"}
            </button>
            <button
              type="button"
              onClick={() => void run(api.queueClear)}
              disabled={view.outstanding === 0}
              aria-label="Remove every waiting task"
            >
              Clear waiting
            </button>
            <button
              type="button"
              onClick={() => void run(api.queueResetProgress)}
              disabled={view.outstanding > 0 || finished === 0}
              aria-label="Dismiss the results of the last batch"
            >
              Dismiss
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
