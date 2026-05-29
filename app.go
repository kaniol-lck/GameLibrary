package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
	"GameLibrary/internal/scanner"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Config = config.Config
type GameInfo = game.GameInfo
type ScanResult = scanner.ScanResult
type Executable = game.Executable
type SavePath = game.SavePath
type Metadata = game.Metadata

type App struct {
	ctx     context.Context
	exeDir  string
	host    string
	config  *config.Config
	scanner *scanner.Scanner
	games   map[string]*game.GameInfo
}

func NewApp() *App {
	return &App{
		games: make(map[string]*game.GameInfo),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = os.Getwd()
	}
	a.exeDir = filepath.Dir(exePath)

	a.host, _ = os.Hostname()

	cfg, err := config.Load(a.exeDir)
	if err != nil {
		cfg = config.Default()
	}
	a.config = cfg
	a.scanner = scanner.New(a.exeDir, a.config)

	a.refreshGameCache()
}

func (a *App) refreshGameCache() {
	a.games = make(map[string]*game.GameInfo)

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
		if info, err := game.LoadFromDir(dir); err == nil {
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

func (a *App) PickGameDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Game Directory",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil
	}

	relPath, err := filepath.Rel(a.exeDir, dir)
	if err != nil {
		return dir, nil
	}
	return ".\\" + relPath, nil
}

func (a *App) GetMachineName() string {
	return a.host
}

func (a *App) GetConfig() *config.Config {
	return a.config
}

func (a *App) SaveConfig(cfg *config.Config) error {
	a.config = cfg
	return cfg.Save(a.exeDir)
}

func (a *App) ScanGames() []scanner.ScanResult {
	results, err := a.scanner.ScanAll()
	if err != nil {
		return []scanner.ScanResult{{
			Error: "scan failed: " + err.Error(),
		}}
	}

	a.refreshGameCache()

	sort.Slice(results, func(i, j int) bool {
		return results[i].GameDir < results[j].GameDir
	})

	return results
}

func (a *App) GetGameList() []*game.GameInfo {
	list := make([]*game.GameInfo, 0, len(a.games))
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

func (a *App) GetGame(id string) *game.GameInfo {
	info, ok := a.games[id]
	if !ok {
		return nil
	}
	infoCopy := *info
	return &infoCopy
}

func (a *App) UpdateGameInfo(info *game.GameInfo) error {
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
		"machineName": a.host,
		"version":     "0.1.0",
		"buildTime":   time.Now().Format(time.RFC3339),
	}
}
