# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

This is a Go workspaces project (go 1.24.3). No Makefile exists.

```bash
# Build core library
cd core && go build

# Build CLI application
cd cleverchatty-cli && go build

# Build server application
cd cleverchatty-server && go build

# Clean up dependencies for a module
cd <module-dir> && go mod tidy

# Run tests (from core directory)
cd core && go test ./...

# Run a single test
cd core && go test -run TestBasicChat

# Install binaries
go install github.com/gelembjuk/cleverchatty/cleverchatty-cli@latest
go install github.com/gelembjuk/cleverchatty/cleverchatty-server@latest
```

## Project Structure

Go workspace with three main modules:
- **core/** - Core library with business logic, LLM providers, tool hosting
- **cleverchatty-cli/** - Terminal UI application (Bubble Tea framework, Cobra CLI)
- **cleverchatty-server/** - Daemon server with A2A protocol support
- **dev_tools/** - Development utilities (email mock, notification test clients)

## Architecture Overview

### LLM Provider Pattern
Located in `core/llm/`. Providers implement common interface for message creation, tool responses, and token tracking.
- `anthropic/` - Anthropic Claude
- `openai/` - OpenAI GPT
- `google/` - Google Generative AI
- `ollama/` - Local Ollama models
- `mock/` - Testing provider

Model format: `provider:model_name` (e.g., `ollama:llama2:7b`, `anthropic:claude-2`)

### Tool/MCP Hosting
`core/tools.go` manages `ToolsHost` which supports multiple transport types:
- STDIO - Local process communication
- HTTP_STREAMING - Streamable HTTP
- SSE - Server-sent events
- A2A - Agent-to-Agent protocol
- REVERSE_MCP - WebSocket-based reverse connection

### Session Management
`core/session.go` provides `SessionManager` for multi-client scenarios. Each session maintains its own `CleverChatty` instance with configurable timeout (default 3600s).

### Callback System
`core/callbacks.go` defines `UICallbacks` for UI event notifications:
- `startedPromptProcessing`, `startedThinking`, `responseReceived`
- `toolCalling`, `toolCallFailed`
- `memoryRetrievalStarted`, `ragRetrievalStarted`

### Message History
`core/history/` manages conversation history with content blocks (text, tool_use, tool_result). Supports window pruning for context management.

### Notification System
`core/notification.go` provides unified notification handling from MCP/A2A sources with monitoring and processing status tracking.

#### Notification Processing Flow
When an MCP server sends a notification:
1. `ToolsHost` receives the raw MCP notification and converts it to a unified `Notification` struct
2. If the notification matches configured monitoring rules (in `tools_servers[name].notification_instructions`), it's marked as "monitored"
3. Monitored notifications are enqueued in `NotificationProcessor` for LLM-based processing
4. The processor's dedicated agent evaluates the notification against user instructions
5. If the agent determines the user should be notified, it calls the `notification_feedback` tool

#### Agent Message Delivery
The `notification_feedback` tool triggers the `AgentMessageCallback` chain:
```
notification_feedback tool called
    ↓
AgentMessageCallback (in NotificationProcessor)
    ↓
Closure in CleverChatty.SetNotificationCallback:
  1. Adds message to assistant.messages via history.NewAgentNotificationMessage()
  2. Calls assistant.agentMessageCallback
    ↓
SessionManager.agentMessageCallback
    ↓
A2AServer.BroadcastAgentMessage() - sends "agent_message" event to all subscribers
    ↓
CLI receives event and displays in chat view (tuiSendAgentMessage)
```

#### Key Types
- `NotificationCallback func(notification Notification)` - for raw notification events
- `AgentMessageCallback func(message string)` - for processed agent messages to user
- `NotificationProcessor` - queue-based processor with dedicated LLM agent

#### Configuration Example
```json
{
  "tools_servers": {
    "email-server": {
      "notification_instructions": {
        "notifications/message": ["Tell me if there's an urgent email"]
      }
    }
  }
}
```

## Configuration

JSON config file `cleverchatty_config.json` with:
- `model` - LLM provider and model
- `system_instruction` - System prompt
- `tools_servers` - MCP servers and A2A agents configuration
- `a2a_settings` - A2A server configuration
- `reverse_mcp_settings` - Reverse MCP listener
- Provider credentials (`anthropic`, `openai`, `google`)

See `docs/Config.md` for full reference.

## Entry Points

**CLI**: `cleverchatty-cli/main.go` - Cobra commands, Bubble Tea TUI
- Standalone mode: local LLM and MCP servers
- Client mode: connects to CleverChatty server via A2A

**Server**: `cleverchatty-server/`
- `start` - daemon mode
- `stop` - stop daemon
- `run` - interactive mode

**Library**: `core.GetCleverChatty()` factory function

## Testing

Tests use mock providers (`mock:mock`) and mock MCP clients. Test files in `core/`:
- `action_test.go` - Basic chat and tool calling tests
- `setup_test.go` - Initialization tests
- Mock implementations in `core/test/`

## Key Dependencies

- `github.com/mark3labs/mcp-go` - MCP protocol
- `trpc.group/trpc-go/trpc-a2a-go` - A2A protocol
- `github.com/charmbracelet/bubbletea` - Terminal UI
- `github.com/spf13/cobra` - CLI framework
