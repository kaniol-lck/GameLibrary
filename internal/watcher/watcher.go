package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"GameLibrary/internal/logger"

	"github.com/fsnotify/fsnotify"
)

type Callback func(newDirs []string)

type RemoveCallback func(removedDirs []string)

type Watcher struct {
	mu         sync.Mutex
	w          *fsnotify.Watcher
	dirs       map[string]bool
	events     map[string]time.Time
	removes    map[string]time.Time
	callback   Callback
	rmCallback RemoveCallback
	done       chan struct{}
	debounce   time.Duration
}

func New(callback Callback, rmCallback RemoveCallback, debounceMs int) *Watcher {
	if debounceMs <= 0 {
		debounceMs = 100
	}
	return &Watcher{
		dirs:       make(map[string]bool),
		events:     make(map[string]time.Time),
		removes:    make(map[string]time.Time),
		callback:   callback,
		rmCallback: rmCallback,
		done:       make(chan struct{}),
		debounce:   time.Duration(debounceMs) * time.Millisecond,
	}
}

func (w *Watcher) WatchDirs(dirs []string, exeDir string) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.w = fw

	for _, dir := range dirs {
		absDir := filepath.Clean(dir)
		if !filepath.IsAbs(absDir) {
			absDir = filepath.Join(exeDir, dir)
		}
		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			continue
		}
		w.addRecursive(absDir, 3)
	}

	go w.loop()
	return nil
}

func (w *Watcher) addRecursive(dir string, depth int) {
	if depth < 0 {
		return
	}
	absDir := filepath.Clean(dir)
	if w.dirs[absDir] {
		return
	}
	w.w.Add(absDir)
	w.dirs[absDir] = true

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		w.addRecursive(filepath.Join(absDir, e.Name()), depth-1)
	}
}

func (w *Watcher) loop() {
	timer := time.NewTimer(w.debounce)
	timer.Stop()

	for {
		select {
		case <-w.done:
			timer.Stop()
			return
		case evt, ok := <-w.w.Events:
			if !ok {
				return
			}
			dir := evt.Name
			if evt.Op&fsnotify.Remove != 0 {
				if info, _ := os.Stat(dir); info == nil {
					w.mu.Lock()
					w.removes[dir] = time.Now()
					w.mu.Unlock()
					timer.Reset(100 * time.Millisecond)
				}
				continue
			}
			if evt.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					if !strings.HasPrefix(filepath.Base(dir), ".") {
						w.mu.Lock()
						w.events[dir] = time.Now()
						w.mu.Unlock()
						timer.Reset(w.debounce)
					}
				}
			}
		case <-timer.C:
			w.flush()
		}
	}
}

func (w *Watcher) flush() {
	w.mu.Lock()
	hasEvents := len(w.events) > 0
	hasRemoves := len(w.removes) > 0

	if hasRemoves && w.rmCallback != nil {
		removed := make([]string, 0, len(w.removes))
		for dir := range w.removes {
			removed = append(removed, dir)
		}
		w.removes = make(map[string]time.Time)
		w.mu.Unlock()
		logger.Info("watcher: game dirs removed", "count", len(removed))
		w.rmCallback(removed)
		w.mu.Lock()
	}

	if hasEvents {
		var newDirs []string
		for dir := range w.events {
			if hasExe(dir) {
				newDirs = append(newDirs, dir)
			}
		}
		w.events = make(map[string]time.Time)
		w.mu.Unlock()

		if len(newDirs) > 0 && w.callback != nil {
			logger.Info("watcher: new game dirs detected", "count", len(newDirs))
			w.callback(newDirs)
		}
	} else {
		w.mu.Unlock()
	}
}

func hasExe(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".exe") && !strings.HasPrefix(name, "unins") {
				return true
			}
		}
	}
	return false
}

func (w *Watcher) Stop() {
	close(w.done)
	if w.w != nil {
		w.w.Close()
	}
}
