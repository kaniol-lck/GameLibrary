import { useState, useEffect, useCallback } from 'react';
import './App.css';
import { GetGameList, ScanGames, GetAppInfo, GetConfig } from '../wailsjs/go/main/App';
import { game, scanner, config } from '../wailsjs/go/models';
import { useScrape } from './hooks/useScrape';
import GameCard from './components/GameCard';
import Settings from './components/Settings';
import Sidebar from './components/Sidebar';
import GameDetail from './components/GameDetail';
import ContextMenu from './components/ContextMenu';

function isIncomplete(g: game.GameInfo): boolean {
  if (!g.metadata) return true;
  const m = g.metadata;
  if (!m.coverUrl) return true;
  if (!m.coverLandscape) return true;
  if (!m.developer && !m.publisher) return true;
  if (!m.description || m.description.length < 10) return true;
  if (!m.releaseDate) return true;
  if (!m.tags || m.tags.length === 0) return true;
  return false;
}

function App() {
  const [collapsed, setCollapsed] = useState(false);
  const [selectedNav, setSelectedNav] = useState('all');
  const [games, setGames] = useState<game.GameInfo[]>([]);
  const [selectedGame, setSelectedGame] = useState<game.GameInfo | null>(null);
  const [appInfo, setAppInfo] = useState<Record<string, string> | null>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [scanResults, setScanResults] = useState<scanner.ScanResult[] | null>(null);
  const [error, setError] = useState('');
  const [ctxMenu, setCtxMenu] = useState<{ game: game.GameInfo; x: number; y: number } | null>(null);
  const [coverRefresh, setCoverRefresh] = useState(0);
  const [pathLabels, setPathLabels] = useState<Record<string, string[]>>({});

  const {
    scrapingIds, scrapedOkIds, scrapedErrIds,
    scrapeDone, scrapeTotal, isScraping, pct,
    scrapeSingle, scrapeBatch,
  } = useScrape(setGames, () => setCoverRefresh((k) => k + 1));

  const loadGames = useCallback(async () => {
    try {
      const list = await GetGameList();
      setGames(list || []);
    } catch (err) {
      setError(String(err));
    }
  }, []);

  useEffect(() => {
    (async () => {
      try { const info = await GetAppInfo(); setAppInfo(info); } catch { /* ignore */ }
      try { const c = await GetConfig(); setPathLabels(c.gameDirectoryLabels || {}); } catch { /* ignore */ }
      await loadGames();
    })();
  }, [loadGames]);

  const handleScan = async () => {
    setIsScanning(true);
    setScanResults(null);
    setError('');
    try {
      const results = await ScanGames();
      setScanResults(results || []);
      await loadGames();
    } catch (err) {
      setError(String(err));
    } finally {
      setIsScanning(false);
    }
  };

  const handleScrapeAll = async () => {
    setError('');
    const targets = games.filter(isIncomplete);
    if (targets.length === 0) {
      setError('All games have complete metadata. Use Force Scrape All to re-scrape.');
      return;
    }
    await scrapeBatch(targets);
  };

  const handleForceScrapeAll = async () => {
    setError('');
    if (games.length === 0) return;
    await scrapeBatch([...games]);
  };

  const handleGameClick = (g: game.GameInfo) => {
    setSelectedGame(g);
  };

  const handleGameContextMenu = (g: game.GameInfo, x: number, y: number) => {
    setCtxMenu({ game: g, x, y });
  };

  const handleDetailClose = () => { setSelectedGame(null); };
  const handleDetailUpdated = () => { loadGames(); };

  const filteredGames = (() => {
    if (selectedNav === 'all') return games.filter((g) => ((g as any).platforms || []).length > 0);
    if (selectedNav === 'starred') return games.filter((g) => g.starred);
    if (selectedNav.startsWith('platform:')) {
      const plat = selectedNav.slice(9);
      if (plat === 'unmatched') {
        return games.filter((g) => ((g as any).platforms || []).length === 0);
      }
      return games.filter((g) => {
        const plats: any[] = (g as any).platforms || [];
        return plats.some((p: any) => p.platform === plat);
      });
    }
    if (selectedNav.startsWith('type:')) return games.filter((g) => g.type === selectedNav.slice(5));
    if (selectedNav.startsWith('tag:')) return games.filter((g) => (g.metadata?.tags || []).includes(selectedNav.slice(4)));
    if (selectedNav.startsWith('usertag:')) return games.filter((g) => (g.tags || []).includes(selectedNav.slice(8)));
    if (selectedNav.startsWith('pathlabel:')) {
      const label = selectedNav.slice(10);
      const exeDir = (appInfo?.['exeDir'] || '').replace(/\\/g, '/');
      return games.filter((g: any) => {
        const gd = (g.gameDir || '').replace(/\\/g, '/');
        for (const [dirPath, labels] of Object.entries(pathLabels)) {
          let absPath = dirPath.replace(/\\/g, '/');
          if (absPath.startsWith('.')) {
            absPath = exeDir + '/' + absPath;
          }
          while (absPath.includes('/./')) absPath = absPath.replace('/./', '/');
          while (absPath.includes('/../')) {
            const parts = absPath.split('/');
            const idx = parts.indexOf('..');
            if (idx > 1) { parts.splice(idx - 1, 2); absPath = parts.join('/'); } else break;
          }
          if (gd.startsWith(absPath) && (labels as string[]).includes(label)) return true;
        }
        return false;
      });
    }
    return games;
  })();

  const newGames = scanResults?.filter((r) => r.isNew).length ?? 0;
  const existingGames = scanResults?.filter((r) => !r.isNew && !r.error).length ?? 0;
  const errorGames = scanResults?.filter((r) => r.error).length ?? 0;

  const getContentTitle = () => {
    if (selectedNav === 'all') return 'All Games';
    if (selectedNav === 'starred') return 'Starred';
    if (selectedNav === 'platform:unmatched') return 'Unmatched';
    if (selectedNav.startsWith('platform:')) return selectedNav.slice(9);
    if (selectedNav.startsWith('type:')) return selectedNav.slice(5);
    if (selectedNav.startsWith('tag:')) return selectedNav.slice(4);
    if (selectedNav.startsWith('usertag:')) return '#' + selectedNav.slice(8);
    if (selectedNav.startsWith('pathlabel:')) return selectedNav.slice(10);
    return 'Games';
  };

  return (
    <div id="App">
      <Sidebar
        collapsed={collapsed}
        onToggle={() => setCollapsed(!collapsed)}
        games={games}
        selectedNav={selectedNav}
        onSelectNav={(key) => { setSelectedNav(key); setSelectedGame(null); }}
        machineName={appInfo?.['machineName'] ?? ''}
        pathLabels={pathLabels}
        exeDir={appInfo?.['exeDir'] || ''}
      />

      <div className="main-area">
        <header className="top-bar">
          <div className="top-bar-left">
            <span className="app-machine">{appInfo?.['machineName'] ?? ''}</span>
            {isScraping && <span className="scrape-progress-text">{scrapeDone} / {scrapeTotal}</span>}
          </div>
          <div className="top-bar-right">
            {games.length > 0 && (
              <>
                <button className="btn btn-secondary" onClick={handleScrapeAll} disabled={isScraping}>
                  {isScraping ? `Scraping... ${scrapeDone}/${scrapeTotal}` : 'Scrape All'}
                </button>
                <button className="btn btn-ghost-sm" onClick={handleForceScrapeAll} disabled={isScraping} title="Force re-scrape all games">Force</button>
              </>
            )}
            <button className="btn btn-primary" onClick={handleScan} disabled={isScanning}>
              {isScanning ? 'Scanning...' : 'Scan Games'}
            </button>
          </div>
        </header>

        {isScraping && <div className="progress-bar-wrapper"><div className="progress-bar" style={{ width: `${pct}%` }} /></div>}

        {error && (
          <div className="alert alert-error">{error}<button onClick={() => setError('')} className="alert-close">&times;</button></div>
        )}

        {scanResults && (
          <div className="scan-summary">
            <span className="scan-stat scan-new">+{newGames} new</span>
            <span className="scan-stat scan-existing">{existingGames} existing</span>
            {errorGames > 0 && <span className="scan-stat scan-error">{errorGames} errors</span>}
            <button className="scan-dismiss" onClick={() => setScanResults(null)}>Dismiss</button>
          </div>
        )}

        <main className="main-content">
          {selectedNav === 'settings' ? (
            <Settings />
          ) : (
            <>
              <div className="content-header">
                <h2 className="content-title">{getContentTitle()}</h2>
                <span className="content-count">{filteredGames.length} game{filteredGames.length !== 1 ? 's' : ''}</span>
              </div>
              <div className="game-grid">
                {filteredGames.length === 0 && !isScanning && (
                  <div className="empty-state">
                    <div className="empty-icon">&#127918;</div>
                    <h2>No games found</h2>
                    <p>Click "Scan Games" to search for games in your configured directories.</p>
                  </div>
                )}
                {filteredGames.map((g) => (
                  <GameCard key={g.id} game={g} onClick={handleGameClick}
                    onContextMenu={handleGameContextMenu}
                    isScraping={scrapingIds.has(g.id)}
                    scrapedOk={scrapedOkIds.has(g.id)}
                    scrapedErr={scrapedErrIds.has(g.id)}
                    refreshKey={coverRefresh} />
                ))}
              </div>
            </>
          )}
        </main>

        <footer className="app-footer">
          <span>GameLibrary v{appInfo?.['version'] ?? '0.1.0'}</span>
          <span>{games.length} games</span>
        </footer>
      </div>

      {selectedGame && (
        <GameDetail game={selectedGame} onClose={handleDetailClose} onUpdated={handleDetailUpdated}
          onScrape={scrapeSingle} isScraping={isScraping} />
      )}

      {ctxMenu && (
        <ContextMenu game={ctxMenu.game} x={ctxMenu.x} y={ctxMenu.y}
          onClose={() => setCtxMenu(null)}
          onScrape={scrapeSingle}
          onUpdated={() => { loadGames(); setCtxMenu(null); }} />
      )}
    </div>
  );
}

export default App;
