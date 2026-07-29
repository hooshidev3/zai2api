// Package server — Anthropic-to-OpenAI bridge for MiMo models.
//
// When a client sends POST /v1/messages (Anthropic format) with a MiMo model,
// the gateway translates the request to OpenAI format, forwards it to the MiMo
// sub-engine, and translates the response back to Anthropic format.
//
// GLM models have a native Anthropic handler and are forwarded directly.
package server

import (
        "bytes"
        "crypto/rand"
        "encoding/hex"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "strings"
        "time"

        "github.com/gin-gonic/gin"
        "mimoproxy/pkg/authctx"
)

// ── Anthropic request structures ───────────────────────────────────

type AnthropicRequest struct {
        Model       string             `json:"model"`
        Messages    []AnthropicMessage `json:"messages"`
        System      any                `json:"system,omitempty"`
        MaxTokens   int                `json:"max_tokens"`
        Temperature *float64           `json:"temperature,omitempty"`
        TopP        *float64           `json:"top_p,omitempty"`
        Stream      bool               `json:"stream,omitempty"`
        StopSeqs    []string           `json:"stop_sequences,omitempty"`
        Tools       []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
        Role    string `json:"role"`
        Content any    `json:"content"` // string or []ContentBlock
}

type AnthropicTool struct {
        Name        string `json:"name"`
        Description string `json:"description,omitempty"`
        InputSchema any    `json:"input_schema"`
}

// ── OpenAI request structures (translation target) ─────────────────

type OpenAIRequest struct {
        Model       string          `json:"model"`
        Messages    []OpenAIMessage `json:"messages"`
        MaxTokens   int             `json:"max_tokens,omitempty"`
        Temperature *float64        `json:"temperature,omitempty"`
        TopP        *float64        `json:"top_p,omitempty"`
        Stream      bool            `json:"stream,omitempty"`
        Stop        []string        `json:"stop,omitempty"`
        Tools       []OpenAITool    `json:"tools,omitempty"`
}

type OpenAIMessage struct {
        Role       string `json:"role"`
        Content    any    `json:"content"`
        ToolCalls  []any  `json:"tool_calls,omitempty"`
        ToolCallID string `json:"tool_call_id,omitempty"`
}

type OpenAITool struct {
        Type     string         `json:"type"`
        Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
        Name        string `json:"name"`
        Description string `json:"description,omitempty"`
        Parameters  any    `json:"parameters"`
}

// ── Translation: Anthropic → OpenAI ────────────────────────────────

func anthropicToOpenAI(body []byte) ([]byte, error) {
        var anth AnthropicRequest
        if err := json.Unmarshal(body, &anth); err != nil {
                return nil, fmt.Errorf("parse anthropic request: %w", err)
        }

        openai := OpenAIRequest{
                Model:       anth.Model,
                MaxTokens:   anth.MaxTokens,
                Temperature: anth.Temperature,
                TopP:        anth.TopP,
                Stream:      anth.Stream,
                Stop:        anth.StopSeqs,
        }

        // 1. Translate system prompt
        if anth.System != nil {
                if sysText := extractSystemText(anth.System); sysText != "" {
                        openai.Messages = append(openai.Messages, OpenAIMessage{
                                Role:    "system",
                                Content: sysText,
                        })
                }
        }

        // 2. Translate messages
        for _, msg := range anth.Messages {
                openai.Messages = append(openai.Messages, convertAnthropicMessage(msg)...)
        }

        // 3. Translate tools
        for _, tool := range anth.Tools {
                openai.Tools = append(openai.Tools, OpenAITool{
                        Type: "function",
                        Function: OpenAIFunction{
                                Name:        tool.Name,
                                Description: tool.Description,
                                Parameters:  tool.InputSchema,
                        },
                })
        }

        return json.Marshal(openai)
}

