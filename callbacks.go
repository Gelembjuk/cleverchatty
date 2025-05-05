package cleverchatty

type uiCallbacks struct {
	// Is called when a prompt processing started. It could be used to display a user's prompt in a progressing state.
	startedPromptProcessing func(prompt string) error
	// Request to LLM started
	startedThinking func() error
	// Final response reveived after all equests to LLM and Tools
	responseReceived func(response string) error
	// Notification received on background and assistant generated the response
	notificationProcessed func(response string) error
	// Tool is called
	toolCalling func(tool string) error
	// Tool call failed. After this the empty response is reported
	// NOTE. This can be changed later to have something more intelligent here
	toolCallFailed func(tool string, err error) error
}

// SetStartedPromptProcessing sets the callback function to be called when a prompt processing starts
func (c *uiCallbacks) SetStartedPromptProcessing(f func(prompt string) error) {
	c.startedPromptProcessing = f
}

// call startedPromptProcessing if it is set
func (c *uiCallbacks) callStartedPromptProcessing(prompt string) error {
	if c.startedPromptProcessing != nil {
		return c.startedPromptProcessing(prompt)
	}
	return nil
}

// SetStartedThinking sets the callback function to be called when a prompt processing starts
func (c *uiCallbacks) SetStartedThinking(f func() error) {
	c.startedThinking = f
}

// call startedThinking if it is set
func (c *uiCallbacks) callStartedThinking() error {
	if c.startedThinking != nil {
		return c.startedThinking()
	}
	return nil
}

// SetResponseReceived sets the callback function to be called when a response is received
func (c *uiCallbacks) SetResponseReceived(f func(response string) error) {
	c.responseReceived = f
}

// call responseReceived if it is set
func (c *uiCallbacks) callResponseReceived(response string) error {
	if c.responseReceived != nil {
		return c.responseReceived(response)
	}
	return nil
}

// SetNotificationProcesses sets the callback function to be called when a notification is received
func (c *uiCallbacks) SetNotificationProcessed(f func(response string) error) {
	c.notificationProcessed = f
}

// call notificationProcesses if it is set
func (c *uiCallbacks) callNotificationProcessed(response string) error {
	if c.notificationProcessed != nil {
		return c.notificationProcessed(response)
	}
	return nil
}

// SetToolCalling sets the callback function to be called when a tool is called
func (c *uiCallbacks) SetToolCalling(f func(tool string) error) {
	c.toolCalling = f
}

// call toolCalling if it is set
func (c *uiCallbacks) callToolCalling(tool string) error {
	if c.toolCalling != nil {
		return c.toolCalling(tool)
	}
	return nil
}

// SetToolCallFailed sets the callback function to be called when a tool call fails
func (c *uiCallbacks) SetToolCallFailed(f func(tool string, err error) error) {
	c.toolCallFailed = f
}

// call toolCallFailed if it is set
func (c *uiCallbacks) callToolCallFailed(tool string, err error) error {
	if c.toolCallFailed != nil {
		return c.toolCallFailed(tool, err)
	}
	return nil
}
