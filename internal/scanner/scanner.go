package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
	"GameLibrary/internal/logger"
)

type ScanResult struct {
	GameDir  string         `json:"gameDir"`
	GameInfo *game.GameInfo `json:"gameInfo"`
	IsNew    bool           `json:"isNew"`
	Error    string         `json:"error,omitempty"`
}

type Scanner struct {
	exeDir string
	config *config.Config
}

func New(exeDir string, cfg *config.Config) *Scanner {
	return &Scanner{
		exeDir: exeDir,
		config: cfg,
	}
}

func (s *Scanner) ScanAll() ([]ScanResult, error) {
	return s.scanAll(false)
}

func (s *Scanner) ForceScanAll() ([]ScanResult, error) {
	return s.scanAll(true)
}

func (s *Scanner) scanAll(force bool) ([]ScanResult, error) {
	logger.ScanStarted(s.config.GameDirectories, s.config.MaxScanDepth)

	var results []ScanResult
	for _, relDir := range s.config.GameDirectories {
		absDir := resolveDir(s.exeDir, relDir)

		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			logger.ScanGameDirNotExist(absDir)
			continue
		}

		dirResults := s.scanDirForce(absDir, 0, force)
		results = append(results, dirResults...)
	}

	newCount := 0
	existingCount := 0
	for _, r := range results {
		if r.Error != "" {
			continue
		}
		if r.IsNew {
			newCount++
		} else {
			existingCount++
		}
	}
	logger.Info("scan finished",
		"totalDirs", len(results),
		"newGames", newCount,
		"existingGames", existingCount,
	)

	return results, nil
}

func (s *Scanner) ScanDir(dir string) []ScanResult {
	return s.scanDirForce(dir, 0, false)
}

func (s *Scanner) scanDir(dir string, depth int) []ScanResult {
	return s.scanDirForce(dir, depth, false)
}

func (s *Scanner) scanDirForce(dir string, depth int, force bool) []ScanResult {
	logger.ScanDirectoryEntered(dir, depth)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	if s.isGameDirectory(entries) {
		result := s.identifyGameForce(dir, force)
		return []ScanResult{result}
	}

	if depth >= s.config.MaxScanDepth {
		logger.ScanMaxDepthReached(dir, depth, s.config.MaxScanDepth)
		return nil
	}

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

		subDir := filepath.Join(dir, name)
		subResults := s.scanDirForce(subDir, depth+1, force)
		results = append(results, subResults...)
	}
	return results
}

func (s *Scanner) isGameDirectory(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".exe") {
			return true
		}
	}
	return false
}

func (s *Scanner) identifyGame(gameDir string) ScanResult {
	return s.identifyGameForce(gameDir, false)
}

func (s *Scanner) identifyGameForce(gameDir string, force bool) ScanResult {
	if !force {
		existing, err := game.LoadFromDir(gameDir)
		if err == nil && existing != nil {
			logger.ScanGameAlreadyExists(gameDir, existing.ID)
			return ScanResult{GameDir: gameDir, GameInfo: existing, IsNew: false}
		}
	}

	entries, _ := os.ReadDir(gameDir)

	steamAppID := s.readSteamAppID(gameDir)
	if steamAppID == "" {
		steamAppID = s.readACFAppID(gameDir)
	}
	if steamAppID != "" {
		logger.ScanGameSteamDetected(gameDir, steamAppID)
	}

	executables := s.findExecutables(entries)

	if len(executables) == 0 {
		logger.ScanGameNoExecutable(gameDir)
		return ScanResult{
			GameDir: gameDir,
			Error:   "no executable found",
		}
	}

	info := game.New(gameDir, executables, steamAppID)

	if info.PrimaryPlatform() == "steam" && steamAppID != "" {
		acf := s.readACF(gameDir)
		if acf != nil {
			if acf.Name != "" {
				info.Title = acf.Name
			}
			logger.Debug("ACF metadata imported",
				"gameId", info.ID,
				"name", acf.Name,
				"lastUpdated", acf.LastUpdated,
				"sizeOnDisk", acf.SizeOnDisk,
			)
		}
	}
	isNew := true

	logger.ScanGameDiscovered(gameDir, info.ID, info.Title, info.PrimaryPlatform(), len(executables))

	if saveErr := info.Save(); saveErr != nil {
		logger.ScanGameSaveFailed(gameDir, saveErr)
		return ScanResult{
			GameDir:  gameDir,
			GameInfo: info,
			IsNew:    isNew,
			Error:    "failed to save: " + saveErr.Error(),
		}
	}

	return ScanResult{GameDir: gameDir, GameInfo: info, IsNew: isNew}
}

