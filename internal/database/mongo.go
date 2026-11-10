package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DB struct {
	client   *mongo.Client
	database *mongo.Database

	// faultHook, when set, is consulted at the start of every generic CRUD
	// helper. If it returns a non-nil error, the operation fails with that
	// error instead of touching MongoDB. It is nil in production (a single
	// nil check, no behaviour change) and exists so tests can deterministically
	// exercise database-error branches, including multi-step ones. See
	// SetFaultHook.
	faultHook func(op, collection string) error
}

// SetFaultHook installs a fault-injection hook for tests. Passing nil clears it.
// Intended for use in tests only.
func (db *DB) SetFaultHook(f func(op, collection string) error) {
	db.faultHook = f
}

// fault consults the fault hook (if any) for the given operation/collection.
func (db *DB) fault(op, collection string) error {
	if db.faultHook != nil {
		return db.faultHook(op, collection)
	}
	return nil
}

func Connect(ctx context.Context, uri, dbName string, extraOpts ...*options.ClientOptions) (*DB, error) {
	base := options.Client().ApplyURI(uri)
	all := append([]*options.ClientOptions{base}, extraOpts...)
	client, err := mongo.Connect(ctx, options.MergeClientOptions(all...))
	if err != nil {
		return nil, err
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	db := &DB{
		client:   client,
		database: client.Database(dbName),
	}

	// Create indexes
	if err := db.createIndexes(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

// OAuthGrantIndex is the unique index that allows one active OAuth grant per
// person and client. SaveGrant inserts grants with revoked_at: null.
func OAuthGrantIndex() mongo.IndexModel {
	return mongo.IndexModel{
		Keys: bson.D{{Key: "client_id", Value: 1}, {Key: "user_id", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.M{"revoked_at": bson.M{"$type": "null"}}).
			SetName("one_active_grant_per_person_and_client"),
	}
}

func (db *DB) createIndexes(ctx context.Context) error {
	// Content full_path index (unique for live content only).
	// Fork copies share the same full_path as their live counterpart, so the
	// uniqueness constraint must be partial — only documents without fork_id.
	// Drop the old unpartitioned index first (ignore error if it doesn't exist).
	// Drop both the old single-field index and any previous partial-index attempts.
	db.database.Collection("content").Indexes().DropOne(ctx, "full_path_1")
	// Compound unique index on (full_path, fork_id).
	// Live pages have no fork_id (stored as null); fork copies have an ObjectID.
	// The compound key ensures live pages can't duplicate full_path, while a live
	// page and its fork copy can share the same full_path without conflicting.
	_, err := db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "full_path", Value: 1},
			{Key: "fork_id", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetSparse(true),
	})
	if err != nil {
		return err
	}

	// Drop old unique slug index if it exists (we're changing it to non-unique)
	db.database.Collection("content").Indexes().DropOne(ctx, "slug_1")

	// Content slug index (non-unique now, since same slug can be in different folders)
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "slug", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Content folder_id index for quick folder lookups
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "folder_id", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Content category index for collections
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "category", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Content compound index for the most common list query: published + not-deleted
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "published", Value: 1}, {Key: "deleted", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Content template_name index — used by scoped bulk operations and export
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "template_name", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Content tags index (multikey — MongoDB automatically handles arrays)
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "tags", Value: 1}},
	})
	if err != nil {
		return err
	}

	// content_versions content_id index — used by saveVersion's Count query on every update.
	// Without this, every UpdateContent call causes a full collection scan.
	_, err = db.database.Collection("content_versions").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "content_id", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Snippet name index (unique)
	_, err = db.database.Collection("snippets").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Template name index
	_, err = db.database.Collection("templates").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Folder path index (unique)
	_, err = db.database.Collection("folders").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "path", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Folder parent_id index
	_, err = db.database.Collection("folders").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "parent_id", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Custom pages slug index
	_, err = db.database.Collection("custom_pages").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "slug", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Redirect from_path index (unique)
	_, err = db.database.Collection("redirects").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "from_path", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Contact messages created_at index for sorting
	_, err = db.database.Collection("contact_messages").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Contact rate limiting index (IP + created_at)
	_, err = db.database.Collection("contact_messages").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "ip_address", Value: 1}, {Key: "created_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Asset full_path index (unique)
	_, err = db.database.Collection("assets").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "full_path", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Asset folder index for filtering
	_, err = db.database.Collection("assets").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "folder", Value: 1}},
	})
	if err != nil {
		return err
	}

	// API key hash index (unique)
	_, err = db.database.Collection("api_keys").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "key_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// OAuth client_id index (unique)
	_, err = db.database.Collection("oauth_clients").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "client_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// OAuth auth codes: unique code hash + TTL auto-cleanup
	_, err = db.database.Collection("oauth_auth_codes").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = db.database.Collection("oauth_auth_codes").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return err
	}

	// OAuth access tokens: unique hash + TTL auto-cleanup
	_, err = db.database.Collection("oauth_access_tokens").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "token_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = db.database.Collection("oauth_access_tokens").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return err
	}

	// OAuth refresh tokens: unique hash + TTL auto-cleanup
	_, err = db.database.Collection("oauth_refresh_tokens").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "token_hash", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}
	_, err = db.database.Collection("oauth_refresh_tokens").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return err
	}

	// OAuth grants: one active grant per person and client. Revoking a grant
	// sets revoked_at, which takes it out of the index.
	_, err = db.database.Collection("oauth_grants").Indexes().CreateOne(ctx, OAuthGrantIndex())
	if err != nil {
		return err
	}

	// Users: unique email
	_, err = db.database.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	// Audit logs: created_at descending (primary query)
	_, err = db.database.Collection("audit_logs").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Audit logs: user_id + created_at for per-user filtering
	_, err = db.database.Collection("audit_logs").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Audit logs: action + created_at for action type filtering
	_, err = db.database.Collection("audit_logs").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "action", Value: 1}, {Key: "created_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Audit logs: resource + resource_id for resource lookups
	_, err = db.database.Collection("audit_logs").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "resource", Value: 1}, {Key: "resource_id", Value: 1}},
	})
	if err != nil {
		return err
	}

	// Audit logs: TTL index — auto-delete after 365 days.
	// Use CreateMany with the same spec but ignore "already exists" errors so that
	// a missing or misconfigured TTL index from a previous install is always corrected.
	_, err = db.database.Collection("audit_logs").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(365 * 24 * 60 * 60).SetName("audit_ttl_365d"),
	})
	if err != nil {
		// Ignore "index already exists with same name" (code 68) and
		// "index options conflict" (code 85) — the index is already in place.
		if cmdErr, ok := err.(mongo.CommandError); !ok || (cmdErr.Code != 68 && cmdErr.Code != 85 && cmdErr.Code != 86) {
			return err
		}
	}

	// Settings type index — speeds up all settings lookups by type field
	_, err = db.database.Collection("settings").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "type", Value: 1}}, Options: options.Index().SetName("settings_type"),
	})
	if err != nil {
		return err
	}

	// Login attempts IP index — needed for rate limiting lookups
	_, err = db.database.Collection("login_attempts").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "ip", Value: 1}}, Options: options.Index().SetName("login_attempts_ip"),
	})
	if err != nil {
		return err
	}

	// Theme versions version index — used for sorting and point lookups
	_, err = db.database.Collection("theme_versions").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "version", Value: 1}}, Options: options.Index().SetName("theme_versions_version"),
	})
	if err != nil {
		return err
	}

	// Content updated_at index — used by admin content list and dashboard (sort by updated_at desc).
	// Without this, MongoDB must sort the entire collection in memory, hitting the 32 MiB limit.
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "updated_at", Value: -1}},
	})
	if err != nil {
		return err
	}

	// Compound index for admin content list: always filters fork_id ($exists false) + deleted,
	// then sorts by updated_at. This covers the most expensive query in the admin UI.
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "fork_id", Value: 1},
			{Key: "deleted", Value: 1},
			{Key: "updated_at", Value: -1},
		},
	})
	if err != nil {
		return err
	}

	// Content plain_text text index — enables full-text search on plain_text field.
	// A collection can only have one text index; create with ignore-existing logic.
	_, err = db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "plain_text", Value: "text"}}, Options: options.Index().SetName("content_plain_text_search"),
	})
	if err != nil {
		// Ignore conflicts with an existing text index (codes 85, 86, 68)
		if cmdErr, ok := err.(mongo.CommandError); !ok || (cmdErr.Code != 68 && cmdErr.Code != 85 && cmdErr.Code != 86) {
			return err
		}
	}

	// webhooks collection indexes
	db.database.Collection("webhooks").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "active", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: -1}}},
	})
	// webhook_deliveries collection — compound + TTL
	db.database.Collection("webhook_deliveries").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "webhook_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 3600)},
	})
	// content_locks collection — unique content_id + TTL
	db.database.Collection("content_locks").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "content_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
	// regen_jobs collection
	db.database.Collection("regen_jobs").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "created_at", Value: -1}}},
	})
	// content publish_at sparse index for scheduled publishing
	db.database.Collection("content").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "publish_at", Value: 1}},
		Options: options.Index().SetSparse(true),
	})

	// import_sources
	db.database.Collection("import_sources").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "active", Value: 1}}},
		{Keys: bson.D{{Key: "schedule", Value: 1}, {Key: "last_run_at", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: -1}}},
	})
	// import_jobs
	db.database.Collection("import_jobs").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "source_id", Value: 1}, {Key: "started_at", Value: -1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "started_at", Value: -1}}},
		{Keys: bson.D{{Key: "started_at", Value: -1}}},
	})
	// import_logs — TTL 90 days, index by job_id+seq for streaming
	db.database.Collection("import_logs").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "job_id", Value: 1}, {Key: "seq", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(90 * 24 * 3600)},
	})

	return nil
}

