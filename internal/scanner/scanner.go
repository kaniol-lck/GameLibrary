// Package scanner discovers games inside the configured library directories.
//
// Discovery is deliberately conservative: a directory is a game when it holds a
// launchable file (see internal/platform) or an existing ".gamemanager" folder.
// The walk always descends into the configured root directories, because a root
// that happens to contain an installer or a shortcut used to be mistaken for a
// single game and hid the entire library behind it.
package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"GameLibrary/internal/config"
	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
	"GameLibrary/internal/logger"
	"GameLibrary/internal/platform"
)

// ScanResult describes the outcome of identifying one directory.
type ScanResult struct {
	GameDir  string         `json:"gameDir"`
	GameInfo *game.GameInfo `json:"gameInfo"`
	IsNew    bool           `json:"isNew"`
	Error    string         `json:"error,omitempty"`
}

// Scanner walks the library and produces game records.
type Scanner struct {
	root   string
	config *config.Config
}

// New creates a scanner rooted at the directory containing the executable.
func New(root string, cfg *config.Config) *Scanner {
	return &Scanner{root: root, config: cfg}
}

// Config returns the configuration the scanner is using. The application
// replaces the whole scanner when the configuration changes rather than mutating
// this pointer, so the value here is always current.
func (s *Scanner) Config() *config.Config { return s.config }

// Root returns the library root.
func (s *Scanner) Root() string { return s.root }

// ScanAll discovers games without re-identifying those already on disk.
func (s *Scanner) ScanAll() ([]ScanResult, error) {
	return s.scanAll(false)
}

// ForceScanAll re-identifies every game directory from scratch while preserving
// user data (star rating, tags, scraped metadata, playtime).
func (s *Scanner) ForceScanAll() ([]ScanResult, error) {
	return s.scanAll(true)
}

func (s *Scanner) scanAll(force bool) ([]ScanResult, error) {
	logger.ScanStarted(s.config.GameDirectories, s.config.MaxScanDepth)

	var results []ScanResult
	for _, relDir := range s.config.GameDirectories {
		absDir := fsutil.Resolve(s.root, relDir)
		if absDir == "" {
			continue
		}
		if !fsutil.DirExists(absDir) {
			logger.ScanGameDirNotExist(absDir)
			continue
		}
		results = append(results, s.walk(absDir, 0, force, true)...)
	}

	newCount, existingCount := 0, 0
	for _, r := range results {
		if r.Error != "" {
			continue
		}
		if r.IsNew {
			newCount++
			continue
		}
		existingCount++
	}
	logger.ScanFinished(len(results), newCount, existingCount)

	return results, nil
}

// ScanDir discovers games below one directory. It is used by the file watcher
// when a new folder appears.
func (s *Scanner) ScanDir(dir string) []ScanResult {
	return s.walk(dir, 0, false, true)
}

// ForceScanDir re-identifies games below one directory, preserving user data.
func (s *Scanner) ForceScanDir(dir string) []ScanResult {
	return s.walk(dir, 0, true, true)
}

// walk descends into dir. root marks a configured game directory, which is
// always descended into even when it looks like a game itself.
func (s *Scanner) walk(dir string, depth int, force bool, root bool) []ScanResult {
	logger.ScanDirectoryEntered(dir, depth)

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("scan: cannot read directory", "dir", dir, "error", err.Error())
		return nil
	}

	isGame := IsGameDirectory(entries)
	if isGame {
		result := s.identify(dir, entries, force)
		if !root {
			return []ScanResult{result}
		}
		// A configured root that is itself a game still gets descended into, so
		// a stray launcher at the top of the library cannot hide the rest.
		results := []ScanResult{result}
		if depth < s.config.MaxScanDepth {
			results = append(results, s.walkChildren(dir, entries, depth, force)...)
		}
		return results
	}

	if depth >= s.config.MaxScanDepth {
		logger.ScanMaxDepthReached(dir, depth, s.config.MaxScanDepth)
		return nil
	}
	return s.walkChildren(dir, entries, depth, force)
}

func (s *Scanner) walkChildren(dir string, entries []os.DirEntry, depth int, force bool) []ScanResult {
	var results []ScanResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			logger.ScanDirectorySkipped(filepath.Join(dir, name), "hidden directory")
			continue
		}
		results = append(results, s.walk(filepath.Join(dir, name), depth+1, force, false)...)
	}
	return results
}

// IsGameDirectory reports whether a directory listing describes a game.
//
// This is the single definition of "game directory" used by both discovery and
// the startup cache load. Previously the scanner required a launcher while the
// application cache also accepted a bare ".gamemanager" folder, so the two
// disagreed about the same directory.
func IsGameDirectory(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == game.ManagerDirName {
				return true
			}
			continue
		}
		if platform.IsLauncher(e.Name()) {
			return true
		}
	}
	return false
}

// IsGameDirPath reports whether a directory on disk is a game directory.
func IsGameDirPath(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return IsGameDirectory(entries)
}

