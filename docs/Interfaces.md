# CleverChatty Tools Interfaces

A tool interface is a "native invention" in this project. It allows to define a tool description required for specific tool server.

Currently there are two interfaces supported:
- `rag` - for RAG (Retrieval-Augmented Generation) tools working over MCP or A2A protocols.
- `memory` - for AI memory tools working over MCP or A2A protocols.

MOre details about these interfaces can be found in this blog post [What’s Missing in MCP](https://gelembjuk.com/blog/post/mcp_missings/).

## Memory interface

This tools allows to use long-term AI memory using a MCP server of specific interface.

### `remember` tool accepts two arguments:
- `role`: The role of the data, e.g. "user", "assistant"
- `contents`: The contents to remember, usually the text of the message

### `recall` tool accepts one argument:
- `query`: The query to search for the data in the memory. If empty, it is expected to return some common memories.

Example:

```python
@mcp.tool()
def remember(role: str, contents) -> str:
    """Remembers new data in the memory
    
    Args:
        role (str): The role of the data, e.g. "user", "assistant"
        contents (str): The contents to remember, usually the text of the message
    """

    Memory(config).remember(role, contents)

    return "ok"

@mcp.tool()
def recall(query: str = "") -> str:
    """Recall the memory"""
    
    r = Memory(config).recall(query)

    if not r:
        return "none"
    
    return r
```

## RAG interface

This tools allows to use RAG using a MCP server of specific interface.

### `knowledge_search` tool accepts two arguments:
- `query`: The query to search for documents and information
- `num`: The number of results to return

The expected interface is the one tool named `knowledge_search` with two arguments: `query` and `num`. 

Example:

```golang
func createServer() *server.MCPServer {
	// Create MCP server
	s := server.NewMCPServer(
		"Retrieval-Augmented Generation Server. Adding the company data to the AI chat.",
		"1.0.0",
	)

	execTool := mcp.NewTool("knowledge_search",
		mcp.WithDescription("Search for documents and information over the company data storage"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The query to search for documents and information"),
		),
		mcp.WithNumber("num",
			mcp.Description("The number of results to return"),
		),
	)

	s.AddTool(execTool, cmdKnowledgeSearch)
	return s
}
```