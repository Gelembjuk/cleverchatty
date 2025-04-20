package test

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
)

type MockMCPClient struct {
}

func (m *MockMCPClient) Initialize(
	ctx context.Context,
	request mcp.InitializeRequest,
) (*mcp.InitializeResult, error) {
	return nil, nil
}

func (m *MockMCPClient) Ping(ctx context.Context) error {
	return nil
}

func (m *MockMCPClient) ListResourcesByPage(
	ctx context.Context,
	request mcp.ListResourcesRequest,
) (*mcp.ListResourcesResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ListResources(
	ctx context.Context,
	request mcp.ListResourcesRequest,
) (*mcp.ListResourcesResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ListResourceTemplatesByPage(
	ctx context.Context,
	request mcp.ListResourceTemplatesRequest,
) (*mcp.ListResourceTemplatesResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ListResourceTemplates(
	ctx context.Context,
	request mcp.ListResourceTemplatesRequest,
) (*mcp.ListResourceTemplatesResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ReadResource(
	ctx context.Context,
	request mcp.ReadResourceRequest,
) (*mcp.ReadResourceResult, error) {
	return nil, nil
}
func (m *MockMCPClient) Subscribe(ctx context.Context, request mcp.SubscribeRequest) error {
	return nil
}
func (m *MockMCPClient) Unsubscribe(ctx context.Context, request mcp.UnsubscribeRequest) error {
	return nil
}
func (m *MockMCPClient) ListPromptsByPage(
	ctx context.Context,
	request mcp.ListPromptsRequest,
) (*mcp.ListPromptsResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ListPrompts(
	ctx context.Context,
	request mcp.ListPromptsRequest,
) (*mcp.ListPromptsResult, error) {
	return nil, nil
}
func (m *MockMCPClient) GetPrompt(
	ctx context.Context,
	request mcp.GetPromptRequest,
) (*mcp.GetPromptResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ListToolsByPage(
	ctx context.Context,
	request mcp.ListToolsRequest,
) (*mcp.ListToolsResult, error) {
	return nil, nil
}
func (m *MockMCPClient) ListTools(
	ctx context.Context,
	request mcp.ListToolsRequest,
) (*mcp.ListToolsResult, error) {
	result := &mcp.ListToolsResult{
		PaginatedResult: mcp.PaginatedResult{},
		Tools: []mcp.Tool{
			{
				Name:           "tool1",
				Description:    "This is tool 1",
				InputSchema:    mcp.ToolInputSchema{},
				RawInputSchema: json.RawMessage{},
			},
		},
	}
	return result, nil
}
func (m *MockMCPClient) CallTool(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if request.Params.Arguments["argument"].(string) == "return_error_response" {
		return nil, errors.New("FAKE_ERROR_RESPONSE")
	}

	// Simulate a tool call
	if request.Params.Arguments["argument"].(string) == "return_empty_text" {
		return &mcp.CallToolResult{
			Result: mcp.Result{
				Meta: map[string]interface{}{},
			},
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "",
				},
			},
		}, nil
	}
	return &mcp.CallToolResult{
		Result: mcp.Result{
			Meta: map[string]interface{}{},
		},
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: "FAKE_TOOL_RESPONSE:" + request.Params.Arguments["argument"].(string),
			},
		},
	}, nil
}
func (m *MockMCPClient) SetLevel(ctx context.Context, request mcp.SetLevelRequest) error {
	return nil
}
func (m *MockMCPClient) Complete(
	ctx context.Context,
	request mcp.CompleteRequest,
) (*mcp.CompleteResult, error) {
	return nil, nil
}
func (m *MockMCPClient) Close() error {
	return nil
}
func (m *MockMCPClient) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	// Do nothing
}
