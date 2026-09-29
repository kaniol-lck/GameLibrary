import { useCallback, useEffect, useRef, useState } from "react";
import { api, type ScrapeReport } from "../api/client";
import { errorMessage } from "../lib/format";

/** How long a success/failure marker stays on a card. */
const MARKER_TIMEOUT_MS = 4000;

export interface UseScrapeReturn {
  /** Games with a scrape currently in flight. */
  scrapingIds: ReadonlySet<string>;
  /** Games whose last scrape succeeded, briefly. */
  scrapedOkIds: ReadonlySet<string>;
  /** Games whose last scrape failed, briefly. */
  scrapedErrIds: ReadonlySet<string>;
  isScraping: boolean;
  /** Message from the most recent failure, for the UI to surface. */
  lastError: string;
  clearError: () => void;
  /** Scrapes one game and returns its report, or null when the call failed. */
  scrapeSingle: (id: string) => Promise<ScrapeReport | null>;
  /** Queues a batch scrape on the backend; returns how many tasks were accepted. */
  queueAll: (force: boolean) => Promise<number>;
}

/**
 * Scrape state.
 *
 * Batch scraping no longer runs in the browser. It used to spin up three worker
 * promises that each called `ScrapeGame` and then re-fetched the entire library
 * **inside the worker loop** — N games meant N full `GetGameList` round trips and N
 * full re-renders, on top of a per-card cover fetch for every card. The backend
 * queue already serialises the work and reports progress through events, so the UI
 * now just asks for the batch and listens.
 */
export function useScrape(onLibraryChanged: () => void): UseScrapeReturn {
  const [scrapingIds, setScrapingIds] = useState<Set<string>>(() => new Set());
  const [scrapedOkIds, setScrapedOkIds] = useState<Set<string>>(() => new Set());
  const [scrapedErrIds, setScrapedErrIds] = useState<Set<string>>(() => new Set());
  const [lastError, setLastError] = useState("");

  // Marker timers are tracked so they can be cancelled on unmount; previously
  // they were fire-and-forget and wrote state after the component was gone.
  const timers = useRef<number[]>([]);
  useEffect(
    () => () => {
      for (const timer of timers.current) {
        window.clearTimeout(timer);
      }
      timers.current = [];
    },
    [],
  );

  const mark = useCallback((id: string, ok: boolean) => {
    setScrapedOkIds((prev) => {
      const next = new Set(prev);
      if (ok) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
    setScrapedErrIds((prev) => {
      const next = new Set(prev);
      if (ok) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });

    const timer = window.setTimeout(() => {
      setScrapedOkIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
      setScrapedErrIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }, MARKER_TIMEOUT_MS);
    timers.current.push(timer);
  }, []);

  const clearError = useCallback(() => setLastError(""), []);

  const scrapeSingle = useCallback(
    async (id: string): Promise<ScrapeReport | null> => {
      // Additive: scraping one game must not clear the markers or the spinner of
      // a batch that is already running, which is what the old implementation did.
      setScrapingIds((prev) => {
        const next = new Set(prev);
        next.add(id);
        return next;
      });
      setLastError("");

      try {
        const report = await api.scrapeGame(id);
        const ok = !report.error;
        if (!ok) {
          setLastError(report.error ?? "scrape failed");
        }
        mark(id, ok);
        onLibraryChanged();
        return report;
      } catch (err) {
        setLastError(errorMessage(err));
        mark(id, false);
        return null;
      } finally {
        setScrapingIds((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      }
    },
    [mark, onLibraryChanged],
  );

  const queueAll = useCallback(async (force: boolean): Promise<number> => {
    setLastError("");
    try {
      return await api.queueScrapeAll(force);
    } catch (err) {
      setLastError(errorMessage(err));
      return 0;
    }
  }, []);

  return {
    scrapingIds,
    scrapedOkIds,
    scrapedErrIds,
    isScraping: scrapingIds.size > 0,
    lastError,
    clearError,
    scrapeSingle,
    queueAll,
  };
}
