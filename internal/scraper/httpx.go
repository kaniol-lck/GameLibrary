package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrorKind classifies a failed request so the pipeline can decide whether to
// give up, retry, or tell the user to fix their settings.
type ErrorKind int

const (
	// KindNoResult means the source answered, but had nothing matching. This is
	// a normal outcome, not a failure.
	KindNoResult ErrorKind = iota
	// KindNetwork means the request never produced a usable response.
	KindNetwork
	// KindAuth means the credentials were rejected or missing.
	KindAuth
	// KindRateLimited means the source asked us to slow down.
	KindRateLimited
	// KindServer means the source failed on its side.
	KindServer
	// KindClient means the request itself was rejected as invalid.
	KindClient
	// KindParse means the response body could not be understood.
	KindParse
)

func (k ErrorKind) String() string {
	switch k {
	case KindNoResult:
		return "no-result"
	case KindNetwork:
		return "network"
	case KindAuth:
		return "auth"
	case KindRateLimited:
		return "rate-limited"
	case KindServer:
		return "server"
	case KindClient:
		return "client"
	case KindParse:
		return "parse"
	default:
		return "unknown"
	}
}

// ErrNoResult is the sentinel every source returns when it simply found nothing.
//
// Before this existed, "nothing matched" and "the API is down" were both plain
// errors, so the pipeline could not tell them apart: it logged both as failures
// and could never retry a transient outage. The distinction also makes the
// "source returned nothing" branch reachable at all.
var ErrNoResult = errors.New("no result")

// NoResult builds a ErrNoResult error naming the source and search term.
func NoResult(source, term string) error {
	return fmt.Errorf("%s: no result for %q: %w", source, term, ErrNoResult)
}

// IsNoResult reports whether err represents a normal "nothing matched" outcome.
func IsNoResult(err error) bool {
	return errors.Is(err, ErrNoResult)
}

// APIError describes a request that failed in a way the user may be able to act
// on (missing API key, rate limit, service outage).
type APIError struct {
	Source  string
	Status  int
	Kind    ErrorKind
	URL     string
	Message string
	Err     error
}

func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString(e.Source)
	b.WriteString(": ")
	if e.Status > 0 {
		fmt.Fprintf(&b, "HTTP %d ", e.Status)
	}
	b.WriteString(e.Kind.String())
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *APIError) Unwrap() error { return e.Err }

// IsRetryable reports whether retrying the request could plausibly succeed.
func (e *APIError) IsRetryable() bool {
	return e.Kind == KindRateLimited || e.Kind == KindServer || e.Kind == KindNetwork
}

// KindOf extracts the ErrorKind from an error, defaulting to KindNetwork.
func KindOf(err error) ErrorKind {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Kind
	}
	if IsNoResult(err) {
		return KindNoResult
	}
	return KindNetwork
}

// classifyStatus maps an HTTP status to an ErrorKind.
func classifyStatus(status int) ErrorKind {
	switch {
	case status == http.StatusTooManyRequests:
		return KindRateLimited
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return KindAuth
	case status >= 500:
		return KindServer
	case status >= 400:
		return KindClient
	default:
		return KindClient
	}
}

// DefaultMaxBytes bounds how much of a response body is read. Every source sends
// a limit; this is the fallback.
const DefaultMaxBytes int64 = 2 << 20

// RetryPolicy controls automatic retries for transient failures.
type RetryPolicy struct {
	Attempts     int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	MaxRetryWait time.Duration
}

// DefaultRetryPolicy retries twice with exponential backoff.
var DefaultRetryPolicy = RetryPolicy{
	Attempts:     3,
	BaseDelay:    400 * time.Millisecond,
	MaxDelay:     5 * time.Second,
	MaxRetryWait: 20 * time.Second,
}

// HTTPClient is the single HTTP client used by every scraper and by cover
// downloads.
//
// Sharing one client keeps a single connection pool (previously there were seven,
// one per source plus http.DefaultClient), applies one coherent timeout, and
// gives one place to add retries and per-host rate limiting.
type HTTPClient struct {
	client *http.Client
	retry  RetryPolicy
	userUA string

	mu       sync.Mutex
	limiters map[string]*hostLimiter
}

// NewHTTPClient creates a client with the given per-request timeout.
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          16,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: timeout,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		retry:    DefaultRetryPolicy,
		userUA:   "GameLibrary/" + ClientVersion,
		limiters: map[string]*hostLimiter{},
	}
}

// ClientVersion is sent as part of the default User-Agent. It is a variable so
// the build can stamp the real version in.
var ClientVersion = "0.8.0"

// SetRetryPolicy replaces the retry policy.
func (c *HTTPClient) SetRetryPolicy(p RetryPolicy) { c.retry = p }

// Underlying exposes the *http.Client, mainly so tests can inject a transport.
func (c *HTTPClient) Underlying() *http.Client { return c.client }

// SetTransport replaces the transport (used by tests).
func (c *HTTPClient) SetTransport(rt http.RoundTripper) { c.client.Transport = rt }

