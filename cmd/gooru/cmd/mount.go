package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gooru.local/gooru/cmd/gooru/display"
	"gooru.local/gooru/internal/mount"
	"gooru.local/gooru/types"
	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
)

var (
	mountOpenExplorer bool
)

// mountCmd represents the mount command
var mountCmd = &cobra.Command{
	Use:   "mount <mountpoint> [expression...]",
	Short: "Mounts a virtual filesystem based on a query.",
	Long: `Mounts a read-only virtual filesystem at the specified mount point.
The filesystem presents a live view of the Gooru database.

The view is a flat directory containing all files that match the given expression.
If no expression is provided, all files in the database are listed.

This feature requires a FUSE implementation to be installed on your system:
- Windows: WinFsp (https://winfsp.dev/)
- macOS: macFUSE (https://osxfuse.github.io/)
- Linux: libfuse (e.g., 'sudo apt-get install libfuse-dev')`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mountpoint := args[0]
		expression := ""
		if len(args) > 1 {
			expression = strings.Join(args[1:], " ")
		}

		isDriveLetter := false
		if runtime.GOOS == "windows" {
			// Check for drive letter format like "G:" on the original argument
			if len(mountpoint) == 2 && mountpoint[1] == ':' && ((mountpoint[0] >= 'a' && mountpoint[0] <= 'z') || (mountpoint[0] >= 'A' && mountpoint[0] <= 'Z')) {
				isDriveLetter = true
			}
		}

		// For non-drive letters, we resolve the path and expect an empty directory.
		if !isDriveLetter {
			absMountpoint, err := filepath.Abs(mountpoint)
			if err != nil {
				return fmt.Errorf("could not resolve mount point path '%s': %w", mountpoint, err)
			}
			mountpoint = filepath.Clean(absMountpoint)

			info, err := os.Stat(mountpoint)
			if err != nil {
				if os.IsNotExist(err) {
					if err := os.MkdirAll(mountpoint, 0755); err != nil {
						return fmt.Errorf("failed to create mount point directory '%s': %w", mountpoint, err)
					}
				} else {
					return fmt.Errorf("failed to access mount point '%s': %w", mountpoint, err)
				}
			} else {
				if !info.IsDir() {
					return fmt.Errorf("mount point '%s' is not a directory", mountpoint)
				}
				dir, err := os.Open(mountpoint)
				if err != nil {
					return fmt.Errorf("failed to open mount point directory for checking: %w", err)
				}
				defer dir.Close()
				_, err = dir.Readdir(1)
				if err != io.EOF {
					return fmt.Errorf("mount point directory '%s' must be empty", mountpoint)
				}
			}
		}
		// For drive letters, we pass them directly to the mount function without checks.

		fmt.Println("Querying database for file list...")
		var files []types.FileInfo
		var err error
		if expression == "" {
			files, err = svc.GetAllFilesInfo()
		} else {
			files, err = svc.GetFilesInfoByQuery(expression, verbose)
		}
		if err != nil {
			return fmt.Errorf("error listing files: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No files found for the given query. Mount will be empty.")
		}

		vfs := mount.NewGooruVFS(files)
		host := fuse.NewFileSystemHost(vfs)

		// Set up a channel to listen for OS signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			host.Unmount()
		}()

		fmt.Printf("Mounting filesystem at '%s'. Press Ctrl-C to unmount.\n", mountpoint)

		// This helper will be called in a goroutine before the blocking mount call.
		openMountPoint := func() {
			if mountOpenExplorer {
				// We wait a moment for the mount to become ready before trying to open it.
				time.Sleep(500 * time.Millisecond)
				if err := openExplorer(mountpoint); err != nil {
					display.Warnf("failed to open mount point in explorer: %v", err)
				}
			}
		}

		// The Mount function is blocking. We must not use it on the main thread on macOS
		if runtime.GOOS == "darwin" {
			go openMountPoint()
			go func() {
				if !host.Mount(mountpoint, nil) {
					// In case of mount failure, signal the main thread to exit
					sigChan <- syscall.SIGINT
				}
			}()
			// Keep the main goroutine alive until an unmount signal is received
			<-sigChan
			fmt.Println("\nUnmounting...")
		} else {
			go openMountPoint()
			if !host.Mount(mountpoint, nil) {
				return fmt.Errorf("failed to mount filesystem")
			}
			fmt.Println("\nUnmounted.")
		}
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(mountCmd)
	mountCmd.Flags().BoolVarP(&mountOpenExplorer, "open", "o", false, "Open the mount point in the file explorer after mounting")
}

func openExplorer(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Use `cmd /C start` for robustness, especially with drive letters.
		// The empty string "" is for the title argument to the start command.
		cmd = exec.Command("cmd", "/C", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default: // linux, freebsd, openbsd, netbsd
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}