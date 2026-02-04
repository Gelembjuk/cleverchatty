package core

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestToolsHost_validateServerInterfaces(t *testing.T) {
	// Capture logs
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	host := &ToolsHost{
		logger: logger,
	}

	tests := []struct {
		name           string
		interfaces     []string
		availableTools []string
		expectWarning  bool
		missingTools   []string
	}{
		{
			name:           "valid memory interface",
			interfaces:     []string{toolsServerInterfaceMemory},
			availableTools: []string{memoryToolRememberName, memoryToolRecallName, "other_tool"},
			expectWarning:  false,
		},
		{
			name:           "missing memory tool",
			interfaces:     []string{toolsServerInterfaceMemory},
			availableTools: []string{memoryToolRememberName}, // Missing recall
			expectWarning:  true,
			missingTools:   []string{memoryToolRecallName},
		},
		{
			name:           "multiple interfaces valid",
			interfaces:     []string{toolsServerInterfaceMemory, toolsServerInterfaceTasks},
			availableTools: []string{memoryToolRememberName, memoryToolRecallName, "start_task", "get_task_status"},
			expectWarning:  false,
		},
		{
			name:           "multiple interfaces missing one",
			interfaces:     []string{toolsServerInterfaceRAG, toolsServerInterfaceServerCard},
			availableTools: []string{ragToolName}, // Missing get_server_card
			expectWarning:  true,
			missingTools:   []string{"get_server_card"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			host.validateServerInterfaces("test-server", tt.interfaces, tt.availableTools)

			logOutput := buf.String()
			if tt.expectWarning {
				if !strings.Contains(logOutput, "WARNING") {
					t.Errorf("expected warning in logs, got: %s", logOutput)
				}
				for _, tool := range tt.missingTools {
					if !strings.Contains(logOutput, tool) {
						t.Errorf("expected missing tool %s in logs, got: %s", tool, logOutput)
					}
				}
			} else {
				if strings.Contains(logOutput, "WARNING") {
					t.Errorf("unexpected warning in logs: %s", logOutput)
				}
			}
		})
	}
}
