package library

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"GameLibrary/internal/game"
)

// fixture creates a game directory on disk with a saved record.
func fixture(t *testing.T, root, rel string) *game.GameInfo {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatalf("write launcher: %v", err)
	}
	info := game.New(root, dir, []game.Executable{{Path: "game.exe", Name: "game", Primary: true}}, "")
	if err := info.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	return info
}

func TestPutAndGetReturnIndependentCopies(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	info := fixture(t, root, "Games/Alpha")
	store.Put(info)

	got, ok := store.Get(info.ID)
	if !ok {
		t.Fatal("expected the game to be present")
	}
	if got == info {
		t.Fatal("Get must hand out a copy, not the cached pointer")
	}

	// Mutating what the caller received must not touch the cache.
	got.Title = "Mutated"
	got.Tags = append(got.Tags, "evil")
	got.Metadata = &game.Metadata{Description: "nope"}

	again, _ := store.Get(info.ID)
	if again.Title != info.Title {
		t.Errorf("cache title changed through a returned record: %q", again.Title)
	}
	if len(again.Tags) != 0 {
		t.Errorf("cache tags changed through a returned record: %v", again.Tags)
	}
	if again.Metadata != nil {
		t.Errorf("cache metadata changed through a returned record: %+v", again.Metadata)
	}
}

func TestGetUnknownID(t *testing.T) {
	store := New(t.TempDir())
	if _, ok := store.Get("nope"); ok {
		t.Fatal("expected ok=false for an unknown id")
	}
}

func TestListSortedByRecentPlay(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	alpha := fixture(t, root, "Games/Alpha")
	beta := fixture(t, root, "Games/Beta")
	gamma := fixture(t, root, "Games/Gamma")

	alpha.LastPlayedAt = "2026-01-01T00:00:00Z"
	beta.LastPlayedAt = "2026-03-01T00:00:00Z"
	store.ReplaceAll([]*game.GameInfo{alpha, beta, gamma})

	list := store.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 games, got %d", len(list))
	}
	if list[0].ID != beta.ID || list[1].ID != alpha.ID {
		t.Fatalf("expected beta then alpha, got %s then %s", list[0].Title, list[1].Title)
	}
	if list[2].ID != gamma.ID {
		t.Fatalf("expected the never-played game last, got %s", list[2].Title)
	}
}

func TestReplaceAllDropsRemovedGames(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	alpha := fixture(t, root, "Games/Alpha")
	beta := fixture(t, root, "Games/Beta")
	store.ReplaceAll([]*game.GameInfo{alpha, beta})
	if store.Len() != 2 {
		t.Fatalf("expected 2 games, got %d", store.Len())
	}

	// nil entries and records without an ID are ignored rather than cached.
	store.ReplaceAll([]*game.GameInfo{alpha, nil, {Title: "no id"}})
	if store.Len() != 1 {
		t.Fatalf("expected 1 game, got %d", store.Len())
	}
	if _, ok := store.Get(beta.ID); ok {
		t.Fatal("a game missing from ReplaceAll should have been dropped")
	}
}

