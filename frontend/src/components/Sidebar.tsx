import { game } from '../../wailsjs/go/models';

interface Category {
  key: string;
  label: string;
  count: number;
  section: 'platform' | 'genre' | 'user' | 'path';
}

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
  games: game.GameInfo[];
  selectedNav: string;
  onSelectNav: (key: string) => void;
  machineName: string;
  pathLabels?: Record<string, string[]>;
}

function deriveCategories(games: game.GameInfo[], pathLabels: Record<string, string[]> | undefined): Category[] {
  const cats: Category[] = [];

  const platformCounts = new Map<string, number>();
  const genreCounts = new Map<string, number>();
  const userCounts = new Map<string, number>();
  const pathCounts = new Map<string, number>();
  let unmatchedCount = 0;

  for (const g of games) {
    const plats: any[] = (g as any).platforms || [];
    if (plats.length === 0) {
      unmatchedCount++;
    } else {
      for (const p of plats) {
        platformCounts.set(p.platform, (platformCounts.get(p.platform) || 0) + 1);
      }
    }

    for (const tag of g.metadata?.tags || []) {
      genreCounts.set(tag, (genreCounts.get(tag) || 0) + 1);
    }

    for (const tag of g.tags || []) {
      userCounts.set(tag, (userCounts.get(tag) || 0) + 1);
    }

    if (pathLabels) {
      const gameDir = ((g as any).gameDir || '').replace(/\\/g, '/');
      for (const [dirPath, labels] of Object.entries(pathLabels)) {
        if (gameDir.startsWith(dirPath.replace(/\\/g, '/'))) {
          for (const l of (labels as string[])) {
            if (l) pathCounts.set(l, (pathCounts.get(l) || 0) + 1);
          }
        }
      }
    }
  }

  if (unmatchedCount > 0) {
    cats.push({ key: 'platform:unmatched', label: 'Unmatched', count: unmatchedCount, section: 'platform' });
  }

  if (platformCounts.size > 0) {
    for (const [key, count] of [...platformCounts].sort((a, b) => b[1] - a[1])) {
      cats.push({ key: `platform:${key}`, label: platformLabel(key), count, section: 'platform' });
    }
  }

  for (const [key, count] of [...genreCounts].sort((a, b) => b[1] - a[1])) {
    cats.push({ key: `tag:${key}`, label: key, count, section: 'genre' });
  }

  for (const [key, count] of [...userCounts].sort((a, b) => b[1] - a[1])) {
    cats.push({ key: `usertag:${key}`, label: key, count, section: 'user' });
  }

  for (const [key, count] of [...pathCounts].sort((a, b) => b[1] - a[1])) {
    cats.push({ key: `pathlabel:${key}`, label: key, count, section: 'path' });
  }

  return cats;
}

function platformLabel(plat: string): string {
  switch (plat) {
    case 'steam': return 'Steam';
    case 'dlsite': return 'DLsite';
    case 'vndb': return 'VNDB';
    case 'bangumi': return 'Bangumi';
    default: return plat;
  }
}

function platformIcon(plat: string): string {
  switch (plat) {
    case 'steam': return '\u25A0';
    case 'dlsite': return '\u25C6';
    case 'vndb': return '\u25B6';
    case 'bangumi': return '\u25CF';
    case 'unmatched': return '\u25CB';
    default: return '\u25A0';
  }
}

