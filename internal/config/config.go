// Package config holds the shared, NAS-resident application configuration and
// its persistence.
//
// config.json lives next to the executable on the share and is therefore read
// and written by every client. Two rules follow from that:
//
//   - Nothing machine specific may be stored in it. The machine name is derived
//     from os.Hostname() at runtime instead.
//   - Directory entries are stored in a portable form (see fsutil.ToPortable) so
//     the same file works whichever drive letter a client mounts the share on.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/logger"
)

// SchemaVersion is the current config.json format version.
const SchemaVersion = 2

const (
	defaultMaxScanDepth      = 3
	maxMaxScanDepth          = 10
	defaultLanguage          = "zh-CN"
	defaultWatcherDebounceMs = 150
	// DefaultScrapeConcurrency is how many games are scraped at once. The scrapers
	// are rate limited per host, so this parallelises the waiting rather than
	// increasing the request rate any single service sees.
	DefaultScrapeConcurrency = 4
	maxScrapeConcurrency     = 16
)

const (
	languageChinese  = "zh-CN"
	languageEnglish  = "en-US"
	languageJapanese = "ja-JP"
)

// MetadataSource is one scrape provider and its per-source settings.
type MetadataSource struct {
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings,omitempty"`
}

// DefaultSources returns the built-in provider list in priority order.
func DefaultSources() []MetadataSource {
	return []MetadataSource{
		{Key: "steam", Name: "Steam", Enabled: true},
		{Key: "vndb", Name: "VNDB (Visual Novel Database)", Enabled: true},
		{Key: "bangumi", Name: "Bangumi (bgm.tv)", Enabled: true},
		{Key: "dlsite", Name: "DLsite", Enabled: true},
		{Key: "rawg", Name: "RAWG.io", Enabled: true},
		{Key: "steamgriddb", Name: "SteamGridDB (Cover Art)", Enabled: false},
	}
}

// Config is the shared application configuration.
type Config struct {
	SchemaVersion int `json:"schemaVersion"`

	MachineID           string              `json:"machineId"`
	GameDirectories     []string            `json:"gameDirectories"`
	GameDirectoryLabels map[string][]string `json:"gameDirectoryLabels,omitempty"`
	MaxScanDepth        int                 `json:"maxScanDepth"`
	Language            string              `json:"language"`
	SteamUserID         string              `json:"steamUserId,omitempty"`
	WatcherEnabled      bool                `json:"watcherEnabled"`
	WatcherDebounceMs   int                 `json:"watcherDebounceMs"`

	// ScrapeConcurrency is how many games are scraped simultaneously. Each host is
	// still rate limited, so raising this shortens a library-wide scrape without
	// hammering any single service.
	ScrapeConcurrency int `json:"scrapeConcurrency,omitempty"`

	// LogLevel is one of debug/info/warn/error.
	LogLevel string `json:"logLevel,omitempty"`
	// LogToLibrary writes logs next to the executable on the share instead of
	// into the per-machine cache directory. Off by default: a portable library
	// keeps its logs local unless the user is explicitly collecting them.
	LogToLibrary bool `json:"logToLibrary,omitempty"`

	Sources []MetadataSource `json:"metadataSources"`
}

// Default returns a configuration for a fresh install.
func Default() *Config {
	return &Config{
		SchemaVersion:       SchemaVersion,
		GameDirectories:     []string{".\\Games"},
		GameDirectoryLabels: map[string][]string{},
		MaxScanDepth:        defaultMaxScanDepth,
		Language:            defaultLanguage,
		WatcherEnabled:      true,
		WatcherDebounceMs:   defaultWatcherDebounceMs,
		ScrapeConcurrency:   DefaultScrapeConcurrency,
		Sources:             DefaultSources(),
	}
}

