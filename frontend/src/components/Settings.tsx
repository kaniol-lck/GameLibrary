import { useState, useEffect } from 'react';
import { GetConfig, SaveConfig } from '../../wailsjs/go/main/App';
import { config } from '../../wailsjs/go/models';

export default function Settings() {
  const [cfg, setCfg] = useState<config.Config | null>(null);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');
  const [newDir, setNewDir] = useState('');

  useEffect(() => {
    (async () => {
      try {
        const c = await GetConfig();
        setCfg(c);
      } catch (err) {
        setError(String(err));
      }
    })();
  }, []);

  const handleSave = async () => {
    if (!cfg) return;
    setError('');
    setSaved(false);
    try {
      await SaveConfig(cfg);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (err) {
      setError(String(err));
    }
  };

  const addGameDir = () => {
    if (!cfg || !newDir.trim()) return;
    setCfg({
      ...cfg,
      gameDirectories: [...cfg.gameDirectories, newDir.trim()],
    });
    setNewDir('');
  };

  const removeGameDir = (index: number) => {
    if (!cfg) return;
    const dirs = [...cfg.gameDirectories];
    dirs.splice(index, 1);
    setCfg({ ...cfg, gameDirectories: dirs });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') addGameDir();
  };

  if (!cfg) {
    return (
      <div className="settings-loading">
        <p>Loading settings...</p>
      </div>
    );
  }

  return (
    <div className="settings-panel">
      <div className="settings-panel-header">
        <h2>Settings</h2>
        <button className="btn btn-primary" onClick={handleSave}>
          Save
        </button>
      </div>

      {error && (
        <div className="alert alert-error">
          {error}
          <button onClick={() => setError('')} className="alert-close">&times;</button>
        </div>
      )}

      {saved && (
        <div className="alert alert-success">Settings saved successfully.</div>
      )}

      <div className="settings-content">
        <section className="settings-section">
          <h3>Machine</h3>
          <div className="form-group">
            <label>Machine Name</label>
            <input
              type="text"
              value={cfg.machineName}
              onChange={(e) =>
                setCfg({ ...cfg, machineName: e.target.value })
              }
              placeholder="Living Room PC"
            />
          </div>
        </section>

        <section className="settings-section">
          <h3>Game Directories</h3>
          <p className="form-hint">
            Relative paths are resolved from the manager's location.
          </p>

          <div className="game-dirs-list">
            {cfg.gameDirectories.length === 0 && (
              <p className="empty-hint">No directories configured. Add one below.</p>
            )}
            {cfg.gameDirectories.map((dir: string, i: number) => (
              <div key={i} className="game-dir-item">
                <span className="game-dir-path">{dir}</span>
                <button
                  className="btn btn-icon btn-danger"
                  onClick={() => removeGameDir(i)}
                  title="Remove"
                >
                  &times;
                </button>
              </div>
            ))}
          </div>

          <div className="game-dir-add">
            <input
              type="text"
              value={newDir}
              onChange={(e) => setNewDir(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder=".\\Games"
            />
            <button className="btn btn-secondary" onClick={addGameDir}>
              Add
            </button>
          </div>
        </section>

        <section className="settings-section">
          <h3>Scanning</h3>
          <div className="form-group">
            <label>Max Scan Depth</label>
            <div className="form-row">
              <input
                type="range"
                min={1}
                max={10}
                value={cfg.maxScanDepth}
                onChange={(e) =>
                  setCfg({ ...cfg, maxScanDepth: parseInt(e.target.value) })
                }
              />
              <span className="form-value">{cfg.maxScanDepth}</span>
            </div>
            <p className="form-hint">
              How many subdirectory levels to search for games. (1 = only top-level,
              3 = default)
            </p>
          </div>
        </section>

        <section className="settings-section">
          <h3>Language</h3>
          <div className="form-group">
            <select
              value={cfg.language}
              onChange={(e) =>
                setCfg({ ...cfg, language: e.target.value })
              }
            >
              <option value="zh-CN">简体中文</option>
              <option value="en-US">English</option>
              <option value="ja-JP">日本語</option>
            </select>
          </div>
        </section>

        <section className="settings-section">
          <h3>Metadata Sources</h3>
          <div className="form-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={cfg.vndbEnabled}
                onChange={(e) =>
                  setCfg({ ...cfg, vndbEnabled: e.target.checked })
                }
              />
              VNDB (Visual Novel Database)
            </label>
          </div>
          <div className="form-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={cfg.dlsiteEnabled}
                onChange={(e) =>
                  setCfg({ ...cfg, dlsiteEnabled: e.target.checked })
                }
              />
              DLsite
            </label>
          </div>
        </section>
      </div>
    </div>
  );
}
