package together

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

// NormalizeResponse lifts inline `<tool_call>{...}</tool_call>` blocks
// emitted in the assistant message content into the standard OpenAI
// `tool_calls` array. Locally-hosted finetunes (Trinity, Mouse-in-progress,
// custom finetunes via LM Studio) often emit tool calls inline as text
// instead of using the native field. This makes the response shape
// consistent regardless of backend so callers don't need to special-case.
//
// No-op when:
//   - the response already has native ToolCalls (don't double-extract)
//   - no `<tool_call>` blocks are present in content
//   - the inline JSON is malformed
func NormalizeResponse(resp *ChatResponse) {
	if resp == nil {
		return
	}
	for i := range resp.Choices {
		ch := &resp.Choices[i]
		if len(ch.Message.ToolCalls) > 0 {
			continue
		}
		content, calls := ExtractInlineToolCalls(ch.Message.Content)
		if len(calls) == 0 {
			continue
		}
		ch.Message.Content = content
		ch.Message.ToolCalls = calls
		if ch.FinishReason == "stop" || ch.FinishReason == "" {
			ch.FinishReason = "tool_calls"
		}
	}
}

// inlineToolCallRE matches `<tool_call>...</tool_call>` with the JSON
// body inside. Case-insensitive, multiline-friendly. Accepts whitespace
// or newlines between the tags and the JSON.
var inlineToolCallRE = regexp.MustCompile(`(?is)<tool_call>\s*(\{.*?\})\s*</tool_call>`)

// inlineToolBody is what we expect inside the tags — name + args (or
// parameters, some finetunes use that).
type inlineToolBody struct {
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

// ExtractInlineToolCalls finds `<tool_call>...</tool_call>` blocks in
// the given content string, parses each as JSON, returns the content
// with the blocks removed plus the parsed tool calls. Malformed blocks
// are kept inline (we don't silently drop them — the user sees the
// model's output instead of a mystery deletion).
func ExtractInlineToolCalls(content string) (string, []ToolCall) {
	matches := inlineToolCallRE.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}
	var calls []ToolCall
	var b strings.Builder
	last := 0
	for _, m := range matches {
		// m[0]..m[1] is the full <tool_call>...</tool_call>; m[2]..m[3] is the JSON.
		jsonBody := content[m[2]:m[3]]
		var body inlineToolBody
		if err := json.Unmarshal([]byte(jsonBody), &body); err != nil || body.Name == "" {
			// Malformed — emit verbatim, don't extract.
			b.WriteString(content[last:m[1]])
			last = m[1]
			continue
		}
		args := body.Arguments
		if len(args) == 0 {
			args = body.Parameters
		}
		if len(args) == 0 {
			args = []byte("{}")
		}
		calls = append(calls, ToolCall{
			ID:   "call_" + shortID(),
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      body.Name,
				Arguments: string(args),
			},
		})
		// Drop the block from the output, preserving the prefix.
		b.WriteString(content[last:m[0]])
		last = m[1]
	}
	b.WriteString(content[last:])
	cleaned := strings.TrimSpace(b.String())
	return cleaned, calls
}

func shortID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
