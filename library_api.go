package main

import (
	"fmt"
	"runtime"
	"strings"

	"GameLibrary/internal/config"
	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
	"GameLibrary/internal/logger"
	"GameLibrary/internal/platform"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppInfo describes the running instance to the UI.
//
// This replaced a map[string]string, which meant every field access in the
// frontend was an unchecked string lookup. It also carried a "buildTime" that was
// actually time.Now(), so it misreported when the binary was built.
type AppInfo struct {
	ExeDir       string `json:"exeDir"`
	MachineID    string `json:"machineId"`
	MachineName  string `json:"machineName"`
	Version      string `json:"version"`
	BuildTime    string `json:"buildTime"`
	LogDir       string `json:"logDir"`
	Platform     string `json:"platform"`
	CoverBaseURL string `json:"coverBaseUrl"`
}

// GetAppInfo returns information about the running application.
func (a *App) GetAppInfo() AppInfo {
	state := a.snapshot()
	info := AppInfo{
		ExeDir:       a.exeDir,
		MachineName:  a.host,
		Version:      version,
		BuildTime:    buildTime,
		LogDir:       logger.Dir(),
		Platform:     runtime.GOOS,
		CoverBaseURL: "/covers",
	}
	if state != nil {
		info.MachineID = state.cfg.MachineID
	}
	return info
}

// GetMachineName returns this client's host name.
func (a *App) GetMachineName() string { return a.host }

// GetConfig returns the shared configuration.
func (a *App) GetConfig() *config.Config {
	state := a.snapshot()
	if state == nil {
		return config.Default()
	}
	return state.cfg
}

// GetGameList returns every cached game, most recently played first.
func (a *App) GetGameList() []*game.GameInfo {
	return a.library.List()
}

// GetGame returns a single game, or nil when the ID is unknown.
func (a *App) GetGame(id string) *game.GameInfo {
	info, ok := a.library.Get(id)
	if !ok {
		return nil
	}
	return info
}

// --- user-editable fields ---------------------------------------------------

// ToggleGameStar flips a game's star rating.
func (a *App) ToggleGameStar(id string) error {
	_, err := a.library.Mutate(id, func(draft *game.GameInfo) error {
		draft.Starred = !draft.Starred
		return nil
	})
	return err
}

// AddGameTag adds a user tag.
func (a *App) AddGameTag(id string, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	_, err := a.library.Mutate(id, func(draft *game.GameInfo) error {
		draft.AddTag(tag)
		return nil
	})
	return err
}

// RemoveGameTag removes a user tag.
func (a *App) RemoveGameTag(id string, tag string) error {
	_, err := a.library.Mutate(id, func(draft *game.GameInfo) error {
		draft.RemoveTag(tag)
		return nil
	})
	return err
}

// SetPreferredSource records which metadata source is authoritative for a game.
func (a *App) SetPreferredSource(id string, source string) error {
	source = strings.TrimSpace(source)
	_, err := a.library.Mutate(id, func(draft *game.GameInfo) error {
		if source != "" && !draft.HasPlatform(source) {
			return fmt.Errorf("game is not linked to %s", source)
		}
		draft.PreferredSource = source
		return nil
	})
	return err
}

// --- settings helpers -------------------------------------------------------

// PickGameDirectory opens the folder browser and returns the chosen directory in
// the portable, root-relative form.
//
// The previous implementation unconditionally prepended ".\", which produced an
// invalid path whenever the chosen directory was not inside the library root.
func (a *App) PickGameDirectory() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("application is still starting up")
	}
	chosen, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Select Game Directory",
		DefaultDirectory:     a.exeDir,
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", err
	}
	if chosen == "" {
		return "", nil
	}
	return config.NormalizeDir(a.exeDir, chosen), nil
}

// SteamUserInfo is a detected Steam account.
type SteamUserInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// GetSteamUsers lists the Steam accounts installed on this client, which is how
// the cover cache and the Steam scraper pick a user context.
func (a *App) GetSteamUsers() []SteamUserInfo {
	users := platform.SteamUsers()
	out := make([]SteamUserInfo, 0, len(users))
	for _, user := range users {
		out = append(out, SteamUserInfo{ID: user.ID, Name: user.Name})
	}
	return out
}

// GetSteamPath returns the detected Steam installation directory, or "".
func (a *App) GetSteamPath() string { return platform.SteamPath() }

// SetSteamUser stores the selected Steam account.
func (a *App) SetSteamUser(id string) error {
	return a.setConfig(func(cfg *config.Config) error {
		cfg.SteamUserID = strings.TrimSpace(id)
		return nil
	})
}

// CommonPaths returns resolved absolute paths for the configured directories, so
// the UI can display and open them without re-implementing path normalisation in
// JavaScript.
func (a *App) CommonPaths() map[string]string {
	state := a.snapshot()
	paths := map[string]string{"exeDir": a.exeDir}
	if state == nil {
		return paths
	}
	for _, dir := range state.cfg.GameDirectories {
		paths[dir] = fsutil.Resolve(a.exeDir, dir)
	}
	return paths
}

// GetGamePathLabels maps every game ID to the labels of the configured directory
// it lives in.
//
// The UI used to derive this itself, which meant re-implementing path
// normalisation and prefix matching in JavaScript and keeping it in step with the
// Go side. The backend already knows both the absolute game directory and the
// configured directories, so it resolves the mapping once.
func (a *App) GetGamePathLabels() map[string][]string {
	state := a.snapshot()
	labels := map[string][]string{}
	if state == nil {
		return labels
	}

	type candidate struct {
		abs    string
		labels []string
	}
	dirs := make([]candidate, 0, len(state.cfg.GameDirectories))
	for _, configured := range state.cfg.GameDirectories {
		dirLabels := state.cfg.GameDirectoryLabels[configured]
		if len(dirLabels) == 0 {
			continue
		}
		dirs = append(dirs, candidate{
			abs:    fsutil.Resolve(a.exeDir, configured),
			labels: dirLabels,
		})
	}
	if len(dirs) == 0 {
		return labels
	}

	for _, info := range a.library.List() {
		// The most specific (longest) match wins when directories are nested.
		bestLen := -1
		var best []string
		for _, dir := range dirs {
			if !fsutil.IsWithin(dir.abs, info.GameDir) {
				continue
			}
			if len(dir.abs) > bestLen {
				bestLen = len(dir.abs)
				best = dir.labels
			}
		}
		if best != nil {
			labels[info.ID] = append([]string(nil), best...)
		}
	}
	return labels
}
