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

	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"
)

func postForm(handler http.HandlerFunc, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func pkcePair(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func authorizeForm(client *models.OAuthClient, challenge, action string) url.Values {
	return url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"action":                {action},
	}
}

// signIn runs the login step and returns the consent page.
func signIn(t *testing.T, handler *Handler, client *models.OAuthClient, challenge, email string) string {
	t.Helper()
	form := authorizeForm(client, challenge, "login")
	form.Set("email", email)
	form.Set("password", testPassword)
	rr := postForm(handler.AuthorizeSubmit, "/oauth/authorize", form)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

func hiddenField(t *testing.T, page, name string) string {
	t.Helper()
	marker := `name="` + name + `" value="`
	i := strings.Index(page, marker)
	if i < 0 {
		t.Fatalf("field %s not on page:\n%s", name, page)
	}
	rest := page[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

// approveFromPage submits the consent page's own hidden fields.
func approveFromPage(handler *Handler, client *models.OAuthClient, challenge, page string, t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	form := authorizeForm(client, challenge, "approve")
	form.Set("resource", hiddenField(t, page, "resource"))
	form.Set("login_user", hiddenField(t, page, "login_user"))
	form.Set("login_proof", hiddenField(t, page, "login_proof"))
	form.Set("login_ts", hiddenField(t, page, "login_ts"))
	return postForm(handler.AuthorizeSubmit, "/oauth/authorize", form)
}

func TestConsent_ConnectionActsAsThePersonWhoSignedIn(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	ctx := context.Background()

	// An admin exists, but the person connecting is a viewer.
	testAccount(t, users, "boss@example.com", models.RoleAdmin)
	viewer := testAccount(t, users, "viewer@example.com", models.RoleViewer)

	client, secret, err := oauthSvc.RegisterClient(ctx, "Some App", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}
	verifier := "viewer-connection-verifier"

	page := signIn(t, handler, client, pkcePair(verifier), "viewer@example.com")
	rr := approveFromPage(handler, client, pkcePair(verifier), page, t)
	if rr.Code != http.StatusFound {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body.String())
	}
	location, _ := url.Parse(rr.Header().Get("Location"))

	rr = postForm(handler.Token, "/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {client.ClientID},
		"client_secret": {secret},
		"code":          {location.Query().Get("code")},
		"redirect_uri":  {"http://localhost/cb"},
		"code_verifier": {verifier},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("token: %d %s", rr.Code, rr.Body.String())
	}
	var tokens map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &tokens)

	grant, err := oauthSvc.ValidateAccessToken(ctx, tokens["access_token"].(string), testResourceURL)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if grant.UserID != viewer.ID {
		t.Errorf("connection belongs to %s, want the viewer %s", grant.UserID.Hex(), viewer.ID.Hex())
	}
}

func TestConsent_ProofCannotBeMovedToAnotherAccount(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()

	admin := testAccount(t, users, "boss@example.com", models.RoleAdmin)
	testAccount(t, users, "viewer@example.com", models.RoleViewer)
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})
	challenge := pkcePair("verifier")

	page := signIn(t, handler, client, challenge, "viewer@example.com")
	form := authorizeForm(client, challenge, "approve")
	form.Set("login_user", admin.ID.Hex()) // the viewer claims to be the admin
	form.Set("login_proof", hiddenField(t, page, "login_proof"))
	form.Set("login_ts", hiddenField(t, page, "login_ts"))

	rr := postForm(handler.AuthorizeSubmit, "/oauth/authorize", form)
	if rr.Code == http.StatusFound {
		t.Fatalf("approved with a proof made for another account; redirected to %s", rr.Header().Get("Location"))
	}
	if !strings.Contains(rr.Body.String(), "Session expired") {
		t.Errorf("expected a sign-in prompt, got: %s", rr.Body.String())
	}
}

