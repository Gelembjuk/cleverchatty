package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	markdown "github.com/MichaelMure/go-term-markdown"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/protocol"
)

var (
	defaultModelFlag = "anthropic:claude-3-5-sonnet-latest"
)

var (
	debugMode        bool
	server           string //A2A server address
	agentid          string // A2A user ID
	configFile       string
	modelFlag        string // New flag for model selection
	promptFlag       string // Single prompt mode
	openaiBaseURL    string // Base URL for OpenAI API
	anthropicBaseURL string // Base URL for Anthropic API
	openaiAPIKey     string
	anthropicAPIKey  string
	googleAPIKey     string
)

var (
	actionInProgress      = false
	actionChannel         = make(chan bool)
	actionCanceledChannel = make(chan bool)
)

var (
	tuiContext      context.Context
	tuiConfig       *cleverchatty.CleverChattyConfig
	tuiCleverChatty *cleverchatty.CleverChatty
	useTUIMode      bool
	// For client mode
	tuiA2AClient *a2aclient.A2AClient
	tuiContextID string
	tuiAgentID   string
)

func getTUICleverChatty() *cleverchatty.CleverChatty {
	return tuiCleverChatty
}

// tuiPrint sends output to TUI if in TUI mode, otherwise prints to stdout
func tuiPrint(msg string) {
	if useTUIMode && program != nil {
		tuiSendChat(msg)
	} else {
		fmt.Print(msg)
	}
}

// initCleverChattyFunc initializes the CleverChatty instance for TUI mode
func initCleverChattyFunc() tea.Msg {
	// If in client mode (A2A client is set), skip CleverChatty initialization
	if tuiA2AClient != nil {
		// Client mode - no local AI initialization needed
		return initCompleteMsg{cleverChatty: nil, err: nil}
	}

	// Standalone mode - initialize local CleverChatty
	// Create a custom logger that writes to the TUI
	customLogger := log.New(&tuiLogWriter{}, "", log.LstdFlags)
	if tuiConfig.DebugMode {
		customLogger.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Create CleverChatty with custom logger
	cleverChattyObject, err := cleverchatty.GetCleverChattyWithLogger(*tuiConfig, tuiContext, customLogger)
	if err != nil {
		return initCompleteMsg{cleverChatty: nil, err: err}
	}

	err = cleverChattyObject.Init()
	if err != nil {
		return initCompleteMsg{cleverChatty: nil, err: err}
	}

	// Set callbacks to use TUI
	cleverChattyObject.Callbacks = composeCallbacks(true)

	// Set notification callback to send notifications to TUI
	cleverChattyObject.SetNotificationCallback(func(notification cleverchatty.Notification) {
		tuiSendNotification(notification)
	})

	// Store globally
	tuiCleverChatty = cleverChattyObject

	return initCompleteMsg{cleverChatty: cleverChattyObject, err: nil}
}

var rootCmd = &cobra.Command{
	Use:   "cleverchatty-cli",
	Short: "Chat with AI models through a unified interface. Version: " + cleverchatty.ThisAppVersion,
	Long: `cleverchatty-cli is a CLI tool that allows you to interact with CleverChatty server using CLI interface.
	The tool can work in two modes:
	- as a client to CleverChatty server, which can be run using:
		 cleverchatty-cli --server <server:port> --agentid <agent_id>
	- as a standalone AI chat client, which can be run using:
		 cleverchatty-cli --config <config_file>
		 or 
		 cleverchatty-cli --model <model_name> ... other flags ...

Available models can be specified using the --model flag (or config file):
- Anthropic Claude (default): anthropic:claude-3-5-sonnet-latest
- OpenAI: openai:gpt-4
- Ollama models: ollama:modelname
- Google: google:modelname

Example:
  cleverchatty-cli --server localhost:8080 --agentid user123
  cleverchatty-cli --config config.json
  cleverchatty-cli --model anthropic:claude-3-5-sonnet-latest
  cleverchatty-cli -m ollama:qwen2.5:3b`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(context.Background())
	},
}
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display the version information of the cleverchatty CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		handleVersionCommand()
		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.PersistentFlags().
		StringVar(&configFile, "config", "", "config file. Use it to run CleverChatty as a standalone tool. Will be ignored if --server and --agentid are set.")
	rootCmd.PersistentFlags().
		StringVar(&server, "server", "", "CleverChatty server address.")
	rootCmd.PersistentFlags().
		StringVar(&agentid, "agentid", "", "Agent ID to be identified by CleverChatty server.")
	rootCmd.PersistentFlags().
		StringVarP(&modelFlag, "model", "m", "",
			"model to use (format: provider:model, e.g. anthropic:claude-3-5-sonnet-latest or ollama:qwen2.5:3b). If not provided then "+defaultModelFlag+" will be used")
	rootCmd.PersistentFlags().
		StringVarP(&promptFlag, "prompt", "p", "",
			"execute a single prompt and exit without starting the interactive UI")

	rootCmd.PersistentFlags().
		BoolP("version", "v", false, "show version and exit")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		showVersion, _ := cmd.Flags().GetBool("version")
		if showVersion {
			handleVersionCommand()
			os.Exit(0)
		}
		return nil
	}
	// Add debug flag
	rootCmd.PersistentFlags().
		BoolVar(&debugMode, "debug", false, "enable debug logging")

	flags := rootCmd.PersistentFlags()
	flags.StringVar(&openaiBaseURL, "openai-url", "", "base URL for OpenAI API (defaults to api.openai.com)")
	flags.StringVar(&anthropicBaseURL, "anthropic-url", "", "base URL for Anthropic API (defaults to api.anthropic.com)")
	flags.StringVar(&openaiAPIKey, "openai-api-key", "", "OpenAI API key")
	flags.StringVar(&anthropicAPIKey, "anthropic-api-key", "", "Anthropic API key")
	flags.StringVar(&googleAPIKey, "google-api-key", "", "Google (Gemini) API key")
}

