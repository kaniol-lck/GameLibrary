package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"GameLibrary/internal/fsutil"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// storedDir is the canonical stored form of a directory directly below the
// library root ("." + separator + name). The separator differs per host, so
// tests must not hard-code a Windows-style path.
func storedDir(name string) string {
	return "." + string(filepath.Separator) + name
}

// storedAbs is the stored form of a path that cannot be expressed relative to
// the root: absolute and with forward slashes rewritten to backslashes.
func storedAbs(path string) string {
	return strings.ReplaceAll(filepath.Clean(path), "/", `\`)
}

func keysOf(sources []MetadataSource) []string {
	keys := make([]string, 0, len(sources))
	for _, s := range sources {
		keys = append(keys, s.Key)
	}
	return keys
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

func writeConfig(t *testing.T, root string, raw any) {
	t.Helper()
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

func hasSteamLabel(labels []string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, "Steam") {
			return true
		}
	}
	return false
}

// --- Default / Normalize ----------------------------------------------------

func TestDefaultIsFullyPopulated(t *testing.T) {
	cfg := Default()

	if cfg.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", cfg.SchemaVersion, SchemaVersion)
	}
	if len(cfg.GameDirectories) == 0 {
		t.Error("GameDirectories should not be empty")
	}
	if cfg.GameDirectoryLabels == nil {
		t.Error("GameDirectoryLabels should be initialised, not nil")
	}
	if cfg.MaxScanDepth != defaultMaxScanDepth {
		t.Errorf("MaxScanDepth = %d, want %d", cfg.MaxScanDepth, defaultMaxScanDepth)
	}
	if cfg.Language != defaultLanguage {
		t.Errorf("Language = %q, want %q", cfg.Language, defaultLanguage)
	}
	if !cfg.WatcherEnabled {
		t.Error("WatcherEnabled should default to true")
	}
	if cfg.WatcherDebounceMs != defaultWatcherDebounceMs {
		t.Errorf("WatcherDebounceMs = %d, want %d", cfg.WatcherDebounceMs, defaultWatcherDebounceMs)
	}
	if len(cfg.Sources) != len(DefaultSources()) {
		t.Errorf("Sources = %d entries, want %d", len(cfg.Sources), len(DefaultSources()))
	}
	seen := make(map[string]bool, len(cfg.Sources))
	for i, src := range cfg.Sources {
		if src.Key == "" {
			t.Errorf("Sources[%d] has an empty key", i)
		}
		if src.Name == "" {
			t.Errorf("Sources[%d] (%q) has an empty display name", i, src.Key)
		}
		if seen[src.Key] {
			t.Errorf("Sources contains %q twice", src.Key)
		}
		seen[src.Key] = true
	}
}

func TestDefaultReturnsIndependentCopies(t *testing.T) {
	first := Default()
	first.Sources[0].Enabled = false
	first.GameDirectories[0] = "mutated"

	second := Default()
	if !second.Sources[0].Enabled {
		t.Error("mutating one Default() must not affect the source list of another")
	}
	if second.GameDirectories[0] == "mutated" {
		t.Error("mutating one Default() must not affect the directory list of another")
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	root := t.TempDir()

	cfg := Default()
	cfg.MachineID = "idempotent-machine"
	if err := cfg.Normalize(root); err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if cfg.MachineID != "idempotent-machine" {
		t.Errorf("Normalize replaced a non-empty MachineID: %q", cfg.MachineID)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q after Normalize", cfg.LogLevel, "info")
	}
	if cfg.Language != defaultLanguage {
		t.Errorf("Language = %q, want %q", cfg.Language, defaultLanguage)
	}
	if cfg.MaxScanDepth != defaultMaxScanDepth {
		t.Errorf("MaxScanDepth = %d, want %d", cfg.MaxScanDepth, defaultMaxScanDepth)
	}
	if cfg.WatcherDebounceMs != defaultWatcherDebounceMs {
		t.Errorf("WatcherDebounceMs = %d, want %d", cfg.WatcherDebounceMs, defaultWatcherDebounceMs)
	}
	if want := []string{storedDir("Games")}; !reflect.DeepEqual(cfg.GameDirectories, want) {
		t.Errorf("GameDirectories = %v, want %v", cfg.GameDirectories, want)
	}

	first := mustJSON(t, cfg)
	if err := cfg.Normalize(root); err != nil {
		t.Fatalf("second Normalize: %v", err)
	}
	if second := mustJSON(t, cfg); second != first {
		t.Errorf("Normalize is not idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}
}

// --- Load / Save ------------------------------------------------------------

func TestLoadOnEmptyDirectoryCreatesDefaults(t *testing.T) {
	root := t.TempDir()

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	path := filepath.Join(root, "config.json")
	if !fileExists(path) {
		t.Fatalf("Load did not create %s", path)
	}
	if cfg.MachineID == "" {
		t.Error("MachineID should be auto-generated on first run")
	}
	if cfg.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", cfg.SchemaVersion, SchemaVersion)
	}
	if cfg.MaxScanDepth != defaultMaxScanDepth {
		t.Errorf("MaxScanDepth = %d, want %d", cfg.MaxScanDepth, defaultMaxScanDepth)
	}
	if cfg.Language != defaultLanguage {
		t.Errorf("Language = %q, want %q", cfg.Language, defaultLanguage)
	}
	if cfg.WatcherDebounceMs != defaultWatcherDebounceMs {
		t.Errorf("WatcherDebounceMs = %d, want %d", cfg.WatcherDebounceMs, defaultWatcherDebounceMs)
	}
	if !cfg.WatcherEnabled {
		t.Error("WatcherEnabled = false, want true")
	}
	if want := []string{storedDir("Games")}; !reflect.DeepEqual(cfg.GameDirectories, want) {
		t.Errorf("GameDirectories = %v, want %v", cfg.GameDirectories, want)
	}
	if got, want := keysOf(cfg.Sources), keysOf(DefaultSources()); !reflect.DeepEqual(got, want) {
		t.Errorf("Sources = %v, want %v", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("config.json is not valid JSON: %s", data)
	}

	reloaded, err := Load(root)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if !reflect.DeepEqual(cfg, reloaded) {
		t.Errorf("reloading the auto-created config changed it:\nfirst:  %s\nsecond: %s",
			mustJSON(t, cfg), mustJSON(t, reloaded))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()

	cfg := Default()
	cfg.MachineID = "round-trip-machine"
	cfg.GameDirectories = []string{storedDir("Games"), storedDir("SteamLibrary")}
	cfg.GameDirectoryLabels = map[string][]string{storedDir("Games"): {"Favourites"}}
	cfg.MaxScanDepth = 5
	cfg.Language = "en-US"
	cfg.SteamUserID = "76561198000000000"
	cfg.WatcherEnabled = false
	cfg.WatcherDebounceMs = 250
	cfg.LogLevel = "debug"
	cfg.LogToLibrary = true
	cfg.Sources[0].Settings = map[string]string{"apiKey": "steam-key"}
	cfg.Sources[1].Enabled = false                 // vndb off
	cfg.Sources[len(cfg.Sources)-1].Enabled = true // steamgriddb on

	if err := cfg.Save(root); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Errorf("round trip changed the config:\nloaded: %s\nsaved:  %s",
			mustJSON(t, loaded), mustJSON(t, cfg))
	}

	// Spot checks so a broken DeepEqual could not hide a real regression.
	if loaded.MachineID != "round-trip-machine" {
		t.Errorf("MachineID = %q, want %q", loaded.MachineID, "round-trip-machine")
	}
	if loaded.MaxScanDepth != 5 {
		t.Errorf("MaxScanDepth = %d, want 5", loaded.MaxScanDepth)
	}
	if loaded.Language != "en-US" {
		t.Errorf("Language = %q, want en-US", loaded.Language)
	}
	if loaded.SteamUserID != "76561198000000000" {
		t.Errorf("SteamUserID = %q", loaded.SteamUserID)
	}
	if loaded.WatcherEnabled {
		t.Error("WatcherEnabled = true, want false")
	}
	if loaded.WatcherDebounceMs != 250 {
		t.Errorf("WatcherDebounceMs = %d, want 250", loaded.WatcherDebounceMs)
	}
	if loaded.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", loaded.LogLevel)
	}
	if !loaded.LogToLibrary {
		t.Error("LogToLibrary = false, want true")
	}
	if got := loaded.SourceSettings("steam")["apiKey"]; got != "steam-key" {
		t.Errorf("steam apiKey = %q, want steam-key", got)
	}
	if loaded.SourceEnabled("vndb") {
		t.Error("vndb should still be disabled after the round trip")
	}
	if !loaded.SourceEnabled("steamgriddb") {
		t.Error("steamgriddb should still be enabled after the round trip")
	}
	if got, want := loaded.GameDirectoryLabels[storedDir("Games")], []string{"Favourites"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels for the user directory = %v, want %v", got, want)
	}
	if got, want := loaded.GameDirectoryLabels[storedDir("SteamLibrary")], []string{"Steam"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels for the Steam library = %v, want %v", got, want)
	}
}

func TestSaveWritesValidJSONAndLeavesNoTempFiles(t *testing.T) {
	root := t.TempDir()

	cfg := Default()
	cfg.MachineID = "json-machine"
	if err := cfg.Save(root); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(root, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("config.json is not valid JSON: %s", data)
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal config.json: %v", err)
	}
	if doc.SchemaVersion != SchemaVersion {
		t.Errorf("written schemaVersion = %d, want %d", doc.SchemaVersion, SchemaVersion)
	}
	if !strings.Contains(string(data), `"schemaVersion"`) {
		t.Errorf("config.json has no schemaVersion field: %s", data)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("Save left unexpected files behind: %v", names)
	}

	loaded, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Errorf("Load of the saved file differs:\nloaded: %s\nsaved:  %s",
			mustJSON(t, loaded), mustJSON(t, cfg))
	}
}

// --- NormalizeDir -----------------------------------------------------------

func TestNormalizeDirCanonicalForms(t *testing.T) {
	base := t.TempDir()
	// A deliberately deep root: it makes the "too many climbs" case below
	// independent of how shallow the OS temporary directory happens to be.
	root := filepath.Join(base, "a", "b", "c", "d", "e", "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	want := storedDir("Games")

	for _, in := range []string{"Games", "./Games", `.\Games`, "Games/", `Games\\`, " Games ", "Games//", "Games/./"} {
		if got := NormalizeDir(root, in); got != want {
			t.Errorf("NormalizeDir(%q) = %q, want %q", in, got, want)
		}
	}
	for _, blank := range []string{"", "   ", "\t\n"} {
		if got := NormalizeDir(root, blank); got != "" {
			t.Errorf("NormalizeDir(%q) = %q, want empty string", blank, got)
		}
	}

	climbInput := func(levels int) string {
		parts := make([]string, 0, levels+1)
		for i := 0; i < levels; i++ {
			parts = append(parts, "..")
		}
		return filepath.Join(append(parts, "Games")...)
	}
	climbStored := func(levels int) string {
		parts := []string{"."}
		for i := 0; i < levels; i++ {
			parts = append(parts, "..")
		}
		return strings.Join(append(parts, "Games"), string(filepath.Separator))
	}

	// ".\\..\\Games" and friends stay relative as long as the climb is small.
	for levels := 1; levels <= 3; levels++ {
		in := climbInput(levels)
		if got, want := NormalizeDir(root, in), climbStored(levels); got != want {
			t.Errorf("NormalizeDir(%q) = %q, want %q", in, got, want)
		}
	}

	// Beyond fsutil's climb limit the absolute path is kept instead.
	deep := climbInput(4)
	deepAbs := fsutil.Resolve(root, deep)
	if got, want := NormalizeDir(root, deep), storedAbs(deepAbs); got != want {
		t.Errorf("NormalizeDir(%q) = %q, want absolute %q", deep, got, want)
	}
	if got := NormalizeDir(root, deep); !filepath.IsAbs(fsutil.ToNative(got)) {
		t.Errorf("a 4-level climb should be stored absolute, got %q", got)
	}

	// An absolute directory outside the root stays absolute.
	outside := t.TempDir()
	if got, want := NormalizeDir(root, outside), storedAbs(outside); got != want {
		t.Errorf("NormalizeDir(%q) = %q, want %q", outside, got, want)
	}
	if got := NormalizeDir(root, outside); !filepath.IsAbs(fsutil.ToNative(got)) {
		t.Errorf("an outside absolute path should be stored absolute, got %q", got)
	}
}

func TestNormalizeCollapsesDuplicateDirectories(t *testing.T) {
	root := t.TempDir()

	cfg := Default()
	cfg.GameDirectories = []string{"Games", "games", "Games/", `.\Games`, "", "   ", "Other", "GAMES", "other"}
	if err := cfg.Normalize(root); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	want := []string{storedDir("Games"), storedDir("Other")}
	if !reflect.DeepEqual(cfg.GameDirectories, want) {
		t.Errorf("GameDirectories = %v, want %v", cfg.GameDirectories, want)
	}

	// A list that only contains blanks becomes an empty (not nil) list.
	blank := Default()
	blank.GameDirectories = []string{"", "  "}
	if err := blank.Normalize(root); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if blank.GameDirectories == nil || len(blank.GameDirectories) != 0 {
		t.Errorf("GameDirectories = %#v, want an empty slice", blank.GameDirectories)
	}
}

// --- Normalize clamps -------------------------------------------------------

func TestNormalizeClampsMaxScanDepth(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		in   int
		want int
	}{
		{0, defaultMaxScanDepth},
		{-7, defaultMaxScanDepth},
		{1, 1},
		{defaultMaxScanDepth, defaultMaxScanDepth},
		{maxMaxScanDepth, maxMaxScanDepth},
		{999, maxMaxScanDepth},
	}
	for _, tc := range cases {
		cfg := Default()
		cfg.MaxScanDepth = tc.in
		if err := cfg.Normalize(root); err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if cfg.MaxScanDepth != tc.want {
			t.Errorf("MaxScanDepth %d normalized to %d, want %d", tc.in, cfg.MaxScanDepth, tc.want)
		}
		if cfg.MaxScanDepth < 1 || cfg.MaxScanDepth > maxMaxScanDepth {
			t.Errorf("MaxScanDepth %d is outside 1..%d", cfg.MaxScanDepth, maxMaxScanDepth)
		}
	}
}

func TestNormalizeClampsScalarFields(t *testing.T) {
	root := t.TempDir()

	logLevels := []struct{ in, want string }{
		{"", "info"},
		{"verbose", "info"},
		{"trace", "info"},
		{"debug", "debug"},
		{"DEBUG", "debug"},
		{"info", "info"},
		{"WARN", "warn"},
		{"warning", "warning"},
		{"Error", "error"},
	}
	for _, tc := range logLevels {
		cfg := Default()
		cfg.LogLevel = tc.in
		if err := cfg.Normalize(root); err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if cfg.LogLevel != tc.want {
			t.Errorf("LogLevel %q normalized to %q, want %q", tc.in, cfg.LogLevel, tc.want)
		}
	}

	languages := []struct{ in, want string }{
		{"", defaultLanguage},
		{"zh-CN", "zh-CN"},
		{"en-US", "en-US"},
		{"ja-JP", "ja-JP"},
	}
	for _, tc := range languages {
		cfg := Default()
		cfg.Language = tc.in
		if err := cfg.Normalize(root); err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if cfg.Language != tc.want {
			t.Errorf("Language %q normalized to %q, want %q", tc.in, cfg.Language, tc.want)
		}
	}

	debounces := []struct{ in, want int }{
		{0, defaultWatcherDebounceMs},
		{-3, defaultWatcherDebounceMs},
		{1, 1},
		{250, 250},
	}
	for _, tc := range debounces {
		cfg := Default()
		cfg.WatcherDebounceMs = tc.in
		if err := cfg.Normalize(root); err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		if cfg.WatcherDebounceMs != tc.want {
			t.Errorf("WatcherDebounceMs %d normalized to %d, want %d", tc.in, cfg.WatcherDebounceMs, tc.want)
		}
	}
}

// --- Metadata sources -------------------------------------------------------

func TestSyncSourcesAppendsMissingDefaults(t *testing.T) {
	cfg := Default()
	cfg.Sources = []MetadataSource{
		{Key: "dlsite", Name: "DLsite", Enabled: false, Settings: map[string]string{"locale": "ja-JP"}},
		{Key: "legacy-provider", Name: "Legacy Provider", Enabled: true, Settings: map[string]string{"token": "t"}},
		{Key: "steam", Name: "Steam", Enabled: false},
		{Key: "steam", Name: "Steam (duplicate)", Enabled: true},
	}

	cfg.SyncSources()

	// A provider with no implementation is dropped rather than kept: it would
	// otherwise keep showing up in the settings UI as a dead, unconfigurable row
	// (which is exactly what happened to "igdb"). The duplicate steam entry is
	// collapsed to the first one, and the remaining keys keep the user's order.
	want := []string{"dlsite", "steam", "vndb", "bangumi", "rawg", "steamgriddb"}
	if got := keysOf(cfg.Sources); !reflect.DeepEqual(got, want) {
		t.Errorf("source keys = %v, want %v", got, want)
	}
	if cfg.SourceEnabled("steam") {
		t.Error("the user's disabled steam flag must be preserved")
	}
	if cfg.SourceEnabled("dlsite") {
		t.Error("the user's disabled dlsite flag must be preserved")
	}
	if got := cfg.SourceSettings("dlsite")["locale"]; got != "ja-JP" {
		t.Errorf("dlsite settings = %q, want ja-JP", got)
	}
	if replaced := cfg.SourceSettings("legacy-provider"); replaced != nil {
		t.Errorf("an unsupported provider must be dropped, got settings %v", replaced)
	}

	before := mustJSON(t, cfg)
	cfg.SyncSources()
	if after := mustJSON(t, cfg); after != before {
		t.Errorf("SyncSources is not idempotent:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestSyncSourcesKeepsDefaultsWhenListIsEmpty(t *testing.T) {
	cfg := Default()
	cfg.Sources = nil
	cfg.SyncSources()
	if got, want := keysOf(cfg.Sources), keysOf(DefaultSources()); !reflect.DeepEqual(got, want) {
		t.Errorf("source keys = %v, want %v", got, want)
	}
}

func TestLegacyBooleanMigration(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, map[string]any{
		"machineId":       "legacy-machine",
		"machineName":     "Legacy Machine",
		"gameDirectories": []string{`.\OldGames`},
		"maxScanDepth":    2,
		"language":        "en-US",
		"vndbEnabled":     true,
		"dlsiteEnabled":   false,
	})

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}
	if len(cfg.Sources) != len(DefaultSources()) {
		t.Fatalf("migrated %d sources, want %d: %v", len(cfg.Sources), len(DefaultSources()), keysOf(cfg.Sources))
	}
	enabled := make(map[string]bool, len(cfg.Sources))
	for _, src := range cfg.Sources {
		enabled[src.Key] = src.Enabled
	}
	if !enabled["vndb"] {
		t.Error("vndb should be enabled from the legacy boolean")
	}
	if enabled["dlsite"] {
		t.Error("dlsite should be disabled from the legacy boolean")
	}
	// Providers the legacy file said nothing about keep their defaults.
	if !enabled["steam"] || !enabled["bangumi"] || !enabled["rawg"] {
		t.Errorf("untouched providers lost their default state: %v", enabled)
	}
	if enabled["steamgriddb"] {
		t.Error("steamgriddb should stay disabled by default")
	}
	if cfg.MachineID != "legacy-machine" {
		t.Errorf("MachineID = %q, want legacy-machine", cfg.MachineID)
	}
	if cfg.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", cfg.SchemaVersion, SchemaVersion)
	}
	if want := []string{storedDir("OldGames")}; !reflect.DeepEqual(cfg.GameDirectories, want) {
		t.Errorf("GameDirectories = %v, want %v", cfg.GameDirectories, want)
	}
	if cfg.MaxScanDepth != 2 || cfg.Language != "en-US" {
		t.Errorf("legacy scalars not preserved: depth=%d language=%q", cfg.MaxScanDepth, cfg.Language)
	}

	// The remaining legacy toggles are honoured as well.
	root2 := t.TempDir()
	writeConfig(t, root2, map[string]any{"bangumiEnabled": false, "steamgriddbEnabled": true})
	cfg2, err := Load(root2)
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}
	if cfg2.SourceEnabled("bangumi") {
		t.Error("bangumiEnabled=false was not applied")
	}
	if !cfg2.SourceEnabled("steamgriddb") {
		t.Error("steamgriddbEnabled=true was not applied")
	}
}

// --- AutoLabelPaths ---------------------------------------------------------

func TestAutoLabelPathsMarksSteamDirectories(t *testing.T) {
	root := t.TempDir()
	dirs := normalizeDirs(root, []string{`.\SteamLibrary`, "./SteamApps", `.\Games`, `.\Steam\common`})
	if len(dirs) != 4 {
		t.Fatalf("expected 4 normalized directories, got %v", dirs)
	}

	cfg := Default()
	cfg.GameDirectories = dirs
	cfg.AutoLabelPaths(root)

	for _, i := range []int{0, 1, 3} {
		if !hasSteamLabel(cfg.GameDirectoryLabels[dirs[i]]) {
			t.Errorf("directory %q (%q) should carry the Steam label, got %v",
				dirs[i], []string{`.\SteamLibrary`, "./SteamApps", `.\Steam\common`}[i], cfg.GameDirectoryLabels[dirs[i]])
		}
	}
	if hasSteamLabel(cfg.GameDirectoryLabels[dirs[2]]) {
		t.Errorf("ordinary directory %q should not carry the Steam label, got %v",
			dirs[2], cfg.GameDirectoryLabels[dirs[2]])
	}
}

func TestAutoLabelPathsPreservesLabelsAndPrunesStaleOnes(t *testing.T) {
	root := t.TempDir()
	dirs := normalizeDirs(root, []string{`.\Games`, `.\SteamLibrary`})
	if len(dirs) != 2 {
		t.Fatalf("expected 2 normalized directories, got %v", dirs)
	}
	stale := storedDir("Gone")

	cfg := Default()
	cfg.GameDirectories = dirs
	cfg.GameDirectoryLabels = map[string][]string{
		dirs[0]:     {"Favourites"},
		dirs[1]:     {"MyGames"},
		stale:       {"Steam"},
		"":          {"junk"},
		"untouched": {"whatever"},
	}

	cfg.AutoLabelPaths(root)

	if got, want := cfg.GameDirectoryLabels[dirs[0]], []string{"Favourites"}; !reflect.DeepEqual(got, want) {
		t.Errorf("user label lost: %v, want %v", got, want)
	}
	if got := cfg.GameDirectoryLabels[dirs[1]]; len(got) != 2 || got[0] != "MyGames" || !strings.EqualFold(got[1], "Steam") {
		t.Errorf("labels for the Steam library = %v, want [MyGames Steam]", got)
	}
	// Regression: labels for directories that are no longer configured used to
	// accumulate in config.json forever.
	if _, ok := cfg.GameDirectoryLabels[stale]; ok {
		t.Errorf("labels for the removed directory %q were not pruned: %v", stale, cfg.GameDirectoryLabels)
	}
	if _, ok := cfg.GameDirectoryLabels[""]; ok {
		t.Error("the blank label key was not pruned")
	}
	if _, ok := cfg.GameDirectoryLabels["untouched"]; ok {
		t.Error("labels for an unconfigured directory were not pruned")
	}
	if len(cfg.GameDirectoryLabels) != 2 {
		t.Errorf("GameDirectoryLabels = %v, want only the two configured directories", cfg.GameDirectoryLabels)
	}

	// Idempotent: repeated runs must not stack duplicate Steam labels.
	for i := 0; i < 3; i++ {
		cfg.AutoLabelPaths(root)
	}
	count := 0
	for _, l := range cfg.GameDirectoryLabels[dirs[1]] {
		if strings.EqualFold(l, "Steam") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the Steam label appears %d times, want exactly 1: %v", count, cfg.GameDirectoryLabels[dirs[1]])
	}
	if got, want := cfg.GameDirectoryLabels[dirs[0]], []string{"Favourites"}; !reflect.DeepEqual(got, want) {
		t.Errorf("user label lost after repeated runs: %v, want %v", got, want)
	}
}

func TestLoadPrunesStaleDirectoryLabels(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, map[string]any{
		"machineId":       "labels-machine",
		"gameDirectories": []string{`.\Games`},
		"gameDirectoryLabels": map[string][]string{
			`.\Gone`:  {"Steam"},
			`.\Games`: {"Keep"},
		},
		"metadataSources": []map[string]any{
			{"key": "steam", "name": "Steam", "enabled": true},
		},
	})

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.GameDirectoryLabels[`.\Gone`]; ok {
		t.Errorf("stale label key survived a Load: %v", cfg.GameDirectoryLabels)
	}
	if got, want := cfg.GameDirectoryLabels[storedDir("Games")], []string{"Keep"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels for the configured directory = %v, want %v", got, want)
	}
}

// --- Accessors --------------------------------------------------------------

func TestSourceAccessors(t *testing.T) {
	cfg := Default()

	if got, want := cfg.EnabledSourceKeys(), []string{"steam", "vndb", "bangumi", "dlsite", "rawg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("EnabledSourceKeys() = %v, want %v", got, want)
	}
	if cfg.SourceEnabled("nope") {
		t.Error("an unknown source must not report as enabled")
	}
	if cfg.SourceSettings("nope") != nil {
		t.Error("an unknown source must have nil settings")
	}
	if cfg.SourceSettings("steam") != nil {
		t.Error("a source without settings must return nil")
	}

	cfg.Sources[0].Settings = map[string]string{"apiKey": "k"}
	cfg.Sources[0].Enabled = false // steam
	cfg.Sources[3].Enabled = false // dlsite
	if cfg.SourceEnabled("steam") {
		t.Error("steam should be disabled")
	}
	if !cfg.SourceEnabled("vndb") {
		t.Error("vndb should still be enabled")
	}
	if got := cfg.SourceSettings("steam")["apiKey"]; got != "k" {
		t.Errorf("steam apiKey = %q, want k", got)
	}
	if got, want := cfg.EnabledSourceKeys(), []string{"vndb", "bangumi", "rawg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("EnabledSourceKeys() = %v, want %v", got, want)
	}

	keys := cfg.EnabledSourceKeys()
	keys[0] = "mutated"
	if got := cfg.EnabledSourceKeys()[0]; got != "vndb" {
		t.Errorf("EnabledSourceKeys returned a view into the config (got %q)", got)
	}
}

func TestResolveDir(t *testing.T) {
	root := t.TempDir()
	cfg := Default()
	want := filepath.Join(root, "Games")

	for _, in := range []string{`.\Games`, "./Games", "Games"} {
		if got := cfg.ResolveDir(root, in); got != want {
			t.Errorf("ResolveDir(%q) = %q, want %q", in, got, want)
		}
	}

	abs := filepath.Join(t.TempDir(), "Elsewhere")
	if got := cfg.ResolveDir(root, abs); got != filepath.Clean(abs) {
		t.Errorf("ResolveDir(%q) = %q, want %q", abs, got, filepath.Clean(abs))
	}
	if got := cfg.ResolveDir(root, ""); got != "" {
		t.Errorf("ResolveDir(root, \"\") = %q, want empty", got)
	}
}

func TestHostnameIsStable(t *testing.T) {
	if first, second := Hostname(), Hostname(); first != second {
		t.Errorf("Hostname() is not stable: %q then %q", first, second)
	}
}
