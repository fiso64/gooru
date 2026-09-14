package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"gooru.local/internal/buildinfo"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Gooru build identity",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Summary()) },
}

func init() {
	rootCmd.Version = buildinfo.Summary()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.AddCommand(versionCmd)
}
