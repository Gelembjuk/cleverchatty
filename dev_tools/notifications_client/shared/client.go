package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TaskStatusResponse represents the response from task_status tool
type TaskStatusResponse struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Progress    int        `json:"progress"`
	Completed   bool       `json:"completed"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

var (
	// Task categories and titles for variety
	categories = []string{"processing", "analysis", "computation", "sync", "import", "export"}

	titleTemplates = []string{
		"Process user data batch",
		"Analyze system metrics",
		"Generate monthly report",
		"Sync database records",
		"Import configuration files",
		"Export analytics data",
		"Compile statistics",
		"Validate data integrity",
		"Optimize cache entries",
		"Backup critical files",
	}
)

// RunTaskLoop runs the main task execution loop
func RunTaskLoop(ctx context.Context, client mcpclient.MCPClient) {
	fmt.Println("Connected to task notification server")
	fmt.Println("Starting task execution loop (Ctrl+C to stop)...")
	fmt.Println()

	taskNum := 1

	// Main loop: start tasks and poll their status
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down gracefully...")
			return
		default:
			// Start a new task
			title := titleTemplates[rand.Intn(len(titleTemplates))]
			category := categories[rand.Intn(len(categories))]

			fmt.Printf("=== Task #%d ===\n", taskNum)
			fmt.Printf("Starting: %s (category: %s)\n", title, category)

			taskID, err := StartTask(ctx, client, title, category)
			if err != nil {
				fmt.Printf("Error starting task: %v\n", err)
				time.Sleep(2 * time.Second)
				continue
			}

			fmt.Printf("Task ID: %s\n", taskID)

			// Poll task status until completion
			if err := PollTaskStatus(ctx, client, taskID); err != nil {
				if ctx.Err() != nil {
					return
				}
				fmt.Printf("Error polling task: %v\n", err)
			}

			fmt.Println()
			taskNum++

			// Small delay between tasks
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
}

// StartTask starts a new task and returns its ID
func StartTask(ctx context.Context, client mcpclient.MCPClient, title, category string) (string, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = "start_task"
	req.Params.Arguments = map[string]interface{}{
		"title":    title,
		"category": category,
	}

	result, err := client.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to call start_task: %w", err)
	}

	// Parse the result
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from server")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected response type")
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(textContent.Text), &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	taskID, ok := response["task_id"].(string)
	if !ok {
		return "", fmt.Errorf("task_id not found in response")
	}

	return taskID, nil
}

// PollTaskStatus polls task status until completion
func PollTaskStatus(ctx context.Context, client mcpclient.MCPClient, taskID string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := GetTaskStatus(ctx, client, taskID)
			if err != nil {
				return err
			}

			// Show progress obtained via polling (request/response)
			fmt.Printf("  [POLLED] Progress: %d%%", status.Progress)
			if status.Completed {
				fmt.Printf(" - COMPLETED\n")
				duration := time.Since(status.StartedAt)
				fmt.Printf("  Duration: %.1f seconds\n", duration.Seconds())
				return nil
			}
			fmt.Println()
		}
	}
}

// GetTaskStatus retrieves the current status of a task
func GetTaskStatus(ctx context.Context, client mcpclient.MCPClient, taskID string) (*TaskStatusResponse, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = "task_status"
	req.Params.Arguments = map[string]interface{}{
		"task_id": taskID,
	}

	result, err := client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call task_status: %w", err)
	}

	// Parse the result
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from server")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}

	var status TaskStatusResponse
	if err := json.Unmarshal([]byte(textContent.Text), &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return &status, nil
}
