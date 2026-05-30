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
			existing.GameDir = gameDir
			existing.InfoRelPath = ".gameinfo.json"
			logger.ScanGameAlreadyExists(gameDir, existing.ID)
			return ScanResult{GameDir: gameDir, GameInfo: existing, IsNew: false}
		}
	}

	entries, _ := os.ReadDir(gameDir)

	steamAppID := s.readSteamAppID(gameDir)
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
