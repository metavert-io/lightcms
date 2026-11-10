package auth

import (
	"context"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"
	"github.com/metavert-io/lightcms/v7/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const credentialTestResource = "https://example.com/mcp"

type credentialFixture struct {
	db       *database.DB
	users    *services.UserService
	keys     *services.APIKeyService
	oauth    *services.OAuthService
	resolver *CredentialResolver
}

func newCredentialFixture(t *testing.T) *credentialFixture {
	t.Helper()
	db, cleanup := testutil.MustConnectTestDB(t)
	t.Cleanup(cleanup)
	f := &credentialFixture{
		db:    db,
		users: services.NewUserService(db),
		keys:  services.NewAPIKeyService(db),
		oauth: services.NewOAuthService(db),
	}
	f.resolver = NewCredentialResolver(f.users, f.keys, f.oauth, credentialTestResource, nil)
	return f
}

func (f *credentialFixture) user(t *testing.T, email, role string) *models.User {
	t.Helper()
	user, err := f.users.CreateUserWithHash(context.Background(), email, email, role, "unused", false)
	if err != nil {
		t.Fatalf("CreateUserWithHash: %v", err)
	}
	return user
}

// accessToken connects a client on behalf of user and returns the access token.
func (f *credentialFixture) accessToken(t *testing.T, user *models.User) (string, *models.OAuthGrant) {
	t.Helper()
	ctx := context.Background()
	client, _, err := f.oauth.RegisterClient(ctx, "Test App", []string{"http://localhost/cb"})
	if err != nil {
		t.Fatalf("RegisterClient: %v", err)
	}
	grant, err := f.oauth.SaveGrant(ctx, client, user.ID, credentialTestResource)
	if err != nil {
		t.Fatalf("SaveGrant: %v", err)
	}
	// The challenge is the S256 of this verifier (RFC 7636 appendix B).
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	code, err := f.oauth.CreateAuthCode(ctx, grant, "http://localhost/cb", "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	access, _, _, err := f.oauth.ExchangeAuthCode(ctx, code, client.ClientID, "http://localhost/cb", verifier)
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}
	return access, grant
}

func caller(t *testing.T, v interface{}, err error) *SessionUser {
	t.Helper()
	if err != nil {
		t.Fatalf("resolving credential: %v", err)
	}
	return v.(*SessionUser)
}

func TestResolveAPIKey_ActsAsItsOwnerWithItsRestrictions(t *testing.T) {
	f := newCredentialFixture(t)
	owner := f.user(t, "owner@example.com", models.RoleEditor)
	raw, key, err := f.keys.CreateScopedAPIKey(context.Background(), "Agent", "", &owner.ID, []string{PermContentView}, true)
	if err != nil {
		t.Fatalf("CreateScopedAPIKey: %v", err)
	}

	v, err := f.resolver.ResolveAPIKey(context.Background(), raw)
	got := caller(t, v, err)
	if got.ID != owner.ID.Hex() || got.Role != models.RoleEditor || !got.ViaAPIKey || got.ViaOAuth {
		t.Errorf("unexpected caller: %+v", got)
	}
	if !got.SandboxOnly || len(got.Scopes) != 1 {
		t.Errorf("key restrictions were dropped: %+v", got)
	}
	if got.CredentialKey() != "apikey:"+key.ID.Hex() {
		t.Errorf("credential = %q", got.CredentialKey())
	}
}

func TestResolveAPIKey_RefusesWhenOwnerIsDisabled(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	owner := f.user(t, "owner@example.com", models.RoleAdmin)
	raw, _, _ := f.keys.CreateAPIKeyForUser(ctx, "Key", "", &owner.ID)
	if err := f.users.DisableUser(ctx, owner.ID, true); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if _, err := f.resolver.ResolveAPIKey(ctx, raw); err == nil {
		t.Error("a disabled owner's key must not work")
	}
}

func TestResolveAPIKey_RefusesWhenOwnerIsGoneAndDoesNotPromote(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	f.user(t, "admin@example.com", models.RoleAdmin)
	missing := primitive.NewObjectID()
	raw, key, _ := f.keys.CreateAPIKeyForUser(ctx, "Key", "", &missing)

	if _, err := f.resolver.ResolveAPIKey(ctx, raw); err == nil {
		t.Fatal("a key whose owner is missing must not work")
	}
	var stored models.APIKey
	if err := f.db.FindOne(ctx, "api_keys", bson.M{"_id": key.ID}, &stored); err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if stored.UserID == nil || *stored.UserID != missing {
		t.Errorf("the key was reassigned to %v", stored.UserID)
	}
}

func TestResolveAPIKey_RefusesWhenOwnerCannotBeLoaded(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	admin := f.user(t, "admin@example.com", models.RoleAdmin)
	raw, _, _ := f.keys.CreateAPIKeyForUser(ctx, "Key", "", &admin.ID)

	// Keys come from a working database, users from a dead one.
	broken := services.NewUserService(testutil.MustConnectBrokenDB(t))
	resolver := NewCredentialResolver(broken, f.keys, f.oauth, credentialTestResource, nil)
	if _, err := resolver.ResolveAPIKey(ctx, raw); err == nil {
		t.Error("a database error must refuse the key")
	}
}

func TestResolveAPIKey_OwnerlessKeyIsAdoptedAndKeepsRestrictions(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	f.user(t, "viewer@example.com", models.RoleViewer)
	disabled := f.user(t, "former@example.com", models.RoleAdmin)
	if err := f.users.DisableUser(ctx, disabled.ID, true); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	admin := f.user(t, "admin@example.com", models.RoleAdmin)
	raw, key, err := f.keys.CreateScopedAPIKey(ctx, "Old", "", nil, []string{PermContentView}, true)
	if err != nil {
		t.Fatalf("CreateScopedAPIKey: %v", err)
	}

	v, err := f.resolver.ResolveAPIKey(ctx, raw)
	got := caller(t, v, err)
	if got.ID != admin.ID.Hex() {
		t.Errorf("adopted by %s, want the active admin", got.Email)
	}
	if !got.SandboxOnly || len(got.Scopes) != 1 {
		t.Errorf("restrictions were dropped: %+v", got)
	}
	var stored models.APIKey
	if err := f.db.FindOne(ctx, "api_keys", bson.M{"_id": key.ID}, &stored); err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if stored.UserID == nil || *stored.UserID != admin.ID {
		t.Errorf("owner on record = %v", stored.UserID)
	}
}

func TestResolveAPIKey_OwnerlessKeyRefusedWithNoAdmin(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	f.user(t, "viewer@example.com", models.RoleViewer)
	raw, _, _ := f.keys.CreateAPIKey(ctx, "Old", "")
	if _, err := f.resolver.ResolveAPIKey(ctx, raw); err == nil {
		t.Error("expected refusal")
	}
}

func TestResolveOAuthToken_ActsAsThePersonWhoConnected(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	f.user(t, "admin@example.com", models.RoleAdmin)
	viewer := f.user(t, "viewer@example.com", models.RoleViewer)
	token, grant := f.accessToken(t, viewer)

	v, err := f.resolver.ResolveOAuthToken(ctx, token)
	got := caller(t, v, err)
	if got.ID != viewer.ID.Hex() || got.Role != models.RoleViewer {
		t.Fatalf("token acts as %s (%s), want the viewer", got.Email, got.Role)
	}
	if !got.ViaOAuth || got.ViaAPIKey {
		t.Errorf("unexpected credential type: %+v", got)
	}
	if got.CredentialKey() != "oauth:"+grant.ID.Hex() {
		t.Errorf("credential = %q", got.CredentialKey())
	}
	if UserHasPermission(got, PermContentEdit) {
		t.Error("a viewer's token must not be able to edit")
	}
}

func TestResolveOAuthToken_FollowsRoleChangesAtOnce(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	user := f.user(t, "person@example.com", models.RoleAdmin)
	token, _ := f.accessToken(t, user)

	if err := f.users.UpdateUser(ctx, user.ID, "person", models.RoleViewer); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	v, err := f.resolver.ResolveOAuthToken(ctx, token)
	if got := caller(t, v, err); got.Role != models.RoleViewer {
		t.Errorf("role = %s, want viewer", got.Role)
	}
}

func TestResolveOAuthToken_RefusesDisabledMissingOrRevoked(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()

	disabled := f.user(t, "leaver@example.com", models.RoleAdmin)
	token, _ := f.accessToken(t, disabled)
	if err := f.users.DisableUser(ctx, disabled.ID, true); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if _, err := f.resolver.ResolveOAuthToken(ctx, token); err == nil {
		t.Error("a disabled person's token must not work")
	}

	revoked := f.user(t, "revoked@example.com", models.RoleAdmin)
	token, grant := f.accessToken(t, revoked)
	if err := f.oauth.RevokeGrant(ctx, grant.ID); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}
	if _, err := f.resolver.ResolveOAuthToken(ctx, token); err == nil {
		t.Error("a revoked connection's token must not work")
	}

	gone := &models.User{ID: primitive.NewObjectID()}
	token, _ = f.accessToken(t, gone)
	if _, err := f.resolver.ResolveOAuthToken(ctx, token); err == nil {
		t.Error("a token for a missing person must not work")
	}
}

func TestResolveOAuthToken_RefusesTokenForAnotherResource(t *testing.T) {
	f := newCredentialFixture(t)
	user := f.user(t, "person@example.com", models.RoleAdmin)
	token, _ := f.accessToken(t, user)

	elsewhere := NewCredentialResolver(f.users, f.keys, f.oauth, "https://other.example/mcp", nil)
	if _, err := elsewhere.ResolveOAuthToken(context.Background(), token); err == nil {
		t.Error("expected refusal")
	}
}

func TestCredentialResolver_ReportsActivity(t *testing.T) {
	f := newCredentialFixture(t)
	ctx := context.Background()
	user := f.user(t, "person@example.com", models.RoleEditor)
	raw, _, _ := f.keys.CreateAPIKeyForUser(ctx, "Key", "", &user.ID)

	var seen string
	resolver := NewCredentialResolver(f.users, f.keys, f.oauth, credentialTestResource, func(id string) { seen = id })
	if _, err := resolver.ResolveAPIKey(ctx, raw); err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if seen != user.ID.Hex() {
		t.Errorf("activity recorded for %q", seen)
	}
}
