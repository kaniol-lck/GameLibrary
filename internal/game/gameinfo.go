// Package game holds the on-disk game metadata model and its persistence.
//
// Every game directory may contain a ".gamemanager" folder holding the managed
// data:
//
//	GameDir/
//	  .gamemanager/
//	    gameinfo.json          core metadata
//	    covers/                cover art served to the UI
//
// Metadata files live on a NAS share and are read and written by every client,
// so persistence is atomic (see internal/fsutil) and nothing machine specific
// is ever stored in them.
package game

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"GameLibrary/internal/fsutil"
)

const (
	gmDir     = ".gamemanager"
	infoRel   = ".gamemanager/gameinfo.json"
	coversRel = ".gamemanager/covers"
	metaRel   = ".gamemanager/meta"

	legacyRel = ".gameinfo.json"

	// SchemaVersion is the current on-disk format version. It is bumped when a
	// change cannot be expressed by adding fields alone.
	SchemaVersion = 2

	// CoverName and CoverLandscapeName are the file stems inside covers/.
	CoverName          = "cover"
	CoverLandscapeName = "cover_landscape"

	// ManagerDirName is the hidden folder holding managed game data.
	ManagerDirName = gmDir
)

// Executable is a launchable file inside the game directory.
type Executable struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
}

// SavePath describes where a game keeps its save data. Populated by later
// phases (cloud saves); retained so the format does not change again.
type SavePath struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Source string `json:"source,omitempty"`
}

// PlatformInfo links a game to an entry on a metadata source.
type PlatformInfo struct {
	Platform string `json:"platform"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
}

// Metadata is the scraped description of a game.
type Metadata struct {
	CoverURL       string            `json:"coverUrl,omitempty"`
	CoverLandscape string            `json:"coverLandscape,omitempty"`
	ReleaseDate    string            `json:"releaseDate,omitempty"`
	Developer      string            `json:"developer,omitempty"`
	Publisher      string            `json:"publisher,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Description    string            `json:"description,omitempty"`
	Links          map[string]string `json:"links,omitempty"`
}

// GameInfo is the complete managed record for one game.
type GameInfo struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	TitleNative   string `json:"titleNative,omitempty"`
	Type          string `json:"type"`

	Platforms       []PlatformInfo `json:"platforms,omitempty"`
	Aliases         []string       `json:"aliases,omitempty"`
	PreferredSource string         `json:"preferredSource,omitempty"`
	Executables     []Executable   `json:"executables"`
	SavePaths       []SavePath     `json:"savePaths,omitempty"`
	Metadata        *Metadata      `json:"metadata,omitempty"`

	ScannedAt     string `json:"scannedAt"`
	TotalPlaytime int64  `json:"totalPlaytime"`
	LastPlayedAt  string `json:"lastPlayedAt,omitempty"`

	Starred bool     `json:"starred,omitempty"`
	Tags    []string `json:"tags,omitempty"`

	// CoverVersion changes whenever cover art is (re)written. The UI appends it
	// to the cover URL so a freshly scraped cover is not masked by the webview
	// cache.
	CoverVersion int64 `json:"coverVersion,omitempty"`

	// GameDir is where this record was loaded from. It is machine specific (the
	// NAS mount point differs per client) and is therefore never persisted.
	GameDir string `json:"-"`
}

// ManagerDir returns the .gamemanager directory for a game.
func ManagerDir(gameDir string) string { return filepath.Join(gameDir, gmDir) }

// CoverDir returns the directory holding cover art for a game.
func CoverDir(gameDir string) string { return filepath.Join(gameDir, coversRel) }

// MetaDir returns the directory holding per-source metadata snapshots.
func MetaDir(gameDir string) string { return filepath.Join(gameDir, metaRel) }

// InfoFilePath returns the path of a game's gameinfo.json.
func InfoFilePath(gameDir string) string { return filepath.Join(gameDir, infoRel) }

// LegacyInfoPath returns the pre-0.6 metadata location.
func LegacyInfoPath(gameDir string) string { return filepath.Join(gameDir, legacyRel) }

// InfoFilePath returns the path of this game's gameinfo.json.
func (g *GameInfo) InfoFilePath() string { return InfoFilePath(g.GameDir) }

