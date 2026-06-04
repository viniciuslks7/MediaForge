// Package storage wraps the S3-compatible object store (MinIO in dev, any S3 in
// prod) used to hold uploaded originals and worker-produced artifacts.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	client *minio.Client
	bucket string
}

type Options struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
	Region    string
}

// New connects to the object store and ensures the bucket exists.
func New(ctx context.Context, o Options) (*Store, error) {
	client, err := minio.New(o.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure: o.UseSSL,
		Region: o.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("connect minio: %w", err)
	}

	exists, err := client.BucketExists(ctx, o.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, o.Bucket, minio.MakeBucketOptions{Region: o.Region}); err != nil {
			return nil, fmt.Errorf("create bucket: %w", err)
		}
	}
	return &Store{client: client, bucket: o.Bucket}, nil
}

// Put streams r into the bucket under key and returns the number of bytes stored.
func (s *Store) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) (int64, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return 0, fmt.Errorf("put object %q: %w", key, err)
	}
	return info.Size, nil
}

// PresignedGet returns a time-limited URL clients can use to download an object.
func (s *Store) PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("presign %q: %w", key, err)
	}
	return u.String(), nil
}
