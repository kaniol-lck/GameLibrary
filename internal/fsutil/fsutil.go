// Package fsutil provides path handling and durable file writes shared by the
// rest of the application.
//
// The library root usually lives on a NAS share, so two properties matter
// everywhere:
//
//   - Paths may be written by a human in either separator style, with or
//     without a leading "./". Resolve and Normalize accept both.
//   - A half-written JSON file is worse than a missing one (it can silently
//     drop a game's metadata). WriteFileAtomic never leaves a partial file
//     behind.
package fsutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve turns a possibly-relative, possibly Unix-style path into an absolute
// OS-native path rooted at base when the input is relative.
//
// Accepted forms:
//
//	".\\Games"        -> base\Games
//	"./Games"         -> base\Games
//	"../shared/Games" -> <parent of base>\shared\Games
//	"E:\\SteamLibrary" -> E:\SteamLibrary   (absolute, returned as-is)
//	""                -> ""                  (caller decides what that means)
func Resolve(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = ToNative(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if base == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

// ToNative converts any mix of separators to the host OS separator.
func ToNative(path string) string {
	if path == "" {
		return ""
	}
	// Normalise to forward slashes first so FromSlash can do a single pass,
	// then let the host decide. This keeps Windows-style input working on
	// Unix hosts (where "\" is a legal filename byte) and vice versa.
	unified := strings.ReplaceAll(path, `\`, "/")
	return filepath.FromSlash(unified)
}

// maxRelativeClimb is how many ".." levels ToPortable is willing to keep in a
// stored relative path before falling back to an absolute one. A library root
// whose game folders sit a couple of levels up (a NAS layout such as
// "<share>\apps\GameLibrary" with games in "<share>\Games") is still portable
// across drive letters; anything further out usually is not.
const maxRelativeClimb = 3

// ToPortable converts an absolute path into the form stored in config.json.
//
// Paths at or near the library root become root-relative (".\Games") so the same
// config.json works no matter which drive letter a client mounts the share on.
// Paths that cannot be expressed that way are kept absolute. Separators are
// always stored as "\" so a config written on Windows reads identically on every
// client.
func ToPortable(root, abs string) string {
	abs = filepath.Clean(abs)
	if root == "" {
		return toStoredStyle(abs)
	}
	root = filepath.Clean(root)

	if rel, err := filepath.Rel(root, abs); err == nil && countClimb(rel) <= maxRelativeClimb {
		if rel == "." {
			return "."
		}
		return "." + string(filepath.Separator) + rel
	}
	return toStoredStyle(abs)
}

// countClimb returns the number of leading ".." components in a relative path,
// or a value above maxRelativeClimb when the path is not relative at all.
func countClimb(rel string) int {
	climb := 0
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".." {
			climb++
			continue
		}
		break
	}
	return climb
}

func toStoredStyle(p string) string {
	return strings.ReplaceAll(filepath.Clean(p), "/", `\`)
}

// IsWithin reports whether path is root itself or lives underneath it.
func IsWithin(root, path string) bool {
	root = filepath.Clean(ToNative(root))
	path = filepath.Clean(ToNative(path))
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// WriteFileAtomic writes data to path without ever exposing a partial file.
//
// The data is written to a temporary file in the same directory, flushed to
// stable storage, and then renamed over the destination. A rename within a
// directory is atomic on both NTFS and SMB, so a crash or a dropped network
// share leaves either the old file or the new one, never a truncated one.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Best-effort cleanup for every failure path below.
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	// On Windows os.Rename fails if the destination exists and is open
	// elsewhere; retry once after removing the destination. Readers only ever
	// see either the old or the new file because we never write in place.
	if err := os.Rename(tmpName, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("replace %s: %w", path, err)
		}
		if err := os.Rename(tmpName, path); err != nil {
			return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
		}
	}
	return nil
}

// ReadFile reads a file, mapping "does not exist" to os.ErrNotExist so callers
// can branch on a single sentinel.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

// StripBOM removes a leading byte order mark from file contents.
//
// Every JSON file the application reads is one a human may have edited with
// Notepad, PowerShell or another Windows tool that writes a BOM. encoding/json
// does not skip one, so a single invisible prefix byte made the whole document
// fail to parse — for config.json that silently discarded every setting and fell
// back to defaults. This is applied at every JSON decode site.
func StripBOM(data []byte) []byte {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8
	data = bytes.TrimPrefix(data, []byte{0xFF, 0xFE})       // UTF-16 LE
	data = bytes.TrimPrefix(data, []byte{0xFE, 0xFF})       // UTF-16 BE
	return data
}

// Exists reports whether path exists as a regular file.
func Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DirExists reports whether path exists as a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
