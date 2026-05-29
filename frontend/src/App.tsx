import { useState, useEffect, useCallback } from 'react';
import './App.css';
import { GetGameList, ScanGames, GetAppInfo } from '../wailsjs/go/main/App';
import { main } from '../wailsjs/go/models';
import GameCard from './components/GameCard';
import Settings from './components/Settings';

type Page = 'library' | 'settings';

function App() {
  const [page, setPage] = useState<Page>('library');
  const [games, setGames] = useState<main.GameInfo[]>([]);
  const [appInfo, setAppInfo] = useState<Record<string, string> | null>(null);
  const [isScanning, setIsScanning] = useState(false);
  const [scanResults, setScanResults] = useState<main.ScanResult[] | null>(null);
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

  const handleGameClick = (game: main.GameInfo) => {
    console.log('Selected game:', game.title);
  };

  const newGames = scanResults?.filter((r) => r.isNew).length ?? 0;
  const existingGames = scanResults?.filter((r) => !r.isNew && !r.error).length ?? 0;
  const errorGames = scanResults?.filter((r) => r.error).length ?? 0;

  if (page === 'settings') {
    return <Settings onBack={() => setPage('library')} />;
  }

  return (
    <div id="App">
      <header className="app-header">
        <div className="header-left">
          <h1 className="app-title">GameLibrary</h1>
          {appInfo && (
            <span className="app-machine">[{appInfo['machineName']}]</span>
          )}
        </div>
        <div className="header-right">
          <button
            className="btn btn-primary"
            onClick={handleScan}
            disabled={isScanning}
          >
            {isScanning ? 'Scanning...' : 'Scan Games'}
          </button>
          <button
            className="btn btn-ghost"
            onClick={() => setPage('settings')}
            title="Settings"
          >
            &#9881;
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
          <button
            className="scan-dismiss"
            onClick={() => setScanResults(null)}
          >
            Dismiss
          </button>
        </div>
      )}

      <main className="game-grid">
        {games.length === 0 && !isScanning && (
          <div className="empty-state">
            <div className="empty-icon">&#127918;</div>
            <h2>No games found</h2>
            <p>Click "Scan Games" to search for games in your configured directories.</p>
          </div>
        )}
        {games.map((game) => (
          <GameCard key={game.id} game={game} onClick={handleGameClick} />
        ))}
      </main>

      <footer className="app-footer">
        <span>GameLibrary v{appInfo?.['version'] ?? '0.1.0'}</span>
        <span>{games.length} games</span>
      </footer>
    </div>
  );
}

export default App;
