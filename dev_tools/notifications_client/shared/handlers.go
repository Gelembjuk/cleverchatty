package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// CreateStartTaskHandler creates the start_task tool handler
func CreateStartTaskHandler(taskManager *TaskManager) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments"), nil
		}

		title, ok := args["title"].(string)
		if !ok || title == "" {
			return mcp.NewToolResultError("title is required"), nil
		}

		category := ""
		if cat, ok := args["category"].(string); ok {
			category = cat
		}

		// Generate unique task ID
		taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

		// Create task
		taskManager.CreateTask(taskID, title)

		// Get server instance from context
		mcpServer := server.ServerFromContext(ctx)

		// Start task execution in goroutine with server and context for notifications
		go ExecuteTask(ctx, mcpServer, taskManager, taskID, title, category)

		// Return task ID
		result := map[string]interface{}{
			"task_id":  taskID,
			"title":    title,
			"category": category,
			"status":   "started",
		}

		jsonResult, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonResult)), nil
	}
}

// CreateTaskStatusHandler creates the task_status tool handler
func CreateTaskStatusHandler(taskManager *TaskManager) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments"), nil
		}

		taskID, ok := args["task_id"].(string)
		if !ok || taskID == "" {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		// Get task status
		task, exists := taskManager.GetTask(taskID)
		if !exists {
			return mcp.NewToolResultError(fmt.Sprintf("task not found: %s", taskID)), nil
		}

		// Return status
		jsonResult, _ := json.Marshal(task)
		return mcp.NewToolResultText(string(jsonResult)), nil
	}
}

// ExecuteTask simulates task execution with progress updates and notifications
func ExecuteTask(ctx context.Context, mcpServer *server.MCPServer, taskManager *TaskManager, taskID, title, category string) {
	// Random duration between 2-6 seconds
	totalDuration := time.Duration(2+rand.Intn(5)) * time.Second
	steps := int(totalDuration.Seconds())

	fmt.Fprintf(os.Stderr, "[Task %s] Started: %s (category: %s, duration: %v)\n",
		taskID, title, category, totalDuration)

	// Send start notification
	if mcpServer != nil {
		err := mcpServer.SendNotificationToClient(ctx, "task/started", map[string]any{
			"task_id":  taskID,
			"title":    title,
			"category": category,
			"duration": totalDuration.String(),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[Task %s] ❌ Failed to send start notification: %v\n", taskID, err)
		} else {
			fmt.Fprintf(os.Stderr, "[Task %s] ✓ NOTIFICATION SENT: task/started\n", taskID)
		}
	}

	// Update progress every second
	for i := 0; i <= steps; i++ {
		progress := (i * 100) / steps
		if progress > 100 {
			progress = 100
		}

		taskManager.UpdateProgress(taskID, progress)

		fmt.Fprintf(os.Stderr, "[Task %s] Progress: %d%%\n", taskID, progress)

		// Send progress notification
		if mcpServer != nil {
			progressFloat := float64(progress)
			totalFloat := float64(100)
			message := fmt.Sprintf("Task '%s' is %d%% complete", title, progress)

			// Send progress notification using MCP standard notification
			progressNotif := mcp.NewProgressNotification(
				mcp.ProgressToken(taskID),
				progressFloat,
				&totalFloat,
				&message,
			)

			// Send as custom notification with progress data
			err := mcpServer.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
				"progressToken":      taskID,
				"progress":           progressFloat,
				"total":              totalFloat,
				"progressPercentage": progress,
				"task_id":            taskID,
				"title":              title,
				"category":           category,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "[Task %s] ❌ Failed to send progress notification (%d%%): %v\n", taskID, progress, err)
			} else {
				fmt.Fprintf(os.Stderr, "[Task %s] ✓ NOTIFICATION SENT: notifications/progress (%d%%)\n", taskID, progress)
			}

			// Also send as logging message for demonstration
			logNotif := mcp.NewLoggingMessageNotification(
				mcp.LoggingLevelInfo,
				"task_executor",
				map[string]any{
					"task_id":  taskID,
					"progress": progress,
					"message":  message,
				},
			)
			_ = progressNotif // Keep for reference
			_ = logNotif      // Keep for reference
		}

		if i < steps {
			time.Sleep(1 * time.Second)
		}
	}

	// Ensure task is marked as completed
	taskManager.UpdateProgress(taskID, 100)
	fmt.Fprintf(os.Stderr, "[Task %s] Completed!\n", taskID)

	// Send completion notification
	if mcpServer != nil {
		err := mcpServer.SendNotificationToClient(ctx, "task/completed", map[string]any{
			"task_id":  taskID,
			"title":    title,
			"category": category,
			"progress": 100,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[Task %s] ❌ Failed to send completion notification: %v\n", taskID, err)
		} else {
			fmt.Fprintf(os.Stderr, "[Task %s] ✓ NOTIFICATION SENT: task/completed\n", taskID)
		}
	}
}

// CreateMCPTools creates the MCP tool definitions
func CreateMCPTools() (mcp.Tool, mcp.Tool) {
	startTaskTool := mcp.NewTool("start_task",
		mcp.WithDescription("Start a new task that will execute asynchronously"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Title or description of the task"),
		),
		mcp.WithString("category",
			mcp.Description("Optional category for task tracking"),
		),
	)

	taskStatusTool := mcp.NewTool("task_status",
		mcp.WithDescription("Check the current status of a task by its ID"),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("The ID of the task to check"),
		),
	)

	return startTaskTool, taskStatusTool
}
