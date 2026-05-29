import { useState, useEffect } from 'react';
import { GetConfig, SaveConfig } from '../../wailsjs/go/main/App';
import { main } from '../../wailsjs/go/models';

export default function Settings() {
  const [config, setConfig] = useState<main.Config | null>(null);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState('');
  const [newDir, setNewDir] = useState('');

  useEffect(() => {
    (async () => {
      try {
        const cfg = await GetConfig();
        setConfig(cfg);
      } catch (err) {
        setError(String(err));
      }
    })();
  }, []);

  const handleSave = async () => {
    if (!config) return;
    setError('');
    setSaved(false);
    try {
      await SaveConfig(config);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (err) {
      setError(String(err));
    }
  };

  const addGameDir = () => {
    if (!config || !newDir.trim()) return;
    setConfig({
      ...config,
      gameDirectories: [...config.gameDirectories, newDir.trim()],
    });
    setNewDir('');
  };

  const removeGameDir = (index: number) => {
    if (!config) return;
    const dirs = [...config.gameDirectories];
    dirs.splice(index, 1);
    setConfig({ ...config, gameDirectories: dirs });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') addGameDir();
  };

  if (!config) {
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
              value={config.machineName}
              onChange={(e) =>
                setConfig({ ...config, machineName: e.target.value })
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
            {config.gameDirectories.length === 0 && (
              <p className="empty-hint">No directories configured. Add one below.</p>
            )}
            {config.gameDirectories.map((dir, i) => (
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
                value={config.maxScanDepth}
                onChange={(e) =>
                  setConfig({ ...config, maxScanDepth: parseInt(e.target.value) })
                }
              />
              <span className="form-value">{config.maxScanDepth}</span>
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
              value={config.language}
              onChange={(e) =>
                setConfig({ ...config, language: e.target.value })
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
                checked={config.vndbEnabled}
                onChange={(e) =>
                  setConfig({ ...config, vndbEnabled: e.target.checked })
                }
              />
              VNDB (Visual Novel Database)
            </label>
          </div>
          <div className="form-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={config.dlsiteEnabled}
                onChange={(e) =>
                  setConfig({ ...config, dlsiteEnabled: e.target.checked })
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
