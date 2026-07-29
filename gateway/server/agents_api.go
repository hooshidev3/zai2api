// Package server — Agents API (proxy to MiMo agent endpoints).
//
// Status code convention when s.mimoEngine is nil:
//   - handleListAgents: returns 200 with empty list (graceful — "no agents")
//   - handleAgentStatus/Run/Stream: returns 503 (these require a working engine)
package server

import (
        "encoding/json"
        "net/http"

        "github.com/gin-gonic/gin"
)

// handleListAgents lists active MiMo agents by proxying to the sub-engine.
// If MiMo doesn't have agent endpoints, returns an empty list.
func (s *Server) handleListAgents(c *gin.Context) {
        if s.mimoEngine == nil {
                c.JSON(http.StatusOK, gin.H{"agents": []any{}})
                return
        }
        resp, err := s.forwardToMiMoAndCapture(c, "/v1/agent/status")
        if err != nil || len(resp) == 0 {
                c.JSON(http.StatusOK, gin.H{"agents": []any{}})
                return
        }

        var result map[string]any
        if json.Unmarshal(resp, &result) != nil {
                c.JSON(http.StatusOK, gin.H{"agents": []any{}})
                return
        }
        c.JSON(http.StatusOK, result)
}

// handleAgentStatus returns the status of a single agent.
func (s *Server) handleAgentStatus(c *gin.Context) {
        if s.mimoEngine == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MiMo engine not available"})
                return
        }
        id := c.Param("id")
        resp, err := s.forwardToMiMoAndCapture(c, "/v1/agent/status/"+id)
        if err != nil {
                c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
                return
        }
        c.Data(http.StatusOK, "application/json", resp)
}

// handleRunAgent starts a new MiMo agent by forwarding to the sub-engine.
func (s *Server) handleRunAgent(c *gin.Context) {
        if s.mimoEngine == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MiMo engine not available"})
                return
        }
        c.Request.URL.Path = "/v1/agent/run"
        s.mimoEngine.ServeHTTP(c.Writer, c.Request)
        c.Abort()
}

// handleAgentStream proxies SSE stream for an agent.
func (s *Server) handleAgentStream(c *gin.Context) {
        if s.mimoEngine == nil {
                c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MiMo engine not available"})
                return
        }
        id := c.Param("id")
        c.Request.URL.Path = "/v1/agent/stream/" + id
        s.mimoEngine.ServeHTTP(c.Writer, c.Request)
        c.Abort()
}
