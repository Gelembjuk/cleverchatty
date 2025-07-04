# Configuration of CleverChatty server and client.

Same confiog is used for both server and client. The server could require some more sections.

```json
{
    "agent_id":"secretary",
    "log_file_path": "log_file.txt",
    "debug_mode": false,
    "model": "ollama:qwen2.5:3b",
    "system_instruction":"You are the helpful assistant.",
    "tools_servers": {
        "some_mcp_stdio_server": {
            "command": "npx",
            "args": [
                "mcp-stdio-server"
            ],
            "env": {
            }
        },
        "some_mcp_streaming_http_server": {
            "url": "https://host/mcp",
            "headers": {
                "Authorization": "Bearer YOUR_ACCESS_TOKEN"
            }
        },
        "some_mcp_sse_server": {
            "transport": "sse",
            "url": "https://host/sse",
            "headers": {
                "Authorization": "Bearer YOUR_ACCESS_TOKEN"
            }
        },
        "some_a2a_server": {
            "endpoint": "http://ai_agent_host/",
            "metadata": {
                "agent_id": "{CLIENT_AGENT_ID}",
                "token": "SECRET_TOKEN"
            }
        }
    },
    "rag_settings": {
        "context_prefix": "Context: ",
        "require_preprocessing": true,
        "preprocessing_prompt": "Extract the most relevant keyword or phrase from the provided text.",
    },
    "server":{
        "session_timeout": 3600
    },
    "a2a_settings":{
        "enabled":true,
        "agent_id_required":true,
        "url":"http://localhost:8080/",
        "listen_host":"0.0.0.0:8080",
        "title":"Organization secretary"
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

## "agent_id"  

Is used to identify the agent in the A2A protocol or in requests to MCP servers.

This value will replace the template `{CLIENT_AGENT_ID}` in the A2A server metadata or in MCP server arguments or headers.

## "log_file_path"

Specifies the file path where the logs will be stored. This is useful for debugging and monitoring the agent's activity.

The value "stdout" can be used to log to the standard output.

If a path is relative then it is relative to the config file directory.

## "debug_mode"

If set to `true`, the agent will log additional debug information. This is useful for development and troubleshooting.

## "model"

Specifies the model to be used by the agent. This includes the provider and the model name. The format is `<provider>:<model_name>`.

Supported providers are:
- `ollama` - Ollama models
- `anthropic` - Anthropic models
- `openai` - OpenAI models
- `google` - Google models

In case of Ollama models, the model name can include the version, e.g. `ollama:llama2:7b`. And it is presumed Ollama is installed, running and the model is available.

## "system_instruction"

Optional.

Specifies the instruction to be given to the LLM on the beginning of each session. It is used to set the context for the agent's behavior. The instruction should be concise and clear.

## "tools_servers"

Specifies the configuration for the tools servers that the agent can use. This includes both MCP Servers andf A2A agents.

MCP servers are supported in three transports:
- `stdio` - Standard input/output transport
- `http_streaming` - Streaming http transport
- `sse` - Server-sent events transport

### STDIO MCP server

The record must include the `command` field with the command to run the MCP server, and optionally `args` and `env` fields for additional command arguments and environment variables. It is very similar to the config format used in many other AI tools.

```json
"some_mcp_stdio_server": {
    "command": "npx",
    "args": [
        "mcp-stdio-server"
    ],
    "env": {
    }
}
```

### Streaming HTTP MCP server

The record must include the `url` field with the server URL and optionally `headers` for authentication or other purposes.

```json
"some_mcp_streaming_http_server": {
    "url": "https://host/mcp",
    "headers": {
        "Authorization": "Bearer YOUR_ACCESS_TOKEN"
    }
}
```

### SSE MCP server

The record must include the `transport` field set to `"sse"` and the `url` field with the server URL. Optionally, you can include headers for authentication or other purposes.

```json
"some_mcp_sse_server": {
    "transport": "sse",
    "url": "https://host/sse",
    "headers": {
        "Authorization": "Bearer YOUR_ACCESS_TOKEN"
    }
}
```

### A2A Agent server

AI Agents supporting A2A protocol can be connected to the CleverChatty as a tool. It works with same principles as MCP servers. Every "skill" of the agent is a tool that can be called by the agent with the only string argument - Message.

```json
"some_a2a_server": {
    "endpoint": "http://ai_agent_host/",
    "metadata": {
        "agent_id": "{CLIENT_AGENT_ID}"
    }
}
```

Limitations of usage of A2A agents as a tool servers:
- A task must be finished in 5 seconds.
- Streaming of artifacts is not supported.
- Additional input request is not supported. (but it is expected to be implemented soon)

**Note**. In this context the CleverChatty acts as a client for the A2A server. It sends requests to some A2A server and receives responses from it.

### Tools interfaces

A tool interface is a "native invention" in this project. It allows to define a tool description required for specific tool server.

Currently there are two interfaces supported:
- `rag` - for RAG (Retrieval-Augmented Generation) tools working over MCP or A2A protocols.
- `memory` - for AI memory tools working over MCP or A2A protocols.

Any tools server listed in the configuration can implement these interfaces. But it must be only one tool with the specific interface per server.

Use the `interface` field to specify the interface type for the tool server.

```json
{
    "agent_id":"secretary",
    "log_file_path": "log_file.txt",
    "debug_mode": false,
    "model": "ollama:qwen2.5:3b",
    "system_instruction":"You are the helpful assistant.",
    "tools_servers": {
        "My_RAG_Server": {
            "url": "http://localhost:8002/sse",
            "headers": [
            ],
            "interface": "rag"
        },
        "My_Memory_Server": {
            "command": "npx",
            "args": [
                "mcp-stdio-server"
            ],
            "env": {
            },
            "interface": "memory"
        }
    }
}
```

## "rag_settings"

Settings for the RAG (Retrieval-Augmented Generation) feature. It allows to provide additional context to the agent based on the user query.

It is used only if there is a tool server with the `rag` interface in the `tools_servers` section.

- `context_prefix`: A prefix to be added to the context provided by the RAG server. It helps to distinguish the context from the user query. The default value is `"Context: "`. 
- `require_preprocessing`: If set to `true`, the agent will preprocess the user query before sending it to the RAG server. The default value is `false`.
- `preprocessing_prompt`: The prompt to be used for preprocessing the user query. It is used only if `require_preprocessing` is set to `true`. The default value is `"Extract the most relevant keyword or phrase from the provided text."`.

Use the `require_preprocessing` set to `true` to enable preprocessing only if your connected RAG server requires it. If your server is just a search engine, you can set it to `true`. Because it will not be able to search by the full user's prompt.

But if your RAG server is some kind of vectorized search engine, you can set it to `false` and the agent will send the full user query to the RAG server.

## "server"

Settings for the CleverChatty server.

- `session_timeout`: The idle timeout for the user/client_agent session in seconds. After this time, the session will be closed and the user will need to start a new session. The default value is `3600` seconds (1 hour).

## "a2a_settings"

Settings for the A2A (Agent-to-Agent) server feature. It is used only by the CleverChatty server. It defines how the server will accept A2A requests and how it will respond to them.

When this is enabled, the CleverChatty server will listen for incoming A2A requests and will be able to respond to them. Each request is like a chat prompt with a single message. The response is a single message as well.

Most of options are visible in the A2A Agent Card. The server will expose only one skill - `ai_chat`, which is used to chat with the agent.

- `enabled`: If set to `true`, the A2A server will be enabled. The default value is `false`.
- `agent_id_required`: If set to `true`, the A2A server will require the `agent_id` in the request metadata. The default value is `false`.
- `url`: The URL of the A2A server. It is used to send requests to the server. It must be a valid URL. Displayed in the A2A Agent Card.
- `listen_host`: The host and port where the A2A server will listen for incoming requests. It must be in the format like `0.0.0.0:8000` (includes IP and port).
- `title`: The title of the AI agent. It is used to identify the agent in the A2A requests. Displayed in the A2A Agent Card.
- `description`: The description of the AI agent. It is used to provide additional information about the agent in the A2A requests. Displayed in the A2A Agent Card.
- `organization`: The organization that owns the AI agent. It is used to provide additional context about the agent in the A2A requests. Displayed in the A2A Agent Card.
- `chat_skill_name`: The name of the skill of the AI agent. It is used to identify the skill in the A2A requests. Displayed in the A2A Agent Card.
- `chat_skill_description`: The description of the skill of the AI agent. It is used to provide additional information about the skill in the A2A requests. Displayed in the A2A Agent Card.
