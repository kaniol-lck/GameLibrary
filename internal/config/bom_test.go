package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadToleratesByteOrderMark is a regression test.
//
// The app was run against a config.json written by Windows tooling, which
// prepends a BOM. encoding/json rejects the leading bytes, so the whole file
// failed to parse and the application silently fell back to defaults — every
// configured directory, the scan depth, the language and every API key were
// discarded with nothing worse than a log line.
func TestLoadToleratesByteOrderMark(t *testing.T) {
	root := t.TempDir()

	cfg := Default()
	cfg.GameDirectories = []string{".\\Library"}
	cfg.MaxScanDepth = 7
	cfg.Language = "ja-JP"
	cfg.LogLevel = "debug"

	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	for name, prefix := range map[string][]byte{
		"utf8":    {0xEF, 0xBB, 0xBF},
		"utf16le": {0xFF, 0xFE},
		"utf16be": {0xFE, 0xFF},
		"none":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, "config.json")
			if err := os.WriteFile(path, append(append([]byte{}, prefix...), body...), 0o644); err != nil {
				t.Fatal(err)
			}

			loaded, err := Load(root)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if loaded.MaxScanDepth != 7 {
				t.Errorf("settings were not read back: MaxScanDepth = %d", loaded.MaxScanDepth)
			}
			if loaded.Language != "ja-JP" {
				t.Errorf("settings were not read back: Language = %q", loaded.Language)
			}
			if len(loaded.GameDirectories) != 1 || loaded.GameDirectories[0] != ".\\Library" {
				t.Errorf("settings were not read back: GameDirectories = %v", loaded.GameDirectories)
			}
		})
	}
}
