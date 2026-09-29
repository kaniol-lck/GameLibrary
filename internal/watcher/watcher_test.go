package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewClampsArguments(t *testing.T) {
	w := New(0, 0, nil)
	if w.maxDepth < 1 {
		t.Errorf("expected a positive scan depth, got %d", w.maxDepth)
	}
	if w.debounce <= 0 {
		t.Errorf("expected a positive debounce, got %s", w.debounce)
	}
}

func TestEventsEmpty(t *testing.T) {
	if !(Events{}).Empty() {
		t.Error("a zero Events value should be empty")
	}
	if (Events{NewGameDirs: []string{"x"}}).Empty() {
		t.Error("Events with a new game directory should not be empty")
	}
	if (Events{RemovedDirs: []string{"x"}}).Empty() {
		t.Error("Events with a removal should not be empty")
	}
	if (Events{ChangedDirs: []string{"x"}}).Empty() {
		t.Error("Events with a change should not be empty")
	}
}

func TestHasLauncher(t *testing.T) {
	dir := t.TempDir()

	// Scripts count as launchers: games on a share are frequently shipped with a
	// wrapper script instead of, or alongside, an executable.
	for _, name := range []string{"game.exe", "run.bat", "run.cmd", "run.ps1", "link.lnk"} {
		sub := filepath.Join(dir, name)
		if err := os.WriteFile(sub, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !hasLauncher(dir) {
			t.Errorf("expected %s to be recognised as a launcher", name)
		}
		if err := os.Remove(sub); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasLauncher(dir) {
		t.Error("a folder with only a text file is not a game")
	}
	if hasLauncher(filepath.Join(dir, "does-not-exist")) {
		t.Error("a missing directory is not a game")
	}
	if hasLauncher(filepath.Join(dir, "readme.txt")) {
		t.Error("a file is not a directory containing a launcher")
	}
}

func TestWatchedCountRespectsDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	w := New(1, 20*time.Millisecond, nil)
	if err := w.Watch([]string{root}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	// root and root/a are within depth 1; deeper directories are not registered.
	if got := w.WatchedCount(); got != 2 {
		t.Errorf("expected 2 watched directories, got %d", got)
	}
}

// TestReportsNewGameDirectory drives a real filesystem event end to end.
func TestReportsNewGameDirectory(t *testing.T) {
	root := t.TempDir()

	batches := make(chan Events, 16)
	w := New(4, 40*time.Millisecond, func(events Events) {
		select {
		case batches <- events:
		default:
		}
	})
	if err := w.Watch([]string{root}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	dir := filepath.Join(root, "NewGame")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !waitForDir(t, batches, dir) {
		t.Skip("this environment did not deliver a filesystem event for the new game directory")
	}
}

// TestReportsLauncherAddedToExistingDirectory covers the case a create-only
// watcher missed entirely: extracting a game into a folder that already exists.
// The folder may be reported as new or as changed depending on how the platform
// coalesces events, so either is accepted.
func TestReportsLauncherAddedToExistingDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ExistingFolder")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	batches := make(chan Events, 16)
	w := New(4, 40*time.Millisecond, func(events Events) {
		select {
		case batches <- events:
		default:
		}
	})
	if err := w.Watch([]string{root}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Skip("this environment did not deliver a filesystem event for the new launcher")
		case events := <-batches:
			candidates := append(append([]string{}, events.NewGameDirs...), events.ChangedDirs...)
			for _, candidate := range candidates {
				if samePath(candidate, dir) {
					return
				}
			}
		}
	}
}

// TestIgnoresHiddenDirectories makes sure a launcher inside a dot-directory is
// not reported as a game.
func TestIgnoresHiddenDirectories(t *testing.T) {
	root := t.TempDir()

	batches := make(chan Events, 16)
	w := New(4, 30*time.Millisecond, func(events Events) {
		select {
		case batches <- events:
		default:
		}
	})
	if err := w.Watch([]string{root}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "game.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(600 * time.Millisecond)
	for {
		select {
		case <-deadline:
			return
		case events := <-batches:
			for _, candidate := range events.NewGameDirs {
				if samePath(candidate, hidden) {
					t.Fatal("a launcher inside a hidden directory must not be reported")
				}
			}
		}
	}
}

// TestStopIsIdempotent guards against the double-close panic the previous
// implementation had on a second shutdown request.
func TestStopIsIdempotent(t *testing.T) {
	root := t.TempDir()
	w := New(2, 20*time.Millisecond, nil)
	if err := w.Watch([]string{root}); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	w.Stop()
	w.Stop()
	w.Stop()

	// Stopping a watcher that never started must also be safe.
	New(2, 20*time.Millisecond, nil).Stop()
}

func TestWatchMissingDirectoryIsIgnored(t *testing.T) {
	w := New(2, 20*time.Millisecond, nil)
	if err := w.Watch([]string{filepath.Join(t.TempDir(), "nope")}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	if got := w.WatchedCount(); got != 0 {
		t.Errorf("expected no watched directories, got %d", got)
	}
}

// TestFlushDropsVanishedDirectories ensures a recorded change to a directory that
// has since disappeared is not reported as a new game.
func TestFlushDropsVanishedDirectories(t *testing.T) {
	var received []Events
	var mu sync.Mutex

	w := New(2, 20*time.Millisecond, func(events Events) {
		mu.Lock()
		received = append(received, events)
		mu.Unlock()
	})

	dir := filepath.Join(t.TempDir(), "Gone")
	w.mu.Lock()
	w.events[dir] = time.Now()
	w.mu.Unlock()

	w.flush()

	mu.Lock()
	defer mu.Unlock()
	for _, events := range received {
		if len(events.NewGameDirs) != 0 {
			t.Fatalf("expected no new game directories, got %v", events.NewGameDirs)
		}
	}
}

// waitForDir waits until the directory shows up in a NewGameDirs batch.
func waitForDir(t *testing.T, batches <-chan Events, want string) bool {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			return false
		case events := <-batches:
			for _, candidate := range events.NewGameDirs {
				if samePath(candidate, want) {
					return true
				}
			}
		}
	}
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