func (s *Scanner) readSteamAppID(gameDir string) string {
	for i := 0; i < 3; i++ {
		path := filepath.Join(gameDir, "steam_appid.txt")
		data, err := os.ReadFile(path)
		if err == nil {
			content := strings.TrimSpace(string(data))
			content = strings.TrimPrefix(content, "\uFEFF")
			content = strings.TrimSpace(content)
			if content != "" {
				return content
			}
		}
		parent := filepath.Dir(gameDir)
		if parent == gameDir {
			break
		}
		gameDir = parent
	}
	return ""
}

type acfInfo struct {
	AppID       string
	Name        string
	InstallDir  string
	LastUpdated string
	SizeOnDisk  string
}

func (s *Scanner) readACF(gameDir string) *acfInfo {
	dirName := filepath.Base(gameDir)
	parent := filepath.Dir(gameDir)

	if strings.EqualFold(filepath.Base(parent), "common") {
		steamappsDir := filepath.Dir(parent)
		entries, err := os.ReadDir(steamappsDir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "appmanifest_") || !strings.HasSuffix(e.Name(), ".acf") {
				continue
			}
			path := filepath.Join(steamappsDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			info := parseACF(string(data))
			if info.AppID != "" && strings.EqualFold(info.InstallDir, dirName) {
				return info
			}
		}
	}
	return nil
}

func parseACF(content string) *acfInfo {
	info := &acfInfo{}
	inAppState := false
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == `"AppState"` {
			inAppState = true
			continue
		}
		if !inAppState {
			continue
		}
		for _, field := range []string{"appid", "name", "installdir", "LastUpdated", "SizeOnDisk"} {
			prefix := `"` + field + `"`
			if strings.HasPrefix(line, prefix) {
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) < 2 {
					parts = strings.SplitN(line, " ", 2)
				}
				val := strings.TrimSpace(parts[len(parts)-1])
				val = strings.Trim(val, `"`)
				switch field {
				case "appid":
					info.AppID = val
				case "name":
					info.Name = val
				case "installdir":
					info.InstallDir = val
				case "LastUpdated":
					info.LastUpdated = val
				case "SizeOnDisk":
					info.SizeOnDisk = val
				}
			}
		}
	}
	return info
}

func (s *Scanner) readACFAppID(gameDir string) string {
	info := s.readACF(gameDir)
	if info != nil {
		return info.AppID
	}
	return ""
}

func (s *Scanner) findExecutables(entries []os.DirEntry) []game.Executable {
	var executables []game.Executable
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".exe") {
			if strings.HasPrefix(lower, "unins") {
				logger.ScanExeFiltered(name, "uninstaller")
				continue
			}
			if strings.HasPrefix(lower, "unitycrashhandler") {
				logger.ScanExeFiltered(name, "unity crash handler")
				continue
			}
			baseName := strings.TrimSuffix(lower, ".exe")
			if isUtilityExe(baseName) {
				logger.ScanExeFiltered(name, "utility")
				continue
			}
			logger.ScanExeFound(name)
			executables = append(executables, game.Executable{
				Path:    name,
				Name:    strings.TrimSuffix(name, ".exe"),
				Primary: false,
			})
		}
	}

	if len(executables) > 0 {
		primary := s.pickPrimaryExec(executables)
		for i := range executables {
			if executables[i].Path == primary.Path {
				executables[i].Primary = true
				break
			}
		}
	}

	return executables
}

func (s *Scanner) pickPrimaryExec(executables []game.Executable) game.Executable {
	primaryKeywords := []string{"game", "launcher", "start", "main", "app"}

	for _, kw := range primaryKeywords {
		for _, exe := range executables {
			lower := strings.ToLower(exe.Name)
			if strings.Contains(lower, kw) {
				logger.ScanExePrimaryPicked(exe.Name, kw)
				return exe
			}
		}
	}

	if len(executables) > 0 {
		shortest := executables[0]
		for _, exe := range executables[1:] {
			if len(exe.Path) < len(shortest.Path) {
				shortest = exe
			}
		}
		logger.ScanExeShortestPicked(shortest.Name)
		return shortest
	}

	return executables[0]
}

func resolveDir(exeDir, dir string) string {
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Join(exeDir, dir)
}

func isUtilityExe(name string) bool {
	utilityPatterns := []string{
		"crashreport", "bugreport", "errorreport",
		"patcher", "patch", "update", "updater",
		"uninstall", "unins000", "unins001",
		"dxsetup", "vcredist", "redist",
		"dotnet", "directx", "xna",
	}
	for _, p := range utilityPatterns {
		if name == p || strings.Contains(name, p) {
			return true
		}
	}
	return false
}
