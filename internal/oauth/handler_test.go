package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/auth"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

func newTestHandler(t *testing.T) (*Handler, *services.OAuthService, func()) {
	t.Helper()
	handler, oauthService, _, cleanup := newTestHandlerWithUsers(t)
	return handler, oauthService, cleanup
}

func newTestHandlerWithUsers(t *testing.T) (*Handler, *services.OAuthService, *services.UserService, func()) {
	t.Helper()
	db, cleanup := testutil.MustConnectTestDB(t)

	oauthService := services.NewOAuthService(db)
	store := sessions.NewCookieStore([]byte("test-secret-32-bytes-long-enough"))
	userService := services.NewUserService(db)
	authManager := auth.NewManager(store, db, userService)
	authManager.MigrateToMultiUser(context.Background())

	handler := NewHandler(oauthService, authManager, "https://example.com", "test-session-secret")
	return handler, oauthService, userService, cleanup
}

const testPassword = "correct horse battery staple"

// testAccount creates an account that has already chosen its own password.
func testAccount(t *testing.T, users *services.UserService, email, role string) *models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hashing password: %v", err)
	}
	user, err := users.CreateUserWithHash(context.Background(), email, email, role, string(hash), false)
	if err != nil {
		t.Fatalf("CreateUserWithHash: %v", err)
	}
	return user
}

const testResourceURL = "https://example.com/mcp"

// testGrant records that a new person approved the client.
func testGrant(t *testing.T, oauthSvc *services.OAuthService, client *models.OAuthClient) *models.OAuthGrant {
	t.Helper()
	grant, err := oauthSvc.SaveGrant(context.Background(), client, primitive.NewObjectID(), testResourceURL)
	if err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}
	return grant
}

