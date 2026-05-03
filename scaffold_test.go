package bucketfill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n1kola-petrovic/bucketfill"
)

func TestReadModulePath(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/foo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := bucketfill.ReadModulePath(goMod)
	if err != nil {
		t.Fatalf("ReadModulePath: %v", err)
	}
	if got != "example.com/foo" {
		t.Fatalf("got %q, want example.com/foo", got)
	}
}

func TestScaffoldVersion_CreatesFiles(t *testing.T) {
	root := t.TempDir()
	dir, err := bucketfill.ScaffoldVersion(root, "migrations", 1)
	if err != nil {
		t.Fatalf("ScaffoldVersion: %v", err)
	}
	for _, want := range []string{"up.go", "down.go", "embed.go", "data/.keep"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}

	// Check up.go has the right package and signature
	data, err := os.ReadFile(filepath.Join(dir, "up.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "package v1") {
		t.Errorf("up.go missing package v1: %s", data)
	}
	if !strings.Contains(string(data), "func Up(ctx context.Context, c *bucketfill.Client) error") {
		t.Errorf("up.go missing expected Up signature: %s", data)
	}
}

func TestScaffoldVersion_DoesNotClobber(t *testing.T) {
	root := t.TempDir()
	dir, err := bucketfill.ScaffoldVersion(root, "migrations", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Replace up.go with custom content; re-run ScaffoldVersion; verify preserved.
	custom := []byte("package v1\n\n// custom user code\n")
	if err := os.WriteFile(filepath.Join(dir, "up.go"), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bucketfill.ScaffoldVersion(root, "migrations", 1); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "up.go"))
	if string(got) != string(custom) {
		t.Fatalf("up.go was overwritten:\n%s", got)
	}
}

func TestGenerateEntryBinary_Idempotent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := bucketfill.ScaffoldVersion(root, "migrations", 1); err != nil {
		t.Fatal(err)
	}
	versions, err := bucketfill.Scan(filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatal(err)
	}

	changed1, err := bucketfill.GenerateEntryBinary(root, "example.com/demo", "migrations", versions)
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	if !changed1 {
		t.Fatal("first generate should report changed=true")
	}

	changed2, err := bucketfill.GenerateEntryBinary(root, "example.com/demo", "migrations", versions)
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if changed2 {
		t.Fatal("second generate should be a no-op (changed=false)")
	}

	body, _ := os.ReadFile(filepath.Join(root, "cmd", "migrate", "main.go"))
	for _, want := range []string{
		"DO NOT EDIT",
		`v1 "example.com/demo/migrations/v1"`,
		"bucketfill.Register",
		"bucketfill.RunCLI",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("entry binary missing %q:\n%s", want, body)
		}
	}
}
