package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/list"
	cleverchatty "github.com/gelembjuk/cleverchatty/core"
	"github.com/mark3labs/mcphost/pkg/history"
	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
	a2aprotocol "trpc.group/trpc-go/trpc-a2a-go/protocol"
)

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
func handleSlashCommandAsClient(prompt string, a2aClient a2aclient.A2AClient, ctx context.Context, contextID string) (bool, error) {
	if !strings.HasPrefix(prompt, "/") {
		return false, nil
	}

	switch strings.ToLower(strings.TrimSpace(prompt)) {
	case "/help":
		handleHelpCommand()
		return true, nil
	case "/version":
		handleVersionCommand()
		return true, nil
	case "/quit":
		taskParams := a2aprotocol.SendMessageParams{
			Message: a2aprotocol.Message{
				Role: a2aprotocol.MessageRoleUser,
				Parts: []a2aprotocol.Part{
					a2aprotocol.NewTextPart("/bye"),
				},
				ContextID: &contextID,
			},
			Metadata: map[string]any{
				"agentid": agentid,
			},
		}

		a2aClient.SendMessage(ctx, taskParams)

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

				if server.IsMCPSSEServer() || server.IsMCPHTTPStreamingServer() {
					markdown.WriteString("*Transport*\n")
					if server.IsMCPHTTPStreamingServer() {
						markdown.WriteString("HTTP Streaming\n\n")
					} else {
						markdown.WriteString("SSE\n\n")
					}
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
				} else if server.IsA2AServer() {
					markdown.WriteString("*Transport*\n")
					markdown.WriteString("A2A\n\n")
					markdown.WriteString("*Endpoint*\n")
					markdown.WriteString(fmt.Sprintf("`%s`\n\n", server.Endpoint))
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
