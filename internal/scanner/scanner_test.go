package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"GameLibrary/internal/config"
	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
	"GameLibrary/internal/platform"
)

// Every fixture in this file lives in a t.TempDir(). The previous version of
// this file scanned the repository's committed testdata/ directory and wrote
// .gamemanager folders into it, so running the tests mutated version-controlled
// files. Nothing here touches anything outside a temporary directory.

// --- helpers ---------------------------------------------------------------

// testScanner builds a scanner over dirs. The configuration is constructed
// directly rather than through config.Default() so that a test never writes a
// config.json anywhere.
func testScanner(t *testing.T, root string, depth int, dirs ...string) *Scanner {
	t.Helper()
	return New(root, &config.Config{
		SchemaVersion:   config.SchemaVersion,
		MachineID:       "test-machine",
		GameDirectories: dirs,
		MaxScanDepth:    depth,
		Language:        "en-US",
	})
}

func mkdirs(t *testing.T, elems ...string) string {
	t.Helper()
	dir := filepath.Join(elems...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// samePath compares two paths the way Windows does.
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func resultFor(results []ScanResult, dir string) *ScanResult {
	for i := range results {
		if samePath(results[i].GameDir, dir) {
			return &results[i]
		}
	}
	return nil
}

func reportedDirs(results []ScanResult) string {
	parts := make([]string, 0, len(results))
	for _, r := range results {
		entry := r.GameDir
		if r.Error != "" {
			entry += " (error: " + r.Error + ")"
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, ", ")
}

// mustResult returns the result for dir, failing the test when the directory was
// not reported, was reported as an error, or carries no record.
func mustResult(t *testing.T, results []ScanResult, dir string) *ScanResult {
	t.Helper()
	r := resultFor(results, dir)
	if r == nil {
		t.Fatalf("no scan result for %s\nreported: %s", dir, reportedDirs(results))
	}
	if r.Error != "" {
		t.Fatalf("scan reported an error for %s: %s", dir, r.Error)
	}
	if r.GameInfo == nil {
		t.Fatalf("scan result for %s carries no game info", dir)
	}
	return r
}

func runScan(t *testing.T, sc *Scanner) []ScanResult {
	t.Helper()
	results, err := sc.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	return results
}

func runForceScan(t *testing.T, sc *Scanner) []ScanResult {
	t.Helper()
	results, err := sc.ForceScanAll()
	if err != nil {
		t.Fatalf("ForceScanAll: %v", err)
	}
	return results
}

func executablePaths(info *game.GameInfo) []string {
	paths := make([]string, 0, len(info.Executables))
	for _, e := range info.Executables {
		paths = append(paths, e.Path)
	}
	return paths
}

func executableSet(info *game.GameInfo) map[string]bool {
	set := make(map[string]bool, len(info.Executables))
	for _, e := range info.Executables {
		set[e.Path] = true
	}
	return set
}

func gameIDs(games []*game.GameInfo) []string {
	ids := make([]string, 0, len(games))
	for _, info := range games {
		ids = append(ids, info.ID)
	}
	return ids
}

// primaryOf asserts that exactly one launcher is flagged primary and returns it.
func primaryOf(t *testing.T, info *game.GameInfo) *game.Executable {
	t.Helper()
	count := 0
	for i := range info.Executables {
		if info.Executables[i].Primary {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one primary launcher, got %d: %v", count, executablePaths(info))
	}
	return info.PrimaryExecutable()
}

// scanLauncherDir creates a directory holding exactly names and returns the
// record the scanner builds for it. The directory is itself the configured game
// directory, which is also the case that exercises descending into a root.
func scanLauncherDir(t *testing.T, names ...string) *game.GameInfo {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		writeFile(t, filepath.Join(dir, name), "MZ")
	}
	sc := testScanner(t, dir, 3, dir)
	return mustResult(t, runScan(t, sc), dir).GameInfo
}

// --- 1. a simple folder with steam_appid.txt + game.exe --------------------

func TestScanSimpleSteamFolder(t *testing.T) {
	t.Run("plain app id file", func(t *testing.T) {
		lib := t.TempDir()
		gameDir := mkdirs(t, lib, "SimpleGame")
		writeFile(t, filepath.Join(gameDir, "steam_appid.txt"), "123456")
		writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

		r := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir)
		if !r.IsNew {
			t.Error("a directory that has never been identified must be reported as new")
		}

		info := r.GameInfo
		if info.ID != "steam_123456" {
			t.Errorf("ID = %q, want %q", info.ID, "steam_123456")
		}
		if got := info.PrimaryPlatform(); got != "steam" {
			t.Errorf("PrimaryPlatform = %q, want %q", got, "steam")
		}
		if got := info.PrimaryPlatformID(); got != "123456" {
			t.Errorf("PrimaryPlatformID = %q, want %q", got, "123456")
		}
		if len(info.Executables) != 1 {
			t.Fatalf("Executables = %v, want exactly game.exe", executablePaths(info))
		}
		if info.Executables[0].Path != "game.exe" {
			t.Errorf("Executables[0].Path = %q, want %q", info.Executables[0].Path, "game.exe")
		}
		if p := primaryOf(t, info); p.Path != "game.exe" {
			t.Errorf("primary launcher = %q, want game.exe", p.Path)
		}
	})

	t.Run("UTF-8 BOM and trailing newline", func(t *testing.T) {
		lib := t.TempDir()
		gameDir := mkdirs(t, lib, "BomGame")
		// Steam's own tooling and Windows editors both produce this.
		writeFile(t, filepath.Join(gameDir, "steam_appid.txt"), "\uFEFF373737\r\n")
		writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

		info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir).GameInfo
		if info.ID != "steam_373737" {
			t.Errorf("ID = %q, want %q (BOM and CRLF must be stripped)", info.ID, "steam_373737")
		}
		if got := info.PrimaryPlatformID(); got != "373737" {
			t.Errorf("PrimaryPlatformID = %q, want %q", got, "373737")
		}
	})

	t.Run("a malformed app id file is ignored", func(t *testing.T) {
		for _, content := range []string{"", "\r\n", "   ", "123 456\n"} {
			lib := t.TempDir()
			gameDir := mkdirs(t, lib, "BadAppID")
			writeFile(t, filepath.Join(gameDir, "steam_appid.txt"), content)
			writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

			info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir).GameInfo
			if strings.HasPrefix(info.ID, "steam_") {
				t.Errorf("app id file %q produced ID %q, want a local game", content, info.ID)
			}
			if got := info.PrimaryPlatform(); got != "" {
				t.Errorf("app id file %q produced platform %q, want none", content, got)
			}
		}
	})
}

