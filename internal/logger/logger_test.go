package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readLog returns the contents of the single log file in dir.
func readLog(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	var name string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			name = entry.Name()
		}
	}
	if name == "" {
		t.Fatal("no log file was created")
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	return string(data)
}

func TestCallerLocationPointsAtTheCallSite(t *testing.T) {
	dir := t.TempDir()
	Init(Options{LocalDir: dir, Level: slog.LevelDebug})
	defer Close()

	// Log through a typed helper, which is where the old implementation went
	// wrong: a fixed stack depth reported logger.go for every wrapper instead of
	// the file that actually called it.
	GameLaunched("steam_570", "Dota 2", "game.exe")

	content := readLog(t, filepath.Join(dir, "logs"))
	if !strings.Contains(content, "game launched") {
		t.Fatalf("expected the message in the log, got:\n%s", content)
	}
	if !strings.Contains(content, "logger_test.go") && !strings.Contains(content, "[testing.go:") {
		t.Errorf("expected a file:line location from outside this package, got:\n%s", content)
	}
	if strings.Contains(content, "[logger.go:") {
		t.Errorf("the log points at the logging wrapper instead of the call site:\n%s", content)
	}
}

func TestLevelFiltersRecords(t *testing.T) {
	dir := t.TempDir()
	Init(Options{LocalDir: dir, Level: slog.LevelWarn})
	defer Close()

	Info("this should not be written")
	Debug("this neither")
	Warn("this should be written")

	content := readLog(t, filepath.Join(dir, "logs"))
	if strings.Contains(content, "should not be written") || strings.Contains(content, "this neither") {
		t.Errorf("records below the configured level were written:\n%s", content)
	}
	if !strings.Contains(content, "this should be written") {
		t.Errorf("expected the warning in the log:\n%s", content)
	}
}

func TestFormatsAttributesReadably(t *testing.T) {
	dir := t.TempDir()
	Init(Options{LocalDir: dir, Level: slog.LevelDebug})
	defer Close()

	Info("scan started",
		"gameDirectories", []string{".\\Games", ".\\Extra"},
		"maxDepth", 3,
		"note", "has spaces",
	)

	content := readLog(t, filepath.Join(dir, "logs"))
	for _, want := range []string{
		"scan started",
		"gameDirectories=[.\\Games, .\\Extra]",
		"maxDepth=3",
		`note="has spaces"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in the log, got:\n%s", want, content)
		}
	}
}

func TestLogToLibraryRoot(t *testing.T) {
	root := t.TempDir()
	local := t.TempDir()
	Init(Options{LibraryRoot: root, LocalDir: local, ToLibrary: true, Level: slog.LevelInfo})
	defer Close()

	Info("hello")
	if Dir() != filepath.Join(root, "logs") {
		t.Errorf("expected logs beside the executable, got %q", Dir())
	}
	Close()

	// Off by default: a portable library keeps its logs on the local machine so
	// several clients do not interleave writes on the share.
	Init(Options{LibraryRoot: root, LocalDir: local, Level: slog.LevelInfo})
	defer Close()
	if Dir() != filepath.Join(local, "logs") {
		t.Errorf("expected logs in the per-machine directory, got %q", Dir())
	}
}

func TestRotatesPerDay(t *testing.T) {
	dir := t.TempDir()
	handler := &rotatingHandler{dir: dir, level: slog.LevelDebug}

	first := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	if err := handler.ensureFile(first); err != nil {
		t.Fatalf("ensureFile: %v", err)
	}
	if handler.fileDate != "2026-01-02" {
		t.Fatalf("expected the first day's file, got %q", handler.fileDate)
	}

	// Same day: the handle is reused.
	openFile := handler.file
	if err := handler.ensureFile(first.Add(2 * time.Hour)); err != nil {
		t.Fatalf("ensureFile: %v", err)
	}
	if handler.file != openFile {
		t.Error("the log file was rotated within the same day")
	}

	// Next day: a new file.
	second := first.Add(24 * time.Hour)
	if err := handler.ensureFile(second); err != nil {
		t.Fatalf("ensureFile: %v", err)
	}
	if handler.fileDate != "2026-01-03" {
		t.Errorf("expected rotation to a new day, got %q", handler.fileDate)
	}
	handler.close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two daily files, got %d", len(entries))
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "gamemanager_") {
			t.Errorf("unexpected log file name %q", entry.Name())
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":    slog.LevelDebug,
		"DEBUG":    slog.LevelDebug,
		"info":     slog.LevelInfo,
		"warn":     slog.LevelWarn,
		"warning":  slog.LevelWarn,
		"error":    slog.LevelError,
		"":         slog.LevelInfo,
		"nonsense": slog.LevelInfo,
	}
	for input, want := range tests {
		if got := ParseLevel(input); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestLoggingBeforeInitIsANoOp(t *testing.T) {
	// The package is in its zero state here (no Init in this test), which is what
	// lets other packages' unit tests run without any logging setup.
	Info("nothing should happen")
	Warn("nothing should happen either")
	Error("still nothing")
	Close()
	if Dir() != "" {
		t.Errorf("expected no log directory before Init, got %q", Dir())
	}
}
