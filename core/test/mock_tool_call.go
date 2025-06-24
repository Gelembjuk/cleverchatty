package test

type MockToolCall struct {
	Name      string
	Arguments map[string]interface{}
	ID        string
}

func (m MockToolCall) GetName() string {
	return m.Name
}
func (m MockToolCall) GetArguments() map[string]interface{} {
	return m.Arguments
}
func (m MockToolCall) GetID() string {
	return m.ID
}
