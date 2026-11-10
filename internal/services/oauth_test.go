package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const testResource = "https://example.com/mcp"

// testGrant records that a new person approved the client.
func testGrant(t *testing.T, svc *OAuthService, client *models.OAuthClient) *models.OAuthGrant {
	t.Helper()
	grant, err := svc.SaveGrant(context.Background(), client, primitive.NewObjectID(), testResource)
	if err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}
	return grant
}

func TestHashToken(t *testing.T) {
	token := "test-token-123"
	h1 := hashToken(token)
	h2 := hashToken(token)

	if h1 != h2 {
		t.Error("hashToken should be deterministic")
	}

	// Verify it's SHA-256
	expected := sha256.Sum256([]byte(token))
	expectedHex := hex.EncodeToString(expected[:])
	if h1 != expectedHex {
		t.Errorf("expected SHA-256 hex, got %s", h1)
	}

	// Different input = different output
	h3 := hashToken("different-token")
	if h1 == h3 {
		t.Error("different tokens should produce different hashes")
	}
}

func TestGenerateToken(t *testing.T) {
	t1, err := generateToken(16)
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	// Should be base64url encoded
	decoded, err := base64.RawURLEncoding.DecodeString(t1)
	if err != nil {
		t.Fatalf("token is not valid base64url: %v", err)
	}
	if len(decoded) != 16 {
		t.Errorf("expected 16 decoded bytes, got %d", len(decoded))
	}

	// Two tokens should be different
	t2, err := generateToken(16)
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}
	if t1 == t2 {
		t.Error("two generated tokens should be different")
	}
}

func TestVerifyPKCE(t *testing.T) {
	// Generate a known verifier and its S256 challenge
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	if !verifyPKCE(verifier, challenge, "S256") {
		t.Error("verifyPKCE should return true for matching verifier/challenge")
	}

	if verifyPKCE("wrong-verifier", challenge, "S256") {
		t.Error("verifyPKCE should return false for wrong verifier")
	}

	if verifyPKCE(verifier, challenge, "plain") {
		t.Error("verifyPKCE should return false for non-S256 method")
	}

	if verifyPKCE(verifier, challenge, "") {
		t.Error("verifyPKCE should return false for empty method")
	}
}

func TestRegisterClient(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	client, rawSecret, err := svc.RegisterClient(ctx, "Test Client", []string{"http://localhost/callback"})
	if err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	if client.ClientID == "" {
		t.Error("expected non-empty ClientID")
	}
	if rawSecret == "" {
		t.Error("expected non-empty raw secret")
	}
	if client.ClientName != "Test Client" {
		t.Errorf("expected client name 'Test Client', got %q", client.ClientName)
	}
	if len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != "http://localhost/callback" {
		t.Error("expected redirect URI to be stored")
	}

	// Verify stored hash matches
	expectedHash := hashToken(rawSecret)
	if client.ClientSecretHash != expectedHash {
		t.Error("stored hash should match hash of raw secret")
	}
}

func TestRegisterClient_EmptyName(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	_, _, err := svc.RegisterClient(context.Background(), "", []string{"http://localhost/callback"})
	if err == nil {
		t.Error("expected error for empty client name")
	}
}

func TestRegisterClient_NoRedirectURIs(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	_, _, err := svc.RegisterClient(context.Background(), "Test", []string{})
	if err == nil {
		t.Error("expected error for empty redirect URIs")
	}
}

func TestValidateClient(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	client, _, err := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb", "http://localhost/cb2"})
	if err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	// Valid client and redirect
	found, err := svc.ValidateClient(ctx, client.ClientID, "http://localhost/cb")
	if err != nil {
		t.Fatalf("ValidateClient failed: %v", err)
	}
	if found.ClientID != client.ClientID {
		t.Error("expected same client ID")
	}

	// Valid client, empty redirect: refused, the registered list is always checked
	_, err = svc.ValidateClient(ctx, client.ClientID, "")
	if err == nil {
		t.Error("expected error for an empty redirect URI")
	}

	// Invalid client ID
	_, err = svc.ValidateClient(ctx, "nonexistent", "http://localhost/cb")
	if err == nil {
		t.Error("expected error for invalid client ID")
	}

	// Wrong redirect URI
	_, err = svc.ValidateClient(ctx, client.ClientID, "http://evil.com/steal")
	if err == nil {
		t.Error("expected error for unregistered redirect URI")
	}
}

func TestValidateClientCredentials(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	client, rawSecret, err := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	// Correct credentials
	_, err = svc.ValidateClientCredentials(ctx, client.ClientID, rawSecret)
	if err != nil {
		t.Fatalf("ValidateClientCredentials should succeed: %v", err)
	}

	// Wrong secret
	_, err = svc.ValidateClientCredentials(ctx, client.ClientID, "wrong-secret")
	if err == nil {
		t.Error("expected error for wrong client secret")
	}

	// Wrong client ID
	_, err = svc.ValidateClientCredentials(ctx, "nonexistent", rawSecret)
	if err == nil {
		t.Error("expected error for wrong client ID")
	}
}

