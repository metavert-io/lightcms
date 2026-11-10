package auth

import (
	"context"
	"fmt"
	"log"

	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"
)

// CredentialResolver turns an API key or OAuth access token into the person it
// acts for. It runs on every request, so a change of role, a disabled account
// or a revoked connection takes effect at once, and any doubt about who is
// behind a credential refuses the request.
type CredentialResolver struct {
	users      *services.UserService
	apiKeys    *services.APIKeyService
	oauth      *services.OAuthService
	resource   string
	onActivity func(userID string)
}

// NewCredentialResolver creates a resolver. resource is the identifier OAuth
// tokens must have been issued for. onActivity, if set, is called with each
// authenticated user's ID.
func NewCredentialResolver(users *services.UserService, apiKeys *services.APIKeyService, oauth *services.OAuthService, resource string, onActivity func(userID string)) *CredentialResolver {
	return &CredentialResolver{users: users, apiKeys: apiKeys, oauth: oauth, resource: resource, onActivity: onActivity}
}

// ResolveAPIKey implements middleware.APIKeyValidateFunc.
func (c *CredentialResolver) ResolveAPIKey(ctx context.Context, rawKey string) (interface{}, error) {
	key, err := c.apiKeys.ValidateAPIKey(ctx, rawKey)
	if err != nil {
		return nil, err
	}
	owner, err := c.keyOwner(ctx, key)
	if err != nil {
		return nil, err
	}
	c.recordActivity(owner)
	return &SessionUser{
		ID:          owner.ID.Hex(),
		Email:       owner.Email,
		Role:        owner.Role,
		ViaAPIKey:   true,
		Credential:  "apikey:" + key.ID.Hex(),
		Scopes:      key.Scopes,
		SandboxOnly: key.SandboxOnly,
	}, nil
}

// ResolveOAuthToken implements middleware.OAuthValidateFunc.
func (c *CredentialResolver) ResolveOAuthToken(ctx context.Context, rawToken string) (interface{}, error) {
	grant, err := c.oauth.ValidateAccessToken(ctx, rawToken, c.resource)
	if err != nil {
		return nil, err
	}
	user, err := c.users.GetByID(ctx, grant.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading connection owner: %w", err)
	}
	if user == nil || user.Disabled {
		return nil, fmt.Errorf("connection owner is missing or disabled")
	}
	c.recordActivity(user)
	return &SessionUser{
		ID:         user.ID.Hex(),
		Email:      user.Email,
		Role:       user.Role,
		ViaOAuth:   true,
		Credential: "oauth:" + grant.ID.Hex(),
	}, nil
}

// keyOwner returns the active user an API key belongs to. A key with no owner
// on record predates ownership and is given to the first active admin, once;
// it keeps its restrictions. A key whose owner cannot be loaded is refused,
// whatever the reason: a passing database error must not change who it acts as.
func (c *CredentialResolver) keyOwner(ctx context.Context, key *models.APIKey) (*models.User, error) {
	if key.UserID == nil {
		return c.adoptOwnerlessKey(ctx, key)
	}
	owner, err := c.users.GetByID(ctx, *key.UserID)
	if err != nil {
		return nil, fmt.Errorf("loading API key owner: %w", err)
	}
	if owner == nil || owner.Disabled {
		return nil, fmt.Errorf("API key owner is missing or disabled")
	}
	return owner, nil
}

func (c *CredentialResolver) adoptOwnerlessKey(ctx context.Context, key *models.APIKey) (*models.User, error) {
	users, err := c.users.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing users for an unowned API key: %w", err)
	}
	for i := range users {
		admin := &users[i]
		if admin.Role != models.RoleAdmin || admin.Disabled {
			continue
		}
		if err := c.apiKeys.AssignOwner(ctx, key.ID, admin.ID); err != nil {
			return nil, err
		}
		log.Printf("[security] API key %q had no owner; assigned to admin %s", key.Name, admin.Email)
		return admin, nil
	}
	return nil, fmt.Errorf("API key has no owner and no admin is available to take it")
}

func (c *CredentialResolver) recordActivity(user *models.User) {
	if c.onActivity != nil {
		c.onActivity(user.ID.Hex())
	}
}
