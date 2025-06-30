# CleverChatty CLi

The CLI application CleverChatty.

This application is the command line AI chat tool. It allows to communicate with variety of LLMs and it has support of external tools calls using the MCP(Model Context Protocol).

It can be execute as a standalone tool or as a client for the CleverChatty server.

## Quick run

### As a standalone tool

Install some model with ollama, for example, `qwen2.5:3b`:

```
ollama list
NAME                   ID              SIZE      MODIFIED    
.... 
qwen2.5:3b             357c53fb659c    1.9 GB    3 weeks ago 
....
```

And chat with this model.

```
go run github.com/gelembjuk/cleverchatty/cleverchatty-cli@latest -m ollama:qwen2.5:3b
```

Or, when you clone the repo, you can run it with:

```
cd cleverchatty-cli
go build
./cleverchatty-cli -m ollama:qwen2.5:3b
```

This tool is really useful when you use some MCP servers. It is possible by creating a config file and adding the list of MCP servers to it.

### As a client for the CleverChatty server

Presume, you have a CleverChatty server running on `somehost:8000` (it is the A2A protocol server), you can run the CLI tool as a client for the server:

```
go run github.com/gelembjuk/cleverchatty/cleverchatty-cli@latest --server http://somehost:8000
```

In this mode you do not need to specify a model, to install and manage it, as the server will handle the request.

## Config

Thye config is an option only for the standalone mode. It allows to specify the model, the MCP servers, and other options.

The config is not required. Most options from the config can be overriden with the command line arguments.

All supported models providers are:

- ollama
- google
- anthropic
- openai

For openai, anthropic, google it is needed to have additional arguments - API key and API endpoint in some cases. See the config file example below. It is possible to provide the additional arguments with the config file or with the command line options.

The config should be stored in `config.json` file.

```json
{
    "log_file_path": "",
    "model": "ollama:qwen2.5:3b",
    "system_instruction": "Talk to a user like you are Elvis Presley.",
    "mcpServers": {
        "File_Storage_Server": {
            "url": "http://localhost:8000/sse",
            "headers": [
                "Authorization: Basic Q0F************MQ=="
            ]
        },
        "Some_STDIO_Server": {
            "command": "uv",
            "args": [
                "-m",
                "uvicorn",
                "example:app",
                "--host"
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

The section `mcpServers` contains all MCP servers config. It supports both STDIO and SSE servers.

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

## Installation

### Prerequisites
- Go 1.20 or later
- Ollama installed and running

### Install

```
go install github.com/gelembjuk/cleverchatty-cli@latest
```