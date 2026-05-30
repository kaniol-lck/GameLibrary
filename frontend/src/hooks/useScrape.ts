import { useState, useCallback } from 'react';
import { ScrapeGame, GetGameList } from '../../wailsjs/go/main/App';
import { game } from '../../wailsjs/go/models';

const SCRAPE_PARALLEL = 3;

export interface UseScrapeReturn {
  scrapingIds: Set<string>;
  scrapedOkIds: Set<string>;
  scrapedErrIds: Set<string>;
  scrapeDone: number;
  scrapeTotal: number;
  isScraping: boolean;
  pct: number;
  scrapeSingle: (id: string) => Promise<void>;
  scrapeBatch: (targets: game.GameInfo[], force?: boolean) => Promise<void>;
  clearScrapeState: () => void;
}

export function useScrape(
  setGames: React.Dispatch<React.SetStateAction<game.GameInfo[]>>,
  onScraped?: () => void,
): UseScrapeReturn {
  const [scrapingIds, setScrapingIds] = useState<Set<string>>(new Set());
  const [scrapedOkIds, setScrapedOkIds] = useState<Set<string>>(new Set());
  const [scrapedErrIds, setScrapedErrIds] = useState<Set<string>>(new Set());
  const [scrapeDone, setScrapeDone] = useState(0);
  const [scrapeTotal, setScrapeTotal] = useState(0);

  const loadGames = useCallback(async () => {
    try {
      const list = await GetGameList();
      setGames(list || []);
    } catch { /* ignore */ }
  }, [setGames]);

  const scrapeSingle = useCallback(async (id: string) => {
    setScrapingIds((prev) => new Set(prev).add(id));
    setScrapeTotal(1);
    setScrapeDone(0);
    setScrapedOkIds(new Set());
    setScrapedErrIds(new Set());

    let ok = false;
    try {
      const r = await ScrapeGame(id);
      ok = !r.error;
    } catch { /* ignore */ }

    setScrapedOkIds(ok ? new Set([id]) : new Set());
    setScrapedErrIds(ok ? new Set() : new Set([id]));
    setScrapeDone(1);
    setScrapingIds(new Set());

    await loadGames();
    if (onScraped) onScraped();

    setTimeout(() => {
      setScrapedOkIds(new Set());
      setScrapedErrIds(new Set());
      setScrapeTotal(0);
      setScrapeDone(0);
    }, 4000);
  }, [loadGames]);

  const scrapeBatch = useCallback(async (targets: game.GameInfo[]) => {
    setScrapeDone(0);
    setScrapeTotal(targets.length);
    setScrapedOkIds(new Set());
    setScrapedErrIds(new Set());

    let idx = 0;
    let done = 0;
    const active = new Set<string>();
    const ok = new Set<string>();
    const err = new Set<string>();

    const report = () => {
      setScrapingIds(new Set(active));
      setScrapeDone(done);
      setScrapedOkIds(new Set(ok));
      setScrapedErrIds(new Set(err));
    };

    const worker = async () => {
      while (idx < targets.length) {
        const i = idx++;
        const g = targets[i];
        active.add(g.id);
        report();
        let success = false;
        try {
          const r = await ScrapeGame(g.id);
          success = !r.error;
        } catch { /* continue */ }
        active.delete(g.id);
        if (success) { ok.add(g.id); } else { err.add(g.id); }
        done++;
        report();
        await loadGames();
      }
    };

    const workers = Array.from(
      { length: Math.min(SCRAPE_PARALLEL, targets.length) },
      () => worker()
    );
    await Promise.all(workers);

    await loadGames();
    if (onScraped) onScraped();
    setScrapingIds(new Set());
    setScrapeDone(0);
    setScrapeTotal(0);
    setTimeout(() => { setScrapedOkIds(new Set()); setScrapedErrIds(new Set()); }, 4000);
  }, [loadGames]);

  const clearScrapeState = useCallback(() => {
    setScrapingIds(new Set());
    setScrapedOkIds(new Set());
    setScrapedErrIds(new Set());
    setScrapeDone(0);
    setScrapeTotal(0);
  }, []);

  const isScraping = scrapingIds.size > 0;
  const pct = scrapeTotal > 0 ? Math.round((scrapeDone / scrapeTotal) * 100) : 0;

  return {
    scrapingIds,
    scrapedOkIds,
    scrapedErrIds,
    scrapeDone,
    scrapeTotal,
    isScraping,
    pct,
    scrapeSingle,
    scrapeBatch,
    clearScrapeState,
  };
}
