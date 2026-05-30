package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"GameLibrary/internal/logger"
)

type SteamScraper struct {
	client   *http.Client
	language string
	apiKey   string
}

func NewSteamScraper() *SteamScraper {
	return &SteamScraper{
		client:   &http.Client{Timeout: 15 * time.Second},
		language: "en-US",
	}
}

func (s *SteamScraper) Key() string { return "steam" }

func (s *SteamScraper) Configure(lang string, settings map[string]string) {
	if lang != "" {
		s.language = lang
	}
	if settings != nil {
		if key, ok := settings["apiKey"]; ok {
			s.apiKey = key
		}
	}
}

func (s *SteamScraper) Search(gameDir string, appID string) (*Result, error) {
	if appID != "" {
		logger.Debug("steam scraper: searching by appID", "appId", appID)
		return s.searchByAppID(appID)
	}

	baseName := filepath.Base(gameDir)
	searchName := normalizeSearchName(baseName)
	logger.Debug("steam scraper: searching by name", "searchTerm", searchName, "gameDir", gameDir)

	terms := nameVariations(searchName)
	for _, term := range terms {
		r, err := s.searchByName(term)
		if err == nil && r != nil {
			return r, nil
		}
	}
	return nil, fmt.Errorf("steam: no results for '%s'", baseName)
}

func (s *SteamScraper) searchByAppID(appID string) (*Result, error) {
	langParam := ""
	if s.language != "en-US" {
		langParam = "&l=" + languageCode(s.language)
	}
	apiURL := fmt.Sprintf("https://store.steampowered.com/api/appdetails?appids=%s%s", appID, langParam)

	resp, err := s.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	var raw map[string]struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
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

	coverPortrait := fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%s/library_600x900_2x.jpg", appID)
	coverLandscape := fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%s/header.jpg", appID)

	return &Result{
		Title:             appData.Name,
		Description:       desc,
		Developer:         firstOrEmpty(appData.Developers),
		Publisher:         firstOrEmpty(appData.Publishers),
		ReleaseDate:       appData.ReleaseDate.Date,
		Tags:              tags,
		CoverURL:          coverPortrait,
		CoverLandscapeURL: coverLandscape,
		Links: map[string]string{
			"steam":      fmt.Sprintf("https://store.steampowered.com/app/%s/", appID),
			"platformId": appID,
		},
	}, nil
}

func (s *SteamScraper) searchByName(name string) (*Result, error) {
	query := url.QueryEscape(name)
	apiURL := fmt.Sprintf("https://store.steampowered.com/api/storesearch/?term=%s&l=%s", query, languageCode(s.language))

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
		return nil, fmt.Errorf("steam: no results for '%s'", name)
	}

	bestID := fmt.Sprintf("%d", searchResult.Items[0].ID)
	logger.Debug("steam name search result", "searchTerm", name, "matchedId", bestID, "matchedName", searchResult.Items[0].Name)
	return s.searchByAppID(bestID)
}

func languageCode(lang string) string {
	switch lang {
	case "zh-CN":
		return "schinese"
	case "ja-JP":
		return "japanese"
	default:
		return "english"
	}
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
