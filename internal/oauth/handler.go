package oauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	loginProofMaxAge = 5 * time.Minute

	// Limits on what an unauthenticated client may register.
	maxRegistrationBody = 16 << 10
	maxClientNameLength = 100
	maxRedirectURIs     = 10
)

// Handler implements OAuth 2.1 endpoints for MCP authorization
type Handler struct {
	oauthService  *services.OAuthService
	authManager   *auth.Manager
	baseURL       string
	resource      string // the one resource tokens are issued for: this server's MCP endpoint
	sessionSecret string
	tmpl          *template.Template
}

// NewHandler creates a new OAuth handler
func NewHandler(oauthService *services.OAuthService, authManager *auth.Manager, baseURL, sessionSecret string) *Handler {
	h := &Handler{
		oauthService:  oauthService,
		authManager:   authManager,
		baseURL:       baseURL,
		resource:      services.MCPResourceURL(baseURL),
		sessionSecret: sessionSecret,
	}
	h.tmpl = template.Must(template.New("authorize").Parse(oauthTemplates["authorize"]))
	return h
}

// Register handles Dynamic Client Registration (RFC 7591)
// POST /oauth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRegistrationBody)
	var req struct {
		ClientName    string   `json:"client_name"`
		RedirectURIs  []string `json:"redirect_uris"`
		GrantTypes    []string `json:"grant_types"`
		ResponseTypes []string `json:"response_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	req.ClientName = cleanClientName(req.ClientName)
	if req.ClientName == "" {
		req.ClientName = "Unknown Client"
	}
	if len(req.RedirectURIs) == 0 {
		jsonError(w, http.StatusBadRequest, "invalid_request", "redirect_uris is required")
		return
	}
	if len(req.RedirectURIs) > maxRedirectURIs {
		jsonError(w, http.StatusBadRequest, "invalid_redirect_uri", "Too many redirect_uris")
		return
	}
	for _, uri := range req.RedirectURIs {
		if err := services.ValidateRedirectURI(uri); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
			return
		}
	}

	client, rawSecret, err := h.oauthService.RegisterClient(r.Context(), req.ClientName, req.RedirectURIs)
	if err != nil {
		log.Printf("[OAuth] Registration failed: %v", err)
		jsonError(w, http.StatusInternalServerError, "server_error", "Failed to register client")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":                  client.ClientID,
		"client_secret":              rawSecret,
		"client_id_issued_at":        client.CreatedAt.Unix(),
		"client_secret_expires_at":   0,
		"client_name":                client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "client_secret_post",
	})
}

// Authorize handles the authorization page
// GET /oauth/authorize
func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	resource := q.Get("resource")
	scope := q.Get("scope")

	// Validate required params
	if responseType != "code" {
		jsonError(w, http.StatusBadRequest, "unsupported_response_type", "Only response_type=code is supported")
		return
	}
	if codeChallenge == "" || codeChallengeMethod != "S256" {
		jsonError(w, http.StatusBadRequest, "invalid_request", "PKCE with S256 is required")
		return
	}

	// Validate client
	_, err := h.oauthService.ValidateClient(r.Context(), clientID, redirectURI)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_client", err.Error())
		return
	}

	resource, ok := h.resolveResource(resource)
	if !ok {
		redirectWithError(w, r, redirectURI, state, "invalid_target", "This server only issues tokens for "+h.resource)
		return
	}

	// Render login page
	h.renderAuthorize(w, map[string]interface{}{
		"ShowLogin":           true,
		"Error":               "",
		"ClientID":            clientID,
		"RedirectURI":         redirectURI,
		"ResponseType":        responseType,
		"State":               state,
		"CodeChallenge":       codeChallenge,
		"CodeChallengeMethod": codeChallengeMethod,
		"Resource":            resource,
		"Scope":               scope,
	})
}

// AuthorizeSubmit processes the login form or consent decision
// POST /oauth/authorize
func (h *Handler) AuthorizeSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid form data")
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	responseType := r.FormValue("response_type")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	resource := r.FormValue("resource")
	scope := r.FormValue("scope")
	action := r.FormValue("action")

	// Validate client
	client, err := h.oauthService.ValidateClient(r.Context(), clientID, redirectURI)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_client", err.Error())
		return
	}

	if action == "login" || action == "approve" {
		if responseType != "code" || codeChallenge == "" || codeChallengeMethod != "S256" {
			jsonError(w, http.StatusBadRequest, "invalid_request", "response_type=code and PKCE with S256 are required")
			return
		}
		var ok bool
		if resource, ok = h.resolveResource(resource); !ok {
			redirectWithError(w, r, redirectURI, state, "invalid_target", "This server only issues tokens for "+h.resource)
			return
		}
	}

	templateData := map[string]interface{}{
		"ClientID":            clientID,
		"RedirectURI":         redirectURI,
		"ResponseType":        responseType,
		"State":               state,
		"CodeChallenge":       codeChallenge,
		"CodeChallengeMethod": codeChallengeMethod,
		"Resource":            resource,
		"Scope":               scope,
	}

	switch action {
	case "login":
		email := r.FormValue("email")
		password := r.FormValue("password")

		// Check rate limiting
		if locked, duration := h.authManager.CheckRateLimit(r.Context(), r); locked {
			templateData["ShowLogin"] = true
			templateData["Error"] = "Too many attempts. Try again in " + duration
			h.renderAuthorize(w, templateData)
			return
		}

		user, err := h.authManager.ValidateCredentials(r.Context(), email, password)
		if err != nil || user == nil {
			h.authManager.RecordFailedLogin(r.Context(), r)
			templateData["ShowLogin"] = true
			templateData["Error"] = "Invalid email or password"
			h.renderAuthorize(w, templateData)
			return
		}

		h.authManager.ClearRateLimit(r.Context(), r)

		if user.IsDefaultPassword {
			templateData["ShowLogin"] = true
			templateData["Error"] = "Set a new password in the admin dashboard before connecting an app."
			h.renderAuthorize(w, templateData)
			return
		}

		// Password valid — show consent page with login proof
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		userID := user.ID.Hex()

		templateData["ShowLogin"] = false
		templateData["ClientName"] = client.ClientName
		templateData["RedirectHost"] = redirectHost(redirectURI)
		templateData["UserEmail"] = user.Email
		templateData["UserRole"] = user.Role
		templateData["RoleSummary"] = roleSummary(user.Role)
		templateData["LoginUser"] = userID
		templateData["LoginProof"] = h.computeLoginProof(userID, clientID, redirectURI, codeChallenge, resource, ts)
		templateData["LoginTS"] = ts
		h.renderAuthorize(w, templateData)

	case "approve":
		loginUser := r.FormValue("login_user")
		loginProof := r.FormValue("login_proof")
		loginTS := r.FormValue("login_ts")

		if !h.verifyLoginProof(loginUser, clientID, redirectURI, codeChallenge, resource, loginTS, loginProof) {
			templateData["ShowLogin"] = true
			templateData["Error"] = "Session expired. Please sign in again."
			h.renderAuthorize(w, templateData)
			return
		}

		// The account may have changed since the password was checked.
		user := h.approvingUser(r, loginUser)
		if user == nil {
			templateData["ShowLogin"] = true
			templateData["Error"] = "This account can't connect apps. Please sign in again."
			h.renderAuthorize(w, templateData)
			return
		}

		grant, err := h.oauthService.SaveGrant(r.Context(), client, user.ID, resource)
		if err != nil {
			log.Printf("[OAuth] Failed to save grant: %v", err)
			redirectWithError(w, r, redirectURI, state, "server_error", "Failed to record the connection")
			return
		}

		// Generate auth code
		code, err := h.oauthService.CreateAuthCode(r.Context(), grant, redirectURI, codeChallenge, codeChallengeMethod)
		if err != nil {
			log.Printf("[OAuth] Failed to create auth code: %v", err)
			redirectWithError(w, r, redirectURI, state, "server_error", "Failed to generate authorization code")
			return
		}

		// Redirect with code
		redirectURL, _ := url.Parse(redirectURI)
		q := redirectURL.Query()
		q.Set("code", code)
		if state != "" {
			q.Set("state", state)
		}
		redirectURL.RawQuery = q.Encode()
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)

	case "deny":
		redirectWithError(w, r, redirectURI, state, "access_denied", "User denied access")

	default:
		jsonError(w, http.StatusBadRequest, "invalid_request", "Unknown action")
	}
}

// Token handles token exchange and refresh
// POST /oauth/token
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid form data")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	grantType := r.FormValue("grant_type")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	// Validate client credentials
	if _, err := h.oauthService.ValidateClientCredentials(r.Context(), clientID, clientSecret); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_client",
			"error_description": "Invalid client credentials",
		})
		return
	}

	if resource := r.FormValue("resource"); resource != "" {
		if _, ok := h.resolveResource(resource); !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_target",
				"error_description": "This server only issues tokens for " + h.resource,
			})
			return
		}
	}

	switch grantType {
	case "authorization_code":
		code := r.FormValue("code")
		redirectURI := r.FormValue("redirect_uri")
		codeVerifier := r.FormValue("code_verifier")

		accessToken, refreshToken, expiresIn, err := h.oauthService.ExchangeAuthCode(r.Context(), code, clientID, redirectURI, codeVerifier)
		if err != nil {
			log.Printf("[OAuth] Token exchange failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_grant",
				"error_description": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  accessToken,
			"token_type":    "Bearer",
			"expires_in":    expiresIn,
			"refresh_token": refreshToken,
		})

	case "refresh_token":
		refreshToken := r.FormValue("refresh_token")

		accessToken, newRefreshToken, expiresIn, err := h.oauthService.RefreshAccessToken(r.Context(), refreshToken, clientID)
		if err != nil {
			log.Printf("[OAuth] Token refresh failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_grant",
				"error_description": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  accessToken,
			"token_type":    "Bearer",
			"expires_in":    expiresIn,
			"refresh_token": newRefreshToken,
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unsupported_grant_type",
			"error_description": "Only authorization_code and refresh_token are supported",
		})
	}
}

// computeLoginProof creates an HMAC proving that this person gave the right
// password for this authorization request. Each field is written with its
// length, so one set of values can't be read as another.
func (h *Handler) computeLoginProof(userID, clientID, redirectURI, codeChallenge, resource, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(h.sessionSecret))
	for _, field := range []string{"lightcms-oauth-consent", userID, clientID, redirectURI, codeChallenge, resource, timestamp} {
		fmt.Fprintf(mac, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyLoginProof checks the HMAC and ensures it's not expired
func (h *Handler) verifyLoginProof(userID, clientID, redirectURI, codeChallenge, resource, timestamp, proof string) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)) > loginProofMaxAge {
		return false
	}
	expected := h.computeLoginProof(userID, clientID, redirectURI, codeChallenge, resource, timestamp)
	return hmac.Equal([]byte(expected), []byte(proof))
}

// approvingUser returns the account behind a verified login proof, or nil if
// it can no longer connect an app.
func (h *Handler) approvingUser(r *http.Request, userID string) *models.User {
	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil
	}
	user, err := h.authManager.GetUserByID(r.Context(), id)
	if err != nil {
		log.Printf("[OAuth] Failed to load approving user: %v", err)
		return nil
	}
	if user == nil || user.Disabled || user.IsDefaultPassword {
		return nil
	}
	return user
}

// resolveResource returns the resource an authorization request is for. A
// client that names none gets this server's MCP endpoint; naming anything else
// is refused (RFC 8707).
func (h *Handler) resolveResource(requested string) (string, bool) {
	if requested == "" || sameResource(requested, h.resource) {
		return h.resource, true
	}
	return "", false
}

// sameResource compares two resource URLs, ignoring the differences that don't
// change what they point at: letter case in the scheme and host, a default
// port and a trailing slash.
func sameResource(a, b string) bool {
	na, okA := normalizeResource(a)
	nb, okB := normalizeResource(b)
	return okA && okB && na == nb
}

func normalizeResource(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && !(scheme == "https" && port == "443") && !(scheme == "http" && port == "80") {
		host += ":" + port
	}
	return scheme + "://" + host + strings.TrimSuffix(u.Path, "/"), true
}

// cleanClientName removes characters that could disguise a client's name on
// the consent page (control, zero-width and text-direction characters) and
// shortens it.
func cleanClientName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name))
	if runes := []rune(name); len(runes) > maxClientNameLength {
		name = string(runes[:maxClientNameLength])
	}
	return name
}

// redirectHost returns the host the browser is sent back to after consent.
func redirectHost(redirectURI string) string {
	if u, err := url.Parse(redirectURI); err == nil && u.Host != "" {
		return u.Host
	}
	return redirectURI
}

// roleSummary says in plain words what a role can do through a connection.
func roleSummary(role string) string {
	switch role {
	case models.RoleAdmin:
		return "everything an admin can do, including changing site settings, templates and API keys, and publishing"
	case models.RoleEditor:
		return "create, edit, delete and publish content, upload assets, and manage forks and approvals"
	case models.RoleContributor:
		return "create drafts, upload assets for review, and comment"
	default:
		return "read content, templates, assets and settings"
	}
}

func (h *Handler) renderAuthorize(w http.ResponseWriter, data map[string]interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.Execute(w, data); err != nil {
		log.Printf("[OAuth] Template render error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, state, errCode, errDesc string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid redirect_uri")
		return
	}
	q := u.Query()
	q.Set("error", errCode)
	q.Set("error_description", errDesc)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func jsonError(w http.ResponseWriter, status int, errCode, errDesc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": errDesc,
	})
}

// ProtectedResourceMetadata returns the RFC 9728 metadata handler
func ProtectedResourceMetadataHandler(baseURL string) http.HandlerFunc {
	metadata, _ := json.Marshal(map[string]interface{}{
		"resource":                 baseURL + "/mcp",
		"authorization_servers":    []string{baseURL},
		"bearer_methods_supported": []string{"header"},
		"resource_name":            "LightCMS MCP Server",
	})
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write(metadata)
	}
}

// AuthorizationServerMetadataHandler returns the RFC 8414 metadata handler
func AuthorizationServerMetadataHandler(baseURL string) http.HandlerFunc {
	metadata, _ := json.Marshal(map[string]interface{}{
		"issuer":                                     baseURL,
		"authorization_endpoint":                     baseURL + "/oauth/authorize",
		"token_endpoint":                             baseURL + "/oauth/token",
		"registration_endpoint":                      baseURL + "/oauth/register",
		"revocation_endpoint":                        baseURL + "/oauth/revoke",
		"response_types_supported":                   []string{"code"},
		"grant_types_supported":                      []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported":      []string{"client_secret_post"},
		"revocation_endpoint_auth_methods_supported": []string{"client_secret_post"},
		"code_challenge_methods_supported":           []string{"S256"},
		"scopes_supported":                           []string{},
	})
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write(metadata)
	}
}

// JWKSHandler returns an empty JWKS document (we use opaque tokens, not JWTs)
func JWKSHandler() http.HandlerFunc {
	body := []byte(`{"keys":[]}`)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(body)
	}
}

// TokenRevocation handles token revocation (RFC 7009)
// POST /oauth/revoke
//
// A client can revoke only its own tokens. Revoking a refresh token ends the
// whole connection. An unknown token still gets a 200, as the RFC requires.
func (h *Handler) TokenRevocation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Invalid form data")
		return
	}

	clientID := r.FormValue("client_id")
	if _, err := h.oauthService.ValidateClientCredentials(r.Context(), clientID, r.FormValue("client_secret")); err != nil {
		jsonError(w, http.StatusUnauthorized, "invalid_client", "Invalid client credentials")
		return
	}

	token := r.FormValue("token")
	if token == "" {
		jsonError(w, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}

	if err := h.oauthService.RevokeToken(r.Context(), clientID, token); err != nil {
		log.Printf("[OAuth] Token revocation failed: %v", err)
		jsonError(w, http.StatusServiceUnavailable, "temporarily_unavailable", "Could not revoke the token. Try again.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "{}")
}
