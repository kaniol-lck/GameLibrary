package scraper

import (
	"fmt"
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
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	html := string(body)

	title := extractMeta(html, "og:title")
	desc := extractMeta(html, "og:description")
	coverURL := extractMeta(html, "og:image")

	if title == "" {
		return nil, fmt.Errorf("dlsite: could not extract metadata for %s", rjCode)
	}

	return &Result{
		Title:             cleanText(title),
		Description:       cleanText(desc),
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
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return strings.TrimSpace(s)
}