func loadConfig() (*cleverchatty.CleverChattyConfig, error) {

	var config *cleverchatty.CleverChattyConfig
	var err error
	// check config file exists
	if configFile == "" {
		if _, err = os.Stat("config.json"); err == nil {
			// try to use the standard name for the config file in the current directory
			configFile = "config.json"
		}
	}
	if configFile == "" {
		// use empty config
		err = nil
		config = &cleverchatty.CleverChattyConfig{}
	} else if _, err = os.Stat(configFile); os.IsNotExist(err) {
		config, err = cleverchatty.CreateStandardConfigFile(configFile)
	} else {
		config, err = cleverchatty.LoadConfig(configFile)
	}
	if err != nil {
		return nil, fmt.Errorf("error loading config file: %v", err)
	}
	if agentid != "" {
		config.AgentID = agentid
	}
	if debugMode {
		config.DebugMode = true
	}
	if modelFlag != "" {
		config.Model = modelFlag
	}
	if config.Model == "" {
		config.Model = defaultModelFlag
	}
	if openaiBaseURL != "" {
		config.OpenAI.BaseURL = openaiBaseURL
	}
	if anthropicBaseURL != "" {
		config.Anthropic.BaseURL = anthropicBaseURL
	}
	if openaiAPIKey != "" {
		config.OpenAI.APIKey = openaiAPIKey
	}
	if config.OpenAI.APIKey == "" {
		config.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if anthropicAPIKey != "" {
		config.Anthropic.APIKey = anthropicAPIKey
	}
	if config.Anthropic.APIKey == "" {
		config.Anthropic.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if googleAPIKey != "" {
		config.Google.APIKey = googleAPIKey
	}
	if config.Google.APIKey == "" {
		config.Google.APIKey = os.Getenv("GOOGLE_API_KEY")
	}
	if config.Google.APIKey == "" {
		// The project structure is provider specific, but Google calls this GEMINI_API_KEY in e.g. AI Studio. Support both.
		config.Google.APIKey = os.Getenv("GEMINI_API_KEY")
	}

	if configFile != "" {
		directoryPath := filepath.Dir(configFile)

		if err = os.Chdir(directoryPath); err != nil {
			err = fmt.Errorf("error changing working directory to %s: %v", directoryPath, err)
			return nil, err
		}
	}

	return config, nil
}

// ============================================
func run(ctx context.Context) error {
	var err error
	if server != "" {
		err = runAsClient(ctx)
	} else {
		err = runAsStandalone(ctx)
	}
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	return nil
}
func composeCallbacks(useTUI bool) cleverchatty.UICallbacks {
	callbacks := cleverchatty.UICallbacks{}

	callbacks.SetStartedPromptProcessing(func(prompt string) error {
		if useTUI {
			// Add separator and spacing before the message
			separator := separatorStyle.Render("─────────────────────────────────────────────────")
			userLabel := promptStyle.Render("You:")
			tuiSendChat(fmt.Sprintf("\n%s\n%s\n%s\n", separator, userLabel, prompt))
		} else {
			fmt.Printf("\n%s%s\n\n", promptStyle.Render("You: "), markdown.Render(prompt, 80, 6))
		}
		return nil
	})
	callbacks.SetStartedThinking(func() error {
		if useTUI {
			tuiSendSpinner("💭  Thinking...")
		} else {
			showSpinner("💭  Thinking...")
		}
		return nil
	})
	callbacks.SetMemoryRetrievalStarted(func() error {
		if useTUI {
			tuiSendSpinner("🕰️  Recalling...")
		} else {
			showSpinner("🕰️  Recalling...")
		}
		return nil
	})
	callbacks.SetRAGRetrievalStarted(func() error {
		if useTUI {
			tuiSendSpinner("🗃️  Searching knowledge database ...")
		} else {
			showSpinner("🗃️  Searching knowledge database ...")
		}
		return nil
	})
	callbacks.SetToolCalling(func(toolName string) error {
		if useTUI {
			tuiSendSpinner("🔧 Using tool: " + toolName)
		} else {
			showSpinner("🔧 Using tool: " + toolName)
		}
		return nil
	})
	callbacks.SetToolCallFailed(func(toolName string, err error) error {
		if useTUI {
			tuiClearSpinner()
			tuiSendChat("\n" + errorStyle.Render("Error using tool: "+toolName) + "\n")
		} else {
			releaseActionSpinner()
			fmt.Printf("\n%s\n", errorStyle.Render("Error using tool: "+toolName))
		}
		return nil
	})
	callbacks.SetResponseReceived(func(response string) error {
		if useTUI {
			tuiClearSpinner()
			// Add separator and spacing before the message
			separator := separatorStyle.Render("─────────────────────────────────────────────────")
			assistantLabel := responseStyle.Render("Assistant:")
			tuiSendChat(fmt.Sprintf("\n%s\n%s\n%s\n", separator, assistantLabel, response))
		} else {
			releaseActionSpinner()
			fmt.Printf("\n%s%s\n\n", responseStyle.Render("Assistant: "), markdown.Render(response, 80, 6))
		}
		return nil
	})

	return callbacks
}
func runAsStandalone(ctx context.Context) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}

	if promptFlag != "" {
		return runSinglePromptStandalone(ctx, config)
	}

	// Always use TUI in standalone mode
	return runWithTUI(ctx, config)
}

