# CleverChatty

**CleverChatty** is a Go package that implements the core functionality of an AI chat system. It encapsulates all business logic required for an AI-powered chat, while remaining independent of any specific user interface.

Also, this package includes the CleverChatty Server, which can be started as a daemon and service many clients with A2A protocol. The server is designed to handle requests from various clients, humans or AI agents, and it can be used to manage multiple AI models and MCP servers.

![<img src="docs/cleverchatty_core.png" width="250"/>](docs/cleverchatty_core.png)

## Key Features

- Decoupled architecture: UI is not included in this package and is intended to be implemented separately. This allows for flexible front-end development across multiple platforms.
- Unified backend logic: Provides a single source of truth for chat behavior, making it easy to maintain and test.
- LLM prompt handling: Send prompts and receive responses from supported large language models.
- MCP server support: Built-in support for MCP servers as part of the chat infrastructure.
- A2A protocol: Implements the A2A (Agent-to-Agent) protocol for communication between AI agents and clients. It can work as A2A server and as a client for the A2A protocol to send requests to other A2A servers.

## Usage

This package is ideal for developers building custom UIs (e.g., CLI, web, mobile) that require AI chat capabilities without duplicating logic across implementations.

## UI for CleverChatty

The package is designed to be used with any UI. However, if you are looking for a simple UI to test the package, you can use [CleverChatty CLI](docs/Cli.md). It is a command-line interface that allows you to interact with the CleverChatty package easily.

The CleverChatty CLI can be used in two modes:
- As a standalone tool that runs locally and communicates with LLMs and MCP servers.
- As a client for the CleverChatty server, which allows you to connect to a remote CleverChatty server and use its capabilities without managing models or MCP servers locally.

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

## Running a server

To run a CleverChatty server, you can use the `cleverchatty-server` package. It provides a simple way to start a server that can handle requests from clients using the A2A protocol.

Quick run of the server. It is needed to install it first:

```bash
git clone https://github.com/gelembjuk/cleverchatty.git
cd cleverchatty/cleverchatty-server
go build
./cleverchatty-server start --directory /path/to/directory
```

`/path/to/directory` where the server config is stored.  

To stop the server, you can use the command:

```bash
./cleverchatty-server stop
```

### Config of the server.

The folder should contains the `cleverchatty_config.json` file with the configuration of the server. The config file is used to specify the model, MCP servers, and other options.

```json 
{
    "log_file_path": "log.txt",
    "model": "ollama:qwen2.5:3b",
    "mcpServers": {
        "File_Storage_Server": {
            "url": "http://localhost:8000/sse",
            "headers": [
                "Authorization: Basic Q0FSQk9OXHRlc3RfdXNlcl9hZG1pbjpDZXJ2ZXphMQ=="
            ]
        }
    },
    "a2a_settings":{
		"enabled":true,
		"url":"http://somehost:8080/",
		"listen_host":"0.0.0.0:8080",
		"title":"Nice Smart AI Agent"
    }
}
```

The section `a2a_settings` is used to configure the A2A protocol server. It allows to specify the URL of the server, the host to listen on, and the title of the AI agent.

While the server is running, you can connect to it using the CleverChatty CLI or any other A2A client. 

```
go run github.com/gelembjuk/cleverchatty/cleverchatty-cli@latest --server http://somehost:8080
```

## Example

It is possible to create custom AI Assistants using this package.

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

### Usig the memory server

This tools allows to use long-term AI memory using a MCP server of specific interface.

```
"mcpServers": {
        "Memory_Server": {
            "url": "http://localhost:8001/sse",
            "headers": [
            ],
            "interface": "memory"
        },
        .... other MCP servers
    },
```

The value "interface": "memory" indicates that this server is a memory server. It will be used to store the chat history and the AI will be able to use it in the future conversations.

The interface requires a MCP server to have two tools - `remember` and `recall`. 

Example:

```python
@mcp.tool()
def remember(role: str, contents) -> str:
    """Remembers new data in the memory
    
    Args:
        role (str): The role of the data, e.g. "user", "assistant"
        contents (str): The contents to remember, usually the text of the message
    """

    Memory(config).remember(role, contents)

    return "ok"

@mcp.tool()
def recall(query: str = "") -> str:
    """Recall the memory"""
    
    r = Memory(config).recall(query)

    if not r:
        return "none"
    
    return r
```

### `remember` tool accepts two arguments:
- `role`: The role of the data, e.g. "user", "assistant"
- `contents`: The contents to remember, usually the text of the message

### `recall` tool accepts one argument:
- `query`: The query to search for the data in the memory. If empty, it is expected to return some common memories.



### Using the Retrieval-Augmented Generation server

This tools allows to use RAG using a MCP server of specific interface.

```
"mcpServers": {
        "RAG_Server": {
            "url": "http://localhost:8002/sse",
            "headers": [
            ],
            "interface": "rag"
        },
        .... other MCP servers
    },
```
The value "interface": "rag" indicates that this server is a RAG server. It will be used to store the chat history and the AI will be able to use it in the future conversations.

The expected interface is the one tool named `knowledge_search` with two arguments: `query` and `num`. 

Example:

```golang
func createServer() *server.MCPServer {
	// Create MCP server
	s := server.NewMCPServer(
		"Retrieval-Augmented Generation Server. Adding the company data to the AI chat.",
		"1.0.0",
	)

	execTool := mcp.NewTool("knowledge_search",
		mcp.WithDescription("Search for documents and information over the company data storage"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The query to search for documents and information"),
		),
		mcp.WithNumber("num",
			mcp.Description("The number of results to return"),
		),
	)

	s.AddTool(execTool, cmdKnowledgeSearch)
	return s
}
```

To run the chat using this config stored in teh `config.json`, you can use the command:

```
 go run github.com/gelembjuk/cleverchatty-cli@latest --config config.json
```

The only required config is `model`. The rest of the config is optional. 

## Credits

The first version of this application was the copy of [mcphost](https://github.com/mark3labs/mcphost) refactored to remove the UI.