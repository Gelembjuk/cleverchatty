# Reverse MCP Server Test

A real MCP server that connects to the Reverse MCP Connector via WebSocket.

This server exposes a single tool `exec_cmd` that executes shell commands on the machine where the server is running.

## Build

```bash
cd dev_tools/reverse_mcp_server
go mod tidy
go build -o reverse-mcp-server
```

## Usage

```bash
# Basic connection (no auth, no TLS)
./reverse-mcp-server -host localhost:9090 -name my-mcp-server

# With authentication token
./reverse-mcp-server -host localhost:9090 -name my-mcp-server -token secret123

# With TLS
./reverse-mcp-server -host localhost:9443 -name my-mcp-server -token secret123 -tls

# With TLS and self-signed certificate
./reverse-mcp-server -host localhost:9443 -name my-mcp-server -token secret123 -tls -insecure
```

## Command Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `-host` | `localhost:9090` | Host:port of the reverse MCP connector |
| `-name` | `reverse-mcp-server` | Server name to identify this MCP server |
| `-token` | (empty) | Authentication token |
| `-tls` | `false` | Use TLS (wss:// instead of ws://) |
| `-insecure` | `false` | Skip TLS certificate verification |

## Exposed Tool

### exec_cmd

Execute a Linux command with optional working directory.

**Parameters:**
- `command` (required): The full shell command to execute
- `working_dir` (optional): Working directory where the command should run

**Example usage (from AI agent):**
```
Use the exec_cmd tool with command "ls -la" to list files
```

## Configuration Example

Add this to your `config.json` on the connector side:

```json
{
  "tools_servers": {
    "my-mcp-server": {
      "transport": "reverse_mcp",
      "auth_token": "secret123"
    }
  },
  "reverse_mcp_settings": {
    "enabled": true,
    "listen_host": ":9090",
    "tls": {
      "enabled": false
    }
  }
}
```

## How It Works

1. The server connects to the Reverse MCP Connector via WebSocket
2. Authentication is done at connection time via headers/query params
3. The MCP protocol runs over the WebSocket using stdio transport
4. The connector can now call `exec_cmd` tool on this server
5. Commands are executed locally and results returned via MCP

## Security Note

⚠️ **WARNING**: This server executes arbitrary shell commands! Only use in trusted environments and with proper authentication configured.
