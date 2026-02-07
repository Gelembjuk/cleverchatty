package core

import (
	"strings"
	"testing"
)

func TestNewNotificationTypeRegistry(t *testing.T) {
	registry := NewNotificationTypeRegistry()
	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}
	if len(registry.GetAll()) != 0 {
		t.Errorf("Expected empty registry, got %d types", len(registry.GetAll()))
	}
}

func TestNotificationTypeRegistryRegister(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	nt := NotificationType{
		Name:        "notifications/message",
		Description: "New message received",
		Kind:        NotificationKindBroadcast,
		ServerName:  "email-server",
	}

	registry.Register(nt)

	all := registry.GetAll()
	if len(all) != 1 {
		t.Fatalf("Expected 1 type, got %d", len(all))
	}
	if all[0].Name != "notifications/message" {
		t.Errorf("Expected name 'notifications/message', got '%s'", all[0].Name)
	}
	if all[0].Kind != NotificationKindBroadcast {
		t.Errorf("Expected kind 'broadcast', got '%s'", all[0].Kind)
	}
}

func TestNotificationTypeRegistryDeduplication(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	nt1 := NotificationType{
		Name:        "notifications/message",
		Description: "First registration",
		Kind:        NotificationKindBroadcast,
		ServerName:  "email-server",
	}
	nt2 := NotificationType{
		Name:        "notifications/message",
		Description: "Duplicate registration",
		Kind:        NotificationKindTriggered,
		ServerName:  "email-server",
	}

	registry.Register(nt1)
	registry.Register(nt2)

	all := registry.GetAll()
	if len(all) != 1 {
		t.Fatalf("Expected 1 type after dedup, got %d", len(all))
	}
	// First registration should win
	if all[0].Description != "First registration" {
		t.Errorf("Expected first registration to be kept, got description '%s'", all[0].Description)
	}
}

func TestNotificationTypeRegistryDifferentServers(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	registry.Register(NotificationType{
		Name:       "notifications/message",
		ServerName: "server-a",
		Kind:       NotificationKindBroadcast,
	})
	registry.Register(NotificationType{
		Name:       "notifications/message",
		ServerName: "server-b",
		Kind:       NotificationKindTriggered,
	})

	all := registry.GetAll()
	if len(all) != 2 {
		t.Fatalf("Expected 2 types (different servers), got %d", len(all))
	}
}

func TestNotificationTypeRegistryGetByServer(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	registry.Register(NotificationType{
		Name:       "notifications/message",
		ServerName: "server-a",
	})
	registry.Register(NotificationType{
		Name:       "notifications/progress",
		ServerName: "server-a",
	})
	registry.Register(NotificationType{
		Name:       "notifications/alert",
		ServerName: "server-b",
	})

	serverATypes := registry.GetByServer("server-a")
	if len(serverATypes) != 2 {
		t.Fatalf("Expected 2 types for server-a, got %d", len(serverATypes))
	}

	serverBTypes := registry.GetByServer("server-b")
	if len(serverBTypes) != 1 {
		t.Fatalf("Expected 1 type for server-b, got %d", len(serverBTypes))
	}

	emptyTypes := registry.GetByServer("nonexistent")
	if len(emptyTypes) != 0 {
		t.Fatalf("Expected 0 types for nonexistent server, got %d", len(emptyTypes))
	}
}

func TestNotificationTypeRegistryGetNotificationType(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	registry.Register(NotificationType{
		Name:        "notifications/message",
		Description: "A message notification",
		ServerName:  "email-server",
		Kind:        NotificationKindBroadcast,
	})

	// Found
	nt := registry.GetNotificationType("email-server", "notifications/message")
	if nt == nil {
		t.Fatal("Expected to find notification type")
	}
	if nt.Description != "A message notification" {
		t.Errorf("Expected description 'A message notification', got '%s'", nt.Description)
	}

	// Not found - wrong server
	nt = registry.GetNotificationType("other-server", "notifications/message")
	if nt != nil {
		t.Error("Expected nil for wrong server")
	}

	// Not found - wrong name
	nt = registry.GetNotificationType("email-server", "notifications/other")
	if nt != nil {
		t.Error("Expected nil for wrong name")
	}
}

func TestNotificationTypeRegistryFormatForToolDescription(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	// Empty registry
	desc := registry.FormatForToolDescription()
	if !strings.Contains(desc, "No notification types") {
		t.Errorf("Expected empty registry message, got '%s'", desc)
	}

	// With entries
	registry.Register(NotificationType{
		Name:        "notifications/message",
		Description: "New email received",
		Kind:        NotificationKindBroadcast,
		ServerName:  "email-server",
	})
	registry.Register(NotificationType{
		Name:        "notifications/status",
		Description: "Task status changed",
		Kind:        NotificationKindTriggered,
		ServerName:  "task-server",
	})

	desc = registry.FormatForToolDescription()
	if !strings.Contains(desc, "Available notification types:") {
		t.Errorf("Expected header, got '%s'", desc)
	}
	// Check for composite key format
	if !strings.Contains(desc, "Key: email-server::notifications/message") {
		t.Errorf("Expected composite key for email-server, got '%s'", desc)
	}
	if !strings.Contains(desc, "Key: task-server::notifications/status") {
		t.Errorf("Expected composite key for task-server, got '%s'", desc)
	}
	if !strings.Contains(desc, "broadcast") {
		t.Errorf("Expected broadcast in description, got '%s'", desc)
	}
	if !strings.Contains(desc, "New email received") {
		t.Errorf("Expected description text in output, got '%s'", desc)
	}
}

