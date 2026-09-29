import { useEffect, useState } from "react";
import { api, type AppConfig } from "@/api/client";
import { errorMessage } from "@/lib/format";
import { config as configModels } from "../../wailsjs/go/models";

interface SourceSettingMeta {
  key: string;
  label: string;
  placeholder: string;
  type: "text" | "password";
}

interface SourceMeta {
  description: string;
  settings: SourceSettingMeta[];
}

/**
 * Presentation metadata for the providers the backend registers.
 *
 * `igdb` is gone (no IGDB scraper is registered) and `rawg` is described here:
 * the backend enables RAWG by default, but it used to render as an empty row
 * with no explanation and no way to enter its required API key.
 */
const SOURCE_META: Record<string, SourceMeta> = {
  steam: {
    description: "Primary source. Detected from steam_appid.txt in game folders.",
    settings: [
      {
        key: "apiKey",
        label: "Steam Web API Key",
        placeholder: "Optional: for higher rate limits",
        type: "password",
      },
    ],
  },
  vndb: {
    description: "Visual Novel Database. Best for VN titles with Japanese origin.",
    settings: [],
  },
  bangumi: {
    description: "Bangumi (bgm.tv). Chinese anime/game database with Chinese titles.",
    settings: [],
  },
  dlsite: {
    description: "Japanese indie/doujin game store. Matches by RJ number in folder name.",
    settings: [],
  },
  rawg: {
    description: "RAWG.io. Broad western catalogue. Needs a free API key before it can match.",
    settings: [
      {
        key: "apiKey",
        label: "RAWG API Key",
        placeholder: "Required: get one from rawg.io/apidocs",
        type: "password",
      },
    ],
  },
  steamgriddb: {
    description: "SteamGridDB. High-quality cover art library. Requires an API key.",
    settings: [
      {
        key: "apiKey",
        label: "SteamGridDB API Key",
        placeholder: "Required: get from steamgriddb.com/profile/preferences/api",
        type: "password",
      },
    ],
  },
};

const LOG_LEVELS = ["debug", "info", "warn", "error"] as const;

const LANGUAGES = [
  { value: "zh-CN", label: "Simplified Chinese" },
  { value: "en-US", label: "English" },
  { value: "ja-JP", label: "Japanese" },
];

const MAX_SCAN_DEPTH = 10;
/** Mirrors config.maxScrapeConcurrency in the backend. */
const MAX_SCRAPE_CONCURRENCY = 16;
/** Mirrors config.DefaultScrapeConcurrency; used while the draft has no value. */
const DEFAULT_SCRAPE_CONCURRENCY = 4;
const DEBOUNCE_MIN_MS = 50;
const DEBOUNCE_MAX_MS = 2000;
const DEBOUNCE_STEP_MS = 50;

/** Copies a config so edits never touch the object the parent is holding. */
function cloneConfig(source: AppConfig): AppConfig {
  return configModels.Config.createFrom({
    ...source,
    gameDirectories: [...source.gameDirectories],
    gameDirectoryLabels: { ...(source.gameDirectoryLabels ?? {}) },
    metadataSources: source.metadataSources.map((metadataSource) => ({
      ...metadataSource,
      settings: { ...(metadataSource.settings ?? {}) },
    })),
  });
}

/** Guesses whether a directory belongs to Steam, to pre-fill its folder label. */
function isSteamPath(path: string): boolean {
  const lower = path.toLowerCase();
  return (
    lower.includes("steamlibrary") || lower.includes("steamapps") || lower.includes("steam\\common")
  );
}

export interface SettingsProps {
  config: AppConfig | null;
  machineName: string;
  logDir: string;
  onConfigChanged: () => void | Promise<void>;
}

