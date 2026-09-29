package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"GameLibrary/internal/config"
	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
	"GameLibrary/internal/library"
	"GameLibrary/internal/scraper"
	"GameLibrary/internal/taskqueue"
)

// newTestApp builds an App whose library root is a temporary directory.
//
// The logger is deliberately left uninitialised: every logging function is a
// no-op until Init runs, so tests produce no files and no console noise.
func newTestApp(t *testing.T) *App {
	t.Helper()

	root := t.TempDir()
	app := &App{
		exeDir:  root,
		host:    "test-host",
		library: library.New(root),
	}

	cfg := config.Default()
	cfg.WatcherEnabled = false
	cfg.LogLevel = "error"
	// Keep scans deterministic: one game immediately under the root.
	cfg.GameDirectories = []string{".\\Games"}

	if err := app.buildState(cfg); err != nil {
		t.Fatalf("buildState: %v", err)
	}

	// A configuration change starts a background scan that writes gameinfo.json.
	// Waiting for it keeps that write from racing the removal of the temporary
	// directory at the end of the test.
	t.Cleanup(app.waitForBackgroundWork)
	return app
}

// makeGame creates a directory containing a launcher file.
func makeGame(t *testing.T, root string, rel string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatalf("write launcher: %v", err)
	}
	return dir
}

// loadGameInfo re-reads a record from disk, bypassing the in-memory cache.
func loadGameInfo(gameDir string) (*GameInfo, error) {
	return game.LoadFromDir(gameDir)
}

// samePath compares two paths case-insensitively, as Windows does.
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// --- P0: configuration changes must take effect immediately ------------------

// TestSaveConfigRebuildsScannerForNewDirectory is the regression test for a bug
// where the scanner kept a pointer to the configuration object that existed at
// startup. Adding a game directory in Settings therefore had no effect on the
// next scan: the scanner still walked the old list.
func TestSaveConfigRebuildsScannerForNewDirectory(t *testing.T) {
	app := newTestApp(t)

	makeGame(t, app.exeDir, "Games/Alpha")
	makeGame(t, app.exeDir, "Extra/Beta")

	results := app.ScanGames()
	if len(results) != 1 {
		t.Fatalf("before config change: expected 1 game, got %d (%+v)", len(results), results)
	}

	cfg := app.GetConfig()
	cfg.GameDirectories = []string{".\\Games", ".\\Extra"}
	if err := app.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	results = app.ScanGames()
	if len(results) != 2 {
		t.Fatalf("after config change: expected 2 games, got %d (%+v)", len(results), results)
	}

	titles := map[string]bool{}
	for _, game := range app.GetGameList() {
		titles[game.Title] = true
	}
	if !titles["Alpha"] || !titles["Beta"] {
		t.Fatalf("expected both Alpha and Beta in the library, got %v", titles)
	}
}

// TestSaveConfigRebuildsScannerForScanDepth pins the same bug for MaxScanDepth:
// the previous scanner cached the depth as well.
func TestSaveConfigRebuildsScannerForScanDepth(t *testing.T) {
	app := newTestApp(t)

	// Four levels below the configured root, so the default depth of three stops
	// before reaching it.
	makeGame(t, app.exeDir, "Games/L1/L2/L3/L4")

	if got := len(app.ScanGames()); got != 0 {
		t.Fatalf("with the default depth the nested game should be invisible, got %d", got)
	}

	cfg := app.GetConfig()
	cfg.MaxScanDepth = 8
	if err := app.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if got := len(app.ScanGames()); got != 1 {
		t.Fatalf("after raising the depth the nested game should be found, got %d", got)
	}
}