func (db *DB) Disconnect(ctx context.Context) error {
	return db.client.Disconnect(ctx)
}

// Template operations
func (db *DB) Templates() *mongo.Collection {
	return db.database.Collection("templates")
}

func (db *DB) Content() *mongo.Collection {
	return db.database.Collection("content")
}

func (db *DB) Collections() *mongo.Collection {
	return db.database.Collection("collections")
}

func (db *DB) CustomPages() *mongo.Collection {
	return db.database.Collection("custom_pages")
}

func (db *DB) Settings() *mongo.Collection {
	return db.database.Collection("settings")
}

// Generic CRUD helpers
func (db *DB) InsertOne(ctx context.Context, collection string, doc interface{}) (primitive.ObjectID, error) {
	if err := db.fault("InsertOne", collection); err != nil {
		return primitive.NilObjectID, err
	}
	result, err := db.database.Collection(collection).InsertOne(ctx, doc)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return result.InsertedID.(primitive.ObjectID), nil
}

func (db *DB) InsertMany(ctx context.Context, collection string, docs []interface{}) error {
	if len(docs) == 0 {
		return nil
	}
	if err := db.fault("InsertMany", collection); err != nil {
		return err
	}
	_, err := db.database.Collection(collection).InsertMany(ctx, docs)
	return err
}

