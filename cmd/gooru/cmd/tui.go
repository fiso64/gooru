/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"github.com/spf13/cobra"
	"gooru.local/gooru/internal/tui"
)

// tuiCmd represents the tui command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Starts an interactive terminal user interface (EXPERIMENTAL).",
	Long:  `Starts an interactive terminal user interface for searching and viewing files (EXPERIMENTAL).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// The service 'svc' is initialized by the rootCmd PersistentPreRunE hook.
		app, err := tui.NewApp(svc)
		if err != nil {
			return err
		}
		return app.Run()
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
