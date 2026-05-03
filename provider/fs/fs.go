// Package fs implements bucketfill.ObjectStorage on the local filesystem.
//
// The "bucket" is a subdirectory under Root. Keys map to file paths under
// <Root>/<bucket>/. Useful for tests and local development.
package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Storage is a filesystem-backed bucketfill.ObjectStorage.
type Storage struct {
	root string
}

// New returns a Storage rooted at the given parent directory. Buckets become
// subfolders of root.
func New(root string) *Storage {
	return &Storage{root: root}
}

func (s *Storage) bucketPath(bucket string) string {
	return filepath.Join(s.root, bucket)
}

func (s *Storage) keyPath(bucket, key string) string {
	return filepath.Join(s.bucketPath(bucket), filepath.FromSlash(key))
}

func (s *Storage) Upload(_ context.Context, bucket, key string, r io.Reader, _ int64, _ string) error {
	path := s.keyPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("fs: mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("fs: create %s: %w", path, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("fs: write %s: %w", path, err)
	}
	return nil
}

func (s *Storage) Download(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	path := s.keyPath(bucket, key)
	f, err := os.Open(path)
	if err != nil {
		// os.Open returns *fs.PathError wrapping os.ErrNotExist for missing
		// files; preserve that with %w so callers can errors.Is(... ErrNotExist).
		return nil, fmt.Errorf("fs: open %s: %w", path, err)
	}
	return f, nil
}

func (s *Storage) Delete(_ context.Context, bucket, key string) error {
	path := s.keyPath(bucket, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fs: remove %s: %w", path, err)
	}
	return nil
}

func (s *Storage) Copy(_ context.Context, bucket, srcKey, dstKey string) error {
	srcPath := s.keyPath(bucket, srcKey)
	dstPath := s.keyPath(bucket, dstKey)

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("fs: open %s: %w", srcPath, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("fs: mkdir %s: %w", filepath.Dir(dstPath), err)
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("fs: create %s: %w", dstPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("fs: copy %s -> %s: %w", srcPath, dstPath, err)
	}
	return nil
}

func (s *Storage) List(_ context.Context, bucket, prefix string) ([]string, error) {
	bucketRoot := s.bucketPath(bucket)
	walkRoot := s.keyPath(bucket, prefix)

	var keys []string
	err := filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(bucketRoot, path)
		if err != nil {
			return err
		}
		keys = append(keys, strings.ReplaceAll(rel, string(filepath.Separator), "/"))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
