package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartupMarkerLifecycle covers the mechanism that turns a silent start-up
// death into a message on the next attempt.
//
// The WebView2 controller is created before OnStartup runs and Wails terminates
// the process when that fails, so the application has no opportunity to report the
// failure as it happens. A marker written before the attempt, and removed once the
// window is alive, is what makes the failure visible afterwards.
func TestStartupMarkerLifecycle(t *testing.T) {
	redirectLocalDir(t)

	marker := startupMarkerPath()
	if marker == "" {
		t.Fatal("expected a marker path")
	}

	// A clean first run has nothing to report.
	if _, err := os.ReadFile(marker); err == nil {
		t.Fatal("expected no marker before the first attempt")
	}

	markStartupInProgress()
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("expected the marker to be written: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected the marker to record something about the attempt")
	}
	if !strings.Contains(string(data), version) {
		t.Errorf("expected the marker to name version %q, got %q", version, data)
	}

	// A successful start removes it, so the next launch stays silent.
	clearStartupMarker()
	if _, err := os.ReadFile(marker); err == nil {
		t.Fatal("expected the marker to be cleared once the window is up")
	}
	// Clearing again must be harmless: shutdown paths can run twice.
	clearStartupMarker()
}

func TestDescribePreviousAttempt(t *testing.T) {
	if got := describePreviousAttempt(nil); !strings.Contains(got, "died before") {
		t.Errorf("expected an explanation for an empty record, got %q", got)
	}
	if got := describePreviousAttempt([]byte("  version 0.8.0  ")); got != "  version 0.8.0" {
		t.Errorf("expected the record to be trimmed and indented, got %q", got)
	}
}

func TestStartupMarkerSitsBesideTheLogs(t *testing.T) {
	base := redirectLocalDir(t)

	// DefaultLocalDir is the per-machine cache directory plus the app name, and
	// the marker sits directly beside the logs folder inside it.
	want := filepath.Join(base, "GameLibrary")
	if got := filepath.Dir(startupMarkerPath()); got != want {
		t.Errorf("expected the marker in %q, got %q", want, got)
	}
}

// redirectLocalDir points the per-machine directory at a temporary location and
// returns it.
func redirectLocalDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("HOME", base)
	return base
}
