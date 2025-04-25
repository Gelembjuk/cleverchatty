# CleverChatty

**CleverChatty** is a Go package that implements the core functionality of an AI chat system. It encapsulates all business logic required for an AI-powered chat, while remaining independent of any specific user interface.

## Key Features

- Decoupled architecture: UI is not included in this package and is intended to be implemented separately. This allows for flexible front-end development across multiple platforms.
- Unified backend logic: Provides a single source of truth for chat behavior, making it easy to maintain and test.
- LLM prompt handling: Send prompts and receive responses from supported large language models.
- MCP server support: Built-in support for MCP servers as part of the chat infrastructure.

## Usage

This package is ideal for developers building custom UIs (e.g., CLI, web, mobile) that require AI chat capabilities without duplicating logic across implementations.

## UI for CleverChatty

The package is designed to be used with any UI. However, if you are looking for a simple UI to test the package, you can use [CleverChatty CLI](https://github.com/Gelembjuk/cleverchatty-cli). It is a command-line interface that allows you to interact with the CleverChatty package easily.

## Models 

The package works with any models supports by Ollama. Also: OpenAI, Anthropic and Google models by APIs. The model is provided in the format: `provider:model`

Supported providers:

- `ollama` - Ollama models
- `anthropic` - Anthropic models
- `openai` - OpenAI models
- `google` - Google models

Examples:

- `ollama:llama2:7b` - Ollama model
- `anthropic:claude-2` - Anthropic model
- `openai:gpt-3.5-turbo` - OpenAI model
- `google:bert` - Google model

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

## Config

The package can help to parse the JSON file with the config for the application using it.

Example of the config:

```json
{
    "log_file_path": "",
    "model": "ollama:qwen2.5:3b",
    "mcpServers": {
        "weather_server": {
            "url": "http://weather-service:8000/mcp",
            "headers": [
            ]
        },
        "get_location_server": {
            "command": "get_location",
            "args": [
                "--location"
			]
        }
    },
    "anthropic": {
        "apikey": "sk-**************AA",
        "base_url": "https://api.anthropic.com/v1",
        "default_model": "claude-2"
    },
    "openai": {
        "apikey": "sk-********0A",
        "base_url": "https://api.openai.com/v1",
        "default_model": "gpt-3.5-turbo"
    },
    "google": {
        "apikey": "AI***************z4",
        "default_model": "google-bert"
    }
}
```

The example above can be transformed to this one

```golang
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gelembjuk/cleverchatty"
)


const configFile = "config.json"

func main() {
	config, err := cleverchatty.LoadMCPConfig(configFile)

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

The only required config is `model`. The rest of the config is optional. 

## Credits

The first version of this application was the copy of [mcphost](https://github.com/mark3labs/mcphost) refactored to remove the UI.