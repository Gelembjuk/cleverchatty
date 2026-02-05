package core

import (
	"context"
	"testing"
)

func TestNewCustomTool(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		description string
		args        []ToolArgument
		handler     CustomToolHandler
		wantErr     bool
	}{
		{
			name:        "Valid tool",
			toolName:    "valid_tool",
			description: "A valid tool",
			args:        []ToolArgument{},
			handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
				return "ok", nil
			},
			wantErr: false,
		},
		{
			name:        "Missing name",
			toolName:    "",
			description: "A tool",
			args:        []ToolArgument{},
			handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
				return "ok", nil
			},
			wantErr: true,
		},
		{
			name:        "Missing description",
			toolName:    "tool",
			description: "",
			args:        []ToolArgument{},
			handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
				return "ok", nil
			},
			wantErr: true,
		},
		{
			name:        "Missing handler",
			toolName:    "tool",
			description: "desc",
			args:        []ToolArgument{},
			handler:     nil,
			wantErr:     true,
		},
		{
			name:        "Invalid argument type",
			toolName:    "tool",
			description: "desc",
			args: []ToolArgument{
				{
					Name: "arg1",
					Type: "invalid_type",
				},
			},
			handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
				return "ok", nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCustomTool(tt.toolName, tt.description, tt.args, tt.handler)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCustomTool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
