// Package server — main gateway server that mounts GLM and MiMo providers.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"zai2api/gateway/auth"
	"zai2api/gateway/proxy"

	"github.com/gin-gonic/gin"
	glmapi "glm-free-api"
	"mimoproxy/pkg/routes"
	"mimoproxy/pkg/services"
)

// Server is the unified gateway.
type Server struct {
	cfg        *Config
	glm        *glmapi.Provider
	router     *gin.Engine
	mimoEngine *gin.Engine
}

// NewServer creates and configures the gateway.
func NewServer(cfg *Config) (*Server, error) {
	// 1. Initialize GLM provider
	glmProv, err := glmapi.NewProvider(glmapi.Options{
		CaptchaDBPath: cfg.GLMCaptchaDB,
		Verbose:       cfg.Verbose,
		AgentMode:     cfg.AgentMode,
	})
	if err != nil {
		log.Printf("⚠️  GLM provider init failed (continuing without GLM): %v", err)
		// Continue without GLM — gateway will return 503 for GLM requests
	}

	// 2. Initialize MiMo DB
	services.InitDB()

	// 3. Build MiMo sub-engine (mounted via ServeHTTP)
	gin.SetMode(gin.ReleaseMode)
	mimoSub := gin.New()
	routes.RegisterChatRoutes(mimoSub, nil)
	routes.RegisterAgentRoutes(mimoSub)

	// 4. Build main gateway router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	// Set trusted proxies (security: only trust loopback by default)
	trustedProxies := os.Getenv("TRUSTED_PROXIES")
	if trustedProxies == "" {
		trustedProxies = "127.0.0.1/32,::1/128"
	}
	if err := r.SetTrustedProxies(strings.Split(trustedProxies, ",")); err != nil {
		log.Printf("warning: SetTrustedProxies failed: %v", err)
	}

	s := &Server{
		cfg:        cfg,
		glm:        glmProv,
		router:     r,
		mimoEngine: mimoSub,
	}

	s.mountRoutes()
	return s, nil
}

func (s *Server) mountRoutes() {
	r := s.router

	// Health (no auth — for monitoring)
	r.GET("/health", s.handleHealth)

	// Auth middleware
	apiAuth := auth.GatewayAuthMiddleware(s.cfg.GatewayToken)

	// OpenAI-compatible endpoints
	v1 := r.Group("/v1", apiAuth)
	{
		v1.GET("/models", s.handleModels)
		v1.POST("/chat/completions", s.handleChatCompletions)
		v1.POST("/messages", s.handleAnthropicMessages)
	}

	// GLM-specific routes (under /glm/)
	if s.glm != nil {
		glmGroup := r.Group("/glm", apiAuth)
		{
			glmGroup.GET("/status", gin.WrapF(s.glm.StatusHandler))
			glmGroup.GET("/features", gin.WrapF(s.glm.FeaturesHandler))
			glmGroup.POST("/features", gin.WrapF(s.glm.FeaturesHandler))
			glmGroup.GET("/admin/stats", gin.WrapF(s.glm.StatsHandler))
			glmGroup.GET("/admin/clients", gin.WrapF(s.glm.ClientsHandler))
			glmGroup.GET("/inject.js", gin.WrapF(s.glm.InjectHandler))
			glmGroup.POST("/stop", gin.WrapF(s.glm.StopHandler))
		}
	}

	// MiMo-specific routes (under /mimo/) — forward to sub-engine
	mimoGroup := r.Group("/mimo", apiAuth)
	mimoGroup.Any("/*any", s.forwardToMiMo)

	// MiMo agent routes (under /v1/agent/)
	agentGroup := r.Group("/v1/agent", apiAuth)
	agentGroup.Any("/*any", s.forwardToMiMoAgent)

	// Root — simple info page
	r.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, `<!DOCTYPE html>
<html><head><title>zai2api Gateway</title>
<style>body{font-family:monospace;background:#0d1117;color:#c9d1d9;padding:40px;max-width:800px;margin:0 auto}
a{color:#58a6ff}h1{color:#58a6ff;border-bottom:1px solid #30363d;padding-bottom:10px}</style>
</head><body>
<h1>zai2api — Unified AI Gateway</h1>
<p>Single endpoint for GLM (Z.AI) and MiMo (Xiaomi) providers.</p>
<h3>Endpoints</h3>
<ul>
<li><code>POST /v1/chat/completions</code> — OpenAI-compatible (dispatches by model)</li>
<li><code>POST /v1/messages</code> — Anthropic-compatible (GLM only)</li>
<li><code>GET  /v1/models</code> — Aggregated model list</li>
<li><code>GET  /health</code> — Health check</li>
<li><code>/glm/*</code> — GLM-specific routes</li>
<li><code>/mimo/*</code> — MiMo-specific routes</li>
<li><code>/v1/agent/*</code> — MiMo agent routes</li>
</ul>
<h3>Quick test</h3>
<pre>curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer %s" \
  -H "Content-Type: application/json" \
  -d '{"model":"glm-5","messages":[{"role":"user","content":"Hello"}]}'</pre>
</body></html>`, s.cfg.GatewayToken)
	})
}

