package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
	"GameLibrary/internal/library"
	"GameLibrary/internal/logger"
	"GameLibrary/internal/mediaserve"
	"GameLibrary/internal/scanner"
	"GameLibrary/internal/scraper"
	"GameLibrary/internal/taskqueue"
	"GameLibrary/internal/watcher"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// version and buildTime are stamped by the release build through -ldflags.
var (
	version   = "0.8.0"
	buildTime = "development"
)

// Type aliases keep the generated TypeScript bindings in a stable namespace while
// the real definitions live in their own packages.
type (
	Config     = config.Config
	GameInfo   = game.GameInfo
	ScanResult = scanner.ScanResult
	Executable = game.Executable
	SavePath   = game.SavePath
	Metadata   = game.Metadata
)

// Frontend event names. They are constants so the backend and the UI cannot drift
// apart over a typo.
const (
	eventQueueStatus    = "queue:status"
	eventQueueDone      = "queue:done"
	eventLibraryChanged = "library:changed"
	eventScanComplete   = "scan:complete"
)

// runtimeState holds the components derived from the configuration. They are
// rebuilt as a unit whenever the configuration changes, which is what makes
// settings take effect immediately: previously the scanner and the scrape
// pipeline kept a pointer to the configuration object that existed at startup, so
// a saved change was silently ignored until the next launch.
type runtimeState struct {
	cfg      *config.Config
	scanner  *scanner.Scanner
	pipeline *scraper.Pipeline
	covers   *scraper.CoverFetcher
}

// App is the backend exposed to the frontend.
type App struct {
	ctx    context.Context
	exeDir string
	host   string

	library *library.Store
	queue   *taskqueue.Queue
	watcher *watcher.Watcher

	// stateMu guards the runtime state bundle. Readers take a snapshot.
	stateMu sync.RWMutex
	state   *runtimeState

	// scanMu serialises background scans so a watcher burst cannot start
	// overlapping walks of the same tree.
	scanMu sync.Mutex

	// bgWork tracks the background scans started after a configuration change, so
	// shutdown (and tests) can wait for them instead of racing a file write against
	// process exit or a temporary directory being removed.
	bgWork sync.WaitGroup
}

// waitForBackgroundWork blocks until every background scan has finished.
func (a *App) waitForBackgroundWork() {
	a.bgWork.Wait()
}

// NewApp builds the application.
//
// The library root is resolved here rather than in startup so the media handler
// can be constructed before wails.Run, which needs it to configure the asset
// server.
func NewApp() *App {
	exeDir := resolveExeDir()
	return &App{
		exeDir:  exeDir,
		library: library.New(exeDir),
	}
}

func resolveExeDir() string {
	exePath, err := os.Executable()
	if err != nil {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			return wd
		}
		return "."
	}
	return filepath.Dir(exePath)
}

// assetHandler returns the HTTP handler mounted behind the bundled assets. It
// serves cover art straight off disk so the UI never has to pull images through
// the Wails binding layer.
//
// It is unexported on purpose: Wails binds every exported method on App, and this
// one is infrastructure rather than an API for the UI.
func (a *App) assetHandler() *mediaserve.Handler {
	mediaserve.LogStartup()
	return mediaserve.New(a.library)
}

// startup runs once the webview is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Bootstrap logging before anything else can fail, then point it at the
	// destination the configuration asks for.
	logger.Init(logger.Options{
		LibraryRoot: a.exeDir,
		LocalDir:    logger.DefaultLocalDir(),
		Level:       logger.ParseLevel("info"),
	})

	a.host, _ = os.Hostname()

	// The window exists, so any later failure is an ordinary runtime error rather
	// than a silent startup death.
	clearStartupMarker()

	cfg, err := config.Load(a.exeDir)
	if err != nil {
		logger.Error("config load failed, using defaults", "error", err.Error())
		cfg = config.Default()
	}

	logger.Reconfigure(logger.Options{
		LibraryRoot: a.exeDir,
		LocalDir:    logger.DefaultLocalDir(),
		ToLibrary:   cfg.LogToLibrary,
		Level:       logger.ParseLevel(cfg.LogLevel),
		Console:     isDevBuild(),
	})

	logger.AppStarted(version, a.exeDir, a.host)

	if err := a.buildState(cfg); err != nil {
		logger.Error("runtime initialisation failed", "error", err.Error())
	}

	a.setupQueue()
	a.loadLibrary()
	a.restartWatcher()

	logger.Info("startup complete", "gameCount", a.library.Len(), "logDir", logger.Dir())
}

