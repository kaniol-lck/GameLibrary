package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..", "testdata")
}

func testConfig() *config.Config {
	return &config.Config{
		MachineID:       "test-machine",
		GameDirectories: []string{".\\testdata"},
		MaxScanDepth:    4,
		Language:        "zh-CN",
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cleanupGameInfo(dir string) {
	path := filepath.Join(dir, ".gameinfo.json")
	os.Remove(path)
}

func findResult(results []ScanResult, suffix string) *ScanResult {
	for i := range results {
		if strings.HasSuffix(results[i].GameDir, suffix) {
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
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "simple_steam_game")
	if r == nil {
		t.Fatal("simple_steam_game not found")
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
	if info.PrimaryPlatform() != "steam" {
		t.Errorf("expected platform steam, got %s", info.PrimaryPlatform())
	}
	if info.PrimaryPlatformID() != "123456" {
		t.Errorf("expected platformId 123456, got %s", info.PrimaryPlatformID())
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
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, filepath.Join("deep_exe_game", "bin", "x64"))
	if r == nil {
		t.Fatal("deep_exe_game/bin/x64 not found")
	}

	info := r.GameInfo
	if info.ID != "steam_789012" {
		t.Errorf("expected ID steam_789012, got %s", info.ID)
	}
	if len(info.Executables) != 1 {
		t.Errorf("expected 1 executable, got %d", len(info.Executables))
	}
}

func TestScanMultiExeGame(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "multi_exe_game")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "multi_exe_game")
	if r == nil {
		t.Fatal("multi_exe_game not found")
	}

	info := r.GameInfo
	if len(info.Executables) != 3 {
		t.Fatalf("expected 3 executables, got %d", len(info.Executables))
	}

	for _, exe := range info.Executables {
		if exe.Path == "game.exe" && !exe.Primary {
			t.Error("expected game.exe to be primary")
		}
		if exe.Path == "uninstall.exe" {
			t.Error("uninstall.exe should be filtered out")
		}
	}
}

func TestScanLocalGame(t *testing.T) {
	testDir := testdataDir(t)
	gameDir := filepath.Join(testDir, "local_game")
	defer cleanupGameInfo(gameDir)

	cfg := testConfig()
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "local_game")
	if r == nil {
		t.Fatal("local_game not found")
	}

	info := r.GameInfo
	if info.ID != "local_game" {
		t.Errorf("expected ID local_game, got %s", info.ID)
	}
	if info.PrimaryPlatform() != "local" {
		t.Errorf("expected platform local, got %s", info.PrimaryPlatform())
	}
}

func TestScanCollection(t *testing.T) {
	testDir := testdataDir(t)
	sub1Dir := filepath.Join(testDir, "collection", "SubGame1")
	sub2Dir := filepath.Join(testDir, "collection", "SubGame2")
	defer cleanupGameInfo(sub1Dir)
	defer cleanupGameInfo(sub2Dir)

	cfg := testConfig()
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r1 := findResult(results, filepath.Join("collection", "SubGame1"))
	if r1 == nil {
		t.Fatal("SubGame1 not found")
	}
	if r1.GameInfo.ID != "steam_333333" {
		t.Errorf("SubGame1: expected steam_333333, got %s", r1.GameInfo.ID)
	}

	r2 := findResult(results, filepath.Join("collection", "SubGame2"))
	if r2 == nil {
		t.Fatal("SubGame2 not found")
	}
	if r2.GameInfo.ID != "steam_444444" {
		t.Errorf("SubGame2: expected steam_444444, got %s", r2.GameInfo.ID)
	}
}

func TestScanAlreadyScanned(t *testing.T) {
	testDir := testdataDir(t)

	cfg := testConfig()
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "already_scanned")
	if r == nil {
		t.Fatal("already_scanned not found")
	}
	if r.IsNew {
		t.Error("expected IsNew to be false")
	}
	if r.GameInfo.Title != "Already Scanned" {
		t.Errorf("expected title 'Already Scanned', got '%s'", r.GameInfo.Title)
	}
}

func TestScanNotAGame(t *testing.T) {
	testDir := testdataDir(t)

	cfg := testConfig()
	scanner := New(testDir, cfg)
	results := scanner.scanDir(testDir, 0)

	r := findResult(results, "not_a_game")
	if r != nil {
		t.Error("not_a_game should not be identified as a game")
	}
}

func TestScanAll(t *testing.T) {
	testDir := testdataDir(t)
	defer func() {
		filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.Name() == ".gameinfo.json" && !strings.Contains(path, "already_scanned") {
				os.Remove(path)
			}
			return nil
		})
	}()

	cfg := testConfig()
	cfg.GameDirectories = []string{"."}
	scanner := New(testDir, cfg)
	results, err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %s", err)
	}

	gameCount := 0
	for _, r := range results {
		if r.Error == "" {
			gameCount++
		}
	}
	if gameCount != 8 {
		t.Errorf("expected 8 games, got %d", gameCount)
	}
}

func TestReadSteamAppID(t *testing.T) {
	dir := t.TempDir()

	deepDir := filepath.Join(dir, "deep", "deeper")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(filepath.Join(dir, "steam_appid.txt"), []byte("parent123"), 0644)

	cfg := testConfig()
	scanner := New(dir, cfg)

	if id := scanner.readSteamAppID(deepDir); id != "parent123" {
		t.Errorf("expected parent123, got %s", id)
	}
	if id := scanner.readSteamAppID(dir); id != "parent123" {
		t.Errorf("expected parent123 at root, got %s", id)
	}
}

func TestPickPrimaryExec(t *testing.T) {
	cfg := testConfig()
	scanner := New(".", cfg)

	tests := []struct {
		name     string
		exes     []game.Executable
		expected string
	}{
		{
			name:     "keyword game wins over launcher",
			exes:     []game.Executable{{Path: "launcher.exe", Name: "launcher"}, {Path: "game.exe", Name: "game"}},
			expected: "game.exe",
		},
		{
			name:     "keyword game",
			exes:     []game.Executable{{Path: "foo.exe", Name: "foo"}, {Path: "game.exe", Name: "game"}},
			expected: "game.exe",
		},
		{
			name:     "shortest wins",
			exes:     []game.Executable{{Path: "verylongname.exe", Name: "verylongname"}, {Path: "s.exe", Name: "s"}},
			expected: "s.exe",
		},
		{
			name:     "keyword start",
			exes:     []game.Executable{{Path: "a.exe", Name: "a"}, {Path: "start.exe", Name: "start"}},
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

func TestEmptyDir(t *testing.T) {
	dir := t.TempDir()

	cfg := testConfig()
	scanner := New(dir, cfg)
	results, err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %s", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestNonexistentDir(t *testing.T) {
	cfg := testConfig()
	cfg.GameDirectories = []string{".\\nonexistent_path_12345"}
	scanner := New(".", cfg)
	results, err := scanner.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %s", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
