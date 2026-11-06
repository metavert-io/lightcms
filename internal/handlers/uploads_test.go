package handlers

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// memFile is an in-memory multipart.File.
type memFile struct{ *bytes.Reader }

func (memFile) Close() error { return nil }

// useUploadDirs points UploadsDir and legacyUploadsDir at fresh temp
// directories for one test.
func useUploadDirs(t *testing.T) (uploads, legacy string) {
	t.Helper()
	oldUploads, oldLegacy := UploadsDir, legacyUploadsDir
	t.Cleanup(func() { UploadsDir, legacyUploadsDir = oldUploads, oldLegacy })
	base := t.TempDir()
	UploadsDir = filepath.Join(base, "content", "uploads") // created on first use
	legacyUploadsDir = filepath.Join(base, "static", "uploads")
	return UploadsDir, legacyUploadsDir
}

func TestSaveUpload_WritesToUploadsDir(t *testing.T) {
	uploads, _ := useUploadDirs(t)

	got, err := saveUpload(memFile{bytes.NewReader([]byte("png bytes"))}, "og_", "card.png")
	if err != nil {
		t.Fatalf("saveUpload: %v", err)
	}
	if !strings.HasPrefix(got, "/uploads/") || !strings.HasSuffix(got, "_og_card.png") {
		t.Fatalf("URL = %q, want /uploads/<nanos>_og_card.png", got)
	}
	data, err := os.ReadFile(filepath.Join(uploads, strings.TrimPrefix(got, "/uploads/")))
	if err != nil || string(data) != "png bytes" {
		t.Fatalf("stored file = %q, %v; want the uploaded bytes", data, err)
	}
}

func TestSaveUpload_ErrorWhenDirCannotBeCreated(t *testing.T) {
	useUploadDirs(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	UploadsDir = filepath.Join(blocker, "uploads") // under a regular file

	if got, err := saveUpload(memFile{bytes.NewReader([]byte("x"))}, "", "a.png"); err == nil {
		t.Fatalf("saveUpload = %q, nil; want an error", got)
	}
}

func TestMigrateLegacyUploads_CopiesOnlyMissingFiles(t *testing.T) {
	uploads, legacy := useUploadDirs(t)
	for _, dir := range []string{uploads, filepath.Join(legacy, "sub")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(legacy, "1_avatar.png"), "avatar")
	write(filepath.Join(legacy, "2_og_card.png"), "old card")
	write(filepath.Join(legacy, ".gitkeep"), "")
	write(filepath.Join(legacy, "sub", "3_nested.png"), "nested")
	write(filepath.Join(uploads, "2_og_card.png"), "newer card")

	n, err := MigrateLegacyUploads()
	if err != nil || n != 1 {
		t.Fatalf("MigrateLegacyUploads = %d, %v; want 1, nil", n, err)
	}
	if data, _ := os.ReadFile(filepath.Join(uploads, "1_avatar.png")); string(data) != "avatar" {
		t.Errorf("1_avatar.png = %q, want copied", data)
	}
	if data, _ := os.ReadFile(filepath.Join(uploads, "2_og_card.png")); string(data) != "newer card" {
		t.Errorf("2_og_card.png = %q, want the volume's copy left alone", data)
	}
	for _, name := range []string{".gitkeep", "sub"} {
		if _, err := os.Lstat(filepath.Join(uploads, name)); err == nil {
			t.Errorf("%s was copied; want dotfiles and folders skipped", name)
		}
	}

	if n, err := MigrateLegacyUploads(); err != nil || n != 0 {
		t.Errorf("second run = %d, %v; want 0, nil", n, err)
	}
}

func TestMigrateLegacyUploads_NoLegacyDir(t *testing.T) {
	useUploadDirs(t)
	if n, err := MigrateLegacyUploads(); err != nil || n != 0 {
		t.Fatalf("MigrateLegacyUploads = %d, %v; want 0, nil", n, err)
	}
}

// TestCreateContent_OGImageUploadGoesToUploadsDir checks the admin editor
// stores an uploaded OG image under UploadsDir (the persistent volume in
// production), not under static/.
func TestCreateContent_OGImageUploadGoesToUploadsDir(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()
	db := testDB(t)
	uploads, _ := useUploadDirs(t)
	tmplID := seedTemplate(t, db, "Upload Page", "upload-page")

	form := url.Values{
		"template_id": {tmplID.Hex()},
		"title":       {"OG Upload Page"},
		"slug":        {"og-upload-page"},
	}
	if rr := postMultipart(t, h.CreateContent, "og_image", "card.png", "png bytes", form, nil); rr.Code >= 500 {
		t.Fatalf("CreateContent: %d (%s)", rr.Code, rr.Body.String())
	}

	var doc struct {
		OGImage string `bson:"og_image"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.FindOne(ctx, "content", bson.M{"slug": "og-upload-page"}, &doc); err != nil {
		t.Fatalf("content not created: %v", err)
	}
	if !strings.HasPrefix(doc.OGImage, "/uploads/") {
		t.Fatalf("og_image = %q, want an /uploads/ URL", doc.OGImage)
	}
	data, err := os.ReadFile(filepath.Join(uploads, strings.TrimPrefix(doc.OGImage, "/uploads/")))
	if err != nil || string(data) != "png bytes" {
		t.Fatalf("uploaded file under UploadsDir = %q, %v; want the uploaded bytes", data, err)
	}
}
