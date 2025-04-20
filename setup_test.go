package cleverchatty

import (
	"context"
	"testing"

	"github.com/gelembjuk/cleverchatty/test"
)

func TestObjectCreate(t *testing.T) {
	cleverChattyObj, err := GetCleverChatty(CleverChattyConfig{
		Model:      "mock:mock",
		MCPServers: map[string]ServerConfigWrapper{},
	}, nil)

	if err != nil {
		t.Fatalf("Failed to create CleverChatty object: %v", err)
	}
	if cleverChattyObj == nil {
		t.Fatal("CleverChatty object is nil")
	}
	if cleverChattyObj.provider == nil {
		t.Fatal("Provider is nil")
	}
	if cleverChattyObj.mcpHost == nil {
		t.Fatal("MCPHost is nil")
	}
	if cleverChattyObj.logger == nil {
		t.Fatal("Logger is nil")
	}
}

func TestObjectWithOneServerCreate(t *testing.T) {
	cleverChattyObj, err := GetCleverChatty(CleverChattyConfig{
		Model: "mock:mock",
		MCPServers: map[string]ServerConfigWrapper{
			"test": {
				Config: InternalServerConfig{
					Kind: "mock",
				},
			},
		},
	}, context.Background())

	if err != nil {
		t.Fatalf("Failed to create CleverChatty object: %v", err)
	}
	if cleverChattyObj == nil {
		t.Fatal("CleverChatty object is nil")
	}
	if cleverChattyObj.provider == nil {
		t.Fatal("Provider is nil")
	}
	if cleverChattyObj.mcpHost == nil {
		t.Fatal("MCPHost is nil")
	}
	if cleverChattyObj.logger == nil {
		t.Fatal("Logger is nil")
	}
	if len(cleverChattyObj.mcpHost.config) != 1 {
		t.Fatalf("Expected 1 server, got %d", len(cleverChattyObj.mcpHost.config))
	}
	if _, ok := cleverChattyObj.mcpHost.config["test"]; !ok {
		t.Fatalf("Expected server 'test' not found in config")
	}
	if _, ok := cleverChattyObj.mcpHost.clients["test"]; !ok {
		t.Fatalf("Expected client for server 'test' not found in clients")
	}
	if _, ok := cleverChattyObj.mcpHost.clients["test"].(*test.MockMCPClient); !ok {
		t.Fatalf("Expected client for server 'test' to be of type MockMCPClient")
	}
	if len(cleverChattyObj.mcpHost.clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(cleverChattyObj.mcpHost.clients))
	}
	if cleverChattyObj.mcpHost.tools == nil {
		t.Fatal("Tools are nil")
	}
	if len(cleverChattyObj.mcpHost.tools) != 1 {
		t.Fatalf("Expected 1 tools, got %d", len(cleverChattyObj.mcpHost.tools))
	}
	if cleverChattyObj.mcpHost.tools[0].Name != "test__tool1" {
		t.Fatalf("Expected tool name 'tool1', got '%s'", cleverChattyObj.mcpHost.tools[0].Name)
	}
}
