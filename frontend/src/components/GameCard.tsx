import { memo, useCallback, useState, type KeyboardEvent, type MouseEvent } from "react";
import { api, type Game } from "@/api/client";
import { coverKey, coverUrl } from "@/lib/cover";
import { errorMessage, formatPlaytime } from "@/lib/format";
import { gamePlatforms, platformMeta } from "@/lib/platform";

/**
 * Scrape state for one card.
 *
 * It is a single prop instead of the three booleans (`isScraping`, `scrapedOk`,
 * `scrapedErr`) the old card took, so the four states cannot contradict each
 * other.
 */
export type GameCardStatus = "idle" | "scraping" | "ok" | "error";

export interface GameCardProps {
  game: Game;
  status: GameCardStatus;
  onClick: (id: string) => void;
  onContextMenu: (id: string, x: number, y: number) => void;
}

/** Genre tags shown on the artwork before the card gets too busy. */
const GENRE_TAG_LIMIT = 3;
/** User tags shown on the artwork. */
const USER_TAG_LIMIT = 2;

function GameCard({ game, status, onClick, onContextMenu }: GameCardProps) {
  // Tracked by cover key rather than a boolean, so re-scraping a game that once
  // failed to load its artwork tries the new URL instead of staying a placeholder.
  const [failedCover, setFailedCover] = useState("");
  const [launchError, setLaunchError] = useState("");

  const platforms = gamePlatforms(game);
  const primaryPlatform = game.preferredSource || platforms[0]?.platform || "";
  const cover = coverUrl(game.id, "cover", game.coverVersion);
  const coverId = coverKey(game.id, game.coverVersion);
  const playtime = formatPlaytime(game.totalPlaytime);
  const genreTags = (game.metadata?.tags ?? []).slice(0, GENRE_TAG_LIMIT);
  const userTags = (game.tags ?? []).slice(0, USER_TAG_LIMIT);
  const scraping = status === "scraping";

  const handleLaunch = useCallback(
    async (event: MouseEvent<HTMLButtonElement>) => {
      event.stopPropagation();
      setLaunchError("");
      try {
        await api.launch(game.id);
      } catch (err) {
        // The old card swallowed this, so a game that failed to start looked
        // like a game that did nothing.
        setLaunchError(errorMessage(err));
      }
    },
    [game.id],
  );

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      // Space would otherwise scroll the grid.
      event.preventDefault();
      onClick(game.id);
    }
  };

  return (
    <div
      className={`game-card${scraping ? " game-card-scraping-dim" : ""}`}
      role="button"
      tabIndex={0}
      aria-label={game.title}
      onClick={() => onClick(game.id)}
      onContextMenu={(event) => {
        event.preventDefault();
        onContextMenu(game.id, event.clientX, event.clientY);
      }}
      onKeyDown={handleKeyDown}
    >
      <div className="game-card-cover">
        {failedCover === coverId ? (
          <div className="game-card-cover-placeholder">
            <span>{game.title.charAt(0).toUpperCase()}</span>
          </div>
        ) : (
          <img
            src={cover}
            alt={game.title}
            loading="lazy"
            onError={() => setFailedCover(coverId)}
          />
        )}

        {game.starred === true && (
          <span className="game-card-star" title="Starred">
            {"\u2605"}
          </span>
        )}

        {platforms.length > 0 && (
          <div className="game-card-platforms">
            {platforms.map((platform) => {
              const meta = platformMeta(platform.platform);
              const label = meta.label || platform.platform;
              return (
                <span
                  key={platform.platform}
                  className={`game-card-plat-tag${
                    platform.platform === primaryPlatform ? " game-card-plat-primary" : ""
                  }`}
                  style={{ backgroundColor: meta.color }}
                  title={label}
                >
                  {label}
                </span>
              );
            })}
          </div>
        )}

        <button
          type="button"
          className="game-card-launch"
          onClick={(event) => void handleLaunch(event)}
          title="Launch"
          aria-label={`Launch ${game.title}`}
        >
          {"\u25B6"}
        </button>

        {scraping && <span className="game-card-scraping" title="Scraping…" />}
        {status === "ok" && (
          <span className="game-card-scraped-ok" title="Scraped successfully">
            {"\u2713"}
          </span>
        )}
        {status === "error" && (
          <span className="game-card-scraped-err" title="Scrape failed">
            {"\u2717"}
          </span>
        )}

        {genreTags.length > 0 && (
          <div className="game-card-genre-tags">
            {genreTags.map((tag) => (
              <span key={tag} className="game-card-genre-tag">
                {tag}
              </span>
            ))}
          </div>
        )}

        {userTags.length > 0 && (
          <div className="game-card-user-tags">
            {userTags.map((tag) => (
              <span key={tag} className="game-card-user-tag">
                #{tag}
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="game-card-body">
        <h3 className="game-card-title">{game.title}</h3>
        {game.titleNative && <p className="game-card-title-native">{game.titleNative}</p>}
        {playtime !== "" && <span className="game-card-playtime">{playtime}</span>}
        {platforms.length > 1 && (
          <div className="game-card-platform-dots">
            {platforms.map((platform) => (
              <span
                key={platform.platform}
                className="game-card-plat-dot"
                style={{ backgroundColor: platformMeta(platform.platform).color }}
                title={platformMeta(platform.platform).label || platform.platform}
              />
            ))}
          </div>
        )}
        {launchError !== "" && (
          <p className="game-card-error" role="alert">
            {launchError}
          </p>
        )}
      </div>
    </div>
  );
}

// The grid re-renders on every queue event; a card only changes when its own
// game, status or callbacks do.
export default memo(GameCard);
