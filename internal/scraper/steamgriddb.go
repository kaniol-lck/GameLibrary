package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// steamGridDBGridsURL is the grid (cover art) endpoint.
const steamGridDBGridsURL = "https://www.steamgriddb.com/api/v2/grids/steam/%s"

// steamGridDBMinInterval keeps the request rate modest on a free API key.
const steamGridDBMinInterval = 800 * time.Millisecond

// SteamGridDBScraper supplies high resolution cover art.
//
// It deliberately returns no title or description: its job is artwork only. The
// pipeline merges fields, so a cover-only result can no longer blank out the name
// and description that another provider already found.
type SteamGridDBScraper struct {
	http     *HTTPClient
	apiKey   string
	endpoint string
}

// NewSteamGridDBScraper creates the provider.
func NewSteamGridDBScraper() *SteamGridDBScraper {
	return &SteamGridDBScraper{endpoint: steamGridDBGridsURL}
}

// Key implements Source.
func (s *SteamGridDBScraper) Key() string { return "steamgriddb" }

// Configure implements Source.
func (s *SteamGridDBScraper) Configure(cfg SourceConfig) error {
	s.http = cfg.HTTP
	s.apiKey = strings.TrimSpace(cfg.APIKey)
	if s.http != nil {
		s.http.LimitHost("www.steamgriddb.com", steamGridDBMinInterval)
	}
	return nil
}

// Search implements Source.
func (s *SteamGridDBScraper) Search(ctx context.Context, q Query) (*Result, error) {
	appID := q.SteamAppID()
	if appID == "" {
		// Without a Steam app ID there is nothing to look up. That is a normal
		// miss, not a failure worth warning about.
		return nil, NoResult("steamgriddb", q.Name)
	}
	if s.apiKey == "" {
		// Signalled as an auth problem so the UI can tell the user to add a key
		// instead of quietly reporting "no cover found".
		return nil, &APIError{Source: "steamgriddb", Kind: KindAuth, Message: "API key required"}
	}

	params := url.Values{}
	params.Set("limit", "10")
	params.Set("styles", "alternate,material,blurred")
	requestURL := fmt.Sprintf(s.endpoint, appID) + "?" + params.Encode()

	resp, err := s.http.Do(ctx, "steamgriddb", Request{
		URL:      requestURL,
		Headers:  map[string]string{"Authorization": "Bearer " + s.apiKey},
		MaxBytes: 1 << 20,
	})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Data []gridEntry `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, &APIError{Source: "steamgriddb", Kind: KindParse, URL: requestURL, Message: "unexpected grids payload", Err: err}
	}
	if len(payload.Data) == 0 {
		return nil, NoResult("steamgriddb", appID)
	}

	portrait, landscape := pickGrids(payload.Data)
	if portrait == "" && landscape == "" {
		return nil, NoResult("steamgriddb", appID)
	}

	return &Result{
		CoverURL:          portrait,
		CoverLandscapeURL: landscape,
		Links:             map[string]string{PlatformIDKey: appID},
	}, nil
}

type gridEntry struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// pickGrids chooses the best portrait and landscape image. SteamGridDB returns
// both orientations in one list, distinguished by their dimensions.
func pickGrids(grids []gridEntry) (portrait, landscape string) {
	for _, grid := range grids {
		if grid.URL == "" {
			continue
		}
		switch {
		case portrait == "" && grid.Height > grid.Width:
			portrait = grid.URL
		case landscape == "" && grid.Width > grid.Height:
			landscape = grid.URL
		}
		if portrait != "" && landscape != "" {
			break
		}
	}
	if portrait == "" {
		for _, grid := range grids {
			if grid.URL != "" {
				portrait = grid.URL
				break
			}
		}
	}
	if landscape == "" {
		landscape = portrait
	}
	return portrait, landscape
}