// Clone returns a deep copy. The library cache hands out clones so that no
// caller can mutate shared state behind the store's back.
func (g *GameInfo) Clone() *GameInfo {
	if g == nil {
		return nil
	}
	clone := *g
	clone.Platforms = append([]PlatformInfo(nil), g.Platforms...)
	clone.Aliases = append([]string(nil), g.Aliases...)
	clone.Executables = append([]Executable(nil), g.Executables...)
	clone.SavePaths = append([]SavePath(nil), g.SavePaths...)
	clone.Tags = append([]string(nil), g.Tags...)
	if g.Metadata != nil {
		meta := *g.Metadata
		meta.Tags = append([]string(nil), g.Metadata.Tags...)
		if g.Metadata.Links != nil {
			meta.Links = make(map[string]string, len(g.Metadata.Links))
			for k, v := range g.Metadata.Links {
				meta.Links[k] = v
			}
		}
		clone.Metadata = &meta
	}
	return &clone
}

// Save writes the record atomically.
func (g *GameInfo) Save() error {
	if g.GameDir == "" {
		return fmt.Errorf("save gameinfo: empty game directory for id %q", g.ID)
	}
	// Writing always produces the current format, whatever version was read, so a
	// migrated record never claims to be older than it is.
	g.SchemaVersion = SchemaVersion
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gameinfo for %q: %w", g.ID, err)
	}
	data = append(data, '\n')
	if err := fsutil.WriteFileAtomic(g.InfoFilePath(), data, 0o644); err != nil {
		return fmt.Errorf("write gameinfo for %q: %w", g.ID, err)
	}
	return nil
}

// SaveMeta writes a per-source metadata snapshot.
func (g *GameInfo) SaveMeta(source string) error {
	if g.Metadata == nil || source == "" {
		return nil
	}
	data, err := json.MarshalIndent(g.Metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s metadata for %q: %w", source, g.ID, err)
	}
	data = append(data, '\n')
	path := filepath.Join(MetaDir(g.GameDir), source+".json")
	if err := fsutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s metadata for %q: %w", source, g.ID, err)
	}
	return nil
}

