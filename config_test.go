package bucketfill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/n1kola-petrovic/bucketfill"
)

// withDir runs fn with the working directory set to dir, restoring on exit.
// LoadConfig reads bucketfill.yaml relative to CWD; tests need to drive that.
func withDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)
	fn()
}

func TestLoadConfig_DefaultsWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	withDir(t, dir, func() {
		cfg, err := bucketfill.LoadConfig(bucketfill.Overrides{})
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Provider != "fs" {
			t.Errorf("provider = %q, want fs", cfg.Provider)
		}
		if cfg.MigrationDir != "migrations" {
			t.Errorf("migrationDir = %q, want migrations", cfg.MigrationDir)
		}
	})
}

func TestLoadConfig_FileFlagOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	yaml := []byte(`provider: fs
bucket: from-yaml
migrationDir: migrations
fs:
  root: ./local-bucket
`)
	if err := os.WriteFile(filepath.Join(dir, "bucketfill.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BUCKETFILL_BUCKET", "from-env")

	withDir(t, dir, func() {
		// env wins over yaml
		cfg, err := bucketfill.LoadConfig(bucketfill.Overrides{})
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Bucket != "from-env" {
			t.Errorf("bucket = %q, want from-env", cfg.Bucket)
		}

		// flag wins over env
		cfg, err = bucketfill.LoadConfig(bucketfill.Overrides{Bucket: "from-flag"})
		if err != nil {
			t.Fatalf("LoadConfig with override: %v", err)
		}
		if cfg.Bucket != "from-flag" {
			t.Errorf("bucket = %q, want from-flag", cfg.Bucket)
		}
	})
}

func TestLoadConfig_S3RequiresRegion(t *testing.T) {
	dir := t.TempDir()
	withDir(t, dir, func() {
		_, err := bucketfill.LoadConfig(bucketfill.Overrides{
			Provider: "s3",
			Bucket:   "x",
		})
		if err == nil {
			t.Fatal("expected validation error for missing region")
		}
	})
}
