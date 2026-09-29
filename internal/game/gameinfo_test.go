package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// mustJSON snapshots a value so a test can prove the original record was not
// touched by edits made through its clone.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return string(data)
}

func TestNew(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "Games", "Portal 2")
	execs := []Executable{{Path: "portal2.exe", Name: "Portal 2", Primary: true}}

	local := New(root, gameDir, execs, "")
	if local.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", local.SchemaVersion, SchemaVersion)
	}
	if local.Type != "game" {
		t.Errorf("Type = %q, want %q", local.Type, "game")
	}
	if local.Title != "Portal 2" {
		t.Errorf("Title = %q, want the folder name %q", local.Title, "Portal 2")
	}
	if local.GameDir != gameDir {
		t.Errorf("GameDir = %q, want %q", local.GameDir, gameDir)
	}
	if !strings.HasPrefix(local.ID, "local_") {
		t.Errorf("ID = %q, want a local_ id when no Steam app id is known", local.ID)
	}
	if len(local.Platforms) != 0 {
		t.Errorf("Platforms = %#v, want empty without a Steam app id", local.Platforms)
	}
	if local.PreferredSource != "" {
		t.Errorf("PreferredSource = %q, want empty without a Steam app id", local.PreferredSource)
	}
	if _, err := time.Parse(time.RFC3339, local.ScannedAt); err != nil {
		t.Errorf("ScannedAt = %q, want RFC3339: %v", local.ScannedAt, err)
	}

	steam := New(root, gameDir, execs, "570")
	if steam.ID != "steam_570" {
		t.Errorf("ID = %q, want %q", steam.ID, "steam_570")
	}
	want := PlatformInfo{Platform: "steam", ID: "570"}
	if len(steam.Platforms) != 1 || steam.Platforms[0] != want {
		t.Errorf("Platforms = %#v, want exactly [%#v]", steam.Platforms, want)
	}
	if steam.PreferredSource != "steam" {
		t.Errorf("PreferredSource = %q, want %q", steam.PreferredSource, "steam")
	}
	if steam.Title != "Portal 2" {
		t.Errorf("Title = %q, want the folder name %q", steam.Title, "Portal 2")
	}
}

func TestNewID(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Games", "Some Game")

	if got := NewID(root, dir, "570"); got != "steam_570" {
		t.Errorf("NewID with app id = %q, want %q", got, "steam_570")
	}
	if got := NewID(root, dir, ""); !strings.HasPrefix(got, "local_") {
		t.Errorf("NewID without app id = %q, want a local_ prefix", got)
	}
	// Legacy migration derives ids with an empty root, which must still work.
	if got := NewID("", dir, ""); !strings.HasPrefix(got, "local_") {
		t.Errorf("NewID with empty root = %q, want a local_ prefix", got)
	}
}

// TestNewIDStableAcrossMountPoints pins the portability property: the library
// root is a per-client mount point, so the identifier must be derived from the
// path relative to the root. The same share mounted at C:\clientA\lib on one
// client and D:\mount\lib on another has to produce the same id, otherwise a
// single shared config.json/cache would be unusable from the second client.
func TestNewIDStableAcrossMountPoints(t *testing.T) {
	rel := filepath.Join("Games", "Samename")

	onC := NewID(`C:\clientA\lib`, filepath.Join(`C:\clientA\lib`, rel), "")
	onD := NewID(`D:\mount\lib`, filepath.Join(`D:\mount\lib`, rel), "")

	if onC != onD {
		t.Errorf("same root-relative path gave different ids:\n C: %q\n D: %q", onC, onD)
	}
	if !strings.HasPrefix(onC, "local_samename_") {
		t.Errorf("id = %q, want a local_samename_ prefix", onC)
	}
}

// TestNewIDDifferentLibrariesDoNotCollide is the regression test for the id
// collision bug: NewID used to be just the sanitized folder base name, so two
// libraries that both contained "Games\Samename" produced the identical id
// "local_samename". The library cache is keyed by id, so scanning the second
// library silently overwrote (or handed out the wrong metadata for) the first
// game. Keying on the root-relative path separates the two libraries.
//
// Note the deliberate limit of that fix: the key stays root-relative, so two
// libraries that are each passed as their own root and expose the same internal
// layout ("<root>\Games\Samename") still derive the same id. That case cannot be
// told apart from the mount-point difference covered above, so portability wins
// there; see the accompanying report.
func TestNewIDDifferentLibrariesDoNotCollide(t *testing.T) {
	root := `C:\library`
	first := NewID(root, filepath.Join(root, "libA", "Games", "Samename"), "")
	second := NewID(root, filepath.Join(root, "libB", "Games", "Samename"), "")

	if first == second {
		t.Errorf("two different libraries produced the same id %q; metadata would overwrite", first)
	}
	for _, id := range []string{first, second} {
		if !strings.HasPrefix(id, "local_samename_") {
			t.Errorf("id = %q, want a local_samename_ prefix", id)
		}
	}
}

