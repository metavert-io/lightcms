package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	AccessTokenTTL  = 1 * time.Hour
	RefreshTokenTTL = 30 * 24 * time.Hour
	AuthCodeTTL     = 10 * time.Minute

	// RefreshReuseLeeway is how long after a refresh token is rotated a second
	// use is refused without ending the connection. Two requests from one
	// client racing to refresh are not an attack; a used token that comes back
	// later is.
	RefreshReuseLeeway = 10 * time.Second

	// grantTouchInterval limits how often a grant's last-used time is written.
	grantTouchInterval = time.Minute

	maxRedirectURILength = 2048
)

// ErrRefreshTokenReplayed reports that an already-used refresh token was
// presented, and the connection it belonged to has been revoked.
var ErrRefreshTokenReplayed = errors.New("refresh token already used; the connection has been revoked")

// MCPResourceURL returns this server's canonical OAuth resource identifier
// (RFC 8707): the one URL its access tokens are good for.
func MCPResourceURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/mcp"
}

// ValidateRedirectURI enforces the redirect rules for OAuth clients: https, or
// http only to a loopback address, where a native app listens on a local port
// (RFC 8252 section 7.3). No fragments and no embedded credentials.
func ValidateRedirectURI(raw string) error {
	if raw == "" || len(raw) > maxRedirectURILength {
		return fmt.Errorf("redirect_uri must be between 1 and %d characters", maxRedirectURILength)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("redirect_uri must be an absolute URL")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return fmt.Errorf("redirect_uri must not contain a fragment")
	}
	if u.User != nil {
		return fmt.Errorf("redirect_uri must not contain credentials")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("redirect_uri must use https unless it points at this device (localhost)")
	default:
		return fmt.Errorf("redirect_uri must use https, or http to localhost")
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// OAuthService handles OAuth 2.1 authorization
type OAuthService struct {
	db *database.DB
}

// NewOAuthService creates a new OAuth service
func NewOAuthService(db *database.DB) *OAuthService {
	return &OAuthService{db: db}
}

// RegisterClient creates a new OAuth client via Dynamic Client Registration (RFC 7591)
func (s *OAuthService) RegisterClient(ctx context.Context, clientName string, redirectURIs []string) (*models.OAuthClient, string, error) {
	if clientName == "" {
		return nil, "", fmt.Errorf("client_name is required")
	}
	if len(redirectURIs) == 0 {
		return nil, "", fmt.Errorf("redirect_uris is required")
	}
	for _, uri := range redirectURIs {
		if err := ValidateRedirectURI(uri); err != nil {
			return nil, "", err
		}
	}

	clientID, err := generateToken(16)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate client_id: %w", err)
	}

	rawSecret, err := generateToken(24)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate client_secret: %w", err)
	}

	now := time.Now()
	client := &models.OAuthClient{
		ClientID:         clientID,
		ClientSecretHash: hashToken(rawSecret),
		ClientName:       clientName,
		RedirectURIs:     redirectURIs,
		GrantTypes:       []string{"authorization_code", "refresh_token"},
		CreatedAt:        now,
	}

	id, err := s.db.InsertOne(ctx, "oauth_clients", client)
	if err != nil {
		return nil, "", fmt.Errorf("failed to register client: %w", err)
	}
	client.ID = id

	return client, rawSecret, nil
}

// ValidateClient looks up a client by ID and validates the redirect URI
func (s *OAuthService) ValidateClient(ctx context.Context, clientID, redirectURI string) (*models.OAuthClient, error) {
	var client models.OAuthClient
	err := s.db.FindOne(ctx, "oauth_clients", bson.M{"client_id": clientID}, &client)
	if err != nil {
		return nil, fmt.Errorf("unknown client_id")
	}

	// An authorization request always names its redirect_uri: an empty one
	// would skip the comparison with the registered list.
	if redirectURI == "" {
		return nil, fmt.Errorf("redirect_uri is required")
	}
	found := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("redirect_uri not registered")
	}
	// Clients registered before these rules existed are held to them too.
	if err := ValidateRedirectURI(redirectURI); err != nil {
		return nil, err
	}

	return &client, nil
}

