package game

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()

	info := New(dir, []Executable{
		{Path: "test.exe", Name: "Test", Primary: true},
	}, "999999")

	info.Title = "Test Game"
	info.Metadata = &Metadata{
		Developer: "TestDev",
		Tags:      []string{"Action", "RPG"},
	}
	info.SetPlatform("dlsite", "RJ123456", "Test Game DLsite")

	err := info.Save()
	if err != nil {
		t.Fatalf("Save failed: %s", err)
	}

	loaded, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %s", err)
	}
	if loaded.ID != "steam_999999" {
		t.Errorf("ID mismatch: %s", loaded.ID)
	}
	if loaded.Title != "Test Game" {
		t.Errorf("Title mismatch: %s", loaded.Title)
	}
	if loaded.Metadata == nil || loaded.Metadata.Developer != "TestDev" {
		t.Error("Metadata mismatch")
	}
	if len(loaded.Platforms) != 2 {
		t.Errorf("expected 2 platforms, got %d", len(loaded.Platforms))
	}
	if !loaded.HasPlatform("dlsite") {
		t.Error("expected dlsite platform")
	}

	_, err = LoadFromDir(filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestNewLocal(t *testing.T) {
	info := New("/games/MyGame", []Executable{
		{Path: "start.exe", Name: "start", Primary: true},
	}, "")

	if info.ID != "MyGame" {
		t.Errorf("expected ID MyGame, got %s", info.ID)
	}
	if info.PrimaryPlatform() != "local" {
		t.Errorf("expected platform local, got %s", info.PrimaryPlatform())
	}
	if info.PrimaryPlatformID() != "" {
		t.Errorf("expected empty primary platformId, got %s", info.PrimaryPlatformID())
	}
}

func TestNewSteam(t *testing.T) {
	info := New("/games/TestGame", []Executable{
		{Path: "game.exe", Name: "game", Primary: true},
	}, "123456")

	if info.ID != "steam_123456" {
		t.Errorf("expected ID steam_123456, got %s", info.ID)
	}
	if info.PrimaryPlatform() != "steam" {
		t.Errorf("expected platform steam, got %s", info.PrimaryPlatform())
	}
	if info.PrimaryPlatformID() != "123456" {
		t.Errorf("expected platformId 123456, got %s", info.PrimaryPlatformID())
	}
}

func TestPlatformMigration(t *testing.T) {
	dir := t.TempDir()

	oldJSON := `{"id":"steam_12345","title":"Old Game","platform":"steam","platformId":"12345","type":"game","scannedAt":"2026-01-01T00:00:00Z","totalPlaytime":0}`
	os.WriteFile(filepath.Join(dir, ".gameinfo.json"), []byte(oldJSON), 0644)

	loaded, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %s", err)
	}
	if !loaded.HasPlatform("steam") {
		t.Error("expected steam platform after migration")
	}
	if loaded.PrimaryPlatformID() != "12345" {
		t.Errorf("expected platformId 12345 after migration, got %s", loaded.PrimaryPlatformID())
	}
}

func TestSetPlatformAndAliases(t *testing.T) {
	info := New("/tmp/test", []Executable{}, "12345")
	info.Title = "Steam Title"

	info.SetPlatform("steam", "12345", "Steam Title")
	info.AddAlias("ALT Title")
	info.AddAlias("ALT Title")
	info.AddAlias("Steam Title")

	if !info.HasPlatform("steam") {
		t.Error("expected steam platform")
	}
	if info.Platforms[0].Name != "Steam Title" {
		t.Errorf("expected platform name Steam Title, got %s", info.Platforms[0].Name)
	}
	if len(info.Aliases) != 1 {
		t.Errorf("expected 1 alias (duplicates filtered, title filtered), got %d: %v", len(info.Aliases), info.Aliases)
	}
	if info.Aliases[0] != "ALT Title" {
		t.Errorf("expected ALT Title, got %s", info.Aliases[0])
	}
}