// InsertManyUnordered inserts docs with ordered=false so failures don't abort the batch.
// Returns the inserted IDs and any bulk write errors (partial success is possible).
func (db *DB) InsertManyUnordered(ctx context.Context, collection string, docs []interface{}) (*mongo.InsertManyResult, error) {
	if len(docs) == 0 {
		return &mongo.InsertManyResult{}, nil
	}
	if err := db.fault("InsertManyUnordered", collection); err != nil {
		return nil, err
	}
	opts := options.InsertMany().SetOrdered(false)
	return db.database.Collection(collection).InsertMany(ctx, docs, opts)
}

func (db *DB) FindOne(ctx context.Context, collection string, filter interface{}, result interface{}) error {
	if err := db.fault("FindOne", collection); err != nil {
		return err
	}
	return db.database.Collection(collection).FindOne(ctx, filter).Decode(result)
}

func (db *DB) FindMany(ctx context.Context, collection string, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
	if err := db.fault("FindMany", collection); err != nil {
		return nil, err
	}
	return db.database.Collection(collection).Find(ctx, filter, opts...)
}

func (db *DB) FindAll(ctx context.Context, collection string, filter interface{}, results interface{}) error {
	if err := db.fault("FindAll", collection); err != nil {
		return err
	}
	cursor, err := db.database.Collection(collection).Find(ctx, filter)
	if err != nil {
		return err
	}
	return cursor.All(ctx, results)
}

func (db *DB) Aggregate(ctx context.Context, collection string, pipeline interface{}, results interface{}) error {
	if err := db.fault("Aggregate", collection); err != nil {
		return err
	}
	cursor, err := db.database.Collection(collection).Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	return cursor.All(ctx, results)
}

func (db *DB) UpdateOne(ctx context.Context, collection string, filter, update interface{}) error {
	if err := db.fault("UpdateOne", collection); err != nil {
		return err
	}
	_, err := db.database.Collection(collection).UpdateOne(ctx, filter, update)
	return err
}

func (db *DB) DeleteOne(ctx context.Context, collection string, filter interface{}) error {
	if err := db.fault("DeleteOne", collection); err != nil {
		return err
	}
	_, err := db.database.Collection(collection).DeleteOne(ctx, filter)
	return err
}

func (db *DB) DeleteMany(ctx context.Context, collection string, filter interface{}) (int64, error) {
	if err := db.fault("DeleteMany", collection); err != nil {
		return 0, err
	}
	result, err := db.database.Collection(collection).DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

func (db *DB) Count(ctx context.Context, collection string, filter interface{}) (int64, error) {
	if err := db.fault("Count", collection); err != nil {
		return 0, err
	}
	return db.database.Collection(collection).CountDocuments(ctx, filter)
}

// ThemeSettings holds the active theme configuration stored in the settings collection.
// Both bson and json tags are required: bson for MongoDB read/write, json for the REST API response.
type ThemeSettings struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"    json:"-"`
	PrimaryColor    string             `bson:"primary_color"    json:"primary_color"`
	SecondaryColor  string             `bson:"secondary_color"  json:"secondary_color"`
	AccentColor     string             `bson:"accent_color"     json:"accent_color"`
	BackgroundColor string             `bson:"background_color" json:"background_color"`
	TextColor       string             `bson:"text_color"       json:"text_color"`
	FontFamily      string             `bson:"font_family"      json:"font_family"`
	HeadingFont     string             `bson:"heading_font"     json:"heading_font"`
	BorderRadius    string             `bson:"border_radius"    json:"border_radius"`
	CustomCSS       string             `bson:"custom_css"       json:"custom_css"`
	SiteName        string             `bson:"site_name"        json:"site_name"`
	SiteTagline     string             `bson:"site_tagline"     json:"site_tagline"`
	LogoURL         string             `bson:"logo_url"         json:"logo_url"`
	HeadHTML        string             `bson:"head_html"        json:"head_html"`
	HeaderHTML      string             `bson:"header_html"      json:"header_html"`
	FooterHTML      string             `bson:"footer_html"      json:"footer_html"`
	UpdatedAt       time.Time          `bson:"updated_at"       json:"updated_at"`
}

