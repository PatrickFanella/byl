package blob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFSStorePutDeletePublicURL(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFS(dir, "http://localhost:8080/media/files")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "images/a b.jpg", "image/jpeg", strings.NewReader("jpegdata"), 8); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "images", "a b.jpg"))
	if err != nil || string(data) != "jpegdata" {
		t.Fatalf("file: %v %q", err, data)
	}
	if got := s.PublicURL("images/a b.jpg"); got != "http://localhost:8080/media/files/images/a%20b.jpg" {
		t.Fatalf("url %q", got)
	}
	if err := s.Put(ctx, "../escape.txt", "text/plain", strings.NewReader("x"), 1); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if err := s.Delete(ctx, "images/a b.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "images", "a b.jpg")); !os.IsNotExist(err) {
		t.Fatal("file should be gone")
	}
	if err := s.Delete(ctx, "images/missing.jpg"); err != nil {
		t.Fatalf("deleting a missing key is not an error: %v", err)
	}
}

func TestKeyFor(t *testing.T) {
	k := KeyFor("images", "My Photo.JPG")
	if !strings.HasPrefix(k, "images/") || !strings.HasSuffix(k, ".jpg") || strings.Contains(k, " ") {
		t.Fatalf("key %q", k)
	}
	if KeyFor("images", "My Photo.JPG") == k {
		t.Fatal("keys must be unique per call")
	}
}