func TestMutatePersistsAndReturnsCopy(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	info := fixture(t, root, "Games/Alpha")
	store.Put(info)

	saved, err := store.Mutate(info.ID, func(draft *game.GameInfo) error {
		draft.Starred = true
		draft.AddTag("rpg")
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if !saved.Starred || len(saved.Tags) != 1 {
		t.Fatalf("unexpected result: %+v", saved)
	}

	// The change has to be on disk, not only in memory.
	reloaded, err := game.LoadFromDir(info.GameDir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.Starred || len(reloaded.Tags) != 1 {
		t.Errorf("change was not persisted: %+v", reloaded)
	}
}

func TestMutateRollsBackOnError(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	info := fixture(t, root, "Games/Alpha")
	store.Put(info)

	sentinel := errors.New("nope")
	if _, err := store.Mutate(info.ID, func(draft *game.GameInfo) error {
		draft.Title = "should not stick"
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the callback error, got %v", err)
	}

	got, _ := store.Get(info.ID)
	if got.Title == "should not stick" {
		t.Error("a failing mutation leaked into the cache")
	}
	reloaded, _ := game.LoadFromDir(info.GameDir)
	if reloaded.Title == "should not stick" {
		t.Error("a failing mutation was written to disk")
	}
}

func TestMutateUnknownID(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.Mutate("nope", func(*game.GameInfo) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReplaceRequiresExistingGame(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	info := fixture(t, root, "Games/Alpha")
	store.Put(info)

	updated, err := store.Replace(info)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if updated.ID != info.ID {
		t.Fatalf("unexpected id %q", updated.ID)
	}

	stranger := fixture(t, root, "Games/Beta")
	if _, err := store.Replace(stranger); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an uncached game, got %v", err)
	}
}

func TestRemoveAndRemoveWithin(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	alpha := fixture(t, root, "Games/Alpha")
	beta := fixture(t, root, "Games/Beta")
	store.ReplaceAll([]*game.GameInfo{alpha, beta})

	if !store.Remove(alpha.ID) {
		t.Fatal("expected Remove to report true")
	}
	if store.Remove(alpha.ID) {
		t.Fatal("expected Remove to report false the second time")
	}

	removed := store.RemoveWithin(filepath.Join(root, "Games"))
	if len(removed) != 1 || removed[0] != beta.ID {
		t.Fatalf("expected beta to be removed, got %v", removed)
	}
	if store.Len() != 0 {
		t.Fatalf("expected an empty store, got %d", store.Len())
	}

	stranger := fixture(t, root, "Elsewhere/Other")
	store.Put(alpha)
	store.Put(stranger)
	if removed := store.RemoveWithin(filepath.Join(root, "Games")); len(removed) != 1 || removed[0] != alpha.ID {
		t.Fatalf("RemoveWithin must not touch siblings, got %v", removed)
	}
}

func TestCoverPath(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	info := fixture(t, root, "Games/Alpha")
	store.Put(info)

	if got := store.CoverPath(info.ID, false); got != "" {
		t.Fatalf("expected no cover, got %q", got)
	}
	if got := store.CoverPath("unknown", false); got != "" {
		t.Fatalf("expected no cover for an unknown id, got %q", got)
	}

	// A directory named like a cover is not a cover.
	coverDir := game.CoverDir(info.GameDir)
	if err := os.MkdirAll(filepath.Join(coverDir, "cover.jpg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := store.CoverPath(info.ID, false); got != "" {
		t.Fatalf("a directory must not be treated as a cover, got %q", got)
	}

	// The managed location wins over the legacy game-root location.
	if err := os.WriteFile(filepath.Join(info.GameDir, "cover.png"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := store.CoverPath(info.ID, false)
	if legacy == "" || filepath.Dir(legacy) != info.GameDir {
		t.Fatalf("expected the game-root cover to be found, got %q", legacy)
	}

	if err := os.RemoveAll(filepath.Join(coverDir, "cover.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, "cover.jpg"), []byte("managed"), 0o644); err != nil {
		t.Fatal(err)
	}
	managed := store.CoverPath(info.ID, false)
	if filepath.Dir(managed) != coverDir {
		t.Fatalf("expected the managed cover to win, got %q", managed)
	}

	if err := os.WriteFile(filepath.Join(coverDir, "cover_landscape.webp"), []byte("wide"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := store.CoverPath(info.ID, true); filepath.Ext(got) != ".webp" {
		t.Fatalf("expected the landscape webp, got %q", got)
	}
}

func TestGameDirOfAndWithinRoot(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	info := fixture(t, root, "Games/Alpha")
	store.Put(info)

	if got := store.GameDirOf(info.ID); got != info.GameDir {
		t.Errorf("expected %q, got %q", info.GameDir, got)
	}
	if got := store.GameDirOf("unknown"); got != "" {
		t.Errorf("expected an empty path, got %q", got)
	}
	if !store.WithinRoot(filepath.Join(root, "Games")) {
		t.Error("expected the library subdirectory to be within the root")
	}
	if store.WithinRoot(filepath.Join(root, "..")) {
		t.Error("expected a parent directory to be outside the root")
	}
}

// TestStoreIsRaceFree mirrors how the application uses the cache: several IPC
// readers, the watcher's scan goroutine and queue-driven mutations all at once.
// It only means something under -race, which CI runs.
func TestStoreIsRaceFree(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	ids := make([]string, 0, 6)
	for _, name := range []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta"} {
		info := fixture(t, root, "Games/"+name)
		store.Put(info)
		ids = append(ids, info.ID)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = store.Get(ids[0])
				_ = store.List()
				_ = store.Len()
				_ = store.IDs()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			_, _ = store.Mutate(ids[i%len(ids)], func(draft *game.GameInfo) error {
				draft.Starred = !draft.Starred
				draft.AddTag("t")
				return nil
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			store.RemoveWithin(filepath.Join(root, "Games", "Nonexistent"))
		}
	}()

	// Readers loop until told to stop, so the signal has to come before waiting.
	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestNormalizeTitle(t *testing.T) {
	if got := NormalizeTitle("  Steins   Gate  "); got != "steins gate" {
		t.Errorf("unexpected normalisation: %q", got)
	}
}
