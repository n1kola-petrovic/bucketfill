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

func TestScaffoldVersion_CreatesUpDownAndDataKeep(t *testing.T) {
	root := t.TempDir()
	dir, err := bucketfill.ScaffoldVersion(root, "migrations", 1)
	if err != nil {
		t.Fatalf("ScaffoldVersion: %v", err)
	}
	for _, want := range []string{"up.go", "down.go", "data/.keep"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	// Per-version embed.go is NOT scaffolded — the single top-level
	// migrations/embed.go covers every version via //go:embed all:v*.
	if _, err := os.Stat(filepath.Join(dir, "embed.go")); err == nil {
		t.Error("did not expect per-version embed.go — should be one top-level file")
	}

	upBody, _ := os.ReadFile(filepath.Join(dir, "up.go"))
	if !strings.Contains(string(upBody), "func Up(ctx context.Context, c *bucketfill.Client) error") {
		t.Errorf("up.go missing expected Up signature:\n%s", upBody)
	}
	if !strings.Contains(string(upBody), "c.PutAll(ctx)") {
		t.Errorf("up.go missing default PutAll body:\n%s", upBody)
	}
	downBody, _ := os.ReadFile(filepath.Join(dir, "down.go"))
	if !strings.Contains(string(downBody), "c.DeleteAll(ctx)") {
		t.Errorf("down.go missing default DeleteAll body:\n%s", downBody)
	}
}

func TestScaffoldVersion_DoesNotClobberCustomized(t *testing.T) {
	root := t.TempDir()
	dir, err := bucketfill.ScaffoldVersion(root, "migrations", 1)
	if err != nil {
		t.Fatal(err)
	}
	custom := []byte("package v1\n\n// user customized\n")
	if err := os.WriteFile(filepath.Join(dir, "up.go"), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bucketfill.ScaffoldVersion(root, "migrations", 1); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "up.go"))
	if string(got) != string(custom) {
		t.Fatalf("up.go was clobbered:\n%s", got)
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

func TestGenerateEntryBinary_RegistersScannedVersions(t *testing.T) {
	root := t.TempDir()
	if _, err := bucketfill.ScaffoldVersion(root, "migrations", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := bucketfill.ScaffoldVersion(root, "migrations", 2); err != nil {
		t.Fatal(err)
	}
	versions, err := bucketfill.Scan(filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatal(err)
	}

	wrote, err := bucketfill.GenerateEntryBinary(root, "example.com/demo", "migrations", versions)
	if err != nil {
		t.Fatalf("GenerateEntryBinary: %v", err)
	}
	if !wrote {
		t.Fatal("first call should write")
	}

	body, _ := os.ReadFile(filepath.Join(root, "cmd", "migrate", "main.go"))
	for _, want := range []string{
		"package main",
		"DO NOT EDIT",
		`migrations "example.com/demo/migrations"`,
		`v1 "example.com/demo/migrations/v1"`,
		`v2 "example.com/demo/migrations/v2"`,
		"Version: 1",
		"Version: 2",
		"v1.Up",
		"v2.Down",
		`fs.Sub(migrations.FS, p)`,
		"bucketfill.RunCLI",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("entry binary missing %q:\n%s", want, body)
		}
	}

	// Idempotent — second call with same versions writes no changes.
	wrote2, err := bucketfill.GenerateEntryBinary(root, "example.com/demo", "migrations", versions)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if wrote2 {
		t.Error("second call with identical versions should be a no-op")
	}
}
