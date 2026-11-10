package auth

import (
	"context"

	"github.com/metavert-io/lightcms/v7/internal/middleware"
)

// UserFromAPIContext extracts the authenticated SessionUser from an API request context.
// The middleware stores the user as interface{}; this provides the typed accessor.
func UserFromAPIContext(ctx context.Context) (*SessionUser, bool) {
	val, ok := middleware.APIUserFromContext(ctx)
	if !ok {
		return nil, false
	}
	user, ok := val.(*SessionUser)
	return user, ok
}

// Permission constants for RBAC
const (
	// Content
	PermContentView    = "content.view"
	PermContentCreate  = "content.create"
	PermContentEdit    = "content.edit"
	PermContentDelete  = "content.delete"
	PermContentPublish = "content.publish"

	// Templates
	PermTemplateView   = "template.view"
	PermTemplateCreate = "template.create"
	PermTemplateEdit   = "template.edit"
	PermTemplateDelete = "template.delete"

	// Assets
	PermAssetView   = "asset.view"
	PermAssetUpload = "asset.upload"
	PermAssetDelete = "asset.delete"

	// Settings (theme, config, redirects, folders, collections)
	PermSettingsView = "settings.view"
	PermSettingsEdit = "settings.edit"

	// Search & replace
	PermSearchReplace = "search_replace.execute"

	// API keys
	PermAPIKeyManage    = "apikey.manage"     // manage own keys
	PermAPIKeyManageAll = "apikey.manage_all" // manage all keys

	// User management
	PermUserManage = "user.manage"

	// Audit logs
	PermAuditView = "audit.view"

	// Content forks
	PermForkCreate = "fork.create" // create + edit fork pages (editor+)
	PermForkMerge  = "fork.merge"  // merge fork into live (admin only)

	// Approvals & discussions
	PermContentSubmitApproval   = "content.submit_approval"   // submit content for approval (contributor+)
	PermApprovalView            = "approval.view"             // view approval requests
	PermApprovalDecide          = "approval.decide"           // approve or reject requests (editor+)
	PermApprovalManageWorkflows = "approval.manage_workflows" // create/edit/delete workflows (editor+)
	PermDiscussionPost          = "discussion.post"           // post comments (contributor+)
	PermCommentDelete           = "comment.delete"            // delete any comment (admin only)
)

// RolePermissions maps each role to its allowed permissions
var RolePermissions = map[string][]string{
	"viewer": {
		PermContentView,
		PermTemplateView,
		PermAssetView,
		PermSettingsView,
	},
	"contributor": {
		PermContentView, PermContentCreate,
		PermContentSubmitApproval,
		PermApprovalView,
		PermDiscussionPost,
		PermTemplateView,
		PermAssetView, PermAssetUpload, // uploads flagged pending_review
		PermSettingsView,
		PermAPIKeyManage,
	},
	"editor": {
		PermContentView, PermContentCreate, PermContentEdit, PermContentDelete, PermContentPublish,
		PermContentSubmitApproval,
		PermApprovalView, PermApprovalDecide, PermApprovalManageWorkflows,
		PermDiscussionPost,
		PermTemplateView,
		PermAssetView, PermAssetUpload, PermAssetDelete,
		PermSettingsView,
		PermAPIKeyManage,
		PermForkCreate,
	},
	"admin": {
		PermContentView, PermContentCreate, PermContentEdit, PermContentDelete, PermContentPublish,
		PermContentSubmitApproval,
		PermApprovalView, PermApprovalDecide, PermApprovalManageWorkflows,
		PermDiscussionPost, PermCommentDelete,
		PermTemplateView, PermTemplateCreate, PermTemplateEdit, PermTemplateDelete,
		PermAssetView, PermAssetUpload, PermAssetDelete,
		PermSettingsView, PermSettingsEdit,
		PermSearchReplace,
		PermAPIKeyManage, PermAPIKeyManageAll,
		PermUserManage,
		PermAuditView,
		PermForkCreate, PermForkMerge,
	},
}

// HasPermission checks if a role has the specified permission
func HasPermission(role, perm string) bool {
	perms, ok := RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// SessionUser represents the authenticated user extracted from a session or API context
type SessionUser struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	ViaAPIKey   bool     `json:"via_api_key,omitempty"`  // true when authenticated via API key (not session)
	ViaOAuth    bool     `json:"via_oauth,omitempty"`    // true when authenticated via an OAuth connection (not session)
	Credential  string   `json:"credential,omitempty"`   // the API key or OAuth connection in use: "apikey:<id>" or "oauth:<grant id>"
	Scopes      []string `json:"scopes,omitempty"`       // API-key permission allowlist; empty = full role permissions
	SandboxOnly bool     `json:"sandbox_only,omitempty"` // key or connection confined to forks it created
	// CanMergeOwnForks: the key or connection may merge forks it created.
	CanMergeOwnForks bool `json:"can_merge_own_forks,omitempty"`
	// ActiveSandboxFork is the fork of the credential's current sandbox
	// (hex ID), or "" when none is active.
	ActiveSandboxFork string `json:"active_sandbox_fork,omitempty"`
}

// CredentialKey implements middleware.CredentialBound.
func (u *SessionUser) CredentialKey() string { return u.Credential }

// Sandboxed reports whether the caller is confined to forks it created right
// now: a key or connection set to Sandbox always, and any other while it has a
// sandbox open.
func (u *SessionUser) Sandboxed() bool {
	return u != nil && (u.SandboxOnly || u.ActiveSandboxFork != "")
}

// sandboxAllowedPerms is the permission set available to a sandboxed caller:
// reading, creating/editing content (in its own forks, enforced by the
// content handlers), uploading new non-executable assets, and working with
// its own forks. Everything that mutates the live site directly is excluded.
// The REST layer also refuses every write route not listed in
// handlers.sandboxWriteRoutes.
var sandboxAllowedPerms = map[string]bool{
	PermContentView:    true,
	PermContentCreate:  true,
	PermContentEdit:    true,
	PermTemplateView:   true,
	PermAssetView:      true,
	PermAssetUpload:    true,
	PermSettingsView:   true,
	PermForkCreate:     true,
	PermDiscussionPost: true,
}

// UserHasPermission checks a permission against the user's role, then
// narrows by the API key's scopes (when set) and sandbox-only restriction.
func UserHasPermission(u *SessionUser, perm string) bool {
	if u.Sandboxed() && !sandboxAllowedPerms[perm] {
		return false
	}
	return RoleAndScopesAllow(u, perm)
}

// RoleAndScopesAllow checks a permission against the user's role and the
// API key's scopes, leaving the sandbox out. It is for the one step a
// separate grant opens to a sandboxed caller: merging a fork it created
// when CanMergeOwnForks is set.
func RoleAndScopesAllow(u *SessionUser, perm string) bool {
	if u == nil || !HasPermission(u.Role, perm) {
		return false
	}
	if len(u.Scopes) > 0 {
		for _, s := range u.Scopes {
			if s == perm {
				return true
			}
		}
		return false
	}
	return true
}

// IsKnownPermission reports whether perm is one of the defined permission
// constants — used to validate API-key scope allowlists at creation time.
func IsKnownPermission(perm string) bool {
	for _, perms := range RolePermissions {
		for _, p := range perms {
			if p == perm {
				return true
			}
		}
	}
	return false
}