// --- 2. steam_appid.txt in a parent directory ------------------------------

func TestScanUsesSteamAppIDFromAParentDirectory(t *testing.T) {
	lib := t.TempDir()
	writeFile(t, filepath.Join(lib, "steam_appid.txt"), "424242")

	// Two levels below the library root, which is as far as the lookup climbs.
	deep := mkdirs(t, lib, "Collection", "Deep")
	writeFile(t, filepath.Join(deep, "game.exe"), "MZ")

	info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), deep).GameInfo
	if info.ID != "steam_424242" {
		t.Errorf("ID = %q, want %q (the app id two levels up must be used)", info.ID, "steam_424242")
	}
	if got := info.PrimaryPlatform(); got != "steam" {
		t.Errorf("PrimaryPlatform = %q, want %q", got, "steam")
	}
}

func TestScanIgnoresSteamAppIDAboveTheLibraryRoot(t *testing.T) {
	base := t.TempDir()
	// The app id file sits next to the library root, not inside it: it belongs to
	// a different library and must not be picked up.
	writeFile(t, filepath.Join(base, "steam_appid.txt"), "999999")
	lib := mkdirs(t, base, "Library")
	gameDir := mkdirs(t, lib, "Orphan")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

	info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir).GameInfo
	if strings.HasPrefix(info.ID, "steam_") {
		t.Errorf("ID = %q: an app id above the library root must be ignored", info.ID)
	}
	if got := info.PrimaryPlatform(); got != "" {
		t.Errorf("PrimaryPlatform = %q, want no platform for a local game", got)
	}
}

// --- 3. Steam ACF manifests ------------------------------------------------

// acfManifest is shaped like a real appmanifest_<id>.acf: nested braces,
// tab-aligned "key"	"value" pairs and an AppState block.
const acfManifest = `"AppState"
{
	"appid"		"570"
	"Universe"		"1"
	"name"		"Dota 2"
	"StateFlags"		"4"
	"installdir"		"TheGame"
	"LastUpdated"		"1700000000"
	"SizeOnDisk"		"12345678"
	"UserConfig"
	{
		"language"		"english"
	}
	"InstalledDepots"
	{
		"570"
		{
			"manifest"		"1111111111111111111"
			"size"		"22222222"
		}
	}
}
`

