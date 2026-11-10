package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// contextKey is an unexported type for context keys in this package
type contextKey int

const (
	// apiUserContextKey stores the authenticated API user in context
	apiUserContextKey contextKey = iota
	// internalCallerContextKey marks a same-process call to the REST API.
	internalCallerContextKey
)

// CredentialBound is implemented by a caller that signed in with an API key or
// OAuth connection. CredentialKey identifies that credential, stably and
// without revealing it, for rate limiting and MCP session binding.
type CredentialBound interface {
	CredentialKey() string
}

// CredentialKeyFromContext returns the credential behind the authenticated
// caller in ctx, or "" for a browser session or no caller.
func CredentialKeyFromContext(ctx context.Context) string {
	if bound, ok := ctx.Value(apiUserContextKey).(CredentialBound); ok {
		return bound.CredentialKey()
	}
	return ""
}

// WithInternalCaller returns a context for calling the REST API from inside
// this process on behalf of caller, who has already been authenticated.
// Context values cannot arrive over the network, so a remote client cannot
// claim this; only the MCP endpoint sets it.
func WithInternalCaller(ctx context.Context, caller interface{}) context.Context {
	return context.WithValue(ctx, internalCallerContextKey, caller)
}

// APIKeyValidateFunc validates an API key and returns the authenticated user (as interface{}).
// The returned value will be stored in the request context and can be retrieved with APIUserFromContext.
type APIKeyValidateFunc func(ctx context.Context, rawKey string) (interface{}, error)

// OAuthValidateFunc validates an OAuth access token and returns the authenticated user (as interface{}).
type OAuthValidateFunc func(ctx context.Context, rawToken string) (interface{}, error)

// APIUserFromContext extracts the authenticated API user from the request context.
// Callers should type-assert the result to the expected user type.
func APIUserFromContext(ctx context.Context) (interface{}, bool) {
	user := ctx.Value(apiUserContextKey)
	return user, user != nil
}

// InjectAPIUser returns a context with the given user injected as the API user.
// Intended for use in tests only.
func InjectAPIUser(ctx context.Context, user interface{}) context.Context {
	return context.WithValue(ctx, apiUserContextKey, user)
}

// SessionValidateFunc validates a session cookie and returns the authenticated user (as interface{}).
// It receives the full HTTP request so it can read cookies.
type SessionValidateFunc func(r *http.Request) interface{}

// APIAuth is middleware that authenticates requests via API key or OAuth token
type APIAuth struct {
	validate            APIKeyValidateFunc
	validateOAuth       OAuthValidateFunc
	validateSession     SessionValidateFunc
	sessionCSRF         func(http.Handler) http.Handler // CSRF check for unsafe session-authenticated requests
	resourceMetadataURL string                          // for WWW-Authenticate header (OAuth discovery)
}

// NewAPIAuth creates a new API auth middleware
func NewAPIAuth(validate APIKeyValidateFunc) *APIAuth {
	return &APIAuth{validate: validate}
}

// SetSessionAuth enables session cookie fallback for browser-based admin UI calls.
// When no Authorization header is present, the session validator is tried as a fallback.
//
// A session cookie is sent by the browser on its own, so a session-authenticated
// request with an unsafe method (anything but GET, HEAD, OPTIONS, TRACE) must
// also pass csrfProtect — middleware.CSRFProtectAPI in the server. With a nil
// csrfProtect those requests are refused outright. Requests authenticated by
// an API key or OAuth bearer token never reach the check: a browser does not
// attach an Authorization header by itself.
func (m *APIAuth) SetSessionAuth(validateSession SessionValidateFunc, csrfProtect func(http.Handler) http.Handler) {
	m.validateSession = validateSession
	m.sessionCSRF = csrfProtect
}

// isSafeMethod reports whether the method is read-only under RFC 7231 (the
// set gorilla/csrf exempts).
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	}
	return false
}

