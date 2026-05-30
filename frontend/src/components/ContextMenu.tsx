import { useState, useRef, useEffect, useCallback } from 'react';
import { game } from '../../wailsjs/go/models';
import { ToggleGameStar, AddGameTag, RemoveGameTag, OpenGameDirectory, OpenGameMetadata, LaunchGame, ScrapeGame } from '../../wailsjs/go/main/App';

interface ContextMenuProps {
  game: game.GameInfo;
  x: number;
  y: number;
  onClose: () => void;
  onUpdated: () => void;
}

export default function ContextMenu({ game, x, y, onClose, onUpdated }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [showTagInput, setShowTagInput] = useState(false);
  const [tagInput, setTagInput] = useState('');

  const adjustedX = Math.min(x, window.innerWidth - 220);
  const adjustedY = Math.min(y, window.innerHeight - 420);

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
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onClose]);

  const handleStar = async () => {
    try { await ToggleGameStar(game.id); onUpdated(); } catch { /* ignore */ }
    onClose();
  };

  const handleOpenDir = async () => {
    try { await OpenGameDirectory(game.id); } catch { /* ignore */ }
    onClose();
  };

  const handleOpenMeta = async () => {
    try { await OpenGameMetadata(game.id); } catch { /* ignore */ }
    onClose();
  };

  const handleLaunch = async () => {
    try { await LaunchGame(game.id); } catch { /* ignore */ }
    onClose();
  };

  const handleReScrape = async () => {
    try { await ScrapeGame(game.id); onUpdated(); } catch { /* ignore */ }
    onClose();
  };

  const handleAddTag = async () => {
    const tag = tagInput.trim();
    if (!tag) { setShowTagInput(false); return; }
    try { await AddGameTag(game.id, tag); onUpdated(); } catch { /* ignore */ }
    setTagInput('');
    setShowTagInput(false);
  };

  const handleRemoveTag = async (tag: string) => {
    try { await RemoveGameTag(game.id, tag); onUpdated(); } catch { /* ignore */ }
  };

  const openPage = (url: string) => {
    if (url) {
      try { (window as any).wails?.Browser?.OpenURL?.(url); } catch { /* fallback */ }
      window.open(url, '_blank');
    }
  };

  const platforms: Array<{platform: string, id: string}> = (game as any).platforms || [];
  const hasExe = (game.executables || []).length > 0;

  return (
    <div
      ref={menuRef}
      className="context-menu"
      style={{ left: adjustedX, top: adjustedY }}
    >
      <div className="context-menu-section">
        <div className="context-menu-title">{game.title}</div>
      </div>

      <button className="context-item" onClick={handleStar}>
        <span className="context-item-icon">{game.starred ? '\u2605' : '\u2606'}</span>
        <span>{game.starred ? 'Unstar' : 'Star'}</span>
      </button>

      <div className="context-divider" />

      {hasExe && (
        <button className="context-item" onClick={handleLaunch}>
          <span className="context-item-icon">{'\u25B6'}</span>
          <span>Launch Game</span>
        </button>
      )}

      <button className="context-item" onClick={handleReScrape}>
        <span className="context-item-icon">{'\u21BB'}</span>
        <span>Re-scrape Metadata</span>
      </button>

      {platforms.length > 0 && <div className="context-divider" />}

      {platforms.map((p) => {
        let url = '';
        if (p.platform === 'steam') url = `https://store.steampowered.com/app/${p.id}/`;
        else if (p.platform === 'dlsite') url = `https://www.dlsite.com/maniax/work/=/product_id/${p.id}.html`;
        else if (p.platform === 'bangumi') url = `https://bgm.tv/subject/${p.id}`;
        return url ? (
          <button key={p.platform} className="context-item" onClick={() => openPage(url)}>
            <span className="context-item-icon">{'\uD83D\uDD17'}</span>
            <span>Open {p.platform.charAt(0).toUpperCase() + p.platform.slice(1)} Page</span>
          </button>
        ) : null;
      })}

      <div className="context-divider" />

      {!showTagInput ? (
        <button className="context-item" onClick={() => setShowTagInput(true)}>
          <span className="context-item-icon">+</span>
          <span>Add Tag</span>
        </button>
      ) : (
        <div className="context-tag-input">
          <input
            autoFocus
            type="text"
            placeholder="Tag name..."
            value={tagInput}
            onChange={(e) => setTagInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleAddTag();
              if (e.key === 'Escape') setShowTagInput(false);
            }}
          />
          <button onClick={handleAddTag}>Add</button>
        </div>
      )}

      {(game.tags && game.tags.length > 0) && (
        <div className="context-tags">
          {game.tags.map((tag) => (
            <span key={tag} className="context-tag-chip">
              {tag}
              <button
                className="context-tag-remove"
                onClick={() => handleRemoveTag(tag)}
                title="Remove tag"
              >
                &times;
              </button>
            </span>
          ))}
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
