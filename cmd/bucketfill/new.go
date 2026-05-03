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
	Short: "Create the next migration version (v<N+1>) with up/down/data scaffolding",
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

		fmt.Printf("scaffolded v%d at %s\n", next, filepath.ToSlash(dir))
		fmt.Printf("  edit %s/up.go and %s/down.go, drop assets in %s/data/\n",
			filepath.ToSlash(dir), filepath.ToSlash(dir), filepath.ToSlash(dir))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
}
