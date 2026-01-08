# Reverse MCP Connector

The Reverse MCP Connector allows the CleverChatty server to accept incoming connections from remote MCP servers. This is useful when the MCP servers are running behind firewalls or NATs and cannot be directly accessed by CleverChatty, or when you want to centrally manage connections.

In the standard MCP model, the client (CleverChatty) initiates the connection to the server (Stdio, SSE, etc.). In the **Reverse MCP** model, CleverChatty opens a listener (WebSocket), and remote MCP servers initiate the connection to CleverChatty.

## Architecture

1.  **CleverChatty Server** starts a "Connector" listener (WebSocket server) on a configured port.
2.  **Remote MCP Servers** (running on edge devices, local machines, etc.) connect to this WebSocket endpoint.
3.  Upon connection, they authenticate using a token.
4.  Once connected, CleverChatty can discover tools and call them exactly as if they were local stdio servers.

## Configuration

### 1. Enable the Connector (Listener)

In your `config.json`, add the `reverse_mcp_settings` section to enable the listener.

```json
{
  "reverse_mcp_settings": {
    "enabled": true,
    "listen_host": ":9090",
    "tls": {
      "enabled": false,
      "cert_file": "/path/to/cert.pem",
      "key_file": "/path/to/key.pem"
    }
  }
}
```

*   **enabled**: Set to `true` to start the listener.
*   **listen_host**: Interface and port to listen on (e.g., `0.0.0.0:9090` or just `:9090`).
*   **tls**: Optional TLS configuration for secure `wss://` connections.

### 2. Define Incoming Servers

You must define which servers are expected to connect in the `tools_servers` section. This acts as an allowlist and configuration for tools discovery.

```json
{
  "tools_servers": {
    "my_remote_server": {
      "transport": "reverse_mcp",
      "auth_token": "my-secret-token-123",
      "interface": "none",
      "disabled": false
    }
  }
}
```

*   **Key (e.g. "my_remote_server")**: This is the `server_name` that the remote client MUST use when connecting.
*   **transport**: Must be set to `"reverse_mcp"`.
*   **auth_token**: The secret token that the remote server must present.
*   **interface**: Standard tool interface setting (e.g., "memory", "rag", or "none").

## connecting a Remote MCP Server

The remote server needs to assume the role of a WebSocket client. It should connect to:

`ws://<cleverchatty-host>:<port>/ws?server_name=<name>&token=<token>`

Or provide headers:
*   `X-MCP-Server-Name`: `<name>`
*   `Authorization`: `Bearer <token>`

### Using the Dev Tool

We provide a helper tool/library in `dev_tools/reverse_mcp_server` that wraps a standard MCP Stdio server and connects it to the Reverse Connector.

```bash
# Build the tool
cd dev_tools/reverse_mcp_server
go build

# Run it
./reverse_mcp_server \
  -host localhost:9090 \
  -name my_remote_server \
  -token my-secret-token-123
```

## Security Recommendation

For production use, it is highly recommended to:
1.  Enable **TLS** in `reverse_mcp_settings`.
2.  Use strong, unique **auth_token**s for each server.
3.  Do not expose the listener port to the public internet without proper firewall rules or VPN.
