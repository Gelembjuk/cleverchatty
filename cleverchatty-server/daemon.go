package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	version = "0.1.0"
)

var directoryPath string
var pidFilePath string

var rootCmd = &cobra.Command{
	Use:   "cleverchatty-server",
	Short: "Universal AI assistant server. Version: " + version,
	Long: `cleverchatty-server is a server tool for running a universal AI assistant. 
	It can be run as a daemon to handle requests and manage AI interactions.
	It supports:
	- A2A protocol to communicate with other AI agents (in both directions, as a client and as a server).
	- MCP protocol to call tools (all MCP transports are supported).
	- UI server allows to communicate with different UI clients (web, cli, mobile, etc.).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("no command specified, use 'start', 'stop', 'reload' or 'version'")
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the server",
	Long:  `Run the cleverchatty server. This command starts the server and listens for incoming requests.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the server as a daemon",
	Long:  `Start the cleverchatty server as a background daemon process. This command forks the server process and runs it in the background.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startDaemon()
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the server daemon",
	Long:  `Stop the cleverchatty server daemon. This command sends a termination signal to the running server process.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopDaemon()
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the server configuration",
	Long:  `Reload the configuration of the cleverchatty server daemon. This command sends a SIGHUP signal to the running server process to reload its configuration without stopping it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reloadDaemon()
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display the version information of the cleverchatty server.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("cleverchatty-server version %s\n", version)
		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(reloadCmd)
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentFlags().
		StringVarP(&directoryPath, "directory", "d", "", "Path to the directory with config files and data")

	if directoryPath == "" {
		directoryPath, _ = os.Getwd()
	}

	pidFilePath = directoryPath + "/cleverchatty-server.pid"
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startDaemon() error {
	if pidRunning() {
		return fmt.Errorf("daemon is already running")
	}

	// Fork the process
	cmd := exec.Command(os.Args[0], "run", "--directory", directoryPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start daemon: %v", err)
	}
	fmt.Printf("Daemon started with PID %d\n", cmd.Process.Pid)
	os.WriteFile(pidFilePath, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	return nil
}
func stopDaemon() error {
	pid, err := readPid()
	if err != nil {
		return fmt.Errorf("no daemon running: %v", err)
	}
	err = syscall.Kill(pid, syscall.SIGTERM)
	if err != nil {
		log.Fatalf("Failed to send SIGTERM: %v", err)
	}
	os.Remove(pidFilePath)
	fmt.Println("Daemon stopped.")
	return nil
}

func reloadDaemon() error {
	pid, err := readPid()
	if err != nil {
		return fmt.Errorf("no daemon running: %v", err)
	}
	err = syscall.Kill(pid, syscall.SIGHUP)
	if err != nil {
		return fmt.Errorf("failed to send SIGHUP to daemon: %v", err)
	}
	fmt.Println("Daemon reload signal sent.")
	return nil
}

func pidRunning() bool {
	pid, err := readPid()
	if err != nil {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Sending signal 0 checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func readPid() (int, error) {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// The actual daemon logic
func runServer() error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP)

	fmt.Println("Daemon running...")

	for {
		select {
		case sig := <-sigs:
			switch sig {
			case syscall.SIGTERM:
				fmt.Println("Stopping daemon...")
				os.Remove(pidFilePath)
				return nil
			case syscall.SIGHUP:
				fmt.Println("Reloading config...")
				// Reload logic here
			}
		default:
			// Your server logic here
			fmt.Println("Server is running...")
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}
