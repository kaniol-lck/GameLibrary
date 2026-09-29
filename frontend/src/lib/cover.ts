/**
 * Cover art URLs.
 *
 * Covers used to be fetched through the Wails binding layer, where every call read
 * the image file, base64 encoded it and pushed the string across the JS bridge —
 * inflating each image by a third and turning a grid of N cards into N full size
 * transfers. The backend now serves them over HTTP from the asset server, so the
 * webview can fetch and cache them like any other asset.
 *
 * The version parameter changes whenever the artwork is rewritten, which is what
 * makes a freshly scraped cover appear instead of a cached one.
 */

export type CoverKind = "cover" | "landscape";

/** Builds the URL for a game's cover art, or "" when it has none. */
export function coverUrl(gameId: string, kind: CoverKind = "cover", coverVersion?: number): string {
  if (!gameId) {
    return "";
  }
  const base = `/covers/${encodeURIComponent(gameId)}/${kind}`;
  return coverVersion ? `${base}?v=${coverVersion}` : base;
}

/**
 * Reports whether the artwork actually changed since the last render.
 *
 * Cover fallbacks are handled by the browser's own onError, so there is no need
 * to probe for the file from JavaScript.
 */
export function coverKey(gameId: string, coverVersion?: number): string {
  return `${gameId}:${coverVersion ?? 0}`;
}
