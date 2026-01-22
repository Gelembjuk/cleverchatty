# Task Notification Client/Server

This is a demonstration MCP (Model Context Protocol) tool that showcases **asynchronous task execution with real-time server-to-client notifications** and status polling.

## Key Features

**MCP Notifications**: The server actively pushes notifications to the client during task execution, demonstrating the MCP notification protocol.

**Multiple Transport Support**: Supports both stdio and HTTP streaming transports with shared business logic.

## Architecture

The project uses a modular architecture with shared code:

```
notifications_client/
├── shared/           # Common business logic
│   ├── tasks.go      # TaskManager and TaskStatus
│   ├── handlers.go   # MCP tool handlers
│   └── notifications.go  # Notification handling
├── server/           # Stdio server (legacy location)
├── client/           # Stdio client (legacy location)
├── http-server/      # HTTP streaming server
└── http-client/      # HTTP streaming client
```

## Components

### Server (Both Transports)
An MCP server that provides two tools and sends real-time notifications:

**Tools:**
- `start_task` - Starts a new task that executes asynchronously (2-6 seconds duration)
- `task_status` - Checks the current status of a running task

**Notifications** (sent from server to client):
- `task/started` - Sent when a task begins execution with task details
- `notifications/progress` - Sent every second with progress updates (includes progress bar data)
- `task/completed` - Sent when a task finishes

The server simulates task execution in goroutines, updating progress and sending notifications every second.

### Client (Both Transports)
An MCP client that:
- Connects to the MCP server (via stdio or HTTP streaming)
- Listens for server notifications in real-time
- Displays visual progress bars from notification data
- Continuously starts new tasks
- Polls task status every 1 second until completion
- Starts the next task automatically
- Gracefully handles Ctrl+C for shutdown

## Building

### Build Stdio Version (Original)
```bash
# Server
cd dev_tools/notifications_client/server
go build -o server

# Client
cd dev_tools/notifications_client/client
go build -o client
```

### Build HTTP Streaming Version
```bash
# Server
cd dev_tools/notifications_client/http-server
go build -o http-server

# Client
cd dev_tools/notifications_client/http-client
go build -o http-client
```

## Running

### Option 1: Stdio Transport (Original)

**Terminal 1 - Start the Server**
```bash
cd dev_tools/notifications_client/server
./server
```

The server will output task execution progress to stderr.

**Terminal 2 - Start the Client**
```bash
cd dev_tools/notifications_client/client
./client
```

### Option 2: HTTP Streaming Transport

**Terminal 1 - Start the HTTP Server**
```bash
cd dev_tools/notifications_client/http-server
./http-server
# Server starts on http://localhost:8080/mcp by default
# Use --host and --port flags to customize
```

**Terminal 2 - Start the HTTP Client**
```bash
cd dev_tools/notifications_client/http-client
./http-client
# Use --host and --port flags to match server address
```

### How the Client Works

The client will:
1. Connect to the server (stdio or HTTP)
2. Start a new task with a random title and category
3. Poll the task status every second
4. Display progress updates (both push notifications and polled status)
5. Start the next task when the current one completes
6. Continue indefinitely until you press Ctrl+C

## Example Output

### Server Output
```
[Task task_1234567890] Started: Process user data batch (category: processing, duration: 4s)
[Task task_1234567890] Progress: 0%
[Task task_1234567890] Progress: 25%
[Task task_1234567890] Progress: 50%
[Task task_1234567890] Progress: 75%
[Task task_1234567890] Progress: 100%
[Task task_1234567890] Completed!
```

### Client Output (with Real-Time Notifications)
```
Connected to task notification server
Starting task execution loop (Ctrl+C to stop)...

=== Task #1 ===
Starting: Process user data batch (category: processing)
Task ID: task_1234567890
[NOTIFICATION RECEIVED: task/started]
📢 [PUSH] Task started!
   Task ID: task_1234567890
   Title: Process user data batch
   Category: processing
   Expected Duration: 4s

[NOTIFICATION RECEIVED: notifications/progress]
📊 [PUSH] [67890] Process user data batch: [░░░░░░░░░░░░░░░░░░░░] 0%
[NOTIFICATION RECEIVED: notifications/progress]
📊 [PUSH] [67890] Process user data batch: [█████░░░░░░░░░░░░░░░] 25%
  [POLLED] Progress: 25%
[NOTIFICATION RECEIVED: notifications/progress]
📊 [PUSH] [67890] Process user data batch: [██████████░░░░░░░░░░] 50%
  [POLLED] Progress: 50%
[NOTIFICATION RECEIVED: notifications/progress]
📊 [PUSH] [67890] Process user data batch: [███████████████░░░░░] 75%
  [POLLED] Progress: 75%
[NOTIFICATION RECEIVED: notifications/progress]
📊 [PUSH] [67890] Process user data batch: [████████████████████] 100%
[NOTIFICATION RECEIVED: task/completed]
✅ [PUSH] Task completed!
   Task ID: task_1234567890
   Title: Process user data batch

  [POLLED] Progress: 100% - COMPLETED
  Duration: 4.0 seconds

=== Task #2 ===
Starting: Analyze system metrics (category: analysis)
...
```

