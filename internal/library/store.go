// Package library owns the in-memory game cache.
//
// The cache is read and written concurrently by four independent actors: Wails
// IPC handlers (one goroutine per call), the scrape queue worker, the filesystem
// watcher callback, and the background post-scan task. Before this package
// existed those actors shared a plain map behind no lock, which is a data race
// and could crash the webview call outright.
//
// Every read hands out a deep copy, so callers may freely mutate what they get
// without corrupting the cache, and every write goes through a single locked
// critical section that also persists the record.
package library

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
)

// Store is a concurrency-safe cache of game records keyed by game ID.
type Store struct {
	mu    sync.RWMutex
	root  string
	games map[string]*game.GameInfo
}

// New creates an empty store. root is the library root directory (the folder
// containing the executable) used for portable identity and path checks.
func New(root string) *Store {
	return &Store{
		root:  root,
		games: make(map[string]*game.GameInfo),
	}
}

// Root returns the library root directory.
func (s *Store) Root() string { return s.root }

// Len returns the number of cached games.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.games)
}

// Get returns a copy of a record, or false when the ID is unknown.
func (s *Store) Get(id string) (*game.GameInfo, bool) {
	s.mu.RLock()
	info, ok := s.games[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return info.Clone(), true
}

// List returns a copy of every record, most recently played first.
func (s *Store) List() []*game.GameInfo {
	s.mu.RLock()
	out := make([]*game.GameInfo, 0, len(s.games))
	for _, info := range s.games {
		out = append(out, info.Clone())
	}
	s.mu.RUnlock()

	game.SortByRecentPlay(out)
	return out
}

// IDs returns the cached game IDs.
func (s *Store) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.games))
	for id := range s.games {
		ids = append(ids, id)
	}
	return ids
}

// Put inserts or replaces a record.
func (s *Store) Put(info *game.GameInfo) {
	if info == nil || info.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[info.ID] = info.Clone()
}

// ReplaceAll swaps the whole cache for a freshly loaded set.
func (s *Store) ReplaceAll(infos []*game.GameInfo) {
	next := make(map[string]*game.GameInfo, len(infos))
	for _, info := range infos {
		if info == nil || info.ID == "" {
			continue
		}
		next[info.ID] = info.Clone()
	}
	s.mu.Lock()
	s.games = next
	s.mu.Unlock()
}

// Remove drops a record and reports whether it was present.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.games[id]; !ok {
		return false
	}
	delete(s.games, id)
	return true
}

// RemoveWithin drops every game whose directory lies inside dir and returns the
// IDs that were removed. It is used when the watcher reports a game folder
// disappearing from the library.
func (s *Store) RemoveWithin(dir string) []string {
	if dir == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var removed []string
	for id, info := range s.games {
		if fsutil.IsWithin(dir, info.GameDir) {
			delete(s.games, id)
			removed = append(removed, id)
		}
	}
	return removed
}

// Mutate applies fn to a record and persists the result inside a single locked
// critical section, so concurrent edits cannot lose each other's changes.
//
// fn receives the live record; returning an error aborts the write and leaves
// the cache untouched. The returned value is a copy of the saved record.
func (s *Store) Mutate(id string, fn func(*game.GameInfo) error) (*game.GameInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, ok := s.games[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Work on a copy so a failing fn cannot leave a half-mutated cache entry.
	draft := info.Clone()
	if err := fn(draft); err != nil {
		return nil, err
	}
	if err := draft.Save(); err != nil {
		return nil, err
	}
	s.games[id] = draft
	return draft.Clone(), nil
}

// Replace swaps in a fully computed record for an existing game.
//
// Scraping works on a detached copy of the record, so this is how the finished
// result re-enters the cache: it validates the identity, persists the record and
// updates the cache in one locked step.
func (s *Store) Replace(info *game.GameInfo) (*game.GameInfo, error) {
	if info == nil || info.ID == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.games[info.ID]; !ok {
		return nil, ErrNotFound
	}
	if err := info.Save(); err != nil {
		return nil, err
	}
	s.games[info.ID] = info.Clone()
	return info.Clone(), nil
}

// Save persists a record already held by the store, keeping the cache in sync.
func (s *Store) Save(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.games[id]
	if !ok {
		return ErrNotFound
	}
	return info.Save()
}

// CoverPath resolves the on-disk cover file for a game, or "" when there is
// none. The legacy game-root location is still honoured so libraries that have
// not been re-scraped keep working.
func (s *Store) CoverPath(id string, landscape bool) string {
	info, ok := s.Get(id)
	if !ok {
		return ""
	}
	stem := game.CoverName
	if landscape {
		stem = game.CoverLandscapeName
	}
	for _, dir := range []string{game.CoverDir(info.GameDir), info.GameDir} {
		for _, ext := range coverExtensions {
			path := filepath.Join(dir, stem+ext)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}

// coverExtensions is ordered by preference: JPEG first because the scraper
// stores fetched art with the extension implied by its content type.
var coverExtensions = []string{".jpg", ".jpeg", ".png", ".webp"}

// GameDirOf returns the directory of a cached game, or "".
func (s *Store) GameDirOf(id string) string {
	info, ok := s.Get(id)
	if !ok {
		return ""
	}
	return info.GameDir
}

// WithinRoot reports whether a directory sits inside the library root. Used to
// decide how a path should be displayed and stored.
func (s *Store) WithinRoot(dir string) bool {
	return fsutil.IsWithin(s.root, dir)
}

// NormalizeTitle is a small helper used by scrapers and the UI when a title has
// to be compared loosely.
func NormalizeTitle(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
