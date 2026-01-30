package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/spf13/cobra"
)

const (
	configFileName = "cleverchatty_config.json"
	pidFileName    = "cleverchatty-server.pid"
)

var directoryPath string
var pidFilePath string

var rootCmd = &cobra.Command{
	Use:   "cleverchatty-server",
	Short: "Universal AI assistant server. Version: " + cleverchatty.ThisAppVersion,
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
	Use:          "run",
	Short:        "Run the server",
	Long:         `Run the cleverchatty server. This command starts the server and listens for incoming requests.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

var startCmd = &cobra.Command{
	Use:          "start",
	Short:        "Start the server as a daemon",
	Long:         `Start the cleverchatty server as a background daemon process. This command forks the server process and runs it in the background.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startDaemon()
	},
}

var stopCmd = &cobra.Command{
	Use:          "stop",
	Short:        "Stop the server daemon",
	Long:         `Stop the cleverchatty server daemon. This command sends a termination signal to the running server process.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopDaemon()
	},
}

var reloadCmd = &cobra.Command{
	Use:          "reload",
	Short:        "Reload the server configuration",
	Long:         `Reload the configuration of the cleverchatty server daemon. This command sends a SIGHUP signal to the running server process to reload its configuration without stopping it.`,
	SilenceUsage: true,
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
		fmt.Printf("cleverchatty-server version %s\n", cleverchatty.ThisAppVersion)
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

	pidFilePath = directoryPath + "/" + pidFileName
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

	// try to load config to verify it is present and valid
	_, _, err := loadConfigAndLogger()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Fork the process
	cmd := exec.Command(os.Args[0], "run", "--directory", directoryPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	err = cmd.Start()
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
	config, logger, err := loadConfigAndLogger()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP)

	logger.Println("Daemon running...")

	commonContext, commonContextCancel := context.WithCancel(context.Background())

	// Test-init the assistant to verify configuration is valid before starting the server
	logger.Println("Verifying assistant initialization...")
	testAI, err := cleverchatty.GetCleverChattyWithLogger(*config, commonContext, logger)
	if err != nil {
		commonContextCancel()
		return fmt.Errorf("assistant init check failed: %v", err)
	}
	err = testAI.Init()
	if err != nil {
		commonContextCancel()
		return fmt.Errorf("assistant init check failed: %v", err)
	}
	testAI.Finish()
	logger.Println("Assistant initialization verified successfully.")

	sessions_manager := cleverchatty.NewSessionManager(config, commonContext, logger)
	sessions_manager.StartCleanupLoop()

	var a2aServer *A2AServer
	a2aServer = nil

	if config.A2AServerConfig.Enabled {
		a2aServer, err = getA2AServer(
			sessions_manager,
			&config.A2AServerConfig,
			directoryPath,
			logger)
		if err != nil {
			commonContextCancel()
			return fmt.Errorf("failed to initialize A2A server: %v", err)
		}
		err = a2aServer.Start()
		if err != nil {
			commonContextCancel()
			return fmt.Errorf("failed to start A2A server: %v", err)
		}
		logger.Println("A2A server started successfully.")

		// Set notification callback to broadcast notifications to A2A clients
		sessions_manager.SetNotificationCallback(func(notification cleverchatty.Notification) {
			// Broadcast the notification to all A2A notification subscribers
			a2aServer.BroadcastNotification(notification)
		})
		logger.Println("MCP notification broadcasting to A2A clients enabled.")

		// Set agent message callback to broadcast agent messages to A2A clients
		sessions_manager.SetAgentMessageCallback(func(message string) {
			a2aServer.BroadcastAgentMessage(message)
		})
		logger.Println("Agent message broadcasting to A2A clients enabled.")
	}

	// Initialize Reverse MCP connector if enabled
	var reverseMCPConnector *ReverseMCPConnector
	reverseMCPConnector = nil

	if config.ReverseMCPListenerConfig.Enabled {
		reverseMCPConnector = NewReverseMCPConnector(
			&config.ReverseMCPListenerConfig,
			config.ToolsServers,
			logger,
		)

		// Set the reverse MCP connector as the client for session manager
		// This allows sessions to access tools from reverse-connected MCP servers
		sessions_manager.SetReverseMCPClient(reverseMCPConnector)

		err = reverseMCPConnector.Start()
		if err != nil {
			if a2aServer != nil {
				a2aServer.Stop()
			}
			commonContextCancel()
			return fmt.Errorf("failed to start Reverse MCP connector: %v", err)
		}
		logger.Println("Reverse MCP connector started successfully.")
	}

	shutDown := func() {
		if reverseMCPConnector != nil {
			logger.Println("Stopping Reverse MCP connector...")
			err := reverseMCPConnector.Stop()
			if err != nil {
				logger.Printf("Error stopping Reverse MCP connector: %v", err)
			} else {
				logger.Println("Reverse MCP connector stopped successfully.")
			}
			reverseMCPConnector = nil
		}
		if a2aServer != nil {
			logger.Println("Stopping A2A server...")
			err := a2aServer.Stop()
			if err != nil {
				logger.Printf("Error stopping A2A server: %v", err)
			} else {
				logger.Println("A2A server stopped successfully.")
			}
			a2aServer = nil
		}
		commonContextCancel()
		logger.Println("Daemon shutting down.")
	}
	defer shutDown()
	for sig := range sigs {
		switch sig {
		case syscall.SIGTERM:
			fmt.Println("Stopping daemon...")
			shutDown()
			os.Remove(pidFilePath)
			return nil
		case syscall.SIGHUP:
			fmt.Println("Reloading config...")
			// Reload logic here
		}
	}
	return nil
}

func loadConfigAndLogger() (config *cleverchatty.CleverChattyConfig, logger *log.Logger, err error) {

	configFile := directoryPath + "/" + configFileName
	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		err = fmt.Errorf("config file not found: %s", configFile)
		return
	}
	config, err = cleverchatty.LoadConfig(configFile)

	if err != nil {
		return
	}
	// change work directory to directoryPath because there could be relative paths in config
	if err = os.Chdir(directoryPath); err != nil {
		err = fmt.Errorf("error changing working directory to %s: %v", directoryPath, err)
		return
	}

	// confirm there is at least one server to run
	if !config.A2AServerConfig.Enabled && !config.ReverseMCPListenerConfig.Enabled {
		err = fmt.Errorf("no any kind of server configured. It must be A2A or Reverse MCP (or other in future)")
		return
	}
	logger, err = cleverchatty.InitLogger(config.LogFilePath, config.DebugMode)
	if err != nil {
		err = fmt.Errorf("error initializing logger: %v", err)
		return
	}

	return
}
