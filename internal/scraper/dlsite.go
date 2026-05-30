package scraper

import (
	"fmt"
	htmlpkg "html"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var rjPattern = regexp.MustCompile(`RJ\d{6,8}`)

type DLsiteScraper struct {
	client *http.Client
}

func NewDLsiteScraper() *DLsiteScraper {
	return &DLsiteScraper{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *DLsiteScraper) Key() string { return "dlsite" }

func (s *DLsiteScraper) Search(gameDir string, appID string) (*Result, error) {
	_ = appID
	name := filepath.Base(gameDir)

	rjCode := rjPattern.FindString(name)
	if rjCode == "" {
		rjCode = rjPattern.FindString(gameDir)
	}
	if rjCode == "" {
		return nil, fmt.Errorf("dlsite: no RJ code found in '%s'", name)
	}

	return s.searchByRJCode(rjCode)
}

func (s *DLsiteScraper) searchByRJCode(rjCode string) (*Result, error) {
	pageURL := fmt.Sprintf("https://www.dlsite.com/maniax/work/=/product_id/%s.html", rjCode)

	resp, err := s.client.Get(pageURL)
	if err != nil {
		return nil, fmt.Errorf("dlsite: network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("dlsite: product %s not found (404)", rjCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dlsite: HTTP %d for %s", resp.StatusCode, rjCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	html := string(body)

	if strings.Contains(html, "この作品は存在しません") || strings.Contains(html, "product not found") {
		return nil, fmt.Errorf("dlsite: product %s not found", rjCode)
	}

	title := extractMeta(html, "og:title")
	desc := extractMeta(html, "og:description")
	coverURL := extractMeta(html, "og:image")

	if title == "" {
		return nil, fmt.Errorf("dlsite: product %s page has no metadata (may not exist or require login)", rjCode)
	}

	return &Result{
		Title:             htmlpkg.UnescapeString(cleanText(title)),
		Description:       htmlpkg.UnescapeString(cleanText(desc)),
		CoverURL:          coverURL,
		CoverLandscapeURL: coverURL,
		Links: map[string]string{
			"dlsite":     pageURL,
			"platformId": rjCode,
		},
		Tags: []string{"Doujin", "DLsite"},
	}, nil
}

func extractMeta(html, property string) string {
	patterns := []string{
		fmt.Sprintf(`<meta property="%s" content="`, property),
		fmt.Sprintf(`<meta name="%s" content="`, property),
	}

	for _, prefix := range patterns {
		idx := strings.Index(html, prefix)
		if idx < 0 {
			continue
		}
		start := idx + len(prefix)
		end := strings.Index(html[start:], `"`)
		if end < 0 {
			end = strings.Index(html[start:], `/>`)
			if end < 0 {
				continue
			}
		}
		return html[start : start+end]
	}
	return ""
}

func cleanText(s string) string {
	return strings.TrimSpace(s)
}
