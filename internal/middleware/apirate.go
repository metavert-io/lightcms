package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

func init() {
	// Prune stale entries from all rate-limiter token maps every 5 minutes.
	// Without pruning, the maps grow unbounded for every unique API key ever seen.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			pruneAPIKeyRateLimiter()
			regenerateLimiter.prune()
			searchReplaceExecuteLimiter.prune()
			assetFromURLLimiter.prune()
			bulkUpdateLimiter.prune()
			exportLimiter.prune()
			reindexLimiter.prune()
			commentCreateLimiter.prune()
		}
	}()
}

func pruneAPIKeyRateLimiter() {
	cutoff := time.Now().Add(-apiRateWindow)
	apiKeyRateLimiter.Lock()
	for token, times := range apiKeyRateLimiter.tokens {
		// Prune old entries from the slice
		start := 0
		for start < len(times) && times[start].Before(cutoff) {
			start++
		}
		if start == len(times) {
			// All entries expired — remove the key entirely
			delete(apiKeyRateLimiter.tokens, token)
		} else {
			apiKeyRateLimiter.tokens[token] = times[start:]
		}
	}
	apiKeyRateLimiter.Unlock()
}

const (
	// apiRateWindow is the sliding window for per-key rate limiting.
	apiRateWindow = time.Minute
	// apiRateLimit is the maximum number of API v1 requests per key per minute.
	// This is intentionally generous — it targets runaway scripts, not normal usage.
	apiRateLimit = 300

	// apiBodySizeLimit is the maximum allowed request body size for JSON API endpoints.
	// Prevents memory exhaustion via oversized payloads.
	apiBodySizeLimit = 10 << 20 // 10 MiB
)

// apiKeyRateLimiter tracks per-token request timestamps.
var apiKeyRateLimiter = struct {
	sync.Mutex
	tokens map[string][]time.Time
}{
	tokens: make(map[string][]time.Time),
}

// rateLimitKey returns what a request is counted against: the credential the
// caller signed in with, or failing that the raw bearer token. A browser session
// has neither and is not limited here.
func rateLimitKey(r *http.Request) string {
	if key := CredentialKeyFromContext(r.Context()); key != "" {
		return key
	}
	if auth := r.Header.Get("Authorization"); len(auth) > 7 {
		return auth[7:] // strip "Bearer "
	}
	return ""
}

// APIRateLimit is middleware that enforces a per-credential sliding-window rate limit
// on the /api/v1/ subrouter. It runs after authentication so the credential is already
// validated; unauthenticated requests are rejected by the auth middleware first.
func APIRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := rateLimitKey(r)
		if token == "" {
			// No token — auth middleware would have blocked this, but be safe.
			next.ServeHTTP(w, r)
			return
		}

		apiKeyRateLimiter.Lock()
		now := time.Now()
		cutoff := now.Add(-apiRateWindow)

		times := apiKeyRateLimiter.tokens[token]
		// Prune entries outside the window
		start := 0
		for start < len(times) && times[start].Before(cutoff) {
			start++
		}
		times = times[start:]

		limited := len(times) >= apiRateLimit
		if !limited {
			apiKeyRateLimiter.tokens[token] = append(times, now)
		} else {
			apiKeyRateLimiter.tokens[token] = times
		}
		apiKeyRateLimiter.Unlock()

		if limited {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "API rate limit exceeded (300 requests/minute). Please slow down.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// BurstRateLimit enforces a per-token per-second rate limit (20 req/s default).
// This prevents clients from overwhelming the server with rapid-fire requests,
// complementing the per-minute APIRateLimit which allows bursty traffic.
var burstLimiter = struct {
	sync.Mutex
	tokens map[string][]time.Time
}{
	tokens: make(map[string][]time.Time),
}

const burstRateLimit = 20
const burstWindow = time.Second

func init() {
	// Prune burst limiter entries every 30 seconds (lightweight — 1-second entries expire fast)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-burstWindow * 2)
			burstLimiter.Lock()
			for token, times := range burstLimiter.tokens {
				start := 0
				for start < len(times) && times[start].Before(cutoff) {
					start++
				}
				if start == len(times) {
					delete(burstLimiter.tokens, token)
				} else {
					burstLimiter.tokens[token] = times[start:]
				}
			}
			burstLimiter.Unlock()
		}
	}()
}

