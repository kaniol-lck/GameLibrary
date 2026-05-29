package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SteamScraper struct {
	client *http.Client
}

func NewSteamScraper() *SteamScraper {
	return &SteamScraper{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *SteamScraper) Key() string { return "steam" }

func (s *SteamScraper) Search(gameDir string, appID string) (*Result, error) {
	if appID != "" {
		return s.searchByAppID(appID)
	}
	return s.searchByName(gameDir)
}

func (s *SteamScraper) searchByAppID(appID string) (*Result, error) {
	apiURL := fmt.Sprintf("https://store.steampowered.com/api/appdetails?appids=%s", appID)

	resp, err := s.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	var raw map[string]struct {
		Success bool              `json:"success"`
		Data    json.RawMessage   `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
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
		HeaderImage string `json:"header_image"`
	}

	for _, v := range raw {
		if !v.Success {
			return nil, fmt.Errorf("steam: app not found")
		}
		if err := json.Unmarshal(v.Data, &appData); err != nil {
			return nil, err
		}
		break
	}

	desc := appData.ShortDescription
	if desc == "" {
		desc = stripHTML(appData.DetailedDescription)
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
	}

	tags := make([]string, len(appData.Genres))
	for i, g := range appData.Genres {
		tags[i] = g.Description
	}

	coverURL := fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%s/library_600x900_2x.jpg", appID)
	if appData.HeaderImage != "" {
		_ = appData.HeaderImage
	}

	return &Result{
		Title:       appData.Name,
		Description: desc,
		Developer:   firstOrEmpty(appData.Developers),
		Publisher:   firstOrEmpty(appData.Publishers),
		ReleaseDate: appData.ReleaseDate.Date,
		Tags:        tags,
		CoverURL:    coverURL,
		Links: map[string]string{
			"steam":      fmt.Sprintf("https://store.steampowered.com/app/%s/", appID),
			"platformId": appID,
		},
	}, nil
}

func (s *SteamScraper) searchByName(dirName string) (*Result, error) {
	query := url.QueryEscape(dirName)
	apiURL := fmt.Sprintf("https://store.steampowered.com/api/storesearch/?term=%s&l=en", query)

	resp, err := s.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var searchResult struct {
		Total int `json:"total"`
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return nil, err
	}

	if searchResult.Total == 0 {
		return nil, fmt.Errorf("steam: no results for '%s'", dirName)
	}

	bestID := fmt.Sprintf("%d", searchResult.Items[0].ID)
	return s.searchByAppID(bestID)
}

func stripHTML(s string) string {
	inTag := false
	var b strings.Builder
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func firstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}
