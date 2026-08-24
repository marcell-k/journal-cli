package cmd

import (
	"journal/internal/tui"

	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the full-screen dashboard (blocks, goals, sleep, projects, metrics)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(conn)
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
