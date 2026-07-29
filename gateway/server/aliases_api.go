// Package server — Model Aliases API (CRUD + resolution).
// All map access is protected by s.aliasesMu (RWMutex) to prevent data races.
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
                if err := rows.Scan(&a.Alias, &a.TargetModel, &a.CreatedAt); err != nil {
                        continue
                }
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

        // Update in-memory cache (thread-safe, defer for panic safety)
        s.aliasesMu.Lock()
        defer s.aliasesMu.Unlock()
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
        if _, err := s.db.Exec("DELETE FROM model_aliases WHERE alias = ?", alias); err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
                return
        }

        // Remove from in-memory cache (thread-safe, defer for panic safety)
        s.aliasesMu.Lock()
        defer s.aliasesMu.Unlock()
        delete(s.aliases, alias)

        c.JSON(http.StatusOK, gin.H{"deleted": alias})
}

// resolveAlias returns the target model for an alias, or the input if no alias exists.
// Thread-safe via aliasesMu.RLock.
func (s *Server) resolveAlias(name string) string {
        s.aliasesMu.RLock()
        defer s.aliasesMu.RUnlock()
        if s.aliases == nil {
                return name
        }
        if target, ok := s.aliases[name]; ok {
                return target
        }
        return name
}

// loadAliasesFromDB loads all aliases into memory at startup.
// Called once during NewServer before any concurrent access.
func (s *Server) loadAliasesFromDB() {
        if s.db == nil {
                return
        }
        rows, err := s.db.Query("SELECT alias, target_model FROM model_aliases")
        if err != nil {
                return
        }
        defer rows.Close()

        s.aliasesMu.Lock()
        defer s.aliasesMu.Unlock()
        for rows.Next() {
                var alias, target string
                if err := rows.Scan(&alias, &target); err != nil {
                        continue
                }
                s.aliases[alias] = target
        }
}
