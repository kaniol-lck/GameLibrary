package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsLauncher(t *testing.T) {
	launchers := []string{
		"game.exe", "GAME.EXE", "start.com",
		"run.bat", "run.cmd", "run.ps1",
		"shortcut.lnk", "bookmark.url",
	}
	for _, name := range launchers {
		if !IsLauncher(name) {
			t.Errorf("expected %s to be recognised as a launcher", name)
		}
	}

	notLaunchers := []string{
		"readme.txt", "data.xp3", "config.ini", "game.exe.bak",
		"archive.zip", "no-extension", "", "movie.mp4",
	}
	for _, name := range notLaunchers {
		if IsLauncher(name) {
			t.Errorf("expected %s NOT to be recognised as a launcher", name)
		}
	}
}

func TestLaunchKindFor(t *testing.T) {
	tests := []struct {
		name string
		want LaunchKind
		ok   bool
	}{
		{"game.exe", LaunchDirect, true},
		{"setup.com", LaunchDirect, true},
		{"link.lnk", LaunchDirect, true},
		{"run.bat", LaunchCommandScript, true},
		{"run.cmd", LaunchCommandScript, true},
		{"script.ps1", LaunchPowerShellScript, true},
		{"readme.txt", LaunchDirect, false},
	}
	for _, tt := range tests {
		kind, ok := LaunchKindFor(tt.name)
		if ok != tt.ok {
			t.Errorf("%s: expected ok=%v, got %v", tt.name, tt.ok, ok)
			continue
		}
		if ok && kind != tt.want {
			t.Errorf("%s: expected kind %v, got %v", tt.name, tt.want, kind)
		}
	}
}

func TestLauncherExtensionsCoversScripts(t *testing.T) {
	extensions := LauncherExtensions()
	seen := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		if !strings.HasPrefix(ext, ".") {
			t.Errorf("extension %q is missing its leading dot", ext)
		}
		if ext != strings.ToLower(ext) {
			t.Errorf("extension %q should be lower case", ext)
		}
		seen[ext] = true
	}

	// Wrapper scripts are first-class launchers: games on a share are frequently
	// shipped with one instead of, or next to, an executable.
	for _, want := range []string{".exe", ".com", ".bat", ".cmd", ".ps1", ".lnk"} {
		if !seen[want] {
			t.Errorf("expected %s among the launcher extensions, got %v", want, extensions)
		}
	}
}

func TestScriptInterpreter(t *testing.T) {
	path := filepath.Join("C:", "Games", "My Game", "run.bat")

	name, args, ok := scriptInterpreter(LaunchCommandScript, path)
	if !ok {
		t.Fatal("expected a command script to have an interpreter")
	}
	if name != "cmd" {
		t.Errorf("expected cmd, got %q", name)
	}
	if len(args) != 2 || args[0] != "/c" {
		t.Fatalf("unexpected cmd arguments: %v", args)
	}
	// The path must be quoted: library paths routinely contain spaces.
	if !strings.Contains(args[1], `"`) || !strings.Contains(args[1], "My Game") {
		t.Errorf("expected a quoted path, got %q", args[1])
	}

	name, args, ok = scriptInterpreter(LaunchPowerShellScript, "run.ps1")
	if !ok || name != "powershell" {
		t.Fatalf("expected powershell, got %q (ok=%v)", name, ok)
	}
	if !contains(args, "-File") || !contains(args, "-ExecutionPolicy") {
		t.Errorf("unexpected powershell arguments: %v", args)
	}

	// A plain executable is started directly, with no interpreter.
	if _, _, ok := scriptInterpreter(LaunchDirect, "game.exe"); ok {
		t.Error("a direct launcher must not go through an interpreter")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestOpenPathRejectsMissingTargets covers the error paths without launching
// anything.
func TestOpenPathRejectsMissingTargets(t *testing.T) {
	if err := OpenPath(""); err == nil {
		t.Error("expected an error for an empty path")
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := OpenPath(missing); err == nil {
		t.Error("expected an error for a missing path")
	}
	if err := EditFile(""); err == nil {
		t.Error("expected an error for an empty path")
	}
	if err := EditFile(missing); err == nil {
		t.Error("expected an error for a missing file")
	}
}

// TestLaunchGameRejectsUnknownLauncher checks the guard before any process is
// started.
func TestLaunchGameRejectsUnknownLauncher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LaunchGame(path, dir); err == nil {
		t.Error("expected an error when the file is not a launcher")
	}
}