func extractSystemText(system any) string {
        switch v := system.(type) {
        case string:
                return v
        case []any:
                var parts []string
                for _, block := range v {
                        if m, ok := block.(map[string]any); ok && m["type"] == "text" {
                                if t, ok := m["text"].(string); ok {
                                        parts = append(parts, t)
                                }
                        }
                }
                return strings.Join(parts, "\n")
        }
        return ""
}

func convertAnthropicMessage(msg AnthropicMessage) []OpenAIMessage {
        // Simple string content
        if text, ok := msg.Content.(string); ok {
                return []OpenAIMessage{{Role: msg.Role, Content: text}}
        }

        // Array of content blocks
        blocks, ok := msg.Content.([]any)
        if !ok {
                return []OpenAIMessage{{Role: msg.Role, Content: ""}}
        }

        var result []OpenAIMessage
        var contentParts []map[string]any
        var toolCalls []map[string]any

        for _, b := range blocks {
                block, ok := b.(map[string]any)
                if !ok {
                        continue
                }
                switch block["type"] {
                case "text":
                        contentParts = append(contentParts, map[string]any{
                                "type": "text",
                                "text": block["text"],
                        })
                case "image":
                        if src, ok := block["source"].(map[string]any); ok {
                                var url string
                                if src["type"] == "base64" {
                                        url = fmt.Sprintf("data:%s;base64,%s", src["media_type"], src["data"])
                                } else if src["type"] == "url" {
                                        url, _ = src["url"].(string)
                                }
                                contentParts = append(contentParts, map[string]any{
                                        "type":      "image_url",
                                        "image_url": map[string]any{"url": url},
                                })
                        }
                case "tool_use":
                        toolCalls = append(toolCalls, map[string]any{
                                "id":   block["id"],
                                "type": "function",
                                "function": map[string]any{
                                        "name":      block["name"],
                                        "arguments": toJSONString(block["input"]),
                                },
                        })
                case "tool_result":
                        result = append(result, OpenAIMessage{
                                Role:       "tool",
                                Content:    extractToolResultContent(block["content"]),
                                ToolCallID: getString(block["tool_use_id"]),
                        })
                }
        }

        if len(toolCalls) > 0 {
                m := OpenAIMessage{Role: "assistant", ToolCalls: make([]any, len(toolCalls))}
                for i, tc := range toolCalls {
                        m.ToolCalls[i] = tc
                }
                if len(contentParts) > 0 {
                        m.Content = contentParts
                }
                result = append([]OpenAIMessage{m}, result...)
        } else if len(contentParts) > 0 {
                result = append([]OpenAIMessage{{Role: msg.Role, Content: contentParts}}, result...)
        }

        return result
}

func extractToolResultContent(content any) string {
        switch v := content.(type) {
        case string:
                return v
        case []any:
                var parts []string
                for _, b := range v {
                        if m, ok := b.(map[string]any); ok && m["type"] == "text" {
                                parts = append(parts, getString(m["text"]))
                        }
                }
                return strings.Join(parts, "\n")
        }
        return ""
}

func getString(v any) string {
        if s, ok := v.(string); ok {
                return s
        }
        return ""
}

func toJSONString(v any) string {
        b, _ := json.Marshal(v)
        return string(b)
}

// ── Translation: OpenAI response → Anthropic (non-streaming) ──────

