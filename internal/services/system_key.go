package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/metavert-io/lightcms/v7/internal/database"

	"go.mongodb.org/mongo-driver/bson"
)

// RemoveLegacySystemKey deletes what is left of the shared admin key that
// OAuth sessions used to run on before 7.5.0: the "System (OAuth)" API key and
// the stored copy of its raw value. The key goes first, so a failure part way
// leaves the stored value for the next startup to find. It reports whether it
// removed anything.
func RemoveLegacySystemKey(ctx context.Context, db *database.DB) (bool, error) {
	var setting struct {
		RawKey string `bson:"raw_key"`
	}
	haveSetting := db.FindOne(ctx, "settings", bson.M{"type": "system_api_key"}, &setting) == nil

	filters := bson.A{bson.M{"name": "System (OAuth)", "description": "Internal key for OAuth-authenticated MCP sessions"}}
	if setting.RawKey != "" {
		sum := sha256.Sum256([]byte(setting.RawKey))
		filters = append(filters, bson.M{"key_hash": hex.EncodeToString(sum[:])})
	}
	removed, err := db.DeleteMany(ctx, "api_keys", bson.M{"$or": filters})
	if err != nil {
		return false, fmt.Errorf("deleting the system API key: %w", err)
	}
	if !haveSetting {
		return removed > 0, nil
	}
	if err := db.DeleteOne(ctx, "settings", bson.M{"type": "system_api_key"}); err != nil {
		return removed > 0, fmt.Errorf("deleting the stored system API key: %w", err)
	}
	return true, nil
}