export default function Settings({ config, machineName, logDir, onConfigChanged }: SettingsProps) {
  const [draft, setDraft] = useState<AppConfig | null>(() => (config ? cloneConfig(config) : null));
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [saving, setSaving] = useState(false);
  const [expandedSource, setExpandedSource] = useState<string | null>(null);
  const [labelDir, setLabelDir] = useState<string | null>(null);
  const [labelInput, setLabelInput] = useState("");

  // The parent owns the config object. Edits happen on a copy, and a new snapshot
  // from the parent (after a save) replaces the copy, so the two never diverge
  // until the app is restarted.
  useEffect(() => {
    setDraft(config ? cloneConfig(config) : null);
  }, [config]);

  const updateDraft = (patch: Partial<AppConfig>) => {
    setNotice("");
    setDraft((current) =>
      current ? configModels.Config.createFrom({ ...current, ...patch }) : current,
    );
  };

  const handleSave = async () => {
    if (!draft) {
      return;
    }
    setSaving(true);
    setError("");
    setNotice("");
    try {
      await api.saveConfig(draft);
      // Re-reading the config also refreshes the path labels and the library.
      await onConfigChanged();
      setNotice("Settings saved.");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const openDirectory = async (dir: string) => {
    setError("");
    try {
      await api.openDirectory(dir);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const browseDirectory = async () => {
    if (!draft) {
      return;
    }
    setError("");
    try {
      const path = await api.pickGameDirectory();
      if (path === "") {
        // The picker was cancelled; nothing to report.
        return;
      }
      if (draft.gameDirectories.includes(path)) {
        setError("That directory is already in the list.");
        return;
      }
      const labels = { ...(draft.gameDirectoryLabels ?? {}) };
      const existing = labels[path] ?? [];
      if (isSteamPath(path) && !existing.some((label) => label.toLowerCase() === "steam")) {
        labels[path] = [...existing, "Steam"];
      }
      updateDraft({
        gameDirectories: [...draft.gameDirectories, path],
        gameDirectoryLabels: labels,
      });
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const removeDirectory = (dir: string) => {
    if (!draft) {
      return;
    }
    const labels = { ...(draft.gameDirectoryLabels ?? {}) };
    delete labels[dir];
    updateDraft({
      gameDirectories: draft.gameDirectories.filter((entry) => entry !== dir),
      gameDirectoryLabels: labels,
    });
  };

  const toggleDirectoryLabel = (dir: string, label: string) => {
    if (!draft) {
      return;
    }
    const labels = { ...(draft.gameDirectoryLabels ?? {}) };
    const current = labels[dir] ?? [];
    const next = current.includes(label)
      ? current.filter((entry) => entry !== label)
      : [...current, label];
    if (next.length === 0) {
      delete labels[dir];
    } else {
      labels[dir] = next;
    }
    updateDraft({ gameDirectoryLabels: labels });
  };

  const addDirectoryLabel = (dir: string) => {
    const label = labelInput.trim();
    setLabelDir(null);
    setLabelInput("");
    if (label === "") {
      return;
    }
    if (!(draft?.gameDirectoryLabels?.[dir] ?? []).includes(label)) {
      toggleDirectoryLabel(dir, label);
    }
  };

  const toggleSource = (key: string) => {
    if (!draft) {
      return;
    }
    updateDraft({
      metadataSources: draft.metadataSources.map((source) =>
        source.key === key
          ? configModels.MetadataSource.createFrom({ ...source, enabled: !source.enabled })
          : source,
      ),
    });
  };

  const moveSource = (key: string, direction: -1 | 1) => {
    if (!draft) {
      return;
    }
    const sources = [...draft.metadataSources];
    const index = sources.findIndex((source) => source.key === key);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= sources.length) {
      return;
    }
    const moving = sources[index];
    const displaced = sources[target];
    sources[index] = displaced;
    sources[target] = moving;
    updateDraft({ metadataSources: sources });
  };

  const updateSourceSetting = (sourceKey: string, settingKey: string, value: string) => {
    if (!draft) {
      return;
    }
    updateDraft({
      metadataSources: draft.metadataSources.map((source) => {
        if (source.key !== sourceKey) {
          return source;
        }
        const settings: Record<string, string> = { ...(source.settings ?? {}) };
        if (value === "") {
          delete settings[settingKey];
        } else {
          settings[settingKey] = value;
        }
        return configModels.MetadataSource.createFrom({ ...source, settings });
      }),
    });
  };

  if (!draft) {
    return (
      <div className="settings-loading">
        <p>Loading settings…</p>
      </div>
    );
  }

  const debounceMs = draft.watcherDebounceMs || 100;
  // The model marks this optional, so fall back to the backend's default rather
  // than rendering an empty control.
  const scrapeConcurrency = draft.scrapeConcurrency || DEFAULT_SCRAPE_CONCURRENCY;

  return (
    <div className="settings-panel">
      <div className="settings-panel-header">
        <h2>Settings</h2>
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => void handleSave()}
          disabled={saving}
        >
          {saving ? "Saving…" : "Save"}
        </button>
      </div>

      {error !== "" && (
        <div className="alert alert-error" role="alert">
          {error}
          <button
            type="button"
            onClick={() => setError("")}
            className="alert-close"
            aria-label="Dismiss error"
          >
            {"\u00D7"}
          </button>
        </div>
      )}

      {notice !== "" && (
        <div className="alert alert-success" role="status">
          {notice}
        </div>
      )}

      <div className="settings-content">
        <div className="settings-grid">
          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83D\uDDC4"}</span>
              <div>
                <h3>Game Directories</h3>
                <p className="form-hint">
                  Paths relative to the manager. Use Browse to pick from this machine&apos;s file
                  system.
                </p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="dir-list">
                {draft.gameDirectories.length === 0 && (
                  <p className="empty-hint">No directories configured.</p>
                )}
                {draft.gameDirectories.map((dir) => {
                  const labels = draft.gameDirectoryLabels?.[dir] ?? [];
                  return (
                    <div key={dir} className="dir-item">
                      <button
                        type="button"
                        className="btn-icon-sm dir-open-btn"
                        onClick={() => void openDirectory(dir)}
                        title="Open in Explorer"
                        aria-label={`Open ${dir}`}
                      >
                        {"\uD83D\uDCC1"}
                      </button>
                      <div className="dir-info">
                        <span
                          className="dir-path"
                          onClick={() => void openDirectory(dir)}
                          title="Click to open"
                        >
                          {dir}
                        </span>
                        <div className="dir-tags">
                          {labels.map((label) => (
                            <span key={label} className="dir-tag-chip">
                              {label}
                              <button
                                type="button"
                                className="context-tag-remove"
                                onClick={() => toggleDirectoryLabel(dir, label)}
                                title="Remove label"
                                aria-label={`Remove label ${label} from ${dir}`}
                              >
                                {"\u00D7"}
                              </button>
                            </span>
                          ))}
                          {labelDir === dir ? (
                            <div className="context-tag-input dir-tag-inline">
                              <input
                                autoFocus
                                type="text"
                                placeholder="label…"
                                value={labelInput}
                                aria-label={`New label for ${dir}`}
                                onChange={(event) => setLabelInput(event.target.value)}
                                onKeyDown={(event) => {
                                  if (event.key === "Enter") {
                                    addDirectoryLabel(dir);
                                  }
                                  if (event.key === "Escape") {
                                    setLabelDir(null);
                                    setLabelInput("");
                                  }
                                }}
                              />
                              <button type="button" onClick={() => addDirectoryLabel(dir)}>
                                Add
                              </button>
                            </div>
                          ) : (
                            <button
                              type="button"
                              className="dir-tag-chip dir-tag-add"
                              onClick={() => {
                                setLabelDir(dir);
                                setLabelInput("");
                              }}
                              aria-label={`Add a label to ${dir}`}
                            >
                              +
                            </button>
                          )}
                        </div>
                      </div>
                      <button
                        type="button"
                        className="btn-icon-sm"
                        onClick={() => removeDirectory(dir)}
                        title="Remove"
                        aria-label={`Remove ${dir}`}
                      >
                        {"\u00D7"}
                      </button>
                    </div>
                  );
                })}
              </div>
              <div className="dir-add">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => void browseDirectory()}
                >
                  Browse…
                </button>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83D\uDD0D"}</span>
              <div>
                <h3>Scanning</h3>
                <p className="form-hint">Controls how deep the scanner searches for executables.</p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="form-group">
                <label htmlFor="settings-max-scan-depth">Maximum Scan Depth</label>
                <div className="form-row">
                  <span className="form-label-sm">Shallow (1)</span>
                  <input
                    id="settings-max-scan-depth"
                    type="range"
                    min={1}
                    max={MAX_SCAN_DEPTH}
                    value={draft.maxScanDepth}
                    onChange={(event) =>
                      updateDraft({ maxScanDepth: Number.parseInt(event.target.value, 10) })
                    }
                  />
                  <span className="form-label-sm">Deep ({MAX_SCAN_DEPTH})</span>
                </div>
                <div className="form-row-center">
                  <span className="depth-value">{draft.maxScanDepth}</span>
                  <span className="form-hint">level{draft.maxScanDepth > 1 ? "s" : ""} deep</span>
                </div>
              </div>

              <div className="form-group">
                <label htmlFor="settings-scrape-concurrency">Parallel Scrapes</label>
                <div className="form-row">
                  <span className="form-label-sm">One at a time (1)</span>
                  <input
                    id="settings-scrape-concurrency"
                    type="range"
                    min={1}
                    max={MAX_SCRAPE_CONCURRENCY}
                    value={scrapeConcurrency}
                    onChange={(event) =>
                      updateDraft({ scrapeConcurrency: Number.parseInt(event.target.value, 10) })
                    }
                  />
                  <span className="form-label-sm">Many ({MAX_SCRAPE_CONCURRENCY})</span>
                </div>
                <div className="form-row-center">
                  <span className="depth-value">{scrapeConcurrency}</span>
                  <span className="form-hint">
                    game{scrapeConcurrency > 1 ? "s" : ""} scraped at once
                  </span>
                </div>
                <p className="form-hint">
                  Each metadata service is still rate limited, so this overlaps the waiting rather
                  than raising the request rate any one service sees.
                </p>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83C\uDF10"}</span>
              <div>
                <h3>Language</h3>
                <p className="form-hint">
                  Preferred language for scraped metadata. Affects Steam, RAWG and VNDB results.
                </p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="form-group">
                <label htmlFor="settings-language">Metadata Language</label>
                <select
                  id="settings-language"
                  value={draft.language}
                  onChange={(event) => updateDraft({ language: event.target.value })}
                >
                  {LANGUAGES.map((language) => (
                    <option key={language.value} value={language.value}>
                      {language.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83D\uDCE6"}</span>
              <div>
                <h3>Metadata Sources</h3>
                <p className="form-hint">
                  Enable and prioritize providers. Click a source to configure its settings.
                </p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="source-list">
                {draft.metadataSources.map((source, index) => {
                  const meta = SOURCE_META[source.key] ?? {
                    description: "No description available for this provider.",
                    settings: [],
                  };
                  const isExpanded = expandedSource === source.key;
                  const hasSettings = meta.settings.length > 0;
                  return (
                    <div key={source.key}>
                      <div className={`source-item ${source.enabled ? "" : "source-disabled"}`}>
                        <button
                          type="button"
                          className={`toggle-switch ${source.enabled ? "toggle-on" : ""}`}
                          onClick={() => toggleSource(source.key)}
                          role="switch"
                          aria-checked={source.enabled === true}
                          aria-label={`Enable ${source.name}`}
                        >
                          <span className="toggle-knob" />
                        </button>

                        <div
                          className="source-info"
                          role={hasSettings ? "button" : undefined}
                          tabIndex={hasSettings ? 0 : undefined}
                          aria-expanded={hasSettings ? isExpanded : undefined}
                          onClick={() => {
                            if (hasSettings) {
                              setExpandedSource(isExpanded ? null : source.key);
                            }
                          }}
                          onKeyDown={(event) => {
                            if (hasSettings && (event.key === "Enter" || event.key === " ")) {
                              event.preventDefault();
                              setExpandedSource(isExpanded ? null : source.key);
                            }
                          }}
                          style={{ cursor: hasSettings ? "pointer" : "default" }}
                        >
                          <span className="source-name">
                            {source.name}
                            {hasSettings && (
                              <span className="source-settings-icon">
                                {isExpanded ? " ▾" : " ▸"}
                              </span>
                            )}
                          </span>
                          <span className="source-desc">{meta.description}</span>
                        </div>

                        <div className="source-order">
                          <button
                            type="button"
                            className="btn-order"
                            onClick={() => moveSource(source.key, -1)}
                            disabled={index === 0}
                            title="Move up"
                            aria-label={`Move ${source.name} up`}
                          >
                            {"\u25B2"}
                          </button>
                          <button
                            type="button"
                            className="btn-order"
                            onClick={() => moveSource(source.key, 1)}
                            disabled={index === draft.metadataSources.length - 1}
                            title="Move down"
                            aria-label={`Move ${source.name} down`}
                          >
                            {"\u25BC"}
                          </button>
                        </div>
                      </div>

                      {isExpanded && hasSettings && (
                        <div className="source-settings-panel">
                          {meta.settings.map((field) => {
                            const inputId = `settings-source-${source.key}-${field.key}`;
                            return (
                              <div key={field.key} className="form-group">
                                <label htmlFor={inputId}>{field.label}</label>
                                <input
                                  id={inputId}
                                  type={field.type}
                                  value={(source.settings ?? {})[field.key] ?? ""}
                                  placeholder={field.placeholder}
                                  onChange={(event) =>
                                    updateSourceSetting(source.key, field.key, event.target.value)
                                  }
                                />
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83D\uDC41"}</span>
              <div>
                <h3>File Watcher</h3>
                <p className="form-hint">Auto-detect new and removed games in real-time.</p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="source-item">
                <button
                  type="button"
                  className={`toggle-switch ${draft.watcherEnabled ? "toggle-on" : ""}`}
                  onClick={() => updateDraft({ watcherEnabled: !draft.watcherEnabled })}
                  role="switch"
                  aria-checked={draft.watcherEnabled === true}
                  aria-label="Enable the file watcher"
                >
                  <span className="toggle-knob" />
                </button>
                <div className="source-info">
                  <span className="source-name">Enable File Watcher</span>
                  <span className="source-desc">
                    Watches game directories for new/deleted games
                  </span>
                </div>
              </div>
              <div className="form-group" style={{ marginTop: 10 }}>
                <label htmlFor="settings-watcher-debounce">Debounce: {debounceMs} ms</label>
                <div className="form-row">
                  <span className="form-label-sm">Fast ({DEBOUNCE_MIN_MS})</span>
                  <input
                    id="settings-watcher-debounce"
                    type="range"
                    min={DEBOUNCE_MIN_MS}
                    max={DEBOUNCE_MAX_MS}
                    step={DEBOUNCE_STEP_MS}
                    value={debounceMs}
                    onChange={(event) =>
                      updateDraft({ watcherDebounceMs: Number.parseInt(event.target.value, 10) })
                    }
                  />
                  <span className="form-label-sm">Slow ({DEBOUNCE_MAX_MS})</span>
                </div>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83D\uDCDC"}</span>
              <div>
                <h3>Logging</h3>
                <p className="form-hint">
                  Verbosity of the application log. &quot;Log to library&quot; writes it next to the
                  executable instead of the default log directory.
                </p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="form-group">
                <label htmlFor="settings-log-level">Log Level</label>
                <select
                  id="settings-log-level"
                  value={draft.logLevel ?? "info"}
                  onChange={(event) => updateDraft({ logLevel: event.target.value })}
                >
                  {LOG_LEVELS.map((level) => (
                    <option key={level} value={level}>
                      {level}
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-group">
                <div className="source-item">
                  <input
                    id="settings-log-to-library"
                    type="checkbox"
                    style={{ accentColor: "#4f6ef7" }}
                    checked={draft.logToLibrary === true}
                    onChange={(event) => updateDraft({ logToLibrary: event.target.checked })}
                  />
                  <div className="source-info">
                    <label className="source-name" htmlFor="settings-log-to-library">
                      Log to Library
                    </label>
                    <span className="source-desc">
                      Write the log next to the executable on the share
                    </span>
                  </div>
                </div>
              </div>
              <div className="about-info">
                <div className="about-row">
                  <span className="about-label">Current Log Directory</span>
                  <span className="about-value">{logDir || "unknown"}</span>
                </div>
              </div>
            </div>
          </section>

          <section className="settings-card">
            <div className="settings-card-header">
              <span className="settings-card-icon">{"\uD83D\uDCBB"}</span>
              <div>
                <h3>About</h3>
                <p className="form-hint">
                  Machine name is auto-detected from your system hostname.
                </p>
              </div>
            </div>
            <div className="settings-card-body">
              <div className="about-info">
                <div className="about-row">
                  <span className="about-label">Machine Name</span>
                  <span className="about-value">{machineName || "unknown"}</span>
                </div>
                <div className="about-row">
                  <span className="about-label">Log Directory</span>
                  <span className="about-value">{logDir || "unknown"}</span>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
