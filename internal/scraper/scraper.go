// Package scraper fetches game metadata from external providers.
//
// Every provider implements Source. The pipeline asks each enabled provider in
// the user's configured priority order and collects whatever matched, so a game
// present on several services ends up linked to all of them.
package scraper

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
	"GameLibrary/internal/logger"
)

// Result is a normalised metadata record returned by a provider.
type Result struct {
	Title             string            `json:"title"`
	TitleNative       string            `json:"titleNative"`
	Description       string            `json:"description"`
	Developer         string            `json:"developer"`
	Publisher         string            `json:"publisher"`
	ReleaseDate       string            `json:"releaseDate"`
	Tags              []string          `json:"tags"`
	CoverURL          string            `json:"coverUrl"`
	CoverLandscapeURL string            `json:"-"`
	Links             map[string]string `json:"links"`
}

// PlatformIDKey is the Links key carrying the provider's own identifier for the
// matched entry, which is stored on the game so later lookups can be exact.
const PlatformIDKey = "platformId"

// Query is everything a provider may need to identify a game.
//
// Providers used to receive a bare (gameDir, appID) pair where the appID was
// whatever happened to be the game's first platform ID. Four of the six
// providers ignored the ID and one ignored the directory, and a DLsite code could
// be handed to Steam as an app ID. The query carries the search name and a
// per-platform ID map instead, so each provider reads exactly what applies to it.
type Query struct {
	// GameDir is the absolute game directory.
	GameDir string
	// Name is the folder name to search for when no identifier is available.
	Name string
	// PlatformIDs maps a provider key to the identifier already known for it.
	PlatformIDs map[string]string
}

// NewQuery builds a query from a game record.
func NewQuery(info *game.GameInfo) Query {
	ids := make(map[string]string, len(info.Platforms))
	for _, p := range info.Platforms {
		if p.Platform != "" && p.ID != "" {
			ids[p.Platform] = p.ID
		}
	}
	return Query{
		GameDir:     info.GameDir,
		Name:        SearchName(filepath.Base(info.GameDir)),
		PlatformIDs: ids,
	}
}

// PlatformID returns the stored identifier for a provider, or "".
func (q Query) PlatformID(source string) string {
	if q.PlatformIDs == nil {
		return ""
	}
	return q.PlatformIDs[source]
}

// SteamAppID returns the Steam app ID when one is known.
func (q Query) SteamAppID() string { return q.PlatformID("steam") }

// SourceConfig is the typed configuration handed to a provider.
type SourceConfig struct {
	Language Lang
	APIKey   string
	HTTP     *HTTPClient
	Timeout  time.Duration
}

// Source is a metadata provider.
type Source interface {
	// Key is the stable identifier used in config.json.
	Key() string
	// Configure applies language, credentials and the shared HTTP client.
	// It is part of the interface so the pipeline can reconfigure every provider
	// when settings change; previously this was done with concrete type
	// assertions in main, which meant a new provider was silently missed and
	// settings only took effect after a restart.
	Configure(cfg SourceConfig) error
	// Search looks the game up. It returns ErrNoResult (wrapped) when the
	// provider simply has nothing matching, and an *APIError for anything the
	// user might act on.
	Search(ctx context.Context, q Query) (*Result, error)
}

// SourceResult pairs a provider's result with the provider that produced it.
type SourceResult struct {
	Result *Result
	Source string
}

// Pipeline runs the configured providers.
type Pipeline struct {
	cfg     *config.Config
	sources map[string]Source
	http    *HTTPClient
}

// NewPipeline creates an empty pipeline sharing one HTTP client.
func NewPipeline(cfg *config.Config) *Pipeline {
	return &Pipeline{
		cfg:     cfg,
		sources: make(map[string]Source),
		http:    NewHTTPClient(20 * time.Second),
	}
}

// HTTP exposes the shared client so cover downloads and providers use one pool.
func (p *Pipeline) HTTP() *HTTPClient { return p.http }

// Register adds a provider.
func (p *Pipeline) Register(src Source) {
	if src == nil || src.Key() == "" {
		return
	}
	p.sources[src.Key()] = src
}

