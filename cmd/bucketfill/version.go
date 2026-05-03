/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/n1kola-petrovic/bucketfill"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the bucketfill version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(bucketfill.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
