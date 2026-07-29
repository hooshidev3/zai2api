// Package server — main gateway server that mounts GLM and MiMo providers.
package server

import (
        "bytes"
        "context"
        "database/sql"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "os"
        "strings"
        "time"

        "zai2api/gateway/auth"

        "github.com/gin-gonic/gin"
        glmapi "glm-free-api"
        "mimoproxy/pkg/authctx"
        "mimoproxy/pkg/routes"
        "mimoproxy/pkg/services"
)

// Server is the unified gateway.
type Server struct {
        cfg        *Config
        glm        *glmapi.Provider
        router     *gin.Engine
        mimoEngine *gin.Engine
        httpSrv    *http.Server
        accounts   *AccountManager
        db         *sql.DB
}

// NewServer creates and configures the gateway.
func NewServer(cfg *Config) (*Server, error) {
        // 0. Initialize gateway DB (accounts, features, request_log)
        gwDB, err := InitDB(getenv("ACCOUNTS_DB", "./data/accounts.sqlite"))
        if err != nil {
                log.Printf("⚠️  Gateway DB init failed: %v", err)
        }

        // 0a. Initialize AccountManager
        var am *AccountManager
        if gwDB != nil {
                am = NewAccountManager(gwDB, getenv("ZAI_STRATEGY", "round-robin"))
        }

        // 1. Initialize GLM provider
        glmProv, err := glmapi.NewProvider(glmapi.Options{
                CaptchaDBPath: cfg.GLMCaptchaDB,
                Verbose:       cfg.Verbose,
                AgentMode:     cfg.AgentMode,
        })
        if err != nil {
                log.Printf("⚠️  GLM provider init failed (continuing without GLM): %v", err)
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
                accounts:   am,
                db:         gwDB,
        }

        // Start retention job for request_log
        if gwDB != nil {
                StartRetentionJob(gwDB)
        }

        s.mountRoutes()
        return s, nil
}

func (s *Server) mountRoutes() {
        r := s.router

        r.GET("/health", s.handleHealth)

        apiAuth := auth.GatewayAuthMiddleware(s.cfg.GatewayToken)

        v1 := r.Group("/v1", apiAuth)
        {
                v1.GET("/models", s.handleModels)
                v1.POST("/chat/completions", s.handleChatCompletions)
                v1.POST("/messages", s.handleAnthropicMessages)
        }

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

        mimoGroup := r.Group("/mimo", apiAuth)
        mimoGroup.Any("/*any", s.forwardToMiMo)

        agentGroup := r.Group("/v1/agent", apiAuth)
        agentGroup.Any("/*any", s.forwardToMiMoAgent)

        // Account management API (Phase 3)
        if s.accounts != nil {
                apiGroup := r.Group("/api/v1", apiAuth)
                {
                        apiGroup.GET("/accounts", s.handleListAccounts)
                        apiGroup.POST("/accounts", s.handleCreateAccount)
                        apiGroup.GET("/accounts/:id", s.handleGetAccount)
                        apiGroup.PUT("/accounts/:id", s.handleUpdateAccount)
                        apiGroup.DELETE("/accounts/:id", s.handleDeleteAccount)
                        apiGroup.POST("/accounts/:id/toggle", s.handleToggleAccount)
                        apiGroup.POST("/accounts/:id/test", s.handleTestAccount)
                }
        }

        // Root — simple info page (no token displayed for security)
        r.GET("/", func(c *gin.Context) {
                c.Header("Content-Type", "text/html; charset=utf-8")
                c.String(200, `<!DOCTYPE html>
<html><head><title>zai2api Gateway</title>
<style>body{font-family:monospace;background:#0d1117;color:#c9d1d9;padding:40px;max-width:800px;margin:0 auto}
a{color:#58a6ff}h1{color:#58a6ff;border-bottom:1px solid #30363d;padding-bottom:10px}
code{background:#21262d;padding:2px 6px;border-radius:4px}</style>
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
  -H "Authorization: Bearer &lt;YOUR_GATEWAY_TOKEN&gt;" \
  -H "Content-Type: application/json" \
  -d '{"model":"glm-5","messages":[{"role":"user","content":"Hello"}]}'</pre>
</body></html>`)
        })
}

