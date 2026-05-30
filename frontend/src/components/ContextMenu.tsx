import { useState, useRef, useEffect, useCallback } from 'react';
import { game } from '../../wailsjs/go/models';
import { ToggleGameStar, AddGameTag, RemoveGameTag, OpenGameDirectory, OpenGameMetadata, LaunchGame, SetPreferredSource, OpenBrowser } from '../../wailsjs/go/main/App';

interface ContextMenuProps {
  game: game.GameInfo;
  x: number;
  y: number;
  onClose: () => void;
  onUpdated: () => void;
  onScrape?: (id: string) => Promise<void>;
}

export default function ContextMenu({ game, x, y, onClose, onUpdated, onScrape }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [showTagInput, setShowTagInput] = useState(false);
  const [tagInput, setTagInput] = useState('');
  const [hoverSubMenu, setHoverSubMenu] = useState('');
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const adjustedX = Math.min(x, window.innerWidth - 420);
  const adjustedY = Math.min(y, window.innerHeight - 300);

  const handleClickOutside = useCallback((e: MouseEvent) => {
    if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
      onClose();
    }
  }, [onClose]);

  useEffect(() => {
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [handleClickOutside]);

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onClose]);

  const handleStar = async () => { try { await ToggleGameStar(game.id); onUpdated(); } catch {} onClose(); };
  const handleOpenDir = async () => { try { await OpenGameDirectory(game.id); } catch {} onClose(); };
  const handleOpenMeta = async () => { try { await OpenGameMetadata(game.id); } catch {} onClose(); };
  const handleLaunch = async () => { try { await LaunchGame(game.id); } catch {} onClose(); };
  const handleReScrape = async () => { if (onScrape) { try { await onScrape(game.id); onUpdated(); } catch {} } onClose(); };
  const handleSetPreferred = async (source: string) => { try { await SetPreferredSource(game.id, source); onUpdated(); } catch {} setHoverSubMenu(''); };
  const handleOpenPage = (url: string) => { if (url) OpenBrowser(url).catch(() => {}); };

  const handleEnter = (key: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setHoverSubMenu(key);
  };
  const handleLeave = () => {
    closeTimer.current = setTimeout(() => setHoverSubMenu(''), 200);
  };
  const handleSubEnter = (key: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current);
    setHoverSubMenu(key);
  };

  const handleAddTag = async () => {
    const tag = tagInput.trim();
    if (!tag) { setShowTagInput(false); return; }
    try { await AddGameTag(game.id, tag); onUpdated(); } catch {}
    setTagInput('');
    setShowTagInput(false);
  };
  const handleRemoveTag = async (tag: string) => { try { await RemoveGameTag(game.id, tag); onUpdated(); } catch {} };

  const platforms: Array<{platform: string, id: string}> = (game as any).platforms || [];
  const preferredSource = (game as any).preferredSource || '';
  const hasExe = (game.executables || []).length > 0;

  const subFlipX = adjustedX + 420 > window.innerWidth;

  return (
    <div ref={menuRef} className="context-menu" style={{ left: adjustedX, top: adjustedY }}>
      <div className="context-menu-section">
        <div className="context-menu-title">{game.title}</div>
      </div>

      {hasExe && (
        <button className="context-item context-launch" onClick={handleLaunch}>
          <span className="context-item-icon">{'\u25B6'}</span>
          <span>Launch Game</span>
        </button>
      )}

      <button className="context-item" onClick={handleStar}>
        <span className="context-item-icon">{game.starred ? '\u2605' : '\u2606'}</span>
        <span>{game.starred ? 'Unstar' : 'Star'}</span>
      </button>

      <div className="context-divider" />

      <button className="context-item" onClick={handleReScrape}>
        <span className="context-item-icon">{'\u21BB'}</span>
        <span>Re-scrape Metadata</span>
      </button>

      <div className="context-divider" />

      {!showTagInput ? (
        <button className="context-item" onClick={() => setShowTagInput(true)}>
          <span className="context-item-icon">+</span>
          <span>Add Tag</span>
        </button>
      ) : (
        <div className="context-tag-input">
          <input autoFocus type="text" placeholder="Tag name..." value={tagInput}
            onChange={(e) => setTagInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') handleAddTag(); if (e.key === 'Escape') setShowTagInput(false); }} />
          <button onClick={handleAddTag}>Add</button>
        </div>
      )}

      {(game.tags && game.tags.length > 0) && (
        <div className="context-tags">
          {game.tags.map((tag) => (
            <span key={tag} className="context-tag-chip">
              {tag}
              <button className="context-tag-remove" onClick={() => handleRemoveTag(tag)} title="Remove tag">&times;</button>
            </span>
          ))}
        </div>
      )}

      {platforms.length > 0 && <div className="context-divider" />}

      {platforms.length > 0 && (
        <div className="ctx-parent"
          onMouseEnter={() => handleEnter('pages')}
          onMouseLeave={handleLeave}>
          <div className="context-item">
            <span className="context-item-icon">{'\uD83D\uDD17'}</span>
            <span>Open Web Page</span>
            <span className="ctx-arrow">{'\u25B8'}</span>
          </div>
          {hoverSubMenu === 'pages' && (
            <div className={`ctx-submenu ${subFlipX ? 'ctx-sub-left' : ''}`}
              onMouseEnter={() => handleSubEnter('pages')}
              onMouseLeave={handleLeave}>
              {platforms.map((p) => {
                let url = '';
                if (p.platform === 'steam' && p.id) url = `https://store.steampowered.com/app/${p.id}/`;
                else if (p.platform === 'dlsite' && p.id) url = `https://www.dlsite.com/maniax/work/=/product_id/${p.id}.html`;
                else if (p.platform === 'bangumi' && p.id) url = `https://bgm.tv/subject/${p.id}`;
                return url ? (
                  <button key={'link-'+p.platform} className="context-item" onClick={() => handleOpenPage(url)}>
                    <span className="context-item-icon">{getPlatIcon(p.platform)}</span>
                    <span>{p.platform.charAt(0).toUpperCase() + p.platform.slice(1)}</span>
                  </button>
                ) : null;
              })}
            </div>
          )}
        </div>
      )}

      {platforms.length > 1 && (
        <div className="ctx-parent"
          onMouseEnter={() => handleEnter('pref')}
          onMouseLeave={handleLeave}>
          <div className="context-item">
            <span className="context-item-icon">{'\u2699'}</span>
            <span>Preferred Source</span>
            <span className="ctx-arrow">{'\u25B8'}</span>
          </div>
          {hoverSubMenu === 'pref' && (
            <div className={`ctx-submenu ${subFlipX ? 'ctx-sub-left' : ''}`}
              onMouseEnter={() => handleSubEnter('pref')}
              onMouseLeave={handleLeave}>
              {platforms.map((p) => (
                <button key={'pref-'+p.platform} className="context-item" onClick={() => handleSetPreferred(p.platform)}>
                  <span className="context-item-icon">{p.platform === preferredSource ? '\u25C9' : '\u25CB'}</span>
                  <span>{p.platform.charAt(0).toUpperCase() + p.platform.slice(1)}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="context-divider" />

      <button className="context-item" onClick={handleOpenDir}>
        <span className="context-item-icon">{'\uD83D\uDCC1'}</span>
        <span>Open Game Folder</span>
      </button>

      <button className="context-item disabled">
        <span className="context-item-icon">{'\uD83D\uDCBE'}</span>
        <span>Open Save Folder</span>
      </button>

      <div className="context-divider" />

      <button className="context-item" onClick={handleOpenMeta}>
        <span className="context-item-icon">{'\uD83D\uDCC4'}</span>
        <span>Open Metadata File</span>
      </button>
    </div>
  );
}

function getPlatIcon(p: string): string {
  switch (p) { case 'steam': return '\u25A0'; case 'dlsite': return '\u25C6'; case 'vndb': return '\u25B6'; case 'bangumi': return '\u25CF'; default: return '\u25CB'; }
}