// TestSaveConfigNormalisesSloppyInput checks that paths typed by hand collapse to
// one stored entry each and still resolve to the directory the user meant.
//
// The assertion is on the resolved path rather than on the stored string: the
// stored form uses the host's separator, so comparing text would only hold on
// Windows, while resolving is what the application actually relies on.
func TestSaveConfigNormalisesSloppyInput(t *testing.T) {
	app := newTestApp(t)

	cfg := app.GetConfig()
	cfg.GameDirectories = []string{"Games/", "./Games", ".\\Games", "  ", "Extra/deep/../deep2"}
	if err := app.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got := app.GetConfig().GameDirectories
	if len(got) != 2 {
		t.Fatalf("expected the four spellings of Games to collapse to one entry plus deep2, got %v", got)
	}

	want := []string{
		filepath.Join(app.exeDir, "Games"),
		filepath.Join(app.exeDir, "Extra", "deep2"),
	}
	for i, expected := range want {
		resolved := fsutil.Resolve(app.exeDir, got[i])
		if !samePath(resolved, expected) {
			t.Errorf("entry %d (%q) resolves to %q, want %q", i, got[i], resolved, expected)
		}
	}
}

// --- P0: the library cache is concurrency safe ------------------------------

// TestLibraryCacheIsRaceFree hammers the cache the way the real application does:
// IPC readers, the watcher's scan goroutine and queue-driven mutations all at
// once. It is meaningful under -race.
func TestLibraryCacheIsRaceFree(t *testing.T) {
	app := newTestApp(t)

	for i := 0; i < 6; i++ {
		makeGame(t, app.exeDir, filepath.Join("Games", "Game"+string(rune('A'+i))))
	}
	app.ScanGames()

	ids := app.library.IDs()
	if len(ids) == 0 {
		t.Fatal("expected games in the cache")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Readers, as the IPC handlers are.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = app.GetGameList()
				_ = app.GetGame(ids[0])
				_ = app.library.Len()
			}
		}()
	}

	// Writers, as the watcher and queue are.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = app.library.Mutate(ids[i%len(ids)], func(draft *GameInfo) error {
				draft.Starred = !draft.Starred
				draft.AddTag("tag")
				return nil
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			app.scanDirs([]string{filepath.Join(app.exeDir, "Games")}, false)
		}
	}()

	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestGetGameListReturnsCopies ensures a caller cannot corrupt the cache.
func TestGetGameListReturnsCopies(t *testing.T) {
	app := newTestApp(t)
	makeGame(t, app.exeDir, "Games/Alpha")
	app.ScanGames()

	list := app.GetGameList()
	if len(list) != 1 {
		t.Fatalf("expected 1 game, got %d", len(list))
	}
	list[0].Title = "Mutated"
	list[0].Tags = append(list[0].Tags, "evil")

	again := app.GetGameList()
	if again[0].Title == "Mutated" {
		t.Fatal("mutating a returned record changed the cache")
	}
	if len(again[0].Tags) != 0 {
		t.Fatalf("mutating a returned record changed the cache: %v", again[0].Tags)
	}
}

// --- P1: forced rescans keep what the user and the scrapers produced ---------

