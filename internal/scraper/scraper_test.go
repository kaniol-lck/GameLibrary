package scraper

// This suite is fully offline: every provider is pointed at an httptest server
// and the shared HTTP client is configured to fail fast (one attempt) so a
// deliberately failing handler never makes the suite sleep.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
)

// --- test helpers -----------------------------------------------------------

// testHTTPClient returns a client with a short timeout and no retries.
func testHTTPClient() *HTTPClient {
	client := NewHTTPClient(5 * time.Second)
	client.SetRetryPolicy(RetryPolicy{Attempts: 1})
	return client
}

// recordedRequest is one request a test server served.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// requestRecorder collects requests so assertions run after Search returns
// without racing the server goroutine.
type requestRecorder struct {
	mu   sync.Mutex
	reqs []recordedRequest
}

func (r *requestRecorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, recordedRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Query:  req.URL.RawQuery,
		Header: req.Header.Clone(),
		Body:   body,
	})
}

func (r *requestRecorder) snapshot() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.reqs))
	copy(out, r.reqs)
	return out
}

func (r *requestRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *requestRecorder) paths() []string {
	out := make([]string, 0, r.count())
	for _, req := range r.snapshot() {
		out = append(out, req.Path)
	}
	return out
}

// providerServer starts a recording test server and closes it when the test
// ends.
func providerServer(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *requestRecorder) {
	t.Helper()
	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		respond(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(payload))
}

// asAPIError asserts err is an *APIError and returns it.
func asAPIError(t *testing.T, err error) *APIError {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an *APIError, got %T: %v", err, err)
	}
	return apiErr
}

func testGameInfo() *game.GameInfo {
	return game.New("root", filepath.Join("C:", "Games", "SteinsGate"), nil, "")
}

// --- 1. Bangumi regression --------------------------------------------------