// ValidateClientCredentials validates client_id and client_secret
func (s *OAuthService) ValidateClientCredentials(ctx context.Context, clientID, clientSecret string) (*models.OAuthClient, error) {
	var client models.OAuthClient
	err := s.db.FindOne(ctx, "oauth_clients", bson.M{"client_id": clientID}, &client)
	if err != nil {
		return nil, fmt.Errorf("unknown client_id")
	}

	if client.ClientSecretHash != hashToken(clientSecret) {
		return nil, fmt.Errorf("invalid client_secret")
	}

	return &client, nil
}

// SaveGrant records that a person approved a client with the given access,
// and returns the grant. One person has one active grant per client:
// approving again reuses it with the access chosen this time.
func (s *OAuthService) SaveGrant(ctx context.Context, client *models.OAuthClient, userID primitive.ObjectID, resource string, access AccessSettings) (*models.OAuthGrant, error) {
	now := time.Now()
	var grant models.OAuthGrant
	save := func() error {
		return s.db.Collection("oauth_grants").FindOneAndUpdate(ctx,
			bson.M{"client_id": client.ClientID, "user_id": userID, "revoked_at": nil},
			bson.M{
				"$set": bson.M{
					"client_name":         client.ClientName,
					"resource":            resource,
					"sandbox_only":        access.SandboxOnly,
					"can_merge_own_forks": access.CanMergeOwnForks,
					"updated_at":          now,
				},
				"$setOnInsert": bson.M{"created_at": now},
			},
			options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
		).Decode(&grant)
	}
	err := save()
	if mongo.IsDuplicateKeyError(err) {
		// Another approval inserted the grant first; update that one.
		err = save()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to save grant: %w", err)
	}
	return &grant, nil
}

// GetGrant returns a grant by ID, revoked or not.
func (s *OAuthService) GetGrant(ctx context.Context, id primitive.ObjectID) (*models.OAuthGrant, error) {
	var grant models.OAuthGrant
	if err := s.db.FindOne(ctx, "oauth_grants", bson.M{"_id": id}, &grant); err != nil {
		return nil, fmt.Errorf("grant not found: %w", err)
	}
	return &grant, nil
}

// ListGrants returns the active connections, newest first: one person's when
// userID is set, everyone's otherwise.
func (s *OAuthService) ListGrants(ctx context.Context, userID *primitive.ObjectID) ([]models.OAuthGrant, error) {
	filter := bson.M{"revoked_at": nil}
	if userID != nil {
		filter["user_id"] = *userID
	}
	cursor, err := s.db.FindMany(ctx, "oauth_grants", filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer cursor.Close(ctx)
	grants := []models.OAuthGrant{}
	if err := cursor.All(ctx, &grants); err != nil {
		return nil, fmt.Errorf("failed to decode connections: %w", err)
	}
	return grants, nil
}

// UpdateGrantAccess changes an active connection's access settings.
func (s *OAuthService) UpdateGrantAccess(ctx context.Context, id primitive.ObjectID, access AccessSettings) error {
	return s.db.UpdateOne(ctx, "oauth_grants", bson.M{"_id": id, "revoked_at": nil}, bson.M{"$set": bson.M{
		"sandbox_only":        access.SandboxOnly,
		"can_merge_own_forks": access.CanMergeOwnForks,
		"updated_at":          time.Now(),
	}})
}

// SetActiveSandbox records the fork a connection's sandbox writes to (nil: none).
func (s *OAuthService) SetActiveSandbox(ctx context.Context, id primitive.ObjectID, fork *primitive.ObjectID) error {
	if fork == nil {
		return s.db.UpdateOne(ctx, "oauth_grants", bson.M{"_id": id}, bson.M{"$unset": bson.M{"active_sandbox_fork": ""}})
	}
	return s.db.UpdateOne(ctx, "oauth_grants", bson.M{"_id": id}, bson.M{"$set": bson.M{"active_sandbox_fork": *fork}})
}

// activeGrant returns a grant that has not been revoked.
func (s *OAuthService) activeGrant(ctx context.Context, id primitive.ObjectID) (*models.OAuthGrant, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("issued before connections were tied to a person; reconnect the app")
	}
	grant, err := s.GetGrant(ctx, id)
	if err != nil {
		return nil, err
	}
	if grant.RevokedAt != nil {
		return nil, fmt.Errorf("connection revoked")
	}
	return grant, nil
}