// LimitHost applies a minimum interval between requests to one host. This keeps
// well inside the documented budgets (VNDB allows 200 requests per 5 minutes)
// without any global coordination.
func (c *HTTPClient) LimitHost(host string, minInterval time.Duration) {
	if minInterval <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiters == nil {
		c.limiters = map[string]*hostLimiter{}
	}
	c.limiters[strings.ToLower(host)] = &hostLimiter{minInterval: minInterval}
}

func (c *HTTPClient) limiterFor(host string) *hostLimiter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limiters[strings.ToLower(host)]
}

// Request describes one HTTP call.
type Request struct {
	Method   string
	URL      string
	Headers  map[string]string
	Body     []byte
	MaxBytes int64
}

// Response is a fully read response body plus its status.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	URL    string
}

// Do performs a request, retrying transient failures and waiting out rate
// limits. It never returns a response with a non-2xx status as success; callers
// get an *APIError instead, which removes the class of bug where an error page
// was silently parsed as an empty result.
func (c *HTTPClient) Do(ctx context.Context, source string, req Request) (*Response, error) {
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.MaxBytes <= 0 {
		req.MaxBytes = DefaultMaxBytes
	}

	attempts := c.retry.Attempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := c.backoffDelay(attempt, lastErr)
			if delay <= 0 {
				break
			}
			if err := sleepContext(ctx, delay); err != nil {
				return nil, err
			}
		}

		if limiter := c.limiterFor(hostOf(req.URL)); limiter != nil {
			if err := limiter.wait(ctx); err != nil {
				return nil, err
			}
		}

		resp, err := c.attempt(ctx, source, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.IsRetryable() {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = &APIError{Source: source, Kind: KindNetwork, URL: req.URL, Message: "request failed"}
	}
	return nil, lastErr
}

func (c *HTTPClient) attempt(ctx context.Context, source string, req Request) (*Response, error) {
	var body io.Reader
	if len(req.Body) > 0 {
		body = strings.NewReader(string(req.Body))
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return nil, &APIError{Source: source, Kind: KindClient, URL: req.URL, Message: "invalid request", Err: err}
	}
	httpReq.Header.Set("User-Agent", c.userUA)
	httpReq.Header.Set("Accept", "application/json")
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &APIError{Source: source, Kind: KindNetwork, URL: req.URL, Message: "request failed", Err: err}
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(io.LimitReader(resp.Body, req.MaxBytes))
	if readErr != nil {
		return nil, &APIError{Source: source, Status: resp.StatusCode, Kind: KindNetwork, URL: req.URL, Message: "reading response", Err: readErr}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Retry-After is carried on the message so the backoff calculation can
		// honour it without threading response headers through every layer.
		message := describeBody(data)
		if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
			message = retryAfterSecondsKey + retryAfter + " " + message
		}
		return nil, &APIError{
			Source:  source,
			Status:  resp.StatusCode,
			Kind:    classifyStatus(resp.StatusCode),
			URL:     req.URL,
			Message: message,
		}
	}

	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: data, URL: req.URL}, nil
}

// backoffDelay computes the wait before the next attempt, honouring a
// Retry-After hint when the source supplied one.
func (c *HTTPClient) backoffDelay(attempt int, lastErr error) time.Duration {
	var apiErr *APIError
	if errors.As(lastErr, &apiErr) {
		if apiErr.Kind == KindRateLimited {
			if wait, ok := retryAfterFromError(apiErr); ok {
				if wait > c.retry.MaxRetryWait {
					// The source wants a long pause; do not block a queue worker
					// on it, just fail this game and move on.
					return 0
				}
				return wait
			}
		}
		if !apiErr.IsRetryable() {
			return 0
		}
	}

	delay := c.retry.BaseDelay << (attempt - 1)
	if delay > c.retry.MaxDelay {
		delay = c.retry.MaxDelay
	}
	// Jitter avoids several workers retrying in lockstep.
	jitter := time.Duration(rand.Int63n(int64(delay/2 + 1)))
	return delay + jitter
}

// retryAfterSecondsKey is stashed on the APIError message so the delay can be
// derived without threading the header through every layer.
const retryAfterSecondsKey = "retry-after:"

func retryAfterFromError(apiErr *APIError) (time.Duration, bool) {
	idx := strings.Index(apiErr.Message, retryAfterSecondsKey)
	if idx < 0 {
		return 0, false
	}
	raw := strings.TrimSpace(apiErr.Message[idx+len(retryAfterSecondsKey):])
	if space := strings.IndexAny(raw, " \t"); space >= 0 {
		raw = raw[:space]
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// describeBody produces a short, log-safe hint about an error response.
func describeBody(data []byte) string {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	return Truncate(text, 200)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func hostOf(rawURL string) string {
	idx := strings.Index(rawURL, "://")
	if idx < 0 {
		return rawURL
	}
	rest := rawURL[idx+3:]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		rest = rest[:slash]
	}
	return rest
}

// hostLimiter enforces a minimum interval between requests to one host.
type hostLimiter struct {
	mu          sync.Mutex
	minInterval time.Duration
	next        time.Time
}

func (l *hostLimiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	if l.next.After(now) {
		wait := l.next.Sub(now)
		l.next = l.next.Add(l.minInterval)
		l.mu.Unlock()
		return sleepContext(ctx, wait)
	}
	l.next = now.Add(l.minInterval)
	l.mu.Unlock()
	return nil
}
