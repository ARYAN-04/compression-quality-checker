package runner

import (
	"encoding/json"
	"fmt"

	"can-i-compress-it/internal/domain"
	"can-i-compress-it/internal/inference"
)

// expectedCalls returns how many underlying invocations the task needs.
func expectedCalls(task domain.Task) int {
	if len(task.MultiHop) > 0 {
		return len(task.MultiHop)
	}
	return 1
}

// assistantToolCall re-renders a call as an assistant message.
func assistantToolCall(call domain.ToolCall) inference.Message {
	args, err := json.Marshal(call.Arguments)
	if err != nil {
		args = []byte("{}")
	}
	id := call.ID
	if id == "" {
		id = fmt.Sprintf("call_%s", call.Name)
	}
	return inference.AssistantToolCall(call.Name, string(args), id)
}

// toolResultMessage feeds a proxy result back into the conversation.
func toolResultMessage(id string, result any, err error) inference.Message {
	content := "{}"
	if err != nil {
		content = fmt.Sprintf(`{"error": %q}`, err.Error())
	} else if data, mErr := json.Marshal(result); mErr == nil {
		content = string(data)
	}
	return inference.Message{Role: "tool", Content: content, ToolCallID: id}
}