func TestScanSteamACFManifest(t *testing.T) {
	lib := t.TempDir()
	steamapps := mkdirs(t, lib, "steamapps")
	gameDir := mkdirs(t, steamapps, "common", "TheGame")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")
	writeFile(t, filepath.Join(steamapps, "appmanifest_570.acf"), acfManifest)
	// A corrupt entry that sorts before the real manifest and cannot be read as
	// a file: the lookup has to skip it and keep going.
	mkdirs(t, steamapps, "appmanifest_000.acf")

	info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir).GameInfo
	if info.ID != "steam_570" {
		t.Errorf("ID = %q, want %q (the app id comes from the manifest)", info.ID, "steam_570")
	}
	if got := info.PrimaryPlatformID(); got != "570" {
		t.Errorf("PrimaryPlatformID = %q, want %q", got, "570")
	}
	if got := info.PrimaryPlatform(); got != "steam" {
		t.Errorf("PrimaryPlatform = %q, want %q", got, "steam")
	}
	if info.Title != "Dota 2" {
		t.Errorf("Title = %q, want %q (the manifest name wins over the folder name)", info.Title, "Dota 2")
	}
	if len(info.Executables) != 1 || info.Executables[0].Path != "game.exe" {
		t.Errorf("Executables = %v, want [game.exe]", executablePaths(info))
	}
}

func TestScanIgnoresACFManifestWithADifferentInstallDir(t *testing.T) {
	lib := t.TempDir()
	steamapps := mkdirs(t, lib, "steamapps")
	// The manifest belongs to TheGame, not to this folder.
	other := mkdirs(t, steamapps, "common", "OtherGame")
	writeFile(t, filepath.Join(other, "game.exe"), "MZ")
	writeFile(t, filepath.Join(steamapps, "appmanifest_570.acf"), acfManifest)

	info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), other).GameInfo
	if strings.HasPrefix(info.ID, "steam_") {
		t.Errorf("ID = %q: a manifest whose installdir does not match must be ignored", info.ID)
	}
	if info.Title != "OtherGame" {
		t.Errorf("Title = %q, want the folder name %q", info.Title, "OtherGame")
	}
	if got := info.PrimaryPlatform(); got != "" {
		t.Errorf("PrimaryPlatform = %q, want no platform", got)
	}
}

func TestParseACF(t *testing.T) {
	info := parseACF(acfManifest)
	if info.AppID != "570" {
		t.Errorf("AppID = %q, want %q", info.AppID, "570")
	}
	if info.Name != "Dota 2" {
		t.Errorf("Name = %q, want %q", info.Name, "Dota 2")
	}
	if info.InstallDir != "TheGame" {
		t.Errorf("InstallDir = %q, want %q", info.InstallDir, "TheGame")
	}
	if info.LastUpdated != "1700000000" {
		t.Errorf("LastUpdated = %q, want %q", info.LastUpdated, "1700000000")
	}
	if info.SizeOnDisk != "12345678" {
		t.Errorf("SizeOnDisk = %q, want %q", info.SizeOnDisk, "12345678")
	}

	t.Run("keys outside an AppState block are ignored", func(t *testing.T) {
		info := parseACF("\"Other\"\n{\n\t\"appid\"\t\t\"1\"\n\t\"name\"\t\t\"Nope\"\n}\n")
		if info.AppID != "" || info.Name != "" || info.InstallDir != "" {
			t.Errorf("parseACF = %+v, want every field empty", info)
		}
	})

	t.Run("a truncated value yields an empty field", func(t *testing.T) {
		// "appid" with no value at all, and a value whose closing quote is gone.
		info := parseACF("\"AppState\"\n{\n\t\"appid\"\n\t\"name\"\t\t\"unterminated\n\t\"installdir\"\t\t\"Dir\"\n}\n")
		if info.AppID != "" {
			t.Errorf("AppID = %q, want empty", info.AppID)
		}
		if info.Name != "" {
			t.Errorf("Name = %q, want empty", info.Name)
		}
		if info.InstallDir != "Dir" {
			t.Errorf("InstallDir = %q, want %q: a broken line must not stop the parse", info.InstallDir, "Dir")
		}
	})
}

// --- 4. scripts are launchable ---------------------------------------------

func TestScanRecognisesScriptLaunchers(t *testing.T) {
	for _, name := range []string{"run.bat", "run.cmd", "run.ps1"} {
		t.Run(name, func(t *testing.T) {
			lib := t.TempDir()
			gameDir := mkdirs(t, lib, "ScriptGame")
			// The folder holds nothing but the script.
			writeFile(t, filepath.Join(gameDir, name), "@echo off\r\n")

			info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir).GameInfo
			if len(info.Executables) != 1 {
				t.Fatalf("Executables = %v, want exactly [%s]", executablePaths(info), name)
			}
			if info.Executables[0].Path != name {
				t.Errorf("Executables[0].Path = %q, want %q", info.Executables[0].Path, name)
			}
			if p := primaryOf(t, info); p.Path != name || !p.Primary {
				t.Errorf("primary launcher = %+v, want %s flagged primary", p, name)
			}
			if got, want := info.Executables[0].Name, strings.TrimSuffix(name, filepath.Ext(name)); got != want {
				t.Errorf("Executables[0].Name = %q, want the file stem %q", got, want)
			}
		})
	}
}

