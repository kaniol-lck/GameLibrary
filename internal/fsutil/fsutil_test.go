package fsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- helpers ---------------------------------------------------------------

// storedDir is the canonical stored form of a directory below the library root
// (".\Games"). The separator is host specific, so tests must never hard-code it.
func storedDir(rel string) string {
	return "." + string(filepath.Separator) + rel
}

// storedAbs is the canonical stored form of a path that cannot be expressed
// relative to the root: cleaned, with forward slashes rewritten to backslashes.
func storedAbs(p string) string {
	return strings.ReplaceAll(filepath.Clean(p), "/", `\`)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

// dirNames returns the sorted listing of dir, which is what the "no temporary
// file left behind" assertions compare against.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// assertOnly fails unless dir contains exactly the given entries (ReadDir
// returns them sorted) and none of them is a temporary file. This is what
// catches a temp file the deferred cleanup failed to remove.
func assertOnly(t *testing.T, dir string, want ...string) {
	t.Helper()
	got := dirNames(t, dir)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("directory %s contains %v, want exactly %v", dir, got, want)
	}
	for _, name := range got {
		if strings.Contains(name, ".tmp") {
			t.Errorf("temporary file %q was left behind in %s", name, dir)
		}
	}
}

func hostPath(elems ...string) string { return filepath.Join(elems...) }

// --- Resolve ---------------------------------------------------------------

func TestResolve(t *testing.T) {
	base := t.TempDir()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty stays empty so the caller can decide", "", ""},
		{"bare relative name", "Games", hostPath(base, "Games")},
		{"windows style relative path", `.\Games`, hostPath(base, "Games")},
		{"unix style relative path", "./Games", hostPath(base, "Games")},
		{"mixed separators and a trailing separator", `.\Games/`, hostPath(base, "Games")},
		{"padded with whitespace", "  ./Games  ", hostPath(base, "Games")},
		{"nested mixed separators", `Games/sub\child`, hostPath(base, "Games", "sub", "child")},
		{"a bare dot is the base itself", ".", base},
		{"a trailing separator is cleaned away", "./Games/sub/", hostPath(base, "Games", "sub")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(base, tc.input); got != tc.want {
				t.Errorf("Resolve(%q, %q) = %q, want %q", base, tc.input, got, tc.want)
			}
		})
	}

	t.Run("Games, .\\Games and ./Games agree", func(t *testing.T) {
		want := Resolve(base, "Games")
		for _, spelling := range []string{"Games", `.\Games`, "./Games", `Games\`, "Games/"} {
			if got := Resolve(base, spelling); got != want {
				t.Errorf("Resolve(%q) = %q, want %q (all spellings must agree)", spelling, got, want)
			}
		}
		if !filepath.IsAbs(want) {
			t.Errorf("Resolve must return an absolute path under a base, got %q", want)
		}
	})

	t.Run("climbs out of the base with ..", func(t *testing.T) {
		got := Resolve(base, "../shared")
		if want := filepath.Clean(hostPath(base, "..", "shared")); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", "../shared", got, want)
		}
		if IsWithin(base, got) {
			t.Errorf("%q climbed out of %q and must not be reported as within it", got, base)
		}
		if got := Resolve(base, `..\..\shared`); got != filepath.Clean(hostPath(base, "..", "..", "shared")) {
			t.Errorf(`Resolve("..\..\shared") = %q`, got)
		}
	})

	t.Run("an absolute path is returned cleaned", func(t *testing.T) {
		sep := string(filepath.Separator)
		abs := hostPath(base, "Games") + sep + "sub" + sep + ".." + sep + "Games2"
		want := hostPath(base, "Games", "Games2")

		got := Resolve(base, abs)
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", abs, got, want)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("Resolve(%q) = %q, want an absolute path", abs, got)
		}
		// An absolute path does not need a base.
		if got := Resolve("", abs); got != want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", "", abs, got, want)
		}
	})

	t.Run("a relative path without a base stays relative", func(t *testing.T) {
		if got := Resolve("", "Games"); got != "Games" {
			t.Errorf(`Resolve("", "Games") = %q, want "Games"`, got)
		}
		if got := Resolve("", `.\Games`); got != "Games" {
			t.Errorf(`Resolve("", ".\Games") = %q, want "Games"`, got)
		}
	})
}

// --- ToNative --------------------------------------------------------------

func TestToNative(t *testing.T) {
	sep := string(filepath.Separator)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"unix separators", "Games/Sub", "Games" + sep + "Sub"},
		{"windows separators", `Games\Sub`, "Games" + sep + "Sub"},
		{"mixed separators", `Games/Sub\Child`, "Games" + sep + "Sub" + sep + "Child"},
		{"leading ./", "./Games", "." + sep + "Games"},
		{"parent hops", `..\shared/Sub`, ".." + sep + "shared" + sep + "Sub"},
		{"no separator at all", "game.exe", "game.exe"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToNative(tc.input); got != tc.want {
				t.Errorf("ToNative(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	t.Run("absolute windows path", func(t *testing.T) {
		got := ToNative(`C:\Games\Sub`)
		if runtime.GOOS == "windows" {
			if want := `C:\Games\Sub`; got != want {
				t.Errorf("ToNative = %q, want %q", got, want)
			}
			return
		}
		if want := "C:/Games/Sub"; got != want {
			t.Errorf("ToNative = %q, want %q", got, want)
		}
	})
}

// --- ToPortable ------------------------------------------------------------

func TestToPortable(t *testing.T) {
	base := t.TempDir()
	// The root sits four levels below base so that the climb limit (3) can be
	// crossed without ever reaching the volume root.
	root := hostPath(base, "a", "b", "c", "d")

	cases := []struct {
		name string
		abs  string
		want string
	}{
		{"the root itself", root, "."},
		{"a direct child", hostPath(root, "Games"), storedDir("Games")},
		{"a nested child", hostPath(root, "Games", "Sub"), storedDir(hostPath("Games", "Sub"))},
		{"one level above the root", hostPath(base, "a", "b", "c"), storedDir("..")},
		{"two levels above the root", hostPath(base, "a", "b"), storedDir(hostPath("..", ".."))},
		{"three levels above the root is still at the limit", hostPath(base, "a"), storedDir(hostPath("..", "..", ".."))},
		{"four levels above the root is beyond the limit", base, storedAbs(base)},
		{"a sibling next to the root", hostPath(base, "elsewhere"), storedAbs(hostPath(base, "elsewhere"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToPortable(root, tc.abs)
			if got != tc.want {
				t.Errorf("ToPortable(%q, %q) = %q, want %q", root, tc.abs, got, tc.want)
			}
		})
	}

	t.Run("a stored absolute path uses backslashes and is clean", func(t *testing.T) {
		sep := string(filepath.Separator)
		// Beyond the climb limit, and with redundant components to clean up.
		abs := base + sep + "sub" + sep + ".." + sep + "elsewhere"
		got := ToPortable(root, abs)
		if want := storedAbs(hostPath(base, "elsewhere")); got != want {
			t.Errorf("ToPortable(%q) = %q, want %q", abs, got, want)
		}
		if strings.Contains(got, "/") {
			t.Errorf("stored path %q must not contain forward slashes", got)
		}
	})

	t.Run("a root-relative path keeps the host separator", func(t *testing.T) {
		got := ToPortable(root, hostPath(root, "Games"))
		if got != "."+string(filepath.Separator)+"Games" {
			t.Errorf("ToPortable = %q, want the root-relative stored form", got)
		}
	})

	t.Run("without a root the path is stored absolute", func(t *testing.T) {
		abs := hostPath(root, "Games")
		if got, want := ToPortable("", abs), storedAbs(abs); got != want {
			t.Errorf("ToPortable(%q, %q) = %q, want %q", "", abs, got, want)
		}
	})

	t.Run("a path on another volume is stored absolute", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("drive letters are a Windows concept")
		}
		other := `D:\SteamLibrary`
		if strings.EqualFold(filepath.VolumeName(base), "D:") {
			other = `C:\SteamLibrary`
		}
		got := ToPortable(root, other)
		if want := storedAbs(other); got != want {
			t.Errorf("ToPortable(%q, %q) = %q, want %q", root, other, got, want)
		}
		if strings.Contains(got, "/") {
			t.Errorf("stored path %q must not contain forward slashes", got)
		}
	})
}

// --- IsWithin --------------------------------------------------------------

func TestIsWithin(t *testing.T) {
	base := t.TempDir()
	root := hostPath(base, "Games")
	sep := string(filepath.Separator)

	cases := []struct {
		name string
		root string
		path string
		want bool
	}{
		{"the root itself", root, root, true},
		{"the root with a trailing separator", root + sep, root, true},
		{"a direct child", root, hostPath(root, "Sub"), true},
		{"a nested child", root, hostPath(root, "Sub", "Deep"), true},
		{"the parent of the root", root, base, false},
		{"an unrelated sibling", root, hostPath(base, "Other"), false},
		// Regression guard for the classical prefix bug: a textual check such as
		// strings.HasPrefix(path, root) reports C:\Games2 as being inside
		// C:\Games, because "C:\Games2" starts with "C:\Games". IsWithin uses
		// filepath.Rel, which compares whole path components and says no.
		{"a textually similar sibling", root, hostPath(base, "Games2"), false},
		{"the literal prefix case on Windows paths", `C:\Games`, `C:\Games2`, false},
		{"a similar sibling with an underscore", `C:\Games`, `C:\Games_backup`, false},
		{"mixed separators in the child", root, filepath.ToSlash(hostPath(root, "Sub")), true},
		{"an empty root is never a container", "", root, false},
		{"an empty path is never contained", root, "", false},
		{"a path that only shares the volume", root, filepath.VolumeName(root) + sep, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsWithin(tc.root, tc.path); got != tc.want {
				t.Errorf("IsWithin(%q, %q) = %v, want %v", tc.root, tc.path, got, tc.want)
			}
		})
	}

	t.Run("a path on another volume", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("drive letters are a Windows concept")
		}
		// filepath.Rel cannot relate two volumes, which must not be mistaken for
		// containment.
		other := `D:\Games\Sub`
		if strings.EqualFold(filepath.VolumeName(root), "D:") {
			other = `C:\Games\Sub`
		}
		if IsWithin(root, other) {
			t.Errorf("IsWithin(%q, %q) = true, want false", root, other)
		}
	})
}

// --- WriteFileAtomic -------------------------------------------------------

func TestWriteFileAtomicWritesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	// The intermediate directories do not exist yet: WriteFileAtomic has to
	// create them.
	path := hostPath(dir, "Games", "nested", "gameinfo.json")
	innerDir := filepath.Dir(path)

	if err := WriteFileAtomic(path, []byte("first payload"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	if got := mustRead(t, path); got != "first payload" {
		t.Errorf("content = %q, want %q", got, "first payload")
	}
	if !Exists(path) {
		t.Error("Exists reports false for a file WriteFileAtomic just wrote")
	}
	if DirExists(path) {
		t.Error("DirExists reports true for a regular file")
	}

	// Exactly the expected names: the temporary file used for the rename must
	// already be gone by the time WriteFileAtomic returns.
	assertOnly(t, dir, "Games")
	assertOnly(t, innerDir, "gameinfo.json")

	if err := WriteFileAtomic(path, []byte("second payload, deliberately longer"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic (overwrite): %v", err)
	}
	if got := mustRead(t, path); got != "second payload, deliberately longer" {
		t.Errorf("content after overwrite = %q", got)
	}
	assertOnly(t, innerDir, "gameinfo.json")

	// A shorter payload must truncate the file rather than leave a tail behind.
	if err := WriteFileAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic (short overwrite): %v", err)
	}
	if got := mustRead(t, path); got != "x" {
		t.Errorf("content after short overwrite = %q, want %q", got, "x")
	}
	assertOnly(t, innerDir, "gameinfo.json")

	// An empty payload is a legitimate value, not a missing file.
	if err := WriteFileAtomic(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFileAtomic (empty): %v", err)
	}
	if got := mustRead(t, path); got != "" {
		t.Errorf("content after empty write = %q, want %q", got, "")
	}
	assertOnly(t, innerDir, "gameinfo.json")
}

func TestWriteFileAtomicFailsCleanlyWhenTheTargetIsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	blocker := hostPath(dir, "blocker")
	mustWrite(t, blocker, "this is a file, not a directory")

	err := WriteFileAtomic(hostPath(blocker, "gameinfo.json"), []byte("data"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic succeeded although the parent path is a regular file")
	}
	// The failure must not leave anything behind next to the blocker.
	assertOnly(t, dir, "blocker")
}

// TestWriteFileAtomicKeepsOldContentWhenTheDestinationIsLocked pins the state
// WriteFileAtomic leaves behind when it cannot replace the destination because
// another handle has it open (see the concurrency test for why that happens on
// Windows). Whatever the host does with the rename, the destination must end up
// holding either the old or the new bytes in full, and no temporary file may
// survive the failure.
func TestWriteFileAtomicKeepsOldContentWhenTheDestinationIsLocked(t *testing.T) {
	dir := t.TempDir()
	path := hostPath(dir, "gameinfo.json")
	mustWrite(t, path, "original")

	reader, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer reader.Close()

	if err := WriteFileAtomic(path, []byte("replacement"), 0o644); err != nil {
		// Expected on Windows: os.Open does not share delete access, so the
		// rename over the open destination is refused.
		t.Logf("write refused while the destination was open: %v", err)
	}

	if got := mustRead(t, path); got != "original" && got != "replacement" {
		t.Errorf("destination holds %q, want either %q or %q", got, "original", "replacement")
	}
	assertOnly(t, dir, "gameinfo.json")
}

// TestWriteFileAtomicConcurrentReaders hammers one path from two goroutines: one
// keeps replacing a large payload, the other keeps reading it.
//
// The invariant under test is that a reader never observes a truncated or
// interleaved payload: any read that returns data must return either the old or
// the new payload in full. os.ErrNotExist is also acceptable, because the
// Windows fallback removes the destination before renaming the replacement over
// it.
//
// KNOWN WINDOWS DEFECT (reported, not worked around in fsutil.go): while
// os.Rename replaces the destination, a read can additionally fail with "The
// process cannot access the file because it is being used by another process",
// and WriteFileAtomic itself fails with "Access is denied" whenever a reader
// holds the destination open, because os.Open asks for no delete sharing. The
// test therefore retries a *transient* failure and counts it, so the defect
// shows up in the verbose output instead of silently passing — but it never
// tolerates content: bytes matching neither payload fail the test immediately.
func TestWriteFileAtomicConcurrentReaders(t *testing.T) {
	dir := t.TempDir()
	path := hostPath(dir, "gameinfo.json")

	oldPayload := bytes.Repeat([]byte("A"), 256*1024)
	newPayload := bytes.Repeat([]byte("B"), 256*1024)

	if err := WriteFileAtomic(path, oldPayload, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	const (
		iterations = 100
		attempts   = 200
	)

	var (
		wg           sync.WaitGroup
		done         atomic.Bool
		reads        atomic.Int64
		absentReads  atomic.Int64
		lockedReads  atomic.Int64
		writeRetries atomic.Int64
		seenOld      atomic.Int64
		seenNew      atomic.Int64
	)
	failures := make(chan string, 16)
	report := func(msg string) {
		select {
		case failures <- msg:
		default:
		}
	}
	isPayload := func(data []byte) bool {
		if bytes.Equal(data, oldPayload) {
			seenOld.Add(1)
			return true
		}
		if bytes.Equal(data, newPayload) {
			seenNew.Add(1)
			return true
		}
		return false
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer done.Store(true)
		for i := 0; i < iterations; i++ {
			payload := oldPayload
			if i%2 == 1 {
				payload = newPayload
			}
			var err error
			for attempt := 0; attempt < attempts; attempt++ {
				if err = WriteFileAtomic(path, payload, 0o644); err == nil {
					break
				}
				writeRetries.Add(1)
				time.Sleep(time.Millisecond)
			}
			if err != nil {
				report(fmt.Sprintf("write %d failed after %d attempts: %v", i, attempts, err))
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; !done.Load() && i < 1_000_000; i++ {
			ok := false
			var lastErr error
			for attempt := 0; attempt < attempts; attempt++ {
				data, err := ReadFile(path)
				if err != nil {
					lastErr = err
					if errors.Is(err, os.ErrNotExist) {
						absentReads.Add(1)
					} else {
						lockedReads.Add(1)
					}
					time.Sleep(time.Millisecond)
					continue
				}
				if !isPayload(data) {
					report(fmt.Sprintf("read %d returned %d bytes matching neither payload", i, len(data)))
					return
				}
				ok = true
				break
			}
			if !ok {
				report(fmt.Sprintf("read %d never produced a complete payload: %v", i, lastErr))
				return
			}
			reads.Add(1)
		}
	}()
	wg.Wait()
	close(failures)

	for msg := range failures {
		t.Error(msg)
	}
	if reads.Load() == 0 {
		t.Error("the reader never observed the file, so the test proved nothing")
	}
	t.Logf("transients while replacing the file: absent=%d locked=%d, write retries=%d; complete reads=%d (old=%d new=%d)",
		absentReads.Load(), lockedReads.Load(), writeRetries.Load(),
		reads.Load(), seenOld.Load(), seenNew.Load())

	// The final state must be one of the two payloads, never a mix, and the
	// failed attempts above must not have left a temporary file behind.
	final, err := ReadFile(path)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	if !bytes.Equal(final, oldPayload) && !bytes.Equal(final, newPayload) {
		t.Errorf("final content is neither payload (%d bytes)", len(final))
	}
	assertOnly(t, dir, "gameinfo.json")
}

// --- ReadFile / Exists / DirExists -----------------------------------------

func TestReadFileMapsMissingFileToErrNotExist(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadFile(hostPath(dir, "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadFile error = %v, want os.ErrNotExist", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile error = %v, want fs.ErrNotExist", err)
	}

	present := hostPath(dir, "present.json")
	mustWrite(t, present, "hello")
	if got := mustRead(t, present); got != "hello" {
		t.Errorf("ReadFile = %q, want %q", got, "hello")
	}
}

func TestExistsAndDirExists(t *testing.T) {
	dir := t.TempDir()
	file := hostPath(dir, "game.exe")
	mustWrite(t, file, "MZ")
	sub := hostPath(dir, "Sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	missing := hostPath(dir, "missing")

	cases := []struct {
		name       string
		path       string
		wantExists bool
		wantDir    bool
	}{
		{"a regular file", file, true, false},
		{"a directory", sub, false, true},
		{"an absent path", missing, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Exists(tc.path); got != tc.wantExists {
				t.Errorf("Exists(%q) = %v, want %v", tc.path, got, tc.wantExists)
			}
			if got := DirExists(tc.path); got != tc.wantDir {
				t.Errorf("DirExists(%q) = %v, want %v", tc.path, got, tc.wantDir)
			}
		})
	}

	t.Run("an empty path exists as neither", func(t *testing.T) {
		if Exists("") {
			t.Error(`Exists("") = true, want false`)
		}
		if DirExists("") {
			t.Error(`DirExists("") = true, want false`)
		}
	})
}
