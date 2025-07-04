package core

var (
	CallbackCodePromptProcessing = "prompt_accepted"
	CallbackCodeStartedThinking  = "thinking"
	CallbackCodeResponseReceived = "response_received"
	CallbackCodeToolCalling      = "tool_calling"
	CallbackCodeToolCallFailed   = "tool_error"
	CallbackCodeMemoryRetrieval  = "memory_retrieval"
	CallbackCodeRAGRetrieval     = "rag_retrieval"
)

type UICallbacks struct {
	// Is called when a prompt processing started. It could be used to display a user's prompt in a progressing state.
	startedPromptProcessing func(prompt string) error
	// Request to LLM started
	startedThinking func() error
	// Final response reveived after all equests to LLM and Tools
	responseReceived func(response string) error
	// Tool is called
	toolCalling func(tool string) error
	// Tool call failed. After this the empty response is reported
	// NOTE. This can be changed later to have something more intelligent here
	toolCallFailed func(tool string, err error) error
	// request to the memory server started
	memoryRetrievalStarted func() error
	// request to the RAG server started
	ragRetrievalStarted func() error
}

// SetStartedPromptProcessing sets the callback function to be called when a prompt processing starts
func (c *UICallbacks) SetStartedPromptProcessing(f func(prompt string) error) {
	c.startedPromptProcessing = f
}

// call startedPromptProcessing if it is set
func (c *UICallbacks) CallStartedPromptProcessing(prompt string) error {
	if c.startedPromptProcessing != nil {
		return c.startedPromptProcessing(prompt)
	}
	return nil
}

// SetStartedThinking sets the callback function to be called when a prompt processing starts
func (c *UICallbacks) SetStartedThinking(f func() error) {
	c.startedThinking = f
}

// call startedThinking if it is set
func (c *UICallbacks) CallStartedThinking() error {
	if c.startedThinking != nil {
		return c.startedThinking()
	}
	return nil
}

// SetResponseReceived sets the callback function to be called when a response is received
func (c *UICallbacks) SetResponseReceived(f func(response string) error) {
	c.responseReceived = f
}

// call responseReceived if it is set
func (c *UICallbacks) CallResponseReceived(response string) error {
	if c.responseReceived != nil {
		return c.responseReceived(response)
	}
	return nil
}

// SetToolCalling sets the callback function to be called when a tool is called
func (c *UICallbacks) SetToolCalling(f func(tool string) error) {
	c.toolCalling = f
}

// call toolCalling if it is set
func (c *UICallbacks) CallToolCalling(tool string) error {
	if c.toolCalling != nil {
		return c.toolCalling(tool)
	}
	return nil
}

// SetToolCallFailed sets the callback function to be called when a tool call fails
func (c *UICallbacks) SetToolCallFailed(f func(tool string, err error) error) {
	c.toolCallFailed = f
}

// call toolCallFailed if it is set
func (c *UICallbacks) CallToolCallFailed(tool string, err error) error {
	if c.toolCallFailed != nil {
		return c.toolCallFailed(tool, err)
	}
	return nil
}

// SetMemoryRetrievalStarted sets the callback function to be called when a memory retrieval starts
func (c *UICallbacks) SetMemoryRetrievalStarted(f func() error) {
	c.memoryRetrievalStarted = f
}

// call memoryRetrievalStarted if it is set
func (c *UICallbacks) CallMemoryRetrievalStarted() error {
	if c.memoryRetrievalStarted != nil {
		return c.memoryRetrievalStarted()
	}
	return nil
}

// SetRAGRetrievalStarted sets the callback function to be called when a RAG retrieval starts
func (c *UICallbacks) SetRAGRetrievalStarted(f func() error) {
	c.ragRetrievalStarted = f
}

// call ragRetrievalStarted if it is set
func (c *UICallbacks) CallRAGRetrievalStarted() error {
	if c.ragRetrievalStarted != nil {
		return c.ragRetrievalStarted()
	}
	return nil
}
