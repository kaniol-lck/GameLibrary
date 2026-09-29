package scraper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"GameLibrary/internal/fsutil"
	"GameLibrary/internal/game"
)

// Cover kinds. The stems double as the file names inside .gamemanager/covers.
const (
	CoverPortrait  = game.CoverName
	CoverLandscape = game.CoverLandscapeName
)

// maxCoverBytes bounds a cover download at 12 MiB.
const maxCoverBytes = 12 << 20

// coverExtensions are the accepted image extensions, in preference order.
var coverExtensions = []string{".jpg", ".png", ".webp"}

// CoverFetcher downloads cover art.
//
// It shares the pipeline's HTTP client, so covers inherit the request timeout,
// retries and connection pooling. Previously this used http.Get, i.e.
// http.DefaultClient with no timeout at all, and a stalled CDN could pin a queue
// worker forever.
type CoverFetcher struct {
	http *HTTPClient
}

// NewCoverFetcher creates a fetcher using the given client.
func NewCoverFetcher(client *HTTPClient) *CoverFetcher {
	return &CoverFetcher{http: client}
}

// Existing returns the path of an already stored cover, or "".
//
// Unlike the old existence check this ignores zero-length files and directories,
// which used to be treated as valid covers and then masked forever because the
// downloader skipped any path that already existed.
func Existing(gameDir, kind string) string {
	for _, dir := range []string{game.CoverDir(gameDir), gameDir} {
		for _, ext := range coverExtensions {
			path := filepath.Join(dir, kind+ext)
			info, err := os.Stat(path)
			if err == nil && !info.IsDir() && info.Size() > 0 {
				return path
			}
		}
	}
	return ""
}

// Fetch downloads coverURL into the game's covers folder.
//
// force replaces an existing file, which is what makes re-scraping able to fix a
// bad or low resolution cover. Returns true when a usable file is present
// afterwards.
func (f *CoverFetcher) Fetch(ctx context.Context, gameDir, gameID, coverURL, kind string, force bool) (bool, error) {
	if strings.TrimSpace(coverURL) == "" {
		// The source simply had no artwork. Not an error.
		return Existing(gameDir, kind) != "", nil
	}
	if !force {
		if existing := Existing(gameDir, kind); existing != "" {
			return true, nil
		}
	}

	resp, err := f.http.Do(ctx, "cover", Request{
		URL:      coverURL,
		MaxBytes: maxCoverBytes,
		Headers:  map[string]string{"Accept": "image/*"},
	})
	if err != nil {
		return false, err
	}
	if len(resp.Body) == 0 {
		return false, &APIError{Source: "cover", Kind: KindParse, URL: coverURL, Message: "empty image body"}
	}

	ext, ok := imageExtension(resp.Body, resp.Header.Get("Content-Type"))
	if !ok {
		return false, &APIError{
			Source:  "cover",
			Kind:    KindParse,
			URL:     coverURL,
			Message: fmt.Sprintf("response is not a recognised image (%s)", sniffType(resp.Body)),
		}
	}

	coverDir := game.CoverDir(gameDir)
	target := filepath.Join(coverDir, kind+ext)

	// Write first, then drop stale copies with a different extension, so there is
	// never a window with no cover at all.
	if err := fsutil.WriteFileAtomic(target, resp.Body, 0o644); err != nil {
		return false, fmt.Errorf("store cover for %s: %w", gameID, err)
	}
	for _, other := range coverExtensions {
		if other == ext {
			continue
		}
		_ = os.Remove(filepath.Join(coverDir, kind+other))
		_ = os.Remove(filepath.Join(gameDir, kind+other))
	}
	return true, nil
}

// imageExtension determines the file extension from the actual bytes, falling
// back to the declared content type.
//
// Trusting Content-Type alone stored WebP and AVIF payloads under a ".jpg" name,
// which produced a broken image that the existence check then considered valid
// forever.
func imageExtension(data []byte, contentType string) (string, bool) {
	switch sniffType(data) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		// Animated GIFs are not useful as covers, but they are still images.
		return ".png", false
	}

	declared := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch declared {
	case "image/jpeg", "image/jpg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	}
	return "", false
}

func sniffType(data []byte) string {
	if len(data) > 512 {
		data = data[:512]
	}
	return http.DetectContentType(data)
}

// Remove deletes stored cover art of a kind, used before a forced re-scrape.
func Remove(gameDir, kind string) error {
	var firstErr error
	for _, dir := range []string{game.CoverDir(gameDir), gameDir} {
		for _, ext := range coverExtensions {
			path := filepath.Join(dir, kind+ext)
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
