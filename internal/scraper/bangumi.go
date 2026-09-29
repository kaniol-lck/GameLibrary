package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// bangumiSearchURL is the Bangumi v0 subject search endpoint.
//
// The previous implementation issued a GET to "/v0/search/subject/{keyword}"
// with legacy query parameters ("responseGroup", "max_results"). No such route
// exists in the v0 API, so every request returned HTTP 404 and the provider could
// never match anything. The documented endpoint is a POST of a JSON body, and the
// response is a paged object whose results live under "data".
const bangumiSearchURL = "https://api.bgm.tv/v0/search/subjects"

// bangumiSubjectTypeGame is the subject type for games.
const bangumiSubjectTypeGame = 4

// bangumiMinInterval keeps the request rate modest; Bangumi publishes no formal
// budget but throttles aggressive clients.
const bangumiMinInterval = 1000 * time.Millisecond

// BangumiScraper resolves games through Bangumi (bgm.tv), which is the most
// reliable source of Chinese titles and release dates for Japanese games.
type BangumiScraper struct {
	http      *HTTPClient
	endpoint  string
	userAgent string
}

// NewBangumiScraper creates the provider.
func NewBangumiScraper() *BangumiScraper {
	return &BangumiScraper{
		endpoint:  bangumiSearchURL,
		userAgent: "GameLibrary/" + ClientVersion + " (https://github.com/kaniol-lck/GameLibrary)",
	}
}

// Key implements Source.
func (s *BangumiScraper) Key() string { return "bangumi" }

// Configure implements Source.
func (s *BangumiScraper) Configure(cfg SourceConfig) error {
	s.http = cfg.HTTP
	if s.http != nil {
		s.http.LimitHost(hostOf(s.endpoint), bangumiMinInterval)
	}
	return nil
}

// Search implements Source.
func (s *BangumiScraper) Search(ctx context.Context, q Query) (*Result, error) {
	var lastErr error
	for _, term := range SearchTerms(q.Name) {
		result, err := s.searchByName(ctx, term)
		if err == nil && result != nil {
			return result, nil
		}
		if err != nil && !IsNoResult(err) {
			lastErr = err
			break
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, NoResult("bangumi", q.Name)
}

func (s *BangumiScraper) searchByName(ctx context.Context, name string) (*Result, error) {
	payload, err := json.Marshal(map[string]any{
		"keyword": name,
		"sort":    "match",
		"filter": map[string]any{
			"type": []int{bangumiSubjectTypeGame},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("bangumi: encode query: %w", err)
	}

	params := url.Values{}
	params.Set("limit", strconv.Itoa(3))
	requestURL := s.endpoint + "?" + params.Encode()

	resp, err := s.http.Do(ctx, "bangumi", Request{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"Content-Type": "application/json",
			// Bangumi asks API clients to identify themselves.
			"User-Agent": s.userAgent,
		},
		Body:     payload,
		MaxBytes: 1 << 20,
	})
	if err != nil {
		return nil, err
	}

	var search struct {
		Total int `json:"total"`
		Data  []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			NameCN  string `json:"name_cn"`
			Summary string `json:"summary"`
			Date    string `json:"date"`
			Images  struct {
				Large  string `json:"large"`
				Common string `json:"common"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &search); err != nil {
		return nil, &APIError{Source: "bangumi", Kind: KindParse, URL: requestURL, Message: "unexpected search payload", Err: err}
	}
	if len(search.Data) == 0 {
		return nil, NoResult("bangumi", name)
	}

	item := search.Data[0]

	// Chinese users want the Chinese title as the display name; the original
	// script is kept alongside it either way.
	title := CleanText(item.NameCN)
	native := CleanText(item.Name)
	if title == "" {
		title = native
	}
	if title == "" {
		return nil, NoResult("bangumi", name)
	}

	cover := item.Images.Large
	if cover == "" {
		cover = item.Images.Common
	}

	return &Result{
		Title:             Unescape(title),
		TitleNative:       Unescape(native),
		Description:       PrepareDescription(item.Summary),
		ReleaseDate:       CleanText(item.Date),
		CoverURL:          cover,
		CoverLandscapeURL: cover,
		Links: map[string]string{
			"bangumi":     fmt.Sprintf("https://bgm.tv/subject/%d", item.ID),
			PlatformIDKey: strconv.Itoa(item.ID),
		},
	}, nil
}

// TrimWrappedParens is a helper used by provider tests when comparing titles that
// Bangumi decorates, e.g. "Steins;Gate (PC)".
func TrimWrappedParens(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, ")") {
		if idx := strings.LastIndexByte(s, '('); idx > 0 {
			return strings.TrimSpace(s[:idx])
		}
	}
	return s
}