func TestPlatformIsLauncher(t *testing.T) {
	for _, name := range []string{"game.exe", "Game.EXE", "run.bat", "run.cmd", "run.ps1", "shortcut.lnk"} {
		if !platform.IsLauncher(name) {
			t.Errorf("platform.IsLauncher(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"readme.txt", "steam_appid.txt", "notes", "game.exe.bak"} {
		if platform.IsLauncher(name) {
			t.Errorf("platform.IsLauncher(%q) = true, want false", name)
		}
	}
}

// --- 5. utility executables are filtered -----------------------------------

func TestScanFiltersUtilityExecutables(t *testing.T) {
	utilities := []string{
		"unins000.exe",
		"UnityCrashHandler64.exe",
		"vcredist_x64.exe",
		"DXSETUP.exe",
		"crashreport.exe",
		"updater.exe",
	}
	kept := []string{"game.exe", "launcher.exe"}

	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "FilterGame")
	for _, name := range append(append([]string{}, utilities...), kept...) {
		writeFile(t, filepath.Join(gameDir, name), "MZ")
	}

	info := mustResult(t, runScan(t, testScanner(t, lib, 3, lib)), gameDir).GameInfo
	got := executableSet(info)
	for _, name := range utilities {
		if got[name] {
			t.Errorf("%s was kept as a launcher, want it filtered out", name)
		}
	}
	for _, name := range kept {
		if !got[name] {
			t.Errorf("%s was filtered out, want it kept", name)
		}
	}
	if len(info.Executables) != len(kept) {
		t.Errorf("Executables = %v, want exactly %v", executablePaths(info), kept)
	}
	if p := primaryOf(t, info); p.Path != "game.exe" {
		t.Errorf("primary launcher = %q, want game.exe", p.Path)
	}
}

// --- 6. primary launcher choice --------------------------------------------

func TestPrimaryLauncherChoice(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"game loses to nothing", []string{"game.exe"}, "game.exe"},
		{"game beats launcher", []string{"launcher.exe", "game.exe"}, "game.exe"},
		{"game beats every later keyword", []string{"app.exe", "main.exe", "start.exe", "launcher.exe", "game.exe"}, "game.exe"},
		{"launcher beats start, main and app", []string{"start.exe", "main.exe", "app.exe", "launcher.exe"}, "launcher.exe"},
		{"start beats main and app", []string{"main.exe", "app.exe", "start.exe"}, "start.exe"},
		{"main beats app", []string{"app.exe", "main.exe"}, "main.exe"},
		{"app beats an unrelated name", []string{"unrelated.exe", "app.exe"}, "app.exe"},
		{"the shortest name is the fallback", []string{"averylongname.exe", "b.exe"}, "b.exe"},
		{"a real executable beats a keyword script", []string{"game.bat", "zip.exe"}, "zip.exe"},
		{"a real executable beats an earlier keyword script", []string{"game.bat", "app.exe"}, "app.exe"},
		{"scripts still get a primary", []string{"run.bat", "other.ps1"}, "run.bat"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := scanLauncherDir(t, tc.files...)
			if got := primaryOf(t, info).Path; got != tc.want {
				t.Errorf("primary launcher = %q, want %q (launchers: %v)", got, tc.want, executablePaths(info))
			}
		})
	}
}

// --- 7. regression: a root that looks like a game is still descended into ---

// TestScanDescendsIntoConfiguredRootThatLooksLikeAGame is the regression test
// for a bug where the walk stopped at the first directory that contained a
// launchable file. A configured root holding one stray launcher — a setup.exe
// copied next to the game folders, or a shortcut — was therefore recorded as a
// single game and every folder underneath it was never visited, so one stray
// file at the top of the library silently hid the entire library.
func TestScanDescendsIntoConfiguredRootThatLooksLikeAGame(t *testing.T) {
	lib := t.TempDir()
	writeFile(t, filepath.Join(lib, "setup.exe"), "MZ") // the stray launcher

	top := mkdirs(t, lib, "TopLevelGame")
	writeFile(t, filepath.Join(top, "game.exe"), "MZ")
	nested := mkdirs(t, lib, "Collection", "NestedGame")
	writeFile(t, filepath.Join(nested, "game.exe"), "MZ")

	results := runScan(t, testScanner(t, lib, 3, lib))

	root := mustResult(t, results, lib)
	if !root.IsNew {
		t.Error("the configured root itself was not identified as a new game")
	}
	mustResult(t, results, top)
	mustResult(t, results, nested)

	games := 0
	for _, r := range results {
		if r.Error != "" {
			t.Errorf("unexpected scan error for %s: %s", r.GameDir, r.Error)
			continue
		}
		games++
	}
	if games != 3 {
		t.Errorf("identified %d games, want 3 (the root plus both nested games)", games)
	}
}

