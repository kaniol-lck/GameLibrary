package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "testdata")
}

func testConfig() *Config {
	return &Config{
		MachineID:       "test-machine",
		MachineName:     "Test",
		GameDirectories: []string{".\\testdata"},
		MaxScanDepth:    4,
		Language:        "zh-CN",
	}
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cleanupGameInfo(dir string) {
	path := filepath.Join(dir, ".gameinfo.json")
	os.Remove(path)
}

func findResult(results []ScanResult, gameDir string) *ScanResult {
	for i := range results {
		if strings.HasSuffix(results[i].GameDir, gameDir) {
			return &results[i]
		}
	}
	return nil
}

func TestScanSimpleSteamGame(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "simple_steam_game")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "simple_steam_game")
	if r == nil {
		t.Fatal("simple_steam_game not found in scan results")
	}
	if r.Error != "" {
		t.Fatalf("unexpected error: %s", r.Error)
	}
	if !r.IsNew {
		t.Error("expected IsNew to be true")
	}

	info := r.GameInfo
	if info == nil {
		t.Fatal("GameInfo is nil")
	}
	if info.ID != "steam_123456" {
		t.Errorf("expected ID steam_123456, got %s", info.ID)
	}
	if info.Platform != "steam" {
		t.Errorf("expected platform steam, got %s", info.Platform)
	}
	if info.PlatformID != "123456" {
		t.Errorf("expected platformId 123456, got %s", info.PlatformID)
	}
	if len(info.Executables) != 1 {
		t.Errorf("expected 1 executable, got %d", len(info.Executables))
	}
	if info.Executables[0].Path != "game.exe" {
		t.Errorf("expected game.exe, got %s", info.Executables[0].Path)
	}
	if !info.Executables[0].Primary {
		t.Error("expected game.exe to be primary")
	}

	if !fileExists(filepath.Join(gameDir, ".gameinfo.json")) {
		t.Error(".gameinfo.json not created")
	}
}

func TestScanDeepExeGame(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "deep_exe_game", "bin", "x64")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, filepath.Join("deep_exe_game", "bin", "x64"))
	if r == nil {
		t.Fatal("deep_exe_game/bin/x64 not found")
	}
	if r.Error != "" {
		t.Fatalf("unexpected error: %s", r.Error)
	}

	info := r.GameInfo
	if info.ID != "steam_789012" {
		t.Errorf("expected ID steam_789012, got %s", info.ID)
	}
	if info.Platform != "steam" {
		t.Errorf("expected platform steam, got %s", info.Platform)
	}
	if len(info.Executables) != 1 {
		t.Errorf("expected 1 executable, got %d", len(info.Executables))
	}
	if info.Executables[0].Path != "launcher.exe" {
		t.Errorf("expected launcher.exe, got %s", info.Executables[0].Path)
	}
}

func TestScanMultiExeGame(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "multi_exe_game")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "multi_exe_game")
	if r == nil {
		t.Fatal("multi_exe_game not found")
	}

	info := r.GameInfo
	if len(info.Executables) != 3 {
		t.Errorf("expected 3 executables (unins filtered), got %d", len(info.Executables))
	}

	hasGame := false
	hasLauncher := false
	hasConfig := false
	for _, exe := range info.Executables {
		switch exe.Path {
		case "game.exe":
			hasGame = true
			if !exe.Primary {
				t.Error("expected game.exe to be primary (keyword 'game')")
			}
		case "launcher.exe":
			hasLauncher = true
		case "config.exe":
			hasConfig = true
		}
	}
	if !hasGame || !hasLauncher || !hasConfig {
		t.Error("missing expected executables")
	}
}

func TestScanLocalGame(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "local_game")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "local_game")
	if r == nil {
		t.Fatal("local_game not found")
	}

	info := r.GameInfo
	if info.ID != "local_game" {
		t.Errorf("expected ID local_game, got %s", info.ID)
	}
	if info.Platform != "local" {
		t.Errorf("expected platform local, got %s", info.Platform)
	}
	if info.PlatformID != "" {
		t.Errorf("expected empty platformId, got %s", info.PlatformID)
	}
	if info.Executables[0].Path != "start.exe" {
		t.Errorf("expected start.exe, got %s", info.Executables[0].Path)
	}
}

func TestScanVisualNovel(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "visual_novel")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "visual_novel")
	if r == nil {
		t.Fatal("visual_novel not found")
	}

	info := r.GameInfo
	if info.ID != "steam_222222" {
		t.Errorf("expected ID steam_222222, got %s", info.ID)
	}
	if info.Executables[0].Path != "vn.exe" {
		t.Errorf("expected vn.exe, got %s", info.Executables[0].Path)
	}
}

func TestScanCollection(t *testing.T) {
	testDir := testdataDir(t)
	sub1Dir := filepath.Join(testDir, "collection", "SubGame1")
	sub2Dir := filepath.Join(testDir, "collection", "SubGame2")
	defer cleanupGameInfo(sub1Dir)
	defer cleanupGameInfo(sub2Dir)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r1 := findResult(results, filepath.Join("collection", "SubGame1"))
	if r1 == nil {
		t.Fatal("SubGame1 not found")
	}
	if r1.GameInfo.ID != "steam_333333" {
		t.Errorf("SubGame1 expected steam_333333, got %s", r1.GameInfo.ID)
	}

	r2 := findResult(results, filepath.Join("collection", "SubGame2"))
	if r2 == nil {
		t.Fatal("SubGame2 not found")
	}
	if r2.GameInfo.ID != "steam_444444" {
		t.Errorf("SubGame2 expected steam_444444, got %s", r2.GameInfo.ID)
	}
}

