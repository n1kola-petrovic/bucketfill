package bucketfill

import (
	"context"
	"io"
)

// ObjectStorage is the interface that storage providers must implement.
// Implementations exist for the local filesystem, GCS, and S3 / S3-compatible services.
type ObjectStorage interface {
	Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, key string) error
	Copy(ctx context.Context, bucket, srcKey, dstKey string) error
	List(ctx context.Context, bucket, prefix string) ([]string, error)
}
