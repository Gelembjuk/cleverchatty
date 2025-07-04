# Using CleverChatty to build custom agents

This repository provides a framework for building custom AI agents using the CleverChatty platform. 

Basic example:

```golang
package main

import (
	"context"
	"fmt"
	"os"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
)

func main() {
	config := &cleverchatty.CleverChattyConfig{
		Model: "ollama:qwen2.5:3b",
		ToolsServers: map[string]cleverchatty.ServerConfigWrapper{
			"weather_server": {
				Config: cleverchatty.HTTPStreamingMCPServerConfig{
					Url: "http://weather-service:8000/mcp",
				},
			},
			"get_location_server": {
				Config: cleverchatty.STDIOMCPServerConfig{
					Command: "get_location",
					Args:    []string{"--location"},
				},
			},
		},
	}
    cleverChattyObject, err := cleverchatty.GetCleverChatty(*config, context.Background())

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(1)
	}

	err = cleverChattyObject.Init()

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(2)
	}

	defer func() {
		cleverChattyObject.Finish()
	}()

	response, err := cleverChattyObject.Prompt("What is the weather like outside today?")

	if err != nil {
		fmt.Errorf("Error getting response: %v", err)
		os.Exit(3)
	}
	fmt.Println("Response:", response)

}
```

Same example can look simpler when useng a config file:

```json
{
    "model": "ollama:qwen2.5:3b",
    "tools_servers": {
        "weather_server": {
            "url": "http://weather-service:8000/mcp",
            "type": "http_streaming"
        },
        "get_location_server": {
            "command": "get_location",
            "args": ["--location"],
            "type": "stdio"
        }
    }
}
```

```golang
package main

import (
	"context"
	"fmt"
	"os"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
)

func main() {
	config, err := cleverchatty.LoadConfig("config.json")
    if err != nil {
        fmt.Printf("Error loading config: %v", err)
        os.Exit(4)
    }
    cleverChattyObject, err := cleverchatty.GetCleverChatty(*config, context.Background())

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(1)
	}

	err = cleverChattyObject.Init()

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(2)
	}

	defer func() {
		cleverChattyObject.Finish()
	}()

	response, err := cleverChattyObject.Prompt("What is the weather like outside today?")

	if err != nil {
		fmt.Errorf("Error getting response: %v", err)
		os.Exit(3)
	}
	fmt.Println("Response:", response)

}
```

One more example. Two models are talking to each other. In this example models do not have access to tools. The conversation could be yet more interesting if they had access to tools.

```golang
package main

import (
	"context"
	"fmt"
	"os"

	cleverchatty "github.com/gelembjuk/cleverchatty/core"
)

func main() {
    config_1 := &cleverchatty.CleverChattyConfig{
		Model: "ollama:qwen2.5:3b",
	}

	config_2 := &cleverchatty.CleverChattyConfig{
		Model: "ollama:llama3.1:latest",
	}

	cleverChattyObject_1, err := cleverchatty.GetCleverChatty(*config_1, context.Background())

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(2)
	}

	err = cleverChattyObject_1.Init()

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(3)
	}

	cleverChattyObject_2, err := cleverchatty.GetCleverChatty(*config_2, context.Background())

	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(2)
	}
	err = cleverChattyObject_2.Init()
	if err != nil {
		fmt.Printf("Error creating assistant: %v", err)
		os.Exit(3)
	}

	defer func() {
		cleverChattyObject_1.Finish()
		cleverChattyObject_2.Finish()
	}()

	prompt := "Prepare the first question to someone you meet on the street. You will get the response."

	for {
		prompt, err := cleverChattyObject_1.Prompt(prompt)

		if err != nil {
			fmt.Printf("Error getting response from cleverChattyObject_1: %v", err)
			os.Exit(4)
		}

		fmt.Println("Agent 1:", prompt)

		prompt, err = cleverChattyObject_2.Prompt(prompt)

		if err != nil {
			fmt.Printf("Error getting response from cleverChattyObject_2: %v", err)
			os.Exit(5)
		}

		fmt.Println("Agent 2:", prompt)
	}
}
```

This conversation was quite interesting. But endless. You can stop it by pressing Ctrl+C.