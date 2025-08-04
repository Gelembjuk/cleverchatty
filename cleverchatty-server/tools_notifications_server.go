package main

import (
	"context"
	"fmt"
	"log"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
)

type ToolsNotificationsServer struct {
	Config          *cleverchatty.CleverChattyConfig
	SessionsManager *cleverchatty.SessionManager
	Logger          *log.Logger
	StopChan        chan struct{}
	ListenTools     []string
}

func getToolsNotificationsServer(
	sessionsManager *cleverchatty.SessionManager,
	config *cleverchatty.CleverChattyConfig,
	logger *log.Logger) (*ToolsNotificationsServer, error) {

	toolsNotificationsServer := &ToolsNotificationsServer{
		Config:          config,
		SessionsManager: sessionsManager,
		Logger:          logger,
	}

	// set list of tools to listen to. we will process notifications for these tools
	toolsNotificationsServer.ListenTools = make([]string, 0)
	for _, server := range config.ToolsListenerConfig.ToolServers {
		for _, toolName := range server.Tools {
			toolsNotificationsServer.ListenTools = append(toolsNotificationsServer.ListenTools, fmt.Sprintf("%s__%s", server.Name, toolName))
		}
	}
	return toolsNotificationsServer, nil
}

func (s *ToolsNotificationsServer) Start() error {
	// Start the tools notifications server
	ctx, cancel := context.WithCancel(context.Background())
	s.Logger.Println("Tools notifications server started with cancellable context.")
	toolsHost, err := cleverchatty.NewToolsHost(
		s.getToolsServers(),
		s.Logger,
		ctx,
	)
	if err != nil {
		return err
	}
	err = toolsHost.Init()
	if err != nil {
		return err
	}
	s.StopChan = make(chan struct{})
	go func() {
		// Server logic here

		<-s.StopChan
	}()
	cancel()
	return nil
}

func (s *ToolsNotificationsServer) Stop() error {
	// Stop the tools notifications server
	close(s.StopChan)
	s.Logger.Println("Tools notifications server stopped.")
	return nil
}

func (s *ToolsNotificationsServer) getToolsServers() map[string]cleverchatty.ServerConfigWrapper {
	list := make(map[string]cleverchatty.ServerConfigWrapper)

	for _, server := range s.Config.ToolsListenerConfig.ToolServers {
		for toolsServerID, toolsServer := range s.Config.ToolsServers {
			if server.ServerID == toolsServerID {
				list[server.ServerID] = toolsServer
			}
		}
	}

	return list
}