func TestNewIDBaseNameSanitization(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		base string
		want string
	}{
		{"punctuation and non-ascii collapse", "Ω Ω!! Game", "local_game_"},
		{"nothing usable falls back to game", "!!!", "local_game_"},
		{"spaces become dashes", "My Game (2004)", "local_my-game-2004_"},
		{"dots underscores and dashes survive", "Game_1.5-final", "local_game_1.5-final_"},
		{"long names truncate to 40 chars", strings.Repeat("a", 60), "local_" + strings.Repeat("a", 40) + "_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NewID(root, filepath.Join(root, tc.base), "")
			if !strings.HasPrefix(got, tc.want) {
				t.Errorf("NewID(%q) = %q, want prefix %q", tc.base, got, tc.want)
			}
		})
	}
}

func TestCloneIsDeepCopy(t *testing.T) {
	orig := &GameInfo{
		SchemaVersion:   2,
		ID:              "steam_570",
		Title:           "Portal 2",
		TitleNative:     "ポータル2",
		Type:            "game",
		Platforms:       []PlatformInfo{{Platform: "steam", ID: "570", Name: "Steam"}},
		Aliases:         []string{"Portal²"},
		PreferredSource: "steam",
		Executables:     []Executable{{Path: "portal2.exe", Name: "Portal 2", Primary: true}},
		SavePaths:       []SavePath{{Type: "dir", Path: "saves", Source: "steam"}},
		Metadata: &Metadata{
			CoverURL:       "https://cdn.example/cover.jpg",
			CoverLandscape: "https://cdn.example/landscape.jpg",
			ReleaseDate:    "2011-04-19",
			Developer:      "Valve",
			Publisher:      "Valve",
			Tags:           []string{"puzzle"},
			Description:    "Think with portals.",
			Links:          map[string]string{"steam": "https://store.steampowered.com/app/570"},
		},
		ScannedAt:     "2026-01-01T00:00:00Z",
		TotalPlaytime: 3600,
		LastPlayedAt:  "2026-01-02T00:00:00Z",
		Starred:       true,
		Tags:          []string{"coop"},
		CoverVersion:  42,
		GameDir:       filepath.Join("C:", "lib", "Games", "Portal 2"),
	}
	before := mustJSON(t, orig)

	clone := orig.Clone()
	if clone == orig {
		t.Fatal("Clone returned the original pointer")
	}

	// Pointers and maps must not be shared at all.
	if clone.Metadata == orig.Metadata {
		t.Error("Clone shares the Metadata pointer")
	}
	if &clone.Platforms[0] == &orig.Platforms[0] {
		t.Error("Clone shares the Platforms backing array")
	}
	if &clone.Executables[0] == &orig.Executables[0] {
		t.Error("Clone shares the Executables backing array")
	}
	if &clone.SavePaths[0] == &orig.SavePaths[0] {
		t.Error("Clone shares the SavePaths backing array")
	}
	if &clone.Aliases[0] == &orig.Aliases[0] {
		t.Error("Clone shares the Aliases backing array")
	}
	if &clone.Tags[0] == &orig.Tags[0] {
		t.Error("Clone shares the Tags backing array")
	}
	if &clone.Metadata.Tags[0] == &orig.Metadata.Tags[0] {
		t.Error("Clone shares Metadata.Tags backing array")
	}

	// Mutate every field through the clone.
	clone.SchemaVersion = 99
	clone.ID = "changed"
	clone.Title = "changed"
	clone.TitleNative = "changed"
	clone.Type = "changed"
	clone.Platforms[0] = PlatformInfo{Platform: "changed", ID: "changed", Name: "changed"}
	clone.Platforms = append(clone.Platforms, PlatformInfo{Platform: "gog", ID: "1"})
	clone.Aliases[0] = "changed"
	clone.Aliases = append(clone.Aliases, "extra")
	clone.PreferredSource = "changed"
	clone.Executables[0] = Executable{Path: "changed", Name: "changed", Primary: false}
	clone.Executables = append(clone.Executables, Executable{Path: "extra.exe"})
	clone.SavePaths[0] = SavePath{Type: "changed", Path: "changed", Source: "changed"}
	clone.SavePaths = append(clone.SavePaths, SavePath{Path: "extra"})
	clone.Metadata.CoverURL = "changed"
	clone.Metadata.CoverLandscape = "changed"
	clone.Metadata.ReleaseDate = "changed"
	clone.Metadata.Developer = "changed"
	clone.Metadata.Publisher = "changed"
	clone.Metadata.Tags[0] = "changed"
	clone.Metadata.Tags = append(clone.Metadata.Tags, "extra")
	clone.Metadata.Description = "changed"
	clone.Metadata.Links["steam"] = "changed"
	clone.Metadata.Links["extra"] = "changed"
	clone.ScannedAt = "changed"
	clone.TotalPlaytime = 0
	clone.LastPlayedAt = "changed"
	clone.Starred = false
	clone.Tags[0] = "changed"
	clone.Tags = append(clone.Tags, "extra")
	clone.CoverVersion = 0
	clone.GameDir = "changed"

	if after := mustJSON(t, orig); after != before {
		t.Errorf("original changed through its clone:\nbefore: %s\nafter:  %s", before, after)
	}
	// GameDir is json:"-", so it needs its own check.
	if want := filepath.Join("C:", "lib", "Games", "Portal 2"); orig.GameDir != want {
		t.Errorf("GameDir = %q, want %q", orig.GameDir, want)
	}
}

