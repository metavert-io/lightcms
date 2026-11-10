package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
	"github.com/metavert-io/lightcms/v7/internal/middleware"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// inProcessBaseURL is never dialled: requests go straight to the router.
	inProcessBaseURL = "http://lightcms.internal"
	// callerExtraKey is where a request's caller travels inside TokenInfo.
	callerExtraKey = "lightcms.caller"
	// callerInfoLifetime satisfies the SDK, which refuses a TokenInfo without an
	// expiry. The credential was checked moments ago, and is again on every
	// request; API keys have no expiry of their own to report.
	callerInfoLifetime = time.Minute
)

// callerContextKey carries the MCP request's caller to the in-process transport.
type callerContextKey struct{}

// NewHTTPHandler creates the handler for the MCP Streamable HTTP endpoint. It
// must run behind APIAuth.MCPMiddleware, which authenticates every request.
//
// Each MCP session gets its own server. Its tools call api, the REST router, in
// this process, as whoever sent the current MCP request, so no credential is
// held by the session or sent over a socket. The session is bound to the
// credential that opened it: the SDK refuses a request from any other.
func NewHTTPHandler(api http.Handler, resourceMetadataURL string) http.Handler {
	transport := &inProcessTransport{api: api}
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server {
			client := apiclient.NewWithTransport(inProcessBaseURL, transport)
			client.SetAgentSession(apiclient.NewAgentSessionID())
			server := NewServer(client).MCPServer()
			server.AddReceivingMiddleware(carryCaller)
			return server
		},
		nil,
	)
	return sdkauth.RequireBearerToken(callerTokenInfo, &sdkauth.RequireBearerTokenOptions{
		ResourceMetadataURL: resourceMetadataURL,
	})(streamable)
}

// callerTokenInfo is the SDK's token verifier. The credential has already been
// validated by APIAuth.MCPMiddleware, so this only reports the result: UserID
// turns on the SDK's session binding and Extra takes the caller to the tools.
func callerTokenInfo(ctx context.Context, _ string, _ *http.Request) (*sdkauth.TokenInfo, error) {
	caller, _ := middleware.APIUserFromContext(ctx)
	key := middleware.CredentialKeyFromContext(ctx)
	if caller == nil || key == "" {
		return nil, fmt.Errorf("%w: no authenticated caller", sdkauth.ErrInvalidToken)
	}
	return &sdkauth.TokenInfo{
		UserID:     key,
		Expiration: time.Now().Add(callerInfoLifetime),
		Extra:      map[string]any{callerExtraKey: caller},
	}, nil
}

// carryCaller moves the caller from the SDK's per-request data into the context
// that tool and resource handlers pass to the API client.
func carryCaller(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if extra := req.GetExtra(); extra != nil && extra.TokenInfo != nil {
			if caller := extra.TokenInfo.Extra[callerExtraKey]; caller != nil {
				ctx = context.WithValue(ctx, callerContextKey{}, caller)
			}
		}
		return next(ctx, method, req)
	}
}

// inProcessTransport serves API client requests by calling the REST router
// directly, as the caller of the MCP request they belong to.
type inProcessTransport struct {
	api http.Handler
}

// RoundTrip implements http.RoundTripper.
func (t *inProcessTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	caller := req.Context().Value(callerContextKey{})
	if caller == nil {
		return nil, errors.New("no authenticated caller for this MCP request")
	}

	// A panic in a handler would otherwise reach the SDK's goroutine and take
	// the whole server down; net/http only recovers its own.
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[MCP] panic serving %s %s: %v\n%s", req.Method, req.URL.Path, p, debug.Stack())
			resp, err = nil, errors.New("internal server error")
		}
	}()

	inner := req.Clone(middleware.WithInternalCaller(req.Context(), caller))
	inner.RequestURI = req.URL.RequestURI()
	inner.RemoteAddr = "127.0.0.1:0"
	if inner.Body == nil {
		inner.Body = http.NoBody
	}

	rec := &bufferedResponse{header: http.Header{}}
	t.api.ServeHTTP(rec, inner)
	return rec.response(req), nil
}

// bufferedResponse is the http.ResponseWriter for an in-process call.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

func (b *bufferedResponse) response(req *http.Request) *http.Response {
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        b.header,
		Body:          io.NopCloser(&b.body),
		ContentLength: int64(b.body.Len()),
		Request:       req,
	}
}
