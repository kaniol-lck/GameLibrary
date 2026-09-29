package scraper

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// rawgSearchURL is the RAWG.io game search endpoint.
const rawgSearchURL = "https://api.rawg.io/api/games"

// rawgMinInterval keeps well inside the free tier's request budget.
const rawgMinInterval = 1200 * time.Millisecond

// RawgScraper resolves games through RAWG.io, which covers western releases well
// and provides large landscape artwork.
type RawgScraper struct {
	http     *HTTPClient
	apiKey   string
	endpoint string
}

// NewRawgScraper creates the provider.
func NewRawgScraper() *RawgScraper {
	return &RawgScraper{endpoint: rawgSearchURL}
}

// Key implements Source.
func (s *RawgScraper) Key() string { return "rawg" }

// Configure implements Source.
func (s *RawgScraper) Configure(cfg SourceConfig) error {
	s.http = cfg.HTTP
	s.apiKey = strings.TrimSpace(cfg.APIKey)
	if s.http != nil {
		s.http.LimitHost("api.rawg.io", rawgMinInterval)
	}
	return nil
}

// Search implements Source.
func (s *RawgScraper) Search(ctx context.Context, q Query) (*Result, error) {
	if s.apiKey == "" {
		// Without a key the API answers 401 and the body parses as "zero
		// results", which used to be reported as a data miss. Report the real
		// problem instead.
		return nil, &APIError{Source: "rawg", Kind: KindAuth, Message: "API key required"}
	}

	var lastErr error
	for _, term := range SearchTerms(q.Name) {
		result, err := s.search(ctx, term)
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
	return nil, NoResult("rawg", q.Name)
}

type rawgName struct {
	Name string `json:"name"`
}

func (s *RawgScraper) search(ctx context.Context, term string) (*Result, error) {
	params := url.Values{}
	params.Set("search", term)
	params.Set("key", s.apiKey)
	params.Set("page_size", "1")
	params.Set("search_precise", "true")
	requestURL := s.endpoint + "?" + params.Encode()

	resp, err := s.http.Do(ctx, "rawg", Request{URL: requestURL, MaxBytes: 1 << 20})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []struct {
			ID            int        `json:"id"`
			Name          string     `json:"name"`
			Slug          string     `json:"slug"`
			Released      string     `json:"released"`
			Description   string     `json:"description_raw"`
			BackgroundImg string     `json:"background_image"`
			Website       string     `json:"website"`
			Genres        []rawgName `json:"genres"`
			Tags          []rawgName `json:"tags"`
			Developers    []rawgName `json:"developers"`
			Publishers    []rawgName `json:"publishers"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, &APIError{Source: "rawg", Kind: KindParse, URL: requestURL, Message: "unexpected search payload", Err: err}
	}
	if len(payload.Results) == 0 {
		return nil, NoResult("rawg", term)
	}

	item := payload.Results[0]
	if strings.TrimSpace(item.Name) == "" {
		return nil, NoResult("rawg", term)
	}

	tags := make([]string, 0, len(item.Genres)+len(item.Tags))
	for _, list := range [][]rawgName{item.Genres, item.Tags} {
		for _, entry := range list {
			if entry.Name != "" {
				tags = append(tags, entry.Name)
			}
		}
	}

	links := map[string]string{}
	if item.Slug != "" {
		links["rawg"] = "https://rawg.io/games/" + item.Slug
	}
	if item.Website != "" {
		links["website"] = item.Website
	}
	if item.ID != 0 {
		links[PlatformIDKey] = strconv.Itoa(item.ID)
	}

	return &Result{
		Title:             Unescape(item.Name),
		Description:       PrepareDescription(item.Description),
		Developer:         firstRawgName(item.Developers),
		Publisher:         firstRawgName(item.Publishers),
		ReleaseDate:       CleanText(item.Released),
		Tags:              tags,
		CoverURL:          item.BackgroundImg,
		CoverLandscapeURL: item.BackgroundImg,
		Links:             links,
	}, nil
}

func firstRawgName(items []rawgName) string {
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0].Name)
}
