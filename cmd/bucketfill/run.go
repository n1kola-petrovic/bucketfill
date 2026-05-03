/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/n1kola-petrovic/bucketfill"
)

// loadConfig is the CLI's wrapper around bucketfill.LoadConfig that threads
// global flags (--config/--provider/--bucket/--dir) through.
func loadConfig() (*bucketfill.Config, error) {
	return bucketfill.LoadConfig(bucketfill.Overrides{
		ConfigFile:   flagConfig,
		Provider:     flagProvider,
		Bucket:       flagBucket,
		MigrationDir: flagMigrationDir,
	})
}

// runEntryBinary regenerates cmd/migrate/main.go and shells out to
// `go run ./cmd/migrate <sub> <extra...>`, passing the global flags through so
// the entry binary's RunCLI sees the same configuration.
func runEntryBinary(sub string, extra []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	versions, err := bucketfill.Scan(cfg.MigrationDir)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return fmt.Errorf("bucketfill: no migrations found in %s — run `bucketfill new` first", cfg.MigrationDir)
	}

	modulePath, err := bucketfill.ReadModulePath("go.mod")
	if err != nil {
		return fmt.Errorf("bucketfill: this command must run from a Go module: %w", err)
	}

	if _, err := bucketfill.GenerateEntryBinary(".", modulePath, cfg.MigrationDir, versions); err != nil {
		return err
	}
	// Ensure go.sum has entries for the entry binary's transitive imports.
	// Cheap if already tidy; necessary when blank-imported provider deps were
	// added since the last time the user ran the toolchain.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("bucketfill: go mod tidy: %w", err)
	}

	args := []string{"run", "./" + filepath.ToSlash(bucketfill.EntryBinaryPath), sub}
	args = append(args, passthroughFlags()...)
	args = append(args, extra...)

	c := exec.Command("go", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// passthroughFlags repeats the global cobra flags as command-line args so the
// entry binary's flag set sees them.
func passthroughFlags() []string {
	var out []string
	if flagConfig != "" {
		out = append(out, "--config", flagConfig)
	}
	if flagProvider != "" {
		out = append(out, "--provider", flagProvider)
	}
	if flagBucket != "" {
		out = append(out, "--bucket", flagBucket)
	}
	if flagMigrationDir != "" {
		out = append(out, "--dir", flagMigrationDir)
	}
	return out
}
