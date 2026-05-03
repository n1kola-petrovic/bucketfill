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

func TestScaffoldVersion_CreatesOnlyDataKeep(t *testing.T) {
	root := t.TempDir()
	dir, err := bucketfill.ScaffoldVersion(root, "migrations", 1)
	if err != nil {
		t.Fatalf("ScaffoldVersion: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", ".keep")); err != nil {
		t.Errorf("expected data/.keep: %v", err)
	}
	// New scaffold should NOT generate per-version up.go / down.go / embed.go.
	for _, unwanted := range []string{"up.go", "down.go", "embed.go"} {
		if _, err := os.Stat(filepath.Join(dir, unwanted)); err == nil {
			t.Errorf("did not expect %s — that's the old per-version layout", unwanted)
		}
	}
}

func TestEnsureMigrationsEmbed_OneTime(t *testing.T) {
	root := t.TempDir()
	wrote, err := bucketfill.EnsureMigrationsEmbed(root, "migrations")
	if err != nil {
		t.Fatalf("EnsureMigrationsEmbed: %v", err)
	}
	if !wrote {
		t.Fatal("first call should report wrote=true")
	}

	body, err := os.ReadFile(filepath.Join(root, "migrations", "embed.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"package migrations",
		`//go:embed all:v*`,
		"var FS embed.FS",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("embed.go missing %q:\n%s", want, body)
		}
	}

	// Second call should be a no-op (idempotent).
	wrote2, err := bucketfill.EnsureMigrationsEmbed(root, "migrations")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if wrote2 {
		t.Error("second call should report wrote=false")
	}
}

func TestEnsureMigrationsEmbed_RespectsDirBasename(t *testing.T) {
	root := t.TempDir()
	if _, err := bucketfill.EnsureMigrationsEmbed(root, "db/seeds"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "db", "seeds", "embed.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "package seeds") {
		t.Errorf("embed.go missing 'package seeds' (basename of db/seeds):\n%s", body)
	}
}

func TestGenerateEntryBinary_OneTimeStatic(t *testing.T) {
	root := t.TempDir()
	wrote, err := bucketfill.GenerateEntryBinary(root, "example.com/demo", "migrations")
	if err != nil {
		t.Fatalf("GenerateEntryBinary: %v", err)
	}
	if !wrote {
		t.Fatal("first call should write")
	}

	body, _ := os.ReadFile(filepath.Join(root, "cmd", "migrate", "main.go"))
	for _, want := range []string{
		"package main",
		`migrations "example.com/demo/migrations"`,
		"bucketfill.RegisterFS(migrations.FS)",
		"bucketfill.RunCLI",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("entry binary missing %q:\n%s", want, body)
		}
	}
	// Should NOT contain per-version registrations.
	if strings.Contains(string(body), "Version: 1,") {
		t.Errorf("entry binary should not register versions explicitly:\n%s", body)
	}

	// Second call must NOT clobber — the file is editable user code.
	wrote2, err := bucketfill.GenerateEntryBinary(root, "example.com/demo", "migrations")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if wrote2 {
		t.Error("second call should be a no-op")
	}
}