func TestRegister_Success(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	body := `{"client_name":"Test App","redirect_uris":["http://localhost/callback"]}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["client_id"] == nil || resp["client_id"] == "" {
		t.Error("expected client_id in response")
	}
	if resp["client_secret"] == nil || resp["client_secret"] == "" {
		t.Error("expected client_secret in response")
	}
	if resp["client_name"] != "Test App" {
		t.Errorf("expected client_name 'Test App', got %v", resp["client_name"])
	}
}

func TestRegister_MissingRedirectURIs(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	body := `{"client_name":"Test App","redirect_uris":[]}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRegister_DefaultClientName(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	body := `{"redirect_uris":["http://localhost/callback"]}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.Register(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["client_name"] != "Unknown Client" {
		t.Errorf("expected default client_name 'Unknown Client', got %v", resp["client_name"])
	}
}

func TestAuthorize_ValidParams(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	// Register a client first
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"code_challenge":        {"test-challenge"},
		"code_challenge_method": {"S256"},
		"state":                 {"test-state"},
	}

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil)
	rr := httptest.NewRecorder()

	handler.Authorize(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Should render login form HTML
	body := rr.Body.String()
	if !strings.Contains(body, "password") {
		t.Error("expected login form with password field")
	}
}

func TestAuthorize_MissingPKCE(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {client.ClientID},
		"redirect_uri":  {"http://localhost/cb"},
	}

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil)
	rr := httptest.NewRecorder()

	handler.Authorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing PKCE, got %d", rr.Code)
	}
}

func TestAuthorize_WrongResponseType(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	params := url.Values{
		"response_type": {"token"},
		"client_id":     {"any"},
	}

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil)
	rr := httptest.NewRecorder()

	handler.Authorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for wrong response_type, got %d", rr.Code)
	}
}

func TestAuthorize_InvalidClient(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {"nonexistent"},
		"redirect_uri":          {"http://localhost/cb"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
	}

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil)
	rr := httptest.NewRecorder()

	handler.Authorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestToken_InvalidGrantType(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	// Register a client
	body := `{"client_name":"Test","redirect_uris":["http://localhost/cb"]}`
	regReq := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	regReq.Header.Set("Content-Type", "application/json")
	regRR := httptest.NewRecorder()
	handler.Register(regRR, regReq)

	var regResp map[string]interface{}
	json.NewDecoder(regRR.Body).Decode(&regResp)

	form := url.Values{
		"grant_type":    {"implicit"},
		"client_id":     {regResp["client_id"].(string)},
		"client_secret": {regResp["client_secret"].(string)},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.Token(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported grant_type, got %d", rr.Code)
	}
}

func TestToken_InvalidClientCredentials(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {"nonexistent"},
		"client_secret": {"wrong"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.Token(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestProtectedResourceMetadataHandler(t *testing.T) {
	handler := ProtectedResourceMetadataHandler("https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["resource"] != "https://example.com/mcp" {
		t.Errorf("expected resource URL, got %v", resp["resource"])
	}
	servers := resp["authorization_servers"].([]interface{})
	if len(servers) != 1 || servers[0] != "https://example.com" {
		t.Errorf("unexpected authorization_servers: %v", servers)
	}
}

func TestProtectedResourceMetadataHandler_CORS(t *testing.T) {
	handler := ProtectedResourceMetadataHandler("https://example.com")

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/oauth-protected-resource", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}
}

func TestAuthorizationServerMetadataHandler(t *testing.T) {
	handler := AuthorizationServerMetadataHandler("https://example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["issuer"] != "https://example.com" {
		t.Errorf("expected issuer, got %v", resp["issuer"])
	}
	if resp["authorization_endpoint"] != "https://example.com/oauth/authorize" {
		t.Errorf("unexpected authorization_endpoint: %v", resp["authorization_endpoint"])
	}
	if resp["token_endpoint"] != "https://example.com/oauth/token" {
		t.Errorf("unexpected token_endpoint: %v", resp["token_endpoint"])
	}
	if resp["registration_endpoint"] != "https://example.com/oauth/register" {
		t.Errorf("unexpected registration_endpoint: %v", resp["registration_endpoint"])
	}

	// Check code_challenge_methods_supported includes S256
	methods := resp["code_challenge_methods_supported"].([]interface{})
	if len(methods) != 1 || methods[0] != "S256" {
		t.Errorf("expected [S256], got %v", methods)
	}
}

func TestJWKSHandler(t *testing.T) {
	handler := JWKSHandler()

	req := httptest.NewRequest(http.MethodGet, "/oauth/jwks", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	keys := resp["keys"].([]interface{})
	if len(keys) != 0 {
		t.Errorf("expected empty keys array, got %v", keys)
	}
}

func TestComputeVerifyLoginProof(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	userID := "user-1"
	clientID := "test-client"
	redirectURI := "http://localhost/cb"
	codeChallenge := "challenge123"
	resource := testResourceURL
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	proof := handler.computeLoginProof(userID, clientID, redirectURI, codeChallenge, resource, ts)
	if proof == "" {
		t.Error("expected non-empty proof")
	}

	// Verify should pass
	if !handler.verifyLoginProof(userID, clientID, redirectURI, codeChallenge, resource, ts, proof) {
		t.Error("expected proof to be valid")
	}

	// Wrong proof should fail
	if handler.verifyLoginProof(userID, clientID, redirectURI, codeChallenge, resource, ts, "wrong-proof") {
		t.Error("expected wrong proof to fail")
	}

	// A proof is good only for the person, client and resource it was made for
	if handler.verifyLoginProof("user-2", clientID, redirectURI, codeChallenge, resource, ts, proof) {
		t.Error("expected proof to be bound to the user")
	}
	if handler.verifyLoginProof(userID, "other-client", redirectURI, codeChallenge, resource, ts, proof) {
		t.Error("expected proof to be bound to the client")
	}
	if handler.verifyLoginProof(userID, clientID, redirectURI, codeChallenge, "https://other.example/mcp", ts, proof) {
		t.Error("expected proof to be bound to the resource")
	}
}

func TestVerifyLoginProof_Expired(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	// Use a timestamp from 10 minutes ago (proof max age is 5 minutes)
	oldTs := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	proof := handler.computeLoginProof("user", "client", "uri", "challenge", testResourceURL, oldTs)

	if handler.verifyLoginProof("user", "client", "uri", "challenge", testResourceURL, oldTs, proof) {
		t.Error("expected expired proof to fail")
	}
}

func TestVerifyLoginProof_InvalidTimestamp(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	if handler.verifyLoginProof("user", "client", "uri", "challenge", testResourceURL, "not-a-number", "proof") {
		t.Error("expected invalid timestamp to fail")
	}
}

func TestJsonError(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonError(rr, http.StatusBadRequest, "invalid_request", "Missing parameter")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["error"] != "invalid_request" {
		t.Errorf("expected error code 'invalid_request', got %q", resp["error"])
	}
	if resp["error_description"] != "Missing parameter" {
		t.Errorf("expected error description, got %q", resp["error_description"])
	}
}

// AuthorizeSubmit tests

func TestAuthorizeSubmit_LoginSuccess(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	testAccount(t, users, "author@example.com", models.RoleEditor)

	client, _, err := oauthSvc.RegisterClient(context.Background(), "Test App", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"state":                 {"test-state"},
		"code_challenge":        {"challenge123"},
		"code_challenge_method": {"S256"},
		"action":                {"login"},
		"email":                 {"author@example.com"},
		"password":              {testPassword},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Should show consent page with login_proof
	body := rr.Body.String()
	if !strings.Contains(body, "login_proof") {
		t.Error("expected consent page with login_proof field")
	}
	if !strings.Contains(body, "Allow Access") {
		t.Error("expected Allow Access button on consent page")
	}
}

func TestAuthorizeSubmit_LoginWrongPassword(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"login"},
		"email":                 {"admin@localhost"},
		"password":              {"wrongpassword"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Invalid email or password") {
		t.Error("expected 'Invalid email or password' error in response")
	}
}

func TestAuthorizeSubmit_Approve(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	user := testAccount(t, users, "author@example.com", models.RoleEditor)

	client, _, err := oauthSvc.RegisterClient(context.Background(), "Test App", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}

	// Generate a valid login proof
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	proof := handler.computeLoginProof(user.ID.Hex(), client.ClientID, "http://localhost/cb", "challenge", testResourceURL, ts)

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"state":                 {"mystate"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"approve"},
		"login_user":            {user.ID.Hex()},
		"login_proof":           {proof},
		"login_ts":              {ts},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d: %s", rr.Code, rr.Body.String())
	}

	loc := rr.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}

	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("failed to parse Location: %v", err)
	}

	if u.Query().Get("code") == "" {
		t.Error("expected code in redirect query")
	}
	if u.Query().Get("state") != "mystate" {
		t.Errorf("expected state=mystate, got %q", u.Query().Get("state"))
	}
}

func TestAuthorizeSubmit_ApproveExpiredProof(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	// Expired timestamp
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	proof := handler.computeLoginProof("user", client.ClientID, "http://localhost/cb", "challenge", testResourceURL, ts)

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"approve"},
		"login_user":            {"user"},
		"login_proof":           {proof},
		"login_ts":              {ts},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Session expired") {
		t.Error("expected session expired error")
	}
}

func TestAuthorizeSubmit_Deny(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"state":                 {"mystate"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"deny"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("error") != "access_denied" {
		t.Errorf("expected error=access_denied, got %q", u.Query().Get("error"))
	}
	if u.Query().Get("state") != "mystate" {
		t.Errorf("expected state=mystate, got %q", u.Query().Get("state"))
	}
}

func TestAuthorizeSubmit_UnknownAction(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"unknown"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAuthorizeSubmit_InvalidClient(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	form := url.Values{
		"client_id":             {"nonexistent"},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"login"},
		"password":              {"admin123"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// Token endpoint full flow tests

func TestToken_AuthorizationCodeGrant(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	// Register client
	client, rawSecret, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	// Create PKCE pair
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	// Create auth code
	code, _ := oauthSvc.CreateAuthCode(context.Background(), testGrant(t, oauthSvc, client), "http://localhost/cb", challenge, "S256")

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {client.ClientID},
		"client_secret": {rawSecret},
		"code":          {code},
		"redirect_uri":  {"http://localhost/cb"},
		"code_verifier": {verifier},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.Token(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["access_token"] == nil || resp["access_token"] == "" {
		t.Error("expected access_token")
	}
	if resp["refresh_token"] == nil || resp["refresh_token"] == "" {
		t.Error("expected refresh_token")
	}
	if resp["token_type"] != "Bearer" {
		t.Errorf("expected token_type Bearer, got %v", resp["token_type"])
	}
	if resp["expires_in"] == nil {
		t.Error("expected expires_in")
	}
}

func TestToken_RefreshTokenGrant(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	// Register client and obtain tokens
	client, rawSecret, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	verifier := "refresh-test-verifier-for-handler"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	code, _ := oauthSvc.CreateAuthCode(context.Background(), testGrant(t, oauthSvc, client), "http://localhost/cb", challenge, "S256")
	_, refreshToken, _, _ := oauthSvc.ExchangeAuthCode(context.Background(), code, client.ClientID, "http://localhost/cb", verifier)

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {client.ClientID},
		"client_secret": {rawSecret},
		"refresh_token": {refreshToken},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.Token(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp["access_token"] == nil || resp["access_token"] == "" {
		t.Error("expected access_token")
	}
	if resp["refresh_token"] == nil || resp["refresh_token"] == "" {
		t.Error("expected new refresh_token")
	}
}

func TestToken_AuthorizationCodeGrant_InvalidCode(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, rawSecret, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {client.ClientID},
		"client_secret": {rawSecret},
		"code":          {"invalid-code"},
		"redirect_uri":  {"http://localhost/cb"},
		"code_verifier": {"verifier"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.Token(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestToken_RefreshTokenGrant_InvalidToken(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()

	client, rawSecret, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {client.ClientID},
		"client_secret": {rawSecret},
		"refresh_token": {"invalid-refresh-token"},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.Token(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAuthorizationServerMetadataHandler_CORS(t *testing.T) {
	h := AuthorizationServerMetadataHandler("https://example.com")

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/oauth-authorization-server", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}
}

func TestRedirectWithError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	redirectWithError(rr, req, "http://localhost/cb", "mystate", "access_denied", "User denied")

	if rr.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("error") != "access_denied" {
		t.Error("expected error in redirect")
	}
	if u.Query().Get("state") != "mystate" {
		t.Error("expected state in redirect")
	}
}

func TestRedirectWithError_NoState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	redirectWithError(rr, req, "http://localhost/cb", "", "server_error", "Oops")

	loc := rr.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("state") != "" {
		t.Error("expected no state param when empty")
	}
}

func TestRedirectWithError_InvalidURI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	redirectWithError(rr, req, "://invalid", "", "error", "desc")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid redirect_uri, got %d", rr.Code)
	}
}

func TestAuthorizeSubmit_ApproveNoState(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	user := testAccount(t, users, "author@example.com", models.RoleEditor)

	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	proof := handler.computeLoginProof(user.ID.Hex(), client.ClientID, "http://localhost/cb", "challenge", testResourceURL, ts)

	form := url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"state":                 {""},
		"code_challenge":        {"challenge"},
		"code_challenge_method": {"S256"},
		"action":                {"approve"},
		"login_user":            {user.ID.Hex()},
		"login_proof":           {proof},
		"login_ts":              {ts},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.AuthorizeSubmit(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("code") == "" {
		t.Error("expected code in redirect")
	}
	if u.Query().Get("state") != "" {
		t.Error("expected no state when empty")
	}
}
