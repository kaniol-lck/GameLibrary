import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { api, type Game, type ScrapeReport } from "@/api/client";
import { coverKey, coverUrl } from "@/lib/cover";
import { errorMessage, formatPlaytime } from "@/lib/format";
import { gamePlatforms, platformMeta, primaryGameUrl } from "@/lib/platform";

export interface GameDetailProps {
  game: Game;
  onClose: () => void;
  onUpdated: () => void | Promise<void>;
  onScrape: (id: string) => Promise<ScrapeReport | null>;
  isScraping: boolean;
}

/** Inline result line shown at the bottom of the panel. */
interface Feedback {
  kind: "ok" | "error";
  text: string;
}

const TITLE_ID = "game-detail-title";
const TAG_INPUT_ID = "game-detail-tag-input";

/**
 * Read-only summary of one game plus the actions the backend exposes for it.
 *
 * Every action goes through `api.*` and then `onUpdated()`, so the panel always
 * renders the parent's freshly fetched game. The previous version mutated the
 * `game` prop in place (`g.starred = !g.starred`), which changed the parent's
 * state object without a re-render and lost the change on the next fetch.
 */
export default function GameDetail({
  game,
  onClose,
  onUpdated,
  onScrape,
  isScraping,
}: GameDetailProps) {
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [pending, setPending] = useState(false);
  const [tagInput, setTagInput] = useState("");
  const [tagInputOpen, setTagInputOpen] = useState(false);
  const [failedCover, setFailedCover] = useState("");
  const dialogRef = useRef<HTMLDivElement>(null);

  const metadata = game.metadata;
  const platforms = gamePlatforms(game);
  const preferredSource = game.preferredSource ?? "";
  const userTags = game.tags ?? [];
  const aliases = game.aliases ?? [];
  const executables = game.executables ?? [];
  const primaryUrl = primaryGameUrl(game);
  const cover = coverUrl(game.id, "landscape", game.coverVersion);
  const coverId = coverKey(game.id, game.coverVersion);
  const busy = pending || isScraping;

  // The dialog is focused on mount so Escape and Tab work before the first click.
  useEffect(() => {
    dialogRef.current?.focus();
  }, []);

  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onClose]);

  /** Runs one backend action, then refreshes the parent from fresh data. */
  const runAction = useCallback(
    async (action: () => Promise<void>, successMessage?: string) => {
      setPending(true);
      setFeedback(null);
      try {
        await action();
        await onUpdated();
        if (successMessage) {
          setFeedback({ kind: "ok", text: successMessage });
        }
      } catch (err) {
        setFeedback({ kind: "error", text: errorMessage(err) });
      } finally {
        setPending(false);
      }
    },
    [onUpdated],
  );

  const handleLaunch = () => runAction(() => api.launch(game.id));
  const handleToggleStar = () => runAction(() => api.toggleStar(game.id));
  const handleRemoveTag = (tag: string) => runAction(() => api.removeTag(game.id, tag));
  const handleSetPreferred = (source: string) =>
    runAction(() => api.setPreferredSource(game.id, source));
  const handleSetPrimary = (path: string) =>
    runAction(() => api.setPrimaryExecutable(game.id, path));
  const handleOpenFolder = () => runAction(() => api.openGameDirectory(game.id));
  const handleOpenMetadata = () => runAction(() => api.openGameMetadata(game.id));
  const handleOpenPage = (url: string) => runAction(() => api.openBrowser(url));

  const handleAddTag = () => {
    const tag = tagInput.trim();
    setTagInput("");
    setTagInputOpen(false);
    if (tag === "") {
      return;
    }
    void runAction(() => api.addTag(game.id, tag));
  };

  const handleScrape = async () => {
    setPending(true);
    setFeedback(null);
    try {
      const report = await onScrape(game.id);
      if (report === null) {
        // The hook records the underlying error in the app-level banner.
        setFeedback({ kind: "error", text: "Scrape failed. See the error banner for details." });
        return;
      }
      if (report.error) {
        setFeedback({ kind: "error", text: report.error });
        return;
      }
      const sources = report.sources?.length ? report.sources.join(", ") : (report.source ?? "");
      setFeedback({
        kind: "ok",
        text: sources === "" ? "Metadata updated." : `Metadata updated from ${sources}.`,
      });
      await onUpdated();
    } catch (err) {
      setFeedback({ kind: "error", text: errorMessage(err) });
    } finally {
      setPending(false);
    }
  };

  const handleExecutableKeyDown = (event: ReactKeyboardEvent<HTMLLIElement>, path: string) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      void handleSetPrimary(path);
    }
  };

  return (
    <div className="detail-overlay" onClick={onClose}>
      <div
        className="detail-dialog"
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={TITLE_ID}
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          className="detail-close"
          onClick={onClose}
          aria-label="Close details"
          title="Close"
        >
          {"\u00D7"}
        </button>

        <div className="detail-cover">
          {failedCover === coverId ? (
            <div className="game-card-cover-placeholder">
              <span>{game.title.charAt(0).toUpperCase()}</span>
            </div>
          ) : (
            <img src={cover} alt={game.title} onError={() => setFailedCover(coverId)} />
          )}
        </div>

        <div className="detail-body">
          <div className="detail-header-row">
            <h2 className="detail-title" id={TITLE_ID}>
              {game.title}
            </h2>
            <button
              type="button"
              className="detail-star-btn"
              onClick={() => void handleToggleStar()}
              title={game.starred === true ? "Unstar" : "Star"}
              aria-label={game.starred === true ? "Remove star" : "Add star"}
              aria-pressed={game.starred === true}
              disabled={busy}
            >
              {game.starred === true ? "\u2605" : "\u2606"}
            </button>
          </div>
          {game.titleNative && <p className="detail-title-native">{game.titleNative}</p>}

          <div className="detail-meta-row">
            <div className="detail-platforms">
              {platforms.map((platform) => {
                const meta = platformMeta(platform.platform);
                const label = meta.label || platform.platform;
                const url = meta.url(platform.id ?? "");
                if (url === "") {
                  return (
                    <span
                      key={platform.platform}
                      className="detail-platform-tag"
                      style={{ backgroundColor: meta.color }}
                    >
                      {label}
                    </span>
                  );
                }
                return (
                  <a
                    key={platform.platform}
                    className="detail-platform-tag detail-platform-link"
                    href={url}
                    style={{ backgroundColor: meta.color }}
                    title={`Open ${label} page`}
                    onClick={(event) => {
                      // The webview must not navigate away from the app.
                      event.preventDefault();
                      event.stopPropagation();
                      void handleOpenPage(url);
                    }}
                  >
                    {label}
                    <span className="detail-plat-arrow">{"\u2197"}</span>
                  </a>
                );
              })}
            </div>
            {game.totalPlaytime > 0 && (
              <span className="detail-meta-text">{formatPlaytime(game.totalPlaytime)}</span>
            )}
          </div>

          <button
            type="button"
            className="btn btn-launch btn-launch-lg"
            onClick={() => void handleLaunch()}
            disabled={executables.length === 0 || busy}
          >
            {"\u25B6"} Launch Game
          </button>

          {(metadata?.developer || metadata?.publisher) && (
            <div className="detail-section">
              {metadata.developer && (
                <div className="detail-field">
                  <label>Developer</label>
                  <span>{metadata.developer}</span>
                </div>
              )}
              {metadata.publisher && (
                <div className="detail-field">
                  <label>Publisher</label>
                  <span>{metadata.publisher}</span>
                </div>
              )}
            </div>
          )}

          {metadata?.releaseDate && (
            <div className="detail-section">
              <div className="detail-field">
                <label>Release Date</label>
                <span>{metadata.releaseDate}</span>
              </div>
            </div>
          )}

          {metadata?.description && (
            <div className="detail-section">
              <label>Description</label>
              <p className="detail-desc">{metadata.description}</p>
            </div>
          )}

          <div className="detail-section">
            <label htmlFor={TAG_INPUT_ID}>Tags</label>
            <div className="detail-tags-wrap">
              {(metadata?.tags ?? []).map((tag) => (
                <span key={tag} className="detail-tag detail-tag-genre">
                  {tag}
                </span>
              ))}
              {userTags.map((tag) => (
                <span key={tag} className="detail-tag detail-tag-user">
                  #{tag}
                  <button
                    type="button"
                    className="detail-tag-remove"
                    onClick={() => void handleRemoveTag(tag)}
                    aria-label={`Remove tag ${tag}`}
                    disabled={busy}
                  >
                    {"\u00D7"}
                  </button>
                </span>
              ))}
              {tagInputOpen ? (
                <div className="context-tag-input detail-tag-inline">
                  <input
                    id={TAG_INPUT_ID}
                    type="text"
                    placeholder="tag…"
                    value={tagInput}
                    autoFocus
                    aria-label="New tag"
                    onChange={(event) => setTagInput(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        handleAddTag();
                      }
                      if (event.key === "Escape") {
                        setTagInputOpen(false);
                        setTagInput("");
                      }
                    }}
                  />
                  <button type="button" onClick={handleAddTag}>
                    Add
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  className="detail-tag detail-tag-add"
                  onClick={() => setTagInputOpen(true)}
                  aria-label="Add tag"
                  disabled={busy}
                >
                  +
                </button>
              )}
            </div>
          </div>

          {platforms.length > 1 && (
            <div className="detail-section">
              <label>Preferred Source</label>
              <div className="detail-pref-row">
                {platforms.map((platform) => (
                  <button
                    key={platform.platform}
                    type="button"
                    className={`detail-pref-btn${
                      platform.platform === preferredSource ? " detail-pref-active" : ""
                    }`}
                    onClick={() => void handleSetPreferred(platform.platform)}
                    aria-pressed={platform.platform === preferredSource}
                    disabled={busy}
                  >
                    {platform.platform === preferredSource ? "\u25C9" : "\u25CB"}{" "}
                    {platformMeta(platform.platform).label || platform.platform}
                  </button>
                ))}
              </div>
            </div>
          )}

          {aliases.length > 0 && (
            <div className="detail-section">
              <label>Aliases</label>
              <div className="detail-tags-wrap">
                {aliases.map((alias) => (
                  <span key={alias} className="detail-tag detail-tag-alias">
                    {alias}
                  </span>
                ))}
              </div>
            </div>
          )}

          <div className="detail-section">
            <label>Executables</label>
            <ul className="detail-exe-list">
              {executables.map((executable) => (
                <li
                  key={executable.path}
                  className="detail-exe-item"
                  role="button"
                  tabIndex={0}
                  aria-pressed={executable.primary === true}
                  title={`Use ${executable.name}.exe as the primary executable`}
                  onClick={() => void handleSetPrimary(executable.path)}
                  onKeyDown={(event) => handleExecutableKeyDown(event, executable.path)}
                >
                  <span className="detail-exe-radio">
                    {executable.primary === true ? "\u25C9" : "\u25CB"}
                  </span>
                  <span>{executable.name}.exe</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="detail-section detail-bottom-actions">
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => void handleScrape()}
              disabled={busy}
            >
              {isScraping ? "Scraping…" : "\u21BB Re-scrape Metadata"}
            </button>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() => void handleOpenFolder()}
              disabled={busy}
            >
              {"\uD83D\uDCC1"} Open Folder
            </button>
            <button
              type="button"
              className="btn btn-ghost-sm"
              onClick={() => void handleOpenMetadata()}
              disabled={busy}
            >
              {"\uD83D\uDCC4"} Metadata
            </button>
            {primaryUrl !== "" && (
              <button
                type="button"
                className="btn btn-ghost-sm"
                onClick={() => void handleOpenPage(primaryUrl)}
                disabled={busy}
              >
                {"\uD83C\uDF10"} Open Page
              </button>
            )}
          </div>

          {feedback && (
            <div
              className={`scrape-result ${feedback.kind === "ok" ? "scrape-ok" : "scrape-err"}`}
              role={feedback.kind === "ok" ? "status" : "alert"}
            >
              {feedback.text}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
