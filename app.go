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
	"GameLibrary/internal/logger"
	"GameLibrary/internal/scanner"
	"GameLibrary/internal/scraper"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var version = "0.5.2-alpha"

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

	logger.Init(a.exeDir)

	a.host, _ = os.Hostname()

	logger.AppStarted(version, a.exeDir, a.host)

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

	rawgScraper := scraper.NewRawgScraper()
	rawgScraper.Configure(a.config.Language, a.config.SourceSettings("rawg"))
	a.pipeline.Register(rawgScraper)

	a.refreshGameCache()

	logger.Info("startup complete", "gameCount", len(a.games))
}

func (a *App) refreshGameCache() {
	a.games = make(map[string]*game.GameInfo)

	for _, relDir := range a.config.GameDirectories {
		absDir := filepath.Clean(relDir)
		if !filepath.IsAbs(absDir) {
			absDir = filepath.Join(a.exeDir, relDir)
		}
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
	return a.doScan(false)
}

func (a *App) ForceScanGames() []scanner.ScanResult {
	return a.doScan(true)
}

func (a *App) doScan(force bool) []scanner.ScanResult {
	var results []scanner.ScanResult
	var err error
	if force {
		results, err = a.scanner.ForceScanAll()
	} else {
		results, err = a.scanner.ScanAll()
	}
	if err != nil {
		logger.Error("scan failed", "error", err.Error())
		return []scanner.ScanResult{{
			Error: "scan failed: " + err.Error(),
		}}
	}

	a.refreshGameCache()

	sort.Slice(results, func(i, j int) bool {
		return results[i].GameDir < results[j].GameDir
	})

	go a.autoScrapeNew(results)

	return results
}

func (a *App) autoScrapeNew(results []scanner.ScanResult) {
	for _, r := range results {
		if r.IsNew && r.GameInfo != nil && r.Error == "" {
			logger.Info("auto-scraping new game", "gameId", r.GameInfo.ID, "title", r.GameInfo.Title)
			a.ScrapeGame(r.GameInfo.ID)
		}
	}
	logger.Info("auto-scrape finished for new games")
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
		logger.GameInfoSaved(info.ID, info.Title, err)
		return err
	}

	logger.GameInfoUpdated(info.ID, info.Title)

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
		logger.ScrapeGameNotFound(id)
		return &ScrapeReport{GameID: id, Error: "game not found"}
	}

	sourceResults := a.pipeline.ScrapeAll(info.GameDir, info)
	if len(sourceResults) == 0 {
		return &ScrapeReport{GameID: id, Title: info.Title, Source: "none", Error: "no source matched"}
	}

	prefIdx := 0
	for i, sr := range sourceResults {
		if sr.Source == info.PreferredSource {
			prefIdx = i
			break
		}
	}

	primary := sourceResults[prefIdx]
	scraper.ApplyResult(info, primary.Result, primary.Source)

	for i, sr := range sourceResults {
		if i == prefIdx {
			continue
		}
		pid := ""
		if sr.Result.Links != nil {
			if id2, ok := sr.Result.Links["platformId"]; ok {
				pid = id2
			}
		}
		info.SetPlatform(sr.Source, pid, sr.Result.Title)
		info.AddAlias(sr.Result.Title)
		info.AddAlias(sr.Result.TitleNative)
	}

	scraper.DownloadCoverWithLog(info.GameDir, info.ID, primary.Result.CoverURL, "cover")
	scraper.DownloadCoverWithLog(info.GameDir, info.ID, primary.Result.CoverLandscapeURL, "cover_landscape")

	if err := info.Save(); err != nil {
		logger.GameInfoSaved(id, info.Title, err)
		return &ScrapeReport{GameID: id, Title: info.Title, Source: primary.Source, Error: "save failed: " + err.Error()}
	}

	logger.GameInfoSaved(id, info.Title, nil)

	return &ScrapeReport{GameID: id, Title: info.Title, Source: primary.Source}
}

func (a *App) ScrapeAllGames() []ScrapeReport {
	logger.Info("batch scrape started", "gameCount", len(a.games))

	var reports []ScrapeReport
	for _, info := range a.games {
		report := a.ScrapeGame(info.ID)
		reports = append(reports, *report)
	}

	successCount := 0
	for _, r := range reports {
		if r.Error == "" {
			successCount++
		}
	}
	logger.Info("batch scrape finished", "total", len(reports), "success", successCount)

	return reports
}

func (a *App) LaunchGame(id string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}

	primary := findPrimaryExec(info.Executables)
	if primary == nil {
		logger.GameLaunchFailed(id, info.Title, fmt.Errorf("no executable found"))
		return fmt.Errorf("no executable found for %s", info.Title)
	}

	exePath := filepath.Join(info.GameDir, primary.Path)
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		logger.GameLaunchFailed(id, info.Title, fmt.Errorf("executable not found: %s", exePath))
		return fmt.Errorf("executable not found: %s", exePath)
	}

	cmd := exec.Command(exePath)
	cmd.Dir = info.GameDir
	if err := cmd.Start(); err != nil {
		logger.GameLaunchFailed(id, info.Title, err)
		return fmt.Errorf("failed to launch: %w", err)
	}

	logger.GameLaunched(id, info.Title, primary.Path)

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

func (a *App) ToggleGameStar(id string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}
	info.Starred = !info.Starred
	return info.Save()
}

func (a *App) SetPreferredSource(id string, source string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}
	info.PreferredSource = source
	return info.Save()
}

func (a *App) AddGameTag(id string, tag string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	for _, t := range info.Tags {
		if strings.EqualFold(t, tag) {
			return nil
		}
	}
	info.Tags = append(info.Tags, tag)
	return info.Save()
}

func (a *App) RemoveGameTag(id string, tag string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}
	idx := -1
	for i, t := range info.Tags {
		if strings.EqualFold(t, tag) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	info.Tags = append(info.Tags[:idx], info.Tags[idx+1:]...)
	return info.Save()
}

func (a *App) OpenGameDirectory(id string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}
	return exec.Command("explorer", info.GameDir).Start()
}

func (a *App) OpenGameMetadata(id string) error {
	info, ok := a.games[id]
	if !ok {
		return fmt.Errorf("game not found: %s", id)
	}
	return exec.Command("notepad", info.InfoFilePath()).Start()
}

func (a *App) OpenBrowser(url string) error {
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}
