package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/metavert-io/lightcms/v7/internal/database"
	"github.com/metavert-io/lightcms/v7/internal/testutil"
)

func TestAssetExistsAt_SeesEveryWayTheSiteServesAFile(t *testing.T) {
	db, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	t.Chdir(t.TempDir())
	ctx := context.Background()
	assets := NewAssetService(db)

	// A file on disk with no record.
	if err := os.MkdirAll(filepath.Join("content", "generated", "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("content", "generated", "images", "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A record from before uploads dropped the /assets prefix.
	if err := db.SaveAsset(ctx, &database.Asset{Filename: "old.png", FullPath: "/assets/old.png", ServePath: "/assets/old.png", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveAsset: %v", err)
	}

	for path, want := range map[string]bool{
		"/images/logo.png":        true,
		"/assets/images/logo.png": true,
		"/assets/old.png":         true,
		"/assets/new.png":         false,
	} {
		got, err := assets.AssetExistsAt(ctx, path)
		if err != nil || got != want {
			t.Errorf("AssetExistsAt(%q) = %v, %v; want %v", path, got, err, want)
		}
	}
}
