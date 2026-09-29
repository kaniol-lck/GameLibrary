import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./App.css";
import {
  api,
  emptyQueueStatus,
  type AppConfig,
  type AppInfo,
  type Game,
  type QueueStatus,
  type ScanResult,
} from "./api/client";
import { useWailsEvents } from "./api/events";
import { useScrape } from "./hooks/useScrape";
import { deriveCounts, filterGames, navTitle, navKeyId, type NavKey } from "./lib/filters";
import { queueView } from "./lib/queue";
import { errorMessage } from "./lib/format";
import GameCard from "./components/GameCard";
import Settings from "./components/Settings";
import Sidebar from "./components/Sidebar";
import GameDetail from "./components/GameDetail";
import ContextMenu from "./components/ContextMenu";

const EMPTY_QUEUE: QueueStatus = emptyQueueStatus();

export default function App() {
  const [collapsed, setCollapsed] = useState(false);
  const [nav, setNav] = useState<NavKey>({ kind: "all" });
  const [games, setGames] = useState<Game[]>([]);
  /**
   * The open game is held by ID, not by object.
   *
   * Holding the object meant the detail panel kept rendering the snapshot it was
   * given, so after a scrape it showed stale metadata until it was closed and
   * reopened — and any mutation of that object mutated the parent's state in place.
   */
  const [selectedGameId, setSelectedGameId] = useState<string | null>(null);
  const [appInfo, setAppInfo] = useState<AppInfo | null>(null);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [pathLabels, setPathLabels] = useState<Record<string, string[]>>({});
  const [isScanning, setIsScanning] = useState(false);
  const [scanResults, setScanResults] = useState<ScanResult[] | null>(null);
  const [error, setError] = useState("");
  const [contextMenu, setContextMenu] = useState<{ id: string; x: number; y: number } | null>(null);
  const [queueStatus, setQueueStatus] = useState<QueueStatus>(EMPTY_QUEUE);
  const [queueCurrentId, setQueueCurrentId] = useState<string | null>(null);

  // Backend events can arrive in bursts (watcher + scan + queue). A sequence
  // number makes the newest list win instead of whichever response lands last.
  const listSequence = useRef(0);

  const loadGames = useCallback(async () => {
    const sequence = ++listSequence.current;
    try {
      const list = await api.listGames();
      if (sequence === listSequence.current) {
        setGames(list ?? []);
      }
    } catch (err) {
      setError(errorMessage(err));
    }
  }, []);

  const loadPathLabels = useCallback(async () => {
    try {
      setPathLabels(await api.getPathLabels());
    } catch {
      /* directory labels are cosmetic; never block the library on them */
    }
  }, []);

  const refreshLibrary = useCallback(async () => {
    await Promise.all([loadGames(), loadPathLabels()]);
  }, [loadGames, loadPathLabels]);

  const {
    scrapingIds,
    scrapedOkIds,
    scrapedErrIds,
    isScraping,
    lastError,
    clearError,
    scrapeSingle,
    queueAll,
  } = useScrape(refreshLibrary);

  useEffect(() => {
    void (async () => {
      try {
        setAppInfo(await api.getAppInfo());
      } catch (err) {
        setError(errorMessage(err));
      }
      try {
        setConfig(await api.getConfig());
      } catch (err) {
        setError(errorMessage(err));
      }
      await refreshLibrary();
    })();
  }, [refreshLibrary]);

  // Subscriptions are removed on unmount by the hook; see src/api/events.ts.
  useWailsEvents({
    "queue:status": (payload) => {
      const status = payload as QueueStatus;
      setQueueStatus(status);
      setQueueCurrentId(status.currentGameId ?? null);
    },
    "queue:done": () => {
      setQueueCurrentId(null);
      void refreshLibrary();
    },
    "library:changed": () => {
      void refreshLibrary();
    },
    "scan:complete": () => {
      void refreshLibrary();
    },
  });

  const runScan = useCallback(
    async (force: boolean) => {
      setIsScanning(true);
      setScanResults(null);
      setError("");
      try {
        const results = force ? await api.forceScan() : await api.scan();
        setScanResults(results ?? []);
        await refreshLibrary();
      } catch (err) {
        setError(errorMessage(err));
      } finally {
        setIsScanning(false);
      }
    },
    [refreshLibrary],
  );

  const runScrapeAll = useCallback(
    async (force: boolean) => {
      const queued = await queueAll(force);
      setQueueStatus(await api.queueStatus().catch(() => EMPTY_QUEUE));
      if (queued === 0) {
        setError("Nothing to scrape: every game already has complete metadata.");
      }
    },
    [queueAll],
  );

  // Derived state is computed once per change rather than once per render.
  const filterContext = useMemo(() => ({ pathLabels }), [pathLabels]);
  const visibleGames = useMemo(
    () => filterGames(games, nav, filterContext),
    [games, nav, filterContext],
  );
  const counts = useMemo(() => deriveCounts(games, filterContext), [games, filterContext]);

  const selectedGame = useMemo(
    () => (selectedGameId ? (games.find((g) => g.id === selectedGameId) ?? null) : null),
    [games, selectedGameId],
  );
  const contextGame = useMemo(
    () => (contextMenu ? (games.find((g) => g.id === contextMenu.id) ?? null) : null),
    [contextMenu, games],
  );

  const newGames = scanResults?.filter((r) => r.isNew).length ?? 0;
  const existingGames = scanResults?.filter((r) => !r.isNew && !r.error).length ?? 0;
  const failedGames = scanResults?.filter((r) => r.error).length ?? 0;

  const queueOutstanding = queueStatus.pending + queueStatus.running;
  const queueProgress = queueView(queueStatus);
  const busy = isScanning || queueOutstanding > 0;
  const statusMessage = error || lastError;

  return (
    <div id="App">
      <Sidebar
        collapsed={collapsed}
        onToggle={() => setCollapsed((value) => !value)}
        counts={counts}
        selected={navKeyId(nav)}
        onSelect={(key) => {
          setNav(key);
          setSelectedGameId(null);
        }}
        machineName={appInfo?.machineName ?? ""}
        version={appInfo?.version ?? ""}
        queueStatus={queueStatus}
        onQueueRetry={(gameId) => void scrapeSingle(gameId)}
      />

      <div className="main-area">
        <header className="top-bar">
          <div className="top-bar-actions">
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => void runScan(false)}
              disabled={busy}
            >
              {isScanning ? "Scanning…" : "Scan"}
            </button>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => void runScan(true)}
              disabled={busy}
              title="Re-identify every game directory. Stars, tags and scraped metadata are kept."
            >
              Force Scan
            </button>
            <button
              type="button"
              className="btn btn-primary btn-sm"
              onClick={() => void runScrapeAll(false)}
              disabled={busy}
              title="Queue every game that is still missing metadata"
            >
              Scrape Missing
            </button>
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => void runScrapeAll(true)}
              disabled={busy}
              title="Queue every game, replacing existing metadata"
            >
              Scrape All
            </button>
          </div>
          {queueOutstanding > 0 && (
            <div className="queue-indicator" title={queueStatus.currentTitle ?? ""}>
              {queueStatus.paused
                ? `Queue paused · ${queueStatus.pending} waiting`
                : `Scraping ${queueProgress.finished}/${queueProgress.total} · ${queueStatus.running} in progress`}
            </div>
          )}
        </header>

        {isScraping || queueOutstanding > 0 ? (
          <div
            className="progress-bar-wrapper"
            role="progressbar"
            aria-valuenow={queueProgress.percent}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label="Scrape progress"
          >
            <div
              className={`progress-bar${queueStatus.paused ? " progress-paused" : ""}`}
              style={{ width: `${queueProgress.percent}%` }}
            />
          </div>
        ) : null}

        {statusMessage && (
          <div className="alert alert-error">
            {statusMessage}
            <button
              type="button"
              onClick={() => {
                setError("");
                clearError();
              }}
              className="alert-close"
              aria-label="Dismiss error"
            >
              ×
            </button>
          </div>
        )}

        {scanResults && (
          <div className="scan-summary">
            <span className="scan-stat scan-new">+{newGames} new</span>
            <span className="scan-stat scan-existing">{existingGames} existing</span>
            {failedGames > 0 && <span className="scan-stat scan-error">{failedGames} errors</span>}
            <button type="button" className="scan-dismiss" onClick={() => setScanResults(null)}>
              Dismiss
            </button>
          </div>
        )}

        <main className="main-content">
          {nav.kind === "settings" ? (
            <Settings
              config={config}
              machineName={appInfo?.machineName ?? ""}
              logDir={appInfo?.logDir ?? ""}
              onConfigChanged={async () => {
                setConfig(await api.getConfig().catch(() => null));
                await refreshLibrary();
              }}
            />
          ) : (
            <>
              <div className="content-header">
                <h2 className="content-title">{navTitle(nav)}</h2>
                <span className="content-count">
                  {visibleGames.length} game{visibleGames.length === 1 ? "" : "s"}
                </span>
              </div>
              <div className="game-grid">
                {visibleGames.length === 0 && !busy && (
                  <div className="empty-state">
                    <div className="empty-icon">🎮</div>
                    <h2>No games here</h2>
                    <p>Add game directories in Settings, then run a scan.</p>
                  </div>
                )}
                {visibleGames.map((game) => (
                  <GameCard
                    key={game.id}
                    game={game}
                    status={
                      scrapingIds.has(game.id) || queueCurrentId === game.id
                        ? "scraping"
                        : scrapedOkIds.has(game.id)
                          ? "ok"
                          : scrapedErrIds.has(game.id)
                            ? "error"
                            : "idle"
                    }
                    onClick={(id) => setSelectedGameId(id)}
                    onContextMenu={(id, x, y) => setContextMenu({ id, x, y })}
                  />
                ))}
              </div>
            </>
          )}
        </main>

        <footer className="app-footer">
          <span>GameLibrary v{appInfo?.version ?? ""}</span>
          <span>{games.length} games</span>
        </footer>
      </div>

      {selectedGame && (
        <GameDetail
          game={selectedGame}
          onClose={() => setSelectedGameId(null)}
          onUpdated={refreshLibrary}
          onScrape={scrapeSingle}
          isScraping={scrapingIds.has(selectedGame.id) || queueCurrentId === selectedGame.id}
        />
      )}

      {contextMenu && contextGame && (
        <ContextMenu
          game={contextGame}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          onScrape={scrapeSingle}
          onUpdated={async () => {
            await refreshLibrary();
            setContextMenu(null);
          }}
        />
      )}
    </div>
  );
}