func openAIToAnthropicResponse(openaiResp []byte, model string) ([]byte, error) {
        var oai struct {
                ID      string `json:"id"`
                Choices []struct {
                        Message struct {
                                Content   string `json:"content"`
                                ToolCalls []struct {
                                        ID       string `json:"id"`
                                        Function struct {
                                                Name      string `json:"name"`
                                                Arguments string `json:"arguments"`
                                        } `json:"function"`
                                } `json:"tool_calls"`
                        } `json:"message"`
                        FinishReason string `json:"finish_reason"`
                } `json:"choices"`
                Usage struct {
                        PromptTokens     int `json:"prompt_tokens"`
                        CompletionTokens int `json:"completion_tokens"`
                } `json:"usage"`
        }
        if err := json.Unmarshal(openaiResp, &oai); err != nil {
                return nil, err
        }

        var content []map[string]any
        stopReason := "end_turn"

        if len(oai.Choices) > 0 {
                choice := oai.Choices[0]
                if choice.Message.Content != "" {
                        content = append(content, map[string]any{
                                "type": "text",
                                "text": choice.Message.Content,
                        })
                }
                for _, tc := range choice.Message.ToolCalls {
                        var input any
                        _ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
                        content = append(content, map[string]any{
                                "type":  "tool_use",
                                "id":    tc.ID,
                                "name":  tc.Function.Name,
                                "input": input,
                        })
                        stopReason = "tool_use"
                }
                switch choice.FinishReason {
                case "stop":
                        stopReason = "end_turn"
                case "length":
                        stopReason = "max_tokens"
                case "tool_calls":
                        stopReason = "tool_use"
                }
        }

        msgID := strings.Replace(oai.ID, "chatcmpl-", "msg_", 1)
        if msgID == oai.ID {
                msgID = "msg_" + oai.ID
        }

        anthropicResp := map[string]any{
                "id":            msgID,
                "type":          "message",
                "role":          "assistant",
                "model":         model,
                "content":       content,
                "stop_reason":   stopReason,
                "stop_sequence": nil,
                "usage": map[string]any{
                        "input_tokens":  oai.Usage.PromptTokens,
                        "output_tokens": oai.Usage.CompletionTokens,
                },
        }
        return json.Marshal(anthropicResp)
}

// ── Main handler ───────────────────────────────────────────────────

// handleAnthropicMessages handles POST /v1/messages (Anthropic format).
// GLM models use native AnthropicMessagesHandler.
// MiMo models are translated to OpenAI, forwarded, and response is translated back.
func (s *Server) handleAnthropicMessages(c *gin.Context) {
        body, err := io.ReadAll(c.Request.Body)
        if err != nil {
                c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
                return
        }

        var peek struct {
                Model  string `json:"model"`
                Stream bool   `json:"stream"`
        }
        _ = json.Unmarshal(body, &peek)
        model := s.resolveAlias(peek.Model)
        provider := routeByModel(model)

        switch provider {
        case "glm":
                // GLM has native Anthropic handler — forward directly
                c.Request.Body = io.NopCloser(bytes.NewReader(body))
                if s.glm != nil {
                        // Per-account if available
                        if s.accounts != nil {
                                acct, err := s.accounts.Next(ProviderGLM)
                                if err == nil {
                                        c.Set("account_id", acct.ID)
                                        client := GetHTTPClient(acct.ID, acct.Proxy, 0)
                                        s.glm.AnthropicMessagesWithAccount(c.Writer, c.Request, acct.ID, acct.ZaiToken, client)
                                        c.Abort()
                                        return
                                }
                        }
                        s.glm.AnthropicMessagesHandler(c.Writer, c.Request)
                        c.Abort()
                        return
                }
                c.JSON(http.StatusServiceUnavailable, gin.H{
                        "error": gin.H{"type": "glm_unavailable", "message": "GLM not initialized"},
                })

        case "mimo":
                // MiMo doesn't have Anthropic — translate to OpenAI and forward
                s.handleAnthropicViaMiMo(c, body, model, peek.Stream)

        default:
                c.JSON(http.StatusNotFound, gin.H{
                        "error": gin.H{"type": "model_not_found", "message": fmt.Sprintf("model %q not supported", model)},
                })
        }
}

