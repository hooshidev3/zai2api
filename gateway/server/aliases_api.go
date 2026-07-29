// Package server — Model Aliases API (CRUD + resolution).
package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Alias maps an alias name to a target model.
type Alias struct {
	Alias       string    `json:"alias"`
	TargetModel string    `json:"target_model"`
	CreatedAt   time.Time `json:"created_at"`
}

// handleListAliases returns all model aliases from DB.
func (s *Server) handleListAliases(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusOK, gin.H{"aliases": []any{}})
		return
	}

	rows, err := s.db.Query("SELECT alias, target_model, created_at FROM model_aliases ORDER BY alias")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var aliases []Alias
	for rows.Next() {
		var a Alias
		rows.Scan(&a.Alias, &a.TargetModel, &a.CreatedAt)
		aliases = append(aliases, a)
	}
	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

// handleAddAlias creates or updates an alias.
func (s *Server) handleAddAlias(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Alias       string `json:"alias" binding:"required"`
		TargetModel string `json:"target_model" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO model_aliases (alias, target_model) VALUES (?, ?)",
		req.Alias, req.TargetModel,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update in-memory cache
	s.aliases[req.Alias] = req.TargetModel

	c.JSON(http.StatusCreated, gin.H{"alias": req.Alias, "target_model": req.TargetModel})
}

// handleDeleteAlias removes an alias.
func (s *Server) handleDeleteAlias(c *gin.Context) {
	if s.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not initialized"})
		return
	}

	alias := c.Param("name")
	_, err := s.db.Exec("DELETE FROM model_aliases WHERE alias = ?", alias)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	delete(s.aliases, alias)
	c.JSON(http.StatusOK, gin.H{"deleted": alias})
}

// resolveAlias returns the target model for an alias, or the input if no alias exists.
func (s *Server) resolveAlias(name string) string {
	if s.aliases == nil {
		return name
	}
	if target, ok := s.aliases[name]; ok {
		return target
	}
	return name
}

// loadAliasesFromDB loads all aliases into memory at startup.
func (s *Server) loadAliasesFromDB() {
	if s.db == nil {
		return
	}
	rows, err := s.db.Query("SELECT alias, target_model FROM model_aliases")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var alias, target string
		rows.Scan(&alias, &target)
		s.aliases[alias] = target
	}
}
