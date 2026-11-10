package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/middleware"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// testCaller is an authenticated caller, as the auth middleware produces.
type testCaller struct {
	name       string
	credential string
}

func (c *testCaller) CredentialKey() string { return c.credential }

// fakeAPI is a REST API that records who each call ran as.
type fakeAPI struct {
	mu       sync.Mutex
	callers  []string
	sessions []string
	headers  []http.Header
	panics   bool
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	panics := f.panics
	caller, _ := middleware.APIUserFromContext(r.Context())
	if c, ok := caller.(*testCaller); ok {
		f.callers = append(f.callers, c.name)
	} else {
		f.callers = append(f.callers, "")
	}
	f.sessions = append(f.sessions, r.Header.Get("X-Agent-Session"))
	f.headers = append(f.headers, r.Header.Clone())
	f.mu.Unlock()
	if panics {
		panic("handler blew up")
	}
	json.NewEncoder(w).Encode([]map[string]interface{}{})
}

func (f *fakeAPI) lastCaller() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.callers) == 0 {
		return ""
	}
	return f.callers[len(f.callers)-1]
}

// newMCPTestServer builds the /mcp endpoint with real authentication in front
// and fake's REST API behind. Tokens: alice-token and bob-token are OAuth,
// lc_carol is an API key.
func newMCPTestServer(t *testing.T, fake *fakeAPI) *httptest.Server {
	t.Helper()
	apiAuth := middleware.NewAPIAuth(func(ctx context.Context, key string) (interface{}, error) {
		if key == "lc_carol" {
			return &testCaller{name: "carol", credential: "apikey:carol"}, nil
		}
		return nil, fmt.Errorf("invalid key")
	})
	apiAuth.SetOAuth(func(ctx context.Context, token string) (interface{}, error) {
		switch token {
		case "alice-token":
			return &testCaller{name: "alice", credential: "oauth:alice"}, nil
		case "bob-token":
			return &testCaller{name: "bob", credential: "oauth:bob"}, nil
		}
		return nil, fmt.Errorf("invalid token")
	}, "https://example.com/.well-known/oauth-protected-resource")

	// The REST side authenticates in-process calls as the real server does.
	rest := apiAuth.Middleware(fake)
	srv := httptest.NewServer(apiAuth.MCPMiddleware(NewHTTPHandler(rest, "https://example.com/.well-known/oauth-protected-resource")))
	t.Cleanup(srv.Close)
	return srv
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func connectAs(t *testing.T, url, token string) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: &bearerTransport{token: token, base: http.DefaultTransport}},
		MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect as %s: %v", token, err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callListTemplates(t *testing.T, session *mcpsdk.ClientSession) *mcpsdk.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_templates", Arguments: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	return result
}

func TestHTTPHandler_ToolCallsRunAsTheCaller(t *testing.T) {
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)

	callListTemplates(t, connectAs(t, srv.URL, "alice-token"))
	if got := fake.lastCaller(); got != "alice" {
		t.Errorf("OAuth call ran as %q, want alice", got)
	}

	callListTemplates(t, connectAs(t, srv.URL, "bob-token"))
	if got := fake.lastCaller(); got != "bob" {
		t.Errorf("OAuth call ran as %q, want bob", got)
	}

	callListTemplates(t, connectAs(t, srv.URL, "lc_carol"))
	if got := fake.lastCaller(); got != "carol" {
		t.Errorf("API key call ran as %q, want carol", got)
	}
}

func TestHTTPHandler_NoCredentialCrossesTheInternalCall(t *testing.T) {
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)

	callListTemplates(t, connectAs(t, srv.URL, "alice-token"))
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, h := range fake.headers {
		if got := h.Get("Authorization"); got != "" {
			t.Errorf("internal call carried Authorization %q", got)
		}
	}
}

