package core

import (
	"encoding/json"
	"testing"
)

func TestServerConfigWrapper_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected []string
	}{
		{
			name:     "single interface string",
			json:     `{"interface": "memory", "url": "http://localhost:8080"}`,
			expected: []string{"memory"},
		},
		{
			name:     "multiple interfaces array",
			json:     `{"interface": ["memory", "tasks"], "url": "http://localhost:8080"}`,
			expected: []string{"memory", "tasks"},
		},
		{
			name:     "empty interface",
			json:     `{"interface": "", "url": "http://localhost:8080"}`,
			expected: nil,
		},
		{
			name:     "mixed types in array (ignore non-strings)",
			json:     `{"interface": ["memory", 123, "tasks"], "url": "http://localhost:8080"}`,
			expected: []string{"memory", "tasks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w ServerConfigWrapper
			err := json.Unmarshal([]byte(tt.json), &w)
			if err != nil {
				t.Fatalf("UnmarshalJSON failed: %v", err)
			}

			if len(w.Interfaces) != len(tt.expected) {
				t.Errorf("expected %d interfaces, got %d: %v", len(tt.expected), len(w.Interfaces), w.Interfaces)
			}

			for i, iface := range w.Interfaces {
				if iface != tt.expected[i] {
					t.Errorf("expected interface %s at index %d, got %s", tt.expected[i], i, iface)
				}
			}
		})
	}
}

func TestServerConfigWrapper_MarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		interfaces   []string
		expectedJSON string // partial match
	}{
		{
			name:       "single interface",
			interfaces: []string{"memory"},
		},
		{
			name:       "multiple interfaces",
			interfaces: []string{"memory", "tasks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := ServerConfigWrapper{
				Interfaces: tt.interfaces,
				Config: SSEMCPServerConfig{
					Url: "http://localhost:8080",
				},
			}

			data, err := json.Marshal(w)
			if err != nil {
				t.Fatalf("MarshalJSON failed: %v", err)
			}

			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			if err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			val, ok := result["interface"]
			if !ok {
				t.Fatalf("interface field missing in json")
			}

			if len(tt.interfaces) == 1 {
				if str, ok := val.(string); !ok || str != tt.interfaces[0] {
					t.Errorf("expected string definition for single interface, got %v", val)
				}
			} else {
				if arr, ok := val.([]interface{}); !ok {
					t.Errorf("expected array definition for multiple interfaces, got %T", val)
				} else {
					if len(arr) != len(tt.interfaces) {
						t.Errorf("expected %d interfaces in json, got %d", len(tt.interfaces), len(arr))
					}
				}
			}
		})
	}
}
