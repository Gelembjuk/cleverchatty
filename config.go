package cleverchatty

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

const (
	thisToolName             = "CleverChatty"
	thisToolVersion          = "0.1.0"
	transportStdio           = "stdio"
	transportHTTPStreaming   = "http_streaming"
	transportSSE             = "sse"
	transportInternal        = "internal"
	mcpServerInterfaceNone   = "none"
	mcpServerInterfaceMemory = "memory"
	mcpServerInterfaceRAG    = "rag"
	defaultMessagesWindow    = 10
	initialBackoff           = 1 * time.Second
	maxBackoff               = 30 * time.Second
	maxRetries               = 5 // Will reach close to max backoff
)

const (
	commentOnNotificationReceived = "Notification received from server: %s. The tool %s has been called. The next message is the content of the notification."
)

type OpenAIConfig struct {
	APIKey       string `json:"apikey"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
}

type AnthropicConfig struct {
	APIKey       string `json:"apikey"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
}

type GoogleConfig struct {
	APIKey       string `json:"apikey"`
	DefaultModel string `json:"default_model"`
}

type ServerConfig interface {
	GetType() string
}

type STDIOServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

func (s STDIOServerConfig) GetType() string {
	return transportStdio
}

type HTTPStreamingServerConfig struct {
	Url     string   `json:"url"`
	Headers []string `json:"headers,omitempty"`
}

func (s HTTPStreamingServerConfig) GetType() string {
	return transportHTTPStreaming
}

type SSEServerConfig struct {
	Url     string   `json:"url"`
	Headers []string `json:"headers,omitempty"`
}

func (s SSEServerConfig) GetType() string {
	return transportSSE
}

type InternalServerConfig struct {
	Kind string `json:"kind"`
}

func (s InternalServerConfig) GetType() string {
	return transportInternal
}

type ServerConfigWrapper struct {
	Config    ServerConfig
	Interface string `json:"interface"`
	Disabled  bool   `json:"disabled"`
	Required  bool   `json:"required"`
}

type RAGConfig struct {
	ContextPrefix        string `json:"context_prefix"`
	RequirePreprocessing bool   `json:"require_preprocessing"`
	PreprocessingPrompt  string `json:"preprocessing_prompt"`
}

type CleverChattyConfig struct {
	LogFilePath       string                         `json:"log_file_path"`
	DebugMode         bool                           `json:"debug_mode"`
	MessageWindow     int                            `json:"message_window"`
	Model             string                         `json:"model"`
	SystemInstruction string                         `json:"system_instruction"`
	Anthropic         AnthropicConfig                `json:"anthropic"`
	OpenAI            OpenAIConfig                   `json:"openai"`
	Google            GoogleConfig                   `json:"google"`
	MCPServers        map[string]ServerConfigWrapper `json:"mcpServers"`
	RAGConfig         RAGConfig                      `json:"rag_settings"`
}

func CreateStandardConfigFile(configPath string) (*CleverChattyConfig, error) {
	// Create the config file with default values
	defaultConfig := CleverChattyConfig{
		LogFilePath:   "cleverchatty.log",
		DebugMode:     false,
		MessageWindow: 10,
		Model:         "",
		MCPServers:    make(map[string]ServerConfigWrapper),
		RAGConfig:     RAGConfig{ContextPrefix: "Context:"},
	}

	configData, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(configPath, configData, 0644)

	if err != nil {
		return nil, fmt.Errorf(
			"error writing config file %s: %w",
			configPath,
			err,
		)
	}
	return &defaultConfig, nil
}

func LoadConfig(configPath string) (*CleverChattyConfig, error) {
	// Read existing config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf(
			"error reading config file %s: %w",
			configPath,
			err,
		)
	}

	var config CleverChattyConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	if config.MessageWindow <= 0 {
		config.MessageWindow = defaultMessagesWindow
	}

	return &config, nil
}

func (w *ServerConfigWrapper) UnmarshalJSON(data []byte) error {
	var typeField struct {
		Url       string `json:"url"`
		Transport string `json:"transport"`
		Interface string `json:"interface"`
		Disabled  bool   `json:"disabled"`
		Required  bool   `json:"required"`
	}

	if err := json.Unmarshal(data, &typeField); err != nil {
		return err
	}
	w.Interface = typeField.Interface
	w.Disabled = typeField.Disabled
	w.Required = typeField.Required

	if typeField.Url != "" {
		if typeField.Transport == transportSSE {
			// If the URL field is present, treat it as an SSE server
			var sse SSEServerConfig
			if err := json.Unmarshal(data, &sse); err != nil {
				return err
			}
			w.Config = sse
		} else {
			// Otherwise, treat it as an HTTP streaming server
			var httpStreaming HTTPStreamingServerConfig
			if err := json.Unmarshal(data, &httpStreaming); err != nil {
				return err
			}
			w.Config = httpStreaming
		}
	} else {
		// Otherwise, treat it as a STDIOServerConfig
		var stdio STDIOServerConfig
		if err := json.Unmarshal(data, &stdio); err != nil {
			return err
		}
		w.Config = stdio
	}

	return nil
}
func (w ServerConfigWrapper) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.Config)
}

func (w ServerConfigWrapper) isMemoryServer() bool {
	return w.Interface == mcpServerInterfaceMemory
}

func (w ServerConfigWrapper) isRAGServer() bool {
	return w.Interface == mcpServerInterfaceRAG
}

func initLogger(config *CleverChattyConfig) (*log.Logger, error) {
	// Initialize the logger with the specified log file path
	var logger *log.Logger

	if config.LogFilePath == "stdout" {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	} else if config.LogFilePath != "" {
		f1, err := os.OpenFile(config.LogFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)

		if err != nil {
			return nil, fmt.Errorf("error opening log file: %v", err)
		}

		logger = log.New(f1, "", log.LstdFlags)
	} else {
		logger = log.New(io.Discard, "", log.LstdFlags)
	}

	// Set the log level based on the debug flag
	if config.DebugMode {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}
	return logger, nil
}
