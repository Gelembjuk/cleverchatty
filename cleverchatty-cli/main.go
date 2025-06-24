package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	markdown "github.com/MichaelMure/go-term-markdown"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/list"
	"github.com/charmbracelet/log"
	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/mark3labs/mcphost/pkg/history"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	version          = "0.1.4"
	defaultModelFlag = "anthropic:claude-3-5-sonnet-latest"
)

var (
	renderer         *glamour.TermRenderer
	debugMode        bool
	configFile       string
	messageWindow    int
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

var (
	// Tokyo Night theme colors
	tokyoPurple = lipgloss.Color("99")  // #9d7cd8
	tokyoCyan   = lipgloss.Color("73")  // #7dcfff
	tokyoBlue   = lipgloss.Color("111") // #7aa2f7
	tokyoGreen  = lipgloss.Color("120") // #73daca
	tokyoRed    = lipgloss.Color("203") // #f7768e
	tokyoOrange = lipgloss.Color("215") // #ff9e64
	tokyoFg     = lipgloss.Color("189") // #c0caf5
	tokyoGray   = lipgloss.Color("237") // #3b4261
	tokyoBg     = lipgloss.Color("234") // #1a1b26

	promptStyle = lipgloss.NewStyle().
			Foreground(tokyoBlue)

	responseStyle = lipgloss.NewStyle().
			Foreground(tokyoFg)

	errorStyle = lipgloss.NewStyle().
			Foreground(tokyoRed).
			PaddingLeft(2).
			Bold(true)

	toolNameStyle = lipgloss.NewStyle().
			Foreground(tokyoCyan).
			PaddingLeft(2).
			Bold(true)

	descriptionStyle = lipgloss.NewStyle().
				Foreground(tokyoFg).
				PaddingLeft(2).
				PaddingBottom(1)

	contentStyle = lipgloss.NewStyle().
			Background(tokyoBg).
			PaddingLeft(4).
			PaddingRight(4)
)

var rootCmd = &cobra.Command{
	Use:   "cleverchatty-cli",
	Short: "Chat with AI models through a unified interface. Version: " + version,
	Long: `cleverchatty-cli is a CLI tool that allows you to interact with various AI models
through a unified interface. It supports various tools through MCP servers
and provides streaming responses.

Available models can be specified using the --model flag:
- Anthropic Claude (default): anthropic:claude-3-5-sonnet-latest
- OpenAI: openai:gpt-4
- Ollama models: ollama:modelname
- Google: google:modelname

Example:
  cleverchatty-cli -m ollama:qwen2.5:3b
  cleverchatty-cli -m openai:gpt-4
  cleverchatty-cli -m google:gemini-2.0-flash`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(context.Background())
	},
}

func init() {
	rootCmd.PersistentFlags().
		StringVar(&configFile, "config", "", "config file (default is $HOME/.mcp.json)")
	rootCmd.PersistentFlags().
		IntVar(&messageWindow, "message-window", 0, "number of messages to keep in context")
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
	if debugMode {
		config.DebugMode = true
	}
	if messageWindow > 0 {
		config.MessageWindow = messageWindow
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

	return config, nil
}

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Fallback width
	}
	return width - 20
}

func updateRenderer() error {
	width := getTerminalWidth()
	var err error
	renderer, err = glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.TokyoNightStyle),
		glamour.WithWordWrap(width),
	)
	return err
}
func handleSlashCommand(prompt string, cleverChattyObject cleverchatty.CleverChatty) (bool, error) {
	if !strings.HasPrefix(prompt, "/") {
		return false, nil
	}

	switch strings.ToLower(strings.TrimSpace(prompt)) {
	case "/tools":
		handleToolsCommand(cleverChattyObject)
		return true, nil
	case "/help":
		handleHelpCommand()
		return true, nil
	case "/version":
		handleVersionCommand()
		return true, nil
	case "/history":
		handleHistoryCommand(cleverChattyObject)
		return true, nil
	case "/servers":
		handleServersCommand(cleverChattyObject)
		return true, nil
	case "/quit":
		fmt.Println("\nGoodbye!")
		defer os.Exit(0)
		return true, nil
	default:
		fmt.Printf("%s\nType /help to see available commands\n\n",
			errorStyle.Render("Unknown command: "+prompt))
		return true, nil
	}
}
func handleHelpCommand() {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}
	var markdown strings.Builder

	markdown.WriteString("# Available Commands\n\n")
	markdown.WriteString("The following commands are available:\n\n")
	markdown.WriteString("- **/help**: Show this help message\n")
	markdown.WriteString("- **/tools**: List all available tools\n")
	markdown.WriteString("- **/servers**: List configured MCP servers\n")
	markdown.WriteString("- **/history**: Display conversation history\n")
	markdown.WriteString("- **/quit**: Exit the application\n")
	markdown.WriteString("\nYou can also press Ctrl+C at any time to quit.\n")

	markdown.WriteString("\n## Available Models\n\n")
	markdown.WriteString("Specify models using the --model or -m flag:\n\n")
	markdown.WriteString(
		"- **Anthropic Claude**: `anthropic:claude-3-5-sonnet-latest`\n",
	)
	markdown.WriteString("- **Ollama Models**: `ollama:modelname`\n")
	markdown.WriteString("\nExamples:\n")
	markdown.WriteString("```\n")
	markdown.WriteString("cleverchatty-cli -m anthropic:claude-3-5-sonnet-latest\n")
	markdown.WriteString("cleverchatty-cli -m ollama:qwen2.5:3b\n")
	markdown.WriteString("```\n")
	markdown.WriteString("\n\n## Version\n\n")
	markdown.WriteString("CleverChatty CLI version: " + version + "\n")

	rendered, err := renderer.Render(markdown.String())
	if err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error rendering help: %v", err)),
		)
		return
	}

	fmt.Print(rendered)
}

