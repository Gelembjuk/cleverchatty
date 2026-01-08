# Reverse MCP Connector Test Client

A simple test client that acts as an MCP server connecting to the Reverse MCP Connector via WebSocket.

## Build

```bash
cd dev_tools/listener_verify
go build -o test-mcp-client
```

## Usage

```bash
# Basic connection (no auth)
./test-mcp-client -host localhost:9090 -name my-mcp-server

# With authentication token
./test-mcp-client -host localhost:9090 -name my-mcp-server -token secret123

# With TLS
./test-mcp-client -host localhost:9443 -name my-mcp-server -token secret123 -tls
```

## Command Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `-host` | `localhost:9090` | Host:port of the reverse MCP connector |
| `-name` | `test-mcp-server` | Server name to identify this MCP server |
| `-token` | (empty) | Authentication token |
| `-tls` | `false` | Use TLS (wss:// instead of ws://) |

## Registered Test Tools

The client registers 3 sample tools:

1. **echo** - Echoes back the input message
   - Input: `{"message": "string"}`

2. **add** - Adds two numbers together
   - Input: `{"a": number, "b": number}`

3. **get_time** - Returns the current server time
   - Input: (none)

## Configuration Example

To test with the reverse MCP connector, add this to your `config.json`:

```json
{
  "tools_servers": {
    "test-mcp-server": {
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

Then run:
```bash
# Start the server
./cleverchatty-server daemon -c config.json

# In another terminal, start the test client
./test-mcp-client -host localhost:9090 -name test-mcp-server -token secret123
```