// --- 8. support folders below a game are not games -------------------------

func TestScanDoesNotReportDirectoriesNestedInsideAGame(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "TheGame")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

	binDir := mkdirs(t, gameDir, "bin")
	writeFile(t, filepath.Join(binDir, "helper.exe"), "MZ")
	engineDir := mkdirs(t, gameDir, "Engine", "Binaries")
	writeFile(t, filepath.Join(engineDir, "ue4.exe"), "MZ")

	results := runScan(t, testScanner(t, lib, 5, lib))
	mustResult(t, results, gameDir)
	if r := resultFor(results, binDir); r != nil {
		t.Errorf("%s is support content inside a game and must not be reported as a game", binDir)
	}
	if r := resultFor(results, engineDir); r != nil {
		t.Errorf("%s is support content inside a game and must not be reported as a game", engineDir)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want only the game itself: %s", len(results), reportedDirs(results))
	}
}

// --- 9. .gamemanager only, and the shared game-directory definition ---------

func TestScanKeepsMetadataOnlyGameDirectory(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "MetadataOnly")

	// The launcher is gone; only the managed folder is left. Such a directory is
	// still a game: its record must be kept rather than reported as broken.
	record := game.New(lib, gameDir, []game.Executable{{Path: "gone.exe", Name: "gone", Primary: true}}, "")
	record.Title = "Metadata Only"
	if err := record.Save(); err != nil {
		t.Fatalf("save record: %v", err)
	}

	results := runScan(t, testScanner(t, lib, 3, lib))
	r := mustResult(t, results, gameDir) // mustResult fails on any Error
	if r.IsNew {
		t.Error("an existing record must not be reported as new")
	}
	if r.GameInfo.ID != record.ID || r.GameInfo.Title != "Metadata Only" {
		t.Errorf("kept record = %s/%q, want %s/%q", r.GameInfo.ID, r.GameInfo.Title, record.ID, "Metadata Only")
	}
	if !IsGameDirPath(gameDir) {
		t.Error("IsGameDirPath must report true for a folder holding only .gamemanager")
	}
}

func TestIsGameDirectoryAcceptsLauncherOrMetadata(t *testing.T) {
	launcherOnly := t.TempDir()
	writeFile(t, filepath.Join(launcherOnly, "game.exe"), "MZ")

	metadataOnly := mkdirs(t, t.TempDir(), "Game")
	writeFile(t, filepath.Join(metadataOnly, game.ManagerDirName, "gameinfo.json"), `{"id":"x"}`)

	unrelated := t.TempDir()
	writeFile(t, filepath.Join(unrelated, "notes.txt"), "not a game")

	empty := t.TempDir()

	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"a launcher-only folder", launcherOnly, true},
		{"a .gamemanager-only folder", metadataOnly, true},
		{"an unrelated folder", unrelated, false},
		{"an empty folder", empty, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := os.ReadDir(tc.dir)
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			if got := IsGameDirectory(entries); got != tc.want {
				t.Errorf("IsGameDirectory(%s) = %v, want %v", tc.dir, got, tc.want)
			}
			if got := IsGameDirPath(tc.dir); got != tc.want {
				t.Errorf("IsGameDirPath(%s) = %v, want %v", tc.dir, got, tc.want)
			}
		})
	}

	t.Run("a missing directory", func(t *testing.T) {
		if IsGameDirPath(filepath.Join(t.TempDir(), "missing")) {
			t.Error("IsGameDirPath must report false for a directory that does not exist")
		}
	})
}

// --- 10. regression: a forced scan must preserve user data -----------------

