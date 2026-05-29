package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()

	cfg := Default()
	cfg.MachineID = "test-123"
	cfg.GameDirectories = []string{".\\Games", ".\\MoreGames"}
	cfg.MaxScanDepth = 5
	cfg.Sources[1].Enabled = false

	err := cfg.Save(dir)
	if err != nil {
		t.Fatalf("Save failed: %s", err)
	}

	if !fileExists(filepath.Join(dir, "config.json")) {
		t.Fatal("config.json not created")
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %s", err)
	}
	if loaded.MachineID != "test-123" {
		t.Errorf("MachineID mismatch: %s", loaded.MachineID)
	}
	if len(loaded.GameDirectories) != 2 {
		t.Errorf("GameDirectories count mismatch: %d", len(loaded.GameDirectories))
	}
	if loaded.MaxScanDepth != 5 {
		t.Errorf("MaxScanDepth mismatch: %d", loaded.MaxScanDepth)
	}
	if len(loaded.Sources) != 4 {
		t.Errorf("Sources count mismatch: %d", len(loaded.Sources))
	}
	if loaded.Sources[1].Enabled {
		t.Error("VNDB should be disabled")
	}
}

func TestDefaultSources(t *testing.T) {
	cfg := Default()
	if len(cfg.Sources) != 4 {
		t.Fatalf("expected 4 sources, got %d", len(cfg.Sources))
	}
	if cfg.Sources[0].Key != "steam" {
		t.Errorf("first source should be steam, got %s", cfg.Sources[0].Key)
	}
	if !cfg.Sources[0].Enabled {
		t.Error("steam should be enabled by default")
	}
}

func TestLegacyMigration(t *testing.T) {
	dir := t.TempDir()

	legacy := map[string]interface{}{
		"machineId":       "legacy-test",
		"machineName":     "Legacy Machine",
		"gameDirectories": []string{".\\OldGames"},
		"maxScanDepth":    2,
		"language":        "en-US",
		"vndbEnabled":     true,
		"dlsiteEnabled":   false,
	}
	data, _ := json.MarshalIndent(legacy, "", "  ")
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load legacy config failed: %s", err)
	}
	if loaded.MachineID != "legacy-test" {
		t.Errorf("MachineID: %s", loaded.MachineID)
	}
	if len(loaded.Sources) != 4 {
		t.Errorf("expected 4 migrated sources, got %d", len(loaded.Sources))
	}
	if !loaded.Sources[1].Enabled {
		t.Error("vndb should be enabled from legacy")
	}
	if loaded.Sources[2].Enabled {
		t.Error("dlsite should be disabled from legacy")
	}
}

func TestJSONRoundtrip(t *testing.T) {
	cfg := Default()
	cfg.MachineID = "json-test"
	cfg.SteamAPIKey = "secret-key-123"
	cfg.Sources = []MetadataSource{
		{Key: "steam", Name: "Steam", Enabled: true},
		{Key: "vndb", Name: "VNDB", Enabled: false},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %s", err)
	}

	var decoded Config
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("unmarshal: %s", err)
	}

	if decoded.MachineID != "json-test" {
		t.Errorf("MachineID: %s", decoded.MachineID)
	}
	if decoded.SteamAPIKey != "secret-key-123" {
		t.Errorf("SteamAPIKey not preserved")
	}
	if len(decoded.Sources) != 2 {
		t.Errorf("Sources count: %d", len(decoded.Sources))
	}
	if decoded.Sources[1].Enabled {
		t.Error("VNDB should be disabled")
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.MaxScanDepth != 3 {
		t.Errorf("expected MaxScanDepth 3, got %d", cfg.MaxScanDepth)
	}
	if cfg.Language != "zh-CN" {
		t.Errorf("expected Language zh-CN, got %s", cfg.Language)
	}
	if len(cfg.GameDirectories) != 1 {
		t.Errorf("expected 1 GameDirectory, got %d", len(cfg.GameDirectories))
	}
}

func TestAutoCreate(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %s", err)
	}
	if cfg.MachineID == "" {
		t.Error("MachineID should be auto-generated")
	}
	if !fileExists(filepath.Join(dir, "config.json")) {
		t.Error("config.json should be auto-created")
	}
}

func TestSourceReorder(t *testing.T) {
	cfg := Default()
	cfg.Sources = []MetadataSource{
		{Key: "steam", Name: "Steam", Enabled: true},
		{Key: "dlsite", Name: "DLsite", Enabled: true},
		{Key: "vndb", Name: "VNDB", Enabled: false},
		{Key: "igdb", Name: "IGDB", Enabled: true},
	}

	if cfg.Sources[1].Key != "dlsite" {
		t.Error("dlsite should be at index 1 after reorder")
	}
	if cfg.Sources[2].Key != "vndb" {
		t.Error("vndb should be at index 2 after reorder")
	}
}