// TestBangumiPostsToSubjectsEndpoint is a regression test.
//
// The provider used to issue GET /v0/search/subject/{keyword}?responseGroup=...
// No such route exists in the Bangumi v0 API, so every request came back 404 and
// the provider could never match anything. The documented endpoint is a POST of
// a JSON body to /v0/search/subjects with a ?limit= parameter, and its results
// live under "data".
func TestBangumiPostsToSubjectsEndpoint(t *testing.T) {
	const payload = `{"total":1,"limit":3,"offset":0,"data":[{"id":123,` +
		`"name":"Steins;Gate","name_cn":"命运石之门","summary":"s","date":"2011-06-16",` +
		`"images":{"large":"https://x/l.jpg"}}]}`

	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, payload)
	})

	bangumi := NewBangumiScraper()
	bangumi.endpoint = srv.URL + "/v0/search/subjects"
	if err := bangumi.Configure(SourceConfig{HTTP: testHTTPClient(), Language: LangChinese}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := bangumi.Search(context.Background(), Query{Name: "Steins;Gate"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}

	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 request, got %d: %v", len(reqs), rec.paths())
	}
	req := reqs[0]

	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST: /v0/search/subject/{kw} does not exist in the v0 API", req.Method)
	}
	if !strings.HasSuffix(req.Path, "/v0/search/subjects") {
		t.Errorf("path = %q, want it to end with /v0/search/subjects", req.Path)
	}
	if !strings.Contains(req.Query, "limit=") {
		t.Errorf("query = %q, want a limit= parameter", req.Query)
	}
	if got := req.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", got)
	}

	var body struct {
		Keyword string `json:"keyword"`
		Sort    string `json:"sort"`
		Filter  struct {
			Type []int `json:"type"`
		} `json:"filter"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("decode request body %q: %v", req.Body, err)
	}
	if body.Keyword != "Steins;Gate" {
		t.Errorf("keyword = %q, want %q", body.Keyword, "Steins;Gate")
	}
	if len(body.Filter.Type) != 1 || body.Filter.Type[0] != bangumiSubjectTypeGame {
		t.Errorf("filter.type = %v, want [%d]", body.Filter.Type, bangumiSubjectTypeGame)
	}

	if result.Title != "命运石之门" {
		t.Errorf("Title = %q, want the Chinese name 命运石之门", result.Title)
	}
	if result.TitleNative != "Steins;Gate" {
		t.Errorf("TitleNative = %q, want %q", result.TitleNative, "Steins;Gate")
	}
	if result.ReleaseDate != "2011-06-16" {
		t.Errorf("ReleaseDate = %q, want 2011-06-16", result.ReleaseDate)
	}
	if result.CoverURL != "https://x/l.jpg" {
		t.Errorf("CoverURL = %q, want https://x/l.jpg", result.CoverURL)
	}
	if result.Links[PlatformIDKey] != "123" {
		t.Errorf("Links[%s] = %q, want 123", PlatformIDKey, result.Links[PlatformIDKey])
	}
}

// --- 2. VNDB regression -----------------------------------------------------

// TestVNDBSearchUsesDocumentedFields is a regression test.
//
// The provider used to request a field named "lang_image", which does not exist
// in the VNDB schema. VNDB rejects unknown fields with HTTP 400 rather than
// ignoring them, so every request failed and the provider never matched
// anything. Language-aware titles come from titles{lang,title,latin} instead.
func TestVNDBSearchUsesDocumentedFields(t *testing.T) {
	const payload = `{
		"results": [{
			"id": "v24",
			"title": "Steins;Gate",
			"alttitle": "シュタインズ・ゲート",
			"released": "2011-06-16",
			"description": "Time travel &amp; parallel worlds.",
			"titles": [
				{"lang": "ja", "title": "シュタインズ・ゲート", "latin": "Steins;Gate", "official": true, "main": true},
				{"lang": "zh-Hans", "title": "命运石之门", "latin": "Ming Yun Shi Zhi Men", "official": false, "main": false},
				{"lang": "en", "title": "Steins;Gate", "latin": "Steins;Gate", "official": true, "main": false}
			],
			"developers": [{"id": "p13", "name": "Nitroplus"}],
			"tags": [{"id": "g7", "name": "Science Fiction", "rating": 2.4}, {"id": "g12", "name": "Time Travel", "rating": 2.1}],
			"image": {"url": "https://t.vndb.org/cv/24/1.jpg", "sexual": 0.0, "violence": 0.0}
		}],
		"more": false
	}`

	cases := []struct {
		name       string
		lang       Lang
		wantTitle  string
		wantNative string
	}{
		{
			name:       "chinese picks the zh-Hans title",
			lang:       LangChinese,
			wantTitle:  "命运石之门",
			wantNative: "シュタインズ・ゲート",
		},
		{
			name:       "japanese keeps the romanised title and the native alttitle",
			lang:       LangJapanese,
			wantTitle:  "Steins;Gate",
			wantNative: "シュタインズ・ゲート",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, payload)
			})

			vndb := NewVNDBScraper()
			vndb.endpoint = srv.URL
			if err := vndb.Configure(SourceConfig{HTTP: testHTTPClient(), Language: tc.lang}); err != nil {
				t.Fatalf("Configure: %v", err)
			}

			result, err := vndb.Search(context.Background(), Query{Name: "Steins;Gate"})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if result == nil {
				t.Fatal("Search returned no result")
			}

			reqs := rec.snapshot()
			if len(reqs) != 1 {
				t.Fatalf("expected exactly 1 request, got %d", len(reqs))
			}
			if reqs[0].Method != http.MethodPost {
				t.Errorf("method = %s, want POST", reqs[0].Method)
			}

			var body struct {
				Filters []any  `json:"filters"`
				Fields  string `json:"fields"`
				Results int    `json:"results"`
				Sort    string `json:"sort"`
			}
			if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
				t.Fatalf("decode request body %q: %v", reqs[0].Body, err)
			}
			if strings.Contains(body.Fields, "lang_image") {
				t.Errorf("fields still requests lang_image, which VNDB rejects with HTTP 400: %q", body.Fields)
			}
			if !strings.Contains(body.Fields, "titles{") {
				t.Errorf("fields = %q, want it to contain titles{", body.Fields)
			}
			if body.Results != 3 {
				t.Errorf("results = %d, want 3", body.Results)
			}

			if result.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", result.Title, tc.wantTitle)
			}
			if result.TitleNative != tc.wantNative {
				t.Errorf("TitleNative = %q, want %q", result.TitleNative, tc.wantNative)
			}
			if result.ReleaseDate != "2011-06-16" {
				t.Errorf("ReleaseDate = %q, want 2011-06-16", result.ReleaseDate)
			}
			if result.Description != "Time travel & parallel worlds." {
				t.Errorf("Description = %q, want the unescaped description", result.Description)
			}
			if result.Developer != "Nitroplus" {
				t.Errorf("Developer = %q, want Nitroplus", result.Developer)
			}
			if !reflect.DeepEqual(result.Tags, []string{"Science Fiction", "Time Travel"}) {
				t.Errorf("Tags = %v, want the tag names", result.Tags)
			}
			if result.CoverURL != "https://t.vndb.org/cv/24/1.jpg" {
				t.Errorf("CoverURL = %q, want the image url", result.CoverURL)
			}
			if result.Links[PlatformIDKey] != "v24" {
				t.Errorf("Links[%s] = %q, want v24", PlatformIDKey, result.Links[PlatformIDKey])
			}
			if result.Links["vndb"] != "https://vndb.org/v24" {
				t.Errorf("Links[vndb] = %q", result.Links["vndb"])
			}
		})
	}
}

// TestVNDBReleaseDateTrimsToDay covers the partial dates VNDB legitimately
// returns.
func TestVNDBReleaseDateTrimsToDay(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2011-06-16", "2011-06-16"},
		{"2011-06-16T00:00:00Z", "2011-06-16"},
		{"2011-06", "2011-06"},
		{"2011", "2011"},
		{"TBA", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := vndbReleaseDate(tc.in); got != tc.want {
			t.Errorf("vndbReleaseDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- 3. ApplyResult merges instead of replacing -----------------------------

// TestApplyResultMergesInsteadOfReplacing is a regression test.
//
// The old implementation replaced the whole metadata record with whatever the
// last provider returned. SteamGridDB returns covers and nothing else, so
// enabling it as the preferred source erased a game's title, description and
// developer.
func TestApplyResultMergesInsteadOfReplacing(t *testing.T) {
	info := testGameInfo()
	info.Title = "Steins;Gate"
	info.Metadata = &game.Metadata{
		Description: "Original description",
		Developer:   "Original Developer",
	}

	// A cover-only result: every metadata field is empty.
	ApplyResult(info, &Result{CoverURL: "https://cdn.example/cover.png"}, "steamgriddb")

	if info.Title != "Steins;Gate" {
		t.Errorf("Title = %q, want the existing title to survive a cover-only result", info.Title)
	}
	if info.Metadata.Description != "Original description" {
		t.Errorf("Description = %q, want the existing description to survive", info.Metadata.Description)
	}
	if info.Metadata.Developer != "Original Developer" {
		t.Errorf("Developer = %q, want the existing developer to survive", info.Metadata.Developer)
	}

	// A second provider fills the remaining gaps and adds a platform + alias.
	ApplyResult(info, &Result{
		Title:       "命运石之门",
		TitleNative: "Steins;Gate",
		Publisher:   "Nitroplus",
		ReleaseDate: "2011-06-16",
		Tags:        []string{"Science Fiction"},
		Links: map[string]string{
			PlatformIDKey: "v24",
			"vndb":        "https://vndb.org/v24",
		},
	}, "vndb")

	if info.Title != "命运石之门" {
		t.Errorf("Title = %q, want the non-empty provider title to win", info.Title)
	}
	if !info.HasPlatform("vndb") {
		t.Error("expected a vndb platform link")
	}
	if got := info.PlatformID("vndb"); got != "v24" {
		t.Errorf("vndb platform id = %q, want v24", got)
	}
	if info.Metadata.Publisher != "Nitroplus" {
		t.Errorf("Publisher = %q, want the gap to be filled", info.Metadata.Publisher)
	}
	if info.Metadata.ReleaseDate != "2011-06-16" {
		t.Errorf("ReleaseDate = %q, want the gap to be filled", info.Metadata.ReleaseDate)
	}
	if len(info.Metadata.Tags) != 1 || info.Metadata.Tags[0] != "Science Fiction" {
		t.Errorf("Tags = %v, want the incoming tags", info.Metadata.Tags)
	}
	if info.Metadata.Links["vndb"] != "https://vndb.org/v24" {
		t.Errorf("Links = %v, want the vndb link recorded and platformId excluded", info.Metadata.Links)
	}
	if _, ok := info.Metadata.Links[PlatformIDKey]; ok {
		t.Errorf("Links = %v, want platformId kept out of the metadata links", info.Metadata.Links)
	}
	foundAlias := false
	for _, alias := range info.Aliases {
		if alias == "Steins;Gate" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Errorf("Aliases = %v, want the provider's native title kept as an alias", info.Aliases)
	}
}

// TestApplyResultIgnoresBlankValues covers a whitespace-only incoming value: it
// must not blank an existing field.
func TestApplyResultIgnoresBlankValues(t *testing.T) {
	info := testGameInfo()
	info.Title = "Steins;Gate"
	info.Metadata = &game.Metadata{
		Description: "Description",
		Developer:   "Developer",
		Publisher:   "Publisher",
		ReleaseDate: "2011-06-16",
	}

	ApplyResult(info, &Result{
		Title:       "   ",
		Description: " \t ",
		Developer:   "\n",
		Publisher:   "  ",
		ReleaseDate: "\t",
		Tags:        []string{"  ", ""},
	}, "blank")

	if info.Title != "Steins;Gate" {
		t.Errorf("Title = %q, want the existing title", info.Title)
	}
	if info.Metadata.Description != "Description" {
		t.Errorf("Description = %q, want the existing description", info.Metadata.Description)
	}
	if info.Metadata.Developer != "Developer" {
		t.Errorf("Developer = %q, want the existing developer", info.Metadata.Developer)
	}
	if info.Metadata.Publisher != "Publisher" {
		t.Errorf("Publisher = %q, want the existing publisher", info.Metadata.Publisher)
	}
	if info.Metadata.ReleaseDate != "2011-06-16" {
		t.Errorf("ReleaseDate = %q, want the existing release date", info.Metadata.ReleaseDate)
	}
	if len(info.Metadata.Tags) != 0 {
		t.Errorf("Tags = %v, want whitespace-only tags dropped", info.Metadata.Tags)
	}
}

// --- 4. Pipeline ------------------------------------------------------------

// stubSource is a configurable in-test provider.
type stubSource struct {
	key    string
	result *Result
	err    error

	mu    *sync.Mutex
	order *[]string
	calls int
	cfg   SourceConfig
}

func (s *stubSource) Key() string { return s.key }

func (s *stubSource) Configure(cfg SourceConfig) error {
	s.cfg = cfg
	return nil
}

func (s *stubSource) Search(context.Context, Query) (*Result, error) {
	if s.mu != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	s.calls++
	if s.order != nil {
		*s.order = append(*s.order, s.key)
	}
	return s.result, s.err
}

func (s *stubSource) callCount() int {
	if s.mu != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	return s.calls
}

func pipelineConfig(sources ...config.MetadataSource) *config.Config {
	return &config.Config{Language: "zh-CN", Sources: sources}
}

func TestPipelineQueriesProvidersInConfiguredOrder(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "first", Enabled: true},
		config.MetadataSource{Key: "second", Enabled: true},
		config.MetadataSource{Key: "disabled", Enabled: false},
	)

	var mu sync.Mutex
	var order []string
	first := &stubSource{key: "first", result: &Result{Title: "First"}, mu: &mu, order: &order}
	second := &stubSource{key: "second", result: &Result{Title: "Second"}, mu: &mu, order: &order}
	disabled := &stubSource{key: "disabled", result: &Result{Title: "Disabled"}, mu: &mu, order: &order}

	pipeline := NewPipeline(cfg)
	pipeline.Register(first)
	pipeline.Register(second)
	pipeline.Register(disabled)
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if got := pipeline.Registered(); !reflect.DeepEqual(got, []string{"disabled", "first", "second"}) {
		t.Errorf("Registered() = %v", got)
	}

	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err != nil {
		t.Fatalf("ScrapeAll: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want one per matching provider: %v", len(results), results)
	}
	if results[0].Source != "first" || results[1].Source != "second" {
		t.Errorf("results came back as %q,%q; want the configured order", results[0].Source, results[1].Source)
	}
	if results[0].Result.Title != "First" || results[1].Result.Title != "Second" {
		t.Errorf("results carry the wrong payloads: %v, %v", results[0].Result, results[1].Result)
	}
	// Providers are queried concurrently, so the order in which they are *called*
	// is not defined. What has to stay deterministic is which results come back and
	// in what order, because the preferred-source selection downstream depends on
	// it.
	queried := map[string]int{}
	for _, key := range order {
		queried[key]++
	}
	if queried["first"] != 1 || queried["second"] != 1 {
		t.Errorf("expected each enabled provider to be queried exactly once, got %v", order)
	}
	if disabled.callCount() != 0 {
		t.Errorf("a disabled provider was queried %d times", disabled.callCount())
	}
	if first.cfg.HTTP != pipeline.HTTP() {
		t.Error("Configure did not hand the pipeline's shared HTTP client to the provider")
	}
	if first.cfg.Language != LangChinese {
		t.Errorf("provider language = %q, want %q", first.cfg.Language, LangChinese)
	}
}

func TestPipelineSkipsNoResultSources(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "miss", Enabled: true},
		config.MetadataSource{Key: "hit", Enabled: true},
	)

	miss := &stubSource{key: "miss", err: NoResult("miss", "SteinsGate")}
	hit := &stubSource{key: "hit", result: &Result{Title: "Steins;Gate"}}

	pipeline := NewPipeline(cfg)
	pipeline.Register(miss)
	pipeline.Register(hit)
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err != nil {
		t.Fatalf("a wrapped ErrNoResult must not fail the run: %v", err)
	}
	if len(results) != 1 || results[0].Source != "hit" {
		t.Fatalf("results = %v, want only the provider that matched", results)
	}
}

func TestPipelineNoMatchIsNoResultError(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "miss", Enabled: true},
		config.MetadataSource{Key: "absent", Enabled: true},
	)

	miss := &stubSource{key: "miss", err: NoResult("miss", "SteinsGate")}

	pipeline := NewPipeline(cfg)
	pipeline.Register(miss)
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err == nil {
		t.Fatal("expected an error when nothing matched")
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want IsNoResult to be true", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want none", results)
	}
}

// TestPipelineSurfacesAuthError covers how a missing API key reaches the user:
// the only enabled provider fails with an auth error, which must be surfaced
// instead of a generic "no result".
func TestPipelineSurfacesAuthError(t *testing.T) {
	cfg := pipelineConfig(config.MetadataSource{Key: "rawg", Enabled: true})

	authErr := &APIError{Source: "rawg", Kind: KindAuth, Message: "API key required"}
	rawg := &stubSource{key: "rawg", err: authErr}

	pipeline := NewPipeline(cfg)
	pipeline.Register(rawg)
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err == nil {
		t.Fatal("expected an error")
	}
	if IsNoResult(err) {
		t.Fatalf("err = %v, want the auth error rather than a generic no-result", err)
	}
	apiErr := asAPIError(t, err)
	if apiErr.Kind != KindAuth {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindAuth)
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("err = %v, want the provider's explanation", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want none", results)
	}
}

func TestPipelineEmptyConfigDoesNotPanic(t *testing.T) {
	pipeline := NewPipeline(&config.Config{})
	pipeline.Register(NewSteamScraper())
	if err := pipeline.Configure(&config.Config{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err == nil || !IsNoResult(err) {
		t.Fatalf("err = %v, want a no-result error when no source is enabled", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want none", results)
	}
}

// --- 5. Steam ---------------------------------------------------------------

func TestSteamSearchByAppID(t *testing.T) {
	const payload = `{"570":{"success":true,"data":{` +
		`"name":"Test Game",` +
		`"short_description":"A <b>short</b> description",` +
		`"detailed_description":"long description",` +
		`"developers":["Dev One","Dev Two"],` +
		`"publishers":["Pub One"],` +
		`"release_date":{"date":"Nov 1, 2023"},` +
		`"genres":[{"description":"Action"},{"description":"Indie"}]}}}`

	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "storesearch") {
			t.Errorf("store search must not run when the app id is known: %s", r.URL)
		}
		writeJSON(t, w, payload)
	})

	steam := NewSteamScraper()
	steam.appDetailsURL = srv.URL + "/api/appdetails"
	steam.storeSearchURL = srv.URL + "/api/storesearch/"
	if err := steam.Configure(SourceConfig{HTTP: testHTTPClient(), Language: LangChinese}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := steam.Search(context.Background(), Query{
		Name:        "Test Game",
		PlatformIDs: map[string]string{"steam": "570"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}

	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 request, got %d: %v", len(reqs), rec.paths())
	}
	params, err := url.ParseQuery(reqs[0].Query)
	if err != nil {
		t.Fatalf("parse query %q: %v", reqs[0].Query, err)
	}
	if !strings.Contains(reqs[0].Query, "appids=") {
		t.Errorf("query = %q, want an appids= parameter", reqs[0].Query)
	}
	if got := params.Get("appids"); got != "570" {
		t.Errorf("appids = %q, want 570", got)
	}
	if got := params.Get("l"); got != "schinese" {
		t.Errorf("l = %q, want schinese for zh-CN", got)
	}

	if result.Title != "Test Game" {
		t.Errorf("Title = %q", result.Title)
	}
	if result.Description != "A short description" {
		t.Errorf("Description = %q, want the HTML stripped", result.Description)
	}
	if result.Developer != "Dev One" {
		t.Errorf("Developer = %q, want the first developer", result.Developer)
	}
	if result.Publisher != "Pub One" {
		t.Errorf("Publisher = %q", result.Publisher)
	}
	if result.ReleaseDate != "Nov 1, 2023" {
		t.Errorf("ReleaseDate = %q", result.ReleaseDate)
	}
	if !reflect.DeepEqual(result.Tags, []string{"Action", "Indie"}) {
		t.Errorf("Tags = %v, want the genre descriptions", result.Tags)
	}
	const wantCover = "https://cdn.cloudflare.steamstatic.com/steam/apps/570/library_600x900_2x.jpg"
	if result.CoverURL != wantCover {
		t.Errorf("CoverURL = %q, want %q", result.CoverURL, wantCover)
	}
	if !strings.HasSuffix(result.CoverLandscapeURL, "/apps/570/header.jpg") {
		t.Errorf("CoverLandscapeURL = %q, want the header image", result.CoverLandscapeURL)
	}
	if result.Links[PlatformIDKey] != "570" {
		t.Errorf("Links[%s] = %q, want 570", PlatformIDKey, result.Links[PlatformIDKey])
	}
	if result.Links["steam"] != "https://store.steampowered.com/app/570/" {
		t.Errorf("Links[steam] = %q", result.Links["steam"])
	}
}

func TestSteamSuccessFalseIsNoResult(t *testing.T) {
	srv, _ := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"570":{"success":false}}`)
	})

	steam := NewSteamScraper()
	steam.appDetailsURL = srv.URL + "/api/appdetails"
	steam.storeSearchURL = srv.URL + "/api/storesearch/"
	if err := steam.Configure(SourceConfig{HTTP: testHTTPClient()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := steam.Search(context.Background(), Query{
		Name:        "Test Game",
		PlatformIDs: map[string]string{"steam": "570"},
	})
	if result != nil {
		t.Errorf("result = %v, want nil for success:false", result)
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want a no-result error", err)
	}
}

// TestSteamSkipsStoreSearchForDLsiteCode covers a folder named after a DLsite
// product code: the Steam store search would only return noise, so it is skipped
// entirely.
func TestSteamSkipsStoreSearchForDLsiteCode(t *testing.T) {
	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP request expected for a DLsite product code, got %s", r.URL)
	})

	steam := NewSteamScraper()
	steam.appDetailsURL = srv.URL + "/api/appdetails"
	steam.storeSearchURL = srv.URL + "/api/storesearch/"
	if err := steam.Configure(SourceConfig{HTTP: testHTTPClient()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := steam.Search(context.Background(), Query{Name: "RJ123456"})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want a no-result error", err)
	}
	if got := rec.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

// TestSteamStoreSearchEmptyItems covers total>0 with an empty items array, which
// used to index out of range and panic the calling webview.
func TestSteamStoreSearchEmptyItems(t *testing.T) {
	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"total":5,"items":[]}`)
	})

	steam := NewSteamScraper()
	steam.appDetailsURL = srv.URL + "/api/appdetails"
	steam.storeSearchURL = srv.URL + "/api/storesearch/"
	if err := steam.Configure(SourceConfig{HTTP: testHTTPClient()}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := steam.Search(context.Background(), Query{Name: "Some Game"})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want a no-result error", err)
	}
	for _, path := range rec.paths() {
		if strings.Contains(path, "appdetails") {
			t.Errorf("appdetails was queried despite an empty items array: %v", rec.paths())
		}
	}
}

// --- 6. SteamGridDB ---------------------------------------------------------

func TestSteamGridDBWithoutSteamIDMakesNoRequest(t *testing.T) {
	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP request expected without a Steam app id, got %s", r.URL)
	})

	griddb := NewSteamGridDBScraper()
	griddb.endpoint = srv.URL + "/api/v2/grids/steam/%s"
	if err := griddb.Configure(SourceConfig{HTTP: testHTTPClient(), APIKey: "key"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := griddb.Search(context.Background(), Query{Name: "Some Game"})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want a no-result error", err)
	}
	if got := rec.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

func TestSteamGridDBMissingAPIKeyIsAuthError(t *testing.T) {
	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP request expected without an API key, got %s", r.URL)
	})

	griddb := NewSteamGridDBScraper()
	griddb.endpoint = srv.URL + "/api/v2/grids/steam/%s"
	if err := griddb.Configure(SourceConfig{HTTP: testHTTPClient(), APIKey: "  "}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := griddb.Search(context.Background(), Query{
		Name:        "Some Game",
		PlatformIDs: map[string]string{"steam": "570"},
	})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	apiErr := asAPIError(t, err)
	if apiErr.Kind != KindAuth {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindAuth)
	}
	if got := rec.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

// TestSteamGridDBUnauthorizedIsAuthError covers the old behaviour of reporting
// "no grids found" for an invalid API key.
func TestSteamGridDBUnauthorizedIsAuthError(t *testing.T) {
	srv, _ := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"error":"invalid key"}`))
	})

	griddb := NewSteamGridDBScraper()
	griddb.endpoint = srv.URL + "/api/v2/grids/steam/%s"
	if err := griddb.Configure(SourceConfig{HTTP: testHTTPClient(), APIKey: "bad"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := griddb.Search(context.Background(), Query{
		Name:        "Some Game",
		PlatformIDs: map[string]string{"steam": "570"},
	})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if IsNoResult(err) {
		t.Fatalf("err = %v, want an auth error rather than a %q report", err, "no grids found")
	}
	apiErr := asAPIError(t, err)
	if apiErr.Kind != KindAuth {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindAuth)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", apiErr.Status)
	}
}