// Load reads config.json from the library root, creating a default file on first
// run and migrating older formats in place.
func Load(root string) (*Config, error) {
	data, err := fsutil.ReadFile(fsutil.Resolve(root, "config.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			logger.ConfigDefaultCreated(root)
			cfg := Default()
			cfg.MachineID = defaultMachineID()
			if saveErr := cfg.Save(root); saveErr != nil {
				// A read-only share must not stop the app from starting.
				logger.Warn("config: could not persist default config", "error", saveErr.Error())
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config.json: %w", err)
	}

	cfg, err := decode(data)
	if err != nil {
		return nil, err
	}
	if err := cfg.Normalize(root); err != nil {
		return nil, err
	}
	logger.ConfigLoaded(root, cfg.GameDirectories, len(cfg.Sources))
	return cfg, nil
}

// decode parses config.json, applying legacy migrations.
func decode(data []byte) (*Config, error) {
	// A BOM is common on a file a human has edited with Windows tooling, and
	// encoding/json rejects it outright — which used to discard every setting.
	data = fsutil.StripBOM(data)

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}

	// Pre-0.2 configs had no metadataSources list, only two booleans.
	if len(cfg.Sources) == 0 {
		var raw struct {
			VNDBEnabled        *bool `json:"vndbEnabled"`
			DLsiteEnabled      *bool `json:"dlsiteEnabled"`
			BangumiEnabled     *bool `json:"bangumiEnabled"`
			SteamGridDBEnabled *bool `json:"steamgriddbEnabled"`
		}
		if err := json.Unmarshal(data, &raw); err == nil {
			sources := DefaultSources()
			applyLegacyToggle(sources, "vndb", raw.VNDBEnabled)
			applyLegacyToggle(sources, "dlsite", raw.DLsiteEnabled)
			applyLegacyToggle(sources, "bangumi", raw.BangumiEnabled)
			applyLegacyToggle(sources, "steamgriddb", raw.SteamGridDBEnabled)
			cfg.Sources = sources
			logger.ConfigMigrated()
		}
	}
	return cfg, nil
}

func applyLegacyToggle(sources []MetadataSource, key string, enabled *bool) {
	if enabled == nil {
		return
	}
	for i := range sources {
		if sources[i].Key == key {
			sources[i].Enabled = *enabled
			return
		}
	}
}

// Normalize repairs and canonicalises the configuration in place. It is safe to
// call on both freshly loaded and freshly submitted configurations, which is what
// makes a save from the settings UI trustworthy.
func (c *Config) Normalize(root string) error {
	c.SchemaVersion = SchemaVersion

	if c.MachineID == "" {
		c.MachineID = defaultMachineID()
	}
	if c.MaxScanDepth <= 0 {
		c.MaxScanDepth = defaultMaxScanDepth
	}
	if c.MaxScanDepth > maxMaxScanDepth {
		c.MaxScanDepth = maxMaxScanDepth
	}
	if c.Language == "" {
		c.Language = defaultLanguage
	}
	if c.WatcherDebounceMs <= 0 {
		c.WatcherDebounceMs = defaultWatcherDebounceMs
	}
	if c.ScrapeConcurrency <= 0 {
		c.ScrapeConcurrency = DefaultScrapeConcurrency
	}
	if c.ScrapeConcurrency > maxScrapeConcurrency {
		c.ScrapeConcurrency = maxScrapeConcurrency
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "warning", "error":
		c.LogLevel = strings.ToLower(c.LogLevel)
	default:
		c.LogLevel = "info"
	}

	c.GameDirectories = normalizeDirs(root, c.GameDirectories)
	c.SyncSources()
	c.AutoLabelPaths(root)
	return nil
}

// normalizeDirs canonicalises directory entries, drops blanks and removes
// duplicates while preserving the user's order.
func normalizeDirs(root string, dirs []string) []string {
	out := make([]string, 0, len(dirs))
	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		normalized := NormalizeDir(root, dir)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, normalized)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// NormalizeDir canonicalises a single directory entry.
//
// The settings UI and hand-edited config files both produce sloppy input:
// "Games", "Games/", "./Games" and ".\Games" all mean the same thing, and a path
// picked with the folder browser arrives absolute. This collapses all of those to
// one stored form.
func NormalizeDir(root, dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	abs := fsutil.Resolve(root, dir)
	if abs == "" {
		return ""
	}
	return fsutil.ToPortable(root, abs)
}

// SyncSources reconciles the configured provider list with the providers the
// application actually ships.
//
// Providers added to the product since this config was written are appended with
// their default state, so an upgrade gains them automatically. Entries for
// providers that no longer exist are dropped rather than carried forever: an
// unimplemented entry would otherwise keep appearing in the settings UI as an
// undescribed, unconfigurable row (which is what happened to "igdb").
func (c *Config) SyncSources() {
	supported := make(map[string]MetadataSource, len(DefaultSources()))
	for _, def := range DefaultSources() {
		supported[def.Key] = def
	}

	known := make(map[string]bool, len(c.Sources))
	merged := make([]MetadataSource, 0, len(c.Sources)+len(supported))

	for _, src := range c.Sources {
		if src.Key == "" || known[src.Key] {
			continue
		}
		def, ok := supported[src.Key]
		if !ok {
			logger.Debug("config: dropping unsupported metadata source", "source", src.Key)
			continue
		}
		known[src.Key] = true
		if src.Name == "" {
			src.Name = def.Name
		}
		merged = append(merged, src)
	}

	for _, def := range DefaultSources() {
		if known[def.Key] {
			continue
		}
		known[def.Key] = true
		merged = append(merged, def)
		logger.Debug("config: added newly available metadata source", "source", def.Key)
	}

	c.Sources = merged
}

// AutoLabelPaths maintains the directory label map: Steam-looking directories
// gain the "Steam" label, and labels for directories that are no longer
// configured are pruned so the sidebar does not accumulate dead sections.
func (c *Config) AutoLabelPaths(root string) {
	if c.GameDirectoryLabels == nil {
		c.GameDirectoryLabels = map[string][]string{}
	}

	configured := make(map[string]bool, len(c.GameDirectories))
	for _, dir := range c.GameDirectories {
		configured[dir] = true
	}

	// Drop labels whose directory is gone. Keys are compared after
	// normalisation so a hand-edited variant does not survive forever.
	for key := range c.GameDirectoryLabels {
		canonical := NormalizeDir(root, key)
		if canonical == "" || !configured[canonical] {
			delete(c.GameDirectoryLabels, key)
		}
	}

	for _, dir := range c.GameDirectories {
		if !looksLikeSteamLibrary(dir) {
			continue
		}
		labels := c.GameDirectoryLabels[dir]
		if !containsLabel(labels, "Steam") {
			c.GameDirectoryLabels[dir] = append(labels, "Steam")
		}
	}
}

func looksLikeSteamLibrary(dir string) bool {
	lower := strings.ToLower(filepathToSlash(dir))
	return strings.Contains(lower, "steamlibrary") ||
		strings.Contains(lower, "steamapps") ||
		strings.Contains(lower, "steam/common")
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}

func containsLabel(labels []string, label string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}

// Save canonicalises and writes config.json atomically.
func (c *Config) Save(root string) error {
	if err := c.Normalize(root); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config.json: %w", err)
	}
	data = append(data, '\n')

	path := fsutil.Resolve(root, "config.json")
	if err := fsutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}
	logger.ConfigSaved(root)
	return nil
}

// SourceSettings returns the settings map for a provider, or nil.
func (c *Config) SourceSettings(key string) map[string]string {
	for _, s := range c.Sources {
		if s.Key == key {
			return s.Settings
		}
	}
	return nil
}

// SourceEnabled reports whether a provider is currently enabled.
func (c *Config) SourceEnabled(key string) bool {
	for _, s := range c.Sources {
		if s.Key == key {
			return s.Enabled
		}
	}
	return false
}

// EnabledSourceKeys returns the enabled provider keys in priority order.
func (c *Config) EnabledSourceKeys() []string {
	keys := make([]string, 0, len(c.Sources))
	for _, s := range c.Sources {
		if s.Enabled {
			keys = append(keys, s.Key)
		}
	}
	return keys
}

// ResolveDir returns the absolute path of a stored directory entry.
func (c *Config) ResolveDir(root, dir string) string {
	return fsutil.Resolve(root, dir)
}

func defaultMachineID() string {
	return "machine-" + Hostname()
}
