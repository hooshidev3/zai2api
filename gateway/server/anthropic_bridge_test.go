// Package server — Tests for the Anthropic ↔ OpenAI translation bridge.
//
// These tests cover three layers:
//   1. Unit tests for request/response translation functions
//      (anthropicToOpenAI, openAIToAnthropicResponse) — pure logic,
//      no I/O, no sub-engine required.
//   2. Integration test with a mock MiMo sub-engine that emits
//      OpenAI-format SSE — verifies the streaming translation end-to-end
//      including the buffer logic for split chunks.
//   3. Non-streaming integration test with the same mock.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ── Layer 1: Unit tests for request translation (Anthropic → OpenAI) ──

func TestAnthropicToOpenAI_SimpleText(t *testing.T) {
	anthropic := `{
		"model": "mimo-v2.5-pro",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hello"}]
	}`

	openai, err := anthropicToOpenAI([]byte(anthropic))
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}

	var req OpenAIRequest
	if err := json.Unmarshal(openai, &req); err != nil {
		t.Fatalf("invalid OpenAI output: %v", err)
	}

	if req.Model != "mimo-v2.5-pro" {
		t.Errorf("model mismatch: got %q", req.Model)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("max_tokens mismatch: got %d", req.MaxTokens)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Errorf("role mismatch: got %q", req.Messages[0].Role)
	}
}

func TestAnthropicToOpenAI_SystemPrompt(t *testing.T) {
	// system as a string — should become a message with role=system
	anthropic := `{
		"model": "mimo-v2.5-pro",
		"max_tokens": 100,
		"system": "You are helpful.",
		"messages": [{"role": "user", "content": "Hi"}]
	}`

	openai, _ := anthropicToOpenAI([]byte(anthropic))
	var req OpenAIRequest
	json.Unmarshal(openai, &req)

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message should be system, got %q", req.Messages[0].Role)
	}
}

func TestAnthropicToOpenAI_StreamFlag(t *testing.T) {
	anthropic := `{
		"model": "mimo-v2.5-pro",
		"max_tokens": 100,
		"stream": true,
		"messages": [{"role": "user", "content": "Hi"}]
	}`

	openai, _ := anthropicToOpenAI([]byte(anthropic))
	var req OpenAIRequest
	json.Unmarshal(openai, &req)

	if !req.Stream {
		t.Error("stream flag not propagated to OpenAI request")
	}
}

func TestAnthropicToOpenAI_Tools(t *testing.T) {
	anthropic := `{
		"model": "mimo-v2.5-pro",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "What is the weather?"}],
		"tools": [{
			"name": "get_weather",
			"description": "Get weather",
			"input_schema": {"type": "object", "properties": {"city": {"type": "string"}}}
		}]
	}`

	openai, _ := anthropicToOpenAI([]byte(anthropic))
	var req OpenAIRequest
	json.Unmarshal(openai, &req)

	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Type != "function" {
		t.Errorf("tool type should be 'function', got %q", req.Tools[0].Type)
	}
	if req.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tool name mismatch: got %q", req.Tools[0].Function.Name)
	}
}

// ── Layer 1: Unit tests for response translation (OpenAI → Anthropic) ──

func TestOpenAIToAnthropic_SimpleText(t *testing.T) {
	openai := `{
		"id": "chatcmpl-abc123",
		"choices": [{
			"message": {"role": "assistant", "content": "Hello there"},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`

	anthropic, err := openAIToAnthropicResponse([]byte(openai), "mimo-v2.5-pro")
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}

	var resp map[string]any
	json.Unmarshal(anthropic, &resp)

	if resp["type"] != "message" {
		t.Errorf("type should be 'message', got %v", resp["type"])
	}
	if resp["role"] != "assistant" {
		t.Errorf("role should be 'assistant', got %v", resp["role"])
	}
	if resp["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason should be 'end_turn', got %v", resp["stop_reason"])
	}

	content := resp["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" {
		t.Errorf("content block type should be 'text', got %v", block["type"])
	}
	if block["text"] != "Hello there" {
		t.Errorf("text mismatch: got %v", block["text"])
	}

	usage := resp["usage"].(map[string]any)
	if usage["input_tokens"] != float64(10) {
		t.Errorf("input_tokens mismatch: got %v", usage["input_tokens"])
	}
	if usage["output_tokens"] != float64(5) {
		t.Errorf("output_tokens mismatch: got %v", usage["output_tokens"])
	}
}

func TestOpenAIToAnthropic_StopReasons(t *testing.T) {
	tests := []struct {
		finishReason string
		expected     string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
	}

	for _, tt := range tests {
		openai := `{
			"id": "chatcmpl-x",
			"choices": [{"message": {"content": "hi"}, "finish_reason": "` + tt.finishReason + `"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1}
		}`
		anthropic, _ := openAIToAnthropicResponse([]byte(openai), "mimo")
		var resp map[string]any
		json.Unmarshal(anthropic, &resp)
		if resp["stop_reason"] != tt.expected {
			t.Errorf("finish_reason %q → expected stop_reason %q, got %v",
				tt.finishReason, tt.expected, resp["stop_reason"])
		}
	}
}

func TestOpenAIToAnthropic_ToolUse(t *testing.T) {
	openai := `{
		"id": "chatcmpl-tool",
		"choices": [{
			"message": {
				"content": "",
				"tool_calls": [{
					"id": "call_123",
					"function": {"name": "get_weather", "arguments": "{\"city\":\"Tehran\"}"}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`

	anthropic, _ := openAIToAnthropicResponse([]byte(openai), "mimo")
	var resp map[string]any
	json.Unmarshal(anthropic, &resp)

	if resp["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason should be 'tool_use', got %v", resp["stop_reason"])
	}

	content := resp["content"].([]any)
	found := false
	for _, c := range content {
		block := c.(map[string]any)
		if block["type"] == "tool_use" {
			found = true
			if block["name"] != "get_weather" {
				t.Errorf("tool name mismatch: got %v", block["name"])
			}
		}
	}
	if !found {
		t.Error("no tool_use content block found")
	}
}

