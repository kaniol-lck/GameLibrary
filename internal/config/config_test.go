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
}

func TestJSONRoundtrip(t *testing.T) {
	cfg := Default()
	cfg.MachineID = "json-test"
	cfg.SteamAPIKey = "secret-key-123"
	cfg.VNDBEnabled = true
	cfg.DLsiteEnabled = false

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
	if !decoded.VNDBEnabled {
		t.Error("VNDBEnabled should be true")
	}
	if decoded.DLsiteEnabled {
		t.Error("DLsiteEnabled should be false")
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