func TestSteamGridDBPicksPortraitAndLandscape(t *testing.T) {
	const payload = `{"success":true,"data":[` +
		`{"id":1,"url":"https://cdn.steamgriddb.com/land.png","width":920,"height":430},` +
		`{"id":2,"url":"https://cdn.steamgriddb.com/port.png","width":600,"height":900},` +
		`{"id":3,"url":"https://cdn.steamgriddb.com/other.png","width":600,"height":900}]}`

	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, payload)
	})

	griddb := NewSteamGridDBScraper()
	griddb.endpoint = srv.URL + "/api/v2/grids/steam/%s"
	if err := griddb.Configure(SourceConfig{HTTP: testHTTPClient(), APIKey: "secret"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := griddb.Search(context.Background(), Query{
		Name:        "Some Game",
		PlatformIDs: map[string]string{"steam": "570"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}
	if result.CoverURL != "https://cdn.steamgriddb.com/port.png" {
		t.Errorf("CoverURL = %q, want the portrait grid (height > width)", result.CoverURL)
	}
	if result.CoverLandscapeURL != "https://cdn.steamgriddb.com/land.png" {
		t.Errorf("CoverLandscapeURL = %q, want the landscape grid (width > height)", result.CoverLandscapeURL)
	}
	if result.Links[PlatformIDKey] != "570" {
		t.Errorf("Links[%s] = %q, want 570", PlatformIDKey, result.Links[PlatformIDKey])
	}

	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(reqs))
	}
	if reqs[0].Path != "/api/v2/grids/steam/570" {
		t.Errorf("path = %q, want the app id in the URL", reqs[0].Path)
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Authorization = %q, want the API key", got)
	}
}

// --- 7. RAWG ----------------------------------------------------------------

func TestRawgMissingAPIKeyIsAuthError(t *testing.T) {
	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP request expected without an API key, got %s", r.URL)
	})

	rawg := NewRawgScraper()
	rawg.endpoint = srv.URL + "/api/games"
	if err := rawg.Configure(SourceConfig{HTTP: testHTTPClient(), Language: LangChinese}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := rawg.Search(context.Background(), Query{Name: "Grand Theft Auto V"})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	apiErr := asAPIError(t, err)
	if apiErr.Kind != KindAuth {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindAuth)
	}
	if got := rec.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

