package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type VNDBScraper struct {
	client *http.Client
}

func NewVNDBScraper() *VNDBScraper {
	return &VNDBScraper{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *VNDBScraper) Key() string { return "vndb" }

func (s *VNDBScraper) Search(gameDir string, appID string) (*Result, error) {
	_ = appID
	name := filepath.Base(gameDir)
	return s.searchByName(name)
}

func (s *VNDBScraper) searchByName(name string) (*Result, error) {
	query := map[string]interface{}{
		"filters": []interface{}{"search", "=", name},
		"fields":  "title, alttitle, released, description, developers.name, tags.name, image.url, image.sexual",
		"results": 3,
		"sort":    "searchrank",
	}

	body, _ := json.Marshal(query)
	req, err := http.NewRequest("POST", "https://api.vndb.org/kana/vn", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var apiResp struct {
		Results []struct {
			Title       string `json:"title"`
			AltTitle    string `json:"alttitle"`
			Description string `json:"description"`
			Released    string `json:"released"`
			Developers  []struct {
				Name string `json:"name"`
			} `json:"developers"`
			Tags []struct {
				Name string `json:"name"`
			} `json:"tags"`
			Image *struct {
				URL    string `json:"url"`
				Sexual float64 `json:"sexual"`
			} `json:"image"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Results) == 0 {
		return nil, fmt.Errorf("vndb: no results for '%s'", name)
	}

	vn := apiResp.Results[0]

	devName := ""
	if len(vn.Developers) > 0 {
		devName = vn.Developers[0].Name
	}

	tags := make([]string, 0, len(vn.Tags))
	for _, t := range vn.Tags {
		tags = append(tags, t.Name)
	}

	coverURL := ""
	if vn.Image != nil {
		coverURL = vn.Image.URL
	}

	desc := ""
	if vn.Description != "" {
		desc = strings.ReplaceAll(vn.Description, "\n", " ")
		desc = strings.TrimSpace(desc)
		if len(desc) > 500 {
			desc = desc[:500] + "..."
		}
	}

	releaseDate := ""
	if len(vn.Released) >= 10 {
		releaseDate = vn.Released[:10]
	}

	result := &Result{
		Title:             vn.Title,
		TitleNative:       vn.AltTitle,
		Description:       desc,
		Developer:         devName,
		ReleaseDate:       releaseDate,
		Tags:              tags,
		CoverURL:          coverURL,
		CoverLandscapeURL: coverURL,
		Links: map[string]string{
			"vndb": fmt.Sprintf("https://vndb.org/v?q=%s", name),
		},
	}

	return result, nil
}
