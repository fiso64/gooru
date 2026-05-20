//go:build !fuse

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var mountCmd = &cobra.Command{
	Use:   "mount <mountpoint> [expression...]",
	Short: "Mounts a virtual filesystem based on a query.",
	Long: `Mounts a read-only virtual filesystem at the specified mount point.

This binary was built without FUSE support. Rebuild with '-tags fuse' on a
system with the appropriate FUSE development headers installed to enable it.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("mount requires a gooru binary built with FUSE support")
	},
}

func init() {
	rootCmd.AddCommand(mountCmd)
}
