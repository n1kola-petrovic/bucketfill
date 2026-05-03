/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show applied vs pending migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEntryBinary("status", nil)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
