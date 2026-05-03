/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/n1kola-petrovic/bucketfill"
)

const sampleConfig = `# bucketfill configuration. Secrets (S3 access keys, etc.) come from
# environment variables — see README.
#
# migrationDir is where bucketfill new creates v<N>/ folders. Edit this if you
# want a name other than "migrations" (e.g. "db/seeds", "bucket-migrations").

provider: fs                # fs | gcs | s3
bucket: default
migrationDir: migrations

fs:
  root: ./local-bucket

# gcs:
#   projectID: my-project
#   credentialsFile: ""     # blank = ADC

# s3:
#   region: us-east-1
#   endpoint: ""            # set for MinIO
#   usePathStyle: false
`

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Write bucketfill.yaml in the current directory",
	Long: `Write a starter bucketfill.yaml in the current directory.

The migrations directory is NOT created here — edit migrationDir in the
generated yaml first if you want a different name, then run "bucketfill new"
which creates the directory and the first version (v1) on demand.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(bucketfill.DefaultConfigFile); err == nil {
			fmt.Printf("%s already exists, leaving as-is\n", bucketfill.DefaultConfigFile)
			return nil
		}
		if err := os.WriteFile(bucketfill.DefaultConfigFile, []byte(sampleConfig), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", bucketfill.DefaultConfigFile, err)
		}
		fmt.Printf("created %s\n", bucketfill.DefaultConfigFile)
		fmt.Println("\nEdit migrationDir if you want a folder other than ./migrations,")
		fmt.Println("then run: bucketfill new")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
