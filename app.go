package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type App struct {
	ctx     context.Context
	exeDir  string
	config  *Config
	scanner *Scanner
	games   map[string]*GameInfo
}

func NewApp() *App {
	return &App{
		games: make(map[string]*GameInfo),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = os.Getwd()
	}
	a.exeDir = filepath.Dir(exePath)

	cfg, err := LoadConfig(a.exeDir)
	if err != nil {
		cfg = DefaultConfig()
	}
	a.config = cfg
	a.scanner = NewScanner(a.exeDir, a.config)

	a.refreshGameCache()
}

func (a *App) refreshGameCache() {
	a.games = make(map[string]*GameInfo)

	for _, relDir := range a.config.GameDirectories {
		absDir := filepath.Join(a.exeDir, relDir)
		absDir = filepath.Clean(absDir)
		a.loadGamesFromDir(absDir, 0)
	}
}

func (a *App) loadGamesFromDir(dir string, depth int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	if isGameDir(entries) {
		if info, err := LoadGameInfo(dir); err == nil {
			a.games[info.ID] = info
		}
		return
	}

	if depth >= a.config.MaxScanDepth {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		a.loadGamesFromDir(filepath.Join(dir, entry.Name()), depth+1)
	}
}

func isGameDir(entries []os.DirEntry) bool {
	for _, e := range entries {
		if !e.IsDir() {
			name := e.Name()
			if name == ".gameinfo.json" {
				return true
			}
			if len(name) > 4 {
				ext := strings.ToLower(name[len(name)-4:])
				if ext == ".exe" {
					return true
				}
			}
		}
	}
	return false
}

func (a *App) GetConfig() *Config {
	return a.config
}

func (a *App) SaveConfig(cfg *Config) error {
	a.config = cfg
	return cfg.Save(a.exeDir)
}

func (a *App) ScanGames() []ScanResult {
	results, err := a.scanner.ScanAll()
	if err != nil {
		return []ScanResult{{
			Error: "scan failed: " + err.Error(),
		}}
	}

	a.refreshGameCache()

	sort.Slice(results, func(i, j int) bool {
		return results[i].GameDir < results[j].GameDir
	})

	return results
}

func (a *App) GetGameList() []*GameInfo {
	list := make([]*GameInfo, 0, len(a.games))
	for _, info := range a.games {
		infoCopy := *info
		list = append(list, &infoCopy)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].LastPlayedAt != "" && list[j].LastPlayedAt != "" {
			return list[i].LastPlayedAt > list[j].LastPlayedAt
		}
		if list[i].LastPlayedAt != "" {
			return true
		}
		if list[j].LastPlayedAt != "" {
			return false
		}
		return list[i].Title < list[j].Title
	})

	return list
}

func (a *App) GetGame(id string) *GameInfo {
	info, ok := a.games[id]
	if !ok {
		return nil
	}
	infoCopy := *info
	return &infoCopy
}

func (a *App) UpdateGameInfo(info *GameInfo) error {
	existing, ok := a.games[info.ID]
	if !ok {
		existing = info
	} else {
		existing.Title = info.Title
		existing.TitleNative = info.TitleNative
		existing.Type = info.Type
		existing.Executables = info.Executables
		existing.SavePaths = info.SavePaths
		existing.Metadata = info.Metadata
		existing.LastPlayedAt = info.LastPlayedAt
		existing.TotalPlaytime = info.TotalPlaytime
	}

	if err := existing.Save(); err != nil {
		return err
	}

	a.games[info.ID] = existing
	return nil
}

func (a *App) GetAppInfo() map[string]string {
	return map[string]string{
		"exeDir":      a.exeDir,
		"machineId":   a.config.MachineID,
		"machineName": a.config.MachineName,
		"version":     "0.1.0",
		"buildTime":   time.Now().Format(time.RFC3339),
	}
}
