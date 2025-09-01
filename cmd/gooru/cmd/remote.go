package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gooru.local/gooru/internal/ipc"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote <subcommand>",
	Short: "Manages running gooru mount instances.",
	Long:  `Provides tools to list and control running mount processes.`,
	// This command does not need the standard DB client, so we override the parent's hook.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all active mount points.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mounts, err := ipc.FindActiveMounts()
		if err != nil {
			return fmt.Errorf("could not find active mounts: %w", err)
		}

		if len(mounts) == 0 {
			fmt.Println("No active mounts found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		defer w.Flush()
		fmt.Fprintln(w, "ID\tPID\tPORT\tMOUNT POINT")
		for _, mount := range mounts {
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", mount.ID, mount.PID, mount.Port, mount.MountPoint)
		}
		return nil
	},
}

var remoteQueryCmd = &cobra.Command{
	Use:   "query <mountpoint> [expression...]",
	Short: "Gets or sets the query for an active mount.",
	Long: `Gets or sets the query for an active mount.
If an expression is provided, the mount's view is updated.
If no expression is provided, the mount's current query is printed.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mountpoint := args[0]
		expression := ""
		isSet := false
		if len(args) > 1 {
			expression = strings.Join(args[1:], " ")
			isSet = true
		}

		mount, err := ipc.FindMountByPath(mountpoint)
		if err != nil {
			return err
		}

		var command string
		if isSet {
			command = fmt.Sprintf("SET_QUERY %s\n", expression)
		} else {
			command = "GET_QUERY\n"
		}

		response, err := ipc.SendIPCCommand(mount.Port, command)
		if err != nil {
			return err
		}

		parts := strings.SplitN(response, " ", 2)
		status := parts[0]
		payload := ""
		if len(parts) > 1 {
			payload = parts[1]
		}

		if status == "OK" {
			if isSet {
				fmt.Println("Query updated successfully.")
			} else {
				fmt.Println(payload)
			}
		} else {
			return fmt.Errorf("mount returned error: %s", payload)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(remoteCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteQueryCmd)
}