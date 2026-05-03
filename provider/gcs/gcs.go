// Package gcs implements bucketfill.ObjectStorage on Google Cloud Storage.
//
// Importing this package for side effects registers it under the name "gcs"
// so OpenProvider can construct it from a Config:
//
//	import _ "github.com/n1kola-petrovic/bucketfill/provider/gcs"
package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/n1kola-petrovic/bucketfill"
)

func init() {
	bucketfill.RegisterProvider("gcs", openFromConfig)
}

// Storage implements bucketfill.ObjectStorage backed by GCS.
type Storage struct {
	client *storage.Client
}

// New builds a Storage with the given GCS client. Caller owns the client lifetime.
func New(client *storage.Client) *Storage {
	return &Storage{client: client}
}

func openFromConfig(cfg *bucketfill.Config) (bucketfill.ObjectStorage, error) {
	ctx := context.Background()
	var opts []option.ClientOption
	if cfg.GCS != nil && cfg.GCS.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.GCS.CredentialsFile))
	}
	c, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcs: new client: %w", err)
	}
	return New(c), nil
}

func (s *Storage) Upload(ctx context.Context, bucket, key string, r io.Reader, _ int64, contentType string) error {
	w := s.client.Bucket(bucket).Object(key).NewWriter(ctx)
	if contentType != "" {
		w.ContentType = contentType
	}
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return fmt.Errorf("gcs: upload %s/%s: %w", bucket, key, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("gcs: close %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *Storage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	rc, err := s.client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, fmt.Errorf("gcs: %s/%s: %w", bucket, key, os.ErrNotExist)
		}
		return nil, fmt.Errorf("gcs: download %s/%s: %w", bucket, key, err)
	}
	return rc, nil
}

func (s *Storage) Delete(ctx context.Context, bucket, key string) error {
	err := s.client.Bucket(bucket).Object(key).Delete(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return fmt.Errorf("gcs: delete %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *Storage) Copy(ctx context.Context, bucket, srcKey, dstKey string) error {
	src := s.client.Bucket(bucket).Object(srcKey)
	dst := s.client.Bucket(bucket).Object(dstKey)
	if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
		return fmt.Errorf("gcs: copy %s -> %s: %w", srcKey, dstKey, err)
	}
	return nil
}

func (s *Storage) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	it := s.client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	var keys []string
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gcs: list %s: %w", bucket, err)
		}
		keys = append(keys, attrs.Name)
	}
	return keys, nil
}
