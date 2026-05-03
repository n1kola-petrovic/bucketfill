package bucketfill_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/n1kola-petrovic/bucketfill"
	"github.com/n1kola-petrovic/bucketfill/provider/fs"
)

// TestE2E_UpThenDown exercises the full pipeline:
// register two migrations, run Up, verify on-disk bucket layout, run Down to 0,
// verify everything (except the state file) is gone.
func TestE2E_UpThenDown(t *testing.T) {
	bucketfill.ResetMigrations()
	defer bucketfill.ResetMigrations()

	root := t.TempDir()
	bucketRoot := filepath.Join(root, "test")
	storage := fs.New(root)
	m := bucketfill.NewMigrator(bucketfill.NewClient(storage, "test"))
	ctx := context.Background()

	v1Data := fstest.MapFS{
		"data/seeds/welcome.txt": {Data: []byte("welcome")},
		"data/seeds/.keep":       {Data: nil},
	}
	v2Data := fstest.MapFS{
		"data/avatars/default.svg": {Data: []byte(`<svg/>`)},
	}

	bucketfill.Register(&bucketfill.Migration{
		Version: 1,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return c.PutAll(ctx) },
		Down: func(ctx context.Context, c *bucketfill.Client) error {
			return c.Delete(ctx, "seeds/welcome.txt")
		},
		Data: subFS(v1Data),
	})
	bucketfill.Register(&bucketfill.Migration{
		Version: 2,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return c.PutAll(ctx) },
		Down: func(ctx context.Context, c *bucketfill.Client) error {
			return c.Delete(ctx, "avatars/default.svg")
		},
		Data: subFS(v2Data),
	})

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	for _, want := range []string{"seeds/welcome.txt", "avatars/default.svg"} {
		if _, err := os.Stat(filepath.Join(bucketRoot, want)); err != nil {
			t.Errorf("missing %s after Up: %v", want, err)
		}
	}

	if err := m.DownTo(ctx, 0); err != nil {
		t.Fatalf("DownTo(0): %v", err)
	}

	keys := listFiles(t, bucketRoot)
	sort.Strings(keys)
	want := []string{"_bucketfill_state.json"} // state remains; user data gone
	if !equalStrings(keys, want) {
		t.Fatalf("after DownTo(0), bucket = %v, want %v", keys, want)
	}
}

// subFS mimics what the scaffolded entry binary's `sub()` helper does: trim
// the leading "data/" prefix off an embed.FS so callers see paths rooted at
// the data tree.
func subFS(m fstest.MapFS) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range m {
		if rel, ok := stripPrefix(k, "data/"); ok {
			out[rel] = v
		}
	}
	return out
}

func stripPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
