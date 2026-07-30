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
        Note         string         `json:"note,omitempty"`
        Uptime       string         `json:"uptime"`
        AccountCount int            `json:"account_count"`
        ActiveCount  int            `json:"active_count"`
        TotalReqs    int64          `json:"total_requests"`
        TotalErrors  int64          `json:"total_errors"`
        AvgLatency   int64          `json:"avg_latency_ms"`
        Details      map[string]any `json:"details,omitempty"`
}

// handleProviderStatus returns status for GLM and MiMo providers.
//
// GLM has four states:
//   - glm==nil && active==0 → unavailable  "no captcha DB, no accounts"
//   - glm==nil && active>0  → degraded     "accounts exist but provider not yet initialized"
//   - glm!=nil && active==0 → degraded     "using global session (free models)"
//   - glm!=nil && active>0  → ready
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
        glmReqs, glmErrs, glmLatency := aggregateAccountStats(glmAccounts)

        glmStatus := "unavailable"
        glmNote := ""
        switch {
        case s.glm == nil && glmActive == 0:
                glmStatus = "unavailable"
                glmNote = "No GLM credentials. Set ZAI_TOKEN env, provide a captcha DB, or add a GLM account."
        case s.glm == nil && glmActive > 0:
                glmStatus = "degraded"
                glmNote = "GLM accounts exist but provider is not yet initialized. Initialization is in progress."
        case s.glm != nil && glmActive == 0:
                glmStatus = "degraded"
                glmNote = "Using global session (free models). Add a GLM account for per-account routing."
        case s.glm != nil && glmActive > 0:
                glmStatus = "ready"
                glmNote = ""
        }

        glmModels := []string{}
        if s.glm != nil {
                glmModels = s.glm.FetchModels()
        }

        providers = append(providers, ProviderStatus{
                Name:         "GLM (Z.AI)",
                Status:       glmStatus,
                Note:         glmNote,
                Uptime:       formatUptime(time.Since(s.startTime)),
                AccountCount: len(glmAccounts),
                ActiveCount:  glmActive,
                TotalReqs:    glmReqs,
                TotalErrors:  glmErrs,
                AvgLatency:   glmLatency,
                Details: map[string]any{
                        "models":     glmModels,
                        "captcha_db": s.cfg.GLMCaptchaDB,
                        "agent_mode": s.cfg.AgentMode,
                },
        })

        // MiMo status
        mimoAccounts := s.accounts.List(ProviderMimo)
        mimoActive := countActiveAccounts(mimoAccounts)
        mimoReqs, mimoErrs, mimoLatency := aggregateAccountStats(mimoAccounts)

        mimoStatus := "unavailable"
        mimoNote := ""
        switch {
        case s.mimoEngine == nil:
                mimoStatus = "unavailable"
                mimoNote = "MiMo sub-engine not initialized."
        case mimoActive == 0:
                mimoStatus = "degraded"
                mimoNote = "MiMo sub-engine ready but no active accounts. Add a MiMo account."
        default:
                mimoStatus = "ready"
                mimoNote = ""
        }

        providers = append(providers, ProviderStatus{
                Name:         "MiMo (Xiaomi)",
                Status:       mimoStatus,
                Note:         mimoNote,
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
