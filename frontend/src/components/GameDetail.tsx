import { useState, useEffect } from 'react';
import { game } from '../../wailsjs/go/models';
import { LaunchGame, GetGameCoverLandscape, ToggleGameStar, AddGameTag, RemoveGameTag, OpenGameDirectory, OpenGameMetadata, SetPreferredSource, OpenBrowser } from '../../wailsjs/go/main/App';

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
  switch (platform) { case 'steam': return '#1a4b8a'; case 'vndb': return '#2255a4'; case 'dlsite': return '#c2185b'; case 'bangumi': return '#e57399'; default: return '#555'; }
}

export default function GameDetail({ game: initialGame, onClose, onUpdated, onScrape, isScraping }: GameDetailProps) {
  const [g, setG] = useState<game.GameInfo>(initialGame);
  const [coverData, setCoverData] = useState('');
  const [scrapeMsg, setScrapeMsg] = useState('');
  const [showTagInput, setShowTagInput] = useState(false);
  const [tagInput, setTagInput] = useState('');

  useEffect(() => {
    if (!g.metadata?.coverUrl) return;
    GetGameCoverLandscape(g.id).then(setCoverData).catch(() => {});
  }, [g.id, g.metadata?.coverUrl]);
  useEffect(() => { setG(initialGame); }, [initialGame]);

  const meta = g.metadata;
  const platforms: Array<{platform: string, id: string}> = (g as any).platforms || [];
  const preferredSource = (g as any).preferredSource || '';

  const handleStar = async () => { try { await ToggleGameStar(g.id); onUpdated(); } catch {} };
  const handleLaunch = async () => { try { await LaunchGame(g.id); } catch (err) { setScrapeMsg(String(err)); } };
  const handleScrape = async () => {
    setScrapeMsg('Scraping...');
    try { if (onScrape) { await onScrape(g.id); setScrapeMsg('Scrape completed'); } onUpdated(); } catch { setScrapeMsg('Scrape failed'); }
  };
  const handleOpenDir = async () => { try { await OpenGameDirectory(g.id); } catch {} };
  const handleOpenMeta = async () => { try { await OpenGameMetadata(g.id); } catch {} };
  const handleSetPreferred = async (src: string) => { try { await SetPreferredSource(g.id, src); onUpdated(); } catch {} };
  const handleOpenPage = (url: string) => { if (url) OpenBrowser(url).catch(() => {}); };
  const handleAddTag = async () => {
    const tag = tagInput.trim(); if (!tag) { setShowTagInput(false); return; }
    try { await AddGameTag(g.id, tag); onUpdated(); setTagInput(''); setShowTagInput(false); } catch {}
  };
  const handleRemoveTag = async (tag: string) => { try { await RemoveGameTag(g.id, tag); onUpdated(); } catch {} };

  const platUrl = (p: any) => {
    if (p.platform === 'steam' && p.id) return `https://store.steampowered.com/app/${p.id}/`;
    if (p.platform === 'dlsite' && p.id) return `https://www.dlsite.com/maniax/work/=/product_id/${p.id}.html`;
    if (p.platform === 'bangumi' && p.id) return `https://bgm.tv/subject/${p.id}`;
    return '';
  };

  return (
    <div className="detail-overlay" onClick={onClose}>
      <div className="detail-dialog" onClick={(e) => e.stopPropagation()}>
        <button className="detail-close" onClick={onClose}>&times;</button>

        <div className="detail-cover">
          {coverData ? <img src={coverData} alt={g.title} />
            : <div className="game-card-cover-placeholder"><span>{g.title.charAt(0).toUpperCase()}</span></div>}
        </div>

        <div className="detail-body">
          <div className="detail-header-row">
            <h2 className="detail-title">{g.title}</h2>
            <button className="detail-star-btn" onClick={handleStar} title={g.starred ? 'Unstar' : 'Star'}>
              {g.starred ? '\u2605' : '\u2606'}
            </button>
          </div>
          {g.titleNative && <p className="detail-title-native">{g.titleNative}</p>}

          <div className="detail-meta-row">
            <div className="detail-platforms">
              {platforms.map((p: any) => {
                const url = platUrl(p);
                return url ? (
                  <a key={p.platform} className="detail-platform-tag detail-platform-link"
                    style={{ backgroundColor: getPlatColor(p.platform) }}
                    onClick={(e) => { e.stopPropagation(); handleOpenPage(url); }} title={'Open ' + p.platform + ' page'}>
                    {p.platform}
                    <span className="detail-plat-arrow">{'\u2197'}</span>
                  </a>
                ) : (
                  <span key={p.platform} className="detail-platform-tag" style={{ backgroundColor: getPlatColor(p.platform) }}>{p.platform}</span>
                );
              })}
            </div>
            {g.totalPlaytime > 0 && <span className="detail-meta-text">{formatPlaytime(g.totalPlaytime)}</span>}
          </div>

          <button className="btn btn-launch btn-launch-lg" onClick={handleLaunch} disabled={g.executables.length === 0}>
            {'\u25B6'} Launch Game
          </button>

          {(meta?.developer || meta?.publisher) && (
            <div className="detail-section">
              {meta.developer && <div className="detail-field"><label>Developer</label><span>{meta.developer}</span></div>}
              {meta.publisher && <div className="detail-field"><label>Publisher</label><span>{meta.publisher}</span></div>}
            </div>
          )}

          {meta?.releaseDate && (
            <div className="detail-section">
              <div className="detail-field"><label>Release Date</label><span>{meta.releaseDate}</span></div>
            </div>
          )}

          {meta?.description && (
            <div className="detail-section">
              <label>Description</label>
              <p className="detail-desc">{meta.description}</p>
            </div>
          )}

          <div className="detail-section">
            <label>Tags</label>
            <div className="detail-tags-wrap">
              {/* genre tags from scraper */}
              {(meta?.tags || []).map((tag: string, i: number) => (
                <span key={'g'+i} className="detail-tag detail-tag-genre">{tag}</span>
              ))}
              {/* custom user tags */}
              {(g.tags || []).map((t: string) => (
                <span key={'u'+t} className="detail-tag detail-tag-user">
                  #{t}
                  <button className="detail-tag-remove" onClick={() => handleRemoveTag(t)}>&times;</button>
                </span>
              ))}
              {!showTagInput ? (
                <button className="detail-tag detail-tag-add" onClick={() => setShowTagInput(true)}>+</button>
              ) : (
                <div className="context-tag-input detail-tag-inline">
                  <input autoFocus type="text" placeholder="tag..." value={tagInput}
                    onChange={(e) => setTagInput(e.target.value)}
                    onKeyDown={(e) => { if (e.key === 'Enter') handleAddTag(); if (e.key === 'Escape') setShowTagInput(false); }} />
                  <button onClick={handleAddTag}>Add</button>
                </div>
              )}
            </div>
          </div>

          {platforms.length > 1 && (
            <div className="detail-section">
              <label>Preferred Source</label>
              <div className="detail-pref-row">
                {platforms.map((p) => (
                  <button key={'pref-'+p.platform}
                    className={`detail-pref-btn ${p.platform === preferredSource ? 'detail-pref-active' : ''}`}
                    onClick={() => handleSetPreferred(p.platform)}>
                    {p.platform === preferredSource ? '\u25C9' : '\u25CB'} {p.platform}
                  </button>
                ))}
              </div>
            </div>
          )}

          {(g.aliases && g.aliases.length > 0) && (
            <div className="detail-section">
              <label>Aliases</label>
              <div className="detail-tags-wrap">
                {g.aliases.map((a: string, i: number) => (
                  <span key={i} className="detail-tag detail-tag-alias">{a}</span>
                ))}
              </div>
            </div>
          )}

          <div className="detail-section">
            <label>Executables</label>
            <ul className="detail-exe-list">
              {g.executables.map((exe, i) => (
                <li key={i}>{exe.name}.exe{exe.primary && <span className="exe-badge">primary</span>}</li>
              ))}
            </ul>
          </div>

          <div className="detail-section detail-bottom-actions">
            <button className="btn btn-secondary" onClick={handleScrape} disabled={isScraping}>
              {isScraping ? 'Scraping...' : '\u21BB Re-scrape Metadata'}
            </button>
            <button className="btn btn-secondary" onClick={handleOpenDir}>{'\uD83D\uDCC1'} Open Folder</button>
            <button className="btn btn-ghost-sm" onClick={handleOpenMeta}>{'\uD83D\uDCC4'} Metadata</button>
          </div>

          {scrapeMsg && (
            <div className={`scrape-result ${scrapeMsg.includes('completed') ? 'scrape-ok' : 'scrape-err'}`}>{scrapeMsg}</div>
          )}
        </div>
      </div>
    </div>
  );
}
