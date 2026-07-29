// Package server — Providers API (status, health, account counts).
package server

import (
        "fmt"
        "net/http"
        "time"

        "github.com/gin-gonic/gin"
)

// ProviderStatus holds health and stats for one provider.
type ProviderStatus struct {
        Name         string         `json:"name"`
        Status       string         `json:"status"` // "ready" | "degraded" | "unavailable"
        Uptime       string         `json:"uptime"`
        AccountCount int            `json:"account_count"`
        ActiveCount  int            `json:"active_count"`
        TotalReqs    int64          `json:"total_requests"`
        TotalErrors  int64          `json:"total_errors"`
        AvgLatency   int64          `json:"avg_latency_ms"`
        Details      map[string]any `json:"details,omitempty"`
}

// handleProviderStatus returns status for GLM and MiMo providers.
func (s *Server) handleProviderStatus(c *gin.Context) {
        if s.accounts == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{
                        "error": gin.H{"type": "accounts_unavailable", "message": "accounts not initialized"},
                })
                return
        }

        var providers []ProviderStatus

        // GLM status
        glmAccounts := s.accounts.List(ProviderGLM)
        glmActive := countActiveAccounts(glmAccounts)
        glmStatus := "unavailable"
        if s.glm != nil && glmActive > 0 {
                glmStatus = "ready"
        } else if s.glm != nil {
                glmStatus = "degraded"
        }
        glmReqs, glmErrs, glmLatency := aggregateAccountStats(glmAccounts)

        glmModels := []string{}
        if s.glm != nil {
                glmModels = s.glm.FetchModels()
        }

        providers = append(providers, ProviderStatus{
                Name:         "GLM (Z.AI)",
                Status:       glmStatus,
                Uptime:       formatUptime(time.Since(s.startTime)),
                AccountCount: len(glmAccounts),
                ActiveCount:  glmActive,
                TotalReqs:    glmReqs,
                TotalErrors:  glmErrs,
                AvgLatency:   glmLatency,
                Details: map[string]any{
                        "models":       glmModels,
                        "captcha_db":   s.cfg.GLMCaptchaDB,
                        "agent_mode":   s.cfg.AgentMode,
                },
        })

        // MiMo status
        mimoAccounts := s.accounts.List(ProviderMimo)
        mimoActive := countActiveAccounts(mimoAccounts)
        mimoStatus := "unavailable"
        if s.mimoEngine != nil && mimoActive > 0 {
                mimoStatus = "ready"
        } else if s.mimoEngine != nil {
                mimoStatus = "degraded"
        }
        mimoReqs, mimoErrs, mimoLatency := aggregateAccountStats(mimoAccounts)

        providers = append(providers, ProviderStatus{
                Name:         "MiMo (Xiaomi)",
                Status:       mimoStatus,
                Uptime:       formatUptime(time.Since(s.startTime)),
                AccountCount: len(mimoAccounts),
                ActiveCount:  mimoActive,
                TotalReqs:    mimoReqs,
                TotalErrors:  mimoErrs,
                AvgLatency:   mimoLatency,
                Details: map[string]any{
                        "sub_engine": s.mimoEngine != nil,
                },
        })

        c.JSON(http.StatusOK, gin.H{"providers": providers})
}

// countActiveAccounts returns the number of enabled accounts.
func countActiveAccounts(accounts []*Account) int {
        n := 0
        for _, a := range accounts {
                if a.Enabled.Load() {
                        n++
                }
        }
        return n
}

// aggregateAccountStats sums reqs, errors, and averages latency across accounts.
func aggregateAccountStats(accounts []*Account) (reqs, errs, latency int64) {
        for _, a := range accounts {
                reqs += a.Stats.ReqCount.Load()
                errs += a.Stats.ErrCount.Load()
                latency += a.Stats.AvgLatency.Load()
        }
        if len(accounts) > 0 {
                latency /= int64(len(accounts))
        }
        return
}

// formatUptime formats a duration as "1d 2h 3m" or "2h 3m" or "3m".
func formatUptime(d time.Duration) string {
        days := int(d.Hours()) / 24
        hours := int(d.Hours()) % 24
        mins := int(d.Minutes()) % 60
        if days > 0 {
                return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
        }
        if hours > 0 {
                return fmt.Sprintf("%dh %dm", hours, mins)
        }
        return fmt.Sprintf("%dm", mins)
}
