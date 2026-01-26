module github.com/gelembjuk/cleverchatty/email_mock_http_server

go 1.23.2

toolchain go1.24.3

require (
	github.com/gelembjuk/cleverchatty/email_mock_shared v0.0.0
	github.com/mark3labs/mcp-go v0.8.1
)

replace github.com/gelembjuk/cleverchatty/email_mock_shared => ../shared

require github.com/google/uuid v1.6.0 // indirect
