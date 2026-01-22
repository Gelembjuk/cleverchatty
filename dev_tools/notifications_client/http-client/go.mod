module github.com/gelembjuk/cleverchatty/notifications_http_client

go 1.24.3

require (
	github.com/gelembjuk/cleverchatty/notifications_shared v0.0.0
	github.com/mark3labs/mcp-go v0.11.0
)

require github.com/google/uuid v1.6.0 // indirect

replace github.com/gelembjuk/cleverchatty/notifications_shared => ../shared
