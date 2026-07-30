// Package server — Pre-create connection testing.
//
// TestBeforeCreate validates an account's proxy and provider credentials
// WITHOUT persisting the account. This lets the dashboard's "Test
// Connection" button verify credentials before the user clicks "Save",
// and lets handleCreateAccount reject invalid accounts before they are
// stored.
package server

import (
        "fmt"
        "net/http"
        "time"

        "github.com/gin-gonic/gin"
)

// TestBeforeCreateResult holds the outcome of a pre-create connection test.
// It mirrors TestResult but does not require an account ID (the account
// does not exist yet).
type TestBeforeCreateResult struct {
        Provider        string        `json:"provider"`
        TestedAt        time.Time     `json:"tested_at"`
        ProxyStatus     string        `json:"proxy_status"`
        ProxyError      string        `json:"proxy_error,omitempty"`
        ProxyLatency    time.Duration `json:"proxy_latency_ms"`
        ProviderStatus  string        `json:"provider_status"`
        ProviderError   string        `json:"provider_error,omitempty"`
        ProviderLatency time.Duration `json:"provider_latency_ms"`
        Overall         string        `json:"overall"`
}

// TestBeforeCreate validates proxy + provider credentials for a
// CreateAccountRequest WITHOUT creating the account.
//
// It builds a temporary HTTP client (with proxy if provided) and calls
// the provider's auth-verification endpoint:
//   - GLM:    GET https://chat.z.ai/api/v1/auths/ with the ZaiToken
//   - MiMo:   GET https://aistudio.xiaomimimo.com/open-apis/bot/config with cookies
//
// Used by:
//   - POST /api/v1/accounts/test-connection (dashboard "Test" button)
//   - POST /api/v1/accounts (mandatory pre-check before Create)
func (am *AccountManager) TestBeforeCreate(req CreateAccountRequest) (*TestBeforeCreateResult, error) {
        if err := am.validate(req); err != nil {
                return nil, err
        }

        result := &TestBeforeCreateResult{
                Provider: string(req.Provider),
                TestedAt: time.Now(),
        }

        // 1. Test proxy (if configured)
        if req.Proxy != nil {
                proxyStart := time.Now()
                if err := testProxy(req.Proxy); err != nil {
                        result.ProxyStatus = "failed"
                        result.ProxyError = err.Error()
                        result.ProxyLatency = time.Since(proxyStart)
                        result.Overall = "failed"
                        return result, nil
                }
                result.ProxyStatus = "ok"
                result.ProxyLatency = time.Since(proxyStart)
        }

        // 2. Test provider — use a temporary client with a 20s timeout.
        // We use a synthetic account ID ("test-before-create") so the client
        // is cached under a stable key and cleaned up later.
        tempID := "test-before-create"
        client := GetHTTPClient(tempID, req.Proxy, 20*time.Second)
        defer InvalidateClient(tempID)

        providerStart := time.Now()
        var providerErr error
        switch req.Provider {
        case ProviderGLM:
                providerErr = testZAIConnection(client, req.ZaiToken)
        case ProviderMimo:
                providerErr = testMiMoConnection(client, req.ServiceToken, req.UserID, req.XiaomichatPH)
        default:
                providerErr = fmt.Errorf("unknown provider: %s", req.Provider)
        }

        result.ProviderLatency = time.Since(providerStart)
        if providerErr != nil {
                result.ProviderStatus = "failed"
                result.ProviderError = providerErr.Error()
                result.Overall = "failed"
                return result, nil
        }

        result.ProviderStatus = "ok"
        result.Overall = "ok"
        return result, nil
}

// handleTestConnectionBeforeCreate is the endpoint for the dashboard's
// "Test Connection" button. It validates credentials without creating
// the account.
func (s *Server) handleTestConnectionBeforeCreate(c *gin.Context) {
        var req CreateAccountRequest
        if err := c.ShouldBindJSON(&req); err != nil {
                c.JSON(http.StatusBadRequest, gin.H{
                        "error": gin.H{"type": "invalid_request", "message": err.Error()},
                })
                return
        }

        result, err := s.accounts.TestBeforeCreate(req)
        if err != nil {
                c.JSON(http.StatusBadRequest, gin.H{
                        "error": gin.H{"type": "validation_error", "message": err.Error()},
                })
                return
        }

        if result.Overall != "ok" {
                c.JSON(http.StatusPreconditionFailed, result)
                return
        }
        c.JSON(http.StatusOK, result)
}
