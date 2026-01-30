package core

import (
	"context"
	"testing"

	"github.com/gelembjuk/cleverchatty/core/test"
)

func TestObjectCreate(t *testing.T) {
	cleverChattyObj, err := GetCleverChatty(CleverChattyConfig{
		Model:        "mock:mock",
		ToolsServers: map[string]ServerConfigWrapper{},
	}, context.Background())

	if err != nil {
		t.Fatalf("Failed to create CleverChatty object: %v", err)
	}
	if cleverChattyObj == nil {
		t.Fatal("CleverChatty object is nil")
	}

	err = cleverChattyObj.Init()
	if err != nil {
		t.Fatalf("Failed to init CleverChatty object: %v", err)
	}

	if cleverChattyObj.provider == nil {
		t.Fatal("Provider is nil")
	}
	if cleverChattyObj.toolsHost == nil {
		t.Fatal("MCPHost is nil")
	}
	if cleverChattyObj.logger == nil {
		t.Fatal("Logger is nil")
	}
}

func TestObjectWithOneServerCreate(t *testing.T) {
	// TODO: This test requires proper mock MCP server infrastructure
	// The internal server config with Kind="mock" is not yet supported
	t.Skip("Skipping: mock MCP server infrastructure not yet implemented")

	cleverChattyObj, err := GetCleverChatty(CleverChattyConfig{
		Model: "mock:mock",
		ToolsServers: map[string]ServerConfigWrapper{
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
	if cleverChattyObj.toolsHost == nil {
		t.Fatal("MCPHost is nil")
	}
	if cleverChattyObj.logger == nil {
		t.Fatal("Logger is nil")
	}
	if len(cleverChattyObj.toolsHost.config) != 1 {
		t.Fatalf("Expected 1 server, got %d", len(cleverChattyObj.toolsHost.config))
	}
	if _, ok := cleverChattyObj.toolsHost.config["test"]; !ok {
		t.Fatalf("Expected server 'test' not found in config")
	}
	if _, ok := cleverChattyObj.toolsHost.mcpClients["test"]; !ok {
		t.Fatalf("Expected client for server 'test' not found in clients")
	}
	if _, ok := cleverChattyObj.toolsHost.mcpClients["test"].(*test.MockMCPClient); !ok {
		t.Fatalf("Expected client for server 'test' to be of type MockMCPClient")
	}
	if len(cleverChattyObj.toolsHost.mcpClients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(cleverChattyObj.toolsHost.mcpClients))
	}
	if cleverChattyObj.toolsHost.tools == nil {
		t.Fatal("Tools are nil")
	}
	if len(cleverChattyObj.toolsHost.tools) != 1 {
		t.Fatalf("Expected 1 tools, got %d", len(cleverChattyObj.toolsHost.tools))
	}
	if cleverChattyObj.toolsHost.tools[0].Name != "test__tool1" {
		t.Fatalf("Expected tool name 'tool1', got '%s'", cleverChattyObj.toolsHost.tools[0].Name)
	}
}
