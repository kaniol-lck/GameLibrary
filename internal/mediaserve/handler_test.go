package mediaserve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// stubResolver maps ids to paths for the handler tests.
type stubResolver struct {
	portrait  map[string]string
	landscape map[string]string
}

func (s stubResolver) CoverPath(id string, landscape bool) string {
	if landscape {
		return s.landscape[id]
	}
	return s.portrait[id]
}

func TestParseCoverPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantID        string
		wantLandscape bool
		wantOK        bool
	}{
		{"bare id is the portrait", "/covers/steam_570", "steam_570", false, true},
		{"explicit cover", "/covers/steam_570/cover", "steam_570", false, true},
		{"portrait alias", "/covers/steam_570/portrait", "steam_570", false, true},
		{"landscape", "/covers/steam_570/landscape", "steam_570", true, true},
		{"trailing slash", "/covers/steam_570/", "steam_570", false, true},
		{"trailing slash on variant", "/covers/steam_570/landscape/", "steam_570", true, true},
		{"missing id", "/covers/", "", false, false},
		{"too many segments", "/covers/a/b/c", "", false, false},
		{"unknown variant", "/covers/a/bogus", "", false, false},
		{"outside the route", "/assets/index.js", "", false, false},
		{"path traversal", "/covers/../secret", "", false, false},
		{"backslash in id", `/covers/a\b`, "", false, false},
		{"empty id segment", "/covers//cover", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, landscape, ok := parseCoverPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, ok)
			}
			if !ok {
				return
			}
			if id != tt.wantID || landscape != tt.wantLandscape {
				t.Fatalf("expected (%q,%v), got (%q,%v)", tt.wantID, tt.wantLandscape, id, landscape)
			}
		})
	}
}

func TestServesRealFile(t *testing.T) {
	dir := t.TempDir()
	payload := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	path := filepath.Join(dir, "cover.png")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	handler := New(stubResolver{portrait: map[string]string{"g1": path}})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/covers/g1/cover", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.Bytes(); string(got) != string(payload) {
		t.Fatalf("unexpected body: %v", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png, got %q", ct)
	}
	if cl := rec.Header().Get("Content-Length"); cl == "" {
		t.Error("expected a Content-Length header")
	}
}

func TestContentTypeByExtension(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		"cover.jpg":  "image/jpeg",
		"cover.jpeg": "image/jpeg",
		"cover.png":  "image/png",
		"cover.webp": "image/webp",
		"cover.gif":  "image/gif",
		"cover.bin":  "image/jpeg",
	}
	for name, want := range tests {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		handler := New(stubResolver{portrait: map[string]string{"g1": path}})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/covers/g1", nil))
		if got := rec.Header().Get("Content-Type"); got != want {
			t.Errorf("%s: expected %q, got %q", name, want, got)
		}
	}
}

func TestCacheControl(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := New(stubResolver{portrait: map[string]string{"g1": path}})

	// A versioned URL is immutable: a re-scrape changes the parameter.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/covers/g1?v=1699999999", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("unexpected Cache-Control for a versioned URL: %q", got)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/covers/g1", nil))
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("unexpected Cache-Control for an unversioned URL: %q", got)
	}
}

func TestNotFoundCases(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.jpg")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	asDir := filepath.Join(dir, "cover.jpg")
	if err := os.MkdirAll(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real.jpg")
	if err := os.WriteFile(real, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := stubResolver{portrait: map[string]string{
		"none":     "",
		"missing":  filepath.Join(dir, "does-not-exist.jpg"),
		"empty":    empty,
		"dir":      asDir,
		"ok":       real,
		"bad-path": "/covers/ok", // wrong shape, but must not escape the resolver
	}}
	handler := New(resolver)

	tests := []struct {
		name string
		path string
		want int
	}{
		{"no cover recorded", "/covers/none", http.StatusNotFound},
		{"file does not exist", "/covers/missing", http.StatusNotFound},
		{"zero length file", "/covers/empty", http.StatusNotFound},
		{"path is a directory", "/covers/dir", http.StatusNotFound},
		{"unknown id", "/covers/unknown", http.StatusNotFound},
		{"malformed request", "/covers/a/bogus", http.StatusNotFound},
		{"present", "/covers/ok", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, rec.Code)
			}
		})
	}
}

func TestMethodHandling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := New(stubResolver{portrait: map[string]string{"g1": path}})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/covers/g1", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/covers/g1", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for HEAD, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD must not return a body, got %d bytes", rec.Body.Len())
	}
}

func TestNilResolverDoesNotPanic(t *testing.T) {
	handler := New(nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/covers/g1", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
