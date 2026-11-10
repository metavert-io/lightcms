package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// OAuthClient represents a dynamically registered OAuth client (RFC 7591)
type OAuthClient struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ClientID         string             `bson:"client_id" json:"client_id"`
	ClientSecretHash string             `bson:"client_secret_hash" json:"-"`
	ClientName       string             `bson:"client_name" json:"client_name"`
	RedirectURIs     []string           `bson:"redirect_uris" json:"redirect_uris"`
	GrantTypes       []string           `bson:"grant_types" json:"grant_types"`
	CreatedAt        time.Time          `bson:"created_at" json:"created_at"`
}

// OAuthGrant is one person's consent for one client: the "connection". Every
// authorization code and token belongs to a grant, and the person it names is
// who the connection acts as. Revoking the grant ends the connection and
// every token issued under it.
type OAuthGrant struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ClientID   string             `bson:"client_id" json:"client_id"`
	ClientName string             `bson:"client_name" json:"client_name"` // as registered, for display; chosen by the client
	UserID     primitive.ObjectID `bson:"user_id" json:"user_id"`
	Resource   string             `bson:"resource" json:"resource"`
	// SandboxOnly confines the connection to forks it created; nothing it
	// does changes the live site unless CanMergeOwnForks lets it merge them.
	SandboxOnly bool `bson:"sandbox_only" json:"sandbox_only"`
	// CanMergeOwnForks lets the connection merge forks it created into the
	// live site. Only a signed-in admin can turn it on.
	CanMergeOwnForks bool `bson:"can_merge_own_forks" json:"can_merge_own_forks"`
	// ActiveSandboxFork is the fork the connection's current sandbox writes to.
	ActiveSandboxFork *primitive.ObjectID `bson:"active_sandbox_fork,omitempty" json:"active_sandbox_fork,omitempty"`
	CreatedAt         time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time           `bson:"updated_at" json:"updated_at"`
	LastUsedAt        *time.Time          `bson:"last_used_at,omitempty" json:"last_used_at,omitempty"`
	RevokedAt         *time.Time          `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
}

// OAuthAuthCode represents a pending authorization code
type OAuthAuthCode struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"`
	CodeHash            string             `bson:"code" json:"-"`
	ClientID            string             `bson:"client_id"`
	GrantID             primitive.ObjectID `bson:"grant_id,omitempty"`
	RedirectURI         string             `bson:"redirect_uri"`
	CodeChallenge       string             `bson:"code_challenge"`
	CodeChallengeMethod string             `bson:"code_challenge_method"`
	Resource            string             `bson:"resource"`
	ExpiresAt           time.Time          `bson:"expires_at"`
	Used                bool               `bson:"used"`
	CreatedAt           time.Time          `bson:"created_at"`
}

// OAuthAccessToken represents an issued access token
type OAuthAccessToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TokenHash string             `bson:"token_hash"`
	ClientID  string             `bson:"client_id"`
	GrantID   primitive.ObjectID `bson:"grant_id,omitempty"`
	Resource  string             `bson:"resource"`
	ExpiresAt time.Time          `bson:"expires_at"`
	CreatedAt time.Time          `bson:"created_at"`
}

// OAuthRefreshToken represents an issued refresh token
type OAuthRefreshToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TokenHash string             `bson:"token_hash"`
	ClientID  string             `bson:"client_id"`
	GrantID   primitive.ObjectID `bson:"grant_id,omitempty"`
	Resource  string             `bson:"resource"`
	ExpiresAt time.Time          `bson:"expires_at"`
	Revoked   bool               `bson:"revoked"`
	RotatedAt *time.Time         `bson:"rotated_at,omitempty"` // when this token was exchanged for a new one
	CreatedAt time.Time          `bson:"created_at"`
}
