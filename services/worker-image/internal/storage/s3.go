// Package storage is the worker's view of the S3-compatible object store: it
// downloads originals and uploads produced variants.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	client *minio.Client
	bucket string
}

type Options struct {
	Endpoint, AccessKey, SecretKey, Bucket, Region string
	UseSSL                                          bool
}

func New(o Options) (*Store, error) {
	client, err := minio.New(o.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure: o.UseSSL,
		Region: o.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("connect minio: %w", err)
	}
	return &Store{client: client, bucket: o.Bucket}, nil
}

// Get downloads an object fully into memory.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get %q: %w", key, err)
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", key, err)
	}
	return data, nil
}

// Put uploads bytes under key and returns the stored size.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) (int64, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return 0, fmt.Errorf("put %q: %w", key, err)
	}
	return info.Size, nil
}
