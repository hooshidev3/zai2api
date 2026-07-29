// Package server — Rate Limit API handlers (CRUD).
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleListRateLimits returns all configured rate limits.
func (s *Server) handleListRateLimits(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusOK, gin.H{"rate_limits": []any{}})
		return
	}

	rows, err := s.db.Query("SELECT model, max_rpm, max_tpm, max_context FROM model_rate_limits ORDER BY model")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var limits []RateLimit
	for rows.Next() {
		var l RateLimit
		rows.Scan(&l.Model, &l.MaxRPM, &l.MaxTPM, &l.MaxContext)
		limits = append(limits, l)
	}
	c.JSON(http.StatusOK, gin.H{"rate_limits": limits})
}

// handleSetRateLimit creates or updates a rate limit for a model.
func (s *Server) handleSetRateLimit(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not initialized"})
		return
	}

	model := c.Param("id")
	var req struct {
		MaxRPM     int `json:"max_rpm"`
		MaxTPM     int `json:"max_tpm"`
		MaxContext int `json:"max_context"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO model_rate_limits (model, max_rpm, max_tpm, max_context, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, model, req.MaxRPM, req.MaxTPM, req.MaxContext)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if s.rateLimiter != nil {
		s.rateLimiter.SetLimit(RateLimit{
			Model:      model,
			MaxRPM:     req.MaxRPM,
			MaxTPM:     req.MaxTPM,
			MaxContext: req.MaxContext,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"model": model,
		"rate_limit": gin.H{
			"max_rpm":     req.MaxRPM,
			"max_tpm":     req.MaxTPM,
			"max_context": req.MaxContext,
		},
	})
}

// handleDeleteRateLimit removes a rate limit.
func (s *Server) handleDeleteRateLimit(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not initialized"})
		return
	}

	model := c.Param("id")
	s.db.Exec("DELETE FROM model_rate_limits WHERE model = ?", model)
	if s.rateLimiter != nil {
		s.rateLimiter.RemoveLimit(model)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": model})
}