// ThemeVersion represents a historical version of theme settings
type ThemeVersion struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Version         int                `bson:"version" json:"version"`
	Comment         string             `bson:"comment,omitempty" json:"comment,omitempty"`
	Locked          bool               `bson:"locked,omitempty" json:"locked,omitempty"` // Pinned — cannot be auto-pruned
	PrimaryColor    string             `bson:"primary_color" json:"primary_color"`
	SecondaryColor  string             `bson:"secondary_color" json:"secondary_color"`
	AccentColor     string             `bson:"accent_color" json:"accent_color"`
	BackgroundColor string             `bson:"background_color" json:"background_color"`
	TextColor       string             `bson:"text_color" json:"text_color"`
	FontFamily      string             `bson:"font_family" json:"font_family"`
	HeadingFont     string             `bson:"heading_font" json:"heading_font"`
	BorderRadius    string             `bson:"border_radius" json:"border_radius"`
	CustomCSS       string             `bson:"custom_css" json:"custom_css"`
	SiteName        string             `bson:"site_name" json:"site_name"`
	SiteTagline     string             `bson:"site_tagline" json:"site_tagline"`
	LogoURL         string             `bson:"logo_url" json:"logo_url"`
	HeadHTML        string             `bson:"head_html" json:"head_html"`
	HeaderHTML      string             `bson:"header_html" json:"header_html"`
	FooterHTML      string             `bson:"footer_html" json:"footer_html"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
}

func (db *DB) GetThemeSettings(ctx context.Context) (*ThemeSettings, error) {
	var settings ThemeSettings
	err := db.Settings().FindOne(ctx, bson.M{"type": "theme"}).Decode(&settings)
	if err == mongo.ErrNoDocuments {
		// Return default settings
		return &ThemeSettings{
			PrimaryColor:    "#6366f1",
			SecondaryColor:  "#8b5cf6",
			AccentColor:     "#06b6d4",
			BackgroundColor: "#0f172a",
			TextColor:       "#f1f5f9",
			FontFamily:      "Inter, system-ui, sans-serif",
			HeadingFont:     "Space Grotesk, system-ui, sans-serif",
			BorderRadius:    "12px",
			CustomCSS:       "",
			SiteName:        "LightCMS",
			SiteTagline:     "A lightweight content management system",
			HeadHTML:        "",
			HeaderHTML:      "",
			FooterHTML:      "",
		}, nil
	}
	return &settings, err
}

func (db *DB) SaveThemeSettings(ctx context.Context, settings *ThemeSettings) error {
	settings.UpdatedAt = time.Now()
	_, err := db.Settings().UpdateOne(
		ctx,
		bson.M{"type": "theme"},
		bson.M{"$set": settings, "$setOnInsert": bson.M{"type": "theme"}},
		options.Update().SetUpsert(true),
	)
	return err
}

// ThemeVersions returns the theme_versions collection
func (db *DB) ThemeVersions() *mongo.Collection {
	return db.database.Collection("theme_versions")
}

// SaveThemeVersion saves a new version of the theme settings
func (db *DB) SaveThemeVersion(ctx context.Context, version *ThemeVersion) error {
	version.CreatedAt = time.Now()
	id, err := db.InsertOne(ctx, "theme_versions", version)
	if err != nil {
		return err
	}
	version.ID = id
	return nil
}

// GetThemeVersions retrieves all theme versions sorted by version number descending
func (db *DB) GetThemeVersions(ctx context.Context) ([]ThemeVersion, error) {
	cursor, err := db.FindMany(ctx, "theme_versions", bson.M{},
		options.Find().SetSort(bson.D{{Key: "version", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var versions []ThemeVersion
	if err := cursor.All(ctx, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// GetThemeVersion retrieves a specific theme version by version number
func (db *DB) GetThemeVersion(ctx context.Context, version int) (*ThemeVersion, error) {
	var v ThemeVersion
	err := db.FindOne(ctx, "theme_versions", bson.M{"version": version}, &v)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetThemeVersionCount returns the number of theme versions
func (db *DB) GetThemeVersionCount(ctx context.Context) (int64, error) {
	return db.Count(ctx, "theme_versions", bson.M{})
}

// SetThemeVersionLocked sets or clears the locked flag on a theme version.
func (db *DB) SetThemeVersionLocked(ctx context.Context, version int, locked bool) error {
	_, err := db.ThemeVersions().UpdateOne(ctx,
		bson.M{"version": version},
		bson.M{"$set": bson.M{"locked": locked}},
	)
	return err
}

// AdminSettings stores admin configuration including password hash
type AdminSettings struct {
	ID                primitive.ObjectID `bson:"_id,omitempty"`
	PasswordHash      string             `bson:"password_hash"`
	IsDefaultPassword bool               `bson:"is_default_password"`
	UpdatedAt         time.Time          `bson:"updated_at"`
}

// LoginAttempt tracks failed login attempts for rate limiting
type LoginAttempt struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	IP          string             `bson:"ip"`
	Attempts    int                `bson:"attempts"`
	LastAttempt time.Time          `bson:"last_attempt"`
	LockedUntil *time.Time         `bson:"locked_until,omitempty"`
}

func (db *DB) GetAdminSettings(ctx context.Context) (*AdminSettings, error) {
	var settings AdminSettings
	err := db.Settings().FindOne(ctx, bson.M{"type": "admin"}).Decode(&settings)
	if err == mongo.ErrNoDocuments {
		return nil, nil // No settings yet
	}
	return &settings, err
}

func (db *DB) SaveAdminSettings(ctx context.Context, settings *AdminSettings) error {
	settings.UpdatedAt = time.Now()
	_, err := db.Settings().UpdateOne(
		ctx,
		bson.M{"type": "admin"},
		bson.M{"$set": settings, "$setOnInsert": bson.M{"type": "admin"}},
		options.Update().SetUpsert(true),
	)
	return err
}

// SiteConfig stores site-wide configuration settings
type SiteConfig struct {
	ID                   primitive.ObjectID `bson:"_id,omitempty"`
	TitleTemplate        string             `bson:"title_template"`                                       // Template when page has title, e.g. "{{title}} | {{site_name}}"
	TitleTemplateNoTitle string             `bson:"title_template_no_title"`                              // Template when page has no title, e.g. "{{site_name}}"
	MarkdownScriptPolicy string             `bson:"markdown_script_policy" json:"markdown_script_policy"` // Values: "all" (default), "admin_only", "none"
	MaxUploadBytes       int64              `bson:"max_upload_bytes" json:"max_upload_bytes"`             // Max file upload size in bytes (default: 1 MiB)
	CloudflareZoneID     string             `bson:"cloudflare_zone_id,omitempty" json:"cloudflare_zone_id,omitempty"`
	CloudflareAPIToken   string             `bson:"cloudflare_api_token,omitempty" json:"cloudflare_api_token,omitempty"`
	CFCacheEnabled       bool               `bson:"cf_cache_enabled,omitempty" json:"cf_cache_enabled,omitempty"`
	UpdatedAt            time.Time          `bson:"updated_at"`
}

// GetSiteConfig retrieves the site configuration
func (db *DB) GetSiteConfig(ctx context.Context) (*SiteConfig, error) {
	var config SiteConfig
	err := db.Settings().FindOne(ctx, bson.M{"type": "site_config"}).Decode(&config)
	if err == mongo.ErrNoDocuments {
		// Return defaults
		return &SiteConfig{
			TitleTemplate:        "{{title}} - {{site_name}}",
			TitleTemplateNoTitle: "{{site_name}}",
			MarkdownScriptPolicy: "all",
			MaxUploadBytes:       1 << 20, // 1 MiB — 2× largest current asset (metavert-bg.js, 517 KB)
		}, nil
	}
	if err == nil && config.MaxUploadBytes == 0 {
		config.MaxUploadBytes = 1 << 20
	}
	return &config, err
}

// SaveSiteConfig saves the site configuration
func (db *DB) SaveSiteConfig(ctx context.Context, config *SiteConfig) error {
	config.UpdatedAt = time.Now()
	_, err := db.Settings().UpdateOne(
		ctx,
		bson.M{"type": "site_config"},
		bson.M{"$set": config, "$setOnInsert": bson.M{"type": "site_config"}},
		options.Update().SetUpsert(true),
	)
	return err
}

// SearchConfig stores configurable search ranking parameters
type SearchConfig struct {
	ID                 primitive.ObjectID `bson:"_id,omitempty"`
	NavBoost           float64            `bson:"nav_boost"`            // Score bonus for nav-linked pages (default 0.15)
	TitleBoost         float64            `bson:"title_boost"`          // Score bonus when query appears in title (default 0.20)
	BoostTemplates     []string           `bson:"boost_templates"`      // Template name substrings that get a boost (default ["concept"])
	BoostTemplateScore float64            `bson:"boost_template_score"` // Score bonus for boosted templates (default 0.05)
	BoostPaths         []string           `bson:"boost_paths"`          // Specific page paths to always boost (exact match, default [])
	BoostPathScore     float64            `bson:"boost_path_score"`     // Score bonus for boosted pages (default 0.15)
	DemotePaths        []string           `bson:"demote_paths"`         // Specific page paths to always demote (exact match, default [])
	DemotePathPrefixes []string           `bson:"demote_path_prefixes"` // URL path prefixes to deprioritise (default ["/videos/", "/video/"])
	DemoteScore        float64            `bson:"demote_score"`         // Score penalty for demoted pages (default -0.05)
	UpdatedAt          time.Time          `bson:"updated_at"`
}

// DefaultSearchConfig returns the built-in default search ranking configuration.
func DefaultSearchConfig() *SearchConfig {
	return &SearchConfig{
		NavBoost:           0.15,
		TitleBoost:         0.20,
		BoostTemplates:     []string{"concept"},
		BoostTemplateScore: 0.05,
		BoostPaths:         []string{},
		BoostPathScore:     0.15,
		DemotePaths:        []string{},
		DemotePathPrefixes: []string{"/videos/", "/video/"},
		DemoteScore:        -0.05,
	}
}

// GetSearchConfig retrieves the search ranking configuration.
func (db *DB) GetSearchConfig(ctx context.Context) (*SearchConfig, error) {
	var cfg SearchConfig
	err := db.Settings().FindOne(ctx, bson.M{"type": "search_config"}).Decode(&cfg)
	if err == mongo.ErrNoDocuments {
		return DefaultSearchConfig(), nil
	}
	if err != nil {
		return DefaultSearchConfig(), err
	}
	// If record exists but was saved with zero values, fill in defaults
	if cfg.NavBoost == 0 && cfg.TitleBoost == 0 && len(cfg.BoostTemplates) == 0 && len(cfg.DemotePathPrefixes) == 0 {
		return DefaultSearchConfig(), nil
	}
	return &cfg, nil
}

// SaveSearchConfig persists the search ranking configuration.
func (db *DB) SaveSearchConfig(ctx context.Context, cfg *SearchConfig) error {
	cfg.UpdatedAt = time.Now()
	_, err := db.Settings().UpdateOne(
		ctx,
		bson.M{"type": "search_config"},
		bson.M{"$set": cfg, "$setOnInsert": bson.M{"type": "search_config"}},
		options.Update().SetUpsert(true),
	)
	return err
}

// DefaultChatSystemPrompt is the default system prompt for the chat widget.
// Use {siteName} as a placeholder — it is replaced at request time.
const DefaultChatSystemPrompt = `You are a friendly, knowledgeable assistant for {siteName}. Use the provided page excerpts to answer questions conversationally and helpfully. Synthesize a clear, direct answer — don't just describe what the pages say. Keep answers concise (2-4 sentences). If the excerpts don't contain enough to answer confidently, say so briefly and naturally.`

// DefaultChatUserPromptTemplate is the default template for the user-turn message sent to the model.
// Use {excerpts} for the numbered excerpt block and {question} for the user's query.
const DefaultChatUserPromptTemplate = `Here are relevant excerpts from the site:

