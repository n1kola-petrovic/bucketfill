// Package bucketfill manages versioned migrations against bucket / object
// storage (local filesystem, GCS, S3). Migrations are Go functions registered
// per version; uploads can be sourced from a per-version data/ subtree whose
// nesting mirrors the bucket layout.
//
// Two ways to use this package:
//
//   - Library: import bucketfill, define migrations, call bucketfill.Up/Down/Status
//     with a *Config you build yourself.
//   - CLI: install the bucketfill binary, run `bucketfill init` / `bucketfill new`,
//     then `bucketfill up` / `bucketfill down`. The CLI scaffolds and invokes a
//     small entry binary in the user's project that calls into RunCLI.
package bucketfill

import (
	"context"
	"errors"
	"flag"
	"fmt"
)

// Version is the bucketfill release.
const Version = "0.1.0"

// DownOpts configures a Down/DownTo run.
type DownOpts struct {
	// To is the target version. -1 means "one step back" (the default Down behavior).
	To int
}

// Up applies all pending migrations.
func Up(ctx context.Context, cfg *Config) error {
	m, err := newMigrator(ctx, cfg)
	if err != nil {
		return err
	}
	return m.Up(ctx)
}

// Down rolls back migrations. opts.To = -1 for a single-step rollback.
func Down(ctx context.Context, cfg *Config, opts DownOpts) error {
	m, err := newMigrator(ctx, cfg)
	if err != nil {
		return err
	}
	if opts.To < 0 {
		return m.Down(ctx)
	}
	return m.DownTo(ctx, opts.To)
}

// Status prints registered migrations and which are applied.
func Status(ctx context.Context, cfg *Config) error {
	m, err := newMigrator(ctx, cfg)
	if err != nil {
		return err
	}
	return m.Status(ctx)
}

func newMigrator(_ context.Context, cfg *Config) (*Migrator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	storage, err := OpenProvider(cfg)
	if err != nil {
		return nil, err
	}
	return NewMigrator(NewClient(storage, cfg.EffectiveBucket())), nil
}

// RunCLI is the dispatch invoked by the scaffolded entry binary's main().
// It parses the up/down/status subcommand and any flags, loads config, and
// runs the corresponding migrator method.
//
// Usage from the entry binary: `bucketfill.RunCLI(ctx, os.Args[1:])`.
func RunCLI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("bucketfill: missing subcommand (want up|down|status)")
	}

	sub, rest := args[0], args[1:]

	fs := flag.NewFlagSet("bucketfill "+sub, flag.ContinueOnError)
	configPath := fs.String("config", "", "path to bucketfill.yaml")
	provider := fs.String("provider", "", "override provider (fs|gcs|s3)")
	bucket := fs.String("bucket", "", "override bucket")
	dir := fs.String("dir", "", "override migrations directory")
	to := fs.Int("to", -1, "(down only) target version; -1 = single step")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	cfg, err := LoadConfig(Overrides{
		ConfigFile:   *configPath,
		Provider:     *provider,
		Bucket:       *bucket,
		MigrationDir: *dir,
	})
	if err != nil {
		return err
	}

	switch sub {
	case "up":
		return Up(ctx, cfg)
	case "down":
		return Down(ctx, cfg, DownOpts{To: *to})
	case "status":
		return Status(ctx, cfg)
	case "version":
		fmt.Println(Version)
		return nil
	default:
		return fmt.Errorf("bucketfill: unknown subcommand %q", sub)
	}
}