// RevokeGrant ends a connection: the grant is marked revoked, which alone
// invalidates everything issued under it, and its tokens are then removed.
func (s *OAuthService) RevokeGrant(ctx context.Context, id primitive.ObjectID) error {
	now := time.Now()
	if err := s.db.UpdateOne(ctx, "oauth_grants",
		bson.M{"_id": id, "revoked_at": nil},
		bson.M{"$set": bson.M{"revoked_at": now, "updated_at": now}}); err != nil {
		return fmt.Errorf("failed to revoke grant: %w", err)
	}
	if _, err := s.db.Collection("oauth_refresh_tokens").UpdateMany(ctx,
		bson.M{"grant_id": id}, bson.M{"$set": bson.M{"revoked": true}}); err != nil {
		log.Printf("[OAuth] grant %s revoked; marking refresh tokens failed: %v", id.Hex(), err)
	}
	if _, err := s.db.DeleteMany(ctx, "oauth_access_tokens", bson.M{"grant_id": id}); err != nil {
		log.Printf("[OAuth] grant %s revoked; removing access tokens failed: %v", id.Hex(), err)
	}
	return nil
}

// RevokeToken implements RFC 7009 for one client. Revoking a refresh token ends
// its connection; revoking an access token ends that token alone. A token that
// is unknown or belongs to another client is ignored, as the RFC requires.
func (s *OAuthService) RevokeToken(ctx context.Context, clientID, rawToken string) error {
	tokenHash := hashToken(rawToken)

	var refresh models.OAuthRefreshToken
	err := s.db.FindOne(ctx, "oauth_refresh_tokens", bson.M{"token_hash": tokenHash}, &refresh)
	switch {
	case err == nil:
		if refresh.ClientID != clientID {
			return nil
		}
		if refresh.GrantID.IsZero() {
			return s.db.UpdateOne(ctx, "oauth_refresh_tokens", bson.M{"_id": refresh.ID}, bson.M{"$set": bson.M{"revoked": true}})
		}
		return s.RevokeGrant(ctx, refresh.GrantID)
	case !errors.Is(err, mongo.ErrNoDocuments):
		return fmt.Errorf("looking up refresh token: %w", err)
	}

	_, err = s.db.DeleteMany(ctx, "oauth_access_tokens", bson.M{"token_hash": tokenHash, "client_id": clientID})
	if err != nil {
		return fmt.Errorf("revoking access token: %w", err)
	}
	return nil
}

// CreateAuthCode generates an authorization code under the given grant.
func (s *OAuthService) CreateAuthCode(ctx context.Context, grant *models.OAuthGrant, redirectURI, codeChallenge, codeChallengeMethod string) (string, error) {
	rawCode, err := generateToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate auth code: %w", err)
	}

	now := time.Now()
	authCode := &models.OAuthAuthCode{
		CodeHash:            hashToken(rawCode),
		ClientID:            grant.ClientID,
		GrantID:             grant.ID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Resource:            grant.Resource,
		ExpiresAt:           now.Add(AuthCodeTTL),
		Used:                false,
		CreatedAt:           now,
	}

	if _, err := s.db.InsertOne(ctx, "oauth_auth_codes", authCode); err != nil {
		return "", fmt.Errorf("failed to store auth code: %w", err)
	}

	return rawCode, nil
}

// claim atomically applies update to the one document filter matches, and
// reports whether this call was the one that changed it.
func (s *OAuthService) claim(ctx context.Context, collection string, filter, update bson.M) (bool, error) {
	result, err := s.db.Collection(collection).UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}