Notice the **two parallel communication channels**:

1. **Server Push Notifications** (marked with `[PUSH]`):
   - `[NOTIFICATION RECEIVED: ...]` - Confirmation log when notification arrives
   - Start notification with task details when execution begins
   - Progress notifications with visual progress bars in real-time (📊)
   - Completion notification when the task finishes (✅)
   - All via MCP's notification protocol (fire-and-forget, no polling needed)

2. **Client Polling** (marked with `[POLLED]`):
   - Uses `task_status` tool every 1 second
   - Traditional request/response pattern
   - Shows percentage without visual bar
   - Runs independently alongside notifications

Both mechanisms track the same task progress but demonstrate different MCP communication patterns!

## Architecture

- **Server**: Uses `sync.RWMutex` to safely manage shared task state across goroutines
- **Client**: Uses stdio transport to communicate with the server process
- **Task Execution**: Each task runs in its own goroutine with randomized duration (2-6 seconds)
- **Status Updates**: Progress is updated every 1 second during task execution
- **MCP Notifications**: Server pushes real-time notifications to client using `SendNotificationToClient()`
- **Dual Communication**: Combines request/response (tool calls) with push notifications (server-initiated)
- **Graceful Shutdown**: Both client and server handle Ctrl+C gracefully

## Use Cases

This tool demonstrates:
- **MCP Notification Protocol**: Server-to-client push notifications during long-running operations
- **MCP server/client communication patterns**: Both request/response and push-based
- **Asynchronous task execution**: Non-blocking task execution with status tracking
- **Thread-safe state management**: Safe concurrent access to shared task state
- **Visual Progress Feedback**: Real-time progress bars via notifications
- **Graceful shutdown handling**: Proper cleanup on interrupt signals
- **Real-world notification/polling patterns**: Hybrid approach combining both methods

## Transport Comparison

### Stdio Transport
- **Pros:**
  - Simple, single process model - client launches server automatically
  - Reliable notification delivery
  - Lower latency
  - Perfect for local tools and development

- **Cons:**
  - Single client per server
  - No network access
  - Client and server lifecycle tightly coupled

### HTTP Streaming Transport
- **Pros:**
  - Multiple clients can connect to one server
  - Network accessible - can be remote
  - Server runs independently
  - Better for production/service scenarios

- **Cons:**
  - Slightly higher latency
  - More complex connection management
  - Session handling required for notifications

**Note:** The HTTP streaming version demonstrates the transport but may have notification delivery timing differences compared to stdio due to HTTP connection lifecycle.

## Technical Details

### How Notifications Work

1. **Server Side** (`server/main.go:219`):
   - Uses `server.ServerFromContext(ctx)` to get the MCP server instance
   - Calls `mcpServer.SendNotificationToClient()` with custom notification methods
   - Sends three notification types: `task/started`, `notifications/progress`, `task/completed`

2. **Client Side** (`client/main.go:68`):
   - Registers notification handler with `client.OnNotification()`
   - Parses notification method and params
   - Displays formatted output with progress bars and status updates

3. **Notification Data**:
   - Sent as JSON-RPC notifications via the MCP protocol
   - Includes custom fields in `params.AdditionalFields`
   - Does not block or wait for client response (fire-and-forget)

### Distinguishing Push vs Pull

The client output clearly shows both communication patterns:

**Push (Server → Client Notifications):**
- Prefix: `[NOTIFICATION RECEIVED: ...]` and `[PUSH]`
- Visual: Progress bars with emoji (📊, 📢, ✅)
- Protocol: MCP notifications via `SendNotificationToClient()`
- No client request needed - server initiates

**Pull (Client Polling):**
- Prefix: `[POLLED]`
- Visual: Simple percentage text
- Protocol: Tool calls to `task_status` every 1 second
- Client initiates request/response

This dual approach demonstrates:
- MCP's flexibility in supporting both patterns
- Real-time updates without constant polling overhead
- How notifications complement traditional request/response