{excerpts}
Question: {question}`

// ChatWidgetConfig stores configuration for the public-facing chat widget
type ChatWidgetConfig struct {
	ID                 primitive.ObjectID `bson:"_id,omitempty"`
	Enabled            bool               `bson:"enabled"`
	WidgetTitle        string             `bson:"widget_title"`
	WelcomeMessage     string             `bson:"welcome_message"`
	Placeholder        string             `bson:"placeholder"`
	PrimaryColor       string             `bson:"primary_color"`
	Position           string             `bson:"position"` // "bottom-right" | "bottom-left"
	MaxResults         int                `bson:"max_results"`
	RateLimitPerIP     int                `bson:"rate_limit_per_ip"`    // max queries/minute per IP (default 5)
	RateLimitGlobal    int                `bson:"rate_limit_global"`    // max queries/minute total (default 30)
	SystemPrompt       string             `bson:"system_prompt"`        // editable system prompt; {siteName} replaced at runtime
	UserPromptTemplate string             `bson:"user_prompt_template"` // editable user prompt; {excerpts} and {question} replaced at runtime
	UpdatedAt          time.Time          `bson:"updated_at"`
}

// DefaultChatWidgetConfig returns sensible defaults for the chat widget
func DefaultChatWidgetConfig() *ChatWidgetConfig {
	return &ChatWidgetConfig{
		Enabled:            false,
		WidgetTitle:        "Chat with Site",
		WelcomeMessage:     "Hi! Ask me anything and I'll find the most relevant pages for you.",
		Placeholder:        "Ask a question...",
		PrimaryColor:       "#6366f1",
		Position:           "bottom-right",
		MaxResults:         3,
		RateLimitPerIP:     5,
		RateLimitGlobal:    30,
		SystemPrompt:       DefaultChatSystemPrompt,
		UserPromptTemplate: DefaultChatUserPromptTemplate,
	}
}

// GetChatWidgetConfig retrieves the chat widget configuration
func (db *DB) GetChatWidgetConfig(ctx context.Context) (*ChatWidgetConfig, error) {
	var cfg ChatWidgetConfig
	err := db.Settings().FindOne(ctx, bson.M{"type": "chat_widget"}).Decode(&cfg)
	if err == mongo.ErrNoDocuments {
		return DefaultChatWidgetConfig(), nil
	}
	if err != nil {
		return DefaultChatWidgetConfig(), err
	}
	return &cfg, nil
}

// SaveChatWidgetConfig persists the chat widget configuration
func (db *DB) SaveChatWidgetConfig(ctx context.Context, cfg *ChatWidgetConfig) error {
	cfg.UpdatedAt = time.Now()
	_, err := db.Settings().UpdateOne(
		ctx,
		bson.M{"type": "chat_widget"},
		bson.M{"$set": cfg, "$setOnInsert": bson.M{"type": "chat_widget"}},
		options.Update().SetUpsert(true),
	)
	return err
}

// DatabaseVersion tracks the software version used with the database
type DatabaseVersion struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Type      string             `bson:"type"`
	Version   string             `bson:"version"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

