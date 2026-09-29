import type { game } from "../../wailsjs/go/models";

/**
 * Platform presentation metadata.
 *
 * The label/colour/URL mapping used to be duplicated across GameCard, GameDetail,
 * Sidebar and ContextMenu, with the palette hardcoded in two of them and VNDB
 * missing from one URL builder. This is now the single source of truth.
 */
export interface PlatformMeta {
  /** Display name for badges and the sidebar. */
  label: string;
  /** Badge background colour. */
  color: string;
  /** Emoji shown in compact listings. */
  icon: string;
  /** Builds the public URL for a platform-specific identifier. */
  url: (id: string) => string;
}

const PLATFORMS: Record<string, PlatformMeta> = {
  steam: {
    label: "Steam",
    color: "#1a4b8a",
    icon: "🎮",
    url: (id) => `https://store.steampowered.com/app/${encodeURIComponent(id)}/`,
  },
  vndb: {
    label: "VNDB",
    color: "#2255a4",
    icon: "📖",
    url: (id) => `https://vndb.org/${encodeURIComponent(id)}`,
  },
  bangumi: {
    label: "Bangumi",
    color: "#c2185b",
    icon: "🌸",
    url: (id) => `https://bgm.tv/subject/${encodeURIComponent(id)}`,
  },
  dlsite: {
    label: "DLsite",
    color: "#e57399",
    icon: "🎧",
    url: (id) => `https://www.dlsite.com/maniax/work/=/product_id/${encodeURIComponent(id)}.html`,
  },
  rawg: {
    label: "RAWG",
    color: "#4f6ef7",
    icon: "🗂️",
    url: (id) => `https://rawg.io/games/${encodeURIComponent(id)}`,
  },
  steamgriddb: {
    label: "SteamGridDB",
    color: "#555555",
    icon: "🖼️",
    url: () => "",
  },
};

const FALLBACK: PlatformMeta = {
  label: "",
  color: "#555555",
  icon: "🏷️",
  url: () => "",
};

/** Returns presentation metadata for a platform key. */
export function platformMeta(platform: string): PlatformMeta {
  const key = platform.toLowerCase();
  const known = PLATFORMS[key];
  if (known) {
    return known;
  }
  if (!platform) {
    return FALLBACK;
  }
  return { ...FALLBACK, label: platform.charAt(0).toUpperCase() + platform.slice(1) };
}

/** Returns the platforms linked to a game, tolerating a null list. */
export function gamePlatforms(info: game.GameInfo): game.PlatformInfo[] {
  return info.platforms ?? [];
}

/** Reports whether a game has no metadata source attached yet. */
export function isUnmatched(info: game.GameInfo): boolean {
  return gamePlatforms(info).length === 0;
}

/** Returns every non-empty ID a game is linked to, for a platform. */
export function platformId(info: game.GameInfo, platform: string): string {
  const match = gamePlatforms(info).find((p) => p.platform === platform);
  return match?.id ?? "";
}

/**
 * Builds the best external URL for a game, preferring the highest priority
 * platform it is linked to.
 */
export function primaryGameUrl(info: game.GameInfo): string {
  for (const candidate of gamePlatforms(info)) {
    const url = platformMeta(candidate.platform).url(candidate.id ?? "");
    if (url) {
      return url;
    }
  }
  const links = info.metadata?.links ?? {};
  for (const key of ["steam", "vndb", "bangumi", "dlsite", "rawg", "website"]) {
    if (links[key]) {
      return links[key];
    }
  }
  return "";
}
