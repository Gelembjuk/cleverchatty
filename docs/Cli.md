# CleverChatty CLI

The CleverChatty CLI is a command-line interface tool that allows users to interact with the CleverChatty server or run it as a standalone AI chat client.

- [Standalone Mode](#standalone-mode)
    - [Usage with config file](#usage-with-config-file)
    - [Usage with inline arguments](#usage-with-inline-arguments)
    - [Hybrid Mode](#hybrid-mode)

## Standalone Mode

In standalone mode, the CLI can be used to chat with AI models without connecting to a server. The tool includes the server internally and it works while the interactive session is running. The user can specify the model to use, and the CLI will handle the chat interactions.

It is the quick way to run A chat with AI models without the need to set up a server. In this mode, the user can specify the model to use, configure tools servers (MCP and A2A servers), connect Memory or RAG servers, and chat with the AI model directly.

### Usage with config file

```bash
cleverchatty-cli --config /path/to/config.json
```

Details of the config file can be found in the [CleverChatty Config](Config.md) documentation.

### Usage with inline arguments

This is the quickest way to run the CLI without a config file. You can specify the model in the command line. But in this mode you cannot use tools servers (MCP and A2A servers), Memory or RAG servers.

```bash
cleverchatty-cli --model ollama:llama2:7b 
```

```bash
cleverchatty-cli --model anthropic:claude-2 --anthropic-api-key YOUR_ANTHROPIC_API_KEY
```

### Hybrid Mode

In the hybrid mode, it is possible to use the config file and inline arguments together. This allows for more flexibility in configuring the CLI while still being able to quickly override settings from the command line.

For example, a model in the config is set to `ollama:llama2:7b`, but you want to use a different model for a specific session. You can do this by specifying the model in the command line:

```bash
cleverchatty-cli --config /path/to/config.json --model anthropic:claude-2 --anthropic-api-key YOUR_ANTHROPIC_API_KEY --agentid user123
```

![<img src="cleverchatty_cli_standalone.png" width="250"/>](cleverchatty_cli_standalone.png)

## Client Mode

In client mode, the CLI connects to a CleverChatty server. This allows users to interact with the server and use its capabilities without managing models or MCP servers locally.

The server can be run anywhere in the network, and the CLI will connect to it using the provided server address and agent ID. 

Agent ID is used to identify the user in the CleverChatty server. It is not always required, it depends if the server is configured to require user identification. In most cases it would be required.

```
cleverchatty-cli --server http://localhost:8080 --agentid user123
```

```
cleverchatty-cli --server https://some_host --agentid user123
```

![<img src="cleverchatty_cli.png" width="250"/>](cleverchatty_cli.png)