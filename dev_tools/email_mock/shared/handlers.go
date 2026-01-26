package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

var (
	verboseLogging = false
)

// SetVerbose enables or disables verbose logging
func SetVerbose(verbose bool) {
	verboseLogging = verbose
}

// logDebug logs a message only if verbose logging is enabled
func logDebug(format string, args ...interface{}) {
	if verboseLogging {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// CreateGetEmailsHandler creates the get_emails tool handler
func CreateGetEmailsHandler(emailManager *EmailManager) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			args = make(map[string]interface{})
		}

		// Optional filter for unread only
		unreadOnly := false
		if val, ok := args["unread_only"].(bool); ok {
			unreadOnly = val
		}

		// Get emails
		emails := emailManager.GetEmails()

		// Filter if needed
		if unreadOnly {
			unreadEmails := make([]*Email, 0)
			for _, email := range emails {
				if !email.Read {
					unreadEmails = append(unreadEmails, email)
				}
			}
			emails = unreadEmails
		}

		// Return emails as JSON
		result := map[string]interface{}{
			"emails": emails,
			"count":  len(emails),
		}

		jsonResult, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonResult)), nil
	}
}

// CreateMarkEmailReadHandler creates the mark_email_read tool handler
func CreateMarkEmailReadHandler(emailManager *EmailManager) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments"), nil
		}

		emailID, ok := args["email_id"].(string)
		if !ok || emailID == "" {
			return mcp.NewToolResultError("email_id is required"), nil
		}

		// Mark email as read
		success := emailManager.MarkAsRead(emailID)
		if !success {
			return mcp.NewToolResultError(fmt.Sprintf("email not found: %s", emailID)), nil
		}

		result := map[string]interface{}{
			"email_id": emailID,
			"status":   "marked as read",
		}

		jsonResult, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonResult)), nil
	}
}

// CreateMCPTools creates the MCP tool definitions
func CreateMCPTools() (mcp.Tool, mcp.Tool) {
	getEmailsTool := mcp.NewTool("get_emails",
		mcp.WithDescription("Get all emails from the inbox"),
		mcp.WithBoolean("unread_only",
			mcp.Description("If true, only return unread emails"),
		),
	)

	markEmailReadTool := mcp.NewTool("mark_email_read",
		mcp.WithDescription("Mark an email as read"),
		mcp.WithString("email_id",
			mcp.Required(),
			mcp.Description("The ID of the email to mark as read"),
		),
	)

	return getEmailsTool, markEmailReadTool
}

// StartEmailNotificationSender starts a background goroutine that sends email notifications
// to all connected clients
func StartEmailNotificationSender(ctx context.Context, mcpServer interface{}, emailManager *EmailManager) {
	// Interface for server that supports broadcasting to all clients
	type notificationBroadcaster interface {
		SendNotificationToAllClients(method string, params map[string]any)
	}

	broadcaster, ok := mcpServer.(notificationBroadcaster)
	if !ok {
		logDebug("[Email Sender] ❌ Server does not support SendNotificationToAllClients\n")
		return
	}

	go func() {
		logDebug("[Email Sender] 🚀 Started email notification sender (broadcast mode)\n")
		hasClients := false

		for {
			select {
			case <-ctx.Done():
				logDebug("[Email Sender] 🛑 Stopping email notification sender\n")
				return
			default:
				// Wait for random delay
				delay := GetRandomDelay()
				logDebug("[Email Sender] ⏰ Waiting %v before sending next email...\n", delay)
				time.Sleep(delay)

				// Generate random email
				email := GenerateRandomEmail()
				emailManager.AddEmail(email)

				logDebug("[Email Sender] 📧 Generated new email: %s - %s\n",
					email.From, email.Subject)

				// Broadcast notification to all connected clients
				// Note: This method doesn't return an error, it silently skips if no clients
				broadcaster.SendNotificationToAllClients("new_email", map[string]any{
					"email_id": email.ID,
					"from":     email.From,
					"subject":  email.Subject,
					"sent_at":  email.SentAt.Format(time.RFC3339),
				})

				// Assume success - the broadcast method handles connected clients internally
				if !hasClients {
					logDebug("[Email Sender] ✓ Broadcasting notifications (sent to all connected clients)\n")
					hasClients = true
				}
				logDebug("[Email Sender] ✓ NOTIFICATION SENT to all clients: new_email (from: %s)\n",
					email.From)
			}
		}
	}()
}
