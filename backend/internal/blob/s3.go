package blob

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config configures an S3-compatible store (MinIO, R2, S3).
type S3Config struct {
	Endpoint   string
	Bucket     string
	AccessKey  string
	SecretKey  string
	UseSSL     bool
	PublicBase string
}

// S3 stores files in a bucket.
type S3 struct {
	client     *minio.Client
	bucket     string
	publicBase string
}

// NewS3 connects to the endpoint. The bucket must already exist.
func NewS3(cfg S3Config) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("blob: s3: %w", err)
	}
	base := strings.TrimRight(cfg.PublicBase, "/")
	if base == "" {
		scheme := "http"
		if cfg.UseSSL {
			scheme = "https"
		}
		base = fmt.Sprintf("%s://%s/%s", scheme, cfg.Endpoint, cfg.Bucket)
	}
	return &S3{client: client, bucket: cfg.Bucket, publicBase: base}, nil
}

func (s *S3) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *S3) PublicURL(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return s.publicBase + "/" + strings.Join(parts, "/")
}
