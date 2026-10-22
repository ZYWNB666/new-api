package metrics

import "github.com/tidwall/gjson"

// HasGenerationContent identifies generated text, reasoning and tool arguments.
// Role-only chunks, usage, terminal snapshots and control events are excluded.
// It never counts SSE chunks as tokens or retains their contents.
func HasGenerationContent(data string) bool {
	if !gjson.Valid(data) {
		return false
	}
	root := gjson.Parse(data)
	if !root.IsObject() {
		return false
	}
	switch root.Get("type").String() {
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta", "response.function_call_arguments.delta":
		return root.Get("delta").Type == gjson.String && root.Get("delta").String() != ""
	case "content_block_delta":
		for _, key := range []string{"delta.text", "delta.thinking", "delta.partial_json"} {
			if value := root.Get(key); value.Type == gjson.String && value.String() != "" {
				return true
			}
		}
		return false
	case "content_block_start":
		return root.Get("content_block.text").String() != "" || root.Get("content_block.thinking").String() != ""
	}
	if root.Get("type").Exists() && root.Get("type").String() != "" {
		return false
	}
	for _, choice := range root.Get("choices").Array() {
		for _, key := range []string{"delta.content", "delta.reasoning_content", "delta.reasoning", "delta.refusal", "delta.function_call.arguments", "text"} {
			if value := choice.Get(key); value.Type == gjson.String && value.String() != "" {
				return true
			}
		}
		for _, tool := range choice.Get("delta.tool_calls").Array() {
			if tool.Get("function.arguments").String() != "" {
				return true
			}
		}
	}
	for _, candidate := range root.Get("candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			if part.Get("text").String() != "" || part.Get("functionCall.name").String() != "" {
				return true
			}
		}
	}
	// Native Ollama NDJSON, Baidu, Dify and Cohere streaming formats.
	if root.Get("done").Exists() && !root.Get("done").Bool() {
		return root.Get("message.content").String() != "" || root.Get("message.thinking").String() != "" || root.Get("response").String() != "" || len(root.Get("message.tool_calls").Array()) > 0
	}
	if root.Get("sentence_id").Exists() && root.Get("result").String() != "" {
		return true
	}
	if event := root.Get("event").String(); event == "message" || event == "agent_message" {
		return root.Get("answer").String() != ""
	}
	if root.Get("event_type").String() == "text-generation" {
		return root.Get("text").String() != ""
	}
	for _, choice := range root.Get("Choices").Array() {
		if choice.Get("Delta.Content").String() != "" || choice.Get("Delta.ReasoningContent").String() != "" {
			return true
		}
	}
	for _, text := range root.Get("payload.choices.text").Array() {
		if text.Get("content").String() != "" {
			return true
		}
	}
	return false
}
