package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"GameLibrary/internal/config"
	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
	"GameLibrary/internal/logger"
	"GameLibrary/internal/platform"
	"GameLibrary/internal/scraper"
	"GameLibrary/internal/taskqueue"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ScrapeReport is the outcome of scraping one game.
type ScrapeReport struct {
	GameID  string   `json:"gameId"`
	Title   string   `json:"title"`
	Source  string   `json:"source,omitempty"`
	Sources []string `json:"sources,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// ScanGames identifies games without re-identifying the ones already on disk.
func (a *App) ScanGames() []ScanResult {
	return a.doScan(false)
}

// ForceScanGames re-identifies every game directory.
//
// This used to be unreachable: the button labelled "Force Scan" called ScanGames,
// so the exported method existed but nothing ever invoked it.
func (a *App) ForceScanGames() []ScanResult {
	return a.doScan(true)
}

func (a *App) doScan(force bool) []ScanResult {
	state := a.snapshot()
	if state == nil {
		return []ScanResult{{Error: "application is still starting up"}}
	}

	a.scanMu.Lock()
	defer a.scanMu.Unlock()

	var (
		results []ScanResult
		err     error
	)
	if force {
		results, err = state.scanner.ForceScanAll()
	} else {
		results, err = state.scanner.ScanAll()
	}
	if err != nil {
		logger.Error("scan failed", "error", err.Error())
		return []ScanResult{{Error: "scan failed: " + err.Error()}}
	}

	// The scan visited every configured directory, so its output is a complete
	// picture of the library. Rebuilding the cache from it avoids walking the
	// tree a second time, which matters on a network share.
	infos := make([]*game.GameInfo, 0, len(results))
	for _, result := range results {
		if result.GameInfo != nil {
			infos = append(infos, result.GameInfo)
		}
	}
	a.library.ReplaceAll(infos)

	a.autoScrapeNew(results)
	a.emit(eventScanComplete, map[string]any{"total": len(results)})
	return results
}

// scrapeOne runs the full scrape pipeline for a single game and persists the
// result. A nil error means metadata was written.
func (a *App) scrapeOne(ctx context.Context, gameID string, forceCover bool) ScrapeReport {
	report := ScrapeReport{GameID: gameID}

	state := a.snapshot()
	if state == nil {
		report.Error = "application is still starting up"
		return report
	}

	info, ok := a.library.Get(gameID)
	if !ok {
		logger.ScrapeGameNotFound(gameID)
		report.Error = "game not found"
		return report
	}
	report.Title = info.Title

	results, err := state.pipeline.ScrapeAll(ctx, info)
	if err != nil {
		report.Error = describeScrapeError(err)
		return report
	}

	// The preferred source wins; otherwise the first provider that matched does.
	primary := results[0]
	for _, candidate := range results {
		if candidate.Source == info.PreferredSource {
			primary = candidate
			break
		}
	}

	scraper.ApplyResult(info, primary.Result, primary.Source)

	// Every other match contributes a platform link and its title variants, so a
	// game present on several services stays linked to all of them.
	for _, candidate := range results {
		if candidate.Source == primary.Source {
			continue
		}
		info.SetPlatform(candidate.Source, candidate.Result.Links[scraper.PlatformIDKey], candidate.Result.Title)
		info.AddAlias(candidate.Result.Title)
		info.AddAlias(candidate.Result.TitleNative)
	}

	for _, candidate := range results {
		report.Sources = append(report.Sources, candidate.Source)
		// Per-source snapshots make it possible to see what each provider said.
		_ = info.SaveMeta(candidate.Source)
	}
	report.Source = primary.Source

	// Cover art is fetched through the shared client and written atomically.
	coverUpdated := false
	if written, err := state.covers.Fetch(ctx, info.GameDir, info.ID, primary.Result.CoverURL, scraper.CoverPortrait, forceCover); err != nil {
		logger.CoverDownloaded(info.ID, scraper.CoverPortrait, primary.Result.CoverURL, err)
	} else if written {
		info.Metadata.CoverURL = game.CoverName
		coverUpdated = true
	}
	if written, err := state.covers.Fetch(ctx, info.GameDir, info.ID, primary.Result.CoverLandscapeURL, scraper.CoverLandscape, forceCover); err != nil {
		logger.CoverDownloaded(info.ID, scraper.CoverLandscape, primary.Result.CoverLandscapeURL, err)
	} else if written {
		info.Metadata.CoverLandscape = game.CoverLandscapeName
		coverUpdated = true
	}
	if coverUpdated {
		// Bumping the version changes the URL, which is what makes the UI show the
		// new artwork instead of a cached copy.
		info.MarkCoverUpdated()
	}

	if _, err := a.library.Replace(info); err != nil {
		logger.GameInfoSaved(info.ID, info.Title, err)
		report.Error = err.Error()
		return report
	}

	report.Title = info.Title
	return report
}

// describeScrapeError turns a pipeline error into something a user can act on.
func describeScrapeError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, scraper.ErrNoResult), scraper.IsNoResult(err):
		return "no metadata source matched this game"
	case errors.Is(err, context.Canceled):
		return "scrape cancelled"
	}

	var apiErr *scraper.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Kind {
		case scraper.KindAuth:
			return fmt.Sprintf("%s needs an API key (configure it in Settings)", apiErr.Source)
		case scraper.KindRateLimited:
			return fmt.Sprintf("%s is rate limiting requests, try again later", apiErr.Source)
		case scraper.KindNetwork:
			return fmt.Sprintf("%s could not be reached (check the network or proxy)", apiErr.Source)
		default:
			return apiErr.Error()
		}
	}
	return err.Error()
}

// ScrapeGame scrapes one game synchronously and returns the outcome, which is
// what the detail panel awaits.
func (a *App) ScrapeGame(id string) *ScrapeReport {
	// An explicit scrape should be able to replace bad artwork, so existing
	// covers are overwritten rather than skipped forever.
	report := a.scrapeOne(context.Background(), id, true)

	// Surface the change to every other view.
	a.emit(eventLibraryChanged, map[string]any{"updated": 1})
	return &report
}

// QueueScrapeAll enqueues every game, or only the ones missing metadata when
// force is false. It returns how many tasks were accepted.
//
// Batch scraping now goes through the queue like everything else. Previously it
// ran as a separate serial loop that ignored pause/resume and reported nothing to
// the UI.
func (a *App) QueueScrapeAll(force bool) int {
	if a.queue == nil {
		return 0
	}

	// Start the batch's progress figures from zero so the task list describes this
	// run rather than everything since the application started.
	a.queue.ResetProgress()

	queued := 0
	for _, info := range a.library.List() {
		if !force && hasCompleteMetadata(info) {
			continue
		}
		if a.queue.Submit(&taskqueue.Task{
			Type:   taskqueue.TaskScrape,
			GameID: info.ID,
			Title:  info.Title,
		}) {
			queued++
		}
	}
	logger.Info("batch scrape queued", "count", queued, "force", force, "concurrency", a.queue.Concurrency())
	return queued
}

// hasCompleteMetadata reports whether a game already has the fields the UI shows.
func hasCompleteMetadata(info *game.GameInfo) bool {
	if info.Metadata == nil {
		return false
	}
	meta := info.Metadata
	if meta.Description == "" || meta.ReleaseDate == "" {
		return false
	}
	if meta.Developer == "" && meta.Publisher == "" {
		return false
	}
	return scraper.Existing(info.GameDir, scraper.CoverPortrait) != ""
}

// --- queue controls ---------------------------------------------------------

// GetQueueInfo returns the current queue state.
func (a *App) GetQueueInfo() taskqueue.Status {
	if a.queue == nil {
		return taskqueue.Status{PendingTitles: []string{}}
	}
	return a.queue.Status()
}

// QueuePause stops new tasks from starting.
func (a *App) QueuePause() {
	if a.queue != nil {
		a.queue.Pause()
	}
}

// QueueResume allows queued tasks to start again.
func (a *App) QueueResume() {
	if a.queue != nil {
		a.queue.Resume()
	}
}

// QueueClear drops every pending task.
func (a *App) QueueClear() {
	if a.queue != nil {
		a.queue.Clear()
	}
}

// QueueResetProgress dismisses the finished-task counters and failure list, once
// the user has read them.
func (a *App) QueueResetProgress() {
	if a.queue != nil {
		a.queue.ResetProgress()
	}
}

// QueueSetConcurrency changes how many games are scraped at once.
func (a *App) QueueSetConcurrency(n int) {
	if a.queue != nil {
		a.queue.SetConcurrency(n)
	}
}

// --- scraping-adjacent helpers used by other API files ----------------------

// resolveGameDir returns a game's directory or an error naming the missing ID.
func (a *App) requireGame(id string) (*game.GameInfo, error) {
	info, ok := a.library.Get(id)
	if !ok {
		return nil, fmt.Errorf("game not found: %s", id)
	}
	return info, nil
}

// setConfig applies a mutation to the shared configuration and persists it.
func (a *App) setConfig(mutate func(*config.Config) error) error {
	state := a.snapshot()
	if state == nil {
		return errors.New("application is still starting up")
	}
	cfg := state.cfg
	if err := mutate(cfg); err != nil {
		return err
	}
	return a.SaveConfig(cfg)
}

// SaveConfig persists the configuration and rebuilds everything derived from it.
//
// Rebuilding is the fix for a real bug: the scanner and the scrape pipeline were
// constructed once at startup with a pointer to the original config object, so
// changing the game directories, the scan depth, the language or an API key had no
// effect until the application was restarted.
func (a *App) SaveConfig(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config: nothing to save")
	}
	if err := cfg.Normalize(a.exeDir); err != nil {
		return err
	}
	if err := cfg.Save(a.exeDir); err != nil {
		return err
	}
	if err := a.buildState(cfg); err != nil {
		return err
	}

	// The logging destination and level are part of the configuration.
	logger.Reconfigure(logger.Options{
		LibraryRoot: a.exeDir,
		LocalDir:    logger.DefaultLocalDir(),
		ToLibrary:   cfg.LogToLibrary,
		Level:       logger.ParseLevel(cfg.LogLevel),
		Console:     isDevBuild(),
	})

	a.restartWatcher()
	a.loadLibrary()

	// The worker pool size is part of the configuration, so a change takes effect
	// without a restart.
	if a.queue != nil {
		a.queue.SetConcurrency(cfg.ScrapeConcurrency)
	}

	a.emit(eventLibraryChanged, map[string]any{"configSaved": true})

	// Discovering games in newly added directories is expensive on a share, so it
	// happens in the background rather than blocking the settings dialog. It is
	// tracked so shutdown — and tests — can wait for the writes to finish.
	a.bgWork.Add(1)
	go func() {
		defer a.bgWork.Done()
		a.scanMu.Lock()
		defer a.scanMu.Unlock()
		state := a.snapshot()
		if state == nil {
			return
		}
		for _, dir := range state.cfg.GameDirectories {
			a.scanDirs([]string{state.cfg.ResolveDir(a.exeDir, dir)}, false)
		}
	}()
	return nil
}

// --- launching and shell integration ---------------------------------------

// LaunchGame starts a game's primary launcher.
func (a *App) LaunchGame(id string) error {
	info, err := a.requireGame(id)
	if err != nil {
		return err
	}

	primary := info.PrimaryExecutable()
	if primary == nil {
		logger.GameLaunchFailed(id, info.Title, fmt.Errorf("no launcher recorded"))
		return fmt.Errorf("%s has no recorded launcher", info.Title)
	}

	exePath := fsutil.Resolve(info.GameDir, primary.Path)
	if !fsutil.Exists(exePath) {
		logger.GameLaunchFailed(id, info.Title, fmt.Errorf("launcher missing: %s", exePath))
		return fmt.Errorf("launcher not found: %s", primary.Path)
	}

	cmd, err := platform.LaunchGame(exePath, info.GameDir)
	if err != nil {
		logger.GameLaunchFailed(id, info.Title, err)
		return err
	}
	logger.GameLaunched(id, info.Title, primary.Path)

	// Reap the process so a long session does not accumulate zombies. Phase 4 will
	// use this hook to measure playtime.
	go func() {
		_ = cmd.Wait()
	}()

	if _, err := a.library.Mutate(id, func(draft *game.GameInfo) error {
		draft.LastPlayedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	}); err != nil {
		logger.Warn("could not record launch time", "gameId", id, "error", err.Error())
	}
	a.emit(eventLibraryChanged, map[string]any{"launched": id})
	return nil
}

// SetPrimaryExecutable records which launcher the launch button should use.
func (a *App) SetPrimaryExecutable(id string, execPath string) error {
	_, err := a.library.Mutate(id, func(draft *game.GameInfo) error {
		found := false
		for i := range draft.Executables {
			draft.Executables[i].Primary = strings.EqualFold(draft.Executables[i].Path, execPath)
			if draft.Executables[i].Primary {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("launcher not found: %s", execPath)
		}
		return nil
	})
	return err
}

// OpenGameDirectory reveals a game's folder in the file manager.
func (a *App) OpenGameDirectory(id string) error {
	info, err := a.requireGame(id)
	if err != nil {
		return err
	}
	return platform.OpenPath(info.GameDir)
}

// OpenGameMetadata opens the game's metadata file in a text editor.
func (a *App) OpenGameMetadata(id string) error {
	info, err := a.requireGame(id)
	if err != nil {
		return err
	}
	return platform.EditFile(info.InfoFilePath())
}

// OpenDirectory reveals a configured directory (relative paths are resolved
// against the library root).
func (a *App) OpenDirectory(dir string) error {
	return platform.OpenPath(fsutil.Resolve(a.exeDir, dir))
}

// OpenBrowser opens a URL in the system browser.
func (a *App) OpenBrowser(rawURL string) error {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("refusing to open non-http URL: %s", rawURL)
	}
	if a.ctx == nil {
		return errors.New("application is still starting up")
	}
	runtime.BrowserOpenURL(a.ctx, rawURL)
	return nil
}
