package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
	"GameLibrary/internal/scanner"
	"GameLibrary/internal/scraper"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var version = "0.2.3-alpha"

type Config = config.Config
type GameInfo = game.GameInfo
type ScanResult = scanner.ScanResult
type Executable = game.Executable
type SavePath = game.SavePath
type Metadata = game.Metadata

type ScrapeReport struct {
	GameID    string `json:"gameId"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	Error     string `json:"error,omitempty"`
}

type App struct {
	ctx      context.Context
	exeDir   string
	host     string
	config   *config.Config
	scanner  *scanner.Scanner
	pipeline *scraper.Pipeline
	games    map[string]*game.GameInfo
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

	a.pipeline = scraper.NewPipeline(a.config)

	steamScraper := scraper.NewSteamScraper()
	steamScraper.Configure(a.config.Language, a.config.SourceSettings("steam"))
	a.pipeline.Register(steamScraper)

	vndbScraper := scraper.NewVNDBScraper()
	vndbScraper.Configure(a.config.Language, a.config.SourceSettings("vndb"))
	a.pipeline.Register(vndbScraper)

	bangumiScraper := scraper.NewBangumiScraper()
	bangumiScraper.Configure(a.config.Language, a.config.SourceSettings("bangumi"))
	a.pipeline.Register(bangumiScraper)

	a.pipeline.Register(scraper.NewDLsiteScraper())

	steamgriddbScraper := scraper.NewSteamGridDBScraper()
	steamgriddbScraper.Configure(a.config.Language, a.config.SourceSettings("steamgriddb"))
	a.pipeline.Register(steamgriddbScraper)

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

func readCoverAsDataURI(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	ext := filepath.Ext(path)
	mime := "image/jpeg"
	if ext == ".png" {
		mime = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
}

func (a *App) GetGameCover(id string) string {
	info, ok := a.games[id]
	if !ok {
		return ""
	}
	return readCoverAsDataURI(scraper.CoverPath(info.GameDir))
}

func (a *App) GetGameCoverLandscape(id string) string {
	info, ok := a.games[id]
	if !ok {
		return ""
	}
	return readCoverAsDataURI(scraper.CoverLandscapePath(info.GameDir))
}

func (a *App) ScrapeGame(id string) *ScrapeReport {
	info, ok := a.games[id]
	if !ok {
		return &ScrapeReport{GameID: id, Error: "game not found"}
	}

	result, source, err := a.pipeline.Scrape(info.GameDir, info)
	if err != nil {
		return &ScrapeReport{GameID: id, Title: info.Title, Error: err.Error()}
	}
	if result == nil {
		return &ScrapeReport{GameID: id, Title: info.Title, Source: "none", Error: "no source matched"}
	}

	scraper.ApplyResult(info, result, source)

	scraper.DownloadCover(info.GameDir, result.CoverURL, "cover")
	scraper.DownloadCover(info.GameDir, result.CoverLandscapeURL, "cover_landscape")

	if err := info.Save(); err != nil {
		return &ScrapeReport{GameID: id, Title: info.Title, Source: source, Error: "save failed: " + err.Error()}
	}

	return &ScrapeReport{GameID: id, Title: info.Title, Source: source}
}

func (a *App) ScrapeAllGames() []ScrapeReport {
	var reports []ScrapeReport
	for _, info := range a.games {
		report := a.ScrapeGame(info.ID)
		reports = append(reports, *report)
	}
	return reports
}

func (a *App) LaunchGame(id string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}

	primary := findPrimaryExec(info.Executables)
	if primary == nil {
		return fmt.Errorf("no executable found for %s", info.Title)
	}

	exePath := filepath.Join(info.GameDir, primary.Path)
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return fmt.Errorf("executable not found: %s", exePath)
	}

	cmd := exec.Command(exePath)
	cmd.Dir = info.GameDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch: %w", err)
	}

	go func() {
		cmd.Wait()
	}()

	info.LastPlayedAt = time.Now().UTC().Format(time.RFC3339)
	info.Save()

	return nil
}

func findPrimaryExec(executables []game.Executable) *game.Executable {
	for i := range executables {
		if executables[i].Primary {
			return &executables[i]
		}
	}
	if len(executables) > 0 {
		return &executables[0]
	}
	return nil
}

func (a *App) GetAppInfo() map[string]string {
	return map[string]string{
		"exeDir":      a.exeDir,
		"machineId":   a.config.MachineID,
		"machineName": a.host,
		"version":     version,
		"buildTime":   time.Now().Format(time.RFC3339),
	}
}
