// Package server — StatsCollector and SSE live stats stream.
package server

import (
        "encoding/json"
        "fmt"
        "sync"
        "time"

        "github.com/gin-gonic/gin"
)

// RequestRecord holds info about a single API request for the dashboard.
type RequestRecord struct {
        Time     time.Time `json:"time"`
        Provider string    `json:"provider"`
        Model    string    `json:"model"`
        Account  string    `json:"account"`
        Tokens   int       `json:"tokens"`
        Duration int64     `json:"duration_ms"`
        Status   int       `json:"status"`
        Error    string    `json:"error,omitempty"`
}

// StatsCollector tracks aggregate and per-request stats in memory.
type StatsCollector struct {
        mu            sync.RWMutex
        totalRequests int64
        totalErrors   int64
        totalLatency  int64
        recent        []RequestRecord
        maxRecent     int
}

// NewStatsCollector creates a collector that keeps the last 1000 requests.
func NewStatsCollector() *StatsCollector {
        return &StatsCollector{maxRecent: 1000}
}

// Record adds a request to the stats.
func (sc *StatsCollector) Record(rec RequestRecord) {
        sc.mu.Lock()
        defer sc.mu.Unlock()
        sc.totalRequests++
        sc.totalLatency += rec.Duration
        if rec.Status >= 400 || rec.Error != "" {
                sc.totalErrors++
        }
        sc.recent = append(sc.recent, rec)
        if len(sc.recent) > sc.maxRecent {
                sc.recent = sc.recent[len(sc.recent)-sc.maxRecent:]
        }
}

// Middleware records stats for every request that passes through.
func (sc *StatsCollector) Middleware() gin.HandlerFunc {
        return func(c *gin.Context) {
                start := time.Now()
                c.Next()
                sc.Record(RequestRecord{
                        Time:     start,
                        Provider: c.GetString("provider"),
                        Model:    c.GetString("model"),
                        Account:  c.GetString("account_id"),
                        Duration: time.Since(start).Milliseconds(),
                        Status:   c.Writer.Status(),
                })
        }
}

// StatsSnapshot is the JSON sent via SSE to the dashboard.
type StatsSnapshot struct {
        Timestamp  string          `json:"timestamp"`
        KPIs       map[string]any  `json:"kpis"`
        Accounts   map[string]any  `json:"accounts"`
        RecentReqs []RequestRecord `json:"recent_requests"`
}

// handleStatsStream sends live stats every 2 seconds via SSE.
// Uses Context().Done() (not deprecated CloseNotify) for client disconnect.
func (s *Server) handleStatsStream(c *gin.Context) {
        c.Header("Content-Type", "text/event-stream")
        c.Header("Cache-Control", "no-cache")
        c.Header("Connection", "keep-alive")
        c.Header("X-Accel-Buffering", "no") // for nginx

        ticker := time.NewTicker(2 * time.Second)
        defer ticker.Stop()

        ctx := c.Request.Context()
        for {
                select {
                case <-ticker.C:
                        stats := s.collectStats()
                        data, _ := json.Marshal(stats)
                        fmt.Fprintf(c.Writer, "event: stats\ndata: %s\n\n", data)
                        c.Writer.Flush()
                case <-ctx.Done():
                        return
                }
        }
}

// collectStats builds a snapshot of current stats for the dashboard.
func (s *Server) collectStats() StatsSnapshot {
        sc := s.stats
        sc.mu.RLock()
        totalReqs := sc.totalRequests
        totalErrs := sc.totalErrors
        avgLatency := int64(0)
        if totalReqs > 0 {
                avgLatency = sc.totalLatency / totalReqs
        }
        recent := make([]RequestRecord, len(sc.recent))
        copy(recent, sc.recent)
        sc.mu.RUnlock()

        // Reverse (newest first) and limit to 50
        for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
                recent[i], recent[j] = recent[j], recent[i]
        }
        if len(recent) > 50 {
                recent = recent[:50]
        }

        return StatsSnapshot{
                Timestamp: time.Now().UTC().Format(time.RFC3339),
                KPIs: map[string]any{
                        "total_requests": totalReqs,
                        "total_errors":   totalErrs,
                        "avg_latency_ms": avgLatency,
                        "uptime_seconds": int(time.Since(s.startTime).Seconds()),
                },
                Accounts:   s.accountsSnapshot(),
                RecentReqs: recent,
        }
}

// accountsSnapshot returns a JSON-safe snapshot of all accounts.
func (s *Server) accountsSnapshot() map[string]any {
        if s.accounts == nil {
                return map[string]any{"glm": []any{}, "mimo": []any{}}
        }
        accounts := s.accounts.List("")
        var glm, mimo []map[string]any
        for _, a := range accounts {
                item := map[string]any{
                        "id":           a.ID,
                        "display_name": a.DisplayName,
                        "enabled":      a.Enabled.Load(),
                        "req_count":    a.Stats.ReqCount.Load(),
                        "err_count":    a.Stats.ErrCount.Load(),
                        "avg_latency":  a.Stats.AvgLatency.Load(),
                        "has_proxy":    a.Proxy != nil,
                }
                if a.Provider == ProviderGLM {
                        glm = append(glm, item)
                } else {
                        mimo = append(mimo, item)
                }
        }
        return map[string]any{"glm": glm, "mimo": mimo}
}
