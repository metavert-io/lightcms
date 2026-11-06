package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/metavert-io/lightcms/v7/internal/services"
	"github.com/metavert-io/lightcms/v7/internal/testutil"
)

// TestMain keeps the suite from touching tracked files. The handlers write
// the sitemap and the theme CSS relative to the working directory, which for
// this package's tests is internal/handlers — where static/sitemap.xml and
// static/css/theme-vars.css are checked in. Both are pointed at a temp
// directory, and the run fails if any tracked file under the package has
// changed by the end (the equivalent of a dirty `git status`).
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "lightcms-handlers-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		os.Exit(1)
	}
	sitemapFile = filepath.Join(tmp, "sitemap.xml")
	services.ThemeCSSFile = filepath.Join(tmp, "css", "theme-vars.css")
	UploadsDir = filepath.Join(tmp, "uploads")
	legacyUploadsDir = filepath.Join(tmp, "legacy-uploads")
	changed := testutil.SnapshotTrackedFiles()

	code := m.Run()

	os.RemoveAll(tmp)
	if dirty := changed(); len(dirty) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: the test run modified tracked files (tests must write to a temp directory): %v\n", dirty)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