// ExchangeAuthCode validates an auth code and returns access + refresh tokens.
// The code is consumed atomically, after every check has passed: of two
// concurrent exchanges exactly one gets tokens, and a failed attempt does not
// burn the code.
func (s *OAuthService) ExchangeAuthCode(ctx context.Context, rawCode, clientID, redirectURI, codeVerifier string) (accessToken, refreshToken string, expiresIn int, err error) {
	codeHash := hashToken(rawCode)

	var authCode models.OAuthAuthCode
	if err := s.db.FindOne(ctx, "oauth_auth_codes", bson.M{"code": codeHash}, &authCode); err != nil {
		return "", "", 0, fmt.Errorf("invalid authorization code")
	}

	// Validate code
	if authCode.Used {
		return "", "", 0, fmt.Errorf("authorization code already used")
	}
	if time.Now().After(authCode.ExpiresAt) {
		return "", "", 0, fmt.Errorf("authorization code expired")
	}
	if authCode.ClientID != clientID {
		return "", "", 0, fmt.Errorf("client_id mismatch")
	}
	if authCode.RedirectURI != redirectURI {
		return "", "", 0, fmt.Errorf("redirect_uri mismatch")
	}

	// Verify PKCE
	if !verifyPKCE(codeVerifier, authCode.CodeChallenge, authCode.CodeChallengeMethod) {
		return "", "", 0, fmt.Errorf("PKCE verification failed")
	}

	grant, err := s.activeGrant(ctx, authCode.GrantID)
	if err != nil {
		return "", "", 0, fmt.Errorf("authorization code: %w", err)
	}

	claimed, err := s.claim(ctx, "oauth_auth_codes",
		bson.M{"_id": authCode.ID, "used": false},
		bson.M{"$set": bson.M{"used": true}})
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to consume authorization code: %w", err)
	}
	if !claimed {
		return "", "", 0, fmt.Errorf("authorization code already used")
	}

	return s.generateTokenPair(ctx, grant)
}

// RefreshAccessToken rotates a refresh token and issues new tokens. Rotation is
// atomic, so one token yields one new pair however many requests race for it.
// A used token that comes back after RefreshReuseLeeway means two parties hold
// it, so the connection is revoked and ErrRefreshTokenReplayed returned.
func (s *OAuthService) RefreshAccessToken(ctx context.Context, rawRefreshToken, clientID string) (newAccessToken, newRefreshToken string, expiresIn int, err error) {
	tokenHash := hashToken(rawRefreshToken)

	var refreshToken models.OAuthRefreshToken
	if err := s.db.FindOne(ctx, "oauth_refresh_tokens", bson.M{"token_hash": tokenHash}, &refreshToken); err != nil {
		return "", "", 0, fmt.Errorf("invalid refresh token")
	}

	if refreshToken.ClientID != clientID {
		return "", "", 0, fmt.Errorf("client_id mismatch")
	}
	if refreshToken.Revoked {
		if refreshToken.RotatedAt != nil && time.Since(*refreshToken.RotatedAt) > RefreshReuseLeeway && !refreshToken.GrantID.IsZero() {
			log.Printf("[OAuth] used refresh token replayed; revoking grant %s", refreshToken.GrantID.Hex())
			if err := s.RevokeGrant(ctx, refreshToken.GrantID); err != nil {
				log.Printf("[OAuth] revoking grant %s after replay failed: %v", refreshToken.GrantID.Hex(), err)
			}
			return "", "", 0, ErrRefreshTokenReplayed
		}
		return "", "", 0, fmt.Errorf("refresh token revoked")
	}
	if time.Now().After(refreshToken.ExpiresAt) {
		return "", "", 0, fmt.Errorf("refresh token expired")
	}

	grant, err := s.activeGrant(ctx, refreshToken.GrantID)
	if err != nil {
		return "", "", 0, fmt.Errorf("refresh token: %w", err)
	}

	now := time.Now()
	claimed, err := s.claim(ctx, "oauth_refresh_tokens",
		bson.M{"_id": refreshToken.ID, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true, "rotated_at": now}})
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to rotate refresh token: %w", err)
	}
	if !claimed {
		return "", "", 0, fmt.Errorf("refresh token revoked")
	}

	return s.generateTokenPair(ctx, grant)
}