// SetOAuth enables OAuth tokens on MCPMiddleware. The validator returns the
// signed-in person the token acts for.
func (m *APIAuth) SetOAuth(validateOAuth OAuthValidateFunc, resourceMetadataURL string) {
	m.validateOAuth = validateOAuth
	m.resourceMetadataURL = resourceMetadataURL
}

// Middleware authenticates a REST API request: an API key, a browser session,
// or a call from the MCP endpoint in this process. OAuth tokens are issued for
// the MCP endpoint alone and are refused here.
func (m *APIAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if caller := r.Context().Value(internalCallerContextKey); caller != nil {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), apiUserContextKey, caller)))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// No Authorization header — try session cookie fallback for browser-based admin UI calls
			if m.validateSession != nil {
				if user := m.validateSession(r); user != nil {
					r = r.WithContext(context.WithValue(r.Context(), apiUserContextKey, user))
					if isSafeMethod(r.Method) {
						next.ServeHTTP(w, r)
						return
					}
					if m.sessionCSRF == nil {
						apiJsonError(w, http.StatusForbidden, "Session-authenticated API requests that change state are not enabled")
						return
					}
					m.sessionCSRF(next).ServeHTTP(w, r)
					return
				}
			}
			m.setWWWAuthenticate(w)
			apiJsonError(w, http.StatusUnauthorized, "Missing Authorization header")
			return
		}

		token, ok := m.bearerToken(w, authHeader)
		if !ok {
			return
		}

		if !strings.HasPrefix(token, "lc_") {
			m.setWWWAuthenticate(w)
			apiJsonError(w, http.StatusUnauthorized, "Invalid API key (OAuth access tokens are accepted only at the MCP endpoint)")
			return
		}
		m.authenticate(w, r, next, m.validate, token, "Invalid API key")
	})
}

// MCPMiddleware authenticates a request to the MCP endpoint: an API key or an
// OAuth token. A browser session is not accepted, because a page must not be
// able to drive an agent session with a visitor's cookie.
func (m *APIAuth) MCPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			m.setWWWAuthenticate(w)
			apiJsonError(w, http.StatusUnauthorized, "Missing Authorization header")
			return
		}
		token, ok := m.bearerToken(w, authHeader)
		if !ok {
			return
		}

		switch {
		case strings.HasPrefix(token, "lc_"):
			m.authenticate(w, r, next, m.validate, token, "Invalid API key")
		case m.validateOAuth != nil:
			m.authenticate(w, r, next, APIKeyValidateFunc(m.validateOAuth), token, "Invalid access token")
		default:
			m.setWWWAuthenticate(w)
			apiJsonError(w, http.StatusUnauthorized, "Invalid API key")
		}
	})
}

// bearerToken extracts the token from "Bearer <token>", writing the 401 itself
// when the header has another form.
func (m *APIAuth) bearerToken(w http.ResponseWriter, authHeader string) (string, bool) {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		m.setWWWAuthenticate(w)
		apiJsonError(w, http.StatusUnauthorized, "Invalid Authorization header format (expected: Bearer <token>)")
		return "", false
	}
	return parts[1], true
}

// authenticate runs validate and, when it succeeds, calls next with the caller
// in the request context.
func (m *APIAuth) authenticate(w http.ResponseWriter, r *http.Request, next http.Handler, validate APIKeyValidateFunc, token, invalidMessage string) {
	user, err := validate(r.Context(), token)
	if err != nil {
		m.setWWWAuthenticate(w)
		apiJsonError(w, http.StatusUnauthorized, invalidMessage)
		return
	}
	if user != nil {
		r = r.WithContext(context.WithValue(r.Context(), apiUserContextKey, user))
	}
	next.ServeHTTP(w, r)
}

// setWWWAuthenticate adds the WWW-Authenticate header for OAuth discovery
func (m *APIAuth) setWWWAuthenticate(w http.ResponseWriter) {
	if m.resourceMetadataURL != "" {
		w.Header().Set("WWW-Authenticate",
			`Bearer resource_metadata="`+m.resourceMetadataURL+`"`)
	}
}

func apiJsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}