// composeSinglePromptCallbacks creates callbacks that print status updates to stderr.
func composeSinglePromptCallbacks() cleverchatty.UICallbacks {
	callbacks := cleverchatty.UICallbacks{}

	callbacks.SetStartedThinking(func() error {
		fmt.Fprintln(os.Stderr, "Thinking...")
		return nil
	})
	callbacks.SetMemoryRetrievalStarted(func() error {
		fmt.Fprintln(os.Stderr, "Recalling...")
		return nil
	})
	callbacks.SetRAGRetrievalStarted(func() error {
		fmt.Fprintln(os.Stderr, "Searching knowledge database...")
		return nil
	})
	callbacks.SetToolCalling(func(toolName string) error {
		fmt.Fprintf(os.Stderr, "Using tool: %s\n", toolName)
		return nil
	})
	callbacks.SetToolCallFailed(func(toolName string, err error) error {
		fmt.Fprintf(os.Stderr, "Tool call failed: %s: %v\n", toolName, err)
		return nil
	})

	return callbacks
}

// runSinglePromptStandalone initializes CleverChatty locally, sends a single prompt, prints the response and exits.
func runSinglePromptStandalone(ctx context.Context, config *cleverchatty.CleverChattyConfig) error {
	// If log destination is stdout, redirect to stderr so it doesn't mix with the response
	if config.LogFilePath == "" || config.LogFilePath == "stdout" {
		config.LogFilePath = "stderr"
	}

	logger, err := cleverchatty.InitLogger(config.LogFilePath, config.DebugMode)
	if err != nil {
		return fmt.Errorf("error initializing logger: %v", err)
	}

	cc, err := cleverchatty.GetCleverChattyWithLogger(*config, ctx, logger)
	if err != nil {
		return fmt.Errorf("error creating CleverChatty: %v", err)
	}

	if err := cc.Init(); err != nil {
		return fmt.Errorf("error initializing CleverChatty: %v", err)
	}
	defer cc.Finish()

	cc.Callbacks = composeSinglePromptCallbacks()

	response, err := cc.Prompt(promptFlag)
	if err != nil {
		return fmt.Errorf("error processing prompt: %v", err)
	}

	fmt.Println(response)
	return nil
}

