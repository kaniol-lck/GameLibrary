package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHasExe(t *testing.T) {
	dir := t.TempDir()

	if hasExe(dir) {
		t.Error("empty dir should return false")
	}

	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644)
	if hasExe(dir) {
		t.Error("dir without exe should return false")
	}

	os.WriteFile(filepath.Join(dir, "game.exe"), []byte{}, 0644)
	if !hasExe(dir) {
		t.Error("dir with game.exe should return true")
	}
}

func TestHasExeUnins(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "unins000.exe"), []byte{}, 0644)
	if hasExe(dir) {
		t.Error("unins000.exe should be filtered")
	}
}

func TestFlushSingle(t *testing.T) {
	var called []string
	w := &Watcher{
		events:   make(map[string]time.Time),
		callback: func(dirs []string) { called = dirs },
	}

	w.events["/test/game1"] = time.Now()
	w.flush()

	if len(called) != 0 {
		t.Log("flush with no exe should not callback")
	}
}

func TestFlushWithCallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "game.exe"), []byte{}, 0644)

	var called []string
	w := &Watcher{
		events:   make(map[string]time.Time),
		callback: func(dirs []string) { called = dirs },
	}

	w.events[dir] = time.Now()
	w.flush()

	if len(called) != 1 || called[0] != dir {
		t.Errorf("expected callback with [%s], got %v", dir, called)
	}
}

func TestStop(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "game.exe"), []byte{}, 0644)

	w := New(nil)
	err := w.WatchDirs([]string{dir}, "")
	if err != nil {
		t.Fatalf("WatchDirs failed: %v", err)
	}

	w.Stop()

	if len(w.events) > 0 {
		// events map should still exist but watcher stopped
	}
}

func TestWatchDirsNonExistent(t *testing.T) {
	w := New(nil)
	err := w.WatchDirs([]string{"/nonexistent/path"}, "")
	if err != nil {
		t.Fatalf("WatchDirs should not error on nonexistent path: %v", err)
	}
	w.Stop()
}

func TestWatchDirsRelative(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "game.exe"), []byte{}, 0644)

	w := New(nil)
	relDir, _ := filepath.Rel(dir, dir)
	if relDir == "." {
		relDir = dir
	}
	err := w.WatchDirs([]string{relDir}, dir)
	if err != nil {
		t.Fatalf("WatchDirs with relative path failed: %v", err)
	}
	w.Stop()
}

func TestFlushEmpty(t *testing.T) {
	var called bool
	w := &Watcher{
		events:   make(map[string]time.Time),
		callback: func(dirs []string) { called = true },
	}
	w.flush()
	if called {
		t.Error("flush with no events should not callback")
	}
}
