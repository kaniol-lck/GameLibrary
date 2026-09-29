import type { game } from "../../wailsjs/go/models";
import { gamePlatforms, isUnmatched } from "./platform.ts";

/**
 * Navigation selection.
 *
 * The selected view used to be a magic string ("platform:steam",
 * "pathlabel:Steam"), parsed back with `slice(9)`, `slice(5)`, `slice(4)`,
 * `slice(8)` and `slice(10)` at eight different call sites — and two of those
 * branches (`type:`) were unreachable because the sidebar never produced them.
 * A discriminated union removes the offset arithmetic entirely.
 */
export type NavKey =
  | { kind: "all" }
  | { kind: "starred" }
  | { kind: "unmatched" }
  | { kind: "settings" }
  | { kind: "platform"; id: string }
  | { kind: "genre"; id: string }
  | { kind: "usertag"; id: string }
  | { kind: "folder"; id: string };

/** A stable string form, for React keys and equality checks. */
export function navKeyId(nav: NavKey): string {
  return nav.kind === "platform" ||
    nav.kind === "genre" ||
    nav.kind === "usertag" ||
    nav.kind === "folder"
    ? `${nav.kind}:${nav.id}`
    : nav.kind;
}

/** Parses the string form produced by navKeyId. */
export function parseNavKey(value: string): NavKey {
  const separator = value.indexOf(":");
  if (separator < 0) {
    switch (value) {
      case "starred":
        return { kind: "starred" };
      case "unmatched":
        return { kind: "unmatched" };
      case "settings":
        return { kind: "settings" };
      default:
        return { kind: "all" };
    }
  }
  const kind = value.slice(0, separator);
  const id = value.slice(separator + 1);
  switch (kind) {
    case "platform":
      return { kind: "platform", id };
    case "genre":
      return { kind: "genre", id };
    case "usertag":
      return { kind: "usertag", id };
    case "folder":
      return { kind: "folder", id };
    default:
      return { kind: "all" };
  }
}

/** Context the filters need beyond the game list itself. */
export interface FilterContext {
  /** Game ID -> labels of the configured directory it lives in, from the backend. */
  pathLabels: Record<string, string[]>;
}

/** Applies the current navigation selection to the game list. */
export function filterGames(
  games: game.GameInfo[],
  nav: NavKey,
  context: FilterContext,
): game.GameInfo[] {
  switch (nav.kind) {
    case "all":
      // "All games" deliberately excludes unmatched entries; they have their own
      // view so the main grid stays meaningful.
      return games.filter((g) => !isUnmatched(g));
    case "starred":
      return games.filter((g) => g.starred);
    case "unmatched":
      return games.filter(isUnmatched);
    case "platform":
      return games.filter((g) => gamePlatforms(g).some((p) => p.platform === nav.id));
    case "genre":
      return games.filter((g) => (g.metadata?.tags ?? []).includes(nav.id));
    case "usertag":
      return games.filter((g) => (g.tags ?? []).includes(nav.id));
    case "folder":
      return games.filter((g) => (context.pathLabels[g.id] ?? []).includes(nav.id));
    case "settings":
      return [];
  }
}

/** Human-readable title for the current view. */
export function navTitle(nav: NavKey): string {
  switch (nav.kind) {
    case "all":
      return "All Games";
    case "starred":
      return "Starred";
    case "unmatched":
      return "Unmatched";
    case "settings":
      return "Settings";
    case "platform":
      return nav.id.charAt(0).toUpperCase() + nav.id.slice(1);
    case "usertag":
      return `#${nav.id}`;
    default:
      return nav.id;
  }
}

/** Counts used to build the sidebar sections. */
export interface NavCounts {
  all: number;
  starred: number;
  unmatched: number;
  platforms: { id: string; count: number }[];
  genres: { id: string; count: number }[];
  userTags: { id: string; count: number }[];
  folders: { id: string; count: number }[];
}

/**
 * Derives every count the sidebar shows in a single pass.
 *
 * The sidebar used to walk the whole library once per section on every render,
 * including while a scrape was streaming in.
 */
export function deriveCounts(games: game.GameInfo[], context: FilterContext): NavCounts {
  const platforms = new Map<string, number>();
  const genres = new Map<string, number>();
  const userTags = new Map<string, number>();
  const folders = new Map<string, number>();

  let starred = 0;
  let unmatched = 0;

  for (const info of games) {
    if (info.starred) {
      starred++;
    }
    const gamePlatformList = gamePlatforms(info);
    if (gamePlatformList.length === 0) {
      unmatched++;
    } else {
      for (const platform of gamePlatformList) {
        if (platform.platform) {
          platforms.set(platform.platform, (platforms.get(platform.platform) ?? 0) + 1);
        }
      }
    }
    for (const genre of info.metadata?.tags ?? []) {
      genres.set(genre, (genres.get(genre) ?? 0) + 1);
    }
    for (const tag of info.tags ?? []) {
      userTags.set(tag, (userTags.get(tag) ?? 0) + 1);
    }
    for (const label of context.pathLabels[info.id] ?? []) {
      folders.set(label, (folders.get(label) ?? 0) + 1);
    }
  }

  return {
    all: games.length - unmatched,
    starred,
    unmatched,
    platforms: sortedCounts(platforms),
    genres: sortedCounts(genres),
    userTags: sortedCounts(userTags),
    folders: sortedCounts(folders),
  };
}

function sortedCounts(counts: Map<string, number>): { id: string; count: number }[] {
  return [...counts.entries()]
    .map(([id, count]) => ({ id, count }))
    .sort((a, b) => (b.count === a.count ? a.id.localeCompare(b.id) : b.count - a.count));
}

/** Reports whether a game already has the metadata the detail panel displays. */
export function needsScrape(info: game.GameInfo): boolean {
  const metadata = info.metadata;
  if (!metadata) {
    return true;
  }
  return (
    !metadata.description || !metadata.releaseDate || (!metadata.developer && !metadata.publisher)
  );
}