func TestRawgSearchPopulatesFields(t *testing.T) {
	const payload = `{"count":1,"results":[{` +
		`"id":3498,"name":"Grand Theft Auto V","slug":"grand-theft-auto-v",` +
		`"released":"2013-09-17",` +
		`"description_raw":"<p>Grand Theft Auto V is an action-adventure game.</p>",` +
		`"background_image":"https://media.rawg.io/media/games/20a/gta-v.jpg",` +
		`"website":"https://www.rockstargames.com/V/",` +
		`"genres":[{"id":4,"name":"Action"},{"id":3,"name":"Adventure"}],` +
		`"tags":[{"id":31,"name":"Singleplayer"},{"id":7,"name":"Multiplayer"}],` +
		`"developers":[{"id":3524,"name":"Rockstar North"}],` +
		`"publishers":[{"id":215,"name":"Rockstar Games"}]}]}`

	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, payload)
	})

	rawg := NewRawgScraper()
	rawg.endpoint = srv.URL + "/api/games"
	if err := rawg.Configure(SourceConfig{HTTP: testHTTPClient(), APIKey: "test-key", Language: LangChinese}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	result, err := rawg.Search(context.Background(), Query{Name: "Grand Theft Auto V"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}

	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(reqs))
	}
	params, err := url.ParseQuery(reqs[0].Query)
	if err != nil {
		t.Fatalf("parse query %q: %v", reqs[0].Query, err)
	}
	if params.Get("key") != "test-key" {
		t.Errorf("key = %q, want the configured API key", params.Get("key"))
	}
	if params.Get("page_size") != "1" {
		t.Errorf("page_size = %q, want 1", params.Get("page_size"))
	}

	if result.Title != "Grand Theft Auto V" {
		t.Errorf("Title = %q", result.Title)
	}
	if result.Description != "Grand Theft Auto V is an action-adventure game." {
		t.Errorf("Description = %q, want the HTML stripped", result.Description)
	}
	if result.Developer != "Rockstar North" {
		t.Errorf("Developer = %q", result.Developer)
	}
	if result.Publisher != "Rockstar Games" {
		t.Errorf("Publisher = %q", result.Publisher)
	}
	if result.ReleaseDate != "2013-09-17" {
		t.Errorf("ReleaseDate = %q", result.ReleaseDate)
	}
	wantTags := []string{"Action", "Adventure", "Singleplayer", "Multiplayer"}
	if !reflect.DeepEqual(result.Tags, wantTags) {
		t.Errorf("Tags = %v, want %v", result.Tags, wantTags)
	}
	if result.CoverURL != "https://media.rawg.io/media/games/20a/gta-v.jpg" {
		t.Errorf("CoverURL = %q", result.CoverURL)
	}
	if result.Links[PlatformIDKey] != "3498" {
		t.Errorf("Links[%s] = %q, want 3498", PlatformIDKey, result.Links[PlatformIDKey])
	}
	if result.Links["rawg"] != "https://rawg.io/games/grand-theft-auto-v" {
		t.Errorf("Links[rawg] = %q", result.Links["rawg"])
	}
	if result.Links["website"] != "https://www.rockstargames.com/V/" {
		t.Errorf("Links[website] = %q", result.Links["website"])
	}
}

