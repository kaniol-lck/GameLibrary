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

type BangumiScraper struct {
	client *http.Client
}

func NewBangumiScraper() *BangumiScraper {
	return &BangumiScraper{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *BangumiScraper) Key() string { return "bangumi" }

func (s *BangumiScraper) Configure(lang string, settings map[string]string) {
	_ = lang
	_ = settings
}

func (s *BangumiScraper) Search(gameDir string, appID string) (*Result, error) {
	_ = appID
	name := filepath.Base(gameDir)
	return s.searchByName(name)
}

func (s *BangumiScraper) searchByName(name string) (*Result, error) {
	query := url.QueryEscape(name)
	searchURL := fmt.Sprintf("https://api.bgm.tv/v0/search/subject/%s?type=4&responseGroup=large&max_results=3", query)

	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "GameLibrary/0.2")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var searchResp struct {
		List []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			NameCN  string `json:"name_cn"`
			Summary string `json:"summary"`
			Date    string `json:"date"`
			Images  struct {
				Large string `json:"large"`
			} `json:"images"`
		} `json:"list"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, err
	}

	if len(searchResp.List) == 0 {
		return nil, fmt.Errorf("bangumi: no results for '%s'", name)
	}

	item := searchResp.List[0]

	title := item.NameCN
	if title == "" {
		title = item.Name
	}

	desc := strings.TrimSpace(item.Summary)
	if len(desc) > 500 {
		desc = desc[:500] + "..."
	}

	return &Result{
		Title:             title,
		TitleNative:       item.Name,
		Description:       desc,
		ReleaseDate:       item.Date,
		CoverURL:          item.Images.Large,
		CoverLandscapeURL: item.Images.Large,
		Links: map[string]string{
			"bangumi": fmt.Sprintf("https://bgm.tv/subject/%d", item.ID),
		},
	}, nil
}