func (s *Server) handleAnthropicViaMiMo(c *gin.Context, body []byte, model string, stream bool) {
        if s.mimoEngine == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{
                        "error": gin.H{"type": "mimo_unavailable", "message": "MiMo not initialized"},
                })
                return
        }

        // 1. Translate Anthropic → OpenAI
        openaiBody, err := anthropicToOpenAI(body)
        if err != nil {
                c.JSON(http.StatusBadRequest, gin.H{
                        "error": gin.H{"type": "translation_error", "message": err.Error()},
                })
                return
        }

        // 2. Build new request for MiMo sub-engine
        req, err := http.NewRequestWithContext(c.Request.Context(), "POST",
                "/v1/chat/completions", bytes.NewReader(openaiBody))
        if err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
                return
        }
        req.Header.Set("Content-Type", "application/json")

        // Inject per-account authctx if available
        if s.accounts != nil {
                acct, err := s.accounts.Next(ProviderMimo)
                if err == nil {
                        c.Set("account_id", acct.ID)
                        client := GetHTTPClient(acct.ID, acct.Proxy, 0)

                        injected := authctx.InjectedAuth{
                                Cookie:    fmt.Sprintf(`serviceToken="%s"; userId=%s; xiaomichatbot_ph="%s"`, acct.ServiceToken, acct.UserID, acct.XiaomichatPH),
                                Ph:        acct.XiaomichatPH,
                                Token:     acct.ServiceToken,
                                AccountID: acct.ID,
                                Provider:  "mimo",
                        }
                        ctx := authctx.WithAuth(req.Context(), injected)
                        ctx = authctx.WithClient(ctx, client)
                        req = req.WithContext(ctx)
                }
        }

        // 3. Non-streaming: capture and translate response
        if !stream {
                recorder := newAnthropicRecorder()
                s.mimoEngine.ServeHTTP(recorder, req)

                if recorder.code != http.StatusOK {
                        c.Data(recorder.code, "application/json", recorder.body.Bytes())
                        return
                }

                anthropicResp, err := openAIToAnthropicResponse(recorder.body.Bytes(), model)
                if err != nil {
                        c.JSON(http.StatusBadGateway, gin.H{
                                "error": gin.H{"type": "response_translation_error", "message": err.Error()},
                        })
                        return
                }
                c.Data(http.StatusOK, "application/json", anthropicResp)
                return
        }

        // 4. Streaming: translate SSE
        s.streamAnthropicFromOpenAI(c, req, model)
}

// streamAnthropicFromOpenAI translates OpenAI SSE response to Anthropic stream format.
//
// Anthropic stream format:
//   event: message_start
//   event: content_block_start
//   event: content_block_delta (text_delta)
//   event: content_block_stop
//   event: message_delta
//   event: message_stop
func (s *Server) streamAnthropicFromOpenAI(c *gin.Context, req *http.Request, model string) {
        c.Header("Content-Type", "text/event-stream")
        c.Header("Cache-Control", "no-cache")
        c.Header("Connection", "keep-alive")
        c.Header("X-Accel-Buffering", "no")

        writer := &anthropicStreamWriter{
                w:     c.Writer,
                model: model,
                msgID: "msg_" + randomID(),
        }
        writer.writeMessageStart()

        // Forward to sub-engine with custom writer that translates each chunk
        s.mimoEngine.ServeHTTP(writer, req)

        writer.writeMessageStop()
}

// ── Helper: response recorder for non-streaming ───────────────────

type anthropicRecorder struct {
        header http.Header
        body   *bytes.Buffer
        code   int
}

func newAnthropicRecorder() *anthropicRecorder {
        return &anthropicRecorder{
                header: make(http.Header),
                body:   &bytes.Buffer{},
                code:   http.StatusOK,
        }
}

func (r *anthropicRecorder) Header() http.Header         { return r.header }
func (r *anthropicRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *anthropicRecorder) WriteHeader(code int)        { r.code = code }

// ── Helper: SSE stream writer for streaming ───────────────────────

// anthropicStreamWriter wraps the gin ResponseWriter and translates
// OpenAI-format SSE chunks (produced by the MiMo sub-engine) into
// Anthropic-format SSE events sent to the client.
//
// A buf field is essential: SSE chunks may be split across multiple
// Write() calls (the sub-engine flushes in arbitrary boundaries).
// Without buffering, a "data: {...}" line that arrives in two halves
// would fail to parse.
type anthropicStreamWriter struct {
        w           gin.ResponseWriter
        model       string
        msgID       string
        blockOpened bool
        buf         string // accumulator for incomplete lines between Write() calls
}

