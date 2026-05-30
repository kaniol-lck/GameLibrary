package scraper

import (
	"testing"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
)

func TestPipelineNoSourceEnabled(t *testing.T) {
	cfg := &config.Config{
		Sources: []config.MetadataSource{},
	}

	pipeline := NewPipeline(cfg)
	pipeline.Register(NewSteamScraper())

	info := game.New("/tmp/testgame", []game.Executable{}, "12345")
	result, source, err := pipeline.Scrape("/tmp/testgame", info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when no sources enabled")
	}
	if source != "" {
		t.Errorf("expected empty source, got %s", source)
	}
}

func TestPipelineSkipDisabled(t *testing.T) {
	cfg := &config.Config{
		Sources: []config.MetadataSource{
			{Key: "steam", Name: "Steam", Enabled: false},
		},
	}

	pipeline := NewPipeline(cfg)
	pipeline.Register(NewSteamScraper())

	info := game.New("/tmp/testgame", []game.Executable{}, "12345")
	result, _, _ := pipeline.Scrape("/tmp/testgame", info)
	if result != nil {
		t.Error("expected nil when source is disabled")
	}
}

func TestPipelinePriority(t *testing.T) {
	cfg := &config.Config{
		Sources: []config.MetadataSource{
			{Key: "vndb", Name: "VNDB", Enabled: true},
			{Key: "steam", Name: "Steam", Enabled: true},
		},
	}

	pipeline := NewPipeline(cfg)
	pipeline.Register(NewSteamScraper())
	pipeline.Register(NewVNDBScraper())

	info := game.New("/tmp/testgame", []game.Executable{}, "99999999")
	result, source, _ := pipeline.Scrape("/tmp/testgame", info)

	if result == nil {
		t.Skip("no network or API unavailable, skipping integration check")
	}
	t.Logf("matched source: %s, title: %s", source, result.Title)
}

func TestSteamScraperKey(t *testing.T) {
	s := NewSteamScraper()
	if s.Key() != "steam" {
		t.Errorf("expected 'steam', got '%s'", s.Key())
	}
}

func TestVNDBScraperKey(t *testing.T) {
	s := NewVNDBScraper()
	if s.Key() != "vndb" {
		t.Errorf("expected 'vndb', got '%s'", s.Key())
	}
}

func TestDLsiteScraperKey(t *testing.T) {
	s := NewDLsiteScraper()
	if s.Key() != "dlsite" {
		t.Errorf("expected 'dlsite', got '%s'", s.Key())
	}
}

func TestApplyResult(t *testing.T) {
	info := game.New("/tmp/testgame", []game.Executable{}, "")
	result := &Result{
		Title:       "Test Game",
		TitleNative: "テストゲーム",
		Description: "A test game description",
		Developer:   "TestDev",
		Publisher:   "TestPub",
		ReleaseDate: "2024-01-15",
		Tags:        []string{"Action", "RPG"},
		CoverURL:    "https://example.com/cover.jpg",
		Links:       map[string]string{"steam": "https://store.steampowered.com/app/9999/"},
	}

	ApplyResult(info, result, "steam")

	if info.Title != "Test Game" {
		t.Errorf("Title: expected 'Test Game', got '%s'", info.Title)
	}
	if info.Platform != "steam" {
		t.Errorf("Platform: expected 'steam', got '%s'", info.Platform)
	}
	if info.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if info.Metadata.Developer != "TestDev" {
		t.Errorf("Developer: expected 'TestDev', got '%s'", info.Metadata.Developer)
	}
	if len(info.Metadata.Tags) != 2 {
		t.Errorf("Tags: expected 2, got %d", len(info.Metadata.Tags))
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<b>Hello</b> World", "Hello World"},
		{"<br/>Line<br>Break", "LineBreak"},
		{"No tags here", "No tags here"},
		{"<a href='x'>link</a>", "link"},
	}

	for _, tc := range tests {
		result := stripHTML(tc.input)
		if result != tc.expected {
			t.Errorf("stripHTML(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestRJPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"RJ123456", "RJ123456"},
		{"Game_RJ00123456_Folder", "RJ00123456"},
		{"RJ12345678", "RJ12345678"},
		{"NoRJHere", ""},
		{"RJ123", ""},
	}

	for _, tc := range tests {
		result := rjPattern.FindString(tc.input)
		if result != tc.expected {
			t.Errorf("FindString(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ArcadeShooter", "Arcade Shooter"},
		{"FateStayNight", "Fate Stay Night"},
		{"SteinsGate", "Steins Gate"},
		{"Clannad", "Clannad"},
		{"MultiExeDemo", "Multi Exe Demo"},
		{"LocalRPG", "Local RPG"},
		{"PuzzleQuest", "Puzzle Quest"},
		{"Higurashi", "Higurashi"},
		{"BaldursGate3", "Baldurs Gate3"},
	}

	for _, tc := range tests {
		result := splitCamelCase(tc.input)
		if result != tc.expected {
			t.Errorf("splitCamelCase(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestNameVariations(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"SteinsGate", []string{"SteinsGate", "Steins Gate"}},
		{"FateStayNight", []string{"FateStayNight", "Fate Stay Night"}},
		{"hello-world", []string{"hello-world", "hello world"}},
	}

	for _, tc := range tests {
		result := nameVariations(tc.input)
		for _, exp := range tc.expected {
			found := false
			for _, r := range result {
				if r == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("nameVariations(%q) should contain %q, got %v", tc.input, exp, result)
			}
		}
	}
}

func TestRawgScraperKey(t *testing.T) {
	s := NewRawgScraper()
	if s.Key() != "rawg" {
		t.Errorf("expected 'rawg', got '%s'", s.Key())
	}
}

func TestNormalizeSearchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"steam_12345", "steam_12345"},
		{"RJ401234", "RJ401234"},
		{"rj401234", "rj401234"},
		{"Clannad", "Clannad"},
		{"Hollow Knight", "Hollow Knight"},
	}

	for _, tc := range tests {
		result := normalizeSearchName(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeSearchName(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}
