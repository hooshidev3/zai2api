// Package server — Lazy GLM provider initialization.
//
// The GLM provider can be initialized at startup (if ZAI_TOKEN env or
// captcha DB is available) or lazily when the first GLM account is added
// via the dashboard. This file contains the thread-safe logic for the
// lazy path.
package server

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"zai2api/gateway/auth"
	glmapi "glm-free-api"
)

// glmInitMu protects lazy GLM initialization so that concurrent
// handleCreateAccount calls (e.g., two GLM accounts added at once) do
// not race to create two Provider instances.
var glmInitMu sync.Mutex

// ensureGLMProvider initializes the GLM provider on demand using the
// given Z.AI token, if it is not already initialized.
//
// This is called from handleCreateAccount when a GLM account is added
// and s.glm is still nil (e.g., the gateway started without ZAI_TOKEN
// and without a captcha DB).
//
// The newly created provider is also wired into the router so that
// /glm/* routes become available.
func (s *Server) ensureGLMProvider(zaiToken string) bool {
	if zaiToken == "" {
		return false
	}

	glmInitMu.Lock()
	defer glmInitMu.Unlock()

	// Double-check after acquiring the lock — another goroutine may have
	// initialized it in the meantime.
	if s.glm != nil {
		// Provider exists; just update the global session token so the
		// free/public models path uses the most recently added account.
		s.glm.SetSessionToken(zaiToken)
		return true
	}

	prov, err := glmapi.NewProvider(glmapi.Options{
		Tokens:    []string{zaiToken},
		Verbose:   s.cfg.Verbose,
		AgentMode: s.cfg.AgentMode,
	})
	if err != nil {
		log.Printf("[glm] lazy init failed: %v", err)
		return false
	}

	s.glm = prov
	s.mountGLMRoutes()
	log.Printf("[glm] lazy init succeeded — GLM provider is now ready")
	return true
}

// mountGLMRoutes registers the /glm/* route group. This is called either
// from mountRoutes() at startup (if GLM was initialized then) or from
// ensureGLMProvider() when GLM is initialized lazily.
//
// Uses a package-level mutex (glmRoutesMu) to make sure the route group
// is only mounted once even if ensureGLMProvider is called concurrently.
var glmRoutesMu sync.Mutex

func (s *Server) mountGLMRoutes() {
	glmRoutesMu.Lock()
	defer glmRoutesMu.Unlock()

	if s.glm == nil {
		return
	}

	apiAuth := auth.GatewayAuthMiddleware(s.cfg.GatewayToken)
	glmGroup := s.router.Group("/glm", apiAuth)
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

// ensure http import is used (for future handlers in this file).
var _ http.HandlerFunc = func(http.ResponseWriter, *http.Request) {}