// runSinglePromptClient sends a single prompt to the A2A server, prints the response and exits.
func runSinglePromptClient(ctx context.Context, a2aClient *a2aclient.A2AClient, contextID string, agentID string) error {
	message := a2aprotocol.Message{
		Role: a2aprotocol.MessageRoleUser,
		Parts: []a2aprotocol.Part{
			a2aprotocol.NewTextPart(promptFlag),
		},
		ContextID: &contextID,
		Metadata: map[string]any{
			"agent_id": agentID,
		},
	}

	taskParams := a2aprotocol.SendMessageParams{
		Message: message,
	}

	streamChan, err := a2aClient.StreamMessage(ctx, taskParams)
	if err != nil {
		return fmt.Errorf("error starting task stream: %v", err)
	}

	response, err := processA2AStreamEvents(ctx, streamChan, composeSinglePromptCallbacks())
	if err != nil {
		return fmt.Errorf("error processing response: %v", err)
	}

	fmt.Println(response)

	// Send /bye to release server resources
	byeMessage := a2aprotocol.Message{
		Role: a2aprotocol.MessageRoleUser,
		Parts: []a2aprotocol.Part{
			a2aprotocol.NewTextPart("/bye"),
		},
		ContextID: &contextID,
		Metadata: map[string]any{
			"agent_id": agentID,
		},
	}
	byeCtx, byeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	a2aClient.SendMessage(byeCtx, a2aprotocol.SendMessageParams{Message: byeMessage})
	byeCancel()

	return nil
}

func runWithTUI(ctx context.Context, config *cleverchatty.CleverChattyConfig) error {
	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error initializing renderer: %v", err)
	}

	// Store config and context for initialization
	tuiContext = ctx
	tuiConfig = config
	useTUIMode = true

	// Redirect all logs to TUI (if not already configured to log to file)
	var oldStderr *os.File
	var w *os.File
	if config.LogFilePath == "" || config.LogFilePath == "stdout" {
		log.SetOutput(&tuiLogWriter{})
		log.SetFlags(log.LstdFlags)

		// Also redirect stderr to capture library logs (like MCP server errors)
		oldStderr = os.Stderr
		r, pipeW, err := os.Pipe()
		if err == nil {
			w = pipeW
			os.Stderr = w
			// Start a goroutine to read from the pipe and send to TUI
			go func() {
				scanner := bufio.NewScanner(r)
				for scanner.Scan() {
					line := scanner.Text()
					if program != nil {
						program.Send(logMsg(line + "\n"))
					}
				}
			}()
		}
	}

	// Create prompt callback
	promptCallback := func(prompt string) error {
		cleverChattyObject := getTUICleverChatty()
		if cleverChattyObject == nil {
			return fmt.Errorf("CleverChatty not initialized")
		}
		// Handle slash commands
		handled, err := handleSlashCommand(prompt, *cleverChattyObject)
		if err != nil {
			tuiSendError(err)
			return err
		}
		if handled {
			return nil
		}

		_, err = cleverChattyObject.Prompt(prompt)
		if err != nil {
			tuiSendError(err)
			return err
		}
		return nil
	}

	// Create TUI model
	model := newTUIModel(true, promptCallback)
	program = tea.NewProgram(model, tea.WithAltScreen())

	// Run the program (initialization will happen after TUI starts)
	finalModel, err := program.Run()

	// Cleanup
	useTUIMode = false
	if tuiCleverChatty != nil {
		tuiCleverChatty.Finish()
	}

	// Restore stderr if we redirected it
	if oldStderr != nil {
		if w != nil {
			w.Close() // Close the pipe writer
		}
		os.Stderr = oldStderr
	}

	// Restore default logger to stderr
	log.SetOutput(os.Stderr)

	if err != nil {
		return fmt.Errorf("error running TUI: %v", err)
	}

	// Additional cleanup check
	if m, ok := finalModel.(tuiModel); ok && m.cleverChatty != nil {
		if cc, ok := m.cleverChatty.(*cleverchatty.CleverChatty); ok && cc != tuiCleverChatty {
			cc.Finish()
		}
	}

	return nil
}

