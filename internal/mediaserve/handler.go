// Package mediaserve serves on-disk media (cover art) to the webview over HTTP.
//
// The UI used to request covers through the Wails binding layer, where each call
// read the whole image, base64 encoded it, and pushed the resulting string across
// the JS bridge. That inflated every image by a third and turned a grid of N cards
// into N full-size transfers. Serving the files from the asset server lets the
// webview fetch, cache and range-request them like any other asset.
package mediaserve

import (
	"net/http"
	"os"
	"path"
	"strings"

	"GameLibrary/internal/logger"
)

// CoverResolver maps a game ID to a cover file on disk.
type CoverResolver interface {
	// CoverPath returns the path of a game's cover art, or "" when there is none.
	CoverPath(id string, landscape bool) string
}

// RoutePrefix is the URL space this handler owns.
const RoutePrefix = "/covers/"

// Handler serves cover art.
type Handler struct {
	resolver CoverResolver
}

// New creates a handler backed by a resolver.
func New(resolver CoverResolver) *Handler {
	return &Handler{resolver: resolver}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, landscape, ok := parseCoverPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if h.resolver == nil {
		http.NotFound(w, r)
		return
	}
	filePath := h.resolver.CoverPath(id, landscape)
	if filePath == "" {
		// A missing cover is normal: the UI shows its own placeholder.
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() || info.Size() == 0 {
		http.NotFound(w, r)
		return
	}

	// Cover files are immutable for a given version: a re-scrape changes the
	// version query parameter, so the response can be cached indefinitely.
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Content-Type", contentTypeFor(filePath))

	http.ServeContent(w, r, path.Base(filePath), info.ModTime(), file)
}

// parseCoverPath extracts the game ID and variant from a request path.
func parseCoverPath(urlPath string) (id string, landscape bool, ok bool) {
	if !strings.HasPrefix(urlPath, RoutePrefix) {
		return "", false, false
	}
	rest := strings.TrimPrefix(urlPath, RoutePrefix)
	// Tolerate a trailing slash, but not an empty leading segment: "/covers//x"
	// is a malformed request rather than a game called "".
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		return "", false, false
	}

	segments := strings.Split(rest, "/")
	switch len(segments) {
	case 1:
		id = segments[0]
	case 2:
		id = segments[0]
		switch segments[1] {
		case "cover", "portrait":
			landscape = false
		case "landscape":
			landscape = true
		default:
			return "", false, false
		}
	default:
		return "", false, false
	}

	// Game IDs are generated from a restricted alphabet, so anything else is a
	// malformed request rather than a path to resolve.
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", false, false
	}
	return id, landscape, true
}

func contentTypeFor(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

// LogStartup records that the media server is mounted.
func LogStartup() {
	logger.Debug("media server mounted", "prefix", RoutePrefix)
}