func TestHTTPHandler_EachSessionHasItsOwnAgentSession(t *testing.T) {
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)

	first := connectAs(t, srv.URL, "alice-token")
	callListTemplates(t, first)
	callListTemplates(t, first)
	callListTemplates(t, connectAs(t, srv.URL, "alice-token"))

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.sessions) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(fake.sessions))
	}
	if fake.sessions[0] == "" || !strings.HasPrefix(fake.sessions[0], "agent-") {
		t.Errorf("session ID = %q", fake.sessions[0])
	}
	if fake.sessions[0] != fake.sessions[1] {
		t.Error("calls in one session should share its ID")
	}
	if fake.sessions[0] == fake.sessions[2] {
		t.Error("separate sessions should have separate IDs")
	}
}

func TestHTTPHandler_SessionIsBoundToTheCredentialThatOpenedIt(t *testing.T) {
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)
	session := connectAs(t, srv.URL, "alice-token")

	body := `{"jsonrpc":"2.0","id":99,"method":"tools/call","params":{"name":"list_templates","arguments":{}}}`
	send := func(token string) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Session-Id", session.ID())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	if resp := send("bob-token"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("another credential got %d, want 403", resp.StatusCode)
	}
	if got := fake.lastCaller(); got != "" {
		t.Errorf("a tool ran as %q on someone else's session", got)
	}
}

func TestHTTPHandler_RefusesRequestsWithoutACredential(t *testing.T) {
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)

	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("WWW-Authenticate"), "resource_metadata") {
		t.Errorf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
	}
}

func TestHTTPHandler_APanicInAHandlerBecomesAToolError(t *testing.T) {
	fake := &fakeAPI{panics: true}
	srv := newMCPTestServer(t, fake)
	session := connectAs(t, srv.URL, "alice-token")

	result := callListTemplates(t, session)
	if !result.IsError {
		t.Fatalf("expected a tool error, got %+v", result)
	}

	// The server survived and the session still works.
	fake.mu.Lock()
	fake.panics = false
	fake.mu.Unlock()
	if result := callListTemplates(t, session); result.IsError {
		t.Errorf("session did not recover: %+v", result)
	}
}

func TestResourcesRunAsTheCaller(t *testing.T) {
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)
	session := connectAs(t, srv.URL, "alice-token")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: "lightcms://content/recent"}); err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if got := fake.lastCaller(); got != "alice" {
		t.Errorf("resource read ran as %q, want alice", got)
	}
}

func TestInProcessTransport_FailsClosedWithoutACaller(t *testing.T) {
	called := false
	transport := &inProcessTransport{api: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })}

	req, _ := http.NewRequest(http.MethodGet, inProcessBaseURL+"/api/v1/templates", nil)
	if _, err := transport.RoundTrip(req); err == nil {
		t.Error("expected an error")
	}
	if called {
		t.Error("the API was called with no caller")
	}
}

func TestBufferedResponse_DefaultsAndFirstStatusWins(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, inProcessBaseURL, nil)

	empty := &bufferedResponse{header: http.Header{}}
	if resp := empty.response(req); resp.StatusCode != http.StatusOK {
		t.Errorf("no writes: status %d", resp.StatusCode)
	}

	b := &bufferedResponse{header: http.Header{}}
	b.WriteHeader(http.StatusForbidden)
	b.WriteHeader(http.StatusOK)
	b.Write([]byte("no"))
	resp := b.response(req)
	if resp.StatusCode != http.StatusForbidden || resp.ContentLength != 2 {
		t.Errorf("status %d, length %d", resp.StatusCode, resp.ContentLength)
	}
}

func TestCallerTokenInfo_RefusesAnUnauthenticatedContext(t *testing.T) {
	if _, err := callerTokenInfo(context.Background(), "token", nil); err == nil {
		t.Error("expected an error")
	}
}

func TestHostedEndpoint_UploadCannotReadTheServersDisk(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secrets.txt")
	if err := os.WriteFile(secret, []byte("do not leak"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fake := &fakeAPI{}
	srv := newMCPTestServer(t, fake)
	session := connectAs(t, srv.URL, "alice-token")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "upload_asset", Arguments: map[string]interface{}{
		"filename": "x.txt", "serve_path": "/assets/x.txt", "file_path": secret,
	}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected a refusal, got %+v", result)
	}
	if got := fake.lastCaller(); got != "" {
		t.Errorf("the upload reached the API as %q", got)
	}
}
