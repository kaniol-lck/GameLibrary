import { useState, useEffect } from 'react';
import { game } from '../../wailsjs/go/models';
import { GetGameCover } from '../../wailsjs/go/main/App';

interface GameCardProps {
  game: game.GameInfo;
  onClick?: (game: game.GameInfo) => void;
  onContextMenu?: (game: game.GameInfo, x: number, y: number) => void;
  isScraping?: boolean;
}

function formatPlaytime(seconds: number): string {
  if (seconds <= 0) return '';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function getPlatformBadge(platform: string): { label: string; color: string } {
  switch (platform) {
    case 'steam':   return { label: 'Steam', color: '#1b2838' };
    case 'vndb':    return { label: 'VNDB', color: '#2255a4' };
    case 'dlsite':  return { label: 'DLsite', color: '#e6005c' };
    case 'bangumi': return { label: 'Bangumi', color: '#e57399' };
    case 'local':
    default:        return { label: 'Local', color: '#555' };
  }
}

export default function GameCard({ game, onClick, onContextMenu, isScraping }: GameCardProps) {
  const [coverData, setCoverData] = useState('');

  useEffect(() => {
    if (!game.metadata?.coverUrl) return;
    GetGameCover(game.id).then(setCoverData).catch(() => {});
  }, [game.id, game.metadata?.coverUrl, isScraping]);

  const badge = getPlatformBadge(game.platform);
  const playtime = formatPlaytime(game.totalPlaytime);

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    onContextMenu?.(game, e.clientX, e.clientY);
  };

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
        <span className="game-card-platform" style={{ backgroundColor: badge.color }}>
          {badge.label}
        </span>
        {game.starred && (
          <span className="game-card-star" title="Starred">{'\u2605'}</span>
        )}
        {isScraping && (
          <span className="game-card-scraping" title="Scraping metadata...">
            &#9881;
          </span>
        )}
        {game.tags && game.tags.length > 0 && (
          <div className="game-card-tags">
            {game.tags.slice(0, 3).map((tag) => (
              <span key={tag} className="game-card-tag">{tag}</span>
            ))}
          </div>
        )}
      </div>
      <div className="game-card-body">
        <h3 className="game-card-title">{game.title}</h3>
        {game.titleNative && <p className="game-card-title-native">{game.titleNative}</p>}
        {playtime && <span className="game-card-playtime">{playtime}</span>}
      </div>
    </div>
  );
}