// GetDatabaseVersion retrieves the current database version
func (db *DB) GetDatabaseVersion(ctx context.Context) (string, error) {
	var ver DatabaseVersion
	err := db.Settings().FindOne(ctx, bson.M{"type": "database_version"}).Decode(&ver)
	if err == mongo.ErrNoDocuments {
		return "", nil // No version set yet
	}
	if err != nil {
		return "", err
	}
	return ver.Version, nil
}

// SetDatabaseVersion updates the database version
func (db *DB) SetDatabaseVersion(ctx context.Context, version string) error {
	_, err := db.Settings().UpdateOne(
		ctx,
		bson.M{"type": "database_version"},
		bson.M{"$set": bson.M{"version": version, "updated_at": time.Now()}, "$setOnInsert": bson.M{"type": "database_version"}},
		options.Update().SetUpsert(true),
	)
	return err
}

// GetLoginAttempts gets the login attempt record for an IP
func (db *DB) GetLoginAttempts(ctx context.Context, ip string) (*LoginAttempt, error) {
	var attempt LoginAttempt
	err := db.database.Collection("login_attempts").FindOne(ctx, bson.M{"ip": ip}).Decode(&attempt)
	if err == mongo.ErrNoDocuments {
		return &LoginAttempt{IP: ip, Attempts: 0}, nil
	}
	return &attempt, err
}

