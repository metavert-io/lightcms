package testutil

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"

	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const testDBName = "lightcms-test"

// sharedConn holds a single MongoDB connection shared across all tests in a
// package. Creating a new Atlas connection + running createIndexes (~30 round-
// trips) per test function was causing the services package (215 tests) to
// exceed the 900s CI timeout. With a shared connection, createIndexes runs once
// per package binary invocation and CleanupCollections (DeleteMany) is fast.
var (
	sharedOnce sync.Once
	sharedDB   *database.DB
	sharedErr  error
)

// loadEnvTest loads the .env.test file into the process environment.
func loadEnvTest() {
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		envPath := filepath.Join(dir, ".env.test")
		data, err := os.ReadFile(envPath)
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.IndexByte(line, '='); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					val := strings.TrimSpace(line[idx+1:])
					os.Setenv(key, val)
				}
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

// MustConnectTestDB returns a shared test database connection. The connection is
// created once per package binary using sync.Once; subsequent calls reuse it.
// Each call wipes all test collections via DeleteMany for isolation.
// The returned cleanup function wipes collections again at test teardown.
func MustConnectTestDB(t *testing.T) (*database.DB, func()) {
	t.Helper()

	// Initialise the shared connection exactly once per test binary.
	sharedOnce.Do(func() {
		loadEnvTest()

		uri := os.Getenv("MONGODB_URI")
		if uri == "" {
			return // sharedDB stays nil; caller will Skip
		}

		dbName := os.Getenv("DATABASE_NAME")
		if dbName == "" {
			dbName = testDBName
		}

		if !strings.Contains(strings.ToLower(dbName), "test") {
			sharedErr = fmt.Errorf("REFUSING to run tests — database name %q does not contain 'test'. "+
				"This safety guard prevents accidental use of production databases.", dbName)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		wc := writeconcern.New(writeconcern.WMajority())
		wcOpts := options.Client().SetWriteConcern(wc)
		db, err := database.Connect(ctx, uri, dbName, wcOpts)
		if err != nil {
			sharedErr = fmt.Errorf("failed to connect to test database: %w", err)
			return
		}
		sharedDB = db
	})

	if sharedErr != nil {
		t.Fatalf("testutil: %v", sharedErr)
	}
	if sharedDB == nil {
		t.Skip("skipping: MONGODB_URI not set (no .env.test file)")
	}

	// Wipe all collections before the test runs.
	CleanupCollections(t, sharedDB)

	// Return a no-op cleanup — the next test's MustConnectTestDB call will
	// wipe collections before it runs. Cleaning at both start and end doubled
	// the number of Atlas round-trips and caused the services package (118+
	// tests × 20 collections × 2) to exceed the CI timeout.
	cleanup := func() {}

	return sharedDB, cleanup
}

var (
	brokenOnce sync.Once
	brokenDB   *database.DB
)

// MustConnectBrokenDB returns a database connection that has been disconnected,
// so every operation returns an error. It lets tests exercise the database-error
// branches of service methods. Skips if MONGODB_URI is not set.
func MustConnectBrokenDB(t *testing.T) *database.DB {
	t.Helper()
	brokenOnce.Do(func() {
		loadEnvTest()
		uri := os.Getenv("MONGODB_URI")
		if uri == "" {
			return
		}
		dbName := os.Getenv("DATABASE_NAME")
		if dbName == "" {
			dbName = testDBName
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := database.Connect(ctx, uri, dbName, options.Client().SetWriteConcern(writeconcern.New(writeconcern.WMajority())))
		if err != nil {
			return
		}
		_ = db.Disconnect(context.Background())
		brokenDB = db
	})
	if brokenDB == nil {
		t.Skip("skipping: MONGODB_URI not set")
	}
	return brokenDB
}

var (
	faultOnce sync.Once
	faultDB   *database.DB
)

// MustConnectFaultDB returns a live database connection (separate from the
// shared test connection) on which tests may install a fault hook via
// db.SetFaultHook to make specific operations fail. Seed data with the hook
// cleared, install the hook, exercise the error branch, then clear the hook.
// Skips if MONGODB_URI is not set.
func MustConnectFaultDB(t *testing.T) *database.DB {
	t.Helper()
	faultOnce.Do(func() {
		loadEnvTest()
		uri := os.Getenv("MONGODB_URI")
		if uri == "" {
			return
		}
		dbName := os.Getenv("DATABASE_NAME")
		if dbName == "" {
			dbName = testDBName
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := database.Connect(ctx, uri, dbName, options.Client().SetWriteConcern(writeconcern.New(writeconcern.WMajority())))
		if err != nil {
			return
		}
		faultDB = db
	})
	if faultDB == nil {
		t.Skip("skipping: MONGODB_URI not set")
	}
	faultDB.SetFaultHook(nil) // ensure a clean slate
	return faultDB
}

// FailOp returns a fault hook that fails every call to the named operation
// (e.g. "UpdateOne", "InsertOne"), letting all other operations succeed. This
// exercises multi-step error branches where an initial read succeeds but a
// subsequent write fails.
func FailOp(op string) func(string, string) error {
	return func(o, _ string) error {
		if o == op {
			return fmt.Errorf("testutil: injected failure for %s", op)
		}
		return nil
	}
}

// CleanupCollections drops test collections for isolation between tests.
// Dropping ensures unique indexes (e.g. users.email) are also cleared, preventing
// duplicate key errors when MigrateToMultiUser runs in subsequent tests.
func CleanupCollections(t *testing.T, db *database.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collections := []string{
		"content", "content_versions", "templates", "folders", "collections",
		"custom_pages", "settings", "theme_versions", "contact_messages",
		"login_attempts", "assets", "api_keys", "redirects", "snippets",
		"users", "audit_logs", "user_activity",
		"oauth_clients", "oauth_grants", "oauth_auth_codes", "oauth_access_tokens", "oauth_refresh_tokens",
		"content_comments", "content_locks", "webhooks", "webhook_deliveries",
		"content_forks", "approval_workflows", "approval_requests",
		"import_sources", "import_jobs", "link_check_jobs", "regen_jobs",
		"maintenance_reports", "agent_digests",
	}
	for _, name := range collections {
		db.Collection(name).Drop(ctx) //nolint:errcheck
	}
}

// ParseJSON decodes a response body into the target struct.
func ParseJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("testutil: failed to parse JSON response: %v", err)
	}
}

// ReadResponseBody reads and returns the full response body as a string.
func ReadResponseBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
	}
	return sb.String()
}

// SnapshotTrackedFiles records the content of every git-tracked file under
// the working directory (a test binary runs in its package directory) and
// returns a function that lists the ones that have since changed or gone.
// A package's TestMain calls it around m.Run() so a test that rewrites a
// tracked file fails the suite instead of leaving the working tree dirty.
// Without git (or outside a checkout) the returned function reports nothing.
func SnapshotTrackedFiles() (changed func() []string) {
	out, err := exec.Command("git", "ls-files", "-z", "--", ".").Output()
	if err != nil {
		return func() []string { return nil }
	}
	hash := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			return "missing"
		}
		return fmt.Sprintf("%x", sha256.Sum256(data))
	}
	before := map[string]string{}
	for _, path := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if path != "" {
			before[path] = hash(path)
		}
	}
	return func() []string {
		var dirty []string
		for path, was := range before {
			if was != "missing" && hash(path) != was {
				dirty = append(dirty, path)
			}
		}
		sort.Strings(dirty)
		return dirty
	}
}