// ValidateAccessToken checks a raw access token, for use at the given resource,
// and returns the live grant it belongs to. A token issued before connections
// were tied to a person has no grant and is refused.
func (s *OAuthService) ValidateAccessToken(ctx context.Context, rawToken, resource string) (*models.OAuthGrant, error) {
	tokenHash := hashToken(rawToken)

	var token models.OAuthAccessToken
	if err := s.db.FindOne(ctx, "oauth_access_tokens", bson.M{"token_hash": tokenHash}, &token); err != nil {
		return nil, fmt.Errorf("invalid access token")
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, fmt.Errorf("access token expired")
	}
	if token.Resource != resource {
		return nil, fmt.Errorf("access token was not issued for this resource")
	}

	grant, err := s.activeGrant(ctx, token.GrantID)
	if err != nil {
		return nil, fmt.Errorf("access token: %w", err)
	}

	if grant.LastUsedAt == nil || time.Since(*grant.LastUsedAt) > grantTouchInterval {
		now := time.Now()
		if err := s.db.UpdateOne(ctx, "oauth_grants", bson.M{"_id": grant.ID}, bson.M{"$set": bson.M{"last_used_at": now}}); err != nil {
			log.Printf("[OAuth] recording last use of grant %s failed: %v", grant.ID.Hex(), err)
		}
		grant.LastUsedAt = &now
	}

	return grant, nil
}

// generateTokenPair creates and stores a new access token + refresh token
func (s *OAuthService) generateTokenPair(ctx context.Context, grant *models.OAuthGrant) (string, string, int, error) {
	rawAccess, err := generateToken(48)
	// Bearer tokens that start with "lc_" are taken to be API keys.
	for err == nil && strings.HasPrefix(rawAccess, "lc_") {
		rawAccess, err = generateToken(48)
	}
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to generate access token: %w", err)
	}

	rawRefresh, err := generateToken(64)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	now := time.Now()

	accessToken := &models.OAuthAccessToken{
		TokenHash: hashToken(rawAccess),
		ClientID:  grant.ClientID,
		GrantID:   grant.ID,
		Resource:  grant.Resource,
		ExpiresAt: now.Add(AccessTokenTTL),
		CreatedAt: now,
	}

	refreshToken := &models.OAuthRefreshToken{
		TokenHash: hashToken(rawRefresh),
		ClientID:  grant.ClientID,
		GrantID:   grant.ID,
		Resource:  grant.Resource,
		ExpiresAt: now.Add(RefreshTokenTTL),
		Revoked:   false,
		CreatedAt: now,
	}

	if _, err := s.db.InsertOne(ctx, "oauth_access_tokens", accessToken); err != nil {
		return "", "", 0, fmt.Errorf("failed to store access token: %w", err)
	}

	if _, err := s.db.InsertOne(ctx, "oauth_refresh_tokens", refreshToken); err != nil {
		// Clean up the access token we just created
		_, _ = s.db.DeleteMany(ctx, "oauth_access_tokens", bson.M{"token_hash": accessToken.TokenHash})
		return "", "", 0, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return rawAccess, rawRefresh, int(AccessTokenTTL.Seconds()), nil
}

// DeleteClientByID removes a client (used in cleanup/admin)
func (s *OAuthService) DeleteClientByID(ctx context.Context, id primitive.ObjectID) error {
	return s.db.DeleteOne(ctx, "oauth_clients", bson.M{"_id": id})
}

// hashToken returns the SHA-256 hex digest of a raw token
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// generateToken returns a base64url-encoded random token of nBytes length
func generateToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// verifyPKCE verifies the PKCE code_verifier against the stored code_challenge
func verifyPKCE(codeVerifier, codeChallenge, method string) bool {
	if method != "S256" {
		return false
	}
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == codeChallenge
}