// TestForceScanAllPreservesUserData is the regression test for a bug where
// ForceScanAll built a brand-new record for every directory and carried over
// nothing but the launchers. Pressing "rescan" therefore silently discarded the
// star rating, the tags, a custom title, everything the scrapers had produced,
// and the playtime history.
func TestForceScanAllPreservesUserData(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "Alpha")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

	sc := testScanner(t, lib, 3, lib)
	info := mustResult(t, runScan(t, sc), gameDir).GameInfo

	info.Starred = true
	info.Tags = []string{"rpg", "favourite"}
	info.Title = "Custom Title"
	info.Metadata = &game.Metadata{
		Description: "a scraped description",
		Developer:   "a developer",
		Tags:        []string{"RPG"},
	}
	info.LastPlayedAt = "2026-01-02T03:04:05Z"
	info.TotalPlaytime = 4242
	info.PreferredSource = "vndb"
	if err := info.Save(); err != nil {
		t.Fatalf("save user data: %v", err)
	}

	got := mustResult(t, runForceScan(t, sc), gameDir).GameInfo

	if got.ID != info.ID {
		t.Errorf("ID = %q, want %q", got.ID, info.ID)
	}
	if !got.Starred {
		t.Error("the star rating was lost by the forced scan")
	}
	if !reflect.DeepEqual(got.Tags, []string{"rpg", "favourite"}) {
		t.Errorf("Tags = %v, want [rpg favourite]", got.Tags)
	}
	if got.Title != "Custom Title" {
		t.Errorf("Title = %q, want %q (a custom title must survive a rescan)", got.Title, "Custom Title")
	}
	if got.Metadata == nil || got.Metadata.Description != "a scraped description" || got.Metadata.Developer != "a developer" {
		t.Errorf("Metadata = %+v, want the scraped description and developer", got.Metadata)
	}
	if got.LastPlayedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("LastPlayedAt = %q, want %q", got.LastPlayedAt, "2026-01-02T03:04:05Z")
	}
	if got.TotalPlaytime != 4242 {
		t.Errorf("TotalPlaytime = %d, want 4242", got.TotalPlaytime)
	}
	if got.PreferredSource != "vndb" {
		t.Errorf("PreferredSource = %q, want %q", got.PreferredSource, "vndb")
	}
	if len(got.Executables) != 1 || got.Executables[0].Path != "game.exe" || !got.Executables[0].Primary {
		t.Errorf("Executables = %+v, want game.exe flagged primary", got.Executables)
	}
}

func TestForceScanRefreshesARecordThatHadNoLauncher(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "Epsilon")
	// A record with a custom title and no launchers: the executable was removed.
	record := game.New(lib, gameDir, nil, "")
	record.Title = "Kept Title"
	if err := record.Save(); err != nil {
		t.Fatalf("save record: %v", err)
	}

	sc := testScanner(t, lib, 3, lib)
	// The launcher reappears, so the forced scan has new information to add.
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

	r := mustResult(t, runForceScan(t, sc), gameDir)
	if r.IsNew {
		t.Error("a directory with an existing record must not be reported as new")
	}
	if r.GameInfo.Title != "Kept Title" {
		t.Errorf("Title = %q, want the user's title to win even when the old record had no launchers", r.GameInfo.Title)
	}
	if len(r.GameInfo.Executables) != 1 || r.GameInfo.Executables[0].Path != "game.exe" {
		t.Errorf("Executables = %v, want the rediscovered game.exe", executablePaths(r.GameInfo))
	}
}

// --- 11. IsNew on the first and the second pass ----------------------------

func TestScanAllReportsIsNewOnceOnly(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "Bravo")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

	sc := testScanner(t, lib, 3, lib)

	first := mustResult(t, runScan(t, sc), gameDir)
	if !first.IsNew {
		t.Error("the first scan must report IsNew = true")
	}

	second := mustResult(t, runScan(t, sc), gameDir)
	if second.IsNew {
		t.Error("the second scan must report IsNew = false")
	}
	if second.GameInfo.ID != first.GameInfo.ID {
		t.Errorf("ID changed between scans: %q then %q", first.GameInfo.ID, second.GameInfo.ID)
	}
}

// --- 12. LoadLibrary -------------------------------------------------------

