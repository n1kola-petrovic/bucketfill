package fs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/n1kola-petrovic/bucketfill/provider/fs"
)

func TestFS_UploadDownloadRoundTrip(t *testing.T) {
	s := fs.New(t.TempDir())
	ctx := context.Background()
	body := []byte("payload")

	if err := s.Upload(ctx, "b", "k", bytes.NewReader(body), int64(len(body)), "application/octet-stream"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	rc, err := s.Download(ctx, "b", "k")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q, want %q", got, body)
	}
}

func TestFS_DownloadMissing_WrapsErrNotExist(t *testing.T) {
	s := fs.New(t.TempDir())
	_, err := s.Download(context.Background(), "b", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error doesn't wrap os.ErrNotExist: %v", err)
	}
}

func TestFS_DeleteIdempotent(t *testing.T) {
	s := fs.New(t.TempDir())
	if err := s.Delete(context.Background(), "b", "absent"); err != nil {
		t.Fatalf("Delete on missing: %v", err)
	}
}

func TestFS_CopyAndList(t *testing.T) {
	s := fs.New(t.TempDir())
	ctx := context.Background()
	body := []byte("x")

	if err := s.Upload(ctx, "b", "src/a.txt", bytes.NewReader(body), 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Copy(ctx, "b", "src/a.txt", "dst/a.txt"); err != nil {
		t.Fatal(err)
	}

	keys, err := s.List(ctx, "b", "")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(keys)
	want := []string{"dst/a.txt", "src/a.txt"}
	if !equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestFS_ListWithPrefix(t *testing.T) {
	s := fs.New(t.TempDir())
	ctx := context.Background()

	for _, k := range []string{"seeds/a.txt", "seeds/sub/b.txt", "other.txt"} {
		if err := s.Upload(ctx, "b", k, bytes.NewReader([]byte("x")), 1, ""); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List(ctx, "b", "seeds")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"seeds/a.txt", "seeds/sub/b.txt"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
