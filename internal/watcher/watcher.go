// Package watcher reports library changes while the application is running.
//
// It watches the configured game directories (down to the configured scan depth)
// and reports three kinds of change: a new game folder appeared, a game folder
// disappeared, and an existing folder gained a launcher. The last case matters
// because extracting a game into a folder that already exists is extremely common:
// a create-directory-only watcher never noticed it, so the game stayed invisible
// until the user pressed Scan.
package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/logger"
	"GameLibrary/internal/platform"

	"github.com/fsnotify/fsnotify"
)

// maxWatchedDirs bounds how many directories are registered, so a deep library on
// a network share cannot exhaust file handles. Directories beyond the cap are
// still picked up by the next full scan.
const maxWatchedDirs = 2000

// Events describes one debounced batch of filesystem changes.
type Events struct {
	// NewGameDirs are directories that now contain a launcher and had no record.
	NewGameDirs []string
	// ChangedDirs are already-known directories whose contents changed.
	ChangedDirs []string
	// RemovedDirs are directories that no longer exist.
	RemovedDirs []string
}

// Empty reports whether the batch carries no news.
func (e Events) Empty() bool {
	return len(e.NewGameDirs) == 0 && len(e.ChangedDirs) == 0 && len(e.RemovedDirs) == 0
}

// Handler receives debounced change batches.
type Handler func(Events)

// Watcher subscribes to filesystem events under the library directories.
type Watcher struct {
	handler  Handler
	fsw      *fsnotify.Watcher
	debounce time.Duration
	maxDepth int

	mu      sync.Mutex
	watched map[string]bool
	events  map[string]time.Time
	removes map[string]time.Time
	changed map[string]time.Time

	done     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// New creates a watcher. maxDepth mirrors the configured scan depth so the
// watcher and the scanner agree about how deep a game may live.
func New(maxDepth int, debounce time.Duration, handler Handler) *Watcher {
	if maxDepth < 1 {
		maxDepth = 1
	}
	if debounce <= 0 {
		debounce = 150 * time.Millisecond
	}
	return &Watcher{
		handler:  handler,
		debounce: debounce,
		maxDepth: maxDepth,
		watched:  make(map[string]bool),
		events:   make(map[string]time.Time),
		removes:  make(map[string]time.Time),
		changed:  make(map[string]time.Time),
		done:     make(chan struct{}),
	}
}

// Watch registers the given absolute directories and their subdirectories.
func (w *Watcher) Watch(dirs []string) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.fsw = fsw

	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		if !fsutil.DirExists(dir) {
			continue
		}
		w.addRecursive(dir, 0)
	}

	w.wg.Add(1)
	go w.loop()
	return nil
}

// WatchedCount returns how many directories are registered.
func (w *Watcher) WatchedCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.watched)
}

func (w *Watcher) addRecursive(dir string, depth int) {
	if depth > w.maxDepth {
		return
	}

	w.mu.Lock()
	if w.watched[dir] || len(w.watched) >= maxWatchedDirs {
		w.mu.Unlock()
		return
	}
	w.watched[dir] = true
	w.mu.Unlock()

	if err := w.fsw.Add(dir); err != nil {
		logger.Debug("watcher: cannot watch directory", "dir", dir, "error", err.Error())
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		w.addRecursive(filepath.Join(dir, entry.Name()), depth+1)
	}
}

func (w *Watcher) loop() {
	defer w.wg.Done()

	timer := time.NewTimer(w.debounce)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	scheduled := false
	schedule := func() {
		if scheduled {
			timer.Reset(w.debounce)
			return
		}
		timer.Reset(w.debounce)
		scheduled = true
	}

	for {
		select {
		case <-w.done:
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if w.handleEvent(event) {
				schedule()
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			logger.Warn("watcher: error", "error", err.Error())

		case <-timer.C:
			scheduled = false
			w.flush()
		}
	}
}

// handleEvent classifies one filesystem event and reports whether it is worth
// debouncing.
func (w *Watcher) handleEvent(event fsnotify.Event) bool {
	path := filepath.Clean(event.Name)

	if event.Op&fsnotify.Remove != 0 {
		if !fsutil.DirExists(path) {
			w.mu.Lock()
			w.removes[path] = time.Now()
			delete(w.watched, path)
			w.mu.Unlock()
			return true
		}
		return false
	}

	if event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Rename != 0 {
		if fsutil.DirExists(path) {
			if strings.HasPrefix(filepath.Base(path), ".") {
				return false
			}
			// Follow the new folder so games extracted into it are seen, and
			// record it as a candidate in case it already holds a launcher.
			w.addRecursive(path, 0)
			w.mu.Lock()
			w.events[path] = time.Now()
			w.mu.Unlock()
			return true
		}
		// A file appeared: a launcher dropped into an existing folder turns that
		// folder into a game.
		if platform.IsLauncher(path) && !strings.Contains(path, string(filepath.Separator)+".") {
			dir := filepath.Dir(path)
			w.mu.Lock()
			w.changed[dir] = time.Now()
			w.mu.Unlock()
			return true
		}
	}
	return false
}

// flush drains the pending sets and hands one batch to the handler.
func (w *Watcher) flush() {
	w.mu.Lock()
	newDirs := make([]string, 0, len(w.events))
	for dir := range w.events {
		if !fsutil.DirExists(dir) {
			continue
		}
		// Only report folders that actually look like a game now.
		if !hasLauncher(dir) {
			continue
		}
		newDirs = append(newDirs, dir)
	}
	changedDirs := make([]string, 0, len(w.changed))
	for dir := range w.changed {
		if hasLauncher(dir) {
			changedDirs = append(changedDirs, dir)
		}
	}
	removedDirs := make([]string, 0, len(w.removes))
	for dir := range w.removes {
		removedDirs = append(removedDirs, dir)
	}
	w.events = make(map[string]time.Time)
	w.changed = make(map[string]time.Time)
	w.removes = make(map[string]time.Time)
	handler := w.handler
	w.mu.Unlock()

	if len(newDirs) == 0 && len(changedDirs) == 0 && len(removedDirs) == 0 {
		return
	}

	batch := Events{NewGameDirs: newDirs, ChangedDirs: changedDirs, RemovedDirs: removedDirs}
	if len(removedDirs) > 0 {
		logger.WatcherRemovedDirs(len(removedDirs))
	}
	if len(newDirs) > 0 {
		logger.WatcherNewDirs(len(newDirs))
	}
	if handler != nil {
		handler(batch)
	}
}

// hasLauncher reports whether a directory directly contains a launchable file.
func hasLauncher(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if platform.IsLauncher(entry.Name()) {
			return true
		}
	}
	return false
}

// Stop shuts the watcher down. It is safe to call more than once.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.done)
		if w.fsw != nil {
			_ = w.fsw.Close()
		}
		w.wg.Wait()
	})
}