// buildState (re)creates the components that depend on the configuration.
func (a *App) buildState(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("build runtime: nil config")
	}
	if err := cfg.Normalize(a.exeDir); err != nil {
		return err
	}

	pipeline := scraper.NewPipeline(cfg)
	pipeline.Register(scraper.NewSteamScraper())
	pipeline.Register(scraper.NewVNDBScraper())
	pipeline.Register(scraper.NewBangumiScraper())
	pipeline.Register(scraper.NewDLsiteScraper())
	pipeline.Register(scraper.NewRawgScraper())
	pipeline.Register(scraper.NewSteamGridDBScraper())

	// Apply language, API keys and the shared HTTP client to every provider.
	if err := pipeline.Configure(cfg); err != nil {
		logger.Warn("scraper configuration incomplete", "error", err.Error())
	}

	state := &runtimeState{
		cfg:      cfg,
		scanner:  scanner.New(a.exeDir, cfg),
		pipeline: pipeline,
		covers:   scraper.NewCoverFetcher(pipeline.HTTP()),
	}

	a.stateMu.Lock()
	a.state = state
	a.stateMu.Unlock()
	return nil
}

// snapshot returns the current runtime state. Callers must not mutate it.
func (a *App) snapshot() *runtimeState {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.state
}

// setupQueue wires the scrape queue and its UI notifications.
func (a *App) setupQueue() {
	a.queue = taskqueue.New(func(ctx context.Context, task *taskqueue.Task) {
		a.runScrapeTask(ctx, task)
	})
	a.queue.SetObserver(func(status taskqueue.Status) {
		a.emit(eventQueueStatus, status)
	}, func(task *taskqueue.Task) {
		a.emit(eventQueueDone, map[string]any{
			"gameId": task.GameID,
			"title":  task.Title,
			"error":  task.Error,
		})
	})

	// Scrape several games at once. Each provider host is rate limited inside the
	// shared HTTP client, so this shortens a library-wide scrape by overlapping the
	// waiting rather than by raising the request rate any one service sees.
	a.queue.SetConcurrency(a.scrapeConcurrency())
}

// scrapeConcurrency returns the configured worker pool size.
func (a *App) scrapeConcurrency() int {
	state := a.snapshot()
	if state == nil || state.cfg.ScrapeConcurrency < 1 {
		return 1
	}
	return state.cfg.ScrapeConcurrency
}

// loadLibrary repopulates the cache from disk.
func (a *App) loadLibrary() {
	state := a.snapshot()
	if state == nil {
		return
	}
	a.library.ReplaceAll(state.scanner.LoadLibrary())
}

// restartWatcher rebuilds the filesystem watcher from the current configuration.
func (a *App) restartWatcher() {
	state := a.snapshot()
	if state == nil {
		return
	}

	if a.watcher != nil {
		a.watcher.Stop()
		a.watcher = nil
	}

	cfg := state.cfg
	if !cfg.WatcherEnabled {
		logger.WatcherRestarted(false, 0)
		return
	}

	dirs := make([]string, 0, len(cfg.GameDirectories))
	for _, dir := range cfg.GameDirectories {
		dirs = append(dirs, cfg.ResolveDir(a.exeDir, dir))
	}

	a.watcher = watcher.New(
		cfg.MaxScanDepth,
		time.Duration(cfg.WatcherDebounceMs)*time.Millisecond,
		a.handleWatchEvents,
	)
	if err := a.watcher.Watch(dirs); err != nil {
		logger.Warn("watcher: failed to start", "error", err.Error())
		a.watcher = nil
		return
	}
	logger.WatcherRestarted(true, len(dirs))
}

