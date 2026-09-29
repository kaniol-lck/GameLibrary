import type { QueueStatus } from "@/api/client";
import { navKeyId, type NavCounts, type NavKey } from "@/lib/filters";
import { platformMeta } from "@/lib/platform";
import { hasQueueActivity } from "@/lib/queue";
import QueuePanel from "./QueuePanel";

export interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
  /** Pre-aggregated counts from `deriveCounts`; the sidebar never walks games. */
  counts: NavCounts;
  selected: string;
  onSelect: (nav: NavKey) => void;
  machineName: string;
  version: string;
  queueStatus: QueueStatus;
  /** Re-scrapes one game; used by the retry button on a queued failure. */
  onQueueRetry?: (gameId: string) => void;
}

interface SidebarItemProps {
  nav: NavKey;
  icon: string;
  label: string;
  count: number;
  collapsed: boolean;
  selected: string;
  onSelect: (nav: NavKey) => void;
}

function SidebarItem({ nav, icon, label, count, collapsed, selected, onSelect }: SidebarItemProps) {
  const id = navKeyId(nav);
  return (
    <button
      type="button"
      className={`sidebar-item${id === selected ? " active" : ""}`}
      onClick={() => onSelect(nav)}
      title={label}
    >
      <span className="sidebar-item-icon">{icon}</span>
      {!collapsed && (
        <>
          <span className="sidebar-item-label">{label}</span>
          <span className="sidebar-item-badge">{count}</span>
        </>
      )}
    </button>
  );
}

/**
 * Navigation built entirely from the counts the app derived once per library
 * change. It used to re-aggregate every game on each render, including one pass
 * per scraped game as the queue streamed updates in.
 */
export default function Sidebar({
  collapsed,
  onToggle,
  counts,
  selected,
  onSelect,
  machineName,
  version,
  queueStatus,
  onQueueRetry,
}: SidebarProps) {
  return (
    <aside className={`sidebar ${collapsed ? "sidebar-collapsed" : ""}`}>
      <div className="sidebar-top">
        <div className="sidebar-brand">
          <span className="sidebar-logo">GL</span>
          {!collapsed && <span className="sidebar-title">GameLibrary</span>}
        </div>
        <button
          type="button"
          className="sidebar-toggle"
          onClick={onToggle}
          title={collapsed ? "Expand" : "Collapse"}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          aria-expanded={!collapsed}
        >
          {collapsed ? "\u25B6" : "\u25C0"}
        </button>
      </div>

      <nav className="sidebar-nav">
        <SidebarItem
          nav={{ kind: "all" }}
          icon={"\u2637"}
          label="All Games"
          count={counts.all}
          collapsed={collapsed}
          selected={selected}
          onSelect={onSelect}
        />
        {counts.starred > 0 && (
          <SidebarItem
            nav={{ kind: "starred" }}
            icon={"\u2605"}
            label="Starred"
            count={counts.starred}
            collapsed={collapsed}
            selected={selected}
            onSelect={onSelect}
          />
        )}
        {counts.unmatched > 0 && (
          <SidebarItem
            nav={{ kind: "unmatched" }}
            icon={"\u25CB"}
            label="Unmatched"
            count={counts.unmatched}
            collapsed={collapsed}
            selected={selected}
            onSelect={onSelect}
          />
        )}

        {counts.platforms.length > 0 && !collapsed && <div className="sidebar-divider" />}
        {counts.platforms.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Platforms</div>
        )}
        {counts.platforms.map((platform) => {
          const meta = platformMeta(platform.id);
          return (
            <SidebarItem
              key={`platform:${platform.id}`}
              nav={{ kind: "platform", id: platform.id }}
              icon={meta.icon}
              label={meta.label || platform.id}
              count={platform.count}
              collapsed={collapsed}
              selected={selected}
              onSelect={onSelect}
            />
          );
        })}

        {counts.genres.length > 0 && !collapsed && <div className="sidebar-divider" />}
        {counts.genres.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Genres</div>
        )}
        {counts.genres.map((genre) => (
          <SidebarItem
            key={`genre:${genre.id}`}
            nav={{ kind: "genre", id: genre.id }}
            icon={"\u25C9"}
            label={genre.id}
            count={genre.count}
            collapsed={collapsed}
            selected={selected}
            onSelect={onSelect}
          />
        ))}

        {counts.folders.length > 0 && !collapsed && <div className="sidebar-divider" />}
        {counts.folders.length > 0 && !collapsed && (
          <div className="sidebar-section-label">Folders</div>
        )}
        {counts.folders.map((folder) => (
          <SidebarItem
            key={`folder:${folder.id}`}
            nav={{ kind: "folder", id: folder.id }}
            icon={"\uD83D\uDCC1"}
            label={folder.id}
            count={folder.count}
            collapsed={collapsed}
            selected={selected}
            onSelect={onSelect}
          />
        ))}

        {counts.userTags.length > 0 && !collapsed && <div className="sidebar-divider" />}
        {counts.userTags.length > 0 && !collapsed && (
          <div className="sidebar-section-label">My Tags</div>
        )}
        {counts.userTags.map((tag) => (
          <SidebarItem
            key={`usertag:${tag.id}`}
            nav={{ kind: "usertag", id: tag.id }}
            icon="#"
            label={tag.id}
            count={tag.count}
            collapsed={collapsed}
            selected={selected}
            onSelect={onSelect}
          />
        ))}

        {collapsed && counts.all > 0 && <div className="sidebar-collapsed-badge">{counts.all}</div>}
      </nav>

      <div className="sidebar-bottom">
        {!collapsed && (
          <span className="sidebar-machine" title={machineName}>
            {machineName}
            {version !== "" ? ` · v${version}` : ""}
          </span>
        )}

        {!collapsed && hasQueueActivity(queueStatus) && (
          <QueuePanel status={queueStatus} onRetry={onQueueRetry} />
        )}

        <button
          type="button"
          className={`sidebar-item sidebar-item-settings${
            selected === navKeyId({ kind: "settings" }) ? " active" : ""
          }`}
          onClick={() => onSelect({ kind: "settings" })}
          title="Settings"
        >
          <span className="sidebar-item-icon">{"\u2699"}</span>
          {!collapsed && <span className="sidebar-item-label">Settings</span>}
        </button>
      </div>
    </aside>
  );
}
