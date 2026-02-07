package core

import (
	"context"
	"testing"
)

func TestNewGetNotificationTypesTool(t *testing.T) {
	mockHandler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "types list", nil
	}

	tool, err := NewGetNotificationTypesTool(mockHandler)
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	if tool.Name != "get_notification_types" {
		t.Errorf("Expected tool name 'get_notification_types', got '%s'", tool.Name)
	}

	if len(tool.Arguments) != 0 {
		t.Errorf("Expected 0 arguments, got %d", len(tool.Arguments))
	}
}

func TestNewGetNotificationTypesTool_NilHandler(t *testing.T) {
	_, err := NewGetNotificationTypesTool(nil)
	if err == nil {
		t.Error("Expected error when handler is nil, got nil")
	}
}

func TestNewGetNotificationTypesTool_Handler(t *testing.T) {
	mockHandler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "Available notification types:\n- Key: server::method, Kind: broadcast", nil
	}

	tool, err := NewGetNotificationTypesTool(mockHandler)
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	result, err := tool.Handler(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Errorf("Handler failed: %v", err)
	}
	if result != "Available notification types:\n- Key: server::method, Kind: broadcast" {
		t.Errorf("Unexpected result: '%s'", result)
	}
}

func TestNewSubscribeNotificationsTool(t *testing.T) {
	mockHandler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		return "subscribed", nil
	}

	tool, err := NewSubscribeNotificationsTool(mockHandler)
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	if tool.Name != "subscribe_notifications" {
		t.Errorf("Expected tool name 'subscribe_notifications', got '%s'", tool.Name)
	}

	if len(tool.Arguments) != 2 {
		t.Fatalf("Expected 2 arguments, got %d", len(tool.Arguments))
	}

	// Verify argument names
	expectedArgs := map[string]bool{
		"notification_key": false,
		"instructions":     false,
	}
	for _, arg := range tool.Arguments {
		if _, ok := expectedArgs[arg.Name]; !ok {
			t.Errorf("Unexpected argument name: %s", arg.Name)
		}
		expectedArgs[arg.Name] = true
		if !arg.Required {
			t.Errorf("Expected argument %s to be required", arg.Name)
		}
		if arg.Type != "string" {
			t.Errorf("Expected argument %s to have type 'string', got '%s'", arg.Name, arg.Type)
		}
	}
	for name, found := range expectedArgs {
		if !found {
			t.Errorf("Missing expected argument: %s", name)
		}
	}
}

func TestNewSubscribeNotificationsTool_NilHandler(t *testing.T) {
	_, err := NewSubscribeNotificationsTool(nil)
	if err == nil {
		t.Error("Expected error when handler is nil, got nil")
	}
}

func TestNewSubscribeNotificationsTool_Handler(t *testing.T) {
	var capturedArgs map[string]interface{}
	mockHandler := func(ctx context.Context, args map[string]interface{}) (string, error) {
		capturedArgs = args
		return "subscribed", nil
	}

	tool, err := NewSubscribeNotificationsTool(mockHandler)
	if err != nil {
		t.Fatalf("Failed to create tool: %v", err)
	}

	args := map[string]interface{}{
		"notification_key": "email-server::notifications/message",
		"instructions":     "Tell me about new emails",
	}

	result, err := tool.Handler(context.Background(), args)
	if err != nil {
		t.Errorf("Handler failed: %v", err)
	}
	if result != "subscribed" {
		t.Errorf("Expected result 'subscribed', got '%s'", result)
	}
	if capturedArgs["notification_key"] != "email-server::notifications/message" {
		t.Errorf("Expected notification_key 'email-server::notifications/message', got '%v'", capturedArgs["notification_key"])
	}
	if capturedArgs["instructions"] != "Tell me about new emails" {
		t.Errorf("Expected instructions 'Tell me about new emails', got '%v'", capturedArgs["instructions"])
	}
}