// RecordFailedLogin increments failed login counter atomically using FindOneAndUpdate.
// If no record exists or the last attempt was over 15 minutes ago, the counter is reset to 1.
// Lockout thresholds: 10 attempts → 1 min, 15 attempts → 5 min, 20+ attempts → 15 min.
func (db *DB) RecordFailedLogin(ctx context.Context, ip string) error {
	now := time.Now()
	cutoff := now.Add(-15 * time.Minute)

	// Attempt an atomic $inc on documents that are still within the 15-minute window.
	var updated LoginAttempt
	err := db.database.Collection("login_attempts").FindOneAndUpdate(
		ctx,
		bson.M{"ip": ip, "last_attempt": bson.M{"$gte": cutoff}},
		bson.M{
			"$inc": bson.M{"attempts": 1},
			"$set": bson.M{"last_attempt": now},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)

	if err == mongo.ErrNoDocuments {
		// No recent record — either first attempt or window expired; reset to 1.
		updated = LoginAttempt{IP: ip, Attempts: 1, LastAttempt: now}
		_, upsertErr := db.database.Collection("login_attempts").UpdateOne(
			ctx,
			bson.M{"ip": ip},
			bson.M{"$set": bson.M{"ip": ip, "attempts": 1, "last_attempt": now, "locked_until": nil}},
			options.Update().SetUpsert(true),
		)
		if upsertErr != nil {
			return upsertErr
		}
	} else if err != nil {
		return err
	}

	// Apply lockout thresholds based on the updated attempt count.
	var lockUntil *time.Time
	switch {
	case updated.Attempts >= 20:
		t := now.Add(15 * time.Minute)
		lockUntil = &t
	case updated.Attempts >= 15:
		t := now.Add(5 * time.Minute)
		lockUntil = &t
	case updated.Attempts >= 10:
		t := now.Add(1 * time.Minute)
		lockUntil = &t
	}

	if lockUntil != nil {
		_, err = db.database.Collection("login_attempts").UpdateOne(
			ctx,
			bson.M{"ip": ip},
			bson.M{"$set": bson.M{"locked_until": lockUntil}},
		)
		return err
	}
	return nil
}

// ClearLoginAttempts resets the login attempt counter (on successful login)
func (db *DB) ClearLoginAttempts(ctx context.Context, ip string) error {
	_, err := db.database.Collection("login_attempts").DeleteOne(ctx, bson.M{"ip": ip})
	return err
}

// GetAllLoginAttempts returns all login attempt records sorted by attempts descending.
func (db *DB) GetAllLoginAttempts(ctx context.Context) ([]LoginAttempt, error) {
	opts := options.Find().SetSort(bson.D{{Key: "attempts", Value: -1}})
	cursor, err := db.database.Collection("login_attempts").Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var attempts []LoginAttempt
	if err := cursor.All(ctx, &attempts); err != nil {
		return nil, err
	}
	return attempts, nil
}

// ClearLoginAttemptsByIP removes all login attempt records for a specific IP.
func (db *DB) ClearLoginAttemptsByIP(ctx context.Context, ip string) error {
	_, err := db.database.Collection("login_attempts").DeleteOne(ctx, bson.M{"ip": ip})
	return err
}

// IsLoginLocked checks if an IP is currently locked out
func (db *DB) IsLoginLocked(ctx context.Context, ip string) (bool, time.Duration) {
	attempt, err := db.GetLoginAttempts(ctx, ip)
	if err != nil || attempt.LockedUntil == nil {
		return false, 0
	}

	if time.Now().Before(*attempt.LockedUntil) {
		return true, time.Until(*attempt.LockedUntil)
	}
	return false, 0
}

// Folders returns the folders collection
func (db *DB) Folders() *mongo.Collection {
	return db.database.Collection("folders")
}

// Redirects returns the redirects collection
func (db *DB) Redirects() *mongo.Collection {
	return db.database.Collection("redirects")
}

// ContactMessages returns the contact_messages collection
func (db *DB) ContactMessages() *mongo.Collection {
	return db.database.Collection("contact_messages")
}

// GetRedirect looks up a redirect by source path
func (db *DB) GetRedirect(ctx context.Context, fromPath string) (*Redirect, error) {
	var redirect Redirect
	err := db.Redirects().FindOne(ctx, bson.M{"from_path": fromPath}).Decode(&redirect)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &redirect, err
}

// Redirect model stored locally for query results
type Redirect struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	FromPath    string             `bson:"from_path"`
	ToPath      string             `bson:"to_path"`
	StatusCode  int                `bson:"status_code"`
	Description string             `bson:"description"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}

// IsContactRateLimited checks if an IP has submitted too many contact forms recently
func (db *DB) IsContactRateLimited(ctx context.Context, ip string) (bool, error) {
	// Allow max 3 submissions per hour per IP
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	count, err := db.ContactMessages().CountDocuments(ctx, bson.M{
		"ip_address": ip,
		"created_at": bson.M{"$gte": oneHourAgo},
	})
	if err != nil {
		return false, err
	}
	return count >= 3, nil
}

// Assets returns the assets collection
func (db *DB) Assets() *mongo.Collection {
	return db.database.Collection("assets")
}

// Asset model stored locally for query results
type Asset struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Filename    string             `bson:"filename"`
	Folder      string             `bson:"folder"`
	FullPath    string             `bson:"full_path"`
	ServePath   string             `bson:"serve_path"` // URL path where file is served (e.g., /favicon.png)
	MimeType    string             `bson:"mime_type"`
	Size        int64              `bson:"size"`
	Data        []byte             `bson:"data"`
	Description string             `bson:"description"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}