func runWithSimpleInput(ctx context.Context, cleverChattyObject *cleverchatty.CleverChatty) error {
	cleverChattyObject.Callbacks = composeCallbacks(false)

	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error initializing renderer: %v", err)
	}

	// Main interaction loop
	for {
		var prompt string
		err := huh.NewForm(huh.NewGroup(huh.NewText().
			Title("Enter your prompt (Type /help for commands, Ctrl+C to quit)").
			Value(&prompt)),
		).WithWidth(getTerminalWidth()).
			WithTheme(huh.ThemeCharm()).
			Run()

		if err != nil {
			// Check if it's a user abort (Ctrl+C)
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nGoodbye!")
				return nil // Exit cleanly
			}
			return err // Return other errors normally
		}

		if prompt == "" {
			continue
		}

		// Handle slash commands
		handled, err := handleSlashCommand(prompt, *cleverChattyObject)
		if err != nil {
			return err
		}
		if handled {
			continue
		}

		_, err = cleverChattyObject.Prompt(prompt)

		if err != nil {
			return err
		}
	}
}

// subscribeToNotifications establishes a persistent notification subscription stream
// with auto-reconnection on disconnect
func subscribeToNotifications(ctx context.Context, a2aClient *a2aclient.A2AClient, contextID string, agentID string) {
	reconnectDelay := 2 * time.Second
	maxReconnectDelay := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("Notification subscription cancelled by context")
			return
		default:
			// Try to establish subscription
			err := subscribeToNotificationsOnce(ctx, a2aClient, contextID, agentID)

			// If context was cancelled, exit
			if ctx.Err() != nil {
				log.Println("Notification subscription cancelled by context")
				return
			}

			// Connection closed or error - attempt reconnection
			if err != nil {
				log.Printf("Notification subscription error: %v. Reconnecting in %v...", err, reconnectDelay)
			} else {
				log.Printf("Notification subscription stream closed. Reconnecting in %v...", reconnectDelay)
			}

			// Wait before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectDelay):
				// Exponential backoff
				reconnectDelay *= 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
			}
		}
	}
}

// subscribeToNotificationsOnce establishes a single notification subscription stream
func subscribeToNotificationsOnce(ctx context.Context, a2aClient *a2aclient.A2AClient, contextID string, agentID string) error {
	// Create subscription request
	message := a2aprotocol.Message{
		Role: a2aprotocol.MessageRoleUser,
		Parts: []a2aprotocol.Part{
			a2aprotocol.NewTextPart("__subscribe_notifications__"),
		},
		ContextID: &contextID,
		Metadata: map[string]any{
			"agent_id": agentID,
		},
	}

	taskParams := a2aprotocol.SendMessageParams{
		Message: message,
	}

	// Open persistent stream
	streamChan, err := a2aClient.StreamMessage(ctx, taskParams)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	log.Println("✓ Notification subscription established")

	// Process notification events until stream closes
	for event := range streamChan {
		handleNotificationEvent(event)
	}

	return nil
}