func (w *anthropicStreamWriter) Header() http.Header         { return w.w.Header() }
func (w *anthropicStreamWriter) WriteHeader(statusCode int)  { w.w.WriteHeader(statusCode) }
func (w *anthropicStreamWriter) Write(b []byte) (int, error) { return w.translateAndWrite(b) }
func (w *anthropicStreamWriter) Flush()                       { w.w.Flush() }

func (w *anthropicStreamWriter) writeSSE(event string, data any) {
        b, _ := json.Marshal(data)
        fmt.Fprintf(w.w, "event: %s\ndata: %s\n\n", event, b)
        w.w.Flush()
}

func (w *anthropicStreamWriter) writeMessageStart() {
        w.writeSSE("message_start", map[string]any{
                "type": "message_start",
                "message": map[string]any{
                        "id":      w.msgID,
                        "type":    "message",
                        "role":    "assistant",
                        "model":   w.model,
                        "content": []any{},
                        "usage":   map[string]any{"input_tokens": 0, "output_tokens": 0},
                },
        })
}

func (w *anthropicStreamWriter) writeMessageStop() {
        if w.blockOpened {
                w.writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
        }
        w.writeSSE("message_delta", map[string]any{
                "type":  "message_delta",
                "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
                "usage": map[string]any{"output_tokens": 0},
        })
        w.writeSSE("message_stop", map[string]any{"type": "message_stop"})
}

// translateAndWrite buffers incoming bytes and processes only complete
// (newline-terminated) lines. This is critical because the MiMo sub-engine
// may flush in arbitrary chunk boundaries — a single "data: {...}\n" line
// could arrive as two separate Write() calls.
func (w *anthropicStreamWriter) translateAndWrite(b []byte) (int, error) {
        w.buf += string(b)
        for {
                idx := strings.Index(w.buf, "\n")
                if idx == -1 {
                        break // no complete line yet
                }
                line := strings.TrimSpace(w.buf[:idx])
                w.buf = w.buf[idx+1:]
                w.processLine(line)
        }
        return len(b), nil
}

// processLine parses a single OpenAI-format SSE line and emits the
// corresponding Anthropic SSE event(s).
func (w *anthropicStreamWriter) processLine(line string) {
        if !strings.HasPrefix(line, "data: ") {
                return // ignore comments, empty lines, event: prefixes
        }
        payload := strings.TrimPrefix(line, "data: ")
        if payload == "[DONE]" {
                return // message_stop is emitted by writeMessageStop()
        }

        var chunk struct {
                Choices []struct {
                        Delta struct {
                                Content string `json:"content"`
                        } `json:"delta"`
                } `json:"choices"`
        }
        if json.Unmarshal([]byte(payload), &chunk) != nil {
                return
        }

        for _, choice := range chunk.Choices {
                if choice.Delta.Content == "" {
                        continue
                }
                if !w.blockOpened {
                        w.writeSSE("content_block_start", map[string]any{
                                "type":          "content_block_start",
                                "index":         0,
                                "content_block": map[string]any{"type": "text", "text": ""},
                        })
                        w.blockOpened = true
                }
                w.writeSSE("content_block_delta", map[string]any{
                        "type":  "content_block_delta",
                        "index": 0,
                        "delta": map[string]any{"type": "text_delta", "text": choice.Delta.Content},
                })
        }
}

// randomID generates a 24-character hex ID using crypto/rand.
// Falls back to time-based ID only if /dev/urandom is unavailable.
func randomID() string {
        b := make([]byte, 12) // 12 bytes → 24 hex chars
        if _, err := rand.Read(b); err != nil {
                return fmt.Sprintf("%x", time.Now().UnixNano())
        }
        return hex.EncodeToString(b)
}
