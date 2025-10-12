package cmd

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
	"gooru.local/cmd/gooru/config"
	"gooru.local/cmd/gooru/display"
	"gooru.local/gooru"
	"gooru.local/internal/ipc"
	"gooru.local/internal/mount"
	"gooru.local/types"
)

var (
	mountOpenExplorer bool
	mountLive         bool
	mountHierarchical bool
)

// mountCmd represents the mount command
var mountCmd = &cobra.Command{
	Use:   "mount <mountpoint> [expression...]",
	Short: "Mounts a virtual filesystem based on a query.",
	Long: `Mounts a read-only virtual filesystem at the specified mount point.
The filesystem presents a live view of the Gooru database.

Two modes are available:
- Flat (default): A single directory containing all files matching the expression.
- Hierarchical (--hierarchical): A browsable directory structure where folders
  are tags, allowing for interactive filtering.

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

		// 1. Finalize mountpoint path and check validity
		absMountpoint, err := resolveMountpoint(mountpoint)
		if err != nil {
			return err
		}
		mountpoint = absMountpoint

		// NEW: Before proceeding, health-check for any existing mount at this path.
		// This will also clean up stale runtime files if a previous mount was killed.
		if existingMount, err := ipc.FindMountByPath(mountpoint); err == nil {
			return fmt.Errorf("mount point '%s' is already active (PID: %d)", mountpoint, existingMount.PID)
		}

		// 2. Setup IPC and runtime registration
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("could not start IPC listener: %w", err)
		}
		defer listener.Close()
		port := listener.Addr().(*net.TCPAddr).Port

		runDir, err := config.GetRunDirPath()
		if err != nil {
			return fmt.Errorf("could not get runtime directory: %w", err)
		}

		mountID := generateMountID(mountpoint)
		runtimeFile := filepath.Join(runDir, mountID+".json")

		mountInfo := types.MountInfo{
			PID:        os.Getpid(),
			MountPoint: mountpoint,
			Port:       port,
			ID:         mountID,
		}



		if err := writeRuntimeFile(runtimeFile, mountInfo); err != nil {
			return fmt.Errorf("could not write runtime file: %w", err)
		}
		defer os.Remove(runtimeFile) // Cleanup on exit

		// 3. Get initial file list
		fmt.Println("Querying database for initial file list...")
		files, err := getInitialFiles(svc, expression)
		if err != nil {
			return fmt.Errorf("error listing files: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No files found for the given query. Mount will be empty.")
		}

		// 4. Setup VFS and FUSE host
		vfs := mount.NewGooruVFS(svc, expression, files, mountLive, mountHierarchical)
		host := fuse.NewFileSystemHost(vfs)

		// 5. Start IPC Server
		go serveIPC(listener, vfs)

		// 6. Setup signal handling for graceful unmount
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			host.Unmount()
		}()

		fmt.Printf("Mounting filesystem at '%s'.\n", mountpoint)
		fmt.Printf("IPC server listening on 127.0.0.1:%d.\n", port)
		fmt.Println("Press Ctrl-C to unmount.")

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

		// 7. Mount (blocking call)
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
	mountCmd.Flags().BoolVarP(&mountLive, "live", "l", false, "Enable live refresh (re-queries database on directory access)")
	mountCmd.Flags().BoolVarP(&mountHierarchical, "hierarchical", "r", false, "Enable hierarchical tag-based directory browsing")
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

func resolveMountpoint(path string) (string, error) {
	isDriveLetter := false
	if runtime.GOOS == "windows" {
		if len(path) == 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) {
			isDriveLetter = true
		}
	}

	if isDriveLetter {
		return strings.ToUpper(path), nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not resolve mount point path '%s': %w", path, err)
	}
	mountpoint := filepath.Clean(absPath)

	info, err := os.Stat(mountpoint)
	if err != nil {
		if os.IsNotExist(err) {
			// On Windows, for a directory path, WinFsp-FUSE must create the mount point.
			// Pre-creating it as a normal directory causes a "mount point in use" error.
			// On other platforms, the directory must exist before mounting.
			if runtime.GOOS != "windows" {
				if err := os.MkdirAll(mountpoint, 0755); err != nil {
					return "", fmt.Errorf("failed to create mount point directory '%s': %w", mountpoint, err)
				}
			}
			// On Windows, if it doesn't exist, we do nothing and let FUSE handle it.
		} else {
			// Other error, e.g. permission denied.
			return "", fmt.Errorf("failed to access mount point '%s': %w", mountpoint, err)
		}
	} else {
		// Path exists.
		if !info.IsDir() {
			return "", fmt.Errorf("mount point '%s' is not a directory", mountpoint)
		}

		// On Windows, mounting to an existing directory is problematic. It's safer to require a non-existent path.
		if runtime.GOOS == "windows" {
			return "", fmt.Errorf("mount point '%s' already exists; on Windows, please specify a path that does not exist", mountpoint)
		}

		// On other platforms, we require the directory to be empty.
		dir, err := os.Open(mountpoint)
		if err != nil {
			return "", fmt.Errorf("failed to open mount point directory for checking: %w", err)
		}
		defer dir.Close()
		_, err = dir.Readdir(1)
		if err != io.EOF {
			return "", fmt.Errorf("mount point directory '%s' must be empty", mountpoint)
		}
	}
	return mountpoint, nil
}

func getInitialFiles(svc *gooru.Client, expression string) ([]types.FileInfo, error) {
	if expression == "" {
		return svc.GetAllFilesInfo()
	}
	return svc.GetFilesInfoByQuery(expression, verbose)
}

func generateMountID(absPath string) string {
	hasher := sha1.New()
	hasher.Write([]byte(absPath))
	return hex.EncodeToString(hasher.Sum(nil))[:12]
}

func writeRuntimeFile(path string, info types.MountInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func serveIPC(listener net.Listener, vfs *mount.GooruVFS) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Listener was closed, so exit.
			return
		}
		go handleIPCConnection(conn, vfs)
	}
}

func handleIPCConnection(conn net.Conn, vfs *mount.GooruVFS) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return // e.g., client disconnected
		}
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, " ", 2)
		cmd := parts[0]

		switch cmd {
		case "PING":
			conn.Write([]byte("PONG\n"))
		case "GET_QUERY":
			query := vfs.GetCurrentQuery()
			fmt.Fprintf(conn, "OK %s\n", query)
		case "SET_QUERY":
			if len(parts) < 2 {
				conn.Write([]byte("ERROR missing query expression\n"))
				continue
			}
			query := parts[1]
			if err := vfs.UpdateQuery(query); err != nil {
				fmt.Fprintf(conn, "ERROR %v\n", err)
			} else {
				conn.Write([]byte("OK\n"))
			}
		case "RESOLVE":
			if len(parts) < 2 {
				conn.Write([]byte("ERROR missing virtual path\n"))
				continue
			}
			vpath := parts[1]
			if realPath, ok := vfs.ResolveVirtualPath(vpath); ok {
				fmt.Fprintf(conn, "OK %s\n", realPath)
			} else {
				conn.Write([]byte("ERROR not found\n"))
			}
		default:
			conn.Write([]byte("ERROR unknown command\n"))
		}
	}
}