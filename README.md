# CleverChatty

**CleverChatty** is a Go package that implements the core functionality of an AI chat system. It encapsulates all business logic required for an AI-powered chat, while remaining independent of any specific user interface.

## Key Features

- Decoupled architecture: UI is not included in this package and is intended to be implemented separately. This allows for flexible front-end development across multiple platforms.
- Unified backend logic: Provides a single source of truth for chat behavior, making it easy to maintain and test.
- LLM prompt handling: Send prompts and receive responses from supported large language models.
- MCP server support: Built-in support for MCP servers as part of the chat infrastructure.

## Usage

This package is ideal for developers building custom UIs (e.g., CLI, web, mobile) that require AI chat capabilities without duplicating logic across implementations.

## Credits

The first version of this application was the copy of [mcphost](https://github.com/mark3labs/mcphost) refactored to remove the UI.