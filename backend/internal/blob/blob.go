// Package blob stores media files.
package blob

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"path"
	"strings"

	"invisible-museum/backend/internal/geo"
)

// Store persists files under keys and serves them at public URLs.
type Store interface {
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	Delete(ctx context.Context, key string) error
	PublicURL(key string) string
}

// KeyFor builds a unique, URL-safe key under prefix that keeps the file's extension.
func KeyFor(prefix, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	base := geo.Slugify(strings.TrimSuffix(filename, path.Ext(filename)))
	if base == "" {
		base = "file"
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return path.Join(prefix, base+"-"+hex.EncodeToString(b)+ext)
}
