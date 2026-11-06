package handlers

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UploadsDir is where the admin editor's uploads (image fields and OG images)
// are written, and where /uploads/ serves from, relative to the working
// directory. It is under content/, the persistent volume in production.
// static/ is part of the image and is replaced on every deploy, which lost
// these files until 7.4.6. A variable so the tests point it at a temp
// directory.
var UploadsDir = "content/uploads"

// legacyUploadsDir is where the editor wrote uploads before 7.4.6. A deploy
// can still carry files there in the image; MigrateLegacyUploads copies them
// into UploadsDir at startup.
var legacyUploadsDir = "static/uploads"

// saveUpload writes an uploaded file to UploadsDir as <unix nanos>_<prefix><name>
// and returns the URL it is served at.
func saveUpload(file multipart.File, prefix, name string) (string, error) {
	if err := os.MkdirAll(UploadsDir, 0755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), prefix, name)
	path := filepath.Join(UploadsDir, filename)
	dst, err := os.Create(path)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(dst, file)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return "/uploads/" + filename, nil
}

// MigrateLegacyUploads copies the files in the pre-7.4.6 uploads directory
// that UploadsDir does not have yet, so uploads an image still carries keep
// being served. It never overwrites a file and returns how many it copied.
func MigrateLegacyUploads() (int, error) {
	entries, err := os.ReadDir(legacyUploadsDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(UploadsDir, 0755); err != nil {
		return 0, err
	}
	copied := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dst := filepath.Join(UploadsDir, e.Name())
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if err := copyNewFile(filepath.Join(legacyUploadsDir, e.Name()), dst); err != nil {
			return copied, fmt.Errorf("copy %s: %w", e.Name(), err)
		}
		copied++
	}
	return copied, nil
}

// copyNewFile copies src to dst, failing if dst already exists.
func copyNewFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
