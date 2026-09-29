package scraper

import (
	"context"
	"errors"
	"testing"
	"time"

	"GameLibrary/internal/config"
)

// delaySource is a provider with a controllable latency, used to observe whether
// providers are queried concurrently.
type delaySource struct {
	key    string
	delay  time.Duration
	result *Result
	err    error
	calls  int
}

func (s *delaySource) Key() string { return s.key }

func (s *delaySource) Configure(SourceConfig) error { return nil }

func (s *delaySource) Search(ctx context.Context, _ Query) (*Result, error) {
	s.calls++
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.result, s.err
}

func TestPipelineQueriesProvidersConcurrently(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "a", Enabled: true},
		config.MetadataSource{Key: "b", Enabled: true},
		config.MetadataSource{Key: "c", Enabled: true},
	)

	const delay = 200 * time.Millisecond
	pipeline := NewPipeline(cfg)
	pipeline.Register(&delaySource{key: "a", delay: delay, result: &Result{Title: "A"}})
	pipeline.Register(&delaySource{key: "b", delay: delay, result: &Result{Title: "B"}})
	pipeline.Register(&delaySource{key: "c", delay: delay, result: &Result{Title: "C"}})
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	start := time.Now()
	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ScrapeAll: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Sequentially this would take 3 × delay; concurrently it is roughly one delay.
	// The bound allows for scheduling noise while still failing loudly if the
	// providers are serialised again.
	if elapsed > 2*delay {
		t.Errorf("providers appear to be serialised: three %s requests took %s", delay, elapsed)
	}
}

// TestPipelineKeepsConfiguredResultOrderWhileRunningConcurrently pins the contract
// the preferred-source selection depends on: results arrive in configured priority
// order even when a lower-priority provider answers first.
func TestPipelineKeepsConfiguredResultOrderWhileRunningConcurrently(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "slow", Enabled: true},
		config.MetadataSource{Key: "fast", Enabled: true},
	)

	pipeline := NewPipeline(cfg)
	pipeline.Register(&delaySource{key: "slow", delay: 150 * time.Millisecond, result: &Result{Title: "Slow"}})
	pipeline.Register(&delaySource{key: "fast", delay: 5 * time.Millisecond, result: &Result{Title: "Fast"}})
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err != nil {
		t.Fatalf("ScrapeAll: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Source != "slow" || results[1].Source != "fast" {
		t.Fatalf("expected configured order [slow fast], got [%s %s]", results[0].Source, results[1].Source)
	}
}

// TestPipelineSkipsNothingWhenAllProvidersMiss keeps the "no result" outcome exact
// under concurrency.
func TestPipelineSkipsNothingWhenAllProvidersMiss(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "a", Enabled: true},
		config.MetadataSource{Key: "b", Enabled: true},
	)

	pipeline := NewPipeline(cfg)
	pipeline.Register(&delaySource{key: "a", err: NoResult("a", "x")})
	pipeline.Register(&delaySource{key: "b", err: NoResult("b", "x")})
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	results, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	// "Nothing matched" is reported as ErrNoResult so the caller can tell it apart
	// from an outage.
	if !IsNoResult(err) {
		t.Fatalf("expected ErrNoResult, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

// TestPipelineSurfacesAuthErrorFromAnyProvider checks that an actionable
// authentication problem is still reported when it comes from a provider that is
// not first in the priority order.
func TestPipelineSurfacesAuthErrorFromAnyProvider(t *testing.T) {
	cfg := pipelineConfig(
		config.MetadataSource{Key: "miss", Enabled: true},
		config.MetadataSource{Key: "needs-key", Enabled: true},
	)

	pipeline := NewPipeline(cfg)
	pipeline.Register(&delaySource{key: "miss", err: NoResult("miss", "x")})
	pipeline.Register(&delaySource{key: "needs-key", err: &APIError{Source: "needs-key", Kind: KindAuth, Message: "API key required"}})
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := pipeline.ScrapeAll(context.Background(), testGameInfo())
	if err == nil {
		t.Fatal("expected the authentication failure to be surfaced")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an *APIError, got %T: %v", err, err)
	}
	if apiErr.Kind != KindAuth {
		t.Errorf("expected an auth error, got %v", apiErr.Kind)
	}
	if apiErr.Source != "needs-key" {
		t.Errorf("expected the failing provider to be named, got %q", apiErr.Source)
	}
}

// TestPipelineHonoursContextCancellation covers the case where the queue stops
// while a game is being scraped.
func TestPipelineHonoursContextCancellation(t *testing.T) {
	cfg := pipelineConfig(config.MetadataSource{Key: "slow", Enabled: true})

	slow := &delaySource{key: "slow", delay: 5 * time.Second, result: &Result{Title: "Slow"}}
	pipeline := NewPipeline(cfg)
	pipeline.Register(slow)
	if err := pipeline.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = pipeline.ScrapeAll(ctx, testGameInfo())
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ScrapeAll did not return after the context was cancelled")
	}
}
