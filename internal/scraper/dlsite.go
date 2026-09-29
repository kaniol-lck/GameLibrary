package scraper

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DLsite serves adult works and all-ages works from two different sections of the
// same host. A product code has to be looked up in both.
var dlsiteSections = []string{
	"https://www.dlsite.com/maniax/work/=/product_id/%s.html",
	"https://www.dlsite.com/home/work/=/product_id/%s.html",
}

// dlsiteMinInterval keeps the scrape polite; DLsite serves plain HTML pages.
const dlsiteMinInterval = 1200 * time.Millisecond

// DLsiteScraper resolves doujin works from their RJ product code.
//
// There is no API, so the product page is parsed. Product codes are matched from
// the folder name, which is how DLsite works are almost always named on disk.
type DLsiteScraper struct {
	http *HTTPClient
}

// NewDLsiteScraper creates the provider.
func NewDLsiteScraper() *DLsiteScraper {
	return &DLsiteScraper{}
}

// Key implements Source.
func (s *DLsiteScraper) Key() string { return "dlsite" }

// Configure implements Source.
func (s *DLsiteScraper) Configure(cfg SourceConfig) error {
	s.http = cfg.HTTP
	if s.http != nil {
		s.http.LimitHost("www.dlsite.com", dlsiteMinInterval)
	}
	return nil
}

// Search implements Source.
func (s *DLsiteScraper) Search(ctx context.Context, q Query) (*Result, error) {
	// Only the folder name identifies the product. Searching the whole path used
	// to match an RJ code in an unrelated parent directory.
	code := RJCode(q.Name)
	if code == "" {
		return nil, NoResult("dlsite", q.Name)
	}
	if id := q.PlatformID("dlsite"); id != "" && !strings.EqualFold(id, code) {
		// A previously confirmed code takes precedence over name inference.
		code = strings.ToUpper(id)
	}

	var lastErr error
	for _, pattern := range dlsiteSections {
		result, err := s.fetchProduct(ctx, code, fmt.Sprintf(pattern, code))
		if err == nil && result != nil {
			return result, nil
		}
		// A genuine network or rate-limit failure means the second section will
		// fail the same way, so stop rather than doubling the request count.
		if err != nil && !IsNoResult(err) && KindOf(err) != KindClient {
			lastErr = err
			break
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, NoResult("dlsite", code)
}

func (s *DLsiteScraper) fetchProduct(ctx context.Context, code, pageURL string) (*Result, error) {
	resp, err := s.http.Do(ctx, "dlsite", Request{
		URL: pageURL,
		Headers: map[string]string{
			// DLsite gates adult pages behind a cookie and rejects requests
			// without a browser-like User-Agent.
			"Cookie":     "adultchecked=1",
			"Referer":    "https://www.dlsite.com/",
			"Accept":     "text/html,application/xhtml+xml",
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) GameLibrary/" + ClientVersion,
		},
		MaxBytes: 3 << 20,
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return nil, NoResult("dlsite", code)
		}
		return nil, err
	}

	html := string(resp.Body)
	if isDLsiteMissingPage(html) {
		return nil, NoResult("dlsite", code)
	}
	// An age gate or interstitial also carries og: tags, so the page is only
	// accepted when it actually references the requested product code.
	if !strings.Contains(strings.ToUpper(html), code) {
		return nil, NoResult("dlsite", code)
	}

	title := extractMeta(html, "og:title")
	if title == "" {
		return nil, NoResult("dlsite", code)
	}

	cover := normalizeURL(extractMeta(html, "og:image"))
	description := extractMeta(html, "og:description")

	return &Result{
		Title:             PrepareTitle(title),
		Description:       PrepareDescription(description),
		CoverURL:          cover,
		CoverLandscapeURL: cover,
		Tags:              []string{"Doujin", "DLsite"},
		Links: map[string]string{
			"dlsite":      pageURL,
			PlatformIDKey: code,
		},
	}, nil
}

// PrepareTitle cleans a scraped page title.
//
// DLsite titles are page titles, so they carry the site's own decoration
// ("... [RJ123456] | DLsite"). The bracket group is stripped.
func PrepareTitle(title string) string {
	title = CleanText(Unescape(title))
	title = strings.TrimSpace(strings.ReplaceAll(title, "| DLsite", ""))
	for _, suffix := range []string{" - DLsite", " | DLsite"} {
		title = strings.TrimSuffix(title, suffix)
	}
	title = strings.TrimRight(strings.TrimSpace(title), "-–|")
	return strings.TrimSpace(title)
}

func isDLsiteMissingPage(html string) bool {
	for _, marker := range []string{
		"この作品は存在しません",
		"お探しのページはありません",
		"product not found",
		"Page Not Found",
	} {
		if strings.Contains(html, marker) {
			return true
		}
	}
	return false
}

// metaTagPattern matches a single <meta ...> tag.
var metaTagPattern = regexp.MustCompile(`(?is)<meta\s[^>]*>`)

// metaAttrPattern matches attribute="value" pairs inside a tag, accepting either
// quote style. The previous extractor required the exact byte sequence
// `<meta property="x" content="`, so any other attribute order or a single-quoted
// document silently produced nothing.
var metaAttrPattern = regexp.MustCompile(`(?is)([a-zA-Z:_-]+)\s*=\s*("([^"]*)"|'([^']*)')`)

// extractMeta returns the content of a meta tag identified by property or name.
func extractMeta(html, property string) string {
	wanted := strings.ToLower(property)
	for _, tag := range metaTagPattern.FindAllString(html, -1) {
		attrs := parseMetaAttrs(tag)
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		if strings.ToLower(key) == wanted {
			return strings.TrimSpace(attrs["content"])
		}
	}
	return ""
}

func parseMetaAttrs(tag string) map[string]string {
	attrs := make(map[string]string, 4)
	for _, match := range metaAttrPattern.FindAllStringSubmatch(tag, -1) {
		value := match[3]
		if value == "" {
			value = match[4]
		}
		attrs[strings.ToLower(match[1])] = value
	}
	return attrs
}

// normalizeURL turns a protocol-relative URL into an absolute one.
func normalizeURL(raw string) string {
	switch {
	case raw == "":
		return ""
	case strings.HasPrefix(raw, "//"):
		return "https:" + raw
	default:
		return raw
	}
}