func TestNotificationKindConstants(t *testing.T) {
	if NotificationKindBroadcast != "broadcast" {
		t.Errorf("Expected NotificationKindBroadcast to be 'broadcast', got '%s'", NotificationKindBroadcast)
	}
	if NotificationKindTriggered != "triggered" {
		t.Errorf("Expected NotificationKindTriggered to be 'triggered', got '%s'", NotificationKindTriggered)
	}
}

func TestMakeNotificationKey(t *testing.T) {
	key := MakeNotificationKey("email-server", "notifications/message")
	if key != "email-server::notifications/message" {
		t.Errorf("Expected 'email-server::notifications/message', got '%s'", key)
	}

	key = MakeNotificationKey("server", "method")
	if key != "server::method" {
		t.Errorf("Expected 'server::method', got '%s'", key)
	}

	// Server name with underscores
	key = MakeNotificationKey("emailbox_http_server", "new_email")
	if key != "emailbox_http_server::new_email" {
		t.Errorf("Expected 'emailbox_http_server::new_email', got '%s'", key)
	}
}

func TestParseNotificationKey(t *testing.T) {
	server, name, err := ParseNotificationKey("email-server::notifications/message")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if server != "email-server" {
		t.Errorf("Expected server 'email-server', got '%s'", server)
	}
	if name != "notifications/message" {
		t.Errorf("Expected name 'notifications/message', got '%s'", name)
	}

	// Server name with underscores - should parse correctly
	server, name, err = ParseNotificationKey("emailbox_http_server::new_email")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if server != "emailbox_http_server" {
		t.Errorf("Expected server 'emailbox_http_server', got '%s'", server)
	}
	if name != "new_email" {
		t.Errorf("Expected name 'new_email', got '%s'", name)
	}

	// Invalid key - no separator
	_, _, err = ParseNotificationKey("noseparator")
	if err == nil {
		t.Error("Expected error for key without separator")
	}
}

func TestNotificationTypeRegistrySubscribe(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	// Register a type first
	registry.Register(NotificationType{
		Name:       "notifications/message",
		ServerName: "email-server",
		Kind:       NotificationKindBroadcast,
	})

	key := MakeNotificationKey("email-server", "notifications/message")

	// Subscribe
	err := registry.Subscribe(key, []string{"Tell me about new emails"})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	sub := registry.GetSubscription(key)
	if sub == nil {
		t.Fatal("Expected subscription, got nil")
	}
	if sub.Key != key {
		t.Errorf("Expected key '%s', got '%s'", key, sub.Key)
	}
	if len(sub.Instructions) != 1 {
		t.Fatalf("Expected 1 instruction, got %d", len(sub.Instructions))
	}
	if sub.Instructions[0] != "Tell me about new emails" {
		t.Errorf("Expected instruction 'Tell me about new emails', got '%s'", sub.Instructions[0])
	}

	// Append more instructions
	err = registry.Subscribe(key, []string{"Also summarize the sender"})
	if err != nil {
		t.Fatalf("Subscribe append failed: %v", err)
	}

	sub = registry.GetSubscription(key)
	if len(sub.Instructions) != 2 {
		t.Fatalf("Expected 2 instructions after append, got %d", len(sub.Instructions))
	}
}

func TestNotificationTypeRegistrySubscribeUnknownType(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	err := registry.Subscribe("unknown::notifications/test", []string{"test"})
	if err == nil {
		t.Error("Expected error subscribing to unregistered type")
	}
}

func TestNotificationTypeRegistrySubscribeInvalidKey(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	err := registry.Subscribe("invalidkey", []string{"test"})
	if err == nil {
		t.Error("Expected error for invalid key format")
	}
}

func TestNotificationTypeRegistryGetSubscriptionByServerAndMethod(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	registry.Register(NotificationType{
		Name:       "notifications/message",
		ServerName: "email-server",
		Kind:       NotificationKindBroadcast,
	})

	key := MakeNotificationKey("email-server", "notifications/message")
	err := registry.Subscribe(key, []string{"Tell me about emails"})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	sub := registry.GetSubscriptionByServerAndMethod("email-server", "notifications/message")
	if sub == nil {
		t.Fatal("Expected subscription, got nil")
	}
	if len(sub.Instructions) != 1 {
		t.Errorf("Expected 1 instruction, got %d", len(sub.Instructions))
	}

	// Not found
	sub = registry.GetSubscriptionByServerAndMethod("other-server", "notifications/message")
	if sub != nil {
		t.Error("Expected nil for unsubscribed server")
	}
}

func TestNotificationTypeRegistryGetAllSubscriptions(t *testing.T) {
	registry := NewNotificationTypeRegistry()

	registry.Register(NotificationType{
		Name:       "notifications/message",
		ServerName: "email-server",
		Kind:       NotificationKindBroadcast,
	})
	registry.Register(NotificationType{
		Name:       "notifications/alert",
		ServerName: "alert-server",
		Kind:       NotificationKindBroadcast,
	})

	key1 := MakeNotificationKey("email-server", "notifications/message")
	key2 := MakeNotificationKey("alert-server", "notifications/alert")

	_ = registry.Subscribe(key1, []string{"instruction 1"})
	_ = registry.Subscribe(key2, []string{"instruction 2"})

	all := registry.GetAllSubscriptions()
	if len(all) != 2 {
		t.Fatalf("Expected 2 subscriptions, got %d", len(all))
	}
	if all[key1] == nil {
		t.Error("Expected subscription for key1")
	}
	if all[key2] == nil {
		t.Error("Expected subscription for key2")
	}
}
