import { useState, useEffect, useCallback } from 'react';
import './App.css';
import { GetGameList, ScanGames, ScrapeGame, GetAppInfo } from '../wailsjs/go/main/App';
import { game, scanner } from '../wailsjs/go/models';
import GameCard from './components/GameCard';
import Settings from './components/Settings';
import Sidebar from './components/Sidebar';
import GameDetail from './components/GameDetail';

function App() {
  const [collapsed, setCollapsed] = useState(false);
  const [selectedNav, setSelectedNav] = useState('all');
  const [games, setGames] = useState<game.GameInfo[]>([]);
  const [selectedGame, setSelectedGame] = useState<game.GameInfo | null>(null);
  const [appInfo, setAppInfo] = useState<Record<string, string> | null>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [scanResults, setScanResults] = useState<scanner.ScanResult[] | null>(null);
  const [scrapingIds, setScrapingIds] = useState<Set<string>>(new Set());
  const [scrapeProgress, setScrapeProgress] = useState('');
  const [error, setError] = useState('');

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
      try {
        const info = await GetAppInfo();
        setAppInfo(info);
      } catch { /* ignore */ }
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
    setScrapeProgress('');
    const ids = new Set<string>();

    const targets = games.filter((g) => !g.metadata?.coverUrl || !g.metadata?.developer);
    if (targets.length === 0) {
      setError('All games already have metadata.');
      return;
    }

    for (let i = 0; i < targets.length; i++) {
      const g = targets[i];
      ids.add(g.id);
      setScrapingIds(new Set(ids));
      setScrapeProgress(`${i + 1} / ${targets.length}`);

      try {
        await ScrapeGame(g.id);
      } catch {
        /* continue */
      }
    }

    await loadGames();
    setScrapingIds(new Set());
    setScrapeProgress('');
  };

  const handleGameClick = (g: game.GameInfo) => {
    setSelectedGame(g);
  };

  const handleDetailClose = () => {
    setSelectedGame(null);
  };

  const handleDetailUpdated = () => {
    loadGames();
  };

  const filteredGames = (() => {
    if (selectedNav === 'all') return games;
    if (selectedNav.startsWith('type:')) {
      return games.filter((g) => g.type === selectedNav.slice(5));
    }
    if (selectedNav.startsWith('tag:')) {
      return games.filter((g) => (g.metadata?.tags || []).includes(selectedNav.slice(4)));
    }
    return games;
  })();

  const newGames = scanResults?.filter((r) => r.isNew).length ?? 0;
  const existingGames = scanResults?.filter((r) => !r.isNew && !r.error).length ?? 0;
  const errorGames = scanResults?.filter((r) => r.error).length ?? 0;
  const isScraping = scrapingIds.size > 0;

  return (
    <div id="App">
      <Sidebar
        collapsed={collapsed}
        onToggle={() => setCollapsed(!collapsed)}
        games={games}
        selectedNav={selectedNav}
        onSelectNav={(key) => { setSelectedNav(key); setSelectedGame(null); }}
        machineName={appInfo?.['machineName'] ?? ''}
      />

      <div className="main-area">
        <header className="top-bar">
          <div className="top-bar-left">
            <span className="app-machine">
              {appInfo?.['machineName'] ?? ''}
            </span>
            {scrapeProgress && (
              <span className="scrape-progress">{scrapeProgress}</span>
            )}
          </div>
          <div className="top-bar-right">
            {games.length > 0 && (
              <button
                className="btn btn-secondary"
                onClick={handleScrapeAll}
                disabled={isScraping}
              >
                {isScraping ? `Scraping... ${scrapeProgress}` : 'Scrape All'}
              </button>
            )}
            <button
              className="btn btn-primary"
              onClick={handleScan}
              disabled={isScanning}
            >
              {isScanning ? 'Scanning...' : 'Scan Games'}
            </button>
          </div>
        </header>

        {error && (
          <div className="alert alert-error">
            {error}
            <button onClick={() => setError('')} className="alert-close">&times;</button>
          </div>
        )}

        {scanResults && (
          <div className="scan-summary">
            <span className="scan-stat scan-new">+{newGames} new</span>
            <span className="scan-stat scan-existing">{existingGames} existing</span>
            {errorGames > 0 && (
              <span className="scan-stat scan-error">{errorGames} errors</span>
            )}
            <button className="scan-dismiss" onClick={() => setScanResults(null)}>
              Dismiss
            </button>
          </div>
        )}

        <main className="main-content">
          {selectedNav === 'settings' ? (
            <Settings />
          ) : (
            <>
              <div className="content-header">
                <h2 className="content-title">
                  {selectedNav === 'all'
                    ? 'All Games'
                    : selectedNav.startsWith('tag:')
                      ? selectedNav.slice(4)
                      : selectedNav.slice(5)}
                </h2>
                <span className="content-count">
                  {filteredGames.length} game{filteredGames.length !== 1 ? 's' : ''}
                </span>
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
                  <GameCard
                    key={g.id}
                    game={g}
                    onClick={handleGameClick}
                    isScraping={scrapingIds.has(g.id)}
                  />
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
        <GameDetail
          game={selectedGame}
          onClose={handleDetailClose}
          onUpdated={handleDetailUpdated}
        />
      )}
    </div>
  );
}

export default App;