// LoadMeta reads a per-source metadata snapshot, or nil when absent.
func (g *GameInfo) LoadMeta(source string) *Metadata {
	data, err := os.ReadFile(filepath.Join(MetaDir(g.GameDir), source+".json"))
	if err != nil {
		return nil
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

// MarkCoverUpdated records that cover art changed, so the UI fetches it again.
func (g *GameInfo) MarkCoverUpdated() {
	g.CoverVersion = time.Now().Unix()
}

// --- platform helpers -------------------------------------------------------

// PreferredPlatformInfo returns the platform entry the user (or the scraper)
// considers authoritative.
//
// PreferredSource wins when it is set and present in Platforms; otherwise the
// first entry carrying a platform key is used. PrimaryPlatform and
// PrimaryPlatformID both derive from this one function, so they can never
// disagree about which platform is primary.
func (g *GameInfo) PreferredPlatformInfo() (PlatformInfo, bool) {
	if g.PreferredSource != "" {
		for _, p := range g.Platforms {
			if p.Platform == g.PreferredSource {
				return p, true
			}
		}
	}
	for _, p := range g.Platforms {
		if p.Platform != "" {
			return p, true
		}
	}
	return PlatformInfo{}, false
}

// PrimaryPlatform returns the identifier of the primary platform, or "".
func (g *GameInfo) PrimaryPlatform() string {
	if p, ok := g.PreferredPlatformInfo(); ok {
		return p.Platform
	}
	return ""
}

// PrimaryPlatformID returns the source-specific ID of the primary platform.
func (g *GameInfo) PrimaryPlatformID() string {
	if p, ok := g.PreferredPlatformInfo(); ok {
		return p.ID
	}
	return ""
}

// PlatformID returns the stored ID for a specific platform, or "".
func (g *GameInfo) PlatformID(platform string) string {
	for _, p := range g.Platforms {
		if p.Platform == platform {
			return p.ID
		}
	}
	return ""
}

// HasPlatform reports whether the game is linked to a platform.
func (g *GameInfo) HasPlatform(platform string) bool {
	for _, p := range g.Platforms {
		if p.Platform == platform {
			return true
		}
	}
	return false
}

// PlatformIDs returns every non-empty platform ID.
func (g *GameInfo) PlatformIDs() []string {
	ids := make([]string, 0, len(g.Platforms))
	for _, p := range g.Platforms {
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// SetPlatform inserts or updates a platform link, promoting it to preferred
// when no preference has been recorded yet.
func (g *GameInfo) SetPlatform(platform, id, name string) {
	if platform == "" {
		return
	}
	for i := range g.Platforms {
		if g.Platforms[i].Platform == platform {
			if id != "" {
				g.Platforms[i].ID = id
			}
			if name != "" {
				g.Platforms[i].Name = name
			}
			return
		}
	}
	g.Platforms = append(g.Platforms, PlatformInfo{Platform: platform, ID: id, Name: name})
	if g.PreferredSource == "" {
		g.PreferredSource = platform
	}
}

// AddAlias records an alternative title, ignoring blanks, the primary title and
// case-insensitive duplicates.
func (g *GameInfo) AddAlias(name string) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, g.Title) {
		return
	}
	for _, a := range g.Aliases {
		if strings.EqualFold(a, name) {
			return
		}
	}
	g.Aliases = append(g.Aliases, name)
}

// HasTag reports whether a user tag is already present (case-insensitive).
func (g *GameInfo) HasTag(tag string) bool {
	for _, t := range g.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// AddTag appends a user tag, ignoring blanks and duplicates.
func (g *GameInfo) AddTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" || g.HasTag(tag) {
		return false
	}
	g.Tags = append(g.Tags, tag)
	return true
}

// RemoveTag removes a user tag (case-insensitive) and reports whether it went.
func (g *GameInfo) RemoveTag(tag string) bool {
	for i, t := range g.Tags {
		if strings.EqualFold(t, tag) {
			g.Tags = append(g.Tags[:i], g.Tags[i+1:]...)
			return true
		}
	}
	return false
}

// PrimaryExecutable returns the executable flagged as primary, falling back to
// the first entry.
func (g *GameInfo) PrimaryExecutable() *Executable {
	for i := range g.Executables {
		if g.Executables[i].Primary {
			return &g.Executables[i]
		}
	}
	if len(g.Executables) > 0 {
		return &g.Executables[0]
	}
	return nil
}

// --- identity ---------------------------------------------------------------

// NewID derives a stable identifier for a game directory.
//
// A Steam app ID is globally unique and is used verbatim. Everything else is
// keyed on the path relative to the library root, so two games that happen to
// share a folder name in different libraries no longer collide, while the same
// game keeps its ID no matter which drive letter a client mounts the share on.
func NewID(root, gameDir, steamAppID string) string {
	if steamAppID != "" {
		return "steam_" + steamAppID
	}
	key := gameDir
	if root != "" {
		if rel, err := filepath.Rel(root, gameDir); err == nil {
			key = rel
		}
	}
	key = strings.ToLower(filepath.ToSlash(filepath.Clean(key)))

	base := sanitizeID(filepath.Base(gameDir))
	if base == "" {
		base = "game"
	}
	return fmt.Sprintf("local_%s_%08x", base, hashString(key))
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// stripBOM removes a UTF-8/UTF-16 byte order mark and surrounding whitespace.
// Steam's steam_appid.txt is routinely written with a BOM by Windows tooling.
func stripBOM(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	s = strings.TrimPrefix(s, "\uFFFE")
	return strings.TrimSpace(s)
}

// --- persistence ------------------------------------------------------------

// New builds a fresh record for a discovered game directory.
func New(root, gameDir string, executables []Executable, steamAppID string) *GameInfo {
	info := &GameInfo{
		SchemaVersion: SchemaVersion,
		ID:            NewID(root, gameDir, steamAppID),
		Title:         filepath.Base(gameDir),
		Type:          "game",
		Executables:   executables,
		ScannedAt:     time.Now().UTC().Format(time.RFC3339),
		GameDir:       gameDir,
	}
	if steamAppID != "" {
		info.Platforms = []PlatformInfo{{Platform: "steam", ID: steamAppID}}
		info.PreferredSource = "steam"
	}
	return info
}

// LoadFromDir reads a game record from its .gamemanager folder, falling back to
// the pre-0.6 .gameinfo.json location, which is migrated in place.
func LoadFromDir(gameDir string) (*GameInfo, error) {
	data, err := os.ReadFile(InfoFilePath(gameDir))
	if err != nil {
		if os.IsNotExist(err) {
			return migrateLegacyFile(gameDir)
		}
		return nil, err
	}
	info, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", InfoFilePath(gameDir), err)
	}
	info.GameDir = gameDir
	return info, nil
}

// decode parses a record, applying legacy field migrations.
func decode(data []byte) (*GameInfo, error) {
	// A human-edited metadata file routinely carries a BOM, which encoding/json
	// rejects; without this the whole record failed to load.
	data = fsutil.StripBOM(data)

	info := &GameInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, err
	}

	// The pre-0.5 format stored a single platform under two flat keys. Reading
	// the raw map is only necessary when the modern field is absent.
	if len(info.Platforms) == 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			migrateFlatPlatform(raw, info)
		}
	}

	if info.SchemaVersion == 0 {
		info.SchemaVersion = 1
	}
	info.ID = stripBOM(info.ID)
	if info.Type == "" {
		info.Type = "game"
	}
	for i := range info.Platforms {
		info.Platforms[i].ID = stripBOM(info.Platforms[i].ID)
	}
	return info, nil
}

