// Package server — Account management HTTP handlers (CRUD + test + export).
package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// handleListAccounts returns all accounts (masked tokens).
func (s *Server) handleListAccounts(c *gin.Context) {
	provider := ProviderType(c.Query("provider"))
	accounts := s.accounts.List(provider)

	var dtos []AccountDTO
	for _, a := range accounts {
		dtos = append(dtos, a.ToDTO())
	}
	c.JSON(http.StatusOK, gin.H{
		"accounts": dtos,
		"count":    len(dtos),
	})
}

// handleGetAccount returns a single account by ID.
func (s *Server) handleGetAccount(c *gin.Context) {
	id := c.Param("id")
	a, ok := s.accounts.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found", "message": "account not found"}})
		return
	}
	c.JSON(http.StatusOK, a.ToDTO())
}

// handleCreateAccount creates a new account.
func (s *Server) handleCreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request", "message": err.Error()}})
		return
	}

	acct, err := s.accounts.Create(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "validation_error", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusCreated, acct.ToDTO())
}

// handleUpdateAccount updates an existing account.
func (s *Server) handleUpdateAccount(c *gin.Context) {
	id := c.Param("id")
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request", "message": err.Error()}})
		return
	}

	acct, err := s.accounts.Update(id, req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, acct.ToDTO())
}

// handleDeleteAccount deletes an account.
func (s *Server) handleDeleteAccount(c *gin.Context) {
	id := c.Param("id")
	if err := s.accounts.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// handleToggleAccount enables or disables an account.
func (s *Server) handleToggleAccount(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request", "message": err.Error()}})
		return
	}

	if err := s.accounts.Toggle(id, req.Enabled); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "enabled": req.Enabled})
}

// handleTestAccount tests an account's proxy and provider connection.
func (s *Server) handleTestAccount(c *gin.Context) {
	id := c.Param("id")
	result, err := s.accounts.TestConnection(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, result)
}

// handleExportAccounts exports all accounts with full tokens (requires EXPORT_PASSWORD).
func (s *Server) handleExportAccounts(c *gin.Context) {
	exportPassword := os.Getenv("EXPORT_PASSWORD")
	if exportPassword == "" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "export_disabled",
				"message": "Export disabled. Set EXPORT_PASSWORD env var to enable.",
			},
		})
		return
	}

	confirmToken := c.GetHeader("X-Confirm-Token")
	if subtle.ConstantTimeCompare([]byte(confirmToken), []byte(exportPassword)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"type":    "confirmation_required",
				"message": "Provide X-Confirm-Token header with EXPORT_PASSWORD to export",
			},
		})
		return
	}

	accounts := s.accounts.List("")
	var export []AccountExportDTO
	for _, a := range accounts {
		export = append(export, AccountExportDTO{
			ID:           a.ID,
			Provider:     a.Provider,
			DisplayName:  a.DisplayName,
			ZaiToken:     a.ZaiToken,
			ServiceToken: a.ServiceToken,
			UserID:       a.UserID,
			XiaomichatPH: a.XiaomichatPH,
			Proxy:        a.Proxy,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"exported_at": time.Now().UTC(),
		"count":       len(export),
		"accounts":    export,
	})
}
