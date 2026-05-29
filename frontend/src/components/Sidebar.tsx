import { game } from '../../wailsjs/go/models';

interface Category {
  key: string;
  label: string;
  count: number;
}

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
  games: game.GameInfo[];
  selectedNav: string;
  onSelectNav: (key: string) => void;
  machineName: string;
}

function deriveCategories(games: game.GameInfo[]): Category[] {
  const map = new Map<string, number>();

  for (const g of games) {
    const tags = g.metadata?.tags || [];
    if (tags.length > 0) {
      for (const tag of tags) {
        const key = `tag:${tag}`;
        map.set(key, (map.get(key) || 0) + 1);
      }
    }
    const typeKey = `type:${g.type}`;
    map.set(typeKey, (map.get(typeKey) || 0) + 1);
  }

  return Array.from(map.entries())
    .sort((a, b) => b[1] - a[1])
    .map(([key, count]) => ({
      key,
      label: key.startsWith('tag:') ? key.slice(4) : key.slice(5),
      count,
    }));
}

export default function Sidebar({
  collapsed,
  onToggle,
  games,
  selectedNav,
  onSelectNav,
  machineName,
}: SidebarProps) {
  const categories = deriveCategories(games);
  const allCount = games.length;

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

        {categories.length > 0 && !collapsed && (
          <div className="sidebar-divider" />
        )}

        {categories.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Categories</div>
        )}

        {categories.map((cat) => (
          <button
            key={cat.key}
            className={`sidebar-item ${selectedNav === cat.key ? 'active' : ''}`}
            onClick={() => onSelectNav(cat.key)}
          >
            <span className="sidebar-item-icon">
              {cat.key.startsWith('type:') ? '\u25A0' : '\u25C9'}
            </span>
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
