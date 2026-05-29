import { useState, useEffect } from 'react';
import { game } from '../../wailsjs/go/models';
import { GetGameCover } from '../../wailsjs/go/main/App';

interface GameCardProps {
  game: game.GameInfo;
  onClick?: (game: game.GameInfo) => void;
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
    case 'local':
    default:        return { label: 'Local', color: '#555' };
  }
}

export default function GameCard({ game, onClick, isScraping }: GameCardProps) {
  const [coverData, setCoverData] = useState('');

  useEffect(() => {
    if (!game.metadata?.coverUrl) return;
    GetGameCover(game.id).then(setCoverData).catch(() => {});
  }, [game.id, game.metadata?.coverUrl, isScraping]);

  const badge = getPlatformBadge(game.platform);
  const playtime = formatPlaytime(game.totalPlaytime);

  return (
    <div
      className="game-card"
      onClick={() => onClick?.(game)}
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
        {isScraping && (
          <span className="game-card-scraping" title="Scraping metadata...">
            &#9881;
          </span>
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
