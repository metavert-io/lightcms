package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const testVerifier = "grant-test-verifier"

func testChallenge() string {
	sum := sha256.Sum256([]byte(testVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type oauthFixture struct {
	db     *database.DB
	svc    *OAuthService
	client *models.OAuthClient
	grant  *models.OAuthGrant
}

func newOAuthFixture(t *testing.T) *oauthFixture {
	t.Helper()
	db, cleanup := testutil.MustConnectTestDB(t)
	t.Cleanup(cleanup)
	svc := NewOAuthService(db)
	client, _, err := svc.RegisterClient(context.Background(), "Test", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}
	return &oauthFixture{db: db, svc: svc, client: client, grant: testGrant(t, svc, client)}
}

func (f *oauthFixture) code(t *testing.T) string {
	t.Helper()
	code, err := f.svc.CreateAuthCode(context.Background(), f.grant, "http://localhost/cb", testChallenge(), "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	return code
}

// tokens returns a fresh access + refresh token under the fixture's grant.
func (f *oauthFixture) tokens(t *testing.T) (string, string) {
	t.Helper()
	access, refresh, _, err := f.svc.ExchangeAuthCode(context.Background(), f.code(t), f.client.ClientID, "http://localhost/cb", testVerifier)
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}
	return access, refresh
}

func TestSaveGrant_OneActiveGrantPerPersonAndClient(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	user := primitive.NewObjectID()

	first, err := f.svc.SaveGrant(ctx, f.client, user, testResource, AccessSettings{})
	if err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}
	again, err := f.svc.SaveGrant(ctx, f.client, user, testResource, AccessSettings{})
	if err != nil {
		t.Fatalf("SaveGrant again: %v", err)
	}
	if first.ID != again.ID {
		t.Error("approving the same client again should reuse the grant")
	}

	other, err := f.svc.SaveGrant(ctx, f.client, primitive.NewObjectID(), testResource, AccessSettings{})
	if err != nil {
		t.Fatalf("SaveGrant other: %v", err)
	}
	if other.ID == first.ID {
		t.Error("another person must get their own grant")
	}

	if err := f.svc.RevokeGrant(ctx, first.ID); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}
	after, err := f.svc.SaveGrant(ctx, f.client, user, testResource, AccessSettings{})
	if err != nil {
		t.Fatalf("SaveGrant after revoke: %v", err)
	}
	if after.ID == first.ID {
		t.Error("a revoked grant must not come back to life")
	}
}

func TestSaveGrant_ConcurrentApprovalsMakeOneGrant(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	if _, err := f.db.Collection("oauth_grants").Indexes().CreateOne(ctx, database.OAuthGrantIndex()); err != nil {
		t.Fatalf("creating index: %v", err)
	}
	user := primitive.NewObjectID()

	var wg sync.WaitGroup
	ids := make(chan primitive.ObjectID, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			grant, err := f.svc.SaveGrant(ctx, f.client, user, testResource, AccessSettings{})
			if err != nil {
				t.Errorf("SaveGrant: %v", err)
				return
			}
			ids <- grant.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[primitive.ObjectID]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Errorf("%d grants for one person and client, want 1", len(seen))
	}
	if n, _ := f.db.Count(ctx, "oauth_grants", bson.M{"user_id": user, "revoked_at": nil}); n != 1 {
		t.Errorf("%d active grants stored, want 1", n)
	}

	// A revoked grant leaves the index, so the person can connect again.
	for id := range seen {
		if err := f.svc.RevokeGrant(ctx, id); err != nil {
			t.Fatalf("RevokeGrant: %v", err)
		}
	}
	if _, err := f.svc.SaveGrant(ctx, f.client, user, testResource, AccessSettings{}); err != nil {
		t.Errorf("reconnecting after a revoke: %v", err)
	}
}

func TestGenerateTokenPair_AccessTokenNeverLooksLikeAnAPIKey(t *testing.T) {
	f := newOAuthFixture(t)
	for i := 0; i < 50; i++ {
		access, _, _, err := f.svc.generateTokenPair(context.Background(), f.grant)
		if err != nil {
			t.Fatalf("generateTokenPair: %v", err)
		}
		if strings.HasPrefix(access, "lc_") {
			t.Fatalf("access token %q starts like an API key", access)
		}
	}
}

func TestValidateAccessToken(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	access, _ := f.tokens(t)

	grant, err := f.svc.ValidateAccessToken(ctx, access, testResource)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if grant.ID != f.grant.ID || grant.UserID != f.grant.UserID {
		t.Errorf("got grant %+v, want %+v", grant, f.grant)
	}
	if grant.LastUsedAt == nil {
		t.Error("expected last use to be recorded")
	}

	if _, err := f.svc.ValidateAccessToken(ctx, "nonexistent", testResource); err == nil {
		t.Error("expected error for an unknown token")
	}
	if _, err := f.svc.ValidateAccessToken(ctx, access, "https://other.example/mcp"); err == nil {
		t.Error("expected error for a token used at another resource")
	}
}

func TestValidateAccessToken_Expired(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	_, err := f.db.InsertOne(ctx, "oauth_access_tokens", bson.M{
		"token_hash": hashToken("expired"),
		"client_id":  f.client.ClientID,
		"grant_id":   f.grant.ID,
		"resource":   testResource,
		"expires_at": time.Now().Add(-time.Hour),
		"created_at": time.Now().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(ctx, "expired", testResource); err == nil {
		t.Error("expected error for an expired token")
	}
}

func TestValidateAccessToken_RefusesTokenFromBeforeGrants(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	_, err := f.db.InsertOne(ctx, "oauth_access_tokens", bson.M{
		"token_hash": hashToken("old"),
		"client_id":  f.client.ClientID,
		"resource":   testResource,
		"expires_at": time.Now().Add(time.Hour),
		"created_at": time.Now(),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(ctx, "old", testResource); err == nil {
		t.Error("a token with no grant must be refused")
	}
}

func TestExchangeAuthCode_ConcurrentExchangesGetOnePair(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	code := f.code(t)

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, _, err := f.svc.ExchangeAuthCode(ctx, code, f.client.ClientID, "http://localhost/cb", testVerifier); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if succeeded != 1 {
		t.Errorf("%d exchanges succeeded, want exactly 1", succeeded)
	}
}

func TestExchangeAuthCode_FailedAttemptDoesNotBurnTheCode(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	code := f.code(t)

	if _, _, _, err := f.svc.ExchangeAuthCode(ctx, code, f.client.ClientID, "http://localhost/cb", "wrong"); err == nil {
		t.Fatal("expected wrong verifier to fail")
	}
	if _, _, _, err := f.svc.ExchangeAuthCode(ctx, code, f.client.ClientID, "http://localhost/cb", testVerifier); err != nil {
		t.Errorf("the right verifier should still work: %v", err)
	}
}

func TestExchangeAuthCode_RefusedAfterGrantRevoked(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	code := f.code(t)
	if err := f.svc.RevokeGrant(ctx, f.grant.ID); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}
	if _, _, _, err := f.svc.ExchangeAuthCode(ctx, code, f.client.ClientID, "http://localhost/cb", testVerifier); err == nil {
		t.Error("a code under a revoked grant must not yield tokens")
	}
}

func TestRefreshAccessToken_ConcurrentRefreshesGetOnePair(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	_, refresh := f.tokens(t)

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, _, err := f.svc.RefreshAccessToken(ctx, refresh, f.client.ClientID); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if succeeded != 1 {
		t.Errorf("%d refreshes succeeded, want exactly 1", succeeded)
	}
	if grant, _ := f.svc.GetGrant(ctx, f.grant.ID); grant.RevokedAt != nil {
		t.Error("racing refreshes from one client must not end the connection")
	}
}

func TestRefreshAccessToken_ReuseWithinLeewayKeepsConnection(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	_, refresh := f.tokens(t)

	_, newRefresh, _, err := f.svc.RefreshAccessToken(ctx, refresh, f.client.ClientID)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	_, _, _, err = f.svc.RefreshAccessToken(ctx, refresh, f.client.ClientID)
	if err == nil {
		t.Fatal("expected the used token to be refused")
	}
	if errors.Is(err, ErrRefreshTokenReplayed) {
		t.Error("a retry within the leeway should not count as a replay")
	}
	if _, _, _, err := f.svc.RefreshAccessToken(ctx, newRefresh, f.client.ClientID); err != nil {
		t.Errorf("the connection should still work: %v", err)
	}
}

func TestRefreshAccessToken_ReplayAfterLeewayEndsConnection(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	_, refresh := f.tokens(t)

	newAccess, newRefresh, _, err := f.svc.RefreshAccessToken(ctx, refresh, f.client.ClientID)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	longAgo := time.Now().Add(-RefreshReuseLeeway - time.Minute)
	if err := f.db.UpdateOne(ctx, "oauth_refresh_tokens", bson.M{"token_hash": hashToken(refresh)}, bson.M{"$set": bson.M{"rotated_at": longAgo}}); err != nil {
		t.Fatalf("backdating: %v", err)
	}

	_, _, _, err = f.svc.RefreshAccessToken(ctx, refresh, f.client.ClientID)
	if !errors.Is(err, ErrRefreshTokenReplayed) {
		t.Fatalf("expected ErrRefreshTokenReplayed, got %v", err)
	}
	if _, err := f.svc.ValidateAccessToken(ctx, newAccess, testResource); err == nil {
		t.Error("the newest access token should be dead after a replay")
	}
	if _, _, _, err := f.svc.RefreshAccessToken(ctx, newRefresh, f.client.ClientID); err == nil {
		t.Error("the newest refresh token should be dead after a replay")
	}
}

func TestRefreshAccessToken_RefusesTokenFromBeforeGrants(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	_, err := f.db.InsertOne(ctx, "oauth_refresh_tokens", bson.M{
		"token_hash": hashToken("old"),
		"client_id":  f.client.ClientID,
		"resource":   testResource,
		"expires_at": time.Now().Add(time.Hour),
		"revoked":    false,
		"created_at": time.Now(),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, _, _, err := f.svc.RefreshAccessToken(ctx, "old", f.client.ClientID); err == nil {
		t.Error("a refresh token with no grant must be refused")
	}
}

func TestRevokeToken(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()

	t.Run("another client's token is ignored", func(t *testing.T) {
		access, refresh := f.tokens(t)
		if err := f.svc.RevokeToken(ctx, "someone-else", refresh); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
		if err := f.svc.RevokeToken(ctx, "someone-else", access); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
		if _, err := f.svc.ValidateAccessToken(ctx, access, testResource); err != nil {
			t.Errorf("token should be untouched: %v", err)
		}
	})

	t.Run("an access token ends alone", func(t *testing.T) {
		access, refresh := f.tokens(t)
		if err := f.svc.RevokeToken(ctx, f.client.ClientID, access); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
		if _, err := f.svc.ValidateAccessToken(ctx, access, testResource); err == nil {
			t.Error("revoked access token still works")
		}
		if _, _, _, err := f.svc.RefreshAccessToken(ctx, refresh, f.client.ClientID); err != nil {
			t.Errorf("the connection should survive: %v", err)
		}
	})

	t.Run("a refresh token ends the connection", func(t *testing.T) {
		access, refresh := f.tokens(t)
		if err := f.svc.RevokeToken(ctx, f.client.ClientID, refresh); err != nil {
			t.Fatalf("RevokeToken: %v", err)
		}
		if _, err := f.svc.ValidateAccessToken(ctx, access, testResource); err == nil {
			t.Error("access token survived its connection")
		}
		grant, err := f.svc.GetGrant(ctx, f.grant.ID)
		if err != nil || grant.RevokedAt == nil {
			t.Errorf("grant not revoked: %+v, %v", grant, err)
		}
	})
}

func TestValidateRedirectURI(t *testing.T) {
	allowed := []string{
		"https://claude.ai/api/mcp/auth_callback",
		"http://localhost:3334/callback",
		"http://127.0.0.1:53682/cb",
		"http://[::1]:53682/cb",
		"HTTPS://app.example/cb",
	}
	for _, uri := range allowed {
		if err := ValidateRedirectURI(uri); err != nil {
			t.Errorf("%q should be allowed: %v", uri, err)
		}
	}
	refused := []string{
		"",
		"http://example.com/cb",
		"http://localhost.example.com/cb",
		"javascript:alert(1)",
		"data:text/html,x",
		"cursor://callback",
		"/relative",
		"https://app.example/cb#fragment",
		"https://user:pass@app.example/cb",
		"https://app.example/" + strings.Repeat("a", maxRedirectURILength),
	}
	for _, uri := range refused {
		if err := ValidateRedirectURI(uri); err == nil {
			t.Errorf("%q should be refused", uri)
		}
	}
}

func TestRegisterClient_RefusesBadRedirect(t *testing.T) {
	f := newOAuthFixture(t)
	if _, _, err := f.svc.RegisterClient(context.Background(), "X", []string{"http://evil.example/cb"}); err == nil {
		t.Error("expected refusal")
	}
}

func TestValidateClient_HoldsExistingClientsToTheRules(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	if _, err := f.db.InsertOne(ctx, "oauth_clients", &models.OAuthClient{
		ClientID:     "old-client",
		ClientName:   "Registered before the rules",
		RedirectURIs: []string{"http://evil.example/cb"},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := f.svc.ValidateClient(ctx, "old-client", "http://evil.example/cb"); err == nil {
		t.Error("expected refusal")
	}
}

func TestRemoveLegacySystemKey(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	ctx := context.Background()
	keys := NewAPIKeyService(db)

	// Nothing to remove
	if removed, err := RemoveLegacySystemKey(ctx, db); err != nil || removed {
		t.Fatalf("empty database: removed=%v err=%v", removed, err)
	}

	rawKey, _, err := keys.CreateAPIKeyForUser(ctx, "Renamed by someone", "Internal key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKeyForUser: %v", err)
	}
	if _, err := db.Settings().InsertOne(ctx, bson.M{"type": "system_api_key", "raw_key": rawKey}); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	_, keep, err := keys.CreateAPIKeyForUser(ctx, "A key of mine", "", nil)
	if err != nil {
		t.Fatalf("CreateAPIKeyForUser: %v", err)
	}
	if _, _, err := keys.CreateAPIKeyForUser(ctx, "System (OAuth)", "Internal key for OAuth-authenticated MCP sessions", nil); err != nil {
		t.Fatalf("CreateAPIKeyForUser: %v", err)
	}

	removed, err := RemoveLegacySystemKey(ctx, db)
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, err := keys.ValidateAPIKey(ctx, rawKey); err == nil {
		t.Error("the system key still works")
	}
	if n, _ := db.Count(ctx, "settings", bson.M{"type": "system_api_key"}); n != 0 {
		t.Error("the stored raw key is still there")
	}
	remaining, err := keys.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != keep.ID {
		t.Errorf("unrelated keys must survive, got %+v", remaining)
	}

	// Safe to run on every boot
	if removed, err := RemoveLegacySystemKey(ctx, db); err != nil || removed {
		t.Errorf("second run: removed=%v err=%v", removed, err)
	}
}

func TestAssignOwner_NeverChangesAnExistingOwner(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	ctx := context.Background()
	keys := NewAPIKeyService(db)

	owner := primitive.NewObjectID()
	_, key, err := keys.CreateAPIKeyForUser(ctx, "Mine", "", &owner)
	if err != nil {
		t.Fatalf("CreateAPIKeyForUser: %v", err)
	}
	if err := keys.AssignOwner(ctx, key.ID, primitive.NewObjectID()); err != nil {
		t.Fatalf("AssignOwner: %v", err)
	}
	list, _ := keys.ListAPIKeysForUser(ctx, owner)
	if len(list) != 1 {
		t.Error("the key's owner changed")
	}
}
