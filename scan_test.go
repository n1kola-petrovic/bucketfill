package bucketfill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n1kola-petrovic/bucketfill"
)

// makeVersionDir creates <root>/<name>/{up.go, down.go, data/.keep}.
func makeVersionDir(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := name
	if !strings.HasPrefix(pkg, "v") {
		pkg = "v" + pkg
	}
	for _, f := range []string{"up.go", "down.go"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("package "+pkg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "data", ".keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_NormalizesPrefix(t *testing.T) {
	root := t.TempDir()
	makeVersionDir(t, root, "1")
	makeVersionDir(t, root, "v2")

	dirs, err := bucketfill.Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("got %d dirs, want 2: %+v", len(dirs), dirs)
	}
	if dirs[0].Version != 1 || dirs[0].PkgName != "v1" {
		t.Errorf("dirs[0] = %+v, want Version=1 PkgName=v1", dirs[0])
	}
	if dirs[1].Version != 2 || dirs[1].PkgName != "v2" {
		t.Errorf("dirs[1] = %+v, want Version=2 PkgName=v2", dirs[1])
	}
}

func TestScan_RejectsDuplicates(t *testing.T) {
	root := t.TempDir()
	makeVersionDir(t, root, "1")
	makeVersionDir(t, root, "v1")

	_, err := bucketfill.Scan(root)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error didn't mention duplicate: %v", err)
	}
}

func TestScan_RequiresUpAndDown(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "v1", "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// no up.go / down.go
	_, err := bucketfill.Scan(root)
	if err == nil {
		t.Fatal("expected missing-up.go error")
	}
	if !strings.Contains(err.Error(), "up.go") {
		t.Fatalf("error didn't mention up.go: %v", err)
	}
}

func TestScan_IgnoresNonVersionFolders(t *testing.T) {
	root := t.TempDir()
	makeVersionDir(t, root, "v1")
	if err := os.MkdirAll(filepath.Join(root, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	dirs, err := bucketfill.Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("got %d, want 1", len(dirs))
	}
}

func TestNextVersion_EmptyDir(t *testing.T) {
	root := t.TempDir()
	v, err := bucketfill.NextVersion(root)
	if err != nil {
		t.Fatalf("NextVersion: %v", err)
	}
	if v != 1 {
		t.Fatalf("got %d, want 1", v)
	}
}

func TestNextVersion_AfterExisting(t *testing.T) {
	root := t.TempDir()
	makeVersionDir(t, root, "v3")
	v, err := bucketfill.NextVersion(root)
	if err != nil {
		t.Fatalf("NextVersion: %v", err)
	}
	if v != 4 {
		t.Fatalf("got %d, want 4", v)
	}
}