// ── Layer 2: Mock MiMo sub-engine ────────────────────────────────────

// newMockMiMoEngine builds a gin engine that mimics the MiMo sub-engine's
// OpenAI-format responses (both streaming and non-streaming). This lets us
// test the translation bridge end-to-end without real MiMo credentials.
func newMockMiMoEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		var req struct {
			Stream bool `json:"stream"`
		}
		json.Unmarshal(body, &req)

		if !req.Stream {
			c.JSON(http.StatusOK, gin.H{
				"id":     "chatcmpl-mock",
				"object": "chat.completion",
				"choices": []gin.H{{
					"index":         0,
					"message":       gin.H{"role": "assistant", "content": "Hello from MiMo"},
					"finish_reason": "stop",
				}},
				"usage": gin.H{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
			})
			return
		}

		// streaming: emit OpenAI-format SSE chunks (same shape as real MiMo)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")

		chunks := []string{"Hello", " from", " MiMo", " streaming"}
		for _, chunk := range chunks {
			data := map[string]any{
				"id":     "chatcmpl-mock",
				"object": "chat.completion.chunk",
				"choices": []map[string]any{{
					"index":         0,
					"delta":         map[string]any{"content": chunk},
					"finish_reason": nil,
				}},
			}
			b, _ := json.Marshal(data)
			fmt.Fprintf(c.Writer, "data: %s\n\n", b)
			c.Writer.Flush()
		}

		// final chunk with finish_reason
		final := map[string]any{
			"id":     "chatcmpl-mock",
			"object": "chat.completion.chunk",
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			}},
		}
		fb, _ := json.Marshal(final)
		fmt.Fprintf(c.Writer, "data: %s\n\n", fb)
		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	})
	return r
}

// ── Layer 2: Integration tests (streaming + non-streaming) ──────────

// TestAnthropicStreamingTranslation exercises the full streaming pipeline:
// Anthropic request → translate to OpenAI → mock MiMo (SSE) →
// translate back to Anthropic SSE → client. Verifies that all 6 Anthropic
// events are emitted and that no OpenAI-format chunks leak through.
func TestAnthropicStreamingTranslation(t *testing.T) {
	s := &Server{
		mimoEngine: newMockMiMoEngine(),
		aliases:    map[string]string{},
	}

	anthropicReq := `{
		"model": "mimo-v2.5-pro",
		"max_tokens": 1024,
		"stream": true,
		"messages": [{"role": "user", "content": "Hello"}]
	}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(anthropicReq))
	c.Request.Header.Set("Content-Type", "application/json")

	s.handleAnthropicMessages(c)

	output := w.Body.String()

	// All 6 Anthropic events must be present
	requiredEvents := []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	}
	for _, ev := range requiredEvents {
		if !strings.Contains(output, ev) {
			t.Errorf("missing required Anthropic event: %q", ev)
		}
	}

	// All translated content words must be present
	for _, word := range []string{"Hello", "from", "MiMo", "streaming"} {
		if !strings.Contains(output, word) {
			t.Errorf("missing translated content: %q", word)
		}
	}

	// text_delta type must appear in content_block_delta events
	if !strings.Contains(output, "text_delta") {
		t.Error("missing text_delta in output")
	}

	// No OpenAI-format chunks should leak through
	if strings.Contains(output, "chat.completion.chunk") {
		t.Error("OpenAI format leaked into Anthropic output — translation failed")
	}
	if strings.Contains(output, `"object":"chat.completion"`) {
		t.Error("OpenAI chunk object leaked")
	}

	t.Logf("=== Translated Anthropic SSE output ===\n%s", output)
}

// TestAnthropicNonStreamingTranslation exercises the non-streaming path:
// Anthropic request → translate to OpenAI → mock MiMo (JSON) →
// translate back to Anthropic JSON → client.
func TestAnthropicNonStreamingTranslation(t *testing.T) {
	s := &Server{
		mimoEngine: newMockMiMoEngine(),
		aliases:    map[string]string{},
	}

	anthropicReq := `{
		"model": "mimo-v2.5-pro",
		"max_tokens": 1024,
		"stream": false,
		"messages": [{"role": "user", "content": "Hello"}]
	}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(anthropicReq))
	c.Request.Header.Set("Content-Type", "application/json")

	s.handleAnthropicMessages(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON (should be Anthropic format): %v\n%s", err, w.Body.String())
	}

	if resp["type"] != "message" {
		t.Errorf("expected Anthropic 'message' type, got %v", resp["type"])
	}
	if !strings.Contains(w.Body.String(), "Hello from MiMo") {
		t.Error("missing translated content")
	}
}

// ── Layer 3: randomID unit test ──────────────────────────────────────

// TestRandomID verifies that randomID produces a 24-char hex string and
// that two consecutive calls produce different values (non-deterministic).
func TestRandomID(t *testing.T) {
	id1 := randomID()
	id2 := randomID()

	if len(id1) != 24 {
		t.Errorf("randomID length = %d, want 24 (12 bytes hex-encoded)", len(id1))
	}
	if id1 == id2 {
		t.Error("two consecutive randomID calls returned the same value — not random")
	}
	// Must be valid hex
	for _, c := range id1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("randomID contains non-hex character %q", c)
		}
	}
}
