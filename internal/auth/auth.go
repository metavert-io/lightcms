package auth

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/middleware"
	"github.com/metavert-io/lightcms/v7/internal/models"
	"github.com/metavert-io/lightcms/v7/internal/services"

	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionName     = "lightcms-session"
	defaultPassword = "admin123"
	bcryptCost      = 12
)

type Manager struct {
	db          *database.DB
	store       *sessions.CookieStore
	proxyConfig *middleware.TrustedProxyConfig
	userService *services.UserService
}

func NewManager(store *sessions.CookieStore, db *database.DB, userService *services.UserService) *Manager {
	return &Manager{
		db:          db,
		store:       store,
		proxyConfig: middleware.DefaultCloudConfig(),
		userService: userService,
	}
}

// NewManagerWithProxyConfig creates a new auth manager with custom proxy configuration
func NewManagerWithProxyConfig(store *sessions.CookieStore, db *database.DB, userService *services.UserService, proxyConfig *middleware.TrustedProxyConfig) *Manager {
	return &Manager{
		db:          db,
		store:       store,
		proxyConfig: proxyConfig,
		userService: userService,
	}
}

// MigrateToMultiUser migrates the legacy single-admin system to multi-user.
// If the users collection is empty, creates the first admin user from the existing password hash.
func (m *Manager) MigrateToMultiUser(ctx context.Context) error {
	count, err := m.userService.UserCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // already migrated
	}

	// Get existing admin password hash
	settings, err := m.db.GetAdminSettings(ctx)
	if err != nil {
		return err
	}

	passwordHash := ""
	isDefault := true
	if settings != nil && settings.PasswordHash != "" {
		passwordHash = settings.PasswordHash
		isDefault = settings.IsDefaultPassword
	} else {
		// No existing password — create default hash
		hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcryptCost)
		if err != nil {
			return err
		}
		passwordHash = string(hash)
	}

	// Determine admin email from environment or use default
	adminEmail := strings.TrimSpace(os.Getenv("LIGHTCMS_ADMIN_EMAIL"))
	if adminEmail == "" {
		adminEmail = "admin@localhost"
	}

	user, err := m.userService.CreateUserWithHash(ctx, adminEmail, "Admin", models.RoleAdmin, passwordHash, isDefault)
	if err != nil {
		return err
	}

	log.Printf("[AUTH] Migrated to multi-user system: created admin user %s (ID: %s)", adminEmail, user.ID.Hex())
	return nil
}

// ValidateCredentials checks email+password and returns the user
func (m *Manager) ValidateCredentials(ctx context.Context, email, password string) (*models.User, error) {
	return m.userService.ValidateCredentials(ctx, email, password)
}

// GetUserByID returns a user account, or nil if there is none.
func (m *Manager) GetUserByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	return m.userService.GetByID(ctx, id)
}

// ChangePassword updates a user's password via the user service
func (m *Manager) ChangePassword(ctx context.Context, userID primitive.ObjectID, currentPassword, newPassword string) error {
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}
	return m.userService.ChangePassword(ctx, userID, currentPassword, newPassword)
}

// CheckRateLimit checks if the IP is rate limited
func (m *Manager) CheckRateLimit(ctx context.Context, r *http.Request) (bool, string) {
	ip := m.getClientIP(r)
	locked, duration := m.db.IsLoginLocked(ctx, ip)
	if locked {
		minutes := int(duration.Minutes())
		seconds := int(duration.Seconds()) % 60
		if minutes > 0 {
			return true, formatDuration(minutes, seconds)
		}
		return true, formatDuration(0, seconds)
	}
	return false, ""
}

// RecordFailedLogin records a failed login attempt
func (m *Manager) RecordFailedLogin(ctx context.Context, r *http.Request) {
	ip := m.getClientIP(r)
	m.db.RecordFailedLogin(ctx, ip)
}

// ClearRateLimit clears rate limiting on successful login
func (m *Manager) ClearRateLimit(ctx context.Context, r *http.Request) {
	ip := m.getClientIP(r)
	m.db.ClearLoginAttempts(ctx, ip)
}