func TestScanAlreadyScanned(t *testing.T) {
	testDir := testdataDir(t)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "already_scanned")
	if r == nil {
		t.Fatal("already_scanned not found")
	}

	if r.IsNew {
		t.Error("expected IsNew to be false for already scanned game")
	}

	info := r.GameInfo
	if info.Title != "Already Scanned" {
		t.Errorf("expected title 'Already Scanned', got '%s'", info.Title)
	}
	if info.ID != "steam_555555" {
		t.Errorf("expected ID steam_555555, got %s", info.ID)
	}
}

func TestScanNotAGame(t *testing.T) {
	testDir := testdataDir(t)

	cfg := testConfig()
	scanner := NewScanner(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "not_a_game")
	if r != nil {
		t.Error("not_a_game should not be identified as a game")
	}

	if len(results) == 0 {
		t.Error("expected other games in results")
	}
}

func TestScanAll(t *testing.T) {
	testDir := testdataDir(t)
	defer func() {
		filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.Name() == ".gameinfo.json" {
				if !strings.Contains(path, "already_scanned") {
					os.Remove(path)
				}
			}
			return nil
		})
	}()

	cfg := testConfig()
	cfg.GameDirectories = []string{"."}
	scanner := NewScanner(testDir, cfg)
	results, err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %s", err)
	}

	gameCount := 0
	errorCount := 0
	for _, r := range results {
		if r.Error != "" {
			errorCount++
			t.Logf("error scanning %s: %s", r.GameDir, r.Error)
		} else {
			gameCount++
		}
	}

	if gameCount != 8 {
		t.Errorf("expected 8 games, got %d", gameCount)
	}
	if errorCount != 0 {
		t.Errorf("expected 0 errors, got %d", errorCount)
	}

	platforms := map[string]int{}
	for _, r := range results {
		if r.GameInfo != nil {
			platforms[r.GameInfo.Platform]++
		}
	}
	if platforms["steam"] != 7 {
		t.Errorf("expected 7 steam games, got %d", platforms["steam"])
	}
	if platforms["local"] != 1 {
		t.Errorf("expected 1 local game, got %d", platforms["local"])
	}
}

func TestConfigSaveLoad(t *testing.T) {
	dir := t.TempDir()

	cfg := DefaultConfig()
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

	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %s", err)
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

func TestGameInfoSaveLoad(t *testing.T) {
	dir := t.TempDir()

	info := newGameInfo(dir, []Executable{
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

	loaded, err := LoadGameInfo(dir)
	if err != nil {
		t.Fatalf("LoadGameInfo failed: %s", err)
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

	loaded, err = LoadGameInfo(filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadSteamAppID(t *testing.T) {
	dir := t.TempDir()

	deepDir := filepath.Join(dir, "deep", "deeper")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(filepath.Join(dir, "steam_appid.txt"), []byte("parent123"), 0644)
	os.WriteFile(filepath.Join(deepDir, "dummy.exe"), []byte(""), 0644)

	cfg := testConfig()
	scanner := NewScanner(dir, cfg)

	appID := scanner.readSteamAppID(deepDir)
	if appID != "parent123" {
		t.Errorf("expected parent123, got %s", appID)
	}

	appID = scanner.readSteamAppID(dir)
	if appID != "parent123" {
		t.Errorf("expected parent123 at root, got %s", appID)
	}
}

func TestConfigJSONRoundtrip(t *testing.T) {
	cfg := DefaultConfig()
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

func TestPickPrimaryExec(t *testing.T) {
	cfg := testConfig()
	scanner := NewScanner(".", cfg)

	tests := []struct {
		name     string
		exes     []Executable
		expected string
	}{
		{
			name:     "keyword game wins over launcher",
			exes:     []Executable{{Path: "launcher.exe", Name: "launcher"}, {Path: "game.exe", Name: "game"}},
			expected: "game.exe",
		},
		{
			name:     "keyword game",
			exes:     []Executable{{Path: "foo.exe", Name: "foo"}, {Path: "game.exe", Name: "game"}},
			expected: "game.exe",
		},
		{
			name:     "shortest wins",
			exes:     []Executable{{Path: "verylongname.exe", Name: "verylongname"}, {Path: "s.exe", Name: "s"}},
			expected: "s.exe",
		},
		{
			name:     "keyword start",
			exes:     []Executable{{Path: "a.exe", Name: "a"}, {Path: "start.exe", Name: "start"}},
			expected: "start.exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := scanner.pickPrimaryExec(tt.exes)
			if primary.Path != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, primary.Path)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
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

func TestEmptyDir(t *testing.T) {
	dir := t.TempDir()

	cfg := testConfig()
	scanner := NewScanner(dir, cfg)
	results, err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %s", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty dir, got %d", len(results))
	}
}

func TestNonexistentDir(t *testing.T) {
	cfg := testConfig()
	cfg.GameDirectories = []string{".\\nonexistent_path_12345"}
	scanner := NewScanner(".", cfg)
	results, err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %s", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent dir, got %d", len(results))
	}
}