// APIBurstRateLimit is middleware that enforces 20 requests/second per bearer token.
func APIBurstRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := rateLimitKey(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		burstLimiter.Lock()
		now := time.Now()
		cutoff := now.Add(-burstWindow)
		times := burstLimiter.tokens[token]
		start := 0
		for start < len(times) && times[start].Before(cutoff) {
			start++
		}
		times = times[start:]
		limited := len(times) >= burstRateLimit
		if !limited {
			burstLimiter.tokens[token] = append(times, now)
		} else {
			burstLimiter.tokens[token] = times
		}
		burstLimiter.Unlock()

		if limited {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Rate limit exceeded (20 requests/second). Please slow down.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// APIBodySizeLimit is middleware that caps JSON request body size to prevent
// memory exhaustion from oversized payloads. Applied globally to /api/v1/.
func APIBodySizeLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, apiBodySizeLimit)
		}
		next.ServeHTTP(w, r)
	})
}

// endpointRateLimiter tracks per-token timestamps for a specific endpoint.
type endpointRateLimiter struct {
	sync.Mutex
	tokens map[string][]time.Time
	limit  int
}

func newEndpointRateLimiter(limit int) *endpointRateLimiter {
	return &endpointRateLimiter{tokens: make(map[string][]time.Time), limit: limit}
}

func (e *endpointRateLimiter) check(token string) bool {
	e.Lock()
	defer e.Unlock()
	now := time.Now()
	cutoff := now.Add(-apiRateWindow)
	times := e.tokens[token]
	start := 0
	for start < len(times) && times[start].Before(cutoff) {
		start++
	}
	times = times[start:]
	if len(times) >= e.limit {
		e.tokens[token] = times
		return true // rate limited
	}
	e.tokens[token] = append(times, now)
	return false
}

func (e *endpointRateLimiter) prune() {
	cutoff := time.Now().Add(-apiRateWindow)
	e.Lock()
	for token, times := range e.tokens {
		start := 0
		for start < len(times) && times[start].Before(cutoff) {
			start++
		}
		if start == len(times) {
			delete(e.tokens, token)
		} else {
			e.tokens[token] = times[start:]
		}
	}
	e.Unlock()
}

// Per-endpoint rate limiters for expensive operations.
var (
	regenerateLimiter           = newEndpointRateLimiter(2)  // 2 full-regen per minute
	searchReplaceExecuteLimiter = newEndpointRateLimiter(10) // 10 search-replace executes per minute
	assetFromURLLimiter         = newEndpointRateLimiter(10) // 10 remote asset fetches per minute
	bulkUpdateLimiter           = newEndpointRateLimiter(5)  // 5 bulk-update calls per minute
	exportLimiter               = newEndpointRateLimiter(5)  // 5 exports per minute
	reindexLimiter              = newEndpointRateLimiter(1)  // 1 full reindex per minute
	commentCreateLimiter        = newEndpointRateLimiter(20) // 20 comments per minute per token
	oauthRegisterLimiter        = newEndpointRateLimiter(10) // 10 client registrations per minute per address
)

// EndpointRateLimit returns middleware that enforces a per-token limit for a specific
// expensive endpoint. Call with the appropriate pre-built limiter.
func EndpointRateLimit(limiter *endpointRateLimiter, description string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := rateLimitKey(r)
			if token != "" && limiter.check(token) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "rate limit exceeded for " + description + ". Please slow down.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RegenerateLimiter returns middleware limiting content regeneration to 2/min per token.
func RegenerateLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(regenerateLimiter, "regenerate (2/min)")
}

// SearchReplaceExecuteLimiter returns middleware limiting search-replace executions to 10/min per token.
func SearchReplaceExecuteLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(searchReplaceExecuteLimiter, "search-replace execute (10/min)")
}

// AssetFromURLLimiter returns middleware limiting remote asset fetches to 10/min per token.
func AssetFromURLLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(assetFromURLLimiter, "asset-from-url (10/min)")
}

// BulkUpdateLimiter returns middleware limiting bulk-update calls to 5/min per token.
func BulkUpdateLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(bulkUpdateLimiter, "bulk-update (5/min)")
}

// ExportLimiter returns middleware limiting content export to 5/min per token.
func ExportLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(exportLimiter, "export (5/min)")
}

// ReindexLimiter returns middleware limiting embedding reindex to 1/min per token.
func ReindexLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(reindexLimiter, "reindex-embeddings (1/min)")
}

// OAuthRegisterLimiter returns middleware limiting OAuth client registration to
// 10/min per client address. Registration is open to anyone, so it has no
// credential to count against.
func OAuthRegisterLimiter(proxy *TrustedProxyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if oauthRegisterLimiter.check(GetClientIP(r, proxy)) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error":             "too_many_requests",
					"error_description": "Too many client registrations. Try again in a minute.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CommentCreateLimiter returns middleware limiting comment creation to 20/min per token.
func CommentCreateLimiter() func(http.Handler) http.Handler {
	return EndpointRateLimit(commentCreateLimiter, "comment creation (20/min)")
}
