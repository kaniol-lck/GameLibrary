package main

import (
	"os"
	"path/filepath"
	"strings"
)

type ScanResult struct {
	GameDir  string    `json:"gameDir"`
	GameInfo *GameInfo `json:"gameInfo"`
	IsNew    bool      `json:"isNew"`
	Error    string    `json:"error,omitempty"`
}

type Scanner struct {
	exeDir string
	config *Config
}

func NewScanner(exeDir string, config *Config) *Scanner {
	return &Scanner{
		exeDir: exeDir,
		config: config,
	}
}

func (s *Scanner) ScanAll() ([]ScanResult, error) {
	var results []ScanResult
	for _, relDir := range s.config.GameDirectories {
		absDir := filepath.Join(s.exeDir, relDir)
		absDir = filepath.Clean(absDir)

		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			continue
		}

		dirResults := s.scanDir(absDir, 0)
		results = append(results, dirResults...)
	}
	return results, nil
}

func (s *Scanner) scanDir(dir string, depth int) []ScanResult {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	if s.isGameDirectory(entries) {
		result := s.identifyGame(dir)
		return []ScanResult{result}
	}

	if depth >= s.config.MaxScanDepth {
		return nil
	}

	var results []ScanResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		subDir := filepath.Join(dir, name)
		subResults := s.scanDir(subDir, depth+1)
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
	existing, err := LoadGameInfo(gameDir)
	if err == nil && existing != nil {
		existing.gameDir = gameDir
		existing.infoRelPath = ".gameinfo.json"
		return ScanResult{GameDir: gameDir, GameInfo: existing, IsNew: false}
	}

	entries, _ := os.ReadDir(gameDir)

	steamAppID := s.readSteamAppID(gameDir)
	executables := s.findExecutables(entries, gameDir)

	if len(executables) == 0 {
		return ScanResult{
			GameDir: gameDir,
			Error:   "no executable found",
		}
	}

	info := newGameInfo(gameDir, executables, steamAppID)
	isNew := true

	if saveErr := info.Save(); saveErr != nil {
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
			return strings.TrimSpace(string(data))
		}
		parent := filepath.Dir(gameDir)
		if parent == gameDir {
			break
		}
		gameDir = parent
	}
	return ""
}

func (s *Scanner) findExecutables(entries []os.DirEntry, gameDir string) []Executable {
	var executables []Executable
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".exe") && !strings.HasPrefix(lower, "unins") {
			executables = append(executables, Executable{
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

func (s *Scanner) pickPrimaryExec(executables []Executable) Executable {
	primaryKeywords := []string{"game", "launcher", "start", "main", "app"}

	for _, kw := range primaryKeywords {
		for _, exe := range executables {
			lower := strings.ToLower(exe.Name)
			if strings.Contains(lower, kw) {
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
		return shortest
	}

	return executables[0]
}
