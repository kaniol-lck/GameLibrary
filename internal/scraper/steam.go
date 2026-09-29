package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// SteamScraper resolves metadata through the public Steam storefront API.
//
// When the app ID is known the lookup is exact; otherwise the store search is
// used, which means a title match is a guess and the caller should treat a
// confusing result as a search quality problem rather than a bug.
type SteamScraper struct {
	http   *HTTPClient
	lang   Lang
	apiKey string

	appDetailsURL  string
	storeSearchURL string
}

// NewSteamScraper creates the provider. apiKey is accepted for parity with the
// other providers but the public storefront endpoints used here need none.
func NewSteamScraper() *SteamScraper {
	return &SteamScraper{
		lang:           LangEnglish,
		appDetailsURL:  "https://store.steampowered.com/api/appdetails",
		storeSearchURL: "https://store.steampowered.com/api/storesearch/",
	}
}

// Key implements Source.
func (s *SteamScraper) Key() string { return "steam" }

// Configure implements Source.
func (s *SteamScraper) Configure(cfg SourceConfig) error {
	s.http = cfg.HTTP
	if cfg.Language != "" {
		s.lang = cfg.Language
	}
	s.apiKey = cfg.APIKey
	return nil
}

// Search implements Source.
func (s *SteamScraper) Search(ctx context.Context, q Query) (*Result, error) {
	if appID := q.SteamAppID(); appID != "" {
		return s.searchByAppID(ctx, appID)
	}

	// Visual novels are distributed under DLsite product codes; sending an
	// "RJ123456" folder name to the Steam search only ever returns noise.
	if RJCode(q.Name) != "" {
		return nil, NoResult("steam", q.Name)
	}

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
	return nil, NoResult("steam", q.Name)
}

func (s *SteamScraper) searchByAppID(ctx context.Context, appID string) (*Result, error) {
	params := url.Values{}
	params.Set("appids", appID)
	params.Set("l", s.lang.SteamCode())
	params.Set("cc", "us")

	requestURL := s.appDetailsURL + "?" + params.Encode()
	resp, err := s.http.Do(ctx, "steam", Request{URL: requestURL, MaxBytes: 2 << 20})
	if err != nil {
		return nil, err
	}

	var payload map[string]struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, &APIError{Source: "steam", Kind: KindParse, URL: requestURL, Message: "unexpected appdetails payload", Err: err}
	}

	entry, ok := payload[appID]
	if !ok {
		return nil, NoResult("steam", appID)
	}
	if !entry.Success {
		return nil, NoResult("steam", appID)
	}

	var appData struct {
		Name                string   `json:"name"`
		ShortDescription    string   `json:"short_description"`
		DetailedDescription string   `json:"detailed_description"`
		Developers          []string `json:"developers"`
		Publishers          []string `json:"publishers"`
		ReleaseDate         struct {
			Date string `json:"date"`
		} `json:"release_date"`
		Genres []struct {
			Description string `json:"description"`
		} `json:"genres"`
	}
	if err := json.Unmarshal(entry.Data, &appData); err != nil {
		return nil, &APIError{Source: "steam", Kind: KindParse, URL: requestURL, Message: "unexpected appdetails data", Err: err}
	}
	if strings.TrimSpace(appData.Name) == "" {
		return nil, NoResult("steam", appID)
	}

	description := appData.ShortDescription
	if strings.TrimSpace(description) == "" {
		description = appData.DetailedDescription
	}

	tags := make([]string, 0, len(appData.Genres))
	for _, genre := range appData.Genres {
		if genre.Description != "" {
			tags = append(tags, genre.Description)
		}
	}

	return &Result{
		Title:             appData.Name,
		Description:       PrepareDescription(description),
		Developer:         FirstOrEmpty(appData.Developers),
		Publisher:         FirstOrEmpty(appData.Publishers),
		ReleaseDate:       appData.ReleaseDate.Date,
		Tags:              tags,
		CoverURL:          steamCoverURL(appID, "library_600x900_2x.jpg"),
		CoverLandscapeURL: steamCoverURL(appID, "header.jpg"),
		Links: map[string]string{
			"steam":       fmt.Sprintf("https://store.steampowered.com/app/%s/", appID),
			PlatformIDKey: appID,
		},
	}, nil
}

func (s *SteamScraper) searchByName(ctx context.Context, term string) (*Result, error) {
	params := url.Values{}
	params.Set("term", term)
	params.Set("l", s.lang.SteamCode())
	params.Set("cc", "us")

	requestURL := s.storeSearchURL + "?" + params.Encode()
	resp, err := s.http.Do(ctx, "steam", Request{URL: requestURL, MaxBytes: 1 << 20})
	if err != nil {
		return nil, err
	}

	var search struct {
		Total int `json:"total"`
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body, &search); err != nil {
		return nil, &APIError{Source: "steam", Kind: KindParse, URL: requestURL, Message: "unexpected storesearch payload", Err: err}
	}

	// Guard against total>0 with an empty items array, which used to index out of
	// range and panic the whole webview call.
	if len(search.Items) == 0 {
		return nil, NoResult("steam", term)
	}

	matchedID := strconv.Itoa(search.Items[0].ID)
	if matchedID == "0" {
		return nil, NoResult("steam", term)
	}
	return s.searchByAppID(ctx, matchedID)
}

// steamCoverURL builds a Steam CDN URL. App IDs are digits from the API, but
// they are validated anyway so a malformed value can never be interpolated into
// a URL path.
func steamCoverURL(appID, asset string) string {
	if !isDigits(appID) {
		return ""
	}
	return fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%s/%s", appID, asset)
}
