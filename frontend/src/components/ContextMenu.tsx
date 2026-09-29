import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { api, type Game, type ScrapeReport } from "@/api/client";
import { errorMessage } from "@/lib/format";
import { gamePlatforms, platformMeta, primaryGameUrl } from "@/lib/platform";

export interface ContextMenuProps {
  game: Game;
  x: number;
  y: number;
  onClose: () => void;
  onScrape: (id: string) => Promise<ScrapeReport | null>;
  onUpdated: () => void | Promise<void>;
}

/** Assumed submenu width, used to decide which side a submenu opens on. */
const SUBMENU_WIDTH = 190;
/** Gap kept between the menu and the window edge. */
const VIEWPORT_MARGIN = 4;
/** How long a submenu survives the pointer leaving it. */
const HOVER_CLOSE_DELAY_MS = 200;

/**
 * Right-click menu for one game.
 *
 * Every action calls `api.*` and then `onUpdated()`; failures are shown inside
 * the menu because the menu previously closed on click and the error was lost.
 */
export default function ContextMenu({
  game,
  x,
  y,
  onClose,
  onScrape,
  onUpdated,
}: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [error, setError] = useState("");
  const [showTagInput, setShowTagInput] = useState(false);
  const [tagInput, setTagInput] = useState("");
  const [hoverSubMenu, setHoverSubMenu] = useState("");
  const [placement, setPlacement] = useState({ left: x, top: y, flip: false });

  // Measured before the first paint, so the menu never opens half off-screen,
  // and re-measured on resize so it cannot stay at a stale position.
  useLayoutEffect(() => {
    const place = () => {
      const menu = menuRef.current;
      const width = menu?.offsetWidth ?? 220;
      const height = menu?.offsetHeight ?? 320;
      const left = Math.max(
        VIEWPORT_MARGIN,
        Math.min(x, window.innerWidth - width - VIEWPORT_MARGIN),
      );
      const top = Math.max(
        VIEWPORT_MARGIN,
        Math.min(y, window.innerHeight - height - VIEWPORT_MARGIN),
      );
      setPlacement({ left, top, flip: left + width + SUBMENU_WIDTH > window.innerWidth });
    };
    place();
    window.addEventListener("resize", place);
    return () => window.removeEventListener("resize", place);
  }, [x, y, game.id]);

  // The pending submenu-close timer used to keep running after unmount.
  useEffect(
    () => () => {
      if (closeTimer.current !== null) {
        clearTimeout(closeTimer.current);
      }
    },
    [],
  );

  const handleClickOutside = useCallback(
    (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [handleClickOutside]);

  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onClose]);

  /** Runs one backend action, refreshing the parent and keeping errors visible. */
  const runAction = useCallback(
    async (action: () => Promise<void>, closeAfter = false) => {
      setError("");
      try {
        await action();
        await onUpdated();
        if (closeAfter) {
          onClose();
        }
      } catch (err) {
        setError(errorMessage(err));
      }
    },
    [onClose, onUpdated],
  );

  const handleScrape = async () => {
    setError("");
    try {
      const report = await onScrape(game.id);
      if (report === null) {
        setError("Scrape failed. See the error banner for details.");
        return;
      }
      if (report.error) {
        setError(report.error);
        return;
      }
      await onUpdated();
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const handleAddTag = () => {
    const tag = tagInput.trim();
    setTagInput("");
    setShowTagInput(false);
    if (tag === "") {
      return;
    }
    void runAction(() => api.addTag(game.id, tag));
  };

  const handleEnter = (key: string) => {
    if (closeTimer.current !== null) {
      clearTimeout(closeTimer.current);
    }
    setHoverSubMenu(key);
  };

  const handleLeave = () => {
    if (closeTimer.current !== null) {
      clearTimeout(closeTimer.current);
    }
    closeTimer.current = setTimeout(() => setHoverSubMenu(""), HOVER_CLOSE_DELAY_MS);
  };

  const platforms = gamePlatforms(game);
  const preferredSource = game.preferredSource ?? "";
  const executables = game.executables ?? [];
  const userTags = game.tags ?? [];
  const savePath = game.savePaths?.[0]?.path ?? "";

  const pageLinks = useMemo(() => {
    const links = gamePlatforms(game)
      .map((platform) => {
        const meta = platformMeta(platform.platform);
        return {
          key: platform.platform,
          label: meta.label || platform.platform,
          icon: meta.icon,
          url: meta.url(platform.id ?? ""),
        };
      })
      .filter((link) => link.url !== "");
    if (links.length === 0) {
      // No per-platform page: fall back to the metadata links when there are any.
      const url = primaryGameUrl(game);
      if (url !== "") {
        links.push({ key: "primary", label: "Open Web Page", icon: "\uD83D\uDD17", url });
      }
    }
    return links;
  }, [game]);

  const submenuClass = `ctx-submenu${placement.flip ? " ctx-sub-left" : ""}`;

  return (
    <div
      ref={menuRef}
      className="context-menu"
      style={{ left: placement.left, top: placement.top }}
    >
      <div className="context-menu-section">
        <div className="context-menu-title">{game.title}</div>
      </div>

      {error !== "" && (
        <div className="context-menu-error" role="alert">
          {error}
        </div>
      )}

      {executables.length > 0 && (
        <button
          type="button"
          className="context-item context-launch"
          onClick={() => void runAction(() => api.launch(game.id), true)}
        >
          <span className="context-item-icon">{"\u25B6"}</span>
          <span>Launch Game</span>
        </button>
      )}

      <button
        type="button"
        className="context-item"
        onClick={() => void runAction(() => api.toggleStar(game.id), true)}
      >
        <span className="context-item-icon">{game.starred === true ? "\u2605" : "\u2606"}</span>
        <span>{game.starred === true ? "Unstar" : "Star"}</span>
      </button>

      <div className="context-divider" />

      <button type="button" className="context-item" onClick={() => void handleScrape()}>
        <span className="context-item-icon">{"\u21BB"}</span>
        <span>Re-scrape Metadata</span>
      </button>

      <div className="context-divider" />

      {showTagInput ? (
        <div className="context-tag-input">
          <input
            autoFocus
            type="text"
            placeholder="Tag name…"
            value={tagInput}
            aria-label="New tag"
            onChange={(event) => setTagInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleAddTag();
              }
              if (event.key === "Escape") {
                setShowTagInput(false);
                setTagInput("");
              }
            }}
          />
          <button type="button" onClick={handleAddTag}>
            Add
          </button>
        </div>
      ) : (
        <button type="button" className="context-item" onClick={() => setShowTagInput(true)}>
          <span className="context-item-icon">+</span>
          <span>Add Tag</span>
        </button>
      )}

      {userTags.length > 0 && (
        <div className="context-tags">
          {userTags.map((tag) => (
            <span key={tag} className="context-tag-chip">
              {tag}
              <button
                type="button"
                className="context-tag-remove"
                onClick={() => void runAction(() => api.removeTag(game.id, tag))}
                aria-label={`Remove tag ${tag}`}
                title="Remove tag"
              >
                {"\u00D7"}
              </button>
            </span>
          ))}
        </div>
      )}

      {pageLinks.length > 0 && (
        <>
          <div className="context-divider" />
          <div
            className="ctx-parent"
            onMouseEnter={() => handleEnter("pages")}
            onMouseLeave={handleLeave}
          >
            <div className="context-item">
              <span className="context-item-icon">{"\uD83D\uDD17"}</span>
              <span>Open Web Page</span>
              <span className="ctx-arrow">{"\u25B8"}</span>
            </div>
            {hoverSubMenu === "pages" && (
              <div
                className={submenuClass}
                onMouseEnter={() => handleEnter("pages")}
                onMouseLeave={handleLeave}
              >
                {pageLinks.map((link) => (
                  <button
                    key={link.key}
                    type="button"
                    className="context-item"
                    onClick={() => void runAction(() => api.openBrowser(link.url))}
                  >
                    <span className="context-item-icon">{link.icon}</span>
                    <span>{link.label}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </>
      )}

      {platforms.length > 1 && (
        <div
          className="ctx-parent"
          onMouseEnter={() => handleEnter("pref")}
          onMouseLeave={handleLeave}
        >
          <div className="context-item">
            <span className="context-item-icon">{"\u2699"}</span>
            <span>Preferred Source</span>
            <span className="ctx-arrow">{"\u25B8"}</span>
          </div>
          {hoverSubMenu === "pref" && (
            <div
              className={submenuClass}
              onMouseEnter={() => handleEnter("pref")}
              onMouseLeave={handleLeave}
            >
              {platforms.map((platform) => (
                <button
                  key={platform.platform}
                  type="button"
                  className="context-item"
                  onClick={() =>
                    void runAction(() => api.setPreferredSource(game.id, platform.platform))
                  }
                >
                  <span className="context-item-icon">
                    {platform.platform === preferredSource ? "\u25C9" : "\u25CB"}
                  </span>
                  <span>{platformMeta(platform.platform).label || platform.platform}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {executables.length > 1 && (
        <div
          className="ctx-parent"
          onMouseEnter={() => handleEnter("exe")}
          onMouseLeave={handleLeave}
        >
          <div className="context-item">
            <span className="context-item-icon">{"\u2699"}</span>
            <span>Default Executable</span>
            <span className="ctx-arrow">{"\u25B8"}</span>
          </div>
          {hoverSubMenu === "exe" && (
            <div
              className={submenuClass}
              onMouseEnter={() => handleEnter("exe")}
              onMouseLeave={handleLeave}
            >
              {executables.map((executable) => (
                <button
                  key={executable.path}
                  type="button"
                  className="context-item"
                  onClick={() =>
                    void runAction(() => api.setPrimaryExecutable(game.id, executable.path))
                  }
                >
                  <span className="context-item-icon">
                    {executable.primary === true ? "\u25C9" : "\u25CB"}
                  </span>
                  <span>{executable.name}.exe</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="context-divider" />

      <button
        type="button"
        className="context-item"
        onClick={() => void runAction(() => api.openGameDirectory(game.id), true)}
      >
        <span className="context-item-icon">{"\uD83D\uDCC1"}</span>
        <span>Open Game Folder</span>
      </button>

      {savePath !== "" && (
        <button
          type="button"
          className="context-item"
          onClick={() => void runAction(() => api.openDirectory(savePath), true)}
        >
          <span className="context-item-icon">{"\uD83D\uDCBE"}</span>
          <span>Open Save Folder</span>
        </button>
      )}

      <div className="context-divider" />

      <button
        type="button"
        className="context-item"
        onClick={() => void runAction(() => api.openGameMetadata(game.id), true)}
      >
        <span className="context-item-icon">{"\uD83D\uDCC4"}</span>
        <span>Open Metadata File</span>
      </button>
    </div>
  );
}
