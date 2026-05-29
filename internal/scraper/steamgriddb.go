package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SteamGridDBScraper struct {
	client *http.Client
	apiKey string
}

func NewSteamGridDBScraper() *SteamGridDBScraper {
	return &SteamGridDBScraper{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *SteamGridDBScraper) Key() string { return "steamgriddb" }

func (s *SteamGridDBScraper) Configure(lang string, settings map[string]string) {
	_ = lang
	if settings != nil {
		if key, ok := settings["apiKey"]; ok {
			s.apiKey = key
		}
	}
}

func (s *SteamGridDBScraper) Search(gameDir string, appID string) (*Result, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("steamgriddb: API key required")
	}
	if appID == "" {
		return nil, fmt.Errorf("steamgriddb: requires steam appID")
	}

	url := fmt.Sprintf("https://www.steamgriddb.com/api/v2/grids/steam/%s?limit=5&styles=alternate,material,blurred", appID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var apiResp struct {
		Data []struct {
			URL    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Style  string `json:"style"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("steamgriddb: no grids found")
	}

	portrait := ""
	landscape := ""
	for _, g := range apiResp.Data {
		if portrait == "" && g.Height > g.Width {
			portrait = g.URL
		}
		if landscape == "" && g.Width > g.Height {
			landscape = g.URL
		}
		if portrait != "" && landscape != "" {
			break
		}
	}
	if portrait == "" && len(apiResp.Data) > 0 {
		portrait = apiResp.Data[0].URL
	}
	if landscape == "" {
		landscape = portrait
	}

	_ = strings.TrimSuffix(gameDir, "")

	return &Result{
		CoverURL:          portrait,
		CoverLandscapeURL: landscape,
	}, nil
}
