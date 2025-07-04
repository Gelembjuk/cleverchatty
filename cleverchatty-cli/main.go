package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	markdown "github.com/MichaelMure/go-term-markdown"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
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
func composeCallbacks() cleverchatty.UICallbacks {
	callbacks := cleverchatty.UICallbacks{}

	callbacks.SetStartedPromptProcessing(func(prompt string) error {
		fmt.Printf("\n%s%s\n\n", promptStyle.Render("You: "), markdown.Render(prompt, 80, 6))
		return nil
	})
	callbacks.SetStartedThinking(func() error {
		showSpinner("💭  Thinking...")
		return nil
	})
	callbacks.SetMemoryRetrievalStarted(func() error {
		showSpinner("🕰️  Recalling...")
		return nil
	})
	callbacks.SetRAGRetrievalStarted(func() error {
		showSpinner("🗃️  Searching knowledge database ...")
		return nil
	})
	callbacks.SetMemoryRetrievalStarted(func() error {
		showSpinner("🕰️  Recalling...")
		return nil
	})
	callbacks.SetRAGRetrievalStarted(func() error {
		showSpinner("🗃️  Searching knowledge database ...")
		return nil
	})
	callbacks.SetToolCalling(func(toolName string) error {
		showSpinner("🔧 Using tool: " + toolName)
		return nil
	})
	callbacks.SetToolCallFailed(func(toolName string, err error) error {
		releaseActionSpinner()
		fmt.Printf("\n%s\n", errorStyle.Render("Error using tool: "+toolName))
		return nil
	})
	callbacks.SetResponseReceived(func(response string) error {
		releaseActionSpinner()
		fmt.Printf("\n%s%s\n\n", responseStyle.Render("Assistant: "), markdown.Render(response, 80, 6))
		return nil
	})

	return callbacks
}
func runAsStandalone(ctx context.Context) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}
	cleverChattyObject, err := cleverchatty.GetCleverChatty(*config, ctx)

	if err != nil {
		return fmt.Errorf("error creating assistant: %v", err)
	}

	cleverChattyObject.Init()
	if err != nil {
		return fmt.Errorf("error initializing assistant: %v", err)
	}

	defer func() {

		log.Info("Shutting down CleverChatty core...")

		cleverChattyObject.Finish()

	}()

	cleverChattyObject.Callbacks = composeCallbacks()

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

func runAsClient(ctx context.Context) error {
	// 1. Check for streaming capability by fetching the agent card
	check, err := checkServerIsCleverChatty(server)
	if err != nil {
		return fmt.Errorf("error checking server capabilities: %v", err)
	}

	err = sendHelloMessage(ctx, server, agentid)
	if err != nil {
		// If the server does not support CleverChatty AI chat, we return an error
		// probably, agentid is not set or is wrong
		return err
	}

	// 2. Create a new client instance.
	a2aClient, err := a2aclient.NewA2AClient(server)
	if err != nil {
		return fmt.Errorf("error creating A2A client: %v", err)
	}

	if !check {
		return fmt.Errorf("the server at %s does not support CleverChatty AI chat", server)
	}

	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error initializing renderer: %v", err)
	}
	// context id
	contextID := fmt.Sprintf("session-%d-%s", time.Now().UnixNano(), uuid.New().String())

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
		handled, err := handleSlashCommandAsClient(prompt, *a2aClient, ctx, contextID)
		if err != nil {
			return err
		}
		if handled {
			continue
		}

		message := a2aprotocol.Message{
			Role: a2aprotocol.MessageRoleUser,
			Parts: []a2aprotocol.Part{
				a2aprotocol.NewTextPart(prompt),
			},
			ContextID: &contextID, // Use the context ID for the session
			Metadata: map[string]any{
				"agent_id": agentid,
			},
		}

		taskParams := a2aprotocol.SendMessageParams{
			Message: message,
		}

		streamChan, err := a2aClient.StreamMessage(ctx, taskParams)
		if err != nil {
			return fmt.Errorf("error starting task stream: %v", err)
		}
		_, err = processA2AStreamEvents(ctx, streamChan, composeCallbacks())
		if err != nil {
			return fmt.Errorf("error processing task stream events: %v", err)
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
