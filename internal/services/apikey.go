package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// APIKeyService handles API key management
type APIKeyService struct {
	db *database.DB
}

// NewAPIKeyService creates a new API key service
func NewAPIKeyService(db *database.DB) *APIKeyService {
	return &APIKeyService{db: db}
}

// CreateAPIKeyForUser generates a new API key owned by a user, stores its hash, and returns the raw key (shown once)
func (s *APIKeyService) CreateAPIKeyForUser(ctx context.Context, name, description string, userID *primitive.ObjectID) (string, *models.APIKey, error) {
	return s.CreateScopedAPIKey(ctx, name, description, userID, nil, false)
}

// CreateScopedAPIKey creates an API key with an optional permission
// allowlist (scopes) and sandbox-only restriction — the governance surface
// for keys handed to AI agents.
func (s *APIKeyService) CreateScopedAPIKey(ctx context.Context, name, description string, userID *primitive.ObjectID, scopes []string, sandboxOnly bool) (string, *models.APIKey, error) {
	if name == "" {
		return "", nil, fmt.Errorf("name is required")
	}

	// Generate 32 random bytes → 64 hex chars, but we use 32 hex chars for the key
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	rawKey := "lc_" + hex.EncodeToString(randomBytes)

	// Hash the key with SHA-256
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	// Store prefix for identification (lc_ + first 8 hex chars)
	prefix := rawKey[:11]

	now := time.Now()
	apiKey := &models.APIKey{
		Name:        name,
		Description: description,
		Prefix:      prefix,
		KeyHash:     keyHash,
		UserID:      userID,
		Scopes:      scopes,
		SandboxOnly: sandboxOnly,
		CreatedAt:   now,
	}

	id, err := s.db.InsertOne(ctx, "api_keys", apiKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create API key: %w", err)
	}
	apiKey.ID = id

	return rawKey, apiKey, nil
}

// CreateAPIKey generates a new API key without user ownership (for system/legacy use)
func (s *APIKeyService) CreateAPIKey(ctx context.Context, name, description string) (string, *models.APIKey, error) {
	return s.CreateAPIKeyForUser(ctx, name, description, nil)
}

// ListAPIKeys returns all API keys (without raw key values)
func (s *APIKeyService) ListAPIKeys(ctx context.Context) ([]models.APIKey, error) {
	cursor, err := s.db.FindMany(ctx, "api_keys", bson.M{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer cursor.Close(ctx)

	var keys []models.APIKey
	if err := cursor.All(ctx, &keys); err != nil {
		return nil, fmt.Errorf("failed to decode API keys: %w", err)
	}
	return keys, nil
}

// ListAPIKeysForUser returns API keys owned by a specific user
func (s *APIKeyService) ListAPIKeysForUser(ctx context.Context, userID primitive.ObjectID) ([]models.APIKey, error) {
	cursor, err := s.db.FindMany(ctx, "api_keys", bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer cursor.Close(ctx)

	var keys []models.APIKey
	if err := cursor.All(ctx, &keys); err != nil {
		return nil, fmt.Errorf("failed to decode API keys: %w", err)
	}
	return keys, nil
}

// DeleteAPIKey deletes an API key by ID. Admins may delete any key.
func (s *APIKeyService) DeleteAPIKey(ctx context.Context, id primitive.ObjectID) error {
	return s.deleteKey(ctx, bson.M{"_id": id})
}

// ErrAPIKeyNotFound is returned when a delete matched no key: the key does
// not exist, or (DeleteAPIKeyForUser) it belongs to another user.
var ErrAPIKeyNotFound = errors.New("API key not found")

// DeleteAPIKeyForUser deletes an API key only if it is owned by the given user.
// Returns ErrAPIKeyNotFound if the key does not exist or belongs to a different user.
func (s *APIKeyService) DeleteAPIKeyForUser(ctx context.Context, id primitive.ObjectID, ownerID primitive.ObjectID) error {
	return s.deleteKey(ctx, bson.M{"_id": id, "user_id": ownerID})
}

// deleteKey deletes the key the filter selects and reports when there was
// none: a delete that matches nothing is not an error to MongoDB, and
// callers were reporting it as a successful revocation.
func (s *APIKeyService) deleteKey(ctx context.Context, filter bson.M) error {
	// DeleteMany because it reports the count (the filter is by _id, so it
	// matches at most one key)
	n, err := s.db.DeleteMany(ctx, "api_keys", filter)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// AssignOwner gives a key with no owner on record to a user. It never changes
// the owner of a key that already has one.
func (s *APIKeyService) AssignOwner(ctx context.Context, id, ownerID primitive.ObjectID) error {
	if err := s.db.UpdateOne(ctx, "api_keys",
		bson.M{"_id": id, "user_id": nil},
		bson.M{"$set": bson.M{"user_id": ownerID}}); err != nil {
		return fmt.Errorf("failed to assign API key owner: %w", err)
	}
	return nil
}

// ValidateAPIKey checks a raw API key, returns the key record if valid, and updates last_used_at
func (s *APIKeyService) ValidateAPIKey(ctx context.Context, rawKey string) (*models.APIKey, error) {
	if len(rawKey) < 4 || rawKey[:3] != "lc_" {
		return nil, fmt.Errorf("invalid API key format")
	}

	// Hash the provided key
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	// Look up by hash
	var apiKey models.APIKey
	err := s.db.FindOne(ctx, "api_keys", bson.M{"key_hash": keyHash}, &apiKey)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	// Update last_used_at (fire-and-forget)
	now := time.Now()
	_ = s.db.UpdateOne(ctx, "api_keys", bson.M{"_id": apiKey.ID}, bson.M{"$set": bson.M{"last_used_at": now}})
	apiKey.LastUsedAt = &now

	return &apiKey, nil
}
