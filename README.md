# CleverChatty

**CleverChatty** is a Go package that implements the core functionality of an AI chat system. It encapsulates all business logic required for an AI-powered chat, while remaining independent of any specific user interface.

## Key Features

- Decoupled architecture: UI is not included in this package and is intended to be implemented separately. This allows for flexible front-end development across multiple platforms.
- Unified backend logic: Provides a single source of truth for chat behavior, making it easy to maintain and test.
- LLM prompt handling: Send prompts and receive responses from supported large language models.
- MCP server support: Built-in support for MCP servers as part of the chat infrastructure.

## Usage

This package is ideal for developers building custom UIs (e.g., CLI, web, mobile) that require AI chat capabilities without duplicating logic across implementations.

## Models 

The package works with any models supports by Ollama. Also: OpenAI, Anthropic and Google models by APIs.

## Example

```golang
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gelembjuk/cleverchatty"
)

func main() {
	config := cleverchatty.CleverChattyConfig{
		Model: "ollama:qwen2.5:3b",
		MCPServers: map[string]cleverchatty.ServerConfigWrapper{
			"weather_server": {
				Config: cleverchatty.SSEServerConfig{
					Url: "http://weather-service:8000/mcp",
				},
			},
			"get_location_server": {
				Config: cleverchatty.STDIOServerConfig{
					Command: "get_location",
					Args:    []string{"--location"},
				},
			},
		},
	}

	cleverChattyObject, err := cleverchatty.GetCleverChatty(config, context.Background())

	if err != nil {
		fmt.Errorf("Error creating assistant: %v", err)
		os.Exit(1)
	}
	defer func() {
		cleverChattyObject.Finish()
	}()

	response, err := cleverChattyObject.Prompt("What is the weather like outside today?")

	if err != nil {
		fmt.Errorf("Error getting response: %v", err)
		os.Exit(1)
	}
	fmt.Println("Response:", response)

}

```

## Credits

The first version of this application was the copy of [mcphost](https://github.com/mark3labs/mcphost) refactored to remove the UI.