// Run starts the HTTP server.
func (s *Server) Run(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       600 * time.Second,
		WriteTimeout:      600 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	return srv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	// TODO: implement graceful shutdown with http.Server.Shutdown
	return nil
}

// Close releases resources.
func (s *Server) Close() error {
	if s.glm != nil {
		return s.glm.Close()
	}
	return nil
}

// ── Handlers ───────────────────────────────────────────────────────

func (s *Server) handleHealth(c *gin.Context) {
	status := gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"providers": gin.H{},
	}
	if s.glm != nil {
		status["providers"].(gin.H)["glm"] = "ready"
	} else {
		status["providers"].(gin.H)["glm"] = "unavailable"
	}
	status["providers"].(gin.H)["mimo"] = "ready"
	c.JSON(200, status)
}

func (s *Server) handleModels(c *gin.Context) {
	// Forward to MiMo's handleModels (which returns aggregated list)
	s.forwardToMiMo(c)
}

func (s *Server) handleChatCompletions(c *gin.Context) {
	// Read body to peek model
	body, err := readBody(c)
	if err != nil {
		c.JSON(400, gin.H{"error": gin.H{"type": "invalid_request", "message": err.Error()}})
		return
	}

	// Parse just the model field
	model := peekModel(body)
	if model == "" {
		model = s.cfg.DefaultModel
	}

	provider := routeByModel(model)
	log.Printf("[dispatch] model=%s → provider=%s", model, provider)

	// Restore body for downstream handler
	restoreBody(c, body)

	switch provider {
	case "mimo":
		s.forwardToMiMo(c)
	case "glm":
		if s.glm == nil {
			c.JSON(503, gin.H{"error": gin.H{"type": "glm_unavailable", "message": "GLM provider not initialized (captcha DB missing?)"}})
			return
		}
		s.glm.ChatCompletionsHandler(c.Writer, c.Request)
		c.Abort()
	default:
		c.JSON(404, gin.H{"error": gin.H{"type": "model_not_found", "message": fmt.Sprintf("model %q not supported", model)}})
	}
}

func (s *Server) handleAnthropicMessages(c *gin.Context) {
	if s.glm == nil {
		c.JSON(503, gin.H{"error": gin.H{"type": "glm_unavailable", "message": "GLM provider not initialized"}})
		return
	}
	s.glm.AnthropicMessagesHandler(c.Writer, c.Request)
	c.Abort()
}

func (s *Server) forwardToMiMo(c *gin.Context) {
	s.mimoEngine.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

func (s *Server) forwardToMiMoAgent(c *gin.Context) {
	// Rewrite path: /v1/agent/run → /v1/agent/run
	c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "")
	s.mimoEngine.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

// ── Helpers ────────────────────────────────────────────────────────

func routeByModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.HasPrefix(m, "mimo"):
		return "mimo"
	case strings.HasPrefix(m, "glm"), strings.HasPrefix(m, "zai"):
		return "glm"
	default:
		return "glm"
	}
}

func readBody(c *gin.Context) ([]byte, error) {
	body := make([]byte, c.Request.ContentLength)
	if c.Request.ContentLength > 0 {
		_, err := c.Request.Body.Read(body)
		if err != nil && err.Error() != "EOF" {
			return nil, err
		}
	}
	c.Request.Body = nil
	return body, nil
}

func restoreBody(c *gin.Context, body []byte) {
	c.Request.Body = noopCloser{bytes: body}
	c.Request.ContentLength = int64(len(body))
}

func peekModel(body []byte) string {
	// Simple JSON peek without full parse
	s := string(body)
	idx := strings.Index(s, `"model"`)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(`"model"`):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = rest[colon+1:]
	quote := strings.Index(rest, `"`)
	if quote < 0 {
		return ""
	}
	rest = rest[quote+1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// noopCloser implements io.ReadCloser over a byte slice.
type noopCloser struct {
	bytes []byte
	pos   int
}

func (n noopCloser) Read(p []byte) (int, error) {
	if n.pos >= len(n.bytes) {
		return 0, fmt.Errorf("EOF")
	}
	copied := copy(p, n.bytes[n.pos:])
	return copied, nil
}

func (n noopCloser) Close() error { return nil }

// unused import suppression
var _ = proxy.ResolveClient