// identify builds or refreshes the record for one game directory.
func (s *Scanner) identify(dir string, entries []os.DirEntry, force bool) ScanResult {
	existing, _ := game.LoadFromDir(dir)

	if !force && existing != nil {
		logger.ScanGameAlreadyExists(dir, existing.ID)
		return ScanResult{GameDir: dir, GameInfo: existing, IsNew: false}
	}

	steamAppID := s.readSteamAppID(dir)
	acf := (*acfInfo)(nil)
	if steamAppID == "" {
		// The ACF manifest carries both the app ID and the real title, so read
		// it once and reuse it instead of parsing the file twice.
		acf = s.readACF(dir)
		if acf != nil {
			steamAppID = acf.AppID
		}
	} else {
		acf = s.readACF(dir)
	}
	if steamAppID != "" {
		logger.ScanGameSteamDetected(dir, steamAppID)
	}

	launchers := s.findLaunchers(entries)
	if len(launchers) == 0 {
		// A directory that only retains metadata (its executable was removed, or
		// a force rescan of a folder whose launcher is gone) keeps its record
		// instead of being reported as broken.
		if existing != nil {
			logger.Debug("scan: no launcher found, keeping existing record", "dir", dir, "id", existing.ID)
			return ScanResult{GameDir: dir, GameInfo: existing, IsNew: false}
		}
		logger.ScanGameNoExecutable(dir)
		return ScanResult{GameDir: dir, Error: "no launchable file found"}
	}

	info := game.New(s.root, dir, launchers, steamAppID)
	if existing != nil {
		// A force rescan must not discard what the user or a scraper produced.
		preserveUserData(existing, info)
	}

	if steamAppID != "" && acf != nil && acf.Name != "" {
		info.Title = acf.Name
		logger.Debug("ACF metadata imported", "gameId", info.ID, "name", acf.Name, "lastUpdated", acf.LastUpdated)
	}

	isNew := existing == nil
	logger.ScanGameDiscovered(dir, info.ID, info.Title, info.PrimaryPlatform(), len(launchers))

	if err := info.Save(); err != nil {
		logger.ScanGameSaveFailed(dir, err)
		return ScanResult{GameDir: dir, GameInfo: info, IsNew: isNew, Error: err.Error()}
	}
	return ScanResult{GameDir: dir, GameInfo: info, IsNew: isNew}
}

// preserveUserData carries everything that is not rediscoverable from the game
// directory over to a freshly identified record.
func preserveUserData(old, fresh *game.GameInfo) {
	fresh.Title = old.Title
	fresh.TitleNative = old.TitleNative
	fresh.Type = old.Type
	fresh.Platforms = old.Platforms
	fresh.Aliases = old.Aliases
	fresh.PreferredSource = old.PreferredSource
	fresh.Metadata = old.Metadata
	fresh.SavePaths = old.SavePaths
	fresh.Starred = old.Starred
	fresh.Tags = old.Tags
	fresh.TotalPlaytime = old.TotalPlaytime
	fresh.LastPlayedAt = old.LastPlayedAt
	fresh.CoverVersion = old.CoverVersion
	fresh.ScannedAt = old.ScannedAt
	if len(fresh.Executables) > 0 && len(old.Executables) > 0 {
		// Keep the user's chosen primary launcher when it still exists.
		oldPrimary := old.PrimaryExecutable()
		if oldPrimary != nil {
			for i := range fresh.Executables {
				fresh.Executables[i].Primary = fresh.Executables[i].Path == oldPrimary.Path
			}
		}
	}
}

// readSteamAppID looks for a steam_appid.txt in the directory or up to two of its
// parents, never climbing above the library root.
func (s *Scanner) readSteamAppID(dir string) string {
	current := dir
	for i := 0; i < 3; i++ {
		data, err := os.ReadFile(filepath.Join(current, "steam_appid.txt"))
		if err == nil {
			if id := firstLine(data); id != "" {
				return id
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		if s.root != "" && !fsutil.IsWithin(s.root, parent) {
			break
		}
		current = parent
	}
	return ""
}

func firstLine(data []byte) string {
	// Steam writes a bare app id, but trailing newlines and a UTF-8 BOM are both
	// common when the file was produced by Windows tooling.
	text := strings.TrimPrefix(string(data), "\uFEFF")
	if idx := strings.IndexAny(text, "\r\n"); idx >= 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, " \t") {
		return ""
	}
	return text
}

type acfInfo struct {
	AppID       string
	Name        string
	InstallDir  string
	LastUpdated string
	SizeOnDisk  string
}

// readACF finds the Steam app manifest that owns this game directory.
//
// Steam only tracks installdir, so a game folder is matched by name against the
// manifests in the parent steamapps directory.
func (s *Scanner) readACF(gameDir string) *acfInfo {
	parent := filepath.Dir(gameDir)
	if !strings.EqualFold(filepath.Base(parent), "common") {
		return nil
	}
	steamappsDir := filepath.Dir(parent)
	entries, err := os.ReadDir(steamappsDir)
	if err != nil {
		return nil
	}
	dirName := filepath.Base(gameDir)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "appmanifest_") || !strings.HasSuffix(name, ".acf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(steamappsDir, name))
		if err != nil {
			continue
		}
		info := parseACF(string(data))
		if info.AppID != "" && strings.EqualFold(info.InstallDir, dirName) {
			return info
		}
	}
	return nil
}

