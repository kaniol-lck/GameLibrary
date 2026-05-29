import { useState, useEffect } from 'react';
import { game } from '../../wailsjs/go/models';
import { ScrapeGame, LaunchGame, GetGameCoverLandscape } from '../../wailsjs/go/main/App';

interface GameDetailProps {
  game: game.GameInfo;
  onClose: () => void;
  onUpdated: () => void;
}

function formatPlaytime(seconds: number): string {
  if (seconds <= 0) return 'Never played';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export default function GameDetail({ game: initialGame, onClose, onUpdated }: GameDetailProps) {
  const [g, setG] = useState<game.GameInfo>(initialGame);
  const [coverData, setCoverData] = useState('');
  const [scraping, setScraping] = useState(false);
  const [scrapeMsg, setScrapeMsg] = useState('');

  useEffect(() => {
    if (!g.metadata?.coverUrl) return;
    GetGameCoverLandscape(g.id).then(setCoverData).catch(() => {});
  }, [g.id, g.metadata?.coverUrl]);

  const handleScrape = async () => {
    setScraping(true);
    setScrapeMsg('');
    try {
      const report = await ScrapeGame(g.id);
      if (report.error) {
        setScrapeMsg(report.error);
      } else {
        setScrapeMsg(`Scraped from ${report.source}: ${report.title}`);
        onUpdated();
      }
    } catch (err) {
      setScrapeMsg(String(err));
    } finally {
      setScraping(false);
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
            <span className="detail-badge">{g.platform}</span>
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
            <button className="btn btn-secondary" onClick={handleScrape} disabled={scraping}>
              {scraping ? 'Scraping...' : 'Scrape Metadata'}
            </button>
            <button className="btn btn-ghost-sm" onClick={handleScrape} disabled={scraping} title="Force re-scrape">
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
