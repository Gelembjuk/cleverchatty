package core

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

const (
	ThisAppName                = "CleverChatty"
	ThisAppVersion             = "0.2.0"
	transportStdio             = "stdio"
	transportHTTPStreaming     = "http_streaming"
	transportSSE               = "sse"
	transportA2A               = "a2a"
	transportInternal          = "internal"
	toolsServerInterfaceNone   = "none"
	toolsServerInterfaceMemory = "memory"
	toolsServerInterfaceRAG    = "rag"
	defaultMessagesWindow      = 10
	initialBackoff             = 1 * time.Second
	maxBackoff                 = 30 * time.Second
	maxRetries                 = 5    // Will reach close to max backoff
	defaultSessionTimeout      = 3600 // Default session timeout
)

const (
// this will be changed in the future. The text will be removed from here
// commentOnNotificationReceived = "Notification received from server: %s. The tool %s has been called. The next message is the content of the notification."
)

type ServerConfig struct {
	SessionTimeout int `json:"session_timeout"`
}

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

type ToolsServerConfig interface {
	GetType() string
}

type STDIOMCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

func (s STDIOMCPServerConfig) GetType() string {
	return transportStdio
}

type HTTPStreamingMCPServerConfig struct {
	Url     string   `json:"url"`
	Headers []string `json:"headers,omitempty"`
}

func (s HTTPStreamingMCPServerConfig) GetType() string {
	return transportHTTPStreaming
}

type SSEMCPServerConfig struct {
	Url     string   `json:"url"`
	Headers []string `json:"headers,omitempty"`
}

func (s SSEMCPServerConfig) GetType() string {
	return transportSSE
}

type A2AToolsServerConfig struct {
	Endpoint string            `json:"endpoint"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (s A2AToolsServerConfig) GetType() string {
	return transportA2A
}

type InternalServerConfig struct {
	Kind string `json:"kind"`
}

func (s InternalServerConfig) GetType() string {
	return transportInternal
}

type ServerConfigWrapper struct {
	Config    ToolsServerConfig
	Interface string `json:"interface"`
	Disabled  bool   `json:"disabled"`
	Required  bool   `json:"required"`
}

type RAGConfig struct {
	ContextPrefix        string `json:"context_prefix"`
	RequirePreprocessing bool   `json:"require_preprocessing"`
	PreprocessingPrompt  string `json:"preprocessing_prompt"`
}

type A2AServerConfig struct {
	Enabled              bool   `json:"enabled"`
	AgentIDRequired      bool   `json:"agent_id_required"`
	Url                  string `json:"url"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	ListenHost           string `json:"listen_host"`
	Organization         string `json:"organization"`
	ChatSkillName        string `json:"chat_skill_name,omitempty"`
	ChatSkillDescription string `json:"chat_skill_description,omitempty"`
}

type CleverChattyConfig struct {
	AgentID           string                         `json:"agent_id"`
	ServerConfig      ServerConfig                   `json:"server"`
	LogFilePath       string                         `json:"log_file_path"`
	DebugMode         bool                           `json:"debug_mode"`
	MessageWindow     int                            `json:"message_window"`
	Model             string                         `json:"model"`
	SystemInstruction string                         `json:"system_instruction"`
	Anthropic         AnthropicConfig                `json:"anthropic"`
	OpenAI            OpenAIConfig                   `json:"openai"`
	Google            GoogleConfig                   `json:"google"`
	ToolsServers      map[string]ServerConfigWrapper `json:"tools_servers,omitempty"`
	RAGConfig         RAGConfig                      `json:"rag_settings"`
	A2AServerConfig   A2AServerConfig                `json:"a2a_settings"`
}

func CreateStandardConfigFile(configPath string) (*CleverChattyConfig, error) {
	// Create the config file with default values
	defaultConfig := CleverChattyConfig{
		ServerConfig: ServerConfig{
			SessionTimeout: defaultSessionTimeout,
		},
		LogFilePath:   "cleverchatty.log",
		DebugMode:     false,
		MessageWindow: 10,
		Model:         "",
		ToolsServers:  make(map[string]ServerConfigWrapper),
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
	if config.ServerConfig.SessionTimeout <= 0 {
		config.ServerConfig.SessionTimeout = defaultSessionTimeout
	}

	return &config, nil
}

func (w *ServerConfigWrapper) UnmarshalJSON(data []byte) error {
	var typeField struct {
		Url       string `json:"url"`
		Endpoint  string `json:"endpoint"`
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
			var sse SSEMCPServerConfig
			if err := json.Unmarshal(data, &sse); err != nil {
				return err
			}
			w.Config = sse
		} else {
			// Otherwise, treat it as an HTTP streaming server
			var httpStreaming HTTPStreamingMCPServerConfig
			if err := json.Unmarshal(data, &httpStreaming); err != nil {
				return err
			}
			w.Config = httpStreaming
		}
	} else if typeField.Endpoint != "" {
		// If the Endpoint field is present, treat it as an A2A server
		var a2a A2AToolsServerConfig
		if err := json.Unmarshal(data, &a2a); err != nil {
			return err
		}
		w.Config = a2a
	} else {
		// Otherwise, treat it as a STDIOServerConfig
		var stdio STDIOMCPServerConfig
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
	return w.Interface == toolsServerInterfaceMemory
}

func (w ServerConfigWrapper) isRAGServer() bool {
	return w.Interface == toolsServerInterfaceRAG
}

func (w ServerConfigWrapper) isMCPServer() bool {
	return w.Config.GetType() == transportSSE ||
		w.Config.GetType() == transportHTTPStreaming ||
		w.Config.GetType() == transportStdio
}

func (w ServerConfigWrapper) isA2AServer() bool {
	return w.Config.GetType() == transportA2A
}

func InitLogger(logFilePath string, debugMMode bool) (*log.Logger, error) {
	// Initialize the logger with the specified log file path
	var logger *log.Logger

	if logFilePath == "stdout" {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	} else if logFilePath != "" {
		f1, err := os.OpenFile(logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)

		if err != nil {
			return nil, fmt.Errorf("error opening log file: %v", err)
		}

		logger = log.New(f1, "", log.LstdFlags)
	} else {
		logger = log.New(io.Discard, "", log.LstdFlags)
	}

	// Set the log level based on the debug flag
	if debugMMode {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}
	return logger, nil
}