// Registered returns the registered provider keys in sorted order.
func (p *Pipeline) Registered() []string {
	keys := make([]string, 0, len(p.sources))
	for key := range p.sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Configure re-applies the current settings to every registered provider and
// adopts a new configuration.
//
// This is what makes the settings page take effect immediately: it used to only
// run once during startup, so changing the language, an API key or the enabled
// provider set required restarting the application.
func (p *Pipeline) Configure(cfg *config.Config) error {
	if cfg != nil {
		p.cfg = cfg
	}
	var firstErr error
	for key, src := range p.sources {
		srcCfg := SourceConfig{
			Language: ParseLang(p.cfg.Language),
			APIKey:   p.cfg.SourceSettings(key)["apiKey"],
			HTTP:     p.http,
			Timeout:  20 * time.Second,
		}
		if err := src.Configure(srcCfg); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// enabledSources returns the providers to query, in configured priority order.
func (p *Pipeline) enabledSources() []Source {
	ordered := make([]Source, 0, len(p.cfg.Sources))
	for _, srcCfg := range p.cfg.Sources {
		if !srcCfg.Enabled {
			continue
		}
		if src, ok := p.sources[srcCfg.Key]; ok {
			ordered = append(ordered, src)
		}
	}
	return ordered
}

// disabledSourceReason explains why a configured provider will not run.
func (p *Pipeline) skipReason(key string) string {
	if _, ok := p.sources[key]; !ok {
		return "not registered"
	}
	return "disabled"
}

// ScrapeAll queries every enabled provider and returns all matches.
//
// A provider that finds nothing (ErrNoResult) or fails is skipped; the method
// only reports an error when nothing at all matched, and distinguishes "no
// provider matched" from "every provider failed" in the returned diagnostics.
func (p *Pipeline) ScrapeAll(ctx context.Context, info *game.GameInfo) ([]SourceResult, error) {
	logger.ScrapeStarted(info.ID, info.Title)

	query := NewQuery(info)

	// Only the enabled, registered providers take part, in configured priority
	// order.
	type job struct {
		index  int
		key    string
		source Source
	}
	jobs := make([]job, 0, len(p.cfg.Sources))
	for _, srcCfg := range p.cfg.Sources {
		src, ok := p.sources[srcCfg.Key]
		if !ok || !srcCfg.Enabled {
			logger.ScrapeSourceSkipped(info.ID, srcCfg.Key, p.skipReason(srcCfg.Key))
			continue
		}
		jobs = append(jobs, job{index: len(jobs), key: srcCfg.Key, source: src})
	}
	if len(jobs) == 0 {
		logger.ScrapeAllSourcesFailed(info.ID, info.Title)
		return nil, ErrNoResult
	}

	// Every provider is queried at the same time.
	//
	// The providers are unrelated services, and each host already has its own rate
	// limiter in the shared HTTP client, so running them together turns a game's
	// scrape from the sum of the providers' latencies into the slowest one. Doing
	// this sequentially was the dominant cost of a library-wide scrape.
	matches := make([]*SourceResult, len(jobs))
	authErrs := make([]error, len(jobs))

	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				authErrs[j.index] = err
				return
			}

			logger.ScrapeSourceAttempt(info.ID, j.key, query.Name)
			result, err := j.source.Search(ctx, query)
			switch {
			case err == nil && result == nil:
				logger.ScrapeSourceEmpty(info.ID, info.Title, j.key)
				return
			case err != nil && IsNoResult(err):
				logger.ScrapeSourceEmpty(info.ID, info.Title, j.key)
				return
			case err != nil:
				logger.ScrapeSourceFailed(info.ID, info.Title, j.key, err)
				authErrs[j.index] = err
				return
			}

			logger.ScrapeSuccess(info.ID, info.Title, j.key, result.Title, result.Links[PlatformIDKey])
			matches[j.index] = &SourceResult{Result: result, Source: j.key}
		}(j)
	}
	wg.Wait()

	// Results are assembled in priority order, not completion order, so the
	// preferred-source selection downstream stays deterministic.
	results := make([]SourceResult, 0, len(jobs))
	var authErr error
	for i, match := range matches {
		if match != nil {
			results = append(results, *match)
			continue
		}
		// The first actionable (authentication) problem, by priority, is the one
		// worth reporting; the rest are ordinary misses.
		if authErr == nil && authErrs[i] != nil && !IsNoResult(authErrs[i]) {
			if KindOf(authErrs[i]) == KindAuth {
				authErr = authErrs[i]
			}
		}
	}

	if len(results) == 0 {
		logger.ScrapeAllSourcesFailed(info.ID, info.Title)
		if authErr != nil {
			return nil, authErr
		}
		return nil, ErrNoResult
	}
	return results, nil
}

// ApplyResult merges a provider result into a game record.
//
// Fields are merged individually: a provider that only supplies cover art (which
// SteamGridDB does) must not blank out the title, description and developer that
// another provider already filled in. The previous implementation replaced the
// whole metadata record and could erase a game's name outright.
func ApplyResult(info *game.GameInfo, result *Result, sourceKey string) {
	if result == nil {
		return
	}

	if title := strings.TrimSpace(result.Title); title != "" {
		info.Title = title
	}
	info.AddAlias(result.TitleNative)

	platformID := ""
	if result.Links != nil {
		platformID = result.Links[PlatformIDKey]
	}
	info.SetPlatform(sourceKey, platformID, strings.TrimSpace(result.Title))

	if info.Metadata == nil {
		info.Metadata = &game.Metadata{}
	}
	mergeMetadata(info.Metadata, result)
}

// mergeMetadata copies only the non-empty fields of result over meta.
func mergeMetadata(meta *game.Metadata, result *Result) {
	if v := strings.TrimSpace(result.Description); v != "" {
		meta.Description = v
	}
	if v := strings.TrimSpace(result.Developer); v != "" {
		meta.Developer = v
	}
	if v := strings.TrimSpace(result.Publisher); v != "" {
		meta.Publisher = v
	}
	if v := strings.TrimSpace(result.ReleaseDate); v != "" {
		meta.ReleaseDate = v
	}
	if len(result.Tags) > 0 {
		meta.Tags = mergeTags(meta.Tags, result.Tags)
	}
	if len(result.Links) > 0 {
		if meta.Links == nil {
			meta.Links = make(map[string]string, len(result.Links))
		}
		for k, v := range result.Links {
			if k == PlatformIDKey || v == "" {
				continue
			}
			meta.Links[k] = v
		}
	}
}

// mergeTags appends new tags, preserving order and skipping duplicates.
func mergeTags(existing, incoming []string) []string {
	seen := make(map[string]bool, len(existing)+len(incoming))
	out := make([]string, 0, len(existing)+len(incoming))
	for _, list := range [][]string{existing, incoming} {
		for _, tag := range list {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			key := strings.ToLower(tag)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tag)
		}
	}
	return out
}
