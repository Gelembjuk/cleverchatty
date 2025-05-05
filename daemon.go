package cleverchatty

/**
 * In this file we define the daemon for the cleverchatty application.
 * Goroutine is created and listens the channel with messages.
 * Also it listens notifications from MCP servers.
 * It can be stopped by context cancellation.
 */

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

type notificationMessage struct {
	serverName   string
	notification mcp.JSONRPCNotification
}

func Daemonize(ctx context.Context, assistant *CleverChatty) chan string {
	// Create a channel to receive messages
	messageChannel := make(chan string)
	notificationChannel := make(chan notificationMessage)

	assistant.mcpHost.SetNotificationCallback(func(serverName string, notification mcp.JSONRPCNotification) {
		// Send the notification to the notification channel
		assistant.logger.Printf("Notification received from server %s: %s", serverName, notification)
		notificationChannel <- notificationMessage{
			serverName:   serverName,
			notification: notification,
		}
	})

	// Start a goroutine to listen for messages
	go func() {
		for {
			select {
			case msg := <-messageChannel:
				// Process the message
				assistant.Prompt(msg)
			case notification := <-notificationChannel:
				// Process the notification
				assistant.serverNotification(notification.serverName, notification.notification)
			// Check if the context is done
			case <-ctx.Done():
				// Context is done, exit the goroutine
				return
			}
		}
	}()

	return messageChannel
}
