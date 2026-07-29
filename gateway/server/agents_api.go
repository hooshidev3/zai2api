// Package server — Agents API (proxy to MiMo agent endpoints).
package server

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleListAgents lists active MiMo agents by proxying to the sub-engine.
// If MiMo doesn't have agent endpoints, returns an empty list.
func (s *Server) handleListAgents(c *gin.Context) {
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
	c.Request.URL.Path = "/v1/agent/run"
	s.mimoEngine.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

// handleAgentStream proxies SSE stream for an agent.
func (s *Server) handleAgentStream(c *gin.Context) {
	id := c.Param("id")
	c.Request.URL.Path = "/v1/agent/stream/" + id
	s.mimoEngine.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}
