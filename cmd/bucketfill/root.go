/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global flags shared across subcommands. They map directly into bucketfill.Overrides.
var (
	flagConfig       string
	flagProvider     string
	flagBucket       string
	flagMigrationDir string
)

var rootCmd = &cobra.Command{
	Use:   "bucketfill",
	Short: "Versioned migrations for bucket storage",
	Long: `Bucketfill manages seeds and migrations for bucket / object storage
(local filesystem, GCS, S3). Run "bucketfill init" to scaffold a project,
"bucketfill new" to add a version, and "bucketfill up" to apply pending
migrations.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "path to bucketfill.yaml")
	rootCmd.PersistentFlags().StringVar(&flagProvider, "provider", "", "override provider (fs|gcs|s3)")
	rootCmd.PersistentFlags().StringVar(&flagBucket, "bucket", "", "override bucket")
	rootCmd.PersistentFlags().StringVar(&flagMigrationDir, "dir", "", "override migrations directory")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
