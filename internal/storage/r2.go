package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicURL       string
}

type Store struct {
	client    *minio.Client
	bucket    string
	publicURL string
}

type Object struct {
	*minio.Object
	ContentType string
}

func New(config Config) (*Store, error) {
	if config.Endpoint == "" || config.AccessKeyID == "" || config.SecretAccessKey == "" {
		return nil, fmt.Errorf("R2_ENDPOINT, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY are required")
	}
	parsed, err := url.Parse(config.Endpoint)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid R2_ENDPOINT")
	}
	// R2.dev is a public CDN domain; the S3 API endpoint is <account-id>.r2.cloudflarestorage.com.
	if strings.HasSuffix(parsed.Host, ".r2.dev") {
		return nil, fmt.Errorf("R2_ENDPOINT %q is a public R2.dev CDN domain, not the S3 API endpoint — use https://<account-id>.r2.cloudflarestorage.com instead", config.Endpoint)
	}
	client, err := minio.New(parsed.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: parsed.Scheme != "http",
	})
	if err != nil {
		return nil, err
	}
	return &Store{client: client, bucket: config.Bucket, publicURL: strings.TrimRight(config.PublicURL, "/")}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("store not initialized")
	}
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

func (s *Store) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *Store) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *Store) Get(ctx context.Context, key string) (*Object, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	info, err := object.Stat()
	if err != nil {
		object.Close()
		return nil, err
	}
	return &Object{Object: object, ContentType: info.ContentType}, nil
}

func (s *Store) PublicURL(key string) string {
	if s.publicURL == "" {
		return ""
	}
	return s.publicURL + "/" + strings.TrimLeft(key, "/")
}

func SafeContentType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	lower := strings.ToLower(contentType)
	for _, risky := range []string{"html", "svg", "xml", "javascript"} {
		if strings.Contains(lower, risky) {
			return "application/octet-stream"
		}
	}
	return contentType
}

func (s *Store) PresignedURL(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	return s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
}