func TestLoadLibraryFindsRecordedGamesWithoutWriting(t *testing.T) {
	lib := t.TempDir()
	alpha := mkdirs(t, lib, "Alpha")
	writeFile(t, filepath.Join(alpha, "game.exe"), "MZ")
	beta := mkdirs(t, lib, "Collection", "Beta")
	writeFile(t, filepath.Join(beta, "game.exe"), "MZ")

	sc := testScanner(t, lib, 3, lib)
	results := runScan(t, sc)

	wantIDs := []string{
		mustResult(t, results, alpha).GameInfo.ID,
		mustResult(t, results, beta).GameInfo.ID,
	}
	sort.Strings(wantIDs)

	// A launcher-only folder that has never been identified: LoadLibrary must
	// find nothing there and must not create a .gamemanager folder.
	launcherOnly := mkdirs(t, lib, "NeverScanned")
	writeFile(t, filepath.Join(launcherOnly, "launcher.exe"), "MZ")

	loaded := sc.LoadLibrary()
	gotIDs := make([]string, 0, len(loaded))
	for _, info := range loaded {
		gotIDs = append(gotIDs, info.ID)
	}
	sort.Strings(gotIDs)

	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("LoadLibrary returned %v, want %v", gotIDs, wantIDs)
	}
	if fsutil.DirExists(game.ManagerDir(launcherOnly)) {
		t.Error("LoadLibrary created a .gamemanager folder for a launcher-only directory")
	}
	if fsutil.Exists(game.InfoFilePath(launcherOnly)) {
		t.Error("LoadLibrary wrote a gameinfo.json for a directory that was never identified")
	}
}

// --- 13. MaxScanDepth ------------------------------------------------------

func TestScanRespectsMaxScanDepth(t *testing.T) {
	lib := t.TempDir()
	// lib\L1\L2\L3\DeepGame
	deep := mkdirs(t, lib, "L1", "L2", "L3", "DeepGame")
	writeFile(t, filepath.Join(deep, "game.exe"), "MZ")

	shallow := testScanner(t, lib, 3, lib)
	if r := resultFor(runScan(t, shallow), deep); r != nil {
		t.Errorf("a game beyond MaxScanDepth=3 was found: %s", r.GameDir)
	}

	deeper := testScanner(t, lib, 4, lib)
	info := mustResult(t, runScan(t, deeper), deep).GameInfo
	if len(info.Executables) != 1 || info.Executables[0].Path != "game.exe" {
		t.Errorf("Executables = %v, want [game.exe] once the limit allows the descent", executablePaths(info))
	}
}

// --- supporting behaviour --------------------------------------------------

func TestScanDirAndForceScanDir(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "Delta")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")

	sc := testScanner(t, lib, 3, lib)

	first := mustResult(t, sc.ScanDir(lib), gameDir)
	if !first.IsNew {
		t.Error("ScanDir must report a first-time directory as new")
	}

	forced := mustResult(t, sc.ForceScanDir(lib), gameDir)
	if forced.IsNew {
		t.Error("ForceScanDir must report an already recorded game as existing")
	}
	if forced.GameInfo.ID != first.GameInfo.ID {
		t.Errorf("ID changed: %q then %q", first.GameInfo.ID, forced.GameInfo.ID)
	}

	// A directory that cannot be read at all yields nothing, not a panic.
	missing := filepath.Join(lib, "missing")
	if got := sc.ScanDir(missing); got != nil {
		t.Errorf("ScanDir on a missing directory = %s, want nil", reportedDirs(got))
	}
	if got := sc.ForceScanDir(missing); got != nil {
		t.Errorf("ForceScanDir on a missing directory = %s, want nil", reportedDirs(got))
	}
}

func TestScanAllSkipsEmptyAndMissingGameDirectories(t *testing.T) {
	root := t.TempDir()
	// An empty entry resolves to "" and a missing one does not exist: both must
	// be skipped without inventing a result or an error.
	sc := testScanner(t, root, 3, "", `.\does-not-exist`)

	results, err := sc.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0: %s", len(results), reportedDirs(results))
	}
	if got := sc.LoadLibrary(); len(got) != 0 {
		t.Errorf("LoadLibrary returned %d games, want 0", len(got))
	}
}

func TestScanSkipsHiddenDirectories(t *testing.T) {
	lib := t.TempDir()
	hidden := mkdirs(t, lib, ".stash", "HiddenGame")
	writeFile(t, filepath.Join(hidden, "game.exe"), "MZ")

	results := runScan(t, testScanner(t, lib, 5, lib))
	if r := resultFor(results, hidden); r != nil {
		t.Errorf("a game inside a hidden directory was reported: %s", r.GameDir)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want none: %s", len(results), reportedDirs(results))
	}
}

