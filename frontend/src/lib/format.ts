/** Formats a playtime in minutes for display. */
export function formatPlaytime(minutes: number | undefined): string {
  if (!minutes || minutes <= 0) {
    return "";
  }
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest > 0 ? `${hours}h ${rest}m` : `${hours}h`;
}

/** Formats an RFC3339 timestamp for display, or "" when absent/invalid. */
export function formatDate(value: string | undefined): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleDateString();
}

/**
 * Normalises a backend error into something worth showing.
 *
 * Wails rejects the promise with the Go error's message, which is already
 * reasonably descriptive; this strips the redundant prefix and never yields
 * "undefined".
 */
export function errorMessage(error: unknown): string {
  if (error == null) {
    return "Unknown error";
  }
  const raw = error instanceof Error ? error.message : String(error);
  const cleaned = raw.replace(/^Error:\s*/, "").trim();
  return cleaned === "" ? "Unknown error" : cleaned;
}
