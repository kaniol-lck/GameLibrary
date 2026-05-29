import { useState, useEffect } from 'react';
import { GetConfig, SaveConfig } from '../../wailsjs/go/main/App';
import { config } from '../../wailsjs/go/models';

interface SourceMeta {
  description: string;
}

const SOURCE_META: Record<string, SourceMeta> = {
  steam:  { description: 'Primary source. Detected from steam_appid.txt in game folders.' },
  vndb:   { description: 'Visual Novel Database. Best for VN titles with Japanese origin.' },
  dlsite: { description: 'Japanese indie/doujin game store. Matches by RJ number.' },
  igdb:   { description: 'Internet Game Database. Large general-purpose game database via Twitch API.' },
};

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

  const updateCfg = (patch: Partial<config.Config>) => {
    if (!cfg) return;
    setCfg(config.Config.createFrom({ ...cfg, ...patch }));
  };

  const addGameDir = () => {
    if (!cfg || !newDir.trim()) return;
    updateCfg({ gameDirectories: [...cfg.gameDirectories, newDir.trim()] });
    setNewDir('');
  };

  const removeGameDir = (index: number) => {
    if (!cfg) return;
    const dirs = [...cfg.gameDirectories];
    dirs.splice(index, 1);
    updateCfg({ gameDirectories: dirs });
  };

  const toggleSource = (index: number) => {
    if (!cfg) return;
    const sources = cfg.metadataSources.map((s, i) =>
      i === index ? config.MetadataSource.createFrom({ ...s, enabled: !s.enabled }) : s
    );
    updateCfg({ metadataSources: sources });
  };

  const moveSource = (index: number, direction: -1 | 1) => {
    if (!cfg) return;
    const sources = [...cfg.metadataSources];
    const target = index + direction;
    if (target < 0 || target >= sources.length) return;
    [sources[index], sources[target]] = [sources[target], sources[index]];
    updateCfg({ metadataSources: sources });
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
        <div className="settings-grid">
          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">&#9881;</span>
              <div>
                <h3>Machine</h3>
                <p className="form-hint">Identify this client for playtime tracking and locks.</p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="form-group">
                <label>Machine Name</label>
                <input
                  type="text"
                  value={cfg.machineName}
                  onChange={(e) => updateCfg({ machineName: e.target.value })}
                  placeholder="Living Room PC"
                />
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">&#128451;</span>
              <div>
                <h3>Game Directories</h3>
                <p className="form-hint">Paths relative to the manager executable.</p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="dir-list">
                {cfg.gameDirectories.length === 0 && (
                  <p className="empty-hint">No directories configured.</p>
                )}
                {cfg.gameDirectories.map((dir: string, i: number) => (
                  <div key={i} className="dir-item">
                    <span className="dir-path">{dir}</span>
                    <button
                      className="btn-icon-sm"
                      onClick={() => removeGameDir(i)}
                      title="Remove"
                    >
                      &times;
                    </button>
                  </div>
                ))}
              </div>
              <div className="dir-add">
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
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">&#128269;</span>
              <div>
                <h3>Scanning</h3>
                <p className="form-hint">Controls how deep the scanner searches for executables.</p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="form-group">
                <div className="form-row">
                  <span className="form-label-sm">Shallow (1)</span>
                  <input
                    type="range"
                    min={1}
                    max={10}
                    value={cfg.maxScanDepth}
                    onChange={(e) => updateCfg({ maxScanDepth: parseInt(e.target.value) })}
                  />
                  <span className="form-label-sm">Deep (10)</span>
                </div>
                <div className="form-row-center">
                  <span className="depth-value">{cfg.maxScanDepth}</span>
                  <span className="form-hint">level{cfg.maxScanDepth > 1 ? 's' : ''} deep</span>
                </div>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">&#127760;</span>
              <div>
                <h3>Language</h3>
                <p className="form-hint">Preferred language for scraped metadata.</p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="form-group">
                <select
                  value={cfg.language}
                  onChange={(e) => updateCfg({ language: e.target.value })}
                >
                  <option value="zh-CN">简体中文</option>
                  <option value="en-US">English</option>
                  <option value="ja-JP">日本語</option>
                </select>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">&#128230;</span>
              <div>
                <h3>Metadata Sources</h3>
                <p className="form-hint">
                  Enable and prioritize metadata providers. Sources are queried in order; the first match wins.
                </p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="source-list">
                {cfg.metadataSources.map((src: config.MetadataSource, i: number) => {
                  const meta = SOURCE_META[src.key] || { description: '' };
                  return (
                    <div
                      key={src.key}
                      className={`source-item ${src.enabled ? '' : 'source-disabled'}`}
                    >
                      <button
                        className={`toggle-switch ${src.enabled ? 'toggle-on' : ''}`}
                        onClick={() => toggleSource(i)}
                        title={src.enabled ? 'Disable' : 'Enable'}
                        role="switch"
                        aria-checked={src.enabled}
                      >
                        <span className="toggle-knob" />
                      </button>

                      <div className="source-info">
                        <span className="source-name">{src.name}</span>
                        <span className="source-desc">{meta.description}</span>
                      </div>

                      <div className="source-order">
                        <button
                          className="btn-order"
                          onClick={() => moveSource(i, -1)}
                          disabled={i === 0}
                          title="Move up"
                        >
                          &#9650;
                        </button>
                        <button
                          className="btn-order"
                          onClick={() => moveSource(i, 1)}
                          disabled={i === cfg.metadataSources.length - 1}
                          title="Move down"
                        >
                          &#9660;
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