// handleWatchEvents reacts to filesystem changes. It runs on the watcher
// goroutine, so the actual work is handed to a serialised background scan.
func (a *App) handleWatchEvents(events watcher.Events) {
	if events.Empty() {
		return
	}

	if len(events.RemovedDirs) > 0 {
		removed := 0
		for _, dir := range events.RemovedDirs {
			removed += len(a.library.RemoveWithin(dir))
		}
		if removed > 0 {
			a.emit(eventLibraryChanged, map[string]any{"removed": removed})
		}
	}

	rescan := make([]string, 0, len(events.NewGameDirs)+len(events.ChangedDirs))
	rescan = append(rescan, events.NewGameDirs...)
	rescan = append(rescan, events.ChangedDirs...)
	if len(rescan) == 0 {
		return
	}

	go func() {
		a.scanMu.Lock()
		defer a.scanMu.Unlock()
		a.scanDirs(rescan, false)
	}()
}

// scanDirs identifies the given directories and folds the outcome into the cache.
func (a *App) scanDirs(dirs []string, force bool) []scanner.ScanResult {
	state := a.snapshot()
	if state == nil {
		return nil
	}

	var results []scanner.ScanResult
	for _, dir := range dirs {
		if force {
			results = append(results, state.scanner.ForceScanDir(dir)...)
			continue
		}
		results = append(results, state.scanner.ScanDir(dir)...)
	}

	newGames := 0
	for _, result := range results {
		if result.GameInfo == nil {
			continue
		}
		a.library.Put(result.GameInfo)
		if result.IsNew {
			newGames++
		}
	}

	if len(results) > 0 {
		a.emit(eventLibraryChanged, map[string]any{"updated": len(results), "new": newGames})
	}
	a.autoScrapeNew(results)
	return results
}

// autoScrapeNew queues metadata scraping for freshly discovered games that have
// no cover yet.
func (a *App) autoScrapeNew(results []scanner.ScanResult) {
	if a.queue == nil {
		return
	}
	for _, result := range results {
		if !result.IsNew || result.GameInfo == nil || result.Error != "" {
			continue
		}
		if scraper.Existing(result.GameDir, scraper.CoverPortrait) != "" {
			logger.Debug("auto-scrape skipped (cover already present)", "gameId", result.GameInfo.ID)
			continue
		}
		a.queue.Submit(&taskqueue.Task{
			Type:   taskqueue.TaskScrape,
			GameID: result.GameInfo.ID,
			Title:  result.GameInfo.Title,
		})
	}
}

// runScrapeTask performs one queued scrape. It is shared with the synchronous
// single-game entry point so both paths behave identically.
func (a *App) runScrapeTask(ctx context.Context, task *taskqueue.Task) {
	report := a.scrapeOne(ctx, task.GameID, false)
	task.Title = report.Title
	task.Error = report.Error
}

// emit publishes a frontend event when a context is available.
func (a *App) emit(event string, payload any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, event, payload)
}

// shutdown releases background resources. It is safe to call more than once.
func (a *App) shutdown() {
	// Let in-flight scans finish first: they write gameinfo.json files, and a scan
	// cut short by process exit can leave a partially written record.
	a.bgWork.Wait()
	if a.queue != nil {
		a.queue.Stop()
	}
	if a.watcher != nil {
		a.watcher.Stop()
	}
	logger.AppStopped()
	logger.Close()
}

// isDevBuild reports whether the binary is a development build, which decides
// whether logs are mirrored to the console.
func isDevBuild() bool {
	return buildTime == "development" || strings.EqualFold(os.Getenv("GAMELIBRARY_DEV"), "1")
}
