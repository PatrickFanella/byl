package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FS stores files in a local directory.
type FS struct {
	dir        string
	publicBase string
}

// NewFS creates the directory if needed.
func NewFS(dir, publicBase string) (*FS, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &FS{dir: abs, publicBase: strings.TrimRight(publicBase, "/")}, nil
}

// Dir returns the root directory.
func (f *FS) Dir() string { return f.dir }

func (f *FS) pathFor(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("blob: invalid key %q", key)
	}
	full := filepath.Join(f.dir, clean)
	if !strings.HasPrefix(full, f.dir+string(filepath.Separator)) {
		return "", fmt.Errorf("blob: invalid key %q", key)
	}
	return full, nil
}

func (f *FS) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	full, err := f.pathFor(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), full)
}

func (f *FS) Delete(_ context.Context, key string) error {
	full, err := f.pathFor(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (f *FS) PublicURL(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return f.publicBase + "/" + strings.Join(parts, "/")
}
