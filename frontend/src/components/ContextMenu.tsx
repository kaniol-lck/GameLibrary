import { useState, useRef, useEffect, useCallback } from 'react';
import { game } from '../../wailsjs/go/models';
import { ToggleGameStar, AddGameTag, RemoveGameTag, OpenGameDirectory, OpenGameMetadata } from '../../wailsjs/go/main/App';

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
  const adjustedY = Math.min(y, window.innerHeight - 360);

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
