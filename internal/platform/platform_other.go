//go:build !windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The application targets Windows. This file keeps the module buildable and
// testable elsewhere (CI runs the Go test suite on Linux) while making it
// obvious at runtime that the shell integration is unavailable.

func OpenPath(path string) error {
	if strings.TrimSpace(path) == "" {
		// Checked before cleaning: filepath.Clean("") is ".".
		return fmt.Errorf("open path: empty path")
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("open path %s: %w", path, err)
	}
	return runOpener(path)
}

func EditFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("edit file: empty path")
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("edit file %s: %w", path, err)
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return exec.Command(editor, path).Start()
	}
	return runOpener(path)
}

func runOpener(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", path).Start()
	}
	return exec.Command("xdg-open", path).Start()
}

// LaunchGame starts a launcher file directly. Script interpreters are not
// wired up on non-Windows platforms.
func LaunchGame(path, workDir string) (*exec.Cmd, error) {
	kind, ok := LaunchKindFor(path)
	if !ok {
		return nil, fmt.Errorf("unsupported launcher type: %s", filepath.Base(path))
	}
	if kind != LaunchDirect {
		return nil, fmt.Errorf("%w: script launchers require Windows", ErrUnsupported)
	}
	cmd := exec.Command(path)
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(path), err)
	}
	return cmd, nil
}

// SteamUser is a detected Steam account.
type SteamUser struct {
	ID   string
	Name string
}

// SteamPath always reports "not found" off Windows.
func SteamPath() string { return "" }

// SteamUsers always reports none off Windows.
func SteamUsers() []SteamUser { return nil }

// ShowFatalError writes the failure to stderr; there is no GUI message box.
func ShowFatalError(title, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
}
