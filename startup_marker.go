package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"GameLibrary/internal/logger"
	"GameLibrary/internal/platform"
)

// startupMarkerName marks a launch that is in progress.
const startupMarkerName = "startup-in-progress"

// startupMarkerPath returns where the in-progress marker is kept.
//
// It sits beside the logs, falling back to the executable's own folder, so it
// always lives somewhere the application can actually write.
func startupMarkerPath() string {
	base := logger.DefaultLocalDir()
	if base == "" {
		base = resolveExeDir()
	}
	return filepath.Join(base, startupMarkerName)
}

// checkPreviousStartup reports a previous launch that died before its window
// appeared, and tells the user why.
//
// The WebView2 environment and its controller are created before OnStartup runs,
// and Wails terminates the process outright when that fails. There is therefore
// no point inside the application at which the failure can be shown: the process
// simply disappears, which from the user's side looks like "I double-clicked it
// and nothing happened". The marker turns that silent death into an explanation
// on the next attempt.
func checkPreviousStartup() {
	marker := startupMarkerPath()
	previous, err := os.ReadFile(marker)
	if err != nil {
		return
	}
	_ = os.Remove(marker)

	logDir := logger.Dir()
	if logDir == "" {
		logDir = logger.DefaultLocalDir()
	}

	message := strings.Join([]string{
		"GameLibrary did not finish starting the last time it was run.",
		"",
		"The window is created by the Microsoft Edge WebView2 runtime, and it",
		"could not be initialised. The usual causes are:",
		"",
		"  1. The WebView2 runtime is missing, broken or out of date.",
		"     Install or repair it from:",
		"     https://developer.microsoft.com/microsoft-edge/webview2/",
		"  2. Another copy of GameLibrary is still running in the background.",
		"     Close it in Task Manager and try again.",
		"  3. The program is being started from a folder the WebView2 runtime",
		"     cannot use. Copy it to a normal folder (or your NAS root) and retry.",
		"",
		"What the previous attempt recorded:",
		describePreviousAttempt(previous),
		"",
		"Log folder:",
		logDir,
	}, "\n")

	platform.ShowFatalError("GameLibrary could not start", message)
}

// describePreviousAttempt renders whatever the dying process managed to record.
func describePreviousAttempt(recorded []byte) string {
	text := strings.TrimSpace(string(recorded))
	if text == "" {
		return "  (the previous attempt died before it could write anything)"
	}
	return "  " + text
}

// markStartupInProgress records the attempt; clearStartupMarker removes it once
// the window is alive.
func markStartupInProgress() {
	marker := startupMarkerPath()
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte(fmt.Sprintf("version %s, started at %s, window never created",
		version, time.Now().UTC().Format(time.RFC3339))), 0o644)
}

// clearStartupMarker is called once the webview is alive.
func clearStartupMarker() {
	_ = os.Remove(startupMarkerPath())
}