func TestConsent_PageSaysWhoAndWhereAndClaimsNothingElse(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	testAccount(t, users, "viewer@example.com", models.RoleViewer)
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"https://app.example.org/callback"})

	form := authorizeForm(client, pkcePair("v"), "login")
	form.Set("redirect_uri", "https://app.example.org/callback")
	form.Set("email", "viewer@example.com")
	form.Set("password", testPassword)
	rr := postForm(handler.AuthorizeSubmit, "/oauth/authorize", form)
	page := rr.Body.String()

	for _, want := range []string{"viewer@example.com", "app.example.org", "viewer role"} {
		if !strings.Contains(page, want) {
			t.Errorf("consent page does not mention %q", want)
		}
	}
	if strings.Contains(page, "full access") {
		t.Error("consent page still claims full access")
	}
}

func TestLogin_TemporaryPasswordCannotConnectApps(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})

	// The migrated default admin still has its default password.
	form := authorizeForm(client, pkcePair("v"), "login")
	form.Set("email", "admin@localhost")
	form.Set("password", "admin123")
	rr := postForm(handler.AuthorizeSubmit, "/oauth/authorize", form)

	if strings.Contains(rr.Body.String(), "login_proof") {
		t.Error("an account on a temporary password reached the consent page")
	}
	if !strings.Contains(rr.Body.String(), "Set a new password") {
		t.Errorf("expected a prompt to change the password, got: %s", rr.Body.String())
	}
}

func TestApprove_RefusesAccountDisabledSinceLogin(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	ctx := context.Background()
	user := testAccount(t, users, "leaver@example.com", models.RoleEditor)
	client, _, _ := oauthSvc.RegisterClient(ctx, "Some App", []string{"http://localhost/cb"})
	challenge := pkcePair("v")

	page := signIn(t, handler, client, challenge, "leaver@example.com")
	if err := users.DisableUser(ctx, user.ID, true); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}

	rr := approveFromPage(handler, client, challenge, page, t)
	if rr.Code == http.StatusFound {
		t.Fatal("a disabled account completed the connection")
	}
}

func TestAuthorize_RefusesAnotherResource(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {client.ClientID},
		"redirect_uri":          {"http://localhost/cb"},
		"code_challenge":        {"c"},
		"code_challenge_method": {"S256"},
		"resource":              {"https://elsewhere.example/mcp"},
	}
	rr := httptest.NewRecorder()
	handler.Authorize(rr, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil))

	if rr.Code != http.StatusFound {
		t.Fatalf("expected a redirect with an error, got %d", rr.Code)
	}
	location, _ := url.Parse(rr.Header().Get("Location"))
	if location.Query().Get("error") != "invalid_target" {
		t.Errorf("expected invalid_target, got %q", location.Query().Get("error"))
	}
}

func TestToken_RefusesAnotherResource(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()
	client, secret, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})

	rr := postForm(handler.Token, "/oauth/token", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {client.ClientID},
		"client_secret": {secret},
		"refresh_token": {"whatever"},
		"resource":      {"https://elsewhere.example/mcp"},
	})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "invalid_target") {
		t.Errorf("expected invalid_target, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSameResource(t *testing.T) {
	const canonical = "https://example.com/mcp"
	same := []string{canonical, "https://example.com/mcp/", "HTTPS://EXAMPLE.com/mcp", "https://example.com:443/mcp"}
	for _, r := range same {
		if !sameResource(r, canonical) {
			t.Errorf("%q should match", r)
		}
	}
	different := []string{"https://example.com", "https://example.com/api/v1", "http://example.com/mcp", "https://example.com/mcp?x=1", "https://example.com:8443/mcp", "not a url"}
	for _, r := range different {
		if sameResource(r, canonical) {
			t.Errorf("%q should not match", r)
		}
	}
}

func TestRegister_Limits(t *testing.T) {
	handler, _, cleanup := newTestHandler(t)
	defer cleanup()

	register := func(body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		handler.Register(rr, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body)))
		return rr
	}

	if rr := register(`{"client_name":"X","redirect_uris":["http://evil.example/cb"]}`); rr.Code != http.StatusBadRequest {
		t.Errorf("plain http to a remote host: %d", rr.Code)
	}
	if rr := register(`{"client_name":"X","redirect_uris":["javascript:alert(1)"]}`); rr.Code != http.StatusBadRequest {
		t.Errorf("javascript scheme: %d", rr.Code)
	}
	if rr := register(`{"client_name":"X","redirect_uris":["https://a.example/cb#frag"]}`); rr.Code != http.StatusBadRequest {
		t.Errorf("fragment: %d", rr.Code)
	}
	many := `"https://a.example/cb"` + strings.Repeat(`,"https://a.example/cb"`, maxRedirectURIs)
	if rr := register(`{"client_name":"X","redirect_uris":[` + many + `]}`); rr.Code != http.StatusBadRequest {
		t.Errorf("too many redirect URIs: %d", rr.Code)
	}
	huge := `{"client_name":"` + strings.Repeat("x", maxRegistrationBody) + `","redirect_uris":["https://a.example/cb"]}`
	if rr := register(huge); rr.Code != http.StatusBadRequest {
		t.Errorf("oversized body: %d", rr.Code)
	}
	if rr := register(`{"client_name":"X","redirect_uris":["https://a.example/cb","http://127.0.0.1:3334/cb","http://[::1]:3334/cb"]}`); rr.Code != http.StatusCreated {
		t.Errorf("https and loopback should register: %d %s", rr.Code, rr.Body.String())
	}
}

