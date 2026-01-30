package core

import (
	"context"
	"fmt"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/gelembjuk/cleverchatty/core/llm"
)

// CustomToolHandler is the callback function type for custom tools.
// It receives the tool arguments as a map and returns the result string and an error.
type CustomToolHandler func(ctx context.Context, args map[string]interface{}) (string, error)

// ToolArgument defines a single argument for a custom tool
type ToolArgument struct {
	Name        string // Argument name
	Type        string // JSON Schema type: "string", "number", "integer", "boolean", "array", "object"
	Description string // Description of the argument for the LLM
	Required    bool   // Whether this argument is required
}

// CustomTool defines a custom tool that can be registered with CleverChatty
type CustomTool struct {
	Name        string            // Tool name (will be prefixed with "custom__")
	Description string            // Tool description for the LLM
	Arguments   []ToolArgument    // List of arguments
	Handler     CustomToolHandler // Callback function to execute when the tool is called
}

// Validate checks if the custom tool definition is valid
func (ct *CustomTool) Validate() error {
	if ct.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if ct.Description == "" {
		return fmt.Errorf("tool description is required")
	}
	if ct.Handler == nil {
		return fmt.Errorf("tool handler is required")
	}
	// Validate argument types
	validTypes := map[string]bool{
		"string":  true,
		"number":  true,
		"integer": true,
		"boolean": true,
		"array":   true,
		"object":  true,
	}
	for _, arg := range ct.Arguments {
		if arg.Name == "" {
			return fmt.Errorf("argument name is required")
		}
		if arg.Type == "" {
			return fmt.Errorf("argument type is required for argument %s", arg.Name)
		}
		if !validTypes[arg.Type] {
			return fmt.Errorf("invalid argument type %s for argument %s", arg.Type, arg.Name)
		}
	}
	return nil
}

// toLLMTool converts a CustomTool to llm.Tool format
func (ct *CustomTool) toLLMTool() llm.Tool {
	properties := make(map[string]interface{})
	var required []string

	for _, arg := range ct.Arguments {
		properties[arg.Name] = map[string]interface{}{
			"type":        arg.Type,
			"description": arg.Description,
		}
		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	return llm.Tool{
		Name:        fmt.Sprintf("%s__%s", customToolsServerName, ct.Name),
		Description: ct.Description,
		InputSchema: llm.Schema{
			Type:       "object",
			Properties: properties,
			Required:   required,
		},
	}
}

const customToolsServerName = "custom"

// callCustomTool executes a custom tool by its name
func (host *ToolsHost) callCustomTool(toolName string, toolArgs map[string]interface{}, ctx context.Context) ToolCallResult {
	host.customToolsMux.RLock()
	tool, ok := host.customTools[toolName]
	host.customToolsMux.RUnlock()

	if !ok {
		return ToolCallResult{
			Error: fmt.Errorf("custom tool %s not found", toolName),
		}
	}

	host.logger.Printf("Calling custom tool %s", toolName)

	result, err := tool.Handler(ctx, toolArgs)
	if err != nil {
		return ToolCallResult{
			Error: fmt.Errorf("custom tool %s failed: %w", toolName, err),
		}
	}

	return ToolCallResult{
		Content: history.NewTextContent(result),
	}
}

// isCustomTool checks if a server name refers to a custom tool
func (host *ToolsHost) isCustomTool(serverName string) bool {
	return serverName == customToolsServerName
}

// AddCustomTool registers a new custom tool
func (host *ToolsHost) AddCustomTool(tool CustomTool) error {
	if err := tool.Validate(); err != nil {
		return fmt.Errorf("invalid custom tool: %w", err)
	}

	host.customToolsMux.Lock()
	defer host.customToolsMux.Unlock()

	if host.customTools == nil {
		host.customTools = make(map[string]CustomTool)
	}

	host.customTools[tool.Name] = tool
	host.logger.Printf("Custom tool %s registered", tool.Name)
	return nil
}

// RemoveCustomTool unregisters a custom tool by name
func (host *ToolsHost) RemoveCustomTool(name string) {
	host.customToolsMux.Lock()
	defer host.customToolsMux.Unlock()

	delete(host.customTools, name)
	host.logger.Printf("Custom tool %s removed", name)
}

// getCustomToolsForLLM returns all custom tools in llm.Tool format
func (host *ToolsHost) getCustomToolsForLLM() []llm.Tool {
	host.customToolsMux.RLock()
	defer host.customToolsMux.RUnlock()

	tools := make([]llm.Tool, 0, len(host.customTools))
	for _, tool := range host.customTools {
		tools = append(tools, tool.toLLMTool())
	}
	return tools
}