func TestCreateAuthCode(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, err := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", "challenge", "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}
	if code == "" {
		t.Error("expected non-empty auth code")
	}

	// Verify code is stored (by its hash)
	codeHash := hashToken(code)
	count, err := db.Count(ctx, "oauth_auth_codes", bson.M{"code": codeHash})
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 auth code in DB, got %d", count)
	}
}

func TestExchangeAuthCode_FullFlow(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	// Generate PKCE pair
	verifier := "a]b%c-d_e.f~g1h2i3j4k5l6m7n8o9p0q1r2s3t4u5v6w7x8y9z0"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, err := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	code, err := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	accessToken, refreshToken, expiresIn, err := svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)
	if err != nil {
		t.Fatalf("ExchangeAuthCode failed: %v", err)
	}

	if accessToken == "" {
		t.Error("expected non-empty access token")
	}
	if refreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if expiresIn <= 0 {
		t.Error("expected positive expires_in")
	}
}

func TestExchangeAuthCode_SingleUse(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	verifier := "test-verifier-for-single-use-check"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, _ := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")

	// First exchange should succeed
	_, _, _, err := svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)
	if err != nil {
		t.Fatalf("first exchange failed: %v", err)
	}

	// Second exchange with same code should fail
	_, _, _, err = svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)
	if err == nil {
		t.Error("expected error for reused auth code")
	}
}

func TestExchangeAuthCode_WrongPKCE(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	verifier := "correct-verifier"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, _ := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")

	_, _, _, err := svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", "wrong-verifier")
	if err == nil {
		t.Error("expected PKCE verification to fail")
	}
}

func TestExchangeAuthCode_ClientMismatch(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	verifier := "verifier-for-mismatch-test"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, _ := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")

	_, _, _, err := svc.ExchangeAuthCode(ctx, code, "wrong-client-id", "http://localhost/cb", verifier)
	if err == nil {
		t.Error("expected error for client ID mismatch")
	}
}

func TestExchangeAuthCode_RedirectMismatch(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	verifier := "verifier-for-redirect-test"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, _ := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")

	_, _, _, err := svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://evil.com/steal", verifier)
	if err == nil {
		t.Error("expected error for redirect URI mismatch")
	}
}

func TestExchangeAuthCode_InvalidCode(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	_, _, _, err := svc.ExchangeAuthCode(ctx, "nonexistent-code", "client", "http://localhost/cb", "verifier")
	if err == nil {
		t.Error("expected error for invalid auth code")
	}
}

func TestRefreshAccessToken(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	verifier := "verifier-for-refresh-test"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, _ := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")

	_, refreshToken, _, _ := svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)

	// Refresh should return new tokens
	newAccess, newRefresh, expiresIn, err := svc.RefreshAccessToken(ctx, refreshToken, client.ClientID)
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}
	if newAccess == "" {
		t.Error("expected non-empty new access token")
	}
	if newRefresh == "" {
		t.Error("expected non-empty new refresh token")
	}
	if expiresIn <= 0 {
		t.Error("expected positive expires_in")
	}

	// Old refresh token should be revoked (rotation)
	_, _, _, err = svc.RefreshAccessToken(ctx, refreshToken, client.ClientID)
	if err == nil {
		t.Error("expected error for revoked refresh token")
	}

	// New refresh token should work
	_, _, _, err = svc.RefreshAccessToken(ctx, newRefresh, client.ClientID)
	if err != nil {
		t.Fatalf("new refresh token should work: %v", err)
	}
}

func TestRefreshAccessToken_ClientMismatch(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	verifier := "verifier-refresh-client-mismatch"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	client, _, _ := svc.RegisterClient(ctx, "Test", []string{"http://localhost/cb"})
	code, _ := svc.CreateAuthCode(ctx, testGrant(t, svc, client), "http://localhost/cb", challenge, "S256")
	_, refreshToken, _, _ := svc.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)

	_, _, _, err := svc.RefreshAccessToken(ctx, refreshToken, "wrong-client")
	if err == nil {
		t.Error("expected error for client ID mismatch on refresh")
	}
}

func TestRefreshAccessToken_InvalidToken(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	_, _, _, err := svc.RefreshAccessToken(context.Background(), "nonexistent", "client")
	if err == nil {
		t.Error("expected error for invalid refresh token")
	}
}

func TestDeleteClientByID(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	svc := NewOAuthService(db)
	ctx := context.Background()

	client, _, err := svc.RegisterClient(ctx, "To Delete", []string{"http://localhost/cb"})
	if err != nil || client == nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	err = svc.DeleteClientByID(ctx, client.ID)
	if err != nil {
		t.Fatalf("DeleteClientByID failed: %v", err)
	}

	// Verify it's gone
	_, err = svc.ValidateClient(ctx, client.ClientID, "")
	if err == nil {
		t.Error("expected error after client deletion")
	}
}