export default function Sidebar({
  collapsed,
  onToggle,
  games,
  selectedNav,
  onSelectNav,
  machineName,
  pathLabels,
}: SidebarProps) {
  const categories = deriveCategories(games, pathLabels);
  const allCount = games.length;
  const starredCount = games.filter((g) => g.starred).length;

  const platforms = categories.filter((c) => c.section === 'platform');
  const genres = categories.filter((c) => c.section === 'genre');
  const userTags = categories.filter((c) => c.section === 'user');
  const pathTags = categories.filter((c) => c.section === 'path');

  return (
    <aside className={`sidebar ${collapsed ? 'sidebar-collapsed' : ''}`}>
      <div className="sidebar-top">
        <div className="sidebar-brand">
          <span className="sidebar-logo">GL</span>
          {!collapsed && <span className="sidebar-title">GameLibrary</span>}
        </div>
        <button
          className="sidebar-toggle"
          onClick={onToggle}
          title={collapsed ? 'Expand' : 'Collapse'}
        >
          {collapsed ? '\u25B6' : '\u25C0'}
        </button>
      </div>

      <nav className="sidebar-nav">
        <button
          className={`sidebar-item ${selectedNav === 'all' ? 'active' : ''}`}
          onClick={() => onSelectNav('all')}
        >
          <span className="sidebar-item-icon">&#9783;</span>
          {!collapsed && (
            <>
              <span className="sidebar-item-label">All Games</span>
              <span className="sidebar-item-badge">{allCount}</span>
            </>
          )}
        </button>

        {starredCount > 0 && (
          <button
            className={`sidebar-item ${selectedNav === 'starred' ? 'active' : ''}`}
            onClick={() => onSelectNav('starred')}
          >
            <span className="sidebar-item-icon">{'\u2605'}</span>
            {!collapsed && (
              <>
                <span className="sidebar-item-label">Starred</span>
                <span className="sidebar-item-badge">{starredCount}</span>
              </>
            )}
          </button>
        )}

        {platforms.length > 0 && !collapsed && (
          <div className="sidebar-divider" />
        )}
        {platforms.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Platforms</div>
        )}
        {platforms.map((cat) => (
          <button
            key={cat.key}
            className={`sidebar-item ${selectedNav === cat.key ? 'active' : ''}`}
            onClick={() => onSelectNav(cat.key)}
          >
            <span className="sidebar-item-icon">{platformIcon(cat.key.slice(9))}</span>
            {!collapsed && (
              <>
                <span className="sidebar-item-label">{cat.label}</span>
                <span className="sidebar-item-badge">{cat.count}</span>
              </>
            )}
          </button>
        ))}

        {genres.length > 0 && !collapsed && (
          <div className="sidebar-divider" />
        )}
        {genres.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Genres</div>
        )}
        {genres.map((cat) => (
          <button
            key={cat.key}
            className={`sidebar-item ${selectedNav === cat.key ? 'active' : ''}`}
            onClick={() => onSelectNav(cat.key)}
          >
            <span className="sidebar-item-icon">{'\u25C9'}</span>
            {!collapsed && (
              <>
                <span className="sidebar-item-label">{cat.label}</span>
                <span className="sidebar-item-badge">{cat.count}</span>
              </>
            )}
          </button>
        ))}

        {userTags.length > 0 && !collapsed && (
          <div className="sidebar-divider" />
        )}
        {userTags.length > 0 && !collapsed && (
          <div className="sidebar-section-label">My Tags</div>
        )}
        {userTags.map((cat) => (
          <button
            key={cat.key}
            className={`sidebar-item ${selectedNav === cat.key ? 'active' : ''}`}
            onClick={() => onSelectNav(cat.key)}
          >
            <span className="sidebar-item-icon">#</span>
            {!collapsed && (
              <>
                <span className="sidebar-item-label">{cat.label}</span>
                <span className="sidebar-item-badge">{cat.count}</span>
              </>
            )}
          </button>
        ))}

        {pathTags.length > 0 && !collapsed && (
          <div className="sidebar-divider" />
        )}
        {pathTags.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Paths</div>
        )}
        {pathTags.map((cat) => (
          <button
            key={cat.key}
            className={`sidebar-item ${selectedNav === cat.key ? 'active' : ''}`}
            onClick={() => onSelectNav(cat.key)}
          >
            <span className="sidebar-item-icon">{'\uD83D\uDCC1'}</span>
            {!collapsed && (
              <>
                <span className="sidebar-item-label">{cat.label}</span>
                <span className="sidebar-item-badge">{cat.count}</span>
              </>
            )}
          </button>
        ))}

        {collapsed && allCount > 0 && (
          <div className="sidebar-collapsed-badge">{allCount}</div>
        )}
      </nav>

      <div className="sidebar-bottom">
        {!collapsed && (
          <span className="sidebar-machine">{machineName}</span>
        )}
        <button
          className={`sidebar-item sidebar-item-settings ${selectedNav === 'settings' ? 'active' : ''}`}
          onClick={() => onSelectNav('settings')}
        >
          <span className="sidebar-item-icon">&#9881;</span>
          {!collapsed && <span className="sidebar-item-label">Settings</span>}
        </button>
      </div>
    </aside>
  );
}
