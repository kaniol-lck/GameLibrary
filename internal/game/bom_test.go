package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadToleratesByteOrderMark is a regression test: a metadata file carrying a
// BOM used to fail to parse entirely, because encoding/json does not skip one.
// The same class of prefix is already handled for steam_appid.txt, and a user
// editing gameinfo.json in Notepad hits this immediately.
func TestLoadToleratesByteOrderMark(t *testing.T) {
	body, err := json.MarshalIndent(&GameInfo{
		SchemaVersion: SchemaVersion,
		ID:            "steam_570",
		Title:         "Dota 2",
		Type:          "game",
		Executables:   []Executable{{Path: "game.exe", Name: "game", Primary: true}},
		Platforms:     []PlatformInfo{{Platform: "steam", ID: "570"}},
		ScannedAt:     "2026-01-01T00:00:00Z",
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	for name, prefix := range map[string][]byte{
		"utf8": {0xEF, 0xBB, 0xBF},
		"none": nil,
	} {
		t.Run(name, func(t *testing.T) {
			gameDir := t.TempDir()
			if err := os.MkdirAll(ManagerDir(gameDir), 0o755); err != nil {
				t.Fatal(err)
			}
			path := InfoFilePath(gameDir)
			if err := os.WriteFile(path, append(append([]byte{}, prefix...), body...), 0o644); err != nil {
				t.Fatal(err)
			}

			info, err := LoadFromDir(gameDir)
			if err != nil {
				t.Fatalf("LoadFromDir: %v", err)
			}
			if info.ID != "steam_570" {
				t.Errorf("unexpected id %q", info.ID)
			}
			if info.Title != "Dota 2" {
				t.Errorf("unexpected title %q", info.Title)
			}
			if info.PrimaryPlatformID() != "570" {
				t.Errorf("unexpected platform id %q", info.PrimaryPlatformID())
			}
			if !filepath.IsAbs(info.GameDir) {
				t.Errorf("expected the directory to be filled in, got %q", info.GameDir)
			}
		})
	}
}
