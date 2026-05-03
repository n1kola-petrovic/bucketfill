/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/n1kola-petrovic/bucketfill"
)

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create the next migration version (v<N+1>) and scaffold the embed/main files on first run",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		next, err := bucketfill.NextVersion(cfg.MigrationDir)
		if err != nil {
			return err
		}

		dir, err := bucketfill.ScaffoldVersion(".", cfg.MigrationDir, next)
		if err != nil {
			return err
		}

		// One-time scaffolds (no-ops if files already exist).
		if wrote, err := bucketfill.EnsureMigrationsEmbed(".", cfg.MigrationDir); err != nil {
			return err
		} else if wrote {
			fmt.Printf("created %s/embed.go\n", filepath.ToSlash(cfg.MigrationDir))
		}

		modulePath, err := bucketfill.ReadModulePath("go.mod")
		if err != nil {
			return fmt.Errorf("bucketfill: this command must run from a Go module: %w", err)
		}
		// Re-scan after scaffolding so register.go's init() picks up the new version.
		versions, err := bucketfill.Scan(cfg.MigrationDir)
		if err != nil {
			return err
		}
		if wrote, err := bucketfill.GenerateEntryBinary(".", modulePath, cfg.MigrationDir); err != nil {
			return err
		} else if wrote {
			fmt.Printf("created %s/main.go (static — never edited again)\n", bucketfill.EntryBinaryPath)
		}
		if wrote, err := bucketfill.GenerateRegistrations(".", modulePath, cfg.MigrationDir, versions); err != nil {
			return err
		} else if wrote {
			fmt.Printf("updated %s/register.go (now registers v1..v%d)\n", filepath.ToSlash(cfg.MigrationDir), next)
		}

		fmt.Printf("scaffolded v%d at %s\n", next, filepath.ToSlash(dir))
		fmt.Printf("  drop assets in %s/data/ — that's it. No registration code to write.\n", filepath.ToSlash(dir))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
}