func handleVersionCommand() {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}
	var markdown strings.Builder

	markdown.WriteString("## CleverChatty CLI version: " + version + "\n")

	rendered, err := renderer.Render(markdown.String())
	if err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error rendering help: %v", err)),
		)
		return
	}

	fmt.Print(rendered)
}

func handleServersCommand(cleverChattyObject cleverchatty.CleverChatty) {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}

	var markdown strings.Builder
	action := func() {
		servers := cleverChattyObject.GetServersInfo()
		if len(servers) == 0 {
			markdown.WriteString("No servers configured.\n")
		} else {
			for _, server := range servers {
				markdown.WriteString(fmt.Sprintf("# %s\n\n", server.Name))

				if server.IsSSE() {
					markdown.WriteString("*Url*\n")
					markdown.WriteString(fmt.Sprintf("`%s`\n\n", server.Url))
					markdown.WriteString("*headers*\n")
					if server.Headers != nil {
						for _, header := range server.Headers {
							parts := strings.SplitN(header, ":", 2)
							if len(parts) == 2 {
								key := strings.TrimSpace(parts[0])
								markdown.WriteString("`" + key + ": [REDACTED]`\n")
							}
						}
					} else {
						markdown.WriteString("*None*\n")
					}

				} else {
					markdown.WriteString("*Command*\n")
					markdown.WriteString(fmt.Sprintf("`%s`\n\n", server.Command))

					markdown.WriteString("*Arguments*\n")
					if len(server.Args) > 0 {
						markdown.WriteString(fmt.Sprintf("`%s`\n", strings.Join(server.Args, " ")))
					} else {
						markdown.WriteString("*None*\n")
					}
				}

				markdown.WriteString("\n") // Add spacing between servers
			}
		}
	}

	_ = spinner.New().
		Title("Loading server configuration...").
		Action(action).
		Run()

	rendered, err := renderer.Render(markdown.String())
	if err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error rendering servers: %v", err)),
		)
		return
	}

	// Create a container style with margins
	containerStyle := lipgloss.NewStyle().
		MarginLeft(4).
		MarginRight(4)

	// Wrap the rendered content in the container
	fmt.Print("\n" + containerStyle.Render(rendered) + "\n")
}

