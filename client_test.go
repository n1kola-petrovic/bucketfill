package bucketfill_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/n1kola-petrovic/bucketfill"
	"github.com/n1kola-petrovic/bucketfill/provider/fs"
)

// newClient builds a Client backed by a temp-dir FS provider. The bucket name
// is "test"; on disk, files land under <tmp>/test/<key>.
func newClient(t *testing.T) (*bucketfill.Client, string) {
	t.Helper()
	root := t.TempDir()
	storage := fs.New(root)
	return bucketfill.NewClient(storage, "test"), root
}

func TestPutFromPath_RoundTrip(t *testing.T) {
	c, root := newClient(t)
	ctx := context.Background()

	if err := c.PutFromPath(ctx, "seeds/note.txt", "testdata/note.txt"); err != nil {
		t.Fatalf("PutFromPath: %v", err)
	}

	// On-disk verification
	got, err := os.ReadFile(filepath.Join(root, "test", "seeds", "note.txt"))
	if err != nil {
		t.Fatalf("read uploaded file: %v", err)
	}
	if !strings.Contains(string(got), "hello, bucketfill") {
		t.Fatalf("uploaded contents wrong: %q", got)
	}

	// Round-trip via Get
	rc, err := c.Get(ctx, "seeds/note.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if !strings.Contains(string(body), "hello, bucketfill") {
		t.Fatalf("Get returned wrong body: %q", body)
	}
}

func TestPut_RequiresDataFS(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()
	err := c.Put(ctx, "anything")
	if err == nil {
		t.Fatal("expected error when no dataFS is set")
	}
	if !strings.Contains(err.Error(), "no data/ folder") {
		t.Fatalf("error didn't mention data/: %v", err)
	}
}

func TestPutAll_MirrorsTree(t *testing.T) {
	c, root := newClient(t)
	ctx := context.Background()

	dataFS := fstest.MapFS{
		"seeds/a.txt":         {Data: []byte("a")},
		"seeds/sub/b.txt":     {Data: []byte("b")},
		"seeds/sub/.keep":     {Data: nil}, // skipped
		"avatars/default.png": {Data: []byte("not really a png")},
	}
	mc := c.WithData(dataFS)

	if err := mc.PutAll(ctx); err != nil {
		t.Fatalf("PutAll: %v", err)
	}

	for _, want := range []string{
		"test/seeds/a.txt",
		"test/seeds/sub/b.txt",
		"test/avatars/default.png",
	} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("expected %s on disk: %v", want, err)
		}
	}
	// .keep must be skipped
	if _, err := os.Stat(filepath.Join(root, "test", "seeds", "sub", ".keep")); err == nil {
		t.Error(".keep was uploaded but should have been skipped")
	}
}

func TestPut_FromDataFS(t *testing.T) {
	c, root := newClient(t)
	ctx := context.Background()

	dataFS := fstest.MapFS{"seeds/logo.svg": {Data: []byte(`<svg>x</svg>`)}}
	mc := c.WithData(dataFS)

	if err := mc.Put(ctx, "seeds/logo.svg"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "test", "seeds", "logo.svg")); err != nil {
		t.Fatalf("expected uploaded file: %v", err)
	}
}

func TestDeleteAll_MirrorsPutAll(t *testing.T) {
	c, root := newClient(t)
	ctx := context.Background()

	dataFS := fstest.MapFS{
		"seeds/a.txt":     {Data: []byte("a")},
		"seeds/sub/b.txt": {Data: []byte("b")},
		"seeds/.keep":     {Data: nil},
	}
	mc := c.WithData(dataFS)

	if err := mc.PutAll(ctx); err != nil {
		t.Fatalf("PutAll: %v", err)
	}
	for _, k := range []string{"seeds/a.txt", "seeds/sub/b.txt"} {
		if _, err := os.Stat(filepath.Join(root, "test", k)); err != nil {
			t.Fatalf("expected %s after PutAll: %v", k, err)
		}
	}

	if err := mc.DeleteAll(ctx); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	for _, k := range []string{"seeds/a.txt", "seeds/sub/b.txt"} {
		if _, err := os.Stat(filepath.Join(root, "test", k)); !os.IsNotExist(err) {
			t.Errorf("expected %s removed after DeleteAll, got err=%v", k, err)
		}
	}
}

func TestRename_AndList(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()

	if err := c.PutFromPath(ctx, "old/note.txt", "testdata/note.txt"); err != nil {
		t.Fatalf("PutFromPath: %v", err)
	}
	if err := c.Rename(ctx, "old/note.txt", "new/note.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	keys, err := c.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sort.Strings(keys)
	want := []string{"new/note.txt"}
	if !equalStrings(keys, want) {
		t.Fatalf("List = %v, want %v", keys, want)
	}
}

func TestDelete_Idempotent(t *testing.T) {
	c, _ := newClient(t)
	ctx := context.Background()

	if err := c.Delete(ctx, "missing/key"); err != nil {
		t.Fatalf("Delete on missing should be nil-error: %v", err)
	}
}

func equalStrings(a, b []string) bool {
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
