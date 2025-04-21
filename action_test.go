package cleverchatty

import (
	"context"
	"testing"
)

func TestBasicChat(t *testing.T) {
	cleverChattyObj, err := GetCleverChatty(CleverChattyConfig{
		Model:      "mock:mock",
		MCPServers: map[string]ServerConfigWrapper{},
	}, context.Background())

	if err != nil {
		t.Fatalf("Failed to create CleverChatty object: %v", err)
	}

	cleverChattyObj.Callbacks.SetResponseReceived(func(response string) error {
		if response != "FAKE_RESPONSE:Hello, how are you?" {
			t.Fatalf("Expected response 'FAKE_RESPONSE:Hello, how are you?', got '%s'", response)
		}
		return nil
	})

	_, err = cleverChattyObj.Prompt("Hello, how are you?")

	if err != nil {
		t.Fatalf("Failed to prompt: %v", err)
	}

	if len(cleverChattyObj.messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(cleverChattyObj.messages))
	}
}

func TestChatWithTool(t *testing.T) {
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

	responseRecevied := false

	cleverChattyObj.Callbacks.SetResponseReceived(func(response string) error {
		if response != "FAKE_ANALYSED_RESPONSE:FAKE_TOOL_RESPONSE:Hello, how are you?" {
			t.Fatalf("Expected response 'FAKE_ANALYSED_RESPONSE:FAKE_TOOL_RESPONSE:Hello, how are you?', got '%s'", response)
		}
		responseRecevied = true
		return nil
	})
	cleverChattyObj.Callbacks.SetToolCalling(func(tool string) error {
		if tool != "test__tool1" {
			t.Fatalf("Expected tool 'test__tool1', got '%s'", tool)
		}
		return nil
	})

	_, err = cleverChattyObj.Prompt("tool:1:Hello, how are you?")

	if err != nil {
		t.Fatalf("Failed to prompt: %v", err)
	}

	if !responseRecevied {
		t.Fatalf("Response not received")
	}
}
