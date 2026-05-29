package game

import (
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
	if info.Platform != "local" {
		t.Errorf("expected platform local, got %s", info.Platform)
	}
	if info.PlatformID != "" {
		t.Errorf("expected empty platformId, got %s", info.PlatformID)
	}
}

func TestNewSteam(t *testing.T) {
	info := New("/games/TestGame", []Executable{
		{Path: "game.exe", Name: "game", Primary: true},
	}, "123456")

	if info.ID != "steam_123456" {
		t.Errorf("expected ID steam_123456, got %s", info.ID)
	}
	if info.Platform != "steam" {
		t.Errorf("expected platform steam, got %s", info.Platform)
	}
	if info.PlatformID != "123456" {
		t.Errorf("expected platformId 123456, got %s", info.PlatformID)
	}
}