// TestScanDoesNotOfferUtilityOnlyDirectoryAsAGame documents what the scanner
// does with a directory that holds nothing but supporting programs: unins000.exe
// and vcredist_x64.exe are filtered out, so there is nothing to launch.
//
// The consequence of the same gate is asserted by
// TestScanUtilityOnlyDirectoryHidesNestedGames.
func TestScanDoesNotOfferUtilityOnlyDirectoryAsAGame(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "SetupOnly")
	writeFile(t, filepath.Join(gameDir, "unins000.exe"), "MZ")
	writeFile(t, filepath.Join(gameDir, "vcredist_x64.exe"), "MZ")

	results := runScan(t, testScanner(t, lib, 3, lib))
	r := resultFor(results, gameDir)
	if r == nil {
		t.Fatalf("no result for %s: %s", gameDir, reportedDirs(results))
	}
	if r.Error == "" {
		t.Error("a directory with no usable launcher must not be reported as a playable game")
	}
	if r.GameInfo != nil {
		t.Errorf("GameInfo = %+v, want nil: nothing was identified", r.GameInfo)
	}
	if fsutil.DirExists(game.ManagerDir(gameDir)) {
		t.Error("a record was written for a directory with no usable launcher")
	}
}

// TestScanUtilityOnlyDirectoryHidesNestedGames is a KNOWN PRODUCTION BUG, hence
// the Skip. It is kept because a fix is a one-line change and because the bug is
// the same class of failure the walk()-descends-into-roots fix addressed.
//
// IsGameDirectory only asks whether a file carries a launchable extension, while
// findLaunchers additionally filters out supporting programs (unins*, crash
// handlers, redistributables, updaters, ...). A directory whose launchable files
// are all filtered out is therefore still treated as a game by walk(), which
// stops there: identify() finds no launcher, the directory is reported as a
// broken game, and every directory below it is never visited. A folder shipped
// as "SomeGame\unins000.exe + SomeGame\bin\game.exe" is consequently invisible,
// exactly like the stray setup.exe case that hid a whole library.
//
// The bug is reported rather than fixed: this task does not touch production
// code. Remove the Skip once walk() applies the same filter as findLaunchers.
func TestScanUtilityOnlyDirectoryHidesNestedGames(t *testing.T) {
	t.Skip("known production bug: IsGameDirectory does not apply the utility filter that findLaunchers does")

	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "SomeGame")
	writeFile(t, filepath.Join(gameDir, "unins000.exe"), "MZ")
	writeFile(t, filepath.Join(gameDir, "vcredist_x64.exe"), "MZ")
	real := mkdirs(t, gameDir, "bin")
	writeFile(t, filepath.Join(real, "game.exe"), "MZ")

	results := runScan(t, testScanner(t, lib, 5, lib))
	if r := resultFor(results, gameDir); r != nil && r.Error != "" {
		t.Errorf("%s was reported as a broken game: %s", gameDir, r.Error)
	}
	mustResult(t, results, real)
}

// TestScanReportsAFailedRecordWrite covers a record that cannot be written at
// all, here because a file occupies the .gamemanager path. The scan has to
// surface the failure in the result instead of reporting a saved game.
func TestScanReportsAFailedRecordWrite(t *testing.T) {
	lib := t.TempDir()
	gameDir := mkdirs(t, lib, "Blocked")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "MZ")
	writeFile(t, filepath.Join(gameDir, game.ManagerDirName), "not a directory")

	results := runScan(t, testScanner(t, lib, 3, lib))
	r := resultFor(results, gameDir)
	if r == nil {
		t.Fatalf("no result for %s: %s", gameDir, reportedDirs(results))
	}
	if r.Error == "" {
		t.Error("a failed record write must be reported in the result")
	}
	if r.GameInfo == nil {
		t.Error("the result should still carry the record that could not be saved")
	}
}

func TestLoadLibraryHonoursMaxScanDepth(t *testing.T) {
	lib := t.TempDir()
	alpha := mkdirs(t, lib, "Alpha")
	writeFile(t, filepath.Join(alpha, "game.exe"), "MZ")
	beta := mkdirs(t, lib, "Collection", "Beta")
	writeFile(t, filepath.Join(beta, "game.exe"), "MZ")

	sc := testScanner(t, lib, 1, lib)
	results := runScan(t, sc)
	if r := resultFor(results, beta); r != nil {
		t.Fatalf("discovery found %s beyond MaxScanDepth=1", beta)
	}
	wantID := mustResult(t, results, alpha).GameInfo.ID

	loaded := sc.LoadLibrary()
	if len(loaded) != 1 || loaded[0].ID != wantID {
		t.Errorf("LoadLibrary = %v, want only %q (the startup cache load must use the same depth limit)",
			gameIDs(loaded), wantID)
	}
}

func TestScannerExposesRootAndConfig(t *testing.T) {
	cfg := &config.Config{GameDirectories: []string{`.\Games`}, MaxScanDepth: 3}
	sc := New(`C:\Library`, cfg)

	if sc.Root() != `C:\Library` {
		t.Errorf("Root() = %q", sc.Root())
	}
	if sc.Config() != cfg {
		t.Error("Config() must return the configuration the scanner was built with")
	}
}
