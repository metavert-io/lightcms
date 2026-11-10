package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIAuth_MissingAuthorizationHeader(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, nil
	})
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] == nil {
		t.Error("expected error in response body")
	}
}

func TestAPIAuth_MalformedAuthorizationHeader(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, nil
	})
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name   string
		header string
	}{
		{"no space", "BearerToken123"},
		{"wrong scheme", "Basic dXNlcjpwYXNz"},
		{"empty token", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			req.Header.Set("Authorization", tt.header)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", rr.Code)
			}
		})
	}
}

func TestAPIAuth_ValidAPIKey(t *testing.T) {
	called := false
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		if rawKey != "lc_testkey123" {
			return nil, fmt.Errorf("invalid key")
		}
		called = true
		return nil, nil
	})
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer lc_testkey123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !called {
		t.Error("expected validate function to be called")
	}
}

func TestAPIAuth_InvalidAPIKey(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("invalid key")
	})
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer lc_badkey")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIAuth_OAuthTokenAcceptedAtMCPWithCallerAndHeaderUntouched(t *testing.T) {
	type person struct{ Email string }
	caller := &person{Email: "viewer@example.com"}
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("not an API key")
	})
	m.SetOAuth(func(ctx context.Context, rawToken string) (interface{}, error) {
		if rawToken != "oauth_access_token_123" {
			return nil, fmt.Errorf("invalid token")
		}
		return caller, nil
	}, "https://example.com/.well-known/oauth-protected-resource")

	var capturedAuth string
	var capturedUser interface{}
	handler := m.MCPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedUser, _ = APIUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer oauth_access_token_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedUser != caller {
		t.Errorf("expected the signed-in person in context, got %+v", capturedUser)
	}
	if capturedAuth != "Bearer oauth_access_token_123" {
		t.Errorf("Authorization header was rewritten to %q", capturedAuth)
	}
}

func TestAPIAuth_OAuthTokenRefusedAtREST(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("not an API key")
	})
	validated := false
	m.SetOAuth(func(ctx context.Context, rawToken string) (interface{}, error) {
		validated = true
		return "someone", nil
	}, "https://example.com/.well-known/oauth-protected-resource")

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run for an OAuth token at /api/v1")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer oauth_access_token_123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if validated {
		t.Error("the OAuth validator must not run at /api/v1")
	}
}

func TestAPIAuth_InvalidOAuthTokenAtMCP(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("not an API key")
	})
	m.SetOAuth(func(ctx context.Context, rawToken string) (interface{}, error) {
		return nil, fmt.Errorf("invalid token")
	}, "https://example.com/.well-known/oauth-protected-resource")

	handler := m.MCPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run for an invalid token")
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer bad_oauth_token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIAuth_MCPRefusesBrowserSession(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("invalid")
	})
	m.SetSessionAuth(func(r *http.Request) interface{} { return "cookie user" }, nil)

	handler := m.MCPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not run for a browser session at /mcp")
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIAuth_InternalCallerIsOnlyReachableThroughContext(t *testing.T) {
	type person struct{ Email string }
	caller := &person{Email: "agent@example.com"}
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("invalid")
	})

	var seen interface{}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = APIUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil).WithContext(WithInternalCaller(context.Background(), caller))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || seen != caller {
		t.Fatalf("in-process call: code %d, caller %+v", rr.Code, seen)
	}

	// The same request without the context value has no caller at all.
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("request from the network: expected 401, got %d", rr.Code)
	}
}

type credentialCaller struct{ key string }

func (c credentialCaller) CredentialKey() string { return c.key }

func TestRateLimitKey_CountsAgainstTheCredential(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer lc_raw")
	if got := rateLimitKey(req); got != "lc_raw" {
		t.Errorf("without a caller: got %q", got)
	}

	withCaller := req.WithContext(context.WithValue(req.Context(), apiUserContextKey, credentialCaller{key: "oauth:abc"}))
	if got := rateLimitKey(withCaller); got != "oauth:abc" {
		t.Errorf("with a caller: got %q", got)
	}

	// An in-process call has no Authorization header but is still counted.
	internal := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	internal = internal.WithContext(context.WithValue(internal.Context(), apiUserContextKey, credentialCaller{key: "oauth:abc"}))
	if got := rateLimitKey(internal); got != "oauth:abc" {
		t.Errorf("in-process: got %q", got)
	}
}

func TestAPIAuth_OAuthNotConfigured_NonLCToken(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("invalid key")
	})
	// OAuth NOT configured

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer some_random_token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIAuth_WWWAuthenticateHeader(t *testing.T) {
	resourceURL := "https://example.com/.well-known/oauth-protected-resource"
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("invalid")
	})
	m.SetOAuth(func(ctx context.Context, rawToken string) (interface{}, error) {
		return nil, fmt.Errorf("invalid")
	}, resourceURL)

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if wwwAuth == "" {
		t.Error("expected WWW-Authenticate header to be set")
	}
	expected := `Bearer resource_metadata="` + resourceURL + `"`
	if wwwAuth != expected {
		t.Errorf("expected WWW-Authenticate %q, got %q", expected, wwwAuth)
	}
}

func TestAPIAuth_NoWWWAuthenticateWithoutResourceMetadata(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, fmt.Errorf("invalid")
	})
	// No OAuth configured, no resource metadata URL

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if wwwAuth != "" {
		t.Errorf("expected no WWW-Authenticate header, got %q", wwwAuth)
	}
}

func TestAPIAuth_CaseInsensitiveBearer(t *testing.T) {
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return nil, nil
	})
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "bearer lc_testkey123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for lowercase bearer, got %d", rr.Code)
	}
}

func TestAPIUserFromContext_Empty(t *testing.T) {
	user, ok := APIUserFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for empty context")
	}
	if user != nil {
		t.Error("expected nil user for empty context")
	}
}

func TestAPIUserFromContext_WithUser(t *testing.T) {
	type fakeUser struct{ Email string }

	// Store a user via the middleware path
	stored := &fakeUser{Email: "api@example.com"}
	m := NewAPIAuth(func(ctx context.Context, rawKey string) (interface{}, error) {
		return stored, nil
	})

	var ctxUser interface{}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxUser, _ = APIUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer lc_somekey")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if ctxUser == nil {
		t.Fatal("expected user in context")
	}
	fu, ok := ctxUser.(*fakeUser)
	if !ok || fu.Email != "api@example.com" {
		t.Errorf("unexpected user: %+v", ctxUser)
	}
}

func TestAPIRateLimit_PassThrough(t *testing.T) {
	called := false
	handler := APIRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer lc_ratelimitpasstest")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAPIRateLimit_NoToken(t *testing.T) {
	// No auth header: should pass through (auth middleware handles rejection)
	called := false
	handler := APIRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called with no auth header")
	}
}

func TestAPIRateLimit_Exceeded(t *testing.T) {
	// Use a unique token so we don't pollute other tests
	token := "lc_ratelimit_exceeded_testonly_unique"

	handler := APIRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Manually pre-fill the rate limiter to the limit
	apiKeyRateLimiter.Lock()
	now := time.Now()
	times := make([]time.Time, apiRateLimit)
	for i := range times {
		times[i] = now
	}
	apiKeyRateLimiter.tokens[token] = times
	apiKeyRateLimiter.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("expected Retry-After: 60, got %q", rr.Header().Get("Retry-After"))
	}
}