func TestCloneNilReceiverAndNilMetadata(t *testing.T) {
	var nilInfo *GameInfo
	if got := nilInfo.Clone(); got != nil {
		t.Errorf("(*GameInfo)(nil).Clone() = %#v, want nil", got)
	}

	orig := &GameInfo{
		ID:          "local_x",
		Executables: []Executable{{Path: "a.exe"}},
		Tags:        []string{"tag"},
	}
	clone := orig.Clone()
	if clone == nil {
		t.Fatal("Clone returned nil for a non-nil receiver")
	}
	if clone.Metadata != nil {
		t.Errorf("Metadata = %#v, want nil", clone.Metadata)
	}
	clone.Executables[0].Path = "mutated"
	clone.Tags[0] = "mutated"
	if orig.Executables[0].Path != "a.exe" {
		t.Errorf("Executables[0].Path = %q, want %q", orig.Executables[0].Path, "a.exe")
	}
	if orig.Tags[0] != "tag" {
		t.Errorf("Tags[0] = %q, want %q", orig.Tags[0], "tag")
	}
}

func TestSaveLoadFromDirRoundTrip(t *testing.T) {
	root := t.TempDir()
	// Nested, not-yet-existing game dir: Save has to create the managed folder.
	gameDir := filepath.Join(root, "Games", "Portal 2")

	info := New(root, gameDir, []Executable{{Path: "portal2.exe", Name: "Portal 2", Primary: true}}, "570")
	info.Title = "Portal 2"
	info.TotalPlaytime = 3600
	info.Starred = true
	info.Tags = []string{"coop"}
	info.AddAlias("Portal²")
	info.Metadata = &Metadata{
		Developer: "Valve",
		Tags:      []string{"puzzle"},
		Links:     map[string]string{"steam": "https://store.steampowered.com/app/570"},
	}

	if err := info.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := InfoFilePath(gameDir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if _, ok := raw["gameDir"]; ok {
		t.Errorf("%s contains a gameDir key; machine-specific mount points must not be persisted: %s", path, data)
	}
	if _, ok := raw["schemaVersion"]; !ok {
		t.Errorf("%s has no schemaVersion key: %s", path, data)
	}

	loaded, err := LoadFromDir(gameDir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if loaded.GameDir != gameDir {
		t.Errorf("GameDir = %q, want the directory loaded from %q", loaded.GameDir, gameDir)
	}
	if loaded.ID != info.ID || loaded.ID != "steam_570" {
		t.Errorf("ID = %q, want %q", loaded.ID, "steam_570")
	}
	if loaded.Title != "Portal 2" {
		t.Errorf("Title = %q, want %q", loaded.Title, "Portal 2")
	}
	if loaded.TotalPlaytime != 3600 || !loaded.Starred {
		t.Errorf("playtime/starred = %d/%v, want 3600/true", loaded.TotalPlaytime, loaded.Starred)
	}
	if !reflect.DeepEqual(loaded.Tags, []string{"coop"}) {
		t.Errorf("Tags = %#v, want [coop]", loaded.Tags)
	}
	if !reflect.DeepEqual(loaded.Aliases, []string{"Portal²"}) {
		t.Errorf("Aliases = %#v, want [Portal²]", loaded.Aliases)
	}
	if !reflect.DeepEqual(loaded.Metadata, info.Metadata) {
		t.Errorf("Metadata = %#v, want %#v", loaded.Metadata, info.Metadata)
	}
	if !reflect.DeepEqual(loaded.Platforms, info.Platforms) {
		t.Errorf("Platforms = %#v, want %#v", loaded.Platforms, info.Platforms)
	}
	if !reflect.DeepEqual(loaded.Executables, info.Executables) {
		t.Errorf("Executables = %#v, want %#v", loaded.Executables, info.Executables)
	}
}

func TestSaveRequiresGameDir(t *testing.T) {
	info := &GameInfo{ID: "local_x"}
	if err := info.Save(); err == nil {
		t.Error("Save with an empty GameDir must fail")
	}
}

// TestLoadFromDirMigratesLegacyFile covers the pre-0.6 layout: flat
// platform/platformId keys in ".gameinfo.json" at the game root, no platforms
// array and no schemaVersion. The record has to be rewritten into the managed
// folder and the legacy file removed.
func TestLoadFromDirMigratesLegacyFile(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "Old Game")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// No "id" (the migration has to derive one from the app id) and no "type"
	// (decode defaults it to "game"). The platform id carries a BOM and padding,
	// as Windows tooling writes it.
	legacy := `{
  "title": "Old Game",
  "platform": "steam",
  "platformId": "\uFEFF 12345 ",
  "scannedAt": "2026-01-01T00:00:00Z",
  "totalPlaytime": 0
}`
	legacyPath := LegacyInfoPath(gameDir)
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	loaded, err := LoadFromDir(gameDir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if !loaded.HasPlatform("steam") {
		t.Errorf("Platforms = %#v, want the flat steam platform migrated", loaded.Platforms)
	}
	if got := loaded.PlatformID("steam"); got != "12345" {
		t.Errorf("PlatformID(steam) = %q, want %q (BOM and padding stripped)", got, "12345")
	}
	if loaded.PreferredSource != "steam" {
		t.Errorf("PreferredSource = %q, want %q", loaded.PreferredSource, "steam")
	}
	if loaded.Title != "Old Game" {
		t.Errorf("Title = %q, want %q", loaded.Title, "Old Game")
	}
	if loaded.Type != "game" {
		t.Errorf("Type = %q, want %q", loaded.Type, "game")
	}
	if loaded.SchemaVersion == 0 {
		t.Error("SchemaVersion = 0, want a versioned record after migration")
	}
	if loaded.ID != "steam_12345" {
		t.Errorf("ID = %q, want %q derived from the migrated steam id", loaded.ID, "steam_12345")
	}
	if loaded.GameDir != gameDir {
		t.Errorf("GameDir = %q, want %q", loaded.GameDir, gameDir)
	}

	data, err := os.ReadFile(InfoFilePath(gameDir))
	if err != nil {
		t.Fatalf("record was not rewritten to %s: %v", InfoFilePath(gameDir), err)
	}
	if !strings.Contains(string(data), `"platforms"`) {
		t.Errorf("rewritten record has no platforms array: %s", data)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy file still present at %s (stat err = %v)", legacyPath, err)
	}

	// A second load must come from the managed file, not the legacy one.
	again, err := LoadFromDir(gameDir)
	if err != nil {
		t.Fatalf("second LoadFromDir: %v", err)
	}
	if again.ID != loaded.ID || !again.HasPlatform("steam") {
		t.Errorf("reloaded record = %#v, want the migrated one", again)
	}
}

// TestLoadFromDirRejectsCorruptRecords pins the failure behaviour: a
// half-written record must surface as an error instead of being silently
// replaced by an empty one, and a failed legacy migration must not destroy or
// half-apply the legacy file.
func TestLoadFromDirRejectsCorruptRecords(t *testing.T) {
	t.Run("managed record", func(t *testing.T) {
		gameDir := filepath.Join(t.TempDir(), "Broken")
		if err := os.MkdirAll(ManagerDir(gameDir), 0o755); err != nil {
			t.Fatalf("mkdir manager dir: %v", err)
		}
		if err := os.WriteFile(InfoFilePath(gameDir), []byte(`{ "id": "steam_570", `), 0o644); err != nil {
			t.Fatalf("write record: %v", err)
		}
		if _, err := LoadFromDir(gameDir); err == nil {
			t.Error("LoadFromDir accepted a truncated gameinfo.json, want an error")
		}
	})

	t.Run("legacy record", func(t *testing.T) {
		gameDir := filepath.Join(t.TempDir(), "Broken")
		if err := os.MkdirAll(gameDir, 0o755); err != nil {
			t.Fatalf("mkdir game dir: %v", err)
		}
		if err := os.WriteFile(LegacyInfoPath(gameDir), []byte(`{ "title": "Broken", `), 0o644); err != nil {
			t.Fatalf("write legacy record: %v", err)
		}
		if _, err := LoadFromDir(gameDir); err == nil {
			t.Error("LoadFromDir accepted a truncated legacy record, want an error")
		}
		if _, err := os.Stat(LegacyInfoPath(gameDir)); err != nil {
			t.Errorf("failed migration destroyed the legacy file: %v", err)
		}
		if _, err := os.Stat(InfoFilePath(gameDir)); !os.IsNotExist(err) {
			t.Errorf("failed migration wrote a managed record (stat err = %v)", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		if _, err := LoadFromDir(filepath.Join(t.TempDir(), "Absent")); !os.IsNotExist(err) {
			t.Errorf("LoadFromDir(missing) error = %v, want os.ErrNotExist", err)
		}
	})
}

// TestMigrateLegacyCovers covers the legacy cover move, which happens as part of
// the same migration.
//
// The destination directory is created up front deliberately: migrateLegacyCovers
// renames with os.Rename and never creates covers/, and it runs before the first
// Save() (the call that creates .gamemanager). In a genuine pre-0.6 game folder
// covers/ therefore does not exist yet, os.Rename fails with "the system cannot
// find the path specified", and the cover silently stays in the game root. That
// failure is a reported production bug (not fixed here); this test pins the
// intended behaviour for the case where the directory is already present.
func TestMigrateLegacyCovers(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "Old Game")
	if err := os.MkdirAll(CoverDir(gameDir), 0o755); err != nil {
		t.Fatalf("mkdir covers: %v", err)
	}
	legacy := `{"title":"Old Game","platform":"steam","platformId":"12345"}`
	if err := os.WriteFile(LegacyInfoPath(gameDir), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	names := []string{CoverName + ".jpg", CoverLandscapeName + ".png"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(gameDir, name), []byte("img"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if _, err := LoadFromDir(gameDir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	for _, name := range names {
		if _, err := os.Stat(filepath.Join(CoverDir(gameDir), name)); err != nil {
			t.Errorf("%s was not moved into %s: %v", name, CoverDir(gameDir), err)
		}
		if _, err := os.Stat(filepath.Join(gameDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s is still in the game root (stat err = %v)", name, err)
		}
	}
}

// TestPrimaryPlatformConsistentWithID is the regression test for the primary
// platform mismatch: PrimaryPlatform looked at PreferredSource while
// PrimaryPlatformID used Platforms[0].ID. With VNDB listed first and steam
// preferred, a caller asking "which platform is primary?" was told "steam" and
// then handed the VNDB id ("v123") by PrimaryPlatformID, so Steam-bound callers
// (ACF lookups, store links) silently operated on the wrong id. Both accessors
// now derive from one lookup and must always describe the same entry.
func TestPrimaryPlatformConsistentWithID(t *testing.T) {
	info := &GameInfo{
		Platforms: []PlatformInfo{
			{Platform: "vndb", ID: "v123"},
			{Platform: "steam", ID: "570"},
		},
		PreferredSource: "steam",
	}

	if got := info.PrimaryPlatform(); got != "steam" {
		t.Errorf("PrimaryPlatform() = %q, want %q", got, "steam")
	}
	if got := info.PrimaryPlatformID(); got != "570" {
		t.Errorf("PrimaryPlatformID() = %q, want %q (must come from the same entry as PrimaryPlatform)", got, "570")
	}
	if p, ok := info.PreferredPlatformInfo(); !ok || p.Platform != "steam" || p.ID != "570" {
		t.Errorf("PreferredPlatformInfo() = %#v, %v, want the steam/570 entry", p, ok)
	}

	// A preferred source that is not linked falls back to the first named entry,
	// and both accessors must still agree.
	info.PreferredSource = "gog"
	if got := info.PrimaryPlatform(); got != "vndb" {
		t.Errorf("fallback PrimaryPlatform() = %q, want %q", got, "vndb")
	}
	if got := info.PrimaryPlatformID(); got != "v123" {
		t.Errorf("fallback PrimaryPlatformID() = %q, want %q", got, "v123")
	}

	// Entries without a platform name never become primary.
	info.Platforms = append([]PlatformInfo{{Platform: "", ID: "orphan"}}, info.Platforms...)
	if got := info.PrimaryPlatform(); got != "vndb" {
		t.Errorf("PrimaryPlatform() = %q, want %q (unnamed entry skipped)", got, "vndb")
	}

	empty := &GameInfo{PreferredSource: "steam"}
	if got := empty.PrimaryPlatform(); got != "" {
		t.Errorf("PrimaryPlatform() = %q, want empty for no platforms", got)
	}
	if got := empty.PrimaryPlatformID(); got != "" {
		t.Errorf("PrimaryPlatformID() = %q, want empty for no platforms", got)
	}
}

func TestSetPlatform(t *testing.T) {
	info := &GameInfo{}

	info.SetPlatform("vndb", "v123", "VNDB title")
	if len(info.Platforms) != 1 {
		t.Fatalf("Platforms = %#v, want one inserted entry", info.Platforms)
	}
	if info.Platforms[0] != (PlatformInfo{Platform: "vndb", ID: "v123", Name: "VNDB title"}) {
		t.Errorf("Platforms[0] = %#v, want vndb/v123/VNDB title", info.Platforms[0])
	}
	if info.PreferredSource != "vndb" {
		t.Errorf("PreferredSource = %q, want %q (first platform becomes preferred)", info.PreferredSource, "vndb")
	}

	// Updating must not insert a second entry, and empty arguments must not wipe
	// stored values.
	info.SetPlatform("vndb", "v999", "")
	if len(info.Platforms) != 1 {
		t.Fatalf("Platforms = %#v, want the existing entry updated in place", info.Platforms)
	}
	if info.Platforms[0].ID != "v999" || info.Platforms[0].Name != "VNDB title" {
		t.Errorf("Platforms[0] = %#v, want id v999 and the name preserved", info.Platforms[0])
	}
	info.SetPlatform("vndb", "", "Renamed")
	if len(info.Platforms) != 1 || info.Platforms[0].ID != "v999" || info.Platforms[0].Name != "Renamed" {
		t.Errorf("Platforms = %#v, want name updated and id preserved", info.Platforms)
	}

	info.SetPlatform("", "ignored", "ignored")
	if len(info.Platforms) != 1 {
		t.Errorf("Platforms = %#v, want an empty platform name ignored", info.Platforms)
	}

	// A second platform is added but must not steal the preference.
	info.SetPlatform("steam", "570", "Steam")
	if len(info.Platforms) != 2 {
		t.Fatalf("Platforms = %#v, want the steam entry appended", info.Platforms)
	}
	if info.PreferredSource != "vndb" {
		t.Errorf("PreferredSource = %q, want the existing preference %q kept", info.PreferredSource, "vndb")
	}

	// With no preference recorded, the next inserted platform takes it.
	info.PreferredSource = ""
	info.SetPlatform("gog", "1", "")
	if info.PreferredSource != "gog" {
		t.Errorf("PreferredSource = %q, want %q", info.PreferredSource, "gog")
	}
	if len(info.Platforms) != 3 {
		t.Errorf("Platforms = %#v, want three entries", info.Platforms)
	}
}

func TestAddAlias(t *testing.T) {
	info := &GameInfo{Title: "Portal 2"}

	info.AddAlias("")
	info.AddAlias("   ")
	info.AddAlias("Portal 2")
	if len(info.Aliases) != 0 {
		t.Fatalf("Aliases = %#v, want blanks and the current title ignored", info.Aliases)
	}
	// Note: the title guard above is a plain == comparison, so a differently
	// cased spelling of the title is not caught by it even though the duplicate
	// check below is case-insensitive. That inconsistency is reported rather than
	// pinned here.

	info.AddAlias("  Portal²  ")
	if !reflect.DeepEqual(info.Aliases, []string{"Portal²"}) {
		t.Fatalf("Aliases = %#v, want [Portal²] trimmed", info.Aliases)
	}
	info.AddAlias("PORTAL²")
	if len(info.Aliases) != 1 {
		t.Errorf("Aliases = %#v, want case-insensitive duplicates ignored", info.Aliases)
	}
	info.AddAlias("Another")
	if !reflect.DeepEqual(info.Aliases, []string{"Portal²", "Another"}) {
		t.Errorf("Aliases = %#v, want [Portal² Another]", info.Aliases)
	}
}

func TestTags(t *testing.T) {
	info := &GameInfo{}

	if !info.AddTag("  Action  ") {
		t.Error("AddTag(Action) = false, want true")
	}
	if !reflect.DeepEqual(info.Tags, []string{"Action"}) {
		t.Fatalf("Tags = %#v, want [Action] trimmed", info.Tags)
	}
	if info.AddTag("action") {
		t.Error("AddTag(action) = true, want false for a case-insensitive duplicate")
	}
	if info.AddTag("") || info.AddTag("   ") {
		t.Error("AddTag(blank) = true, want false")
	}
	if !info.AddTag("rpg") {
		t.Error("AddTag(rpg) = false, want true")
	}
	if len(info.Tags) != 2 {
		t.Errorf("Tags = %#v, want two entries", info.Tags)
	}

	if !info.HasTag("RPG") || !info.HasTag("action") {
		t.Errorf("HasTag is not case-insensitive: %#v", info.Tags)
	}
	if info.HasTag("missing") || info.HasTag("") {
		t.Errorf("HasTag reported a tag that is not present: %#v", info.Tags)
	}

	if !info.RemoveTag("ACTION") {
		t.Error("RemoveTag(ACTION) = false, want true")
	}
	if !reflect.DeepEqual(info.Tags, []string{"rpg"}) {
		t.Errorf("Tags = %#v, want [rpg]", info.Tags)
	}
	if info.RemoveTag("action") {
		t.Error("RemoveTag on a missing tag = true, want false")
	}
	if info.RemoveTag("") {
		t.Error("RemoveTag(blank) = true, want false")
	}
}

func TestPrimaryExecutable(t *testing.T) {
	empty := &GameInfo{}
	if got := empty.PrimaryExecutable(); got != nil {
		t.Errorf("PrimaryExecutable() = %#v, want nil without executables", got)
	}

	none := &GameInfo{Executables: []Executable{
		{Path: "a.exe", Name: "A"},
		{Path: "b.exe", Name: "B"},
	}}
	if got := none.PrimaryExecutable(); got == nil || got.Path != "a.exe" {
		t.Errorf("PrimaryExecutable() = %#v, want the first entry a.exe", got)
	}

	flagged := &GameInfo{Executables: []Executable{
		{Path: "a.exe", Name: "A"},
		{Path: "b.exe", Name: "B", Primary: true},
		{Path: "c.exe", Name: "C"},
	}}
	if got := flagged.PrimaryExecutable(); got == nil || got.Path != "b.exe" || !got.Primary {
		t.Errorf("PrimaryExecutable() = %#v, want the flagged b.exe", got)
	}
}

func TestSortByRecentPlay(t *testing.T) {
	games := []*GameInfo{
		{ID: "never2", Title: "banana"},
		{ID: "old", Title: "cherry", LastPlayedAt: "2026-01-01T00:00:00Z"},
		{ID: "new", Title: "apple", LastPlayedAt: "2026-06-01T00:00:00Z"},
		{ID: "never1", Title: "Apple"},
		{ID: "tie", Title: "aardvark", LastPlayedAt: "2026-06-01T00:00:00Z"},
	}

	SortByRecentPlay(games)

	var got []string
	for _, g := range games {
		got = append(got, g.ID)
	}
	// Played first (most recent first, ties broken case-insensitively by title),
	// then never-played in title order.
	want := []string{"tie", "new", "old", "never1", "never2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestMarkCoverUpdated(t *testing.T) {
	before := time.Now().Unix()

	info := &GameInfo{ID: "steam_570"}
	info.MarkCoverUpdated()

	if info.CoverVersion == 0 {
		t.Fatal("CoverVersion = 0, want a non-zero version after a cover update")
	}
	if info.CoverVersion < before || info.CoverVersion > time.Now().Unix()+1 {
		t.Errorf("CoverVersion = %d, want a current unix timestamp (around %d)", info.CoverVersion, before)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	gameDir := t.TempDir()
	info := &GameInfo{
		ID:      "steam_570",
		GameDir: gameDir,
		Metadata: &Metadata{
			Developer: "Valve",
			Tags:      []string{"puzzle", "coop"},
			Links:     map[string]string{"steam": "https://store.steampowered.com/app/570"},
		},
	}

	if got := info.LoadMeta("vndb"); got != nil {
		t.Errorf("LoadMeta(missing source) = %#v, want nil", got)
	}

	if err := info.SaveMeta("vndb"); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	path := filepath.Join(MetaDir(gameDir), "vndb.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("SaveMeta did not write %s: %v", path, err)
	}
	got := info.LoadMeta("vndb")
	if !reflect.DeepEqual(got, info.Metadata) {
		t.Errorf("LoadMeta = %#v, want %#v", got, info.Metadata)
	}

	// A corrupt snapshot reads as absent rather than as an error.
	if err := os.WriteFile(filepath.Join(MetaDir(gameDir), "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt meta: %v", err)
	}
	if got := info.LoadMeta("broken"); got != nil {
		t.Errorf("LoadMeta(corrupt) = %#v, want nil", got)
	}

	// No-op cases must not create files.
	if err := info.SaveMeta(""); err != nil {
		t.Errorf("SaveMeta(empty source) = %v, want nil", err)
	}
	noMeta := &GameInfo{ID: "steam_570", GameDir: gameDir}
	if err := noMeta.SaveMeta("gog"); err != nil {
		t.Errorf("SaveMeta(nil Metadata) = %v, want nil", err)
	}
	for _, name := range []string{".json", "gog.json"} {
		if _, err := os.Stat(filepath.Join(MetaDir(gameDir), name)); !os.IsNotExist(err) {
			t.Errorf("SaveMeta wrote %s (stat err = %v), want nothing", name, err)
		}
	}
}

func TestPlatformLookups(t *testing.T) {
	info := &GameInfo{Platforms: []PlatformInfo{
		{Platform: "vndb", ID: "v123"},
		{Platform: "steam", ID: "570"},
		{Platform: "dlsite"},
	}}

	if !info.HasPlatform("steam") || info.HasPlatform("gog") {
		t.Errorf("HasPlatform is wrong for %#v", info.Platforms)
	}
	if got := info.PlatformID("steam"); got != "570" {
		t.Errorf("PlatformID(steam) = %q, want %q", got, "570")
	}
	if got := info.PlatformID("gog"); got != "" {
		t.Errorf("PlatformID(gog) = %q, want empty", got)
	}
	if got := info.PlatformIDs(); !reflect.DeepEqual(got, []string{"v123", "570"}) {
		t.Errorf("PlatformIDs() = %#v, want only the non-empty ids", got)
	}
}

func TestPathHelpers(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "Games", "Portal 2")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ManagerDir", ManagerDir(gameDir), filepath.Join(gameDir, ".gamemanager")},
		{"CoverDir", CoverDir(gameDir), filepath.Join(gameDir, ".gamemanager", "covers")},
		{"MetaDir", MetaDir(gameDir), filepath.Join(gameDir, ".gamemanager", "meta")},
		{"InfoFilePath", InfoFilePath(gameDir), filepath.Join(gameDir, ".gamemanager", "gameinfo.json")},
		{"LegacyInfoPath", LegacyInfoPath(gameDir), filepath.Join(gameDir, ".gameinfo.json")},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
		if !strings.HasPrefix(tc.got, gameDir+string(filepath.Separator)) {
			t.Errorf("%s = %q, want a path under %q", tc.name, tc.got, gameDir)
		}
	}

	if got := ManagerDirName; got != ".gamemanager" {
		t.Errorf("ManagerDirName = %q, want %q", got, ".gamemanager")
	}

	info := &GameInfo{GameDir: gameDir}
	if got := info.InfoFilePath(); got != InfoFilePath(gameDir) {
		t.Errorf("(*GameInfo).InfoFilePath() = %q, want %q", got, InfoFilePath(gameDir))
	}
}
