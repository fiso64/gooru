package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"gooru.local/gooru/cmd/gooru/config"
	"gooru.local/gooru/types"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

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
		runDir, err := config.GetRunDirPath()
		if err != nil {
			return fmt.Errorf("could not get runtime directory: %w", err)
		}

		mounts, err := findActiveMounts(runDir)
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

		mount, err := findMountByPath(mountpoint)
		if err != nil {
			return err
		}

		var command string
		if isSet {
			command = fmt.Sprintf("SET_QUERY %s\n", expression)
		} else {
			command = "GET_QUERY\n"
		}

		response, err := sendIPCCommand(mount.Port, command)
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

func resolveRemoteTargetPath(path string) (string, error) {
	isDriveLetter := false
	if runtime.GOOS == "windows" {
		if len(path) == 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) {
			isDriveLetter = true
		}
	}

	if isDriveLetter {
		return strings.ToUpper(path), nil
	}

	// For non-drive letters, get the cleaned absolute path.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not resolve path '%s': %w", path, err)
	}
	return filepath.Clean(absPath), nil
}

func findActiveMounts(runDir string) ([]types.MountInfo, error) {
	var mounts []types.MountInfo
	err := filepath.WalkDir(runDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil // Skip unreadable files
			}
			var info types.MountInfo
			if err := json.Unmarshal(data, &info); err != nil {
				return nil // Skip malformed files
			}

			// Health check
			resp, err := sendIPCCommand(info.Port, "PING\n")
			if err == nil && resp == "PONG" {
				mounts = append(mounts, info)
			} else {
				// Stale file, clean it up
				os.Remove(path)
			}
		}
		return nil
	})
	return mounts, err
}

func findMountByPath(path string) (types.MountInfo, error) {
	canonicalPath, err := resolveRemoteTargetPath(path)
	if err != nil {
		return types.MountInfo{}, err
	}

	runDir, err := config.GetRunDirPath()
	if err != nil {
		return types.MountInfo{}, err
	}

	mounts, err := findActiveMounts(runDir)
	if err != nil {
		return types.MountInfo{}, err
	}

	for _, mount := range mounts {
		if mount.MountPoint == canonicalPath {
			return mount, nil
		}
	}
	return types.MountInfo{}, fmt.Errorf("no active mount found for path: %s", path)
}

func sendIPCCommand(port int, command string) (string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("could not connect to mount process: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(command)); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	return strings.TrimSpace(response), nil
}