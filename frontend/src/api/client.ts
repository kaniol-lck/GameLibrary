/**
 * The single place the UI talks to the backend.
 *
 * Generated Wails bindings were imported directly by six components, which meant
 * seven backend mutations were implemented twice (once in the detail panel, once
 * in the context menu), `GetGameList` had two separate call sites with two
 * different error behaviours, and there was nowhere to normalise errors. This
 * module is the only file that imports `wailsjs/go/main/App`.
 */
import * as Backend from "../../wailsjs/go/main/App";
import { taskqueue } from "../../wailsjs/go/models";
import type { config, game, main, scanner } from "../../wailsjs/go/models";

/** Application and build information. */
export type AppInfo = main.AppInfo;

/** Outcome of scraping one game. */
export type ScrapeReport = main.ScrapeReport;

/** A detected Steam account. */
export type SteamUser = main.SteamUserInfo;

/** Current queue state. */
export type QueueStatus = taskqueue.Status;

/** Configuration model. */
export type AppConfig = config.Config;

/** A game record. */
export type Game = game.GameInfo;

/** Result of one directory scan. */
export type ScanResult = scanner.ScanResult;

/**
 * An empty queue snapshot.
 *
 * The generated model is a class, so a plain object literal does not satisfy it;
 * building it through the class keeps the initial state exactly the shape the
 * backend sends.
 */
export function emptyQueueStatus(): QueueStatus {
  return taskqueue.Status.createFrom({
    pending: 0,
    running: 0,
    paused: false,
    concurrency: 1,
    runningTitles: [],
    pendingTitles: [],
    completed: 0,
    failed: 0,
    total: 0,
    failures: [],
  });
}

export const api = {
  // --- library ---
  listGames: (): Promise<Game[]> => Backend.GetGameList(),
  getGame: (id: string): Promise<Game | null> => Backend.GetGame(id),
  getPathLabels: (): Promise<Record<string, string[]>> => Backend.GetGamePathLabels(),
  commonPaths: (): Promise<Record<string, string>> => Backend.CommonPaths(),

  // --- scanning and scraping ---
  scan: (): Promise<ScanResult[]> => Backend.ScanGames(),
  forceScan: (): Promise<ScanResult[]> => Backend.ForceScanGames(),
  scrapeGame: (id: string): Promise<ScrapeReport> => Backend.ScrapeGame(id),
  queueScrapeAll: (force: boolean): Promise<number> => Backend.QueueScrapeAll(force),

  // --- queue ---
  queueStatus: (): Promise<QueueStatus> => Backend.GetQueueInfo(),
  queuePause: (): Promise<void> => Backend.QueuePause(),
  queueResume: (): Promise<void> => Backend.QueueResume(),
  queueClear: (): Promise<void> => Backend.QueueClear(),
  queueResetProgress: (): Promise<void> => Backend.QueueResetProgress(),
  queueSetConcurrency: (n: number): Promise<void> => Backend.QueueSetConcurrency(n),

  // --- game actions ---
  launch: (id: string): Promise<void> => Backend.LaunchGame(id),
  toggleStar: (id: string): Promise<void> => Backend.ToggleGameStar(id),
  addTag: (id: string, tag: string): Promise<void> => Backend.AddGameTag(id, tag),
  removeTag: (id: string, tag: string): Promise<void> => Backend.RemoveGameTag(id, tag),
  setPreferredSource: (id: string, source: string): Promise<void> =>
    Backend.SetPreferredSource(id, source),
  setPrimaryExecutable: (id: string, execPath: string): Promise<void> =>
    Backend.SetPrimaryExecutable(id, execPath),
  openGameDirectory: (id: string): Promise<void> => Backend.OpenGameDirectory(id),
  openGameMetadata: (id: string): Promise<void> => Backend.OpenGameMetadata(id),
  openDirectory: (dir: string): Promise<void> => Backend.OpenDirectory(dir),
  openBrowser: (url: string): Promise<void> => Backend.OpenBrowser(url),

  // --- settings ---
  getConfig: (): Promise<AppConfig> => Backend.GetConfig(),
  saveConfig: (cfg: AppConfig): Promise<void> => Backend.SaveConfig(cfg),
  pickGameDirectory: (): Promise<string> => Backend.PickGameDirectory(),
  getSteamUsers: (): Promise<SteamUser[]> => Backend.GetSteamUsers(),
  setSteamUser: (id: string): Promise<void> => Backend.SetSteamUser(id),
  getMachineName: (): Promise<string> => Backend.GetMachineName(),
  getAppInfo: (): Promise<AppInfo> => Backend.GetAppInfo(),
};