func migrateFlatPlatform(raw map[string]json.RawMessage, info *GameInfo) {
	var platform, platformID string
	if v, ok := raw["platform"]; ok {
		_ = json.Unmarshal(v, &platform)
	}
	if v, ok := raw["platformId"]; ok {
		_ = json.Unmarshal(v, &platformID)
	}
	platform = stripBOM(platform)
	platformID = stripBOM(platformID)
	if platform != "" || platformID != "" {
		info.Platforms = []PlatformInfo{{Platform: platform, ID: platformID}}
		if platform != "" {
			info.PreferredSource = platform
		}
	}
}

// migrateLegacyFile upgrades a pre-0.6 ".gameinfo.json" into the managed
// folder. The legacy file is only removed once the new one is safely written.
func migrateLegacyFile(gameDir string) (*GameInfo, error) {
	legacyPath := LegacyInfoPath(gameDir)
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return nil, err
	}
	info, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("parse legacy %s: %w", legacyPath, err)
	}
	info.GameDir = gameDir
	if info.ID == "" {
		info.ID = NewID("", gameDir, info.PlatformID("steam"))
	}

	migrateLegacyCovers(gameDir)
	if err := info.Save(); err != nil {
		return nil, err
	}
	_ = os.Remove(legacyPath)
	return info, nil
}

// migrateLegacyCovers moves cover art from the game root into covers/.
//
// The destination directory has to be created first: os.Rename into a
// non-existent directory fails on Windows, and this used to run before anything
// else created .gamemanager, so legacy cover art was silently left behind in the
// game root.
func migrateLegacyCovers(gameDir string) {
	coverDir := CoverDir(gameDir)
	for _, stem := range []string{CoverName, CoverLandscapeName} {
		for _, ext := range coverExtensions {
			oldPath := filepath.Join(gameDir, stem+ext)
			if !fsutil.Exists(oldPath) {
				continue
			}
			target := filepath.Join(coverDir, stem+ext)
			if fsutil.Exists(target) {
				break
			}
			if err := os.MkdirAll(coverDir, 0o755); err != nil {
				break
			}
			if err := os.Rename(oldPath, target); err != nil {
				break
			}
			break
		}
	}
}

// coverExtensions are the accepted cover file extensions, in preference order.
var coverExtensions = []string{".jpg", ".jpeg", ".png", ".webp"}

// --- sorting ----------------------------------------------------------------

// SortByRecentPlay orders games most-recently-played first, then by title.
// Games that have never been played come after those that have.
func SortByRecentPlay(games []*GameInfo) {
	sort.SliceStable(games, func(i, j int) bool {
		a, b := games[i], games[j]
		switch {
		case a.LastPlayedAt != "" && b.LastPlayedAt != "":
			if a.LastPlayedAt != b.LastPlayedAt {
				return a.LastPlayedAt > b.LastPlayedAt
			}
		case a.LastPlayedAt != "":
			return true
		case b.LastPlayedAt != "":
			return false
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
}