// --- 8. DLsite --------------------------------------------------------------

// withDLsiteSections points the DLsite product URLs at a test server. The
// sections are package state because the provider has no endpoint field.
func withDLsiteSections(t *testing.T, sections []string) {
	t.Helper()
	original := dlsiteSections
	dlsiteSections = sections
	t.Cleanup(func() { dlsiteSections = original })
}

const dlsiteProductPage = `<html><head>
<meta property="og:title" content="Sample Work [RJ123456] | DLsite">
<meta property="og:image" content="//img.dlsite.jp/resize/works/=/x.jpg">
<meta property="og:description" content="A sample work with &#39;quotes&#39;">
</head><body>RJ123456</body></html>`

func newDLsiteForTest(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) (*DLsiteScraper, *requestRecorder) {
	t.Helper()
	srv, rec := providerServer(t, respond)
	withDLsiteSections(t, []string{
		srv.URL + "/maniax/work/=/product_id/%s.html",
		srv.URL + "/home/work/=/product_id/%s.html",
	})
	dlsite := NewDLsiteScraper()
	if err := dlsite.Configure(SourceConfig{HTTP: testHTTPClient(), Language: LangChinese}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return dlsite, rec
}

func TestDLsiteParsesProductPage(t *testing.T) {
	dlsite, rec := newDLsiteForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(dlsiteProductPage))
	})

	result, err := dlsite.Search(context.Background(), Query{Name: "RJ123456"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}

	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected the first section to match and stop, got %d requests: %v", len(reqs), rec.paths())
	}
	if reqs[0].Path != "/maniax/work/=/product_id/RJ123456.html" {
		t.Errorf("path = %q, want the RJ code from the folder name", reqs[0].Path)
	}

	if result.Title != "Sample Work [RJ123456]" {
		t.Errorf("Title = %q, want the page title with the site decoration stripped", result.Title)
	}
	if result.Description != "A sample work with 'quotes'" {
		t.Errorf("Description = %q", result.Description)
	}
	if result.CoverURL != "https://img.dlsite.jp/resize/works/=/x.jpg" {
		t.Errorf("CoverURL = %q, want the protocol-relative og:image made absolute", result.CoverURL)
	}
	if result.Links[PlatformIDKey] != "RJ123456" {
		t.Errorf("Links[%s] = %q, want RJ123456", PlatformIDKey, result.Links[PlatformIDKey])
	}
	if !strings.HasSuffix(result.Links["dlsite"], reqs[0].Path) {
		t.Errorf("Links[dlsite] = %q, want it to end with %q", result.Links["dlsite"], reqs[0].Path)
	}
	if !reflect.DeepEqual(result.Tags, []string{"Doujin", "DLsite"}) {
		t.Errorf("Tags = %v", result.Tags)
	}
}

// TestDLsiteCodeFromFolderNameDrivesURL covers a lower-case code in the folder
// name being normalised before it is used.
func TestDLsiteCodeFromFolderNameDrivesURL(t *testing.T) {
	dlsite, rec := newDLsiteForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(dlsiteProductPage))
	})

	result, err := dlsite.Search(context.Background(), Query{Name: "rj123456"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}
	reqs := rec.snapshot()
	if len(reqs) == 0 {
		t.Fatal("no request was made")
	}
	if reqs[0].Path != "/maniax/work/=/product_id/RJ123456.html" {
		t.Errorf("path = %q, want the upper-cased code", reqs[0].Path)
	}
	if result.Links[PlatformIDKey] != "RJ123456" {
		t.Errorf("Links[%s] = %q, want the normalised code", PlatformIDKey, result.Links[PlatformIDKey])
	}
}