// GetAssetByPath retrieves an asset by its full path
func (db *DB) GetAssetByPath(ctx context.Context, fullPath string) (*Asset, error) {
	var asset Asset
	err := db.Assets().FindOne(ctx, bson.M{"full_path": fullPath}).Decode(&asset)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &asset, err
}

// ListAssets returns all assets, optionally filtered by folder
func (db *DB) ListAssets(ctx context.Context, folder string) ([]*Asset, error) {
	filter := bson.M{}
	if folder != "" {
		filter["folder"] = folder
	}

	opts := options.Find().SetSort(bson.D{{Key: "folder", Value: 1}, {Key: "filename", Value: 1}}).SetProjection(bson.M{"data": 0})
	cursor, err := db.Assets().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var assets []*Asset
	if err := cursor.All(ctx, &assets); err != nil {
		return nil, err
	}
	return assets, nil
}

// SaveAsset creates or updates an asset
func (db *DB) SaveAsset(ctx context.Context, asset *Asset) error {
	now := time.Now()
	if asset.ID.IsZero() {
		asset.CreatedAt = now
	}
	asset.UpdatedAt = now

	_, err := db.Assets().UpdateOne(
		ctx,
		bson.M{"full_path": asset.FullPath},
		bson.M{"$set": asset},
		options.Update().SetUpsert(true),
	)
	return err
}

// MigrateAssetServePaths updates legacy assets to have serve_path set
func (db *DB) MigrateAssetServePaths(ctx context.Context) error {
	// Find all assets without serve_path or with empty serve_path
	cursor, err := db.Assets().Find(ctx, bson.M{
		"$or": []bson.M{
			{"serve_path": ""},
			{"serve_path": bson.M{"$exists": false}},
		},
	})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var asset Asset
		if err := cursor.Decode(&asset); err != nil {
			continue
		}

		// Set serve_path to /assets/ + full_path for legacy assets
		servePath := "/assets" + asset.FullPath
		_, err := db.Assets().UpdateOne(ctx,
			bson.M{"_id": asset.ID},
			bson.M{"$set": bson.M{"serve_path": servePath}},
		)
		if err != nil {
			fmt.Printf("Warning: Failed to migrate asset %s: %v\n", asset.Filename, err)
		} else {
			fmt.Printf("Migrated asset %s to serve_path %s\n", asset.Filename, servePath)
		}
	}
	return nil
}

// GetAsset retrieves an asset by ID
func (db *DB) GetAsset(ctx context.Context, id primitive.ObjectID) (*Asset, error) {
	var asset Asset
	err := db.Assets().FindOne(ctx, bson.M{"_id": id}).Decode(&asset)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &asset, err
}

func (db *DB) DeleteAsset(ctx context.Context, id primitive.ObjectID) error {
	_, err := db.Assets().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// GetAssetFolders returns a list of unique folder paths
func (db *DB) GetAssetFolders(ctx context.Context) ([]string, error) {
	results, err := db.Assets().Distinct(ctx, "folder", bson.M{})
	if err != nil {
		return nil, err
	}

	folders := make([]string, 0, len(results))
	for _, r := range results {
		if s, ok := r.(string); ok {
			folders = append(folders, s)
		}
	}
	return folders, nil
}

// APIKeys returns the api_keys collection
func (db *DB) APIKeys() *mongo.Collection {
	return db.database.Collection("api_keys")
}

// Users returns the users collection
func (db *DB) Users() *mongo.Collection {
	return db.database.Collection("users")
}

// AuditLogs returns the audit_logs collection
func (db *DB) AuditLogs() *mongo.Collection {
	return db.database.Collection("audit_logs")
}

// Collection returns a named collection (useful for tests and generic operations)
func (db *DB) Collection(name string) *mongo.Collection {
	return db.database.Collection(name)
}

// WatchCollection returns a change stream for the specified collection.
// The caller is responsible for closing the stream.
func (db *DB) WatchCollection(ctx context.Context, collection string, pipeline mongo.Pipeline) (*mongo.ChangeStream, error) {
	return db.database.Collection(collection).Watch(ctx, pipeline)
}
