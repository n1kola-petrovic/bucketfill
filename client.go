package bucketfill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// Client is the high-level handle migrations use to act on a bucket.
// A migration's Up/Down receives a Client whose dataFS is rooted at that
// migration's data/ subtree (when present), so Put(ctx, key) reads from
// data/<key> and PutAll(ctx) mirrors the whole tree.
type Client struct {
	storage ObjectStorage
	bucket  string
	dataFS  fs.FS // nil if the migration has no data/ folder
}

// NewClient builds a Client without a dataFS. Useful for library callers who
// orchestrate uploads themselves; the Migrator wraps this with a per-migration
// view via forMigration.
func NewClient(storage ObjectStorage, bucket string) *Client {
	return &Client{storage: storage, bucket: bucket}
}

// Bucket returns the configured bucket name.
func (c *Client) Bucket() string { return c.bucket }

// WithData returns a copy of c whose Put / PutAll resolve sources from dataFS.
// The Migrator wraps the Client this way before invoking each migration; users
// of the library API can do the same to scope a Client to an arbitrary fs.FS.
func (c *Client) WithData(dataFS fs.FS) *Client {
	cp := *c
	cp.dataFS = dataFS
	return &cp
}

// Put uploads a single file from the migration's data/ folder to the bucket
// using the same key on both sides. data/<key> -> bucket:<key>.
//
// Returns an error if the migration has no data/ folder or the file is missing.
func (c *Client) Put(ctx context.Context, key string) error {
	if c.dataFS == nil {
		return fmt.Errorf("bucketfill: Put(%q): migration has no data/ folder; use PutFromPath for external files", key)
	}
	return c.putFromFS(ctx, key, key)
}

// PutAll mirrors the migration's entire data/ tree into the bucket, preserving
// nesting. Files are uploaded with the same relative path as their location
// under data/. A nil or non-existent dataFS is treated as a no-op.
func (c *Client) PutAll(ctx context.Context) error {
	if c.dataFS == nil {
		return nil
	}
	err := fs.WalkDir(c.dataFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip the .keep placeholder used to make //go:embed succeed on empty data/.
		if path.Base(p) == ".keep" {
			return nil
		}
		return c.putFromFS(ctx, p, p)
	})
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// DeleteAll removes from the bucket every key that has a corresponding entry
// in the migration's data/ tree. It mirrors PutAll's path scheme so that
// calling PutAll in Up and DeleteAll in Down forms a clean round-trip.
//
// Only files present in the data/ tree are deleted — other bucket contents
// are untouched. A nil or non-existent dataFS is treated as a no-op.
func (c *Client) DeleteAll(ctx context.Context) error {
	if c.dataFS == nil {
		return nil
	}
	err := fs.WalkDir(c.dataFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if path.Base(p) == ".keep" {
			return nil
		}
		return c.storage.Delete(ctx, c.bucket, p)
	})
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// PutFromPath uploads a file from an arbitrary local path on disk to the given
// bucket key. Use this for files outside the migration's data/ folder.
func (c *Client) PutFromPath(ctx context.Context, key, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("bucketfill: open %s: %w", localPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("bucketfill: stat %s: %w", localPath, err)
	}

	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("bucketfill: read header %s: %w", localPath, err)
	}
	contentType := detectContentType(header[:n])

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("bucketfill: seek %s: %w", localPath, err)
	}

	return c.storage.Upload(ctx, c.bucket, key, f, info.Size(), contentType)
}

// PutReader uploads bytes streamed from r to the given key.
func (c *Client) PutReader(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	return c.storage.Upload(ctx, c.bucket, key, r, size, contentType)
}

// Get returns a reader for the object at key. The caller must close it.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return c.storage.Download(ctx, c.bucket, key)
}

// Delete removes the object at key.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.storage.Delete(ctx, c.bucket, key)
}

// Rename moves an object from oldKey to newKey (copy + delete).
func (c *Client) Rename(ctx context.Context, oldKey, newKey string) error {
	if err := c.storage.Copy(ctx, c.bucket, oldKey, newKey); err != nil {
		return fmt.Errorf("bucketfill: copy %s -> %s: %w", oldKey, newKey, err)
	}
	return c.storage.Delete(ctx, c.bucket, oldKey)
}

// List returns all object keys with the given prefix.
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	return c.storage.List(ctx, c.bucket, prefix)
}

// putFromFS reads srcPath out of dataFS and uploads it to the bucket as dstKey.
func (c *Client) putFromFS(ctx context.Context, srcPath, dstKey string) error {
	data, err := fs.ReadFile(c.dataFS, srcPath)
	if err != nil {
		return fmt.Errorf("bucketfill: read data/%s: %w", srcPath, err)
	}
	contentType := detectContentType(data)
	return c.storage.Upload(ctx, c.bucket, dstKey, bytes.NewReader(data), int64(len(data)), contentType)
}

// detectContentType wraps http.DetectContentType with a fix for SVG, which is
// detected as text/plain by the standard library.
func detectContentType(buf []byte) string {
	probe := buf
	if len(probe) > 512 {
		probe = probe[:512]
	}
	ct := http.DetectContentType(probe)
	if strings.Contains(ct, "text/plain") && strings.Contains(strings.ToLower(string(probe)), "<svg") {
		return "image/svg+xml"
	}
	return ct
}
