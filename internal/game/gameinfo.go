package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Executable struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
}

type SavePath struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Source string `json:"source,omitempty"`
}

type Metadata struct {
	CoverURL    string            `json:"coverUrl,omitempty"`
	ReleaseDate string            `json:"releaseDate,omitempty"`
	Developer   string            `json:"developer,omitempty"`
	Publisher   string            `json:"publisher,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Description string            `json:"description,omitempty"`
	Links       map[string]string `json:"links,omitempty"`
}

type GameInfo struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	TitleNative  string       `json:"titleNative,omitempty"`
	Platform     string       `json:"platform"`
	PlatformID   string       `json:"platformId,omitempty"`
	Type         string       `json:"type"`
	Executables  []Executable `json:"executables"`
	SavePaths    []SavePath   `json:"savePaths,omitempty"`
	Metadata     *Metadata    `json:"metadata,omitempty"`
	ScannedAt    string       `json:"scannedAt"`

	TotalPlaytime int64  `json:"totalPlaytime"`
	LastPlayedAt  string `json:"lastPlayedAt,omitempty"`

	GameDir     string `json:"-"`
	InfoRelPath string `json:"-"`
}

func (g *GameInfo) InfoFilePath() string {
	return filepath.Join(g.GameDir, g.InfoRelPath)
}

func (g *GameInfo) CoverFilePath() string {
	return filepath.Join(g.GameDir, "cover.jpg")
}

func (g *GameInfo) Save() error {
	path := g.InfoFilePath()
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadFromDir(gameDir string) (*GameInfo, error) {
	path := filepath.Join(gameDir, ".gameinfo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var info GameInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	info.GameDir = gameDir
	info.InfoRelPath = ".gameinfo.json"
	return &info, nil
}

func New(gameDir string, executables []Executable, steamAppID string) *GameInfo {
	now := time.Now().UTC().Format(time.RFC3339)

	id := filepath.Base(gameDir)
	platform := "local"
	platformID := ""

	if steamAppID != "" {
		id = "steam_" + steamAppID
		platform = "steam"
		platformID = steamAppID
	}

	return &GameInfo{
		ID:          id,
		Title:       filepath.Base(gameDir),
		Platform:    platform,
		PlatformID:  platformID,
		Type:        "game",
		Executables: executables,
		ScannedAt:   now,
		GameDir:     gameDir,
		InfoRelPath: ".gameinfo.json",
	}
}