func TestDLsiteRejectsPageWithoutCode(t *testing.T) {
	cases := []struct {
		name string
		page string
	}{
		{"age gate without the product code", `<html><head><meta property="og:title" content="年齢確認"></head><body>Adults only</body></html>`},
		{"product not found marker", `<html><head><meta property="og:title" content="Not Found"></head><body>この作品は存在しません</body></html>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dlsite, rec := newDLsiteForTest(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.page))
			})

			result, err := dlsite.Search(context.Background(), Query{Name: "RJ123456"})
			if result != nil {
				t.Errorf("result = %v, want nil", result)
			}
			if !IsNoResult(err) {
				t.Errorf("err = %v, want a no-result error", err)
			}
			if got := rec.count(); got != 2 {
				t.Errorf("the handler was called %d times, want both sections tried", got)
			}
		})
	}
}

// TestExtractMetaHandlesQuoteStylesAndAttributeOrder is a regression test: the
// old extractor required the exact byte sequence `<meta property="x" content="`
// and silently returned nothing for any other order or a single-quoted document.
func TestExtractMetaHandlesQuoteStylesAndAttributeOrder(t *testing.T) {
	cases := []struct {
		name string
		html string
		prop string
		want string
	}{
		{
			name: "double quotes, property first",
			html: `<meta property="og:title" content="A">`,
			prop: "og:title",
			want: "A",
		},
		{
			name: "single quotes, property first",
			html: `<meta property='og:title' content='Single Quoted'>`,
			prop: "og:title",
			want: "Single Quoted",
		},
		{
			name: "reversed attribute order",
			html: `<meta content="Reversed" property="og:title">`,
			prop: "og:title",
			want: "Reversed",
		},
		{
			name: "reversed order and single quotes",
			html: `<meta content='Reversed Single' property='og:title'>`,
			prop: "og:title",
			want: "Reversed Single",
		},
		{
			name: "name attribute instead of property",
			html: `<meta name="description" content="ByName">`,
			prop: "description",
			want: "ByName",
		},
		{
			name: "property with other attributes between",
			html: `<meta data-x="1" property="og:image" data-y="2" content="https://x/i.png">`,
			prop: "og:image",
			want: "https://x/i.png",
		},
		{
			name: "absent",
			html: `<meta property="og:title" content="A">`,
			prop: "og:image",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractMeta(tc.html, tc.prop); got != tc.want {
				t.Errorf("extractMeta(%q, %q) = %q, want %q", tc.html, tc.prop, got, tc.want)
			}
		})
	}
}

func TestDLsiteNoCodeInNameIsNoResult(t *testing.T) {
	dlsite, rec := newDLsiteForTest(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP request expected without a product code, got %s", r.URL)
	})

	result, err := dlsite.Search(context.Background(), Query{Name: "Some Visual Novel"})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want a no-result error", err)
	}
	if got := rec.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

func TestDLsiteRemembersConfirmedCode(t *testing.T) {
	const stored = "RJ999999"
	dlsite, rec := newDLsiteForTest(t, func(w http.ResponseWriter, r *http.Request) {
		page := fmt.Sprintf(`<html><head><meta property="og:title" content="Stored %s">`+
			`<meta property="og:image" content="//img.dlsite.jp/x.jpg"></head><body>%s</body></html>`, stored, stored)
		_, _ = w.Write([]byte(page))
	})

	// A previously confirmed platform id takes precedence over name inference.
	result, err := dlsite.Search(context.Background(), Query{
		Name:        "RJ123456",
		PlatformIDs: map[string]string{"dlsite": "rj999999"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result == nil {
		t.Fatal("Search returned no result")
	}
	reqs := rec.snapshot()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].Path != "/maniax/work/=/product_id/RJ999999.html" {
		t.Errorf("path = %q, want the stored code to win over the folder name", reqs[0].Path)
	}
	if result.Links[PlatformIDKey] != stored {
		t.Errorf("Links[%s] = %q, want %q", PlatformIDKey, result.Links[PlatformIDKey], stored)
	}
}

// TestDLsiteStoredCodeAloneIsNotEnough documents a limitation of the current
// implementation: a stored DLsite platform id is only consulted after a product
// code has been found in the folder name, so a game whose folder carries no code
// is never looked up even when its id is already known. Reported, not worked
// around here.
func TestDLsiteStoredCodeAloneIsNotEnough(t *testing.T) {
	dlsite, rec := newDLsiteForTest(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s", r.URL)
	})

	result, err := dlsite.Search(context.Background(), Query{
		Name:        "unhelpful folder",
		PlatformIDs: map[string]string{"dlsite": "RJ123456"},
	})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if !IsNoResult(err) {
		t.Errorf("err = %v, want a no-result error", err)
	}
	if got := rec.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

// --- 9. HTTPClient ----------------------------------------------------------

func TestHTTPClientRetriesServerError(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewHTTPClient(5 * time.Second)
	client.SetRetryPolicy(RetryPolicy{
		Attempts:  3,
		BaseDelay: time.Millisecond,
		MaxDelay:  time.Millisecond,
	})

	resp, err := client.Do(context.Background(), "test", Request{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("body = %q, want ok", resp.Body)
	}
	if got := atomic.LoadInt32(&attempts); got < 2 {
		t.Errorf("the handler saw %d requests, want at least 2 (a 500 is retryable)", got)
	}
}

func TestHTTPClientDoesNotRetryNotFound(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	client := NewHTTPClient(5 * time.Second)
	client.SetRetryPolicy(RetryPolicy{
		Attempts:  3,
		BaseDelay: time.Millisecond,
		MaxDelay:  time.Millisecond,
	})

	resp, err := client.Do(context.Background(), "test", Request{URL: srv.URL})
	if resp != nil {
		t.Errorf("resp = %v, want nil for a 404", resp)
	}
	apiErr := asAPIError(t, err)
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", apiErr.Status)
	}
	if apiErr.Kind != KindClient {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindClient)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("the handler saw %d requests, want exactly 1: a 404 is not retryable", got)
	}
}

func TestHTTPClientRateLimited(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("slow down"))
	}))
	defer srv.Close()

	client := NewHTTPClient(5 * time.Second)
	client.SetRetryPolicy(RetryPolicy{
		Attempts:  2,
		BaseDelay: time.Millisecond,
		MaxDelay:  time.Millisecond,
	})

	resp, err := client.Do(context.Background(), "test", Request{URL: srv.URL})
	if resp != nil {
		t.Errorf("resp = %v, want nil", resp)
	}
	apiErr := asAPIError(t, err)
	if apiErr.Kind != KindRateLimited {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindRateLimited)
	}
	if apiErr.Status != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want 429", apiErr.Status)
	}
	if !apiErr.IsRetryable() {
		t.Error("a rate limit should be retryable")
	}
}

// TestHTTPClientLongRetryAfterDoesNotBlock covers the queue-worker guard: a
// Retry-After longer than MaxRetryWait fails the request instead of sleeping.
func TestHTTPClientLongRetryAfterDoesNotBlock(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewHTTPClient(5 * time.Second)
	client.SetRetryPolicy(RetryPolicy{
		Attempts:     3,
		BaseDelay:    time.Millisecond,
		MaxDelay:     time.Millisecond,
		MaxRetryWait: 10 * time.Millisecond,
	})

	start := time.Now()
	_, err := client.Do(context.Background(), "test", Request{URL: srv.URL})
	if err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Do blocked for %v, want an immediate give-up", elapsed)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("the handler saw %d requests, want 1", got)
	}
}

func TestHTTPClientMaxBytesTruncates(t *testing.T) {
	const body = "0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewHTTPClient(5 * time.Second)
	client.SetRetryPolicy(RetryPolicy{Attempts: 1})

	resp, err := client.Do(context.Background(), "test", Request{URL: srv.URL, MaxBytes: 5})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(resp.Body) != "01234" {
		t.Errorf("body = %q, want the first 5 bytes", resp.Body)
	}
}