// TestForceScanKeepsUserData is the app-level companion to the scanner test: a
// forced rescan used to rebuild the record from scratch and discard the star,
// the tags, the scraped metadata and the playtime.
func TestForceScanKeepsUserData(t *testing.T) {
	app := newTestApp(t)
	makeGame(t, app.exeDir, "Games/Alpha")
	app.ScanGames()

	id := app.GetGameList()[0].ID
	if _, err := app.library.Mutate(id, func(draft *GameInfo) error {
		draft.Starred = true
		draft.AddTag("favourite")
		draft.Metadata = &Metadata{
			Description: "a description",
			Developer:   "a developer",
			Tags:        []string{"RPG"},
		}
		draft.LastPlayedAt = "2026-01-02T03:04:05Z"
		draft.TotalPlaytime = 4242
		return nil
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	app.ForceScanGames()

	got := app.GetGame(id)
	if got == nil {
		t.Fatal("game disappeared after a forced rescan")
	}
	if !got.Starred {
		t.Error("star was lost")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "favourite" {
		t.Errorf("tags were lost: %v", got.Tags)
	}
	if got.Metadata == nil || got.Metadata.Description != "a description" || got.Metadata.Developer != "a developer" {
		t.Errorf("metadata was lost: %+v", got.Metadata)
	}
	if got.TotalPlaytime != 4242 {
		t.Errorf("playtime was lost: %d", got.TotalPlaytime)
	}
	if got.LastPlayedAt == "" {
		t.Error("last played timestamp was lost")
	}
}

// --- P1: settings mutations are atomic --------------------------------------

func TestGameMutationsPersist(t *testing.T) {
	app := newTestApp(t)
	makeGame(t, app.exeDir, "Games/Alpha")
	app.ScanGames()
	id := app.GetGameList()[0].ID

	if err := app.ToggleGameStar(id); err != nil {
		t.Fatalf("ToggleGameStar: %v", err)
	}
	if err := app.AddGameTag(id, "  Action  "); err != nil {
		t.Fatalf("AddGameTag: %v", err)
	}
	// Adding the same tag in a different case must be a no-op.
	if err := app.AddGameTag(id, "action"); err != nil {
		t.Fatalf("AddGameTag duplicate: %v", err)
	}

	got := app.GetGame(id)
	if !got.Starred {
		t.Error("star was not persisted")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "Action" {
		t.Errorf("expected exactly one trimmed tag, got %v", got.Tags)
	}

	// The change must be on disk, not just in memory.
	reloaded, err := loadGameInfo(got.GameDir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.Starred || len(reloaded.Tags) != 1 {
		t.Errorf("changes were not written to disk: %+v", reloaded)
	}

	if err := app.RemoveGameTag(id, "ACTION"); err != nil {
		t.Fatalf("RemoveGameTag: %v", err)
	}
	if tags := app.GetGame(id).Tags; len(tags) != 0 {
		t.Errorf("tag was not removed: %v", tags)
	}
}

func TestSetPreferredSourceRequiresPlatform(t *testing.T) {
	app := newTestApp(t)
	makeGame(t, app.exeDir, "Games/Alpha")
	app.ScanGames()
	id := app.GetGameList()[0].ID

	if err := app.SetPreferredSource(id, "steam"); err == nil {
		t.Fatal("expected an error when the game is not linked to that platform")
	}
	if _, err := app.library.Mutate(id, func(draft *GameInfo) error {
		draft.SetPlatform("steam", "570", "Alpha")
		return nil
	}); err != nil {
		t.Fatalf("SetPlatform: %v", err)
	}
	if err := app.SetPreferredSource(id, "steam"); err != nil {
		t.Fatalf("SetPreferredSource: %v", err)
	}
	if got := app.GetGame(id).PreferredSource; got != "steam" {
		t.Errorf("expected preferred source steam, got %q", got)
	}
}

// --- P1: batch scraping goes through the queue ------------------------------

func TestQueueScrapeAllDeduplicates(t *testing.T) {
	app := newTestApp(t)
	for i := 0; i < 3; i++ {
		makeGame(t, app.exeDir, filepath.Join("Games", "Game"+string(rune('A'+i))))
	}
	app.ScanGames()

	// Hold the worker on a channel so all tasks stay pending while we assert.
	release := make(chan struct{})
	app.queue = taskqueue.New(func(ctx context.Context, task *taskqueue.Task) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	defer func() {
		close(release)
		app.queue.Stop()
	}()

	if got := app.QueueScrapeAll(true); got != 3 {
		t.Fatalf("expected 3 tasks queued, got %d", got)
	}
	// A second request while the first batch is still pending must add nothing.
	if got := app.QueueScrapeAll(true); got != 0 {
		t.Fatalf("expected duplicate submissions to be rejected, got %d", got)
	}

	app.QueuePause()
	status := app.GetQueueInfo()
	if status.Pending+status.Running != 3 {
		t.Fatalf("expected 3 outstanding tasks, got %+v", status)
	}
	app.QueueClear()
	if status := app.GetQueueInfo(); status.Pending != 0 {
		t.Fatalf("expected the queue to be emptied, got %+v", status)
	}
}

// TestQueueScrapeAllSkipsCompleteGames covers the selective batch path.
func TestQueueScrapeAllSkipsCompleteGames(t *testing.T) {
	app := newTestApp(t)
	makeGame(t, app.exeDir, "Games/Alpha")
	makeGame(t, app.exeDir, "Games/Beta")
	app.ScanGames()

	// Give Alpha complete metadata and a cover on disk.
	ids := app.library.IDs()
	if _, err := app.library.Mutate(ids[0], func(draft *GameInfo) error {
		draft.Metadata = &Metadata{Description: "d", ReleaseDate: "2020", Developer: "dev"}
		return nil
	}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	info := app.GetGame(ids[0])
	if err := os.MkdirAll(filepath.Join(info.GameDir, ".gamemanager", "covers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info.GameDir, ".gamemanager", "covers", "cover.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	app.queue = taskqueue.New(func(ctx context.Context, _ *taskqueue.Task) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	defer func() {
		close(release)
		app.queue.Stop()
	}()

	if got := app.QueueScrapeAll(false); got != 1 {
		t.Fatalf("expected only the incomplete game to be queued, got %d", got)
	}
}

// --- app information --------------------------------------------------------

// TestGetAppInfoReportsRealBuildTime pins a small correctness fix: the field used
// to be filled with time.Now(), so it reported the current time as the build time.
func TestGetAppInfoReportsRealBuildTime(t *testing.T) {
	app := newTestApp(t)

	info := app.GetAppInfo()
	if info.ExeDir != app.exeDir {
		t.Errorf("expected exeDir %q, got %q", app.exeDir, info.ExeDir)
	}
	if info.BuildTime != buildTime {
		t.Errorf("expected the stamped build time %q, got %q", buildTime, info.BuildTime)
	}
	if info.Version != version {
		t.Errorf("expected version %q, got %q", version, info.Version)
	}
	if info.CoverBaseURL == "" {
		t.Error("expected a cover base URL")
	}
	if info.MachineID == "" {
		t.Error("expected a machine id")
	}
}

func TestCommonPathsResolvesDirectories(t *testing.T) {
	app := newTestApp(t)
	paths := app.CommonPaths()

	if paths["exeDir"] != app.exeDir {
		t.Errorf("expected exeDir %q, got %q", app.exeDir, paths["exeDir"])
	}

	// The map is keyed by the stored (host-separated) directory string, so the
	// entry is located by what it resolves to rather than by spelling the key.
	want := filepath.Join(app.exeDir, "Games")
	found := false
	for key, resolved := range paths {
		if key == "exeDir" {
			continue
		}
		if !filepath.IsAbs(resolved) {
			t.Errorf("expected an absolute path for %q, got %q", key, resolved)
		}
		if samePath(resolved, want) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an entry resolving to %q, got %v", want, paths)
	}
}

// --- error reporting --------------------------------------------------------

func TestDescribeScrapeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"no result", scraper.ErrNoResult, "no metadata source matched this game"},
		{"wrapped no result", scraper.NoResult("steam", "x"), "no metadata source matched this game"},
		{"auth", &scraper.APIError{Source: "rawg", Kind: scraper.KindAuth}, "rawg needs an API key (configure it in Settings)"},
		{"rate limited", &scraper.APIError{Source: "vndb", Kind: scraper.KindRateLimited}, "vndb is rate limiting requests, try again later"},
		{"network", &scraper.APIError{Source: "bangumi", Kind: scraper.KindNetwork}, "bangumi could not be reached (check the network or proxy)"},
		{"cancelled", context.Canceled, "scrape cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeScrapeError(tt.err); got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// --- path handling ----------------------------------------------------------

// TestOpenDirectoryResolvesRelativePaths checks that a relative configured
// directory resolves against the library root rather than the process working
// directory.
func TestOpenDirectoryResolvesRelativePaths(t *testing.T) {
	app := newTestApp(t)
	dir := makeGame(t, app.exeDir, "Games/Alpha")

	// OpenPath shells out, so instead assert the resolution logic the method uses.
	// The stored form carries the host separator, so the round trip is asserted
	// rather than the literal string.
	stored := config.NormalizeDir(app.exeDir, "Games/Alpha")
	if resolved := fsutil.Resolve(app.exeDir, stored); !samePath(resolved, dir) {
		t.Fatalf("%q resolves to %q, want %q", stored, resolved, dir)
	}
	if !samePath(dir, filepath.Join(app.exeDir, "Games", "Alpha")) {
		t.Fatal("fixture layout is not what the test expects")
	}
}

// TestLaunchGameReportsMissingLauncher verifies the error path without starting a
// process.
func TestLaunchGameReportsMissingLauncher(t *testing.T) {
	app := newTestApp(t)

	if err := app.LaunchGame("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown game id")
	}

	makeGame(t, app.exeDir, "Games/Alpha")
	app.ScanGames()
	id := app.GetGameList()[0].ID

	info := app.GetGame(id)
	if err := os.Remove(filepath.Join(info.GameDir, "game.exe")); err != nil {
		t.Fatal(err)
	}
	if err := app.LaunchGame(id); err == nil {
		t.Fatal("expected an error when the launcher file is gone")
	}
}

// --- migration from the previously released format --------------------------

// TestLoadsRealLegacyMetadata proves that metadata written by the previous
// release still loads.
//
// The fixtures under testdata/ were produced by 0.7.5: they carry an absolute
// `gameDir` and no `schemaVersion`. This test copies one out of the repository
// before touching it, because the earlier test suite used to rewrite the committed
// files as a side effect.
func TestLoadsRealLegacyMetadata(t *testing.T) {
	const legacyFixture = "testdata/simple_steam_game"
	source := filepath.Join(legacyFixture, ".gamemanager", "gameinfo.json")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Skipf("legacy fixture is unavailable: %v", err)
	}
	if !strings.Contains(string(raw), `"gameDir"`) {
		t.Fatal("the fixture is supposed to be pre-0.8 data containing a gameDir field")
	}

	gameDir := filepath.Join(t.TempDir(), "simple_steam_game")
	if err := os.MkdirAll(filepath.Join(gameDir, ".gamemanager"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, ".gamemanager", "gameinfo.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := loadGameInfo(gameDir)
	if err != nil {
		t.Fatalf("legacy metadata did not load: %v", err)
	}

	if info.ID != "steam_123456" {
		t.Errorf("expected the stored id to survive, got %q", info.ID)
	}
	if info.PrimaryPlatform() != "steam" || info.PrimaryPlatformID() != "123456" {
		t.Errorf("expected the steam platform to survive, got %q/%q",
			info.PrimaryPlatform(), info.PrimaryPlatformID())
	}
	// The mount point is taken from where the record was loaded, not from the
	// stale absolute path that was persisted by the old version.
	if !samePath(info.GameDir, gameDir) {
		t.Errorf("expected GameDir %q, got %q", gameDir, info.GameDir)
	}

	// Re-saving drops the machine-specific path from the shared file.
	if err := info.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rewritten, err := os.ReadFile(filepath.Join(gameDir, ".gamemanager", "gameinfo.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rewritten), `"gameDir"`) {
		t.Error("the rewritten metadata still contains a machine-specific gameDir field")
	}
	if !strings.Contains(string(rewritten), `"schemaVersion"`) {
		t.Error("the rewritten metadata is missing a schemaVersion")
	}
}
