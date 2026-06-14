package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// AnthropicProvider talks directly to the Anthropic Messages API using the
// user's own API key. The API streams structured events with native tool-use
// blocks; this provider marshals requests and decodes the event stream.
type AnthropicProvider struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// DefaultAnthropicURL is the Anthropic API endpoint used when no base URL
// override is configured.
const DefaultAnthropicURL = "https://api.anthropic.com"

// NewAnthropicProvider returns a provider for the Anthropic API. baseURL may
// be empty to use the default endpoint.
func NewAnthropicProvider(baseURL, apiKey, model string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = DefaultAnthropicURL
	}
	return &AnthropicProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http: &http.Client{
			Timeout: 900 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   15 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 300 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic:" + p.model }

// ── wire format ───────────────────────────────────────────────────────────────

type wireBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// systemBlock carries the system prompt with a cache_control breakpoint so
// the (large, stable) system prompt and tool definitions are prompt-cached
// across the many turns of an agent run.
type systemBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

type anthropicRequest struct {
	Model     string        `json:"model"`
	System    []systemBlock `json:"system,omitempty"`
	Messages  []wireMessage `json:"messages"`
	Tools     []wireTool    `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
}

func (p *AnthropicProvider) toWire(req Request) anthropicRequest {
	out := anthropicRequest{Model: p.model, MaxTokens: req.MaxTokens, Stream: true}
	if out.MaxTokens <= 0 {
		out.MaxTokens = 16384
	}
	if req.System != "" {
		out.System = []systemBlock{{
			Type:         "text",
			Text:         req.System,
			CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
		}}
	}
	for _, m := range req.Messages {
		wm := wireMessage{Role: m.Role}
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				if b.Text == "" {
					continue
				}
				wm.Content = append(wm.Content, wireBlock{Type: "text", Text: b.Text})
			case "tool_use":
				in := b.Input
				if len(in) == 0 {
					in = json.RawMessage("{}")
				}
				wm.Content = append(wm.Content, wireBlock{Type: "tool_use", ID: b.ID, Name: b.Name, Input: in})
			case "tool_result":
				wm.Content = append(wm.Content, wireBlock{
					Type: "tool_result", ToolUseID: b.ToolUseID, Content: b.Content, IsError: b.IsError,
				})
			}
		}
		if len(wm.Content) == 0 {
			wm.Content = append(wm.Content, wireBlock{Type: "text", Text: "(empty)"})
		}
		out.Messages = append(out.Messages, wm)
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, wireTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out
}

// ── streaming ─────────────────────────────────────────────────────────────────

// Stream implements Provider. Connection-level failures before any content
// has been received are retried with backoff; mid-stream failures surface
// as an error event so the caller can retry the whole turn.
func (p *AnthropicProvider) Stream(ctx context.Context, req Request) <-chan Event {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)

		body, err := json.Marshal(p.toWire(req))
		if err != nil {
			ch <- Event{Type: "error", Err: fmt.Errorf("encoding request: %w", err)}
			return
		}

		const maxAttempts = 4
		backoff := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if attempt > 1 {
				ch <- Event{Type: "retry", Attempt: attempt}
				select {
				case <-time.After(backoff[min(attempt-2, len(backoff)-1)]):
				case <-ctx.Done():
					ch <- Event{Type: "error", Err: ctx.Err()}
					return
				}
			}

			res, err := p.post(ctx, body)
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

			emitted, err := p.consume(res.Body, ch)
			res.Body.Close()
			if err != nil {
				// Retry silently only when nothing has reached the caller yet.
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

func (p *AnthropicProvider) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	return p.http.Do(req)
}

// consume decodes the Anthropic streaming event protocol. Returns whether any
// event was delivered to the caller, and a terminal error if the stream broke
// before a normal end-of-message.
func (p *AnthropicProvider) consume(body io.Reader, ch chan<- Event) (emitted bool, err error) {
	var (
		msg        Message
		usage      Usage
		stopReason string
		curBlock   *Block
		jsonBuf    strings.Builder
		finished   bool
		streamErr  error
	)
	msg.Role = "assistant"

	closeBlock := func() {
		if curBlock == nil {
			return
		}
		if curBlock.Type == "tool_use" {
			raw := strings.TrimSpace(jsonBuf.String())
			if raw == "" {
				raw = "{}"
			}
			curBlock.Input = json.RawMessage(raw)
		}
		msg.Blocks = append(msg.Blocks, *curBlock)
		curBlock = nil
		jsonBuf.Reset()
	}

	readErr := readSSE(body, func(f sseFrame) bool {
		if f.data == "[DONE]" {
			finished = true
			return false
		}
		var ev struct {
			Type    string `json:"type"`
			Message *struct {
				Usage struct {
					InputTokens        int `json:"input_tokens"`
					CacheCreationInput int `json:"cache_creation_input_tokens"`
					CacheReadInput     int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			ContentBlock *struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
				Text string `json:"text"`
			} `json:"content_block"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Error *struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(f.data), &ev) != nil {
			return true
		}

		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				usage.InputTokens = ev.Message.Usage.InputTokens
				usage.CacheWriteTokens = ev.Message.Usage.CacheCreationInput
				usage.CacheReadTokens = ev.Message.Usage.CacheReadInput
			}
		case "content_block_start":
			closeBlock()
			if ev.ContentBlock != nil {
				switch ev.ContentBlock.Type {
				case "tool_use":
					curBlock = &Block{Type: "tool_use", ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}
					ch <- Event{Type: "tool_start", ToolID: ev.ContentBlock.ID, ToolName: ev.ContentBlock.Name}
					emitted = true
				default:
					curBlock = &Block{Type: "text", Text: ev.ContentBlock.Text}
				}
			}
		case "content_block_delta":
			if ev.Delta == nil || curBlock == nil {
				return true
			}
			switch ev.Delta.Type {
			case "text_delta":
				curBlock.Text += ev.Delta.Text
				ch <- Event{Type: "text", Text: ev.Delta.Text}
				emitted = true
			case "input_json_delta":
				jsonBuf.WriteString(ev.Delta.PartialJSON)
				ch <- Event{Type: "tool_delta", ToolID: curBlock.ID, Delta: ev.Delta.PartialJSON}
				emitted = true
			}
		case "content_block_stop":
			closeBlock()
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage != nil {
				usage.OutputTokens = ev.Usage.OutputTokens
				if ev.Usage.InputTokens > 0 {
					usage.InputTokens = ev.Usage.InputTokens
				}
			}
		case "message_stop":
			finished = true
			return false
		case "error":
			m := "engine stream error"
			if ev.Error != nil && ev.Error.Message != "" {
				m = ev.Error.Message
			}
			streamErr = errors.New(m)
			return false
		}
		return true
	})

	if streamErr != nil {
		return emitted, streamErr
	}
	if readErr != nil {
		return emitted, fmt.Errorf("reading stream: %w", readErr)
	}
	if !finished {
		return emitted, errors.New("stream ended unexpectedly")
	}

	closeBlock()
	ch <- Event{Type: "done", Message: &msg, Usage: usage, StopReason: stopReason}
	return true, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func retryable(err error, status int) bool {
	if status == 429 || status == 529 || (status >= 500 && status <= 599) {
		return true
	}
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "Client.Timeout") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "EOF") ||
		strings.Contains(s, "i/o timeout")
}

func readErrorBody(res *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	// Flat form: {"error":"..."} (OpenAI-compatible servers vary).
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return e.Error
	}
	// Nested form: {"error":{"type":...,"message":...}} (Anthropic, OpenAI).
	var ne struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &ne) == nil && ne.Error.Message != "" {
		return ne.Error.Message
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		s = res.Status
	}
	if len(s) > 400 {
		s = s[:400]
	}
	return s
}
