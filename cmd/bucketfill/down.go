/*
Copyright © 2026 Nikola Petrovic @github.com/n1kola-petrovic
*/
package main

import (
	"strconv"

	"github.com/spf13/cobra"
)

var downTo int

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back the most recent migration (or to --to)",
	Long: `Roll back applied migrations. By default rolls back exactly one step
(the most recently applied migration). Pass --to N to roll back until the
state version is N (use --to 0 to roll back everything).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var extra []string
		if cmd.Flags().Changed("to") {
			extra = append(extra, "--to", strconv.Itoa(downTo))
		}
		return runEntryBinary("down", extra)
	},
}

func init() {
	downCmd.Flags().IntVar(&downTo, "to", -1, "target version (omit for single step)")
	rootCmd.AddCommand(downCmd)
}