// handleNotificationEvent processes notification events from the A2A stream
func handleNotificationEvent(event a2aprotocol.StreamingMessageEvent) {
	switch e := event.Result.(type) {
	case *a2aprotocol.TaskStatusUpdateEvent:
		if e.Status.Message != nil && len(e.Status.Message.Parts) > 0 {
			// Check if this is a notification event
			if textPart, ok := e.Status.Message.Parts[0].(*a2aprotocol.TextPart); ok {
				if textPart.Text == "notification_subscribed" {
					log.Println("✓ Notification subscription confirmed by server")
					return
				}

				// Ignore keepalive messages (they just keep connection alive)
				if textPart.Text == "keepalive" {
					return
				}

				// Handle agent messages (from notification processor)
				if textPart.Text == "agent_message" && len(e.Status.Message.Parts) >= 2 {
					message := ""
					if part, ok := e.Status.Message.Parts[1].(*a2aprotocol.TextPart); ok {
						message = part.Text
					}

					if useTUIMode && program != nil {
						tuiSendAgentMessage(message)
					} else {
						fmt.Printf("\n🤖 Agent: %s\n\n", message)
					}
					return
				}

				if textPart.Text == "mcp_notification" && len(e.Status.Message.Parts) >= 3 {
					// Extract notification details from the A2A stream
					serverName := ""
					method := ""
					description := ""
					monitoringStatus := ""
					processingStatus := ""
					paramsStr := ""

					if len(e.Status.Message.Parts) > 1 {
						if part, ok := e.Status.Message.Parts[1].(*a2aprotocol.TextPart); ok {
							serverName = part.Text
						}
					}
					if len(e.Status.Message.Parts) > 2 {
						if part, ok := e.Status.Message.Parts[2].(*a2aprotocol.TextPart); ok {
							method = part.Text
						}
					}
					if len(e.Status.Message.Parts) > 3 {
						if part, ok := e.Status.Message.Parts[3].(*a2aprotocol.TextPart); ok {
							description = part.Text
						}
					}
					if len(e.Status.Message.Parts) > 4 {
						if part, ok := e.Status.Message.Parts[4].(*a2aprotocol.TextPart); ok {
							monitoringStatus = part.Text
						}
					}
					if len(e.Status.Message.Parts) > 5 {
						if part, ok := e.Status.Message.Parts[5].(*a2aprotocol.TextPart); ok {
							processingStatus = part.Text
						}
					}
					if len(e.Status.Message.Parts) > 6 {
						if part, ok := e.Status.Message.Parts[6].(*a2aprotocol.TextPart); ok {
							paramsStr = part.Text
						}
					}

					// Parse params string back to map for notification
					params := make(map[string]interface{})
					// TODO: Parse paramsStr properly if needed
					// For now, just store as a single value
					if paramsStr != "" && paramsStr != "map[]" {
						params["raw"] = paramsStr
					}

					// Create unified Notification structure
					notification := cleverchatty.Notification{
						ServerName:       serverName,
						Method:           method,
						Description:      description,
						MonitoringStatus: cleverchatty.MonitoringStatus(monitoringStatus),
						ProcessingStatus: cleverchatty.ProcessingStatus(processingStatus),
						Params:           params,
					}

					// Send to TUI if in TUI mode, otherwise log
					if useTUIMode && program != nil {
						tuiSendNotification(notification)
					} else {
						log.Printf("📧 Notification from %s: %s", serverName, method)
						if description != "" {
							log.Printf("   Description: %s", description)
						}
					}
				}
			}
		}
	}
}

