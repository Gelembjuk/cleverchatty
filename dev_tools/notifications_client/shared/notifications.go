package shared

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// HandleNotification processes incoming notifications from the server
func HandleNotification(notification mcp.JSONRPCNotification) {
	method := notification.Method
	params := notification.Params.AdditionalFields

	// Log that we received a notification
	fmt.Printf("[NOTIFICATION RECEIVED: %s]\n", method)

	switch method {
	case "task/started":
		if params != nil {
			taskID, _ := params["task_id"].(string)
			title, _ := params["title"].(string)
			category, _ := params["category"].(string)
			duration, _ := params["duration"].(string)
			fmt.Printf("📢 [PUSH] Task started!\n")
			fmt.Printf("   Task ID: %s\n", taskID)
			fmt.Printf("   Title: %s\n", title)
			fmt.Printf("   Category: %s\n", category)
			fmt.Printf("   Expected Duration: %s\n\n", duration)
		}

	case "notifications/progress":
		if params != nil {
			taskID, _ := params["task_id"].(string)
			title, _ := params["title"].(string)
			progressPct := 0
			if pct, ok := params["progressPercentage"].(float64); ok {
				progressPct = int(pct)
			}

			// Create a simple progress bar
			bar := CreateProgressBar(progressPct, 20)

			// Show progress obtained via server push notification
			shortID := taskID
			if len(taskID) > 8 {
				shortID = taskID[len(taskID)-8:]
			}
			fmt.Printf("📊 [PUSH] [%s] %s: [%s] %d%%\n", shortID, title, bar, progressPct)
		}

	case "task/completed":
		if params != nil {
			taskID, _ := params["task_id"].(string)
			title, _ := params["title"].(string)
			fmt.Printf("✅ [PUSH] Task completed!\n")
			fmt.Printf("   Task ID: %s\n", taskID)
			fmt.Printf("   Title: %s\n\n", title)
		}

	default:
		// Log other notifications for debugging
		fmt.Printf("📨 [PUSH] Unknown notification [%s]: %+v\n", method, params)
	}
}

// CreateProgressBar creates a visual progress bar
func CreateProgressBar(progress, length int) string {
	filled := (progress * length) / 100
	bar := ""
	for i := 0; i < length; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}