func handleToolsCommand(cleverChattyObject cleverchatty.CleverChatty) {
	// Get terminal width for proper wrapping
	width := getTerminalWidth()

	// Adjust width to account for margins and list indentation
	contentWidth := width - 12 // Account for margins and list markers

	results := cleverChattyObject.GetToolsInfo()
	// If tools are disabled (empty client map), show a message
	if len(results) == 0 {
		fmt.Print(
			"\n" + contentStyle.Render(
				"Tools are currently disabled for this model.\n",
			) + "\n\n",
		)
		return
	}

	// Create a list for all servers
	l := list.New().
		EnumeratorStyle(lipgloss.NewStyle().Foreground(tokyoPurple).MarginRight(1))

	for _, server := range results {
		if server.Err != nil {
			fmt.Printf(
				"\n%s\n",
				errorStyle.Render(
					fmt.Sprintf(
						"Error fetching tools from %s: %v",
						server.Name,
						server.Err,
					),
				),
			)
			continue
		}

		// Create a sublist for each server's tools
		serverList := list.New().EnumeratorStyle(lipgloss.NewStyle().Foreground(tokyoCyan).MarginRight(1))

		if len(server.Tools) == 0 {
			serverList.Item("No tools available")
		} else {
			for _, tool := range server.Tools {
				// Create a description style with word wrap
				descStyle := lipgloss.NewStyle().
					Foreground(tokyoFg).
					Width(contentWidth).
					Align(lipgloss.Left)

				// Create a description sublist for each tool
				toolDesc := list.New().
					EnumeratorStyle(lipgloss.NewStyle().Foreground(tokyoGreen).MarginRight(1)).
					Item(descStyle.Render(tool.Description))

				// Add the tool with its description as a nested list
				serverList.Item(toolNameStyle.Render(tool.Name)).
					Item(toolDesc)
			}
		}

		// Add the server and its tools to the main list
		l.Item(server.Name).Item(serverList)
	}

	// Create a container style with margins
	containerStyle := lipgloss.NewStyle().
		Margin(2).
		Width(width)

	// Wrap the entire content in the container
	fmt.Print("\n" + containerStyle.Render(l.String()) + "\n")
}
func handleHistoryCommand(cleverChattyObject cleverchatty.CleverChatty) {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}

	var markdown strings.Builder
	markdown.WriteString("# Conversation History\n\n")

	for _, msg := range cleverChattyObject.GetMessages() {
		roleTitle := "## User"
		if msg.Role == "assistant" {
			roleTitle = "## Assistant"
		} else if msg.Role == "system" {
			roleTitle = "## System"
		}
		markdown.WriteString(roleTitle + "\n\n")

		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				markdown.WriteString("### Text\n")
				markdown.WriteString(block.Text + "\n\n")

			case "tool_use":
				markdown.WriteString("### Tool Use\n")
				markdown.WriteString(
					fmt.Sprintf("**Tool:** %s\n\n", block.Name),
				)
				if block.Input != nil {
					prettyInput, err := json.MarshalIndent(
						block.Input,
						"",
						"  ",
					)
					if err != nil {
						markdown.WriteString(
							fmt.Sprintf("Error formatting input: %v\n\n", err),
						)
					} else {
						markdown.WriteString("**Input:**\n```json\n")
						markdown.WriteString(string(prettyInput))
						markdown.WriteString("\n```\n\n")
					}
				}

			case "tool_result":
				markdown.WriteString("### Tool Result\n")
				markdown.WriteString(
					fmt.Sprintf("**Tool ID:** %s\n\n", block.ToolUseID),
				)
				switch v := block.Content.(type) {
				case string:
					markdown.WriteString("```\n")
					markdown.WriteString(v)
					markdown.WriteString("\n```\n\n")
				case []history.ContentBlock:
					for _, contentBlock := range v {
						if contentBlock.Type == "text" {
							markdown.WriteString("```\n")
							markdown.WriteString(contentBlock.Text)
							markdown.WriteString("\n```\n\n")
						}
					}
				}
			}
		}
		markdown.WriteString("---\n\n")
	}

	// Render the markdown
	rendered, err := renderer.Render(markdown.String())
	if err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error rendering history: %v", err)),
		)
		return
	}

	// Print directly without box
	fmt.Print("\n" + rendered + "\n")
}
func releaseActionSpinner() {
	if actionInProgress {
		actionInProgress = false
		actionChannel <- true
		<-actionCanceledChannel
	}
}

func showSpinner(text string) {
	if actionInProgress {
		releaseActionSpinner()
	}
	actionInProgress = true
	go func() {
		_ = spinner.New().Title(text).Action(func() {
			<-actionChannel
		}).Run()

		actionCanceledChannel <- true
	}()
}
func run(ctx context.Context) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}
	cleverChattyObject, err := cleverchatty.GetCleverChatty(*config, ctx)

	if err != nil {
		return fmt.Errorf("error creating assistant: %v", err)
	}

	//cleverChattyObject.WithLogger(log)

	defer func() {

		log.Info("Shutting down CleverChatty core...")

		cleverChattyObject.Finish()

	}()

	cleverChattyObject.Callbacks.SetStartedPromptProcessing(func(prompt string) error {
		fmt.Printf("\n%s%s\n\n", promptStyle.Render("You: "), markdown.Render(prompt, 80, 6))
		return nil
	})
	cleverChattyObject.Callbacks.SetStartedThinking(func() error {
		showSpinner("💭  Thinking...")
		return nil
	})
	cleverChattyObject.Callbacks.SetMemoryRetrievalStarted(func() error {
		showSpinner("🕰️  Recalling...")
		return nil
	})
	cleverChattyObject.Callbacks.SetRAGRetrievalStarted(func() error {
		showSpinner("🗃️  Searching knowledge database ...")
		return nil
	})
	cleverChattyObject.Callbacks.SetToolCalling(func(toolName string) error {
		showSpinner("🔧 Using tool: " + toolName)
		return nil
	})
	cleverChattyObject.Callbacks.SetToolCallFailed(func(toolName string, err error) error {
		releaseActionSpinner()
		fmt.Printf("\n%s\n", errorStyle.Render("Error using tool: "+toolName))
		return nil
	})
	cleverChattyObject.Callbacks.SetResponseReceived(func(response string) error {
		releaseActionSpinner()
		fmt.Printf("\n%s%s\n\n", responseStyle.Render("Assistant: "), markdown.Render(response, 80, 6))
		return nil
	})

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

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
