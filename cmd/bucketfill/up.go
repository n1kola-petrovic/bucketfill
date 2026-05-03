/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	Long:  `Apply every registered migration whose version is greater than the current state, in ascending order.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEntryBinary("up", nil)
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
