package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// vndbEndpoint is the Kana API query endpoint.
const vndbEndpoint = "https://api.vndb.org/kana/vn"

// vndbMinInterval keeps requests inside the documented budget of 200 requests
// per 5 minutes.
const vndbMinInterval = 1600 * time.Millisecond

// vndbFields lists the fields requested from POST /vn.
//
// Every field has to exist in the documented schema, and requesting an unknown
// one is an error rather than a warning: this string previously included
// "lang_image", which does not exist, so every VNDB request failed with HTTP 400
// and the provider never returned a single result.
//
// The type and official markers on titles are what makes language-aware title
// selection possible without the non-existent field.
const vndbFields = "id, title, alttitle, released, description, " +
	"developers.name, tags.name, image{url, sexual}, " +
	"titles{lang, title, latin, official, main}"

// VNDBScraper resolves visual novels through the VNDB Kana API.
type VNDBScraper struct {
	http     *HTTPClient
	lang     Lang
	endpoint string
}

// NewVNDBScraper creates the provider.
func NewVNDBScraper() *VNDBScraper {
	return &VNDBScraper{
		lang:     LangEnglish,
		endpoint: vndbEndpoint,
	}
}

// Key implements Source.
func (s *VNDBScraper) Key() string { return "vndb" }

// Configure implements Source.
func (s *VNDBScraper) Configure(cfg SourceConfig) error {
	s.http = cfg.HTTP
	if cfg.Language != "" {
		s.lang = cfg.Language
	}
	if s.http != nil {
		s.http.LimitHost(hostOf(s.endpoint), vndbMinInterval)
	}
	return nil
}

// Search implements Source.
func (s *VNDBScraper) Search(ctx context.Context, q Query) (*Result, error) {
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
	return nil, NoResult("vndb", q.Name)
}

type vndbVN struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	AltTitle    string `json:"alttitle"`
	Description string `json:"description"`
	Released    string `json:"released"`
	Titles      []struct {
		Lang     string `json:"lang"`
		Title    string `json:"title"`
		Latin    string `json:"latin"`
		Official bool   `json:"official"`
		Main     bool   `json:"main"`
	} `json:"titles"`
	Developers []struct {
		Name string `json:"name"`
	} `json:"developers"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	Image *struct {
		URL    string  `json:"url"`
		Sexual float64 `json:"sexual"`
	} `json:"image"`
}

func (s *VNDBScraper) searchByName(ctx context.Context, name string) (*Result, error) {
	body, err := json.Marshal(map[string]any{
		"filters": []any{"search", "=", name},
		"fields":  vndbFields,
		"results": 3,
		"sort":    "searchrank",
	})
	if err != nil {
		return nil, fmt.Errorf("vndb: encode query: %w", err)
	}

	resp, err := s.http.Do(ctx, "vndb", Request{
		Method:   "POST",
		URL:      s.endpoint,
		Headers:  map[string]string{"Content-Type": "application/json"},
		Body:     body,
		MaxBytes: 1 << 20,
	})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []vndbVN `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, &APIError{Source: "vndb", Kind: KindParse, URL: s.endpoint, Message: "unexpected /vn payload", Err: err}
	}
	if len(payload.Results) == 0 {
		return nil, NoResult("vndb", name)
	}

	vn := payload.Results[0]
	if strings.TrimSpace(vn.Title) == "" && strings.TrimSpace(vn.AltTitle) == "" {
		return nil, NoResult("vndb", name)
	}

	title, native := s.selectTitles(vn)

	tags := make([]string, 0, len(vn.Tags))
	for _, tag := range vn.Tags {
		if tag.Name != "" {
			tags = append(tags, tag.Name)
		}
	}

	coverURL := ""
	if vn.Image != nil {
		coverURL = vn.Image.URL
	}

	links := map[string]string{"vndb": "https://vndb.org/" + vn.ID}
	if vn.ID != "" {
		// Recording the entry ID turns later lookups into exact hits instead of
		// repeated name searches.
		links[PlatformIDKey] = vn.ID
	}

	return &Result{
		Title:             title,
		TitleNative:       native,
		Description:       PrepareDescription(vn.Description),
		Developer:         FirstOrEmptyDeveloper(vn.Developers),
		ReleaseDate:       vndbReleaseDate(vn.Released),
		Tags:              tags,
		CoverURL:          coverURL,
		CoverLandscapeURL: coverURL,
		Links:             links,
	}, nil
}

// selectTitles chooses the display title and the original-script title.
//
// A translated title is preferred when one exists for the configured language,
// which is the behaviour the non-existent "lang_image" field was reaching for.
// The native title always stays the original script so the UI can show both.
func (s *VNDBScraper) selectTitles(vn vndbVN) (title, native string) {
	title = strings.TrimSpace(vn.Title)
	native = strings.TrimSpace(vn.AltTitle)

	main := ""
	for _, t := range vn.Titles {
		if t.Main {
			main = strings.TrimSpace(t.Title)
			break
		}
	}
	if main == "" && len(vn.Titles) > 0 {
		main = strings.TrimSpace(vn.Titles[0].Title)
	}
	if native == "" {
		native = main
	}

	if localized := s.localizedTitle(vn); localized != "" {
		title = localized
	}
	if title == "" {
		title = native
	}
	return Unescape(title), Unescape(native)
}

// localizedTitle returns the title in the configured language, if the entry has
// one. Japanese entries already carry their Japanese title in the native field,
// so only non-original languages are worth overriding the romanised title with.
func (s *VNDBScraper) localizedTitle(vn vndbVN) string {
	wanted := s.lang.VNDBCode()
	if wanted == "" || wanted == "ja" {
		return ""
	}
	for _, t := range vn.Titles {
		if !strings.EqualFold(t.Lang, wanted) {
			continue
		}
		if title := strings.TrimSpace(t.Title); title != "" {
			return title
		}
		if title := strings.TrimSpace(t.Latin); title != "" {
			return title
		}
	}
	return ""
}

// vndbReleaseDate trims a VNDB date to the day, allowing for partial dates such
// as "2011" or "2011-06" which the API legitimately returns.
func vndbReleaseDate(released string) string {
	released = strings.TrimSpace(released)
	switch {
	case released == "", strings.EqualFold(released, "TBA"):
		return ""
	case len(released) >= len("2006-01-02"):
		return released[:len("2006-01-02")]
	default:
		return released
	}
}

// FirstOrEmptyDeveloper pulls the first developer name out of the API shape.
func FirstOrEmptyDeveloper(developers []struct {
	Name string `json:"name"`
}) string {
	if len(developers) == 0 {
		return ""
	}
	return strings.TrimSpace(developers[0].Name)
}
