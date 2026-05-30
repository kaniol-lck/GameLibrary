package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

type PlatformInfo struct {
	Platform string `json:"platform"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
}

type Metadata struct {
	CoverURL        string            `json:"coverUrl,omitempty"`
	CoverLandscape  string            `json:"coverLandscape,omitempty"`
	ReleaseDate     string            `json:"releaseDate,omitempty"`
	Developer       string            `json:"developer,omitempty"`
	Publisher       string            `json:"publisher,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Description     string            `json:"description,omitempty"`
	Links           map[string]string `json:"links,omitempty"`
}

type GameInfo struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	TitleNative     string         `json:"titleNative,omitempty"`
	Platforms       []PlatformInfo `json:"platforms,omitempty"`
	Aliases         []string       `json:"aliases,omitempty"`
	PreferredSource string         `json:"preferredSource,omitempty"`
	Type            string         `json:"type"`
	Executables     []Executable   `json:"executables"`
	SavePaths       []SavePath     `json:"savePaths,omitempty"`
	Metadata     *Metadata      `json:"metadata,omitempty"`
	ScannedAt    string         `json:"scannedAt"`

	TotalPlaytime int64    `json:"totalPlaytime"`
	LastPlayedAt  string   `json:"lastPlayedAt,omitempty"`
	Starred       bool     `json:"starred,omitempty"`
	Tags          []string `json:"tags,omitempty"`

	GameDir     string `json:"-"`
	InfoRelPath string `json:"-"`
}

func (g *GameInfo) InfoFilePath() string {
	return filepath.Join(g.GameDir, g.InfoRelPath)
}

func (g *GameInfo) Save() error {
	path := g.InfoFilePath()
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (g *GameInfo) PrimaryPlatform() string {
	if g.PreferredSource != "" {
		return g.PreferredSource
	}
	for _, p := range g.Platforms {
		if p.Platform != "" {
			return p.Platform
		}
	}
	return ""
}

func (g *GameInfo) PrimaryPlatformID() string {
	if len(g.Platforms) == 0 {
		return ""
	}
	return g.Platforms[0].ID
}

func (g *GameInfo) HasPlatform(platform string) bool {
	for _, p := range g.Platforms {
		if p.Platform == platform {
			return true
		}
	}
	return false
}

func (g *GameInfo) PlatformIDs() []string {
	ids := make([]string, 0, len(g.Platforms))
	for _, p := range g.Platforms {
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func (g *GameInfo) SetPlatform(platform, id, name string) {
	for i, p := range g.Platforms {
		if p.Platform == platform {
			if id != "" {
				g.Platforms[i].ID = id
			}
			if name != "" {
				g.Platforms[i].Name = name
			}
			return
		}
	}
	info := PlatformInfo{Platform: platform, ID: id, Name: name}
	g.Platforms = append(g.Platforms, info)
	if g.PreferredSource == "" && platform != "" {
		g.PreferredSource = platform
	}
}

func (g *GameInfo) AddAlias(name string) {
	if name == "" || name == g.Title {
		return
	}
	for _, a := range g.Aliases {
		if strings.EqualFold(a, name) {
			return
		}
	}
	g.Aliases = append(g.Aliases, name)
}

func LoadFromDir(gameDir string) (*GameInfo, error) {
	path := filepath.Join(gameDir, ".gameinfo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	migratePlatform(raw)

	remediated, _ := json.Marshal(raw)
	var info GameInfo
	if err := json.Unmarshal(remediated, &info); err != nil {
		return nil, err
	}

	info.ID = stripBOM(info.ID)
	for i := range info.Platforms {
		info.Platforms[i].ID = stripBOM(info.Platforms[i].ID)
	}

	info.GameDir = gameDir
	info.InfoRelPath = ".gameinfo.json"
	return &info, nil
}

func migratePlatform(raw map[string]json.RawMessage) {
	if _, hasPlatforms := raw["platforms"]; hasPlatforms {
		return
	}

	plat := ""
	pid := ""
	if v, ok := raw["platform"]; ok {
		json.Unmarshal(v, &plat)
		delete(raw, "platform")
	}
	if v, ok := raw["platformId"]; ok {
		json.Unmarshal(v, &pid)
		delete(raw, "platformId")
	}

	if plat != "" || pid != "" {
		platforms := []PlatformInfo{{Platform: plat, ID: pid}}
		data, _ := json.Marshal(platforms)
		raw["platforms"] = data
	}
}

func stripBOM(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	s = strings.TrimPrefix(s, "\uFFFE")
	return strings.TrimSpace(s)
}

func New(gameDir string, executables []Executable, steamAppID string) *GameInfo {
	now := time.Now().UTC().Format(time.RFC3339)

	id := filepath.Base(gameDir)
	var platforms []PlatformInfo

	if steamAppID != "" {
		id = "steam_" + steamAppID
		platforms = []PlatformInfo{{Platform: "steam", ID: steamAppID}}
	}

	return &GameInfo{
		ID:          id,
		Title:       filepath.Base(gameDir),
		Platforms:   platforms,
		Type:        "game",
		Executables: executables,
		ScannedAt:   now,
		GameDir:     gameDir,
		InfoRelPath: ".gameinfo.json",
	}
}
