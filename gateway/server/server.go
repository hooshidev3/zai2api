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
        "sync"
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
        cfg         *Config
        glm         *glmapi.Provider
        router      *gin.Engine
        mimoEngine  *gin.Engine
        httpSrv     *http.Server
        accounts    *AccountManager
        db          *sql.DB
        stats       *StatsCollector
        startTime   time.Time
        aliases     map[string]string
        aliasesMu   sync.RWMutex
        rateLimiter *RateLimiter
}

// NewServer creates and configures the gateway.
func NewServer(cfg *Config) (*Server, error) {
        // 0. Initialize gateway DB (accounts, features, request_log)
        gwDB, err := InitDB(cfg.AccountsDB)
        if err != nil {
                log.Printf("⚠️  Gateway DB init failed: %v", err)
        }

        // 0a. Initialize AccountManager
        var am *AccountManager
        if gwDB != nil {
                am = NewAccountManager(gwDB, cfg.ZAIStrategy)
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

        // Load HTML templates and serve static files
        r.LoadHTMLGlob("templates/*")
        r.Static("/static", "./static")

        s := &Server{
                cfg:         cfg,
                glm:         glmProv,
                router:      r,
                mimoEngine:  mimoSub,
                accounts:    am,
                db:          gwDB,
                stats:       NewStatsCollector(),
                startTime:   time.Now(),
                aliases:     make(map[string]string),
                rateLimiter: NewRateLimiter(),
        }

        // Load aliases and rate limits from DB
        if gwDB != nil {
                s.loadAliasesFromDB()
                s.rateLimiter.LoadLimitsFromDB(gwDB)
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

        // Stats middleware (records every request)
        r.Use(s.stats.Middleware())

        // Health (no auth — for monitoring)
        r.GET("/health", s.handleHealth)

        // Login routes (no auth required)
        r.GET("/login", s.handleLoginPage)
        r.POST("/login", s.handleLoginSubmit)
        r.GET("/logout", s.handleLogout)

        // Dashboard (behind DashboardAuthMiddleware)
        dashboardMW := DashboardAuthMiddleware(s.cfg.DashboardToken)
        r.GET("/", dashboardMW, s.handleDashboard)

        // SSE stats stream (behind dashboard auth)
        r.GET("/api/v1/stats/stream", dashboardMW, s.handleStatsStream)

        apiAuth := auth.GatewayAuthMiddleware(s.cfg.GatewayToken)

        v1 := r.Group("/v1", apiAuth)
        {
                v1.GET("/models", s.handleModels)
                v1.POST("/chat/completions", s.handleChatCompletions)
                v1.POST("/messages", s.handleAnthropicMessages)
                v1.POST("/files", s.handleFileUpload)
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
                        apiGroup.GET("/models", s.handleAggregatedModels)
                        apiGroup.PUT("/models/:id/features", s.handleUpdateModelFeatures)

                        // Phase 5: Providers, Stats, Aliases, Rate Limits, Agents
                        apiGroup.GET("/providers/status", s.handleProviderStatus)
                        apiGroup.GET("/stats/detailed", s.handleDetailedStats)
                        apiGroup.GET("/stats/export", s.handleExportStatsCSV)
                        apiGroup.GET("/models/aliases", s.handleListAliases)
                        apiGroup.POST("/models/aliases", s.handleAddAlias)
                        apiGroup.DELETE("/models/aliases/:name", s.handleDeleteAlias)
                        apiGroup.GET("/models/rate-limits", s.handleListRateLimits)
                        apiGroup.PUT("/models/:id/rate-limit", s.handleSetRateLimit)
                        apiGroup.DELETE("/models/:id/rate-limit", s.handleDeleteRateLimit)
                        apiGroup.GET("/agents", s.handleListAgents)
                        apiGroup.GET("/agents/:id", s.handleAgentStatus)
                        apiGroup.POST("/agents/run", s.handleRunAgent)
                        apiGroup.GET("/agents/:id/stream", s.handleAgentStream)
                }
        }
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
        type modelItem struct {
                ID       string `json:"id"`
                Object   string `json:"object"`
                OwnedBy  string `json:"owned_by"`
                Provider string `json:"_provider"`
        }

        var models []modelItem

        // GLM models — try to fetch from provider, fallback to static list
        if s.glm != nil {
                glmModels := s.glm.FetchModels()
                if len(glmModels) == 0 {
                        glmModels = []string{"glm-5", "glm-5.1", "glm-4.7", "GLM-5-Turbo", "GLM-5v-Turbo"}
                }
                for _, m := range glmModels {
                        models = append(models, modelItem{
                                ID: m, Object: "model", OwnedBy: "zai", Provider: "glm",
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
                                        ID: m.ID, Object: "model", OwnedBy: m.OwnedBy, Provider: "mimo",
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

        // Resolve alias (e.g., "fast" → "glm-4.5-air")
        resolvedModel := s.resolveAlias(model)
        if resolvedModel != model {
                log.Printf("[dispatch] alias: %s → %s", model, resolvedModel)
                model = resolvedModel
        }

        // Check rate limit
        if s.rateLimiter != nil && !s.rateLimiter.Allow(model, 0) {
                c.JSON(http.StatusTooManyRequests, gin.H{
                        "error": gin.H{
                                "type":    "rate_limit_exceeded",
                                "message": fmt.Sprintf("Rate limit for model %s exceeded", model),
                        },
                })
                return
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

// handleAnthropicMessages is now in anthropic_bridge.go (supports GLM + MiMo translation)

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
//
// ⚠️ This buffers the entire response — only use for non-streaming
// endpoints (e.g., GET /v1/models). For streaming endpoints (like
// chat/completions with stream=true), use forwardToMiMo instead.
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
