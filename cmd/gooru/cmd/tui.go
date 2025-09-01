/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"gooru.local/gooru/internal/tui"
	"github.com/spf13/cobra"
)

// tuiCmd represents the tui command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Starts an interactive terminal user interface.",
	Long:  `Starts an interactive terminal user interface for searching and viewing files.`,
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