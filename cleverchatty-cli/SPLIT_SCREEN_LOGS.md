# Split-Screen Logs Feature

The cleverchatty-cli now supports a split-screen interface when running in standalone mode with logs enabled.

## How to Enable

To enable the split-screen interface with logs visible on the right side:

### Option 1: Using config file

In your `config.json`, set the log file path to "stdout":

```json
{
  "log_file_path": "stdout",
  "debug_mode": true,
  ...
}
```

### Option 2: Using command-line flag

Currently, the `log_file_path` needs to be set via config file. If you want to add a CLI flag for this, you can add:

```bash
cleverchatty-cli --config config.json
```

Where `config.json` contains `"log_file_path": "stdout"`.

## How It Works

When `log_file_path` is set to "stdout" in standalone mode:
- The interface switches to a split-screen layout
- **Left side**: Chat area showing your conversation with the AI
- **Right side**: Logs area showing real-time debug logs
- **Bottom**: Input area for entering prompts
- Logs auto-scroll to show the latest entries

When logs are NOT enabled (default or when `log_file_path` is a file path):
- The interface uses the simple single-screen layout
- Only the chat conversation is shown
- Logs are written to the specified file

## Navigation

### Input
- Type your prompts in the input area at the bottom (supports multi-line input)
- Press **Enter** to submit your prompt
- Press **Alt+Enter** to add a new line within your prompt (for multi-line messages)
- Press **Ctrl+C** to quit

### Scrolling
- Press **Page Up** to scroll up through chat history
- Press **Page Down** to scroll down through chat history
- Press **Ctrl+Up** / **Ctrl+Down** to scroll one line at a time
- Press **Ctrl+Home** to jump to the top of chat history
- Press **Ctrl+End** to jump to the bottom (latest messages)
- When scrolled up, you'll see: "↑ More messages above (PgUp/PgDn to scroll, Ctrl+Home for top) ↑"

### Commands
- Use slash commands like `/help`, `/tools`, `/servers`, `/history`, etc.

## Notes

- This feature is **only available in standalone mode**
- In client mode (when connecting to a CleverChatty server), logs are handled server-side
- The split-screen interface uses Bubble Tea for smooth terminal rendering
- The logs viewport automatically scrolls to show the latest logs
