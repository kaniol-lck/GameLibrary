import { useState, useEffect } from 'react';
import { game } from '../../wailsjs/go/models';
import { LaunchGame, GetGameCoverLandscape } from '../../wailsjs/go/main/App';

interface GameDetailProps {
  game: game.GameInfo;
  onClose: () => void;
  onUpdated: () => void;
  onScrape?: (id: string) => Promise<void>;
  isScraping?: boolean;
}

function formatPlaytime(seconds: number): string {
  if (seconds <= 0) return '';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function getPlatColor(platform: string): string {
  switch (platform) {
    case 'steam': return '#1a3a5c';
    case 'vndb': return '#2255a4';
    case 'dlsite': return '#c2185b';
    case 'bangumi': return '#e57399';
    default: return '#555';
  }
}

export default function GameDetail({ game: initialGame, onClose, onUpdated, onScrape, isScraping }: GameDetailProps) {
  const [g, setG] = useState<game.GameInfo>(initialGame);
  const [coverData, setCoverData] = useState('');
  const [scrapeMsg, setScrapeMsg] = useState('');

  useEffect(() => {
    if (!g.metadata?.coverUrl) return;
    GetGameCoverLandscape(g.id).then(setCoverData).catch(() => {});
  }, [g.id, g.metadata?.coverUrl]);

  const handleScrape = async () => {
    setScrapeMsg('Scraping...');
    try {
      if (onScrape) {
        await onScrape(g.id);
        setScrapeMsg('Scrape completed');
      }
      onUpdated();
    } catch {
      setScrapeMsg('Scrape failed');
    }
  };

  const meta = g.metadata;

  return (
    <div className="detail-overlay" onClick={onClose}>
      <div className="detail-panel" onClick={(e) => e.stopPropagation()}>
        <button className="detail-close" onClick={onClose}>&times;</button>

        <div className="detail-cover">
          {coverData ? (
            <img src={coverData} alt={g.title} />
          ) : (
            <div className="game-card-cover-placeholder">
              <span>{g.title.charAt(0).toUpperCase()}</span>
            </div>
          )}
        </div>

        <div className="detail-body">
          <h2 className="detail-title">{g.title}</h2>
          {g.titleNative && <p className="detail-title-native">{g.titleNative}</p>}

          <div className="detail-meta-row">
            <div className="detail-platforms">
              {((g as any).platforms || []).map((p: any) => (
                <span key={p.platform} className="detail-platform-tag" style={{ backgroundColor: getPlatColor(p.platform) }}>
                  {p.platform}
                </span>
              ))}
            </div>
            <span className="detail-meta-text">{formatPlaytime(g.totalPlaytime)}</span>
          </div>

          {meta && (
            <>
              {(meta.developer || meta.publisher) && (
                <div className="detail-section">
                  {meta.developer && (
                    <div className="detail-field"><label>Developer</label><span>{meta.developer}</span></div>
                  )}
                  {meta.publisher && (
                    <div className="detail-field"><label>Publisher</label><span>{meta.publisher}</span></div>
                  )}
                </div>
              )}

              {meta.releaseDate && (
                <div className="detail-section">
                  <div className="detail-field"><label>Release Date</label><span>{meta.releaseDate}</span></div>
                </div>
              )}

              {meta.description && (
                <div className="detail-section">
                  <label>Description</label>
                  <p className="detail-desc">{meta.description}</p>
                </div>
              )}

              {meta.tags && meta.tags.length > 0 && (
                <div className="detail-section">
                  <label>Tags</label>
                  <div className="detail-tags">
                    {meta.tags.map((tag: string, i: number) => (
                      <span key={i} className="detail-tag">{tag}</span>
                    ))}
                  </div>
                </div>
              )}

              {meta.links && Object.keys(meta.links).length > 0 && (
                <div className="detail-section">
                  <label>Links</label>
                  <div className="detail-links">
                    {Object.entries(meta.links).filter(([k]) => k !== 'platformId').map(([key, url]) => (
                      <a key={key} className="detail-link" href={url as string} target="_blank" rel="noopener">
                        {key}
                      </a>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}

          <div className="detail-section">
            <label>Executables</label>
            <ul className="detail-exe-list">
              {g.executables.map((exe, i) => (
                <li key={i} className={exe.primary ? 'exe-primary' : ''}>
                  {exe.name}.exe
                  {exe.primary && <span className="exe-badge">primary</span>}
                </li>
              ))}
            </ul>
          </div>

          <div className="detail-actions">
            <button className="btn btn-launch" onClick={async () => {
              try { await LaunchGame(g.id); }
              catch (err) { setScrapeMsg(String(err)); }
            }}>
              &#9654; Launch Game
            </button>
            <button className="btn btn-secondary" onClick={handleScrape} disabled={isScraping}>
              {isScraping ? 'Scraping...' : 'Scrape Metadata'}
            </button>
            <button className="btn btn-ghost-sm" onClick={handleScrape} disabled={isScraping} title="Force re-scrape">
              Re-scrape
            </button>
          </div>

          {scrapeMsg && (
            <div className={`scrape-result ${scrapeMsg.includes(':') ? 'scrape-ok' : 'scrape-err'}`}>
              {scrapeMsg}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
