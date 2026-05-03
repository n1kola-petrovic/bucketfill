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

// runEntryBinary ensures the static cmd/migrate scaffolding exists, then
// shells out to `go run ./cmd/migrate <sub> <extra...>`. The entry binary
// itself is static — adding new versions does not require regenerating it,
// so this command does not rewrite it on every invocation.
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

	// Bootstrap the embed.go and entry binary if they don't exist yet (e.g.
	// projects that scaffolded versions on an older bucketfill release).
	if _, err := bucketfill.EnsureMigrationsEmbed(".", cfg.MigrationDir); err != nil {
		return err
	}
	if _, err := bucketfill.GenerateEntryBinary(".", modulePath, cfg.MigrationDir); err != nil {
		return err
	}
	// Always tidy before `go run` — entry binary's blank-imported provider deps
	// must be in go.sum, and tidy is cheap when there's nothing to do.
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