func TestCleanClientName(t *testing.T) {
	cases := map[string]string{
		"  Claude  ":             "Claude",
		"Trusted‮evil":           "Trustedevil",
		"Zero​Width":             "ZeroWidth",
		"Line\nBreak":            "LineBreak",
		strings.Repeat("é", 150): strings.Repeat("é", maxClientNameLength),
	}
	for in, want := range cases {
		if got := cleanClientName(in); got != want {
			t.Errorf("cleanClientName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenRevocation_ClientMustAuthenticate(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})

	rr := postForm(handler.TokenRevocation, "/oauth/revoke", url.Values{
		"client_id":     {client.ClientID},
		"client_secret": {"wrong"},
		"token":         {"anything"},
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTokenRevocation_RevokesTheToken(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	client, secret, _ := oauthSvc.RegisterClient(ctx, "Some App", []string{"http://localhost/cb"})
	verifier := "revocation-verifier"
	code, _ := oauthSvc.CreateAuthCode(ctx, testGrant(t, oauthSvc, client), "http://localhost/cb", pkcePair(verifier), "S256")
	access, refresh, _, err := oauthSvc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}

	rr := postForm(handler.TokenRevocation, "/oauth/revoke", url.Values{
		"client_id":     {client.ClientID},
		"client_secret": {secret},
		"token":         {refresh},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := oauthSvc.ValidateAccessToken(ctx, access, testResourceURL); err == nil {
		t.Error("access token still works after its connection was revoked")
	}
}

func TestTokenRevocation_UnknownTokenStillSucceeds(t *testing.T) {
	handler, oauthSvc, cleanup := newTestHandler(t)
	defer cleanup()
	client, secret, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})

	rr := postForm(handler.TokenRevocation, "/oauth/revoke", url.Values{
		"client_id":     {client.ClientID},
		"client_secret": {secret},
		"token":         {"never-issued"},
	})
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestConsent_ExpiredProofRefused(t *testing.T) {
	handler, oauthSvc, users, cleanup := newTestHandlerWithUsers(t)
	defer cleanup()
	user := testAccount(t, users, "author@example.com", models.RoleEditor)
	client, _, _ := oauthSvc.RegisterClient(context.Background(), "Some App", []string{"http://localhost/cb"})
	challenge := pkcePair("v")

	ts := strconv.FormatInt(time.Now().Add(-loginProofMaxAge-time.Minute).Unix(), 10)
	form := authorizeForm(client, challenge, "approve")
	form.Set("login_user", user.ID.Hex())
	form.Set("login_ts", ts)
	form.Set("login_proof", handler.computeLoginProof(user.ID.Hex(), client.ClientID, "http://localhost/cb", challenge, services.MCPResourceURL("https://example.com"), ts))

	rr := postForm(handler.AuthorizeSubmit, "/oauth/authorize", form)
	if rr.Code == http.StatusFound {
		t.Fatal("approved with an expired proof")
	}
}
