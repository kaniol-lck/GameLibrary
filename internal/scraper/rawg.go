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
)

type RawgScraper struct {
	client *http.Client
	apiKey string
}

func NewRawgScraper() *RawgScraper {
	return &RawgScraper{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *RawgScraper) Key() string { return "rawg" }

func (s *RawgScraper) Configure(lang string, settings map[string]string) {
	_ = lang
	if settings != nil {
		if key, ok := settings["apiKey"]; ok {
			s.apiKey = key
		}
	}
}

func (s *RawgScraper) Search(gameDir string, appID string) (*Result, error) {
	_ = appID
	name := filepath.Base(gameDir)
	name = normalizeSearchName(name)

	searchTerms := nameVariations(name)
	for _, term := range searchTerms {
		r, err := s.search(term)
		if err == nil && r != nil {
			return r, nil
		}
	}
	return nil, fmt.Errorf("rawg: no results for '%s'", name)
}

func (s *RawgScraper) search(name string) (*Result, error) {
	query := url.QueryEscape(name)
	apiURL := fmt.Sprintf("https://api.rawg.io/api/games?search=%s&key=%s&page_size=1", query, s.apiKey)

	resp, err := s.client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var searchResp struct {
		Results []struct {
			ID             int    `json:"id"`
			Name           string `json:"name"`
			Slug           string `json:"slug"`
			Released       string `json:"released"`
			Description    string `json:"description_raw"`
			Metacritic     int    `json:"metacritic"`
			BackgroundImg  string `json:"background_image"`
			Genres         []struct{ Name string } `json:"genres"`
			Platforms      []struct {
				Platform struct{ Name string } `json:"platform"`
			} `json:"platforms"`
			Developers []struct{ Name string } `json:"developers"`
			Publishers []struct{ Name string } `json:"publishers"`
			Tags       []struct{ Name string } `json:"tags"`
			Website    string `json:"website"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, err
	}

	if len(searchResp.Results) == 0 {
		return nil, fmt.Errorf("rawg: no results for '%s'", name)
	}

	item := searchResp.Results[0]

	desc := item.Description
	if len(desc) > 500 {
		desc = desc[:500] + "..."
	}

	tags := make([]string, 0)
	for _, g := range item.Genres {
		tags = append(tags, g.Name)
	}
	for _, t := range item.Tags {
		tags = append(tags, t.Name)
	}

	platforms := make([]string, len(item.Platforms))
	for i, p := range item.Platforms {
		platforms[i] = p.Platform.Name
	}

	dev := ""
	pub := ""
	if len(item.Developers) > 0 {
		dev = item.Developers[0].Name
	}
	if len(item.Publishers) > 0 {
		pub = item.Publishers[0].Name
	}

	links := map[string]string{
		"rawg":    fmt.Sprintf("https://rawg.io/games/%s", item.Slug),
		"website": item.Website,
	}

	return &Result{
		Title:             item.Name,
		Description:       desc,
		Developer:         dev,
		Publisher:         pub,
		ReleaseDate:       item.Released,
		Tags:              tags,
		CoverURL:          item.BackgroundImg,
		CoverLandscapeURL: item.BackgroundImg,
		Links:             links,
	}, nil
}

func normalizeSearchName(name string) string {
	name = strings.TrimSpace(name)

	prefixes := []string{"steam_", "RJ", "rj"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return name
		}
	}

	return name
}

func nameVariations(name string) []string {
	vars := []string{name}
	seen := map[string]bool{name: true}

	spaced := splitCamelCase(name)
	if !seen[spaced] {
		vars = append(vars, spaced)
		seen[spaced] = true
	}

	noDash := strings.ReplaceAll(name, "-", " ")
	if !seen[noDash] {
		vars = append(vars, noDash)
		seen[noDash] = true
	}

	noUnderscore := strings.ReplaceAll(name, "_", " ")
	if !seen[noUnderscore] {
		vars = append(vars, noUnderscore)
		seen[noUnderscore] = true
	}

	noDashNoUC := strings.ReplaceAll(spaced, "-", " ")
	if !seen[noDashNoUC] {
		vars = append(vars, noDashNoUC)
		seen[noDashNoUC] = true
	}

	return vars
}

func splitCamelCase(s string) string {
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && isUpper(r) && !isUpper(runes[i-1]) {
			result.WriteRune(' ')
		}
		if i > 1 && i < len(runes)-1 && isUpper(runes[i-1]) && isLower(r) {
			result.WriteRune(' ')
		}
		result.WriteRune(r)
	}
	return result.String()
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
