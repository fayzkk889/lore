package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// CompatProvider talks to any endpoint that implements the industry-standard
// chat-completions wire format with function calling (hosted services or a
// local runtime such as Ollama). It lets Lore run against user-supplied
// models: weaker models may produce lower-quality code, but the harness
// still drives them to a complete, runnable result.
type CompatProvider struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewCompatProvider returns a provider for baseURL (e.g. a local runtime at
// http://localhost:11434/v1) using the given model id. name labels the
// provider in the UI (e.g. "openrouter", "ollama"); apiKey may be empty for
// local endpoints.
func NewCompatProvider(name, baseURL, apiKey, model string) *CompatProvider {
	if name == "" {
		name = "custom"
	}
	return &CompatProvider{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http: &http.Client{
			Timeout: 1800 * time.Second, // local models can be very slow
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   15 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ResponseHeaderTimeout: 600 * time.Second,
			},
		},
	}
}

func (p *CompatProvider) Name() string { return p.name + ":" + p.model }

// ── wire format ───────────────────────────────────────────────────────────────

type oaToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

func (p *CompatProvider) toWire(req Request) ([]byte, error) {
	var msgs []oaMessage
	if req.System != "" {
		msgs = append(msgs, oaMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			om := oaMessage{Role: "assistant", Content: m.Text()}
			for _, b := range m.ToolCalls() {
				var tc oaToolCall
				tc.ID = b.ID
				tc.Type = "function"
				tc.Function.Name = b.Name
				tc.Function.Arguments = string(b.Input)
				if tc.Function.Arguments == "" {
					tc.Function.Arguments = "{}"
				}
				om.ToolCalls = append(om.ToolCalls, tc)
			}
			msgs = append(msgs, om)
		default: // user turns: split tool results into individual "tool" messages
			var text string
			for _, b := range m.Blocks {
				switch b.Type {
				case "tool_result":
					content := b.Content
					if b.IsError {
						content = "ERROR: " + content
					}
					msgs = append(msgs, oaMessage{Role: "tool", Content: content, ToolCallID: b.ToolUseID})
				case "text":
					text += b.Text
				}
			}
			if text != "" {
				msgs = append(msgs, oaMessage{Role: "user", Content: text})
			}
		}
	}

	type oaFunction struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}
	type oaTool struct {
		Type     string     `json:"type"`
		Function oaFunction `json:"function"`
	}
	var tools []oaTool
	for _, t := range req.Tools {
		tools = append(tools, oaTool{Type: "function", Function: oaFunction{
			Name: t.Name, Description: t.Description, Parameters: t.InputSchema,
		}})
	}

	body := map[string]any{
		"model":    p.model,
		"messages": msgs,
		"stream":   true,
		"stream_options": map[string]bool{
			"include_usage": true,
		},
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	return json.Marshal(body)
}

// ── streaming ─────────────────────────────────────────────────────────────────

// Stream implements Provider.
func (p *CompatProvider) Stream(ctx context.Context, req Request) <-chan Event {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)

		body, err := p.toWire(req)
		if err != nil {
			ch <- Event{Type: "error", Err: fmt.Errorf("encoding request: %w", err)}
			return
		}

		const maxAttempts = 3
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if attempt > 1 {
				ch <- Event{Type: "retry", Attempt: attempt}
				select {
				case <-time.After(3 * time.Second):
				case <-ctx.Done():
					ch <- Event{Type: "error", Err: ctx.Err()}
					return
				}
			}

			hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
			if err != nil {
				ch <- Event{Type: "error", Err: err}
				return
			}
			hreq.Header.Set("Content-Type", "application/json")
			if p.apiKey != "" {
				hreq.Header.Set("Authorization", "Bearer "+p.apiKey)
			}

			res, err := p.http.Do(hreq)
			if err != nil {
				if retryable(err, 0) && attempt < maxAttempts {
					continue
				}
				ch <- Event{Type: "error", Err: err}
				return
			}
			if res.StatusCode != http.StatusOK {
				msg := readErrorBody(res)
				res.Body.Close()
				if retryable(nil, res.StatusCode) && attempt < maxAttempts {
					continue
				}
				ch <- Event{Type: "error", Err: fmt.Errorf("engine error (HTTP %d): %s", res.StatusCode, msg)}
				return
			}

			emitted, err := p.consume(res, ch)
			res.Body.Close()
			if err != nil {
				if !emitted && attempt < maxAttempts {
					continue
				}
				ch <- Event{Type: "error", Err: err}
			}
			return
		}
	}()
	return ch
}

func (p *CompatProvider) consume(res *http.Response, ch chan<- Event) (emitted bool, err error) {
	var (
		text       strings.Builder
		usage      Usage
		stopReason string
		gotDone    bool
	)
	// tool calls accumulate by index: streamed argument fragments append.
	type pendingCall struct {
		id, name, args string
		started        bool
	}
	var calls []*pendingCall

	callAt := func(idx int) *pendingCall {
		for len(calls) <= idx {
			calls = append(calls, &pendingCall{})
		}
		return calls[idx]
	}

	readErr := readSSE(res.Body, func(f sseFrame) bool {
		if f.data == "[DONE]" {
			gotDone = true
			return false
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string       `json:"content"`
					ToolCalls []oaToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(f.data), &chunk) != nil {
			return true
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			err = errors.New(chunk.Error.Message)
			return false
		}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			return true
		}
		c := chunk.Choices[0]
		if c.Delta.Content != "" {
			text.WriteString(c.Delta.Content)
			ch <- Event{Type: "text", Text: c.Delta.Content}
			emitted = true
		}
		for i, tc := range c.Delta.ToolCalls {
			idx := i
			if tc.Index != nil {
				idx = *tc.Index
			}
			pc := callAt(idx)
			if tc.ID != "" {
				pc.id = tc.ID
			}
			if tc.Function.Name != "" {
				pc.name = tc.Function.Name
			}
			if !pc.started && pc.name != "" {
				pc.started = true
				ch <- Event{Type: "tool_start", ToolID: pc.id, ToolName: pc.name}
				emitted = true
			}
			if tc.Function.Arguments != "" {
				pc.args += tc.Function.Arguments
				ch <- Event{Type: "tool_delta", ToolID: pc.id, Delta: tc.Function.Arguments}
			}
		}
		if c.FinishReason != "" {
			stopReason = c.FinishReason
		}
		return true
	})

	if err != nil {
		return emitted, err
	}
	if readErr != nil {
		return emitted, fmt.Errorf("reading stream: %w", readErr)
	}
	_ = gotDone // some servers close without [DONE]; treat EOF as completion

	msg := &Message{Role: "assistant"}
	if t := text.String(); t != "" {
		msg.Blocks = append(msg.Blocks, TextBlock(t))
	}
	for i, pc := range calls {
		if pc.name == "" {
			continue
		}
		id := pc.id
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		args := strings.TrimSpace(pc.args)
		if args == "" {
			args = "{}"
		}
		msg.Blocks = append(msg.Blocks, Block{
			Type: "tool_use", ID: id, Name: pc.name, Input: json.RawMessage(args),
		})
	}

	// Normalise stop reason to the structured-tool-use vocabulary.
	switch stopReason {
	case "tool_calls", "function_call":
		stopReason = "tool_use"
	case "stop", "":
		stopReason = "end_turn"
	case "length":
		stopReason = "max_tokens"
	}
	if len(msg.ToolCalls()) > 0 {
		stopReason = "tool_use"
	}

	ch <- Event{Type: "done", Message: msg, Usage: usage, StopReason: stopReason}
	return true, nil
}
