import { useState, useEffect } from 'react';
import { game } from '../../wailsjs/go/models';
import { GetGameCover, LaunchGame, ScrapeGame } from '../../wailsjs/go/main/App';

interface GameCardProps {
  game: game.GameInfo;
  onClick?: (game: game.GameInfo) => void;
  onContextMenu?: (game: game.GameInfo, x: number, y: number) => void;
  onUpdated?: () => void;
  isScraping?: boolean;
  scrapedOk?: boolean;
  scrapedErr?: boolean;
  refreshKey?: number;
}

function formatPlaytime(seconds: number): string {
  if (seconds <= 0) return '';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function getPlatformBadge(platform: string): { label: string; color: string } | null {
  if (!platform) return null;
  switch (platform) {
    case 'steam':   return { label: 'Steam', color: '#1a3a5c' };
    case 'vndb':    return { label: 'VNDB', color: '#2255a4' };
    case 'dlsite':  return { label: 'DLsite', color: '#c2185b' };
    case 'bangumi': return { label: 'Bangumi', color: '#e57399' };
    default:        return { label: platform, color: '#555' };
  }
}

function platformUrl(platform: string, id: string): string {
  switch (platform) {
    case 'steam': return id ? `https://store.steampowered.com/app/${id}/` : '';
    case 'dlsite': return id ? `https://www.dlsite.com/maniax/work/=/product_id/${id}.html` : '';
    case 'vndb': return id ? `https://vndb.org/v${id}` : '';
    case 'bangumi': return id ? `https://bgm.tv/subject/${id}` : '';
    default: return '';
  }
}

export default function GameCard({ game, onClick, onContextMenu, onUpdated, isScraping, scrapedOk, scrapedErr, refreshKey }: GameCardProps) {
  const [coverData, setCoverData] = useState('');

  const primaryPlatform = (game as any).preferredSource || (game as any).platforms?.[0]?.platform || (game as any).platform || 'local';

  useEffect(() => {
    if (!game.metadata?.coverUrl) return;
    GetGameCover(game.id).then(setCoverData).catch(() => {});
  }, [game.id, game.metadata?.coverUrl, refreshKey]);

  const badge = getPlatformBadge(primaryPlatform);
  const playtime = formatPlaytime(game.totalPlaytime);
  const genreTags = (game.metadata?.tags || []).slice(0, 3);
  const userTags = (game.tags || []).slice(0, 2);
  const allPlatforms: Array<{platform: string, id: string}> = (game as any).platforms || [];

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    onContextMenu?.(game, e.clientX, e.clientY);
  };

  const handleLaunch = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try { await LaunchGame(game.id); } catch { /* ignore */ }
  };

  const showBadge = scrapedOk || scrapedErr || isScraping;

  return (
    <div
      className="game-card"
      onClick={() => onClick?.(game)}
      onContextMenu={handleContextMenu}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => { if (e.key === 'Enter') onClick?.(game); }}
    >
      <div className="game-card-cover">
        {coverData ? (
          <img src={coverData} alt={game.title} loading="lazy" />
        ) : (
          <div className="game-card-cover-placeholder">
            <span>{game.title.charAt(0).toUpperCase()}</span>
          </div>
        )}
        {badge && (
          <span className="game-card-platform" style={{ backgroundColor: badge.color }}>
            {badge.label}
          </span>
        )}
        <div className="game-card-platforms">
          {allPlatforms.map((p) => (
            <span key={p.platform} className={`game-card-plat-tag ${p.platform === primaryPlatform ? 'game-card-plat-primary' : ''}`}
              style={{ backgroundColor: (getPlatformBadge(p.platform) || { color: '#555' }).color }}
              title={p.platform}>
              {(getPlatformBadge(p.platform) || { label: p.platform }).label}
            </span>
          ))}
        </div>
        {game.starred && (
          <span className="game-card-star" title="Starred">{'\u2605'}</span>
        )}
        <button className="game-card-launch" onClick={handleLaunch} title="Launch">
          {'\u25B6'}
        </button>
        {showBadge && isScraping && (
          <span className="game-card-scraping" title="Scraping..." />
        )}
        {scrapedOk && (
          <span className="game-card-scraped-ok" title="Scraped successfully">{'\u2713'}</span>
        )}
        {scrapedErr && (
          <span className="game-card-scraped-err" title="Scrape failed">{'\u2717'}</span>
        )}
        {genreTags.length > 0 && (
          <div className="game-card-genre-tags">
            {genreTags.map((tag) => (
              <span key={tag} className="game-card-genre-tag">{tag}</span>
            ))}
          </div>
        )}
        {userTags.length > 0 && (
          <div className="game-card-user-tags">
            {userTags.map((tag) => (
              <span key={tag} className="game-card-user-tag">#{tag}</span>
            ))}
          </div>
        )}
      </div>
      <div className="game-card-body">
        <h3 className="game-card-title">{game.title}</h3>
        {game.titleNative && <p className="game-card-title-native">{game.titleNative}</p>}
        {playtime && <span className="game-card-playtime">{playtime}</span>}
        {allPlatforms.length > 1 && (
          <div className="game-card-platform-dots">
            {allPlatforms.map((p) => (
              <span key={p.platform} className="game-card-plat-dot" style={{ backgroundColor: (getPlatformBadge(p.platform) || { color: '#555' }).color }} title={p.platform} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