func runAsClientWithTUI(ctx context.Context, a2aClient *a2aclient.A2AClient, contextID string, agentID string) error {
	// Store client info globally for TUI callbacks
	tuiContext = ctx
	tuiA2AClient = a2aClient
	tuiContextID = contextID
	tuiAgentID = agentID
	useTUIMode = true

	// Redirect all logs to TUI
	log.SetOutput(&tuiLogWriter{})
	log.SetFlags(log.LstdFlags)

	// Also redirect stderr to capture library logs (like A2A client errors)
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err == nil {
		os.Stderr = w
		// Start a goroutine to read from the pipe and send to TUI
		go func() {
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				line := scanner.Text()
				if program != nil {
					program.Send(logMsg(line + "\n"))
				}
			}
		}()
	}

	// Immediately establish persistent notification subscription
	go subscribeToNotifications(ctx, a2aClient, contextID, agentID)

	// Create prompt callback for client mode
	promptCallback := func(prompt string) error {
		if tuiA2AClient == nil {
			return fmt.Errorf("A2A client not initialized")
		}

		// Handle slash commands
		handled, err := handleSlashCommandAsClient(prompt, *tuiA2AClient, ctx, tuiContextID)
		if err != nil {
			tuiSendError(err)
			return err
		}
		if handled {
			return nil
		}

		// Send message via A2A streaming
		message := a2aprotocol.Message{
			Role: a2aprotocol.MessageRoleUser,
			Parts: []a2aprotocol.Part{
				a2aprotocol.NewTextPart(prompt),
			},
			ContextID: &tuiContextID,
			Metadata: map[string]any{
				"agent_id": tuiAgentID,
			},
		}

		taskParams := a2aprotocol.SendMessageParams{
			Message: message,
		}

		streamChan, err := tuiA2AClient.StreamMessage(ctx, taskParams)
		if err != nil {
			tuiSendError(err)
			return fmt.Errorf("error starting task stream: %v", err)
		}

		// Process stream events with TUI callbacks
		_, err = processA2AStreamEvents(ctx, streamChan, composeCallbacks(true))
		if err != nil {
			tuiSendError(err)
			return fmt.Errorf("error processing task stream events: %v", err)
		}

		return nil
	}

	// Create TUI model with notifications enabled
	model := newTUIModel(true, promptCallback)
	program = tea.NewProgram(model, tea.WithAltScreen())

	// Run the program
	var runErr error
	_, runErr = program.Run()

	// Send /bye to server to terminate the session before cleanup
	// This ensures the server releases resources immediately instead of waiting for timeout
	if a2aClient != nil {
		byeMessage := a2aprotocol.Message{
			Role: a2aprotocol.MessageRoleUser,
			Parts: []a2aprotocol.Part{
				a2aprotocol.NewTextPart("/bye"),
			},
			ContextID: &contextID,
			Metadata: map[string]any{
				"agent_id": agentID,
			},
		}
		byeParams := a2aprotocol.SendMessageParams{
			Message: byeMessage,
		}
		// Use a short timeout context for the bye message
		byeCtx, byeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		a2aClient.SendMessage(byeCtx, byeParams)
		byeCancel()
	}

	// Cleanup
	useTUIMode = false
	tuiA2AClient = nil

	// Restore stderr if we redirected it
	if oldStderr != nil {
		w.Close() // Close the pipe writer
		os.Stderr = oldStderr
	}

	// Restore default logger to stderr
	log.SetOutput(os.Stderr)

	if runErr != nil {
		return fmt.Errorf("error running TUI: %v", runErr)
	}

	return nil
}

func runAsClient(ctx context.Context) error {
	// 1. Check for streaming capability by fetching the agent card
	check, err := checkServerIsCleverChatty(server)
	if err != nil {
		return fmt.Errorf("error checking server capabilities: %v", err)
	}

	// context id
	contextID := fmt.Sprintf("session-%d-%s", time.Now().UnixNano(), uuid.New().String())

	err = sendHelloMessage(ctx, server, agentid, &contextID)
	if err != nil {
		// If the server does not support CleverChatty AI chat, we return an error
		// probably, agentid is not set or is wrong
		return err
	}

	// 2. Create a new client instance with custom HTTP client for long-lived connections
	// Use no timeout for SSE streams (notification subscriptions are persistent)
	httpClient := &http.Client{
		Timeout: 0, // No timeout for long-lived notification streams
	}
	a2aClient, err := a2aclient.NewA2AClient(server, a2aclient.WithHTTPClient(httpClient))
	if err != nil {
		return fmt.Errorf("error creating A2A client: %v", err)
	}

	if !check {
		return fmt.Errorf("the server at %s does not support CleverChatty AI chat", server)
	}

	if promptFlag != "" {
		return runSinglePromptClient(ctx, a2aClient, contextID, agentid)
	}

	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error initializing renderer: %v", err)
	}

	// Always use TUI in client mode
	return runAsClientWithTUI(ctx, a2aClient, contextID, agentid)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
