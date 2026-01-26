.PHONY: all cc clean core cli server dev_tools

# Build cleverchatty CLI and server
cc: cli server

# Build everything
all: cc dev_tools

# Build core library
core:
	cd core && go build ./...

# Build CLI
cli: core
	cd cleverchatty-cli && go build -o cleverchatty-cli

# Build server
server: core
	cd cleverchatty-server && go build -o cleverchatty-server

# Build all dev tools
dev_tools: dev_tools_email dev_tools_notifications dev_tools_misc

dev_tools_email:
	cd dev_tools/email_mock/http-server && go build -o email-http-server
	cd dev_tools/email_mock/stdio-server && go build -o email-stdio-server

dev_tools_notifications:
	cd dev_tools/notifications_client/http-server && go build -o notifications-http-server
	cd dev_tools/notifications_client/stdio-server && go build -o notifications-stdio-server
	cd dev_tools/notifications_client/http-client && go build -o notifications-http-client
	cd dev_tools/notifications_client/stdio-client && go build -o notifications-stdio-client

dev_tools_misc:
	cd dev_tools/reverse_mcp_server && go build -o reverse-mcp-server
	cd dev_tools/listener_verify && go build -o listener-verify

# Clean build artifacts
clean:
	rm -f cleverchatty-cli/cleverchatty-cli
	rm -f cleverchatty-server/cleverchatty-server
	rm -f dev_tools/email_mock/http-server/email-http-server
	rm -f dev_tools/email_mock/stdio-server/email-stdio-server
	rm -f dev_tools/notifications_client/http-server/notifications-http-server
	rm -f dev_tools/notifications_client/stdio-server/notifications-stdio-server
	rm -f dev_tools/notifications_client/http-client/notifications-http-client
	rm -f dev_tools/notifications_client/stdio-client/notifications-stdio-client
	rm -f dev_tools/reverse_mcp_server/reverse-mcp-server
	rm -f dev_tools/listener_verify/listener-verify

# Run tests
test:
	cd core && go test ./...

# Tidy all modules
tidy:
	cd core && go mod tidy
	cd cleverchatty-cli && go mod tidy
	cd cleverchatty-server && go mod tidy