func TestClassifyStatus(t *testing.T) {
	cases := []struct {
		status int
		want   ErrorKind
	}{
		{http.StatusUnauthorized, KindAuth},
		{http.StatusForbidden, KindAuth},
		{http.StatusTooManyRequests, KindRateLimited},
		{http.StatusNotFound, KindClient},
		{http.StatusBadRequest, KindClient},
		{http.StatusInternalServerError, KindServer},
		{http.StatusBadGateway, KindServer},
		{http.StatusServiceUnavailable, KindServer},
	}
	for _, tc := range cases {
		if got := classifyStatus(tc.status); got != tc.want {
			t.Errorf("classifyStatus(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestKindOf(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"api error", &APIError{Kind: KindAuth}, KindAuth},
		{"wrapped api error", fmt.Errorf("wrapped: %w", &APIError{Kind: KindServer}), KindServer},
		{"no result sentinel", NoResult("steam", "term"), KindNoResult},
		{"plain error", errors.New("boom"), KindNetwork},
		{"nil", nil, KindNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := KindOf(tc.err); got != tc.want {
				t.Errorf("KindOf(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestErrorKindString(t *testing.T) {
	cases := map[ErrorKind]string{
		KindNoResult:    "no-result",
		KindNetwork:     "network",
		KindAuth:        "auth",
		KindRateLimited: "rate-limited",
		KindServer:      "server",
		KindClient:      "client",
		KindParse:       "parse",
		ErrorKind(99):   "unknown",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}

func TestAPIErrorMessage(t *testing.T) {
	err := &APIError{Source: "rawg", Status: 401, Kind: KindAuth, Message: "API key required"}
	got := err.Error()
	for _, want := range []string{"rawg", "HTTP 401", "auth", "API key required"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
}

func TestHostOfAndURLBoundaries(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.vndb.org/kana/vn", "api.vndb.org"},
		{"http://127.0.0.1:1234/x", "127.0.0.1:1234"},
		{"no-scheme", "no-scheme"},
	}
	for _, tc := range cases {
		if got := hostOf(tc.in); got != tc.want {
			t.Errorf("hostOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- 10. Pure helpers -------------------------------------------------------

func TestStripHTML(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<b>Hello</b> World", "Hello World"},
		{"<br/>Line<br>Break", "LineBreak"},
		{"No tags here", "No tags here"},
		{"<a href='x'>link</a>", "link"},
		{"<p>one</p>\n<p>two</p>", "one two"},
		{"  spaced   out  ", "spaced out"},
		{"<p>a &amp; b</p>", "a &amp; b"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := StripHTML(tc.in); got != tc.want {
			t.Errorf("StripHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  a \n\t b  ", "a b"},
		{"\u3000日本語\u3000", "日本語"},
		{"", ""},
		{"single", "single"},
	}
	for _, tc := range cases {
		if got := CleanText(tc.in); got != tc.want {
			t.Errorf("CleanText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTruncateCutsOnRuneBoundaries covers the old byte-slicing truncation, which
// produced invalid UTF-8 for every Japanese or Chinese description.
func TestTruncateCutsOnRuneBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short enough is untouched", "hello", 10, "hello"},
		{"ascii cut", "hello world", 5, "hello..."},
		{"trailing space is not left before the ellipsis", "ab cd", 2, "ab..."},
		{"japanese cut on a rune boundary", "こんにちは世界", 4, "こんにち..."},
		{"japanese fits exactly", "こんにちは", 5, "こんにちは"},
		{"zero max returns the input", "hello", 0, "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("Truncate(%q, %d) produced invalid UTF-8: %q", tc.in, tc.max, got)
			}
		})
	}
}

func TestTruncateLongJapaneseStringIsValidUTF8(t *testing.T) {
	in := strings.Repeat("あ", 1200)
	got := Truncate(in, MaxDescriptionRunes)
	if !utf8.ValidString(got) {
		t.Fatal("Truncate produced invalid UTF-8")
	}
	if runes := utf8.RuneCountInString(got); runes > MaxDescriptionRunes+3 {
		t.Errorf("rune count = %d, want at most %d", runes, MaxDescriptionRunes+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("want a trailing ellipsis, got %q", got[len(got)-8:])
	}
}

func TestPrepareDescription(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"tags and entities", "<p>Hello &amp; welcome</p>", "Hello & welcome"},
		{"plain text", "  Just text  ", "Just text"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrepareDescription(tc.in); got != tc.want {
				t.Errorf("PrepareDescription(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	long := strings.Repeat("あ", MaxDescriptionRunes+50)
	got := PrepareDescription(long)
	if !utf8.ValidString(got) {
		t.Fatal("PrepareDescription produced invalid UTF-8")
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("a too-long description should be truncated with an ellipsis")
	}
	if runes := utf8.RuneCountInString(got); runes > MaxDescriptionRunes+3 {
		t.Errorf("rune count = %d, want at most %d", runes, MaxDescriptionRunes+3)
	}
}

// TestPrepareDescriptionStripsMarkupBeforeUnescaping documents an ordering wart
// in PrepareDescription: it calls CleanText(Unescape(StripHTML(s))), so markup
// that the page encoded as HTML entities (&lt;p&gt;) is decoded *after* the tags
// have been removed and survives as literal text in the description. Reported,
// not worked around here.
func TestPrepareDescriptionStripsMarkupBeforeUnescaping(t *testing.T) {
	if got, want := PrepareDescription("A <b>bold</b> claim"), "A bold claim"; got != want {
		t.Errorf("PrepareDescription with literal markup = %q, want %q", got, want)
	}
	if got, want := PrepareDescription("A &lt;b&gt;bold&lt;/b&gt; claim"), "A <b>bold</b> claim"; got != want {
		t.Errorf("PrepareDescription with entity-encoded markup = %q, want %q", got, want)
	}
}

func TestFirstOrEmpty(t *testing.T) {
	if got := FirstOrEmpty([]string{"a", "b"}); got != "a" {
		t.Errorf("FirstOrEmpty([]string) = %q, want a", got)
	}
	if got := FirstOrEmpty([]string(nil)); got != "" {
		t.Errorf("FirstOrEmpty(nil) = %q, want the zero value", got)
	}
	if got := FirstOrEmpty([]int{7}); got != 7 {
		t.Errorf("FirstOrEmpty([]int{7}) = %d, want 7", got)
	}
	if got := FirstOrEmpty([]int{}); got != 0 {
		t.Errorf("FirstOrEmpty([]int{}) = %d, want 0", got)
	}
}

func TestRJCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"RJ123456", "RJ123456"},
		{"rj123456", "RJ123456"},
		{"RJ12345678", "RJ12345678"},
		{"rj00123456", "RJ00123456"},
		{"[RJ123456]", "RJ123456"},
		{"Game RJ123456 v1", "RJ123456"},
		{"XRJ1234567", ""},
		{"RJ123", ""},
		{"NoRJHere", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := RJCode(tc.in); got != tc.want {
			t.Errorf("RJCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSearchTerms(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "camel case is split",
			in:   "SteinsGate",
			want: []string{"SteinsGate", "Steins Gate"},
		},
		{
			name: "separators become spaces",
			in:   "hello-world",
			want: []string{"hello-world", "hello world"},
		},
		{
			name: "duplicates are dropped",
			in:   "Clannad",
			want: []string{"Clannad"},
		},
		{
			name: "release group bracket is stripped",
			in:   "Clannad [FitGirl Repack]",
			want: []string{"Clannad [FitGirl Repack]", "Clannad [Fit Girl Repack]", "Clannad"},
		},
		{
			name: "year suffix is stripped",
			in:   "Game (2019)",
			want: []string{"Game (2019)", "Game"},
		},
		{
			name: "blank input has no terms",
			in:   "   ",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SearchTerms(tc.in)
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("SearchTerms(%q) = %v, want none", tc.in, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SearchTerms(%q) = %v, want %v", tc.in, got, tc.want)
			}
			seen := map[string]bool{}
			for _, term := range got {
				key := strings.ToLower(term)
				if seen[key] {
					t.Errorf("SearchTerms(%q) returned the duplicate %q", tc.in, term)
				}
				seen[key] = true
			}
		})
	}
}

func TestBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Games\SteinsGate`, "SteinsGate"},
		{`C:\Games\SteinsGate\`, "SteinsGate"},
		{"/mnt/games/SteinsGate/", "SteinsGate"},
		{"SteinsGate", "SteinsGate"},
		{"  SteinsGate  ", "SteinsGate"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := BaseName(tc.in); got != tc.want {
			t.Errorf("BaseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSearchName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"steam_570", "570"},
		{"STEAM_570", "570"},
		{"steam_abc", "steam_abc"},
		{"Steins;Gate", "Steins;Gate"},
		{"  padded  ", "padded"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := SearchName(tc.in); got != tc.want {
			t.Errorf("SearchName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLangCodes(t *testing.T) {
	parse := []struct {
		in   string
		want Lang
	}{
		{"zh-CN", LangChinese},
		{"zh", LangChinese},
		{"schinese", LangChinese},
		{"zh-hans", LangChinese},
		{"ZH-CN", LangChinese},
		{"ja-JP", LangJapanese},
		{"ja", LangJapanese},
		{"japanese", LangJapanese},
		{"en-US", LangEnglish},
		{"", LangEnglish},
		{"klingon", LangEnglish},
	}
	for _, tc := range parse {
		if got := ParseLang(tc.in); got != tc.want {
			t.Errorf("ParseLang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	steam := []struct {
		lang Lang
		want string
	}{
		{LangChinese, "schinese"},
		{LangJapanese, "japanese"},
		{LangEnglish, "english"},
	}
	for _, tc := range steam {
		if got := tc.lang.SteamCode(); got != tc.want {
			t.Errorf("%q.SteamCode() = %q, want %q", tc.lang, got, tc.want)
		}
	}

	vndb := []struct {
		lang Lang
		want string
	}{
		{LangChinese, "zh-Hans"},
		{LangJapanese, "ja"},
		{LangEnglish, "en"},
	}
	for _, tc := range vndb {
		if got := tc.lang.VNDBCode(); got != tc.want {
			t.Errorf("%q.VNDBCode() = %q, want %q", tc.lang, got, tc.want)
		}
	}
}

func TestTrimWrappedParens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Steins;Gate (PC)", "Steins;Gate"},
		{"Steins;Gate", "Steins;Gate"},
		{"(PC)", "(PC)"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := TrimWrappedParens(tc.in); got != tc.want {
			t.Errorf("TrimWrappedParens(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"//img.dlsite.jp/x.jpg", "https://img.dlsite.jp/x.jpg"},
		{"https://img.dlsite.jp/x.jpg", "https://img.dlsite.jp/x.jpg"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeURL(tc.in); got != tc.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNewQueryBuildsPlatformMap(t *testing.T) {
	info := game.New("root", filepath.Join("C:", "Games", "steam_570"), nil, "570")
	info.Platforms = append(info.Platforms, game.PlatformInfo{Platform: "vndb", ID: "v24"})
	info.Platforms = append(info.Platforms, game.PlatformInfo{Platform: "empty", ID: ""})

	query := NewQuery(info)
	// NewQuery runs the folder name through SearchName, so the "steam_" prefix is
	// already gone.
	if query.Name != "570" {
		t.Errorf("Name = %q, want the steam_ prefix stripped", query.Name)
	}
	if got := query.SteamAppID(); got != "570" {
		t.Errorf("SteamAppID() = %q, want 570", got)
	}
	if got := query.PlatformID("vndb"); got != "v24" {
		t.Errorf("PlatformID(vndb) = %q, want v24", got)
	}
	if _, ok := query.PlatformIDs["empty"]; ok {
		t.Errorf("PlatformIDs = %v, want empty ids omitted", query.PlatformIDs)
	}
	if got := (Query{}).PlatformID("vndb"); got != "" {
		t.Errorf("PlatformID on a zero Query = %q, want empty", got)
	}
}

// --- 11. CoverFetcher -------------------------------------------------------

// minimalPNG is a real 1x1 PNG produced by image/png. http.DetectContentType
// reports it as image/png, which is what the extension sniffing relies on.
const minimalPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGJSckkDBAAA//8BYADP647EsAAAAABJRU5ErkJggg=="

func minimalPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(minimalPNGBase64)
	if err != nil {
		t.Fatalf("decode the embedded PNG: %v", err)
	}
	if got := http.DetectContentType(data); got != "image/png" {
		t.Fatalf("the embedded bytes sniff as %q, want image/png", got)
	}
	return data
}

// TestCoverFetchDerivesExtensionFromContent is a regression test: the old code
// trusted the Content-Type header, so a PNG or WebP payload served as
// "image/jpeg" was stored as ".jpg" and rendered as a broken image that the
// existence check then considered valid forever.
func TestCoverFetchDerivesExtensionFromContent(t *testing.T) {
	pngData := minimalPNG(t)
	gameDir := t.TempDir()

	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg") // deliberately wrong
		_, _ = w.Write(pngData)
	})

	fetcher := NewCoverFetcher(testHTTPClient())
	ok, err := fetcher.Fetch(context.Background(), gameDir, "local_x", srv.URL+"/cover.jpg", CoverPortrait, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !ok {
		t.Fatal("Fetch reported no usable cover")
	}
	if got := rec.count(); got != 1 {
		t.Errorf("the handler was called %d times, want 1", got)
	}

	path := Existing(gameDir, CoverPortrait)
	if path == "" {
		t.Fatal("no stored cover was found")
	}
	if !strings.HasSuffix(path, CoverPortrait+".png") {
		t.Errorf("stored cover = %q, want a .png name derived from the bytes", path)
	}
	if dir := filepath.Dir(path); dir != game.CoverDir(gameDir) {
		t.Errorf("stored cover = %q, want it inside %q", path, game.CoverDir(gameDir))
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stored cover: %v", err)
	}
	if !bytes.Equal(stored, pngData) {
		t.Error("the stored bytes differ from the served bytes")
	}
	if _, err := os.Stat(filepath.Join(game.CoverDir(gameDir), CoverPortrait+".jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a .jpg copy should not exist: %v", err)
	}
}

func TestCoverFetchRejectsNonImage(t *testing.T) {
	gameDir := t.TempDir()
	srv, _ := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("definitely not an image"))
	})

	fetcher := NewCoverFetcher(testHTTPClient())
	ok, err := fetcher.Fetch(context.Background(), gameDir, "local_x", srv.URL+"/cover.jpg", CoverPortrait, false)
	if ok {
		t.Error("Fetch accepted a non-image body")
	}
	apiErr := asAPIError(t, err)
	if apiErr.Kind != KindParse {
		t.Errorf("Kind = %v, want %v", apiErr.Kind, KindParse)
	}
	if Existing(gameDir, CoverPortrait) != "" {
		t.Error("a rejected body must not leave a cover behind")
	}
}

// TestCoverExistingIgnoresZeroLength covers the old check, which treated an
// empty file as a valid cover and then skipped the download forever.
func TestCoverExistingIgnoresZeroLength(t *testing.T) {
	pngData := minimalPNG(t)
	gameDir := t.TempDir()
	covers := game.CoverDir(gameDir)
	if err := os.MkdirAll(covers, 0o755); err != nil {
		t.Fatalf("create covers dir: %v", err)
	}
	empty := filepath.Join(covers, CoverPortrait+".png")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatalf("write empty cover: %v", err)
	}
	if got := Existing(gameDir, CoverPortrait); got != "" {
		t.Fatalf("Existing = %q, want a zero-length file to be ignored", got)
	}

	srv, rec := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	})
	fetcher := NewCoverFetcher(testHTTPClient())
	ok, err := fetcher.Fetch(context.Background(), gameDir, "local_x", srv.URL+"/cover.png", CoverPortrait, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !ok {
		t.Fatal("an empty placeholder must not block the download")
	}
	if got := rec.count(); got != 1 {
		t.Errorf("the handler was called %d times, want 1", got)
	}

	// A non-empty cover does count as existing and skips the download.
	if Existing(gameDir, CoverPortrait) == "" {
		t.Fatal("the downloaded cover was not found")
	}
	srv2, rec2 := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected fetch when a cover already exists: %s", r.URL)
	})
	ok, err = fetcher.Fetch(context.Background(), gameDir, "local_x", srv2.URL+"/cover.png", CoverPortrait, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !ok {
		t.Error("Fetch reported failure although a cover was already stored")
	}
	if got := rec2.count(); got != 0 {
		t.Errorf("the handler was called %d times, want 0", got)
	}
}

func TestCoverRemove(t *testing.T) {
	pngData := minimalPNG(t)
	gameDir := t.TempDir()
	srv, _ := providerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	})

	fetcher := NewCoverFetcher(testHTTPClient())
	if ok, err := fetcher.Fetch(context.Background(), gameDir, "local_x", srv.URL+"/cover.png", CoverLandscape, false); err != nil || !ok {
		t.Fatalf("Fetch = %v, %v", ok, err)
	}
	path := Existing(gameDir, CoverLandscape)
	if path == "" {
		t.Fatal("no landscape cover stored")
	}

	if err := Remove(gameDir, CoverLandscape); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := Existing(gameDir, CoverLandscape); got != "" {
		t.Errorf("Existing after Remove = %q, want empty", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the cover file still exists: %v", err)
	}
	// Removing again is not an error.
	if err := Remove(gameDir, CoverLandscape); err != nil {
		t.Errorf("second Remove: %v", err)
	}
}

func TestCoverFetchEmptyURLUsesExisting(t *testing.T) {
	gameDir := t.TempDir()
	fetcher := NewCoverFetcher(testHTTPClient())

	ok, err := fetcher.Fetch(context.Background(), gameDir, "local_x", "   ", CoverPortrait, false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if ok {
		t.Error("Fetch reported a cover although none exists and no URL was given")
	}

	pngData := minimalPNG(t)
	if err := os.MkdirAll(game.CoverDir(gameDir), 0o755); err != nil {
		t.Fatalf("create covers dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(game.CoverDir(gameDir), CoverPortrait+".png"), pngData, 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	if ok, err := fetcher.Fetch(context.Background(), gameDir, "local_x", "", CoverPortrait, false); err != nil || !ok {
		t.Errorf("Fetch with no URL and an existing cover = %v, %v; want true, nil", ok, err)
	}
}