// parseACF extracts the fields of interest from a Valve KeyValues manifest.
func parseACF(content string) *acfInfo {
	info := &acfInfo{}
	inAppState := false
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == `"AppState"` {
			inAppState = true
			continue
		}
		if !inAppState {
			continue
		}
		for _, field := range []string{"appid", "name", "installdir", "LastUpdated", "SizeOnDisk"} {
			prefix := `"` + field + `"`
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			value := acfValue(line)
			switch field {
			case "appid":
				info.AppID = value
			case "name":
				info.Name = value
			case "installdir":
				info.InstallDir = value
			case "LastUpdated":
				info.LastUpdated = value
			case "SizeOnDisk":
				info.SizeOnDisk = value
			}
		}
	}
	return info
}

// acfValue returns the quoted value of a `"key"		"value"` line.
func acfValue(line string) string {
	rest := line
	if idx := strings.Index(rest, `"`); idx >= 0 {
		rest = rest[idx+1:]
	}
	if idx := strings.Index(rest, `"`); idx >= 0 {
		rest = rest[idx+1:]
	}
	start := strings.Index(rest, `"`)
	if start < 0 {
		return ""
	}
	rest = rest[start+1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// findLaunchers lists the startable files in a game directory, marking one as
// primary.
func (s *Scanner) findLaunchers(entries []os.DirEntry) []game.Executable {
	var launchers []game.Executable
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !platform.IsLauncher(name) {
			continue
		}
		lower := strings.ToLower(name)
		switch {
		case strings.HasPrefix(lower, "unins"):
			logger.ScanExeFiltered(name, "uninstaller")
			continue
		case strings.HasPrefix(lower, "unitycrashhandler"):
			logger.ScanExeFiltered(name, "unity crash handler")
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if isUtilityLauncher(strings.ToLower(stem)) {
			logger.ScanExeFiltered(name, "utility")
			continue
		}
		logger.ScanExeFound(name)
		launchers = append(launchers, game.Executable{Path: name, Name: stem})
	}
	if len(launchers) == 0 {
		return nil
	}

	sort.Slice(launchers, func(i, j int) bool {
		return strings.ToLower(launchers[i].Path) < strings.ToLower(launchers[j].Path)
	})
	primary := pickPrimaryLauncher(launchers)
	for i := range launchers {
		if launchers[i].Path == primary.Path {
			launchers[i].Primary = true
			break
		}
	}
	return launchers
}

func pickPrimaryLauncher(launchers []game.Executable) game.Executable {
	// A real game binary beats a wrapper script.
	executables := make([]game.Executable, 0, len(launchers))
	for _, l := range launchers {
		if strings.EqualFold(filepath.Ext(l.Path), ".exe") {
			executables = append(executables, l)
		}
	}
	if len(executables) == 0 {
		executables = launchers
	}

	for _, keyword := range []string{"game", "launcher", "start", "main", "app"} {
		for _, exe := range executables {
			if strings.Contains(strings.ToLower(exe.Name), keyword) {
				logger.ScanExePrimaryPicked(exe.Name, keyword)
				return exe
			}
		}
	}

	shortest := executables[0]
	for _, exe := range executables[1:] {
		if len(exe.Path) < len(shortest.Path) {
			shortest = exe
		}
	}
	logger.ScanExeShortestPicked(shortest.Name)
	return shortest
}

// utilityLaunchers are supporting programs shipped alongside games that must not
// be offered as the thing to start.
var utilityLaunchers = []string{
	"crashreport", "bugreport", "errorreport",
	"patcher", "patch", "update", "updater",
	"uninstall", "unins000", "unins001",
	"dxsetup", "vcredist", "redist",
	"dotnet", "directx", "xna",
	"steamsetup", "eadesktop", "uplayinstaller",
}

func isUtilityLauncher(name string) bool {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	for _, pattern := range utilityLaunchers {
		if stem == pattern || strings.Contains(stem, pattern) {
			return true
		}
	}
	return false
}

// LoadLibrary returns every game already recorded under the configured
// directories, without writing anything. It backs the startup cache.
func (s *Scanner) LoadLibrary() []*game.GameInfo {
	var found []*game.GameInfo
	for _, relDir := range s.config.GameDirectories {
		absDir := fsutil.Resolve(s.root, relDir)
		if absDir == "" || !fsutil.DirExists(absDir) {
			continue
		}
		s.loadFrom(absDir, 0, &found)
	}
	return found
}

func (s *Scanner) loadFrom(dir string, depth int, out *[]*game.GameInfo) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	isGame := IsGameDirectory(entries)
	if isGame {
		if info, err := game.LoadFromDir(dir); err == nil && info != nil {
			*out = append(*out, info)
		}
		// Nested directories below a game are supporting content, not games.
		return
	}
	if depth >= s.config.MaxScanDepth {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		s.loadFrom(filepath.Join(dir, entry.Name()), depth+1, out)
	}
}