// LoginUser creates a session for an authenticated user
func (m *Manager) LoginUser(w http.ResponseWriter, r *http.Request, user *models.User) error {
	session, err := m.store.Get(r, sessionName)
	if err != nil {
		session = sessions.NewSession(m.store, sessionName)
		session.Options = m.store.Options
		session.IsNew = true
	}
	session.Values["authenticated"] = true
	session.Values["user_id"] = user.ID.Hex()
	session.Values["user_email"] = user.Email
	session.Values["user_role"] = user.Role
	session.Values["force_password_change"] = user.IsDefaultPassword
	return session.Save(r, w)
}

func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) error {
	session, err := m.store.Get(r, sessionName)
	if err != nil {
		return err
	}
	session.Values["authenticated"] = false
	delete(session.Values, "user_id")
	delete(session.Values, "user_email")
	delete(session.Values, "user_role")
	delete(session.Values, "force_password_change")
	session.Options.MaxAge = -1
	return session.Save(r, w)
}

func (m *Manager) IsAuthenticated(r *http.Request) bool {
	session, err := m.store.Get(r, sessionName)
	if err != nil {
		return false
	}
	auth, ok := session.Values["authenticated"].(bool)
	return ok && auth
}

// GetCurrentUser extracts the authenticated user from the session
func (m *Manager) GetCurrentUser(r *http.Request) (*SessionUser, bool) {
	session, err := m.store.Get(r, sessionName)
	if err != nil {
		return nil, false
	}
	auth, ok := session.Values["authenticated"].(bool)
	if !ok || !auth {
		return nil, false
	}
	id, _ := session.Values["user_id"].(string)
	email, _ := session.Values["user_email"].(string)
	role, _ := session.Values["user_role"].(string)
	if id == "" || email == "" || role == "" {
		return nil, false
	}
	return &SessionUser{ID: id, Email: email, Role: role}, true
}

// MustChangePassword returns true if the session user must change their password before proceeding
func (m *Manager) MustChangePassword(r *http.Request) bool {
	session, err := m.store.Get(r, sessionName)
	if err != nil {
		return false
	}
	force, _ := session.Values["force_password_change"].(bool)
	return force
}

// ClearForcePasswordChange removes the force_password_change flag from the session
func (m *Manager) ClearForcePasswordChange(w http.ResponseWriter, r *http.Request) error {
	session, err := m.store.Get(r, sessionName)
	if err != nil {
		return err
	}
	session.Values["force_password_change"] = false
	return session.Save(r, w)
}

// RequireAuth wraps handlers that require authentication
func (m *Manager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.IsAuthenticated(r) {
			http.Redirect(w, r, "/cm/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// Password validation errors
type PasswordError struct {
	Message string
}

func (e PasswordError) Error() string {
	return e.Message
}

var (
	ErrInvalidCurrentPassword = PasswordError{"Current password is incorrect"}
	ErrPasswordTooShort       = PasswordError{"Password must be at least 8 characters"}
	ErrPasswordNoUppercase    = PasswordError{"Password must contain at least one uppercase letter"}
	ErrPasswordNoLowercase    = PasswordError{"Password must contain at least one lowercase letter"}
	ErrPasswordNoNumber       = PasswordError{"Password must contain at least one number"}
)

// ValidatePasswordStrength checks password meets security requirements
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}

	hasUpper := false
	hasLower := false
	hasNumber := false

	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasNumber = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUppercase
	}
	if !hasLower {
		return ErrPasswordNoLowercase
	}
	if !hasNumber {
		return ErrPasswordNoNumber
	}

	return nil
}

// getClientIP extracts the client IP address using the configured proxy settings
func (m *Manager) getClientIP(r *http.Request) string {
	return middleware.GetClientIP(r, m.proxyConfig)
}

func formatDuration(minutes, seconds int) string {
	if minutes > 0 {
		return itoa(minutes) + " minute" + pluralize(minutes)
	}
	return itoa(seconds) + " second" + pluralize(seconds)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
