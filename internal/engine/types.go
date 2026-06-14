// Package engine defines the model-agnostic provider interface used by the
// Lore agent harness. A Provider streams one assistant turn at a time; the
// turn may contain plain text and/or structured tool calls. The harness
// executes tool calls and feeds results back as tool_result blocks, so the
// same loop works against any backend that supports native tool calling.
package engine

import (
	"context"
	"encoding/json"
	"strings"
)

// Block is one content block inside a message. Exactly one of the block
// kinds is populated, selected by Type.
type Block struct {
	Type string // "text" | "tool_use" | "tool_result"

	// text
	Text string

	// tool_use
	ID    string
	Name  string
	Input json.RawMessage

	// tool_result
	ToolUseID string
	Content   string
	IsError   bool
}

// TextBlock builds a text block.
func TextBlock(s string) Block { return Block{Type: "text", Text: s} }

// ToolResultBlock builds a tool_result block answering tool call id.
func ToolResultBlock(id, content string, isErr bool) Block {
	return Block{Type: "tool_result", ToolUseID: id, Content: content, IsError: isErr}
}

// Message is a single conversation turn.
type Message struct {
	Role   string // "user" | "assistant"
	Blocks []Block
}

// UserText builds a user message containing a single text block.
func UserText(s string) Message {
	return Message{Role: "user", Blocks: []Block{TextBlock(s)}}
}

// Text returns the concatenated text blocks of the message.
func (m Message) Text() string {
	var out strings.Builder
	for _, b := range m.Blocks {
		if b.Type == "text" {
			out.WriteString(b.Text)
		}
	}
	return out.String()
}

// ToolCalls returns the tool_use blocks of the message, in order.
func (m Message) ToolCalls() []Block {
	var calls []Block
	for _, b := range m.Blocks {
		if b.Type == "tool_use" {
			calls = append(calls, b)
		}
	}
	return calls
}

// ToolDef describes one tool offered to the model.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema for the input object
}

// Usage holds token accounting for one request.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// Total returns all tokens billed for the request.
func (u Usage) Total() int {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

// Add accumulates another usage record into u.
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
}

// Request is one model invocation: full history plus tool definitions.
type Request struct {
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
}

// Event is one item from a streaming response.
type Event struct {
	Type string // "text" | "tool_start" | "tool_delta" | "retry" | "done" | "error"

	Text     string // text: incremental text delta
	ToolID   string // tool_start / tool_delta
	ToolName string // tool_start
	Delta    string // tool_delta: partial JSON input

	// done
	Message    *Message
	Usage      Usage
	StopReason string // "end_turn" | "tool_use" | "max_tokens" | ...

	// retry
	Attempt int

	// error
	Err error
}

// Provider streams one assistant turn for a request. The returned channel
// delivers zero or more text/tool events followed by exactly one "done" or
// "error" event, after which it is closed.
type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request) <-chan Event
}
