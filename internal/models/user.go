package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents a CMS user account
type User struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email             string             `bson:"email" json:"email"`
	DisplayName       string             `bson:"display_name" json:"display_name"`
	PasswordHash      string             `bson:"password_hash" json:"-"`
	Role              string             `bson:"role" json:"role"` // "admin", "editor", "viewer"
	IsDefaultPassword bool               `bson:"is_default_password" json:"is_default_password"`
	Disabled          bool               `bson:"disabled" json:"disabled"`
	LastLoginAt       *time.Time         `bson:"last_login_at,omitempty" json:"last_login_at,omitempty"`
	CreatedBy         primitive.ObjectID `bson:"created_by,omitempty" json:"created_by,omitempty"`
	CreatedAt         time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time          `bson:"updated_at" json:"updated_at"`
}

// AuditLog records a security or content event
type AuditLog struct {
	ID         primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID     `bson:"user_id" json:"user_id"`
	UserEmail  string                 `bson:"user_email" json:"user_email"`
	ViaAPI     bool                   `bson:"via_api,omitempty" json:"via_api,omitempty"` // true when action was performed via API key
	Action     string                 `bson:"action" json:"action"`                       // e.g. "content.create", "login.success"
	Resource   string                 `bson:"resource" json:"resource"`                   // e.g. "content", "template", "user"
	ResourceID string                 `bson:"resource_id,omitempty" json:"resource_id,omitempty"`
	Details    map[string]interface{} `bson:"details,omitempty" json:"details,omitempty"`
	IPAddress  string                 `bson:"ip_address,omitempty" json:"ip_address,omitempty"`
	// AgentSession groups all changes made by one AI-agent session (from the
	// X-Agent-Session request header) so they can be reviewed and rolled
	// back as a unit.
	AgentSession string `bson:"agent_session,omitempty" json:"agent_session,omitempty"`
	// Credential names the API key ("apikey:<id>") or OAuth connection
	// ("oauth:<grant id>") a change was made with.
	Credential string    `bson:"credential,omitempty" json:"credential,omitempty"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
}

// Role constants
const (
	RoleAdmin       = "admin"
	RoleEditor      = "editor"
	RoleContributor = "contributor" // Can create content (→ approval queue); upload assets (→ review); post comments
	RoleViewer      = "viewer"
)