// Run starts the HTTP server.
func (s *Server) Run(addr string) error {
        s.httpSrv = &http.Server{
                Addr:              addr,
                Handler:           s.router,
                ReadHeaderTimeout: 10 * time.Second,
                ReadTimeout:       600 * time.Second,
                WriteTimeout:      600 * time.Second,
                MaxHeaderBytes:    1 << 20,
        }
        return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
        if s.httpSrv != nil {
                return s.httpSrv.Shutdown(ctx)
        }
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

// handleModels returns an aggregated list of GLM + MiMo models.
func (s *Server) handleModels(c *gin.Context) {
        // Collect models from both providers in parallel
        type modelItem struct {
                ID      string `json:"id"`
                Object  string `json:"object"`
                OwnedBy string `json:"owned_by"`
        }

        var models []modelItem

        // GLM models (static fallback list — in Phase 3, fetch from provider)
        if s.glm != nil {
                glmModels := []string{"glm-5", "glm-5.1", "glm-4.7", "GLM-5-Turbo", "GLM-5v-Turbo"}
                for _, m := range glmModels {
                        models = append(models, modelItem{
                                ID: m, Object: "model", OwnedBy: "zai",
                        })
                }
        }

        // MiMo models (forward to sub-engine and merge)
        mimoResp, err := s.forwardToMiMoAndCapture(c, "/v1/models")
        if err == nil && mimoResp != nil {
                var mimoList struct {
                        Data []struct {
                                ID      string `json:"id"`
                                OwnedBy string `json:"owned_by"`
                        } `json:"data"`
                }
                if json.Unmarshal(mimoResp, &mimoList) == nil {
                        for _, m := range mimoList.Data {
                                models = append(models, modelItem{
                                        ID: m.ID, Object: "model", OwnedBy: m.OwnedBy,
                                })
                        }
                }
        }

        c.JSON(200, gin.H{
                "object": "list",
                "data":   models,
        })
}

func (s *Server) handleChatCompletions(c *gin.Context) {
        // Limit body size to 50MB
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20)

        // Read body with TeeReader for single-pass peek + restore
        buf := new(bytes.Buffer)
        tee := io.TeeReader(c.Request.Body, buf)
        var peek struct {
                Model string `json:"model"`
        }
        _ = json.NewDecoder(tee).Decode(&peek)

        // Restore body for downstream handler
        c.Request.Body = io.NopCloser(buf)
        c.Request.ContentLength = int64(buf.Len())

        model := peek.Model
        if model == "" {
                model = s.cfg.DefaultModel
        }

        provider := routeByModel(model)
        c.Set("provider", provider)
        c.Set("model", model)

        log.Printf("[dispatch] model=%s → provider=%s", model, provider)

        switch provider {
        case "glm":
                s.forwardToGLMChat(c)
        case "mimo":
                s.forwardToMiMoWithAccount(c)
        default:
                c.JSON(http.StatusNotFound, gin.H{
                        "error": gin.H{
                                "type":    "model_not_found",
                                "message": fmt.Sprintf("model %q not supported", model),
                        },
                })
        }
}

// forwardToGLMChat selects a GLM account and forwards with per-account client.
func (s *Server) forwardToGLMChat(c *gin.Context) {
        if s.glm == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{
                        "error": gin.H{"type": "glm_unavailable", "message": "GLM provider not initialized"},
                })
                return
        }

        // If AccountManager has GLM accounts, select one
        if s.accounts != nil {
                acct, err := s.accounts.Next(ProviderGLM)
                if err != nil {
                        c.JSON(http.StatusServiceUnavailable, gin.H{
                                "error": gin.H{"type": "no_account", "message": "no active GLM accounts"},
                        })
                        return
                }
                c.Set("account_id", acct.ID)
                client := GetHTTPClient(acct.ID, acct.Proxy, 0) // 0 = no timeout (streaming)
                s.glm.ChatCompletionsWithAccount(c.Writer, c.Request, acct.ID, acct.ZaiToken, client)
                c.Abort()
                return
        }

        // Fallback: no AccountManager, use global session
        s.glm.ChatCompletionsHandler(c.Writer, c.Request)
        c.Abort()
}

