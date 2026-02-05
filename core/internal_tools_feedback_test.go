package core

import (
	"context"
	"testing"
)

func TestNewNotificationFeedbackTool(t *testing.T) {
	mockHandler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		message, ok := args["message"].(string)
		if !ok {
			return "", nil
		}
		return "Handled: " + message, nil
	}

	tool, err := NewNotificationFeedbackTool(mockHandler)
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	if tool.Name != "notification_feedback" {
		t.Errorf("Expected tool name 'notification_feedback', got '%s'", tool.Name)
	}

	if len(tool.Arguments) != 1 {
		t.Fatalf("Expected 1 argument, got %d", len(tool.Arguments))
	}

	if tool.Arguments[0].Name != "message" {
		t.Errorf("Expected argument name 'message', got '%s'", tool.Arguments[0].Name)
	}

	// Test the handler indirectly
	ctx := context.Background()
	args := map[string]interface{}{
		"message": "test",
	}
	result, err := tool.Handler(ctx, args)
	if err != nil {
		t.Errorf("Handler failed: %v", err)
	}
	if result != "Handled: test" {
		t.Errorf("Expected result 'Handled: test', got '%s'", result)
	}
}

func TestNewNotificationFeedbackTool_NilHandler(t *testing.T) {
	_, err := NewNotificationFeedbackTool(nil)
	if err == nil {
		t.Error("Expected error when handler is nil, got nil")
	}
}