// forwardToMiMoWithAccount selects a MiMo account and injects authctx.
func (s *Server) forwardToMiMoWithAccount(c *gin.Context) {
        // If AccountManager has MiMo accounts, select one and inject authctx
        if s.accounts != nil {
                acct, err := s.accounts.Next(ProviderMimo)
                if err != nil {
                        c.JSON(http.StatusServiceUnavailable, gin.H{
                                "error": gin.H{"type": "no_mimo_account", "message": err.Error()},
                        })
                        return
                }
                c.Set("account_id", acct.ID)

                client := GetHTTPClient(acct.ID, acct.Proxy, 0)

                injected := authctx.InjectedAuth{
                        Cookie: fmt.Sprintf(`serviceToken="%s"; userId=%s; xiaomichatbot_ph="%s"`,
                                acct.ServiceToken, acct.UserID, acct.XiaomichatPH),
                        Ph:        acct.XiaomichatPH,
                        Token:     acct.ServiceToken,
                        AccountID: acct.ID,
                        Provider:  "mimo",
                }
                if acct.Proxy != nil {
                        injected.ProxyInfo = acct.Proxy.URL()
                }

                ctx := authctx.WithAuth(c.Request.Context(), injected)
                ctx = authctx.WithClient(ctx, client)
                c.Request = c.Request.WithContext(ctx)
        }

        s.mimoEngine.ServeHTTP(c.Writer, c.Request)
        c.Abort()
}

func (s *Server) handleAnthropicMessages(c *gin.Context) {
        if s.glm == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "glm_unavailable", "message": "GLM provider not initialized"}})
                return
        }

        // Per-account if AccountManager available
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
}

// forwardToMiMo strips the /mimo prefix before forwarding to the sub-engine.
func (s *Server) forwardToMiMo(c *gin.Context) {
        originalPath := c.Request.URL.Path
        c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/mimo")
        if c.Request.URL.Path == "" {
                c.Request.URL.Path = "/"
        }
        s.mimoEngine.ServeHTTP(c.Writer, c.Request)
        c.Request.URL.Path = originalPath // restore for logging
        c.Abort()
}

// forwardToMiMoAgent forwards /v1/agent/* to the MiMo sub-engine.
func (s *Server) forwardToMiMoAgent(c *gin.Context) {
        s.mimoEngine.ServeHTTP(c.Writer, c.Request)
        c.Abort()
}

// forwardToMiMoAndCapture forwards a request to the MiMo sub-engine and
// captures the response body (used for model aggregation).
func (s *Server) forwardToMiMoAndCapture(c *gin.Context, path string) ([]byte, error) {
        // Create a new request to the MiMo sub-engine
        req, err := http.NewRequestWithContext(c.Request.Context(), "GET", path, nil)
        if err != nil {
                return nil, err
        }
        // Copy auth header
        if auth := c.GetHeader("Authorization"); auth != "" {
                req.Header.Set("Authorization", auth)
        }

        // Use a response recorder
        w := &responseRecorder{header: http.Header{}, body: &strings.Builder{}}
        s.mimoEngine.ServeHTTP(w, req)

        if w.status != 200 {
                return nil, fmt.Errorf("mimo returned %d", w.status)
        }
        return []byte(w.body.String()), nil
}

// responseRecorder captures HTTP responses for internal forwarding.
type responseRecorder struct {
        header http.Header
        body   *strings.Builder
        status int
}

func (r *responseRecorder) Header() http.Header {
        if r.header == nil {
                r.header = http.Header{}
        }
        return r.header
}
func (r *responseRecorder) Write(data []byte) (int, error) {
        return r.body.Write(data)
}
func (r *responseRecorder) WriteHeader(statusCode int) {
        r.status = statusCode
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

// readBody reads the entire request body using io.ReadAll.
func readBody(c *gin.Context) ([]byte, error) {
        body, err := io.ReadAll(c.Request.Body)
        if err != nil {
                return nil, err
        }
        c.Request.Body.Close()
        return body, nil
}

// restoreBody replaces the request body with the given bytes.
func restoreBody(c *gin.Context, body []byte) {
        c.Request.Body = &bodyReader{data: body}
        c.Request.ContentLength = int64(len(body))
}

// peekModel extracts the "model" field from a JSON body using proper parsing.
func peekModel(body []byte) string {
        var peek struct {
                Model string `json:"model"`
        }
        _ = json.Unmarshal(body, &peek)
        return peek.Model
}

// bodyReader implements io.ReadCloser over a byte slice with proper
// position tracking and io.EOF signaling.
type bodyReader struct {
        data []byte
        pos  int
}

func (b *bodyReader) Read(p []byte) (int, error) {
        if b.pos >= len(b.data) {
                return 0, io.EOF
        }
        copied := copy(p, b.data[b.pos:])
        b.pos += copied
        return copied, nil
}

func (b *bodyReader) Close() error { return nil }
